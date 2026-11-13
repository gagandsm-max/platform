//go:build linux

package agentreceiver

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const synchronizedMarker = "/run/systemd/timesync/synchronized"

type linuxClockEvidenceProvider struct{}

func productionClockEvidenceProvider() ClockEvidenceProvider { return linuxClockEvidenceProvider{} }

func (linuxClockEvidenceProvider) Sample(acceptedAt time.Time) ClockSample {
	// The kernel supplies a maximum error, while timesyncd's volatile marker
	// supplies the last successful synchronization time. Neither alone proves
	// both bounds. Other time daemons fail closed until a supported source exists.
	var tx unix.Timex
	state, err := unix.Adjtimex(&tx)
	if err != nil || state != unix.TIME_OK || tx.Status&(unix.STA_UNSYNC|unix.STA_CLOCKERR) != 0 ||
		tx.Maxerror < 0 || tx.Maxerror > 5_000_000 {
		return unqualifiedClock("LINUX_KERNEL_TIME_UNPROVEN")
	}
	fi, err := os.Lstat(synchronizedMarker)
	if err != nil || !fi.Mode().IsRegular() {
		return unqualifiedClock("LINUX_TIMESYNCD_SYNC_UNPROVEN")
	}
	age := acceptedAt.Sub(fi.ModTime())
	if age < 0 || age > 300_000*time.Millisecond {
		return unqualifiedClock("LINUX_TIMESYNCD_SYNC_UNPROVEN")
	}
	maxErrorMs := (tx.Maxerror + 999) / 1000
	syncAgeMs := int64(age / time.Millisecond)
	if age%time.Millisecond != 0 {
		syncAgeMs++
	}
	return ClockSample{
		Quality: "QUALIFIED", MaxErrorMs: &maxErrorMs, SyncAgeMs: &syncAgeMs,
		Source: "LINUX_ADJTIMEX_SYSTEMD_TIMESYNCD",
	}
}
