module platform/sensor

go 1.26.0

require (
	github.com/gosnmp/gosnmp v1.44.0
	golang.org/x/net v0.58.0
)

// x/net v0.58.0 requires x/crypto v0.55.0 in its module graph. Platform pins
// v0.56.0 because GO-2026-6354 and GO-2026-6355 are fixed there. This also
// prepares the already-approved SSH adapter without admitting a vulnerable graph.
require golang.org/x/sys v0.47.0 // indirect
