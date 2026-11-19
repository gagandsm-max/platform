package syslog

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var testFixedTime = time.Date(2026, 9, 7, 3, 30, 0, 0, time.UTC)

func TestSyslogBoundary_PRIContract(t *testing.T) {
	p := mustParser(t, testConfig())

	// 1. Boundary extremes: PRI 0 and PRI 191
	e0 := parse(t, p, "<0>minimal message")
	if e0.Facility != 0 || e0.Severity != 0 {
		t.Fatalf("PRI 0 => facility %d, severity %d; want 0, 0", e0.Facility, e0.Severity)
	}

	e191 := parse(t, p, "<191>maximal message")
	if e191.Facility != 23 || e191.Severity != 7 {
		t.Fatalf("PRI 191 => facility %d, severity %d; want 23, 7", e191.Facility, e191.Severity)
	}

	// 2. PRI values outside the 0..191 range must be rejected with CodeMalformedPRI
	outOfRangeCases := []string{
		"<192>msg",
		"<193>msg",
		"<200>msg",
		"<999>msg",
		"<1000>msg",
		"<-1>msg",
	}
	for _, input := range outOfRangeCases {
		_, err := p.Parse([]byte(input), testFixedTime, SourceMetadata{})
		requireCode(t, err, CodeMalformedPRI)
	}

	// 3. Incomplete and malformed PRI framing
	malformedFramingCases := []string{
		"<",
		"<1",
		"<12",
		"<>",
		"<>msg",
		"12>msg",
		"msg",
		"<0>",   // missing content after delimiter
		"<191>", // missing content after delimiter
		"< 0>msg",
		"<0 >msg",
		"<+0>msg",
		"<1a>msg",
		"< 12>msg",
	}
	for _, input := range malformedFramingCases {
		_, err := p.Parse([]byte(input), testFixedTime, SourceMetadata{})
		requireCode(t, err, CodeMalformedPRI)
	}
}

func TestSyslogBoundary_StrictUTF8Rejection(t *testing.T) {
	p := mustParser(t, testConfig())

	invalidUTF8Cases := []struct {
		name  string
		input []byte
	}{
		{"invalid byte 0xFF", []byte("<13>err \xff payload")},
		{"invalid byte 0xFE", []byte("<13>err \xfe payload")},
		{"truncated 2-byte sequence", []byte("<13>incomplete \xc2")},
		{"truncated 3-byte sequence", []byte("<13>incomplete \xe2\x82")},
		{"truncated 4-byte sequence", []byte("<13>incomplete \xf0\x90\x80")},
		{"lone continuation byte", []byte("<13>continuation \x80 payload")},
		{"overlong 2-byte encoding", []byte("<13>overlong \xc0\xaf payload")},
		{"overlong 3-byte encoding", []byte("<13>overlong \xe0\x80\xaf payload")},
	}

	for _, tc := range invalidUTF8Cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := p.Parse(tc.input, testFixedTime, SourceMetadata{})
			requireCode(t, err, CodeInvalidUTF8)
		})
	}

	// Valid UTF-8 with multi-byte runes must succeed
	validInput := []byte("<13>valid multi-byte UTF-8: \u20ac \u00e9 \U0001f600")
	e, err := p.Parse(validInput, testFixedTime, SourceMetadata{})
	if err != nil {
		t.Fatalf("expected valid UTF-8 to succeed, got: %v", err)
	}
	if !utf8.ValidString(e.RawEvidence) {
		t.Fatalf("RawEvidence is not valid UTF-8: %q", e.RawEvidence)
	}
}

