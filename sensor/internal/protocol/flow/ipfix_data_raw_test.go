package flow

import (
	"encoding/binary"
	"errors"
	"math/rand"
	"reflect"
	"testing"
)

func ipfixRawDataTestSet(id uint16, body []byte) []byte {
	b := make([]byte, 4+len(body))
	binary.BigEndian.PutUint16(b[0:2], id)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(b)))
	copy(b[4:], body)
	return b
}

func ipfixRawDataFixedTemplate(id uint16, widths ...uint16) IPFIXRawTemplate {
	fields := make([]IPFIXRawField, len(widths))
	for i, w := range widths {
		fields[i] = IPFIXRawField{ElementID: uint16(8 + i), Length: w}
	}
	return IPFIXRawTemplate{ID: id, Fields: fields}
}

func TestIPFIXRawDataFixedVariableEnterpriseOptionsOwnership(t *testing.T) {
	tmpl := IPFIXRawTemplate{
		ID:         256,
		Options:    true,
		ScopeCount: 2,
		Fields: []IPFIXRawField{
			{ElementID: 141, Length: 4},
			{ElementID: 123, Length: 2, EnterpriseNumber: 333, Enterprise: true},
			{ElementID: 45, Length: 65535},
			{ElementID: 8, Length: 4},
			{ElementID: 8, Length: 4}, // Ordered repeated IE.
			{ElementID: 99, Length: 65535, EnterpriseNumber: 424242, Enterprise: true},
		},
	}
	// Record 1: scopes 0x0a0b0c0d / 0x1111; var len=5 "hello"; two IPv4; enterprise var via 3-byte prefix len=0.
	body := []byte{
		0x0a, 0x0b, 0x0c, 0x0d,
		0x11, 0x11,
		5, 'h', 'e', 'l', 'l', 'o',
		1, 2, 3, 4,
		5, 6, 7, 8,
		255, 0, 0, // zero-length variable via three-byte encoding
	}
	// Record 2: scopes; one-byte zero var; fixed; fixed; one-byte var "xy"
	body = append(body,
		0, 0, 0, 9,
		0x22, 0x22,
		0, // zero-length variable via one-byte encoding
		9, 9, 9, 9,
		8, 8, 8, 8,
		2, 'x', 'y',
	)
	body = append(body, 0, 0) // Valid padding shorter than min record length.
	set := ipfixRawDataTestSet(256, body)
	got, err := ParseIPFIXRawDataSet(set, tmpl, IPFIXRawDataLimits{})
	if err != nil || got == nil {
		t.Fatalf("valid options data: %+v %v", got, err)
	}
	if got.TemplateID != 256 || !got.Options || got.ScopeCount != 2 || len(got.Records) != 2 {
		t.Fatalf("header/scope/record count: %+v", got)
	}
	r0 := got.Records[0].Fields
	if len(r0) != 6 || !reflect.DeepEqual(r0[0].Value, []byte{0x0a, 0x0b, 0x0c, 0x0d}) {
		t.Fatalf("scope0: %+v", r0[0])
	}
	if !r0[1].Enterprise || r0[1].EnterpriseNumber != 333 || !reflect.DeepEqual(r0[1].Value, []byte{0x11, 0x11}) {
		t.Fatalf("enterprise scope: %+v", r0[1])
	}
	if r0[2].Length != 65535 || string(r0[2].Value) != "hello" {
		t.Fatalf("short variable: %+v", r0[2])
	}
	if !reflect.DeepEqual(r0[3].Value, []byte{1, 2, 3, 4}) || !reflect.DeepEqual(r0[4].Value, []byte{5, 6, 7, 8}) {
		t.Fatalf("repeated fixed IE: %+v %+v", r0[3], r0[4])
	}
	if !r0[5].Enterprise || r0[5].EnterpriseNumber != 424242 || len(r0[5].Value) != 0 {
		t.Fatalf("three-byte zero variable: %+v", r0[5])
	}
	r1 := got.Records[1].Fields
	if len(r1[2].Value) != 0 || string(r1[5].Value) != "xy" {
		t.Fatalf("record1 variables: %+v", r1)
	}
	for i := range set {
		set[i] = 0xff
	}
	if string(got.Records[0].Fields[2].Value) != "hello" || string(got.Records[1].Fields[5].Value) != "xy" {
		t.Fatal("returned values alias caller set bytes")
	}
	prior := got.Records[1].Fields[0]
	got.Records[0].Fields = append(got.Records[0].Fields, IPFIXRawDataField{ElementID: 999})
	if got.Records[1].Fields[0].ElementID != prior.ElementID || !reflect.DeepEqual(got.Records[1].Fields[0].Value, prior.Value) {
		t.Fatal("append on one record aliases another record's field storage")
	}

	// Value append isolation: cap==len so append must not mutate sibling values.
	hello := got.Records[0].Fields[2].Value
	if cap(hello) != len(hello) {
		t.Fatalf("value cap!=len: len=%d cap=%d", len(hello), cap(hello))
	}
	_ = append(hello, '!')
	if string(got.Records[0].Fields[2].Value) != "hello" {
		t.Fatal("append mutated owned value storage")
	}
	if string(got.Records[1].Fields[5].Value) != "xy" {
		t.Fatal("append on one value mutated another field")
	}

	// Caller template mutation must not rewrite already-decoded metadata.
	tmpl.Fields[0].ElementID = 0
	tmpl.Fields[0].Length = 1
	tmpl.Fields[1].EnterpriseNumber = 0
	tmpl.Fields[1].Enterprise = false
	tmpl.ScopeCount = 99
	tmpl.Options = false
	if got.Records[0].Fields[0].ElementID != 141 || got.Records[0].Fields[0].Length != 4 {
		t.Fatalf("caller template mutation leaked into result: %+v", got.Records[0].Fields[0])
	}
	if !got.Records[0].Fields[1].Enterprise || got.Records[0].Fields[1].EnterpriseNumber != 333 {
		t.Fatalf("caller enterprise mutation leaked: %+v", got.Records[0].Fields[1])
	}
	if !got.Options || got.ScopeCount != 2 {
		t.Fatalf("caller options/scope mutation leaked: %+v", got)
	}
}

