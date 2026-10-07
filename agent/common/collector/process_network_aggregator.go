package collector

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"sync"
	"time"
)

// ProcessNetworkAggregatorConfig is an engineering safety config, not a SKU.
// SourceProvenance is aggregator-owned collection provenance, not tenant or
// entity authority, and is never taken from untrusted per-event fields.
type ProcessNetworkAggregatorConfig struct {
	SourceProvenance     string
	Window               time.Duration
	MaxGlobalKeys        int
	MaxKeysPerWorkload   int
	MaxInputBatch        int
	MaxRetainedWindows   int
	MaxClosedWindows     int
	MaxAccountingStubs   int
	TestAllowShortWindow bool
}

func (c ProcessNetworkAggregatorConfig) normalized() (ProcessNetworkAggregatorConfig, error) {
	if c.Window == 0 {
		c.Window = defaultProcessNetworkWindow
	}
	if c.TestAllowShortWindow {
		if c.Window < time.Second || c.Window > maxProcessNetworkWindow {
			return c, fmt.Errorf("PROCESS_NETWORK_WINDOW_INVALID: test window must be in [1s, %s]", maxProcessNetworkWindow)
		}
	} else if c.Window < minProcessNetworkWindow || c.Window > maxProcessNetworkWindow {
		return c, fmt.Errorf("PROCESS_NETWORK_WINDOW_INVALID: production window must be in [%s, %s]", minProcessNetworkWindow, maxProcessNetworkWindow)
	}
	if c.Window%time.Second != 0 {
		return c, errors.New("PROCESS_NETWORK_WINDOW_INVALID: window must be an exact whole-second duration")
	}
	if c.MaxGlobalKeys == 0 {
		c.MaxGlobalKeys = maxProcessNetworkGlobalKeys
	}
	if c.MaxKeysPerWorkload == 0 {
		c.MaxKeysPerWorkload = maxProcessNetworkKeysPerWorkload
	}
	if c.MaxInputBatch == 0 {
		c.MaxInputBatch = maxProcessNetworkInputBatch
	}
	if c.MaxRetainedWindows == 0 {
		c.MaxRetainedWindows = maxProcessNetworkRetainedWindows
	}
	if c.MaxClosedWindows == 0 {
		c.MaxClosedWindows = maxProcessNetworkClosedWindows
	}
	if c.MaxAccountingStubs == 0 {
		c.MaxAccountingStubs = maxProcessNetworkAccountingStubs
	}
	if c.MaxGlobalKeys < 1 || c.MaxKeysPerWorkload < 1 || c.MaxInputBatch < 1 ||
		c.MaxRetainedWindows < 1 || c.MaxClosedWindows < 1 || c.MaxAccountingStubs < 1 {
		return c, errors.New("PROCESS_NETWORK_BOUND_INVALID")
	}
	if c.MaxKeysPerWorkload > c.MaxGlobalKeys {
		c.MaxKeysPerWorkload = c.MaxGlobalKeys
	}
	if err := validateProcessNetworkProvenance(c.SourceProvenance); err != nil {
		return c, err
	}
	return c, nil
}

// ProcessNetworkFlushOutput is locally emitted evidence. It is not delivery.
type ProcessNetworkFlushOutput struct {
	Evidence   []ProcessNetworkEvidence
	Accounting []ProcessNetworkWindowAccounting
}

// ProcessNetworkAggregator is a mutex-protected, timer-free local aggregator.
type ProcessNetworkAggregator struct {
	mu     sync.Mutex
	cfg    ProcessNetworkAggregatorConfig
	sink   StructuredEvidenceSink
	closed bool

	windows          map[int64]*pnWindow
	closedStubs      map[int64]*pnClosedStub
	accountingStubs  map[int64]*pnClosedStub
	globalKeys       map[string]struct{}
	closedThrough    time.Time
	closedThroughSet bool
}

type pnWindow struct {
	start             time.Time
	end               time.Time
	workloads         map[string]*pnWorkload
	unattributedDrops uint32
	shed              uint32
	sampled           uint32
	late              uint32
	quality           ProcessNetworkQualityState
	partial           bool
	sampledF          bool
	guarded           bool
	saturated         bool
}

type pnClosedStub struct {
	start    time.Time
	end      time.Time
	dropped  uint32
	shed     uint32
	late     uint32
	sampled  uint32
	quality  ProcessNetworkQualityState
	partial  bool
	guarded  bool
	sampledF bool
}

type pnWorkload struct {
	identity   ProcessNetworkWorkloadIdentity
	candidates map[string]*pnCandidate
	overflow   map[ProcessNetworkProtocol]*pnOverflow
	quality    ProcessNetworkQualityState
	partial    bool
	sampledF   bool
	guarded    bool
	dropped    uint32
	shed       uint32
	sampled    uint32
	saturated  bool
}

type pnCandidate struct {
	remote    netip.Addr
	port      uint16
	proto     ProcessNetworkProtocol
	heat      uint64
	exact     pnMetrics
	pid       mixUint32
	localIP   mixAddr
	localPort mixUint16
	svc       mixString
	exeID     mixString
	exePath   mixString
	name      mixString
	cgroup    mixString
	quality   ProcessNetworkQualityState
	partial   bool
	sampledF  bool
	guarded   bool
	dropped   uint32
	shed      uint32
	sampled   uint32
	saturated bool
}

type pnMetrics struct {
	eventCount uint64
	bytesTx    uint64
	hasTx      bool
	bytesRx    uint64
	hasRx      bool
	active     uint32
	hasActive  bool
	newConn    uint32
	hasNew     bool
	closed     uint32
	hasClosed  bool
	duration   uint64
	hasDur     bool
	rttSum     uint64
	rttN       uint32
	rtx        uint32
	hasRtx     bool
	fail       uint32
	hasFail    bool
	saturated  bool
}

type pnOverflow struct {
	proto   ProcessNetworkProtocol
	metrics pnMetrics
	evicted uint32
	shed    uint32
	sampled uint32
}

type mixString struct {
	set   bool
	mixed bool
	value string
}

type mixUint32 struct {
	set   bool
	mixed bool
	value uint32
}

type mixUint16 struct {
	set   bool
	mixed bool
	value uint16
}

type mixAddr struct {
	set   bool
	mixed bool
	value netip.Addr
}

func (m *mixString) observe(v string) {
	if m.mixed {
		return
	}
	if !m.set {
		m.set = true
		m.value = v
		return
	}
	if m.value != v {
		m.mixed = true
		m.value = ""
	}
}

