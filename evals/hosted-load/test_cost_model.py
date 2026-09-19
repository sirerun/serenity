import contextlib
import copy
import hashlib
import importlib.util
import io
import json
import math
import re
import sys
import tempfile
import unittest
from datetime import datetime, timezone
from decimal import Decimal
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
SCRIPT = REPO / "scripts" / "hosted" / "cost_model.py"
EVIDENCE = REPO / "docs" / "launch" / "evidence" / "T23.60"
MANIFEST = EVIDENCE / "manifest.json"
spec = importlib.util.spec_from_file_location("cost_model", SCRIPT)
cost_model = importlib.util.module_from_spec(spec)
spec.loader.exec_module(cost_model)

NOW = datetime(2026, 9, 19, 12, 0, tzinfo=timezone.utc)
SHA = "a" * 64
FULL = "full_limit_mix"
HOSTILE = "SECRET-TOKEN-do-not-echo"


def record(scenario=FULL, quantity="snapshot_size_bytes", unit=None, value=1_000_000_000, **over):
    """A valid measurement record; keyword overrides replace a field, and None is a value like any other."""
    unit = unit or cost_model.MEASURED_QUANTITIES[quantity]["unit"]
    out = {
        "scenario": scenario, "quantity": quantity, "unit": unit, "value": value,
        "observed_at": "2026-09-01T00:00:00Z",
        "source": {"kind": "backup_snapshot_listing", "ref": "evidence/T23.60/snapshot.json", "sha256": SHA},
    }
    out.update(over)
    return out


def document(*records):
    return {"schema": cost_model.MEASUREMENT_SCHEMA, "schema_version": 1, "records": list(records)}


def run_cli(*argv):
    """Run main() with argv; return (exit code, stdout parsed as JSON)."""
    out = io.StringIO()
    saved = sys.argv
    sys.argv = ["cost_model.py", *argv]
    try:
        with contextlib.redirect_stdout(out):
            code = cost_model.main()
    finally:
        sys.argv = saved
    return code, json.loads(out.getvalue())


def catalog_prices(doc, code):
    """Every price a receipt states for a rate code: an EC2 product row or a price dimension."""
    found = []

    def walk(node):
        if isinstance(node, dict):
            if node.get("rateCode") == code and "price" in node:
                found.append(Decimal(node["price"]))
            dim = node.get(code)
            if isinstance(dim, dict) and "pricePerUnit" in dim:
                found.append(Decimal(dim["pricePerUnit"]["USD"]))
            for value in node.values():
                walk(value)
        elif isinstance(node, list):
            for value in node:
                walk(value)

    walk(doc)
    return found


class RateTableTests(unittest.TestCase):
    def test_every_named_rate_has_a_kind_and_is_never_silently_measured(self):
        for name, entry in cost_model.RATE_TABLE.items():
            self.assertIn("kind", entry, name)
            self.assertIn(entry["kind"], ("published", "constant"), name)

    def test_every_published_rate_carries_a_source_and_a_verification_label(self):
        for name, entry in cost_model.RATE_TABLE.items():
            if entry["kind"] == "published":
                self.assertTrue(entry.get("source"), name)
                self.assertIn(entry.get("verification"), cost_model.VERIFICATIONS, name)

    def test_unknown_rates_have_explicit_reasons_not_omitted(self):
        for name, reason in cost_model.UNKNOWN_RATES.items():
            self.assertTrue(reason, name)

    def test_t4g_burst_credit_matches_official_rate_not_the_old_wrong_recollection(self):
        """Regression: an earlier PARTIAL receipt used an unverified $0.05/vCPU-hour recollection."""
        self.assertEqual(cost_model.rate("ec2_t4g_burst_credit_usd_per_vcpu_hour"), 0.04)

    def test_s3_rate_is_the_us_west_2_figure_not_us_east_1(self):
        self.assertEqual(cost_model.rate("s3_standard_usd_per_gb_month"), 0.023)

    def test_ec2_hourly_rate_is_a_primary_regional_receipt_not_a_third_party_parity_assumption(self):
        entry = cost_model.RATE_TABLE["ec2_t4g_small_usd_per_hour"]
        self.assertEqual(entry["value"], 0.0168)
        self.assertEqual(entry["verification"], cost_model.PRIMARY)
        self.assertNotIn("region_caveat", entry)
        self.assertNotIn("economize", json.dumps(entry).lower())
        self.assertNotIn("parity", json.dumps(entry).lower())
        self.assertEqual(entry["receipt"]["rate_code"], "NTSJZ6S2KD2YFRVB.JRTCKXETXF.6YS6EN2CT7")
        self.assertIn("2026-09-18T20:33:44Z", entry["source"])

    def test_unverified_rates_stay_labeled_as_such(self):
        for name in ("s3_put_usd_per_1000_requests", "s3_get_usd_per_1000_requests", "data_transfer_out_usd_per_gb", "data_transfer_out_free_gb_per_month"):
            self.assertEqual(cost_model.RATE_TABLE[name]["verification"], cost_model.SECONDARY, name)
        for name in ("resend_free_emails_per_month", "resend_free_emails_per_day_cap", "resend_pro_usd_per_month_low"):
            self.assertEqual(cost_model.RATE_TABLE[name]["verification"], cost_model.VENDOR, name)

    def test_every_primary_rate_names_a_receipt_and_carries_no_stale_third_party_label(self):
        for name, entry in cost_model.RATE_TABLE.items():
            if entry.get("verification") != cost_model.PRIMARY:
                continue
            self.assertIn(entry["receipt"]["file"], cost_model.RATE_RECEIPTS, name)
            text = json.dumps(entry).lower()
            for stale in ("websearch", "worker search", "economize", "parity", "region_caveat", "us-east-1 confirmed"):
                self.assertNotIn(stale, text, name)


