// Copyright (c) 2019 FOSS contributors of https://github.com/nxadm/tail
// +build !windows

package tail

import (
	"os"
)

// OpenFile proxies a file os.Open so the file can be correctly tailed
// on POSIX and non-POSIX OSes like MS Windows.
//
// This function is only useful internally and as such it will be
// deprecated from the API in a future major release.
func OpenFile(name string) (file *os.File, err error) {
	return os.Open(name)
}
