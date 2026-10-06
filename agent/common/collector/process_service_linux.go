//go:build linux

package collector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Implementation safety bounds for Linux process/service source collection.
// These are not licensed/customer capacity claims.
const (
	maxProcDirents        = 8192
	maxProcFileBytes      = 64 * 1024
	maxProcCommBytes      = 256
	maxProcCgroupBytes    = 4096
	maxProcIOBytes        = 4096
	defaultSystemdTimeout = 3 * time.Second
	procPIDLeafStat       = "stat"
	procPIDLeafStatus     = "status"
	procPIDLeafCgroup     = "cgroup"
	procPIDLeafIO         = "io"
	procPIDLeafComm       = "comm"
	procPIDLeafExe        = "exe"
)

var allowedProcPIDLeaves = map[string]struct{}{
	procPIDLeafStat:   {},
	procPIDLeafStatus: {},
	procPIDLeafCgroup: {},
	procPIDLeafIO:     {},
	procPIDLeafComm:   {},
	procPIDLeafExe:    {},
}

type systemctlRunResult struct {
	stdout    []byte
	stderr    []byte
	truncated bool
	err       error
}

type systemctlRunner func(ctx context.Context, executable string, args []string) systemctlRunResult

// LinuxProcessServiceSource collects bounded /proc and read-only systemd evidence.
// It is not a HostCollector and does not enqueue scalar CollectionBatch data.
type LinuxProcessServiceSource struct {
	procRoot       string
	now            func() time.Time
	runner         systemctlRunner
	systemctlPath  string
	maxEntries     int
	maxDirents     int
	systemdTimeout time.Duration
	pageSize       uint64
}

func NewLinuxProcessServiceSource() *LinuxProcessServiceSource {
	return &LinuxProcessServiceSource{
		procRoot:       "/proc",
		now:            func() time.Time { return time.Now().UTC() },
		runner:         execSystemctl,
		systemctlPath:  detectSystemctlPath(),
		maxEntries:     maxProcessServiceCatalogueEntries,
		maxDirents:     maxProcDirents,
		systemdTimeout: defaultSystemdTimeout,
		pageSize:       uint64(os.Getpagesize()),
	}
}

