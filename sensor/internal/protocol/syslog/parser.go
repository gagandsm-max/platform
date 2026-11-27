package syslog

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Parser struct {
	config ParserConfig
}

func NewParser(config ParserConfig) (*Parser, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Parser{config: config}, nil
}

func (p *Parser) Parse(input []byte, receivedAt time.Time, source SourceMetadata) (Event, error) {
	if p == nil {
		return Event{}, fail(CodeInvalidConfig, "Syslog parser is not initialized", nil)
	}
	if receivedAt.IsZero() {
		return Event{}, fail(CodeInvalidConfig, "received timestamp is required", nil)
	}
	if len(input) == 0 {
		return Event{}, fail(CodeMalformedPRI, "Syslog message is empty", nil)
	}
	if len(input) > p.config.MaxInputMessageBytes {
		return Event{}, fail(CodeMessageTooLarge, "Syslog message exceeds configured maximum", nil)
	}
	if len(source.Address) > p.config.MaxSourceAddressBytes || !validTransport(source.Transport) {
		return Event{}, fail(CodeInvalidConfig, "Syslog source metadata exceeds configured bounds or uses invalid transport", nil)
	}
	if !utf8.Valid(input) {
		return Event{}, fail(CodeInvalidUTF8, "Syslog message is not valid UTF-8", nil)
	}
	text := string(input)
	pri, body, err := parsePRI(text)
	if err != nil {
		return Event{}, err
	}
	raw, rawTruncated := boundedEvidence(text, p.config.MaxRawEvidenceBytes)
	base := Event{
		SchemaVersion:        EventSchemaVersion,
		ReceivedAt:           receivedAt.UTC(),
		Source:               source,
		Facility:             pri / 8,
		Severity:             pri % 8,
		RawEvidence:          raw,
		RawEvidenceTruncated: rawTruncated,
	}

	first, _, hasSpace := strings.Cut(body, " ")
	if isDecimal(first) {
		if !hasSpace {
			return Event{}, fail(CodeMalformedRFC5424, "RFC5424 header is incomplete", nil)
		}
		event, err := p.parseRFC5424(base, body)
		if err != nil {
			return Event{}, err
		}
		return event, nil
	}
	if looksRFC3164(body) {
		event, confident, err := p.parseRFC3164(base, body)
		if err != nil {
			return Event{}, err
		}
		if confident {
			return event, nil
		}
		base.RawTimestamp = event.RawTimestamp
		return p.partial(base, body)
	}
	return p.partial(base, body)
}

func parsePRI(input string) (int, string, error) {
	if len(input) < 3 || input[0] != '<' {
		return 0, "", fail(CodeMalformedPRI, "Syslog message must begin with a PRI field", nil)
	}
	end := strings.IndexByte(input, '>')
	if end < 0 {
		return 0, "", fail(CodeMalformedPRI, "Syslog PRI is missing closing delimiter", nil)
	}
	if end == 1 {
		return 0, "", fail(CodeMalformedPRI, "Syslog PRI is empty", nil)
	}
	priText := input[1:end]
	if len(priText) > 3 || !isDecimal(priText) {
		return 0, "", fail(CodeMalformedPRI, "Syslog PRI must be decimal in range 0..191", nil)
	}
	pri, err := strconv.Atoi(priText)
	if err != nil || pri < 0 || pri > 191 {
		return 0, "", fail(CodeMalformedPRI, "Syslog PRI must be decimal in range 0..191", nil)
	}
	if end+1 >= len(input) {
		return 0, "", fail(CodeMalformedPRI, "Syslog message has no content after PRI", nil)
	}
	return pri, input[end+1:], nil
}

func (p *Parser) parseRFC5424(base Event, body string) (Event, error) {
	fields := make([]string, 0, 6)
	remaining := body
	for i := 0; i < 6; i++ {
		field, rest, ok := strings.Cut(remaining, " ")
		if !ok || field == "" {
			return Event{}, fail(CodeMalformedRFC5424, "RFC5424 header is incomplete", nil)
		}
		fields = append(fields, field)
		remaining = rest
	}
	if fields[0] != "1" {
		return Event{}, fail(CodeMalformedRFC5424, "unsupported RFC5424 version", nil)
	}

	base.Format = FormatRFC5424
	base.Quality = QualityGood
	base.StatusCode = ""

	if fields[1] == "-" {
		base.TimestampQuality = TimestampUnavailable
	} else {
		parsed, err := parseRFC5424Timestamp(fields[1])
		if err != nil {
			return Event{}, err
		}
		base.EventTime = &parsed
		base.RawTimestamp = fields[1]
		base.TimestampQuality = TimestampAuthoritativeRFC3339
	}
	var err error
	if base.Hostname, err = rfc5424HeaderValue(fields[2], p.config.MaxHostnameBytes, 255, "hostname"); err != nil {
		return Event{}, err
	}
	if base.AppName, err = rfc5424HeaderValue(fields[3], p.config.MaxAppNameBytes, 48, "app name"); err != nil {
		return Event{}, err
	}
	if base.ProcessID, err = rfc5424HeaderValue(fields[4], p.config.MaxProcessIDBytes, 128, "process ID"); err != nil {
		return Event{}, err
	}
	if base.MessageID, err = rfc5424HeaderValue(fields[5], p.config.MaxMessageIDBytes, 32, "message ID"); err != nil {
		return Event{}, err
	}

	elements, message, err := p.parseStructuredDataAndMessage(remaining)
	if err != nil {
		return Event{}, err
	}
	if len(message) > p.config.MaxMessageBytes {
		return Event{}, fail(CodeResourceLimit, "RFC5424 message exceeds configured maximum", nil)
	}
	base.StructuredData = elements
	base.Message = message
	if err := base.Validate(p.config); err != nil {
		return Event{}, err
	}
	return base, nil
}

