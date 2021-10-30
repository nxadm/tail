// Tail a file and print its contents in file end location.
// command. Exit with Ctrl+C.
package main

import (
	"flag"
	"fmt"

	"github.com/nxadm/tail"
)

func showInput() {
	for {
		var t string
		fmt.Scanln(&t)
	}
}

func main() {
	go showInput()
	var fileName string
	seek := tail.SeekInfo{
		Offset: 0,
		Whence: 2, // 相对文件结尾
	}
	flag.StringVar(&fileName, "f", "1.txt", "文件路径|file path")
	flag.Parse()
	t, err := tail.TailFile(fileName, tail.Config{Location: &seek, Follow: true})
	if err != nil {
		panic(err)
	}

	for line := range t.Lines {
		fmt.Println(line.Text)
	}
}