func TestIPFIXRawDataPaddingVersusMinimumRecord(t *testing.T) {
	tmpl := ipfixRawDataFixedTemplate(300, 4, 4)
	// minLen=8; one record + 3 zero pad octets is legal; 1..7 pad after a record OK.
	for pad := 0; pad <= 7; pad++ {
		body := append([]byte{1, 2, 3, 4, 5, 6, 7, 8}, make([]byte, pad)...)
		got, err := ParseIPFIXRawDataSet(ipfixRawDataTestSet(300, body), tmpl, IPFIXRawDataLimits{})
		if err != nil || got == nil || len(got.Records) != 1 {
			t.Fatalf("pad %d: %+v %v", pad, got, err)
		}
	}
	// Eight trailing zeros are a second all-zero record, not padding.
	body := append([]byte{1, 2, 3, 4, 5, 6, 7, 8}, make([]byte, 8)...)
	got, err := ParseIPFIXRawDataSet(ipfixRawDataTestSet(300, body), tmpl, IPFIXRawDataLimits{})
	if err != nil || got == nil || len(got.Records) != 2 {
		t.Fatalf("all-zero record discarded as padding: %+v %v", got, err)
	}
	if !reflect.DeepEqual(got.Records[1].Fields[0].Value, []byte{0, 0, 0, 0}) {
		t.Fatalf("zero fixed values not preserved: %+v", got.Records[1])
	}
	// Non-zero padding is illegal under §3.3.1 collector acceptance here.
	bad := append([]byte{1, 2, 3, 4, 5, 6, 7, 8}, 0, 1)
	if pkt, err := ParseIPFIXRawDataSet(ipfixRawDataTestSet(300, bad), tmpl, IPFIXRawDataLimits{}); pkt != nil || !errors.Is(err, ErrMalformedIPFIXRawData) {
		t.Fatalf("nonzero padding accepted: %+v %v", pkt, err)
	}
}

