package syslog

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testConfig() ParserConfig {
	return ParserConfig{
		MaxInputMessageBytes:              4096,
		MaxRawEvidenceBytes:               1024,
		MaxHostnameBytes:                  128,
		MaxAppNameBytes:                   64,
		MaxProcessIDBytes:                 32,
		MaxMessageIDBytes:                 64,
		MaxMessageBytes:                   2048,
		MaxStructuredDataElements:         8,
		MaxStructuredDataParamsPerElement: 8,
		MaxStructuredDataIDBytes:          64,
		MaxStructuredDataParamNameBytes:   64,
		MaxStructuredDataParamValueBytes:  256,
		MaxSourceAddressBytes:             256,
	}
}

func mustParser(t *testing.T, cfg ParserConfig) *Parser {
	t.Helper()
	p, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("NewParser: %v", err)
	}
	return p
}

func parse(t *testing.T, p *Parser, s string) Event {
	t.Helper()
	e, err := p.Parse([]byte(s), time.Date(2026, 9, 7, 3, 30, 0, 0, time.UTC), SourceMetadata{})
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return e
}

func requireCode(t *testing.T, err error, want ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error", want)
	}
	if got := CodeOf(err); got != want {
		t.Fatalf("code=%s want=%s err=%v", got, want, err)
	}
}

func TestParserConfigRequiresExplicitPositiveBounds(t *testing.T) {
	if _, err := NewParser(ParserConfig{}); err == nil {
		t.Fatal("expected invalid config")
	}
}

func TestPRIExtremesAndMapping(t *testing.T) {
	p := mustParser(t, testConfig())
	cases := []struct{ pri, facility, severity int }{{0, 0, 0}, {191, 23, 7}, {13, 1, 5}, {160, 20, 0}}
	for _, tc := range cases {
		e := parse(t, p, "<"+itoa(tc.pri)+">partial")
		if e.Facility != tc.facility || e.Severity != tc.severity {
			t.Fatalf("PRI %d => %d/%d want %d/%d", tc.pri, e.Facility, e.Severity, tc.facility, tc.severity)
		}
	}
}

func TestEverySeverity(t *testing.T) {
	p := mustParser(t, testConfig())
	for severity := 0; severity < 8; severity++ {
		pri := 8*7 + severity
		e := parse(t, p, "<"+itoa(pri)+">x")
		if e.Severity != severity || e.Facility != 7 {
			t.Fatalf("severity mapping failed for PRI %d", pri)
		}
	}
}

func TestMalformedPRIRejected(t *testing.T) {
	p := mustParser(t, testConfig())
	cases := []string{"<192>x", "<-1>x", "<x>x", "<>", "<12x", "12>x", "<12>"}
	for _, input := range cases {
		_, err := p.Parse([]byte(input), time.Now().UTC(), SourceMetadata{})
		requireCode(t, err, CodeMalformedPRI)
	}
}

func TestRFC5424MinimalValid(t *testing.T) {
	p := mustParser(t, testConfig())
	e := parse(t, p, `<34>1 - - - - - -`)
	if e.Format != FormatRFC5424 || e.Quality != QualityGood || e.TimestampQuality != TimestampUnavailable {
		t.Fatalf("unexpected event: %+v", e)
	}
	if e.Hostname != "" || e.AppName != "" || e.ProcessID != "" || e.MessageID != "" {
		t.Fatalf("NILVALUE not normalized: %+v", e)
	}
}

func TestRFC5424FullMessageAndTimestamps(t *testing.T) {
	p := mustParser(t, testConfig())
	// RFC 5424 §6 TIME-SECFRAC is "." 1*6DIGIT; nine fractional digits are not valid.
	for _, ts := range []string{"2003-10-11T22:14:15Z", "2003-10-11T22:14:15.003123+02:00"} {
		input := `<165>1 ` + ts + ` host app 8710 ID47 [exampleSDID@32473 iut="3" eventSource="Application"] BOMAn application event log entry`
		e := parse(t, p, input)
		if e.EventTime == nil || e.TimestampQuality != TimestampAuthoritativeRFC3339 || e.RawTimestamp != ts {
			t.Fatalf("timestamp not preserved: %+v", e)
		}
		if e.Hostname != "host" || e.AppName != "app" || e.ProcessID != "8710" || e.MessageID != "ID47" {
			t.Fatalf("header fields wrong: %+v", e)
		}
		if e.Message != "BOMAn application event log entry" {
			t.Fatalf("message=%q", e.Message)
		}
		if ts == "2003-10-11T22:14:15.003123+02:00" {
			want := time.Date(2003, 10, 11, 20, 14, 15, 3123000, time.UTC)
			if !e.EventTime.Equal(want) {
				t.Fatalf("UTC conversion wrong: got=%v want=%v", e.EventTime, want)
			}
		}
	}
}

