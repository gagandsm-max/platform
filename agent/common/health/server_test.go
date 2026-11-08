package health

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/collector"
	"platform/agent/common/delivery"
	"platform/agent/common/queue"
	"platform/agent/common/scheduler"
)

type mockStatsProvider struct {
	mu sync.Mutex

	lifecycle                string
	admission                scheduler.AdmissionStatus
	schedulerRunning         bool
	enabledCollectors        int
	collectors               []CollectorSnapshot
	collectorRunErrors       int
	collectorLastRun         time.Time
	receiverAvailable        bool
	atCapacity               bool
	quarantineAtCapacity     bool
	quarantine               int
	activeCount              int
	lastErrorCode            string
	lastFailureClass         client.FailureClass
	deliveryBlocked          bool
	unknownSequence          bool
	locationUncertain        bool
	metaUncertain            bool
	sequenceGap              bool
	sequenceConflict         bool
	ackRecoveryConflict      bool
	processNetworkCapability collector.ProcessNetworkCapabilitySnapshot
	processNetworkSupport    collector.ProcessNetworkSupportSnapshot
}

func healthyProvider() *mockStatsProvider {
	return &mockStatsProvider{
		lifecycle:         "RUNNING",
		schedulerRunning:  true,
		enabledCollectors: 1,
		collectors: []CollectorSnapshot{{
			Name:    "cpu",
			Enabled: true,
		}},
		receiverAvailable: true,
		activeCount:       0,
	}
}

func (m *mockStatsProvider) RuntimeSnapshot() RuntimeSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	life := m.lifecycle
	if life == "" {
		life = "RUNNING"
	}
	collectors := append([]CollectorSnapshot(nil), m.collectors...)

	return RuntimeSnapshot{
		Lifecycle:          life,
		Admission:          m.admission,
		SchedulerRunning:   m.schedulerRunning,
		EnabledCollectors:  m.enabledCollectors,
		Collectors:         collectors,
		CollectorLastRun:   m.collectorLastRun,
		CollectorRunErrors: m.collectorRunErrors,
		Queue: queue.QueueStats{
			ActiveCount:          m.activeCount,
			ActiveBytes:          1024,
			AtCapacity:           m.atCapacity,
			QuarantineCount:      m.quarantine,
			QuarantineAtCapacity: m.quarantineAtCapacity,
			DeliveryBlocked: m.deliveryBlocked || m.unknownSequence || m.locationUncertain ||
				m.metaUncertain || m.sequenceGap || m.sequenceConflict || m.ackRecoveryConflict,
			UnknownSequenceBlock:     m.unknownSequence,
			LocationUncertain:        m.locationUncertain,
			MetaUncertain:            m.metaUncertain,
			SequenceGapBlock:         m.sequenceGap,
			SequenceConflictBlock:    m.sequenceConflict,
			AckRecoveryConflictBlock: m.ackRecoveryConflict,
		},
		Delivery: delivery.DeliveryStats{
			ReceiverAvailable: m.receiverAvailable,
			Attempts:          10,
			Successes:         5,
			LastErrorCode:     m.lastErrorCode,
			LastFailureClass:  m.lastFailureClass,
		},
		ProcessNetworkCapability: m.processNetworkCapability.Copy(),
		ProcessNetworkSupport:    m.processNetworkSupport.Copy(),
	}
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	return body
}

func assertReady(t *testing.T, s *Server, wantCode int, wantStatus string, wantReady bool, wantReason string) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleReady(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if w.Code != wantCode {
		t.Fatalf("/ready code=%d want=%d body=%s", w.Code, wantCode, w.Body.String())
	}
	body := decodeBody(t, w)
	if body["status"] != wantStatus {
		t.Fatalf("/ready status=%v want=%s body=%v", body["status"], wantStatus, body)
	}
	if body["ready"] != wantReady {
		t.Fatalf("/ready ready=%v want=%v body=%v", body["ready"], wantReady, body)
	}
	if body["http_semantic"] != float64(wantCode) {
		t.Fatalf("/ready http_semantic=%v want=%d body=%v", body["http_semantic"], wantCode, body)
	}
	if wantReason != "" && body["reason_code"] != wantReason {
		t.Fatalf("/ready reason_code=%v want=%s body=%v", body["reason_code"], wantReason, body)
	}
	return body
}

func assertHealth(t *testing.T, s *Server, wantCode int, wantState string, wantReason string) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleHealth(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != wantCode {
		t.Fatalf("/health code=%d want=%d body=%s", w.Code, wantCode, w.Body.String())
	}
	body := decodeBody(t, w)
	if body["document_type"] != "SUPPORT_EVIDENCE" {
		t.Fatalf("/health document_type=%v", body["document_type"])
	}
	if body["component_state"] != wantState {
		t.Fatalf("/health component_state=%v want=%s body=%v", body["component_state"], wantState, body)
	}
	if wantReason != "" && !strings.Contains(w.Body.String(), wantReason) {
		t.Fatalf("/health missing reason %s body=%s", wantReason, w.Body.String())
	}
	return body
}