func TestSyslogBoundary_RFC5424StructuredDataEscapes(t *testing.T) {
	p := mustParser(t, testConfig())

	// 1. Valid escapes: \", \\, \]
	validInput := `<34>1 2026-09-07T03:30:00Z host app 1 ID [sdID q="\"" bs="\\" br="\]" mixed="a\"b\\c\]d"] message`
	e := parse(t, p, validInput)
	if len(e.StructuredData) != 1 {
		t.Fatalf("expected 1 StructuredData element, got %d", len(e.StructuredData))
	}
	params := e.StructuredData[0].Params
	if len(params) != 4 {
		t.Fatalf("expected 4 params, got %d", len(params))
	}
	wantParams := []struct{ name, val string }{
		{"q", `"`},
		{"bs", `\`},
		{"br", `]`},
		{"mixed", `a"b\c]d`},
	}
	for i, want := range wantParams {
		if params[i].Name != want.name || params[i].Value != want.val {
			t.Fatalf("param %d (%s): got %q, want %q", i, want.name, params[i].Value, want.val)
		}
	}

	// 2. Unescaped closing bracket inside param value must fail
	unescapedBracket := `<34>1 2026-09-07T03:30:00Z host app 1 ID [sdID key="unescaped]bracket"] msg`
	_, err := p.Parse([]byte(unescapedBracket), testFixedTime, SourceMetadata{})
	requireCode(t, err, CodeStructuredData)

	// 3. Incomplete escape at end of parameter value or input
	incompleteEscapes := []string{
		`<34>1 2026-09-07T03:30:00Z host app 1 ID [sdID key="trailing\`,
		`<34>1 2026-09-07T03:30:00Z host app 1 ID [sdID key="trailing\"`,
	}
	for _, input := range incompleteEscapes {
		_, err := p.Parse([]byte(input), testFixedTime, SourceMetadata{})
		requireCode(t, err, CodeStructuredData)
	}

	// 4. Invalid escape sequences (only \", \\, \] permitted)
	invalidEscapes := []string{
		`<34>1 2026-09-07T03:30:00Z host app 1 ID [sdID key="bad\nescape"] msg`,
		`<34>1 2026-09-07T03:30:00Z host app 1 ID [sdID key="bad\tescape"] msg`,
		`<34>1 2026-09-07T03:30:00Z host app 1 ID [sdID key="bad\/escape"] msg`,
		`<34>1 2026-09-07T03:30:00Z host app 1 ID [sdID key="bad\0escape"] msg`,
		`<34>1 2026-09-07T03:30:00Z host app 1 ID [sdID key="bad\aescape"] msg`,
	}
	for _, input := range invalidEscapes {
		_, err := p.Parse([]byte(input), testFixedTime, SourceMetadata{})
		requireCode(t, err, CodeStructuredData)
	}

	// 5. Malformed structured data framing
	malformedFraming := []string{
		`<34>1 2026-09-07T03:30:00Z host app 1 ID [sdID key="unterminated`,
		`<34>1 2026-09-07T03:30:00Z host app 1 ID [sdID key="val"`,
		`<34>1 2026-09-07T03:30:00Z host app 1 ID -missing-space`,
	}
	for _, input := range malformedFraming {
		_, err := p.Parse([]byte(input), testFixedTime, SourceMetadata{})
		requireCode(t, err, CodeStructuredData)
	}
}

func TestSyslogBoundary_EvidenceTruncationUTF8Safety(t *testing.T) {
	// Configure small evidence limit
	cfg := testConfig()
	cfg.MaxRawEvidenceBytes = 16
	p := mustParser(t, cfg)

	// Case 1: len < MaxRawEvidenceBytes
	shortInput := "<13>short"
	eShort := parse(t, p, shortInput)
	if eShort.RawEvidenceTruncated {
		t.Fatal("expected RawEvidenceTruncated == false for short input")
	}
	if eShort.RawEvidence != shortInput {
		t.Fatalf("RawEvidence = %q, want %q", eShort.RawEvidence, shortInput)
	}

	// Case 2: len == MaxRawEvidenceBytes
	exactInput := "<13>abcdefghijkl" // exactly 16 bytes
	if len(exactInput) != 16 {
		t.Fatalf("test fixture length %d != 16", len(exactInput))
	}
	eExact := parse(t, p, exactInput)
	if eExact.RawEvidenceTruncated {
		t.Fatal("expected RawEvidenceTruncated == false for exact bound input")
	}
	if eExact.RawEvidence != exactInput {
		t.Fatalf("RawEvidence = %q, want %q", eExact.RawEvidence, exactInput)
	}

	// Case 3: len > MaxRawEvidenceBytes (pure ASCII truncation)
	longInput := "<13>abcdefghijklmno" // 19 bytes
	eLong := parse(t, p, longInput)
	if !eLong.RawEvidenceTruncated {
		t.Fatal("expected RawEvidenceTruncated == true for over-bound input")
	}
	if len(eLong.RawEvidence) != 16 {
		t.Fatalf("RawEvidence len = %d, want 16", len(eLong.RawEvidence))
	}
	if eLong.RawEvidence != exactInput {
		t.Fatalf("RawEvidence = %q, want %q", eLong.RawEvidence, exactInput)
	}

	// Case 4: Truncation boundary cutting through a 2-byte UTF-8 rune ('é' = \xc3\xa9)
	// Prefix "<13>" (4 bytes) + "é" (2 bytes) = 6 bytes.
	// If MaxRawEvidenceBytes is 5, the boundary falls between \xc3 and \xa9.
	cfg2 := testConfig()
	cfg2.MaxRawEvidenceBytes = 5
	p2 := mustParser(t, cfg2)
	e2 := parse(t, p2, "<13>é extra content")
	if !e2.RawEvidenceTruncated {
		t.Fatal("expected RawEvidenceTruncated == true")
	}
	if !utf8.ValidString(e2.RawEvidence) {
		t.Fatalf("RawEvidence contains broken UTF-8: %q", e2.RawEvidence)
	}
	if e2.RawEvidence != "<13>" {
		t.Fatalf("RawEvidence = %q, want %q", e2.RawEvidence, "<13>")
	}

	// Case 5: Truncation boundary cutting through a 3-byte UTF-8 rune ('€' = \xe2\x82\xac)
	// Prefix "<13>" (4 bytes) + "€" (3 bytes) = 7 bytes.
	// If MaxRawEvidenceBytes is 6, the cut falls on \xe2\x82 (2 bytes in).
	cfg3 := testConfig()
	cfg3.MaxRawEvidenceBytes = 6
	p3 := mustParser(t, cfg3)
	e3 := parse(t, p3, "<13>€ extra content")
	if !e3.RawEvidenceTruncated {
		t.Fatal("expected RawEvidenceTruncated == true")
	}
	if !utf8.ValidString(e3.RawEvidence) {
		t.Fatalf("RawEvidence contains broken UTF-8: %q", e3.RawEvidence)
	}
	if e3.RawEvidence != "<13>" {
		t.Fatalf("RawEvidence = %q, want %q", e3.RawEvidence, "<13>")
	}

	// If MaxRawEvidenceBytes is 7, the cut includes the complete 3-byte rune.
	cfg4 := testConfig()
	cfg4.MaxRawEvidenceBytes = 7
	p4 := mustParser(t, cfg4)
	e4 := parse(t, p4, "<13>€ extra content")
	if !e4.RawEvidenceTruncated {
		t.Fatal("expected RawEvidenceTruncated == true")
	}
	if !utf8.ValidString(e4.RawEvidence) {
		t.Fatalf("RawEvidence contains broken UTF-8: %q", e4.RawEvidence)
	}
	if e4.RawEvidence != "<13>€" {
		t.Fatalf("RawEvidence = %q, want %q", e4.RawEvidence, "<13>€")
	}

	// Case 6: Truncation boundary cutting through a 4-byte UTF-8 rune ('🚀' = \xf0\x9f\x9a\x80)
	// Prefix "<13>" (4 bytes) + "🚀" (4 bytes) = 8 bytes.
	// If MaxRawEvidenceBytes is 7, the cut falls inside the 4-byte rune.
	e5 := parse(t, p4, "<13>🚀 extra content") // p4 has MaxRawEvidenceBytes = 7
	if !e5.RawEvidenceTruncated {
		t.Fatal("expected RawEvidenceTruncated == true")
	}
	if !utf8.ValidString(e5.RawEvidence) {
		t.Fatalf("RawEvidence contains broken UTF-8: %q", e5.RawEvidence)
	}
	if e5.RawEvidence != "<13>" {
		t.Fatalf("RawEvidence = %q, want %q", e5.RawEvidence, "<13>")
	}
}

func TestSyslogBoundary_MaxInputMessageBytesLimit(t *testing.T) {
	// Configure explicit small input message limit
	const limit = 128
	cfg := testConfig()
	cfg.MaxInputMessageBytes = limit
	cfg.MaxRawEvidenceBytes = limit
	cfg.MaxMessageBytes = limit
	p := mustParser(t, cfg)

	// 1. Empty message (len == 0) rejected as CodeMalformedPRI
	_, err := p.Parse([]byte{}, testFixedTime, SourceMetadata{})
	requireCode(t, err, CodeMalformedPRI)

	// 2. Exact boundary: message of length exactly limit
	prefix := "<13>"
	fillLen := limit - len(prefix)
	exactMsg := []byte(prefix + strings.Repeat("a", fillLen))
	if len(exactMsg) != limit {
		t.Fatalf("test fixture length %d != limit %d", len(exactMsg), limit)
	}
	e, err := p.Parse(exactMsg, testFixedTime, SourceMetadata{})
	if err != nil {
		t.Fatalf("Parse at exact MaxInputMessageBytes failed: %v", err)
	}
	if len(e.RawEvidence) != limit {
		t.Fatalf("RawEvidence len = %d, want %d", len(e.RawEvidence), limit)
	}
	if e.RawEvidenceTruncated {
		t.Fatal("expected RawEvidenceTruncated == false at exact limit")
	}

	// 3. Off-by-one: message of length limit + 1 rejected with CodeMessageTooLarge
	overMsg := []byte(prefix + strings.Repeat("a", fillLen) + "b")
	if len(overMsg) != limit+1 {
		t.Fatalf("test fixture length %d != limit+1 %d", len(overMsg), limit+1)
	}
	_, err = p.Parse(overMsg, testFixedTime, SourceMetadata{})
	requireCode(t, err, CodeMessageTooLarge)

	// 4. Significantly oversized message rejected with CodeMessageTooLarge
	hugeMsg := []byte(prefix + strings.Repeat("x", limit*2))
	_, err = p.Parse(hugeMsg, testFixedTime, SourceMetadata{})
	requireCode(t, err, CodeMessageTooLarge)
}