func TestIPFIXRawDataVariableLengthEncodingsAndTruncation(t *testing.T) {
	tmpl := IPFIXRawTemplate{ID: 400, Fields: []IPFIXRawField{{ElementID: 45, Length: 65535}}}
	// One-byte length 3.
	got, err := ParseIPFIXRawDataSet(ipfixRawDataTestSet(400, []byte{3, 'a', 'b', 'c'}), tmpl, IPFIXRawDataLimits{})
	if err != nil || got == nil || len(got.Records) != 1 || string(got.Records[0].Fields[0].Value) != "abc" {
		t.Fatalf("one-byte variable: %+v %v", got, err)
	}
	// One-byte length 254 (max short prefix).
	payload254 := make([]byte, 254)
	for i := range payload254 {
		payload254[i] = byte(i)
	}
	got, err = ParseIPFIXRawDataSet(ipfixRawDataTestSet(400, append([]byte{254}, payload254...)), tmpl, IPFIXRawDataLimits{})
	if err != nil || got == nil || !reflect.DeepEqual(got.Records[0].Fields[0].Value, payload254) {
		t.Fatalf("prefix 254: %+v %v", got, err)
	}
	// Three-byte length 255 (extended short).
	payload255 := make([]byte, 255)
	for i := range payload255 {
		payload255[i] = byte(i)
	}
	got, err = ParseIPFIXRawDataSet(ipfixRawDataTestSet(400, append([]byte{255, 0, 255}, payload255...)), tmpl, IPFIXRawDataLimits{})
	if err != nil || got == nil || !reflect.DeepEqual(got.Records[0].Fields[0].Value, payload255) {
		t.Fatalf("three-byte length 255: %+v %v", got, err)
	}
	// Three-byte length 256.
	payload := make([]byte, 256)
	for i := range payload {
		payload[i] = byte(i)
	}
	body := append([]byte{255, 1, 0}, payload...)
	got, err = ParseIPFIXRawDataSet(ipfixRawDataTestSet(400, body), tmpl, IPFIXRawDataLimits{})
	if err != nil || got == nil || !reflect.DeepEqual(got.Records[0].Fields[0].Value, payload) {
		t.Fatalf("three-byte variable: %+v %v", got, err)
	}
	// Three-byte encoding of a short length (0) remains valid; empty Value.
	got, err = ParseIPFIXRawDataSet(ipfixRawDataTestSet(400, []byte{255, 0, 0}), tmpl, IPFIXRawDataLimits{})
	if err != nil || got == nil || len(got.Records) != 1 || len(got.Records[0].Fields[0].Value) != 0 {
		t.Fatalf("three-byte empty variable: %+v %v", got, err)
	}
	invalid := map[string][]byte{
		"truncated_one_byte_value":   {5, 1, 2},
		"truncated_prefix_254":      {254, 1, 2},
		"truncated_three_byte_hdr":  {255, 1},
		"truncated_three_byte_value": append([]byte{255, 0, 4}, 1, 2, 3),
		"truncated_length_65535":    func() []byte {
			b := make([]byte, 1+2+10)
			b[0] = 255
			binary.BigEndian.PutUint16(b[1:3], 65535)
			return b
		}(),
		"empty_body": {},
	}
	for name, bad := range invalid {
		t.Run(name, func(t *testing.T) {
			pkt, err := ParseIPFIXRawDataSet(ipfixRawDataTestSet(400, bad), tmpl, IPFIXRawDataLimits{})
			if pkt != nil || !errors.Is(err, ErrMalformedIPFIXRawData) {
				t.Fatalf("expected malformed; got %+v %v", pkt, err)
			}
		})
	}
}

