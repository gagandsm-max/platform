package collector

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
)

const dpkgInstalledStatus = "install ok installed"

type dpkgFieldSet struct {
	Package      string
	Status       string
	Version      string
	Architecture string
	Maintainer   string
	sawPackage    bool
	sawStatus     bool
	sawVersion    bool
	sawArch       bool
	sawMaintainer bool
}

func (c *LinuxSoftwareInventoryCollector) collectDPKG(ctx context.Context, filter itemEmitFilter) (inventoryParseResult, error) {
	losses := newLossSet()
	if err := ctx.Err(); err != nil {
		return inventoryParseResult{}, err
	}
	rc, err := c.openDPKG(ctx)
	if err != nil {
		if err := ctx.Err(); err != nil {
			return inventoryParseResult{}, err
		}
		return classifyOpenError(err, losses), nil
	}
	defer rc.Close()

	maxBytes := int64(c.bounds.MaxDpkgBytes)
	if maxBytes <= 0 {
		maxBytes = maxDpkgStatusBytes
	}
	limited := &countingLimitReader{r: rc, limit: maxBytes}

	lineLimit := c.bounds.MaxDpkgLineBytes
	if lineLimit <= 0 {
		lineLimit = maxDpkgLineBytes
	}
	stanzaLimit := c.bounds.MaxDpkgStanzaBytes
	if stanzaLimit <= 0 {
		stanzaLimit = maxDpkgStanzaBytes
	}
	br := bufio.NewReaderSize(limited, lineLimit+1)

	var (
		items        []SoftwareInventoryItem
		stanza       []byte
		consumed     int
		skipStanza   bool
		sawAnyStanza bool
		malformed    int
		usable       int
		itemBound    bool
	)

	resetStanza := func() {
		stanza = stanza[:0]
		consumed = 0
		skipStanza = false
	}

	flush := func(raw []byte, truncatedEOF bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			return nil
		}
		sawAnyStanza = true
		if skipStanza {
			return nil
		}
		item, ok, reason := dpkgStanzaToItem(string(raw), truncatedEOF)
		if !ok {
			malformed++
			losses.add(reason)
			return nil
		}
		if item == nil {
			return nil
		}
		usable++
		var overflow bool
		items, overflow = emitInventoryItem(items, *item, filter, losses)
		if overflow {
			itemBound = true
		}
		return nil
	}

	noteOversized := func() {
		losses.add(inventoryLossRecordOversized)
		skipStanza = true
		stanza = stanza[:0]
	}

	for {
		if err := ctx.Err(); err != nil {
			return inventoryParseResult{}, err
		}
		line, readErr := br.ReadSlice('\n')
		if readErr == bufio.ErrBufferFull {
			if dpkgLineIsAuthoritativePrefix(line) {
				noteOversized()
			}
			n, drainErr := drainLineCount(br)
			consumed += len(line) + n
			if consumed > stanzaLimit {
				noteOversized()
			}
			if drainErr == io.EOF {
				break
			}
			if drainErr != nil {
				if ctx.Err() != nil {
					return inventoryParseResult{}, ctx.Err()
				}
				losses.add(inventoryLossSourceMalformed)
				return inventoryParseResult{items: items, truncated: limited.truncated || itemBound, sourceFailed: true, losses: losses}, nil
			}
			continue
		}
		atEOF := false
		if readErr == io.EOF {
			atEOF = true
			readErr = nil
		} else if readErr != nil {
			if ctx.Err() != nil {
				return inventoryParseResult{}, ctx.Err()
			}
			losses.add(inventoryLossSourceUnavailable)
			return inventoryParseResult{items: items, truncated: limited.truncated || itemBound, sourceFailed: true, losses: losses}, nil
		}

		isBlank := len(bytes.TrimSpace(line)) == 0
		if isBlank {
			if flushErr := flush(stanza, false); flushErr != nil {
				return inventoryParseResult{}, flushErr
			}
			resetStanza()
			if atEOF {
				break
			}
			continue
		}

		if !skipStanza {
			consumed += len(line)
			if consumed > stanzaLimit {
				noteOversized()
			} else if shouldRetainDpkgLine(line) {
				stanza = append(stanza, line...)
			}
		}
		if atEOF {
			break
		}
	}

	if err := flush(stanza, limited.truncated); err != nil {
		return inventoryParseResult{}, err
	}

	truncated := limited.truncated || itemBound
	if limited.truncated {
		losses.add(inventoryLossSourceTruncated)
	}

	sourceFailed := false
	if limited.truncated && usable == 0 && malformed == 0 && !sawAnyStanza {
		sourceFailed = true
	}
	if malformed > 0 && usable == 0 && !limited.truncated {
		sourceFailed = true
		losses.add(inventoryLossSourceMalformed)
	}
	return inventoryParseResult{
		items:        items,
		truncated:    truncated,
		sourceFailed: sourceFailed,
		losses:       losses,
	}, nil
}

