package collector

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"time"

	"platform/agent/common/telemetry"
)

const (
	linuxDPKGInventoryScope = "linux.dpkg.v1"
	linuxRPMInventoryScope  = "linux.rpm.v1"

	inventoryQualityAuthoritative = "AUTHORITATIVE_SOURCE"
	inventoryQualityPartial       = "PARTIAL_SOURCE"
	inventoryQualityFailed        = "SOURCE_FAILED"

	inventoryProvenanceDPKG = "agent.linux.dpkg.status"
	inventoryProvenanceRPM  = "agent.linux.rpm.query"

	defaultDpkgStatusPath = "/var/lib/dpkg/status"

	localCaptureIDPrefix = "local-capture-"

	inventoryLossSourceMissing          = "SOURCE_MISSING"
	inventoryLossSourcePermissionDenied = "SOURCE_PERMISSION_DENIED"
	inventoryLossSourceUnavailable      = "SOURCE_UNAVAILABLE"
	inventoryLossSourceExecutionFailed  = "SOURCE_EXECUTION_FAILED"
	inventoryLossSourceTimeout          = "SOURCE_TIMEOUT"
	inventoryLossSourceMalformed        = "SOURCE_MALFORMED"
	inventoryLossSourceTruncated        = "SOURCE_TRUNCATED"
	inventoryLossRecordMalformed        = "RECORD_MALFORMED"
	inventoryLossRecordOversized        = "RECORD_OVERSIZED"
	inventoryLossIdentityInvalid        = "IDENTITY_INVALID"
	inventoryLossItemBoundReached       = "ITEM_BOUND_REACHED"
	inventoryLossSelectorInvalid        = "TARGETED_SELECTOR_INVALID"
	inventoryLossSelectorBoundExceeded  = "TARGETED_SELECTOR_BOUND_EXCEEDED"
	inventoryLossCallerRequestInvalid   = "CALLER_REQUEST_INVALID"
)

// Compile-time boundary: inventory collection is not HostCollector and is not Sample.
var (
	_ StructuredEvidenceSink = UnwiredStructuredEvidenceSink{}
	_ StructuredEvidenceSink = (*LocalRecordingStructuredEvidenceSink)(nil)
)

// SoftwareInventorySelector is a TARGETED literal allowlist entry.
// Values are data only: no regex, glob, shell, or package-query expressions.
type SoftwareInventorySelector struct {
	ProductName     string
	NativePackageID string
}

// SoftwareInventoryRequest is the typed FULL/TARGETED capture request.
// SnapshotID is caller lifecycle authority when supplied. An empty SnapshotID
// causes a process-local capture id (local-capture-*) that is NOT a durable
// cross-process replay identity.
type SoftwareInventoryRequest struct {
	SnapshotID   string
	SnapshotMode SnapshotMode
	SourceType   InventorySourceType
	Selectors    []SoftwareInventorySelector
}

type softwareInventoryBounds struct {
	MaxItems               int
	MaxDpkgBytes           int
	MaxDpkgStanzaBytes     int
	MaxDpkgLineBytes       int
	MaxRPMStdoutBytes      int
	MaxRPMStderrBytes      int
	RPMQueryTimeout        time.Duration
	MaxTargetedSelectors   int
	MaxTargetedSelectorLen int
}

func defaultSoftwareInventoryBounds() softwareInventoryBounds {
	return softwareInventoryBounds{
		MaxItems:               maxSoftwareInventoryItems,
		MaxDpkgBytes:           maxDpkgStatusBytes,
		MaxDpkgStanzaBytes:     maxDpkgStanzaBytes,
		MaxDpkgLineBytes:       maxDpkgLineBytes,
		MaxRPMStdoutBytes:      maxRPMStdoutBytes,
		MaxRPMStderrBytes:      maxRPMStderrBytes,
		RPMQueryTimeout:        rpmQueryTimeout,
		MaxTargetedSelectors:   maxTargetedSelectors,
		MaxTargetedSelectorLen: maxTargetedSelectorLen,
	}
}

type dpkgOpenFunc func(ctx context.Context) (io.ReadCloser, error)

// LinuxSoftwareInventoryCollector produces typed SoftwareInventorySnapshot
// evidence from DPKG status or an allowlisted read-only RPM query.
// It is not a HostCollector and must not be registered on the scalar path.
type LinuxSoftwareInventoryCollector struct {
	bounds      softwareInventoryBounds
	dpkgPath    string
	openDPKG    dpkgOpenFunc
	rpmResolver func() (string, error)
	rpmRunner   rpmCommandRunner
	now         func() time.Time
	newID       func() (string, error)
	sink        StructuredEvidenceSink
}

