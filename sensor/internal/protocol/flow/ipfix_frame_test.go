package flow

import (
	"encoding/binary"
	"errors"
	"math/rand"
	"reflect"
	"testing"
)

func ipfixEnvelopeFixture(sets ...[]byte) []byte {
	wire := make([]byte, ipfixMessageHeaderLength)
	binary.BigEndian.PutUint16(wire[:2], 10)
	binary.BigEndian.PutUint32(wire[4:8], 0xfedcba98)
	binary.BigEndian.PutUint32(wire[8:12], 0xfffffffe)
	binary.BigEndian.PutUint32(wire[12:16], 0xdeadbeef)
	for _, set := range sets {
		wire = append(wire, set...)
	}
	if len(wire) <= ipfixMaxMessageLength {
		binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)))
	}
	return wire
}

func ipfixFrameFixture(id uint16, body []byte) []byte {
	wire := make([]byte, ipfixSetHeaderLength+len(body))
	binary.BigEndian.PutUint16(wire[0:2], id)
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)))
	copy(wire[4:], body)
	return wire
}

func TestIPFIXEnvelopeOrderedSetsAndUntrustedMetadata(t *testing.T) {
	wire := ipfixEnvelopeFixture(
		ipfixFrameFixture(2, []byte{1, 2, 3, 4}),
		ipfixFrameFixture(256, []byte{5, 6, 7, 8, 9}),
		ipfixFrameFixture(3, []byte{0, 0, 0, 0}),
	)
	env, err := ParseIPFIXEnvelope(wire)
	if err != nil {
		t.Fatal(err)
	}
	if env.Version != 10 || int(env.Length) != len(wire) || env.ExportTime != 0xfedcba98 || env.SequenceNumber != 0xfffffffe || env.ObservationDomainID != 0xdeadbeef {
		t.Fatalf("header mismatch: %+v", env)
	}
	want := []IPFIXSet{{2, 16, 8}, {256, 24, 9}, {3, 33, 8}}
	if !reflect.DeepEqual(env.Sets, want) {
		t.Fatalf("sets = %+v, want %+v", env.Sets, want)
	}
	for i := range wire {
		wire[i] = 0
	}
	if !reflect.DeepEqual(env.Sets, want) || env.SequenceNumber != 0xfffffffe {
		t.Fatal("envelope retained mutable input")
	}
}

func TestIPFIXEnvelopePermitsZeroSetsAndOpaqueReservedIDs(t *testing.T) {
	zero := ipfixEnvelopeFixture()
	env, err := ParseIPFIXEnvelope(zero)
	if err != nil || len(env.Sets) != 0 {
		t.Fatalf("RFC 7011 permits zero Sets: env=%+v err=%v", env, err)
	}
	wire := ipfixEnvelopeFixture(
		ipfixFrameFixture(0, nil),
		ipfixFrameFixture(1, nil),
		ipfixFrameFixture(4, nil),
		ipfixFrameFixture(255, nil),
	)
	env, err = ParseIPFIXEnvelope(wire)
	if err != nil || len(env.Sets) != 4 {
		t.Fatalf("opaque reserved Sets should be framed: %+v %v", env, err)
	}
	for i, id := range []uint16{0, 1, 4, 255} {
		if env.Sets[i].ID != id {
			t.Fatalf("set %d ID %d, want %d", i, env.Sets[i].ID, id)
		}
	}
}

func TestIPFIXEnvelopeMalformedLengthsFailAtomically(t *testing.T) {
	valid := ipfixEnvelopeFixture(ipfixFrameFixture(256, []byte{1, 2, 3, 4}))
	cases := map[string][]byte{
		"empty":            nil,
		"short_header":     valid[:15],
		"short_set_header": ipfixEnvelopeFixture([]byte{0, 2, 0}),
		"oversized":        make([]byte, 65536),
	}
	wrongVersion := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(wrongVersion[:2], 9)
	cases["unsupported_version"] = wrongVersion
	shorter := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(shorter[2:4], uint16(len(shorter)-1))
	cases["header_shorter_than_data"] = shorter
	longer := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(longer[2:4], uint16(len(longer)+1))
	cases["header_longer_than_data"] = longer
	zeroLength := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(zeroLength[18:20], 0)
	cases["zero_set_length"] = zeroLength
	tinySet := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(tinySet[18:20], 3)
	cases["set_shorter_than_header"] = tinySet
	tooLargeSet := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(tooLargeSet[18:20], 65535)
	cases["set_overruns_message"] = tooLargeSet
	truncatedLast := ipfixEnvelopeFixture(ipfixFrameFixture(256, nil), []byte{0, 2, 0})
	cases["partial_final_set"] = truncatedLast
	for name, wire := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseIPFIXEnvelope(wire)
			if got != nil || err == nil {
				t.Fatalf("want nil and error, got %+v, %v", got, err)
			}
			if name == "unsupported_version" {
				if !errors.Is(err, ErrUnsupportedIPFIXVersion) {
					t.Fatalf("wrong version error %v", err)
				}
			} else if !errors.Is(err, ErrMalformedIPFIXEnvelope) {
				t.Fatalf("wrong malformed error %v", err)
			}
		})
	}
}

