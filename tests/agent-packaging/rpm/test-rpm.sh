#!/bin/bash
# Separate-process source fixtures; full mode additionally requires a real build.
set -euo pipefail
exec python3 - "$@" <<'PY'
import hashlib
import json
import os
from pathlib import Path
import shutil
import struct
import subprocess
import sys
import tempfile

mode = sys.argv[1:] or ['--full']
if mode not in (['--source-only'], ['--full']):
    sys.exit('usage: test-rpm.sh [--source-only|--full]')
full = mode == ['--full']
if not os.environ.get('RPM_LANE_ADMISSION'):
    print('RPM_TEST_REFUSED: fresh shared-lane admission reference required', file=sys.stderr)
    sys.exit(78)
if full:
    for tool in ('rpm', 'rpmbuild', 'rpmspec', 'rpm2cpio', 'cpio'):
        if not shutil.which(tool):
            print('RPM_TEST_REFUSED: required RPM tool missing: ' + tool, file=sys.stderr)
            sys.exit(78)

source = Path.cwd()
base = 'agent/packaging/linux/'
recipe = [base + 'rpm/' + n for n in ('build-rpm.sh', 'platform-agent.spec.in', 'README.md')]
recipe += ['tests/agent-packaging/rpm/' + n for n in ('test-rpm.sh', 'compose.yaml')]
payload = [base + n for n in ('scripts/platform-agent-lifecycle.sh', 'package-layout.json', 'README.md')]
payload += ['agent/linux/systemd/' + n for n in ('platform-agent.service', 'platform-agent.env.example')]
payload += ['deploy/systemd/platform-agent.service', 'project_documentation/Platform_07_OSS_Catalog.md']
count = 0

def check(condition, label):
    global count
    if not condition:
        raise AssertionError(label)
    count += 1
    print('PASS ' + label, flush=True)

def call(args, **kwargs):
    return subprocess.run([str(x) for x in args], stdout=subprocess.PIPE, stderr=subprocess.PIPE, **kwargs)

def ok(args, **kwargs):
    r = call(args, **kwargs)
    if r.returncode:
        raise AssertionError(f'command exit={r.returncode}: {args}\n{r.stdout.decode()}\n{r.stderr.decode()}')
    return r.stdout

def elf_header(kind=2, machine=62, entry=0x1000, phoff=64, phentsize=56, phnum=1, ehsize=64, version=1):
    header = bytearray(64)
    header[0:7] = b'\x7fELF\x02\x01\x01'
    struct.pack_into('<HHIQQQIHHHHHH', header, 16, kind, machine, version, entry, phoff, 0, 0,
                     ehsize, phentsize, phnum, 0, 0, 0)
    return header

def pt_load(flags=1, offset=0, vaddr=0x1000, filesz=0x100, memsz=0x100):
    return struct.pack('<IIQQQQQQ', 1, flags, offset, vaddr, 0, filesz, memsz, 0)

payload.append('agent/linux/systemd/ebpf/20-ebpf.conf')

