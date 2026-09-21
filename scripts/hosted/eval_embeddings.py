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

from lib import forgotten_targets, ledger as ledger_lib, lexical_control, manifest as manifest_lib, mcp_client, scoring, seeding  # noqa: E402
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
    brains: list = []  # every disposable brain this arm creates, removed on every path
    try:
        brain = lexical_control.build_disposable_brain(serenity_bin, seedbrain_bin, facts_for_brain)
        brains.append(brain)

        # Ensure the lexical control can find a fact when given its exact text.
        # Otherwise a broken parser/index would manufacture an attractive
        # lexical-negative score by returning no hits for every paraphrase.
        control_fact = facts_for_brain[0]
        control_ids = [
            chunk_ref_to_fact_id(ref)
            for ref in brain.search(control_fact["text"], limit=scoring.K)
        ]
        if control_fact["id"] not in control_ids:
            raise lexical_control.LexicalControlFailed("lexical positive control missed its verbatim fact query")

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
            brains.append(isolated)
            refs = isolated.search(case["query"], limit=scoring.K)
            ranked_ids = [chunk_ref_to_fact_id(r) for r in refs]
            # Strict: the control brain holds one unrelated filler fact, and
            # returning it counts as a violation like any other result.
            empty_results.append(scoring.score_empty_case(case["id"], ranked_ids))
    finally:
        for b in brains:
            b.close()

    return positive_results, empty_results