func (p *Parser) parseRFC3164(base Event, body string) (Event, bool, error) {
	if len(body) < 15 {
		return base, false, nil
	}
	rawTS := body[:15]
	if _, err := time.Parse("Jan _2 15:04:05", rawTS); err != nil {
		return Event{}, false, fail(CodeMalformedRFC3164, "RFC3164 timestamp is invalid", nil)
	}
	base.RawTimestamp = rawTS
	base.TimestampQuality = TimestampRFC3164Unresolved
	if len(body) == 15 || body[15] != ' ' {
		return base, false, nil
	}
	remainder := strings.TrimLeft(body[16:], " ")
	space := strings.IndexByte(remainder, ' ')
	if space <= 0 {
		return base, false, nil
	}
	host := remainder[:space]
	if len(host) > p.config.MaxHostnameBytes {
		return Event{}, false, fail(CodeResourceLimit, "RFC3164 hostname exceeds configured maximum", nil)
	}
	afterHost := strings.TrimLeft(remainder[space+1:], " ")
	colon := strings.IndexByte(afterHost, ':')
	if colon <= 0 {
		return base, false, nil
	}
	tag := afterHost[:colon]
	message := strings.TrimPrefix(afterHost[colon+1:], " ")
	if len(message) > p.config.MaxMessageBytes {
		return Event{}, false, fail(CodeResourceLimit, "RFC3164 message exceeds configured maximum", nil)
	}
	app, pid, ok := parseTag(tag)
	if !ok {
		return Event{}, false, fail(CodeMalformedRFC3164, "RFC3164 tag/process identifier is invalid", nil)
	}
	if len(app) > p.config.MaxAppNameBytes || len(pid) > p.config.MaxProcessIDBytes {
		return Event{}, false, fail(CodeResourceLimit, "RFC3164 app/process field exceeds configured maximum", nil)
	}
	base.Format = FormatRFC3164
	base.Quality = QualityGood
	base.Hostname = host
	base.AppName = app
	base.ProcessID = pid
	base.Message = message
	if err := base.Validate(p.config); err != nil {
		return Event{}, false, err
	}
	return base, true, nil
}

func (p *Parser) partial(base Event, body string) (Event, error) {
	if len(body) > p.config.MaxMessageBytes {
		return Event{}, fail(CodeResourceLimit, "partial Syslog content exceeds configured message maximum", nil)
	}
	base.Format = FormatUnknown
	base.Quality = QualityPartial
	base.StatusCode = CodePartialUnknown
	if base.TimestampQuality == "" {
		base.TimestampQuality = TimestampUnavailable
	}
	base.Message = body
	if err := base.Validate(p.config); err != nil {
		return Event{}, err
	}
	return base, nil
}

// RFC 5424 section 6 restricts these header fields to PRINTUSASCII (33..126)
// and assigns a separate octet limit to each. MSG and SD-PARAM values have
// different grammars. Keep the receiver's configured budget error distinct.
func rfc5424HeaderValue(value string, configMax, wireMax int, field string) (string, error) {
	if value == "-" {
		return "", nil
	}
	if len(value) > configMax {
		return "", fail(CodeResourceLimit, "RFC5424 "+field+" exceeds configured maximum", nil)
	}
	if len(value) > wireMax {
		return "", fail(CodeMalformedRFC5424, "RFC5424 "+field+" exceeds protocol maximum", nil)
	}
	for i := 0; i < len(value); i++ {
		if value[i] < 33 || value[i] > 126 {
			return "", fail(CodeMalformedRFC5424, "RFC5424 "+field+" contains a non-PRINTUSASCII byte", nil)
		}
	}
	return value, nil
}

