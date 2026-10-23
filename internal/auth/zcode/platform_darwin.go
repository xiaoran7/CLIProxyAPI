//go:build darwin

package zcode

import (
	"strings"

	"golang.org/x/sys/unix"
)

// osRelease reads the kernel release ZCode's X-Os-Version header carries.
func osRelease() string {
	var name unix.Utsname
	if errUname := unix.Uname(&name); errUname != nil {
		return ""
	}
	return strings.TrimRight(string(name.Release[:]), "\x00")
}
