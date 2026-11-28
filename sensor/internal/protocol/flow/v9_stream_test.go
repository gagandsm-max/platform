package flow

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
	"time"
)

func testV9Stream(t *testing.T, ids, fields int) (*V9Stream, V9StreamKey, time.Time) {
	t.Helper()
	key := V9StreamKey{ExporterBinding: "fixture", Source: netip.MustParseAddrPort("192.0.2.1:12345"), Collector: netip.MustParseAddrPort("[2001:db8::1]:2055"), SourceID: 0xdeadbeef, Epoch: 1}
	const testOnlyExpandedFieldBudget = 16 * 1024 * 1024
	s, err := NewV9Stream(key, V9StreamLimits{Timeout: 10 * time.Second, MaxTemplates: ids, MaxFields: fields, MaxExpandedFields: testOnlyExpandedFieldBudget})
	if err != nil {
		t.Fatal(err)
	}
	return s, key, time.Unix(1000, 0).UTC()
}
func v9WirePacket(seq, uptime, unix uint32, sets ...[]byte) []byte {
	pkt := v9TemplateTestPacket(sets...)
	binary.BigEndian.PutUint32(pkt[4:], uptime)
	binary.BigEndian.PutUint32(pkt[8:], unix)
	binary.BigEndian.PutUint32(pkt[12:], seq)
	return pkt
}

func TestV9CacheRefreshExpiryAndReclamation(t *testing.T) {
	s, k, at := testV9Stream(t, 2, 3)
	def := func(id uint16, fields ...uint16) []byte {
		words := []uint16{id, uint16(len(fields))}
		for _, width := range fields {
			words = append(words, 8, width)
		}
		return v9TemplateTestSet(0, v9TemplateTestWords(words...))
	}
	data := func(id uint16) []byte { return v9TemplateTestSet(id, []byte{1, 2, 3, 4}) }
	call := func(seq uint32, sec int, expect error, sets ...[]byte) *V9RecordPacket {
		t.Helper()
		p, e := s.Decode(k, at.Add(time.Duration(sec)*time.Second), v9WirePacket(seq, 0, 0, sets...))
		if !errors.Is(e, expect) || (p == nil) != (expect != nil) {
			t.Fatalf("seq %d got %+v, %v; want %v", seq, p, e, expect)
		}
		return p
	}
	call(1, 0, nil, def(256, 4), def(257, 2, 2))
	call(2, 3, nil, def(256, 4))
	p := call(3, 10, nil, data(257), data(256), def(258, 2, 2), data(257), data(258))
	for i, want := range []V9SetStatus{V9TemplateExpired, V9Decoded, V9TemplateMissing, V9Decoded} {
		if p.Sets[i].Status != want {
			t.Fatalf("set %d: %+v", i, p.Sets[i])
		}
	}
	if p.Sets[0].RefreshedAt != at || p.Sets[1].RefreshedAt != at.Add(3*time.Second) {
		t.Fatal("wrong refresh evidence")
	}
	p = call(4, 13, nil, data(256))
	if p.Sets[0].Status != V9TemplateExpired {
		t.Fatal("accepted at exact deadline")
	}
	call(5, 13, ErrMalformedV9Record, def(256, 8), data(256))
	call(5, 13, ErrMalformedV9Template, def(256, 4), def(257, 0))
	p = call(5, 13, nil, data(256), data(258))
	if p.Sets[0].Status != V9TemplateExpired || p.Sets[1].Count != 1 {
		t.Fatal("malformed packet mutated cache")
	}
}