class RateReceiptTests(unittest.TestCase):
    """The rate table is checked against the receipt files kept beside the evidence, not against itself."""

    def test_every_receipt_file_matches_its_recorded_hash(self):
        for path, info in cost_model.RATE_RECEIPTS.items():
            data = (EVIDENCE / path).read_bytes()
            self.assertEqual(hashlib.sha256(data).hexdigest(), info["sha256"], path)

    def test_every_primary_rate_equals_the_receipt_price_times_its_unit_factor(self):
        checked = 0
        for name, entry in cost_model.RATE_TABLE.items():
            if entry.get("verification") != cost_model.PRIMARY:
                continue
            receipt = entry["receipt"]
            doc = json.loads((EVIDENCE / receipt["file"]).read_text())
            code = receipt["rate_code"]
            if code.startswith("sku:"):
                sku = code[4:].split("@")[0]
                prices = [Decimal(r["usd"]) for r in doc["rates"] if r["sku"] == sku and r["begin_range"] == "0"]
            else:
                prices = catalog_prices(doc, code)
            self.assertEqual(len(prices), 1, f"{name}: {code} must appear once in {receipt['file']}")
            self.assertEqual(prices[0], Decimal(receipt["catalog_usd"]), name)
            if name == "kms_free_requests_per_month":
                continue  # a free tier: the catalog price is 0 and the value is the range end, checked below.
            self.assertAlmostEqual(float(prices[0]) * receipt["catalog_to_rate_factor"], entry["value"], places=9, msg=name)
            checked += 1
        self.assertEqual(checked, 15)

    def test_the_global_kms_free_tier_is_the_receipt_range_end_and_priced_at_zero(self):
        doc = json.loads((EVIDENCE / cost_model.RATE_TABLE["kms_free_requests_per_month"]["receipt"]["file"]).read_text())
        dims = [d for s in doc["sources"] for terms in s["terms"].values() for t in terms.values() for d in t["priceDimensions"].values() if d["rateCode"] == "VTFSAGP364M6QE4A.A429C66SYZ.7K6Z2V4Y4Q"]
        self.assertEqual(len(dims), 1)
        self.assertEqual(dims[0]["endRange"], str(cost_model.rate("kms_free_requests_per_month")))
        self.assertEqual(Decimal(dims[0]["pricePerUnit"]["USD"]), 0)
        self.assertIn("Global", dims[0]["description"])

    def test_gp3_throughput_keeps_explicit_units_and_the_catalog_conversion(self):
        entry = cost_model.RATE_TABLE["ebs_gp3_extra_throughput_usd_per_mibps_month"]
        self.assertNotIn("ebs_gp3_extra_throughput_usd_per_mbps_month", cost_model.RATE_TABLE)
        self.assertEqual(entry["receipt"]["catalog_unit"], "GiBps-mo")
        self.assertEqual(Decimal(entry["receipt"]["catalog_usd"]), Decimal("40.96"))
        self.assertEqual(entry["receipt"]["catalog_to_rate_factor"], 1 / 1024)
        self.assertEqual(entry["value"], 0.04)
        self.assertAlmostEqual(40.96 / 1024, 0.04)

    def test_per_request_catalog_prices_are_converted_to_per_10000(self):
        self.assertEqual(cost_model.RATE_TABLE["kms_usd_per_10000_requests"]["receipt"]["catalog_usd"], "0.0000030000")
        self.assertEqual(cost_model.rate("kms_usd_per_10000_requests"), 0.03)
        self.assertEqual(cost_model.RATE_TABLE["secrets_manager_usd_per_10000_calls"]["receipt"]["catalog_usd"], "0.0000050000")
        self.assertEqual(cost_model.rate("secrets_manager_usd_per_10000_calls"), 0.05)

    def test_regional_receipt_sources_match_the_recorded_upstream_hashes(self):
        doc = json.loads((EVIDENCE / "aws-rates/aws-regional-rates-selected.json").read_text())
        recorded = {s["name"]: s for s in cost_model.RATE_RECEIPTS["aws-rates/aws-regional-rates-selected.json"]["sources"]}
        self.assertEqual({s["service"] for s in doc["sources"]}, set(recorded))
        for source in doc["sources"]:
            info = recorded[source["service"]]
            self.assertEqual((source["sha256"], source["version"], source["publicationDate"]), (info["sha256"], info["version"], info["publication_date"]))
        self.assertEqual(doc["region"], "us-west-2")

    def test_ec2_receipt_names_us_west_oregon_linux_t4g_small_on_demand(self):
        doc = json.loads((EVIDENCE / "aws-rates/ec2-oregon-rate-receipt.json").read_text())
        self.assertEqual(doc["product"]["Instance Type"], "t4g.small")
        self.assertEqual(doc["product"]["Location"], "US West (Oregon)")
        self.assertEqual(doc["product"]["Operating System"], "Linux")
        self.assertEqual(doc["source_sha256"], cost_model.RATE_RECEIPTS["aws-rates/ec2-oregon-rate-receipt.json"]["sources"][0]["sha256"])
        self.assertEqual(doc["manifest"]["hawkFilePublicationDate"], cost_model.RATE_RECEIPTS["aws-rates/ec2-oregon-rate-receipt.json"]["sources"][0]["publication_date"])

    def test_kms_rotation_count_matches_the_saved_pricing_page_wording_and_the_template_enables_rotation(self):
        template = json.loads((REPO / "deploy" / "hosted" / "stack.json").read_text())
        keys = [r for r in template["Resources"].values() if r["Type"] == "AWS::KMS::Key"]
        self.assertEqual(len(keys), 1)
        self.assertIs(keys[0]["Properties"]["EnableKeyRotation"], True)
        self.assertEqual(cost_model.rate("kms_rotation_billed_versions_max"), 2)
        self.assertEqual(cost_model.rate("kms_key_usd_per_month") * (1 + cost_model.rate("kms_rotation_billed_versions_max")), 3.0)

    def test_the_template_sets_no_credit_specification(self):
        """The credit-mode exposure is reported as unverified only while this holds."""
        text = (REPO / "deploy" / "hosted" / "stack.json").read_text()
        self.assertNotIn("CreditSpecification", text)
        self.assertNotIn("CpuCredits", text)


class PlanTableTests(unittest.TestCase):
    def test_plan_allowances_match_the_go_plan_table(self):
        source = (REPO / "internal" / "hosted" / "plans" / "plans.go").read_text()
        rows = re.findall(r'\{"(\w+)", (\d+), (\d+), (\d+), (\d+), (\d+), (\d+), (\d+)\}', source)
        self.assertEqual({r[0] for r in rows}, set(cost_model.PLAN_ALLOWANCES))
        for plan, _cents, brains, memories, writes, recalls, tokens, storage in rows:
            allowance = cost_model.PLAN_ALLOWANCES[plan]
            self.assertEqual(
                (allowance["brains"], allowance["memories"], allowance["writes"], allowance["recalls"], allowance["input_tokens"], allowance["storage_bytes"]),
                (int(brains), int(memories), int(writes), int(recalls), int(tokens), int(storage)), plan)

    def test_the_ratified_mix_has_90000_memories_and_29_allowed_brains(self):
        mix = cost_model.SCENARIOS[FULL]["mix"]
        self.assertEqual(mix, {"free": 10, "builder": 3, "scale": 1})
        self.assertEqual(sum(n * cost_model.PLAN_ALLOWANCES[p]["memories"] for p, n in mix.items()), 90_000)
        totals, _ = cost_model.account_totals(mix, 1.0)
        self.assertEqual((totals["accounts"], totals["brains_allowed"]), (14, 29))