func (m *mixUint32) observePtr(p *uint32) {
	if p == nil {
		if m.set {
			m.mixed = true
			m.set = false
		}
		return
	}
	if m.mixed {
		return
	}
	if !m.set {
		m.set = true
		m.value = *p
		return
	}
	if m.value != *p {
		m.mixed = true
		m.set = false
	}
}

func (m *mixUint16) observePtr(p *uint16) {
	if p == nil {
		if m.set {
			m.mixed = true
			m.set = false
		}
		return
	}
	if m.mixed {
		return
	}
	if !m.set {
		m.set = true
		m.value = *p
		return
	}
	if m.value != *p {
		m.mixed = true
		m.set = false
	}
}

func (m *mixAddr) observe(raw string) {
	if raw == "" {
		if m.set {
			m.mixed = true
			m.set = false
		}
		return
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		m.mixed = true
		m.set = false
		return
	}
	addr = addr.Unmap()
	if m.mixed {
		return
	}
	if !m.set {
		m.set = true
		m.value = addr
		return
	}
	if m.value != addr {
		m.mixed = true
		m.set = false
	}
}

func NewProcessNetworkAggregator(cfg ProcessNetworkAggregatorConfig, sink StructuredEvidenceSink) (*ProcessNetworkAggregator, error) {
	n, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	if sink == nil {
		sink = UnwiredStructuredEvidenceSink{}
	}
	return &ProcessNetworkAggregator{
		cfg:             n,
		sink:            sink,
		windows:         make(map[int64]*pnWindow),
		closedStubs:     make(map[int64]*pnClosedStub),
		accountingStubs: make(map[int64]*pnClosedStub),
		globalKeys:      make(map[string]struct{}),
	}, nil
}

func (a *ProcessNetworkAggregator) Ingest(ctx context.Context, events []ProcessNetworkSyntheticEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return errors.New("PROCESS_NETWORK_AGGREGATOR_CLOSED")
	}
	if len(events) > a.cfg.MaxInputBatch {
		return fmt.Errorf("PROCESS_NETWORK_INPUT_BATCH_BOUND_EXCEEDED: implementation safety bound is %d", a.cfg.MaxInputBatch)
	}
	for i := range events {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := a.ingestOne(events[i]); err != nil {
			return err
		}
	}
	return nil
}

func (a *ProcessNetworkAggregator) ingestOne(e ProcessNetworkSyntheticEvent) error {
	if e.Class == ProcessNetworkEventBackendDrop {
		return a.ingestBackendDrop(e)
	}
	if e.ObservedAt.IsZero() {
		return ErrProcessNetworkObservedAtInvalid
	}
	id, err := durableWorkloadFromEvent(e)
	if err != nil {
		return a.noteDrop(e)
	}
	if err := optionalProcessToken("container_id", e.ContainerID, maxProcessNetworkContainerIDLen); err != nil {
		return a.noteDrop(e)
	}
	switch e.Class {
	case ProcessNetworkEventOpen, ProcessNetworkEventClose, ProcessNetworkEventFail,
		ProcessNetworkEventBytes, ProcessNetworkEventRTT, ProcessNetworkEventRetransmit:
	default:
		return a.noteDrop(e)
	}
	switch e.Protocol {
	case ProcessNetworkProtocolTCP, ProcessNetworkProtocolUDP:
	default:
		return a.noteDrop(e)
	}
	if err := validateSyntheticLocalEndpoint(e.LocalIP, e.LocalPort); err != nil {
		return a.noteDrop(e)
	}
	addr, err := canonicalizeRemoteAddr(e.RemoteIP)
	if err != nil {
		return a.noteDrop(e)
	}
	if e.RemotePort < 1 {
		return a.noteDrop(e)
	}

	start := alignUTC(e.ObservedAt, a.cfg.Window)
	w, stub, lateOrphan := a.windowFor(start)
	if stub != nil {
		if lateOrphan || a.closedStubs[start.UnixNano()] != nil {
			return a.creditStub(stub, acctLate, 1, e.ResourceGuarded, e.Sampled)
		}
		return a.creditStub(stub, acctShed, 1, true, e.Sampled)
	}
	if lateOrphan {
		return a.accountExact(start, acctLate, 1, e.ResourceGuarded, e.Sampled)
	}
	if w == nil {
		return a.accountExact(start, acctShed, 1, true, e.Sampled)
	}
	if e.Sampled {
		if satIncU32(&w.sampled) {
			markWindowSaturated(w)
		}
		w.sampledF = true
		w.quality = mergeQuality(w.quality, ProcessNetworkQualitySampled)
	}
	if e.ResourceGuarded {
		w.guarded = true
		w.quality = mergeQuality(w.quality, ProcessNetworkQualityResourceGuarded)
	}

	wk := id.aggregationKey()
	wl, ok := w.workloads[wk]
	if !ok {
		if len(w.workloads) >= a.cfg.MaxGlobalKeys {
			if satIncU32(&w.shed) {
				markWindowSaturated(w)
			}
			markResourcePartial(w)
			return nil
		}
		wl = &pnWorkload{
			identity:   id,
			candidates: make(map[string]*pnCandidate),
			overflow:   make(map[ProcessNetworkProtocol]*pnOverflow),
			quality:    ProcessNetworkQualityValid,
		}
		w.workloads[wk] = wl
	}
	if e.Sampled {
		if satIncU32(&wl.sampled) {
			markResourcePartial(wl)
			markWindowSaturated(w)
		}
		wl.sampledF = true
		wl.quality = mergeQuality(wl.quality, ProcessNetworkQualitySampled)
	}
	if e.ResourceGuarded {
		wl.guarded = true
		wl.quality = mergeQuality(wl.quality, ProcessNetworkQualityResourceGuarded)
	}

	dkey := destinationKey(addr, e.RemotePort, e.Protocol)
	if cand, exists := wl.candidates[dkey]; exists {
		satIncHeat(cand)
		applyExact(cand, e, addr)
		propagateSaturated(w, wl, cand)
		return nil
	}

	gkey := globalKey(start, wk, dkey)
	if _, exists := a.globalKeys[gkey]; exists {
		return nil
	}

	if len(wl.candidates) < a.cfg.MaxKeysPerWorkload && len(a.globalKeys) < a.cfg.MaxGlobalKeys {
		cand := newCandidate(addr, e.RemotePort, e.Protocol)
		cand.heat = 1
		applyExact(cand, e, addr)
		wl.candidates[dkey] = cand
		a.globalKeys[gkey] = struct{}{}
		propagateSaturated(w, wl, cand)
		return nil
	}

	if len(wl.candidates) >= a.cfg.MaxKeysPerWorkload {
		a.replaceLowestHeat(w, wl, start, wk, addr, e)
		return nil
	}

	ov := wl.ensureOverflow(e.Protocol)
	if applyMetrics(&ov.metrics, e) {
		markResourcePartial(w)
		markResourcePartial(wl)
	}
	if satIncU32(&ov.shed) {
		markResourcePartial(wl)
		markWindowSaturated(w)
	}
	wl.partial = true
	wl.quality = mergeQuality(wl.quality, ProcessNetworkQualityPartial)
	w.partial = true
	w.quality = mergeQuality(w.quality, ProcessNetworkQualityPartial)
	if e.Sampled {
		if satIncU32(&ov.sampled) {
			markResourcePartial(wl)
			markWindowSaturated(w)
		}
	}
	return nil
}

