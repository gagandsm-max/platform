package ebpf

import "testing"

// Observational only — not BUILD-019/130 production GC/p99 certification.
func BenchmarkBackendSupportSnapshot(b *testing.B) {
	backend := testBackendInactive(b)
	backend.loss.Add(LossClassMapUpdateFail, 1)
	backend.loss.Add(LossClassDecoderReject, 2)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap := backend.SupportSnapshot()
		if !snap.Present {
			b.Fatal("expected present support snapshot")
		}
	}
}
