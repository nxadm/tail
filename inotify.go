// Copyright (c) 2015 HPE Software Inc. All rights reserved.
// Copyright (c) 2013 ActiveState Software Inc. All rights reserved.

package tail

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// inotifyFileWatcher uses inotify to monitor file changes.
type inotifyFileWatcher struct {
	filename string
	size     int64
}

func newInotifyFileWatcher(filename string) *inotifyFileWatcher {
	fw := &inotifyFileWatcher{filepath.Clean(filename), 0}
	return fw
}

func (fw *inotifyFileWatcher) blockUntilExists(ctx context.Context) error {
	err := watchCreate(fw.filename)
	if err != nil {
		return err
	}
	defer removeWatchCreate(fw.filename)

	// Do a real check now as the file might have been created before
	// calling `WatchFlags` above.
	if _, err = os.Stat(fw.filename); !os.IsNotExist(err) {
		// file exists, or stat returned an error.
		return err
	}

	events := events(fw.filename)

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
			fwFilename, err := filepath.Abs(fw.filename)
			if err != nil {
				return err
			}
			if evtName == fwFilename {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (fw *inotifyFileWatcher) changeEvents(ctx context.Context, pos int64) (*fileChanges, error) {
	err := startWatch(fw.filename)
	if err != nil {
		return nil, err
	}

	changes := newFileChanges()
	fw.size = pos

	go func() {

		events := events(fw.filename)

		for {
			prevSize := fw.size

			var evt fsnotify.Event
			var ok bool

			select {
			case evt, ok = <-events:
				if !ok {
					removeWatch(fw.filename)
					return
				}
			case <-ctx.Done():
				removeWatch(fw.filename)
				return
			}

			switch {
			case evt.Op&fsnotify.Remove == fsnotify.Remove:
				fallthrough

			case evt.Op&fsnotify.Rename == fsnotify.Rename:
				removeWatch(fw.filename)
				changes.notifyDeleted()
				return

			//With an open fd, unlink(fd) - inotify returns IN_ATTRIB (==fsnotify.Chmod)
			case evt.Op&fsnotify.Chmod == fsnotify.Chmod:
				fallthrough

			case evt.Op&fsnotify.Write == fsnotify.Write:
				fi, err := os.Stat(fw.filename)
				if err != nil {
					if os.IsNotExist(err) {
						removeWatch(fw.filename)
						changes.notifyDeleted()
						return
					}
					// XXX: report this error back to the user
					fatal("Failed to stat file %v: %v", fw.filename, err)
				}
				fw.size = fi.Size()

				if prevSize > 0 && prevSize > fw.size {
					changes.notifyTruncated()
				} else {
					changes.notifyModified()
				}
				prevSize = fw.size
			}
		}
	}()

	return changes, nil
}
