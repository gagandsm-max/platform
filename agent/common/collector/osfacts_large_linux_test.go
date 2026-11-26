//go:build linux

package collector

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestOSFactsCompleteRecords(t *testing.T) {
	const budget = 4 << 20
	const facts = "btime 1700000000\nprocs_running 4\nprocs_blocked 17\n"
	var cpu strings.Builder
	for i := 0; i < 1536; i++ {
		fmt.Fprintf(&cpu, "cpu%d 142953005 179814 77215431 1646430653 4453721 19874 1174722 8276 0 0\n", i)
	}
	// Align fake fact text at the start of a continuation buffer, inside intr.
	fragment := "intr" + strings.Repeat(" ", 4096-4) + "btime 1700000000\n"
	prefix := func(n int) string { return "intr " + strings.Repeat("0", n-6) + "\n" }
	for _, tc := range []struct {
		name  string
		input string
		want  bool
	}{
		{"ordinary complete records", facts, true},
		{"large CPU list and long IRQ row", cpu.String() + "intr 4" + strings.Repeat(" 0", 100000) + "\n" + facts, true},
		{"last newline exactly at budget", prefix(budget-len(facts)) + facts, true},
		{"numeric suffix beyond budget", prefix(budget-len(facts)+2) + facts, false},
		{"cut numeric suffix on EOF", strings.TrimSuffix(facts, "7\n"), false},
		{"continuation is not a fact", fragment + "procs_running 4\nprocs_blocked 17\n", false},
		{"missing fact beyond budget", strings.Repeat("0", budget) + "\n" + facts, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.NewReader(tc.input)
			got, ok := parseOSFacts(src)
			if ok != tc.want || (ok && (got.btime != 1700000000 || got.procsRunning != 4 || got.procsBlocked != 17)) {
				t.Errorf("got %+v, complete=%v, want complete=%v", got, ok, tc.want)
			}
			if n := src.Size() - int64(src.Len()); n > budget {
				t.Errorf("read %d bytes beyond %d-byte budget", n, budget)
			}
		})
	}
}

func TestOSFactsStopsAtLastRequiredFact(t *testing.T) {
	const facts = "btime 1700000000\nprocs_running 4\nprocs_blocked 0\n"
	tail := strings.NewReader(strings.Repeat(" 0", 100000))
	got, ok := parseOSFacts(io.MultiReader(strings.NewReader(facts), tail))
	if !ok || got.procsBlocked != 0 || tail.Len() < 190000 {
		t.Fatalf("complete=%v blocked=%d unused tail=%d", ok, got.procsBlocked, tail.Len())
	}
	broken := io.MultiReader(strings.NewReader(strings.TrimSuffix(facts, "\n")), iotest.ErrReader(errors.New("read failed")))
	if _, ok := parseOSFacts(broken); ok {
		t.Fatal("accepted an unfinished record after a read error")
	}
}
