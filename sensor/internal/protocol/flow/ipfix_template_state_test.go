package flow

import (
	"encoding/binary"
	"errors"
	"math"
	"net/netip"
	"reflect"
	"sync"
	"testing"
	"time"
)

func ipfixStateKey() IPFIXTemplateStreamKey {
	return IPFIXTemplateStreamKey{
		ExporterBinding:     "exporter-a",
		Source:              netip.MustParseAddrPort("192.0.2.1:30473"),
		Collector:           netip.MustParseAddrPort("[2001:db8::1]:4739"),
		ObservationDomainID: 7,
		Epoch:               1,
	}
}

func ipfixStateDefinition(id uint16, widths ...uint16) IPFIXRawTemplate {
	definition := IPFIXRawTemplate{ID: id}
	for i, width := range widths {
		definition.Fields = append(definition.Fields, IPFIXRawField{ElementID: uint16(i + 1), Length: width})
	}
	return definition
}

func ipfixStatePacket(exportTime uint32, definitions ...IPFIXRawTemplate) *IPFIXRawTemplatePacket {
	return &IPFIXRawTemplatePacket{
		ObservationDomainID: ipfixStateKey().ObservationDomainID,
		ExportTime:          exportTime,
		Templates:           definitions,
	}
}

func requireIPFIXStateError(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("error = %v, want %v", got, want)
	}
}

func TestIPFIXTemplateStateAdmission(t *testing.T) {
	key := ipfixStateKey()
	now := time.Unix(1000, 0)
	cache, err := NewIPFIXTemplateCache(key, IPFIXTemplateCacheLimits{time.Minute, 2, 3})
	requireIPFIXStateError(t, err, nil)
	a := ipfixStateDefinition(256, 4)
	b := ipfixStateDefinition(257, ipfixVariableLength, ipfixMaxRawDataSetBody-1)
	apply := func(offset time.Duration, export uint32, want error, definitions ...IPFIXRawTemplate) {
		t.Helper()
		requireIPFIXStateError(t, cache.ApplyOrdered(key, now.Add(offset), ipfixStatePacket(export, definitions...)), want)
	}
	lookup := func(offset time.Duration, export uint32, id uint16, want error) IPFIXTemplateSnapshot {
		t.Helper()
		snapshot, err := cache.LookupOrdered(key, now.Add(offset), export, id)
		requireIPFIXStateError(t, err, want)
		if want != nil && !reflect.DeepEqual(snapshot, IPFIXTemplateSnapshot{}) {
			t.Fatalf("refused lookup returned %+v", snapshot)
		}
		return snapshot
	}

	apply(0, 10, nil, a, a, b)
	accepted := lookup(0, 10, 256, nil)
	if accepted.Generation == 0 || lookup(0, 10, 257, nil).Generation <= accepted.Generation {
		t.Fatal("generations must increase within a batch")
	}
	a.Fields[0].Length = 55
	detached := lookup(0, 10, 256, nil)
	detached.Template.Fields[0].Length = 66
	if got := lookup(0, 10, 256, nil); !reflect.DeepEqual(got, accepted) || got.Template.Fields[0].Length != 4 {
		t.Fatalf("snapshot aliases supplied or returned descriptors: %+v", got)
	}

	rawInvalid := ipfixRawTestDefinition(256, false, 0, ipfixRawTestField(1, 0, 0, false))
	rawFresh := ipfixRawTestDefinition(258, false, 0, ipfixRawTestField(1, 1, 0, false))
	wire := ipfixRawTestWire(key.ObservationDomainID, ipfixRawTestSet(2, append(rawInvalid, rawFresh...)))
	binary.BigEndian.PutUint32(wire[4:8], 11)
	parsed, err := ParseIPFIXRawTemplates(wire)
	requireIPFIXStateError(t, err, nil)
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now.Add(time.Second), parsed), ErrIPFIXTemplateRedefinitionInvalid)
	if got := cache.entries[256].IPFIXTemplateSnapshot; !reflect.DeepEqual(got, accepted) {
		t.Fatalf("refusal changed last accepted layout, generation or timestamps: %+v", got)
	}
	lookup(time.Second, 10, 256, ErrIPFIXTemplateOrder)
	lookup(2*time.Second, 11, 256, ErrIPFIXTemplateUncertain)
	lookup(2*time.Second, 11, 257, nil)
	lookup(2*time.Second, 11, 258, ErrIPFIXTemplateMissing)
	apply(2*time.Second, 10, ErrIPFIXTemplateOrder, accepted.Template)
	apply(2*time.Second, 11, ErrIPFIXTemplateOrder, accepted.Template)
	apply(3*time.Second, 12, nil, accepted.Template)
	recovered := lookup(3*time.Second, 12, 256, nil)
	if recovered.Generation <= accepted.Generation || !recovered.RefreshedAt.Equal(now.Add(3*time.Second)) {
		t.Fatalf("recovery did not establish a new version: %+v", recovered)
	}

	// Growing one definition before shrinking another must fit the final footprint.
	options := ipfixStateDefinition(256, 4, 8)
	options.Options, options.ScopeCount = true, 1
	short := ipfixStateDefinition(257, 1)
	apply(4*time.Second, 13, nil, options, short)
	current := lookup(4*time.Second, 13, 256, nil)
	if !current.Template.Options || current.Generation <= recovered.Generation {
		t.Fatalf("Options replacement did not share the Data Template namespace: %+v", current)
	}
	apply(5*time.Second, 14, ErrIPFIXTemplateCapacity, options, ipfixStateDefinition(258, 1))
	lookup(5*time.Second, 14, 256, ErrIPFIXTemplateUncertain)
	lookup(5*time.Second, 14, 257, nil)
	lookup(5*time.Second, 14, 258, ErrIPFIXTemplateMissing)
	if got := cache.entries[256].IPFIXTemplateSnapshot; !reflect.DeepEqual(got, current) {
		t.Fatalf("capacity refusal changed the accepted snapshot: %+v", got)
	}
	apply(6*time.Second, 15, ErrIPFIXTemplateCapacity, ipfixStateDefinition(257, 1, 2))
	lookup(6*time.Second, 15, 257, ErrIPFIXTemplateUncertain)
	apply(7*time.Second, 16, ErrIPFIXTemplateRedefinitionInvalid, options, ipfixStateDefinition(256, 4, 8))
	apply(8*time.Second, 17, ErrIPFIXTemplateRedefinitionInvalid, ipfixStateDefinition(259, ipfixMaxRawDataSetBody, 1))
	lookup(8*time.Second, 17, 259, ErrIPFIXTemplateMissing)
}

