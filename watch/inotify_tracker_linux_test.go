package watch

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// kernelWatches counts the inotify watches this process holds.
func kernelWatches(t *testing.T) int {
	t.Helper()
	fds, err := filepath.Glob("/proc/self/fdinfo/*")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, fd := range fds {
		f, err := os.Open(fd)
		if err != nil {
			continue
		}
		s := bufio.NewScanner(f)
		for s.Scan() {
			if strings.HasPrefix(s.Text(), "inotify wd:") {
				n++
			}
		}
		f.Close()
	}
	return n
}

func waitKernelWatches(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := kernelWatches(t)
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("inotify watches: got %d, want %d", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The kernel watch stays while any caller still holds the file, goes away
// with the last RemoveWatch, and extra calls do not touch other files.
func TestRemoveWatchKernelWatch(t *testing.T) {
	once.Do(goRun)
	base := kernelWatches(t)
	f := writeFile(t, "k.log")
	defer os.RemoveAll(filepath.Dir(f))
	g := writeFile(t, "other.log")
	defer os.RemoveAll(filepath.Dir(g))
	if err := Watch(g); err != nil {
		t.Fatal(err)
	}
	defer RemoveWatch(g)
	for i := 0; i < 2; i++ {
		if err := Watch(f); err != nil {
			t.Fatal(err)
		}
	}
	waitKernelWatches(t, base+2)
	RemoveWatch(f)
	waitKernelWatches(t, base+2)
	RemoveWatch(f)
	waitKernelWatches(t, base+1)
	RemoveWatch(f)
	waitKernelWatches(t, base+1)
}
