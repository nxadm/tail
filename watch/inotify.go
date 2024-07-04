// Copyright (c) 2019 FOSS contributors of https://github.com/nxadm/tail
// Copyright (c) 2015 HPE Software Inc. All rights reserved.
// Copyright (c) 2013 ActiveState Software Inc. All rights reserved.

package watch

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/nxadm/tail/util"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/tomb.v1"
)

// InotifyFileWatcher uses inotify to monitor file changes.
type InotifyFileWatcher struct {
	Filename string
	Size     int64
	// inodeId record cur watch file id
	inodeId uint64
}

func NewInotifyFileWatcher(filename string) *InotifyFileWatcher {
	fw := &InotifyFileWatcher{filepath.Clean(filename), 0, 0}
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
	err := Watch(fw.Filename)
	if err != nil {
		return nil, err
	}

	changes := NewFileChanges()
	fw.Size = pos
	var stat syscall.Stat_t
	// record file inodeId
	err = syscall.Stat(fw.Filename, &stat)
	if err == nil {
		fw.inodeId = stat.Ino
	}

	go func() {

		events := Events(fw.Filename)

		for {
			prevSize := fw.Size

			var evt fsnotify.Event
			var ok bool

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

			switch {
			case evt.Op&fsnotify.Remove == fsnotify.Remove:
				fallthrough

			case evt.Op&fsnotify.Rename == fsnotify.Rename:
				RemoveWatch(fw.Filename)
				changes.NotifyDeleted()
				return

			//With an open fd, unlink(fd) - inotify returns IN_ATTRIB (==fsnotify.Chmod)
			case evt.Op&fsnotify.Chmod == fsnotify.Chmod:
				fallthrough

			case evt.Op&fsnotify.Write == fsnotify.Write:
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

				// No matter what, it is necessary to notify a write event.
				changes.NotifyModified()

				if prevSize > 0 && prevSize > fw.Size {
					// file change; if file inodeId changed, notify delete event
					if fw.inodeId > 0 && fi.Sys() != nil {
						if statT, fok := fi.Sys().(*syscall.Stat_t); fok {
							if statT.Ino != fw.inodeId {
								RemoveWatch(fw.Filename)
								changes.NotifyDeleted()
								return
							}
						}
					}
					changes.NotifyTruncated()
				}
				prevSize = fw.Size
			}
		}
	}()

	return changes, nil
}