class AccountTotalsTests(unittest.TestCase):
    def test_idle_scenario_has_zero_accounts_and_zero_usage(self):
        totals, per_plan = cost_model.account_totals({}, 0.0)
        self.assertEqual(totals["accounts"], 0)
        self.assertEqual(totals["storage_bytes"], 0.0)
        self.assertEqual(per_plan, {})

    def test_full_limit_mix_totals_nine_gb_storage(self):
        totals, per_plan = cost_model.account_totals({"free": 10, "builder": 3, "scale": 1}, 1.0)
        self.assertEqual(totals["accounts"], 14)
        # 10*100MB + 3*1GB + 1*5GB = 9 GB customer storage at full limit.
        self.assertAlmostEqual(totals["storage_bytes"], 9_000_000_000, delta=1)
        self.assertEqual(set(per_plan), {"free", "builder", "scale"})
        self.assertEqual(per_plan["scale"]["accounts"], 1)

    def test_usage_fraction_scales_linearly(self):
        full, _ = cost_model.account_totals({"builder": 1}, 1.0)
        half, _ = cost_model.account_totals({"builder": 1}, 0.5)
        self.assertAlmostEqual(half["recalls"], full["recalls"] / 2)


class BackupRetentionMathTests(unittest.TestCase):
    """The real driver is Expiration(30d) then NoncurrentVersionExpiration(30d) in series: roughly a
    60-day object lifetime, ~1440 retained hourly backup-sets at steady state, not ~720 same-key versions."""

    def test_retained_backup_sets_is_about_sixty_days_not_thirty(self):
        self.assertEqual(cost_model.S3_OBJECT_LIFETIME_DAYS, 60)
        self.assertEqual(cost_model.RETAINED_BACKUP_SETS, 60 * 24 + 1)
        self.assertGreater(cost_model.RETAINED_BACKUP_SETS, 1400)
        self.assertLess(cost_model.RETAINED_BACKUP_SETS, 1500)

    def test_the_baseline_retention_is_the_deployed_template_not_a_thinning_proposal(self):
        template = json.loads((REPO / "deploy" / "hosted" / "stack.json").read_text())
        rules = [r for res in template["Resources"].values() if res["Type"] == "AWS::S3::Bucket" for r in res["Properties"].get("LifecycleConfiguration", {}).get("Rules", [])]
        self.assertEqual([(r["ExpirationInDays"], r["NoncurrentVersionExpiration"]["NoncurrentDays"]) for r in rules], [(30, 30)])
        self.assertEqual(cost_model.RETAINED_BACKUP_SETS, (30 + 30) * 24 + 1)

    def test_objects_per_backup_counts_manifest_control_db_complete_and_bundles(self):
        scenario = cost_model.price_scenario("t", {"mix": {"free": 2}, "usage_fraction": 1.0, "assumption": "t"}, None)
        # 2 accounts, default brain only -> manifest.json + control.db + COMPLETE + 2 bundles = 5.
        self.assertEqual(scenario["known_categories_usd"]["s3_storage_backups"]["objects_per_backup"], 5)

    def test_the_full_limit_mix_counts_all_29_allowed_brains_not_the_14_primary_ones(self):
        backups = cost_model.price_scenario(FULL, cost_model.SCENARIOS[FULL], None)["known_categories_usd"]
        self.assertEqual(backups["s3_storage_backups"]["brains_in_backup"], 29)
        self.assertEqual(backups["s3_storage_backups"]["objects_per_backup"], 3 + 29)
        self.assertEqual(backups["s3_requests"]["monthly_put_count"], cost_model.BACKUPS_PER_MONTH * 32)

    def test_a_scale_only_mix_with_all_allowed_brains_counts_ten_bundles(self):
        spec = {"mix": {"scale": 1}, "usage_fraction": 1.0, "brains": cost_model.ALL_ALLOWED_BRAINS, "assumption": "t"}
        self.assertEqual(cost_model.price_scenario("t", spec, None)["known_categories_usd"]["s3_storage_backups"]["objects_per_backup"], 13)
        spec["brains"] = cost_model.ONE_DEFAULT_BRAIN
        self.assertEqual(cost_model.price_scenario("t", spec, None)["known_categories_usd"]["s3_storage_backups"]["objects_per_backup"], 4)

    def test_light_scenarios_state_their_default_brain_only_assumption_and_show_the_ceiling(self):
        ten = cost_model.price_scenario("10_accounts_light", cost_model.SCENARIOS["10_accounts_light"], None)
        backups = ten["known_categories_usd"]["s3_storage_backups"]
        self.assertEqual((backups["brains_in_backup"], backups["allowed_brains_ceiling"]), (10, 8 * 1 + 2 * 3))
        self.assertIn("default brain only", ten["assumption"])

    def test_the_idle_scenario_has_no_brains_and_a_three_object_backup(self):
        idle = cost_model.price_scenario("idle", cost_model.SCENARIOS["idle_0_accounts"], None)
        self.assertEqual(idle["known_categories_usd"]["s3_storage_backups"]["objects_per_backup"], 3)


