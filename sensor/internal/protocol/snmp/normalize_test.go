package snmp

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
)

func TestGetNormalization(t *testing.T) {
	when := time.Unix(1700000000, 0).UTC()
	obs, err := normalizePDU(".1.3.6.1.2.1.1.3.0", gosnmp.SnmpPDU{
		Name:  ".1.3.6.1.2.1.1.3.0",
		Type:  gosnmp.Integer,
		Value: int(42),
	}, when)
	if err != nil {
		t.Fatalf("normalizePDU() error = %v", err)
	}
	if obs.OID != ".1.3.6.1.2.1.1.3.0" || obs.SourceOID != obs.OID || obs.Value.Integer == nil || *obs.Value.Integer != 42 {
		t.Fatalf("unexpected observation: %+v", obs)
	}
	if obs.CollectedAt != when || obs.Status != ObservationValue || obs.Quality != QualityGood {
		t.Fatalf("unexpected metadata: %+v", obs)
	}
}

func TestCounter64PreservesFullUnsignedValue(t *testing.T) {
	obs, err := normalizePDU(".1.3.6.1.2.1.31.1.1.1.10.1", gosnmp.SnmpPDU{
		Name:  ".1.3.6.1.2.1.31.1.1.1.10.1",
		Type:  gosnmp.Counter64,
		Value: uint64(math.MaxUint64),
	}, time.Now())
	if err != nil {
		t.Fatalf("normalizePDU() error = %v", err)
	}
	if obs.Value.Counter64 == nil || *obs.Value.Counter64 != math.MaxUint64 {
		t.Fatalf("Counter64 = %+v, want %d", obs.Value.Counter64, uint64(math.MaxUint64))
	}

	encoded, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	value := decoded["value"].(map[string]any)
	counter, ok := value["counter64"].(string)
	if !ok || counter != "18446744073709551615" {
		t.Fatalf("counter64 JSON representation = %#v, want exact decimal string", value["counter64"])
	}
	if _, exists := value["unsigned"]; exists {
		t.Fatal("Counter64 must not be serialized through generic JSON number field")
	}
}

func TestOctetStringHandling(t *testing.T) {
	input := []byte{0x00, 0xff, 0x41, 0x42}
	obs, err := normalizePDU(".1.3.6.1.2.1.1.4.0", gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.1.4.0", Type: gosnmp.OctetString, Value: input}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	input[0] = 0x99
	if !reflect.DeepEqual(obs.Value.Bytes, []byte{0x00, 0xff, 0x41, 0x42}) {
		t.Fatalf("octets not copied safely: %v", obs.Value.Bytes)
	}
}

func TestTimeTicksHandling(t *testing.T) {
	obs, err := normalizePDU(".1.3.6.1.2.1.1.3.0", gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(4294967295)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if obs.Value.Unsigned == nil || *obs.Value.Unsigned != math.MaxUint32 {
		t.Fatalf("TimeTicks = %+v", obs.Value)
	}
}

func TestObjectIdentifierHandling(t *testing.T) {
	obs, err := normalizePDU(".1.3.6.1.2.1.1.2.0", gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.1.2.0", Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.9"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if obs.Value.Text == nil || *obs.Value.Text != ".1.3.6.1.4.1.9" {
		t.Fatalf("OID value = %+v", obs.Value)
	}
}

func TestIPAddressHandling(t *testing.T) {
	obs, err := normalizePDU(".1.3.6.1.2.1.4.20.1.1.1", gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.4.20.1.1.1", Type: gosnmp.IPAddress, Value: "192.0.2.7"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if obs.Value.Text == nil || *obs.Value.Text != "192.0.2.7" {
		t.Fatalf("IP value = %+v", obs.Value)
	}
}

func TestSpecialExceptionMappings(t *testing.T) {
	cases := []struct {
		ber  gosnmp.Asn1BER
		code ErrorCode
	}{
		{gosnmp.NoSuchObject, CodeNoSuchObject},
		{gosnmp.NoSuchInstance, CodeNoSuchInstance},
		{gosnmp.EndOfMibView, CodeEndOfMibView},
	}
	for _, tc := range cases {
		obs, err := normalizePDU(".1.3.6.1.2.1.1.99.0", gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.1.99.0", Type: tc.ber}, time.Now())
		if err != nil {
			t.Fatalf("type %v: %v", tc.ber, err)
		}
		if obs.Status != ObservationException || obs.ErrorCode != tc.code || obs.Value.Kind != ValueException {
			t.Fatalf("type %v mapped to %+v", tc.ber, obs)
		}
	}
}

func FuzzOctetStringNormalization(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte{0x00, 0xff})
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = normalizePDU(".1.3.6.1.2.1.1.4.0", gosnmp.SnmpPDU{
			Name:  ".1.3.6.1.2.1.1.4.0",
			Type:  gosnmp.OctetString,
			Value: input,
		}, time.Unix(0, 0))
	})
}
