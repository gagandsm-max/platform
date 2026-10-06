package collector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const rpmQueryTimeout = 15 * time.Second

// rpmQueryFormat is Platform-owned and never interpolates package/customer values.
const rpmQueryFormat = `%{NAME}\t%{EPOCH}\t%{VERSION}\t%{RELEASE}\t%{ARCH}\t%{VENDOR}\t%{INSTALLTIME}\n`

var rpmExecutableAllowlist = []string{
	"/usr/bin/rpm",
	"/bin/rpm",
}

var rpmFixedArgs = []string{"-qa", "--queryformat", rpmQueryFormat}

var rpmMutationTokens = []string{
	"install", "upgrade", "erase", "reinstall", "rebuilddb", "initdb",
	"import", "setperms", "setugids", "-e", "-i", "-U", "--erase",
	"--install", "--upgrade", "--reinstall", "--rebuilddb", "--initdb",
	"--import", "--setperms", "--setugids", "--scripts", "--triggers",
}

type rpmInvocation struct {
	Executable string
	Args       []string
}

type rpmCommandRunner func(ctx context.Context, inv rpmInvocation, stdout, stderr *boundedBuffer) error

func rpmInventoryArgv() []string {
	out := make([]string, len(rpmFixedArgs))
	copy(out, rpmFixedArgs)
	return out
}

func isAllowlistedRPMExecutable(path string) bool {
	if path == "" || strings.Contains(path, "\x00") {
		return false
	}
	if !strings.HasPrefix(path, "/") {
		return false
	}
	for _, allowed := range rpmExecutableAllowlist {
		if path == allowed {
			return true
		}
	}
	return false
}

func rpmArgsSafe(args []string) error {
	if len(args) != len(rpmFixedArgs) {
		return errors.New("RPM_ARGV_INVALID: argument count is not the Platform-fixed query")
	}
	for i := range args {
		if args[i] != rpmFixedArgs[i] {
			return errors.New("RPM_ARGV_INVALID: arguments are not the Platform-fixed query")
		}
	}
	joined := strings.Join(args, " ")
	lower := strings.ToLower(joined)
	if strings.Contains(lower, "sh -c") || strings.Contains(lower, "bash -c") {
		return errors.New("RPM_ARGV_INVALID: shell invocation is prohibited")
	}
	for _, tok := range rpmMutationTokens {
		for _, arg := range args {
			if arg == tok || strings.EqualFold(arg, tok) {
				return fmt.Errorf("RPM_ARGV_INVALID: mutation verb %q is prohibited", tok)
			}
		}
	}
	return nil
}

func resolveRPMExecutable() (string, error) {
	for _, path := range rpmExecutableAllowlist {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		mode := info.Mode()
		if mode&0o111 == 0 && mode&0o100 == 0 {
			// Still allow invoking a regular file at an allowlisted path; OS will fail if not executable.
		}
		return path, nil
	}
	return "", os.ErrNotExist
}

