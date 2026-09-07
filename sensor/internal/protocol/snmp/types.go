package snmp

import (
	"encoding/json"
	"strconv"
	"time"
)

const ResultSchemaVersion = 1

type Version string

const (
	Version1  Version = "1"
	Version2c Version = "2c"
	Version3  Version = "3"
)

type Operation string

const (
	OperationGet      Operation = "GET"
	OperationGetNext  Operation = "GETNEXT"
	OperationGetBulk  Operation = "GETBULK"
	OperationWalk     Operation = "WALK"
	OperationBulkWalk Operation = "BULKWALK"
)

type SecurityLevel string

const (
	SecurityNoAuthNoPriv SecurityLevel = "NO_AUTH_NO_PRIV"
	SecurityAuthNoPriv   SecurityLevel = "AUTH_NO_PRIV"
	SecurityAuthPriv     SecurityLevel = "AUTH_PRIV"
)

type AuthProtocol string

const (
	AuthNone   AuthProtocol = "NONE"
	AuthMD5    AuthProtocol = "MD5"
	AuthSHA1   AuthProtocol = "SHA1"
	AuthSHA224 AuthProtocol = "SHA224"
	AuthSHA256 AuthProtocol = "SHA256"
	AuthSHA384 AuthProtocol = "SHA384"
	AuthSHA512 AuthProtocol = "SHA512"
)

type PrivacyProtocol string

const (
	PrivacyNone    PrivacyProtocol = "NONE"
	PrivacyDES     PrivacyProtocol = "DES"
	PrivacyAES     PrivacyProtocol = "AES"
	PrivacyAES192  PrivacyProtocol = "AES192"
	PrivacyAES256  PrivacyProtocol = "AES256"
	PrivacyAES192C PrivacyProtocol = "AES192C"
	PrivacyAES256C PrivacyProtocol = "AES256C"
)

type Limits struct {
	MaxOIDs            int    `json:"maxOids"`
	MaxWalkEntries     int    `json:"maxWalkEntries"`
	MaxBulkRepetitions uint32 `json:"maxBulkRepetitions"`
	MaxResults         int    `json:"maxResults"`
	MaxResultBytes     int    `json:"maxResultBytes"`
}

type RetryPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	JitterFraction float64
}

type Request struct {
	Target           string
	Port             int
	Version          Version
	Credentials      Credentials
	OIDs             []string
	Operation        Operation
	Timeout          time.Duration
	ExecutionTimeout time.Duration
	Retry            RetryPolicy
	Limits           Limits
	CorrelationID    string
}

func (r Request) String() string {
	return "SNMPRequest{target=" + r.Target + ",port=" + strconv.Itoa(r.Port) + ",version=" + string(r.Version) + ",operation=" + string(r.Operation) + ",oids=" + strconv.Itoa(len(r.OIDs)) + ",credentials=" + r.Credentials.String() + "}"
}

func (r Request) GoString() string { return r.String() }

type Quality string

const (
	QualityGood     Quality = "GOOD"
	QualityDegraded Quality = "DEGRADED"
	QualityUnknown  Quality = "UNKNOWN"
)

type ObservationStatus string

const (
	ObservationValue     ObservationStatus = "VALUE"
	ObservationException ObservationStatus = "EXCEPTION"
)

type ValueKind string

const (
	ValueInteger   ValueKind = "INTEGER"
	ValueUnsigned  ValueKind = "UNSIGNED"
	ValueCounter64 ValueKind = "COUNTER64"
	ValueOctets    ValueKind = "OCTETS"
	ValueBitString ValueKind = "BIT_STRING"
	ValueOID       ValueKind = "OBJECT_IDENTIFIER"
	ValueIPAddress ValueKind = "IP_ADDRESS"
	ValueNull      ValueKind = "NULL"
	ValueOpaque    ValueKind = "OPAQUE"
	ValueException ValueKind = "EXCEPTION"
)

type Value struct {
	Kind      ValueKind `json:"kind"`
	Integer   *int64    `json:"integer,omitempty"`
	Unsigned  *uint64   `json:"unsigned,omitempty"`
	Counter64 *uint64   `json:"-"`
	Bytes     []byte    `json:"bytes,omitempty"`
	Text      *string   `json:"text,omitempty"`
	BitLength *int      `json:"bitLength,omitempty"`
}

// MarshalJSON renders Counter64 as a decimal string so downstream JSON consumers
// cannot lose precision by interpreting the value as an IEEE-754 number.
func (v Value) MarshalJSON() ([]byte, error) {
	type valueWire struct {
		Kind      ValueKind `json:"kind"`
		Integer   *int64    `json:"integer,omitempty"`
		Unsigned  *uint64   `json:"unsigned,omitempty"`
		Counter64 *string   `json:"counter64,omitempty"`
		Bytes     []byte    `json:"bytes,omitempty"`
		Text      *string   `json:"text,omitempty"`
		BitLength *int      `json:"bitLength,omitempty"`
	}
	wire := valueWire{
		Kind:      v.Kind,
		Integer:   v.Integer,
		Unsigned:  v.Unsigned,
		Bytes:     v.Bytes,
		Text:      v.Text,
		BitLength: v.BitLength,
	}
	if v.Counter64 != nil {
		encoded := strconv.FormatUint(*v.Counter64, 10)
		wire.Counter64 = &encoded
	}
	return json.Marshal(wire)
}

type Observation struct {
	OID         string            `json:"oid"`
	SourceOID   string            `json:"sourceOid"`
	BERType     uint8             `json:"berType"`
	BERTypeName string            `json:"berTypeName"`
	CollectedAt time.Time         `json:"collectedAt"`
	Quality     Quality           `json:"quality"`
	Status      ObservationStatus `json:"status"`
	ErrorCode   ErrorCode         `json:"errorCode,omitempty"`
	Value       Value             `json:"value"`
}

type CollectionResult struct {
	SchemaVersion int           `json:"schemaVersion"`
	Version       Version       `json:"version"`
	Operation     Operation     `json:"operation"`
	CollectedAt   time.Time     `json:"collectedAt"`
	Observations  []Observation `json:"observations"`
}
