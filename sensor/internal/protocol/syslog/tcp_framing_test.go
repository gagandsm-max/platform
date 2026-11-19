package syslog

import (
	"bytes"
	"strings"
	"testing"
)

func TestTCPFramingConfigValidation(t *testing.T) {
	// Invalid maxFrameBytes <= 0
	cfg := TCPFramingConfig{MaxFrameBytes: 0, MaxPrefixDigits: 5, MaxBufferedBytes: 1024}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for MaxFrameBytes=0")
	} else {
		requireCode(t, err, CodeInvalidConfig)
	}

	// Invalid maxFrameBytes < 0
	cfg = TCPFramingConfig{MaxFrameBytes: -1, MaxPrefixDigits: 5, MaxBufferedBytes: 1024}
	requireCode(t, cfg.Validate(), CodeInvalidConfig)

	// Invalid maxFrameBytes > 65535
	cfg = TCPFramingConfig{MaxFrameBytes: 65536, MaxPrefixDigits: 5, MaxBufferedBytes: 131072}
	requireCode(t, cfg.Validate(), CodeInvalidConfig)

	// Invalid maxPrefixDigits <= 0
	cfg = TCPFramingConfig{MaxFrameBytes: 1024, MaxPrefixDigits: 0, MaxBufferedBytes: 2048}
	requireCode(t, cfg.Validate(), CodeInvalidConfig)

	// Invalid maxPrefixDigits > 10
	cfg = TCPFramingConfig{MaxFrameBytes: 1024, MaxPrefixDigits: 11, MaxBufferedBytes: 2048}
	requireCode(t, cfg.Validate(), CodeInvalidConfig)

	// Invalid maxBufferedBytes < maxFrameBytes + maxPrefixDigits + 1
	cfg = TCPFramingConfig{MaxFrameBytes: 100, MaxPrefixDigits: 5, MaxBufferedBytes: 105}
	requireCode(t, cfg.Validate(), CodeInvalidConfig)

	// Valid config exactly at minimum buffer
	cfg = TCPFramingConfig{MaxFrameBytes: 100, MaxPrefixDigits: 5, MaxBufferedBytes: 106}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	// Constructor validations
	if _, err := NewOctetCountedDecoder(0); err == nil {
		t.Fatal("expected error for NewOctetCountedDecoder(0)")
	}
	if _, err := NewOctetCountedDecoder(65536); err == nil {
		t.Fatal("expected error for NewOctetCountedDecoder(65536)")
	}
	dec, err := NewOctetCountedDecoder(1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Config().MaxFrameBytes != 1024 {
		t.Fatalf("unexpected MaxFrameBytes: %d", dec.Config().MaxFrameBytes)
	}
}

func TestSingleCompleteFrame(t *testing.T) {
	dec, err := NewOctetCountedDecoder(4096)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoder: %v", err)
	}

	msg := "<134>1 2026-11-19T10:00:00Z host app 123 - test message"
	frameInput := []byte("55 " + msg)

	frames, err := dec.Decode(frameInput)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	if !bytes.Equal(frames[0], []byte(msg)) {
		t.Fatalf("frame mismatch: got %q, want %q", frames[0], msg)
	}
	if dec.Buffered() != 0 {
		t.Fatalf("expected 0 buffered, got %d", dec.Buffered())
	}
}

func TestMultipleFramesInSingleChunk(t *testing.T) {
	dec, err := NewOctetCountedDecoder(4096)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoder: %v", err)
	}

	input := []byte("5 alpha4 beta5 gamma")
	frames, err := dec.Decode(input)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}
	expected := []string{"alpha", "beta", "gamma"}
	for i, exp := range expected {
		if !bytes.Equal(frames[i], []byte(exp)) {
			t.Fatalf("frame %d: got %q, want %q", i, frames[i], exp)
		}
	}
	if dec.Buffered() != 0 {
		t.Fatalf("expected 0 buffered, got %d", dec.Buffered())
	}
}

