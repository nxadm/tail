// Copyright (c) 2015 HPE Software Inc. All rights reserved.
// Copyright (c) 2013 ActiveState Software Inc. All rights reserved.

package tail

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/fsnotify/fsnotify"
)

type inotifyTracker struct {
	mux       sync.Mutex
	watcher   *fsnotify.Watcher
	chans     map[string]chan fsnotify.Event
	done      map[string]chan bool
	watchNums map[string]int
	watch     chan *watchInfo
	remove    chan *watchInfo
	error     chan error
}

type watchInfo struct {
	op    fsnotify.Op
	fname string
}

func (w *watchInfo) isCreate() bool {
	return w.op == fsnotify.Create
}

var (
	// globally shared inotifyTracker; ensures only one fsnotify.Watcher is used
	shared *inotifyTracker

	// these are used to ensure the shared inotifyTracker is run exactly once
	once  = sync.Once{}
	goRun = func() {
		shared = &inotifyTracker{
			mux:       sync.Mutex{},
			chans:     make(map[string]chan fsnotify.Event),
			done:      make(map[string]chan bool),
			watchNums: make(map[string]int),
			watch:     make(chan *watchInfo),
			remove:    make(chan *watchInfo),
			error:     make(chan error),
		}
		go shared.run()
	}

	trackerLogger = log.New(os.Stderr, "", log.LstdFlags)
)

// startWatch signals the run goroutine to begin watching the input filename
func startWatch(fname string) error {
	return watch(&watchInfo{
		fname: fname,
	})
}

// watchCreate create signals the run goroutine to begin watching the input filename
// if call the watchCreate function, don't call the cleanup, call the removeWatchCreate
func watchCreate(fname string) error {
	return watch(&watchInfo{
		op:    fsnotify.Create,
		fname: fname,
	})
}

func watch(winfo *watchInfo) error {
	// start running the shared inotifyTracker if not already running
	once.Do(goRun)

	winfo.fname = filepath.Clean(winfo.fname)
	shared.watch <- winfo
	return <-shared.error
}

// removeWatch signals the run goroutine to remove the watch for the input filename
func removeWatch(fname string) error {
	return remove(&watchInfo{
		fname: fname,
	})
}

// removeWatchCreate create signals the run goroutine to remove the watch for the input filename
func removeWatchCreate(fname string) error {
	return remove(&watchInfo{
		op:    fsnotify.Create,
		fname: fname,
	})
}

func remove(winfo *watchInfo) error {
	// start running the shared inotifyTracker if not already running
	once.Do(goRun)

	winfo.fname = filepath.Clean(winfo.fname)
	shared.mux.Lock()
	done := shared.done[winfo.fname]
	if done != nil {
		delete(shared.done, winfo.fname)
		close(done)
	}
	shared.mux.Unlock()

	shared.remove <- winfo
	return <-shared.error
}

// events returns a channel to which FileEvents corresponding to the input filename
// will be sent. This channel will be closed when removeWatch is called on this
// filename.
func events(fname string) <-chan fsnotify.Event {
	shared.mux.Lock()
	defer shared.mux.Unlock()

	return shared.chans[fname]
}

// cleanup removes the watch for the input filename if necessary.
func cleanup(fname string) error {
	return removeWatch(fname)
}

// watchFlags calls fsnotify.WatchFlags for the input filename and flags, creating
// a new Watcher if the previous Watcher was closed.
func (shared *inotifyTracker) addWatch(winfo *watchInfo) error {
	shared.mux.Lock()
	defer shared.mux.Unlock()

	if shared.chans[winfo.fname] == nil {
		shared.chans[winfo.fname] = make(chan fsnotify.Event)
	}
	if shared.done[winfo.fname] == nil {
		shared.done[winfo.fname] = make(chan bool)
	}

	fname := winfo.fname
	if winfo.isCreate() {
		// startWatch for new files to be created in the parent directory.
		fname = filepath.Dir(fname)
	}

	var err error
	// already in inotify watch
	if shared.watchNums[fname] == 0 {
		err = shared.watcher.Add(fname)
	}
	if err == nil {
		shared.watchNums[fname]++
	}
	return err
}

// removeWatch calls fsnotify.removeWatch for the input filename and closes the
// corresponding events channel.
func (shared *inotifyTracker) removeWatch(winfo *watchInfo) error {
	shared.mux.Lock()

	ch := shared.chans[winfo.fname]
	if ch != nil {
		delete(shared.chans, winfo.fname)
		close(ch)
	}

	fname := winfo.fname
	if winfo.isCreate() {
		// startWatch for new files to be created in the parent directory.
		fname = filepath.Dir(fname)
	}
	shared.watchNums[fname]--
	watchNum := shared.watchNums[fname]
	if watchNum == 0 {
		delete(shared.watchNums, fname)
	}
	shared.mux.Unlock()

	var err error
	// If we were the last ones to watch this file, unsubscribe from inotify.
	// This needs to happen after releasing the lock because fsnotify waits
	// synchronously for the kernel to acknowledge the removal of the watch
	// for this file, which causes us to deadlock if we still held the lock.
	if watchNum == 0 {
		err = shared.watcher.Remove(fname)
	}

	return err
}

// sendEvent sends the input event to the appropriate Tail.
func (shared *inotifyTracker) sendEvent(event fsnotify.Event) {
	name := filepath.Clean(event.Name)

	shared.mux.Lock()
	ch := shared.chans[name]
	done := shared.done[name]
	shared.mux.Unlock()

	if ch != nil && done != nil {
		select {
		case ch <- event:
		case <-done:
		}
	}
}

// run starts the goroutine in which the shared struct reads events from its
// Watcher's Event channel and sends the events to the appropriate Tail.
func (shared *inotifyTracker) run() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fatal("failed to create Watcher")
	}
	shared.watcher = watcher

	for {
		select {
		case winfo := <-shared.watch:
			shared.error <- shared.addWatch(winfo)

		case winfo := <-shared.remove:
			shared.error <- shared.removeWatch(winfo)

		case event, open := <-shared.watcher.Events:
			if !open {
				return
			}
			shared.sendEvent(event)

		case err, open := <-shared.watcher.Errors:
			if !open {
				return
			} else if err != nil {
				sysErr, ok := err.(*os.SyscallError)
				if !ok || sysErr.Err != syscall.EINTR {
					trackerLogger.Printf("Error in Watcher Error channel: %s", err)
				}
			}
		}
	}
}