func TestIPFIXTemplateStateOrderAndExpiry(t *testing.T) {
	key := ipfixStateKey()
	now := time.Unix(2000, 0)
	timeout := 10 * time.Second
	cache, err := NewIPFIXTemplateCache(key, IPFIXTemplateCacheLimits{timeout, 2, 2})
	requireIPFIXStateError(t, err, nil)
	a, b := ipfixStateDefinition(256, 1), ipfixStateDefinition(257, 2)
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now, ipfixStatePacket(1, a)), nil)
	old, err := cache.LookupOrdered(key, now, 1, a.ID)
	requireIPFIXStateError(t, err, nil)
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now.Add(time.Second), ipfixStatePacket(1, a, b)), nil)
	same, err := cache.LookupOrdered(key, now.Add(time.Second), 1, a.ID)
	requireIPFIXStateError(t, err, nil)
	if !reflect.DeepEqual(old, same) {
		t.Fatalf("equal-time duplicate refreshed template: %+v", same)
	}
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now, ipfixStatePacket(2, a)), ErrIPFIXTemplateOrder)
	_, err = cache.LookupOrdered(key, now, 1, a.ID)
	requireIPFIXStateError(t, err, ErrIPFIXTemplateOrder)
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now.Add(2*time.Second), ipfixStatePacket(2, a)), nil)
	refresh, err := cache.LookupOrdered(key, now.Add(2*time.Second), 2, a.ID)
	requireIPFIXStateError(t, err, nil)
	if refresh.Generation != old.Generation || refresh.ExportTime != 2 || !refresh.RefreshedAt.Equal(now.Add(2*time.Second)) {
		t.Fatalf("later identical refresh changed generation or kept old times: %+v", refresh)
	}
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now.Add(3*time.Second), ipfixStatePacket(2, ipfixStateDefinition(a.ID, 9))), ErrIPFIXTemplateOrder)
	_, err = cache.LookupOrdered(key, now.Add(3*time.Second), 2, a.ID)
	requireIPFIXStateError(t, err, ErrIPFIXTemplateUncertain)
	_, err = cache.LookupOrdered(key, now.Add(timeout+time.Second), 2, b.ID)
	requireIPFIXStateError(t, err, ErrIPFIXTemplateExpired)
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now.Add(timeout+time.Second), ipfixStatePacket(3, b)), nil)
	reborn, err := cache.LookupOrdered(key, now.Add(timeout+time.Second), 3, b.ID)
	requireIPFIXStateError(t, err, nil)
	if reborn.Generation <= refresh.Generation+1 {
		t.Fatalf("expired definition reused its generation: %+v", reborn)
	}
	atExpiry := now.Add(2*timeout + time.Second)
	requireIPFIXStateError(t, cache.ApplyOrdered(key, atExpiry, ipfixStatePacket(3, b)), nil)
	_, err = cache.LookupOrdered(key, atExpiry, 3, b.ID)
	requireIPFIXStateError(t, err, ErrIPFIXTemplateExpired)
}

