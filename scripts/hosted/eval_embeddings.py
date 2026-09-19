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

from lib import lexical_control, manifest as manifest_lib, mcp_client, scoring  # noqa: E402
from lib.cosine import rank_by_cosine  # noqa: E402
from lib.fixture_embedder import FixedVectorEmbedder, HashBagEmbedder  # noqa: E402

EXIT_PASS = 0
EXIT_TESTED_FAILURE = 1
EXIT_BLOCKED = 2
EXIT_INVALID_INPUT = 3


def load_corpus(corpus_path: Path, facts_path: Path):
    corpus = json.loads(corpus_path.read_text(encoding="utf-8"))
    facts = json.loads(facts_path.read_text(encoding="utf-8"))
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
        empty_results.append(scoring.score_empty_case(case["id"], ranked_ids, allowed_ids={filler["id"]}))

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
    corpus, facts, fact_by_id = load_corpus(args.corpus, args.facts)
    source_sha = git_source_sha(REPO_ROOT)
    all_fact_ids = sorted(fact_by_id.keys())

    result = base_result(args.task_id, args.profile, source_sha, "fixture")
    result["corpus_sha256"] = corpus["meta"]["corpus_hash_sha256"]
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

    overall_status = "PARTIAL" if lexical_summary is not None else "PARTIAL"
    result["status"] = overall_status
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


def build_blocked_result(args: argparse.Namespace, corpus: dict, source_sha: str, problems: list[str]) -> dict:
    result = base_result(args.task_id, args.profile, source_sha, "live-provider")
    result["corpus_sha256"] = corpus["meta"]["corpus_hash_sha256"]
    result["status"] = "BLOCKED"
    result["cases"]["planned"] = len(corpus["cases"])
    result["cases"]["skipped"] = len(corpus["cases"])
    result["cases"]["skip_reasons"] = problems
    result["acceptance"] = [
        acceptance_row(
            criterion="Budget exhausted/provider unavailable yields BLOCKED with partial results and no further calls",
            expected="Missing/null budget or credential refuses the run rather than defaulting to unlimited access",
            observed="; ".join(problems),
            status="PASS",
        )
    ]
    result["blockers"] = [{"owner": "coordinator/David", "required_input": p} for p in problems]
    return result


