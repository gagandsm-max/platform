package syslog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"
	"unicode/utf8"
)

const EventSchemaVersion = 1

type Format string

const (
	FormatRFC3164 Format = "RFC3164"
	FormatRFC5424 Format = "RFC5424"
	FormatUnknown Format = "UNKNOWN"
)

type Quality string

const (
	QualityGood    Quality = "GOOD"
	QualityPartial Quality = "PARTIAL"
)

type TimestampQuality string

const (
	TimestampAuthoritativeRFC3339 TimestampQuality = "AUTHORITATIVE_RFC3339"
	TimestampUnavailable          TimestampQuality = "UNAVAILABLE"
	TimestampRFC3164Unresolved    TimestampQuality = "UNRESOLVED_RFC3164_YEAR_TIMEZONE"
)

type Transport string

const (
	TransportUDP Transport = "UDP"
	TransportTCP Transport = "TCP"
	TransportTLS Transport = "TLS"
)

type SourceMetadata struct {
	Transport Transport `json:"transport,omitempty"`
	Address   string    `json:"address,omitempty"`
}

type StructuredDataElement struct {
	ID     string                `json:"id"`
	Params []StructuredDataParam `json:"params,omitempty"`
}

type StructuredDataParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Event struct {
	SchemaVersion        int                     `json:"schemaVersion"`
	ReceivedAt           time.Time               `json:"receivedAt"`
	EventTime            *time.Time              `json:"eventTime,omitempty"`
	RawTimestamp         string                  `json:"rawTimestamp,omitempty"`
	TimestampQuality     TimestampQuality        `json:"timestampQuality"`
	Source               SourceMetadata          `json:"source"`
	Facility             int                     `json:"facility"`
	Severity             int                     `json:"severity"`
	Hostname             string                  `json:"hostname,omitempty"`
	AppName              string                  `json:"appName,omitempty"`
	ProcessID            string                  `json:"processId,omitempty"`
	MessageID            string                  `json:"messageId,omitempty"`
	StructuredData       []StructuredDataElement `json:"structuredData,omitempty"`
	Message              string                  `json:"message,omitempty"`
	RawEvidence          string                  `json:"rawEvidence,omitempty"`
	RawEvidenceTruncated bool                    `json:"rawEvidenceTruncated,omitempty"`
	Format               Format                  `json:"format"`
	Quality              Quality                 `json:"quality"`
	StatusCode           ErrorCode               `json:"statusCode,omitempty"`
}

type ParserConfig struct {
	MaxInputMessageBytes              int
	MaxRawEvidenceBytes               int
	MaxHostnameBytes                  int
	MaxAppNameBytes                   int
	MaxProcessIDBytes                 int
	MaxMessageIDBytes                 int
	MaxMessageBytes                   int
	MaxStructuredDataElements         int
	MaxStructuredDataParamsPerElement int
	MaxStructuredDataIDBytes          int
	MaxStructuredDataParamNameBytes   int
	MaxStructuredDataParamValueBytes  int
	MaxSourceAddressBytes             int
}

func (c ParserConfig) Validate() error {
	values := []int{
		c.MaxInputMessageBytes, c.MaxRawEvidenceBytes, c.MaxHostnameBytes,
		c.MaxAppNameBytes, c.MaxProcessIDBytes, c.MaxMessageIDBytes, c.MaxMessageBytes,
		c.MaxStructuredDataElements, c.MaxStructuredDataParamsPerElement,
		c.MaxStructuredDataIDBytes, c.MaxStructuredDataParamNameBytes,
		c.MaxStructuredDataParamValueBytes, c.MaxSourceAddressBytes,
	}
	for _, value := range values {
		if value <= 0 {
			return fail(CodeInvalidConfig, "all Syslog parser bounds must be positive", nil)
		}
	}
	if c.MaxRawEvidenceBytes > c.MaxInputMessageBytes {
		return fail(CodeInvalidConfig, "raw evidence bound cannot exceed input message bound", nil)
	}
	return nil
}

