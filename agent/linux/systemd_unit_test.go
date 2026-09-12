package linux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemdUnitMatchesLLDContract(t *testing.T) {
	unitPath := filepath.Join("systemd", "platform-agent.service")
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read systemd unit: %v", err)
	}
	text := string(data)
	required := []string{
		"User=platform-agent",
		"Group=platform-agent",
		"Restart=on-failure",
		"RestartSec=5",
		"TimeoutStartSec=30",
		"TimeoutStopSec=20",
		"NoNewPrivileges=true",
		"ExecStart=/usr/local/bin/platform-agent-linux",
		"EnvironmentFile=/etc/platform/agent.env",
		"StateDirectory=platform/agent",
		"KillSignal=SIGTERM",
		"ReadWritePaths=/var/lib/platform/agent",
	}
	if strings.Contains(text, "EnvironmentFile=-") {
		t.Error("optional EnvironmentFile (-) would start without config; fail-closed requires the file")
	}
	for _, want := range required {
		if !strings.Contains(text, want) {
			t.Errorf("systemd unit missing %q", want)
		}
	}
	if strings.Contains(text, "Restart=always") {
		t.Error("Restart=always is unbounded; LLD requires on-failure")
	}
}

func TestEnvExampleHasNoSecrets(t *testing.T) {
	path := filepath.Join("systemd", "platform-agent.env.example")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read env example: %v", err)
	}
	text := string(data)
	for _, forbidden := range []string{"BEGIN ", "PRIVATE KEY", "password=", "secret="} {
		if strings.Contains(text, forbidden) {
			t.Errorf("env example must not contain %q", forbidden)
		}
	}
	if !strings.Contains(text, "PLATFORM_AGENT_SENSOR_URL=") {
		t.Error("env example must declare Sensor telemetry URL")
	}
}
