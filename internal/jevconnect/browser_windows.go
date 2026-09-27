//go:build windows

package jevconnect

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func browserCommand(target string) (string, []string, error) {
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		return "", nil, err
	}
	return filepath.Join(systemDirectory, "rundll32.exe"), []string{"url.dll,FileProtocolHandler", target}, nil
}

func browserEnvironment(parent []string) []string {
	return allowlistedBrowserEnvironment(parent)
}