class PriceScenarioTests(unittest.TestCase):
    def test_storage_amplification_scales_with_customer_data_not_flat(self):
        """Doubling the customer data size must roughly double the S3 backup storage cost, proving the
        model reads scenario size rather than returning a constant."""
        small = cost_model.price_scenario("s", {"mix": {"free": 1}, "usage_fraction": 1.0, "assumption": "t"}, None)
        big = cost_model.price_scenario("b", {"mix": {"free": 10}, "usage_fraction": 1.0, "assumption": "t"}, None)
        self.assertGreater(big["known_categories_usd"]["s3_storage_backups"]["value"], small["known_categories_usd"]["s3_storage_backups"]["value"] * 5)

    def test_idle_scenario_has_no_storage_risk(self):
        idle = cost_model.price_scenario("idle", cost_model.SCENARIOS["idle_0_accounts"], None)
        self.assertFalse(idle["storage_risk"]["exceeds_usable_data_volume"])

    def test_idle_cheaper_than_full_limit_mix(self):
        idle = cost_model.price_scenario("idle", cost_model.SCENARIOS["idle_0_accounts"], None)
        full = cost_model.price_scenario("full", cost_model.SCENARIOS[FULL], None)
        self.assertLess(idle["known_subtotal_usd"], full["known_subtotal_usd"])

    def test_embeddings_reported_as_unknown_not_zero_and_includes_query_and_readiness_components(self):
        full = cost_model.price_scenario("full", cost_model.SCENARIOS[FULL], None)
        embed = full["unknown_categories"]["embeddings"]
        self.assertIsNone(embed["monthly_usd"])
        self.assertGreater(embed["write_tokens"]["value"], 0)
        self.assertGreater(embed["query_tokens"]["value"], 0)
        self.assertGreater(embed["readiness_tokens"]["value"], 0)
        self.assertEqual(embed["total_tokens"], embed["write_tokens"]["value"] + embed["query_tokens"]["value"] + embed["rebuild_reembed_tokens"]["value"] + embed["readiness_tokens"]["value"])

    def test_readiness_probing_is_nonzero_even_at_zero_accounts(self):
        idle = cost_model.price_scenario("idle", cost_model.SCENARIOS["idle_0_accounts"], None)
        self.assertGreater(idle["unknown_categories"]["embeddings"]["readiness_tokens"]["value"], 0)

    def test_kms_requests_free_tier_absorbs_light_scenarios(self):
        ten = cost_model.price_scenario("t", cost_model.SCENARIOS["10_accounts_light"], None)
        self.assertEqual(ten["known_categories_usd"]["kms_requests"]["value"], 0.0)

    def test_data_transfer_free_tier_absorbs_all_scenarios_at_these_volumes(self):
        full = cost_model.price_scenario("f", cost_model.SCENARIOS[FULL], None)
        self.assertEqual(full["known_categories_usd"]["data_transfer_out"]["value"], 0.0)

    def test_email_within_resend_free_tier_at_full_limit_mix(self):
        full = cost_model.price_scenario("f", cost_model.SCENARIOS[FULL], None)
        self.assertTrue(full["known_categories_usd"]["email"]["within_free_tier"])
        self.assertEqual(full["known_categories_usd"]["email"]["value"], 0.0)

    def test_per_plan_contribution_apportions_variable_costs_by_storage_share(self):
        full = cost_model.price_scenario("f", cost_model.SCENARIOS[FULL], None)
        by_plan = full["per_plan_contribution"]["by_plan"]
        self.assertEqual(set(by_plan), {"free", "builder", "scale"})
        total = sum(p["variable_cost_usd"] for p in by_plan.values())
        self.assertAlmostEqual(total, full["per_plan_contribution"]["variable_total_usd"], places=2)
        self.assertGreater(by_plan["scale"]["variable_cost_usd"], by_plan["free"]["variable_cost_usd"])

    def test_per_plan_contribution_empty_for_idle(self):
        idle = cost_model.price_scenario("idle", cost_model.SCENARIOS["idle_0_accounts"], None)
        self.assertEqual(idle["per_plan_contribution"]["by_plan"], {})

    def test_known_subtotal_is_the_sum_of_the_known_lines_and_excludes_every_sensitivity(self):
        full = cost_model.price_scenario("f", cost_model.SCENARIOS[FULL], None)
        total = sum((v["value"] if isinstance(v, dict) else v) or 0.0 for v in full["known_categories_usd"].values())
        self.assertAlmostEqual(full["known_subtotal_usd"], total, places=2)
        self.assertGreater(full["unknown_categories"]["control_db_lifetime_growth"]["sensitivity_by_horizon"][-1]["added_s3_backup_storage_usd_per_month"], 0)
        self.assertNotIn("control_db_lifetime_growth", full["known_categories_usd"])


class MeasurementRecordTests(unittest.TestCase):
    def check(self, rec, fragment=None):
        with self.assertRaises(cost_model.MeasurementError) as caught:
            cost_model.validate_record(rec, "records[0]", None, NOW)
        if fragment:
            self.assertIn(fragment, str(caught.exception))
        return str(caught.exception)

    def test_a_valid_record_is_normalized_with_its_scenario_unit_and_source(self):
        out = cost_model.validate_record(record(), "records[0]", FULL, NOW)
        self.assertEqual((out["scenario"], out["quantity"], out["unit"], out["value"]), (FULL, "snapshot_size_bytes", "bytes", 1_000_000_000))
        self.assertEqual(out["observed_at"], "2026-09-01T00:00:00Z")
        self.assertEqual(set(out["source"]), {"kind", "ref", "sha256"})

    def test_negative_snapshot_is_rejected_not_priced_as_a_negative_bill(self):
        """Reproduction: snapshot_size_bytes=-9000000000 was accepted and produced known_subtotal_usd -275.30."""
        self.check(record(value=-9_000_000_000), "value")

    def test_boolean_values_are_rejected_not_read_as_one_byte(self):
        for boolean in (True, False):
            self.check(record(value=boolean), "JSON number")
            self.check(record(quantity="ec2_surplus_credit_vcpu_hours", value=boolean), "JSON number")

    def test_zero_and_below_range_snapshots_are_rejected(self):
        self.check(record(value=0), "value")
        self.check(record(quantity="s3_put_requests_per_month", value=0), "value")

    def test_non_finite_and_oversized_numbers_are_rejected(self):
        for bad in (float("nan"), float("inf"), float("-inf"), 1e999, 10 ** 400, 2 ** 63, 30_000_000_001):
            self.check(record(quantity="snapshot_size_bytes", value=bad))
        for bad in (float("nan"), float("inf"), 10 ** 400, 1168.5, -0.5):
            self.check(record(quantity="ec2_surplus_credit_vcpu_hours", value=bad))
        self.check(record(quantity="s3_put_requests_per_month", value=10_000_001))

    def test_wrong_types_and_containers_for_the_value_are_rejected(self):
        for bad in ("9", None, [9], {"v": 9}, "NaN", b"9"):
            self.check(record(value=bad), "JSON number")

    def test_a_float_is_rejected_for_a_byte_or_request_count(self):
        self.check(record(value=1_000_000.0), "JSON integer")
        self.check(record(quantity="s3_put_requests_per_month", value=100.5), "JSON integer")

    def test_surplus_vcpu_hours_accepts_zero_the_ceiling_and_a_fraction(self):
        for value in (0, 0.0, 12.5, 1168, 1168.0):
            cost_model.validate_record(record(quantity="ec2_surplus_credit_vcpu_hours", value=value), "r", FULL, NOW)

    def test_the_unit_must_be_exact_and_present(self):
        for bad in ("GB", "byte", "Bytes", "bytes ", "", None, 1, ["bytes"]):
            rec = record()
            rec["unit"] = bad
            self.check(rec, "unit")
        rec = record()
        del rec["unit"]
        self.check(rec, "keys")
        self.check(record(quantity="s3_put_requests_per_month", unit="bytes", value=10), "unit")

    def test_unknown_or_missing_scenario_and_quantity_are_rejected(self):
        for bad in ("prod", "", None, 4, ["full_limit_mix"]):
            self.check(record(scenario=bad), "scenario")
        for bad in ("cpu", "snapshot_size_gb", None, 3):
            rec = record()
            rec["quantity"] = bad
            self.check(rec, "quantity")
        rec = record()
        del rec["scenario"]
        self.check(rec, "keys")

    def test_a_record_for_another_scenario_is_rejected_when_pricing_one(self):
        with self.assertRaises(cost_model.MeasurementError):
            cost_model.validate_record(record(scenario="idle_0_accounts"), "r", FULL, NOW)

    def test_extra_keys_are_rejected(self):
        rec = record()
        rec["note"] = "extra"
        self.check(rec, "keys")

    def test_source_identity_is_required_and_strict(self):
        for bad_source in (None, "backup", [], {}, {"kind": "k", "ref": "r"}, {"kind": "k", "ref": "r", "sha256": SHA, "x": 1}):
            self.check(record(source=bad_source), "source")
        base = {"kind": "backup_snapshot_listing", "ref": "evidence/x.json", "sha256": SHA}
        for field, bad in (("kind", "Backup"), ("kind", ""), ("kind", "a b"), ("kind", 1), ("ref", ""), ("ref", "has space"), ("ref", "line\nbreak"), ("ref", "x" * 257), ("ref", 7), ("sha256", "A" * 64), ("sha256", "a" * 63), ("sha256", "g" * 64), ("sha256", 5)):
            self.check(record(source={**base, field: bad}), f"source.{field}")

    def test_observation_time_must_be_a_past_rfc3339_utc_string(self):
        for bad in ("2026-09-01", "2026-09-01T00:00:00", "2026-09-01T00:00:00+02:00", "yesterday", "", None, 20260901, "2026-13-45T00:00:00Z"):
            self.check(record(observed_at=bad), "observed_at")
        self.check(record(observed_at="2999-01-01T00:00:00Z"), "future")
        cost_model.validate_record(record(observed_at="2026-09-19T12:03:00Z"), "r", FULL, NOW)  # within the clock-skew allowance.

    def test_a_record_that_is_not_an_object_is_rejected(self):
        for bad in (None, [], "record", 3, True):
            self.check(bad, "JSON object")

    def test_error_messages_never_echo_the_input(self):
        hostile = {
            "scenario": HOSTILE, "quantity": HOSTILE, "unit": HOSTILE, "value": HOSTILE, "observed_at": HOSTILE,
            "source": {"kind": HOSTILE, "ref": HOSTILE + " x", "sha256": HOSTILE},
        }
        for field in hostile:
            rec = record()
            rec[field] = hostile[field]
            message = self.check(rec)
            self.assertNotIn(HOSTILE, message, field)
        for field in ("kind", "ref", "sha256"):
            rec = record()
            rec["source"][field] = hostile["source"][field]
            self.assertNotIn(HOSTILE, self.check(rec), field)
        self.assertNotIn(HOSTILE, self.check({HOSTILE: 1}))


