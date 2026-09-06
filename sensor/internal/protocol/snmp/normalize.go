package snmp

import (
	"fmt"
	"math"
	"net"
	"time"

	"github.com/gosnmp/gosnmp"
)

func normalizePDU(sourceOID string, pdu gosnmp.SnmpPDU, collectedAt time.Time) (Observation, error) {
	oid, err := CanonicalOID(pdu.Name)
	if err != nil {
		return Observation{}, newError(CodeMalformedResponse, "response contains malformed OID", err, false)
	}
	source, err := CanonicalOID(sourceOID)
	if err != nil {
		return Observation{}, newError(CodeInvalidRequest, "source OID is malformed", err, false)
	}

	observation := Observation{
		OID:         oid,
		SourceOID:   source,
		BERType:     uint8(pdu.Type),
		BERTypeName: pdu.Type.String(),
		CollectedAt: collectedAt.UTC(),
		Quality:     QualityGood,
		Status:      ObservationValue,
	}

	switch pdu.Type {
	case gosnmp.Integer:
		value, ok := signedInteger(pdu.Value)
		if !ok {
			return Observation{}, malformedType(pdu.Type)
		}
		observation.Value = Value{Kind: ValueInteger, Integer: &value}
	case gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks, gosnmp.Uinteger32:
		value, ok := unsigned32(pdu.Value)
		if !ok {
			return Observation{}, malformedType(pdu.Type)
		}
		observation.Value = Value{Kind: ValueUnsigned, Unsigned: &value}
	case gosnmp.Counter64:
		value, ok := unsigned64(pdu.Value)
		if !ok {
			return Observation{}, malformedType(pdu.Type)
		}
		observation.Value = Value{Kind: ValueCounter64, Counter64: &value}
	case gosnmp.OctetString:
		value, ok := octets(pdu.Value)
		if !ok {
			return Observation{}, malformedType(pdu.Type)
		}
		observation.Value = Value{Kind: ValueOctets, Bytes: value}
	case gosnmp.Opaque:
		value, ok := octets(pdu.Value)
		if !ok {
			return Observation{}, malformedType(pdu.Type)
		}
		observation.Value = Value{Kind: ValueOpaque, Bytes: value}
	case gosnmp.BitString:
		value, ok := pdu.Value.(gosnmp.BitStringValue)
		if !ok {
			return Observation{}, malformedType(pdu.Type)
		}
		bytesCopy := append([]byte(nil), value.Bytes...)
		bitLength := value.BitLength
		observation.Value = Value{Kind: ValueBitString, Bytes: bytesCopy, BitLength: &bitLength}
	case gosnmp.ObjectIdentifier:
		value, ok := pdu.Value.(string)
		if !ok {
			return Observation{}, malformedType(pdu.Type)
		}
		canonical, err := CanonicalOID(value)
		if err != nil {
			return Observation{}, newError(CodeMalformedResponse, "object identifier value is malformed", err, false)
		}
		observation.Value = Value{Kind: ValueOID, Text: &canonical}
	case gosnmp.IPAddress:
		value, ok := normalizeIP(pdu.Value)
		if !ok {
			return Observation{}, malformedType(pdu.Type)
		}
		observation.Value = Value{Kind: ValueIPAddress, Text: &value}
	case gosnmp.Null:
		observation.Value = Value{Kind: ValueNull}
	case gosnmp.NoSuchObject:
		observation.Status = ObservationException
		observation.Quality = QualityUnknown
		observation.ErrorCode = CodeNoSuchObject
		observation.Value = Value{Kind: ValueException}
	case gosnmp.NoSuchInstance:
		observation.Status = ObservationException
		observation.Quality = QualityUnknown
		observation.ErrorCode = CodeNoSuchInstance
		observation.Value = Value{Kind: ValueException}
	case gosnmp.EndOfMibView:
		observation.Status = ObservationException
		observation.Quality = QualityUnknown
		observation.ErrorCode = CodeEndOfMibView
		observation.Value = Value{Kind: ValueException}
	default:
		return Observation{}, newError(CodeMalformedResponse, "unsupported BER value type in response", nil, false)
	}
	return observation, nil
}

func malformedType(ber gosnmp.Asn1BER) error {
	return newError(CodeMalformedResponse, fmt.Sprintf("response value does not match BER type %s", ber.String()), nil, false)
}

func signedInteger(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	default:
		return 0, false
	}
}

func unsigned32(value any) (uint64, bool) {
	switch v := value.(type) {
	case uint8:
		return uint64(v), true
	case uint16:
		return uint64(v), true
	case uint32:
		return uint64(v), true
	case uint:
		if uint64(v) <= math.MaxUint32 {
			return uint64(v), true
		}
	case uint64:
		if v <= math.MaxUint32 {
			return v, true
		}
	case int:
		if v >= 0 && uint64(v) <= math.MaxUint32 {
			return uint64(v), true
		}
	}
	return 0, false
}

func unsigned64(value any) (uint64, bool) {
	switch v := value.(type) {
	case uint64:
		return v, true
	case uint:
		return uint64(v), true
	case uint32:
		return uint64(v), true
	case uint16:
		return uint64(v), true
	case uint8:
		return uint64(v), true
	default:
		return 0, false
	}
}

func octets(value any) ([]byte, bool) {
	switch v := value.(type) {
	case []byte:
		return append([]byte(nil), v...), true
	case string:
		return []byte(v), true
	default:
		return nil, false
	}
}

func normalizeIP(value any) (string, bool) {
	var ip net.IP
	switch v := value.(type) {
	case string:
		ip = net.ParseIP(v)
	case []byte:
		ip = net.IP(append([]byte(nil), v...))
	case net.IP:
		ip = append(net.IP(nil), v...)
	default:
		return "", false
	}
	if ip == nil {
		return "", false
	}
	return ip.String(), true
}
