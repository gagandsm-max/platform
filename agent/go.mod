module platform/agent

go 1.27.0

require github.com/cilium/ebpf v0.22.0

require golang.org/x/sys v0.43.0 // indirect

replace github.com/cilium/ebpf => ./third_party/github.com/cilium/ebpf