func (a *ProcessNetworkAggregator) replaceLowestHeat(w *pnWindow, wl *pnWorkload, start time.Time, wk string, addr netip.Addr, e ProcessNetworkSyntheticEvent) {
	victimKey, victim := selectEvictionCandidate(wl.candidates)
	if victim == nil {
		return
	}
	ov := wl.ensureOverflow(victim.proto)
	if foldMetrics(&ov.metrics, victim.exact) {
		markResourcePartial(w)
		markResourcePartial(wl)
	}
	if satIncU32(&ov.evicted) {
		markResourcePartial(wl)
		markResourcePartial(w)
	}
	add, sat := satAddUint32(ov.sampled, victim.sampled)
	ov.sampled = add
	if sat {
		markResourcePartial(wl)
		markResourcePartial(w)
	}
	delete(wl.candidates, victimKey)
	delete(a.globalKeys, globalKey(start, wk, victimKey))

	dkey := destinationKey(addr, e.RemotePort, e.Protocol)
	gkey := globalKey(start, wk, dkey)
	if len(a.globalKeys) >= a.cfg.MaxGlobalKeys {
		if applyMetrics(&ov.metrics, e) {
			markResourcePartial(w)
			markResourcePartial(wl)
		}
		if satIncU32(&ov.shed) {
			markResourcePartial(wl)
			markResourcePartial(w)
		}
		wl.partial = true
		w.partial = true
		wl.quality = mergeQuality(wl.quality, ProcessNetworkQualityPartial)
		w.quality = mergeQuality(w.quality, ProcessNetworkQualityPartial)
		return
	}

	cand := newCandidate(addr, e.RemotePort, e.Protocol)
	heat, sat := satAddUint64(victim.heat, 1)
	cand.heat = heat
	if sat {
		cand.saturated = true
	}
	applyExact(cand, e, addr)
	cand.partial = true
	wl.candidates[dkey] = cand
	a.globalKeys[gkey] = struct{}{}
	wl.partial = true
	wl.quality = mergeQuality(wl.quality, ProcessNetworkQualityPartial)
	w.partial = true
	w.quality = mergeQuality(w.quality, ProcessNetworkQualityPartial)
	propagateSaturated(w, wl, cand)
}

func selectEvictionCandidate(cands map[string]*pnCandidate) (string, *pnCandidate) {
	var bestKey string
	var best *pnCandidate
	for k, c := range cands {
		if best == nil || c.heat < best.heat || (c.heat == best.heat && k < bestKey) {
			bestKey = k
			best = c
		}
	}
	return bestKey, best
}

func newCandidate(addr netip.Addr, port uint16, proto ProcessNetworkProtocol) *pnCandidate {
	return &pnCandidate{
		remote:  addr,
		port:    port,
		proto:   proto,
		quality: ProcessNetworkQualityValid,
	}
}

func applyExact(c *pnCandidate, e ProcessNetworkSyntheticEvent, addr netip.Addr) {
	_ = addr
	if applyMetrics(&c.exact, e) {
		markCandidateSaturated(c)
	}
	c.pid.observePtr(e.PID)
	c.localIP.observe(e.LocalIP)
	c.localPort.observePtr(e.LocalPort)
	c.svc.observe(e.ServiceID)
	c.exeID.observe(e.ExecutableIdentity)
	c.exePath.observe(e.ExecutablePath)
	c.name.observe(e.ProcessName)
	c.cgroup.observe(e.CgroupPath)
	if e.Sampled {
		if satIncU32(&c.sampled) {
			markCandidateSaturated(c)
		}
		c.sampledF = true
		c.quality = mergeQuality(c.quality, ProcessNetworkQualitySampled)
	}
	if e.ResourceGuarded {
		c.guarded = true
		c.quality = mergeQuality(c.quality, ProcessNetworkQualityResourceGuarded)
	}
}

func applyMetrics(m *pnMetrics, e ProcessNetworkSyntheticEvent) bool {
	sat := false
	if satIncU64(&m.eventCount) {
		sat = true
	}
	if e.BytesTx != nil {
		m.hasTx = true
		if addSatU64(&m.bytesTx, *e.BytesTx) {
			sat = true
		}
	}
	if e.BytesRx != nil {
		m.hasRx = true
		if addSatU64(&m.bytesRx, *e.BytesRx) {
			sat = true
		}
	}
	if e.Protocol != ProcessNetworkProtocolTCP {
		if sat {
			m.saturated = true
		}
		return sat
	}
	switch e.Class {
	case ProcessNetworkEventOpen:
		m.hasNew = true
		if satIncU32(&m.newConn) {
			sat = true
		}
		m.hasActive = true
		if satIncU32(&m.active) {
			sat = true
		}
	case ProcessNetworkEventClose:
		m.hasClosed = true
		if satIncU32(&m.closed) {
			sat = true
		}
		if m.hasActive && m.active > 0 {
			m.active--
		}
		if e.DurationMs != nil {
			m.hasDur = true
			if addSatU64(&m.duration, *e.DurationMs) {
				sat = true
			}
		}
	case ProcessNetworkEventFail:
		m.hasFail = true
		if satIncU32(&m.fail) {
			sat = true
		}
	case ProcessNetworkEventRTT:
		if e.RTTUs != nil {
			if addSatU64(&m.rttSum, uint64(*e.RTTUs)) {
				sat = true
			}
			if satIncU32(&m.rttN) {
				sat = true
			}
		}
	case ProcessNetworkEventRetransmit:
		m.hasRtx = true
		n := uint32(1)
		if e.RetransmitCount != nil {
			n = *e.RetransmitCount
		}
		if addSatU32(&m.rtx, n) {
			sat = true
		}
	}
	if sat {
		m.saturated = true
	}
	return sat
}

