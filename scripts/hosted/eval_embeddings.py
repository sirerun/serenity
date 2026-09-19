#!/usr/bin/env python3
"""T23.43 CLI harness: qualifies (or, absent live credentials, mechanically
rehearses) semantic retrieval quality over the frozen 100-case corpus in
evals/hosted/corpus.json.

See docs/launch/hosted-completion/embedding-eval.md for the full design
(corpus composition, frozen Hit@5 thresholds, freeze receipt) and
docs/launch/hosted-completion/evidence.md for the harness command contract
this file implements (--fixtures/--live, --manifest, --output, exit codes).

Exit codes (evidence.md): 0 = every required assertion passed; 1 = a
tested assertion failed; 2 = blocked/missing prerequisite or exhausted
budget; 3 = invalid input. A result JSON is always written, even on
failure.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parents[1]
EVALS_HOSTED = REPO_ROOT / "evals" / "hosted"
sys.path.insert(0, str(EVALS_HOSTED))

from lib import forgotten_targets, lexical_control, manifest as manifest_lib, mcp_client, scoring, seeding  # noqa: E402
from lib.budget import BudgetGuard  # noqa: E402
from lib.cosine import rank_by_cosine  # noqa: E402
from lib.fixture_embedder import FixedVectorEmbedder, HashBagEmbedder  # noqa: E402

# Uncommitted changes here would make a recorded source_sha name code that
# did not run. Evidence output paths (docs/launch/evidence) are deliberately
# outside this list so writing a result never dirties the check.
DIRTY_CHECK_PATHS = ("evals/hosted", "scripts/hosted")
TARGETS = ((seeding.ROLE_POSITIVE, "hosted_mcp"), (seeding.ROLE_EMPTY_CASE, "empty_case_hosted_mcp"))

EXIT_PASS = 0
EXIT_TESTED_FAILURE = 1
EXIT_BLOCKED = 2
EXIT_INVALID_INPUT = 3


def canonical_sha256(obj) -> str:
    # Same recipe as evals/hosted/corpus_gen.py (canonical_json + sha256_of).
    text = json.dumps(obj, sort_keys=True, ensure_ascii=False, separators=(",", ":"))
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def load_corpus(corpus_path: Path, facts_path: Path):
    corpus = json.loads(corpus_path.read_text(encoding="utf-8"))
    facts = json.loads(facts_path.read_text(encoding="utf-8"))
    # The frozen hashes live in the same file as the cases they pin, so
    # reading meta alone would let an edited case or fact text keep a stale
    # hash. Recompute both and refuse a mismatch: a corpus edit must go
    # through corpus_gen.py and a reviewed new hash, never around it.
    if canonical_sha256(corpus["cases"]) != corpus["meta"]["corpus_hash_sha256"]:
        raise ValueError("corpus.json cases do not hash to its frozen meta.corpus_hash_sha256; the corpus was edited outside corpus_gen.py")
    if canonical_sha256(facts["facts"]) != corpus["meta"]["facts_hash_sha256"]:
        raise ValueError("facts.json does not hash to the frozen meta.facts_hash_sha256; a fact was edited outside corpus_gen.py")
    fact_by_id = {f["id"]: f for f in facts["facts"]}
    return corpus, facts, fact_by_id


def positive_cases(corpus: dict) -> list[dict]:
    return [c for c in corpus["cases"] if c["category"] != "empty"]


def empty_cases(corpus: dict) -> list[dict]:
    return [c for c in corpus["cases"] if c["category"] == "empty"]


def git_source_sha(repo_root: Path) -> str:
    out = subprocess.run(
        ["git", "rev-parse", "HEAD"], cwd=repo_root, capture_output=True, text=True, check=True
    )
    return out.stdout.strip()


def sha256_file(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def chunk_ref_to_fact_id(ref: str) -> str:
    return ref.split(":", 1)[1] if ":" in ref else ref


def rank_with_local_embedder(embedder, query: str, fact_ids: list[str], fact_by_id: dict) -> list[str]:
    qvec = embedder.embed(query)
    candidates = [(fid, embedder.embed(fact_by_id[fid]["text"])) for fid in fact_ids]
    ranked = rank_by_cosine(qvec, candidates)
    return [fid for fid, _score in ranked]


def run_local_scorer_arm(embedder, corpus: dict, fact_by_id: dict, all_fact_ids: list[str]):
    results = []
    for case in positive_cases(corpus):
        ranked = rank_with_local_embedder(embedder, case["query"], all_fact_ids, fact_by_id)
        results.append(
            scoring.score_positive_case(case["id"], case["category"], case["expected_fact_id"], ranked)
        )
    summary = scoring.summarize(results, empty_results=[])
    return results, summary


def run_lexical_arm(serenity_bin: str, seedbrain_bin: str, corpus: dict, fact_by_id: dict):
    pos_cases = positive_cases(corpus)
    all_ids = sorted(
        {c["expected_fact_id"] for c in pos_cases} | {d for c in pos_cases for d in c["distractor_fact_ids"]}
    )
    facts_for_brain = [fact_by_id[fid] for fid in all_ids]
    brain = lexical_control.build_disposable_brain(serenity_bin, seedbrain_bin, facts_for_brain)

    positive_results = []
    for case in pos_cases:
        refs = brain.search(case["query"], limit=scoring.K)
        ranked_ids = [chunk_ref_to_fact_id(r) for r in refs]
        positive_results.append(
            scoring.score_positive_case(case["id"], case["category"], case["expected_fact_id"], ranked_ids)
        )

    empty_results = []
    for case in empty_cases(corpus):
        filler = fact_by_id[case["isolated_filler_fact_id"]]
        isolated = lexical_control.build_disposable_brain(serenity_bin, seedbrain_bin, [filler])
        refs = isolated.search(case["query"], limit=scoring.K)
        ranked_ids = [chunk_ref_to_fact_id(r) for r in refs]
        # Strict: the control brain holds one unrelated filler fact, and
        # returning it counts as a violation like any other result.
        empty_results.append(scoring.score_empty_case(case["id"], ranked_ids))

    return positive_results, empty_results


def lexical_negative_check(pos_semantic_results, pos_lexical_results, corpus):
    """T23.43.md acceptance: "at least 18/20 lexical-negative paraphrases
    hit semantically and miss in the lexical control." A case counts as a
    pass here when the semantic arm hit (expected id in top 5) AND the
    lexical arm missed (expected id absent from its top 5) -- the exact
    property that shows semantic retrieval buys something lexical search
    cannot.
    """
    flagged_ids = {
        c["id"] for c in corpus["cases"] if c["category"] == "paraphrase" and c.get("lacks_content_word_overlap")
    }
    sem_by_id = {r.case_id: r for r in pos_semantic_results}
    lex_by_id = {r.case_id: r for r in pos_lexical_results}
    results = []
    for cid in sorted(flagged_ids):
        sem = sem_by_id.get(cid)
        lex = lex_by_id.get(cid)
        if sem is None or lex is None:
            continue
        passed = sem.hit and not lex.hit
        results.append(
            scoring.CaseResult(cid, "paraphrase", sem.expected_fact_id, sem.ranked_ids, hit=passed, rank=sem.rank)
        )
    return results


def acceptance_row(criterion: str, expected: str, observed: str, status: str) -> dict:
    return {"criterion": criterion, "expected": expected, "observed": observed, "status": status}


def write_result(output_path: Path, result: dict) -> None:
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def now_iso() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def base_result(task_id: str, profile: str, source_sha: str, evidence_level: str) -> dict:
    return {
        "task_id": task_id,
        "profile": profile,
        "status": "NOT_RUN",
        "source_sha": source_sha,
        "recorded_at": now_iso(),
        "evidence_level": evidence_level,
        "dependencies": {},
        "commands": [],
        "cases": {"planned": 0, "executed": 0, "passed": 0, "failed": 0, "skipped": 0, "skip_reasons": []},
        "acceptance": [],
        "artifacts": [],
        "cost": {"actual_usd": None, "approved_max_usd": None, "authorization_refs": []},
        "limitations": [],
        "blockers": [],
        "binary_sha256": None,
        "configuration_sha256": None,
        "corpus_sha256": None,
        "provider": None,
    }


def run_fixtures(args: argparse.Namespace) -> tuple[dict, int]:
    started = time.time()
    corpus, facts, fact_by_id = load_corpus(args.corpus, args.facts)
    manifest_lib.load_manifest(args.manifest)  # unreadable/non-object manifest is invalid input, not silently ignored
    source_sha = git_source_sha(REPO_ROOT)
    all_fact_ids = sorted(fact_by_id.keys())

    result = base_result(args.task_id, args.profile, source_sha, "fixture")
    result["corpus_sha256"] = corpus["meta"]["corpus_hash_sha256"]
    result["configuration_sha256"] = sha256_file(args.manifest)
    result["commands"].append(
        f"python3 {Path(__file__).relative_to(REPO_ROOT)} --fixtures --manifest {args.manifest} --output {args.output}"
    )

    total_cases = len(corpus["cases"])
    result["cases"]["planned"] = total_cases

    # Arm 1: harness-mechanics pass (weak-but-real local embedder). Proves
    # the corpus/scoring/reporting pipeline runs end-to-end over every
    # positive case without crashing.
    hashbag_results, hashbag_summary = run_local_scorer_arm(
        HashBagEmbedder(), corpus, fact_by_id, all_fact_ids
    )
    mechanics_ok = len(hashbag_results) == len(positive_cases(corpus))

    # Arm 2: fixed-vector stub -- required negative control. Must fail the
    # quality floor; if it doesn't, the scoring pipeline itself is broken.
    fixed_results, fixed_summary = run_local_scorer_arm(
        FixedVectorEmbedder(), corpus, fact_by_id, all_fact_ids
    )
    if fixed_summary.overall_floor_pass:
        raise AssertionError(
            "fixed-vector negative control unexpectedly cleared the quality floor -- "
            "the scoring pipeline is broken, not a real provider's quality"
        )

    # Arm 3: real lexical-only control against a disposable brain (never
    # skipped silently -- absence of the pre-built binaries is a recorded,
    # honest limitation).
    lexical_pos = lexical_empty = None
    lexical_summary = None
    try:
        serenity_bin, seedbrain_bin = lexical_control.resolve_binaries(args.serenity_bin, args.seedbrain_bin)
        result["commands"].append(f"{serenity_bin} --root <disposable> search <query> --limit 5")
        lexical_pos, lexical_empty = run_lexical_arm(serenity_bin, seedbrain_bin, corpus, fact_by_id)
        lexical_summary = scoring.summarize(lexical_pos, lexical_empty)
    except lexical_control.LexicalControlUnavailable as e:
        result["limitations"].append(f"lexical-only control arm skipped: {e}")

    executed = len(positive_cases(corpus)) + (len(empty_cases(corpus)) if lexical_empty is not None else 0)
    skipped = total_cases - executed
    result["cases"].update(
        executed=executed,
        passed=executed,  # mechanical completion, not a quality verdict -- see acceptance[] below
        failed=0,
        skipped=skipped,
        skip_reasons=(["empty-case arm requires the lexical control binaries; see limitations"] if skipped else []),
    )

    ln_results = None
    if lexical_pos is not None:
        ln_results = lexical_negative_check(hashbag_results, lexical_pos, corpus)

    observed_quality = (
        f"hashbag(mechanics-only) overall_hit_rate={hashbag_summary.overall_hit_rate:.3f}; "
        f"fixed-vector(must-fail) overall_hit_rate={fixed_summary.overall_hit_rate:.3f}"
    )
    if lexical_summary is not None:
        observed_quality += (
            f"; lexical-control overall_hit_rate={lexical_summary.overall_hit_rate:.3f} "
            f"(none of these three arms is a live semantic-quality measurement)"
        )

    result["acceptance"] = [
        acceptance_row(
            criterion="Hit@5 thresholds (overall>=0.90, category floors, empty leakage=0, lexical-negative>=18/20) frozen before live results",
            expected="Measured only against a live hosted MCP recall endpoint (see --live)",
            observed=observed_quality,
            status="BLOCKED",
        ),
        acceptance_row(
            criterion="Fixed-vector/lexical-only stub fails the quality predicate; fixture mode passes harness mechanics but never marks quality PASS",
            expected="fixed-vector overall_hit_rate < 0.90 AND all 95 positive cases mechanically scored",
            observed=(
                f"fixed-vector overall_hit_rate={fixed_summary.overall_hit_rate:.3f} (<0.90: "
                f"{fixed_summary.overall_hit_rate < scoring.OVERALL_HIT_AT_5_MIN}); "
                f"mechanics completed for {len(hashbag_results)}/{len(positive_cases(corpus))} positive cases"
            ),
            status="PASS" if (mechanics_ok and not fixed_summary.overall_floor_pass) else "FAIL",
        ),
        acceptance_row(
            criterion="Budget exhausted/provider unavailable yields BLOCKED with partial results and no further calls",
            expected="N/A in --fixtures mode (no external budget/provider involved)",
            observed="fixtures mode never calls a live provider; see --live for the budget-enforcement path",
            status="NOT_RUN",
        ),
    ]

    result["limitations"].append(
        "Empty-case fixture arm approximates 'forgotten/expired' as 'never authored': T23.43 does not "
        "own the remember/forget/expiry pipeline (T23.42/44/48), so isolated brains contain only an "
        "unrelated filler fact rather than a genuinely retracted one."
    )
    result["limitations"].append(
        "--live mode's actual semantic quality has never been measured (no EMBEDDINGS key, no seeded "
        "hosted account); this run is fixtures-only mechanical rehearsal."
    )

    dirty = dirty_paths(REPO_ROOT)
    if dirty:
        result["limitations"].append(dirty_message(dirty) + " -- recorded, not blocked, because fixtures qualify nothing")
    result["per_query"] = {
        "hashbag": [case_row(r) for r in hashbag_results],
        "fixed_vector": [case_row(r) for r in fixed_results],
        "lexical": [case_row(r) for r in (lexical_pos or []) + (lexical_empty or [])],
    }
    result["usage"] = {"calls_this_run": 0}
    result["elapsed_seconds"] = round(time.time() - started, 3)
    result["status"] = "PARTIAL"
    result["blockers"] = [
        {
            "owner": "David / EMBEDDINGS gate owner",
            "required_input": "EMBEDDINGS_API_KEY provisioned and T23.42's provider adapter merged, "
            "plus a seeded hosted test account confirmed via a live manifest's "
            "hosted_mcp.corpus_seeded_confirmation field, before --live can run",
        }
    ]

    debug = {
        "hashbag_summary": vars(hashbag_summary),
        "fixed_vector_summary": vars(fixed_summary),
        "lexical_summary": vars(lexical_summary) if lexical_summary else None,
        "lexical_negative_check": (
            {"hits": sum(1 for r in ln_results if r.hit), "total": len(ln_results)} if ln_results else None
        ),
    }
    result["_debug"] = debug

    return result, EXIT_PASS


def build_blocked_result(
    args: argparse.Namespace, corpus: dict, source_sha: str, problems: list[str], *, evidence_level: str = "live-provider"
) -> dict:
    result = base_result(args.task_id, args.profile, source_sha, evidence_level)
    result["corpus_sha256"] = corpus["meta"]["corpus_hash_sha256"]
    result["status"] = "BLOCKED"
    result["cases"]["planned"] = len(corpus["cases"])
    result["cases"]["skipped"] = len(corpus["cases"])
    result["cases"]["skip_reasons"] = problems
    result["acceptance"] = [
        acceptance_row(
            criterion="Budget exhausted/provider unavailable yields BLOCKED with partial results and no further calls",
            expected="Missing/null budget, credential, seed receipt or provider pin refuses the run rather than defaulting to unlimited access",
            observed="; ".join(problems),
            status="PASS",
        )
    ]
    result["blockers"] = [{"owner": "coordinator/David", "required_input": p} for p in problems]
    return result


def dirty_paths(repo_root: Path, paths: tuple[str, ...] = DIRTY_CHECK_PATHS) -> list[str]:
    """Uncommitted changes under the harness's own code. A recorded
    source_sha names a commit; if the code that ran differs from it, the
    evidence would name bytes nobody ran."""
    out = subprocess.run(
        ["git", "status", "--porcelain", "--", *paths], cwd=repo_root, capture_output=True, text=True, check=True
    )
    return [line[3:] for line in out.stdout.splitlines() if line.strip()]


def resolve_receipt_path(manifest_path: Path, raw: str) -> Path:
    p = Path(raw)
    return p if p.is_absolute() else (manifest_path.resolve().parent / p)


def load_target_receipts(m: dict, manifest_path: Path, corpus: dict, facts_sha256: str) -> tuple[dict, list[str]]:
    """Loads and verifies both seed receipts against this manifest, offline.
    Returns ({role: receipt}, problems)."""
    corpus_sha256 = corpus["meta"]["corpus_hash_sha256"]
    receipts: dict[str, dict] = {}
    problems: list[str] = []
    for role, key in TARGETS:
        target = m[key]
        receipt, errs = seeding.load_receipt(
            resolve_receipt_path(manifest_path, target["seed_receipt_path"]), target["corpus_seeded_confirmation"]
        )
        if receipt is not None:
            errs += seeding.verify_receipt(
                receipt,
                role=role,
                corpus=corpus,
                corpus_sha256=corpus_sha256,
                facts_sha256=facts_sha256,
                endpoint_url=target["endpoint_url"],
                credential_secret_ref=target["credential_secret_ref"],
            )
            receipts[role] = receipt
        problems += [f"{key}: {e}" for e in errs]
    return receipts, problems


def credential_problems(m: dict) -> tuple[dict[str, str], list[str]]:
    """Both credentials must be present in the environment and differ (one
    credential is one account). Values are never logged or returned in a
    message -- only the env-var names."""
    tokens: dict[str, str] = {}
    problems: list[str] = []
    for role, key in TARGETS:
        ref = m[key]["credential_secret_ref"]
        val = os.environ.get(ref)
        if not val:
            problems.append(f"${ref} is not set in the environment ({key})")
        else:
            tokens[role] = val
    if len(tokens) == 2 and tokens[seeding.ROLE_POSITIVE] == tokens[seeding.ROLE_EMPTY_CASE]:
        problems.append("hosted_mcp and empty_case_hosted_mcp resolve to the same credential value; they must be separate accounts")
    return tokens, problems


def case_row(r: scoring.CaseResult) -> dict:
    return {
        "case_id": r.case_id,
        "category": r.category,
        "expected_fact_id": r.expected_fact_id,
        "rank": r.rank,
        "hit": r.hit,
        "top5": r.ranked_ids[: scoring.K],
        "forbidden_ids": r.forbidden_ids,
    }


def default_lexical_provider(args, corpus: dict, fact_by_id: dict):
    serenity_bin, seedbrain_bin = lexical_control.resolve_binaries(args.serenity_bin, args.seedbrain_bin)
    return run_lexical_arm(serenity_bin, seedbrain_bin, corpus, fact_by_id)


def provider_record(m: dict) -> dict:
    p = m["provider"]
    return {
        "base_url": p.get("base_url"),
        "model": p.get("model"),
        "version_pin": p.get("version_pin"),
        "dimensions": p.get("dimensions"),
        "serving_provider": p.get("serving_provider"),
        "privacy_review_ref": p.get("privacy_review_ref"),
        "hosted_endpoint": m["hosted_mcp"].get("endpoint_url"),
    }


def guard_check_for(guard: BudgetGuard):
    def check() -> None:
        reason = guard.exhausted_reason()
        if reason:
            raise seeding.SeedBlocked(reason)

    return check


def run_live(args: argparse.Namespace, *, lexical_provider=None) -> tuple[dict, int]:
    """Scores the real hosted recall endpoint. Everything that can be
    checked without spending is checked first (manifest, clean tree, seed
    receipts, credentials, the local lexical control), so a missing
    prerequisite blocks before the first paid call."""
    started = time.time()
    corpus, facts, fact_by_id = load_corpus(args.corpus, args.facts)
    source_sha = git_source_sha(REPO_ROOT)
    actual_hash = corpus["meta"]["corpus_hash_sha256"]

    m = manifest_lib.load_manifest(args.manifest)
    problems = manifest_lib.validate_live_manifest(m, manifest_lib.PHASE_LIVE)
    if problems:
        return build_blocked_result(args, corpus, source_sha, problems), EXIT_BLOCKED
    if m.get("corpus_sha256") != actual_hash:
        return (
            build_blocked_result(
                args, corpus, source_sha,
                [f"manifest corpus_sha256={m.get('corpus_sha256')!r} does not match on-disk corpus={actual_hash!r}"],
            ),
            EXIT_BLOCKED,
        )
    dirty = dirty_paths(REPO_ROOT)
    if dirty:
        return build_blocked_result(args, corpus, source_sha, [dirty_message(dirty)]), EXIT_BLOCKED

    receipts, problems = load_target_receipts(m, args.manifest, corpus, corpus["meta"]["facts_hash_sha256"])
    tokens, cred_problems = credential_problems(m)
    problems += cred_problems
    if problems:
        return build_blocked_result(args, corpus, source_sha, problems), EXIT_BLOCKED

    # The lexical control is local and free; run it before any paid call so
    # a missing binary blocks here and not after the spend.
    try:
        lexical_pos, _lexical_empty = (lexical_provider or (lambda: default_lexical_provider(args, corpus, fact_by_id)))()
    except lexical_control.LexicalControlUnavailable as e:
        return build_blocked_result(args, corpus, source_sha, [f"lexical control unavailable: {e}"]), EXIT_BLOCKED

    guard = BudgetGuard(
        m["budget"],
        prior_calls=sum(r["usage"]["calls_made"] for r in receipts.values()),
        prior_request_bytes=sum(r["usage"]["request_bytes"] for r in receipts.values()),
    )
    check = guard_check_for(guard)

    def run_target(role: str, key: str, cases: list[dict], scorer) -> tuple[list, str | None]:
        target, receipt = m[key], receipts[role]
        client = guard.client(target["endpoint_url"], tokens[role], target["allowed_origins"])
        id_map = receipt["id_map"]
        # The positive account must still hold exactly what was seeded. The
        # empty-case account must hold NOTHING: every target was forgotten.
        expected_current = set(id_map) if role == seeding.ROLE_POSITIVE else set()
        results: list = []
        try:
            check()
            client.initialize()
            # Drift/contamination proof at evaluation time, not only at seed time.
            facts_now = seeding.fetch_inventory(client, check)
            if set(facts_now) != expected_current:
                raise seeding.SeedBlocked(
                    f"{key}: account inventory differs from its seed receipt (extra, missing or replaced facts)"
                )
            for case in cases:
                ranked, fact_ids = seeding.recall_query(client, check, case["query"], id_map)
                if role == seeding.ROLE_POSITIVE and any(x.startswith("unresolved:") for x in ranked):
                    raise seeding.SeedBlocked(
                        f"case {case['id']}: recall returned a result that is not in the seed receipt "
                        "(foreign content in the account, or the recall slug contract changed)"
                    )
                results.append(scorer(case, ranked, fact_ids))
        except (seeding.SeedBlocked, mcp_client.BudgetExceeded) as e:
            return results, str(e)  # authored by this harness, never upstream text
        except mcp_client.MCPError as e:
            return results, f"live call failed: {e}"  # fixed classes only
        return results, None

    positive_results, blocked_reason = run_target(
        seeding.ROLE_POSITIVE,
        "hosted_mcp",
        positive_cases(corpus),
        lambda case, ranked, _facts: scoring.score_positive_case(
            case["id"], case["category"], case["expected_fact_id"], ranked
        ),
    )
    empty_results: list = []
    sentinel_results: list = []
    if blocked_reason is None:
        # The 5 frozen expected-empty queries, then the cross-account
        # sentinel probe. All are scored strictly: any returned result or
        # fact fails. The sentinel is not one of the 5 cases and is reported
        # separately.
        probe = {"id": "sentinel-probe", "query": forgotten_targets.SENTINEL_FACT_TEXT}
        both, blocked_reason = run_target(
            seeding.ROLE_EMPTY_CASE,
            "empty_case_hosted_mcp",
            empty_cases(corpus) + [probe],
            lambda case, ranked, facts_arm: scoring.score_empty_case(case["id"], ranked, facts_arm),
        )
        n_empty = len(empty_cases(corpus))
        empty_results, sentinel_results = both[:n_empty], both[n_empty:]

    ln_results = lexical_negative_check(positive_results, lexical_pos, corpus) if blocked_reason is None else []
    summary = scoring.summarize(positive_results, empty_results, ln_results)

    result = base_result(args.task_id, args.profile, source_sha, "live-provider")
    result["corpus_sha256"] = actual_hash
    result["configuration_sha256"] = sha256_file(args.manifest)
    result["provider"] = provider_record(m)
    result["commands"].append(f"python3 {Path(__file__).relative_to(REPO_ROOT)} --live --manifest {args.manifest} --output {args.output}")
    executed = len(positive_results) + len(empty_results)
    result["cases"].update(
        planned=len(corpus["cases"]),
        executed=executed,
        passed=summary.overall_hits + sum(1 for r in empty_results if r.hit),
        failed=(len(positive_results) - summary.overall_hits) + sum(1 for r in empty_results if not r.hit),
        skipped=len(corpus["cases"]) - executed,
        skip_reasons=[blocked_reason] if blocked_reason else [],
    )
    sentinel_leaked = sum(len(r.forbidden_ids) for r in sentinel_results)
    sentinel_ok = len(sentinel_results) == 1 and sentinel_leaked == 0
    quality_pass = (
        summary.overall_floor_pass and summary.empty_all_pass and summary.lexical_negative_pass and sentinel_ok
    )
    result["acceptance"] = [
        acceptance_row(
            criterion="Seed receipts verified: controlled seeding, empty-account proof and exact inventory for both accounts",
            expected="Both receipts complete, hash-bound to the manifest, matching this corpus and endpoint origin",
            observed="; ".join(
                f"{role}: {len(r['seeded'])} facts, calls={r['usage']['calls_made']}, empty_proof={r['empty_account_proof']['passed']}"
                for role, r in sorted(receipts.items())
            ),
            status="PASS",
        ),
        acceptance_row(
            criterion="Hit@5 thresholds (overall>=0.90, category floors, empty leakage=0, lexical-negative>=18/20) met over live results",
            expected="overall>=0.90; paraphrase>=36/40; name_entity>=19/20; preference>=19/20; multilingual>=9/10; temporal=5/5; empty leakage=0; lexical-negative>=18 of the flagged paraphrases",
            observed=(
                f"overall={summary.overall_hits}/{summary.overall_total} ({summary.overall_hit_rate:.3f}); "
                f"by_category={summary.by_category}; empty_leakage={summary.empty_leakage_total}; "
                f"cross_account_sentinel_leakage={sentinel_leaked if sentinel_results else 'not run'}; "
                f"lexical_negative={summary.lexical_negative_hits}/{summary.lexical_negative_total}"
            ),
            status="BLOCKED" if blocked_reason else ("PASS" if quality_pass else "FAIL"),
        ),
        acceptance_row(
            criterion="Fixed-vector/lexical-only stub fails the quality predicate (proven in a prior --fixtures run)",
            expected="See the --fixtures run's own result.json for this run's source SHA",
            observed="Not re-verified by --live; this criterion is fixtures-mode's responsibility",
            status="NOT_RUN",
        ),
        acceptance_row(
            criterion="Budget exhausted/provider unavailable yields BLOCKED with partial results and no further calls",
            expected="On exhaustion or search_degraded, stop immediately and report partial results, never continue calling",
            observed=blocked_reason or f"budget not exhausted: {guard.total_calls()}/{guard.max_calls} calls used",
            status="PASS" if blocked_reason or guard.total_calls() <= guard.max_calls else "FAIL",
        ),
    ]
    result["usage"] = {
        "calls_this_run": guard.calls_made(),
        "calls_from_seed_receipts": guard.prior_calls,
        "request_bytes": guard.request_bytes(),
        "response_bytes": guard.response_bytes(),
        "input_token_upper_bound_including_seed": guard.tokens_used(),  # one token per request byte
    }
    result["elapsed_seconds"] = round(time.time() - started, 3)
    result["seed_receipts"] = {
        role: {"confirmation": m[key]["corpus_seeded_confirmation"], "facts": len(receipts[role]["seeded"])}
        for role, key in TARGETS
    }
    result["per_query"] = {
        "positive": [case_row(r) for r in positive_results],
        "empty": [case_row(r) for r in empty_results],
        "cross_account_sentinel": [case_row(r) for r in sentinel_results],
        "lexical_negative": [case_row(r) for r in ln_results],
    }
    result["limitations"] += [
        "Expected-empty scoring is strict: any returned search result or fact fails the case. The empty-case "
        "account holds zero current facts because each synthetic target was remembered, shown retrievable, and "
        "forgotten through the ordinary forget API by the seed run (see the seed receipt). Only `forget` was "
        "exercised; TTL expiry was not.",
        "The forgotten-target and sentinel texts (lib/forgotten_targets.py) are outside corpus.json and are a "
        "proposed addition pending task41 reviewer confirmation; the frozen queries, positive cases and "
        "thresholds are unchanged.",
        "cost.actual_usd is calls * budget.max_cost_per_call_usd (an operator ceiling), not a provider invoice.",
        "provider fields are the manifest's declared pin; the hosted recall response does not report a model or dimensions.",
    ]
    result["cost"]["actual_usd"] = guard.projected_cost_usd()
    result["cost"]["approved_max_usd"] = guard.approved_max_usd
    result["cost"]["authorization_refs"] = [m["budget"]["authorization_ref"]]

    if blocked_reason:
        result["status"], exit_code = "BLOCKED", EXIT_BLOCKED
        result["blockers"] = [{"owner": "coordinator", "required_input": blocked_reason}]
    elif quality_pass:
        result["status"], exit_code = "PASS", EXIT_PASS
    else:
        result["status"], exit_code = "FAIL", EXIT_TESTED_FAILURE
    return result, exit_code


def dirty_message(dirty: list[str]) -> str:
    return (
        f"uncommitted changes under the harness code ({', '.join(dirty[:5])}); commit them so source_sha "
        "names the code that ran"
    )


def run_seed(args: argparse.Namespace) -> tuple[dict, int]:
    """Seeds both qualification accounts under the same budget caps the live
    run uses, after proving each was empty. Never runs without
    seeding.authorized, never overwrites a receipt, stops at the first
    blocked step, and prints each receipt's confirmation string for the live
    manifest."""
    started = time.time()
    corpus, facts, fact_by_id = load_corpus(args.corpus, args.facts)
    source_sha = git_source_sha(REPO_ROOT)
    corpus_sha256 = corpus["meta"]["corpus_hash_sha256"]
    facts_sha256 = corpus["meta"]["facts_hash_sha256"]

    m = manifest_lib.load_manifest(args.manifest)
    problems = manifest_lib.validate_live_manifest(m, manifest_lib.PHASE_SEED)
    if m.get("corpus_sha256") != corpus_sha256:
        problems.append(f"manifest corpus_sha256={m.get('corpus_sha256')!r} does not match on-disk corpus={corpus_sha256!r}")
    if not args.receipt_dir:
        problems.append("--receipt-dir is required for --seed; receipts are the only attribution record")
    if not problems:
        dirty = dirty_paths(REPO_ROOT)
        if dirty:
            problems.append(dirty_message(dirty))
        tokens, cred_problems = credential_problems(m)
        problems += cred_problems
    if not problems:
        existing = [p for p in receipt_paths(args.receipt_dir).values() if p.exists()]
        if existing:
            problems.append(f"refusing to overwrite existing seed receipt(s): {', '.join(str(p) for p in existing)}")
    if problems:
        return build_blocked_result(args, corpus, source_sha, problems), EXIT_BLOCKED

    guard = BudgetGuard(m["budget"])
    check = guard_check_for(guard)
    paths = receipt_paths(args.receipt_dir)
    outcomes: dict[str, seeding.SeedOutcome] = {}
    confirmations: dict[str, str] = {}
    blocked_reason: str | None = None
    for role, key in TARGETS:
        target = m[key]
        client = guard.client(target["endpoint_url"], tokens[role], target["allowed_origins"])
        outcome = seeding.seed_target(
            client, check, role=role, corpus=corpus, fact_by_id=fact_by_id, corpus_sha256=corpus_sha256,
            facts_sha256=facts_sha256, source_sha=source_sha, recorded_at=now_iso(),
            credential_secret_ref=target["credential_secret_ref"],
            preceded_by_seeded_facts=sum(len(o.receipt["seeded"]) for o in outcomes.values()),
        )
        outcomes[role] = outcome
        confirmations[role] = seeding.write_receipt(paths[role], outcome.receipt)
        if outcome.blocked_reason:
            blocked_reason = f"{key}: {outcome.blocked_reason}"
            break

    seeded = sum(len(o.receipt["seeded"]) for o in outcomes.values())
    result = base_result(args.task_id, args.profile, source_sha, "live-provider")
    result["corpus_sha256"] = corpus_sha256
    result["configuration_sha256"] = sha256_file(args.manifest)
    result["provider"] = provider_record(m)
    result["commands"].append(
        f"python3 {Path(__file__).relative_to(REPO_ROOT)} --seed --manifest {args.manifest} --receipt-dir {args.receipt_dir} --output {args.output}"
    )
    planned = sum(len(seeding.plan_fact_ids(corpus, role)) for role, _ in TARGETS)
    empty_receipt = outcomes[seeding.ROLE_EMPTY_CASE].receipt if seeding.ROLE_EMPTY_CASE in outcomes else {}
    result["cases"].update(
        planned=planned, executed=seeded, passed=seeded, failed=0, skipped=planned - seeded,
        skip_reasons=[blocked_reason] if blocked_reason else [],
    )
    both_complete = len(outcomes) == 2 and all(o.receipt["complete"] for o in outcomes.values())
    result["acceptance"] = [
        acceptance_row(
            criterion="Empty-account proof before the first seed write, for each account",
            expected="recall facts_total=0, no search results, search not degraded",
            observed="; ".join(
                f"{role}: {(o.receipt['empty_account_proof'] or {'passed': 'not reached'})['passed']}" for role, o in sorted(outcomes.items())
            ),
            status="PASS" if len(outcomes) == 2 and all((o.receipt["empty_account_proof"] or {}).get("passed") for o in outcomes.values()) else "BLOCKED",
        ),
        acceptance_row(
            criterion="Controlled seeding: every planned fact inserted with a stored vector, inventory equals the seed",
            expected="status=inserted and search_state=semantic for each fact; post-seed inventory equals the seeded ids exactly",
            observed=blocked_reason or f"{seeded}/{planned} facts seeded across both accounts",
            status="PASS" if both_complete else "BLOCKED",
        ),
        acceptance_row(
            criterion="Forgotten-fact protocol: each expected-empty target shown present, forgotten via forget, then absent from inventory and search; cross-account sentinel unreachable",
            expected="presence rank<=5 for 5/5, forget expired=true for 5/5, post-forget inventory 0 and zero results/facts for 5/5, sentinel probe empty",
            observed=blocked_reason or (
                f"presence={len(empty_receipt.get('presence') or [])}/5, forgotten={len(empty_receipt.get('forget') or [])}/5, "
                f"post_forget_inventory={(empty_receipt.get('post_forget') or {}).get('inventory_total')}, "
                f"sentinel_probe_passed={(empty_receipt.get('cross_account_probe') or {}).get('passed')}"
            ),
            status="PASS" if both_complete else "BLOCKED",
        ),
    ]
    result["usage"] = {
        "calls_this_run": guard.calls_made(),
        "request_bytes": guard.request_bytes(),
        "response_bytes": guard.response_bytes(),
        "input_token_upper_bound": guard.tokens_used(),  # one token per request byte
    }
    result["elapsed_seconds"] = round(time.time() - started, 3)
    result["seed_receipts"] = {
        role: {"path": str(paths[role]), "confirmation": confirmations[role], "complete": outcomes[role].receipt["complete"]}
        for role in outcomes
    }
    result["cost"]["actual_usd"] = guard.projected_cost_usd()
    result["cost"]["approved_max_usd"] = guard.approved_max_usd
    result["cost"]["authorization_refs"] = [m["budget"]["authorization_ref"], m["seeding"]["authorization_ref"]]
    result["limitations"].append(
        "Seeding proves the accounts held the corpus; it measures no retrieval quality. Status stays PARTIAL until a "
        "separate --live run scores recall against the receipts."
    )
    if both_complete:
        result["status"] = "PARTIAL"
        exit_code = EXIT_PASS
    else:
        result["status"] = "BLOCKED"
        result["blockers"] = [{"owner": "coordinator", "required_input": blocked_reason or "seeding incomplete"}]
        exit_code = EXIT_BLOCKED
    return result, exit_code


def receipt_paths(receipt_dir) -> dict[str, Path]:
    d = Path(receipt_dir or ".")
    return {role: d / f"T23.43-seed-{role}.json" for role, _ in TARGETS}


def plan_requests(corpus: dict, fact_by_id: dict, corpus_sha256: str) -> dict:
    """Exact call and request-byte plan for the whole qualification (seed
    then live), computed from the frozen corpus with no network. It mirrors
    seeding.seed_target and run_live request for request; the preflight test
    compares it with a real run, so drift fails a test."""
    K = scoring.K
    tool = lambda name, args: ("tools/call", mcp_client.tool_call_params(name, args))  # noqa: E731
    recall = lambda args: tool("recall", args)  # noqa: E731
    init = ("initialize", mcp_client.INITIALIZE_PARAMS)
    inventory = recall({"limit": seeding.INVENTORY_LIMIT})
    probes = [recall({"limit": 1}), recall({"query": seeding.EMPTY_PROBE_QUERY, "limit": K})]
    empty = empty_cases(corpus)
    sentinel = recall({"query": forgotten_targets.SENTINEL_FACT_TEXT, "limit": K})
    empty_queries = [recall({"query": c["query"], "limit": K}) for c in empty]

    def remembers(role: str) -> list:
        return [
            tool("remember", {
                "fact": text,
                "provenance": seeding.provenance(corpus_sha256, fid),
                "operation_key": seeding.operation_key(corpus_sha256, fid),
            })
            for fid, text in seeding.plan_facts(corpus, fact_by_id, role)
        ]

    forgets = [tool("forget", {"id": "0" * 64, "reason": "T23.43 expected-empty qualification: forgotten target"}) for _ in empty]
    requests = {
        (seeding.ROLE_POSITIVE, "seed"): [init, *probes, *remembers(seeding.ROLE_POSITIVE), inventory],
        (seeding.ROLE_EMPTY_CASE, "seed"): [
            init, *probes, *remembers(seeding.ROLE_EMPTY_CASE), inventory,
            *empty_queries, *forgets, inventory, *empty_queries, sentinel,
        ],
        (seeding.ROLE_POSITIVE, "live"): [
            init, inventory, *[recall({"query": c["query"], "limit": K}) for c in positive_cases(corpus)]
        ],
        (seeding.ROLE_EMPTY_CASE, "live"): [init, inventory, *empty_queries, sentinel],
    }
    size = lambda reqs: sum(len(mcp_client.request_body(1000, m, p)) for m, p in reqs)  # noqa: E731
    plan: dict = {"per_role": {}}
    for role, _ in TARGETS:
        plan["per_role"][role] = {
            "facts_to_seed": len(seeding.plan_fact_ids(corpus, role)),
            "seed_calls": len(requests[(role, "seed")]),
            "live_calls": len(requests[(role, "live")]),
            "request_bytes": size(requests[(role, "seed")]) + size(requests[(role, "live")]),
        }
    plan["total_calls"] = sum(r["seed_calls"] + r["live_calls"] for r in plan["per_role"].values())
    plan["total_request_bytes"] = sum(r["request_bytes"] for r in plan["per_role"].values())
    # Charged at one token per request byte (lib/budget.py), an upper bound.
    plan["input_token_upper_bound"] = plan["total_request_bytes"]
    return plan


def run_preflight(args: argparse.Namespace) -> tuple[dict, int]:
    """Offline readiness check for a separately authorized real run: makes
    no network call and spends nothing. Validates the manifest for the
    chosen phase, the corpus pin, the clean tree, credentials present (names
    only), receipts (live phase), the lexical-control binaries, and that the
    manifest's caps cover the exact planned calls, bytes and cost."""
    corpus, facts, fact_by_id = load_corpus(args.corpus, args.facts)
    source_sha = git_source_sha(REPO_ROOT)
    corpus_sha256 = corpus["meta"]["corpus_hash_sha256"]
    m = manifest_lib.load_manifest(args.manifest)
    phase = args.phase
    problems = manifest_lib.validate_live_manifest(m, phase)
    if m.get("corpus_sha256") != corpus_sha256:
        problems.append(f"manifest corpus_sha256={m.get('corpus_sha256')!r} does not match on-disk corpus={corpus_sha256!r}")
    dirty = dirty_paths(REPO_ROOT)
    if dirty:
        problems.append(dirty_message(dirty))
    plan = plan_requests(corpus, fact_by_id, corpus_sha256)
    budget = m.get("budget") or {}
    if not [p for p in problems if p.startswith("budget.")]:
        if plan["total_calls"] > budget["max_calls"]:
            problems.append(f"budget.max_calls={budget['max_calls']} is below the {plan['total_calls']} calls the full seed+live plan needs")
        if plan["input_token_upper_bound"] > budget["max_input_tokens"]:
            problems.append(
                f"budget.max_input_tokens={budget['max_input_tokens']} is below the {plan['input_token_upper_bound']} "
                "the plan needs (one token per serialized request byte, an upper bound)"
            )
        if plan["total_calls"] * budget["max_cost_per_call_usd"] > budget["approved_max_usd"]:
            problems.append(
                f"budget.approved_max_usd={budget['approved_max_usd']} is below {plan['total_calls']} calls x "
                f"max_cost_per_call_usd={budget['max_cost_per_call_usd']}"
            )
    if all(k in m for _, k in TARGETS):
        _tokens, cred_problems = credential_problems(m)
        problems += cred_problems
    if phase == manifest_lib.PHASE_LIVE and not problems:
        _receipts, receipt_problems = load_target_receipts(m, args.manifest, corpus, corpus["meta"]["facts_hash_sha256"])
        problems += receipt_problems
    result = build_blocked_result(args, corpus, source_sha, problems, evidence_level="static") if problems else base_result(
        args.task_id, args.profile, source_sha, "static"
    )
    result["corpus_sha256"] = corpus_sha256
    result["configuration_sha256"] = sha256_file(args.manifest)
    result["commands"].append(f"python3 {Path(__file__).relative_to(REPO_ROOT)} --preflight --phase {phase} --manifest {args.manifest} --output {args.output}")
    result["plan"] = plan
    if not problems:
        result["status"] = "PARTIAL"
        result["cases"]["planned"] = len(corpus["cases"])
        result["acceptance"] = [
            acceptance_row(
                criterion=f"Preflight ({phase} phase): manifest, pins, credentials, caps cover the planned run; no network call made",
                expected="No problems",
                observed=f"{plan['total_calls']} planned calls, {plan['input_token_upper_bound']} input-token upper bound; nothing executed",
                status="PASS",
            )
        ]
        result["limitations"].append("Preflight is an offline readiness check. It measures no retrieval quality and authorizes nothing.")
        return result, EXIT_PASS
    return result, EXIT_BLOCKED


