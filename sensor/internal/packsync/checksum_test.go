package packsync

import (
	"strings"
	"testing"
)

func TestComputeDesiredChecksumGoldenVectors(t *testing.T) {
	if got := ComputeDesiredChecksum(nil); got != "" {
		t.Fatalf("empty nil: got %q", got)
	}
	if got := ComputeDesiredChecksum([]AppliedPackRef{}); got != "" {
		t.Fatalf("empty slice: got %q", got)
	}

	a := AppliedPackRef{PackID: "fixture.a", PackVersion: "1.0.0", ManifestSHA256: strings.Repeat("a", 64)}
	b := AppliedPackRef{PackID: "fixture.b", PackVersion: "2.0.0", ManifestSHA256: strings.Repeat("b", 64)}
	forward := ComputeDesiredChecksum([]AppliedPackRef{a, b})
	reverse := ComputeDesiredChecksum([]AppliedPackRef{b, a})
	const wantAB = "10cb4cc9eefa76882b871d611309910ef80d5d2a9848d9e7efcad3687816253b"
	if forward != wantAB || reverse != wantAB {
		t.Fatalf("order-independent checksum: forward=%s reverse=%s want=%s", forward, reverse, wantAB)
	}
	if len(forward) != 64 {
		t.Fatalf("checksum length: got %d", len(forward))
	}

	ambiguous := AppliedPackRef{PackID: "ab", PackVersion: "c", ManifestSHA256: strings.Repeat("d", 64)}
	split := AppliedPackRef{PackID: "a", PackVersion: "bc", ManifestSHA256: strings.Repeat("d", 64)}
	if ComputeDesiredChecksum([]AppliedPackRef{ambiguous}) == ComputeDesiredChecksum([]AppliedPackRef{split}) {
		t.Fatal("length prefix must prevent delimiter ambiguity")
	}
	const wantAmbiguous = "aab8d19a484c6341ccc386d1ae927943844f11dda738e821ec6b3031ec711fd1"
	const wantSplit = "7c5369498c14f808167a4f857052fd026b8b4ac75c9e6c3b4264ee20d805eb67"
	if got := ComputeDesiredChecksum([]AppliedPackRef{ambiguous}); got != wantAmbiguous {
		t.Fatalf("ambiguous: got %s want %s", got, wantAmbiguous)
	}
	if got := ComputeDesiredChecksum([]AppliedPackRef{split}); got != wantSplit {
		t.Fatalf("split: got %s want %s", got, wantSplit)
	}

	v1 := AppliedPackRef{PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: strings.Repeat("a", 64)}
	v2 := AppliedPackRef{PackID: "fixture.pack", PackVersion: "1.0.1", ManifestSHA256: strings.Repeat("a", 64)}
	const wantV1 = "6c46e505799e42f9a6ec01092063f6f44ce6b9a945ea75cd511b87aca90c49ee"
	const wantV2 = "c1a174e55ee63f8cc8f337133f245f4e4ca9dd0b2ef3d2488527f996be489e68"
	if got := ComputeDesiredChecksum([]AppliedPackRef{v1}); got != wantV1 {
		t.Fatalf("v1: got %s want %s", got, wantV1)
	}
	if got := ComputeDesiredChecksum([]AppliedPackRef{v2}); got != wantV2 {
		t.Fatalf("v2: got %s want %s", got, wantV2)
	}
	if wantV1 == wantV2 {
		t.Fatal("version change must alter checksum")
	}
}
