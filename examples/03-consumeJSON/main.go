// Consume and unmarshall JSON.
//
// In this example JSON lines are added by createJSON in a tight loop.
// Each line is unmarshalled and a field printed.
// Exit with Ctrl+C.
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	
	"github.com/nxadm/tail"
)

type jsonStruct struct {
	Counter string `json:"counter"`
}

func main() {
	file, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		panic(err)
	}
	fmt.Println(file.Name())
	defer file.Close()
	defer os.Remove(file.Name())

	t, err := tail.TailFile(file.Name(), tail.Config{Follow: true})
	if err != nil {
		panic(err)
	}

	go createJSON(file)
	var js jsonStruct
	for line := range t.Lines {
		fmt.Printf("JSON: " + line.Text + "\n")

		err := json.Unmarshal([]byte(line.Text), &js)
		if err != nil {
			panic(err)
		}
		fmt.Printf("JSON counter field: " + js.Counter + "\n")
	}
}

func createJSON(file *os.File) {
	var counter int
	for {
		file.WriteString("{ \"counter\": \"" + strconv.Itoa(counter) + "\"}\n")
		counter++
	}
}