def parse_args(argv: list[str]) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__)
    mode = p.add_mutually_exclusive_group(required=True)
    mode.add_argument("--fixtures", action="store_true", help="local synthetic provider only; no network")
    mode.add_argument("--preflight", action="store_true", help="offline readiness check for --seed/--live; no network")
    mode.add_argument("--seed", action="store_true", help="seed both hosted accounts (writes; needs seeding.authorized)")
    mode.add_argument("--live", action="store_true", help="real hosted MCP endpoint; requires a valid manifest and seed receipts")
    p.add_argument("--manifest", required=True, help="path to the qualification manifest JSON")
    p.add_argument("--output", required=True, help="path to write the result JSON")
    p.add_argument("--phase", default="seed", choices=["seed", "live"], help="--preflight only: which phase to validate")
    p.add_argument("--receipt-dir", default=None, help="--seed only: directory the seed receipts are written to")
    p.add_argument("--corpus", default=str(EVALS_HOSTED / "corpus.json"))
    p.add_argument("--facts", default=str(EVALS_HOSTED / "facts.json"))
    p.add_argument("--task-id", default="T23.43")
    p.add_argument("--profile", default="paid", choices=["paid", "pilot"])
    p.add_argument("--serenity-bin", default=None, help="path to a pre-built serenity binary (or $SERENITY_BIN)")
    p.add_argument("--seedbrain-bin", default=None, help="path to a pre-built seedbrain binary (or $SEEDBRAIN_BIN)")
    args = p.parse_args(argv)
    args.corpus = Path(args.corpus)
    args.facts = Path(args.facts)
    args.output = Path(args.output)
    args.manifest = Path(args.manifest)
    return args


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    if not args.corpus.is_file() or not args.facts.is_file():
        print(f"eval_embeddings: corpus/facts not found ({args.corpus}, {args.facts})", file=sys.stderr)
        return EXIT_INVALID_INPUT

    try:
        if args.fixtures:
            result, exit_code = run_fixtures(args)
        elif args.preflight:
            result, exit_code = run_preflight(args)
        elif args.seed:
            result, exit_code = run_seed(args)
        else:
            result, exit_code = run_live(args)
    except (OSError, ValueError) as e:
        # Unreadable or malformed manifest/receipt: invalid input, but the
        # contract still requires a result file explaining why.
        corpus = json.loads(args.corpus.read_text(encoding="utf-8"))
        result = build_blocked_result(args, corpus, git_source_sha(REPO_ROOT), [f"invalid input: {type(e).__name__}: {e}"], evidence_level="static")
        exit_code = EXIT_INVALID_INPUT

    write_result(args.output, result)
    print(f"eval_embeddings: status={result['status']} written to {args.output}")
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
