package ebpf

import (
	"encoding/binary"
	"testing"
	"time"

	"platform/agent/common/collector"
)

func putEventV1(t *testing.T, mut func([]byte)) []byte {
	t.Helper()
	b := make([]byte, ProcessNetRecordV1Size)
	order := binary.LittleEndian
	order.PutUint16(b[0:2], ProcessNetABIV1)
	order.PutUint16(b[2:4], ProcessNetRecordV1Size)
	b[4] = EventTypeInetState
	b[5] = 6 // TCP
	order.PutUint16(b[6:8], 2)
	order.PutUint64(b[8:16], 1000)
	order.PutUint32(b[16:20], 42)
	order.PutUint32(b[20:24], 42)
	order.PutUint16(b[24:26], 12345)
	order.PutUint16(b[26:28], 443)
	order.PutUint32(b[28:32], 2) // SYN_SENT
	order.PutUint32(b[32:36], 1) // ESTABLISHED
	copy(b[36:40], []byte{127, 0, 0, 1})
	copy(b[52:56], []byte{203, 0, 113, 10})
	copy(b[68:84], []byte("curl\x00"))
	if mut != nil {
		mut(b)
	}
	return b
}

func TestDecoderValidIPv4Open(t *testing.T) {
	raw := putEventV1(t, nil)
	ev, err := decodeRawRecord(raw, time.Unix(0, 0).UTC(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Class != collector.ProcessNetworkEventOpen || ev.Protocol != collector.ProcessNetworkProtocolTCP {
		t.Fatalf("got class=%s proto=%s", ev.Class, ev.Protocol)
	}
	if ev.RemoteIP.String() != "203.0.113.10" || ev.Comm != "curl" {
		t.Fatalf("remote=%s comm=%q", ev.RemoteIP, ev.Comm)
	}
	syn, err := toSynthetic(ev)
	if err != nil {
		t.Fatal(err)
	}
	if syn.RemoteIP != "203.0.113.10" || syn.ProcessName != "curl" || syn.PID == nil {
		t.Fatalf("%+v", syn)
	}
}

func TestDecoderValidIPv6(t *testing.T) {
	raw := putEventV1(t, func(b []byte) {
		binary.LittleEndian.PutUint16(b[6:8], 10)
		for i := 36; i < 68; i++ {
			b[i] = 0
		}
		// 2001:db8::1
		b[52] = 0x20
		b[53] = 0x01
		b[54] = 0x0d
		b[55] = 0xb8
		b[67] = 0x01
	})
	ev, err := decodeRawRecord(raw, time.Now().UTC(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !ev.RemoteIP.Is6() {
		t.Fatalf("want v6 got %s", ev.RemoteIP)
	}
}

func TestDecoderHostileInputs(t *testing.T) {
	cases := []struct {
		name string
		mut  func([]byte)
	}{
		{"bad_abi", func(b []byte) { binary.LittleEndian.PutUint16(b[0:2], 99) }},
		{"bad_size", func(b []byte) { binary.LittleEndian.PutUint16(b[2:4], 4) }},
		{"bad_type", func(b []byte) { b[4] = 9 }},
		{"bad_family", func(b []byte) { binary.LittleEndian.PutUint16(b[6:8], 99) }},
		{"unspecified_remote", func(b []byte) {
			for i := 52; i < 56; i++ {
				b[i] = 0
			}
		}},
		{"bad_comm", func(b []byte) { b[68] = 0xff; b[69] = 0xfe }},
		{"bad_proto", func(b []byte) { b[5] = 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := putEventV1(t, tc.mut)
			if _, err := decodeRawRecord(raw, time.Now().UTC(), 0); err == nil {
				t.Fatal("expected reject")
			}
		})
	}
	if _, err := decodeRawRecord([]byte{1, 2, 3}, time.Now().UTC(), 0); err == nil {
		t.Fatal("short reject")
	}
}

func TestDecoderRetransmit(t *testing.T) {
	raw := putEventV1(t, func(b []byte) {
		b[4] = EventTypeTCPRexmit
		binary.LittleEndian.PutUint32(b[84:88], FlagOptionalProbe)
	})
	ev, err := decodeRawRecord(raw, time.Now().UTC(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Class != collector.ProcessNetworkEventRetransmit || !ev.Optional {
		t.Fatalf("%+v", ev)
	}
	syn, err := toSynthetic(ev)
	if err != nil || syn.RetransmitCount == nil || *syn.RetransmitCount != 1 {
		t.Fatalf("%+v %v", syn, err)
	}
}

func TestDecoderNoFabricatedZeroEvidence(t *testing.T) {
	raw := putEventV1(t, nil)
	ev, err := decodeRawRecord(raw, time.Now().UTC(), 0)
	if err != nil {
		t.Fatal(err)
	}
	syn, err := toSynthetic(ev)
	if err != nil {
		t.Fatal(err)
	}
	if syn.BytesTx != nil || syn.BytesRx != nil || syn.RTTUs != nil {
		t.Fatalf("fabricated metrics: %+v", syn)
	}
}
