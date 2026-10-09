/* SPDX-License-Identifier: GPL-2.0-only WITH Linux-syscall-note
 *
 * Platform BUILD-011 Slice 7 — bounded Linux process-network eBPF object.
 * Governed product bytecode. No payload/TLS/HTTP/SQL/env/cmdline capture.
 * No XDP/tc/LSM/socket-filter/kprobe/uprobe. Classic tracepoints only.
 */
#define BPF_NO_PRESERVE_ACCESS_INDEX
#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_core_read.h"

char LICENSE[] SEC("license") = "GPL";

/* Deterministic engineering safety ceilings — not customer SKU sizing. */
#define STATE_MAX_ENTRIES 4096
#define RING_SIZE_BYTES   (1 * 1024 * 1024)

#define PROC_NET_ABI_V1   1
#define PROC_NET_COMM_LEN 16

#define EVT_INET_STATE 1
#define EVT_TCP_REXMIT 2

#define LOSS_MAP_UPDATE_FAIL   0
#define LOSS_RING_RESERVE_FAIL 1
#define LOSS_COUNTER_SLOTS     4

#define FLAG_OPTIONAL_PROBE 0x1

struct state_key {
	__u64 cookie;
};

struct state_val {
	__u64 last_ts_ns;
	__u32 hits;
	__u16 sport;
	__u16 dport;
};

/* Versioned fixed ABI v1 — userspace must validate abi_version + record_size. */
struct process_net_event_v1 {
	__u16 abi_version;
	__u16 record_size;
	__u8  event_type;
	__u8  protocol;
	__u16 family;
	__u64 ts_ns;
	__u32 tgid;
	__u32 pid;
	__u16 sport;
	__u16 dport;
	__s32 oldstate;
	__s32 newstate;
	__u8  saddr[16];
	__u8  daddr[16];
	char  comm[PROC_NET_COMM_LEN];
	__u32 flags;
};

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, STATE_MAX_ENTRIES);
	__type(key, struct state_key);
	__type(value, struct state_val);
} conn_state SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, RING_SIZE_BYTES);
} events SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, LOSS_COUNTER_SLOTS);
	__type(key, __u32);
	__type(value, __u64);
} loss_counters SEC(".maps");

static __always_inline void bump_loss(__u32 idx)
{
	__u64 *v = bpf_map_lookup_elem(&loss_counters, &idx);

	if (!v)
		return;
	if (*v < ~((__u64)0))
		(*v)++;
}

static __always_inline void emit_event(struct process_net_event_v1 *ev)
{
	struct process_net_event_v1 *dst;

	dst = bpf_ringbuf_reserve(&events, sizeof(*ev), 0);
	if (!dst) {
		bump_loss(LOSS_RING_RESERVE_FAIL);
		return;
	}
	*dst = *ev;
	bpf_ringbuf_submit(dst, 0);
}

static __always_inline void track_state(__u64 cookie, __u64 ts,
					__u16 sport, __u16 dport)
{
	struct state_key key = {.cookie = cookie};
	struct state_val *cur;
	struct state_val neu = {
		.last_ts_ns = ts,
		.hits = 1,
		.sport = sport,
		.dport = dport,
	};
	long err;

	if (cookie == 0)
		return;

	cur = bpf_map_lookup_elem(&conn_state, &key);
	if (cur) {
		cur->last_ts_ns = ts;
		if (cur->hits < 0xffffffff)
			cur->hits++;
		cur->sport = sport;
		cur->dport = dport;
		return;
	}

	err = bpf_map_update_elem(&conn_state, &key, &neu, BPF_NOEXIST);
	if (err)
		bump_loss(LOSS_MAP_UPDATE_FAIL);
}

static __always_inline __u16 core_read_sk_family(const void *skaddr)
{
	__u16 family = 0;

	if (!skaddr)
		return 0;
	family = BPF_CORE_READ((const struct sock *)skaddr, __sk_common.skc_family);
	return family;
}

