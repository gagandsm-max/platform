package packsync

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"sort"
)

// ComputeDesiredChecksum mirrors Core SensorPackDesiredChecksum.compute.
// Empty pack sets yield "" (Core returns null; JSON/Go empty string).
// Assignments are sorted by packId ascending. Each row contributes
// length-prefixed UTF-8 fields packId, packVersion, manifestSha256
// (4-byte big-endian length + bytes). Output is lowercase hex SHA-256.
func ComputeDesiredChecksum(packs []AppliedPackRef) string {
	if len(packs) == 0 {
		return ""
	}
	sorted := append([]AppliedPackRef(nil), packs...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].PackID < sorted[j].PackID
	})
	h := sha256.New()
	var lenBuf [4]byte
	for _, row := range sorted {
		writeLengthPrefixed(h, lenBuf[:], row.PackID)
		writeLengthPrefixed(h, lenBuf[:], row.PackVersion)
		writeLengthPrefixed(h, lenBuf[:], row.ManifestSHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeLengthPrefixed(h hash.Hash, lenBuf []byte, value string) {
	b := []byte(value)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(b)))
	_, _ = h.Write(lenBuf)
	_, _ = h.Write(b)
}