func TestRFC5424TimestampGrammar(t *testing.T) {
	p := mustParser(t, testConfig())
	received := time.Date(2026, 9, 7, 3, 30, 0, 0, time.UTC)

	positives := []struct {
		ts   string
		want time.Time
	}{
		{"2003-10-11T22:14:15Z", time.Date(2003, 10, 11, 22, 14, 15, 0, time.UTC)},
		{"2003-10-11T22:14:15.1Z", time.Date(2003, 10, 11, 22, 14, 15, 100000000, time.UTC)},
		{"2003-10-11T22:14:15.12Z", time.Date(2003, 10, 11, 22, 14, 15, 120000000, time.UTC)},
		{"2003-10-11T22:14:15.123456Z", time.Date(2003, 10, 11, 22, 14, 15, 123456000, time.UTC)},
		{"2003-10-11T22:14:15+00:00", time.Date(2003, 10, 11, 22, 14, 15, 0, time.UTC)},
		{"2003-10-11T22:14:15-00:00", time.Date(2003, 10, 11, 22, 14, 15, 0, time.UTC)},
		{"2003-10-11T22:14:15+02:00", time.Date(2003, 10, 11, 20, 14, 15, 0, time.UTC)},
		{"2003-10-11T22:14:15-05:30", time.Date(2003, 10, 12, 3, 44, 15, 0, time.UTC)},
		{"2003-10-11T22:14:15.003123+02:00", time.Date(2003, 10, 11, 20, 14, 15, 3123000, time.UTC)},
		{"2003-10-11T00:00:00Z", time.Date(2003, 10, 11, 0, 0, 0, 0, time.UTC)},
		{"2003-10-11T23:59:59Z", time.Date(2003, 10, 11, 23, 59, 59, 0, time.UTC)},
		{"2004-02-29T12:00:00Z", time.Date(2004, 2, 29, 12, 0, 0, 0, time.UTC)},
		{"2000-02-29T12:00:00Z", time.Date(2000, 2, 29, 12, 0, 0, 0, time.UTC)},
		{"2003-10-11T22:14:15+23:59", time.Date(2003, 10, 10, 22, 15, 15, 0, time.UTC)},
	}
	for _, tc := range positives {
		input := `<34>1 ` + tc.ts + ` h a p m -`
		e, err := p.Parse([]byte(input), received, SourceMetadata{})
		if err != nil {
			t.Fatalf("valid timestamp rejected %q: %v", tc.ts, err)
		}
		if e.Format != FormatRFC5424 || e.Quality != QualityGood {
			t.Fatalf("unexpected event for %q: %+v", tc.ts, e)
		}
		if e.EventTime == nil || e.TimestampQuality != TimestampAuthoritativeRFC3339 || e.RawTimestamp != tc.ts {
			t.Fatalf("timestamp quality/raw lost for %q: %+v", tc.ts, e)
		}
		if !e.EventTime.Equal(tc.want) {
			t.Fatalf("UTC mismatch for %q: got=%v want=%v", tc.ts, e.EventTime, tc.want)
		}
	}

	// NILVALUE remains distinct from an authoritative timestamp.
	e := parse(t, p, `<34>1 - h a p m -`)
	if e.EventTime != nil || e.RawTimestamp != "" || e.TimestampQuality != TimestampUnavailable {
		t.Fatalf("NILVALUE timestamp semantics broken: %+v", e)
	}

	negatives := []string{
		"2003-10-11T22:14:15.0031234Z",   // 7 fractional digits
		"2003-10-11T22:14:15.00312345Z",  // 8 fractional digits
		"2003-10-11T22:14:15.003123456Z", // 9 fractional digits (Go RFC3339Nano-permissive)
		"2003-10-11T22:14:15.003123456+02:00",
		"2003-10-11T22:14:15,003Z", // comma fraction
		"2003-10-11T22:14:15.",     // missing fraction digits
		"2003-10-11t22:14:15Z",     // lowercase T
		"2003-10-11T22:14:15z",     // lowercase Z
		"2003-10-11T2:14:15Z",      // single-digit hour
		"2003-10-11T22:14:15+24:00",
		"2003-10-11T22:14:15+01:60",
		"2003-10-11T22:14:15-24:00",
		"2003-10-11T22:14:15+02:0",
		"2003-10-11T22:14:15+2:00",
		"2003-10-11T22:14:15", // missing offset
		"2003-10-11T24:00:00Z",
		"2003-10-11T22:60:00Z",
		"2003-10-11T22:14:60Z", // leap second rejected
		"2003-02-29T12:00:00Z", // non-leap day
		"2003-13-01T12:00:00Z",
		"2003-10-32T12:00:00Z",
		"2003-04-31T12:00:00Z",
		"2003-10-11T22:14:15Zx",
		"2003-10-11T22:14:15.1234567Z",
	}
	for _, ts := range negatives {
		input := `<34>1 ` + ts + ` h a p m -`
		ev, err := p.Parse([]byte(input), received, SourceMetadata{})
		requireCode(t, err, CodeMalformedRFC5424)
		if ev.EventTime != nil || ev.RawTimestamp != "" || ev.Format != "" || ev.Quality != "" || ev.Message != "" || len(ev.StructuredData) != 0 {
			t.Fatalf("expected zero Event for invalid timestamp %q, got %+v", ts, ev)
		}
	}
}