struct inet_sock_set_state_ctx {
	__u64 pad;
	const void *skaddr;
	int oldstate;
	int newstate;
	__u16 sport;
	__u16 dport;
	__u16 family;
	__u16 protocol;
	__u8 saddr[4];
	__u8 daddr[4];
	__u8 saddr_v6[16];
	__u8 daddr_v6[16];
};

SEC("tracepoint/sock/inet_sock_set_state")
int handle_inet_sock_set_state(struct inet_sock_set_state_ctx *ctx)
{
	struct process_net_event_v1 ev = {};
	__u64 pid_tgid = bpf_get_current_pid_tgid();
	__u16 core_family;

	ev.abi_version = PROC_NET_ABI_V1;
	ev.record_size = sizeof(ev);
	ev.ts_ns = bpf_ktime_get_ns();
	ev.event_type = EVT_INET_STATE;
	ev.tgid = pid_tgid >> 32;
	ev.pid = (__u32)pid_tgid;
	ev.oldstate = ctx->oldstate;
	ev.newstate = ctx->newstate;
	ev.sport = ctx->sport;
	ev.dport = ctx->dport;
	ev.family = ctx->family;
	ev.protocol = (__u8)ctx->protocol;
	bpf_get_current_comm(&ev.comm, sizeof(ev.comm));

	core_family = core_read_sk_family(ctx->skaddr);
	if (core_family != 0)
		ev.family = core_family;

	if (ev.family == 2 /* AF_INET */) {
		__builtin_memcpy(ev.saddr, ctx->saddr, 4);
		__builtin_memcpy(ev.daddr, ctx->daddr, 4);
	} else if (ev.family == 10 /* AF_INET6 */) {
		__builtin_memcpy(ev.saddr, ctx->saddr_v6, 16);
		__builtin_memcpy(ev.daddr, ctx->daddr_v6, 16);
	}

	/* Classic TRACEPOINT cannot use get_socket_cookie on certified path. */
	track_state(0, ev.ts_ns, ev.sport, ev.dport);
	emit_event(&ev);
	return 0;
}

struct tcp_retransmit_skb_ctx {
	__u64 pad;
	const void *skbaddr;
	const void *skaddr;
	int state;
	__u16 sport;
	__u16 dport;
	__u16 family;
	__u8 saddr[4];
	__u8 daddr[4];
	__u8 saddr_v6[16];
	__u8 daddr_v6[16];
};

SEC("tracepoint/tcp/tcp_retransmit_skb")
int handle_tcp_retransmit_skb(struct tcp_retransmit_skb_ctx *ctx)
{
	struct process_net_event_v1 ev = {};
	__u64 pid_tgid = bpf_get_current_pid_tgid();
	__u16 core_family;

	ev.abi_version = PROC_NET_ABI_V1;
	ev.record_size = sizeof(ev);
	ev.ts_ns = bpf_ktime_get_ns();
	ev.event_type = EVT_TCP_REXMIT;
	ev.flags = FLAG_OPTIONAL_PROBE;
	ev.tgid = pid_tgid >> 32;
	ev.pid = (__u32)pid_tgid;
	ev.oldstate = ctx->state;
	ev.newstate = ctx->state;
	ev.sport = ctx->sport;
	ev.dport = ctx->dport;
	ev.family = ctx->family;
	ev.protocol = 6; /* IPPROTO_TCP */
	bpf_get_current_comm(&ev.comm, sizeof(ev.comm));

	core_family = core_read_sk_family(ctx->skaddr);
	if (core_family != 0)
		ev.family = core_family;

	if (ev.family == 2) {
		__builtin_memcpy(ev.saddr, ctx->saddr, 4);
		__builtin_memcpy(ev.daddr, ctx->daddr, 4);
	} else if (ev.family == 10) {
		__builtin_memcpy(ev.saddr, ctx->saddr_v6, 16);
		__builtin_memcpy(ev.daddr, ctx->daddr_v6, 16);
	}

	track_state(0, ev.ts_ns, ev.sport, ev.dport);
	emit_event(&ev);
	return 0;
}
