module platform/sensor

go 1.26.0

require (
	github.com/gosnmp/gosnmp v1.44.0
	golang.org/x/net v0.58.0
)

// golang.org/x/sys is the active indirect dependency of golang.org/x/net for the
// packages Sensor currently imports. x/crypto, x/term, and x/text are not imported
// by this module's packages, so they are not pinned here. This does not select or
// freeze OSS-025 SSH.
require golang.org/x/sys v0.47.0 // indirect