func (p *Parser) parseStructuredDataAndMessage(input string) ([]StructuredDataElement, string, error) {
	if input == "" {
		return nil, "", fail(CodeMalformedRFC5424, "RFC5424 structured-data field is missing", nil)
	}
	if input[0] == '-' {
		if len(input) == 1 {
			return nil, "", nil
		}
		if input[1] != ' ' {
			return nil, "", fail(CodeStructuredData, "RFC5424 NILVALUE structured-data must be followed by message separator", nil)
		}
		return nil, input[2:], nil
	}
	if input[0] != '[' {
		return nil, "", fail(CodeStructuredData, "RFC5424 structured-data must begin with '[' or NILVALUE", nil)
	}

	var elements []StructuredDataElement
	i := 0
	for i < len(input) && input[i] == '[' {
		if len(elements) >= p.config.MaxStructuredDataElements {
			return nil, "", fail(CodeResourceLimit, "structured-data element count exceeds configured maximum", nil)
		}
		element, next, err := p.parseStructuredElement(input, i)
		if err != nil {
			return nil, "", err
		}
		elements = append(elements, element)
		i = next
	}
	if i == len(input) {
		return elements, "", nil
	}
	if input[i] != ' ' {
		return nil, "", fail(CodeStructuredData, "structured-data is followed by invalid content", nil)
	}
	return elements, input[i+1:], nil
}

func (p *Parser) parseStructuredElement(input string, start int) (StructuredDataElement, int, error) {
	i := start + 1
	idStart := i
	for i < len(input) && input[i] != ' ' && input[i] != ']' {
		if !validSDNameByte(input[i]) {
			return StructuredDataElement{}, 0, fail(CodeStructuredData, "structured-data ID contains invalid character", nil)
		}
		i++
	}
	if i == idStart || i >= len(input) {
		return StructuredDataElement{}, 0, fail(CodeStructuredData, "structured-data element has invalid or unterminated ID", nil)
	}
	id := input[idStart:i]
	if len(id) > p.config.MaxStructuredDataIDBytes {
		return StructuredDataElement{}, 0, fail(CodeResourceLimit, "structured-data ID exceeds configured maximum", nil)
	}
	element := StructuredDataElement{ID: id}
	if input[i] == ']' {
		return element, i + 1, nil
	}

	for {
		for i < len(input) && input[i] == ' ' {
			i++
		}
		if i >= len(input) {
			return StructuredDataElement{}, 0, fail(CodeStructuredData, "structured-data element is unterminated", nil)
		}
		if input[i] == ']' {
			return element, i + 1, nil
		}
		if len(element.Params) >= p.config.MaxStructuredDataParamsPerElement {
			return StructuredDataElement{}, 0, fail(CodeResourceLimit, "structured-data parameter count exceeds configured maximum", nil)
		}
		nameStart := i
		for i < len(input) && input[i] != '=' {
			if input[i] == ' ' || input[i] == ']' || !validSDNameByte(input[i]) {
				return StructuredDataElement{}, 0, fail(CodeStructuredData, "structured-data parameter name is invalid", nil)
			}
			i++
		}
		if i == nameStart || i >= len(input) || input[i] != '=' {
			return StructuredDataElement{}, 0, fail(CodeStructuredData, "structured-data parameter assignment is invalid", nil)
		}
		name := input[nameStart:i]
		if len(name) > p.config.MaxStructuredDataParamNameBytes {
			return StructuredDataElement{}, 0, fail(CodeResourceLimit, "structured-data parameter name exceeds configured maximum", nil)
		}
		i++
		if i >= len(input) || input[i] != '"' {
			return StructuredDataElement{}, 0, fail(CodeStructuredData, "structured-data parameter value must be quoted", nil)
		}
		i++
		var value strings.Builder
		for {
			if i >= len(input) {
				return StructuredDataElement{}, 0, fail(CodeStructuredData, "structured-data parameter value is unterminated", nil)
			}
			ch := input[i]
			if ch == '"' {
				i++
				break
			}
			if ch == ']' {
				return StructuredDataElement{}, 0, fail(CodeStructuredData, "structured-data closing bracket must be escaped inside parameter value", nil)
			}
			if ch == '\\' {
				i++
				if i >= len(input) {
					return StructuredDataElement{}, 0, fail(CodeStructuredData, "structured-data escape is incomplete", nil)
				}
				ch = input[i]
				if ch != '"' && ch != '\\' && ch != ']' {
					return StructuredDataElement{}, 0, fail(CodeStructuredData, "structured-data escape sequence is invalid", nil)
				}
			}
			if value.Len()+1 > p.config.MaxStructuredDataParamValueBytes {
				return StructuredDataElement{}, 0, fail(CodeResourceLimit, "structured-data parameter value exceeds configured maximum", nil)
			}
			value.WriteByte(ch)
			i++
		}
		element.Params = append(element.Params, StructuredDataParam{Name: name, Value: value.String()})
		if i >= len(input) {
			return StructuredDataElement{}, 0, fail(CodeStructuredData, "structured-data element is unterminated", nil)
		}
		if input[i] != ' ' && input[i] != ']' {
			return StructuredDataElement{}, 0, fail(CodeStructuredData, "structured-data parameter separator is invalid", nil)
		}
	}
}

