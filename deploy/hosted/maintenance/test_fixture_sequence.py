"""Synthetic root-namespace receipt continuity; no child/provider execution."""
import copy
import os
from pathlib import Path
import tempfile
import unittest

from test_fixture_proof import cohort, consumer, producer


class SequenceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        os.chmod(self.temp.name, 0o700)
        pins, binding, authority, self.now = cohort()
        self.proof, _ = producer.assemble(pins, binding, authority, 'prepare', now=self.now, nonce='b'*64)
        self.adapter = consumer.Adapter(root=self.temp.name, owner=os.getuid(), clock=lambda: self.now)
        self.adapter.proof = self.proof
        self.stage = self.adapter.path(self.proof['maintenance_hold']['stage'])
        self.stage.mkdir(parents=True, mode=0o700)
        self.addCleanup(lambda: [os.close(fd) for fd in self.adapter.fds])

    def put(self, name, value):
        p = self.stage / name
        p.write_bytes(value)
        p.chmod(0o600)

    def previous(self, nonce='c'*64, sequence=1, result='success'):
        previous = copy.deepcopy(self.proof)
        previous['nonce'] = nonce
        raw = producer.raw(previous)
        outcome = dict(version=1, purpose=previous['purpose'], operation_id=previous['maintenance_hold']['operation_id'], nonce=nonce, operation=previous['operation'], proof_sha256=producer.digest(raw), tool_source=previous['tool_source'], tool_sha256=previous['tool_sha256'], completed_at=self.now, result=result, sequence=sequence)
        self.put('.fixture-used-'+nonce, b'consumed\n')
        self.put('fixture-proof-'+nonce+'.json', raw)
        self.put('fixture-outcome-'+nonce+'.json', producer.raw(outcome))
        return nonce

    def test_complete_sequence_and_current_inflight(self):
        self.assertEqual(self.adapter.outcome_sequence(), 1)
        self.previous()
        self.assertEqual(self.adapter.outcome_sequence(), 2)
        nonce = self.proof['nonce']
        self.put('.fixture-used-'+nonce, b'consumed\n')
        self.put('fixture-proof-'+nonce+'.json', producer.raw(self.proof))
        with self.assertRaises(RuntimeError): self.adapter.outcome_sequence()
        self.assertEqual(self.adapter.outcome_sequence(nonce), 2)

    def test_unknown_refused_gapped_and_boolean_sequence_refuse(self):
        for sequence, result in ((2, 'success'), (True, 'success'), (1, 'refused'), (1, 'uncertain')):
            with self.subTest(sequence=sequence, result=result):
                self.previous(sequence=sequence, result=result)
                with self.assertRaises(RuntimeError): self.adapter.outcome_sequence()
        self.previous()
        (self.stage / ('fixture-outcome-'+'c'*64+'.json')).unlink()
        with self.assertRaises(RuntimeError): self.adapter.outcome_sequence()

    def test_exact_original_proof_exclusive_publication(self):
        request = str(Path(self.proof['maintenance_hold']['stage']) / 'fixture-request.json')
        raw = producer.raw(self.proof)
        self.put('fixture-request.json', raw)
        self.adapter.open_pin(request)
        self.adapter.save_v3_proof(request)
        target = self.stage / ('fixture-proof-'+self.proof['nonce']+'.json')
        self.assertEqual(target.read_bytes(), raw)
        self.assertEqual(target.stat().st_mode & 0o777, 0o600)
        self.assertEqual(target.stat().st_nlink, 1)
        with self.assertRaises(RuntimeError): self.adapter.save_v3_proof(request)
        self.assertEqual(target.read_bytes(), raw)

    def test_pending_record_blocks_sequence(self):
        self.put('fixture-outcome-'+'c'*64+'.json.pending', b'{')
        with self.assertRaises(RuntimeError): self.adapter.outcome_sequence()


if __name__ == '__main__': unittest.main()
