package flow

import (
	"encoding/binary"
	"errors"
	"math/rand"
	"reflect"
	"testing"
)

func ipfixRawTestField(id, width uint16, enterprise uint32, withEnterprise bool) []byte {
	b := make([]byte, 4)
	if withEnterprise {
		id |= 0x8000
	}
	binary.BigEndian.PutUint16(b[:2], id)
	binary.BigEndian.PutUint16(b[2:4], width)
	if withEnterprise {
		p := make([]byte, 4)
		binary.BigEndian.PutUint32(p, enterprise)
		b = append(b, p...)
	}
	return b
}

func ipfixRawTestDefinition(id uint16, options bool, scopes uint16, fields ...[]byte) []byte {
	n := 4
	if options {
		n = 6
	}
	b := make([]byte, n)
	binary.BigEndian.PutUint16(b[:2], id)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(fields)))
	if options {
		binary.BigEndian.PutUint16(b[4:6], scopes)
	}
	for _, f := range fields {
		b = append(b, f...)
	}
	return b
}

func ipfixRawTestWithdraw(id uint16) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[:2], id)
	return b
}

func ipfixRawTestSet(id uint16, body []byte) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[:2], id)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(body)+4))
	return append(b, body...)
}

func ipfixRawTestWire(domain uint32, sets ...[]byte) []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint16(b[:2], 10)
	binary.BigEndian.PutUint32(b[4:8], 1700000000)
	binary.BigEndian.PutUint32(b[12:16], domain)
	for _, set := range sets {
		b = append(b, set...)
	}
	if len(b) <= 65535 {
		binary.BigEndian.PutUint16(b[2:4], uint16(len(b)))
	}
	return b
}

func TestIPFIXRawTemplatesOrderedFieldsEnterpriseScopesAndOwnership(t *testing.T) {
	regular := ipfixRawTestDefinition(256, false, 0,
		ipfixRawTestField(8, 4, 0, false),
		ipfixRawTestField(45, 65535, 424242, true),
		ipfixRawTestField(8, 4, 0, false), // Repeated IE is real wire evidence.
	)
	options := ipfixRawTestDefinition(257, true, 2,
		ipfixRawTestField(141, 4, 0, false),
		ipfixRawTestField(123, 2, 333, true),
		ipfixRawTestField(41, 8, 0, false),
	)
	// Interleaving Data Sets does not decode or install template authority.
	wire := ipfixRawTestWire(0xdeadbeef,
		ipfixRawTestSet(2, regular),
		ipfixRawTestSet(256, []byte{1, 2, 3, 4}),
		ipfixRawTestSet(3, append(options, 0, 0)), // Valid 2-octet padding.
		ipfixRawTestSet(2, regular),               // Duplicate definition preserved in wire order.
	)
	got, err := ParseIPFIXRawTemplates(wire)
	if err != nil || got == nil {
		t.Fatalf("valid mixed wire: %+v %v", got, err)
	}
	if got.ObservationDomainID != 0xdeadbeef || got.ExportTime != 1700000000 || len(got.Templates) != 3 {
		t.Fatalf("untrusted header or definition count: %+v", got)
	}
	regularFields := []IPFIXRawField{
		{ElementID: 8, Length: 4},
		{ElementID: 45, Length: 65535, EnterpriseNumber: 424242, Enterprise: true},
		{ElementID: 8, Length: 4},
	}
	optionsFields := []IPFIXRawField{
		{ElementID: 141, Length: 4},
		{ElementID: 123, Length: 2, EnterpriseNumber: 333, Enterprise: true},
		{ElementID: 41, Length: 8},
	}
	for i, want := range []IPFIXRawTemplate{
		{ID: 256, Fields: regularFields},
		{ID: 257, Options: true, ScopeCount: 2, Fields: optionsFields},
		{ID: 256, Fields: regularFields},
	} {
		if !reflect.DeepEqual(got.Templates[i], want) {
			t.Fatalf("template %d: got %+v want %+v", i, got.Templates[i], want)
		}
	}
	for i := range wire {
		wire[i] = 0
	}
	if !reflect.DeepEqual(got.Templates[1].Fields, optionsFields) || !reflect.DeepEqual(got.Templates[2].Fields, regularFields) {
		t.Fatal("returned records alias caller datagram")
	}
	// An append to one descriptor group cannot overwrite the next record.
	prior := got.Templates[1].Fields[0]
	got.Templates[0].Fields = append(got.Templates[0].Fields, IPFIXRawField{ElementID: 999})
	if got.Templates[1].Fields[0] != prior {
		t.Fatal("a record's descriptor slice can overwrite another record")
	}
}

