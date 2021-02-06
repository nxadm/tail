// Copyright (c) 2019 nxadm. All rights reserved.
// Copyright (c) 2015 HPE Software Inc. All rights reserved.
// Copyright (c) 2013 ActiveState Software Inc. All rights reserved.

package watch

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/nxadm/tail/util"
	"gopkg.in/tomb.v1"
)

// InotifyFileWatcher uses inotify to monitor file changes.
type InotifyFileWatcher struct {
	Filename string
	Size     int64
}

func NewInotifyFileWatcher(filename string) *InotifyFileWatcher {
	fw := &InotifyFileWatcher{filepath.Clean(filename), 0}
	return fw
}

func (fw *InotifyFileWatcher) BlockUntilExists(t *tomb.Tomb) error {
	err := WatchCreate(fw.Filename)
	if err != nil {
		return err
	}
	defer RemoveWatchCreate(fw.Filename)

	// Do a real check now as the file might have been created before
	// calling `WatchFlags` above.
	if _, err = os.Stat(fw.Filename); !os.IsNotExist(err) {
		// file exists, or stat returned an error.
		return err
	}

	events := Events(fw.Filename)

	for {
		select {
		case evt, ok := <-events:
			if !ok {
				return fmt.Errorf("inotify watcher has been closed")
			}
			evtName, err := filepath.Abs(evt.Name)
			if err != nil {
				return err
			}
			fwFilename, err := filepath.Abs(fw.Filename)
			if err != nil {
				return err
			}
			if evtName == fwFilename {
				return nil
			}
		case <-t.Dying():
			return tomb.ErrDying
		}
	}
	panic("unreachable")
}

func (fw *InotifyFileWatcher) ChangeEvents(t *tomb.Tomb, pos int64) (*FileChanges, error) {
	fmt.Println("entered ChangeEvents") // DEBUG
	err := Watch(fw.Filename)
	if err != nil {
		return nil, err
	}

	changes := NewFileChanges()
	fw.Size = pos

	go func() {
		fmt.Println("entered ChangeEvents goroutine") // DEBUG

		events := Events(fw.Filename)

		for {
			fmt.Println("entered ChangeEvents goroutine watcher loop") // DEBUG
			prevSize := fw.Size

			var evt fsnotify.Event
			var ok bool

			fmt.Println("ChangeEvents goroutine watcher before events") // DEBUG
			select {
			case evt, ok = <-events:
				if !ok {
					RemoveWatch(fw.Filename)
					return
				}
			case <-t.Dying():
				RemoveWatch(fw.Filename)
				return
			}
			fmt.Println("ChangeEvents goroutine watcher after events") // DEBUG

			fmt.Println("ChangeEvents goroutine watcher before events opss") // DEBUG
			switch {
			case evt.Op&fsnotify.Remove == fsnotify.Remove:
				fmt.Println("event REMOVE") // DEBUG
				fallthrough

			case evt.Op&fsnotify.Rename == fsnotify.Rename:
				fmt.Println("event RENAME") // DEBUG
				RemoveWatch(fw.Filename)
				changes.NotifyDeleted()
				return

			//With an open fd, unlink(fd) - inotify returns IN_ATTRIB (==fsnotify.Chmod)
			case evt.Op&fsnotify.Chmod == fsnotify.Chmod:
				fmt.Println("event CHMOD") // DEBUG
				fallthrough

			case evt.Op&fsnotify.Write == fsnotify.Write:
				fmt.Println("event WRITE") // DEBUG
				fi, err := os.Stat(fw.Filename)
				if err != nil {
					if os.IsNotExist(err) {
						RemoveWatch(fw.Filename)
						changes.NotifyDeleted()
						return
					}
					// XXX: report this error back to the user
					util.Fatal("Failed to stat file %v: %v", fw.Filename, err)
				}
				fw.Size = fi.Size()

				if prevSize > 0 && prevSize > fw.Size {
					changes.NotifyTruncated()
				} else {
					changes.NotifyModified()
				}
				prevSize = fw.Size
			}
		}
	}()

	return changes, nil
}
