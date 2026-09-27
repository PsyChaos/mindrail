package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/credential"
)

const jevCredentialTimeout = 10 * time.Second

type jevCommandDeps struct {
	store           credential.Store
	connect         func(context.Context, credential.Store) error
	getenv          func(string) string
	mutationTimeout time.Duration
}

func productionJEVCommandDeps(o Options) jevCommandDeps {
	return jevCommandDeps{
		store: credential.NewOSStore(), connect: o.RunJEVConnect, getenv: os.Getenv,
		mutationTimeout: jevCredentialTimeout,
	}
}

// newJEVCommand builds the client-neutral JEV credential workflow. It does not
// bootstrap a repository: one OS credential belongs to the local user and is
// deliberately independent of whichever coding agent or repository invokes it.
func newJEVCommand(o Options) *cobra.Command { return newJEVCommandWith(productionJEVCommandDeps(o)) }

func newJEVCommandWith(deps jevCommandDeps) *cobra.Command {
	group := &cobra.Command{Use: "jev", Short: "Connect optional JEV routing securely", Args: cobra.NoArgs}
	group.AddCommand(
		&cobra.Command{
			Use: "connect", Short: "Open a local browser page and save the key in the OS credential store", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return runJEVConnect(cmd, deps) },
		},
		&cobra.Command{
			Use: "status", Short: "Report whether JEV routing has a credential", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return runJEVStatus(cmd, deps) },
		},
		&cobra.Command{
			Use: "disconnect", Short: "Remove the JEV key from the OS credential store", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return runJEVDisconnect(cmd, deps) },
		},
	)
	return group
}

type jevStatusResult struct {
	Connected bool   `json:"connected"`
	Source    string `json:"source"`
}

func (r jevStatusResult) renderHuman(w io.Writer, _ bool) error {
	if !r.Connected {
		_, err := io.WriteString(w, "JEV is not connected. Run mindrail jev connect to add a key.\n")
		return err
	}
	_, err := fmt.Fprintf(w, "JEV is connected (source: %s).\n", r.Source)
	return err
}

type jevDisconnectResult struct {
	Removed             bool   `json:"removed"`
	Connected           bool   `json:"connected"`
	Source              string `json:"source"`
	EnvironmentOverride bool   `json:"environment_override"`
}

func (r jevDisconnectResult) renderHuman(w io.Writer, _ bool) error {
	if r.EnvironmentOverride {
		if r.Removed {
			_, err := io.WriteString(w, "The keyring credential was removed. JEV remains connected through TYPESAFE_API_KEY.\n")
			return err
		}
		_, err := io.WriteString(w, "No keyring credential was stored. JEV remains connected through TYPESAFE_API_KEY.\n")
		return err
	}
	if r.Removed {
		_, err := io.WriteString(w, "JEV was disconnected and its keyring credential was removed.\n")
		return err
	}
	_, err := io.WriteString(w, "JEV was already disconnected.\n")
	return err
}

type jevConnectResult struct {
	Connected           bool   `json:"connected"`
	Source              string `json:"source"`
	StoredIn            string `json:"stored_in"`
	EnvironmentOverride bool   `json:"environment_override"`
}

func (r jevConnectResult) renderHuman(w io.Writer, _ bool) error {
	if r.EnvironmentOverride {
		_, err := io.WriteString(w, "The key was saved in the operating-system credential store. JEV still uses the TYPESAFE_API_KEY environment override until it is removed.\n")
		return err
	}
	_, err := io.WriteString(w, "JEV is connected. The key is stored in the operating-system credential store.\n")
	return err
}

func runJEVStatus(cmd *cobra.Command, deps jevCommandDeps) error {
	inv, err := newInvocation(cmd, "jev status", Options{})
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}
	result, verdict := resolveJEVStatus(cmd.Context(), deps)
	return inv.emit(result, result.renderHuman, nil, "", verdict)
}

func resolveJEVStatus(ctx context.Context, deps jevCommandDeps) (jevStatusResult, error) {
	if strings.TrimSpace(deps.getenv("TYPESAFE_API_KEY")) != "" {
		return jevStatusResult{Connected: true, Source: "environment"}, nil
	}
	storeCtx, cancel := context.WithTimeout(ctx, jevCredentialTimeout)
	defer cancel()
	key, err := deps.store.Get(storeCtx)
	switch {
	case err == nil && strings.TrimSpace(key) != "":
		return jevStatusResult{Connected: true, Source: "keyring"}, nil
	case err == nil, errors.Is(err, credential.ErrNotFound):
		return jevStatusResult{Connected: false, Source: "none"}, nil
	default:
		return jevStatusResult{Connected: false, Source: "unavailable"}, jevCredentialError("read", err)
	}
}