func TestV9ContinuityAndScope(t *testing.T) {
	s, k, at := testV9Stream(t, 1, 1)
	def := v9TemplateTestSet(0, v9TemplateTestWords(256, 1, 8, 4))
	data := v9TemplateTestSet(256, []byte{1, 2, 3, 4})
	initial := v9WirePacket(0xfffffffe, 0xffffff00, 100, def)
	mustFail := func(key V9StreamKey, when time.Time, wire []byte, want error) {
		t.Helper()
		p, err := s.Decode(key, when, wire)
		if p != nil || !errors.Is(err, want) {
			t.Fatalf("got %+v %v, wanted %v", p, err, want)
		}
	}
	if _, err := s.Decode(k, at, initial); err != nil {
		t.Fatal(err)
	}
	mustFail(k, at, initial, ErrV9DuplicatePacket)
	for _, change := range []func(*V9StreamKey){
		func(k *V9StreamKey) { k.Epoch++ },
		func(k *V9StreamKey) { k.SourceID++ },
		func(k *V9StreamKey) { k.Collector = k.Source },
		func(k *V9StreamKey) { k.Source = k.Collector },
		func(k *V9StreamKey) { k.ExporterBinding = "other" },
	} {
		wrongKey := k
		change(&wrongKey)
		mustFail(wrongKey, at, initial, ErrV9StreamScope)
	}
	wrong := v9WirePacket(0xffffffff, 200, 101, data)
	binary.BigEndian.PutUint32(wrong[16:], 9)
	mustFail(k, at, wrong, ErrV9StreamScope)
	p, err := s.Decode(k, at, v9WirePacket(0xffffffff, 744, 101, data))
	if err != nil || p.Sets[0].Count != 1 {
		t.Fatalf("ordinary uptime wrap: %+v %v", p, err)
	}
	p, err = s.Decode(k, at, v9WirePacket(0, 745, 101, data))
	if err != nil || p.MissingPackets != 0 || p.Sets[0].Count != 1 {
		t.Fatal("ordinary sequence wrap")
	}
	mustFail(k, at, v9WirePacket(0xffffffff, 744, 101, def), ErrV9LatePacket)
	p, err = s.Decode(k, at, v9WirePacket(3, 800, 101, data, def, data))
	if err != nil || p.MissingPackets != 2 || p.Sets[0].Status != V9TemplateMissing || p.Sets[1].Count != 1 {
		t.Fatal("gap reused stale definition")
	}
	collisionAt := at.Add(time.Second)
	mustFail(k, collisionAt, v9WirePacket(3, 800, 101, v9TemplateTestSet(0, v9TemplateTestWords(257, 2, 8, 2, 8, 2))), ErrV9SequenceCollision)
	mustFail(k, at, v9WirePacket(4, 801, 101, def), ErrV9LatePacket)
	p, err = s.Decode(k, collisionAt, v9WirePacket(4, 801, 101, data))
	if err != nil || p.Sets[0].Status != V9TemplateMissing {
		t.Fatal("collision permitted stale template")
	}
	mustFail(k, collisionAt, v9WirePacket(5, 100, 102, data), ErrV9EpochRequired)
	mustFail(k, collisionAt, v9WirePacket(6, 905, 102, def), ErrV9EpochRequired)
	k.Epoch++
	fresh, err := NewV9Stream(k, s.limits)
	if err != nil {
		t.Fatal(err)
	}
	p, err = fresh.Decode(k, at, v9WirePacket(1, 100, 102, data))
	if err != nil || p.Sets[0].Status != V9TemplateMissing {
		t.Fatal("epoch reused template")
	}
}

func TestV9StreamConfigAndContradictoryUptime(t *testing.T) {
	s, k, at := testV9Stream(t, 1, 1)
	if _, e := NewV9Stream(k, V9StreamLimits{}); !errors.Is(e, ErrV9StreamConfig) {
		t.Fatal(e)
	}
	for _, addr := range []string{"0.0.0.0:2055", "[::ffff:0.0.0.0]:2055", "[::]:2055", "[::%eth0]:2055", "192.0.2.4:0"} {
		bad := k
		bad.Collector = netip.MustParseAddrPort(addr)
		if _, e := NewV9Stream(bad, s.limits); !errors.Is(e, ErrV9StreamConfig) {
			t.Fatal(addr)
		}
	}
	kBlank := k
	kBlank.ExporterBinding = " \t"
	if _, e := NewV9Stream(kBlank, s.limits); !errors.Is(e, ErrV9StreamConfig) {
		t.Fatal("blank binding accepted")
	}
	def := v9TemplateTestSet(0, v9TemplateTestWords(256, 1, 8, 4))
	if _, e := s.Decode(k, at, v9WirePacket(7, 0xffff0000, 100, def)); e != nil {
		t.Fatal(e)
	}
	if p, e := s.Decode(k, at, v9WirePacket(8, 8, 101, def)); p != nil || !errors.Is(e, ErrV9EpochRequired) {
		t.Fatalf("implausible wrap %+v %v", p, e)
	}
}

