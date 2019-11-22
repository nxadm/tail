// Copyright (c) 2015 HPE Software Inc. All rights reserved.
// Copyright (c) 2013 ActiveState Software Inc. All rights reserved.

package tail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nxadm/tail/ratelimiter"
)


var errStop = errors.New("tail should now stop")
var errStopAtEOF = errors.New("tail: stop at eof")

var (
	// DefaultLogger is used when Config.Logger == nil
	DefaultLogger = log.New(os.Stderr, "", log.LstdFlags)
	// DiscardingLogger can be used to disable logging output
	DiscardingLogger = log.New(ioutil.Discard, "", 0)
)

type Line struct {
	Text     string
	Num      int
	SeekInfo SeekInfo
	Time     time.Time
	Err      error // Error from tail
}

//// NewLine returns a Line with present time.
//func NewLine(text string, lineNum int) *Line {
//	return &Line{text, lineNum, SeekInfo{}, time.Now(), nil}
//}

// SeekInfo represents arguments to `io.Seek`
type SeekInfo struct {
	Offset int64
	Whence int // io.Seek*
}

type logger interface {
	Fatal(v ...interface{})
	Fatalf(format string, v ...interface{})
	Fatalln(v ...interface{})
	Panic(v ...interface{})
	Panicf(format string, v ...interface{})
	Panicln(v ...interface{})
	Print(v ...interface{})
	Printf(format string, v ...interface{})
	Println(v ...interface{})
}

// Config is used to specify how a file must be tailed.
type Config struct {
	// File-specifc
	Location    *SeekInfo // Seek to this location before tailing
	ReOpen      bool      // Reopen recreated files (tail -F)
	MustExist   bool      // Fail early if the file does not exist
	Poll        bool      // Poll for file changes instead of using inotify
	Pipe        bool      // Is a named pipe (mkfifo)
	RateLimiter *ratelimiter.LeakyBucket

	// Generic IO
	Follow      bool // Continue looking for new lines (tail -f)
	MaxLineSize int  // If non-zero, split longer lines into multiple lines

	// Logger, when nil, is set to tail.DefaultLogger
	// To disable logging: set field to tail.DiscardingLogger
	Logger logger
}

type Tail struct {
	Filename string
	Lines    chan *Line
	Config

	file    *os.File
	reader  *bufio.Reader
	lineNum int

	watcher fileWatcher
	changes *fileChanges

	context.Context
	context.CancelFunc

	lk sync.Mutex
}



// TailFile begins tailing the file. Output stream is made available
// via the `Tail.Lines` channel. To handle errors during tailing,
// invoke the `Wait` or `Err` method after finishing reading from the
// `Lines` channel.
func TailFile(filename string, config Config) (*Tail, error) {
	if config.ReOpen && !config.Follow {
		fatal("cannot set ReOpen without Follow.")
	}

	ctx, cancel := context.WithCancel(context.Background())
	t := &Tail{
		Filename: filename,
		Lines:    make(chan *Line),
		Config:   config,
	}
	t.Context = ctx
	t.CancelFunc = cancel

	// when Logger was not specified in config, use default trackerLogger
	if t.Logger == nil {
		t.Logger = DefaultLogger
	}

	if t.Poll {
		t.watcher = newPollingFileWatcher(filename)
	} else {
		t.watcher = newInotifyFileWatcher(filename)
	}

	if t.MustExist {
		var err error
		t.file, err = openFile(t.Filename)
		if err != nil {
			return nil, err
		}
	}

	go t.tailFileSync()

	return t, nil
}

// Tell returns the file's current position, like stdio's ftell().
// But this value is not very accurate.
// One line from the chan(tail.Lines) may have been read,
// so it may have lost one line.
func (tail *Tail) Tell() (offset int64, err error) {
	if tail.file == nil {
		return
	}
	offset, err = tail.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return
	}

	tail.lk.Lock()
	defer tail.lk.Unlock()
	if tail.reader == nil {
		return
	}

	offset -= int64(tail.reader.Buffered())
	return
}

// Stop stops the tailing activity.
func (tail *Tail) Stop() error {
	tail.CancelFunc()
	<-tail.Context.Done()
	return nil
}

// StopAtEOF stops tailing as soon as the end of the file is reached.
func (tail *Tail) StopAtEOF() error {
	tail.CancelFunc()
	return errStopAtEOF
}


func (tail *Tail) close() {
	close(tail.Lines)
	tail.closeFile()
}

func (tail *Tail) closeFile() {
	if tail.file != nil {
		tail.file.Close()
		tail.file = nil
	}
}

func (tail *Tail) reopen() error {
	tail.closeFile()
	tail.lineNum = 0
	for {
		var err error
		tail.file, err = openFile(tail.Filename)
		if err != nil {
			if os.IsNotExist(err) {
				tail.Logger.Printf("Waiting for %s to appear...", tail.Filename)
				if err := tail.watcher.blockUntilExists(tail.Context); err != nil {
					return fmt.Errorf("Failed to detect creation of %s: %s", tail.Filename, err)
				}
				continue
			}
			return fmt.Errorf("Unable to open file %s: %s", tail.Filename, err)
		}
		break
	}
	return nil
}

func (tail *Tail) readLine() (string, error) {
	tail.lk.Lock()
	line, err := tail.reader.ReadString('\n')
	tail.lk.Unlock()
	if err != nil {
		// Note ReadString "returns the data read before the error" in
		// case of an error, including EOF, so we return it as is. The
		// caller is expected to process it if err is EOF.
		return line, err
	}

	line = strings.TrimRight(line, "\n")

	return line, err
}

