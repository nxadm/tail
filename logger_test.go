// Copyright (c) 2026 FOSS contributors of https://github.com/nxadm/tail

package tail_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nxadm/tail"
)

type printfLogger func(string, ...interface{})

func (log printfLogger) Printf(format string, args ...interface{}) {
	log(format, args...)
}

func TestPrintfOnlyLogger(t *testing.T) {
	dir, err := ioutil.TempDir("", "tail-printf-logger")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	messages := make(chan string, 1)
	filename := filepath.Join(dir, "not-created.log")
	tailed, err := tail.TailFile(filename, tail.Config{
		Follow: true,
		Logger: printfLogger(func(format string, args ...interface{}) {
			messages <- fmt.Sprintf(format, args...)
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tailed.Cleanup()
	defer tailed.Stop()

	select {
	case message := <-messages:
		if !strings.Contains(message, filename) {
			t.Errorf("log message %q does not contain filename %q", message, filename)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no message received while waiting for the file")
	}
}
