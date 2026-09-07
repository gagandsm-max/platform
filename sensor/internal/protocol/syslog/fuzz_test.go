package syslog

import (
	"testing"
	"time"
)

func FuzzParseSyslog(f *testing.F) {
	p, err := NewParser(testConfig())
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][]byte{
		[]byte(`<34>1 - h a p m - message`),
		[]byte(`<34>Oct 11 22:14:15 host app[1]: message`),
		[]byte(`<13>partial`),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 4096 {
			return
		}
		_, _ = p.Parse(input, time.Unix(1, 0).UTC(), SourceMetadata{})
	})
}

func FuzzRFC5424StructuredData(f *testing.F) {
	p, err := NewParser(testConfig())
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range []string{`-`, `[id a="b"]`, `[id1][id2 c="d"]`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 1024 {
			return
		}
		_, _, _ = p.parseStructuredDataAndMessage(input)
	})
}