class MeasurementDocumentTests(unittest.TestCase):
    def check(self, doc, fragment=None):
        with self.assertRaises(cost_model.MeasurementError) as caught:
            cost_model.parse_measurements(doc, NOW)
        if fragment:
            self.assertIn(fragment, str(caught.exception))

    def test_a_valid_document_is_grouped_by_scenario_then_quantity(self):
        out = cost_model.parse_measurements(document(record(), record(quantity="s3_put_requests_per_month", value=5000), record(scenario="idle_0_accounts", value=123456)), NOW)
        self.assertEqual(set(out), {FULL, "idle_0_accounts"})
        self.assertEqual(set(out[FULL]), {"snapshot_size_bytes", "s3_put_requests_per_month"})

    def test_wrong_containers_are_rejected(self):
        for bad in (None, [], [record()], "x", 3, True, {}, {"records": [record()]}):
            self.check(bad, "measurements must be an object")

    def test_a_load_client_result_is_not_a_measurement_document(self):
        load_result = {"mode": "live", "status": "BLOCKED", "calls_used": 0, "tokens_used": 0, "reason": "x"}
        self.check(load_result, "exactly the keys")
        self.check({"schema": "load", "schema_version": 1, "records": [record()]}, "schema must be")

    def test_schema_version_must_be_the_integer_one(self):
        for bad in (True, 2, "1", 1.5, None):
            self.check({"schema": cost_model.MEASUREMENT_SCHEMA, "schema_version": bad, "records": [record()]}, "schema must be")

    def test_records_must_be_a_nonempty_bounded_array(self):
        for bad in (None, {}, "x", 3):
            self.check({"schema": cost_model.MEASUREMENT_SCHEMA, "schema_version": 1, "records": bad}, "JSON array")
        self.check(document(), "empty")
        self.check(document(*[record() for _ in range(cost_model.MAX_MEASUREMENT_RECORDS + 1)]), "more than")

    def test_a_repeated_scenario_and_quantity_is_rejected(self):
        self.check(document(record(), record(value=2_000_000_000)), "repeats")

    def test_one_bad_record_rejects_the_whole_document(self):
        self.check(document(record(), record(quantity="s3_put_requests_per_month", value=-5)), "records[1]")


class MeasuredPricingTests(unittest.TestCase):
    def priced(self, name, *records):
        by_scenario = cost_model.parse_measurements(document(*records), NOW) if records else {}
        return cost_model.price_scenario(name, cost_model.SCENARIOS[name], by_scenario.get(name))

    def test_a_measured_snapshot_prices_exactly_size_times_retained_sets_times_rate(self):
        s = self.priced(FULL, record(value=1_000_000_000))
        s3 = s["known_categories_usd"]["s3_storage_backups"]
        self.assertEqual(s3["value"], round(1.0 * cost_model.RETAINED_BACKUP_SETS * 0.023, 4))
        self.assertEqual(s3["snapshot_size_gb"], 1.0)

    def test_a_measured_snapshot_is_partly_measured_and_names_its_source_and_the_modeled_inputs(self):
        s3 = self.priced(FULL, record())["known_categories_usd"]["s3_storage_backups"]
        self.assertEqual(s3["kind"], "partly_measured")
        self.assertEqual(s3["measured_inputs"], ["snapshot_size_bytes"])
        self.assertIn("retained_backup_sets", s3["modeled_inputs"])
        self.assertEqual(s3["measurement_source"]["sha256"], SHA)

    def test_a_measurement_changes_only_the_scenario_it_names(self):
        measured = self.priced(FULL, record())
        for other in ("idle_0_accounts", "10_accounts_light", "100_accounts_light"):
            s = self.priced(other)
            self.assertEqual(s["known_categories_usd"]["s3_storage_backups"]["kind"], "assumption", other)
            self.assertEqual(s["measurements_applied"], [], other)
        self.assertEqual([m["quantity"] for m in measured["measurements_applied"]], ["snapshot_size_bytes"])

    def test_measured_puts_are_measured_and_the_kms_line_derived_from_them_stays_an_assumption(self):
        s = self.priced(FULL, record(quantity="s3_put_requests_per_month", value=50_000))
        put = s["known_categories_usd"]["s3_requests"]
        self.assertEqual((put["kind"], put["monthly_put_count"], put["value"]), ("measured", 50_000, round(50_000 / 1000 * 0.005, 4)))
        kms = s["known_categories_usd"]["kms_requests"]
        self.assertEqual(kms["kind"], "assumption")
        self.assertIn("measured s3_put_requests_per_month", kms["derived_from"])
        self.assertEqual(kms["value"], round((50_000 - 20_000) / 10_000 * 0.03, 4))

    def test_measured_surplus_hours_replace_the_mean_case_assumption(self):
        s = self.priced(FULL, record(quantity="ec2_surplus_credit_vcpu_hours", value=0))
        self.assertEqual(s["known_categories_usd"]["ec2_burst_credits"]["value"], 0.0)
        self.assertEqual(s["known_categories_usd"]["ec2_burst_credits"]["kind"], "measured")
        s = self.priced(FULL, record(quantity="ec2_surplus_credit_vcpu_hours", value=100))
        self.assertEqual(s["known_categories_usd"]["ec2_burst_credits"]["value"], 4.0)

    def test_a_measurement_never_touches_the_peak_credit_or_control_database_exposure(self):
        base = self.priced(FULL)
        measured = self.priced(FULL, record(quantity="ec2_surplus_credit_vcpu_hours", value=0))
        self.assertEqual(base["peak_exposure"]["components_usd"]["ec2_surplus_credits_sustained_100_percent_cpu"], measured["peak_exposure"]["components_usd"]["ec2_surplus_credits_sustained_100_percent_cpu"])
        self.assertEqual(base["unknown_categories"]["control_db_lifetime_growth"]["sensitivity_by_horizon"], measured["unknown_categories"]["control_db_lifetime_growth"]["sensitivity_by_horizon"])

    def test_price_scenario_rejects_the_old_bare_number_shape(self):
        """Reproduction: measurements={'snapshot_size_bytes': -9e9} priced a negative bill."""
        for bad in ({"snapshot_size_bytes": -9_000_000_000}, {"snapshot_size_bytes": True}, {"snapshot_size_bytes": 1_000_000}):
            with self.assertRaises(cost_model.MeasurementError):
                cost_model.price_scenario(FULL, cost_model.SCENARIOS[FULL], bad)

    def test_price_scenario_rejects_wrong_containers_and_mismatched_keys(self):
        for bad in ([record()], "x", 3, True):
            with self.assertRaises(cost_model.MeasurementError):
                cost_model.price_scenario(FULL, cost_model.SCENARIOS[FULL], bad)
        with self.assertRaises(cost_model.MeasurementError):
            cost_model.price_scenario(FULL, cost_model.SCENARIOS[FULL], {"s3_put_requests_per_month": record()})
        with self.assertRaises(cost_model.MeasurementError):
            cost_model.price_scenario(FULL, cost_model.SCENARIOS[FULL], {"unknown_quantity": record()})

    def test_price_scenario_rejects_a_record_bound_to_another_scenario(self):
        with self.assertRaises(cost_model.MeasurementError):
            cost_model.price_scenario(FULL, cost_model.SCENARIOS[FULL], {"snapshot_size_bytes": record(scenario="idle_0_accounts")})

    def test_a_bad_measurement_never_yields_a_priced_result(self):
        for bad in (-1, 0, True, float("nan"), 10 ** 400):
            with self.assertRaises(cost_model.MeasurementError):
                cost_model.price_scenario(FULL, cost_model.SCENARIOS[FULL], {"snapshot_size_bytes": record(value=bad)})


