//go:build linux

package zcode

import (
	"os"
	"strings"
)

// osRelease reads the kernel release ZCode's X-Os-Version header carries.
func osRelease() string {
	raw, errRead := os.ReadFile("/proc/sys/kernel/osrelease")
	if errRead != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
