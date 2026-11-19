package syslog

// Error codes for Syslog RFC6587 TCP framing.
const (
	CodeFramingInvalidPrefix  ErrorCode = "SYSLOG_FRAMING_INVALID_PREFIX"
	CodeFramingLengthOversize ErrorCode = "SYSLOG_FRAMING_LENGTH_OVERSIZE"
	CodeFramingFailedClosed   ErrorCode = "SYSLOG_FRAMING_FAILED_CLOSED"
)

const (
	// MinFrameBytes is the minimum valid frame length (1 octet).
	MinFrameBytes = 1
	// MaxFrameBytesLimit is the upper bound on maxFrameBytes (RFC 6587 / RFC 5426 64KB envelope).
	MaxFrameBytesLimit = 65535
	// DefaultMaxPrefixDigits is the default maximum decimal digits for length prefix ("65535" is 5 digits).
	DefaultMaxPrefixDigits = 5
	// MaxPrefixDigitsLimit is the maximum permissible length prefix digit bound.
	MaxPrefixDigitsLimit = 10
)

// TCPFramingConfig configures bounds for RFC6587 octet-counted TCP framing.
type TCPFramingConfig struct {
	MaxFrameBytes    int
	MaxPrefixDigits  int
	MaxBufferedBytes int
}

// Validate verifies that framing bounds are positive and within permissible limits.
func (c TCPFramingConfig) Validate() error {
	if c.MaxFrameBytes < MinFrameBytes || c.MaxFrameBytes > MaxFrameBytesLimit {
		return fail(CodeInvalidConfig, "Syslog framing maxFrameBytes must be >0 and <=65535", nil)
	}
	if c.MaxPrefixDigits <= 0 || c.MaxPrefixDigits > MaxPrefixDigitsLimit {
		return fail(CodeInvalidConfig, "Syslog framing maxPrefixDigits must be positive and <=10", nil)
	}
	minBuffer := c.MaxFrameBytes + c.MaxPrefixDigits + 1
	if c.MaxBufferedBytes < minBuffer {
		return fail(CodeInvalidConfig, "Syslog framing maxBufferedBytes must be at least maxFrameBytes + maxPrefixDigits + 1", nil)
	}
	return nil
}

// OctetCountedDecoder is a small, deterministic, bounded, in-memory decoder
// for RFC6587 octet-counted Syslog TCP frame boundaries.
// It manages no sockets, listeners, TLS, or routing.
type OctetCountedDecoder struct {
	config           TCPFramingConfig
	maxFrameBytes    int
	maxPrefixDigits  int
	maxBufferedBytes int
	buf              []byte
	failed           bool
}

// TCPFrameDecoder is an alias for OctetCountedDecoder.
type TCPFrameDecoder = OctetCountedDecoder

// FrameDecoder is an alias for OctetCountedDecoder.
type FrameDecoder = OctetCountedDecoder

// NewOctetCountedDecoder constructs an RFC6587 octet-counted decoder with
// explicit maxFrameBytes and safe default prefix digit and buffer limits.
func NewOctetCountedDecoder(maxFrameBytes int) (*OctetCountedDecoder, error) {
	minBuffer := maxFrameBytes + DefaultMaxPrefixDigits + 1
	defaultBuffer := 2 * minBuffer
	if defaultBuffer < 65536 {
		defaultBuffer = 65536
	}
	cfg := TCPFramingConfig{
		MaxFrameBytes:    maxFrameBytes,
		MaxPrefixDigits:  DefaultMaxPrefixDigits,
		MaxBufferedBytes: defaultBuffer,
	}
	return NewOctetCountedDecoderWithConfig(cfg)
}

// NewOctetCountedDecoderWithConfig constructs an RFC6587 octet-counted decoder with explicit configuration.
func NewOctetCountedDecoderWithConfig(config TCPFramingConfig) (*OctetCountedDecoder, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &OctetCountedDecoder{
		config:           config,
		maxFrameBytes:    config.MaxFrameBytes,
		maxPrefixDigits:  config.MaxPrefixDigits,
		maxBufferedBytes: config.MaxBufferedBytes,
	}, nil
}

// NewTCPFramingDecoder is an alias constructor for NewOctetCountedDecoder.
func NewTCPFramingDecoder(maxFrameBytes int) (*OctetCountedDecoder, error) {
	return NewOctetCountedDecoder(maxFrameBytes)
}

