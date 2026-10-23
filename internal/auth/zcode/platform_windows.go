//go:build windows

package zcode

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// osRelease reads the OS version ZCode's X-Os-Version header carries.
func osRelease() string {
	info := windows.RtlGetVersion()
	if info == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d", info.MajorVersion, info.MinorVersion, info.BuildNumber)
}