func detectSystemctlPath() string {
	for _, p := range systemdAllowedExecutables {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func (s *LinuxProcessServiceSource) Collect(ctx context.Context) (ProcessServiceSourceSnapshot, error) {
	return s.CollectMatching(ctx, nil)
}

func (s *LinuxProcessServiceSource) CollectMatching(ctx context.Context, selectors []ProcessServiceSelector) (ProcessServiceSourceSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return ProcessServiceSourceSnapshot{}, err
	}
	now := s.now()
	procClass, entries, truncated, reason, err := s.collectProcesses(ctx, now)
	if err != nil {
		return ProcessServiceSourceSnapshot{}, err
	}
	quality := "OBSERVED"
	if procClass != ProcessServiceSourceObserved {
		quality = string(procClass)
	}
	if truncated && quality == "OBSERVED" {
		quality = "PARTIAL"
	}
	cat := ProcessServiceDiscoveryCatalogue{
		SchemaVersion:    ProcessServiceSchemaVersion,
		CatalogueID:      "linux-proc-systemd",
		CollectedAt:      now,
		SourceProvenance: "agent.linux.proc",
		Quality:          quality,
		Truncated:        truncated,
		IncompleteReason: reason,
		Entries:          entries,
	}
	if err := cat.Validate(); err != nil {
		return ProcessServiceSourceSnapshot{}, err
	}

	svcClass, services, svcTrunc, svcReason := s.collectSystemd(ctx)
	if svcTrunc {
		cat.Truncated = true
		if cat.IncompleteReason == "" {
			cat.IncompleteReason = svcReason
		} else if svcReason != "" && !strings.Contains(cat.IncompleteReason, svcReason) {
			merged := cat.IncompleteReason + "; " + svcReason
			if len(merged) <= maxProcessServiceIncompleteReasonLen {
				cat.IncompleteReason = merged
			}
		}
		if cat.Quality == "OBSERVED" {
			cat.Quality = "PARTIAL"
		}
	}

	snap := ProcessServiceSourceSnapshot{
		Catalogue:          cat,
		Services:           services,
		ProcessSourceClass: procClass,
		SystemdSourceClass: svcClass,
	}
	if len(selectors) > 0 {
		snap.Matches = MatchProcessServiceSelectors(cat.Entries, selectors)
		for i := range snap.Matches {
			if snap.Matches[i].Selector.Kind == SelectorKindServiceID {
				for j := range services {
					if services[j].UnitID == snap.Matches[i].Selector.Value {
						svc := services[j]
						snap.Matches[i].Service = &svc
						if !snap.Matches[i].Matched {
							snap.Matches[i].Matched = true
							snap.Matches[i].SourceClass = ProcessServiceSourceObserved
						}
						break
					}
				}
			}
		}
	}
	return snap, nil
}

func (s *LinuxProcessServiceSource) collectProcesses(ctx context.Context, now time.Time) (ProcessServiceSourceClass, []ProcessServiceCatalogueEntry, bool, string, error) {
	root := s.procRoot
	if root == "" {
		root = "/proc"
	}
	maxEntries := s.maxEntries
	if maxEntries <= 0 || maxEntries > maxProcessServiceCatalogueEntries {
		maxEntries = maxProcessServiceCatalogueEntries
	}
	maxDirents := s.maxDirents
	if maxDirents <= 0 {
		maxDirents = maxProcDirents
	}

	dir, err := os.Open(root)
	if err != nil {
		return classifyProcOpenError(err), nil, false, "", nil
	}
	defer dir.Close()

	var (
		entries   []ProcessServiceCatalogueEntry
		dirents   int
		truncated bool
		reason    string
		class     = ProcessServiceSourceObserved
	)
	for {
		if err := ctx.Err(); err != nil {
			return ProcessServiceSourceClass(""), nil, false, "", err
		}
		batch, readErr := dir.Readdirnames(128)
		if len(batch) == 0 && readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return classifyProcOpenError(readErr), entries, truncated, reason, nil
		}
		for _, name := range batch {
			dirents++
			if dirents > maxDirents {
				truncated = true
				reason = "proc directory scan bound reached"
				class = ProcessServiceSourceTruncated
				return class, entries, truncated, reason, nil
			}
			pid, ok := parseProcPIDName(name)
			if !ok {
				continue
			}
			if err := ctx.Err(); err != nil {
				return ProcessServiceSourceClass(""), nil, false, "", err
			}
			entry, skipped, skipClass := s.inspectPID(ctx, root, pid, now)
			_ = skipClass
			if skipped {
				if skipClass == ProcessServiceSourcePIDRace || skipClass == ProcessServiceSourcePermissionDenied || skipClass == ProcessServiceSourceMalformed {
					continue
				}
				continue
			}
			if len(entries) >= maxEntries {
				truncated = true
				reason = "process entry safety bound reached"
				class = ProcessServiceSourceTruncated
				return class, entries, truncated, reason, nil
			}
			entries = append(entries, entry)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return classifyProcOpenError(readErr), entries, truncated, reason, nil
		}
	}
	return class, entries, truncated, reason, nil
}