func (tail *Tail) tailFileSync() {
	defer tail.Done()
	defer tail.close()

	if !tail.MustExist {
		// deferred first open.
		err := tail.reopen()
		if err != nil {
			return
		}
	}

	// Seek to requested location on first open of the file.
	if tail.Location != nil {
		_, err := tail.file.Seek(tail.Location.Offset, tail.Location.Whence)
		if err != nil {
			tail.CancelFunc()
			fmt.Errorf("Seek error on %s: %s", tail.Filename, err)
			return
		}
	}

	tail.openReader()

	// Read line by line.
	for {
		// do not seek in named pipes
		if !tail.Pipe {
			// grab the position in case we need to back up in the event of a half-line
			if _, err := tail.Tell(); err != nil {
				tail.CancelFunc()
				fmt.Errorf("%s", err)
				return
			}
		}

		line, err := tail.readLine()

		// Process `line` even if err is EOF.
		if err == nil {
			cooloff := !tail.sendLine(line)
			if cooloff {
				// Wait a second before seeking till the end of
				// file when rate limit is reached.
				msg := ("Too much log activity; waiting a second before resuming tailing")
				offset, _ := tail.Tell()
				tail.Lines <- &Line{msg, tail.lineNum, SeekInfo{Offset: offset}, time.Now(), errors.New(msg)}
				select {
				case <-time.After(time.Second):
				case <-tail.Done():
					return
				}
				if err := tail.seekEnd(); err != nil {
					tail.CancelFunc()
					fmt.Errorf("%s", err)
					return
				}
			}
		} else if err == io.EOF {
			if !tail.Follow {
				if line != "" {
					tail.sendLine(line)
				}
				return
			}

			if tail.Follow && line != "" {
				tail.sendLine(line)
				if err := tail.seekEnd(); err != nil {
					tail.CancelFunc()
					fmt.Errorf("%s", err)
					return
				}
			}

			// When EOF is reached, wait for more data to become
			// available. Wait strategy is based on the `tail.watcher`
			// implementation (inotify or polling).
			err := tail.waitForChanges()
			if err != nil {
				if err != errStop {
					tail.CancelFunc()
					fmt.Errorf("%s", err)
				}
				return
			}
		} else {
			// non-EOF error
			tail.CancelFunc()
			fmt.Errorf("Error reading %s: %s", tail.Filename, err)
			return
		}

		select {
		case <-tail.Done():
			if tail.Err() == errStopAtEOF {
				continue
			}
			return
		default:
		}
	}
}

// waitForChanges waits until the file has been appended, deleted,
// moved or truncated. When moved or deleted - the file will be
// reopened if ReOpen is true. Truncated files are always reopened.
func (tail *Tail) waitForChanges() error {
	if tail.changes == nil {
		pos, err := tail.file.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		tail.changes, err = tail.watcher.changeEvents(tail.Context, pos)
		if err != nil {
			return err
		}
	}

	select {
	case <-tail.changes.modified:
		return nil
	case <-tail.changes.deleted:
		tail.changes = nil
		if tail.ReOpen {
			// XXX: we must not log from a library.
			tail.Logger.Printf("Re-opening moved/deleted file %s ...", tail.Filename)
			if err := tail.reopen(); err != nil {
				return err
			}
			tail.Logger.Printf("Successfully reopened %s", tail.Filename)
			tail.openReader()
			return nil
		}
		tail.Logger.Printf("Stopping tail as file no longer exists: %s", tail.Filename)
		return errStop
	case <-tail.changes.Truncated:
		// Always reopen truncated files (Follow is true)
		tail.Logger.Printf("Re-opening truncated file %s ...", tail.Filename)
		if err := tail.reopen(); err != nil {
			return err
		}
		tail.Logger.Printf("Successfully reopened truncated %s", tail.Filename)
		tail.openReader()
		return nil
	case <-tail.Done():
		return errStop
	}
}

func (tail *Tail) openReader() {
	tail.lk.Lock()
	if tail.MaxLineSize > 0 {
		// add 2 to account for newline characters
		tail.reader = bufio.NewReaderSize(tail.file, tail.MaxLineSize+2)
	} else {
		tail.reader = bufio.NewReader(tail.file)
	}
	tail.lk.Unlock()
}

func (tail *Tail) seekEnd() error {
	return tail.seekTo(SeekInfo{Offset: 0, Whence: io.SeekEnd})
}

func (tail *Tail) seekTo(pos SeekInfo) error {
	_, err := tail.file.Seek(pos.Offset, pos.Whence)
	if err != nil {
		return fmt.Errorf("Seek error on %s: %s", tail.Filename, err)
	}
	// Reset the read buffer whenever the file is re-seek'ed
	tail.reader.Reset(tail.file)
	return nil
}

// sendLine sends the line(s) to Lines channel, splitting longer lines
// if necessary. Return false if rate limit is reached.
func (tail *Tail) sendLine(line string) bool {
	now := time.Now()
	lines := []string{line}

	// Split longer lines
	if tail.MaxLineSize > 0 && len(line) > tail.MaxLineSize {
		lines = partitionString(line, tail.MaxLineSize)
	}

	for _, line := range lines {
		tail.lineNum++
		offset, _ := tail.Tell()
		select {
		case tail.Lines <- &Line{line, tail.lineNum, SeekInfo{Offset: offset}, now, nil}:
		case <-tail.Done():
			return true
		}
	}

	if tail.Config.RateLimiter != nil {
		ok := tail.Config.RateLimiter.Pour(uint16(len(lines)))
		if !ok {
			tail.Logger.Printf("Leaky bucket full (%v); entering 1s cooloff period.",
				tail.Filename)
			return false
		}
	}

	return true
}

// cleanup removes inotify watches added by the tail package. This function is
// meant to be invoked from a process's exit handler. Linux kernel may not
// automatically remove inotify watches after the process exits.
func (tail *Tail) Cleanup() {
	cleanup(tail.Filename)
}
