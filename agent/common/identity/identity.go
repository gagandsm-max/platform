package identity

import (
	"crypto/rand"
	"fmt"
	"os"
	"runtime"
)

type AgentIdentity struct {
	AgentID  string `json:"agent_id"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

func GenerateOrLoadIdentity(configuredID string) (*AgentIdentity, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}

	id := configuredID
	if id == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		id = fmt.Sprintf("agent-%x", b[:8])
	}

	return &AgentIdentity{
		AgentID:  id,
		Hostname: hostname,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
	}, nil
}
