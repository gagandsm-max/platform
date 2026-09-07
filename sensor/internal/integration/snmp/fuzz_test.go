package snmpintegration

import "testing"

func FuzzSNMPResultToObservationEnvelope(f *testing.F) {
	f.Add([]byte(`{"schemaVersion":1,"version":"2c","operation":"GET","collectedAt":"2026-09-07T00:00:00Z","observations":[]}`))
	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) > 4096 {
			return
		}
		_, _ = decodeResult(payload, 4096)
	})
}
