//go:build !windows

package jevconnect

import (
	"errors"
	"os"
	"runtime"
)

func browserCommand(target string) (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "/usr/bin/open", []string{target}, nil
	case "linux":
		if isExecutable("/usr/bin/xdg-open") {
			return "/usr/bin/xdg-open", []string{target}, nil
		}
		if isExecutable("/usr/bin/gio") {
			return "/usr/bin/gio", []string{"open", target}, nil
		}
		return "", nil, errors.New("no supported browser launcher")
	default:
		return "", nil, errors.New("unsupported platform")
	}
}

func browserEnvironment(parent []string) []string {
	// xdg-open is a shell script and legitimately searches for desktop tools.
	// Give it a fixed system-only search path, never the caller's PATH.
	return append(allowlistedBrowserEnvironment(parent), "PATH=/usr/bin:/bin")
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}
