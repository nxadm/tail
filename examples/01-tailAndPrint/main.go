// Tail a file and print its contents.
//
// In this example you can add lines to the syslog log by using the logger
// command. Exit with Ctrl+C.
package main

import (
	"fmt"

	"github.com/nxadm/tail"
)

var logFile = "/var/log/syslog"

func main() {
	t, err := tail.TailFile(logFile, tail.Config{Follow: true})
	if err != nil {
		panic(err)
	}

	for line := range t.Lines {
		fmt.Println(line.Text)
	}
}
