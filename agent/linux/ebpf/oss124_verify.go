package ebpf

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// DefaultOSS124ManifestName is the provenance sidecar manifest outside the
// imported upstream tree.
const DefaultOSS124ManifestName = "OSS-124-SOURCE-MANIFEST.sha256"

// VerifyOSS124Tree checks the governed local OSS-124 source tree against the
// canonical path-sorted regular-file manifest. Fail closed on missing,
// modified, or unexpected additional regular files. Symlinks are not
// integrity authorities. Timestamps are not integrity authorities.
func VerifyOSS124Tree(treeRoot, manifestPath string) error {
	expected, err := loadOSS124Manifest(manifestPath)
	if err != nil {
		return err
	}
	actual, err := hashOSS124RegularFiles(treeRoot)
	if err != nil {
		return err
	}
	var missing, modified, extra []string
	for path, digest := range expected {
		got, ok := actual[path]
		if !ok {
			missing = append(missing, path)
			continue
		}
		if got != digest {
			modified = append(modified, path)
		}
	}
	for path := range actual {
		if _, ok := expected[path]; !ok {
			extra = append(extra, path)
		}
	}
	sort.Strings(missing)
	sort.Strings(modified)
	sort.Strings(extra)
	if len(missing) > 0 || len(modified) > 0 || len(extra) > 0 {
		return fmt.Errorf("OSS124_SOURCE_MANIFEST_FAIL missing=%d modified=%d extra=%d sample_missing=%v sample_modified=%v sample_extra=%v",
			len(missing), len(modified), len(extra), trimSample(missing, 3), trimSample(modified, 3), trimSample(extra, 3))
	}
	return nil
}

// DefaultOSS124Paths resolves the in-repo governed replace tree and manifest
// from this source file location (agent/linux/ebpf → agent/third_party/...).
func DefaultOSS124Paths() (treeRoot, manifestPath string, err error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", "", fmt.Errorf("OSS124_PATH_RESOLVE_FAILED")
	}
	agentRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	treeRoot = filepath.Join(agentRoot, "third_party", "github.com", "cilium", "ebpf")
	manifestPath = filepath.Join(agentRoot, "third_party", DefaultOSS124ManifestName)
	return treeRoot, manifestPath, nil
}

func loadOSS124Manifest(path string) (map[string]string, error) {
	f, err := os.Open(path) // #nosec G304 -- governed path.
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := make(map[string]string)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	var prev string
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		digest, filePath, ok := strings.Cut(line, "  ")
		if !ok || len(digest) != 64 || !strings.HasPrefix(filePath, "./") {
			return nil, fmt.Errorf("OSS124_MANIFEST_LINE_INVALID: line %d", lineNo)
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return nil, fmt.Errorf("OSS124_MANIFEST_DIGEST_INVALID: line %d", lineNo)
		}
		if prev != "" && filePath <= prev {
			return nil, fmt.Errorf("OSS124_MANIFEST_ORDER_INVALID: %q after %q", filePath, prev)
		}
		if _, exists := out[filePath]; exists {
			return nil, fmt.Errorf("OSS124_MANIFEST_DUPLICATE: %s", filePath)
		}
		out[filePath] = digest
		prev = filePath
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("OSS124_MANIFEST_EMPTY")
	}
	return out, nil
}

func hashOSS124RegularFiles(root string) (map[string]string, error) {
	out := make(map[string]string)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = "./" + filepath.ToSlash(rel)
		sum, err := hashOSS124File(path)
		if err != nil {
			return err
		}
		out[rel] = sum
		return nil
	})
	return out, err
}

func hashOSS124File(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func trimSample(in []string, n int) []string {
	if len(in) <= n {
		return in
	}
	return in[:n]
}
