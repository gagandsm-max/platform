#!/bin/bash
# All writes go to a fresh private output child. Run in an admitted container.
set -euo pipefail
exec python3 - "$@" <<'PY'
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import struct
import subprocess
import sys

BASE = 'agent/packaging/linux/'
RECIPE = [BASE + 'rpm/' + p for p in ('build-rpm.sh', 'platform-agent.spec.in', 'README.md')]
RECIPE += ['tests/agent-packaging/rpm/' + p for p in ('test-rpm.sh', 'compose.yaml')]
PAYLOAD = {
    'agent/linux/systemd/ebpf/20-ebpf.conf': ('usr/share/platform-agent/systemd/20-ebpf.conf', 0o644),
    'agent/linux/systemd/platform-agent.service': ('usr/lib/systemd/system/platform-agent.service', 0o644),
    BASE + 'scripts/platform-agent-lifecycle.sh': ('usr/lib/platform/agent/packaging/platform-agent-lifecycle.sh', 0o755),
    BASE + 'package-layout.json': ('usr/lib/platform/agent/packaging/package-layout.json', 0o644),
    'agent/linux/systemd/platform-agent.env.example': ('usr/share/doc/platform-agent/platform-agent.env.example', 0o644),
    BASE + 'README.md': ('usr/share/doc/platform-agent/README.packaging.md', 0o644),
}
MIRROR = 'deploy/systemd/platform-agent.service'
LICENSE = 'project_documentation/Platform_07_OSS_Catalog.md'

def refuse(message):
    print('PLATFORM_AGENT_RPM_BUILD_REFUSED: ' + message, file=sys.stderr)
    sys.exit(78)

def require(condition, message):
    if not condition:
        refuse(message)

def digest(data):
    return hashlib.sha256(data).hexdigest()

def run(args, **kw):
    # No shell interpolation; tool diagnostics and actual exits are preserved.
    return subprocess.run(args, check=True, **kw)

def capture(args, **kw):
    return run(args, stdout=subprocess.PIPE, **kw).stdout

def regular(path):
    require(path.is_file() and not path.is_symlink(), 'not a regular non-symlink file: ' + str(path))
    require(not any(p.is_symlink() for p in path.parents), 'symlink ancestor: ' + str(path))
    return path.read_bytes()

def write(path, data, mode=0o644):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)
    path.chmod(mode)

def git_blob(commit, name):
    mode = capture(['git', 'ls-tree', commit, '--', name]).split(b' ', 1)[0]
    require(mode in (b'100644', b'100755'), 'non-regular or absent Git input: ' + name)
    return capture(['git', 'show', commit + ':' + name])

def require_elf64_le_x86_64(data):
    # Same bounded ELF64 LE executable / program-header / file-backed entry
    # contract as the accepted DEB builder, adapted to retained bytes (x86_64=62).
    size = len(data)
    if size < 64 or data[:7] != b'\x7fELF\x02\x01\x01':
        refuse('binary must be an ELF64 little-endian executable')
    kind, machine, version, entry, phoff, _, _, ehsize, phentsize, phnum, *_ = (
        struct.unpack_from('<HHIQQQIHHHHHH', data, 16)
    )
    if machine != 62:
        refuse(f'binary ELF machine mismatch for x86_64: expected 62, found {machine}')
    if (kind not in (2, 3) or version != 1 or ehsize != 64 or phentsize != 56
            or not entry or not phnum or phoff < ehsize
            or phoff + phnum * phentsize > size):
        refuse('binary must be an ELF64 little-endian executable with valid program headers')
    offset = phoff
    for _ in range(phnum):
        segment = data[offset:offset + phentsize]
        if len(segment) != phentsize:
            refuse('binary ELF program headers are truncated')
        segment_type, flags, seg_offset, vaddr, _, filesz, memsz, _ = struct.unpack(
            '<IIQQQQQQ', segment
        )
        if (segment_type == 1 and flags & 1 and filesz <= memsz
                and seg_offset + filesz <= size and vaddr <= entry < vaddr + filesz):
            return
        offset += phentsize
    refuse('binary ELF entry point is not backed by an executable load segment')