func assertLiveOK(t *testing.T, s *Server) {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleLive(w, httptest.NewRequest(http.MethodGet, "/live", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("/live code=%d", w.Code)
	}
}

func TestHealthyRunningReady(t *testing.T) {
	s, err := NewServer("127.0.0.1:0", healthyProvider(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertReady(t, s, http.StatusOK, "UP", true, "")
	assertHealth(t, s, http.StatusOK, "UP", "")
	assertLiveOK(t, s)
}

func TestLifecycleStatesNotReady(t *testing.T) {
	cases := []struct {
		state  string
		reason string
	}{
		{"NEW", "AGENT_NOT_STARTED"},
		{"STARTING", "AGENT_STARTING"},
		{"STOPPING", "AGENT_SHUTTING_DOWN"},
		{"STOPPED", "AGENT_STOPPED"},
		{"FAILED", "AGENT_FAILED"},
		{"BOGUS", "AGENT_LIFECYCLE_INVALID"},
	}
	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			prov := healthyProvider()
			prov.lifecycle = tc.state
			s, err := NewServer("127.0.0.1:0", prov, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertReady(t, s, http.StatusServiceUnavailable, "NOT_READY", false, tc.reason)
			assertHealth(t, s, http.StatusServiceUnavailable, "NOT_READY", tc.reason)
			assertLiveOK(t, s)
		})
	}
}

func TestSensorUpstreamDegradedButReady(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*mockStatsProvider)
		reason string
	}{
		{
			name: "receiver_unavailable",
			mutate: func(m *mockStatsProvider) {
				m.receiverAvailable = false
			},
			reason: "SENSOR_TRANSPORT_UNAVAILABLE",
		},
		{
			name: "temporary",
			mutate: func(m *mockStatsProvider) {
				m.lastFailureClass = client.FailureTemporary
				m.lastErrorCode = "DELIVERY_NETWORK_ERROR"
			},
			reason: "DELIVERY_NETWORK_ERROR",
		},
		{
			name: "authentication",
			mutate: func(m *mockStatsProvider) {
				m.lastFailureClass = client.FailureAuthentication
				m.lastErrorCode = "DELIVERY_AUTH_FAILED"
			},
			reason: "DELIVERY_AUTH_FAILED",
		},
		{
			name: "permanent_remote",
			mutate: func(m *mockStatsProvider) {
				m.lastFailureClass = client.FailurePermanent
				m.lastErrorCode = "DELIVERY_REMOTE_REJECTED"
			},
			reason: "DELIVERY_REMOTE_REJECTED",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prov := healthyProvider()
			tc.mutate(prov)
			s, err := NewServer("127.0.0.1:0", prov, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertReady(t, s, http.StatusOK, "DEGRADED", true, tc.reason)
			assertHealth(t, s, http.StatusOK, "DEGRADED", tc.reason)
			assertLiveOK(t, s)
		})
	}
}

func TestDeliveryBlockedAndOversizedNotReady(t *testing.T) {
	t.Run("blocked", func(t *testing.T) {
		prov := healthyProvider()
		prov.lastFailureClass = client.FailureBlocked
		prov.lastErrorCode = "QUEUE_ACK_COMMIT_FAILED"
		s, err := NewServer("127.0.0.1:0", prov, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertReady(t, s, http.StatusServiceUnavailable, "NOT_READY", false, "QUEUE_ACK_COMMIT_FAILED")
		body := assertHealth(t, s, http.StatusServiceUnavailable, "NOT_READY", "QUEUE_ACK_COMMIT_FAILED")
		deps, _ := body["dependency_status"].(map[string]any)
		if deps["sensor_transport"] != "UP" {
			t.Fatalf("local blocked hold must keep sensor_transport UP: %v", body)
		}
	})
	t.Run("oversized", func(t *testing.T) {
		prov := healthyProvider()
		prov.lastFailureClass = client.FailureOversized
		prov.lastErrorCode = "ERR_REQUEST_TOO_LARGE"
		s, err := NewServer("127.0.0.1:0", prov, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertReady(t, s, http.StatusServiceUnavailable, "NOT_READY", false, "ERR_REQUEST_TOO_LARGE")
		assertHealth(t, s, http.StatusServiceUnavailable, "NOT_READY", "ERR_REQUEST_TOO_LARGE")
	})
}

func TestQueueHardUnsafeFlagsNotReady(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*mockStatsProvider)
		reason string
	}{
		{"at_capacity", func(m *mockStatsProvider) { m.atCapacity = true; m.activeCount = 5 }, "QUEUE_FULL"},
		{"quarantine_capacity", func(m *mockStatsProvider) { m.quarantineAtCapacity = true }, "QUEUE_QUARANTINE_FULL"},
		{"delivery_blocked", func(m *mockStatsProvider) { m.deliveryBlocked = true }, "QUEUE_DELIVERY_BLOCKED"},
		{"unknown_sequence", func(m *mockStatsProvider) { m.unknownSequence = true }, "QUEUE_UNKNOWN_SEQUENCE_BLOCK"},
		{"location_uncertain", func(m *mockStatsProvider) { m.locationUncertain = true }, "QUEUE_QUARANTINE_UNCERTAIN"},
		{"meta_uncertain", func(m *mockStatsProvider) { m.metaUncertain = true }, "QUEUE_META_UNCERTAIN"},
		{"sequence_gap", func(m *mockStatsProvider) { m.sequenceGap = true }, "QUEUE_SEQUENCE_GAP"},
		{"sequence_conflict", func(m *mockStatsProvider) { m.sequenceConflict = true }, "QUEUE_SEQUENCE_CONFLICT"},
		{"ack_recovery", func(m *mockStatsProvider) { m.ackRecoveryConflict = true }, "QUEUE_ACK_RECOVERY_CONFLICT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prov := healthyProvider()
			tc.mutate(prov)
			s, err := NewServer("127.0.0.1:0", prov, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertReady(t, s, http.StatusServiceUnavailable, "NOT_READY", false, tc.reason)
			assertHealth(t, s, http.StatusServiceUnavailable, "NOT_READY", tc.reason)
			assertLiveOK(t, s)
		})
	}
}