func TestRFC5424StructuredDataOrderMultipleAndEscapes(t *testing.T) {
	p := mustParser(t, testConfig())
	input := `<34>1 2003-10-11T22:14:15Z host app 123 ID [id1 a="b" q="a\"b" slash="a\\b" bracket="a\]b"][id2 c="d"] message`
	e := parse(t, p, input)
	if len(e.StructuredData) != 2 || e.StructuredData[0].ID != "id1" || e.StructuredData[1].ID != "id2" {
		t.Fatalf("element order lost: %+v", e.StructuredData)
	}
	params := e.StructuredData[0].Params
	if len(params) != 4 {
		t.Fatalf("params=%+v", params)
	}
	want := []string{"b", `a"b`, `a\b`, `a]b`}
	for i := range want {
		if params[i].Value != want[i] {
			t.Fatalf("param %d=%q want=%q", i, params[i].Value, want[i])
		}
	}
}

func TestRFC5424StructuredDataMalformed(t *testing.T) {
	p := mustParser(t, testConfig())
	cases := []string{
		`<34>1 - host app 1 ID x`,
		`<34>1 - host app 1 ID [id a="b"`,
		`<34>1 - host app 1 ID [id a=b]`,
		`<34>1 - host app 1 ID [id a="b]c"]`,
		`<34>1 - host app 1 ID [id a="bad\q"]`,
	}
	for _, input := range cases {
		_, err := p.Parse([]byte(input), time.Now().UTC(), SourceMetadata{})
		requireCode(t, err, CodeStructuredData)
	}
}

func TestRFC5424StructuredDataBounds(t *testing.T) {
	cfg := testConfig()
	p := mustParser(t, cfg)
	cfg.MaxStructuredDataElements = 1
	p = mustParser(t, cfg)
	_, err := p.Parse([]byte(`<34>1 - h a p m [id1][id2]`), time.Now().UTC(), SourceMetadata{})
	requireCode(t, err, CodeResourceLimit)

	cfg = testConfig()
	cfg.MaxStructuredDataParamsPerElement = 1
	p = mustParser(t, cfg)
	_, err = p.Parse([]byte(`<34>1 - h a p m [id a="b" c="d"]`), time.Now().UTC(), SourceMetadata{})
	requireCode(t, err, CodeResourceLimit)

	cfg = testConfig()
	cfg.MaxStructuredDataParamValueBytes = 3
	p = mustParser(t, cfg)
	_, err = p.Parse([]byte(`<34>1 - h a p m [id a="four"]`), time.Now().UTC(), SourceMetadata{})
	requireCode(t, err, CodeResourceLimit)
}

func TestRFC5424HeaderBoundsVersionAndTimestamp(t *testing.T) {
	cfg := testConfig()
	cfg.MaxHostnameBytes = 3
	p := mustParser(t, cfg)
	_, err := p.Parse([]byte(`<34>1 - host app p m -`), time.Now().UTC(), SourceMetadata{})
	requireCode(t, err, CodeResourceLimit)

	p = mustParser(t, testConfig())
	for _, input := range []string{`<34>2 - h a p m -`, `<34>1 definitely-not-a-time h a p m -`} {
		_, err := p.Parse([]byte(input), time.Now().UTC(), SourceMetadata{})
		requireCode(t, err, CodeMalformedRFC5424)
	}
}

func TestRFC5424MessageBound(t *testing.T) {
	cfg := testConfig()
	cfg.MaxMessageBytes = 3
	p := mustParser(t, cfg)
	_, err := p.Parse([]byte(`<34>1 - h a p m - four`), time.Now().UTC(), SourceMetadata{})
	requireCode(t, err, CodeResourceLimit)
}