def main():
    os.umask(0o022)
    p = argparse.ArgumentParser(description='Unsigned retained-payload RPM adapter; no install/sign/download.')
    p.add_argument('--source-only', action='store_true', help='generate only; no RPM parser/build claim')
    p.add_argument('--fixture-inputs', action='store_true', help='source-only synthetic inputs; never an RPM')
    p.add_argument('--binary', required=True)
    p.add_argument('--binary-sha256', required=True)
    p.add_argument('--source-commit', required=True)
    p.add_argument('--recipe-commit', help='exact commit containing all five unchanged recipe/test files')
    p.add_argument('--builder-image', required=True, help='admitted local image ID sha256:HEX64')
    p.add_argument('--version', required=True)
    p.add_argument('--release', required=True)
    p.add_argument('--architecture', default='x86_64')
    p.add_argument('--source-date-epoch', default='0')
    p.add_argument('--out', required=True)
    a = p.parse_args()
    require(not a.fixture_inputs or a.source_only, 'fixture inputs are source-only')
    # This check is intentionally early: default mode must not masquerade as source-only.
    commands = ['git', 'bash', 'cp']
    if not a.source_only:
        commands += ['rpm', 'rpmbuild', 'rpmspec', 'rpm2cpio', 'cpio']
    for cmd in commands:
        require(shutil.which(cmd), 'required tool missing: ' + cmd)
    require(re.fullmatch(r'[0-9][A-Za-z0-9.+~]{0,63}', a.version), 'invalid RPM version')
    require(re.fullmatch(r'[1-9][A-Za-z0-9.]{0,31}', a.release), 'invalid RPM release')
    require(a.architecture == 'x86_64', 'only x86_64 (amd64) is in scope')
    require(re.fullmatch(r'0|[1-9][0-9]{0,9}', a.source_date_epoch), 'invalid source-date-epoch')
    require(int(a.source_date_epoch) <= 4294967295, 'source-date-epoch exceeds RPM timestamp range')
    require(re.fullmatch(r'[0-9a-f]{40}', a.source_commit), 'invalid source commit')
    require(re.fullmatch(r'[0-9a-f]{64}', a.binary_sha256), 'invalid binary hash')
    require(re.fullmatch(r'sha256:[0-9a-f]{64}', a.builder_image), 'builder image must be an immutable local image ID')
    if a.recipe_commit:
        require(re.fullmatch(r'[0-9a-f]{40}', a.recipe_commit), 'invalid recipe commit')
    require(a.source_only or a.recipe_commit, 'full build requires a committed recipe')
    root = Path.cwd().resolve()
    require((root / BASE).is_dir(), 'run from repository root')
    binary = Path(os.path.abspath(a.binary))
    require(binary.is_file() and 0 < binary.stat().st_size <= 268435456, 'binary size/type out of bounds')
    binary_data = regular(binary)
    require(digest(binary_data) == a.binary_sha256, 'binary sha256 mismatch')
    if not a.fixture_inputs:
        require_elf64_le_x86_64(binary_data)
    inputs = {name: regular(root / name) for name in [*PAYLOAD, MIRROR, LICENSE, *RECIPE]}
    for name in [*PAYLOAD, MIRROR]:
        require(inputs[name] == git_blob(a.source_commit, name), 'payload source mismatch: ' + name)
    if a.recipe_commit:
        for name in [*RECIPE, LICENSE]:
            require(inputs[name] == git_blob(a.recipe_commit, name), 'recipe source mismatch: ' + name)
    require(inputs[MIRROR] == inputs['agent/linux/systemd/platform-agent.service'], 'unit mirror mismatch')
    require(b'Platform is a clean-room proprietary commercial product' in inputs[LICENSE], 'licensing authority changed')
    helper = inputs[BASE + 'scripts/platform-agent-lifecycle.sh']
    terminal = b'main "$@"\n'
    require(helper.startswith(b'#!/bin/bash\n') and helper.endswith(terminal), 'helper terminal invocation changed')
    require(helper.splitlines().count(terminal.rstrip(b'\n')) == 1, 'ambiguous helper main invocation')
    # Remove exactly one terminal invocation. Preserve every other helper byte.
    embedded = helper[:-len(terminal)]
    scriptlets = {}
    mappings = {'pre': ('preinst', 'install', 'upgrade'), 'post': ('postinst', 'configure', 'configure'),
                'preun': ('prerm', 'remove', 'upgrade'), 'postun': ('postrm', 'remove', 'upgrade')}
    for slot, (phase, first, other) in mappings.items():
        adapter = '''
# RPM count adapter. Decimal strings avoid arithmetic overflow at large counts.
[[ "$#" == 1 ]] || die RPM_COUNT_INVALID 'expected exactly one package count'
[[ "$1" =~ ^(0|[1-9][0-9]*)$ ]] || die RPM_COUNT_INVALID 'malformed package count'
'''
        if slot in ('pre', 'post'):
            adapter += '[[ "$1" != 0 ]] || die RPM_COUNT_INVALID "zero in installation slot"\n'
        discriminator = '1' if slot == 'pre' else '0'
        adapter += f'if [[ "$1" == {discriminator} ]]; then\n  main {phase} {first}\nelse\n  main {phase} {other}\nfi\n'
        scriptlets[slot] = embedded + adapter.encode()
    spec = inputs[BASE + 'rpm/platform-agent.spec.in'].decode()
    values = {'VERSION': a.version, 'RELEASE': a.release}
    values.update({k.upper(): v.decode().replace('%', '%%') for k, v in scriptlets.items()})
    # One pass: inserted helper text cannot introduce a second template substitution.
    spec = re.sub(r'@([A-Z]+)@', lambda m: values[m[1]], spec)
    out = Path(os.path.abspath(a.out))
    require(out.is_dir() and not out.is_symlink() and not any(x.is_symlink() for x in out.parents), 'unsafe output directory')
    require(re.fullmatch(r'/[A-Za-z0-9_./-]+', str(out)), 'output path must be absolute RPM-safe characters')
    out = out.resolve()
    require(out != root and root not in out.parents and out not in root.parents, 'source/output overlap')
    for flag in ('--absolute-git-dir', '--git-common-dir'):
        metadata = Path(capture(['git', 'rev-parse', flag]).decode().strip()).resolve()
        require(out != metadata and metadata not in out.parents and out not in metadata.parents, 'Git metadata/output overlap')
    require(out != binary.parent and out not in binary.parents, 'binary/output overlap')
    dest = out / f'platform-agent-{a.version}-{a.release}.x86_64'
    require(not dest.exists() and not dest.is_symlink(), 'output collision: ' + str(dest))
    dest.mkdir(mode=0o700)
    for name in ('BUILD', 'BUILDROOT', 'RPMS', 'SRPMS', 'SOURCES', 'SPECS', 'scriptlets'):
        (dest / name).mkdir()
    stage = dest / 'SOURCES/payload'
    for name, (target, mode) in PAYLOAD.items():
        write(stage / target, inputs[name], mode)
    write(stage / 'opt/platform/agent/platform-agent-linux', binary_data, 0o755)
    write(stage / 'usr/share/doc/platform-agent/README.rpm.md', inputs[BASE + 'rpm/README.md'])
    versions = {'python': sys.version, 'bash': capture(['bash', '--version']).decode().splitlines()[0],
                'git': capture(['git', '--version']).decode().strip()}
    tool_hashes = {cmd: digest(Path(shutil.which(cmd)).resolve().read_bytes()) for cmd in commands}
    tool_hashes['python3'] = digest(Path(sys.executable).resolve().read_bytes())
    if not a.source_only:
        for cmd in ('rpm', 'rpmbuild', 'rpmspec', 'cpio'):
            versions[cmd] = capture([cmd, '--version']).decode().strip()
        write(dest / 'rpm-showrc.txt', capture(['rpm', '--showrc']))
    provenance = {
        'schema': 'PLATFORM_AGENT_RPM_PROVENANCE_V1', 'signing': 'UNSIGNED_INTERNAL_ONLY',
        'payload_source_commit': a.source_commit, 'retained_binary_sha256': digest(binary_data),
        'recipe_commit': a.recipe_commit or 'UNCOMMITTED_SOURCE_ONLY',
        'recipe_sha256': {n: digest(inputs[n]) for n in RECIPE},
        'payload_source_sha256': {n: digest(inputs[n]) for n in PAYLOAD},
        'licensing_source': LICENSE, 'licensing_source_sha256': digest(inputs[LICENSE]),
        'builder_image_id_input': a.builder_image, 'builder_identity_attestation': 'EXTERNAL_ADMISSION_REQUIRED',
        'tools': versions, 'tool_executable_sha256': tool_hashes,
        'target': 'RHEL9.8 amd64', 'architecture': a.architecture,
        'nevra': f'platform-agent-0:{a.version}-{a.release}.x86_64',
        'source_date_epoch': int(a.source_date_epoch), 'fixture_inputs': a.fixture_inputs,
        'evidence': 'SOURCE_ONLY' if a.source_only else 'RPM_BUILD',
        'rpm_transaction': 'NOT_RUN', 'native_lifecycle': 'NOT_RUN',
    }
    write(stage / 'usr/lib/platform/agent/packaging/rpm-provenance.json', (json.dumps(provenance, sort_keys=True, indent=2) + '\n').encode())
    for slot, data in scriptlets.items():
        write(dest / 'scriptlets' / slot, data, 0o755)
    specpath = dest / 'SPECS/platform-agent.spec'
    write(specpath, spec.encode())
    for path in sorted(stage.rglob('*'), reverse=True):
        os.utime(path, (int(a.source_date_epoch), int(a.source_date_epoch)))
    os.utime(stage, (int(a.source_date_epoch), int(a.source_date_epoch)))
    if a.source_only:
        print('SOURCE_ONLY_GENERATED=' + str(dest))
        print('RPM_PARSE=NOT_RUN RPM_BUILD=NOT_RUN RPM_TRANSACTION=NOT_RUN NATIVE=NOT_RUN')
        return
    env = dict(os.environ, SOURCE_DATE_EPOCH=a.source_date_epoch, LC_ALL='C', TZ='UTC')
    defines = ['--define', '_topdir ' + str(dest), '--define', '_buildhost admitted-builder']
    write(dest / 'parsed.spec', capture(['rpmspec', *defines, '--parse', str(specpath)], env=env))
    with (dest / 'rpmbuild.log').open('wb') as log:
        result = subprocess.run(['rpmbuild', *defines, '-bb', str(specpath)], env=env, stdout=log, stderr=subprocess.STDOUT)
    print('RPMBUILD_EXIT=' + str(result.returncode), flush=True)
    if result.returncode:
        sys.stderr.buffer.write((dest / 'rpmbuild.log').read_bytes())
        sys.exit(result.returncode)
    artifacts = list((dest / 'RPMS').rglob('*.rpm'))
    require(len(artifacts) == 1, 'expected exactly one RPM')
    rpm = artifacts[0]
    nevra = capture(['rpm', '-qp', '--qf', '%{NAME}-%{EPOCHNUM}:%{VERSION}-%{RELEASE}.%{ARCH}', str(rpm)]).decode()
    require(nevra == provenance['nevra'], 'built NEVRA mismatch')
    # Query actual stored scripts, not just our pre-parser generated inputs.
    for slot, tag in [('pre', 'PREIN'), ('post', 'POSTIN'), ('preun', 'PREUN'), ('postun', 'POSTUN')]:
        actual = capture(['rpm', '-qp', '--qf', '%{' + tag + '}', str(rpm)])
        interp = capture(['rpm', '-qp', '--qf', '%{' + tag + 'PROG}', str(rpm)])
        require(interp == b'/bin/bash', 'scriptlet interpreter changed: ' + slot)
        require(actual.rstrip(b'\n') == scriptlets[slot].rstrip(b'\n'), 'RPM scriptlet bytes changed: ' + slot)
        write(dest / 'extracted-scriptlets' / slot, actual, 0o755)
    archive = dest / 'payload.cpio'
    with archive.open('wb') as f:
        run(['rpm2cpio', str(rpm)], stdout=f)
    with archive.open('rb') as f:
        members = capture(['cpio', '-it', '--quiet'], stdin=f).decode().splitlines()
    expected = {str(x.relative_to(stage)) for x in stage.rglob('*') if x.is_file()}
    dirs = {str(x.relative_to(stage)) for x in stage.rglob('*') if x.is_dir()}
    require(all(m.startswith('./') and '..' not in m.split('/') and m[2:] in expected | dirs for m in members), 'unexpected RPM payload member')
    extracted = dest / 'extracted-payload'
    extracted.mkdir()
    with archive.open('rb') as f:
        run(['cpio', '-id', '--quiet', '--no-absolute-filenames', '--no-preserve-owner'], stdin=f, cwd=extracted)
    require({str(x.relative_to(extracted)) for x in extracted.rglob('*') if x.is_file()} == expected, 'RPM payload inventory changed')
    for name in expected:
        require(regular(extracted / name) == regular(stage / name), 'RPM changed payload bytes: ' + name)
        require((extracted / name).stat().st_mode & 0o777 == (stage / name).stat().st_mode & 0o777, 'RPM changed payload mode: ' + name)
    receipt = dict(provenance, rpm_path=str(rpm), rpm_sha256=digest(rpm.read_bytes()), rpm_bytes=rpm.stat().st_size,
                   rpm_parse='PASS', rpm_build='PASS', extracted_payload='PASS', extracted_scriptlets='PASS')
    write(dest / 'build-receipt.json', (json.dumps(receipt, sort_keys=True, indent=2) + '\n').encode())
    print(json.dumps(receipt, sort_keys=True))

try:
    main()
except subprocess.CalledProcessError as e:
    print('PLATFORM_AGENT_RPM_TOOL_FAILED: exit=' + str(e.returncode) + ' tool=' + str(e.cmd[0]), file=sys.stderr)
    sys.exit(e.returncode if e.returncode > 0 else 1)
except (OSError, ValueError, KeyError) as e:
    refuse(str(e))
PY