func FuzzV9StreamDecode(f *testing.F) {
	f.Add(v9WirePacket(2, 0, 0, v9TemplateTestSet(256, []byte{5, 6, 7, 8})))
	f.Add(v9MixedPacket())
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, wire []byte) {
		s, k, at := testV9Stream(t, 32, 128)
		def := v9TemplateTestSet(0, v9TemplateTestWords(256, 1, 8, 4))
		if _, err := s.Decode(k, at, v9WirePacket(1, 0, 0, def)); err != nil {
			t.Fatal(err)
		}
		p, err := s.Decode(k, at, wire)
		if err != nil {
			if p != nil {
				t.Fatal("partial result")
			}
			return
		}
		total := 0
		for _, set := range p.Sets {
			total += len(set.Raw)
			if set.Status == V9Decoded {
				if set.Count <= 0 || set.Layout == nil || set.Count != len(set.Raw)/set.Layout.Width() {
					t.Fatal("invalid decoded dimensions")
				}
			} else if set.Count != 0 || set.Layout != nil || len(set.Raw) != 0 {
				t.Fatal("unqualified data exposed")
			}
		}
		if total > len(wire) {
			t.Fatal("decoded bytes exceed datagram")
		}
	})
}

func TestV9CapacityRefusalRevokesAllAffectedIDs(t *testing.T) {
	s, k, at := testV9Stream(t, 3, 3)
	def := func(id uint16) []byte { return v9TemplateTestSet(0, v9TemplateTestWords(id, 1, 8, 4)) }
	data := func(id uint16) []byte { return v9TemplateTestSet(id, []byte{1, 2, 3, 4}) }
	initial := v9WirePacket(1, 0, 0, def(256), def(257), def(258))
	if _, err := s.Decode(k, at, initial); err != nil {
		t.Fatal(err)
	}
	// The first replacement exceeds MaxFields. The later replacement also
	// revokes its prior ID even though neither replacement is admitted.
	wide := v9TemplateTestSet(0, v9TemplateTestWords(256, 2, 7, 2, 11, 2))
	bad := v9WirePacket(2, 0, 0, wide, def(257), v9TemplateTestSet(0, v9TemplateTestWords(256, 1)))
	if p, err := s.Decode(k, at, bad); p != nil || !errors.Is(err, ErrMalformedV9Template) {
		t.Fatalf("should be malformed: %+v, %v", p, err)
	}
	malformedData := v9WirePacket(2, 0, 0, wide, v9TemplateTestSet(256, []byte{1}))
	if p, err := s.Decode(k, at, malformedData); p != nil || !errors.Is(err, ErrMalformedV9Record) {
		t.Fatalf("invalid row: %+v, %v", p, err)
	}
	p, err := s.Decode(k, at, v9WirePacket(2, 0, 0, data(256), data(257), data(258)))
	if err != nil {
		t.Fatal(err)
	}
	for _, set := range p.Sets {
		if set.Count != 1 {
			t.Fatal("malformed packet revoked valid state")
		}
	}
	refused := v9WirePacket(3, 0, 0, wide, def(257))
	for range 2 {
		if p, e := s.Decode(k, at, refused); p != nil || !errors.Is(e, ErrV9StreamCapacity) {
			t.Fatalf("unexpected refusal: %+v %v", p, e)
		}
	}
	p, err = s.Decode(k, at, v9WirePacket(4, 0, 0, data(256), data(257), data(258)))
	if err != nil {
		t.Fatal(err)
	}
	if p.Sets[0].Status != V9TemplateMissing || p.Sets[1].Status != V9TemplateMissing || p.Sets[2].Count != 1 {
		t.Fatal("refused definitions unsafe or unaffected ID lost")
	}
	if _, err = s.Decode(k, at, v9WirePacket(5, 0, 0, def(256), def(257))); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Decode(k, at, v9WirePacket(6, 0, 0, wide)); !errors.Is(err, ErrV9StreamCapacity) {
		t.Fatal(err)
	}
	if p, e := s.Decode(k, at, v9WirePacket(6, 0, 0, data(256))); p != nil || !errors.Is(e, ErrV9SequenceCollision) {
		t.Fatalf("altered retry: %+v %v", p, e)
	}
	p, err = s.Decode(k, at, v9WirePacket(7, 0, 0, data(258), def(256), data(256)))
	if err != nil || p.Sets[0].Status != V9TemplateMissing || p.Sets[1].Count != 1 {
		t.Fatal("collision recovery")
	}
	// The dropped gap packet must also invalidate IDs unrelated to its own.
	invalidAtGap := v9TemplateTestSet(0, v9TemplateTestWords(300, 4, 8, 1, 8, 1, 8, 1, 8, 1))
	if p, e := s.Decode(k, at, v9WirePacket(9, 0, 0, invalidAtGap)); p != nil || !errors.Is(e, ErrV9StreamCapacity) {
		t.Fatalf("gap refusal: %+v %v", p, e)
	}
	p, err = s.Decode(k, at, v9WirePacket(10, 0, 0, data(256)))
	if err != nil || p.Sets[0].Status != V9TemplateMissing {
		t.Fatal("refused gap retained old authority")
	}
}
