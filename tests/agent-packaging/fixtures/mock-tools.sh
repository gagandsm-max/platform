#!/bin/bash
# MOCKED package boundary tools for maintainer-script fixtures.
# Evidence produced with these tools must be labelled mocked — not native systemd proof.
set -euo pipefail

STATE_DIR="${PLATFORM_AGENT_MOCK_STATE:?PLATFORM_AGENT_MOCK_STATE required}"
mkdir -p "$STATE_DIR"
STATE_FILE="${STATE_DIR}/boundary-state.json"

if [[ ! -f "$STATE_FILE" ]]; then
  printf '%s\n' '{"calls":[],"groups":{},"users":{},"units":{},"fail":null,"getent_error":{},"suppress_user_readback":false,"suppress_group_readback":false}' > "$STATE_FILE"
fi

python3 - "$STATE_FILE" "$(basename "$0")" "$@" <<'PY'
import json, pathlib, sys
state_path = pathlib.Path(sys.argv[1])
name = sys.argv[2]
args = sys.argv[3:]
state = json.loads(state_path.read_text())
state.setdefault("calls", []).append([name, *args])
state.setdefault("getent_error", {})
state.setdefault("suppress_user_readback", False)
state.setdefault("suppress_group_readback", False)

fail = state.get("fail")
# fail may be ["systemctl","stop"] or ["getent","passwd"] matching tool + first arg.
if isinstance(fail, list) and len(fail) >= 1 and fail[0] == name:
    if len(fail) == 1 or (args and fail[1] == args[0]):
        state_path.write_text(json.dumps(state))
        sys.exit(int(state.get("fail_exit", 1)))

def save():
    state_path.write_text(json.dumps(state))

if name == "getent":
    kind, key = args[0], args[1]
    err_map = state.get("getent_error") or {}
    kind_map = err_map.get(kind) or {}
    if key in kind_map:
        save()
        sys.exit(int(kind_map[key]))
    if kind == "group" and state.get("suppress_group_readback") and key in state.get("groups", {}):
        # Simulate create succeeded but subsequent lookup cannot observe the record.
        table = dict(state.get("groups", {}))
        table.pop(key, None)
        save()
        sys.exit(2)
    if kind == "passwd" and state.get("suppress_user_readback") and key in state.get("users", {}):
        table = dict(state.get("users", {}))
        # Keep stored user for inspection but pretend getent cannot see it.
        save()
        sys.exit(2)
    table = state["groups" if kind == "group" else "users"]
    save()
    if key not in table:
        sys.exit(2)
    print(table[key])
elif name == "groupadd":
    gname = args[-1]
    if gname in state.get("groups", {}):
        save()
        sys.exit(9)
    gid = 32001 + len(state["groups"])
    state["groups"][gname] = f"{gname}:x:{gid}:"
    save()
elif name == "useradd":
    uname = args[-1]
    if uname in state.get("users", {}):
        save()
        sys.exit(9)
    gid_name = args[args.index("--gid") + 1]
    home = args[args.index("--home-dir") + 1]
    gid = state["groups"][gid_name].split(":")[2]
    # Allocate a non-root synthetic UID distinct from GID unless only one id exists.
    uid = 33001 + len(state["users"])
    state["users"][uname] = f"{uname}:x:{uid}:{gid}::{home}:/usr/sbin/nologin"
    save()
elif name == "systemctl":
    units = state.setdefault("units", {})
    if args[:1] == ["daemon-reload"]:
        save()
        sys.exit(0)
    if args[0] == "is-active":
        # support: systemctl is-active [--quiet] UNIT
        unit = args[-1]
        u = units.get(unit, {})
        # Explicit query-error must not be collapsed to inactive.
        if u.get("query_error"):
            save()
            sys.exit(int(u.get("query_error_exit", 1)))
        active = u.get("active", "inactive")
        quiet = "--quiet" in args
        save()
        if active == "active":
            if not quiet:
                print("active")
            sys.exit(0)
        # Established non-active states use systemd-like rc=3 with the state word.
        if not quiet:
            print(active)
        sys.exit(3)
    if args[0] in ("enable", "disable", "stop", "try-restart", "start"):
        unit = args[-1]
        u = units.setdefault(unit, {"active": "inactive", "enabled": "disabled"})
        if args[0] == "enable":
            u["enabled"] = "enabled"
        elif args[0] == "disable":
            u["enabled"] = "disabled"
        elif args[0] == "stop":
            u["active"] = "inactive"
        elif args[0] == "start":
            u["active"] = "active"
        elif args[0] == "try-restart":
            if u.get("active") == "active":
                u["active"] = "active"
        save()
        sys.exit(0)
    save()
    sys.exit(90)
else:
    save()
    sys.exit(99)
PY
