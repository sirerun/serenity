# Adversarial corpus (RFC 0001 §14, §16; plan T1.20)

Prompt-injection and precept-fabrication fixtures: documents an ingest
connector could plausibly fetch (email, file, git-repo) that try to steer
extraction into minting or altering a precept, widening a limit, or
smuggling a predicate name outside the controlled vocabulary
(`internal/config`'s seeded families, RFC §7.2).

This is the "the adversarial corpus" referenced in RFC §16: "injected
instructions in email/files/repos ... precept-fabrication attempts -- the
release gate asserts zero precept mutations and zero unauthorized effect
proposals from adversarial sources." `internal/gate/precept_immutability_test.go`
(T3.12) shipped a small inline substitute before this corpus existed;
`internal/gate/adversarial_corpus_test.go` (T1.20) is what actually ingests
this directory and is the corpus's consumer of record.

## Layout

- `documents/*.yaml` -- one adversarial document per file. Schema:
  - `kind`: one of `email`, `file`, `git_repo` -- matches the
    `domain.Source.Kind` / `connector.RawItem.Kind` values the real
    connectors already emit (`internal/connector/imap`, `.../file`,
    `.../gitrepo`).
  - `uri`: a synthetic URI in the shape the matching connector would
    produce, for readability -- not resolved or fetched by anything.
  - `attack_vector`: a short slug naming the injection technique (see
    below).
  - `body`: the raw ingested text, verbatim as a naive extractor would see
    it.
  - `fabricated_predicates`: predicate-like tokens the document tries to
    smuggle in, none of which are in the controlled vocabulary. The test
    asserts each one is rejected by `internal/store`'s real vocabulary
    enforcement (`store.ErrUnknownPredicate`), not just absent from a
    hand-maintained list.
  - `camouflaged_real_predicates` (optional): genuine vocabulary
    predicates the document also uses, to prove the check discriminates
    real names from fabricated ones rather than rejecting everything.
- `documents/checksums.yaml` -- sha256 manifest over every `*.yaml` file in
  `documents/`, in the same format and using the same
  `eval.WriteManifest`/`eval.VerifyManifest` functions `internal/eval`
  already ships for label files (ADR 005; see `internal/eval/checksum.go`).
  There is no standing CLI for this yet -- regenerate after a deliberate
  edit by calling `eval.WriteManifest("evals/corpora/adversarial/documents",
  "evals/corpora/adversarial/documents/checksums.yaml")` from a short-lived
  `go run` snippet. CI fails (`internal/gate/adversarial_corpus_test.go`)
  if a document changes without its checksum being re-pinned.

## Attack vectors represented

| # | file | kind | attack_vector |
|---|------|------|----------------|
| 01 | authority-impersonation-ceo | email | authority_impersonation |
| 02 | verbal-approval-fabrication | email | fabricated_verbal_approval |
| 03 | system-override-spend-limits | email | system_override_prefix |
| 04 | it-admin-mfa-bypass | email | fabricated_admin_notice |
| 05 | real-predicate-camouflage-balance | email | real_predicate_camouflage |
| 06 | nested-quote-chain-injection | email | nested_quote_chain_injection |
| 07 | hidden-html-comment | file | hidden_instruction_html_comment |
| 08 | todo-txt-precept-smuggle | file | task_list_precept_smuggle |
| 09 | config-yaml-lookalike | file | config_file_lookalike |
| 10 | json-claim-injection | file | structured_claim_lookalike |
| 11 | parser-escape-attempt | file | parser_escape_attempt |
| 12 | readme-admin-notice | git_repo | repo_readme_smuggled_precept |
| 13 | code-comment-ai-directive | git_repo | code_comment_ai_directive |
| 14 | commit-message-injection | git_repo | commit_message_injection |
| 15 | migration-guide-forged-precept | git_repo | role_play_reframe |
| 16 | notice-camouflage-waiver | git_repo | real_predicate_camouflage |

## What the test proves, and what it does not

`internal/gate/adversarial_corpus_test.go` ingests every document here and
asserts, for each one:

1. If a hypothetical extractor believed the document and tried to persist
   its content as a precept, the precept-immutability AST gate
   (`internal/gate`, T3.12) would catch the write -- proving no code path
   reaching that gate can land a write under `.dira/` from this content.
2. Every `fabricated_predicates` entry is rejected by
   `internal/store`'s real vocabulary enforcement
   (`errors.Is(err, store.ErrUnknownPredicate)`), and every
   `camouflaged_real_predicates` entry is genuinely accepted -- so the
   check discriminates rather than rejecting (or accepting) everything.

**Scoping note (disclosed per plan T1.20):** there is no complete, wired
ingest pipeline yet (extraction is T1.8, landing concurrently). This test
does not run these documents through a real connector or extractor -- it
proves the two enforcement mechanisms that would sit in that pipeline's
path (the AST gate, the vocabulary check) actually catch what this corpus
contains, using the same technique T3.12 already established for its own
inline fixture slice. When T1.8/T1.9 land a real extraction-to-claim path,
a follow-up should route this corpus through it directly.