func TestArbitraryStreamChunkSplitsByteByByte(t *testing.T) {
	dec, err := NewOctetCountedDecoder(4096)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoder: %v", err)
	}

	stream := []byte("11 first frame12 second frame11 third frame")
	var collected [][]byte

	for i := 0; i < len(stream); i++ {
		frames, err := dec.Decode([]byte{stream[i]})
		if err != nil {
			t.Fatalf("Decode byte %d: %v", i, err)
		}
		if len(frames) > 0 {
			collected = append(collected, frames...)
		}
	}

	if len(collected) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(collected))
	}
	expected := []string{"first frame", "second frame", "third frame"}
	for i, exp := range expected {
		if !bytes.Equal(collected[i], []byte(exp)) {
			t.Fatalf("frame %d mismatch: got %q, want %q", i, collected[i], exp)
		}
	}
	if dec.Buffered() != 0 {
		t.Fatalf("expected 0 buffered, got %d", dec.Buffered())
	}
}

func TestArbitraryChunkSplitsVariableSizes(t *testing.T) {
	for splitSize := 1; splitSize <= 16; splitSize++ {
		dec, err := NewOctetCountedDecoder(4096)
		if err != nil {
			t.Fatalf("NewOctetCountedDecoder: %v", err)
		}

		stream := []byte("5 apple6 banana6 cherry")
		var collected [][]byte

		for offset := 0; offset < len(stream); offset += splitSize {
			end := offset + splitSize
			if end > len(stream) {
				end = len(stream)
			}
			frames, err := dec.Decode(stream[offset:end])
			if err != nil {
				t.Fatalf("splitSize %d, offset %d: %v", splitSize, offset, err)
			}
			collected = append(collected, frames...)
		}

		if len(collected) != 3 {
			t.Fatalf("splitSize %d: expected 3 frames, got %d", splitSize, len(collected))
		}
		expected := []string{"apple", "banana", "cherry"}
		for i, exp := range expected {
			if !bytes.Equal(collected[i], []byte(exp)) {
				t.Fatalf("splitSize %d, frame %d: got %q, want %q", splitSize, i, collected[i], exp)
			}
		}
		if dec.Buffered() != 0 {
			t.Fatalf("splitSize %d: expected 0 buffered, got %d", splitSize, dec.Buffered())
		}
	}
}

func TestZeroBytesChunkAndNoBusyLoop(t *testing.T) {
	dec, err := NewOctetCountedDecoder(4096)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoder: %v", err)
	}

	// Empty chunks return nil, nil
	frames, err := dec.Decode(nil)
	if err != nil || frames != nil {
		t.Fatalf("expected nil, nil on nil chunk: frames=%v, err=%v", frames, err)
	}

	frames, err = dec.Decode([]byte{})
	if err != nil || frames != nil {
		t.Fatalf("expected nil, nil on empty chunk: frames=%v, err=%v", frames, err)
	}

	// Partial frame
	frames, err = dec.Decode([]byte("11 hel"))
	if err != nil || len(frames) != 0 {
		t.Fatalf("expected 0 frames on partial, got %d, err=%v", len(frames), err)
	}
	if dec.Buffered() != 6 {
		t.Fatalf("expected 6 buffered bytes, got %d", dec.Buffered())
	}

	// Multiple empty calls while partially buffered
	for i := 0; i < 5; i++ {
		frames, err = dec.Decode(nil)
		if err != nil || len(frames) != 0 {
			t.Fatalf("busy loop or error on nil chunk while buffered: frames=%v, err=%v", frames, err)
		}
		frames, err = dec.Decode([]byte{})
		if err != nil || len(frames) != 0 {
			t.Fatalf("busy loop or error on empty chunk while buffered: frames=%v, err=%v", frames, err)
		}
	}
	if dec.Buffered() != 6 {
		t.Fatalf("expected 6 buffered bytes preserved, got %d", dec.Buffered())
	}

	// Complete the frame
	frames, err = dec.Decode([]byte("lo-world"))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(frames) != 1 || !bytes.Equal(frames[0], []byte("hello-world")) {
		t.Fatalf("expected 'hello-world', got %q", frames)
	}
	if dec.Buffered() != 0 {
		t.Fatalf("expected 0 buffered, got %d", dec.Buffered())
	}
}