func TestIPFIXEnvelopeMaximumMessageAndDenseSets(t *testing.T) {
	// RFC 7011's maximum declared message length is 65,535 octets.
	max := ipfixEnvelopeFixture(ipfixFrameFixture(256, make([]byte, 65515)))
	if len(max) != 65535 {
		t.Fatalf("maximum fixture got %d", len(max))
	}
	got, err := ParseIPFIXEnvelope(max)
	if err != nil || len(got.Sets) != 1 || got.Sets[0].Length != 65519 {
		t.Fatalf("maximum message: %+v %v", got, err)
	}
	// Dense sets allocate metadata only after all boundaries are validated.
	wire := make([]byte, 65532)
	binary.BigEndian.PutUint16(wire[:2], 10)
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)))
	for off := 16; off < len(wire); off += 4 {
		binary.BigEndian.PutUint16(wire[off:off+2], 256)
		binary.BigEndian.PutUint16(wire[off+2:off+4], 4)
	}
	got, err = ParseIPFIXEnvelope(wire)
	if err != nil || len(got.Sets) != (len(wire)-16)/4 {
		t.Fatalf("dense message: sets=%d err=%v", len(got.Sets), err)
	}
	wire[len(wire)-2] = 0
	wire[len(wire)-1] = 3
	got, err = ParseIPFIXEnvelope(wire)
	if got != nil || !errors.Is(err, ErrMalformedIPFIXEnvelope) {
		t.Fatalf("invalid final Set leaked partial result: %+v %v", got, err)
	}
}

func FuzzIPFIXEnvelope(f *testing.F) {
	f.Add(ipfixEnvelopeFixture())
	f.Add(ipfixEnvelopeFixture(ipfixFrameFixture(2, []byte{0, 0, 0, 0})))
	f.Add([]byte{10})
	f.Fuzz(func(t *testing.T, wire []byte) {
		env, err := ParseIPFIXEnvelope(wire)
		if err != nil {
			if env != nil {
				t.Fatal("malformed input returned partial envelope")
			}
			return
		}
		if env == nil || int(env.Length) != len(wire) || len(wire) < 16 {
			t.Fatal("invalid accepted envelope length")
		}
		last := 16
		for _, set := range env.Sets {
			if int(set.Offset) != last || set.Length < 4 || int(set.Offset)+int(set.Length) > len(wire) {
				t.Fatalf("invalid accepted Set: %+v", set)
			}
			last += int(set.Length)
		}
		if last != len(wire) {
			t.Fatal("Set descriptors did not cover message")
		}
	})
}

func TestIPFIXEnvelopeDeterministicMutations(t *testing.T) {
	seed := rand.New(rand.NewSource(7011))
	for n := 0; n < 500; n++ {
		body := make([]byte, seed.Intn(700))
		if _, err := seed.Read(body); err != nil {
			t.Fatal(err)
		}
		wire := ipfixEnvelopeFixture(ipfixFrameFixture(256, body))
		env, err := ParseIPFIXEnvelope(wire)
		if err != nil || len(env.Sets) != 1 || env.Sets[0].Length != uint16(len(body)+4) {
			t.Fatalf("mutation %d valid frame failed: %+v %v", n, env, err)
		}
		broken := append([]byte(nil), wire...)
		binary.BigEndian.PutUint16(broken[18:20], uint16(len(broken)))
		if v, err := ParseIPFIXEnvelope(broken); v != nil || err == nil {
			t.Fatalf("mutation %d invalid length accepted: %+v %v", n, v, err)
		}
	}
}