func TestQuarantinePresenceAloneDegradedReady(t *testing.T) {
	prov := healthyProvider()
	prov.quarantine = 2
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertReady(t, s, http.StatusOK, "DEGRADED", true, "QUEUE_QUARANTINE_PRESENT")
	assertHealth(t, s, http.StatusOK, "DEGRADED", "QUEUE_QUARANTINE_PRESENT")
}

func TestOrdinaryBacklogRemainsReady(t *testing.T) {
	prov := healthyProvider()
	prov.activeCount = 42
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertReady(t, s, http.StatusOK, "UP", true, "")
	body := assertHealth(t, s, http.StatusOK, "UP", "")
	if body["delivery_status"] != "BACKLOG" {
		t.Fatalf("expected BACKLOG delivery_status, got %v", body["delivery_status"])
	}
}

func TestSchedulerReadinessMatrix(t *testing.T) {
	t.Run("not_running", func(t *testing.T) {
		prov := healthyProvider()
		prov.schedulerRunning = false
		s, err := NewServer("127.0.0.1:0", prov, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertReady(t, s, http.StatusServiceUnavailable, "NOT_READY", false, "AGENT_SCHEDULER_NOT_RUNNING")
	})
	t.Run("zero_enabled", func(t *testing.T) {
		prov := healthyProvider()
		prov.enabledCollectors = 0
		prov.collectors = []CollectorSnapshot{{Name: "cpu", Enabled: false}}
		s, err := NewServer("127.0.0.1:0", prov, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertReady(t, s, http.StatusServiceUnavailable, "NOT_READY", false, "AGENT_NO_ENABLED_COLLECTORS")
	})
	t.Run("partial_outstanding_degraded", func(t *testing.T) {
		prov := healthyProvider()
		prov.enabledCollectors = 2
		prov.collectors = []CollectorSnapshot{
			{Name: "cpu", Enabled: true, Outstanding: true},
			{Name: "mem", Enabled: true, Outstanding: false},
		}
		s, err := NewServer("127.0.0.1:0", prov, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertReady(t, s, http.StatusOK, "DEGRADED", true, "COLLECTOR_OUTSTANDING")
	})
	t.Run("all_outstanding_not_ready", func(t *testing.T) {
		prov := healthyProvider()
		prov.enabledCollectors = 2
		prov.collectors = []CollectorSnapshot{
			{Name: "cpu", Enabled: true, Outstanding: true},
			{Name: "mem", Enabled: true, Outstanding: true},
		}
		s, err := NewServer("127.0.0.1:0", prov, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertReady(t, s, http.StatusServiceUnavailable, "NOT_READY", false, "AGENT_ALL_COLLECTORS_BLOCKED")
	})
	t.Run("current_last_error_degraded", func(t *testing.T) {
		prov := healthyProvider()
		prov.enabledCollectors = 2
		prov.collectors = []CollectorSnapshot{
			{Name: "cpu", Enabled: true, LastError: "collection_timeout"},
			{Name: "mem", Enabled: true},
		}
		s, err := NewServer("127.0.0.1:0", prov, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertReady(t, s, http.StatusOK, "DEGRADED", true, "COLLECTOR_DEGRADED")
	})
	t.Run("historical_errors_alone_remain_ready", func(t *testing.T) {
		prov := healthyProvider()
		prov.collectorRunErrors = 99
		prov.collectors = []CollectorSnapshot{
			{Name: "cpu", Enabled: true, LastError: ""},
		}
		s, err := NewServer("127.0.0.1:0", prov, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertReady(t, s, http.StatusOK, "UP", true, "")
	})
}

func TestHealthzMatchesReady(t *testing.T) {
	prov := healthyProvider()
	prov.receiverAvailable = false
	prov.lastErrorCode = "DELIVERY_NETWORK_ERROR"
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	wReady := httptest.NewRecorder()
	s.handleReady(wReady, httptest.NewRequest(http.MethodGet, "/ready", nil))
	wAlias := httptest.NewRecorder()
	s.handleReady(wAlias, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if wReady.Code != wAlias.Code {
		t.Fatalf("/healthz code=%d ready code=%d", wAlias.Code, wReady.Code)
	}
	ready := decodeBody(t, wReady)
	alias := decodeBody(t, wAlias)
	for _, key := range []string{"status", "ready", "http_semantic", "reason_code", "component"} {
		if ready[key] != alias[key] {
			t.Fatalf("/healthz %s=%v ready %s=%v", key, alias[key], key, ready[key])
		}
	}
}

func TestHealthUsesSameClassification(t *testing.T) {
	prov := healthyProvider()
	prov.atCapacity = true
	prov.activeCount = 3
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	ready := assertReady(t, s, http.StatusServiceUnavailable, "NOT_READY", false, "QUEUE_FULL")
	healthBody := assertHealth(t, s, http.StatusServiceUnavailable, "NOT_READY", "QUEUE_FULL")
	if ready["http_semantic"] != float64(http.StatusServiceUnavailable) {
		t.Fatalf("ready http_semantic mismatch: %v", ready)
	}
	_ = healthBody
}

func TestDeliveryBlockedWithQuarantineReasons(t *testing.T) {
	prov := healthyProvider()
	prov.deliveryBlocked = true
	prov.quarantine = 1
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := assertHealth(t, s, http.StatusServiceUnavailable, "NOT_READY", "QUEUE_DELIVERY_BLOCKED")
	raw, _ := json.Marshal(body["reason_codes"])
	if !strings.Contains(string(raw), "QUEUE_QUARANTINE_PRESENT") {
		t.Fatalf("expected quarantine secondary reason, got %s", raw)
	}
}

func TestNoSecretOrPayloadLeakage(t *testing.T) {
	prov := healthyProvider()
	prov.ackRecoveryConflict = true
	prov.lastErrorCode = "QUEUE_ACK_RECOVERY_CONFLICT"
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.handleHealth(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	body := w.Body.String()
	for _, banned := range []string{"payload", "replacement", "secret", "token", "password", "BEGIN CERTIFICATE"} {
		if strings.Contains(strings.ToLower(body), banned) {
			t.Fatalf("health leaked %q: %s", banned, body)
		}
	}
}

func TestOptionalEBPFCapabilityEvidenceDoesNotAffectLiveOrReady(t *testing.T) {
	for _, state := range []collector.ProcessNetworkCapabilityState{
		collector.ProcessNetworkCapabilityUnsupported,
		collector.ProcessNetworkCapabilityUnavailable,
		collector.ProcessNetworkCapabilityPrivilegeDenied,
		collector.ProcessNetworkCapabilityKernelIncompatible,
		collector.ProcessNetworkCapabilityResourceGuarded,
	} {
		t.Run(string(state), func(t *testing.T) {
			prov := healthyProvider()
			prov.processNetworkCapability = collector.ProcessNetworkCapabilitySnapshot{
				State:             state,
				ReasonCodes:       []collector.ProcessNetworkCapabilityReason{collector.ReasonBackendNotWired},
				ObservedAt:        time.Now().UTC(),
				TransitionAt:      time.Now().UTC(),
				ValidUntil:        time.Now().UTC().Add(time.Hour),
				NextRecheckAt:     time.Now().UTC().Add(time.Hour),
				AttemptCount:      1,
				OS:                "linux",
				Arch:              "amd64",
				KernelRelease:     "6.8.0-fixture",
				BootIDDigest:      strings.Repeat("a", 64),
				KernelFingerprint: strings.Repeat("b", 64),
				SourceQuality:     "FIXTURE",
			}
			s, err := NewServer("127.0.0.1:0", prov, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertReady(t, s, http.StatusOK, "UP", true, "")
			assertLiveOK(t, s)
			body := assertHealth(t, s, http.StatusOK, "UP", "")
			if _, exists := body["optional_capabilities"]; exists {
				t.Fatalf("/health must remain within SUPPORT_EVIDENCE schema: %v", body)
			}
			ebpfEvidence := assertProcessNetworkCapability(t, s)
			if ebpfEvidence["state"] != string(state) {
				t.Fatalf("eBPF evidence mismatch: %v", ebpfEvidence)
			}
			raw := wJSON(t, ebpfEvidence)
			for _, banned := range []string{"cmdline", "environ", "password", "token", "payload", "/proc/", "/sys/"} {
				if strings.Contains(strings.ToLower(raw), banned) {
					t.Fatalf("optional support evidence leaked %q: %s", banned, raw)
				}
			}
		})
	}
}

func TestStaleOptionalEBPFEvidenceBecomesUnavailableWithoutAffectingReadiness(t *testing.T) {
	prov := healthyProvider()
	prov.processNetworkCapability = collector.ProcessNetworkCapabilitySnapshot{
		State:         collector.ProcessNetworkCapabilityKernelIncompatible,
		ReasonCodes:   []collector.ProcessNetworkCapabilityReason{collector.ReasonBTFMissing},
		ObservedAt:    time.Now().UTC().Add(-2 * time.Hour),
		TransitionAt:  time.Now().UTC().Add(-2 * time.Hour),
		ValidUntil:    time.Now().UTC().Add(-time.Hour),
		NextRecheckAt: time.Now().UTC().Add(-time.Hour),
		AttemptCount:  1,
		SourceQuality: "FIXTURE",
	}
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertReady(t, s, http.StatusOK, "UP", true, "")
	assertHealth(t, s, http.StatusOK, "UP", "")
	evidence := assertProcessNetworkCapability(t, s)
	if evidence["state"] != string(collector.ProcessNetworkCapabilityUnavailable) {
		t.Fatalf("stale state=%v", evidence["state"])
	}
	reasons := evidence["reason_codes"].([]any)
	if len(reasons) != 1 || reasons[0] != string(collector.ReasonEvidenceStale) {
		t.Fatalf("stale reasons=%v", reasons)
	}
}

func TestCurrentAuthorityCapabilityDoesNotFalseStaleOnEndpoint(t *testing.T) {
	cases := []struct {
		name   string
		snap   collector.ProcessNetworkCapabilitySnapshot
		state  collector.ProcessNetworkCapabilityState
		reason collector.ProcessNetworkCapabilityReason
	}{
		{
			name: "runtime_active",
			snap: collector.ProcessNetworkCapabilitySnapshot{
				State:            collector.ProcessNetworkCapabilityActive,
				ObservedAt:       time.Now().UTC().Add(-time.Hour),
				TransitionAt:     time.Now().UTC().Add(-time.Hour),
				ValidUntil:       time.Now().UTC().Add(-time.Minute),
				CurrentAuthority: true,
				SourceQuality:    "FIXTURE",
			},
			state: collector.ProcessNetworkCapabilityActive,
		},
		{
			name: "runtime_privilege_denied",
			snap: collector.ProcessNetworkCapabilitySnapshot{
				State:            collector.ProcessNetworkCapabilityPrivilegeDenied,
				ReasonCodes:      []collector.ProcessNetworkCapabilityReason{collector.ReasonPrivilegeDenied},
				ObservedAt:       time.Now().UTC().Add(-time.Hour),
				TransitionAt:     time.Now().UTC().Add(-time.Hour),
				ValidUntil:       time.Now().UTC().Add(-time.Minute),
				CurrentAuthority: true,
				SourceQuality:    "FIXTURE",
			},
			state:  collector.ProcessNetworkCapabilityPrivilegeDenied,
			reason: collector.ReasonPrivilegeDenied,
		},
		{
			name: "runtime_backend_not_wired_zero_valid_until",
			snap: collector.ProcessNetworkCapabilitySnapshot{
				State:            collector.ProcessNetworkCapabilityUnavailable,
				ReasonCodes:      []collector.ProcessNetworkCapabilityReason{collector.ReasonBackendNotWired},
				ObservedAt:       time.Now().UTC(),
				TransitionAt:     time.Now().UTC(),
				CurrentAuthority: true,
				SourceQuality:    "FIXTURE",
			},
			state:  collector.ProcessNetworkCapabilityUnavailable,
			reason: collector.ReasonBackendNotWired,
		},
		{
			name: "feature_disabled",
			snap: collector.ProcessNetworkCapabilitySnapshot{
				State:            collector.ProcessNetworkCapabilityUnavailable,
				ReasonCodes:      []collector.ProcessNetworkCapabilityReason{collector.ReasonFeatureDisabled},
				ObservedAt:       time.Now().UTC().Add(-time.Hour),
				TransitionAt:     time.Now().UTC().Add(-time.Hour),
				ValidUntil:       time.Now().UTC().Add(-time.Minute),
				CurrentAuthority: true,
				SourceQuality:    "DISABLED",
			},
			state:  collector.ProcessNetworkCapabilityUnavailable,
			reason: collector.ReasonFeatureDisabled,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prov := healthyProvider()
			prov.processNetworkCapability = tc.snap
			s, err := NewServer("127.0.0.1:0", prov, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertReady(t, s, http.StatusOK, "UP", true, "")
			evidence := assertProcessNetworkCapability(t, s)
			if evidence["state"] != string(tc.state) {
				t.Fatalf("state=%v want=%s", evidence["state"], tc.state)
			}
			if tc.reason != "" {
				reasons := evidence["reason_codes"].([]any)
				if len(reasons) != 1 || reasons[0] != string(tc.reason) {
					t.Fatalf("reasons=%v want=%s", reasons, tc.reason)
				}
			}
		})
	}
}

func TestMissingOptionalEBPFEvidenceIsBoundedUnavailable(t *testing.T) {
	s, err := NewServer("127.0.0.1:0", healthyProvider(), nil)
	if err != nil {
		t.Fatal(err)
	}
	evidence := assertProcessNetworkCapability(t, s)
	if evidence["state"] != string(collector.ProcessNetworkCapabilityUnavailable) {
		t.Fatalf("missing state=%v", evidence["state"])
	}
	reasons := evidence["reason_codes"].([]any)
	if len(reasons) != 1 || reasons[0] != string(collector.ReasonSourceUnavailable) {
		t.Fatalf("missing reasons=%v", reasons)
	}
}

func TestProcessNetworkSupportCountersOnCapabilityEndpoint(t *testing.T) {
	prov := healthyProvider()
	now := time.Now().UTC()
	prov.processNetworkCapability = collector.ProcessNetworkCapabilitySnapshot{
		State:         collector.ProcessNetworkCapabilityActive,
		ObservedAt:    now,
		TransitionAt:  now,
		ValidUntil:    now.Add(time.Hour),
		NextRecheckAt: now.Add(time.Hour),
		SourceQuality: "FIXTURE",
	}
	prov.processNetworkSupport = collector.ProcessNetworkSupportSnapshot{
		Present:               true,
		MapUpdateFail:         1,
		RingReserveFail:       2,
		DecoderReject:         3,
		UserspaceChannelFull:  4,
		TransportNotWired:     5,
		AggregatorFaults:      6,
		ReaderFaults:          7,
		OptionalProbeAttached: true,
	}
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertReady(t, s, http.StatusOK, "UP", true, "")
	evidence := assertProcessNetworkCapability(t, s)
	counters, ok := evidence["support_counters"].(map[string]any)
	if !ok {
		t.Fatalf("missing support_counters: %v", evidence)
	}
	if counters["MAP_UPDATE_FAIL"].(float64) != 1 ||
		counters["RING_RESERVE_FAIL"].(float64) != 2 ||
		counters["DECODER_REJECT"].(float64) != 3 ||
		counters["USERSPACE_CHANNEL_FULL"].(float64) != 4 ||
		counters["TRANSPORT_NOT_WIRED"].(float64) != 5 ||
		counters["aggregator_faults"].(float64) != 6 ||
		counters["reader_faults"].(float64) != 7 ||
		counters["optional_probe_attached"] != true {
		t.Fatalf("support_counters=%v", counters)
	}
	body := assertHealth(t, s, http.StatusOK, "UP", "")
	if _, exists := body["support_counters"]; exists {
		t.Fatal("/health must not absorb eBPF support counters into SUPPORT_EVIDENCE")
	}
}

func assertProcessNetworkCapability(t *testing.T, s *Server) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleProcessNetworkCapability(w, httptest.NewRequest(http.MethodGet, "/capabilities/process-network-ebpf", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("capability status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["document_type"] != "CAPABILITY_EVIDENCE" || body["capability"] != "process_network_ebpf" {
		t.Fatalf("unexpected capability evidence: %v", body)
	}
	return body
}

func wJSON(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDeterministicReasonOrdering(t *testing.T) {
	prov := healthyProvider()
	prov.atCapacity = true
	prov.activeCount = 1
	prov.quarantineAtCapacity = true
	prov.unknownSequence = true
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	d1 := s.evaluateReadiness()
	d2 := s.evaluateReadiness()
	if strings.Join(d1.reasons, ",") != strings.Join(d2.reasons, ",") {
		t.Fatalf("non-deterministic reasons: %v vs %v", d1.reasons, d2.reasons)
	}
	wantPrefix := []string{"QUEUE_FULL", "QUEUE_QUARANTINE_FULL", "QUEUE_UNKNOWN_SEQUENCE_BLOCK"}
	for i, code := range wantPrefix {
		if i >= len(d1.reasons) || d1.reasons[i] != code {
			t.Fatalf("reason order=%v want prefix %v", d1.reasons, wantPrefix)
		}
	}
}

func TestReadyNotReadyWhileShuttingDown(t *testing.T) {
	s, err := NewServer("127.0.0.1:0", healthyProvider(), nil,
		WithIdentity("0.1.0-test", "agent-shutdown-ready"))
	if err != nil {
		t.Fatal(err)
	}
	s.shuttingDown.Store(true)

	assertReady(t, s, http.StatusServiceUnavailable, "NOT_READY", false, "AGENT_SHUTTING_DOWN")
	wAlias := httptest.NewRecorder()
	s.handleReady(wAlias, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if wAlias.Code != http.StatusServiceUnavailable {
		t.Fatalf("healthz during shutdown=%d", wAlias.Code)
	}
	wLive := httptest.NewRecorder()
	s.handleLive(wLive, httptest.NewRequest(http.MethodGet, "/live", nil))
	if wLive.Code != http.StatusOK {
		t.Fatalf("live during shutdown must remain 200, got %d", wLive.Code)
	}
	live := decodeBody(t, wLive)
	if live["status"] != "SHUTTING_DOWN" {
		t.Fatalf("live shutdown body=%v", live)
	}
	healthBody := assertHealth(t, s, http.StatusServiceUnavailable, "SHUTTING_DOWN", "AGENT_SHUTTING_DOWN")
	_ = healthBody
}

func TestLiveAndReadyCanonicalEnvelopes(t *testing.T) {
	s, err := NewServer("127.0.0.1:0", healthyProvider(), nil,
		WithIdentity("0.1.0-test", "agent-instance-test"))
	if err != nil {
		t.Fatal(err)
	}

	wLive := httptest.NewRecorder()
	s.handleLive(wLive, httptest.NewRequest(http.MethodGet, "/live", nil))
	if wLive.Code != http.StatusOK {
		t.Fatalf("live=%d", wLive.Code)
	}
	live := decodeBody(t, wLive)
	for _, key := range []string{"status", "component", "ts", "service_instance_id"} {
		if _, ok := live[key]; !ok {
			t.Fatalf("live missing %s: %v", key, live)
		}
	}
	if live["status"] != "UP" || live["component"] != "platform-agent" {
		t.Fatalf("live body=%v", live)
	}
	if _, exists := live["runtime"]; exists {
		t.Fatalf("live must not emit legacy runtime field: %v", live)
	}

	assertReady(t, s, http.StatusOK, "UP", true, "")
}

func TestHealthServerNonLoopbackRejection(t *testing.T) {
	_, err := NewServer("0.0.0.0:8089", nil, nil)
	if err == nil {
		t.Fatal("expected error on 0.0.0.0 non-loopback address")
	}
}

func TestHealthServerLifecycle(t *testing.T) {
	s, err := NewServer("127.0.0.1:0", healthyProvider(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestHealthzMuxAliasIsReadiness(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	s, err := NewServer(addr, healthyProvider(), nil,
		WithIdentity("0.1.0-test", "agent-mux-test"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	}()

	var resp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err = http.Get("http://" + addr + "/healthz")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("healthz mux probe failed: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status=%d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["ready"] != true || body["http_semantic"] != float64(200) {
		t.Fatalf("healthz readiness body=%v", body)
	}
	if _, ok := body["document_type"]; ok {
		t.Fatalf("healthz must not be SUPPORT_EVIDENCE: %v", body)
	}
}

func TestConcurrentSnapshotReadinessRace(t *testing.T) {
	prov := healthyProvider()
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			prov.mu.Lock()
			prov.lifecycle = "RUNNING"
			prov.receiverAvailable = false
			prov.mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			prov.mu.Lock()
			prov.lifecycle = "STOPPING"
			prov.receiverAvailable = true
			prov.mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			_ = s.evaluateReadiness()
			w := httptest.NewRecorder()
			s.handleReady(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
			s.handleHealth(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
			s.handleLive(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/live", nil))
		}()
	}
	wg.Wait()
}

// countingSnapshotProvider counts RuntimeSnapshot calls for request-consistency proofs.
type countingSnapshotProvider struct {
	mu    sync.Mutex
	calls int
	snap  RuntimeSnapshot
}

func (p *countingSnapshotProvider) RuntimeSnapshot() RuntimeSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.snap
}

func (p *countingSnapshotProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// alternatingSnapshotProvider returns distinct snapshots on successive calls.
type alternatingSnapshotProvider struct {
	mu     sync.Mutex
	calls  int
	first  RuntimeSnapshot
	second RuntimeSnapshot
}

func (p *alternatingSnapshotProvider) RuntimeSnapshot() RuntimeSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.calls == 1 {
		return p.first
	}
	return p.second
}

func (p *alternatingSnapshotProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func degradedSafeSnapshot() RuntimeSnapshot {
	return RuntimeSnapshot{
		Lifecycle:         "RUNNING",
		SchedulerRunning:  true,
		EnabledCollectors: 1,
		Collectors: []CollectorSnapshot{{
			Name:    "cpu",
			Enabled: true,
		}},
		Queue: queue.QueueStats{
			ActiveCount: 3,
			ActiveBytes: 512,
		},
		Delivery: delivery.DeliveryStats{
			ReceiverAvailable: false,
			Attempts:          4,
			Successes:         1,
			LastErrorCode:     "SENSOR_TRANSPORT_UNAVAILABLE",
			LastFailureClass:  client.FailureTemporary,
		},
	}
}

func hardUnsafeSnapshot() RuntimeSnapshot {
	return RuntimeSnapshot{
		Lifecycle:         "RUNNING",
		SchedulerRunning:  true,
		EnabledCollectors: 1,
		Collectors: []CollectorSnapshot{{
			Name:    "cpu",
			Enabled: true,
		}},
		Queue: queue.QueueStats{
			ActiveCount: 99,
			ActiveBytes: 4096,
			AtCapacity:  true,
		},
		Delivery: delivery.DeliveryStats{
			ReceiverAvailable: true,
			Attempts:          10,
			Successes:         9,
		},
	}
}

func TestHealthSingleRuntimeSnapshotPerRequest(t *testing.T) {
	prov := &countingSnapshotProvider{snap: healthyProvider().RuntimeSnapshot()}
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertHealth(t, s, http.StatusOK, "UP", "")
	if got := prov.callCount(); got != 1 {
		t.Fatalf("RuntimeSnapshot calls=%d want=1", got)
	}
}

func TestHealthConsumesOnlyFirstCapturedSnapshotDegradedThenUnsafe(t *testing.T) {
	prov := &alternatingSnapshotProvider{
		first:  degradedSafeSnapshot(),
		second: hardUnsafeSnapshot(),
	}
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := assertHealth(t, s, http.StatusOK, "DEGRADED", "SENSOR_TRANSPORT_UNAVAILABLE")
	if got := prov.callCount(); got != 1 {
		t.Fatalf("RuntimeSnapshot calls=%d want=1 (second hard-unsafe must not be consumed)", got)
	}
	if body["queue_depth"] != float64(3) {
		t.Fatalf("queue_depth mixed second snapshot: %v", body["queue_depth"])
	}
	if body["delivery_status"] != "FAILED" {
		t.Fatalf("delivery_status=%v want=FAILED from first snapshot", body["delivery_status"])
	}
	deps, _ := body["dependency_status"].(map[string]any)
	if deps["sensor_transport"] != "DOWN" {
		t.Fatalf("sensor_transport=%v want=DOWN from first snapshot", deps["sensor_transport"])
	}
	if _, hasCap := body["queue_capacity"]; hasCap {
		t.Fatalf("queue_capacity must not appear from second hard-unsafe snapshot: %v", body)
	}
	counters, _ := body["counters"].(map[string]any)
	if counters["queue_active_count"] != float64(3) || counters["delivery_attempts"] != float64(4) {
		t.Fatalf("counters mixed second snapshot: %v", counters)
	}
	raw, _ := json.Marshal(body["reason_codes"])
	if strings.Contains(string(raw), "QUEUE_FULL") {
		t.Fatalf("reason_codes mixed second hard-unsafe snapshot: %s", raw)
	}
}

func TestHealthConsumesOnlyFirstCapturedSnapshotUnsafeThenSafe(t *testing.T) {
	prov := &alternatingSnapshotProvider{
		first:  hardUnsafeSnapshot(),
		second: degradedSafeSnapshot(),
	}
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := assertHealth(t, s, http.StatusServiceUnavailable, "NOT_READY", "QUEUE_FULL")
	if got := prov.callCount(); got != 1 {
		t.Fatalf("RuntimeSnapshot calls=%d want=1 (second safe snapshot must not be consumed)", got)
	}
	if body["queue_depth"] != float64(99) {
		t.Fatalf("queue_depth mixed second snapshot: %v", body["queue_depth"])
	}
	if body["queue_capacity"] != float64(99) {
		t.Fatalf("queue_capacity=%v want=99 from first hard-unsafe snapshot", body["queue_capacity"])
	}
	deps, _ := body["dependency_status"].(map[string]any)
	if deps["sensor_transport"] != "UP" {
		t.Fatalf("sensor_transport=%v want=UP from first snapshot", deps["sensor_transport"])
	}
	counters, _ := body["counters"].(map[string]any)
	if counters["queue_active_count"] != float64(99) || counters["delivery_attempts"] != float64(10) {
		t.Fatalf("counters mixed second snapshot: %v", counters)
	}
	raw, _ := json.Marshal(body["reason_codes"])
	if strings.Contains(string(raw), "SENSOR_TRANSPORT_UNAVAILABLE") {
		t.Fatalf("reason_codes mixed second degraded snapshot: %s", raw)
	}
}

func TestHealthShutdownCaptureConsistentWithComponentState(t *testing.T) {
	t.Run("false_then_flip_true_during_snapshot", func(t *testing.T) {
		var s *Server
		prov := &shutdownFlipProvider{
			snap: healthyProvider().RuntimeSnapshot(),
			flip: func() {
				s.shuttingDown.Store(true)
			},
		}
		var err error
		s, err = NewServer("127.0.0.1:0", prov, nil)
		if err != nil {
			t.Fatal(err)
		}
		// Capture starts with shuttingDown=false; provider flips during snapshot.
		// Decision and component_state must both use the captured false value.
		body := assertHealth(t, s, http.StatusOK, "UP", "")
		if body["component_state"] != "UP" {
			t.Fatalf("component_state=%v want=UP from captured shutdown=false", body["component_state"])
		}
		if s.shuttingDown.Load() != true {
			t.Fatal("provider did not flip shuttingDown during snapshot")
		}
		if got := prov.callCount(); got != 1 {
			t.Fatalf("RuntimeSnapshot calls=%d want=1", got)
		}
	})
	t.Run("true_then_flip_false_during_snapshot", func(t *testing.T) {
		var s *Server
		prov := &shutdownFlipProvider{
			snap: healthyProvider().RuntimeSnapshot(),
			flip: func() {
				s.shuttingDown.Store(false)
			},
		}
		var err error
		s, err = NewServer("127.0.0.1:0", prov, nil)
		if err != nil {
			t.Fatal(err)
		}
		s.shuttingDown.Store(true)
		body := assertHealth(t, s, http.StatusServiceUnavailable, "SHUTTING_DOWN", "AGENT_SHUTTING_DOWN")
		if body["component_state"] != "SHUTTING_DOWN" {
			t.Fatalf("component_state=%v want=SHUTTING_DOWN from captured shutdown=true", body["component_state"])
		}
		if s.shuttingDown.Load() != false {
			t.Fatal("provider did not clear shuttingDown during snapshot")
		}
		if got := prov.callCount(); got != 1 {
			t.Fatalf("RuntimeSnapshot calls=%d want=1", got)
		}
	})
}

// shutdownFlipProvider flips Server.shuttingDown while returning a snapshot,
// proving readiness and component_state share one request capture.
type shutdownFlipProvider struct {
	mu    sync.Mutex
	calls int
	snap  RuntimeSnapshot
	flip  func()
}

func (p *shutdownFlipProvider) RuntimeSnapshot() RuntimeSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.flip != nil {
		p.flip()
	}
	return p.snap
}

func (p *shutdownFlipProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestIssue302AdmissionHealthCannotMaskLossOrRecovery(t *testing.T) {
	cases := []struct {
		name          string
		admission     scheduler.AdmissionStatus
		code          int
		state, reason string
	}{
		{"backlog", scheduler.AdmissionStatus{BacklogFragments: 1, BacklogBytes: 512, BacklogObservations: 3}, 200, "DEGRADED", "COLLECTION_ADMISSION_BACKLOG"},
		{"observation_loss", scheduler.AdmissionStatus{DroppedObservations: 2}, 200, "DEGRADED", "COLLECTION_ADMISSION_LOSS"},
		{"error_loss", scheduler.AdmissionStatus{DroppedErrors: 1}, 200, "DEGRADED", "COLLECTION_ADMISSION_LOSS"},
		{"recovery", scheduler.AdmissionStatus{RecoveryRequired: true}, 503, "NOT_READY", "COLLECTION_ADMISSION_RECOVERY_REQUIRED"},
		{"uncertain", scheduler.AdmissionStatus{UncertainObservations: 1}, 503, "NOT_READY", "COLLECTION_ADMISSION_RECOVERY_REQUIRED"},
		{"write_blocked", scheduler.AdmissionStatus{WriteBlocked: true, BacklogFragments: 1}, 503, "NOT_READY", "COLLECTION_ADMISSION_BLOCKED"},
		{"blocked_with_loss", scheduler.AdmissionStatus{WriteBlocked: true, BacklogFragments: 1, DroppedObservations: 2}, 503, "NOT_READY", "COLLECTION_ADMISSION_BLOCKED"},
		{"recovery_with_backlog", scheduler.AdmissionStatus{RecoveryRequired: true, BacklogFragments: 1}, 503, "NOT_READY", "COLLECTION_ADMISSION_RECOVERY_REQUIRED"},
		{"recovered_capacity", scheduler.AdmissionStatus{FailedAttempts: 1, RejectedAttempts: 1, DurableObservations: 3}, 200, "UP", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := healthyProvider()
			p.admission = tc.admission
			s, err := NewServer("127.0.0.1:0", p, nil)
			if err != nil {
				t.Fatal(err)
			}
			readyBody := assertReady(t, s, tc.code, tc.state, tc.code == 200, tc.reason)
			body := assertHealth(t, s, tc.code, tc.state, tc.reason)
			w := httptest.NewRecorder()
			s.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
			if w.Code != tc.code {
				t.Fatalf("healthz code=%d want=%d", w.Code, tc.code)
			}
			for _, evidence := range []map[string]any{readyBody, body, decodeBody(t, w)} {
				// /ready and /healthz use reason_code alone for a single reason;
				// reason_codes is optional until secondary reasons are present.
				reasons, _ := json.Marshal([]any{evidence["reason_code"], evidence["reason_codes"]})
				if tc.admission.BacklogFragments > 0 && !strings.Contains(string(reasons), "COLLECTION_ADMISSION_BACKLOG") {
					t.Fatalf("secondary backlog evidence missing: %s", reasons)
				}
				if tc.admission.DroppedObservations+tc.admission.DroppedErrors > 0 && !strings.Contains(string(reasons), "COLLECTION_ADMISSION_LOSS") {
					t.Fatalf("secondary loss evidence missing: %s", reasons)
				}
			}
			counters := body["counters"].(map[string]any)
			if counters["admission_dropped_observations"] != float64(tc.admission.DroppedObservations) || counters["admission_backlog_bytes"] != float64(tc.admission.BacklogBytes) {
				t.Fatalf("lost counters: %v", counters)
			}
			assertLiveOK(t, s)
		})
	}
	if boundedCounter(^uint64(0)) != int64(1<<53-1) {
		t.Fatal("counter wrapped")
	}
}