def lexical_negative_check(pos_semantic_results, pos_lexical_results, corpus, case_ids=None):
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
    selected_ids = sorted(flagged_ids if case_ids is None else case_ids)
    if not set(selected_ids) <= flagged_ids:
        raise ValueError("frozen lexical-negative case ids are not a subset of the flagged corpus cases")
    sem_by_id = {r.case_id: r for r in pos_semantic_results}
    lex_by_id = {r.case_id: r for r in pos_lexical_results}
    case_by_id = {c["id"]: c for c in corpus["cases"]}
    results = []
    for cid in selected_ids:
        sem = sem_by_id.get(cid)
        lex = lex_by_id.get(cid)
        passed = sem is not None and lex is not None and sem.hit and not lex.hit
        case = case_by_id[cid]
        results.append(
            scoring.CaseResult(
                cid, "paraphrase", case["expected_fact_id"],
                sem.ranked_ids if sem is not None else [], hit=passed,
                rank=sem.rank if sem is not None else None,
            )
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
    except lexical_control.LexicalControlFailed as e:
        # The binaries exist but a step failed or hung: not a tested failure and
        # not a skip. A result file explains why (fixed text, no stderr).
        return (
            build_blocked_result(args, corpus, source_sha, [f"lexical control failed: {e}"], evidence_level="static"),
            EXIT_BLOCKED,
        )

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
        "The local lexical control's empty cases run against isolated brains holding one unrelated filler "
        "fact (seedbrain refuses an empty brain) and are scored strictly: returning the filler fails. That "
        "arm is a control, not the forgotten/expired qualification. The real protocol (remember a target, show "
        "it retrievable, then remove it by forget or by a fixed absolute TTL and show it absent) runs only "
        "under --seed/--live and has been exercised only against a loopback fake, never a hosted account."
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


BUDGET_CRITERION = "Budget exhausted/provider unavailable yields BLOCKED with partial results and no further calls"


def budget_row(*, exercised: bool, expected: str, observed: str) -> dict:
    """PASS only when this run actually met the boundary the criterion is about
    (a budget cap or an unavailable provider stopped it). Any other outcome, a
    successful run included, did not exercise it and is NOT_RUN; the unit tests
    (TestLiveModeBudgetEnforcement, TestCumulativeLedger) carry that evidence."""
    return acceptance_row(
        BUDGET_CRITERION, expected,
        observed if exercised else f"not exercised by this run: {observed}; the unit tests cover the boundary",
        "PASS" if exercised else "NOT_RUN",
    )


def build_blocked_result(
    args: argparse.Namespace, corpus: dict, source_sha: str, problems: list[str], *, evidence_level: str = "static"
) -> dict:
    """A run that never started: nothing was measured, so the evidence level is
    `static` unless the caller says otherwise."""
    result = base_result(args.task_id, args.profile, source_sha, evidence_level)
    result["corpus_sha256"] = corpus["meta"]["corpus_hash_sha256"]
    result["status"] = "BLOCKED"
    result["cases"]["planned"] = len(corpus["cases"])
    result["cases"]["skipped"] = len(corpus["cases"])
    result["cases"]["skip_reasons"] = problems
    exercised = any(p.startswith(("budget.", "ledger")) for p in problems)
    result["acceptance"] = [
        budget_row(
            exercised=exercised,
            expected="Missing/null budget, credential, seed receipt or provider pin refuses the run rather than defaulting to unlimited access",
            observed="; ".join(problems),
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


def classify_run(m: dict) -> dict:
    """What kind of evidence a run against this manifest's endpoints can be.
    Any local endpoint (loopback or private literal address, `localhost`), or an
    environment declared `local-fixture`, makes it a FIXTURE: the harness cannot
    tell a loopback oracle from a provider, so it never labels one live-provider
    or lets it PASS. A remote endpoint is not thereby proven to serve the
    declared provider either (see qualification_prerequisites)."""
    classes = {key: manifest_lib.endpoint_class(m[key]["endpoint_url"]) for _role, key in TARGETS}
    kind = (m.get("environment") or {}).get("kind")
    local_targets = sorted(k for k, c in classes.items() if c != manifest_lib.ENDPOINT_CLASS_REMOTE)
    fixture = bool(local_targets) or kind == "local-fixture"
    return {
        "fixture": fixture,
        "evidence_level": "fixture" if fixture else "live-provider",
        "endpoint_classes": classes,
        "environment_kind": kind,
        "local_targets": local_targets,
    }


def qualification_prerequisites(cls: dict) -> list[str]:
    """What must still hold before a run counts toward the qualification. The
    harness lists them; it does not observe them."""
    out = []
    if cls["fixture"]:
        out.append(
            "Run against the provisioned hosted test accounts on a remote endpoint: a local endpoint "
            f"({', '.join(cls['local_targets']) or 'environment.kind=local-fixture'}) is a fixture and cannot satisfy a live-provider gate."
        )
    out += [
        "The embedding provider, model pin and dimensions are declared by the manifest and are not observed: the "
        "hosted recall response reports neither. Deployment configuration evidence (the EMBEDDINGS gate's key, allowance "
        "and model pin, and T23.42's provider adapter) must show which provider served the run.",
        "budget.authorization_ref must name the EMBEDDINGS authority's approval of these caps; the ledger enforces the "
        "caps but does not check who approved them.",
        "task41's reviewer must confirm the freeze receipt (corpus hash, lexical-negative criterion, supplemental hash).",
    ]
    return out


def provider_record(m: dict, cls: dict) -> dict:
    """Declared and observed are kept apart. `declared` is the manifest's pin,
    copied verbatim; `observed` holds only what the harness saw (the endpoints
    it called and where they are), never a model or dimension the service did
    not report."""
    p = m["provider"]
    return {
        "declared": {
            "base_url": p.get("base_url"),
            "model": p.get("model"),
            "version_pin": p.get("version_pin"),
            "dimensions": p.get("dimensions"),
            "serving_provider": p.get("serving_provider"),
            "privacy_review_ref": p.get("privacy_review_ref"),
            "allow_fallback": p.get("allow_fallback"),
            "environment_kind": cls["environment_kind"],
        },
        "observed": {
            "hosted_endpoints": {
                key: {"url": m[key].get("endpoint_url"), "class": cls["endpoint_classes"][key]} for _role, key in TARGETS
            },
            "model": None,
            "dimensions": None,
            "provider_identity_verified": False,
        },
        "qualification_prerequisites": qualification_prerequisites(cls),
    }


def classification_limitations(cls: dict) -> list[str]:
    out = []
    if cls["fixture"]:
        out.append(
            f"Local endpoint(s) ({', '.join(cls['local_targets']) or 'environment.kind=local-fixture'}): this is a fixture run. "
            "evidence_level is 'fixture' and the status cannot exceed PARTIAL, however the rankings score. A loopback "
            "oracle can rank every expected fact first, so a local score is a mechanics rehearsal and never semantic-quality evidence."
        )
    out.append(
        "provider.declared is the manifest's pin, not an observation; provider.observed lists only the endpoints called and "
        "their class. An endpoint host name is not resolved, so a name pointed at a local address is not detected."
    )
    return out


def _ledger_arguments(m: dict, manifest_path: Path, corpus: dict) -> tuple[Path, dict]:
    path = resolve_receipt_path(manifest_path, m["budget"]["ledger_path"])
    identity = ledger_lib.identity_of(
        m,
        corpus_sha256=corpus["meta"]["corpus_hash_sha256"],
        facts_sha256=corpus["meta"]["facts_hash_sha256"],
        supplemental_sha256=forgotten_targets.targets_sha256(),
    )
    return path, dict(
        authorization_ref=m["budget"]["authorization_ref"], caps=ledger_lib.caps_of(m["budget"]), identity=identity
    )


def open_ledger(m: dict, manifest_path: Path, corpus: dict) -> ledger_lib.Ledger:
    """Opens the EXISTING cumulative ledger this manifest's `budget.ledger_path`
    names, bound to its authorization reference, caps and identity. A seed and a
    live run only ever continue a ledger: a missing, empty or damaged one blocks
    and is never re-created. `--init-ledger` is the one way to begin one."""
    path, kwargs = _ledger_arguments(m, manifest_path, corpus)
    return ledger_lib.Ledger.open(path, **kwargs)


def initialize_ledger(m: dict, manifest_path: Path, corpus: dict) -> ledger_lib.Ledger:
    """Begins the ledger at this manifest's `budget.ledger_path`. An intentional
    operator step, never automatic budget renewal: it refuses a path where any
    ledger, stale file or lock file already exists."""
    path, kwargs = _ledger_arguments(m, manifest_path, corpus)
    return ledger_lib.Ledger.initialize(path, **kwargs)


def record_cost(result: dict, guard: BudgetGuard, authorization_refs: list[str]) -> None:
    """`cost.actual_usd` stays null: nothing here measures a provider or
    hosting invoice, and reporting a projection as actual spend would be a
    fabricated measurement. The worst-case reservation the budget guard
    enforces (calls made times the operator's per-call ceiling) is reported
    under its own name."""
    result["cost"]["actual_usd"] = None
    result["cost"]["operator_ceiling_projection_usd"] = guard.projected_cost_usd()
    result["cost"]["approved_max_usd"] = guard.approved_max_usd
    result["cost"]["authorization_refs"] = authorization_refs


COST_LIMITATION = (
    "cost.actual_usd is null: no billing measurement exists for this run. "
    "cost.operator_ceiling_projection_usd is calls x budget.max_cost_per_call_usd, a worst-case reservation the "
    "budget guard enforces, not a spend figure or an invoice."
)


def guard_check_for(guard: BudgetGuard):
    def check() -> None:
        reason = guard.exhausted_reason()
        if reason:
            raise seeding.BudgetBlocked(reason)

    return check


def run_live(args: argparse.Namespace, *, lexical_provider=None) -> tuple[dict, int]:
    """Scores the hosted recall endpoint. Everything that can be checked
    without spending is checked first (manifest, clean tree, seed receipts,
    credentials, the cumulative ledger, the local lexical control), so a
    missing prerequisite blocks before the first request.

    Every request is reserved in the ledger before it is sent, so the manifest's
    caps hold across this run, its retries and the seed run. A run against a
    local endpoint is a fixture: evidence_level 'fixture', status never above
    PARTIAL."""
    started = time.time()
    corpus, facts, fact_by_id = load_corpus(args.corpus, args.facts)
    source_sha = git_source_sha(REPO_ROOT)
    actual_hash = corpus["meta"]["corpus_hash_sha256"]

    m = manifest_lib.load_manifest(args.manifest)
    problems = manifest_lib.validate_live_manifest(m, manifest_lib.PHASE_LIVE)
    problems += manifest_lib.validate_reviewer_freeze(
        m, source_sha=source_sha, corpus=corpus,
        facts_sha256=corpus["meta"]["facts_hash_sha256"],
        supplemental_sha256=forgotten_targets.targets_sha256(),
    )
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

    # The ledger, not the receipts, establishes what was spent. A receipt that
    # was not produced under this ledger, or whose counters differ from what
    # the ledger recorded, cannot stand in for spend.
    try:
        ledger = open_ledger(m, args.manifest, corpus)
        for role, key in TARGETS:
            problems += [f"{key}: {p}" for p in ledger.receipt_problems(receipts[role], role)]
    except ledger_lib.LedgerError as e:
        problems.append(f"ledger: {e}")
    if problems:
        return build_blocked_result(args, corpus, source_sha, problems), EXIT_BLOCKED

    # The lexical control is local and free; run it before any request so a
    # missing or failing binary blocks here and not after the spend.
    try:
        lexical_pos, _lexical_empty = (lexical_provider or (lambda: default_lexical_provider(args, corpus, fact_by_id)))()
    except lexical_control.LexicalControlError as e:
        return build_blocked_result(args, corpus, source_sha, [f"lexical control unavailable: {e}"]), EXIT_BLOCKED

    cls = classify_run(m)
    guard = BudgetGuard(m["budget"], ledger, mode="live")
    check = guard_check_for(guard)

    def run_target(role: str, key: str, cases: list[dict], scorer) -> tuple[list, str | None, str | None]:
        target, receipt = m[key], receipts[role]
        client = guard.client(target["endpoint_url"], tokens[role], target["allowed_origins"], role=role)
        id_map = receipt["id_map"]
        # The positive account must still hold exactly what was seeded. The
        # empty-case account must hold NOTHING: every target was forgotten or expired.
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
            return results, str(e), seeding.blocked_kind(e)  # authored by this harness, never upstream text
        except mcp_client.MCPError as e:
            return results, f"live call failed: {e}", "other"  # fixed classes only
        except Exception as e:  # noqa: BLE001 -- partial results and usage must survive an unexpected error
            return results, f"unexpected error: {type(e).__name__}", "unexpected"  # class name only
        return results, None, None

    positive_results, blocked_reason, blocked_kind = run_target(
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
        both, blocked_reason, blocked_kind = run_target(
            seeding.ROLE_EMPTY_CASE,
            "empty_case_hosted_mcp",
            empty_cases(corpus) + [probe],
            lambda case, ranked, facts_arm: scoring.score_empty_case(case["id"], ranked, facts_arm),
        )
        n_empty = len(empty_cases(corpus))
        empty_results, sentinel_results = both[:n_empty], both[n_empty:]

    freeze = m["threshold_freeze"]["lexical_negative"]
    ln_results = lexical_negative_check(positive_results, lexical_pos, corpus, freeze["case_ids"]) if blocked_reason is None else []
    summary = scoring.summarize(
        positive_results, empty_results, ln_results,
        lexical_negative_min_hits=freeze["min_hits"],
        lexical_negative_min_denom=len(freeze["case_ids"]),
    )

    result = base_result(args.task_id, args.profile, source_sha, cls["evidence_level"])
    result["corpus_sha256"] = actual_hash
    result["configuration_sha256"] = sha256_file(args.manifest)
    result["provider"] = provider_record(m, cls)
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
    if blocked_reason:
        quality_status = "BLOCKED"
    elif cls["fixture"]:
        # A local oracle can rank every expected fact first: the scores are a
        # mechanics rehearsal. Missing the thresholds is still a fact worth
        # reporting; meeting them is never a PASS.
        quality_status = "NOT_RUN" if quality_pass else "FAIL"
    else:
        quality_status = "PASS" if quality_pass else "FAIL"
    exercised = blocked_kind in ("budget", "provider_unavailable")
    result["acceptance"] = [
        acceptance_row(
            criterion="Seed receipts verified: controlled seeding, empty-account proof and exact inventory for both accounts",
            expected="Both receipts complete, hash-bound to the manifest, matching this corpus and endpoint origin, and ledger-backed",
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
                + ("; measured against a LOCAL endpoint: a mechanics rehearsal, not quality evidence" if cls["fixture"] and not blocked_reason else "")
            ),
            status=quality_status,
        ),
        acceptance_row(
            criterion="Fixed-vector/lexical-only stub fails the quality predicate (proven in a prior --fixtures run)",
            expected="See the --fixtures run's own result.json for this run's source SHA",
            observed="Not re-verified by --live; this criterion is fixtures-mode's responsibility",
            status="NOT_RUN",
        ),
        budget_row(
            exercised=exercised,
            expected="On exhaustion or search_degraded, stop immediately and report partial results, never continue calling",
            observed=blocked_reason or (
                f"budget not exhausted: {guard.total_calls()}/{guard.max_calls} calls charged to the qualification"
            ),
        ),
    ]
    ledger_report = ledger.report(guard.invocation)
    result["usage"] = {
        "calls_this_run": guard.calls_made(),
        "request_bytes": guard.request_bytes(),
        "response_bytes": guard.response_bytes(),
        "input_token_upper_bound_cumulative": ledger_report["cumulative_request_bytes"],  # one token per request byte
        "reserved_not_sent": guard.reserved_not_sent,  # reservations that landed past max_elapsed_seconds: counted, never sent
        "ledger": ledger_report,
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
        "then removed by the seed run: three by the ordinary forget API and two by lapsing past a fixed absolute "
        "TTL, shown absent only after the service's clock passed it (see the seed receipt).",
        "The supplemental plan (lib/forgotten_targets.py: target texts, removal modes, expiry timing and the "
        "sentinel; hash in the seed receipts as forgotten_targets_sha256) is outside corpus.json and is a "
        "proposed addition pending task41 reviewer confirmation; the frozen queries, positive cases and "
        "thresholds are unchanged.",
        COST_LIMITATION,
        ledger_lib.OPERATOR_GUARD_STATEMENT,
        *classification_limitations(cls),
    ]
    record_cost(result, guard, [m["budget"]["authorization_ref"]])

    if blocked_reason:
        result["status"], exit_code = "BLOCKED", EXIT_BLOCKED
        result["blockers"] = [{"owner": "coordinator", "required_input": blocked_reason}]
    elif quality_pass and not cls["fixture"]:
        result["status"], exit_code = "PASS", EXIT_PASS
    elif quality_pass:
        result["status"], exit_code = "PARTIAL", EXIT_PASS
        result["blockers"] = [{"owner": "coordinator", "required_input": p} for p in qualification_prerequisites(cls)[:1]]
    else:
        result["status"], exit_code = "FAIL", EXIT_TESTED_FAILURE
    return result, exit_code


def elapsed_floor_problems(m: dict) -> list[str]:
    """The expiry protocol waits out a real TTL, so a total elapsed cap below
    TTL + margin + tail cannot complete it. Reported before any call."""
    budget = m.get("budget")
    cap = budget.get("max_elapsed_seconds") if isinstance(budget, dict) else None
    floor = forgotten_targets.expiry_wait_floor_seconds()
    if isinstance(cap, (int, float)) and not isinstance(cap, bool) and cap < floor:
        return [
            f"budget.max_elapsed_seconds={cap} is below {floor}s: the TTL-expiry protocol waits "
            f"{forgotten_targets.EXPIRY_TTL_SECONDS}s + {forgotten_targets.EXPIRY_MARGIN_SECONDS}s margin and needs "
            f"{forgotten_targets.EXPIRY_TAIL_SECONDS}s after it for the probes"
        ]
    return []


def dirty_message(dirty: list[str]) -> str:
    return (
        f"uncommitted changes under the harness code ({', '.join(dirty[:5])}); commit them so source_sha "
        "names the code that ran"
    )


def run_seed(args: argparse.Namespace, *, clock=time.time, sleep=time.sleep) -> tuple[dict, int]:
    """Seeds both qualification accounts under the same budget caps the live
    run uses, after proving each was empty. Never runs without
    seeding.authorized, never overwrites a receipt, stops at the first
    blocked step, and prints each receipt's confirmation string for the live
    manifest. The empty-case account's TTL expiry is waited out for real: the
    wait counts against max_elapsed_seconds, and a cap too short for it blocks
    the run before its first call. `clock` and `sleep` exist for tests."""
    started = time.time()
    corpus, facts, fact_by_id = load_corpus(args.corpus, args.facts)
    source_sha = git_source_sha(REPO_ROOT)
    corpus_sha256 = corpus["meta"]["corpus_hash_sha256"]
    facts_sha256 = corpus["meta"]["facts_hash_sha256"]

    m = manifest_lib.load_manifest(args.manifest)
    problems = manifest_lib.validate_live_manifest(m, manifest_lib.PHASE_SEED)
    problems += manifest_lib.validate_reviewer_freeze(
        m, source_sha=source_sha, corpus=corpus,
        facts_sha256=corpus["meta"]["facts_hash_sha256"],
        supplemental_sha256=forgotten_targets.targets_sha256(),
    )
    if m.get("corpus_sha256") != corpus_sha256:
        problems.append(f"manifest corpus_sha256={m.get('corpus_sha256')!r} does not match on-disk corpus={corpus_sha256!r}")
    if not args.receipt_dir:
        problems.append("--receipt-dir is required for --seed; receipts are the only attribution record")
    problems += elapsed_floor_problems(m)
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
    ledger = None
    if not problems:
        try:
            ledger = open_ledger(m, args.manifest, corpus)
        except ledger_lib.LedgerError as e:
            problems.append(f"ledger: {e}")
    if problems:
        return build_blocked_result(args, corpus, source_sha, problems), EXIT_BLOCKED

    cls = classify_run(m)
    guard = BudgetGuard(m["budget"], ledger, mode="seed")
    check = guard_check_for(guard)
    paths = receipt_paths(args.receipt_dir)
    outcomes: dict[str, seeding.SeedOutcome] = {}
    confirmations: dict[str, str] = {}
    blocked_reason: str | None = None
    blocked_kind: str | None = None
    for role, key in TARGETS:
        target = m[key]
        client = guard.client(target["endpoint_url"], tokens[role], target["allowed_origins"], role=role)
        outcome = seeding.seed_target(
            client, check, role=role, corpus=corpus, fact_by_id=fact_by_id, corpus_sha256=corpus_sha256,
            facts_sha256=facts_sha256, source_sha=source_sha, recorded_at=now_iso(),
            credential_secret_ref=target["credential_secret_ref"],
            preceded_by_seeded_facts=sum(len(o.receipt["seeded"]) for o in outcomes.values()),
            clock=clock, sleep=sleep, remaining_seconds=guard.remaining_seconds,
        )
        outcomes[role] = outcome
        try:
            # The ledger, not the client's counters, says what this account cost.
            outcome.receipt["ledger"] = ledger.provenance(guard.invocation, role)
            confirmations[role] = seeding.write_receipt(paths[role], outcome.receipt)
        except (OSError, ledger_lib.LedgerError) as e:
            confirmations[role] = ""
            outcome.receipt["complete"] = False
            outcome.blocked_reason = outcome.blocked_reason or f"receipt could not be written: {type(e).__name__}"
            outcome.blocked_kind = outcome.blocked_kind or "other"
        if outcome.blocked_reason:
            blocked_reason = f"{key}: {outcome.blocked_reason}"
            blocked_kind = outcome.blocked_kind
            break

    seeded = sum(len(o.receipt["seeded"]) for o in outcomes.values())
    result = base_result(args.task_id, args.profile, source_sha, cls["evidence_level"])
    result["corpus_sha256"] = corpus_sha256
    result["configuration_sha256"] = sha256_file(args.manifest)
    result["provider"] = provider_record(m, cls)
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
            criterion="Forgotten/expired protocol: each expected-empty target shown present, then removed by forget (3) or by a fixed absolute TTL shown absent only after the service clock passed it (2), with the still-present forget targets as a control; final inventory empty; cross-account sentinel unreachable",
            expected="presence rank<=5 for 5/5; forget expired=true for the 3 forget-mode targets; the 2 expire-mode targets absent from inventory, results and facts a full margin after valid_until; final inventory 0 and zero results/facts for 5/5; sentinel probe empty",
            observed=blocked_reason or (
                f"presence={len(empty_receipt.get('presence') or [])}/5, forgotten={len(empty_receipt.get('forget') or [])}/"
                f"{len(forgotten_targets.cases_with_mode(forgotten_targets.MODE_FORGET))}, "
                f"expired_absent={(empty_receipt.get('expiry_check') or {}).get('expired_absent_from_inventory')} "
                f"({(empty_receipt.get('expiry') or {}).get('probed_after_valid_until_seconds')}s past valid_until), "
                f"post_removal_inventory={(empty_receipt.get('post_forget') or {}).get('inventory_total')}, "
                f"sentinel_probe_passed={(empty_receipt.get('cross_account_probe') or {}).get('passed')}"
            ),
            status="PASS" if both_complete else "BLOCKED",
        ),
    ]
    ledger_report = ledger.report(guard.invocation)
    result["usage"] = {
        "calls_this_run": guard.calls_made(),
        "request_bytes": guard.request_bytes(),
        "response_bytes": guard.response_bytes(),
        "input_token_upper_bound_cumulative": ledger_report["cumulative_request_bytes"],  # one token per request byte
        "reserved_not_sent": guard.reserved_not_sent,  # reservations that landed past max_elapsed_seconds: counted, never sent
        "ledger": ledger_report,
    }
    result["elapsed_seconds"] = round(time.time() - started, 3)
    result["seed_receipts"] = {
        role: {"path": str(paths[role]), "confirmation": confirmations[role], "complete": outcomes[role].receipt["complete"]}
        for role in outcomes
    }
    record_cost(result, guard, [m["budget"]["authorization_ref"], m["seeding"]["authorization_ref"]])
    result["limitations"].append(COST_LIMITATION)
    result["limitations"].append(ledger_lib.OPERATOR_GUARD_STATEMENT)
    result["limitations"] += classification_limitations(cls)
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
    # The handshake is two charged requests: `initialize`, then the
    # `notifications/initialized` notification (params None: no id, no reply body).
    init = [("initialize", mcp_client.INITIALIZE_PARAMS), (mcp_client.INITIALIZED_METHOD, None)]
    inventory = recall({"limit": seeding.INVENTORY_LIMIT})
    probes = [recall({"limit": 1}), recall({"query": seeding.EMPTY_PROBE_QUERY, "limit": K})]
    empty = empty_cases(corpus)
    sentinel = recall({"query": forgotten_targets.SENTINEL_FACT_TEXT, "limit": K})
    empty_queries = [recall({"query": c["query"], "limit": K}) for c in empty]

    def remembers(role: str) -> list:
        out = []
        for fid, text in seeding.plan_facts(corpus, fact_by_id, role):
            args = {
                "fact": text,
                "provenance": seeding.provenance(corpus_sha256, fid),
                "operation_key": seeding.operation_key(corpus_sha256, fid),
            }
            if seeding.target_mode(fid) == forgotten_targets.MODE_EXPIRE:
                args["ttl"] = seeding.iso_utc(0)  # every instant this century serializes to the same 20 bytes
            out.append(tool("remember", args))
        return out

    by_id = {c["id"]: c for c in empty}
    expire_queries = [recall({"query": by_id[cid]["query"], "limit": K}) for cid in forgotten_targets.cases_with_mode(forgotten_targets.MODE_EXPIRE)]
    forgets = [
        tool("forget", {"id": "0" * 64, "reason": "T23.43 expected-empty qualification: forgotten target"})
        for _ in forgotten_targets.cases_with_mode(forgotten_targets.MODE_FORGET)
    ]
    requests = {
        (seeding.ROLE_POSITIVE, "seed"): [*init, *probes, *remembers(seeding.ROLE_POSITIVE), inventory],
        (seeding.ROLE_EMPTY_CASE, "seed"): [
            *init, *probes, *remembers(seeding.ROLE_EMPTY_CASE), inventory,
            *empty_queries,  # presence, all five
            inventory, *expire_queries,  # after the wait: expire-mode targets gone, forget-mode targets still there
            *forgets, inventory, *empty_queries, sentinel,  # then the strict zero-current-facts proof
        ],
        (seeding.ROLE_POSITIVE, "live"): [
            *init, inventory, *[recall({"query": c["query"], "limit": K}) for c in positive_cases(corpus)]
        ],
        (seeding.ROLE_EMPTY_CASE, "live"): [*init, inventory, *empty_queries, sentinel],
    }
    size = lambda reqs: sum(len(mcp_client.frame_body(1000, m, p)) for m, p in reqs)  # noqa: E731
    plan: dict = {"per_role": {}}
    for role, _ in TARGETS:
        plan["per_role"][role] = {
            "facts_to_seed": len(seeding.plan_fact_ids(corpus, role)),
            "seed_calls": len(requests[(role, "seed")]),
            "live_calls": len(requests[(role, "live")]),
            "seed_request_bytes": size(requests[(role, "seed")]),
            "live_request_bytes": size(requests[(role, "live")]),
            "request_bytes": size(requests[(role, "seed")]) + size(requests[(role, "live")]),
        }
    plan["expiry"] = {
        "expire_cases": forgotten_targets.cases_with_mode(forgotten_targets.MODE_EXPIRE),
        "forget_cases": forgotten_targets.cases_with_mode(forgotten_targets.MODE_FORGET),
        "ttl_seconds": forgotten_targets.EXPIRY_TTL_SECONDS,
        "margin_seconds": forgotten_targets.EXPIRY_MARGIN_SECONDS,
        "tail_seconds": forgotten_targets.EXPIRY_TAIL_SECONDS,
        "min_elapsed_seconds": forgotten_targets.expiry_wait_floor_seconds(),
        "supplemental_sha256": forgotten_targets.targets_sha256(),
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
    ledger_summary = None
    if not [p for p in problems if p.startswith("budget.")]:
        if phase == manifest_lib.PHASE_SEED:
            problems += elapsed_floor_problems(m)
        # What is already charged to this authorization, from the ledger. A
        # ledger that was never initialized is fine before the seed (this
        # preflight is how an operator checks the caps BEFORE binding them with
        # --init-ledger, and the seed itself blocks until that step is done) and
        # a problem before the live run (receipts cannot stand in for it). A
        # ledger that is empty, damaged, or missing next to its lock file is a
        # problem in both phases.
        used_calls = used_bytes = 0
        try:
            ledger = open_ledger(m, args.manifest, corpus)
            state = ledger.snapshot()
            used_calls, used_bytes = state.calls, state.request_bytes
            ledger_summary = ledger.report()
        except ledger_lib.LedgerError as e:
            never_initialized = "no ledger exists" in str(e)
            if never_initialized and phase == manifest_lib.PHASE_SEED:
                ledger_summary = {
                    "state": "not initialized",
                    "next_step": "after this preflight passes, run --init-ledger with the same manifest; it binds these caps to "
                                 "budget.authorization_ref, and --seed blocks until it has been done",
                }
            else:
                problems.append(f"ledger: {e}")
        needs = plan["per_role"]
        need_calls = sum(r["live_calls"] + (r["seed_calls"] if phase == manifest_lib.PHASE_SEED else 0) for r in needs.values())
        need_bytes = sum(r["live_request_bytes"] + (r["seed_request_bytes"] if phase == manifest_lib.PHASE_SEED else 0) for r in needs.values())
        scope = "the full seed+live plan" if phase == manifest_lib.PHASE_SEED else "the live plan"
        charged = f" (plus {used_calls} already charged in the ledger)" if used_calls else ""
        if used_calls + need_calls > budget["max_calls"]:
            problems.append(f"budget.max_calls={budget['max_calls']} is below the {need_calls} calls {scope} needs{charged}")
        if used_bytes + need_bytes > budget["max_input_tokens"]:
            problems.append(
                f"budget.max_input_tokens={budget['max_input_tokens']} is below the {need_bytes} "
                f"{scope} needs{charged} (one token per serialized request byte, an upper bound)"
            )
        if (used_calls + need_calls) * budget["max_cost_per_call_usd"] > budget["approved_max_usd"]:
            problems.append(
                f"budget.approved_max_usd={budget['approved_max_usd']} is below {used_calls + need_calls} calls x "
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
    if ledger_summary is not None:
        result["ledger"] = ledger_summary
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


def run_init_ledger(args: argparse.Namespace) -> tuple[dict, int]:
    """The explicit step that begins a qualification's cumulative ledger. It runs
    the seed-phase preflight first (manifest, pins, credentials present, clean
    tree, caps that cover the whole plan), because the caps it binds are the
    authorization's caps from then on: changing them never resets the ledger. It
    makes no network call and spends nothing. It refuses a path where a ledger,
    an empty file or a lock file already exists; a new authorization takes a new
    path, chosen on purpose. Nothing else creates a ledger."""
    args.phase = manifest_lib.PHASE_SEED
    result, code = run_preflight(args)
    if code != EXIT_PASS:
        return result, code
    corpus, _facts, _fact_by_id = load_corpus(args.corpus, args.facts)
    m = manifest_lib.load_manifest(args.manifest)
    try:
        ledger = initialize_ledger(m, args.manifest, corpus)
        report = ledger.report()
    except ledger_lib.LedgerError as e:
        source_sha = git_source_sha(REPO_ROOT)
        return build_blocked_result(args, corpus, source_sha, [f"ledger: {e}"]), EXIT_BLOCKED
    result["commands"] = [
        f"python3 {Path(__file__).relative_to(REPO_ROOT)} --init-ledger --manifest {args.manifest} --output {args.output}"
    ]
    result["ledger"] = report
    result["acceptance"] = [
        acceptance_row(
            criterion="Cumulative ledger initialized for this authorization, on purpose, at a path where nothing existed",
            expected="a new ledger bound to budget.authorization_ref, the four caps and the corpus/provider/environment identity",
            observed=f"ledger {report['ledger_id']} created; {report['cumulative_calls']} calls charged; remaining_calls={report['remaining_calls']}",
            status="PASS",
        )
    ]
    result["limitations"].append(ledger_lib.OPERATOR_GUARD_STATEMENT)
    result["limitations"].append(
        "Initializing the ledger measures nothing and spends nothing; it starts no run. Status stays PARTIAL."
    )
    return result, EXIT_PASS


def parse_args(argv: list[str]) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__)
    mode = p.add_mutually_exclusive_group(required=True)
    mode.add_argument("--fixtures", action="store_true", help="local synthetic provider only; no network")
    mode.add_argument("--preflight", action="store_true", help="offline readiness check for --seed/--live; no network")
    mode.add_argument("--init-ledger", action="store_true", help="begin the cumulative budget ledger for this authorization (offline; never resets one)")
    mode.add_argument("--seed", action="store_true", help="seed both hosted accounts (writes; needs seeding.authorized and an initialized ledger)")
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


def safe_source_sha() -> str:
    try:
        return git_source_sha(REPO_ROOT)
    except (OSError, subprocess.SubprocessError):
        return "unavailable"


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
        elif args.init_ledger:
            result, exit_code = run_init_ledger(args)
        elif args.seed:
            result, exit_code = run_seed(args)
        else:
            result, exit_code = run_live(args)
    except (OSError, ValueError) as e:
        # Unreadable or malformed manifest/receipt: invalid input, but the
        # contract still requires a result file explaining why.
        corpus = json.loads(args.corpus.read_text(encoding="utf-8"))
        result = build_blocked_result(args, corpus, safe_source_sha(), [f"invalid input: {type(e).__name__}: {e}"], evidence_level="static")
        exit_code = EXIT_INVALID_INPUT
    except Exception as e:  # noqa: BLE001 -- backstop: the contract says a result file is always written
        # Ordinary exceptions only (KeyboardInterrupt and SystemExit still
        # propagate). The class name is the whole message: the text of an
        # unexpected error can carry a path or a value nobody reviewed. Spend is
        # safe regardless: the ledger recorded every reservation before its request.
        corpus = json.loads(args.corpus.read_text(encoding="utf-8"))
        result = build_blocked_result(args, corpus, safe_source_sha(), [f"unexpected error: {type(e).__name__}"], evidence_level="static")
        exit_code = EXIT_BLOCKED

    write_result(args.output, result)
    print(f"eval_embeddings: status={result['status']} written to {args.output}")
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
