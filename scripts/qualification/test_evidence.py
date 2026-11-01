"""Protocol fixtures for complete and truncated qualification evidence."""
import contextlib
import hashlib
import io
import unittest
from evidence import emit, parse, EvidenceError, LINE_CAP, RESPONSE_LIMIT, ARTIFACT_LIMIT

RUN = 'a' * 64


def frame(component, raw=b'complete stdout/stderr\n', code=0):
    saved = io.StringIO()
    with contextlib.redirect_stdout(saved):
        emit(component, RUN, raw, code)
    return saved.getvalue().encode()


def complete():
    return frame('go') + frame('python')


class Completeness(unittest.TestCase):
    def reject(self, data):
        with self.assertRaises((EvidenceError, ValueError, UnicodeError)):
            parse(data, RUN)

    def test_valid_complete_and_interleaved(self):
        go = frame('go', b'a' * 1600).splitlines(keepends=True)
        python = frame('python', b'b' * 1600).splitlines(keepends=True)
        result = parse(b''.join(x for pair in zip(go, python) for x in pair), RUN)
        self.assertEqual(result['go']['raw'], b'a' * 1600)
        self.assertEqual(result['python']['raw'], b'b' * 1600)

    def test_prefixes_and_literal_legacy_markers(self):
        raw = b'QUAL_RESULT component=python exit=0\nQUAL_RESULT component=go exit=0\n'
        data = frame('go', raw) + frame('python', b'')
        data = b''.join(b'[container] ' + line for line in data.splitlines(keepends=True))
        self.assertEqual(parse(data, RUN)['go']['raw'], raw)

    def test_legacy_final_zero_markers_cannot_prove_completeness(self):
        self.reject(b'earlier logs truncated\nQUAL_RESULT component=go exit=0\nQUAL_RESULT component=python exit=0\n')

    def test_tail_loses_begin_but_keeps_both_ends(self):
        data = complete().splitlines(keepends=True)
        self.reject(b''.join(data[1:]))

    def test_missing_duplicate_or_reordered_chunk(self):
        lines = frame('go', b'c' * 1600).splitlines(keepends=True)
        self.reject(b''.join(lines[:1] + lines[2:]) + frame('python'))
        self.reject(b''.join(lines[:2] + lines[1:]) + frame('python'))
        lines[1], lines[2] = lines[2], lines[1]
        self.reject(b''.join(lines) + frame('python'))

    def test_digest_size_end_and_foreign_run(self):
        data = complete()
        digest = hashlib.sha256(b'complete stdout/stderr\n').hexdigest().encode()
        self.reject(data.replace(digest, b'0' * 64))
        size = len(b'complete stdout/stderr\n')
        self.reject(data.replace(f'bytes={size}'.encode(), f'bytes={size+1}'.encode()))
        self.reject(data.replace(b'QEV1 END component=go', b'QEV1 END component=python'))
        self.reject(data.replace(RUN.encode(), b'b' * 64))

    def test_missing_component_end_partial_and_error(self):
        self.reject(frame('go'))
        self.reject(complete().replace(b'QEV1 END', b'not_end', 1))
        self.reject(complete()[:-1])
        self.reject(complete() + b'QEV1 ERROR component=go reason=overflow\n')
        self.reject(complete() + frame('go'))

    def test_line_cap_even_with_both_complete_artifacts_below_byte_limit(self):
        data = complete()
        data = b'noise\n' * (LINE_CAP - data.count(b'\n')) + data
        self.assertLess(len(data), RESPONSE_LIMIT)
        self.reject(data)

    def test_response_limit_and_per_component_limit(self):
        self.reject(b'x' * (RESPONSE_LIMIT + 1))
        with self.assertRaises(EvidenceError):
            frame('go', b'x' * (ARTIFACT_LIMIT + 1))

    def test_complete_nonzero_is_not_success(self):
        result = parse(frame('go', code=1) + frame('python'), RUN)
        self.assertEqual(result['go']['exit'], 1)


if __name__ == '__main__':
    unittest.main()