func TestIPFIXRawDataVariableMinimumPadding(t *testing.T) {
	// Variable IE minimum record length is 1 (empty value via one-byte prefix).
	tmpl := IPFIXRawTemplate{ID: 410, Fields: []IPFIXRawField{{ElementID: 45, Length: 65535}}}
	got, err := ParseIPFIXRawDataSet(ipfixRawDataTestSet(410, []byte{0}), tmpl, IPFIXRawDataLimits{})
	if err != nil || got == nil || len(got.Records) != 1 || len(got.Records[0].Fields[0].Value) != 0 {
		t.Fatalf("empty variable record: %+v %v", got, err)
	}
	// After a complete record, zero padding shorter than minLen=1 is impossible;
	// a trailing zero is itself another empty variable record.
	got, err = ParseIPFIXRawDataSet(ipfixRawDataTestSet(410, []byte{3, 'a', 'b', 'c', 0}), tmpl, IPFIXRawDataLimits{})
	if err != nil || got == nil || len(got.Records) != 2 {
		t.Fatalf("trailing empty variable record: %+v %v", got, err)
	}
	// Mixed fixed+variable: minLen=5; 1..4 zero pad octets after one record OK.
	mixed := IPFIXRawTemplate{ID: 411, Fields: []IPFIXRawField{
		{ElementID: 8, Length: 4},
		{ElementID: 45, Length: 65535},
	}}
	for pad := 0; pad <= 4; pad++ {
		body := append([]byte{1, 2, 3, 4, 0}, make([]byte, pad)...)
		got, err := ParseIPFIXRawDataSet(ipfixRawDataTestSet(411, body), mixed, IPFIXRawDataLimits{})
		if err != nil || got == nil || len(got.Records) != 1 {
			t.Fatalf("var min pad %d: %+v %v", pad, got, err)
		}
	}
	if pkt, err := ParseIPFIXRawDataSet(ipfixRawDataTestSet(411, []byte{1, 2, 3, 4, 0, 0x5a}), mixed, IPFIXRawDataLimits{}); pkt != nil || !errors.Is(err, ErrMalformedIPFIXRawData) {
		t.Fatalf("nonzero var-min padding accepted: %+v %v", pkt, err)
	}
}

func TestIPFIXRawDataRejectsInvalidWithdrawnMismatchedTemplates(t *testing.T) {
	body := []byte{1, 2, 3, 4}
	set := ipfixRawDataTestSet(256, body)
	valid := IPFIXRawTemplate{ID: 256, Fields: []IPFIXRawField{{ElementID: 8, Length: 4}}}
	cases := map[string]IPFIXRawTemplate{
		"withdrawal":       {ID: 256, Withdrawal: true, Fields: []IPFIXRawField{{ElementID: 8, Length: 4}}},
		"withdraw_all":     {ID: 256, AllTemplates: true, Fields: []IPFIXRawField{{ElementID: 8, Length: 4}}},
		"empty_fields":     {ID: 256},
		"reserved_id":      {ID: 255, Fields: []IPFIXRawField{{ElementID: 8, Length: 4}}},
		"zero_fixed":       {ID: 256, Fields: []IPFIXRawField{{ElementID: 8, Length: 0}}},
		"options_no_scope": {ID: 256, Options: true, Fields: []IPFIXRawField{{ElementID: 8, Length: 4}}},
		"too_many_scopes":  {ID: 256, Options: true, ScopeCount: 2, Fields: []IPFIXRawField{{ElementID: 8, Length: 4}}},
		"scope_on_data":    {ID: 256, ScopeCount: 1, Fields: []IPFIXRawField{{ElementID: 8, Length: 4}}},
		"pen_without_bit":  {ID: 256, Fields: []IPFIXRawField{{ElementID: 8, Length: 4, EnterpriseNumber: 9}}},
		"element_id_0x8000": {ID: 256, Fields: []IPFIXRawField{{ElementID: 0x8000, Length: 4}}},
		"element_id_high":   {ID: 256, Fields: []IPFIXRawField{{ElementID: 0x8001, Length: 4}}},
	}
	for name, tmpl := range cases {
		t.Run(name, func(t *testing.T) {
			pkt, err := ParseIPFIXRawDataSet(set, tmpl, IPFIXRawDataLimits{})
			if pkt != nil || !errors.Is(err, ErrInvalidIPFIXRawDataTemplate) {
				t.Fatalf("invalid template accepted: %+v %v", pkt, err)
			}
		})
	}
	mismatch := IPFIXRawTemplate{ID: 257, Fields: []IPFIXRawField{{ElementID: 8, Length: 4}}}
	if pkt, err := ParseIPFIXRawDataSet(set, mismatch, IPFIXRawDataLimits{}); pkt != nil || !errors.Is(err, ErrMalformedIPFIXRawData) {
		t.Fatalf("mismatched template id accepted: %+v %v", pkt, err)
	}
	if pkt, err := ParseIPFIXRawDataSet(ipfixRawDataTestSet(2, body), valid, IPFIXRawDataLimits{}); pkt != nil || !errors.Is(err, ErrMalformedIPFIXRawData) {
		t.Fatalf("template-set id accepted as data: %+v %v", pkt, err)
	}
	short := []byte{1, 0, 0, 3}
	if pkt, err := ParseIPFIXRawDataSet(short, valid, IPFIXRawDataLimits{}); pkt != nil || !errors.Is(err, ErrMalformedIPFIXRawData) {
		t.Fatalf("length mismatch accepted: %+v %v", pkt, err)
	}
	if _, err := ParseIPFIXRawDataSet(nil, valid, IPFIXRawDataLimits{}); !errors.Is(err, ErrMalformedIPFIXRawData) {
		t.Fatalf("nil set: %v", err)
	}
}

