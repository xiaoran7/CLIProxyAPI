package zcode

import (
	"runtime"
	"time"
)

// platformOS renders the OS token ZCode's headers and prompt use, e.g. "win32".
func platformOS() string {
	switch runtime.GOOS {
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}

// archName renders the architecture token ZCode names, e.g. "x64".
func archName() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "ia32"
	default:
		return runtime.GOARCH
	}
}

// platformName renders the combined platform token, e.g. "win32-x64".
func platformName() string {
	return platformOS() + "-" + archName()
}

// osCategory renders the coarse platform family ZCode names.
func osCategory() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

// osVersion renders the OS release ZCode reports in X-Os-Version.
func osVersion() string {
	if release := osRelease(); release != "" {
		return release
	}
	return runtime.GOOS
}

// promptOSVersion renders the OS line the prompt's environment section names.
func promptOSVersion() string {
	return platformOS() + " " + osVersion() + " " + archName()
}

// clientLocale renders the X-Client-Language value.
func clientLocale() string {
	return "unknown"
}

// clientTimezone renders the X-Client-Timezone value.
func clientTimezone() string {
	if tz := time.Now().Location().String(); tz != "" && tz != "Local" {
		return tz
	}
	return "unknown"
}
