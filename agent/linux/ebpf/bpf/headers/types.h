/* SPDX-License-Identifier: MIT
 *
 * SYNTHETIC TEST-ONLY definitions (not host vmlinux.h, not Linux UAPI dump).
 * Retained for provenance contrast. process_net.bpf.c now includes generated
 * bpf/headers/vmlinux.h from /sys/kernel/btf/vmlinux instead of this file.
 */
#ifndef LIVE_GATE_B_TYPES_H
#define LIVE_GATE_B_TYPES_H

typedef unsigned char __u8;
typedef signed char __s8;
typedef unsigned short __u16;
typedef short __s16;
typedef unsigned int __u32;
typedef int __s32;
typedef unsigned long long __u64;
typedef long long __s64;

typedef __u16 __be16;
typedef __u32 __be32;
typedef __u64 __be64;
typedef __u16 __le16;
typedef __u32 __le32;
typedef __u64 __le64;
typedef __u32 __wsum;

#ifndef NULL
#define NULL ((void *)0)
#endif

/* Minimal uapi constants normally provided by linux/bpf.h / vmlinux.h. */
#define BPF_MAP_TYPE_HASH    1
#define BPF_MAP_TYPE_ARRAY   2
#define BPF_MAP_TYPE_RINGBUF 27

#define BPF_ANY     0
#define BPF_NOEXIST 1
#define BPF_EXIST   2

struct __sk_buff;
struct bpf_sock;
struct bpf_sock_ops;
struct bpf_sock_addr;
struct xdp_md;
struct sk_msg_md;
struct sk_reuseport_md;
struct bpf_fib_lookup;
struct bpf_redir_neigh;
struct bpf_perf_event_data;
struct bpf_perf_event_value;
struct bpf_pidns_info;
struct bpf_sysctl;
struct bpf_sockopt;
struct bpf_timer;
struct bpf_dynptr;
struct bpf_map;
struct bpf_spin_lock;
struct bpf_seq_file;
struct bpf_func_info;
struct bpf_line_info;
struct btf_ptr;
struct bpf_core_relo;
struct pt_regs;
struct task_struct;
struct file;
struct path;
struct inode;
struct socket;
struct sockaddr;
struct tcp_sock;
struct bpf_tcp_sock;
struct sk_buff;
struct net_device;
struct bpf_xdp_sock;
struct linux_binprm;
struct cgroup;
struct bpf_cgroup_storage_key;
struct bpf_local_storage;
struct bpf_local_storage_get;
struct bpf_rc_ctrl_key;
struct bpf_rc_keydown;
struct bpf_rc_pointer_rel;
struct bpf_flow_keys;
struct bpf_tunnel_key;
struct user_namespace;
struct bpf_cpumask;
struct bpf_wq;
struct kptr;
struct bpf_rb_root;
struct bpf_rb_node;
struct bpf_list_head;
struct bpf_list_node;
struct bpf_refcount;
struct bpf_arena;
struct bpf_arena_alloc;
struct bpf_arena_free;
struct bpf_qdisc;
struct bpf_skb_data_end;
struct bpf_ktls_ops;
struct mptcp_sock;
struct bpf_mptcp_sock;
struct bpf_xdp_sock_ops;

#endif /* LIVE_GATE_B_TYPES_H */