func NewLinuxSoftwareInventoryCollector() *LinuxSoftwareInventoryCollector {
	c := &LinuxSoftwareInventoryCollector{
		bounds:      defaultSoftwareInventoryBounds(),
		dpkgPath:    defaultDpkgStatusPath,
		rpmResolver: resolveRPMExecutable,
		rpmRunner:   runAllowlistedRPM,
		now:         func() time.Time { return time.Now().UTC() },
		newID:       newLocalCaptureID,
		sink:        UnwiredStructuredEvidenceSink{},
	}
	c.openDPKG = c.openDefaultDPKG
	return c
}

func (c *LinuxSoftwareInventoryCollector) openDefaultDPKG(ctx context.Context) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.Open(c.dpkgPath)
}

// Capture enumerates installed software from the requested native source.
// Caller context cancellation/deadline is returned as ctx.Err() and is never
// rewritten into a COMPLETE/FAILED snapshot. Internal RPM source timeout while
// the parent context remains live becomes categorical source-failure evidence.
func (c *LinuxSoftwareInventoryCollector) Capture(ctx context.Context, req SoftwareInventoryRequest) (SoftwareInventorySnapshot, error) {
	if err := ctx.Err(); err != nil {
		return SoftwareInventorySnapshot{}, err
	}
	started := c.now().UTC()
	snap, err := c.capture(ctx, req, started)
	if err != nil {
		return SoftwareInventorySnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return SoftwareInventorySnapshot{}, err
	}
	if c.sink != nil {
		if sinkErr := c.sink.AcceptSoftwareInventory(ctx, snap); sinkErr != nil && !errors.Is(sinkErr, ErrStructuredTransportNotWired) {
			// Local/unwired sinks may only fail validation; do not claim transport.
			return SoftwareInventorySnapshot{}, sinkErr
		}
	}
	return snap, nil
}

func (c *LinuxSoftwareInventoryCollector) capture(ctx context.Context, req SoftwareInventoryRequest, started time.Time) (SoftwareInventorySnapshot, error) {
	id, idErr := c.resolveSnapshotID(req.SnapshotID)
	if idErr != nil {
		generated, genErr := c.newID()
		if genErr != nil {
			return SoftwareInventorySnapshot{}, genErr
		}
		return c.mustFailedSnapshot(generated, req, started, inventoryLossCallerRequestInvalid)
	}
	if req.SnapshotMode != SnapshotModeFull && req.SnapshotMode != SnapshotModeTargeted {
		return c.mustFailedSnapshot(id, req, started, inventoryLossCallerRequestInvalid)
	}
	if req.SourceType != InventorySourceTypeDPKG && req.SourceType != InventorySourceTypeRPM {
		return c.mustFailedSnapshot(id, req, started, inventoryLossCallerRequestInvalid)
	}

	losses := newLossSet()
	selectors, selectorOK := c.normalizeSelectors(req, losses)
	if !selectorOK {
		return c.mustFailedSnapshot(id, req, started, losses.join())
	}

	filter := itemEmitFilter{mode: req.SnapshotMode, selectors: selectors, maxItems: c.bounds.MaxItems}
	var parsed inventoryParseResult
	var err error
	switch req.SourceType {
	case InventorySourceTypeDPKG:
		parsed, err = c.collectDPKG(ctx, filter)
	case InventorySourceTypeRPM:
		parsed, err = c.collectRPM(ctx, filter)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return SoftwareInventorySnapshot{}, err
		}
		return SoftwareInventorySnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return SoftwareInventorySnapshot{}, err
	}

	losses.merge(parsed.losses)
	items := parsed.items
	truncated := parsed.truncated
	status, quality, reason := classifyCapture(parsed.sourceFailed, truncated, losses, len(items))
	completed := c.now().UTC()
	if completed.Before(started) {
		completed = started
	}
	snap := SoftwareInventorySnapshot{
		SchemaVersion:          SoftwareInventorySchemaVersion,
		SnapshotID:             id,
		SourceMode:             InventorySourceModeAgent,
		SourceType:             req.SourceType,
		InventoryScope:         inventoryScopeFor(req.SourceType),
		SnapshotMode:           req.SnapshotMode,
		StartedAt:              started,
		CompletedAt:            completed,
		CaptureStatus:          status,
		ItemCount:              len(items),
		Truncated:              truncated,
		IncompleteOrLossReason: reason,
		SourceProvenance:       provenanceFor(req.SourceType),
		Quality:                quality,
		Items:                  items,
	}
	if err := snap.Validate(); err != nil {
		return SoftwareInventorySnapshot{}, err
	}
	return snap, nil
}

