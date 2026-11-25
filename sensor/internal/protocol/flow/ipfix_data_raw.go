package flow

import (
	"encoding/binary"
	"errors"
)

const (
	ipfixVariableLength = 65535
	// Legal enclosing Data Set length is bounded by the IPFIX Message size
	// ceiling minus the 16-octet Message Header (RFC 7011 §§3.1, 3.3.2).
	ipfixMaxRawDataSetLength = ipfixMaxMessageLength - ipfixMessageHeaderLength // 65519
	ipfixMaxRawDataSetBody   = ipfixMaxRawDataSetLength - ipfixSetHeaderLength  // 65515
	ipfixMaxRawDataFields    = 65535
	ipfixMaxElementID        = 0x7fff
)

var (
	ErrMalformedIPFIXRawData       = errors.New("malformed IPFIX raw data set")
	ErrInvalidIPFIXRawDataTemplate = errors.New("invalid IPFIX raw data template")
	ErrIPFIXRawDataBudgetExceeded  = errors.New("IPFIX raw data decode budget exceeded")
)

// IPFIXRawDataLimits optionally caps metadata amplification beyond the hard
// wire-geometry bound (records ≤ len(body)/minRecord, value bytes ≤ len(body)).
// A zero MaxRecords or MaxFieldBytes disables that individual soft cap.
// Negative values are rejected with ErrIPFIXRawDataBudgetExceeded.
// Callers with a remaining budget of zero must stop decoding themselves;
// do not pass zero as a "cap of zero" (zero means disable).
// Limits do not invent a production capacity target; callers supply policy.
type IPFIXRawDataLimits struct {
	MaxRecords    int
	MaxFieldBytes int
}

// IPFIXRawDataField is one decoded Information Element value with the template
// descriptor that produced it. Value owns its storage and never includes the
// variable-length prefix octets. Length retains the template-declared size
// (65535 for variable-length fields). Enterprise metadata is preserved as-is.
type IPFIXRawDataField struct {
	ElementID        uint16
	Length           uint16
	EnterpriseNumber uint32
	Enterprise       bool
	Value            []byte
}

// IPFIXRawDataRecord is one Data Record in wire order. For Options Templates,
// the first ScopeCount fields are the scope; the remainder are non-scope.
type IPFIXRawDataRecord struct {
	Fields []IPFIXRawDataField
}

// IPFIXRawDataSet is the atomic decode of one already-framed Data Set using an
// explicitly supplied raw template. It is not a template cache, exporter
// binding, sequence tracker or persistence layer. Observation-domain and
// tenant authority remain owned by later exporter-bound layers.
type IPFIXRawDataSet struct {
	TemplateID uint16
	Options    bool
	ScopeCount uint16
	Records    []IPFIXRawDataRecord
}

// ParseIPFIXRawDataSet decodes one complete IPFIX Data Set (Set Header plus
// body) with an explicitly supplied Template or Options Template definition.
//
// Callers MUST NOT mutate set or template field descriptors concurrently with
// this call. On success the returned set owns all record/field/value storage
// independently of the inputs. On any error the result is nil (atomic reject):
// no partial records are returned.
//
// Rejection covers withdrawn/empty/mismatched/invalid templates, Set ID < 256,
// enclosing Set longer than ipfixMaxRawDataSetLength, truncated or
// nonadvancing records, illegal padding (non-zero or not shorter than the
// template minimum record length), and optional caller budgets.
func ParseIPFIXRawDataSet(set []byte, template IPFIXRawTemplate, limits IPFIXRawDataLimits) (*IPFIXRawDataSet, error) {
	if limits.MaxRecords < 0 || limits.MaxFieldBytes < 0 {
		return nil, ErrIPFIXRawDataBudgetExceeded
	}
	if err := validateIPFIXRawDataTemplate(template); err != nil {
		return nil, err
	}
	if len(set) < ipfixSetHeaderLength {
		return nil, ErrMalformedIPFIXRawData
	}
	setID := binary.BigEndian.Uint16(set[0:2])
	length := int(binary.BigEndian.Uint16(set[2:4]))
	if length != len(set) || length > ipfixMaxRawDataSetLength || setID != template.ID || setID < 256 {
		return nil, ErrMalformedIPFIXRawData
	}
	body := set[ipfixSetHeaderLength:]
	minLen, err := ipfixRawDataMinRecordLength(template.Fields)
	if err != nil {
		return nil, err
	}
	// Impossible for even one record: reject before scanning or copying values.
	if minLen > len(body) {
		return nil, ErrMalformedIPFIXRawData
	}
	records, valueBytes, err := scanIPFIXRawDataSet(body, template.Fields, minLen, nil, nil, nil, limits)
	if err != nil {
		return nil, err
	}
	out := &IPFIXRawDataSet{
		TemplateID: template.ID,
		Options:    template.Options,
		ScopeCount: template.ScopeCount,
		Records:    make([]IPFIXRawDataRecord, records),
	}
	fields := make([]IPFIXRawDataField, records*len(template.Fields))
	values := make([]byte, valueBytes)
	if _, _, err := scanIPFIXRawDataSet(body, template.Fields, minLen, out.Records, fields, values, limits); err != nil {
		return nil, err
	}
	return out, nil
}

