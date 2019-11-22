// +build windows

package tail

import (
	"github.com/nxadm/tail/winfile"
	"os"
)

func openFile(name string) (file *os.File, err error) {
	return winfile.OpenFile(name, os.O_RDONLY, 0)
}
