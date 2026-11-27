package syslog

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRFC5424HeaderIdentityGrammar(t *testing.T) {
	cfg := testConfig()
	cfg.MaxHostnameBytes, cfg.MaxAppNameBytes, cfg.MaxProcessIDBytes, cfg.MaxMessageIDBytes = 512, 512, 512, 512
	parser := mustParser(t, cfg)
	receivedAt := time.Unix(42, 0)
	names := [4]string{"hostname", "app", "procid", "msgid"}
	limits := [4]int{255, 48, 128, 32}
	original := [4]string{"host", "app", "42", "ID"}
	for i, name := range names {
		t.Run(name, func(t *testing.T) {
			for _, bad := range []string{"café", "a\tb", "a\x00b", "a\x7fb", strings.Repeat("x", limits[i]+1)} {
				fields := original
				fields[i] = bad
				event, err := parser.Parse([]byte("<34>1 - "+strings.Join(fields[:], " ")+" - message"), receivedAt, SourceMetadata{})
				requireCode(t, err, CodeMalformedRFC5424)
				if !reflect.DeepEqual(event, Event{}) {
					t.Fatalf("rejected %s returned partial event: %+v", name, event)
				}
			}
		})
	}

	// Exact wire limits, including bytes at both ends of the allowed range.
	var exact [4]string
	for i, size := range limits {
		exact[i] = "!" + strings.Repeat("x", size-2) + "~"
	}
	const suffix = `[test label="café"] 温度正常`
	event := parse(t, parser, "<34>1 - "+strings.Join(exact[:], " ")+" "+suffix)
	if got := [4]string{event.Hostname, event.AppName, event.ProcessID, event.MessageID}; got != exact ||
		event.Quality != QualityGood || event.Message != "温度正常" || len(event.StructuredData) != 1 ||
		len(event.StructuredData[0].Params) != 1 || event.StructuredData[0].Params[0].Value != "café" {
		t.Fatalf("valid text was changed: %+v", event)
	}
	event = parse(t, parser, "<34>1 - - - - - - café")
	if event.Quality != QualityGood || event.Hostname != "" || event.AppName != "" ||
		event.ProcessID != "" || event.MessageID != "" || event.Message != "café" {
		t.Fatalf("invalid NILVALUE normalization: %+v", event)
	}
}

func TestRFC5424HeaderIdentityConfiguredLimits(t *testing.T) {
	cfg := testConfig()
	cfg.MaxHostnameBytes, cfg.MaxAppNameBytes, cfg.MaxProcessIDBytes, cfg.MaxMessageIDBytes = 3, 3, 3, 3
	parser := mustParser(t, cfg)
	atLimit := parse(t, parser, "<34>1 - srv app 123 ID! - message")
	if atLimit.Quality != QualityGood || atLimit.MessageID != "ID!" {
		t.Fatalf("value at configured limit was changed: %+v", atLimit)
	}
	for i := range 4 {
		for _, oversized := range []string{"fits-the-protocol", strings.Repeat("x", 256)} {
			fields := [4]string{"h", "a", "p", "m"}
			fields[i] = oversized
			event, err := parser.Parse([]byte("<34>1 - "+strings.Join(fields[:], " ")+" - message"), time.Unix(42, 0), SourceMetadata{})
			requireCode(t, err, CodeResourceLimit)
			if !reflect.DeepEqual(event, Event{}) {
				t.Fatalf("configured rejection returned partial event: %+v", event)
			}
		}
	}
}