func parseProcPIDName(name string) (int, bool) {
	if name == "" {
		return 0, false
	}
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return 0, false
		}
	}
	pid, err := strconv.Atoi(name)
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func (s *LinuxProcessServiceSource) inspectPID(ctx context.Context, root string, pid int, now time.Time) (ProcessServiceCatalogueEntry, bool, ProcessServiceSourceClass) {
	statPath, err := procPIDPath(root, pid, procPIDLeafStat)
	if err != nil {
		return ProcessServiceCatalogueEntry{}, true, ProcessServiceSourceMalformed
	}
	statBytes, err := readBoundedFile(statPath, maxProcFileBytes)
	if err != nil {
		return ProcessServiceCatalogueEntry{}, true, classifyProcPIDError(err)
	}
	parsed, ok := parseProcStat(pid, string(statBytes))
	if !ok {
		return ProcessServiceCatalogueEntry{}, true, ProcessServiceSourceMalformed
	}

	name := parsed.comm
	if commBytes, err := readBoundedFileMustExist(root, pid, procPIDLeafComm, maxProcCommBytes); err == nil {
		n := strings.TrimSpace(strings.TrimSuffix(string(commBytes), "\n"))
		if n != "" && !hasControlRunes(n) && len(n) <= maxProcessServiceNameLen {
			name = n
		}
	}

	statusRSS, hasStatusRSS := readStatusRSS(root, pid)
	cgroupService := readCgroupService(root, pid)
	exePath, exeIdent := readExeIdentity(root, pid)

	if hasControlRunes(name) || len(name) > maxProcessServiceNameLen {
		name = ""
	}
	if name == "" && exePath == "" && exeIdent == "" && cgroupService == "" {
		return ProcessServiceCatalogueEntry{}, true, ProcessServiceSourceMalformed
	}

	rt := &ProcessServiceRuntimeInstance{
		PID:        pid,
		ObservedAt: now,
		State:      parsed.state,
	}
	if parsed.hasStart {
		rt.HasStartTime = true
		rt.StartTimeTicks = parsed.startTime
		rt.InstanceToken = strconv.Itoa(pid) + ":" + strconv.FormatUint(parsed.startTime, 10)
	}
	if parsed.hasCPU {
		rt.HasCPU = true
		rt.CPUUserTicks = parsed.utime
		rt.CPUSystemTicks = parsed.stime
	}
	if hasStatusRSS {
		rt.HasRSS = true
		rt.RSSBytes = statusRSS
	} else if parsed.hasRSS {
		if bytes, ok := rssPagesToBytes(parsed.rssPages, s.pageSize); ok {
			rt.HasRSS = true
			rt.RSSBytes = bytes
		}
	}
	if ioRead, ioWrite, ok := readProcIO(root, pid); ok {
		rt.HasIO = true
		rt.IOReadBytes = ioRead
		rt.IOWriteBytes = ioWrite
	}

	quality := "OBSERVED"
	if name != "" && exePath == "" && exeIdent == "" && cgroupService == "" {
		quality = "AMBIGUOUS_NAME_ONLY"
	}
	entry := ProcessServiceCatalogueEntry{
		ServiceID:          cgroupService,
		ExecutableIdentity: exeIdent,
		ExecutablePath:     exePath,
		ProcessFamilyName:  name,
		Runtime:            rt,
		SourceProvenance:   "proc",
		Quality:            quality,
	}
	if err := entry.validate(0); err != nil {
		return ProcessServiceCatalogueEntry{}, true, ProcessServiceSourceMalformed
	}
	return entry, false, ProcessServiceSourceObserved
}

type procStatFields struct {
	comm      string
	state     string
	utime     uint64
	stime     uint64
	startTime uint64
	rssPages  uint64
	hasCPU    bool
	hasStart  bool
	hasRSS    bool
}

func parseProcStat(expectedPID int, raw string) (procStatFields, bool) {
	if raw == "" || len(raw) > maxProcFileBytes {
		return procStatFields{}, false
	}
	if strings.IndexByte(raw, 0) >= 0 {
		return procStatFields{}, false
	}
	lparen := strings.IndexByte(raw, '(')
	rparen := strings.LastIndexByte(raw, ')')
	if lparen < 0 || rparen <= lparen+1 {
		return procStatFields{}, false
	}
	pidStr := strings.TrimSpace(raw[:lparen])
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid != expectedPID {
		return procStatFields{}, false
	}
	comm := raw[lparen+1 : rparen]
	if comm == "" || hasControlRunes(comm) || len(comm) > maxProcessServiceNameLen {
		return procStatFields{}, false
	}
	rest := strings.TrimSpace(raw[rparen+1:])
	fields := strings.Fields(rest)
	if len(fields) < 22 {
		return procStatFields{}, false
	}
	state := fields[0]
	if len(state) == 0 || len(state) > maxProcessServiceStateLen || hasControlRunes(state) {
		return procStatFields{}, false
	}
	out := procStatFields{comm: comm, state: state}
	if u, err := strconv.ParseUint(fields[11], 10, 64); err == nil {
		if s, err := strconv.ParseUint(fields[12], 10, 64); err == nil {
			out.utime = u
			out.stime = s
			out.hasCPU = true
		}
	}
	if st, err := strconv.ParseUint(fields[19], 10, 64); err == nil {
		out.startTime = st
		out.hasStart = true
	}
	if rss, err := strconv.ParseUint(fields[21], 10, 64); err == nil {
		out.rssPages = rss
		out.hasRSS = true
	}
	return out, true
}

func rssPagesToBytes(pages, pageSize uint64) (uint64, bool) {
	if pageSize == 0 {
		return 0, false
	}
	if pages > math.MaxUint64/pageSize {
		return 0, false
	}
	return pages * pageSize, true
}