class PeakExposureTests(unittest.TestCase):
    def setUp(self):
        self.full = cost_model.price_scenario(FULL, cost_model.SCENARIOS[FULL], None)
        self.peak = self.full["peak_exposure"]

    def test_sustained_and_mean_cpu_surplus_match_the_reviewed_arithmetic(self):
        """t4g.small: 2 vCPU at a 20% baseline. Sustained 100% with exhausted credits bills 2*(1-0.2)*730 vCPU-hours."""
        cpu = cost_model.cpu_credit_exposure()
        self.assertEqual(cpu["instance"], {"type": "t4g.small", "vcpus": 2, "baseline_per_vcpu": 0.2, "credits_earned_per_hour": 24})
        self.assertAlmostEqual(cpu["sustained_100_percent_credits_exhausted"]["surplus_vcpu_hours"], 1168.0)
        self.assertAlmostEqual(cpu["sustained_100_percent_credits_exhausted"]["monthly_usd"], 46.72)
        self.assertAlmostEqual(cpu["mean_70_percent"]["surplus_vcpu_hours"], 730.0)
        self.assertAlmostEqual(cpu["mean_70_percent"]["monthly_usd"], 29.20)
        self.assertEqual(cpu["usd_per_surplus_vcpu_hour"], 0.04)

    def test_the_two_percent_mean_case_is_kept_and_labeled_not_a_peak(self):
        burst = self.full["known_categories_usd"]["ec2_burst_credits"]
        self.assertAlmostEqual(burst["value"], 1.168)
        self.assertIn("not a peak", burst["note"])
        cpu = cost_model.cpu_credit_exposure()
        self.assertIn("not a peak", cpu["mean_case_assumption_in_known_subtotal"]["note"])
        self.assertAlmostEqual(cpu["sustained_100_percent_credits_exhausted"]["monthly_usd"] - burst["value"], 45.552)

    def test_credit_mode_is_reported_unverified_and_neither_mode_is_set(self):
        mode = cost_model.cpu_credit_exposure()["credit_mode"]
        self.assertEqual(mode["status"], "unverified")
        self.assertIn("no CreditSpecification", mode["template"])
        self.assertIn("unlimited or standard", mode["requirement"])
        self.assertIsNone(self.full["unknown_categories"]["ec2_cpu_credit_mode"]["monthly_usd"])
        self.assertNotIn("standard mode is set", json.dumps(mode).lower())

    def test_key_storage_peaks_at_the_first_two_rotations(self):
        self.assertEqual(self.peak["components_usd"]["kms_key_storage_after_two_rotations"], 3.0)
        self.assertEqual(self.full["known_categories_usd"]["kms_key"]["value"], 1.0)
        self.assertIn("3 USD", self.full["known_categories_usd"]["kms_key"]["note"])

    def test_kms_requests_and_transfer_peak_without_any_global_free_allowance(self):
        puts = self.full["known_categories_usd"]["s3_requests"]["monthly_put_count"]
        self.assertEqual(puts, 23_040)
        self.assertAlmostEqual(self.peak["components_usd"]["kms_requests_no_free_allowance"], round(puts / 10_000 * 0.03, 4))
        self.assertAlmostEqual(self.full["known_categories_usd"]["kms_requests"]["value"], round((puts - 20_000) / 10_000 * 0.03, 4))
        self.assertGreater(self.peak["components_usd"]["kms_requests_no_free_allowance"], self.full["known_categories_usd"]["kms_requests"]["value"])
        transfer_gb = (700_000 * 4096 + 40_000 * 1024) / 1_000_000_000
        self.assertAlmostEqual(self.peak["components_usd"]["data_transfer_out_no_free_allowance"], round(transfer_gb * 0.09, 4))
        self.assertEqual(self.full["known_categories_usd"]["data_transfer_out"]["value"], 0.0)

    def test_the_peak_subtotal_replaces_exactly_four_lines_and_adds_nothing_else(self):
        known = self.full["known_categories_usd"]
        replaced = known["ec2_burst_credits"]["value"] + known["kms_key"]["value"] + known["kms_requests"]["value"] + known["data_transfer_out"]["value"]
        components = self.peak["components_usd"]
        expected = self.full["known_subtotal_usd"] - replaced + 3.0 + components["kms_requests_no_free_allowance"] + components["data_transfer_out_no_free_allowance"] + 46.72
        self.assertAlmostEqual(self.peak["peak_known_subtotal_usd_sustained_100_percent_cpu"], expected, delta=0.02)
        self.assertAlmostEqual(self.peak["peak_known_subtotal_usd_sustained_100_percent_cpu"] - self.peak["peak_known_subtotal_usd_mean_70_percent_cpu"], 46.72 - 29.20, places=2)

    def test_the_peak_is_at_least_the_known_subtotal_in_every_scenario(self):
        for name, spec in cost_model.SCENARIOS.items():
            s = cost_model.price_scenario(name, spec, None)
            self.assertGreater(s["peak_exposure"]["peak_known_subtotal_usd_sustained_100_percent_cpu"], s["known_subtotal_usd"], name)
            self.assertGreater(s["peak_exposure"]["peak_known_subtotal_usd_sustained_100_percent_cpu"], s["peak_exposure"]["peak_known_subtotal_usd_mean_70_percent_cpu"], name)

    def test_the_idle_peak_is_the_floor_plus_the_cpu_key_and_request_worst_case(self):
        idle = cost_model.price_scenario("idle", cost_model.SCENARIOS["idle_0_accounts"], None)
        floor = idle["known_subtotal_usd"]
        self.assertAlmostEqual(idle["peak_exposure"]["peak_known_subtotal_usd_sustained_100_percent_cpu"], floor - 1.0 + 3.0 + 46.72 + idle["peak_exposure"]["components_usd"]["kms_requests_no_free_allowance"], delta=0.02)

    def test_the_peak_states_it_is_not_a_full_maximum_and_lists_what_it_leaves_out(self):
        self.assertIs(self.peak["full_maximum_computable"], False)
        for unknown in ("control_db_lifetime_growth", "embeddings", "cloudwatch_logs", "cloudwatch_custom_metrics", "s3_get_list_restore", "s3_multipart_requests", "email_peak_volume"):
            self.assertIn(unknown, self.peak["not_included"])
            self.assertIn(unknown, self.full["unknown_categories"])
        self.assertEqual(self.peak["kind"], "sensitivity_not_forecast")
        self.assertFalse(self.peak["within_60usd_ceiling_peak_known_subtotal"])