func runAllowlistedRPM(ctx context.Context, inv rpmInvocation, stdout, stderr *boundedBuffer) error {
	if !isAllowlistedRPMExecutable(inv.Executable) {
		return os.ErrNotExist
	}
	if err := rpmArgsSafe(inv.Args); err != nil {
		return err
	}
	if inv.Executable == "/bin/sh" || inv.Executable == "/usr/bin/sh" || inv.Executable == "/bin/bash" || inv.Executable == "/usr/bin/bash" {
		return errors.New("RPM_ARGV_INVALID: shell executable is prohibited")
	}
	cmd := exec.CommandContext(ctx, inv.Executable, inv.Args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = nil
	cmd.Env = []string{"LC_ALL=C", "LANG=C"}
	return cmd.Run()
}

func (c *LinuxSoftwareInventoryCollector) collectRPM(ctx context.Context, filter itemEmitFilter) (inventoryParseResult, error) {
	losses := newLossSet()
	if err := ctx.Err(); err != nil {
		return inventoryParseResult{}, err
	}

	exe, err := c.rpmResolver()
	if err != nil {
		if ctx.Err() != nil {
			return inventoryParseResult{}, ctx.Err()
		}
		return classifyOpenError(err, losses), nil
	}
	if !isAllowlistedRPMExecutable(exe) {
		losses.add(inventoryLossSourceUnavailable)
		return inventoryParseResult{sourceFailed: true, losses: losses}, nil
	}

	timeout := c.bounds.RPMQueryTimeout
	if timeout <= 0 {
		timeout = rpmQueryTimeout
	}
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdout := newBoundedBuffer(c.bounds.MaxRPMStdoutBytes)
	stderr := newBoundedBuffer(c.bounds.MaxRPMStderrBytes)
	inv := rpmInvocation{Executable: exe, Args: rpmInventoryArgv()}
	runErr := c.rpmRunner(queryCtx, inv, stdout, stderr)
	_ = stderr.Bytes() // drain only; raw stderr is never inventory payload

	if err := ctx.Err(); err != nil {
		return inventoryParseResult{}, err
	}

	internalTimeout := queryCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil
	if internalTimeout {
		losses.add(inventoryLossSourceTimeout)
	}

	if stdout.truncated {
		losses.add(inventoryLossSourceTruncated)
	}

	items, parseLosses, itemBound := parseRPMQueryOutput(stdout.Bytes(), filter)
	losses.merge(parseLosses)

	sourceFailed := false
	if runErr != nil && !internalTimeout {
		if ctx.Err() != nil {
			return inventoryParseResult{}, ctx.Err()
		}
		switch {
		case errors.Is(runErr, os.ErrPermission):
			losses.add(inventoryLossSourcePermissionDenied)
			sourceFailed = true
		case errors.Is(runErr, os.ErrNotExist):
			losses.add(inventoryLossSourceUnavailable)
			sourceFailed = true
		case errors.Is(runErr, exec.ErrNotFound):
			losses.add(inventoryLossSourceUnavailable)
			sourceFailed = true
		default:
			if len(items) == 0 {
				losses.add(inventoryLossSourceExecutionFailed)
				sourceFailed = true
			} else {
				losses.add(inventoryLossSourceExecutionFailed)
			}
		}
	}
	if internalTimeout && len(items) == 0 {
		sourceFailed = true
	}
	if stdout.truncated && len(items) == 0 {
		sourceFailed = true
	}
	if parseLosses != nil && parseLosses.has(inventoryLossSourceMalformed) && len(items) == 0 {
		sourceFailed = true
	}

	return inventoryParseResult{
		items:        items,
		truncated:    stdout.truncated || itemBound,
		sourceFailed: sourceFailed,
		losses:       losses,
	}, nil
}

func parseRPMQueryOutput(raw []byte, filter itemEmitFilter) ([]SoftwareInventoryItem, *lossSet, bool) {
	losses := newLossSet()
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, losses, false
	}
	lines := bytes.Split(raw, []byte("\n"))
	var items []SoftwareInventoryItem
	usable := 0
	malformed := 0
	itemBound := false
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		item, ok, reason := rpmLineToItem(string(line))
		if !ok {
			malformed++
			losses.add(reason)
			continue
		}
		usable++
		var overflow bool
		items, overflow = emitInventoryItem(items, item, filter, losses)
		if overflow {
			itemBound = true
		}
	}
	if usable == 0 && malformed > 0 {
		losses.add(inventoryLossSourceMalformed)
	}
	return items, losses, itemBound
}

