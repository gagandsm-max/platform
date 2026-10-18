#!/usr/bin/env bash
# Behavioral controls for the ACTUAL sourceable Test19 helper. Docker lifecycle
# responses are fixtures; HTTP completion/timeouts use real curl and TCP sockets.
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
python3 - "$ROOT_DIR" <<'PY'
import http.server, json, os, pathlib, shlex, shutil, signal, subprocess, sys, tempfile, threading, time
root = pathlib.Path(sys.argv[1])
helper = root / 'tests/run-all-tests.sh'
real_curl = shutil.which('curl')
assert real_curl and shutil.which('timeout'), 'curl and GNU timeout required'
passed = 0
with tempfile.TemporaryDirectory(prefix='test19-bounds-') as td:
    work = pathlib.Path(td)
    bindir = work / 'bin'; bindir.mkdir()
    docker = bindir / 'docker'
    docker.write_text(r'''#!/bin/bash
state=$(<"$T19_FIXTURE/state")
mode=${T19_MODE-normal}; op=$1; id=$(printf 'a%.0s' {1..64})
printf '["%s","%s"]\n' "$op" "${!#}" >> "$T19_FIXTURE/docker.log"
paused=false; [[ $state != *'"paused": true'* ]] || paused=true
running=true; [[ $state != *'"running": false'* ]] || running=false
unpaused=false; [[ $state != *'"unpaused": true'* ]] || unpaused=true
if [[ $op == inspect ]]; then
    [[ ${!#} == platform-kafka || ${!#} == "$id" ]] || exit 3
    [[ $unpaused != true || $mode != ambiguous-unpause ]] || exit 1
    [[ $unpaused != true || $mode != replaced ]] || id=$(printf 'b%.0s' {1..64})
    printf '%s %s %s\n' "$id" "$running" "$paused"
    exit 0
fi
[[ ${!#} == "$id" ]] || exit 3
if [[ $op == pause ]]; then
    [[ $mode != pause-rejected ]] || exit 1
    printf '{"paused": true, "running": %s}\n' "$running" > "$T19_FIXTURE/state"
    [[ $mode != pause-hang ]] || /bin/sleep 10
    [[ $mode != pause-error-after-effect ]] || exit 1
elif [[ $op == unpause ]]; then
    [[ $mode != unpause-failed ]] || exit 1
    [[ $mode != unpause-hang ]] || /bin/sleep 10
    printf '{"paused": false, "running": %s, "unpaused": true}\n' "$running" > "$T19_FIXTURE/state"
    [[ $mode != unpause-error-after-effect ]] || exit 1
else
    exit 3
fi
''')
    docker.chmod(0o755)
    # Log real curl argv and prove hostile curlrc/proxy settings are overridden.
    curl = bindir / 'curl'
    curl.write_text(r'''#!/bin/bash
printf '%q ' "$@" >> "$T19_FIXTURE/curl.log"
printf '\n' >> "$T19_FIXTURE/curl.log"
exec CURL_EXECUTABLE "$@"
'''.replace('CURL_EXECUTABLE',shlex.quote(real_curl)))
    curl.chmod(0o755)

    def case(name, mode='normal', down='complete', up='complete', expected=0, initially_paused=False,
             settings=None, interrupt=False, running=True):
        global passed
        path = work / name; path.mkdir()
        (path/'state').write_text(json.dumps({'paused':initially_paused,'running':running}))
        (path/'.curlrc').write_text('retry = 5\nproxy = "http://127.0.0.1:1"\nmax-time = 9\n')
        ready = threading.Event(); requests = []
        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self,*a): pass
            def do_GET(self):
                state=json.loads((path/'state').read_text()); phase='down' if state['paused'] else 'up'
                kind=down if phase=='down' else up; code=503 if phase=='down' else 200
                requests.append((phase,kind,time.monotonic()));ready.set()
                try:
                    if kind=='accepted-stall':time.sleep(2);return
                    self.send_response(418 if kind=='wrong' else code)
                    self.send_header('Content-Length','12' if kind in ('headers-stall','incomplete') else '2')
                    self.end_headers();self.wfile.write(b'OK');self.wfile.flush()
                    if kind=='headers-stall':time.sleep(2)
                    # incomplete: closes a short body, generating real curl exit18.
                except (BrokenPipeError,ConnectionResetError): pass
        server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Handler);server.daemon_threads=True
        thread=threading.Thread(target=server.serve_forever,kwargs={'poll_interval':.02},daemon=True);thread.start()
        env=dict(os.environ,PATH=str(bindir)+':'+os.environ['PATH'],T19_FIXTURE=str(path),T19_MODE=mode,
                 CURL_HOME=str(path),http_proxy='http://127.0.0.1:1',https_proxy='http://127.0.0.1:1',ALL_PROXY='http://127.0.0.1:1',
                 PLATFORM_TEST19_DOWN_MS='650',PLATFORM_TEST19_UP_MS='650',PLATFORM_TEST19_DOCKER_MS='250',
                 PLATFORM_TEST19_CLEANUP_MS='650',PLATFORM_TEST19_CONNECT_MS='80',PLATFORM_TEST19_REQUEST_MS='140',PLATFORM_TEST19_POLL_MS='20')
        env.update(settings or {})
        # exec inside a new session makes signal delivery to this owned fixture
        # deterministic. Neither process nor group belongs to the host workload.
        command=['bash','-c','source "$1"; trap : EXIT; before=$(trap -p); platform_test19_kafka_health "$2"; result=$?; [[ $(trap -p) == "$before" ]] || exit 99; exit "$result"','test19',str(helper),f'http://127.0.0.1:{server.server_port}/actuator/health']
        start=time.monotonic()
        proc=subprocess.Popen(command,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,start_new_session=True)
        try:
            if interrupt:
                assert ready.wait(2),name+': request readiness not reached'
                os.killpg(proc.pid,signal.SIGTERM)
            output=proc.communicate(timeout=4)[0].decode()
        except BaseException:
            os.killpg(proc.pid,signal.SIGKILL);proc.communicate();raise
        finally:server.shutdown();server.server_close()
        elapsed=time.monotonic()-start
        actions=[json.loads(line) for line in (path/'docker.log').read_text().splitlines()] if (path/'docker.log').exists() else []
        curls=[shlex.split(line) for line in (path/'curl.log').read_text().splitlines()] if (path/'curl.log').exists() else []
        state=json.loads((path/'state').read_text())
        print(f'CASE {name}: exit={proc.returncode} elapsed_monotonic={elapsed:.3f}s requests={len(requests)}',flush=True)
        print(output,end='')
        assert proc.returncode in (expected if isinstance(expected,tuple) else (expected,)),(name,proc.returncode,expected)
        assert elapsed<3.4,(name,'whole-fixture deadline exceeded',elapsed)
        if initially_paused or not running or settings and any(v in ('0','-1','1.5','','999999999999','abc') for v in settings.values()):
            assert not any(a[0] in ('pause','unpause') for a in actions),name
        else:
            assert all(a[-1]=='a'*64 for a in actions if a[0] in ('pause','unpause')),name
            if mode not in ('unpause-failed','unpause-hang'): assert state['paused'] is False,(name,state)
        for args in curls:
            assert args[0]=='-q' and args[args.index('--retry')+1]=='0',name
            assert args[args.index('--proxy')+1]=='' and args[args.index('--noproxy')+1]=='*',name
            assert '--max-time' in args and '--connect-timeout' in args and '--fail' not in args,name
            assert float(args[args.index('--max-time')+1])<=int(env['PLATFORM_TEST19_REQUEST_MS'])/1000,name
            assert float(args[args.index('--connect-timeout')+1])<=float(args[args.index('--max-time')+1]),name
        for phase,ceiling in [('down',8),('up',15)]:assert sum(p==phase for p,_,_ in requests)<=ceiling,name
        if name=='attempt-ceilings':
            assert sum(p=='down' for p,_,_ in requests)==8,name
            assert sum(p=='up' for p,_,_ in requests)==15,name
        if name=='phase-budget-exhaustion':assert elapsed<1.3,(name,elapsed)
        if name=='request-capped-by-phase':
            assert float(curls[0][curls[0].index('--max-time')+1])<.65,name
            assert all(float(a[a.index('--max-time')+1])<=.65 for a in curls),name
        if expected==0:
            assert [p for p,_,_ in requests]==['down','up'],name
            assert 'curl_exit=0 http=503' in output and 'curl_exit=0 http=200' in output,name
        if mode in ('ambiguous-unpause','replaced','unpause-failed','unpause-hang'):
            assert not any(p=='up' for p,_,_ in requests),name
            assert 'restoration FAILED/UNKNOWN' in output,name
        if down=='incomplete':assert 'curl_exit=18 http=503' in output,name
        if up=='incomplete':assert 'curl_exit=18 http=200' in output,name
        passed+=1

    # Source guard must precede .env/options/deployment work and retain traps.
    guard_env=dict(os.environ,PATH=str(bindir)+':'+os.environ['PATH'],T19_FIXTURE=str(work))
    guard=subprocess.run(['bash','-c', '''set +u; unset ROOT_DIR TOTAL_TESTS; trap ':' EXIT; before=$(trap -p); opts=$-; cwd=$PWD
source "$1"
[[ $(trap -p) == "$before" && $- == "$opts" && $PWD == "$cwd" ]] || exit 1
[[ ! -v ROOT_DIR && ! -v TOTAL_TESTS ]] || exit 1
declare -F platform_test19_kafka_health >/dev/null
''','source-guard',str(helper)],env=guard_env,capture_output=True,text=True,timeout=2)
    assert guard.returncode==0,guard.stderr
    assert not (work/'docker.log').exists(),'source invoked deployment'
    print('CASE side-effect-free source/options/traps: PASS');passed+=1
    case('completed-503-200')
    case('accepted-stalled-down',down='accepted-stall',expected=1)
    case('headers-incomplete-down',down='headers-stall',expected=1)
    case('closed-incomplete-down',down='incomplete',expected=1)
    case('accepted-stalled-up',up='accepted-stall',expected=1)
    case('headers-incomplete-up',up='headers-stall',expected=1)
    case('closed-incomplete-up',up='incomplete',expected=1)
    case('attempt-ceilings',down='wrong',up='wrong',expected=1,settings={'PLATFORM_TEST19_DOWN_MS':'2000','PLATFORM_TEST19_UP_MS':'2000','PLATFORM_TEST19_POLL_MS':'1'})
    case('phase-budget-exhaustion',down='wrong',expected=1,settings={'PLATFORM_TEST19_DOWN_MS':'330','PLATFORM_TEST19_REQUEST_MS':'140','PLATFORM_TEST19_POLL_MS':'200'})
    case('request-capped-by-phase',down='accepted-stall',expected=1,settings={'PLATFORM_TEST19_REQUEST_MS':'2000','PLATFORM_TEST19_CONNECT_MS':'3000'})
    case('pause-rejected',mode='pause-rejected',expected=1)
    case('pause-took-effect-error',mode='pause-error-after-effect',expected=1)
    case('pause-took-effect-timeout',mode='pause-hang',expected=1)
    case('unpause-rejected',mode='unpause-failed',expected=2)
    case('unpause-unknown',mode='ambiguous-unpause',expected=2)
    case('unpause-wrong-identity',mode='replaced',expected=2)
    case('unpause-hung',mode='unpause-hang',expected=2)
    case('unpause-error-but-restored',mode='unpause-error-after-effect')
    case('interrupt-while-paused',down='accepted-stall',expected=(143,-15),interrupt=True)
    case('initially-paused-untouched',initially_paused=True,expected=2)
    case('initially-stopped-untouched',running=False,expected=2)
    for value in ['0','-1','1.5','','999999999999','abc']:
        case('invalid-duration-'+(value or 'empty'),settings={'PLATFORM_TEST19_REQUEST_MS':value},expected=2)
    def main_abort_case(name, mode, paused=False):
        global passed
        path=work/name;path.mkdir();(path/'tests').mkdir();(path/'bin').mkdir()
        # Whole-file byte copy isolates .env/deployment side effects; the other
        # controls source the original helper directly. No line extraction.
        script=path/'tests/run-all-tests.sh';script.write_bytes(helper.read_bytes())
        (path/'.env').write_text('');(path/'state').write_text(json.dumps({'paused':paused}))
        marker=path/'test20-entered'
        wrappers={
            'bash': '#!/bin/bash\nif [[ $1 == deploy/docker/kafka/deployment-preflight.sh ]]; then [[ $2 != down ]] || touch "$T19_FIXTURE/test20-entered"; exit 0; fi\nexec /bin/bash "$@"\n',
            'make':'#!/bin/bash\nexit 0\n',
            'sleep':'#!/bin/bash\nexec /bin/sleep "$@"\n',
            'docker': '#!/bin/bash\nif [[ "$*" == *"INSERT INTO platform.tenant"* ]]; then touch "$T19_FIXTURE/test20-entered"; fi\nif [[ $1 == inspect && $* == *"--type container"* || $1 == pause || $1 == unpause ]]; then touch "$T19_FIXTURE/lifecycle"; exec "'+str(docker)+'" "$@"; fi\n[[ $1 != exec ]] || echo 1\nexit 0\n',
            'curl': '#!/bin/bash\nif [[ -e "$T19_FIXTURE/lifecycle" ]]; then args=("$@"); args[${#args[@]}-1]="$T19_URL"; exec "'+str(curl)+'" "${args[@]}"; fi\nif [[ "$*" == *count* ]]; then echo 1; elif [[ "$*" == *http_code* ]]; then echo 200; else echo \'{"status":"UP","product":"Platform"}\'; fi\n'
        }
        for cmd,body in wrappers.items():p=path/'bin'/cmd;p.write_text(body);p.chmod(0o755)
        class Complete(http.server.BaseHTTPRequestHandler):
            def log_message(self,*a):pass
            def do_GET(self):self.send_response(503);self.send_header('Content-Length','0');self.end_headers()
        server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Complete);server.daemon_threads=True
        threading.Thread(target=server.serve_forever,kwargs={'poll_interval':.02},daemon=True).start()
        env=dict(os.environ,PATH=str(path/'bin')+':'+str(bindir)+':'+os.environ['PATH'],T19_FIXTURE=str(path),T19_MODE=mode,
                 T19_URL=f'http://127.0.0.1:{server.server_port}/',PLATFORM_TEST19_DOWN_MS='700',PLATFORM_TEST19_UP_MS='700',
                 PLATFORM_TEST19_DOCKER_MS='250',PLATFORM_TEST19_CLEANUP_MS='600')
        start=time.monotonic()
        try:result=subprocess.run(['/bin/bash',str(script)],env=env,capture_output=True,text=True,timeout=5)
        finally:server.shutdown();server.server_close()
        assert result.returncode==1,(name,result.returncode,result.stdout,result.stderr)
        assert not marker.exists(),name+': entered Test20 before restoration proof'
        assert 'Aborting before Test20' in result.stderr,name
        assert 'Test 19:' in result.stdout and '[FAIL]' in result.stdout,name
        assert json.loads((path/'state').read_text())['paused']==(paused or mode=='unpause-failed'),name
        print(f'CASE {name}: PASS exit={result.returncode} elapsed_monotonic={time.monotonic()-start:.3f}s; Test19 failed, Test20 NOT_RUN')
        passed+=1

    main_abort_case('main-aborts-on-failed-restoration','unpause-failed')
    main_abort_case('main-aborts-on-ambiguous-restoration','ambiguous-unpause')
    main_abort_case('main-leaves-initially-paused-broker','normal',paused=True)
print(f'PASS: {passed} Test19 behavioral controls (mocked Docker, real curl; physical integration NOT_RUN)')
PY