func TestIPFIXTemplateStateWithdrawals(t *testing.T) {
	key, now := ipfixStateKey(), time.Unix(3000, 0)
	cache, err := NewIPFIXTemplateCache(key, IPFIXTemplateCacheLimits{10 * time.Second, 2, 2})
	requireIPFIXStateError(t, err, nil)
	a, b := ipfixStateDefinition(256, 1), ipfixStateDefinition(257, 1)
	b.Options, b.ScopeCount = true, 1
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now, ipfixStatePacket(1, a, b)), nil)
	withdrawals := []IPFIXRawTemplate{
		{ID: 256, Withdrawal: true},
		{ID: 257, Withdrawal: true, Options: true},
		{ID: 2, Withdrawal: true, AllTemplates: true},
		{ID: 3, Withdrawal: true, AllTemplates: true, Options: true},
	}
	later := now.Add(time.Second)
	requireIPFIXStateError(t, cache.ApplyOrdered(key, later, ipfixStatePacket(2, withdrawals...)), nil)
	malformed := []IPFIXRawTemplate{
		{ID: 256, Withdrawal: true, Fields: a.Fields},
		{ID: 257, Withdrawal: true, Options: true, ScopeCount: 1},
		{ID: 2, Withdrawal: true, AllTemplates: true, Options: true},
		{ID: 3, Withdrawal: true, AllTemplates: true},
		{ID: 2, Withdrawal: true},
		{ID: 256, Withdrawal: true, AllTemplates: true},
	}
	for _, record := range malformed {
		requireIPFIXStateError(t, cache.ApplyOrdered(key, later, ipfixStatePacket(3, record)), ErrIPFIXTemplateRedefinitionInvalid)
	}
	for _, definition := range []IPFIXRawTemplate{a, b} {
		snapshot, err := cache.LookupOrdered(key, later, 3, definition.ID)
		requireIPFIXStateError(t, err, nil)
		if !snapshot.RefreshedAt.Equal(now) || snapshot.ExportTime != 1 || !reflect.DeepEqual(snapshot.Template, definition) {
			t.Fatalf("withdrawal modified accepted snapshot: %+v", snapshot)
		}
	}
}