func foldMetrics(dst *pnMetrics, src pnMetrics) bool {
	sat := src.saturated || dst.saturated
	if addSatU64(&dst.eventCount, src.eventCount) {
		sat = true
	}
	if src.hasTx {
		dst.hasTx = true
		if addSatU64(&dst.bytesTx, src.bytesTx) {
			sat = true
		}
	}
	if src.hasRx {
		dst.hasRx = true
		if addSatU64(&dst.bytesRx, src.bytesRx) {
			sat = true
		}
	}
	if src.hasNew {
		dst.hasNew = true
		if addSatU32(&dst.newConn, src.newConn) {
			sat = true
		}
	}
	if src.hasClosed {
		dst.hasClosed = true
		if addSatU32(&dst.closed, src.closed) {
			sat = true
		}
	}
	if src.hasFail {
		dst.hasFail = true
		if addSatU32(&dst.fail, src.fail) {
			sat = true
		}
	}
	if src.hasDur {
		dst.hasDur = true
		if addSatU64(&dst.duration, src.duration) {
			sat = true
		}
	}
	if src.hasRtx {
		dst.hasRtx = true
		if addSatU32(&dst.rtx, src.rtx) {
			sat = true
		}
	}
	if addSatU64(&dst.rttSum, src.rttSum) {
		sat = true
	}
	if addSatU32(&dst.rttN, src.rttN) {
		sat = true
	}
	if src.hasActive {
		dst.hasActive = true
		if addSatU32(&dst.active, src.active) {
			sat = true
		}
	}
	if sat {
		dst.saturated = true
	}
	return sat
}

func (wl *pnWorkload) ensureOverflow(p ProcessNetworkProtocol) *pnOverflow {
	ov, ok := wl.overflow[p]
	if !ok {
		ov = &pnOverflow{proto: p}
		wl.overflow[p] = ov
	}
	return ov
}

type acctKind int

const (
	acctDrop acctKind = iota
	acctShed
	acctLate
)

func (a *ProcessNetworkAggregator) noteDrop(e ProcessNetworkSyntheticEvent) error {
	if e.ObservedAt.IsZero() {
		return ErrProcessNetworkObservedAtInvalid
	}
	start := alignUTC(e.ObservedAt, a.cfg.Window)
	w, stub, lateOrphan := a.windowFor(start)
	if stub != nil {
		kind := acctDrop
		if lateOrphan || a.closedStubs[start.UnixNano()] != nil {
			kind = acctLate
		}
		return a.creditStub(stub, kind, 1, false, false)
	}
	if lateOrphan {
		return a.accountExact(start, acctLate, 1, false, false)
	}
	if w == nil {
		return a.accountExact(start, acctDrop, 1, false, false)
	}
	if satIncU32(&w.unattributedDrops) {
		markWindowSaturated(w)
	}
	w.partial = true
	w.quality = mergeQuality(w.quality, ProcessNetworkQualityPartial)
	return nil
}

func (a *ProcessNetworkAggregator) ingestBackendDrop(e ProcessNetworkSyntheticEvent) error {
	n := uint32(1)
	if e.DroppedCount != nil {
		n = *e.DroppedCount
	}
	if e.ObservedAt.IsZero() {
		return ErrProcessNetworkObservedAtInvalid
	}
	start := alignUTC(e.ObservedAt, a.cfg.Window)
	w, stub, lateOrphan := a.windowFor(start)
	if stub != nil {
		if lateOrphan || a.closedStubs[start.UnixNano()] != nil {
			return a.creditStub(stub, acctLate, n, e.ResourceGuarded, e.Sampled)
		}
		return a.creditStub(stub, acctDrop, n, e.ResourceGuarded, e.Sampled)
	}
	if lateOrphan {
		return a.accountExact(start, acctLate, n, e.ResourceGuarded, e.Sampled)
	}
	if w == nil {
		return a.accountExact(start, acctShed, n, true, e.Sampled)
	}
	if addSatU32(&w.unattributedDrops, n) {
		markWindowSaturated(w)
	}
	w.partial = true
	w.quality = mergeQuality(w.quality, ProcessNetworkQualityPartial)
	if e.ResourceGuarded {
		w.guarded = true
		w.quality = mergeQuality(w.quality, ProcessNetworkQualityResourceGuarded)
	}
	return nil
}

func (a *ProcessNetworkAggregator) windowFor(start time.Time) (*pnWindow, *pnClosedStub, bool) {
	key := start.UnixNano()
	if stub, ok := a.closedStubs[key]; ok {
		return nil, stub, false
	}
	if stub, ok := a.accountingStubs[key]; ok {
		if a.closedThroughSet && !start.After(a.closedThrough) {
			return nil, stub, true
		}
		return nil, stub, false
	}
	if w, ok := a.windows[key]; ok {
		return w, nil, false
	}
	if a.closedThroughSet && !start.After(a.closedThrough) {
		return nil, nil, true
	}
	if len(a.windows)+len(a.closedStubs) >= a.cfg.MaxRetainedWindows {
		return nil, nil, false
	}
	w := &pnWindow{
		start:     start,
		end:       start.Add(a.cfg.Window),
		workloads: make(map[string]*pnWorkload),
		quality:   ProcessNetworkQualityValid,
	}
	a.windows[key] = w
	return w, nil, false
}

func (a *ProcessNetworkAggregator) accountExact(start time.Time, kind acctKind, n uint32, guarded, sampled bool) error {
	key := start.UnixNano()
	if w, ok := a.windows[key]; ok {
		return a.creditWindow(w, kind, n, guarded, sampled)
	}
	if stub, ok := a.closedStubs[key]; ok {
		return a.creditStub(stub, kind, n, guarded, sampled)
	}
	if stub, ok := a.accountingStubs[key]; ok {
		return a.creditStub(stub, kind, n, guarded, sampled)
	}
	if len(a.accountingStubs) >= a.cfg.MaxAccountingStubs && !a.recycleEmptyAccountingStub() {
		return ErrProcessNetworkAccountingBoundExceeded
	}
	stub := &pnClosedStub{
		start:   start,
		end:     start.Add(a.cfg.Window),
		quality: ProcessNetworkQualityPartial,
		partial: true,
	}
	a.accountingStubs[key] = stub
	return a.creditStub(stub, kind, n, guarded, sampled)
}