func classifyCapture(sourceFailed, truncated bool, losses *lossSet, itemCount int) (CaptureStatus, string, string) {
	reason := losses.join()
	if sourceFailed {
		if itemCount > 0 {
			if reason == "" {
				reason = inventoryLossSourceExecutionFailed
			}
			return CaptureStatusPartial, inventoryQualityPartial, reason
		}
		if reason == "" {
			reason = inventoryLossSourceUnavailable
		}
		return CaptureStatusFailed, inventoryQualityFailed, reason
	}
	if truncated || !losses.empty() {
		if reason == "" {
			reason = inventoryLossSourceTruncated
		}
		return CaptureStatusPartial, inventoryQualityPartial, reason
	}
	return CaptureStatusComplete, inventoryQualityAuthoritative, ""
}

func (c *LinuxSoftwareInventoryCollector) mustFailedSnapshot(id string, req SoftwareInventoryRequest, started time.Time, reason string) (SoftwareInventorySnapshot, error) {
	snap := c.failedSnapshot(id, req, started, reason, inventoryQualityFailed)
	if err := snap.Validate(); err != nil {
		return SoftwareInventorySnapshot{}, err
	}
	return snap, nil
}

func (c *LinuxSoftwareInventoryCollector) failedSnapshot(id string, req SoftwareInventoryRequest, started time.Time, reason, quality string) SoftwareInventorySnapshot {
	if strings.TrimSpace(id) == "" {
		id, _ = newLocalCaptureID()
	}
	mode := req.SnapshotMode
	if mode != SnapshotModeFull && mode != SnapshotModeTargeted {
		mode = SnapshotModeFull
	}
	src := req.SourceType
	if src != InventorySourceTypeDPKG && src != InventorySourceTypeRPM {
		src = InventorySourceTypeDPKG
	}
	completed := c.now().UTC()
	if completed.Before(started) {
		completed = started
	}
	if strings.TrimSpace(reason) == "" {
		reason = inventoryLossCallerRequestInvalid
	}
	return SoftwareInventorySnapshot{
		SchemaVersion:          SoftwareInventorySchemaVersion,
		SnapshotID:             id,
		SourceMode:             InventorySourceModeAgent,
		SourceType:             src,
		InventoryScope:         inventoryScopeFor(src),
		SnapshotMode:           mode,
		StartedAt:              started,
		CompletedAt:            completed,
		CaptureStatus:          CaptureStatusFailed,
		ItemCount:              0,
		Truncated:              false,
		IncompleteOrLossReason: reason,
		SourceProvenance:       provenanceFor(src),
		Quality:                quality,
		Items:                  nil,
	}
}

func (c *LinuxSoftwareInventoryCollector) resolveSnapshotID(supplied string) (string, error) {
	if strings.TrimSpace(supplied) == "" {
		return c.newID()
	}
	if err := requireBoundedToken("snapshot_id", supplied, maxSoftwareInventorySnapshotIDLen); err != nil {
		return supplied, err
	}
	return supplied, nil
}

func (c *LinuxSoftwareInventoryCollector) normalizeSelectors(req SoftwareInventoryRequest, losses *lossSet) ([]SoftwareInventorySelector, bool) {
	if req.SnapshotMode != SnapshotModeTargeted {
		return nil, true
	}
	if len(req.Selectors) > c.bounds.MaxTargetedSelectors {
		losses.add(inventoryLossSelectorBoundExceeded)
		return nil, false
	}
	out := make([]SoftwareInventorySelector, 0, len(req.Selectors))
	for _, sel := range req.Selectors {
		name := sel.ProductName
		id := sel.NativePackageID
		if name == "" && id == "" {
			losses.add(inventoryLossSelectorInvalid)
			continue
		}
		if name != "" {
			if err := requireBoundedToken("targeted.product_name", name, c.bounds.MaxTargetedSelectorLen); err != nil {
				losses.add(inventoryLossSelectorInvalid)
				continue
			}
		}
		if id != "" {
			if err := requireBoundedToken("targeted.native_package_id", id, c.bounds.MaxTargetedSelectorLen); err != nil {
				losses.add(inventoryLossSelectorInvalid)
				continue
			}
		}
		out = append(out, SoftwareInventorySelector{ProductName: name, NativePackageID: id})
	}
	if len(req.Selectors) > 0 && len(out) == 0 {
		return nil, false
	}
	return out, true
}