func TestIPFIXRawWithdrawalsAreEvidenceOnly(t *testing.T) {
	wire := ipfixRawTestWire(0,
		ipfixRawTestSet(2, append(ipfixRawTestWithdraw(256), ipfixRawTestWithdraw(2)...)),
		ipfixRawTestSet(3, append(ipfixRawTestWithdraw(257), ipfixRawTestWithdraw(3)...)),
		ipfixRawTestSet(2, ipfixRawTestDefinition(256, false, 0, ipfixRawTestField(8, 4, 0, false))),
	)
	got, err := ParseIPFIXRawTemplates(wire)
	if err != nil || got == nil {
		t.Fatalf("valid withdrawals: %+v %v", got, err)
	}
	if got.ObservationDomainID != 0 || len(got.Templates) != 5 {
		t.Fatalf("withdrawals not preserved or ODID=0 refused: %+v", got)
	}
	for i, want := range []IPFIXRawTemplate{
		{ID: 256, Withdrawal: true},
		{ID: 2, Withdrawal: true, AllTemplates: true},
		{ID: 257, Options: true, Withdrawal: true},
		{ID: 3, Options: true, Withdrawal: true, AllTemplates: true},
		{ID: 256, Fields: []IPFIXRawField{{ElementID: 8, Length: 4}}},
	} {
		if !reflect.DeepEqual(got.Templates[i], want) {
			t.Fatalf("withdrawal %d got %+v want %+v", i, got.Templates[i], want)
		}
	}
}

func TestIPFIXRawDataOnlyAndHeaderOnlyHaveNoDefinitions(t *testing.T) {
	for _, wire := range [][]byte{
		ipfixRawTestWire(3),
		ipfixRawTestWire(3, ipfixRawTestSet(256, []byte{1, 2}), ipfixRawTestSet(0, []byte{0, 0, 0, 0})),
	} {
		got, err := ParseIPFIXRawTemplates(wire)
		if err != nil || got == nil || len(got.Templates) != 0 {
			t.Fatalf("data-only message yielded definitions: %+v %v", got, err)
		}
	}
}

func TestIPFIXRawMalformedTemplateAbortsWholeMessage(t *testing.T) {
	good := ipfixRawTestSet(2, ipfixRawTestDefinition(256, false, 0, ipfixRawTestField(8, 4, 0, false)))
	wrongCount := ipfixRawTestDefinition(257, false, 0, ipfixRawTestField(8, 4, 0, false))
	binary.BigEndian.PutUint16(wrongCount[2:4], 2)
	invalid := map[string][]byte{
		"empty_template_set":           ipfixRawTestSet(2, nil),
		"truncated_regular_header":     ipfixRawTestSet(2, []byte{0, 1}),
		"reserved_definition_id":       ipfixRawTestSet(2, ipfixRawTestDefinition(255, false, 0, ipfixRawTestField(8, 4, 0, false))),
		"zero_scope_count":             ipfixRawTestSet(3, ipfixRawTestDefinition(257, true, 0, ipfixRawTestField(8, 4, 0, false))),
		"too_many_scopes":              ipfixRawTestSet(3, ipfixRawTestDefinition(257, true, 2, ipfixRawTestField(8, 4, 0, false))),
		"truncated_options_header":     ipfixRawTestSet(3, []byte{1, 0, 0, 1, 0}),
		"truncated_field":              ipfixRawTestSet(2, []byte{1, 0, 0, 1, 0, 8, 0}),
		"truncated_enterprise_pen":     ipfixRawTestSet(2, []byte{1, 0, 0, 1, 0x80, 8, 0, 4, 0, 0}),
		"overstated_field_count":       ipfixRawTestSet(2, wrongCount),
		"nonzero_trailing_padding":     ipfixRawTestSet(3, append(ipfixRawTestDefinition(257, true, 1, ipfixRawTestField(8, 4, 0, false)), 0, 1)),
		"invalid_withdrawal_id":        ipfixRawTestSet(2, ipfixRawTestWithdraw(255)),
		"invalid_options_withdraw_all": ipfixRawTestSet(3, ipfixRawTestWithdraw(2)),
		"bogus_full_four_byte_padding": ipfixRawTestSet(2, append(ipfixRawTestDefinition(256, false, 0, ipfixRawTestField(8, 4, 0, false)), 0, 0, 0, 0)),
	}
	for name, bad := range invalid {
		t.Run(name, func(t *testing.T) {
			// A valid preceding definition must never leak if a later Set is malformed.
			pkt, err := ParseIPFIXRawTemplates(ipfixRawTestWire(3, good, bad))
			if pkt != nil || !errors.Is(err, ErrMalformedIPFIXRawTemplate) {
				t.Fatalf("atomic rejection expected; got %+v %v", pkt, err)
			}
		})
	}
	badFrame := ipfixRawTestWire(3, good)
	binary.BigEndian.PutUint16(badFrame[2:4], uint16(len(badFrame)-1))
	if pkt, err := ParseIPFIXRawTemplates(badFrame); pkt != nil || !errors.Is(err, ErrMalformedIPFIXEnvelope) {
		t.Fatalf("invalid enclosing IPFIX envelope accepted: %+v %v", pkt, err)
	}
}

