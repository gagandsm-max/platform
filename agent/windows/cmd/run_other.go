//go:build !windows

package main

import (
	"log/slog"
)

// runEntrypoint executes console/debug mode on non-Windows hosts. Real SCM
// semantics are Windows-only; this path preserves the cross-compile / vet
// boundary while sharing the same bootstrap factory as Windows console mode.
func runEntrypoint(levelVar *slog.LevelVar, bootstrap *slog.Logger, serviceInstanceID string) error {
	return runConsoleEntrypoint(defaultBootstrapDeps(levelVar, bootstrap, serviceInstanceID))
}