func runJEVDisconnect(cmd *cobra.Command, deps jevCommandDeps) error {
	inv, err := newInvocation(cmd, "jev disconnect", Options{})
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}
	removed := true
	if err := deleteJEVCredential(cmd.Context(), deps); err != nil {
		if errors.Is(err, credential.ErrNotFound) {
			removed = false
		} else if errors.Is(err, errJEVDisconnectIndeterminate) {
			return inv.emit(nil, nil, nil, "", jevDisconnectIndeterminateError())
		} else {
			return inv.emit(nil, nil, nil, "", jevCredentialError("remove", err))
		}
	}
	override := strings.TrimSpace(deps.getenv("TYPESAFE_API_KEY")) != ""
	source := "none"
	if override {
		source = "environment"
	}
	result := jevDisconnectResult{
		Removed: removed, Connected: override,
		Source:              source,
		EnvironmentOverride: override,
	}
	return inv.emit(result, result.renderHuman, nil, "", nil)
}

var errJEVDisconnectIndeterminate = errors.New("JEV credential deletion outcome is indeterminate")

func deleteJEVCredential(ctx context.Context, deps jevCommandDeps) error {
	timeout := deps.mutationTimeout
	if timeout <= 0 {
		timeout = jevCredentialTimeout
	}
	operationCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	outcome := make(chan error, 1)
	go func() { outcome <- deps.store.Delete(operationCtx) }()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-outcome:
		return err
	case <-ctx.Done():
	case <-timer.C:
	}

	// Prefer a result that became available at the boundary. Otherwise the
	// command returns without guessing: a native deletion already dispatched
	// may still complete after this point.
	cancel()
	select {
	case err := <-outcome:
		return err
	default:
		return errJEVDisconnectIndeterminate
	}
}

func jevDisconnectIndeterminateError() error {
	return app.NewError(app.CodeJEVDisconnectIndeterminate, app.KindUnavailable,
		"The operating-system credential store did not confirm JEV key deletion before Mindrail stopped waiting",
		"The key may still be present, or deletion may complete after this command returns.",
		"Run mindrail jev status to check the effective credential source; if TYPESAFE_API_KEY is set, remove that override before checking keyring status.")
}

func runJEVConnect(cmd *cobra.Command, deps jevCommandDeps) error {
	inv, err := newInvocation(cmd, "jev connect", Options{})
	if err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}
	if deps.connect == nil {
		return inv.emit(nil, nil, nil, "", app.NewError(app.CodeStartupIncomplete, app.KindUnavailable,
			"JEV browser connector is unavailable in this command composition", "No JEV key was saved.",
			"Run the packaged mindrail binary, which wires the browser connector."))
	}
	if err := deps.connect(cmd.Context(), deps.store); err != nil {
		return inv.emit(nil, nil, nil, "", err)
	}
	override := strings.TrimSpace(deps.getenv("TYPESAFE_API_KEY")) != ""
	source := "keyring"
	if override {
		source = "environment"
	}
	result := jevConnectResult{
		Connected: true, StoredIn: "keyring", EnvironmentOverride: override,
		Source: source,
	}
	return inv.emit(result, result.renderHuman, nil, "", nil)
}

func jevCredentialError(action string, err error) error {
	if errors.Is(err, credential.ErrUnsupportedPlatform) {
		return app.NewError(app.CodeJEVPersistenceUnsupported, app.KindUnavailable,
			"Mindrail does not support secure persistent JEV credentials on this platform",
			"No persistent JEV credential was read or changed.",
			"Set TYPESAFE_API_KEY in the environment of the agent process when JEV routing is needed, or continue without JEV.")
	}
	return app.NewError(app.CodeJEVCredentialUnavailable, app.KindUnavailable,
		"the operating-system credential store could not "+action+" the JEV key",
		"No key was exposed or written to repository files.",
		"Unlock or configure the operating-system credential service, then try again.")
}