func TestIPFIXRawDataRejectsExcessiveDescriptors(t *testing.T) {
	fields := make([]IPFIXRawField, ipfixMaxRawDataFields+1)
	for i := range fields {
		fields[i] = IPFIXRawField{ElementID: 8, Length: 1}
	}
	tmpl := IPFIXRawTemplate{ID: 256, Fields: fields}
	pkt, err := ParseIPFIXRawDataSet(ipfixRawDataTestSet(256, []byte{1}), tmpl, IPFIXRawDataLimits{})
	if pkt != nil || !errors.Is(err, ErrInvalidIPFIXRawDataTemplate) {
		t.Fatalf("65536 descriptors accepted: %+v %v", pkt, err)
	}
	// Exact 65535 descriptors with impossible geometry is still rejected
	// without scanning (minLen would exceed max Set body).
	fields = fields[:ipfixMaxRawDataFields]
	tmpl.Fields = fields
	pkt, err = ParseIPFIXRawDataSet(ipfixRawDataTestSet(256, []byte{1}), tmpl, IPFIXRawDataLimits{})
	if pkt != nil || !errors.Is(err, ErrInvalidIPFIXRawDataTemplate) {
		t.Fatalf("65535x1-byte minLen not rejected as impossible: %+v %v", pkt, err)
	}
}

func TestIPFIXRawDataCheckedGeometryNoPanic(t *testing.T) {
	set := ipfixRawDataTestSet(256, []byte{1, 2, 3, 4})
	// Synthetic excessive fixed widths must not panic via int overflow.
	wide := IPFIXRawTemplate{ID: 256, Fields: []IPFIXRawField{
		{ElementID: 8, Length: 65534},
		{ElementID: 9, Length: 65534},
		{ElementID: 10, Length: 65534},
	}}
	pkt, err := ParseIPFIXRawDataSet(set, wide, IPFIXRawDataLimits{})
	if pkt != nil || !errors.Is(err, ErrInvalidIPFIXRawDataTemplate) {
		t.Fatalf("excessive widths accepted: %+v %v", pkt, err)
	}
	// Single field larger than max Set body.
	huge := IPFIXRawTemplate{ID: 256, Fields: []IPFIXRawField{{ElementID: 8, Length: 65535}}} // variable OK
	if _, err := ipfixRawDataMinRecordLength(huge.Fields); err != nil {
		t.Fatalf("variable length min should be 1: %v", err)
	}
	overBody := IPFIXRawTemplate{ID: 256, Fields: []IPFIXRawField{{ElementID: 8, Length: 65516}}}
	pkt, err = ParseIPFIXRawDataSet(set, overBody, IPFIXRawDataLimits{})
	if pkt != nil || !errors.Is(err, ErrInvalidIPFIXRawDataTemplate) {
		t.Fatalf("field longer than max body accepted: %+v %v", pkt, err)
	}
	// minLen fits template check but exceeds this Set body: reject without copy.
	tall := IPFIXRawTemplate{ID: 256, Fields: []IPFIXRawField{{ElementID: 8, Length: 8}}}
	pkt, err = ParseIPFIXRawDataSet(ipfixRawDataTestSet(256, []byte{1, 2, 3, 4}), tall, IPFIXRawDataLimits{})
	if pkt != nil || !errors.Is(err, ErrMalformedIPFIXRawData) {
		t.Fatalf("body shorter than minLen accepted: %+v %v", pkt, err)
	}
}