func drainLineCount(br *bufio.Reader) (int, error) {
	n := 0
	for {
		chunk, err := br.ReadSlice('\n')
		n += len(chunk)
		if err == bufio.ErrBufferFull {
			continue
		}
		return n, err
	}
}

func shouldRetainDpkgLine(line []byte) bool {
	trim := bytes.TrimRight(line, "\r\n")
	if len(trim) == 0 {
		return false
	}
	if trim[0] == ' ' || trim[0] == '\t' {
		return false
	}
	if dpkgLineIsAuthoritativePrefix(trim) {
		return true
	}
	// Retain non-field lines so malformed stanzas remain detectable.
	if idx := bytes.IndexByte(trim, ':'); idx <= 0 {
		return true
	}
	return false
}

func dpkgLineIsAuthoritativePrefix(line []byte) bool {
	trim := bytes.TrimLeft(line, " \t")
	for _, prefix := range []string{"Package:", "Status:", "Version:", "Architecture:", "Maintainer:"} {
		if bytes.HasPrefix(trim, []byte(prefix)) {
			return true
		}
	}
	return false
}

func dpkgStanzaToItem(raw string, truncatedEOF bool) (*SoftwareInventoryItem, bool, string) {
	fields, ok := parseDpkgStanza(raw)
	if !ok {
		return nil, false, inventoryLossRecordMalformed
	}
	if !fields.sawPackage || !fields.sawStatus {
		if truncatedEOF {
			return nil, false, inventoryLossSourceTruncated
		}
		return nil, false, inventoryLossRecordMalformed
	}
	if fields.Status != dpkgInstalledStatus {
		return nil, true, ""
	}
	if !fields.sawVersion || !fields.sawArch {
		return nil, false, inventoryLossRecordMalformed
	}
	if !isDPKGNativeSafePackageName(fields.Package) {
		return nil, false, inventoryLossIdentityInvalid
	}
	if !isDPKGNativeSafeArchitecture(fields.Architecture) {
		return nil, false, inventoryLossIdentityInvalid
	}
	if err := requireBoundedToken("product_name", fields.Package, maxSoftwareInventoryProductNameLen); err != nil {
		return nil, false, inventoryLossIdentityInvalid
	}
	if err := requireBoundedToken("version", fields.Version, maxSoftwareInventoryVersionLen); err != nil {
		return nil, false, inventoryLossIdentityInvalid
	}
	if err := requireBoundedToken("architecture", fields.Architecture, maxSoftwareInventoryArchitectureLen); err != nil {
		return nil, false, inventoryLossIdentityInvalid
	}

	native := formatDPKGNativePackageID(fields.Architecture, fields.Package, fields.Version)
	if err := requireBoundedToken("native_package_id", native, maxSoftwareInventoryNativePackageIDLen); err != nil {
		return nil, false, inventoryLossIdentityInvalid
	}

	vendor := ""
	if fields.Maintainer != "" {
		if optionalBoundedText("vendor_publisher", fields.Maintainer, maxSoftwareInventoryVendorLen) == nil {
			vendor = fields.Maintainer
		}
	}

	item := SoftwareInventoryItem{
		NativePackageID:                native,
		ProductName:                    fields.Package,
		Version:                        fields.Version,
		VendorPublisher:                vendor,
		Architecture:                   fields.Architecture,
		InstallationScope:              InstallationScopeMachine,
		AuthoritativeInstallOrUpdateAt: nil,
	}
	return &item, true, ""
}

