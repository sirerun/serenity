# Check a plan

`serenity check` evaluates a plan against every active constraint precept
in the brain's ledger (DIRECTION v1 `check_plan`, RFC 0001 §8.3, ADR 010)
and, in its text report, audits the brain's entity pages.

```sh
serenity -C /path/to/brain check "Spend 500 on the new vendor"
serenity -C /path/to/brain check --actions '[{"action":"spend_over","params":{"amount":500}}]'
serenity -C /path/to/brain check --actions '[...]' --json
serenity -C /path/to/brain check --pages
```

Plan text and `--actions` are mutually exclusive. `--claude-hook` reads a
Claude Code `PreToolUse` envelope from stdin instead; `serenity connect
claude` installs that form as the `ExitPlanMode` gate (see
[Connect Claude Code](claude.md)).

## Exit codes

Exit codes follow ADR 010 and never vary by output mode:

| Verdict | Exit |
| --- | --- |
| `pass` | 0 |
| `no_applicable_constraints` | 0 (the verdict string, in stdout or `--json`, tells it apart from `pass`) |
| `violated` | 2 |
| `unverified`, or any error (bad input, ledger read failure, unknown action) | 1 |

Free-text plans report `unverified` until a classification model is
configured; `--actions` never needs one.

## Text report

```
status: violated
considered: 1 active constraint(s)
violated: cst-0001
  why_not: Unbounded spend risk: "no ceiling" was rejected outright.
  revisit_if: quarterly budget review
quarantined pages: 0
non-conforming slugs: 0
```

The status line prints the verdict verbatim; each violated constraint's
`why_not` and `revisit_if` are printed exactly as stored.

## Page audit: `quarantined pages` and `non-conforming slugs`

Every text report ends with two sections about the entity pages under
`brain/entities/`. Both are warning class in ADR 010's sense: they are
printed for the operator and never move the exit code, which follows the
plan verdict alone. `--json` output is unchanged; it carries the
`check_plan` wire shape only.

```
quarantined pages: 1
  brain/entities/person/injected.md: entity frontmatter: yaml: unmarshal errors: line 2: mapping key "type" already defined at line 1
non-conforming slugs: 1
  brain/entities/person/Bad_Slug.md: slug "Bad_Slug"
```

Paths are relative to the brain root. Each entry is one line, so the
sections are greppable.

**quarantined pages** lists every page whose frontmatter cannot be parsed
(for example a duplicate YAML key) or that parses with no slug. Such a
page is quarantined rather than fatal: `serenity sync`'s index rebuild,
`serenity ask`, MCP `synthesize` and the MCP `entity` tool all skip it,
log its path once, and continue with the rest of the brain. Nothing from
a quarantined page is indexed or served; a stale index row for it is
refused on read. Fix the page's frontmatter by hand (or delete the page)
and run `serenity sync`; the entry disappears once the page parses.

**non-conforming slugs** lists every parsable page whose `slug:` does not
satisfy the canonical slug grammar: lowercase ASCII letters, digits and
hyphens, 1 to 64 characters, never starting or ending with a hyphen. The
remediation of deep review finding SEC-H03 makes the writer refuse a new
page whose slug fails that grammar; this section finds the pages that
already exist so you can rename them before the same rule is enforced on
read. Renaming means changing the `slug:` value
and the file name together, then running `serenity sync`. Nothing is
renamed automatically.

`--pages` prints only these two sections, with no plan verdict, and
exits 0 whenever the pages could be listed at all. It cannot be combined
with plan text, `--actions`, `--json` or `--claude-hook`.