func TestIPFIXRawDataBudgetsAndMaxMessageBoundary(t *testing.T) {
	tmpl := ipfixRawDataFixedTemplate(500, 1)
	// Exact legal enclosing Set max: message ceiling minus message header.
	body := make([]byte, ipfixMaxRawDataSetBody)
	for i := range body {
		body[i] = byte(i)
	}
	set := ipfixRawDataTestSet(500, body)
	if len(set) != ipfixMaxRawDataSetLength {
		t.Fatalf("fixture not at max legal set size: %d want %d", len(set), ipfixMaxRawDataSetLength)
	}
	got, err := ParseIPFIXRawDataSet(set, tmpl, IPFIXRawDataLimits{})
	if err != nil {
		t.Fatalf("max legal set rejected: err=%v", err)
	}
	if got == nil {
		t.Fatal("max legal set returned nil result without error")
	}
	if len(got.Records) != ipfixMaxRawDataSetBody {
		t.Fatalf("max legal set record count: got=%d want=%d", len(got.Records), ipfixMaxRawDataSetBody)
	}

	// One octet over the legal enclosing Set max must be rejected.
	overBody := make([]byte, ipfixMaxRawDataSetBody+1)
	over := ipfixRawDataTestSet(500, overBody)
	if len(over) != ipfixMaxRawDataSetLength+1 {
		t.Fatalf("oversize fixture length: %d", len(over))
	}
	if pkt, err := ParseIPFIXRawDataSet(over, tmpl, IPFIXRawDataLimits{}); pkt != nil || !errors.Is(err, ErrMalformedIPFIXRawData) {
		t.Fatalf("65520-byte set accepted: %+v %v", pkt, err)
	}

	nRecords := len(got.Records)
	totalBytes := 0
	for _, rec := range got.Records {
		for _, f := range rec.Fields {
			totalBytes += len(f.Value)
		}
	}

	// Negative soft caps reject with budget error (zero remains disable).
	if pkt, err := ParseIPFIXRawDataSet(set, tmpl, IPFIXRawDataLimits{MaxRecords: -1}); pkt != nil || !errors.Is(err, ErrIPFIXRawDataBudgetExceeded) {
		t.Fatalf("negative MaxRecords accepted: %+v %v", pkt, err)
	}
	if pkt, err := ParseIPFIXRawDataSet(set, tmpl, IPFIXRawDataLimits{MaxFieldBytes: -1}); pkt != nil || !errors.Is(err, ErrIPFIXRawDataBudgetExceeded) {
		t.Fatalf("negative MaxFieldBytes accepted: %+v %v", pkt, err)
	}

	// Exact cap accepts; N-1 rejects.
	exactRec, err := ParseIPFIXRawDataSet(set, tmpl, IPFIXRawDataLimits{MaxRecords: nRecords})
	if err != nil || exactRec == nil || len(exactRec.Records) != nRecords {
		t.Fatalf("exact MaxRecords rejected: %+v %v", exactRec, err)
	}
	if pkt, err := ParseIPFIXRawDataSet(set, tmpl, IPFIXRawDataLimits{MaxRecords: nRecords - 1}); pkt != nil || !errors.Is(err, ErrIPFIXRawDataBudgetExceeded) {
		t.Fatalf("MaxRecords N-1 not enforced: %+v %v", pkt, err)
	}
	exactBytes, err := ParseIPFIXRawDataSet(set, tmpl, IPFIXRawDataLimits{MaxFieldBytes: totalBytes})
	if err != nil || exactBytes == nil {
		t.Fatalf("exact MaxFieldBytes rejected: %+v %v", exactBytes, err)
	}
	if pkt, err := ParseIPFIXRawDataSet(set, tmpl, IPFIXRawDataLimits{MaxFieldBytes: totalBytes - 1}); pkt != nil || !errors.Is(err, ErrIPFIXRawDataBudgetExceeded) {
		t.Fatalf("MaxFieldBytes N-1 not enforced: %+v %v", pkt, err)
	}

	if pkt, err := ParseIPFIXRawDataSet(set, tmpl, IPFIXRawDataLimits{MaxRecords: 10}); pkt != nil || !errors.Is(err, ErrIPFIXRawDataBudgetExceeded) {
		t.Fatalf("MaxRecords not enforced: %+v %v", pkt, err)
	}
	if pkt, err := ParseIPFIXRawDataSet(set, tmpl, IPFIXRawDataLimits{MaxFieldBytes: 100}); pkt != nil || !errors.Is(err, ErrIPFIXRawDataBudgetExceeded) {
		t.Fatalf("MaxFieldBytes not enforced: %+v %v", pkt, err)
	}
	// minLen=1 ⇒ no trailing padding octets are legal.
	if pkt, err := ParseIPFIXRawDataSet(ipfixRawDataTestSet(500, []byte{1, 0}), tmpl, IPFIXRawDataLimits{}); pkt == nil || err != nil || len(pkt.Records) != 2 {
		t.Fatalf("zero value record treated as illegal pad: %+v %v", pkt, err)
	}
}