func TestRFC3164StandardTagPIDAndTimestampSemantics(t *testing.T) {
	p := mustParser(t, testConfig())
	cases := []struct{ input, app, pid, msg string }{
		{`<34>Oct 11 22:14:15 mymachine su: test message`, "su", "", "test message"},
		{`<34>Oct  9 02:04:05 mymachine app[123]: hello`, "app", "123", "hello"},
	}
	for _, tc := range cases {
		e := parse(t, p, tc.input)
		if e.Format != FormatRFC3164 || e.Quality != QualityGood || e.AppName != tc.app || e.ProcessID != tc.pid || e.Message != tc.msg {
			t.Fatalf("unexpected RFC3164 parse: %+v", e)
		}
		if e.RawTimestamp == "" || e.TimestampQuality != TimestampRFC3164Unresolved {
			t.Fatalf("RFC3164 time semantics wrong: %+v", e)
		}
		if e.EventTime != nil {
			t.Fatalf("RFC3164 fabricated authoritative time: %v", e.EventTime)
		}
	}
}

func TestRFC3164PartialMissingHostname(t *testing.T) {
	p := mustParser(t, testConfig())
	e := parse(t, p, `<34>Oct 11 22:14:15 app: hello`)
	if e.Format != FormatUnknown || e.Quality != QualityPartial || e.EventTime != nil {
		t.Fatalf("expected partial unknown: %+v", e)
	}
	if e.RawTimestamp != "Oct 11 22:14:15" {
		t.Fatalf("timestamp evidence lost: %q", e.RawTimestamp)
	}
}

func TestRFC3164MalformedTimestampRejected(t *testing.T) {
	p := mustParser(t, testConfig())
	_, err := p.Parse([]byte(`<34>Oct 99 99:99:99 host app: message`), time.Now().UTC(), SourceMetadata{})
	requireCode(t, err, CodeMalformedRFC3164)
}

func TestFallbackValidPRIBecomesPartialUnknown(t *testing.T) {
	p := mustParser(t, testConfig())
	e := parse(t, p, `<13>useful but nonstandard content`)
	if e.Format != FormatUnknown || e.Quality != QualityPartial || e.StatusCode != CodePartialUnknown || e.Message != "useful but nonstandard content" {
		t.Fatalf("unexpected fallback: %+v", e)
	}
}

func TestInvalidUTF8Rejected(t *testing.T) {
	p := mustParser(t, testConfig())
	input := []byte{'<', '1', '3', '>', 'x', 0xff, 'y'}
	_, err := p.Parse(input, time.Now().UTC(), SourceMetadata{})
	requireCode(t, err, CodeInvalidUTF8)
}

func TestInputAndRawEvidenceBounds(t *testing.T) {
	cfg := testConfig()
	cfg.MaxInputMessageBytes = 16
	cfg.MaxRawEvidenceBytes = 8
	cfg.MaxMessageBytes = 16
	p := mustParser(t, cfg)
	_, err := p.Parse([]byte(`<13>`+strings.Repeat("x", 20)), time.Now().UTC(), SourceMetadata{})
	requireCode(t, err, CodeMessageTooLarge)

	cfg = testConfig()
	cfg.MaxRawEvidenceBytes = 7
	p = mustParser(t, cfg)
	e := parse(t, p, `<13>éééé useful`)
	if len(e.RawEvidence) > 7 || !json.Valid([]byte(`"`+e.RawEvidence+`"`)) || !e.RawEvidenceTruncated {
		t.Fatalf("raw evidence not safely bounded: %q", e.RawEvidence)
	}
}

func TestMessageBoundForPartial(t *testing.T) {
	cfg := testConfig()
	cfg.MaxMessageBytes = 3
	p := mustParser(t, cfg)
	_, err := p.Parse([]byte(`<13>four`), time.Now().UTC(), SourceMetadata{})
	requireCode(t, err, CodeResourceLimit)
}

func TestParserErrorsDoNotEchoMaliciousInput(t *testing.T) {
	p := mustParser(t, testConfig())
	secret := "TOP-SECRET-ATTACKER-CONTENT-DO-NOT-ECHO"
	_, err := p.Parse([]byte(`<999>`+secret), time.Now().UTC(), SourceMetadata{})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked inbound message: %v", err)
	}
}

func TestControlCharactersDoNotPanic(t *testing.T) {
	p := mustParser(t, testConfig())
	inputs := []string{"<13>\x00\x01", "<34>1 - h a p m [id a=\"x\x00y\"]", "<34>Oct 11 22:14:15 h a: \x00"}
	for _, input := range inputs {
		_, _ = p.Parse([]byte(input), time.Now().UTC(), SourceMetadata{})
	}
}

