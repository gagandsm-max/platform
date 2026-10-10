package ebpf

import (
	"errors"
	"fmt"
	"os"
	"strings"

	cilium "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

type loadedPrimary struct {
	HandleInetSockSetState *cilium.Program `ebpf:"handle_inet_sock_set_state"`
	ConnState              *cilium.Map     `ebpf:"conn_state"`
	Events                 *cilium.Map     `ebpf:"events"`
	LossCounters           *cilium.Map     `ebpf:"loss_counters"`
}

type loadedOptional struct {
	HandleTCPRetransmit *cilium.Program `ebpf:"handle_tcp_retransmit_skb"`
}

type kernelHandles struct {
	primary   loadedPrimary
	optional  *loadedOptional
	links     []link.Link
	optLoaded bool
	optAttached bool
}

// CloseLinks detaches/breaks all probe links without releasing maps or programs.
// cilium/ebpf semantics: unpinned link.Close() breaks the attachment (stops new
// kernel production) while map/program FDs remain independently valid for drain
// and final loss-counter reads. Prefer Close over Detach: Detach may return
// ErrNotSupported on some link implementations, while Close is authoritative
// for unpinned links used by BUILD-011.
func (h *kernelHandles) CloseLinks() error {
	if h == nil {
		return nil
	}
	var first error
	for i := len(h.links) - 1; i >= 0; i-- {
		if err := h.links[i].Close(); err != nil && first == nil {
			first = err
		}
	}
	h.links = nil
	h.optAttached = false
	return first
}

func (h *kernelHandles) Close() {
	if h == nil {
		return
	}
	_ = h.CloseLinks()
	if h.optional != nil && h.optional.HandleTCPRetransmit != nil {
		_ = h.optional.HandleTCPRetransmit.Close()
		h.optional.HandleTCPRetransmit = nil
	}
	if h.primary.HandleInetSockSetState != nil {
		_ = h.primary.HandleInetSockSetState.Close()
		h.primary.HandleInetSockSetState = nil
	}
	if h.primary.ConnState != nil {
		_ = h.primary.ConnState.Close()
		h.primary.ConnState = nil
	}
	if h.primary.Events != nil {
		_ = h.primary.Events.Close()
		h.primary.Events = nil
	}
	if h.primary.LossCounters != nil {
		_ = h.primary.LossCounters.Close()
		h.primary.LossCounters = nil
	}
}

// acquireKernel loads and attaches with rollback after each step.
// No retry loop. No bpffs pins.
func acquireKernel(spec *cilium.CollectionSpec) (*kernelHandles, error, string) {
	if spec == nil {
		return nil, errors.New("LOADER_NIL_SPEC"), "object_integrity"
	}
	_ = rlimit.RemoveMemlock() // best-effort; never broadens capabilities.

	h := &kernelHandles{}
	rollback := func(err error, class string) (*kernelHandles, error, string) {
		h.Close()
		return nil, err, class
	}

	primarySpec := keepPrograms(spec, ProgInetSockSetState)
	if err := primarySpec.LoadAndAssign(&h.primary, nil); err != nil {
		return rollback(fmt.Errorf("primary load: %w", err), classifyLoadErr(err))
	}

	l, err := link.Tracepoint("sock", "inet_sock_set_state", h.primary.HandleInetSockSetState, nil)
	if err != nil {
		return rollback(fmt.Errorf("primary attach: %w", err), classifyAttachErr(err))
	}
	h.links = append(h.links, l)

	// Optional probe is independently degradable.
	if _, ok := spec.Programs[ProgTCPRetransmit]; ok {
		opt := &loadedOptional{}
		rexOnly := keepPrograms(spec, ProgTCPRetransmit)
		replacements := map[string]*cilium.Map{
			MapConnState:    h.primary.ConnState,
			MapEvents:       h.primary.Events,
			MapLossCounters: h.primary.LossCounters,
		}
		if err := rexOnly.LoadAndAssign(opt, &cilium.CollectionOptions{MapReplacements: replacements}); err != nil {
			// Primary remains ACTIVE; optional absent.
			h.optLoaded = false
		} else {
			h.optional = opt
			h.optLoaded = true
			ol, err := link.Tracepoint("tcp", "tcp_retransmit_skb", opt.HandleTCPRetransmit, nil)
			if err != nil {
				_ = opt.HandleTCPRetransmit.Close()
				h.optional = nil
				h.optLoaded = false
				h.optAttached = false
			} else {
				h.links = append(h.links, ol)
				h.optAttached = true
			}
		}
	}
	return h, nil, ""
}

func classifyLoadErr(err error) string {
	msg := err.Error()
	switch {
	case errors.Is(err, os.ErrPermission), containsAny(msg, "permission", "EPERM", "EACCES", "operation not permitted"):
		return "privilege"
	case containsAny(msg, "invalid", "Verifier", "verifier", "bytecode"):
		return "verifier"
	case containsAny(msg, "BTF", "CO-RE", "relocation", "btf"):
		return "btf"
	default:
		return "load"
	}
}

func classifyAttachErr(err error) string {
	msg := err.Error()
	switch {
	case errors.Is(err, os.ErrPermission), containsAny(msg, "permission", "EPERM", "EACCES", "operation not permitted"):
		return "privilege"
	case containsAny(msg, "no such file", "ENOENT", "cannot find", "not found"):
		return "tracepoint_unavailable"
	default:
		return "attach"
	}
}

func containsAny(s string, parts ...string) bool {
	for _, p := range parts {
		if p != "" && strings.Contains(s, p) {
			return true
		}
	}
	return false
}