func (a *ProcessNetworkAggregator) recycleEmptyAccountingStub() bool {
	var oldest int64
	found := false
	for k, s := range a.accountingStubs {
		if s.dropped == 0 && s.shed == 0 && s.late == 0 && s.sampled == 0 {
			if !found || k < oldest {
				oldest = k
				found = true
			}
		}
	}
	if !found {
		return false
	}
	delete(a.accountingStubs, oldest)
	return true
}

func (a *ProcessNetworkAggregator) creditWindow(w *pnWindow, kind acctKind, n uint32, guarded, sampled bool) error {
	switch kind {
	case acctShed:
		if addSatU32(&w.shed, n) {
			markWindowSaturated(w)
		}
		w.guarded = true
		w.quality = mergeQuality(w.quality, ProcessNetworkQualityResourceGuarded)
	case acctLate:
		if addSatU32(&w.late, n) {
			markWindowSaturated(w)
		}
		if addSatU32(&w.unattributedDrops, n) {
			markWindowSaturated(w)
		}
	default:
		if addSatU32(&w.unattributedDrops, n) {
			markWindowSaturated(w)
		}
	}
	if sampled {
		if addSatU32(&w.sampled, 1) {
			markWindowSaturated(w)
		}
		w.sampledF = true
		w.quality = mergeQuality(w.quality, ProcessNetworkQualitySampled)
	}
	if guarded {
		w.guarded = true
		w.quality = mergeQuality(w.quality, ProcessNetworkQualityResourceGuarded)
	}
	w.partial = true
	w.quality = mergeQuality(w.quality, ProcessNetworkQualityPartial)
	return nil
}

func (a *ProcessNetworkAggregator) creditStub(stub *pnClosedStub, kind acctKind, n uint32, guarded, sampled bool) error {
	switch kind {
	case acctShed:
		if addSatU32(&stub.shed, n) {
			stub.guarded = true
			stub.quality = mergeQuality(stub.quality, ProcessNetworkQualityResourceGuarded)
		}
		stub.guarded = true
		stub.quality = mergeQuality(stub.quality, ProcessNetworkQualityResourceGuarded)
	case acctLate:
		if addSatU32(&stub.late, n) {
			stub.guarded = true
			stub.quality = mergeQuality(stub.quality, ProcessNetworkQualityResourceGuarded)
		}
		if addSatU32(&stub.dropped, n) {
			stub.guarded = true
			stub.quality = mergeQuality(stub.quality, ProcessNetworkQualityResourceGuarded)
		}
	default:
		if addSatU32(&stub.dropped, n) {
			stub.guarded = true
			stub.quality = mergeQuality(stub.quality, ProcessNetworkQualityResourceGuarded)
		}
	}
	if sampled {
		if addSatU32(&stub.sampled, 1) {
			stub.guarded = true
		}
		stub.sampledF = true
	}
	if guarded {
		stub.guarded = true
		stub.quality = mergeQuality(stub.quality, ProcessNetworkQualityResourceGuarded)
	}
	stub.partial = true
	stub.quality = mergeQuality(stub.quality, ProcessNetworkQualityPartial)
	return nil
}

func satIncU32(dst *uint32) bool {
	v, sat := satIncUint32(*dst)
	*dst = v
	return sat
}

func satIncU64(dst *uint64) bool {
	v, sat := satIncUint64(*dst)
	*dst = v
	return sat
}

func addSatU32(dst *uint32, n uint32) bool {
	v, sat := satAddUint32(*dst, n)
	*dst = v
	return sat
}

func addSatU64(dst *uint64, n uint64) bool {
	v, sat := satAddUint64(*dst, n)
	*dst = v
	return sat
}

func satIncHeat(c *pnCandidate) {
	v, sat := satIncUint64(c.heat)
	c.heat = v
	if sat {
		markCandidateSaturated(c)
	}
}

func markCandidateSaturated(c *pnCandidate) {
	c.saturated = true
	c.partial = true
	c.guarded = true
	c.quality = mergeQuality(c.quality, ProcessNetworkQualityPartial)
	c.quality = mergeQuality(c.quality, ProcessNetworkQualityResourceGuarded)
}

func markWindowSaturated(w *pnWindow) {
	w.saturated = true
	w.partial = true
	w.guarded = true
	w.quality = mergeQuality(w.quality, ProcessNetworkQualityPartial)
	w.quality = mergeQuality(w.quality, ProcessNetworkQualityResourceGuarded)
}

func markResourcePartial(v any) {
	switch t := v.(type) {
	case *pnWindow:
		markWindowSaturated(t)
	case *pnWorkload:
		t.saturated = true
		t.partial = true
		t.guarded = true
		t.quality = mergeQuality(t.quality, ProcessNetworkQualityPartial)
		t.quality = mergeQuality(t.quality, ProcessNetworkQualityResourceGuarded)
	case *pnCandidate:
		markCandidateSaturated(t)
	}
}

func propagateSaturated(w *pnWindow, wl *pnWorkload, c *pnCandidate) {
	if c.saturated || c.exact.saturated {
		markResourcePartial(w)
		markResourcePartial(wl)
		markCandidateSaturated(c)
	}
}

func (a *ProcessNetworkAggregator) FlushDue(ctx context.Context, now time.Time) (ProcessNetworkFlushOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProcessNetworkFlushOutput{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return ProcessNetworkFlushOutput{}, errors.New("PROCESS_NETWORK_AGGREGATOR_CLOSED")
	}
	now = now.UTC()
	var due []*pnWindow
	for _, w := range a.windows {
		if !w.end.After(now) {
			due = append(due, w)
		}
	}
	sort.Slice(due, func(i, j int) bool { return due[i].start.Before(due[j].start) })
	out := ProcessNetworkFlushOutput{}
	for _, w := range due {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		part := a.emitWindow(w)
		out.Evidence = append(out.Evidence, part.Evidence...)
		out.Accounting = append(out.Accounting, part.Accounting...)
		delete(a.windows, w.start.UnixNano())
		out.Accounting = append(out.Accounting, a.retainClosed(w, part)...)
	}
	out.Accounting = append(out.Accounting, a.emitExactBucketAccounting()...)
	return a.deliver(ctx, out)
}