func TestIPFIXRawTemplateCapacityAndPaddingLimits(t *testing.T) {
	// A dense one-field template exercises bounded memory without relying on
	// exporter-reported Header SequenceNumber or template ID monotonicity.
	field := ipfixRawTestField(8, 4, 0, false)
	body := make([]byte, 0, 65000)
	for len(body)+8 <= 65000 {
		body = append(body, ipfixRawTestDefinition(256, false, 0, field)...)
	}
	wire := ipfixRawTestWire(1234, ipfixRawTestSet(2, body))
	pkt, err := ParseIPFIXRawTemplates(wire)
	if err != nil || pkt == nil || len(pkt.Templates) != len(body)/8 {
		t.Fatalf("dense templates rejected or wrong count: %+v %v", pkt, err)
	}
	// All valid trailing template padding is 1..3 zero octets, shorter than
	// the smallest record header; nonzero padding must always be refused.
	for i := 1; i <= 3; i++ {
		set := ipfixRawTestSet(2, append(ipfixRawTestDefinition(256, false, 0, field), make([]byte, i)...))
		got, err := ParseIPFIXRawTemplates(ipfixRawTestWire(42, set))
		if err != nil || got == nil || len(got.Templates) != 1 {
			t.Fatalf("padding len %d: %+v %v", i, got, err)
		}
	}
}

func FuzzIPFIXRawTemplates(f *testing.F) {
	f.Add(ipfixRawTestWire(11, ipfixRawTestSet(2, ipfixRawTestDefinition(256, false, 0, ipfixRawTestField(8, 4, 0, false)))))
	f.Add(ipfixRawTestWire(12, ipfixRawTestSet(3, ipfixRawTestDefinition(257, true, 1, ipfixRawTestField(123, 65535, 888, true)))))
	f.Add(ipfixRawTestWire(13, ipfixRawTestSet(2, ipfixRawTestWithdraw(2))))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, wire []byte) {
		result, err := ParseIPFIXRawTemplates(wire)
		if err != nil {
			if result != nil {
				t.Fatalf("error leaked partial result: %+v", result)
			}
			return
		}
		if result == nil || len(wire) < 16 {
			t.Fatal("nil result for accepted IPFIX frame")
		}
		for _, def := range result.Templates {
			if def.Withdrawal {
				if len(def.Fields) != 0 || def.ScopeCount != 0 {
					t.Fatal("withdrawal acquired data fields")
				}
			} else if len(def.Fields) == 0 || (def.Options && (def.ScopeCount == 0 || int(def.ScopeCount) > len(def.Fields))) {
				t.Fatal("invalid accepted descriptor geometry")
			}
		}
	})
}

func TestIPFIXRawTemplateRandomizedRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(70110))
	for run := 0; run < 300; run++ {
		count := rng.Intn(24) + 1
		body := []byte{}
		for i := 0; i < count; i++ {
			body = append(body, ipfixRawTestDefinition(uint16(256+i), false, 0,
				ipfixRawTestField(uint16(1+rng.Intn(32000)), uint16(1+rng.Intn(1500)), rng.Uint32(), rng.Intn(2) == 0))...)
		}
		wire := ipfixRawTestWire(uint32(run), ipfixRawTestSet(2, body))
		pkt, err := ParseIPFIXRawTemplates(wire)
		if err != nil || pkt == nil || len(pkt.Templates) != count {
			t.Fatalf("run %d valid input rejected: %+v %v", run, pkt, err)
		}
		broken := append([]byte(nil), wire...)
		binary.BigEndian.PutUint16(broken[22:24], 65535)
		if pkt, err := ParseIPFIXRawTemplates(broken); pkt != nil || !errors.Is(err, ErrMalformedIPFIXRawTemplate) {
			t.Fatalf("run %d malformed descriptor not rejected: %+v %v", run, pkt, err)
		}
	}
}