func TestPayloadWithNullAndControlBytes(t *testing.T) {
	dec, err := NewOctetCountedDecoder(4096)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoder: %v", err)
	}

	payload := []byte{0x00, 'a', 0x01, 0x02, 0xff, 0xfe, 0x00}
	frameInput := append([]byte("7 "), payload...)

	frames, err := dec.Decode(frameInput)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	if !bytes.Equal(frames[0], payload) {
		t.Fatalf("payload mismatch: got %v, want %v", frames[0], payload)
	}
}

func TestMaxFrameBytesLimits(t *testing.T) {
	// Minimum bound: 1 byte
	dec1, err := NewOctetCountedDecoder(1)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoder(1): %v", err)
	}
	frames, err := dec1.Decode([]byte("1 X"))
	if err != nil || len(frames) != 1 || !bytes.Equal(frames[0], []byte("X")) {
		t.Fatalf("1-byte frame failed: frames=%v, err=%v", frames, err)
	}

	// Oversize on 1-byte bound
	dec1Fail, err := NewOctetCountedDecoder(1)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoder(1): %v", err)
	}
	_, err = dec1Fail.Decode([]byte("2 XY"))
	requireCode(t, err, CodeFramingLengthOversize)

	// Maximum bound: 65535 bytes
	decMax, err := NewOctetCountedDecoder(MaxFrameBytesLimit)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoder(65535): %v", err)
	}
	largePayload := bytes.Repeat([]byte("A"), MaxFrameBytesLimit)
	largeFrame := append([]byte("65535 "), largePayload...)
	frames, err = decMax.Decode(largeFrame)
	if err != nil {
		t.Fatalf("Decode max frame: %v", err)
	}
	if len(frames) != 1 || len(frames[0]) != MaxFrameBytesLimit {
		t.Fatalf("expected 1 frame of %d bytes, got len=%d", MaxFrameBytesLimit, len(frames[0]))
	}

	// Exceeding 65535 length prefix
	decMax2, err := NewOctetCountedDecoder(MaxFrameBytesLimit)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoder: %v", err)
	}
	_, err = decMax2.Decode([]byte("65536 "))
	requireCode(t, err, CodeFramingLengthOversize)
}

func TestMalformedLengthPrefixes(t *testing.T) {
	malformedInputs := []struct {
		name  string
		input string
		want  ErrorCode
	}{
		{"zero prefix", "0 hello", CodeFramingInvalidPrefix},
		{"zero alone with space", "0 ", CodeFramingInvalidPrefix},
		{"leading zero", "05 hello", CodeFramingInvalidPrefix},
		{"leading zero two digits", "00 hello", CodeFramingInvalidPrefix},
		{"negative prefix", "-1 hello", CodeFramingInvalidPrefix},
		{"negative prefix multi", "-10 hello", CodeFramingInvalidPrefix},
		{"explicit plus prefix", "+5 hello", CodeFramingInvalidPrefix},
		{"leading space", " 5 hello", CodeFramingInvalidPrefix},
		{"alpha prefix", "abc hello", CodeFramingInvalidPrefix},
		{"alphanumeric mixed prefix", "12a 45", CodeFramingInvalidPrefix},
		{"newline delimiter instead of space", "5\nhello", CodeFramingInvalidPrefix},
		{"CR delimiter instead of space", "5\rhello", CodeFramingInvalidPrefix},
		{"tab delimiter instead of space", "5\thello", CodeFramingInvalidPrefix},
		{"raw syslog PRI without length", "<134>1 2026-11-19T10:00:00Z", CodeFramingInvalidPrefix},
	}

	for _, tc := range malformedInputs {
		t.Run(tc.name, func(t *testing.T) {
			dec, err := NewOctetCountedDecoder(4096)
			if err != nil {
				t.Fatalf("NewOctetCountedDecoder: %v", err)
			}
			frames, err := dec.Decode([]byte(tc.input))
			if err == nil {
				t.Fatalf("expected error for input %q, got frames: %v", tc.input, frames)
			}
			requireCode(t, err, tc.want)
			if !dec.FailedClosed() {
				t.Fatal("expected decoder to be failed closed")
			}
			if dec.Buffered() != 0 {
				t.Fatalf("expected 0 buffered after error, got %d", dec.Buffered())
			}
		})
	}
}

