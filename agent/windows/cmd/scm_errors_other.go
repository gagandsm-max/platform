//go:build !windows

package main

import "errors"

// errSCMRun is declared on non-Windows so classifyRunExit compiles. SCM run is
// Windows-only; this sentinel is unused on !windows paths.
var errSCMRun = errors.New("AGENT_SCM_RUN_FAILED")
