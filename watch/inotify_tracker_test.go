package watch

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// assertNoEntries checks that the shared tracker holds no entries for fname.
// RemoveWatch only returns after removeWatch has run on the tracker goroutine.
func assertNoEntries(t *testing.T, fname string) {
	t.Helper()
	fname = filepath.Clean(fname)
	shared.mux.Lock()
	_, c := shared.chans[fname]
	_, d := shared.done[fname]
	_, n := shared.watchNums[fname]
	shared.mux.Unlock()
	if c || d || n {
		t.Fatalf("tracker still holds %s: chans=%v done=%v watchNums=%v", fname, c, d, n)
	}
}

// tempDir returns a new directory; the caller removes it.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := ioutil.TempDir("", "tail-watch-test")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeFile creates an empty file in a new directory; the caller removes
// filepath.Dir of the result.
func writeFile(t *testing.T, name string) string {
	t.Helper()
	f := filepath.Join(tempDir(t), name)
	if err := ioutil.WriteFile(f, nil, 0644); err != nil {
		t.Fatal(err)
	}
	return f
}

// Tail.Stop makes the change-events goroutine call RemoveWatch, and a later
// Tail.Cleanup calls it again. The second call must not leave a -1 counter
// behind: a program that tails many distinct files gains one entry per file.
func TestRemoveWatchTwice(t *testing.T) {
	f := writeFile(t, "a.log")
	defer os.RemoveAll(filepath.Dir(f))
	if err := Watch(f); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWatch(f); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWatch(f); err != nil {
		t.Fatalf("extra RemoveWatch: %v", err)
	}
	assertNoEntries(t, f)
}

// RemoveWatch after a failed Watch (file already gone) must clean up the
// channels Watch created and must not leave a negative counter.
func TestRemoveWatchAfterFailedWatch(t *testing.T) {
	dir := tempDir(t)
	defer os.RemoveAll(dir)
	f := filepath.Join(dir, "gone.log")
	if err := Watch(f); err == nil {
		t.Fatal("expected Watch on a missing file to fail")
	}
	if err := RemoveWatch(f); err != nil {
		t.Fatalf("RemoveWatch after failed Watch: %v", err)
	}
	assertNoEntries(t, f)
}

// More removals than watches on the same file still leave nothing behind.
func TestRemoveWatchMoreThanWatched(t *testing.T) {
	f := writeFile(t, "b.log")
	defer os.RemoveAll(filepath.Dir(f))
	for i := 0; i < 2; i++ {
		if err := Watch(f); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := RemoveWatch(f); err != nil {
			t.Fatalf("RemoveWatch #%d: %v", i+1, err)
		}
	}
	assertNoEntries(t, f)
}

// Issue hpcloud/tail#153: after an extra RemoveWatch the counter went
// negative, so the next Watch skipped adding the inotify watch and the new
// watcher never received events.
func TestWatchAgainAfterExtraRemove(t *testing.T) {
	f := writeFile(t, "c.log")
	defer os.RemoveAll(filepath.Dir(f))
	if err := Watch(f); err != nil {
		t.Fatal(err)
	}
	RemoveWatch(f)
	RemoveWatch(f)

	if err := Watch(f); err != nil {
		t.Fatal(err)
	}
	defer RemoveWatch(f)
	events := Events(f)
	if err := ioutil.WriteFile(f, []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-events:
	case <-time.After(2 * time.Second):
		t.Fatal("no event after re-watching the file")
	}
}