func TestPrefixDigitsExceedMaxDigits(t *testing.T) {
	cfg := TCPFramingConfig{
		MaxFrameBytes:    100,
		MaxPrefixDigits:  3,
		MaxBufferedBytes: 256,
	}
	dec, err := NewOctetCountedDecoderWithConfig(cfg)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoderWithConfig: %v", err)
	}

	// 4 digits exceeds MaxPrefixDigits: 3
	_, err = dec.Decode([]byte("1234 "))
	requireCode(t, err, CodeFramingLengthOversize)
	if !dec.FailedClosed() {
		t.Fatal("expected failed closed")
	}
}

func TestFrameSizeExceedsConfiguredMax(t *testing.T) {
	cfg := TCPFramingConfig{
		MaxFrameBytes:    50,
		MaxPrefixDigits:  5,
		MaxBufferedBytes: 256,
	}
	dec, err := NewOctetCountedDecoderWithConfig(cfg)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoderWithConfig: %v", err)
	}

	payload := strings.Repeat("A", 51)
	input := "51 " + payload
	_, err = dec.Decode([]byte(input))
	requireCode(t, err, CodeFramingLengthOversize)
	if !dec.FailedClosed() {
		t.Fatal("expected failed closed")
	}
}

func TestBufferOverflowResourceLimit(t *testing.T) {
	cfg := TCPFramingConfig{
		MaxFrameBytes:    20,
		MaxPrefixDigits:  5,
		MaxBufferedBytes: 30,
	}
	dec, err := NewOctetCountedDecoderWithConfig(cfg)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoderWithConfig: %v", err)
	}

	// Chunk 1: 20 bytes incomplete frame
	frames, err := dec.Decode([]byte("20 12345678901234567"))
	if err != nil || len(frames) != 0 {
		t.Fatalf("expected incomplete frame, got frames=%v, err=%v", frames, err)
	}
	if dec.Buffered() != 20 {
		t.Fatalf("expected 20 buffered, got %d", dec.Buffered())
	}

	// Chunk 2: 15 bytes -> total 35 > MaxBufferedBytes 30
	_, err = dec.Decode([]byte("123456789012345"))
	requireCode(t, err, CodeResourceLimit)
	if !dec.FailedClosed() {
		t.Fatal("expected failed closed")
	}
	if dec.Buffered() != 0 {
		t.Fatalf("expected 0 buffered, got %d", dec.Buffered())
	}
}

func TestFailClosedThereafter(t *testing.T) {
	dec, err := NewOctetCountedDecoder(4096)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoder: %v", err)
	}

	// Provoke failure
	_, err = dec.Decode([]byte("0 invalid"))
	requireCode(t, err, CodeFramingInvalidPrefix)
	if !dec.FailedClosed() {
		t.Fatal("expected FailedClosed() to be true")
	}

	// Subsequent calls with valid frames must still fail closed
	frames, err := dec.Decode([]byte("5 valid"))
	if err == nil {
		t.Fatal("expected error on subsequent call after failure")
	}
	requireCode(t, err, CodeFramingFailedClosed)
	if frames != nil {
		t.Fatalf("expected nil frames, got %v", frames)
	}

	// Subsequent call with nil chunk must still fail closed
	_, err = dec.Decode(nil)
	requireCode(t, err, CodeFramingFailedClosed)

	// Feed alias must also fail closed
	_, err = dec.Feed([]byte("5 valid"))
	requireCode(t, err, CodeFramingFailedClosed)
}