func rpmLineToItem(line string) (SoftwareInventoryItem, bool, string) {
	line = strings.TrimSuffix(line, "\r")
	parts := strings.SplitN(line, "\t", 7)
	if len(parts) < 5 {
		return SoftwareInventoryItem{}, false, inventoryLossRecordMalformed
	}
	name := parts[0]
	epoch := parts[1]
	version := parts[2]
	release := parts[3]
	arch := parts[4]
	vendor := ""
	if len(parts) >= 6 {
		vendor = parts[5]
	}
	installRaw := ""
	if len(parts) >= 7 {
		installRaw = parts[6]
	}

	if hasControlRunes(name) || hasControlRunes(epoch) || hasControlRunes(version) || hasControlRunes(release) || hasControlRunes(arch) {
		return SoftwareInventoryItem{}, false, inventoryLossIdentityInvalid
	}

	name = strings.TrimSpace(name)
	epoch = strings.TrimSpace(epoch)
	version = strings.TrimSpace(version)
	release = strings.TrimSpace(release)
	arch = strings.TrimSpace(arch)
	vendor = strings.TrimSpace(vendor)
	installRaw = strings.TrimSpace(installRaw)

	if name == "" || version == "" || release == "" || arch == "" {
		return SoftwareInventoryItem{}, false, inventoryLossRecordMalformed
	}
	epochNorm, ok := normalizeRPMEpoch(epoch)
	if !ok {
		return SoftwareInventoryItem{}, false, inventoryLossRecordMalformed
	}
	if err := requireBoundedToken("product_name", name, maxSoftwareInventoryProductNameLen); err != nil {
		if strings.Contains(err.Error(), "BOUND_EXCEEDED") {
			return SoftwareInventoryItem{}, false, inventoryLossRecordOversized
		}
		return SoftwareInventoryItem{}, false, inventoryLossIdentityInvalid
	}
	if err := requireBoundedToken("version", version, maxSoftwareInventoryVersionLen); err != nil {
		if strings.Contains(err.Error(), "BOUND_EXCEEDED") {
			return SoftwareInventoryItem{}, false, inventoryLossRecordOversized
		}
		return SoftwareInventoryItem{}, false, inventoryLossIdentityInvalid
	}
	if err := requireBoundedToken("architecture", arch, maxSoftwareInventoryArchitectureLen); err != nil {
		if strings.Contains(err.Error(), "BOUND_EXCEEDED") {
			return SoftwareInventoryItem{}, false, inventoryLossRecordOversized
		}
		return SoftwareInventoryItem{}, false, inventoryLossIdentityInvalid
	}
	if err := requireBoundedToken("release", release, maxSoftwareInventoryVersionLen); err != nil {
		if strings.Contains(err.Error(), "BOUND_EXCEEDED") {
			return SoftwareInventoryItem{}, false, inventoryLossRecordOversized
		}
		return SoftwareInventoryItem{}, false, inventoryLossIdentityInvalid
	}

	native := formatRPMNativePackageID(arch, name, epochNorm, version, release)
	if err := requireBoundedToken("native_package_id", native, maxSoftwareInventoryNativePackageIDLen); err != nil {
		return SoftwareInventoryItem{}, false, inventoryLossIdentityInvalid
	}

	vendorOut := ""
	if vendor != "" && !isRPMNone(vendor) {
		if optionalBoundedText("vendor_publisher", vendor, maxSoftwareInventoryVendorLen) == nil {
			vendorOut = vendor
		}
	}

	var installAt *time.Time
	if ts, ok := parseRPMInstallTime(installRaw); ok {
		installAt = ts
	}

	return SoftwareInventoryItem{
		NativePackageID:                native,
		ProductName:                    name,
		Version:                        version,
		VendorPublisher:                vendorOut,
		Architecture:                   arch,
		InstallationScope:              InstallationScopeMachine,
		AuthoritativeInstallOrUpdateAt: installAt,
	}, true, ""
}

func formatRPMNativePackageID(arch, name, epoch, version, release string) string {
	return "rpm:" + arch + ":" + name + "=" + epoch + ":" + version + "-" + release
}

func normalizeRPMEpoch(epoch string) (string, bool) {
	if epoch == "" || isRPMNone(epoch) {
		return "0", true
	}
	n, err := strconv.ParseUint(epoch, 10, 32)
	if err != nil {
		return "", false
	}
	return strconv.FormatUint(n, 10), true
}

func isRPMNone(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "(none)", "none", "(null)", "null":
		return true
	default:
		return false
	}
}

func parseRPMInstallTime(raw string) (*time.Time, bool) {
	if raw == "" || isRPMNone(raw) {
		return nil, false
	}
	sec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || sec <= 0 {
		return nil, false
	}
	t := time.Unix(sec, 0).UTC()
	return &t, true
}

type boundedBuffer struct {
	max       int
	buf       []byte
	truncated bool
}

func newBoundedBuffer(max int) *boundedBuffer {
	if max <= 0 {
		max = maxRPMStdoutBytes
	}
	return &boundedBuffer{max: max, buf: make([]byte, 0, minInt(max, 4096))}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.truncated {
		return len(p), nil
	}
	remain := b.max - len(b.buf)
	if len(p) <= remain {
		b.buf = append(b.buf, p...)
		return len(p), nil
	}
	if remain > 0 {
		b.buf = append(b.buf, p[:remain]...)
	}
	b.truncated = true
	return len(p), nil
}

func (b *boundedBuffer) Bytes() []byte {
	return b.buf
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (l *lossSet) has(code string) bool {
	if l == nil {
		return false
	}
	_, ok := l.seen[code]
	return ok
}