func parseTag(tag string) (string, string, bool) {
	if tag == "" {
		return "", "", false
	}
	open := strings.LastIndexByte(tag, '[')
	if open < 0 {
		return tag, "", true
	}
	if !strings.HasSuffix(tag, "]") || open == 0 || open == len(tag)-2 {
		return "", "", false
	}
	pid := tag[open+1 : len(tag)-1]
	if strings.ContainsAny(pid, "[]") {
		return "", "", false
	}
	return tag[:open], pid, true
}

func looksRFC3164(body string) bool {
	if len(body) < 3 {
		return false
	}
	switch body[:3] {
	case "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec":
		return true
	default:
		return false
	}
}

func isDecimal(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// parseRFC5424Timestamp enforces RFC 5424 section 6 TIMESTAMP lexical grammar
// before semantic time.Parse. Rejects leap seconds, invalid calendar dates,
// overlong/missing fractions, lowercase T/Z, and illegal offsets. Valid values
// are converted to UTC. No silent truncation or permissive compatibility.
func parseRFC5424Timestamp(raw string) (time.Time, error) {
	if !validRFC5424TimestampLexical(raw) {
		return time.Time{}, newRFC5424TimestampError()
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, newRFC5424TimestampError()
	}
	return parsed.UTC(), nil
}

func newRFC5424TimestampError() error {
	return fail(CodeMalformedRFC5424, "RFC5424 timestamp is invalid", nil)
}

// validRFC5424TimestampLexical checks FULL-DATE "T" FULL-TIME per RFC 5424 §6:
// fixed-width date/time, uppercase T, optional "." 1*6DIGIT, then "Z" or
// signed HH:MM offset with hours 00..23 and minutes 00..59; seconds 00..59.
func validRFC5424TimestampLexical(raw string) bool {
	// Minimum without fraction: YYYY-MM-DDTHH:MM:SSZ
	if len(raw) < 20 {
		return false
	}
	if !asciiDigitRun(raw, 0, 4) || raw[4] != '-' ||
		!asciiDigitRun(raw, 5, 2) || raw[7] != '-' ||
		!asciiDigitRun(raw, 8, 2) {
		return false
	}
	year := asciiAtoi(raw, 0, 4)
	month := asciiAtoi(raw, 5, 2)
	day := asciiAtoi(raw, 8, 2)
	if month < 1 || month > 12 || day < 1 || day > gregorianDaysInMonth(year, month) {
		return false
	}
	if raw[10] != 'T' {
		return false
	}
	if !asciiDigitRun(raw, 11, 2) || raw[13] != ':' ||
		!asciiDigitRun(raw, 14, 2) || raw[16] != ':' ||
		!asciiDigitRun(raw, 17, 2) {
		return false
	}
	hour := asciiAtoi(raw, 11, 2)
	minute := asciiAtoi(raw, 14, 2)
	second := asciiAtoi(raw, 17, 2)
	if hour > 23 || minute > 59 || second > 59 {
		return false
	}
	i := 19
	if i < len(raw) && raw[i] == '.' {
		i++
		start := i
		for i < len(raw) && raw[i] >= '0' && raw[i] <= '9' {
			i++
		}
		fracLen := i - start
		if fracLen < 1 || fracLen > 6 {
			return false
		}
	}
	if i >= len(raw) {
		return false
	}
	switch raw[i] {
	case 'Z':
		return i+1 == len(raw)
	case '+', '-':
		i++
		if i+5 != len(raw) {
			return false
		}
		if !asciiDigitRun(raw, i, 2) || raw[i+2] != ':' || !asciiDigitRun(raw, i+3, 2) {
			return false
		}
		offHour := asciiAtoi(raw, i, 2)
		offMinute := asciiAtoi(raw, i+3, 2)
		return offHour <= 23 && offMinute <= 59
	default:
		return false
	}
}

func asciiDigitRun(s string, start, n int) bool {
	if start < 0 || n < 0 || start+n > len(s) {
		return false
	}
	for i := 0; i < n; i++ {
		c := s[start+i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func asciiAtoi(s string, start, n int) int {
	v := 0
	for i := 0; i < n; i++ {
		v = v*10 + int(s[start+i]-'0')
	}
	return v
}

func gregorianDaysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if gregorianLeapYear(year) {
			return 29
		}
		return 28
	default:
		return 0
	}
}

func gregorianLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

func validSDNameByte(b byte) bool {
	return b >= 33 && b <= 126 && b != '=' && b != ']' && b != '"'
}