func validateIPFIXRawDataTemplate(template IPFIXRawTemplate) error {
	if template.Withdrawal || template.AllTemplates || template.ID < 256 || len(template.Fields) == 0 {
		return ErrInvalidIPFIXRawDataTemplate
	}
	if len(template.Fields) > ipfixMaxRawDataFields {
		return ErrInvalidIPFIXRawDataTemplate
	}
	if template.Options {
		if template.ScopeCount == 0 || int(template.ScopeCount) > len(template.Fields) {
			return ErrInvalidIPFIXRawDataTemplate
		}
	} else if template.ScopeCount != 0 {
		return ErrInvalidIPFIXRawDataTemplate
	}
	for _, field := range template.Fields {
		if field.ElementID > ipfixMaxElementID {
			return ErrInvalidIPFIXRawDataTemplate
		}
		if field.Length == 0 {
			return ErrInvalidIPFIXRawDataTemplate
		}
		if !field.Enterprise && field.EnterpriseNumber != 0 {
			return ErrInvalidIPFIXRawDataTemplate
		}
	}
	return nil
}

// ipfixRawDataMinRecordLength is the shortest encoding of one Data Record:
// fixed fields contribute their declared sizes; each variable-length field
// contributes at least the one-octet length prefix used for a zero value.
// Accumulation uses checked addition bounded by the maximum legal Set body
// so synthetic excessive widths cannot overflow before reject.
func ipfixRawDataMinRecordLength(fields []IPFIXRawField) (int, error) {
	min := 0
	for _, field := range fields {
		var add int
		if field.Length == ipfixVariableLength {
			add = 1
		} else if field.Length == 0 {
			return 0, ErrInvalidIPFIXRawDataTemplate
		} else {
			add = int(field.Length)
		}
		if add > ipfixMaxRawDataSetBody-min {
			return 0, ErrInvalidIPFIXRawDataTemplate
		}
		min += add
	}
	if min == 0 {
		return 0, ErrInvalidIPFIXRawDataTemplate
	}
	return min, nil
}

// scanIPFIXRawDataSet validates record geometry. With nil destinations it only
// counts; with destinations it copies every Information Element value into the
// pre-sized private values buffer and attaches capped field slices per record.
func scanIPFIXRawDataSet(body []byte, descriptors []IPFIXRawField, minLen int, records []IPFIXRawDataRecord, fields []IPFIXRawDataField, values []byte, limits IPFIXRawDataLimits) (int, int, error) {
	if len(body) == 0 {
		return 0, 0, ErrMalformedIPFIXRawData
	}
	var (
		count      int
		valueBytes int
		pos        int
		valueOff   int
		fieldSlot  int
	)
	for {
		remaining := len(body) - pos
		if remaining == 0 {
			break
		}
		if remaining < minLen {
			if count == 0 || !allZeroIPFIXPadding(body[pos:]) {
				return 0, 0, ErrMalformedIPFIXRawData
			}
			break
		}
		start := pos
		for _, desc := range descriptors {
			var (
				src []byte
				n   int
			)
			if desc.Length == ipfixVariableLength {
				if len(body)-pos < 1 {
					return 0, 0, ErrMalformedIPFIXRawData
				}
				prefix := body[pos]
				pos++
				if prefix < 255 {
					n = int(prefix)
				} else {
					if len(body)-pos < 2 {
						return 0, 0, ErrMalformedIPFIXRawData
					}
					n = int(binary.BigEndian.Uint16(body[pos : pos+2]))
					pos += 2
				}
				if len(body)-pos < n {
					return 0, 0, ErrMalformedIPFIXRawData
				}
				src = body[pos : pos+n]
				pos += n
			} else {
				n = int(desc.Length)
				if len(body)-pos < n {
					return 0, 0, ErrMalformedIPFIXRawData
				}
				src = body[pos : pos+n]
				pos += n
			}
			if records != nil {
				field := IPFIXRawDataField{
					ElementID:        desc.ElementID,
					Length:           desc.Length,
					EnterpriseNumber: desc.EnterpriseNumber,
					Enterprise:       desc.Enterprise,
				}
				if n == 0 {
					field.Value = values[valueOff:valueOff:valueOff]
				} else {
					copy(values[valueOff:valueOff+n], src)
					field.Value = values[valueOff : valueOff+n : valueOff+n]
					valueOff += n
				}
				fields[fieldSlot] = field
				fieldSlot++
			}
			valueBytes += n
		}
		if pos-start < minLen {
			return 0, 0, ErrMalformedIPFIXRawData
		}
		count++
		if limits.MaxRecords > 0 && count > limits.MaxRecords {
			return 0, 0, ErrIPFIXRawDataBudgetExceeded
		}
		if limits.MaxFieldBytes > 0 && valueBytes > limits.MaxFieldBytes {
			return 0, 0, ErrIPFIXRawDataBudgetExceeded
		}
		if records != nil {
			from := (count - 1) * len(descriptors)
			to := count * len(descriptors)
			records[count-1].Fields = fields[from:to:to]
		}
	}
	if count == 0 {
		return 0, 0, ErrMalformedIPFIXRawData
	}
	return count, valueBytes, nil
}