func TestCopyIsolation(t *testing.T) {
	dec, err := NewOctetCountedDecoder(4096)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoder: %v", err)
	}

	frames1, err := dec.Decode([]byte("5 hello"))
	if err != nil || len(frames1) != 1 {
		t.Fatalf("Decode: %v", err)
	}
	frame1 := frames1[0]
	if !bytes.Equal(frame1, []byte("hello")) {
		t.Fatalf("unexpected frame: %q", frame1)
	}

	// Caller mutates emitted slice
	frame1[0] = 'Z'
	frame1[1] = 'Z'

	// Decode another frame
	frames2, err := dec.Decode([]byte("5 world"))
	if err != nil || len(frames2) != 1 {
		t.Fatalf("Decode: %v", err)
	}
	frame2 := frames2[0]
	if !bytes.Equal(frame2, []byte("world")) {
		t.Fatalf("frame corrupted: got %q, want %q", frame2, "world")
	}

	// Mutating frame2 does not affect frame1
	frame2[0] = 'X'
	if !bytes.Equal(frame1, []byte("ZZllo")) {
		t.Fatalf("frame1 mutated unexpectedly: %q", frame1)
	}
}

func TestErrorsDoNotEchoAttackerPayload(t *testing.T) {
	secretPayload := "SECRET-ATTACKER-PAYLOAD-DO-NOT-LEAK-987654321"

	cases := []string{
		"0 " + secretPayload,
		"-5 " + secretPayload,
		"BADPREFIX " + secretPayload,
		"9999999999" + secretPayload,
		"<13>" + secretPayload,
	}

	for _, input := range cases {
		dec, err := NewOctetCountedDecoder(100)
		if err != nil {
			t.Fatalf("NewOctetCountedDecoder: %v", err)
		}

		_, err = dec.Decode([]byte(input))
		if err == nil {
			t.Fatalf("expected error for input %q", input)
		}
		if strings.Contains(err.Error(), secretPayload) {
			t.Fatalf("error message leaked attacker payload: %v", err)
		}
	}
}

func TestNoConflationWithDelimiters(t *testing.T) {
	dec, err := NewOctetCountedDecoder(4096)
	if err != nil {
		t.Fatalf("NewOctetCountedDecoder: %v", err)
	}

	// Frame containing internal newline must not be split
	msgWithLF := "hello\nworld"
	input := []byte("11 " + msgWithLF)
	frames, err := dec.Decode(input)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	if !bytes.Equal(frames[0], []byte(msgWithLF)) {
		t.Fatalf("expected %q, got %q", msgWithLF, frames[0])
	}

	// Frame containing CRLF must not be split
	msgWithCRLF := "line1\r\nline2"
	inputCRLF := []byte("12 " + msgWithCRLF)
	frames, err = dec.Decode(inputCRLF)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	if !bytes.Equal(frames[0], []byte(msgWithCRLF)) {
		t.Fatalf("expected %q, got %q", msgWithCRLF, frames[0])
	}
}

func TestNilDecoderSafety(t *testing.T) {
	var dec *OctetCountedDecoder
	_, err := dec.Decode([]byte("5 hello"))
	requireCode(t, err, CodeInvalidConfig)
	if dec.Buffered() != 0 {
		t.Fatalf("expected 0 buffered on nil decoder, got %d", dec.Buffered())
	}
	if !dec.FailedClosed() {
		t.Fatal("expected FailedClosed() to be true on nil decoder")
	}
	cfg := dec.Config()
	if cfg.MaxFrameBytes != 0 {
		t.Fatalf("expected zero config on nil decoder, got %+v", cfg)
	}
}