class ControlDatabaseTests(unittest.TestCase):
    def test_the_twenty_megabyte_constant_is_a_scenario_assumption_never_a_lifetime_bound(self):
        cdb = cost_model.price_scenario(FULL, cost_model.SCENARIOS[FULL], None)["unknown_categories"]["control_db_lifetime_growth"]
        self.assertEqual(cdb["status"], "unbounded_in_current_runtime")
        self.assertEqual(cdb["assumed_control_db_gb_in_snapshot"], 0.02)
        self.assertIn("never a lifetime bound", cdb["assumed_control_db_kind"])
        self.assertIsNone(cdb["monthly_usd"])
        self.assertIn("not an upper bound", cost_model.UNKNOWN_RATES["control_db_lifetime_growth"])
        self.assertIn("not a forecast and not a bound", cdb["sensitivity_basis"])

    def test_sampled_row_sizes_come_from_the_real_writer_audit_and_are_labeled_a_sample(self):
        cdb = cost_model.price_scenario(FULL, cost_model.SCENARIOS[FULL], None)["unknown_categories"]["control_db_lifetime_growth"]
        self.assertEqual(cdb["sampled_row_bytes"], {"reservation": 301, "audit": 229, "per_write": 1060, "per_recall": 530})
        self.assertIn("cff5bb74012f404b345240e54acd83b62a8fdaeb3a4ae8cf9e0ed8b818a4ef40", cdb["sample"])
        self.assertIn("1,500 writes", cdb["sample"])

    def test_the_full_mix_sensitivity_is_the_sampled_slope_times_usage_times_retention(self):
        cdb = cost_model.price_scenario(FULL, cost_model.SCENARIOS[FULL], None)["unknown_categories"]["control_db_lifetime_growth"]
        monthly = 40_000 * 1060 + 700_000 * 530
        self.assertEqual(cdb["growth_bytes_per_month_at_scenario_usage"], monthly)
        by_months = {h["months"]: h for h in cdb["sensitivity_by_horizon"]}
        self.assertEqual(set(by_months), {1, 12, 24})
        for months, entry in by_months.items():
            grown_gb = months * monthly / 1e9
            self.assertAlmostEqual(entry["control_db_gb"], 0.02 + grown_gb, places=3)
            self.assertAlmostEqual(entry["added_s3_backup_storage_usd_per_month"], grown_gb * 1441 * 0.023, places=1)
        self.assertGreater(by_months[12]["added_s3_backup_storage_usd_per_month"], 100)

    def test_growth_is_not_replaced_by_a_hard_bound_in_the_snapshot_or_the_subtotal(self):
        no_growth = cost_model.price_scenario(FULL, cost_model.SCENARIOS[FULL], None)
        self.assertEqual(no_growth["known_categories_usd"]["s3_storage_backups"]["snapshot_size_gb"], round(9.0 + 0.02, 4))

    def test_an_idle_scenario_grows_nothing_but_is_still_reported_unbounded(self):
        cdb = cost_model.price_scenario("idle", cost_model.SCENARIOS["idle_0_accounts"], None)["unknown_categories"]["control_db_lifetime_growth"]
        self.assertEqual(cdb["growth_bytes_per_month_at_scenario_usage"], 0)
        self.assertEqual(cdb["status"], "unbounded_in_current_runtime")

    def test_multipart_requests_are_a_sensitivity_outside_every_subtotal(self):
        s = cost_model.price_scenario(FULL, cost_model.SCENARIOS[FULL], None)
        mp = s["unknown_categories"]["s3_multipart_requests"]
        parts = math.ceil(9_020_000_000 / (8 * 1024 * 1024))
        self.assertEqual(mp["approx_parts_per_backup"], parts)
        self.assertEqual(mp["extra_put_requests_per_month_beyond_one_per_object"], (parts - 32) * 720)
        self.assertAlmostEqual(mp["sensitivity_usd_per_month"], (parts - 32) * 720 / 1000 * 0.005, places=3)
        self.assertIsNone(mp["monthly_usd"])
        self.assertNotIn("s3_multipart_requests", s["known_categories_usd"])


