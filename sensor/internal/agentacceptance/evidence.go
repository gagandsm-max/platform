package agentacceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/spool"
)

const dirName = "acceptance-evidence"

type Entry struct {
	SpoolStreamID string                                `json:"spoolStreamId"`
	SpoolSequence uint64                                `json:"spoolSequence"`
	PayloadSHA256 string                                `json:"payloadSha256"`
	AcceptedAtUTC string                                `json:"acceptedAtUtc"`
	Evidence      coreclient.SensorAcceptanceEvidenceV1 `json:"evidence"`
}

// Guard serializes append/finalization with missing-sidecar recovery. The OS
// releases the lock after a Sensor crash, so recovery can safely resume.
type Guard struct {
	file *os.File
	dir  string
}

func Lock(spoolDir string) (*Guard, error) {
	dir := filepath.Join(spoolDir, dirName)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("unsafe acceptance evidence directory: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Guard{file: f, dir: dir}, nil
}

func (g *Guard) Close() error {
	err := syscall.Flock(int(g.file.Fd()), syscall.LOCK_UN)
	closeErr := g.file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func path(dir string, seq uint64) string { return filepath.Join(dir, fmt.Sprintf("%020d.json", seq)) }

func (g *Guard) Persist(rec spool.Record, streamID string, acceptedAt time.Time,
	evidence coreclient.SensorAcceptanceEvidenceV1) error {
	if acceptedAt.IsZero() || rec.Sequence == 0 || streamID == "" || evidence.EvidenceVersion != 1 ||
		evidence.AcceptedAtUTC != acceptedAt.UTC().Format(time.RFC3339Nano) ||
		evidence.DurableSpoolSequence != rec.Sequence || evidence.SensorServiceInstanceID == "" {
		return errors.New("incomplete acceptance evidence")
	}
	sum := sha256.Sum256(rec.Payload)
	digest := hex.EncodeToString(sum[:])
	if digest != rec.Checksum {
		return errors.New("spool payload checksum mismatch")
	}
	entry := Entry{SpoolStreamID: streamID, SpoolSequence: rec.Sequence, PayloadSHA256: digest,
		AcceptedAtUTC: evidence.AcceptedAtUTC, Evidence: evidence}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(g.dir, ".acceptance-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0640); err == nil {
		_, err = tmp.Write(encoded)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	target := path(g.dir, rec.Sequence)
	if info, statErr := os.Lstat(target); statErr == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe acceptance evidence path")
		}
		// A persisted evidence record may never be silently rewritten.
		prior, readErr := os.ReadFile(target)
		if readErr != nil {
			return readErr
		}
		if string(prior) != string(encoded) {
			return errors.New("conflicting acceptance evidence")
		}
		return syncEvidence(target, g.dir)
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	if err := os.Rename(name, target); err != nil {
		return err
	}
	dirFile, err := os.Open(g.dir)
	if err != nil {
		return err
	}
	err = dirFile.Sync()
	_ = dirFile.Close()
	return err
}

// Resolve fails closed on a corrupt sidecar. A missing sidecar means append
// survived a crash before evidence finalization (or a pre-V021 record); persist
// conservative UNQUALIFIED evidence after durable recovery before delivery.
func (g *Guard) Resolve(rec spool.Record, streamID string) (Entry, error) {
	if streamID == "" {
		return Entry{}, errors.New("missing durable spool stream identity")
	}
	target := path(g.dir, rec.Sequence)
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		now := time.Now().UTC()
		evidence := coreclient.SensorAcceptanceEvidenceV1{EvidenceVersion: 1,
			AcceptedAtUTC: now.Format(time.RFC3339Nano), ClockQuality: "UNQUALIFIED",
			ClockSource: "POST_CRASH_RECOVERY", SensorServiceInstanceID: "recovered-unknown",
			DurableSpoolSequence: rec.Sequence}
		if err := g.Persist(rec, streamID, now, evidence); err != nil {
			return Entry{}, err
		}
		info, err = os.Lstat(target)
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Entry{}, errors.New("unsafe acceptance evidence path")
	}
	encoded, err := os.ReadFile(target)
	if err != nil {
		return Entry{}, err
	}
	var entry Entry
	if err := json.Unmarshal(encoded, &entry); err != nil {
		return Entry{}, err
	}
	sum := sha256.Sum256(rec.Payload)
	if entry.SpoolStreamID != streamID || entry.SpoolSequence != rec.Sequence ||
		entry.PayloadSHA256 != hex.EncodeToString(sum[:]) ||
		entry.PayloadSHA256 != rec.Checksum || entry.AcceptedAtUTC == "" ||
		entry.Evidence.EvidenceVersion != 1 ||
		entry.Evidence.AcceptedAtUTC != entry.AcceptedAtUTC ||
		entry.Evidence.DurableSpoolSequence != rec.Sequence ||
		entry.Evidence.SensorServiceInstanceID == "" {
		return Entry{}, errors.New("acceptance evidence does not match durable spool record")
	}
	if _, err := time.Parse(time.RFC3339Nano, entry.AcceptedAtUTC); err != nil {
		return Entry{}, err
	}
	if err := syncEvidence(target, g.dir); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func syncEvidence(target, dir string) error {
	file, err := os.Open(target)
	if err != nil {
		return err
	}
	err = file.Sync()
	_ = file.Close()
	if err != nil {
		return err
	}
	dirFile, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = dirFile.Sync()
	_ = dirFile.Close()
	return err
}

func (g *Guard) RemoveThrough(sequence uint64) error {
	entries, err := os.ReadDir(g.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		var seq uint64
		if _, err := fmt.Sscanf(entry.Name(), "%020d.json", &seq); err != nil || seq > sequence {
			continue
		}
		if !entry.Type().IsRegular() {
			return errors.New("unsafe acceptance evidence cleanup target")
		}
		if err := os.Remove(filepath.Join(g.dir, entry.Name())); err != nil {
			return err
		}
	}
	dirFile, err := os.Open(g.dir)
	if err != nil {
		return err
	}
	err = dirFile.Sync()
	_ = dirFile.Close()
	return err
}

// RemoveRange performs bounded sidecar cleanup after the matching spool ACK.
func (g *Guard) RemoveRange(first, last uint64) error {
	if first == 0 || last < first || last-first > 4096 {
		return errors.New("invalid acceptance cleanup range")
	}
	for seq := first; seq <= last; seq++ {
		target := path(g.dir, seq)
		info, err := os.Lstat(target)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe acceptance evidence cleanup target")
		}
		if err := os.Remove(target); err != nil {
			return err
		}
	}
	dirFile, err := os.Open(g.dir)
	if err != nil {
		return err
	}
	err = dirFile.Sync()
	_ = dirFile.Close()
	return err
}
