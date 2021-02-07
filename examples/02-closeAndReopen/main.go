// Tail a file, print its contents, close it and reopen it.
// In this example you can add lines to the syslog log by using the logger
// command. Exit with Ctrl+C.
package main

import (
	"fmt"
	"time"

	"github.com/nxadm/tail"
)

var logFile = "/var/log/syslog"

func main() {
	// Open the file
	t, err := tail.TailFile(logFile, tail.Config{Follow: true})
	if err != nil {
		panic(err)
	}

	go func() {
		for line := range t.Lines {
			fmt.Println(line.Text)
		}
	}()

	time.Sleep(time.Second * 5) // Give time to the go routine to print stuff
	fmt.Println("Closing the logfile " + logFile)
	err = t.Stop()
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
	}
	fmt.Println("Closed the logfile " + logFile)
	// If you plan to reread the same file, do not call Cleanup() as inotify/Linux will get confused.
	// As the documentation states: "This function is meant to be invoked from a process's exit handler".
	//t.Cleanup()

	// Reopen the file and print it
	t, err = tail.TailFile(logFile, tail.Config{Follow: true})
	if err != nil {
		panic(err)
	}
	defer t.Cleanup()

	for line := range t.Lines {
		fmt.Println(line.Text)
	}
}
