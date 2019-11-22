// +build windows

package tail

import (
	"os"
)

func openFile(name string) (file *os.File, err error) {
	return winfile.OpenFile(name, os.O_RDONLY, 0)
}