func readStatusRSS(root string, pid int) (uint64, bool) {
	b, err := readBoundedFileMustExist(root, pid, procPIDLeafStatus, maxProcFileBytes)
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		if kb > math.MaxUint64/1024 {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}

func readCgroupService(root string, pid int) string {
	b, err := readBoundedFileMustExist(root, pid, procPIDLeafCgroup, maxProcCgroupBytes)
	if err != nil {
		return ""
	}
	return parseCgroupService(string(b))
}

func parseCgroupService(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		path := line
		if _, rest, ok := strings.Cut(line, ":"); ok {
			if _, rest2, ok2 := strings.Cut(rest, ":"); ok2 {
				path = rest2
			} else {
				path = rest
			}
		}
		path = strings.TrimSpace(path)
		if i := strings.IndexByte(path, ' '); i >= 0 {
			path = path[:i]
		}
		base := path
		if slash := strings.LastIndex(path, "/"); slash >= 0 {
			base = path[slash+1:]
		}
		if !strings.HasSuffix(base, ".service") {
			continue
		}
		if validateSystemdUnitID(base) != nil {
			continue
		}
		return base
	}
	return ""
}

func readExeIdentity(root string, pid int) (exePath, exeIdent string) {
	exe, err := procPIDPath(root, pid, procPIDLeafExe)
	if err != nil {
		return "", ""
	}
	link, err := os.Readlink(exe)
	if err != nil {
		return "", ""
	}
	link = strings.TrimSuffix(link, " (deleted)")
	link = strings.TrimSpace(link)
	if link != "" && !hasControlRunes(link) && len(link) <= maxProcessServiceIdentityLen {
		exePath = link
	}
	st, err := os.Stat(exe)
	if err != nil {
		return exePath, ""
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return exePath, ""
	}
	ident := fmt.Sprintf("linux-dev-ino:%d:%d:%d", sys.Dev, sys.Ino, st.Size())
	if len(ident) <= maxProcessServiceIdentityLen && !hasControlRunes(ident) {
		exeIdent = ident
	}
	return exePath, exeIdent
}

func readProcIO(root string, pid int) (readBytes, writeBytes uint64, ok bool) {
	b, err := readBoundedFileMustExist(root, pid, procPIDLeafIO, maxProcIOBytes)
	if err != nil {
		return 0, 0, false
	}
	var hasR, hasW bool
	for _, line := range strings.Split(string(b), "\n") {
		key, val, cutOK := strings.Cut(line, ":")
		if !cutOK {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		n, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "read_bytes":
			readBytes = n
			hasR = true
		case "write_bytes":
			writeBytes = n
			hasW = true
		}
	}
	return readBytes, writeBytes, hasR && hasW
}

func procPIDPath(root string, pid int, leaf string) (string, error) {
	if pid <= 0 {
		return "", errors.New("PROC_PID_INVALID")
	}
	if _, ok := allowedProcPIDLeaves[leaf]; !ok {
		return "", errors.New("PROC_LEAF_REJECTED")
	}
	if strings.Contains(leaf, "/") || strings.Contains(leaf, "..") {
		return "", errors.New("PROC_LEAF_REJECTED")
	}
	cleanRoot := filepath.Clean(root)
	p := filepath.Join(cleanRoot, strconv.Itoa(pid), leaf)
	rel, err := filepath.Rel(cleanRoot, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("PROC_PATH_TRAVERSAL_REJECTED")
	}
	return p, nil
}

func readBoundedFileMustExist(root string, pid int, leaf string, limit int) ([]byte, error) {
	p, err := procPIDPath(root, pid, leaf)
	if err != nil {
		return nil, err
	}
	return readBoundedFile(p, limit)
}

func readBoundedFile(path string, limit int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return data[:limit], errors.New("PROC_RECORD_OVERSIZED")
	}
	if bytes.IndexByte(data, 0) >= 0 && !strings.HasSuffix(path, "/"+procPIDLeafComm) {
		// stat/status/cgroup/io should be text; NUL is malformed except possibly comm.
		if !strings.HasSuffix(path, procPIDLeafComm) {
			return nil, errors.New("PROC_RECORD_MALFORMED")
		}
	}
	return data, nil
}

func classifyProcOpenError(err error) ProcessServiceSourceClass {
	if err == nil {
		return ProcessServiceSourceObserved
	}
	if errors.Is(err, os.ErrNotExist) {
		return ProcessServiceSourceUnavailable
	}
	if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return ProcessServiceSourcePermissionDenied
	}
	return ProcessServiceSourceUnavailable
}

func classifyProcPIDError(err error) ProcessServiceSourceClass {
	if err == nil {
		return ProcessServiceSourceObserved
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
		return ProcessServiceSourcePIDRace
	}
	if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return ProcessServiceSourcePermissionDenied
	}
	if err.Error() == "PROC_RECORD_OVERSIZED" || err.Error() == "PROC_RECORD_MALFORMED" {
		return ProcessServiceSourceMalformed
	}
	return ProcessServiceSourcePIDRace
}

