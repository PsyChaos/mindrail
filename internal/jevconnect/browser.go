package jevconnect

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

func openDefaultBrowser(ctx context.Context, target string) error {
	return openBrowser(ctx, target, os.Environ(), runBrowserCommand)
}

func openBrowser(
	ctx context.Context,
	target string,
	parentEnvironment []string,
	run func(context.Context, string, []string, []string) error,
) error {
	command, args, err := browserCommand(target)
	if err != nil {
		return err
	}
	launchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return run(launchCtx, command, args, browserEnvironment(parentEnvironment))
}

func runBrowserCommand(ctx context.Context, command string, args, environment []string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = environment
	return cmd.Run()
}

func allowlistedBrowserEnvironment(parent []string) []string {
	environment := make([]string, 0, len(parent))
	for _, entry := range parent {
		name, _, found := strings.Cut(entry, "=")
		if !found || !browserEnvironmentVariableAllowed(name) {
			continue
		}
		environment = append(environment, entry)
	}
	return environment
}

func browserEnvironmentVariableAllowed(name string) bool {
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "LC_") {
		return true
	}
	switch upper {
	case "HOME", "USER", "LOGNAME", "LANG", "LANGUAGE", "TZ",
		"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "XDG_RUNTIME_DIR",
		"XDG_CURRENT_DESKTOP", "DESKTOP_SESSION", "DBUS_SESSION_BUS_ADDRESS",
		"TMPDIR", "TEMP", "TMP", "USERPROFILE":
		return true
	default:
		return false
	}
}