def run_live(args: argparse.Namespace) -> tuple[dict, int]:
    corpus, facts, fact_by_id = load_corpus(args.corpus, args.facts)
    source_sha = git_source_sha(REPO_ROOT)

    m = manifest_lib.load_manifest(args.manifest)
    problems = manifest_lib.validate_live_manifest(m)
    if problems:
        return build_blocked_result(args, corpus, source_sha, problems), EXIT_BLOCKED

    expected_hash = m.get("corpus_sha256")
    actual_hash = corpus["meta"]["corpus_hash_sha256"]
    if expected_hash != actual_hash:
        return (
            build_blocked_result(
                args, corpus, source_sha,
                [f"manifest corpus_sha256={expected_hash!r} does not match on-disk corpus={actual_hash!r}"],
            ),
            EXIT_BLOCKED,
        )

    cred_ref = m["hosted_mcp"]["credential_secret_ref"]
    cred_token = os.environ.get(cred_ref)
    if not cred_token:
        return (
            build_blocked_result(args, corpus, source_sha, [f"${cred_ref} is not set in the environment"]),
            EXIT_BLOCKED,
        )

    budget = m["budget"]
    max_calls = int(budget["max_calls"])
    max_elapsed = float(budget["max_elapsed_seconds"])
    max_input_tokens = float(budget["max_input_tokens"])

    def estimated_tokens_used(client: "mcp_client.MCPClient") -> float:
        # char/4 estimate, the same convention internal/server/memory's
        # own charEstimate uses for budget packing -- an approximation,
        # disclosed as such, not a provider-reported token count.
        return client.usage().request_bytes / 4.0

    client = mcp_client.MCPClient(m["hosted_mcp"]["endpoint_url"], cred_token)

    result = base_result(args.task_id, args.profile, source_sha, "live-provider")
    result["corpus_sha256"] = actual_hash
    result["provider"] = {
        "base_url": m["provider"].get("base_url"),
        "model": m["provider"].get("model"),
        "hosted_endpoint": m["hosted_mcp"].get("endpoint_url"),
    }

    positive_results = []
    started = time.time()
    blocked_reason = None
    for case in positive_cases(corpus):
        if client.calls_made >= max_calls:
            blocked_reason = f"budget exhausted: max_calls={max_calls} reached after {len(positive_results)} cases"
            break
        if time.time() - started >= max_elapsed:
            blocked_reason = f"budget exhausted: max_elapsed_seconds={max_elapsed} reached after {len(positive_results)} cases"
            break
        if estimated_tokens_used(client) >= max_input_tokens:
            blocked_reason = f"budget exhausted: max_input_tokens={max_input_tokens} reached after {len(positive_results)} cases"
            break
        try:
            resp = client.call_tool("recall", {"query": case["query"], "limit": scoring.K})
        except mcp_client.MCPError as e:
            blocked_reason = f"live call failed for case {case['id']}: {e}"
            break
        ranked_ids = [r.get("slug") for r in resp.get("results", []) if r.get("slug")]
        positive_results.append(
            scoring.score_positive_case(case["id"], case["category"], case["expected_fact_id"], ranked_ids)
        )

    empty_results = []
    if blocked_reason is None:
        for case in empty_cases(corpus):
            if (
                client.calls_made >= max_calls
                or time.time() - started >= max_elapsed
                or estimated_tokens_used(client) >= max_input_tokens
            ):
                blocked_reason = "budget exhausted before the empty-case phase completed"
                break
            try:
                resp = client.call_tool("recall", {"query": case["query"], "limit": scoring.K})
            except mcp_client.MCPError as e:
                blocked_reason = f"live call failed for case {case['id']}: {e}"
                break
            ranked_ids = [r.get("slug") for r in resp.get("results", []) if r.get("slug")]
            empty_results.append(scoring.score_empty_case(case["id"], ranked_ids, allowed_ids=set()))

    summary = scoring.summarize(positive_results, empty_results)

    result["cases"]["planned"] = len(corpus["cases"])
    result["cases"]["executed"] = len(positive_results) + len(empty_results)
    result["cases"]["passed"] = summary.overall_hits + sum(1 for r in empty_results if r.hit)
    result["cases"]["failed"] = len(positive_results) - summary.overall_hits + sum(1 for r in empty_results if not r.hit)
    result["cases"]["skipped"] = len(corpus["cases"]) - result["cases"]["executed"]
    result["cases"]["skip_reasons"] = [blocked_reason] if blocked_reason else []

    result["acceptance"] = [
        acceptance_row(
            criterion="Hit@5 thresholds (overall>=0.90, category floors, empty leakage=0, lexical-negative>=18/20) met over live results",
            expected="overall>=0.90; paraphrase>=36/40; name_entity>=19/20; preference>=19/20; multilingual>=9/10; temporal=5/5; empty leakage=0",
            observed=(
                f"overall={summary.overall_hits}/{summary.overall_total} ({summary.overall_hit_rate:.3f}); "
                f"by_category={summary.by_category}; empty_leakage={summary.empty_leakage_total}"
            ),
            status="PASS" if (summary.overall_floor_pass and summary.empty_all_pass and blocked_reason is None) else (
                "BLOCKED" if blocked_reason else "FAIL"
            ),
        ),
        acceptance_row(
            criterion="Fixed-vector/lexical-only stub fails the quality predicate (proven in a prior --fixtures run)",
            expected="See the --fixtures run's own result.json for this run's source SHA",
            observed="Not re-verified by --live; this criterion is fixtures-mode's responsibility",
            status="NOT_RUN",
        ),
        acceptance_row(
            criterion="Budget exhausted/provider unavailable yields BLOCKED with partial results and no further calls",
            expected="On exhaustion, stop immediately and report partial results, never continue calling",
            observed=blocked_reason or f"budget not exhausted: {client.calls_made}/{max_calls} calls used",
            status="PASS" if blocked_reason or client.calls_made <= max_calls else "FAIL",
        ),
    ]

    result["cost"]["approved_max_usd"] = budget.get("approved_max_usd")
    result["cost"]["authorization_refs"] = [budget.get("authorization_ref")] if budget.get("authorization_ref") else []

    if blocked_reason:
        result["status"] = "BLOCKED"
        result["blockers"] = [{"owner": "coordinator", "required_input": blocked_reason}]
        exit_code = EXIT_BLOCKED
    elif summary.overall_floor_pass and summary.empty_all_pass:
        result["status"] = "PASS"
        exit_code = EXIT_PASS
    else:
        result["status"] = "FAIL"
        exit_code = EXIT_TESTED_FAILURE

    return result, exit_code


def parse_args(argv: list[str]) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__)
    mode = p.add_mutually_exclusive_group(required=True)
    mode.add_argument("--fixtures", action="store_true", help="local synthetic provider only; no network")
    mode.add_argument("--live", action="store_true", help="real hosted MCP endpoint; requires a valid manifest")
    p.add_argument("--manifest", required=True, help="path to the qualification manifest JSON")
    p.add_argument("--output", required=True, help="path to write the result JSON")
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
    return args


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    if not args.corpus.is_file() or not args.facts.is_file():
        print(f"eval_embeddings: corpus/facts not found ({args.corpus}, {args.facts})", file=sys.stderr)
        return EXIT_INVALID_INPUT

    if args.fixtures:
        result, exit_code = run_fixtures(args)
    else:
        result, exit_code = run_live(args)

    write_result(args.output, result)
    print(f"eval_embeddings: status={result['status']} written to {args.output}")
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