func (a *ProcessNetworkAggregator) FlushAll(ctx context.Context) (ProcessNetworkFlushOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProcessNetworkFlushOutput{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return ProcessNetworkFlushOutput{}, errors.New("PROCESS_NETWORK_AGGREGATOR_CLOSED")
	}
	var due []*pnWindow
	for _, w := range a.windows {
		due = append(due, w)
	}
	sort.Slice(due, func(i, j int) bool { return due[i].start.Before(due[j].start) })
	out := ProcessNetworkFlushOutput{}
	for _, w := range due {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		part := a.emitWindow(w)
		out.Evidence = append(out.Evidence, part.Evidence...)
		out.Accounting = append(out.Accounting, part.Accounting...)
		delete(a.windows, w.start.UnixNano())
		out.Accounting = append(out.Accounting, a.retainClosed(w, part)...)
	}
	out.Accounting = append(out.Accounting, a.emitExactBucketAccounting()...)
	return a.deliver(ctx, out)
}

func (a *ProcessNetworkAggregator) Close(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	var due []*pnWindow
	for _, w := range a.windows {
		due = append(due, w)
	}
	sort.Slice(due, func(i, j int) bool { return due[i].start.Before(due[j].start) })
	out := ProcessNetworkFlushOutput{}
	for _, w := range due {
		part := a.emitWindow(w)
		out.Evidence = append(out.Evidence, part.Evidence...)
		out.Accounting = append(out.Accounting, part.Accounting...)
		delete(a.windows, w.start.UnixNano())
	}
	out.Accounting = append(out.Accounting, a.emitExactBucketAccounting()...)
	a.closedStubs = make(map[int64]*pnClosedStub)
	a.accountingStubs = make(map[int64]*pnClosedStub)
	a.closed = true
	_, err := a.deliverLocked(ctx, out)
	return err
}

func (a *ProcessNetworkAggregator) emitExactBucketAccounting() []ProcessNetworkWindowAccounting {
	return append(a.emitStubMap(a.closedStubs), a.emitStubMap(a.accountingStubs)...)
}

func (a *ProcessNetworkAggregator) emitStubMap(stubs map[int64]*pnClosedStub) []ProcessNetworkWindowAccounting {
	if len(stubs) == 0 {
		return nil
	}
	keys := make([]int64, 0, len(stubs))
	for k := range stubs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	var out []ProcessNetworkWindowAccounting
	for _, k := range keys {
		if acc, ok := a.stubToAccounting(stubs[k]); ok {
			out = append(out, acc)
		}
	}
	return out
}

func (a *ProcessNetworkAggregator) stubToAccounting(stub *pnClosedStub) (ProcessNetworkWindowAccounting, bool) {
	if stub.late == 0 && stub.dropped == 0 && stub.shed == 0 {
		return ProcessNetworkWindowAccounting{}, false
	}
	q := mergeQuality(stub.quality, ProcessNetworkQualityPartial)
	if stub.guarded {
		q = mergeQuality(q, ProcessNetworkQualityResourceGuarded)
	}
	acc := ProcessNetworkWindowAccounting{
		SchemaVersion:       ProcessNetworkSchemaVersion,
		SourceProvenance:    a.cfg.SourceProvenance,
		BucketStartAt:       stub.start.UTC(),
		BucketEndAt:         stub.end.UTC(),
		BucketSeconds:       a.bucketSeconds(),
		DroppedRecordsCount: stub.dropped,
		ShedRecordsCount:    stub.shed,
		SampledEventsCount:  stub.sampled,
		LateEventsCount:     stub.late,
		Quality:             q,
		PartialFlag:         true,
		SampledFlag:         stub.sampledF,
		ResourceGuardedFlag: stub.guarded,
	}
	stub.late = 0
	stub.dropped = 0
	stub.shed = 0
	stub.sampled = 0
	return acc, true
}

func (a *ProcessNetworkAggregator) retainClosed(w *pnWindow, emitted ProcessNetworkFlushOutput) []ProcessNetworkWindowAccounting {
	for k, c := range w.workloads {
		for dkey := range c.candidates {
			delete(a.globalKeys, globalKey(w.start, k, dkey))
		}
	}
	if !a.closedThroughSet || w.start.After(a.closedThrough) {
		a.closedThrough = w.start
		a.closedThroughSet = true
	}
	if a.cfg.MaxClosedWindows <= 0 {
		return nil
	}
	var extra []ProcessNetworkWindowAccounting
	for len(a.closedStubs) >= a.cfg.MaxClosedWindows {
		var oldest int64
		first := true
		for k := range a.closedStubs {
			if first || k < oldest {
				oldest = k
				first = false
			}
		}
		if acc, ok := a.stubToAccounting(a.closedStubs[oldest]); ok {
			extra = append(extra, acc)
		}
		delete(a.closedStubs, oldest)
	}
	q := ProcessNetworkQualityPartial
	if len(emitted.Accounting) > 0 {
		q = emitted.Accounting[0].Quality
	}
	a.closedStubs[w.start.UnixNano()] = &pnClosedStub{
		start:   w.start,
		end:     w.end,
		quality: q,
		partial: true,
	}
	return extra
}

func (a *ProcessNetworkAggregator) bucketSeconds() uint32 {
	return uint32(a.cfg.Window / time.Second)
}

