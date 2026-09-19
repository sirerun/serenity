import importlib.util
import json
import random
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("cardinality_headroom.py")
spec = importlib.util.spec_from_file_location("cardinality_headroom", SCRIPT)
ch = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ch)

REPO = Path(__file__).resolve().parents[2]
EVIDENCE = REPO / "docs" / "launch" / "evidence" / "T23.60"
WORKLOAD = json.loads(ch.WORKLOAD.read_text())


class Fixture(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.client = ch._load_client()
        cls.accounts = cls.client.harness.build_accounts(WORKLOAD["cardinalities"], "paid")
        cls.arrivals = cls.client.harness.generate_arrivals(WORKLOAD, cls.accounts, random.Random(cls.client.BASE_SEED))
        cls.phases = [p["name"] for p in WORKLOAD["phases"]]
        cls.caps = {a["id"]: a["allowance"]["memories"] for a in cls.accounts}


class ReplayAtCapTests(Fixture):
    def test_the_ratified_cap_total_is_90000_over_14_accounts(self):
        self.assertEqual(sum(self.caps.values()), 90_000)
        self.assertEqual(len(self.accounts), 14)

    def test_the_root_replay_numbers_reproduce_exactly(self):
        """Root's coordinator replay (726 of 7,379 steady requests refused, 1,130 remembers against 396 forgets, 89,993 at the end)."""
        run = ch.replay_at_cap(self.arrivals, self.accounts, self.phases, "forget_removes_any_fact")
        steady = run["by_phase"]["steady"]
        self.assertEqual((steady["offered"], steady["remember_offered"], steady["forget_offered"]), (7_379, 1_130, 396))
        self.assertEqual((steady["remember_refused_at_cap"], steady["remember_accepted"]), (726, 404))
        self.assertEqual(steady["refused_pct_of_all_offered"], 9.8387)
        self.assertEqual(steady["completion_ceiling_pct"], 90.1613)  # below the frozen 99% before any other failure.
        self.assertEqual((run["by_phase"]["warmup"]["remember_refused_at_cap"], run["by_phase"]["burst"]["remember_refused_at_cap"]), (246, 256))
        self.assertEqual((run["starting_memories"], run["ending_memories"]), (90_000, 89_993))

    def test_the_current_client_at_the_cap_can_neither_remember_nor_forget(self):
        run = ch.replay_at_cap(self.arrivals, self.accounts, self.phases, "current_client")
        steady = run["by_phase"]["steady"]
        self.assertEqual((steady["remember_accepted"], steady["forget_dispatched"]), (0, 0))
        self.assertEqual((steady["remember_refused_at_cap"], steady["forget_skipped_no_id"]), (1_130, 396))
        self.assertEqual(run["ending_memories"], 90_000)
        self.assertEqual(steady["completion_ceiling_pct"], 79.3197)

    def test_the_frozen_completion_threshold_is_missed_by_the_memory_count_rule_alone(self):
        floor = WORKLOAD["thresholds"]["min_offered_completion_pct"]
        for model in ("forget_removes_any_fact", "current_client"):
            steady = ch.replay_at_cap(self.arrivals, self.accounts, self.phases, model)["by_phase"]["steady"]
            self.assertLess(steady["completion_ceiling_pct"], floor, model)

    def test_a_replay_that_is_offered_no_remember_refuses_nothing(self):
        only_recalls = [a for a in self.arrivals if a["verb"] == "recall"]
        run = ch.replay_at_cap(only_recalls, self.accounts, self.phases, "forget_removes_any_fact")
        self.assertEqual(sum(s["remember_refused_at_cap"] for s in run["by_phase"].values()), 0)


class HeadroomTests(Fixture):
    def start_below_cap(self, peaks):
        return {k: self.caps[k] - peaks.get(k, 0) for k in self.caps}

    def test_the_headroom_is_sufficient_and_a_slot_less_is_not(self):
        """Starting each account at cap minus its peak net growth refuses nothing; one slot fewer refuses at least one remember."""
        for model in ("forget_removes_any_fact", "current_client"):
            for scope in ch.SCOPES:
                scoped = ch.scope_arrivals(self.arrivals, scope)
                peaks = ch.headroom_needed(scoped, self.accounts, model)
                names = sorted({a["phase"] for a in scoped}, key=self.phases.index)
                enough = ch.replay_at_cap(scoped, self.accounts, names, model, self.start_below_cap(peaks))
                self.assertEqual(sum(s["remember_refused_at_cap"] for s in enough["by_phase"].values()), 0, (model, scope))
                for account, peak in peaks.items():
                    if peak == 0:
                        continue
                    start = self.start_below_cap(peaks)
                    start[account] += 1
                    short = ch.replay_at_cap(scoped, self.accounts, names, model, start)
                    self.assertGreater(sum(s["remember_refused_at_cap"] for s in short["by_phase"].values()), 0, (model, scope, account))

    def test_the_headroom_grows_with_the_scope_and_is_a_small_share_of_the_cap(self):
        data = ch.build(WORKLOAD, self.client)["headroom_needed_to_avoid_every_memory_count_refusal"]["forget_removes_any_fact"]
        totals = [data[scope]["total_headroom"] for scope in ch.SCOPES]
        self.assertEqual(totals, sorted(totals))
        self.assertEqual(totals, [737, 972, 1_228, 3_670])
        self.assertEqual(data["one_repetition"]["starting_memories_if_each_account_starts_at_cap_minus_headroom"], 90_000 - 1_228)
        self.assertEqual(data["one_repetition"]["by_account"]["scale-0"]["headroom"], 626)  # the hot tenant.
        self.assertEqual(data["one_repetition"]["ending_memories_after_the_scope"], 89_993)

    def test_the_hot_tenant_is_the_scale_account(self):
        self.assertEqual(self.accounts[0]["id"], "scale-0")


class EvidenceTests(Fixture):
    def test_the_committed_evidence_regenerates_byte_identically(self):
        expected = json.dumps(ch.build(WORKLOAD, self.client), indent=2) + "\n"
        self.assertEqual((EVIDENCE / "cardinality-headroom.json").read_text(), expected)

    def test_the_evidence_says_it_is_not_a_measured_failure(self):
        data = json.loads((EVIDENCE / "cardinality-headroom.json").read_text())
        self.assertIn("Not a measured runtime failure", data["scope"])
        self.assertEqual(data["workload_sha256"], __import__("hashlib").sha256(ch.WORKLOAD.read_bytes()).hexdigest())


if __name__ == "__main__":
    unittest.main()