func formatDPKGNativePackageID(arch, name, version string) string {
	return "dpkg:" + arch + ":" + name + "=" + version
}

// isDPKGNativeSafePackageName is the smallest Debian-native identity charset
// that keeps dpkg:<arch>:<name>=<version> unambiguous. It is not a policy parser.
func isDPKGNativeSafePackageName(name string) bool {
	if len(name) < 2 {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !isDPKGNativeAlnum(r) {
				return false
			}
			continue
		}
		if isDPKGNativeAlnum(r) || r == '+' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

// isDPKGNativeSafeArchitecture rejects architecture tokens that contain
// native-ID structural delimiters or characters outside dpkg architecture form.
func isDPKGNativeSafeArchitecture(arch string) bool {
	if arch == "" {
		return false
	}
	for i, r := range arch {
		if i == 0 {
			if !isDPKGNativeAlnum(r) {
				return false
			}
			continue
		}
		if isDPKGNativeAlnum(r) || r == '+' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func isDPKGNativeAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

func parseDpkgStanza(raw string) (dpkgFieldSet, bool) {
	var fields dpkgFieldSet
	var current string
	keep := false
	for _, line := range splitLines(raw) {
		if len(line) == 0 {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			// Continuations belong to the previous field; never stored unless keep.
			_ = keep
			_ = current
			continue
		}
		key, val, ok := splitDpkgField(line)
		if !ok {
			return dpkgFieldSet{}, false
		}
		current = key
		switch key {
		case "Package":
			if fields.sawPackage {
				return dpkgFieldSet{}, false
			}
			fields.Package = val
			fields.sawPackage = true
			keep = true
		case "Status":
			if fields.sawStatus {
				return dpkgFieldSet{}, false
			}
			fields.Status = val
			fields.sawStatus = true
			keep = true
		case "Version":
			if fields.sawVersion {
				return dpkgFieldSet{}, false
			}
			fields.Version = val
			fields.sawVersion = true
			keep = true
		case "Architecture":
			if fields.sawArch {
				return dpkgFieldSet{}, false
			}
			fields.Architecture = val
			fields.sawArch = true
			keep = true
		case "Maintainer":
			if fields.sawMaintainer {
				return dpkgFieldSet{}, false
			}
			fields.Maintainer = val
			fields.sawMaintainer = true
			keep = true
		default:
			keep = false
		}
	}
	return fields, true
}

func splitDpkgField(line string) (string, string, bool) {
	idx := strings.IndexByte(line, ':')
	if idx <= 0 {
		return "", "", false
	}
	key := line[:idx]
	val := strings.TrimSpace(line[idx+1:])
	if key == "" || strings.IndexFunc(key, func(r rune) bool { return r == ' ' || r == '\t' }) >= 0 {
		return "", "", false
	}
	return key, val, true
}

func splitLines(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.TrimSuffix(raw, "\n")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

type countingLimitReader struct {
	r         io.Reader
	n         int64
	limit     int64
	truncated bool
	peeked    bool
}

func (c *countingLimitReader) Read(p []byte) (int, error) {
	if c.n >= c.limit {
		if !c.peeked {
			c.peeked = true
			var one [1]byte
			n, err := c.r.Read(one[:])
			if n > 0 {
				c.truncated = true
			}
			if err != nil && err != io.EOF {
				return 0, err
			}
		}
		return 0, io.EOF
	}
	max := c.limit - c.n
	if int64(len(p)) > max {
		p = p[:max]
	}
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