func (a *ProcessNetworkAggregator) emitWindow(w *pnWindow) ProcessNetworkFlushOutput {
	out := ProcessNetworkFlushOutput{}
	secs := a.bucketSeconds()
	prov := a.cfg.SourceProvenance

	type recSort struct {
		key string
		rec ProcessNetworkEvidence
	}
	var recs []recSort
	var accs []ProcessNetworkWindowAccounting

	wkKeys := make([]string, 0, len(w.workloads))
	for k := range w.workloads {
		wkKeys = append(wkKeys, k)
	}
	sort.Strings(wkKeys)

	windowDropped := w.unattributedDrops
	windowShed := w.shed
	windowSampled := w.sampled
	var windowEvicted uint32
	var windowOverflowRecords uint64

	for _, wk := range wkKeys {
		wl := w.workloads[wk]
		dkeys := make([]string, 0, len(wl.candidates))
		for k := range wl.candidates {
			dkeys = append(dkeys, k)
		}
		sort.Strings(dkeys)
		for _, dk := range dkeys {
			c := wl.candidates[dk]
			q := mergeQuality(c.quality, wl.quality)
			rec := ProcessNetworkEvidence{
				SchemaVersion:       ProcessNetworkSchemaVersion,
				SourceProvenance:    prov,
				BucketStartAt:       w.start.UTC(),
				BucketEndAt:         w.end.UTC(),
				BucketSeconds:       secs,
				Workload:            emitWorkload(wl.identity, c),
				RemoteIP:            c.remote,
				RemotePort:          c.port,
				Protocol:            c.proto,
				IPVersion:           ipVersion(c.remote),
				Quality:             q,
				DroppedRecordsCount: c.dropped,
				ShedRecordsCount:    c.shed,
				SampledEventsCount:  c.sampled,
				SampledFlag:         c.sampledF || wl.sampledF,
				PartialFlag:         c.partial || wl.partial || q != ProcessNetworkQualityValid,
				ResourceGuardedFlag: c.guarded || wl.guarded || c.saturated || wl.saturated || w.saturated,
			}
			if rec.PartialFlag && qualityRank(rec.Quality) < qualityRank(ProcessNetworkQualityPartial) {
				rec.Quality = ProcessNetworkQualityPartial
			}
			if rec.SampledFlag {
				rec.Quality = mergeQuality(rec.Quality, ProcessNetworkQualitySampled)
			}
			if rec.ResourceGuardedFlag {
				rec.Quality = mergeQuality(rec.Quality, ProcessNetworkQualityResourceGuarded)
			}
			if c.pid.set && !c.pid.mixed {
				v := c.pid.value
				rec.PID = &v
			}
			if c.localIP.set && !c.localIP.mixed && c.localIP.value.IsValid() && !c.localIP.value.IsUnspecified() && c.localIP.value == c.localIP.value.Unmap() {
				v := c.localIP.value
				rec.LocalIP = &v
			}
			if c.localPort.set && !c.localPort.mixed && c.localPort.value >= 1 {
				v := c.localPort.value
				rec.LocalPort = &v
			}
			fillTraffic(&rec, c.exact, c.proto)
			recs = append(recs, recSort{key: dk, rec: rec})
		}

		ovKeys := make([]ProcessNetworkProtocol, 0, len(wl.overflow))
		for p := range wl.overflow {
			ovKeys = append(ovKeys, p)
		}
		sort.Slice(ovKeys, func(i, j int) bool { return ovKeys[i] < ovKeys[j] })
		summaries := make([]ProcessNetworkOverflowSummary, 0, len(ovKeys))
		var ovEvicted uint32
		var ovShed uint32
		var ovSampled uint32
		var ovRecords uint64
		ovSat := false
		for _, p := range ovKeys {
			ov := wl.overflow[p]
			sum := fillOverflowSummary(ov)
			if ov.metrics.saturated {
				ovSat = true
			}
			summaries = append(summaries, sum)
			if addSatU32(&ovEvicted, ov.evicted) {
				ovSat = true
			}
			if addSatU32(&ovShed, ov.shed) {
				ovSat = true
			}
			if addSatU32(&ovSampled, ov.sampled) {
				ovSat = true
			}
			if addSatU64(&ovRecords, ov.metrics.eventCount) {
				ovSat = true
			}
		}
		idCopy := wl.identity
		accQ := mergeQuality(wl.quality, w.quality)
		if len(summaries) > 0 || wl.shed > 0 || wl.dropped > 0 {
			accQ = mergeQuality(accQ, ProcessNetworkQualityPartial)
		}
		sampled := wl.sampled
		if addSatU32(&sampled, ovSampled) {
			ovSat = true
		}
		acc := ProcessNetworkWindowAccounting{
			SchemaVersion:          ProcessNetworkSchemaVersion,
			SourceProvenance:       prov,
			BucketStartAt:          w.start.UTC(),
			BucketEndAt:            w.end.UTC(),
			BucketSeconds:          secs,
			Workload:               &idCopy,
			DroppedRecordsCount:    wl.dropped,
			ShedRecordsCount:       ovShed,
			SampledEventsCount:     sampled,
			EvictedCandidateCount:  ovEvicted,
			OverflowedRecordsCount: ovRecords,
			Overflows:              summaries,
			Quality:                accQ,
			PartialFlag:            wl.partial || len(summaries) > 0 || w.partial,
			SampledFlag:            wl.sampledF || w.sampledF,
			ResourceGuardedFlag:    wl.guarded || w.guarded || wl.saturated || ovSat,
		}
		if acc.ResourceGuardedFlag {
			acc.Quality = mergeQuality(acc.Quality, ProcessNetworkQualityResourceGuarded)
		}
		if acc.PartialFlag && qualityRank(acc.Quality) < qualityRank(ProcessNetworkQualityPartial) {
			acc.Quality = ProcessNetworkQualityPartial
		}
		accs = append(accs, acc)
		_ = addSatU32(&windowEvicted, ovEvicted)
		_ = addSatU32(&windowShed, ovShed)
		_ = addSatU64(&windowOverflowRecords, ovRecords)
	}

	if windowDropped > 0 || windowShed > 0 || windowEvicted > 0 || windowOverflowRecords > 0 || w.late > 0 || a.needsWindowAcc(w) {
		q := w.quality
		if w.partial || windowDropped > 0 || windowEvicted > 0 || windowOverflowRecords > 0 {
			q = mergeQuality(q, ProcessNetworkQualityPartial)
		}
		if w.guarded || w.saturated {
			q = mergeQuality(q, ProcessNetworkQualityResourceGuarded)
		}
		accs = append([]ProcessNetworkWindowAccounting{{
			SchemaVersion:          ProcessNetworkSchemaVersion,
			SourceProvenance:       prov,
			BucketStartAt:          w.start.UTC(),
			BucketEndAt:            w.end.UTC(),
			BucketSeconds:          secs,
			DroppedRecordsCount:    windowDropped,
			ShedRecordsCount:       windowShed,
			SampledEventsCount:     windowSampled,
			LateEventsCount:        w.late,
			EvictedCandidateCount:  windowEvicted,
			OverflowedRecordsCount: windowOverflowRecords,
			Quality:                q,
			PartialFlag:            w.partial || windowDropped > 0 || windowEvicted > 0 || windowOverflowRecords > 0,
			SampledFlag:            w.sampledF,
			ResourceGuardedFlag:    w.guarded || w.saturated,
		}}, accs...)
	}

	sort.Slice(recs, func(i, j int) bool { return recs[i].key < recs[j].key })
	for _, r := range recs {
		out.Evidence = append(out.Evidence, r.rec)
	}
	out.Accounting = accs
	return out
}

func (a *ProcessNetworkAggregator) needsWindowAcc(w *pnWindow) bool {
	return w.unattributedDrops > 0 || w.shed > 0 || w.sampled > 0 || w.late > 0 || w.partial || w.guarded
}