with tempfile.TemporaryDirectory(prefix='rpm-fixture-') as tmp:
    temp = Path(tmp)
    repo = temp / 'repo'
    repo.mkdir()
    # Explicit bounded regular-file inventory, not a copy of a developer checkout.
    for name in recipe + payload:
        src = source / name
        assert src.is_file() and not src.is_symlink(), name
        dst = repo / name
        dst.parent.mkdir(parents=True, exist_ok=True)
        dst.write_bytes(src.read_bytes())
    git_env = dict(os.environ, GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL='/dev/null',
                   GIT_AUTHOR_NAME='RPM fixture', GIT_AUTHOR_EMAIL='fixture@example.invalid',
                   GIT_COMMITTER_NAME='RPM fixture', GIT_COMMITTER_EMAIL='fixture@example.invalid')
    ok(['git', 'init', '-q', repo], env=git_env)
    ok(['git', 'add', '--', *recipe, *payload], cwd=repo, env=git_env)
    ok(['git', '-c', 'core.hooksPath=/dev/null', 'commit', '-qm', 'isolated source fixture'], cwd=repo, env=git_env)
    commit = ok(['git', 'rev-parse', 'HEAD'], cwd=repo).decode().strip()
    binary = temp / 'fixture-binary'
    binary.write_bytes(b'not-a-production-binary\n')
    binary_hash = hashlib.sha256(binary.read_bytes()).hexdigest()
    out = temp / 'out'
    out.mkdir()
    args = ['bash', repo / (base + 'rpm/build-rpm.sh'), '--source-only', '--fixture-inputs',
            '--source-commit', commit, '--recipe-commit', commit, '--binary', binary,
            '--binary-sha256', binary_hash, '--builder-image', os.environ['RPM_TEST_IMAGE'],
            '--version', '0.0.0', '--release', '1.fixture', '--architecture', 'x86_64', '--out', out]
    generated = out / 'platform-agent-0.0.0-1.fixture.x86_64'
    def change(flag, value, original=args):
        result = list(original)
        result[result.index(flag) + 1] = value
        return result
    ok(args, cwd=repo)
    scripts = generated / 'scriptlets'
    helper = (repo / (base + 'scripts/platform-agent-lifecycle.sh')).read_bytes()
    for slot in ('pre', 'post', 'preun', 'postun'):
        data = (scripts / slot).read_bytes()
        check(data.startswith(helper[:-len(b'main "$@"\n')]), 'exact embedded helper prefix ' + slot)
        ok(['bash', '-n', scripts / slot])
    second = temp / 'second'
    second.mkdir()
    ok(change('--out', second), cwd=repo)
    second_gen = second / generated.name
    check((generated / 'SPECS/platform-agent.spec').read_bytes() == (second_gen / 'SPECS/platform-agent.spec').read_bytes(), 'deterministic spec generation')
    for slot in ('pre', 'post', 'preun', 'postun'):
        check((scripts / slot).read_bytes() == (second_gen / 'scriptlets' / slot).read_bytes(), 'deterministic scriptlet ' + slot)
    spec = (generated / 'SPECS/platform-agent.spec').read_text()
    for slot, following in [('pre', 'post'), ('post', 'preun'), ('preun', 'postun'), ('postun', 'files')]:
        body = spec.split('%' + slot + ' -p /bin/bash\n', 1)[1].split('%' + following, 1)[0]
        check(body == (scripts / slot).read_text().replace('%', '%%') + '\n', 'literal percent escaping ' + slot)
    stage = generated / 'SOURCES/payload'
    check(not (stage / 'etc').exists() and not (stage / 'var').exists(), 'no customer config credentials identity or spool payload ownership')
    check((stage / 'opt/platform/agent/platform-agent-linux').read_bytes() == binary.read_bytes(), 'retained binary stage bytes')
    check((stage / 'usr/lib/platform/agent/packaging/platform-agent-lifecycle.sh').read_bytes() == helper, 'retained helper stage bytes')
    check((stage / 'usr/lib/systemd/system/platform-agent.service').read_bytes() == (repo / payload[3]).read_bytes(), 'retained unit stage bytes')
    check('%global __spec_install_post %{nil}' in spec and '%global __os_install_post %{nil}' in spec and '%global debug_package %{nil}' in spec, 'post-processing disabled in source (RPM execution separate)')
    check(call(args, cwd=repo).returncode == 78, 'output collision rejected')
    for flag, value in [('--version', '1%{lua:bad}'), ('--version', '../1'), ('--version', '1-2'),
                        ('--release', '0'), ('--release', '1\nName: injected'), ('--release', '../2'),
                        ('--architecture', 'amd64'), ('--architecture', 'aarch64'),
                        ('--source-commit', 'a' * 39), ('--source-date-epoch', '-1'),
                        ('--binary-sha256', '0' * 64), ('--builder-image', 'latest'),
                        ('--out', repo), ('--out', repo.parent)]:
        negative = change('--out', second)
        if flag == '--source-date-epoch':
            negative += [flag, value]
        else:
            negative = change(flag, value, negative)
        r = call(negative, cwd=repo)
        check(r.returncode == 78 and b'BUILD_REFUSED' in r.stderr, 'invalid input refused ' + flag + '=' + repr(str(value)))
    alias = temp / 'alias'
    alias.symlink_to(out, target_is_directory=True)
    check(call(change('--out', alias), cwd=repo).returncode == 78, 'symlink output refused')
    linkbin = temp / 'binary-link'
    linkbin.symlink_to(binary)
    check(call(change('--binary', linkbin), cwd=repo).returncode == 78, 'symlink binary refused')
    before = (repo / payload[0]).read_bytes()
    (repo / payload[0]).write_bytes(before + b'# changed\n')
    r = call(change('--out', second), cwd=repo)
    check(r.returncode == 78 and b'payload source mismatch' in r.stderr, 'altered helper source refused')
    (repo / payload[0]).write_bytes(before)
    recipe_path = repo / recipe[0]
    recipe_bytes = recipe_path.read_bytes()
    recipe_path.write_bytes(recipe_bytes + b'# drift\n')
    r = call(change('--out', second), cwd=repo)
    check(r.returncode == 78 and b'recipe source mismatch' in r.stderr, 'recipe commit drift refused')
    recipe_path.write_bytes(recipe_bytes)
    r = call([x for x in args if x != '--source-only'], cwd=repo)
    check(r.returncode == 78 and b'fixture inputs are source-only' in r.stderr, 'fixture inputs cannot build an RPM')
    prod = [x for x in args if x != '--fixture-inputs']
    r = call(prod, cwd=repo)
    check(r.returncode == 78 and b'ELF64 little-endian executable' in r.stderr, 'synthetic binary refused outside fixture mode')

    # Master-reproduced defect: 20-byte ELF magic/machine prefix + matching SHA passed
    # the legacy weak check. Failing-before / fixed-after regression (source-only).
    truncated = b'\x7fELF\x02\x01' + bytes(12) + b'\x3e\x00'
    check(len(truncated) == 20 and truncated[:5] == b'\x7fELF\x02' and truncated[5:6] == b'\x01'
          and truncated[18:20] == b'\x3e\x00', 'legacy weak ELF magic/machine check would accept truncated prefix')
    print('LEGACY_WEAK_ELF_CHECK_WOULD_PASS truncated_20', flush=True)
    trunc_path = temp / 'truncated-elf'
    trunc_path.write_bytes(truncated)
    trunc_hash = hashlib.sha256(truncated).hexdigest()
    trunc_out = temp / 'trunc-out'
    trunc_out.mkdir()
    trunc_args = change('--out', trunc_out, change('--binary-sha256', trunc_hash, change('--binary', trunc_path, prod)))
    r = call(trunc_args, cwd=repo)
    check(r.returncode == 78 and b'ELF64 little-endian executable' in r.stderr
          and b'SOURCE_ONLY_GENERATED=' not in r.stdout, 'truncated ELF refused after integrity fix')

    malformed_headers = bytes(elf_header(phnum=2)) + pt_load()
    check(64 + 2 * 56 > len(malformed_headers), 'malformed program-header bound precondition')
    bad_ph_path = temp / 'bad-ph'
    bad_ph_path.write_bytes(malformed_headers)
    bad_ph_out = temp / 'bad-ph-out'
    bad_ph_out.mkdir()
    r = call(change('--out', bad_ph_out, change('--binary-sha256', hashlib.sha256(malformed_headers).hexdigest(),
                                                 change('--binary', bad_ph_path, prod))), cwd=repo)
    check(r.returncode == 78 and b'valid program headers' in r.stderr, 'malformed program headers refused')

    # Bounds-valid load so refusal remains entry-backing (not a bounds class).
    unbound_entry = bytes(elf_header(entry=0x400000)) + pt_load(vaddr=0x1000, filesz=120, memsz=120)
    bad_entry_path = temp / 'bad-entry'
    bad_entry_path.write_bytes(unbound_entry)
    bad_entry_out = temp / 'bad-entry-out'
    bad_entry_out.mkdir()
    r = call(change('--out', bad_entry_out, change('--binary-sha256', hashlib.sha256(unbound_entry).hexdigest(),
                                                   change('--binary', bad_entry_path, prod))), cwd=repo)
    check(r.returncode == 78 and b'entry point is not backed' in r.stderr
          and not (bad_entry_out / generated.name).exists(), 'unbacked ELF entry refused')

    arch_mismatch = bytes(elf_header(machine=183)) + pt_load()
    arch_path = temp / 'arch-mismatch'
    arch_path.write_bytes(arch_mismatch)
    arch_out = temp / 'arch-out'
    arch_out.mkdir()
    r = call(change('--out', arch_out, change('--binary-sha256', hashlib.sha256(arch_mismatch).hexdigest(),
                                              change('--binary', arch_path, prod))), cwd=repo)
    check(r.returncode == 78 and b'machine mismatch for x86_64' in r.stderr, 'architecture mismatch refused')

    # Six malformed two-segment PT_LOAD controls: three bounds classes × before/after
    # a valid executable entry-backing load. Source-only nonfixture; exit 78; no stage.
    two_payload = b'\x90' * 16
    two_body = 64 + 2 * 56
    two_size = two_body + len(two_payload)
    valid_entry_load = pt_load(flags=5, offset=two_body, vaddr=0x1000, filesz=16, memsz=16)
    malformed_classes = [
        ('filesz-gt-memsz', pt_load(flags=4, offset=0, vaddr=0x3000, filesz=8, memsz=4)),
        ('offset-gt-size', pt_load(flags=4, offset=two_size + 1, vaddr=0x3000, filesz=1, memsz=1)),
        ('filesz-gt-extent', pt_load(flags=4, offset=two_size - 8, vaddr=0x3000, filesz=64, memsz=64)),
    ]
    for class_name, bad_load in malformed_classes:
        for position, segments in (('before', (bad_load, valid_entry_load)),
                                   ('after', (valid_entry_load, bad_load))):
            blob = bytes(elf_header(entry=0x1000, phnum=2)) + segments[0] + segments[1] + two_payload
            check(len(blob) == two_size, 'two-segment size precondition ' + class_name + '-' + position)
            case_path = temp / ('bad-pt-load-' + class_name + '-' + position)
            case_path.write_bytes(blob)
            case_out = temp / ('bad-pt-load-out-' + class_name + '-' + position)
            case_out.mkdir()
            r = call(change('--out', case_out, change('--binary-sha256', hashlib.sha256(blob).hexdigest(),
                                                      change('--binary', case_path, prod))), cwd=repo)
            check(r.returncode == 78 and b'load segment is bounds-invalid' in r.stderr
                  and b'SOURCE_ONLY_GENERATED=' not in r.stdout
                  and not (case_out / generated.name).exists(),
                  'malformed PT_LOAD ' + class_name + ' ' + position + ' refused without staging')

    # Positive synthetic two-PT_LOAD retained control (both loads in-bounds; entry backed).
    valid_two = (bytes(elf_header(entry=0x1000, phnum=2))
                 + valid_entry_load
                 + pt_load(flags=4, offset=two_body, vaddr=0x2000, filesz=16, memsz=16)
                 + two_payload)
    valid_two_path = temp / 'valid-two-pt-load'
    valid_two_path.write_bytes(valid_two)
    valid_two_hash = hashlib.sha256(valid_two).hexdigest()
    valid_two_out = temp / 'valid-two-pt-load-out'
    valid_two_out.mkdir()
    ok(change('--out', valid_two_out, change('--binary-sha256', valid_two_hash,
                                             change('--binary', valid_two_path, prod))), cwd=repo)
    valid_two_stage = valid_two_out / generated.name / 'SOURCES/payload/opt/platform/agent/platform-agent-linux'
    check(valid_two_stage.read_bytes() == valid_two, 'retained valid two-PT_LOAD source-only stage bytes')
    print('VALID_TWO_PT_LOAD_SOURCE_ONLY_CONTROL_PASS sha256=' + valid_two_hash, flush=True)

    # Positive retained real ELF source-only control (never full RPM; never fixture-inputs).
    host_elf = Path('/bin/true')
    check(host_elf.is_file(), 'host /bin/true available for retained ELF control')
    real_elf = temp / 'retained-real-elf'
    real_elf.write_bytes(host_elf.read_bytes())
    real_hash = hashlib.sha256(real_elf.read_bytes()).hexdigest()
    real_out = temp / 'real-elf-out'
    real_out.mkdir()
    real_args = change('--out', real_out, change('--binary-sha256', real_hash, change('--binary', real_elf, prod)))
    ok(real_args, cwd=repo)
    real_stage = real_out / generated.name / 'SOURCES/payload/opt/platform/agent/platform-agent-linux'
    check(real_stage.read_bytes() == real_elf.read_bytes(), 'retained real ELF source-only stage bytes')
    print('REAL_ELF_SOURCE_ONLY_CONTROL_PASS sha256=' + real_hash, flush=True)

    toolpath = temp / 'no-rpm-tools'
    toolpath.mkdir()
    for name in ('python3', 'git', 'bash', 'cp'):
        (toolpath / name).symlink_to(shutil.which(name))
    no_rpm_env = dict(os.environ, PATH=str(toolpath))
    r = call([x for x in prod if x != '--source-only'], cwd=repo, env=no_rpm_env)
    check(r.returncode == 78 and b'required tool missing: rpm' in r.stderr, 'default builder fails when RPM tools absent')
    r = call(['/bin/bash', source / 'tests/agent-packaging/rpm/test-rpm.sh'], env=no_rpm_env)
    check(r.returncode == 78 and b'required RPM tool missing: rpm' in r.stderr, 'default test mode fails when RPM tools absent')

    mock_source = source / 'tests/agent-packaging/fixtures/mock-tools.sh'
    sequence = 0
    def fixture():
        global sequence
        sequence += 1
        home = temp / ('case-' + str(sequence))
        root = home / 'root'
        tools = home / 'tools'
        state = home / 'state'
        for directory in (root, tools, state):
            directory.mkdir(parents=True)
        for name in ('getent', 'groupadd', 'useradd', 'systemctl'):
            target = tools / name
            target.write_bytes(mock_source.read_bytes())
            target.chmod(0o755)
        initial = {'calls': [], 'groups': {}, 'users': {}, 'units': {}}
        env = dict(os.environ, PLATFORM_AGENT_ROOT=str(root), PLATFORM_AGENT_PACKAGE_TOOLS=str(tools), PLATFORM_AGENT_MOCK_STATE=str(state))
        statefile = state / 'boundary-state.json'
        statefile.write_text(json.dumps(initial))
        return root, statefile, env

    def invoke(scriptdir, slot, argv, env):
        # Never source the helper, call inside a shell conditional, or mask errexit.
        return call(['/bin/bash', scriptdir / slot, *argv], env=env)

    def seed_env(root, text='fixture secret reference\n'):
        config = root / 'etc/platform/agent.env'
        config.parent.mkdir(parents=True, exist_ok=True)
        config.write_text(text)
        config.chmod(0o600)
        return config

    def no_activation(calls):
        return not any(c[:2] in (['systemctl', 'enable'], ['systemctl', 'start'], ['systemctl', 'unmask']) for c in calls)

    def suite(scriptdir, label):
        for slot in ('pre', 'post', 'preun', 'postun'):
            for invalid in ([], [''], ['-1'], ['+1'], ['01'], ['1.0'], ['1 '], ['x'], ['1', '2']):
                root, sf, env = fixture()
                r = invoke(scriptdir, slot, invalid, env)
                check(r.returncode == 78 and b'RPM_COUNT_INVALID' in r.stderr and not json.loads(sf.read_text())['calls'], label + ' invalid count ' + slot + repr(invalid))
            if slot in ('pre', 'post'):
                root, sf, env = fixture()
                r = invoke(scriptdir, slot, ['0'], env)
                check(r.returncode == 78 and b'RPM_COUNT_INVALID' in r.stderr, label + ' install zero refused ' + slot)
        for slot, n, marker in [('pre', '1', 'PREINST_OK action=install'), ('pre', '2', 'PREINST_OK action=upgrade'),
                                ('pre', '999999999999999999999999', 'PREINST_OK action=upgrade'),
                                ('post', '1', 'POSTINST_OK action=configure'), ('post', '2', 'POSTINST_OK action=configure'),
                                ('preun', '0', 'PRERM_OK action=remove'), ('preun', '1', 'PRERM_OK action=upgrade'),
                                ('preun', '2', 'PRERM_OK action=upgrade'), ('postun', '0', 'POSTRM_OK action=remove'),
                                ('postun', '1', 'POSTRM_OK action=upgrade'), ('postun', '2', 'POSTRM_OK action=upgrade')]:
            root, sf, env = fixture()
            r = invoke(scriptdir, slot, [n], env)
            check(r.returncode == 0 and marker.encode() in r.stdout, label + ' mapping ' + slot + ' ' + n)
        root, sf, env = fixture()
        ok(['/bin/bash', scriptdir / 'pre', '1'], env=env)
        r = invoke(scriptdir, 'post', ['1'], env)
        state = json.loads(sf.read_text())
        check(r.returncode == 0 and b'STAGED_INACTIVE' in r.stdout and b'STAGED_UNCONFIGURED' not in r.stdout
              and no_activation(state['calls']), label + ' staged install STAGED_INACTIVE never enables')

        # disabled/inactive => STAGED_INACTIVE; no enable/restart
        root, sf, env = fixture()
        seed_env(root)
        state = json.loads(sf.read_text())
        state['units']['platform-agent.service'] = {'active': 'inactive', 'enabled': 'disabled'}
        sf.write_text(json.dumps(state))
        r = invoke(scriptdir, 'post', ['1'], env)
        state = json.loads(sf.read_text())
        check(r.returncode == 0 and b'STAGED_INACTIVE' in r.stdout and b'DISABLED_PRESERVED' not in r.stdout
              and state['units']['platform-agent.service'] == {'active': 'inactive', 'enabled': 'disabled'}
              and no_activation(state['calls'])
              and not any(c[:2] == ['systemctl', 'try-restart'] for c in state['calls']),
              label + ' disabled/inactive STAGED_INACTIVE')

        # disabled/active => DISABLED_PRESERVED; no try-restart
        root, sf, env = fixture()
        seed_env(root)
        state = json.loads(sf.read_text())
        state['units']['platform-agent.service'] = {'active': 'active', 'enabled': 'disabled'}
        sf.write_text(json.dumps(state))
        r = invoke(scriptdir, 'post', ['2'], env)
        state = json.loads(sf.read_text())
        check(r.returncode == 0 and b'DISABLED_PRESERVED' in r.stdout and b'STAGED_INACTIVE' not in r.stdout
              and state['units']['platform-agent.service']['enabled'] == 'disabled'
              and state['units']['platform-agent.service']['active'] == 'active'
              and no_activation(state['calls'])
              and not any(c[:2] == ['systemctl', 'try-restart'] for c in state['calls']),
              label + ' disabled/active DISABLED_PRESERVED')

        # enabled/inactive => ENABLED_PRESERVED; no invented start/restart
        root, sf, env = fixture()
        seed_env(root)
        state = json.loads(sf.read_text())
        state['units']['platform-agent.service'] = {'active': 'inactive', 'enabled': 'enabled'}
        sf.write_text(json.dumps(state))
        r = invoke(scriptdir, 'post', ['1'], env)
        state = json.loads(sf.read_text())
        check(r.returncode == 0 and b'ENABLED_PRESERVED' in r.stdout
              and state['units']['platform-agent.service'] == {'active': 'inactive', 'enabled': 'enabled'}
              and no_activation(state['calls'])
              and not any(c[:2] == ['systemctl', 'try-restart'] for c in state['calls']),
              label + ' enabled/inactive ENABLED_PRESERVED no start')

        # masked / runtime-only preserved without enable/restart
        root, sf, env = fixture()
        seed_env(root)
        state = json.loads(sf.read_text())
        state['units']['platform-agent.service'] = {'active': 'inactive', 'enabled': 'masked'}
        sf.write_text(json.dumps(state))
        r = invoke(scriptdir, 'post', ['1'], env)
        state = json.loads(sf.read_text())
        check(r.returncode == 0 and b'MASKED_PRESERVED' in r.stdout
              and state['units']['platform-agent.service']['enabled'] == 'masked'
              and no_activation(state['calls'])
              and not any(c[:2] == ['systemctl', 'try-restart'] for c in state['calls']),
              label + ' masked MASKED_PRESERVED')

        root, sf, env = fixture()
        seed_env(root)
        state = json.loads(sf.read_text())
        state['units']['platform-agent.service'] = {'active': 'active', 'enabled': 'enabled-runtime'}
        sf.write_text(json.dumps(state))
        r = invoke(scriptdir, 'post', ['2'], env)
        state = json.loads(sf.read_text())
        check(r.returncode == 0 and b'RUNTIME_ONLY_PRESERVED' in r.stdout
              and state['units']['platform-agent.service']['enabled'] == 'enabled-runtime'
              and no_activation(state['calls'])
              and not any(c[:2] == ['systemctl', 'try-restart'] for c in state['calls']),
              label + ' runtime-only RUNTIME_ONLY_PRESERVED')

        # Contradictory is-enabled status/exit pairs fail closed
        for name, unit, expect_absent in [
            ('disabled exit0', {'active': 'inactive', 'enabled': 'disabled', 'enable_exit': 0}, b'STAGED_INACTIVE'),
            ('masked exit0', {'active': 'inactive', 'enabled': 'masked', 'enable_exit': 0}, b'MASKED_PRESERVED'),
            ('enabled-runtime exit1', {'active': 'active', 'enabled': 'enabled-runtime', 'enable_exit': 1}, b'RUNTIME_ONLY_PRESERVED'),
        ]:
            root, sf, env = fixture()
            seed_env(root)
            state = json.loads(sf.read_text())
            state['units']['platform-agent.service'] = unit
            sf.write_text(json.dumps(state))
            r = invoke(scriptdir, 'post', ['1'], env)
            state = json.loads(sf.read_text())
            check(r.returncode == 78 and b'SERVICE_STATE_QUERY_FAILED' in r.stderr
                  and expect_absent not in r.stdout and b'POSTINST_OK' not in r.stdout
                  and no_activation(state['calls']),
                  label + ' contradictory ' + name)

        # disabled + active-state query failure: no STAGED_INACTIVE claim
        root, sf, env = fixture()
        seed_env(root)
        state = json.loads(sf.read_text())
        state['units']['platform-agent.service'] = {
            'active': 'inactive', 'enabled': 'disabled', 'query_error': True, 'query_error_exit': 1
        }
        sf.write_text(json.dumps(state))
        r = invoke(scriptdir, 'post', ['1'], env)
        check(r.returncode == 78 and b'SERVICE_STATE_QUERY_FAILED' in r.stderr
              and b'STAGED_INACTIVE' not in r.stdout and b'POSTINST_OK' not in r.stdout,
              label + ' disabled active-query failure no STAGED_INACTIVE')

        # Repeat configure/upgrade preserves state bytes and never enables
        root, sf, env = fixture()
        config = seed_env(root, 'PLATFORM_AGENT_SENSOR_URL=https://sensor.example.invalid:8443\nSENTINEL_ENV\n')
        credential = root / 'etc/platform/agent/cert.pem'
        credential.parent.mkdir(parents=True, exist_ok=True)
        credential.write_text('SENTINEL_CERT\n')
        credential.chmod(0o600)
        spool = root / 'var/lib/platform/agent/spool'
        spool.parent.mkdir(parents=True, exist_ok=True)
        spool.write_bytes(b'SENTINEL_SPOOL\n')
        original = {p: p.read_bytes() for p in (config, credential, spool)}
        state = json.loads(sf.read_text())
        state['units']['platform-agent.service'] = {'active': 'active', 'enabled': 'enabled'}
        sf.write_text(json.dumps(state))
        for n in ('1', '2', '1', '2'):
            ok(['/bin/bash', scriptdir / 'post', n], env=env)
        state = json.loads(sf.read_text())
        check(state['units']['platform-agent.service']['enabled'] == 'enabled'
              and state['units']['platform-agent.service']['active'] == 'active'
              and all(p.read_bytes() == data for p, data in original.items())
              and no_activation(state['calls'])
              and sum(1 for c in state['calls'] if c[:2] == ['systemctl', 'try-restart']) >= 3,
              label + ' repeat configure preserves state and controlled restart')

        root, sf, env = fixture()
        config = seed_env(root)
        credential = root / 'etc/platform/agent/cert.pem'
        credential.parent.mkdir(parents=True, exist_ok=True)
        credential.write_text('fixture certificate\n')
        spool = root / 'var/lib/platform/agent/spool'
        spool.parent.mkdir(parents=True, exist_ok=True)
        spool.write_bytes(b'unacknowledged fixture\n')
        original = {p: p.read_bytes() for p in (config, credential, spool)}
        state = json.loads(sf.read_text())
        state['units']['platform-agent.service'] = {'active': 'active', 'enabled': 'enabled'}
        state['calls'] = []
        sf.write_text(json.dumps(state))
        for slot, n in [('pre', '2'), ('post', '2'), ('preun', '1'), ('postun', '1')]:
            ok(['/bin/bash', scriptdir / slot, n], env=env)
        state = json.loads(sf.read_text())
        check(state['units']['platform-agent.service'] == {'active': 'active', 'enabled': 'enabled'} and
              not any(c[:2] in (['systemctl', 'stop'], ['systemctl', 'disable']) for c in state['calls']), label + ' old package update removal leaves replacement active enabled')
        check(any(c[:2] == ['systemctl', 'try-restart'] for c in state['calls']), label + ' configured active upgrade restarts')
        check(no_activation(state['calls']), label + ' upgrade path never enables')
        for slot in ('preun', 'postun'):
            ok(['/bin/bash', scriptdir / slot, '0'], env=env)
        state = json.loads(sf.read_text())
        check(state['units']['platform-agent.service'] == {'active': 'inactive', 'enabled': 'disabled'}, label + ' erase stop disable')
        check(all(p.read_bytes() == data for p, data in original.items()) and bool(state['users']) and bool(state['groups']), label + ' erase retains state credentials configuration accounts')
        # Errors run in their own interpreter; no false success from shell function contexts.
        # Package hooks never call enable; do not assert SERVICE_ENABLE_FAILED.
        cases = [('groupadd', None, 'post', '1', 37, b'', b'POSTINST_OK'),
                 ('useradd', None, 'post', '1', 37, b'', b'POSTINST_OK'),
                 ('getent', 'group', 'post', '1', 78, b'IDENTITY_LOOKUP_FAILED', b'POSTINST_OK'),
                 ('systemctl', 'daemon-reload', 'post', '1', 78, b'SERVICE_DAEMON_RELOAD_FAILED', b'POSTINST_OK'),
                 ('systemctl', 'is-active', 'post', '2', 78, b'SERVICE_STATE_QUERY_FAILED', b'POSTINST_OK'),
                 ('systemctl', 'try-restart', 'post', '2', 78, b'SERVICE_RESTART_FAILED', b'POSTINST_OK'),
                 ('systemctl', 'stop', 'preun', '0', 78, b'SERVICE_STOP_FAILED', b'PRERM_OK'),
                 ('systemctl', 'disable', 'preun', '0', 78, b'SERVICE_DISABLE_FAILED', b'PRERM_OK')]
        for tool, action, slot, n, exitcode, diagnostic, success in cases:
            root, sf, env = fixture()
            seed_env(root, 'fixture\n')
            state = json.loads(sf.read_text())
            state.update(fail=[tool, action] if action else [tool], fail_exit=37,
                         units={'platform-agent.service': {'active': 'active', 'enabled': 'enabled'}})
            sf.write_text(json.dumps(state))
            if tool == 'groupadd':
                shim = Path(env['PLATFORM_AGENT_PACKAGE_TOOLS']) / 'groupadd'
                shim.write_text('#!/bin/bash\necho FIXTURE_GROUPADD_DIAGNOSTIC >&2\nexit 37\n')
                diagnostic = b'FIXTURE_GROUPADD_DIAGNOSTIC'
            r = invoke(scriptdir, slot, [n], env)
            check(r.returncode == exitcode and diagnostic in r.stderr and success not in r.stdout,
                  label + f' failure {tool}/{action} scriptlet_exit={r.returncode} transaction=NOT_RUN')
            if action == 'stop':
                state = json.loads(sf.read_text())
                check(not any(c[:2] == ['systemctl', 'disable'] for c in state['calls']), label + ' stop failure prevents disable')
        # Preserve shared helper semantics: postrm remove reload is deliberately best effort.
        root, sf, env = fixture()
        state = json.loads(sf.read_text())
        state.update(fail=['systemctl', 'daemon-reload'], fail_exit=37)
        sf.write_text(json.dumps(state))
        r = invoke(scriptdir, 'postun', ['0'], env)
        check(r.returncode == 0 and b'STATE_PRESERVED' in r.stdout, label + ' existing best-effort postrm reload retained')
        root, sf, env = fixture()
        legacy = root / 'usr/local/bin/platform-agent-linux'
        legacy.parent.mkdir(parents=True)
        legacy.write_text('unmanaged\n')
        r = invoke(scriptdir, 'pre', ['1'], env)
        check(r.returncode == 78 and b'UNMANAGED_USR_LOCAL' in r.stderr and legacy.read_text() == 'unmanaged\n', label + ' unmanaged legacy preserved and refused')
        root, sf, env = fixture()
        outside = root.parent / 'outside'
        outside.mkdir()
        (root / 'opt').symlink_to(outside, target_is_directory=True)
        r = invoke(scriptdir, 'pre', ['1'], env)
        check(r.returncode == 78 and b'SYMLINK_REFUSED' in r.stderr and not list(outside.iterdir()), label + ' unsafe ancestor refused without outside mutation')
        root, sf, env = fixture()
        state = json.loads(sf.read_text())
        state['groups']['platform-agent'] = 'platform-agent:x:32001:'
        state['users']['platform-agent'] = 'platform-agent:x:0:32001::/var/lib/platform/agent:/usr/sbin/nologin'
        sf.write_text(json.dumps(state))
        r = invoke(scriptdir, 'post', ['1'], env)
        check(r.returncode == 78 and b'IDENTITY_UNSAFE' in r.stderr and b'POSTINST_OK' not in r.stdout, label + ' UID zero refused')
        root, sf, env = fixture()
        config = root / 'etc/platform/agent.env'
        config.parent.mkdir(parents=True)
        config.write_text('fixture\n')
        config.chmod(0o644)
        r = invoke(scriptdir, 'post', ['1'], env)
        check(r.returncode == 78 and b'ENV_PERMS' in r.stderr and config.read_text() == 'fixture\n', label + ' insecure configuration refused and preserved')
        root, sf, env = fixture()
        (Path(env['PLATFORM_AGENT_PACKAGE_TOOLS']) / 'systemctl').unlink()
        r = invoke(scriptdir, 'post', ['1'], env)
        check(r.returncode == 78 and b'SERVICE_MANAGER_UNSUPPORTED' in r.stderr, label + ' missing service manager refused')

    suite(scripts, 'SOURCE_MOCK')
    if full:
        artifact_dir = os.environ.get('RPM_ARTIFACT_DIR')
        if not artifact_dir:
            print('RPM_TEST_REFUSED: full mode requires RPM_ARTIFACT_DIR from an admitted retained-input build', file=sys.stderr)
            sys.exit(78)
        artifact = Path(artifact_dir)
        receipt = json.loads((artifact / 'build-receipt.json').read_text())
        rpms = list((artifact / 'RPMS').rglob('*.rpm'))
        assert len(rpms) == 1 and not receipt['fixture_inputs']
        rpm = rpms[0]
        check(hashlib.sha256(rpm.read_bytes()).hexdigest() == receipt['rpm_sha256'], 'actual RPM receipt hash')
        actual_scripts = temp / 'actual-scriptlets'
        actual_scripts.mkdir()
        for slot, tag in [('pre', 'PREIN'), ('post', 'POSTIN'), ('preun', 'PREUN'), ('postun', 'POSTUN')]:
            actual = ok(['rpm', '-qp', '--qf', '%{' + tag + '}', rpm])
            check(actual.rstrip(b'\n') == (scripts / slot).read_bytes().rstrip(b'\n'), 'actual RPM queried scriptlet ' + slot)
            (actual_scripts / slot).write_bytes(actual)
        suite(actual_scripts, 'RPM_EXTRACTED_MOCK')
        print('RPM_BUILD_RECEIPT_CHECKED; RPM_TRANSACTION=NOT_RUN NATIVE=NOT_RUN')
    else:
        print('RPM_PARSE=NOT_RUN RPM_BUILD=NOT_RUN RPM_EXTRACTED_SCRIPTLETS=NOT_RUN RPM_TRANSACTION=NOT_RUN NATIVE=NOT_RUN')
    print(f'RPM_FIXTURE_ASSERTIONS={count} MODE={mode[0]}')
PY