func (e Event) Validate(c ParserConfig) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if e.SchemaVersion != EventSchemaVersion || e.ReceivedAt.IsZero() {
		return fail(CodeInternal, "normalized Syslog event schema/timestamp is invalid", nil)
	}
	if e.Facility < 0 || e.Facility > 23 || e.Severity < 0 || e.Severity > 7 {
		return fail(CodeInternal, "normalized Syslog PRI fields are invalid", nil)
	}
	if !validFormat(e.Format) || !validQuality(e.Quality) || !validTimestampQuality(e.TimestampQuality) {
		return fail(CodeInternal, "normalized Syslog event enum is invalid", nil)
	}
	if e.TimestampQuality == TimestampAuthoritativeRFC3339 && e.EventTime == nil {
		return fail(CodeInternal, "authoritative timestamp quality requires event time", nil)
	}
	if len(e.Hostname) > c.MaxHostnameBytes || len(e.AppName) > c.MaxAppNameBytes || len(e.ProcessID) > c.MaxProcessIDBytes || len(e.MessageID) > c.MaxMessageIDBytes || len(e.Message) > c.MaxMessageBytes || len(e.RawEvidence) > c.MaxRawEvidenceBytes || len(e.Source.Address) > c.MaxSourceAddressBytes {
		return fail(CodeResourceLimit, "normalized Syslog event exceeds configured field bound", nil)
	}
	if !validTransport(e.Source.Transport) {
		return fail(CodeInternal, "normalized Syslog source transport is invalid", nil)
	}
	if len(e.StructuredData) > c.MaxStructuredDataElements {
		return fail(CodeResourceLimit, "normalized structured-data element count exceeds configured bound", nil)
	}
	for _, element := range e.StructuredData {
		if element.ID == "" || len(element.ID) > c.MaxStructuredDataIDBytes || len(element.Params) > c.MaxStructuredDataParamsPerElement {
			return fail(CodeResourceLimit, "normalized structured-data element exceeds configured bound", nil)
		}
		for _, param := range element.Params {
			if param.Name == "" || len(param.Name) > c.MaxStructuredDataParamNameBytes || len(param.Value) > c.MaxStructuredDataParamValueBytes {
				return fail(CodeResourceLimit, "normalized structured-data parameter exceeds configured bound", nil)
			}
		}
	}
	return nil
}

func EncodeEvent(event Event, maxBytes int, c ParserConfig) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fail(CodeInvalidConfig, "maximum encoded event bytes must be positive", nil)
	}
	if err := event.Validate(c); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return nil, fail(CodeInternal, "normalized Syslog event cannot be encoded", err)
	}
	if len(payload) > maxBytes {
		return nil, fail(CodeResourceLimit, "encoded normalized Syslog event exceeds configured maximum", nil)
	}
	return payload, nil
}

func DecodeEvent(payload []byte, maxBytes int, c ParserConfig) (Event, error) {
	if maxBytes <= 0 || len(payload) == 0 || len(payload) > maxBytes {
		return Event{}, fail(CodeResourceLimit, "encoded normalized Syslog event is outside configured bound", nil)
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var event Event
	if err := dec.Decode(&event); err != nil {
		return Event{}, fail(CodeInternal, "normalized Syslog event JSON is invalid", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Event{}, fail(CodeInternal, "normalized Syslog event JSON contains trailing data", err)
	}
	if err := event.Validate(c); err != nil {
		return Event{}, err
	}
	return event, nil
}

func validFormat(v Format) bool {
	return v == FormatRFC3164 || v == FormatRFC5424 || v == FormatUnknown
}
func validQuality(v Quality) bool { return v == QualityGood || v == QualityPartial }
func validTimestampQuality(v TimestampQuality) bool {
	return v == TimestampAuthoritativeRFC3339 || v == TimestampUnavailable || v == TimestampRFC3164Unresolved
}
func validTransport(v Transport) bool {
	return v == "" || v == TransportUDP || v == TransportTCP || v == TransportTLS
}

func boundedEvidence(input string, maxBytes int) (string, bool) {
	if len(input) <= maxBytes {
		return input, false
	}
	end := maxBytes
	for end > 0 && !utf8.ValidString(input[:end]) {
		end--
	}
	return input[:end], true
}