func TestIPFIXTemplateStateScopeAndBounds(t *testing.T) {
	key, now := ipfixStateKey(), time.Unix(4000, 0)
	limits := IPFIXTemplateCacheLimits{time.Second, 1, 1}
	for _, edit := range []func(*IPFIXTemplateStreamKey){
		func(k *IPFIXTemplateStreamKey) { k.ExporterBinding = " \t" },
		func(k *IPFIXTemplateStreamKey) { k.Epoch = 0 },
		func(k *IPFIXTemplateStreamKey) { k.Source = netip.MustParseAddrPort("192.0.2.1:0") },
		func(k *IPFIXTemplateStreamKey) { k.Collector = netip.MustParseAddrPort("[::ffff:0.0.0.0]:4739") },
		func(k *IPFIXTemplateStreamKey) { k.Collector = netip.AddrPort{} },
	} {
		invalid := key
		edit(&invalid)
		_, err := NewIPFIXTemplateCache(invalid, limits)
		requireIPFIXStateError(t, err, ErrIPFIXTemplateConfig)
	}
	for _, invalid := range []IPFIXTemplateCacheLimits{{0, 1, 1}, {time.Second, 0, 1}, {time.Second, 1, -1}} {
		_, err := NewIPFIXTemplateCache(key, invalid)
		requireIPFIXStateError(t, err, ErrIPFIXTemplateConfig)
	}

	cache, err := NewIPFIXTemplateCache(key, limits)
	requireIPFIXStateError(t, err, nil)
	a := ipfixStateDefinition(256, 1)
	packet := ipfixStatePacket(0, a)
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now, nil), ErrIPFIXTemplateConfig)
	requireIPFIXStateError(t, cache.ApplyOrdered(key, time.Time{}, packet), ErrIPFIXTemplateConfig)
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now, packet), nil)
	newEpoch := key
	newEpoch.Epoch++
	for _, edit := range []func(*IPFIXTemplateStreamKey){
		func(k *IPFIXTemplateStreamKey) { k.Epoch++ },
		func(k *IPFIXTemplateStreamKey) { k.ExporterBinding += " " },
		func(k *IPFIXTemplateStreamKey) { k.ObservationDomainID++ },
		func(k *IPFIXTemplateStreamKey) { k.Source = netip.MustParseAddrPort("192.0.2.1:30474") },
		func(k *IPFIXTemplateStreamKey) { k.Collector = netip.MustParseAddrPort("[2001:db8::2]:4739") },
	} {
		other := key
		edit(&other)
		requireIPFIXStateError(t, cache.ApplyOrdered(other, now, packet), ErrIPFIXTemplateScope)
		_, err := cache.LookupOrdered(other, now, 0, a.ID)
		requireIPFIXStateError(t, err, ErrIPFIXTemplateScope)
	}
	badDomain := *packet
	badDomain.ObservationDomainID++
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now, &badDomain), ErrIPFIXTemplateScope)
	freshCache, err := NewIPFIXTemplateCache(newEpoch, limits)
	requireIPFIXStateError(t, err, nil)
	_, err = freshCache.LookupOrdered(newEpoch, now, 0, a.ID)
	requireIPFIXStateError(t, err, ErrIPFIXTemplateMissing)

	for id := uint16(257); id < 512; id++ {
		requireIPFIXStateError(t, cache.ApplyOrdered(key, now, ipfixStatePacket(uint32(id), ipfixStateDefinition(id, 1))), ErrIPFIXTemplateCapacity)
	}
	if len(cache.entries) != 1 {
		t.Fatalf("capacity refusals retained unknown IDs: %d entries", len(cache.entries))
	}
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now, ipfixStatePacket(512)), nil)
	_, err = cache.LookupOrdered(key, now.Add(time.Second), 512, a.ID)
	requireIPFIXStateError(t, err, ErrIPFIXTemplateExpired)
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now.Add(time.Second), ipfixStatePacket(513, ipfixStateDefinition(512, 1))), nil)
	if len(cache.entries) != 1 || cache.entries[512].Generation != 2 {
		t.Fatalf("expired definition did not release capacity: %+v", cache.entries)
	}

	cache.generation = math.MaxUint64
	requireIPFIXStateError(t, cache.ApplyOrdered(key, now.Add(time.Second), ipfixStatePacket(514, ipfixStateDefinition(512, 2))), ErrIPFIXTemplateCapacity)
	if cache.generation != math.MaxUint64 || cache.entries[512].Generation != 2 {
		t.Fatal("generation exhaustion changed accepted state")
	}
}

func TestIPFIXTemplateStateConcurrent(t *testing.T) {
	key, now := ipfixStateKey(), time.Unix(5000, 0)
	cache, err := NewIPFIXTemplateCache(key, IPFIXTemplateCacheLimits{time.Minute, 16, 16})
	requireIPFIXStateError(t, err, nil)
	var workers sync.WaitGroup
	for id := uint16(256); id < 272; id++ {
		workers.Add(1)
		go func(id uint16) {
			defer workers.Done()
			packet := ipfixStatePacket(1, ipfixStateDefinition(id, 1))
			for i := 0; i < 20; i++ {
				if err := cache.ApplyOrdered(key, now, packet); err != nil {
					t.Errorf("apply template %d: %v", id, err)
					return
				}
				snapshot, err := cache.LookupOrdered(key, now, 1, id)
				if err != nil || snapshot.Generation == 0 || snapshot.Template.Fields[0].Length != 1 {
					t.Errorf("lookup template %d: %+v, %v", id, snapshot, err)
					return
				}
				snapshot.Template.Fields[0].Length = 2
			}
		}(id)
	}
	workers.Wait()
	if len(cache.entries) != 16 || cache.generation != 16 {
		t.Fatalf("concurrent duplicates changed cache accounting: %d entries, generation %d", len(cache.entries), cache.generation)
	}
}
