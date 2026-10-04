package linux

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// agentServiceShutdownBudget is the AgentService Stop() upper bound from
// platform/agent/common (defaultAgentShutdownBudget). Kept as a local literal so
// this package test does not import runtime service code solely for the gate.
const agentServiceShutdownBudget = 10 * time.Second

func TestSystemdUnitMatchesLLDContract(t *testing.T) {
	unitPath := filepath.Join("systemd", "platform-agent.service")
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read systemd unit: %v", err)
	}
	dirs := parseUnitDirectives(string(data))

	assertDirective(t, dirs, "Type", "simple")
	assertDirective(t, dirs, "User", "platform-agent")
	assertDirective(t, dirs, "Group", "platform-agent")
	assertDirective(t, dirs, "EnvironmentFile", "/etc/platform/agent.env")
	assertDirective(t, dirs, "ExecStart", "/usr/local/bin/platform-agent-linux")
	assertDirective(t, dirs, "WorkingDirectory", "/var/lib/platform/agent")
	assertDirective(t, dirs, "StateDirectory", "platform/agent")
	assertDirective(t, dirs, "Restart", "on-failure")
	assertDirective(t, dirs, "RestartSec", "5")
	assertDirective(t, dirs, "StartLimitIntervalSec", "60")
	assertDirective(t, dirs, "StartLimitBurst", "5")
	assertDirective(t, dirs, "TimeoutStartSec", "30")
	assertDirective(t, dirs, "TimeoutStopSec", "20")
	assertDirective(t, dirs, "NoNewPrivileges", "true")
	assertDirective(t, dirs, "ProtectSystem", "strict")
	assertDirective(t, dirs, "ProtectHome", "true")
	assertDirective(t, dirs, "PrivateTmp", "true")
	assertDirective(t, dirs, "ReadWritePaths", "/var/lib/platform/agent")
	assertDirective(t, dirs, "KillSignal", "SIGTERM")
	assertDirective(t, dirs, "SendSIGKILL", "yes")
	assertDirective(t, dirs, "WantedBy", "multi-user.target")

	for _, v := range dirs["Restart"] {
		if v == "always" {
			t.Fatal("Restart=always must be absent; LLD requires on-failure")
		}
	}

	// Fail-closed: optional EnvironmentFile=- prefix must not appear.
	for _, v := range dirs["EnvironmentFile"] {
		if strings.HasPrefix(v, "-") {
			t.Fatalf("EnvironmentFile is optional (%q); fail-closed requires the file", v)
		}
	}

	if dirs["User"][0] == "root" || dirs["Group"][0] == "root" {
		t.Fatal("service must not run as root")
	}
	if dirs["User"][0] != "platform-agent" || dirs["Group"][0] != "platform-agent" {
		t.Fatalf("service account=%s:%s want platform-agent:platform-agent", dirs["User"][0], dirs["Group"][0])
	}

	stopSec, err := strconv.Atoi(dirs["TimeoutStopSec"][0])
	if err != nil {
		t.Fatalf("TimeoutStopSec parse: %v", err)
	}
	if time.Duration(stopSec)*time.Second <= agentServiceShutdownBudget {
		t.Fatalf("TimeoutStopSec=%ds must be greater than AgentService shutdown budget %s",
			stopSec, agentServiceShutdownBudget)
	}
}

func TestSystemdUnitAndEnvMirrorsByteIdentical(t *testing.T) {
	agentUnit := mustRead(t, filepath.Join("systemd", "platform-agent.service"))
	agentEnv := mustRead(t, filepath.Join("systemd", "platform-agent.env.example"))

	deployUnitPath, ok := findRepoRelative(filepath.Join("deploy", "systemd", "platform-agent.service"))
	if !ok {
		// Agent Docker verify stages ship only the agent module. Full-repo
		// validation (writer/CI with repository checkout) must still run this gate.
		t.Skip("deploy/systemd/platform-agent.service not found in agent-only test layout")
	}
	deployEnvPath, ok := findRepoRelative(filepath.Join("deploy", "systemd", "platform-agent.env.example"))
	if !ok {
		t.Fatal("deploy/systemd/platform-agent.env.example missing while service mirror is present")
	}

	deployUnit := mustRead(t, deployUnitPath)
	deployEnv := mustRead(t, deployEnvPath)

	if !bytes.Equal(agentUnit, deployUnit) {
		t.Fatal("agent/linux/systemd/platform-agent.service and deploy/systemd/platform-agent.service must be byte-identical")
	}
	if !bytes.Equal(agentEnv, deployEnv) {
		t.Fatal("agent/linux/systemd/platform-agent.env.example and deploy/systemd/platform-agent.env.example must be byte-identical")
	}
}

func TestEnvExampleHasNoSecrets(t *testing.T) {
	paths := []string{filepath.Join("systemd", "platform-agent.env.example")}
	if deployEnvPath, ok := findRepoRelative(filepath.Join("deploy", "systemd", "platform-agent.env.example")); ok {
		paths = append(paths, deployEnvPath)
	}
	for _, path := range paths {
		text := string(mustRead(t, path))
		for _, forbidden := range []string{
			"BEGIN ",
			"PRIVATE KEY",
			"password=",
			"secret=",
			"-----BEGIN",
		} {
			if strings.Contains(text, forbidden) {
				t.Errorf("env example %s must not contain %q", path, forbidden)
			}
		}
		if !strings.Contains(text, "PLATFORM_AGENT_SENSOR_URL=") {
			t.Errorf("env example %s must declare Sensor telemetry URL", path)
		}
	}
}

// parseUnitDirectives maps directive keys to ordered values (comments/blank/section headers ignored).
func parseUnitDirectives(text string) map[string][]string {
	out := make(map[string][]string)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		out[key] = append(out[key], value)
	}
	return out
}

func assertDirective(t *testing.T, dirs map[string][]string, key, want string) {
	t.Helper()
	values, ok := dirs[key]
	if !ok || len(values) == 0 {
		t.Fatalf("systemd unit missing directive %s", key)
	}
	if values[0] != want {
		t.Fatalf("systemd unit %s=%q want %q (all=%v)", key, values[0], want, values)
	}
	if len(values) != 1 {
		t.Fatalf("systemd unit %s has %d values, want exactly one: %v", key, len(values), values)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// findRepoRelative walks upward from the package working directory looking for
// relPath (for example deploy/systemd/platform-agent.service). Also accepts an
// optional absolute mount at /deploy/... for containerized full-repo gates.
func findRepoRelative(relPath string) (string, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	dir := wd
	for {
		candidate := filepath.Join(dir, relPath)
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if strings.HasPrefix(relPath, "deploy/") || strings.HasPrefix(relPath, "deploy"+string(filepath.Separator)) {
		abs := filepath.Join("/deploy", strings.TrimPrefix(strings.TrimPrefix(relPath, "deploy/"), "deploy"+string(filepath.Separator)))
		if st, err := os.Stat(abs); err == nil && !st.IsDir() {
			return abs, true
		}
	}
	return "", false
}
