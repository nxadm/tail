// Copyright (c) 2015 HPE Software Inc. All rights reserved.
// Copyright (c) 2013 ActiveState Software Inc. All rights reserved.

package tail

import (
	"context"
	"os"
	"runtime"
	"time"
)

// pollingFileWatcher polls the file for changes.
type pollingFileWatcher struct {
	filename string
	size     int64
}

func newPollingFileWatcher(filename string) *pollingFileWatcher {
	fw := &pollingFileWatcher{filename, 0}
	return fw
}

var POLL_DURATION time.Duration

func (fw *pollingFileWatcher) blockUntilExists(ctx context.Context) error {
	for {
		if _, err := os.Stat(fw.filename); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		select {
		case <-time.After(POLL_DURATION):
			continue
		case <-ctx.Done():
			return nil
		}
	}
}

func (fw *pollingFileWatcher) changeEvents(ctx context.Context, pos int64) (*fileChanges, error) {
	origFi, err := os.Stat(fw.filename)
	if err != nil {
		return nil, err
	}

	changes := newFileChanges()
	var prevModTime time.Time

	// XXX: use tomb.Tomb to cleanly manage these goroutines. replace
	// the fatal (below) with tomb's Kill.

	fw.size = pos

	go func() {
		prevSize := fw.size
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			time.Sleep(POLL_DURATION)
			fi, err := os.Stat(fw.filename)
			if err != nil {
				// Windows cannot delete a file if a handle is still open (tail keeps one open)
				// so it gives access denied to anything trying to read it until all handles are released.
				if os.IsNotExist(err) || (runtime.GOOS == "windows" && os.IsPermission(err)) {
					// File does not exist (has been deleted).
					changes.notifyDeleted()
					return
				}

				// XXX: report this error back to the user
				fatal("Failed to stat file %v: %v", fw.filename, err)
			}

			// File got moved/renamed?
			if !os.SameFile(origFi, fi) {
				changes.notifyDeleted()
				return
			}

			// File got truncated?
			fw.size = fi.Size()
			if prevSize > 0 && prevSize > fw.size {
				changes.notifyTruncated()
				prevSize = fw.size
				continue
			}
			// File got bigger?
			if prevSize > 0 && prevSize < fw.size {
				changes.notifyModified()
				prevSize = fw.size
				continue
			}
			prevSize = fw.size

			// File was appended to (changed)?
			modTime := fi.ModTime()
			if modTime != prevModTime {
				prevModTime = modTime
				changes.notifyModified()
			}
		}
	}()

	return changes, nil
}

func init() {
	POLL_DURATION = 250 * time.Millisecond
}