func emitWorkload(base ProcessNetworkWorkloadIdentity, c *pnCandidate) ProcessNetworkWorkloadIdentity {
	out := ProcessNetworkWorkloadIdentity{
		Kind:             base.Kind,
		Value:            base.Value,
		ContainerID:      base.ContainerID,
		NetworkNamespace: base.NetworkNamespace,
	}
	if c.svc.set && !c.svc.mixed {
		out.ServiceID = c.svc.value
	}
	if c.exeID.set && !c.exeID.mixed {
		out.ExecutableIdentity = c.exeID.value
	}
	if c.exePath.set && !c.exePath.mixed {
		out.ExecutablePath = c.exePath.value
	}
	if c.name.set && !c.name.mixed {
		out.ProcessName = c.name.value
	}
	if c.cgroup.set && !c.cgroup.mixed {
		out.CgroupPath = c.cgroup.value
	}
	return out
}

func fillTraffic(rec *ProcessNetworkEvidence, m pnMetrics, proto ProcessNetworkProtocol) {
	if m.hasTx {
		rec.BytesTx = uint64Ptr(m.bytesTx)
	}
	if m.hasRx {
		rec.BytesRx = uint64Ptr(m.bytesRx)
	}
	if m.saturated {
		rec.PartialFlag = true
		rec.ResourceGuardedFlag = true
		rec.Quality = mergeQuality(rec.Quality, ProcessNetworkQualityPartial)
		rec.Quality = mergeQuality(rec.Quality, ProcessNetworkQualityResourceGuarded)
	}
	if proto != ProcessNetworkProtocolTCP {
		return
	}
	if m.hasActive {
		rec.ActiveConnections = uint32Ptr(m.active)
	}
	if m.hasNew {
		rec.NewConnections = uint32Ptr(m.newConn)
	}
	if m.hasClosed {
		rec.ClosedConnections = uint32Ptr(m.closed)
	}
	if m.hasDur {
		rec.DurationTotalMs = uint64Ptr(m.duration)
	}
	if m.rttN > 0 {
		avg, sat := meanRTTUs(m.rttSum, m.rttN)
		rec.MeanRTTUs = uint32Ptr(avg)
		if sat {
			rec.PartialFlag = true
			rec.ResourceGuardedFlag = true
			rec.Quality = mergeQuality(rec.Quality, ProcessNetworkQualityPartial)
			rec.Quality = mergeQuality(rec.Quality, ProcessNetworkQualityResourceGuarded)
		}
	}
	if m.hasRtx {
		rec.RetransmitCount = uint32Ptr(m.rtx)
	}
	if m.hasFail {
		rec.ConnectionFailures = uint32Ptr(m.fail)
	}
}

func fillOverflowSummary(ov *pnOverflow) ProcessNetworkOverflowSummary {
	sum := ProcessNetworkOverflowSummary{
		Protocol:               ov.proto,
		OverflowedRecordsCount: ov.metrics.eventCount,
		EvictedCandidateCount:  ov.evicted,
		ShedRecordsCount:       ov.shed,
		SampledEventsCount:     ov.sampled,
	}
	if ov.metrics.hasTx {
		sum.BytesTx = uint64Ptr(ov.metrics.bytesTx)
	}
	if ov.metrics.hasRx {
		sum.BytesRx = uint64Ptr(ov.metrics.bytesRx)
	}
	if ov.proto != ProcessNetworkProtocolTCP {
		return sum
	}
	if ov.metrics.hasActive {
		sum.ActiveConnections = uint32Ptr(ov.metrics.active)
	}
	if ov.metrics.hasDur {
		sum.DurationTotalMs = uint64Ptr(ov.metrics.duration)
	}
	if ov.metrics.rttN > 0 {
		avg, _ := meanRTTUs(ov.metrics.rttSum, ov.metrics.rttN)
		sum.MeanRTTUs = uint32Ptr(avg)
	}
	if ov.metrics.hasRtx {
		sum.RetransmitCount = uint32Ptr(ov.metrics.rtx)
	}
	if ov.metrics.hasNew {
		sum.NewConnections = uint32Ptr(ov.metrics.newConn)
	}
	if ov.metrics.hasClosed {
		sum.ClosedConnections = uint32Ptr(ov.metrics.closed)
	}
	if ov.metrics.hasFail {
		sum.ConnectionFailures = uint32Ptr(ov.metrics.fail)
	}
	return sum
}

func (a *ProcessNetworkAggregator) deliver(ctx context.Context, out ProcessNetworkFlushOutput) (ProcessNetworkFlushOutput, error) {
	return a.deliverLocked(ctx, out)
}

func (a *ProcessNetworkAggregator) deliverLocked(ctx context.Context, out ProcessNetworkFlushOutput) (ProcessNetworkFlushOutput, error) {
	var first error
	for i := range out.Evidence {
		if err := ctx.Err(); err != nil {
			if first == nil {
				first = err
			}
			break
		}
		if err := out.Evidence[i].Validate(); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		if err := a.sink.AcceptProcessNetworkEvidence(ctx, out.Evidence[i]); err != nil {
			if first == nil {
				first = err
			}
		}
	}
	for i := range out.Accounting {
		if err := ctx.Err(); err != nil {
			if first == nil {
				first = err
			}
			break
		}
		if err := out.Accounting[i].Validate(); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		if err := a.sink.AcceptProcessNetworkWindowAccounting(ctx, out.Accounting[i]); err != nil {
			if first == nil {
				first = err
			}
		}
	}
	if first != nil && errors.Is(first, ErrStructuredTransportNotWired) {
		return out, first
	}
	return out, first
}

func (a *ProcessNetworkAggregator) ActiveKeyCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.globalKeys)
}

func (a *ProcessNetworkAggregator) RetainedWindowCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.windows) + len(a.closedStubs) + len(a.accountingStubs)
}

func alignUTC(t time.Time, d time.Duration) time.Time {
	t = t.UTC()
	ns := t.UnixNano()
	dur := d.Nanoseconds()
	if dur <= 0 {
		return t
	}
	aligned := (ns / dur) * dur
	if aligned < 0 && ns%dur != 0 {
		aligned -= dur
	}
	return time.Unix(0, aligned).UTC()
}

func globalKey(start time.Time, workloadKey, destKey string) string {
	return fmt.Sprintf("%d|%s|%s", start.UnixNano(), workloadKey, destKey)
}