func TestIPFIXRawDataAtomicRejectDoesNotLeakPartial(t *testing.T) {
	tmpl := IPFIXRawTemplate{
		ID: 600,
		Fields: []IPFIXRawField{
			{ElementID: 8, Length: 4},
			{ElementID: 45, Length: 65535},
		},
	}
	// Complete first record, truncated second variable field.
	body := []byte{1, 2, 3, 4, 2, 'o', 'k', 5, 6, 7, 8, 9 /* claims 9 bytes */, 1, 2}
	pkt, err := ParseIPFIXRawDataSet(ipfixRawDataTestSet(600, body), tmpl, IPFIXRawDataLimits{})
	if pkt != nil || !errors.Is(err, ErrMalformedIPFIXRawData) {
		t.Fatalf("partial tail leaked: %+v %v", pkt, err)
	}
}

func FuzzIPFIXRawDataSet(f *testing.F) {
	tmpl := IPFIXRawTemplate{
		ID: 700,
		Fields: []IPFIXRawField{
			{ElementID: 8, Length: 4},
			{ElementID: 45, Length: 65535},
			{ElementID: 123, Length: 2, Enterprise: true, EnterpriseNumber: 9},
		},
	}
	f.Add(ipfixRawDataTestSet(700, []byte{1, 2, 3, 4, 0, 0, 9}))
	f.Add(ipfixRawDataTestSet(700, []byte{1, 2, 3, 4, 255, 0, 0, 0, 9}))
	f.Add(ipfixRawDataTestSet(700, []byte{1, 2, 3, 4, 3, 'a', 'b', 'c', 0, 1}))
	f.Add([]byte{})
	const maxRecords = 4096
	const maxFieldBytes = 1 << 20
	f.Fuzz(func(t *testing.T, set []byte) {
		result, err := ParseIPFIXRawDataSet(set, tmpl, IPFIXRawDataLimits{MaxRecords: maxRecords, MaxFieldBytes: maxFieldBytes})
		if err != nil {
			if result != nil {
				t.Fatalf("error leaked partial result: %+v", result)
			}
			return
		}
		if result == nil || result.TemplateID != 700 || len(result.Records) == 0 {
			t.Fatal("nil/empty accepted result")
		}
		if len(result.Records) > maxRecords {
			t.Fatalf("records exceed MaxRecords cap: %d", len(result.Records))
		}
		totalValueBytes := 0
		for _, rec := range result.Records {
			if len(rec.Fields) != len(tmpl.Fields) {
				t.Fatal("field count drift")
			}
			for i, field := range rec.Fields {
				desc := tmpl.Fields[i]
				if field.ElementID != desc.ElementID || field.Length != desc.Length ||
					field.Enterprise != desc.Enterprise || field.EnterpriseNumber != desc.EnterpriseNumber {
					t.Fatalf("field metadata drift: got=%+v want=%+v", field, desc)
				}
				if field.Value == nil {
					t.Fatal("nil value slice")
				}
				if cap(field.Value) != len(field.Value) {
					t.Fatalf("value cap!=len: len=%d cap=%d", len(field.Value), cap(field.Value))
				}
				if desc.Length != ipfixVariableLength && len(field.Value) != int(desc.Length) {
					t.Fatalf("fixed width mismatch: len=%d want=%d", len(field.Value), desc.Length)
				}
				totalValueBytes += len(field.Value)
			}
		}
		if totalValueBytes > maxFieldBytes {
			t.Fatalf("total value bytes exceed MaxFieldBytes: %d", totalValueBytes)
		}
	})
}

func TestIPFIXRawDataRandomizedRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(70111))
	for run := 0; run < 200; run++ {
		fieldCount := rng.Intn(6) + 1
		fields := make([]IPFIXRawField, fieldCount)
		for i := range fields {
			if rng.Intn(3) == 0 {
				fields[i] = IPFIXRawField{ElementID: uint16(100 + i), Length: 65535, Enterprise: rng.Intn(2) == 0, EnterpriseNumber: uint32(rng.Intn(1000))}
				if !fields[i].Enterprise {
					fields[i].EnterpriseNumber = 0
				}
			} else {
				fields[i] = IPFIXRawField{ElementID: uint16(100 + i), Length: uint16(1 + rng.Intn(8))}
			}
		}
		tmpl := IPFIXRawTemplate{ID: uint16(256 + run%1000), Fields: fields}
		records := rng.Intn(5) + 1
		body := []byte{}
		wantValues := make([][][]byte, records)
		for r := 0; r < records; r++ {
			wantValues[r] = make([][]byte, fieldCount)
			for i, f := range fields {
				if f.Length == 65535 {
					n := rng.Intn(20)
					val := make([]byte, n)
					rng.Read(val)
					wantValues[r][i] = val
					if rng.Intn(2) == 0 && n < 255 {
						body = append(body, byte(n))
						body = append(body, val...)
					} else {
						body = append(body, 255, byte(n>>8), byte(n))
						body = append(body, val...)
					}
				} else {
					val := make([]byte, f.Length)
					rng.Read(val)
					wantValues[r][i] = val
					body = append(body, val...)
				}
			}
		}
		pad := rng.Intn(ipfixRawDataMustMin(t, fields))
		body = append(body, make([]byte, pad)...)
		set := ipfixRawDataTestSet(tmpl.ID, body)
		got, err := ParseIPFIXRawDataSet(set, tmpl, IPFIXRawDataLimits{})
		if err != nil || got == nil || len(got.Records) != records {
			t.Fatalf("run %d valid rejected: %+v %v", run, got, err)
		}
		for r := 0; r < records; r++ {
			for i := 0; i < fieldCount; i++ {
				if !reflect.DeepEqual(got.Records[r].Fields[i].Value, wantValues[r][i]) {
					t.Fatalf("run %d record %d field %d mismatch", run, r, i)
				}
				if cap(got.Records[r].Fields[i].Value) != len(got.Records[r].Fields[i].Value) {
					t.Fatalf("run %d cap!=len", run)
				}
			}
		}
		brokenLen := append([]byte(nil), set...)
		binary.BigEndian.PutUint16(brokenLen[2:4], uint16(len(brokenLen)+1))
		if pkt, err := ParseIPFIXRawDataSet(brokenLen, tmpl, IPFIXRawDataLimits{}); pkt != nil || !errors.Is(err, ErrMalformedIPFIXRawData) {
			t.Fatalf("run %d overstated set length accepted: %+v %v", run, pkt, err)
		}
		min, _ := ipfixRawDataMinRecordLength(fields)
		if min > 1 {
			// Non-zero stub shorter than minLen cannot be padding or a record.
			stub := append(append([]byte(nil), body[:len(body)-pad]...), 0x5a)
			if pkt, err := ParseIPFIXRawDataSet(ipfixRawDataTestSet(tmpl.ID, stub), tmpl, IPFIXRawDataLimits{}); pkt != nil || !errors.Is(err, ErrMalformedIPFIXRawData) {
				t.Fatalf("run %d incomplete non-pad tail accepted: %+v %v", run, pkt, err)
			}
		}
	}
}

func ipfixRawDataMustMin(t *testing.T, fields []IPFIXRawField) int {
	t.Helper()
	min, err := ipfixRawDataMinRecordLength(fields)
	if err != nil || min < 1 {
		t.Fatalf("min length: %d %v", min, err)
	}
	return min
}