func (s *LinuxProcessServiceSource) collectSystemd(ctx context.Context) (ProcessServiceSourceClass, []SystemdServiceObservation, bool, string) {
	if s.systemctlPath == "" || !systemctlExecutableAllowed(s.systemctlPath) {
		return ProcessServiceSourceUnavailable, nil, false, ""
	}
	timeout := s.systemdTimeout
	if timeout <= 0 {
		timeout = defaultSystemdTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	runner := s.runner
	if runner == nil {
		runner = execSystemctl
	}
	listRes := runner(runCtx, s.systemctlPath, systemdListUnitsArgv())
	if class, ok := classifySystemctlResult(runCtx, listRes); !ok {
		return class, nil, false, ""
	}
	units, listTrunc, err := parseSystemdListUnits(string(listRes.stdout))
	if err != nil {
		return ProcessServiceSourceMalformed, nil, false, ""
	}
	truncated := listTrunc || listRes.truncated
	reason := ""
	if truncated {
		reason = "systemd unit safety bound reached"
	}
	if len(units) == 0 {
		if truncated {
			return ProcessServiceSourceTruncated, nil, true, reason
		}
		return ProcessServiceSourceObserved, nil, false, ""
	}
	showArgs, err := systemdShowArgv(units)
	if err != nil {
		return ProcessServiceSourceMalformed, nil, truncated, reason
	}
	if err := runCtx.Err(); err != nil {
		return classifyCtxOrTimeout(runCtx), nil, truncated, reason
	}
	showRes := runner(runCtx, s.systemctlPath, showArgs)
	if class, ok := classifySystemctlResult(runCtx, showRes); !ok {
		return class, nil, truncated, reason
	}
	obs, err := parseSystemdShow(string(showRes.stdout))
	if err != nil {
		return ProcessServiceSourceMalformed, nil, truncated, reason
	}
	if showRes.truncated {
		truncated = true
		reason = "systemd stdout truncated"
	}
	class := ProcessServiceSourceObserved
	if truncated {
		class = ProcessServiceSourceTruncated
	}
	return class, obs, truncated, reason
}

func classifySystemctlResult(ctx context.Context, res systemctlRunResult) (ProcessServiceSourceClass, bool) {
	if res.err == nil {
		return ProcessServiceSourceObserved, true
	}
	if errors.Is(res.err, context.Canceled) {
		return ProcessServiceSourceUnavailable, false
	}
	if errors.Is(res.err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ProcessServiceSourceUnavailable, false
	}
	if errors.Is(res.err, os.ErrNotExist) || errors.Is(res.err, exec.ErrNotFound) {
		return ProcessServiceSourceUnavailable, false
	}
	if errors.Is(res.err, os.ErrPermission) || errors.Is(res.err, syscall.EACCES) || errors.Is(res.err, syscall.EPERM) {
		return ProcessServiceSourcePermissionDenied, false
	}
	var ee *exec.ExitError
	if errors.As(res.err, &ee) {
		// Missing units / systemd refusal is source quality, not target DOWN.
		return ProcessServiceSourceUnavailable, false
	}
	return ProcessServiceSourceUnavailable, false
}

func classifyCtxOrTimeout(ctx context.Context) ProcessServiceSourceClass {
	if errors.Is(ctx.Err(), context.Canceled) {
		return ProcessServiceSourceUnavailable
	}
	return ProcessServiceSourceUnavailable
}

func execSystemctl(ctx context.Context, executable string, args []string) systemctlRunResult {
	if !systemctlExecutableAllowed(executable) {
		return systemctlRunResult{err: os.ErrNotExist}
	}
	if err := assertSystemctlArgvSafe(args); err != nil {
		return systemctlRunResult{err: err}
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	var stdout, stderr limitedBuffer
	stdout.limit = maxSystemdStdoutBytes
	stderr.limit = maxSystemdStderrBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = nil
	err := cmd.Run()
	return systemctlRunResult{
		stdout:    stdout.buf,
		stderr:    stderr.buf,
		truncated: stdout.truncated || stderr.truncated,
		err:       err,
	}
}

type limitedBuffer struct {
	buf       []byte
	limit     int
	truncated bool
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.limit <= 0 {
		l.truncated = true
		return len(p), nil
	}
	remain := l.limit - len(l.buf)
	if remain <= 0 {
		l.truncated = true
		return len(p), nil
	}
	if len(p) > remain {
		l.buf = append(l.buf, p[:remain]...)
		l.truncated = true
		return len(p), nil
	}
	l.buf = append(l.buf, p...)
	return len(p), nil
}