type itemEmitFilter struct {
	mode      SnapshotMode
	selectors []SoftwareInventorySelector
	maxItems  int
}

func (f itemEmitFilter) accept(item SoftwareInventoryItem) bool {
	if f.mode != SnapshotModeTargeted {
		return true
	}
	return targetedMatch(item, f.selectors)
}

func targetedMatch(item SoftwareInventoryItem, selectors []SoftwareInventorySelector) bool {
	for _, sel := range selectors {
		if sel.NativePackageID != "" && sel.ProductName != "" {
			if item.NativePackageID == sel.NativePackageID && item.ProductName == sel.ProductName {
				return true
			}
			continue
		}
		if sel.NativePackageID != "" && item.NativePackageID == sel.NativePackageID {
			return true
		}
		if sel.ProductName != "" && item.ProductName == sel.ProductName {
			return true
		}
	}
	return false
}

func inventoryScopeFor(src InventorySourceType) string {
	switch src {
	case InventorySourceTypeRPM:
		return linuxRPMInventoryScope
	default:
		return linuxDPKGInventoryScope
	}
}

func provenanceFor(src InventorySourceType) string {
	switch src {
	case InventorySourceTypeRPM:
		return inventoryProvenanceRPM
	default:
		return inventoryProvenanceDPKG
	}
}

func newLocalCaptureID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return localCaptureIDPrefix + hex.EncodeToString(b[:]), nil
}

func classifyOpenError(err error, losses *lossSet) inventoryParseResult {
	res := inventoryParseResult{losses: losses, sourceFailed: true}
	switch {
	case errors.Is(err, os.ErrNotExist):
		losses.add(inventoryLossSourceMissing)
	case errors.Is(err, os.ErrPermission):
		losses.add(inventoryLossSourcePermissionDenied)
	default:
		losses.add(inventoryLossSourceUnavailable)
	}
	return res
}

type inventoryParseResult struct {
	items        []SoftwareInventoryItem
	truncated    bool
	sourceFailed bool
	losses       *lossSet
}

type lossSet struct {
	order []string
	seen  map[string]struct{}
}

func newLossSet() *lossSet {
	return &lossSet{seen: make(map[string]struct{})}
}

func (l *lossSet) add(code string) {
	if l == nil || code == "" {
		return
	}
	if _, ok := l.seen[code]; ok {
		return
	}
	l.seen[code] = struct{}{}
	l.order = append(l.order, code)
}

func (l *lossSet) merge(other *lossSet) {
	if other == nil {
		return
	}
	for _, code := range other.order {
		l.add(code)
	}
}

func (l *lossSet) empty() bool {
	return l == nil || len(l.order) == 0
}

func (l *lossSet) join() string {
	if l == nil || len(l.order) == 0 {
		return ""
	}
	joined := strings.Join(l.order, ",")
	if len(joined) > maxSoftwareInventoryReasonLen {
		return joined[:maxSoftwareInventoryReasonLen]
	}
	return joined
}

func emitInventoryItem(items []SoftwareInventoryItem, item SoftwareInventoryItem, filter itemEmitFilter, losses *lossSet) ([]SoftwareInventoryItem, bool) {
	item.NormalizedProductID = ""
	item.NormalizationVersion = ""
	item.NormalizationConfidence = nil
	if err := item.validate(len(items)); err != nil {
		losses.add(inventoryLossIdentityInvalid)
		return items, false
	}
	if !filter.accept(item) {
		return items, false
	}
	maxItems := filter.maxItems
	if maxItems <= 0 {
		maxItems = maxSoftwareInventoryItems
	}
	if len(items) >= maxItems {
		losses.add(inventoryLossItemBoundReached)
		return items, true
	}
	return append(items, item), false
}

// inventoryCollectorImplementsHostCollector reports whether Capture is wired as
// a scalar HostCollector. Always false; used by boundary tests.
func inventoryCollectorImplementsHostCollector() bool {
	typ := reflect.TypeOf((*LinuxSoftwareInventoryCollector)(nil))
	host := reflect.TypeOf((*HostCollector)(nil)).Elem()
	return typ.Implements(host) || typ.Elem().Implements(host)
}

func inventorySnapshotAsSample(_ SoftwareInventorySnapshot) (telemetry.Sample, bool) {
	return telemetry.Sample{}, false
}