func TestHugeStructuredCandidateBounded(t *testing.T) {
	cfg := testConfig()
	cfg.MaxInputMessageBytes = 256
	cfg.MaxRawEvidenceBytes = 128
	p := mustParser(t, cfg)
	input := `<34>1 - h a p m [id a="` + strings.Repeat("x", 300) + `"]`
	_, err := p.Parse([]byte(input), time.Now().UTC(), SourceMetadata{})
	requireCode(t, err, CodeMessageTooLarge)
}

func TestNormalizedEventRoundTripPreservesStructuredOrder(t *testing.T) {
	p := mustParser(t, testConfig())
	e := parse(t, p, `<34>1 - h a p m [id1 a="b"][id2 c="d"] message`)
	encoded, err := EncodeEvent(e, 8192, testConfig())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeEvent(encoded, 8192, testConfig())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.StructuredData) != 2 || decoded.StructuredData[0].ID != "id1" || decoded.StructuredData[1].ID != "id2" || decoded.StructuredData[1].Params[0].Value != "d" {
		t.Fatalf("structured order/content lost: %+v", decoded.StructuredData)
	}
}

func TestNormalizedEventSchemaAndUnknownFieldsRejected(t *testing.T) {
	p := mustParser(t, testConfig())
	e := parse(t, p, `<34>1 - h a p m - ok`)
	e.SchemaVersion = 99
	if _, err := EncodeEvent(e, 8192, testConfig()); err == nil {
		t.Fatal("expected schema rejection")
	}

	payload := []byte(`{"schemaVersion":1,"receivedAt":"2026-09-07T00:00:00Z","timestampQuality":"UNAVAILABLE","source":{},"facility":4,"severity":2,"format":"UNKNOWN","quality":"PARTIAL","unexpected":true}`)
	if _, err := DecodeEvent(payload, 8192, testConfig()); err == nil {
		t.Fatal("expected unknown-field rejection")
	}
}

func TestSourceMetadataBoundsAndTransport(t *testing.T) {
	p := mustParser(t, testConfig())
	if _, err := p.Parse([]byte(`<13>x`), time.Now().UTC(), SourceMetadata{Transport: "BOGUS"}); err == nil {
		t.Fatal("expected transport rejection")
	}
	cfg := testConfig()
	cfg.MaxSourceAddressBytes = 3
	p = mustParser(t, cfg)
	if _, err := p.Parse([]byte(`<13>x`), time.Now().UTC(), SourceMetadata{Address: "long"}); err == nil {
		t.Fatal("expected source address bound")
	}
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func TestRFC5424TimestampErrorsAreIndependent(t *testing.T) {
	p := mustParser(t, testConfig())
	received := time.Date(2026, 9, 7, 3, 30, 0, 0, time.UTC)

	_, err1 := p.Parse([]byte(`<34>1 2003-10-11T22:14:60Z h a p m -`), received, SourceMetadata{})
	_, err2 := p.Parse([]byte(`<34>1 2003-10-11t22:14:15Z h a p m -`), received, SourceMetadata{})
	e1, ok1 := err1.(*Error)
	e2, ok2 := err2.(*Error)
	if !ok1 || !ok2 {
		t.Fatalf("expected *Error values, got %T and %T", err1, err2)
	}
	if e1 == e2 {
		t.Fatal("timestamp errors must be distinct allocations")
	}
	if e1.Code != CodeMalformedRFC5424 || e1.Message != "RFC5424 timestamp is invalid" {
		t.Fatalf("err1 unexpected: %+v", e1)
	}
	if e2.Code != CodeMalformedRFC5424 || e2.Message != "RFC5424 timestamp is invalid" {
		t.Fatalf("err2 unexpected: %+v", e2)
	}

	e1.Code = ErrorCode("MUTATED")
	e1.Message = "mutated"

	_, err3 := p.Parse([]byte(`<34>1 2003-02-29T12:00:00Z h a p m -`), received, SourceMetadata{})
	e3, ok3 := err3.(*Error)
	if !ok3 {
		t.Fatalf("expected *Error, got %T", err3)
	}
	if e3 == e1 || e3 == e2 {
		t.Fatal("subsequent timestamp error aliased a prior error")
	}
	if e3.Code != CodeMalformedRFC5424 || e3.Message != "RFC5424 timestamp is invalid" {
		t.Fatalf("subsequent error corrupted: %+v", e3)
	}
	if e2.Code != CodeMalformedRFC5424 || e2.Message != "RFC5424 timestamp is invalid" {
		t.Fatalf("unmutated sibling corrupted: %+v", e2)
	}
}
