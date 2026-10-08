# OSS-124 temporary in-repo governed source provenance

**THIS IS NOT A GENERAL THIRD_PARTY ADMISSION MECHANISM.**

This sidecar records the OSS-124-specific temporary implementation seam authorized
for BUILD-011 Slice 7 (`IN_REPO_GOVERNED_REPLACE`). It does not authorize dumping
arbitrary third-party sources into the Platform tree.

## Identity

| Field | Value |
|---|---|
| OSS ID | OSS-124 |
| Component | `github.com/cilium/ebpf` |
| Engineering decision | USE/WRAP |
| Truth state before Slice 7 | APPROVED_NOT_IMPLEMENTED |
| Base | v0.22.0 |
| Base upstream commit | `e55144e17360b60cc4583229c35c2dbf0935b308` |
| Accepted upstream backports ONLY | `a204b5faca7fcadd2e5f835f47b02ecc66317aa6`, `4f576b5923e565087a46ed107683196fefb8fb23` |
| Derived archive SHA-256 | `50abc98cb507b16868419018909fd4da934b2089bfbea4dcd8d0875216d67a4a` |
| Canonical source manifest | `agent/third_party/OSS-124-SOURCE-MANIFEST.sha256` (byte-identical to audited `patched-source-tree.sha256`, SHA-256 `8a77e59c7bcfa1fd4e0e8f6e8f44798f0f519a848159573dad5065274c231ba2`) |
| Local replace path | `agent/third_party/github.com/cilium/ebpf/` |
| Evidence bundle reference | `/home/gagan/nms-oss124-patched-v0220-evidence` (independent OSS-124 patched v0.22.0 evidence closure) |
| Governing issue | #168 |
| Post-merge OSS-124 governance certification | Issue #168 comment 6061698697 |
| Dependency-integration master decision | Issue #168 comment 6061699760 (`A — IN_REPO_GOVERNED_REPLACE`) |

## Temporary derived-source status

The committed tree under `agent/third_party/github.com/cilium/ebpf/` is the exact
admitted derived source from the archive above (base v0.22.0 + the two accepted
upstream backports only). Platform does **not** modify files inside that upstream
tree. Upstream LICENSE, copyrights, notices, `go.mod`, and source are preserved.

Integrity authority is the path-sorted regular-file SHA-256 manifest
(`OSS-124-SOURCE-MANIFEST.sha256`), matching the audited evidence generator
(`find . -type f`). Symlinks present in the upstream archive are preserved but
are not integrity authorities. Timestamps are not integrity authorities.

## Migration condition

Replace this temporary local replace with the first acceptable stable upstream
release that contains equivalent fixes, after normal Platform OSS admission
(`Platform_07` + SBOM/licence/CVE review). Until that migration completes, Agent
builds MUST compile the governed local bytes via:

```text
require github.com/cilium/ebpf v0.22.0
replace github.com/cilium/ebpf => ./third_party/github.com/cilium/ebpf
```

Never treat pristine module-proxy `v0.22.0` as the build input while this seam is active.

## Non-goals

- Not a general third-party source dumping architecture.
- Not permission to vendor unrelated modules without catalogue admission.
- Not BUILD-018 packaging capability delivery.
- Not live kernel certification (Antigravity / later gates).
