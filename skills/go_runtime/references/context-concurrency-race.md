# Go context, concurrency, race, fuzz, benchmark, pprof

## Context cancellation / deadline
- Propagate `context.Context` through RPC/I/O; honor cancel and deadline.
- On cancel, in-flight work must stop or be bounded—no orphan goroutines.

## Bounded concurrency / goroutine lifetime
- Worker pools/semaphores bound fan-out.
- Every goroutine has an owner and shutdown path.
- Channel send/receive ownership documented; no send-on-closed surprises.

## Race / fuzz / benchmark / pprof
- `go test -race` on concurrency-sensitive packages.
- Fuzz targets for parsers/codecs where inputs are untrusted.
- Benchmarks diagnose regressions; **benchmark ≠ production capacity certification**.
- pprof is diagnostic evidence under controlled load—not a substitute for capacity_performance certification.

## Integer overflow / saturation
- Prefer checked/saturating arithmetic for counters and byte sizes that can overflow.
- Document wrap behavior when saturation is intentional.
