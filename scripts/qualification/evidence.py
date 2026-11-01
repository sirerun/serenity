"""Bounded Serenity qualification evidence protocol. Importing performs no effects.

QEV1 frames carry the complete combined stdout/stderr of one owned component.
Transport prefixes are permitted; missing/reordered/duplicated data is refused.
"""
import base64
import hashlib
import re

RESPONSE_LIMIT = 16 * 1024 * 1024
ARTIFACT_LIMIT = 4 * 1024 * 1024
LINE_CAP = 100000
CHUNK = 1024
COMPONENTS = {'go', 'python'}
META = re.compile(r'QEV1 (BEGIN|END) component=(go|python) run=([a-f0-9]{64}) chunks=([0-9]+) bytes=([0-9]+) sha256=([a-f0-9]{64}) exit=([0-9]+)')
DATA = re.compile(r'QEV1 DATA component=(go|python) run=([a-f0-9]{64}) seq=([0-9]+) data=([A-Za-z0-9+/=]+)')


class EvidenceError(ValueError):
    pass


def emit(component, run, raw, code):
    if component not in COMPONENTS or not re.fullmatch('[a-f0-9]{64}', run):
        raise EvidenceError('invalid identity')
    if len(raw) > ARTIFACT_LIMIT or not 0 <= code <= 255:
        raise EvidenceError('artifact limit or invalid exit')
    encoded = base64.b64encode(raw).decode('ascii')
    chunks = [encoded[i:i + CHUNK] for i in range(0, len(encoded), CHUNK)]
    meta = f'component={component} run={run} chunks={len(chunks)} bytes={len(raw)} sha256={hashlib.sha256(raw).hexdigest()} exit={code}'
    print('QEV1 BEGIN ' + meta, flush=True)
    for i, chunk in enumerate(chunks):
        print(f'QEV1 DATA component={component} run={run} seq={i} data={chunk}', flush=True)
    print('QEV1 END ' + meta, flush=True)


def parse(raw, expected_run):
    """Return complete verified artifacts or raise; never merge partial snapshots."""
    if len(raw) > RESPONSE_LIMIT:
        raise EvidenceError('response byte limit')
    if raw.count(b'\n') >= LINE_CAP:
        raise EvidenceError('transport line cap reached; completeness unknown')
    if not raw.endswith(b'\n'):
        raise EvidenceError('partial or absent final line')
    if not re.fullmatch('[a-f0-9]{64}', expected_run):
        raise EvidenceError('invalid expected run')
    states = {}
    for line in raw.decode('utf-8', errors='strict').splitlines():
        pos = line.find('QEV1 ')
        if pos < 0:
            continue  # transport diagnostics cannot establish a pass
        frame = line[pos:]
        match = META.fullmatch(frame)
        if match:
            kind, component, run, chunks, size, digest, code = match.groups()
            if run != expected_run:
                raise EvidenceError('foreign run')
            meta = (int(chunks), int(size), digest, int(code))
            if meta[1] > ARTIFACT_LIMIT or meta[0] != ((meta[1] + 2) // 3 * 4 + CHUNK - 1) // CHUNK or meta[3] > 255:
                raise EvidenceError('invalid bounded metadata')
            if kind == 'BEGIN':
                if component in states:
                    raise EvidenceError('duplicate begin')
                states[component] = {'meta': meta, 'chunks': [], 'ended': False}
            else:
                state = states.get(component)
                if state is None or state['ended'] or state['meta'] != meta or len(state['chunks']) != meta[0]:
                    raise EvidenceError('incomplete or mismatched end')
                encoded = ''.join(state['chunks'])
                decoded = base64.b64decode(encoded, validate=True)
                if base64.b64encode(decoded).decode('ascii') != encoded:
                    raise EvidenceError('noncanonical base64')
                if len(decoded) != meta[1] or hashlib.sha256(decoded).hexdigest() != meta[2]:
                    raise EvidenceError('artifact digest or size mismatch')
                state.update(ended=True, raw=decoded)
            continue
        match = DATA.fullmatch(frame)
        if not match:
            raise EvidenceError('malformed or error frame')
        component, run, seq, data = match.groups()
        state = states.get(component)
        if run != expected_run or state is None or state['ended']:
            raise EvidenceError('data without matching open component')
        if int(seq) != len(state['chunks']) or int(seq) >= state['meta'][0]:
            raise EvidenceError('missing, reordered or duplicate chunk')
        expected_length = min(CHUNK, (state['meta'][1] + 2) // 3 * 4 - int(seq) * CHUNK)
        if len(data) != expected_length:
            raise EvidenceError('chunk size mismatch')
        state['chunks'].append(data)
    if set(states) != COMPONENTS or not all(s['ended'] for s in states.values()):
        raise EvidenceError('missing complete component')
    return {c: {'raw': s['raw'], 'exit': s['meta'][3], 'sha256': s['meta'][2], 'bytes': s['meta'][1]} for c, s in states.items()}
