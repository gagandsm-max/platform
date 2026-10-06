package collector

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Read-only systemd introspection plan (BUILD-011 Slice 3).
// Executable is Platform-owned and allowlisted. Arguments are structured argv
// only — never shell interpolation, never user-provided command text.

const (
	systemdShowVerb      = "show"
	systemdListUnitsVerb = "list-units"
	// Implementation safety bound for parsed systemd stdout (not customer sizing).
	maxSystemdStdoutBytes = 1 << 20
	maxSystemdStderrBytes = 64 * 1024
	maxSystemdUnits       = maxProcessServiceCatalogueEntries
	maxSystemdFieldLen    = maxProcessServiceIdentityLen
)

var systemdAllowedExecutables = []string{
	"/usr/bin/systemctl",
	"/bin/systemctl",
}

var systemdReadOnlyVerbs = map[string]struct{}{
	systemdShowVerb:      {},
	systemdListUnitsVerb: {},
}

var systemdMutationVerbs = []string{
	"start", "stop", "restart", "try-restart", "reload", "reload-or-restart",
	"isolate", "kill", "enable", "disable", "reenable", "mask", "unmask",
	"reset-failed", "preset", "edit", "daemon-reload", "daemon-reexec",
	"reboot", "poweroff", "halt", "kexec", "switch-root",
}

var systemdShowProperties = []string{
	"Id",
	"ActiveState",
	"SubState",
	"Result",
	"NRestarts",
	"MainPID",
	"ExecMainStatus",
}

func systemdShowPropertyArg() string {
	return "--property=" + strings.Join(systemdShowProperties, ",")
}

func systemctlExecutableAllowed(path string) bool {
	for _, allowed := range systemdAllowedExecutables {
		if path == allowed {
			return true
		}
	}
	return false
}

func validateSystemdUnitID(id string) error {
	sel := ProcessServiceSelector{Kind: SelectorKindServiceID, Value: id}
	if err := sel.Validate(); err != nil {
		return err
	}
	if strings.ContainsAny(id, "/") {
		return errors.New("SYSTEMD_UNIT_ID_INVALID: path separators are prohibited")
	}
	if strings.Contains(id, "..") {
		return errors.New("SYSTEMD_UNIT_ID_INVALID: path traversal is prohibited")
	}
	return nil
}

func systemdListUnitsArgv() []string {
	return []string{
		systemdListUnitsVerb,
		"--type=service",
		"--all",
		"--plain",
		"--no-legend",
		"--no-pager",
		"--no-ask-password",
	}
}

func systemdShowArgv(unitIDs []string) ([]string, error) {
	if len(unitIDs) == 0 {
		return nil, errors.New("SYSTEMD_SHOW_UNITS_REQUIRED")
	}
	if len(unitIDs) > maxSystemdUnits {
		return nil, fmt.Errorf("SYSTEMD_UNIT_BOUND_EXCEEDED: implementation safety bound is %d", maxSystemdUnits)
	}
	args := []string{
		systemdShowVerb,
		"--no-pager",
		"--no-legend",
		"--no-ask-password",
		systemdShowPropertyArg(),
	}
	for _, id := range unitIDs {
		if err := validateSystemdUnitID(id); err != nil {
			return nil, err
		}
		args = append(args, id)
	}
	if err := assertSystemctlArgvSafe(args); err != nil {
		return nil, err
	}
	return args, nil
}

func assertSystemctlArgvSafe(args []string) error {
	if len(args) == 0 {
		return errors.New("SYSTEMD_ARGV_INVALID: empty argv")
	}
	verb := args[0]
	if _, ok := systemdReadOnlyVerbs[verb]; !ok {
		return fmt.Errorf("SYSTEMD_ARGV_VERB_REJECTED: %q is not an allowlisted read-only verb", verb)
	}
	for _, a := range args {
		if a == "sh" || a == "bash" || a == "-c" || a == "/bin/sh" || a == "/bin/bash" {
			return errors.New("SYSTEMD_ARGV_SHELL_REJECTED")
		}
		for _, mut := range systemdMutationVerbs {
			if a == mut {
				return fmt.Errorf("SYSTEMD_ARGV_MUTATION_REJECTED: %q", a)
			}
		}
	}
	return nil
}

func parseSystemdListUnits(stdout string) (units []string, truncated bool, err error) {
	if len(stdout) > maxSystemdStdoutBytes {
		stdout = stdout[:maxSystemdStdoutBytes]
		truncated = true
	}
	seen := make(map[string]struct{})
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "UNIT") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		id := fields[0]
		if !strings.HasSuffix(id, ".service") {
			continue
		}
		if err := validateSystemdUnitID(id); err != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		if len(units) >= maxSystemdUnits {
			truncated = true
			break
		}
		seen[id] = struct{}{}
		units = append(units, id)
	}
	return units, truncated, nil
}

func parseSystemdShow(stdout string) ([]SystemdServiceObservation, error) {
	if len(stdout) > maxSystemdStdoutBytes {
		stdout = stdout[:maxSystemdStdoutBytes]
	}
	blocks := strings.Split(stdout, "\n\n")
	out := make([]SystemdServiceObservation, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		obs, ok := parseSystemdShowBlock(block)
		if !ok {
			continue
		}
		if len(out) >= maxSystemdUnits {
			break
		}
		out = append(out, obs)
	}
	return out, nil
}

func parseSystemdShowBlock(block string) (SystemdServiceObservation, bool) {
	obs := SystemdServiceObservation{
		SourceClass: ProcessServiceSourceObserved,
		Quality:     "OBSERVED",
	}
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return SystemdServiceObservation{}, false
		}
		if len(key) > maxSystemdFieldLen || len(val) > maxSystemdFieldLen {
			return SystemdServiceObservation{}, false
		}
		if hasControlRunes(key) || hasControlRunes(val) {
			return SystemdServiceObservation{}, false
		}
		switch key {
		case "Id":
			obs.UnitID = val
		case "ActiveState":
			obs.ActiveState = val
		case "SubState":
			obs.SubState = val
		case "Result":
			obs.Result = val
		case "NRestarts":
			n, err := strconv.ParseUint(val, 10, 64)
			if err != nil {
				return SystemdServiceObservation{}, false
			}
			obs.NRestarts = n
			obs.HasNRestarts = true
		case "MainPID":
			n, err := strconv.ParseInt(val, 10, 32)
			if err != nil || n < 0 {
				return SystemdServiceObservation{}, false
			}
			if n > 0 {
				obs.MainPID = int(n)
				obs.HasMainPID = true
			}
		case "ExecMainStatus":
			n, err := strconv.ParseInt(val, 10, 32)
			if err != nil {
				return SystemdServiceObservation{}, false
			}
			obs.ExecMainStatus = int(n)
			obs.HasExecMainStatus = true
		default:
			// Ignore unexpected properties; never treat them as command text.
		}
	}
	if validateSystemdUnitID(obs.UnitID) != nil {
		return SystemdServiceObservation{}, false
	}
	if strings.TrimSpace(obs.ActiveState) == "" {
		obs.ActiveState = "unknown"
	}
	return obs, true
}