// NewTCPFramingDecoderWithConfig is an alias constructor for NewOctetCountedDecoderWithConfig.
func NewTCPFramingDecoderWithConfig(config TCPFramingConfig) (*OctetCountedDecoder, error) {
	return NewOctetCountedDecoderWithConfig(config)
}

// NewFrameDecoder is an alias constructor for NewOctetCountedDecoder.
func NewFrameDecoder(maxFrameBytes int) (*OctetCountedDecoder, error) {
	return NewOctetCountedDecoder(maxFrameBytes)
}

// Config returns the configured bounds.
func (d *OctetCountedDecoder) Config() TCPFramingConfig {
	if d == nil {
		return TCPFramingConfig{}
	}
	return d.config
}

// Buffered returns the number of currently buffered unconsumed stream bytes.
func (d *OctetCountedDecoder) Buffered() int {
	if d == nil {
		return 0
	}
	return len(d.buf)
}

// FailedClosed returns true if the decoder encountered a framing violation and failed closed.
func (d *OctetCountedDecoder) FailedClosed() bool {
	if d == nil {
		return true
	}
	return d.failed
}

// Feed is an alias for Decode.
func (d *OctetCountedDecoder) Feed(chunk []byte) ([][]byte, error) {
	return d.Decode(chunk)
}

// Decode processes an arbitrary stream chunk and returns all complete frames found.
// Frames are emitted with copy isolation.
// If malformed, negative, zero, overflow, or oversize length prefixes or buffer limits
// are encountered, the decoder fails closed permanently and discards all buffered data.
func (d *OctetCountedDecoder) Decode(chunk []byte) ([][]byte, error) {
	if d == nil {
		return nil, fail(CodeInvalidConfig, "Syslog framing decoder is not initialized", nil)
	}
	if d.failed {
		return nil, fail(CodeFramingFailedClosed, "Syslog RFC6587 TCP framing decoder is closed due to previous framing error", nil)
	}

	if len(chunk) > 0 {
		if len(d.buf)+len(chunk) > d.maxBufferedBytes {
			d.failed = true
			d.buf = nil
			return nil, fail(CodeResourceLimit, "Syslog RFC6587 TCP framing buffer limit exceeded", nil)
		}
		d.buf = append(d.buf, chunk...)
	}

	var frames [][]byte

	for len(d.buf) > 0 {
		// RFC6587 §3.4.1: MSG-LEN = NONZERO-DIGIT *DIGIT
		// First byte must be ASCII '1'..'9'.
		if d.buf[0] < '1' || d.buf[0] > '9' {
			d.failed = true
			d.buf = nil
			return nil, fail(CodeFramingInvalidPrefix, "Syslog RFC6587 framing: length prefix must begin with non-zero digit 1-9", nil)
		}

		spaceIdx := -1
		for i := 1; i < len(d.buf); i++ {
			b := d.buf[i]
			if b == ' ' {
				spaceIdx = i
				break
			}
			if b < '0' || b > '9' {
				d.failed = true
				d.buf = nil
				return nil, fail(CodeFramingInvalidPrefix, "Syslog RFC6587 framing: invalid character in length prefix", nil)
			}
			if i >= d.maxPrefixDigits {
				d.failed = true
				d.buf = nil
				return nil, fail(CodeFramingLengthOversize, "Syslog RFC6587 framing: length prefix exceeds configured maximum digits", nil)
			}
		}

		if spaceIdx == -1 {
			// Incomplete length prefix; wait for more stream data.
			break
		}

		// Parse msgLen
		msgLen := 0
		for i := 0; i < spaceIdx; i++ {
			msgLen = msgLen*10 + int(d.buf[i]-'0')
			if msgLen > d.maxFrameBytes {
				d.failed = true
				d.buf = nil
				return nil, fail(CodeFramingLengthOversize, "Syslog RFC6587 framing: frame length exceeds configured maximum", nil)
			}
		}

		msgStart := spaceIdx + 1
		msgEnd := msgStart + msgLen

		if len(d.buf) < msgEnd {
			// Incomplete message body; wait for more stream data.
			break
		}

		// Full frame received: emit with COPY isolation.
		frame := make([]byte, msgLen)
		copy(frame, d.buf[msgStart:msgEnd])
		frames = append(frames, frame)

		d.buf = d.buf[msgEnd:]
	}

	if len(d.buf) == 0 {
		d.buf = nil
	} else {
		// Compact remaining unconsumed bytes to prevent slice memory drift.
		compacted := make([]byte, len(d.buf))
		copy(compacted, d.buf)
		d.buf = compacted
	}

	return frames, nil
}
