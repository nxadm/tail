// +build windows

package tail

import (
	"os"
)

func openFile(name string) (file *os.File, err error) {
	return winOpenFile(name, os.O_RDONLY, 0)
}