class CLITests(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.tmp = Path(self._tmp.name)
        self.out = self.tmp / "cost.json"

    def tearDown(self):
        self._tmp.cleanup()

    def measurements(self, content, *, raw=False):
        path = self.tmp / "measurements.json"
        path.write_bytes(content if raw else json.dumps(content).encode())
        return path

    def run_measured(self, path):
        return run_cli("--manifest", str(MANIFEST), "--measurements", str(path), "--output", str(self.out))

    def assert_blocked(self, code, printed):
        self.assertEqual(code, 2)
        self.assertEqual(printed["status"], "BLOCKED")
        self.assertEqual(printed["launch_cost_qualification"], "NOT_QUALIFIED")
        self.assertFalse(self.out.exists(), "a blocked run must not leave an output file that reads as a result")
        self.assertNotIn(HOSTILE, json.dumps(printed))

    def test_blocked_on_missing_manifest(self):
        code, printed = run_cli("--manifest", str(self.tmp / "missing.json"), "--output", str(self.out))
        self.assert_blocked(code, printed)

    def test_blocked_not_crashed_on_invalid_json_manifest(self):
        bad = self.tmp / "manifest.json"
        bad.write_text("{not valid json")
        self.assert_blocked(*run_cli("--manifest", str(bad), "--output", str(self.out)))

    def test_blocked_on_a_manifest_that_is_not_an_object_or_has_a_bad_profile(self):
        for content in ("[]", "3", "null", '"paid"', json.dumps({"profile": 7}), json.dumps({"profile": "Paid Plan!"}), json.dumps({"profile": HOSTILE})):
            path = self.tmp / "manifest.json"
            path.write_text(content)
            self.assert_blocked(*run_cli("--manifest", str(path), "--output", str(self.out)))

    def test_blocked_on_unreadable_binary_deep_or_oversized_input(self):
        (self.tmp / "dir.json").mkdir()
        cases = {
            "directory": self.tmp / "dir.json",
            "binary": self.measurements(b"\xff\xfe\x00 not utf-8", raw=True),
            "deep": self.measurements(b"[" * 200_000, raw=True),
            "oversized": self.measurements(b" " * (cost_model.MAX_MEASUREMENTS_FILE_BYTES + 1), raw=True),
        }
        for label, path in cases.items():
            code, printed = self.run_measured(path)
            self.assert_blocked(code, printed)

    def test_blocked_on_missing_measurements_file(self):
        self.assert_blocked(*self.run_measured(self.tmp / "nope.json"))

    def test_blocked_on_non_finite_literals_and_repeated_keys_in_the_file(self):
        base = json.dumps(document(record()))
        for label, text in {
            "NaN": base.replace("1000000000", "NaN"),
            "Infinity": base.replace("1000000000", "Infinity"),
            "-Infinity": base.replace("1000000000", "-Infinity"),
            "duplicate": base.replace('"scenario"', '"scenario": "idle_0_accounts", "scenario"', 1),
        }.items():
            code, printed = self.run_measured(self.measurements(text.encode(), raw=True))
            self.assert_blocked(code, printed)
            self.assertIn("not valid JSON", printed["reason"], label)

    def test_blocked_on_the_reproduced_negative_and_boolean_snapshots(self):
        for bad in (-9_000_000_000, True, False, 0, 10 ** 400):
            path = self.measurements(document(record(value=bad)))
            code, printed = self.run_measured(path)
            self.assert_blocked(code, printed)
            self.assertIn("measurements rejected", printed["reason"])

    def test_blocked_on_a_load_client_result_an_empty_document_and_an_empty_record_list(self):
        for content in ({"mode": "live", "status": "BLOCKED", "calls_used": 0}, {}, [], document()):
            self.assert_blocked(*self.run_measured(self.measurements(content)))

    def test_blocked_reason_never_echoes_file_content(self):
        rec = record()
        rec["unit"] = HOSTILE
        rec["source"]["ref"] = HOSTILE + " spaced"
        code, printed = self.run_measured(self.measurements(document(rec)))
        self.assert_blocked(code, printed)
        code, printed = self.run_measured(self.measurements(("{" + f'"{HOSTILE}": ').encode() + b"nan}", raw=True))
        self.assert_blocked(code, printed)

    def test_a_successful_run_reports_calculated_not_qualified_and_writes_the_result(self):
        code, printed = run_cli("--manifest", str(MANIFEST), "--output", str(self.out))
        self.assertEqual(code, 0)
        self.assertEqual(printed["status"], "CALCULATED")
        self.assertNotEqual(printed["status"], "PASS")
        self.assertEqual(printed["launch_cost_qualification"], "NOT_QUALIFIED")
        self.assertIs(printed["full_maximum_computable"], False)
        data = json.loads(self.out.read_text())
        self.assertEqual(data["calculation"]["status"], "CALCULATED")
        self.assertEqual(data["launch_cost_qualification"]["status"], "NOT_QUALIFIED")
        self.assertIs(data["launch_cost_qualification"]["full_maximum_computable"], False)
        for unknown in ("control_db_lifetime_growth", "ec2_cpu_credit_mode", "global_free_allowances", "embeddings"):
            self.assertIn(unknown, data["launch_cost_qualification"]["blocking_unknowns"])
        self.assertEqual(data["task_id"], "T23.60")
        self.assertIn("full_limit_mix", data["scenarios"])
        self.assertIn("citation_note", data)
        self.assertTrue(data["scenarios"]["full_limit_mix"]["known_subtotal_usd"] > 60.0)
        self.assertEqual(data["measurements"], {**data["measurements"], "supplied": False, "applied_count": 0})

    def test_a_valid_measurement_is_bound_to_its_scenario_and_source_in_the_output(self):
        path = self.measurements(document(record(value=2_000_000_000)))
        code, printed = self.run_measured(path)
        self.assertEqual(code, 0)
        self.assertEqual((printed["status"], printed["measurements_applied"]), ("CALCULATED", 1))
        data = json.loads(self.out.read_text())
        self.assertEqual(data["measurements"]["schema"], cost_model.MEASUREMENT_SCHEMA)
        self.assertEqual(data["scenarios"][FULL]["known_categories_usd"]["s3_storage_backups"]["kind"], "partly_measured")
        self.assertEqual(data["scenarios"][FULL]["measurements_applied"][0]["source"]["sha256"], SHA)
        for other in ("idle_0_accounts", "10_accounts_light", "100_accounts_light"):
            self.assertEqual(data["scenarios"][other]["known_categories_usd"]["s3_storage_backups"]["kind"], "assumption")
            self.assertEqual(data["scenarios"][other]["measurements_applied"], [])
        self.assertEqual(data["launch_cost_qualification"]["status"], "NOT_QUALIFIED")

    def test_the_committed_evidence_regenerates_byte_identically_without_measurements(self):
        code, _ = run_cli("--manifest", str(MANIFEST), "--output", str(self.out))
        self.assertEqual(code, 0)
        self.assertEqual(self.out.read_bytes(), (EVIDENCE / "cost.json").read_bytes())

    def test_unwritable_output_is_blocked(self):
        blocker = self.tmp / "file"
        blocker.write_text("x")
        code, printed = run_cli("--manifest", str(MANIFEST), "--output", str(blocker / "cost.json"))
        self.assertEqual((code, printed["status"]), (2, "BLOCKED"))


if __name__ == "__main__":
    unittest.main()
