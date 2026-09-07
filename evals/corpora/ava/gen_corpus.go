//go:build ignore

// Command gen_corpus generates the T1.14 "Ava Standardo" extraction corpus
// (RFC 0001 §16, plan T1.14), expanded by T1.32 to give the held-out split
// enough scored units per family for a real bootstrap confidence interval
// (chief-architect's finding: at 4 units, a true recall of 0.85 fails a
// point-estimate 0.80 bar about one run in three from sampling noise
// alone; at 20 units, under one in eight -- see docs/lore.md and
// docs/plans/E1-m1-ingest.md T1.32). One YAML label file per span, a
// held-out split file, an embedded-contradiction-pairs index, and a
// checksum manifest -- reusing internal/eval's WriteManifest unmodified
// (ADR-005), the same tooling T1.13/T1.20/T3.13 already use.
//
// Layout per predicate family, generated from one hand-authored persona
// dataset below (a coherent Ava Standardo timeline): 44 spans total --
// 20 "regular" spans (5 facts x 4 original phrasings, T1.14's original
// train pool, UNCHANGED by T1.32 down to the byte) + 20 "extra" spans (the
// same 5 facts x 4 NEW T1.32 phrasings, entirely held out) + 4
// hand-written contradiction spans (T1.14's original, unchanged, never
// held out). Held-out total per family: 24 (4 from the original scheme's
// heldOutFactPositions + all 20 of the T1.32 extra block) -- comfortably
// above T1.32's >= 20 floor. The T1.32 extra block is purely additive: no
// span that was previously train moves to held-out, and no span that was
// previously held-out moves to train, so every familyGuidance few-shot
// example already cross-checked against the old split.yaml (T1.28/T1.29)
// stays valid without re-checking.
//
// Every sentence is still real, family-appropriate English -- there is no
// "{subject} {predicate} {object}" placeholder text anywhere in the
// output. The T1.32 phrasings simulate more of the document-type
// diversity a real per-connector extraction P/R eval needs (insurance
// forms, patient portals, OKR docs, reimbursement requests, and so on) on
// top of T1.14's original four (plain statement, formal record, chat/
// email, bio-style).
//
// Regenerate after a deliberate edit to the dataset below:
//
//	go run evals/corpora/ava/gen_corpus.go
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/sirerun/serenity/internal/eval"
)

const (
	corpusDir = "evals/corpora/ava"
	labelsDir = corpusDir + "/labels"
	// manifestPath deliberately lives one directory ABOVE labelsDir,
	// unlike T1.20/T3.13's manifest-inside-labels-dir layout. Those two
	// corpora each wrote their own bespoke loader (direction.LoadRows,
	// gate.loadAdversarialCorpus) that explicitly skips a file named
	// "checksums.yaml"; this corpus is the first real consumer of the
	// generic eval.LoadLabels, which has no such exclusion (it treats
	// every *.yaml directly under the directory as a label file). Rather
	// than modify T1.13's shared, already-merged code, this places the
	// manifest outside the directory eval.LoadLabels scans -- a
	// configuration WriteManifest/VerifyManifest already fully support
	// unmodified via their own "does the manifest live inside labelsDir"
	// basename check (checksum.go's computeManifest).
	manifestPath = corpusDir + "/checksums.yaml"
	splitPath    = corpusDir + "/split.yaml"
	contraPath   = corpusDir + "/contradictions.yaml"
)

// record mirrors internal/eval.Label's exact YAML shape (span, expected:
// {predicate, object, valid_from, valid_to}, labeler, adjudicated) so every
// file this program writes round-trips through eval.LoadLabels unchanged,
// plus two forward-compatible extra fields (yaml.v3 -- and eval.LoadLabels
// -- silently ignore fields a struct doesn't declare, the same
// forward-compatibility label.go's doc comment describes) that embed the
// contradiction-pair linkage directly in the label file: which pair a span
// belongs to, and which side of the pair it argues.
type record struct {
	Span     string `yaml:"span"`
	Expected struct {
		Predicate string `yaml:"predicate"`
		Object    string `yaml:"object"`
		ValidFrom string `yaml:"valid_from,omitempty"`
		ValidTo   string `yaml:"valid_to,omitempty"`
	} `yaml:"expected"`
	Labeler             string `yaml:"labeler"`
	Adjudicated         bool   `yaml:"adjudicated"`
	ContradictionPairID string `yaml:"contradiction_pair_id,omitempty"`
	ContradictionRole   string `yaml:"contradiction_role,omitempty"`
}

// fact is one (span, object, validity window) triple before labeler/
// adjudication/held-out/contradiction metadata is stamped on in
// buildFamily.
type fact struct {
	span      string
	object    string
	validFrom string
	validTo   string
}

func mk(span, object, validFrom, validTo string) fact {
	return fact{span: span, object: object, validFrom: validFrom, validTo: validTo}
}

// gen1 renders a single-placeholder template list against one row of
// (field, object, validFrom, validTo) data, producing len(tmpl) facts --
// simulating the same underlying claim as it would appear across that many
// differently-phrased documents.
func gen1(tmpl []string, field, object, validFrom, validTo string) []fact {
	facts := make([]fact, 0, len(tmpl))
	for _, t := range tmpl {
		facts = append(facts, mk(fmt.Sprintf(t, field), object, validFrom, validTo))
	}
	return facts
}

// gen2 is gen1 for a two-placeholder template list, applied in a fixed
// (fieldA, fieldB) argument order across every template in the list.
func gen2(tmpl []string, fieldA, fieldB, object, validFrom, validTo string) []fact {
	facts := make([]fact, 0, len(tmpl))
	for _, t := range tmpl {
		facts = append(facts, mk(fmt.Sprintf(t, fieldA, fieldB), object, validFrom, validTo))
	}
	return facts
}

// familySpec is one predicate family's generated corpus slice: 20
// original "regular" spans (5 facts x 4 original phrasings -- T1.14,
// unchanged), 20 "extra" spans (the same 5 facts x 4 NEW T1.32 phrasings,
// entirely held out), and exactly 4 hand-written contradiction spans (2
// phrased restatements of "claim A", 2 of "claim B") that are NOT run
// through the generic templates -- a genuine logical conflict needs
// editorial control over both sides' wording, which a shared template
// list for an unrelated set of regular facts cannot guarantee.
type familySpec struct {
	predicate     string
	regular       []fact  // exactly 20, in fixed order (T1.14, unchanged)
	extraHeldOut  []fact  // exactly 20, in fixed order (T1.32, all held out)
	contradiction [4]fact // [0],[1] = claim A restated twice; [2],[3] = claim B restated twice
	pairWhy       string  // human-readable conflict description for contradictions.yaml
}

func buildFamilies() []familySpec {
	var out []familySpec

	// works_at -- Ava's employer history. The contradiction: as of
	// February 2026, one source (HR record) still shows Acme Corp as her
	// current employer while another (a welcome email) shows Beta LLC as
	// her new employer, both open-ended from the same month.
	{
		tmpl := []string{
			"Ava Standardo works at %s.",
			"According to her HR record, Ava Standardo is employed by %s.",
			"In a Slack intro, Ava mentioned she's now with %s.",
			"Ava's LinkedIn-style bio lists her current employer as %s.",
		}
		tmplExtra := []string{
			"A February 2026 badge-system export lists Ava Standardo's employer as %s.",
			"Ava's pay stub identifies her employer as %s.",
			"In her conference speaker bio, Ava lists %s as where she currently works.",
			"Ava's benefits enrollment form names %s as her employer.",
		}
		rows := []struct{ field, object, from, to string }{
			{"Contoso Systems", "contoso-systems", "2019-01", "2020-12"},
			{"Initech", "initech", "2021-01", "2022-06"},
			{"Globex Corporation", "globex-corporation", "2022-07", "2023-12"},
			{"Northwind Traders", "northwind-traders", "2024-01", "2025-05"},
			{"Acme Corp", "acme-corp", "2025-06", "2026-01"},
		}
		var regular, extra []fact
		for _, r := range rows {
			regular = append(regular, gen1(tmpl, r.field, r.object, r.from, r.to)...)
			extra = append(extra, gen1(tmplExtra, r.field, r.object, r.from, r.to)...)
		}
		out = append(out, familySpec{
			predicate:    "works_at",
			regular:      regular,
			extraHeldOut: extra,
			contradiction: [4]fact{
				mk("As of February 2026, Ava Standardo's HR record still lists Acme Corp as her employer.", "acme-corp", "2026-02", ""),
				mk("Payroll's February 2026 export still runs Ava Standardo's paycheck through Acme Corp.", "acme-corp", "2026-02", ""),
				mk("Ava Standardo joined Beta LLC in February 2026 as her new employer.", "beta-llc", "2026-02", ""),
				mk("Beta LLC's February 2026 new-hire welcome email addresses Ava Standardo as its newest employee.", "beta-llc", "2026-02", ""),
			},
			pairWhy: "Two sources both claim to be Ava's current (open-ended) employer starting the same month -- she cannot be employed by both Acme Corp and Beta LLC at once.",
		})
	}

	// has_role -- mirrors the same career timeline; object is the role,
	// not the company. Contradiction lands in the same disputed month.
	{
		tmpl := []string{
			"Ava Standardo's job title is %s.",
			"Per the org chart, Ava holds the role of %s.",
			"In her performance review, Ava is listed as a %s.",
			"Ava introduced herself on a customer call as a %s.",
		}
		tmplExtra := []string{
			"Ava's conference speaker bio lists her title as %s.",
			"The team wiki's org page shows Ava's role as %s.",
			"Ava's business card reads \"Ava Standardo, %s.\"",
			"In a project kickoff doc, Ava is credited as the %s.",
		}
		rows := []struct{ field, object, from, to string }{
			{"QA Analyst", "qa-analyst", "2019-01", "2020-12"},
			{"Backend Engineer", "backend-engineer", "2021-01", "2022-06"},
			{"Senior Backend Engineer", "senior-backend-engineer", "2022-07", "2023-12"},
			{"Engineering Manager", "engineering-manager", "2024-01", "2025-05"},
			{"Staff Engineer", "staff-engineer", "2025-06", "2026-01"},
		}
		var regular, extra []fact
		for _, r := range rows {
			regular = append(regular, gen1(tmpl, r.field, r.object, r.from, r.to)...)
			extra = append(extra, gen1(tmplExtra, r.field, r.object, r.from, r.to)...)
		}
		out = append(out, familySpec{
			predicate:    "has_role",
			regular:      regular,
			extraHeldOut: extra,
			contradiction: [4]fact{
				mk("As of February 2026, Ava is still listed as Staff Engineer.", "staff-engineer", "2026-02", ""),
				mk("Ava's February 2026 badge-system profile still shows her title as Staff Engineer.", "staff-engineer", "2026-02", ""),
				mk("A promotion announcement in February 2026 named Ava Standardo Engineering Manager.", "engineering-manager", "2026-02", ""),
				mk("Ava's new February 2026 email signature reads \"Ava Standardo, Engineering Manager.\"", "engineering-manager", "2026-02", ""),
			},
			pairWhy: "One source keeps Ava's title at Staff Engineer from February 2026 onward while another has her promoted to Engineering Manager the same month -- one title is stale.",
		})
	}

	// owns_account -- financial/software accounts. Contradiction: does
	// Ava's Chase checking account still exist in February 2026, or was
	// it closed -- a genuine either/or about the same account.
	{
		tmpl := []string{
			"Ava Standardo owns a %s.",
			"Ava's records show she holds a %s.",
			"An onboarding email confirms Ava was granted a %s.",
			"Ava's password manager lists a saved login for her %s.",
		}
		tmplExtra := []string{
			"Ava's tax documents reference a %s under her name.",
			"A support ticket Ava filed mentions her %s.",
			"Ava's estate-planning worksheet lists a %s among her assets.",
			"In a chat with IT, Ava confirmed she still has a %s.",
		}
		rows := []struct{ field, object, from, to string }{
			{"Chase checking account", "chase-checking", "2018-03", ""},
			{"Fidelity 401k account", "fidelity-401k", "2019-05", ""},
			{"GitHub organization account", "github-org-account", "2020-09", ""},
			{"AWS billing account", "aws-billing-account", "2021-11", ""},
			{"Coinbase wallet", "coinbase-wallet", "2022-04", ""},
		}
		var regular, extra []fact
		for _, r := range rows {
			regular = append(regular, gen1(tmpl, r.field, r.object, r.from, r.to)...)
			extra = append(extra, gen1(tmplExtra, r.field, r.object, r.from, r.to)...)
		}
		out = append(out, familySpec{
			predicate:    "owns_account",
			regular:      regular,
			extraHeldOut: extra,
			contradiction: [4]fact{
				mk("Ava's bank records show her Chase checking account is still open and active as of February 2026.", "chase-checking-active", "2026-02", ""),
				mk("A February 2026 statement was mailed to Ava for her still-open Chase checking account.", "chase-checking-active", "2026-02", ""),
				mk("A February 2026 closure notice confirms Ava's Chase checking account was closed and no longer exists.", "chase-checking-closed", "2026-02", ""),
				mk("Ava's budgeting app stopped syncing her Chase checking account in February 2026 because the bank reported it closed.", "chase-checking-closed", "2026-02", ""),
			},
			pairWhy: "One source shows Ava's Chase checking account still open in February 2026 while another shows it closed that same month -- an account cannot be both.",
		})
	}

	// has_balance (shard tier) -- numeric account balances. Contradiction:
	// two disagreeing figures for the same account in the same month.
	{
		tmpl := []string{
			"Ava's %s balance is %s.",
			"The statement shows Ava's %s at %s.",
			"Ava's banking app displays a current %s balance of %s.",
			"A finance summary lists Ava's %s holding %s.",
		}
		tmplExtra := []string{
			"Ava's year-end financial summary lists her %s balance at %s.",
			"A screenshot Ava shared with her accountant shows her %s at %s.",
			"The mobile app's account overview page shows Ava's %s balance as %s.",
			"Ava's monthly reconciliation spreadsheet records her %s at %s.",
		}
		rows := []struct{ acct, amountText, object, from, to string }{
			{"Chase checking", "$4,230.18", "4230.18-usd", "2026-03", ""},
			{"Fidelity 401k", "$88,410.02", "88410.02-usd", "2026-03", ""},
			{"Coinbase wallet", "$2,015.67", "2015.67-usd", "2026-02", ""},
			{"AWS billing account", "$312.00 owed", "312.00-usd-owed", "2026-04", ""},
			{"GitHub organization account", "$0.00", "0.00-usd", "2026-04", ""},
		}
		var regular, extra []fact
		for _, r := range rows {
			regular = append(regular, gen2(tmpl, r.acct, r.amountText, r.object, r.from, r.to)...)
			extra = append(extra, gen2(tmplExtra, r.acct, r.amountText, r.object, r.from, r.to)...)
		}
		out = append(out, familySpec{
			predicate:    "has_balance",
			regular:      regular,
			extraHeldOut: extra,
			contradiction: [4]fact{
				mk("According to the March 2026 bank statement, Ava's Chase checking balance was $4,230.18.", "4230.18-usd", "2026-03", ""),
				mk("The PDF statement Ava's bank mailed for March 2026 lists her Chase checking balance at $4,230.18.", "4230.18-usd", "2026-03", ""),
				mk("Ava's budgeting app showed her Chase checking balance at $5,910.44 for March 2026.", "5910.44-usd", "2026-03", ""),
				mk("A March 2026 screenshot Ava sent her accountant shows Chase checking at $5,910.44.", "5910.44-usd", "2026-03", ""),
			},
			pairWhy: "Two sources report different balances for the same Chase checking account in the same month -- at most one figure is current.",
		})
	}

	// has_condition -- health conditions. Contradiction: is her lower
	// back strain still active or resolved, as of the same month.
	{
		tmpl := []string{
			"Ava Standardo has been diagnosed with %s.",
			"Ava's medical intake form lists %s as an ongoing condition.",
			"In a message to her manager, Ava mentioned she has %s.",
			"Ava's wellness app tracks %s as a chronic condition.",
		}
		tmplExtra := []string{
			"Ava's insurance claim form lists %s as a pre-existing condition.",
			"In a note to HR requesting accommodations, Ava mentioned %s.",
			"Ava's fitness tracker app flags %s under her health profile.",
			"A referral letter from Ava's primary care doctor cites %s.",
		}
		rows := []struct{ field, object, from, to string }{
			{"seasonal allergies", "seasonal-allergies", "2015-04", ""},
			{"mild asthma", "mild-asthma", "2010-01", ""},
			{"chronic migraine", "chronic-migraine", "2020-06", ""},
			{"generalized anxiety", "generalized-anxiety", "2021-09", ""},
			{"a lower back strain", "lower-back-strain", "2024-11", ""},
		}
		var regular, extra []fact
		for _, r := range rows {
			regular = append(regular, gen1(tmpl, r.field, r.object, r.from, r.to)...)
			extra = append(extra, gen1(tmplExtra, r.field, r.object, r.from, r.to)...)
		}
		out = append(out, familySpec{
			predicate:    "has_condition",
			regular:      regular,
			extraHeldOut: extra,
			contradiction: [4]fact{
				mk("Ava's physical therapy chart lists her lower back strain as still active in February 2026.", "lower-back-strain-active", "2026-02", ""),
				mk("A February 2026 PT progress note keeps Ava's lower back strain open as an active issue.", "lower-back-strain-active", "2026-02", ""),
				mk("Ava's annual physical in February 2026 recorded her lower back strain as fully resolved with no ongoing symptoms.", "lower-back-strain-resolved", "2026-02", ""),
				mk("Ava's February 2026 physical exam notes close out the lower back strain as resolved.", "lower-back-strain-resolved", "2026-02", ""),
			},
			pairWhy: "One record keeps Ava's lower back strain open as active in February 2026 while another closes it out as resolved the same month -- it cannot be both.",
		})
	}

	// takes_medication -- contradiction: is Ava still taking sertraline
	// or did she stop, both claimed the same month.
	{
		tmpl := []string{
			"Ava Standardo takes %s.",
			"Ava's medication list includes %s.",
			"A pharmacy refill record shows Ava picked up %s.",
			"Ava mentioned to her doctor that she's been taking %s.",
		}
		tmplExtra := []string{
			"Ava's insurance claim lists a prescription for %s.",
			"In a text to her sister, Ava mentioned she just refilled %s.",
			"Ava's patient portal medication list includes %s.",
			"In a note to her care team, Ava confirmed she is still taking %s.",
		}
		rows := []struct{ field, object, from, to string }{
			{"Zyrtec daily", "zyrtec", "2016-01", ""},
			{"an albuterol inhaler as needed", "albuterol-inhaler", "2010-01", ""},
			{"sumatriptan as needed for migraines", "sumatriptan", "2020-07", ""},
			{"sertraline daily", "sertraline", "2021-10", ""},
			{"melatonin nightly", "melatonin", "2023-02", ""},
		}
		var regular, extra []fact
		for _, r := range rows {
			regular = append(regular, gen1(tmpl, r.field, r.object, r.from, r.to)...)
			extra = append(extra, gen1(tmplExtra, r.field, r.object, r.from, r.to)...)
		}
		out = append(out, familySpec{
			predicate:    "takes_medication",
			regular:      regular,
			extraHeldOut: extra,
			contradiction: [4]fact{
				mk("Ava's pharmacy record shows an active sertraline prescription refilled in February 2026.", "sertraline-active", "2026-02", ""),
				mk("A February 2026 pharmacy auto-refill notice went out for Ava's sertraline.", "sertraline-active", "2026-02", ""),
				mk("Ava told her therapist in February 2026 that she had stopped taking sertraline entirely.", "sertraline-discontinued", "2026-02", ""),
				mk("Ava's February 2026 therapy notes record that she discontinued sertraline on her own.", "sertraline-discontinued", "2026-02", ""),
			},
			pairWhy: "One record shows an active sertraline refill for Ava in February 2026 while another has her stopping it that same month.",
		})
	}

	// prefers -- contradiction: which editor Ava currently prefers.
	{
		tmpl := []string{
			"Ava prefers %s.",
			"Ava mentioned in a team survey that she prefers %s.",
			"Ava's standing order confirms she prefers %s.",
			"According to her setup notes, Ava prefers %s.",
		}
		tmplExtra := []string{
			"Ava's team onboarding doc notes that she prefers %s.",
			"In a retro survey, Ava wrote that she prefers %s.",
			"Ava's assistant's briefing notes mention she prefers %s.",
			"A colleague's calendar invite note reminds the team Ava prefers %s.",
		}
		rows := []struct{ field, object, from, to string }{
			{"dark roast coffee", "dark-roast-coffee", "", ""},
			{"oat milk", "oat-milk", "", ""},
			{"VS Code as her editor", "vs-code", "", ""},
			{"a standing desk", "standing-desk", "", ""},
			{"remote work on Fridays", "remote-fridays", "", ""},
		}
		var regular, extra []fact
		for _, r := range rows {
			regular = append(regular, gen1(tmpl, r.field, r.object, r.from, r.to)...)
			extra = append(extra, gen1(tmplExtra, r.field, r.object, r.from, r.to)...)
		}
		out = append(out, familySpec{
			predicate:    "prefers",
			regular:      regular,
			extraHeldOut: extra,
			contradiction: [4]fact{
				mk("As of February 2026, Ava's IDE settings sync still shows VS Code as her preferred editor.", "vs-code", "2026-02", ""),
				mk("A February 2026 dotfiles commit from Ava still targets VS Code as her editor of choice.", "vs-code", "2026-02", ""),
				mk("Ava told a new hire in February 2026 that she now prefers JetBrains Fleet over anything else for editing code.", "jetbrains-fleet", "2026-02", ""),
				mk("Ava's February 2026 tool-survey response names JetBrains Fleet as her preferred editor.", "jetbrains-fleet", "2026-02", ""),
			},
			pairWhy: "Two sources claim Ava's current preferred editor for the same month, naming two different tools.",
		})
	}

	// committed_to -- contradiction: conflicting Postgres migration
	// cutover commitments made the same week.
	{
		tmpl := []string{
			"Ava committed to %s.",
			"In a planning meeting, Ava committed to %s.",
			"Ava's task tracker shows a commitment to %s.",
			"Ava told her manager she is committed to %s.",
		}
		tmplExtra := []string{
			"Ava's 1:1 notes record her commitment to %s.",
			"In a status update, Ava said she is committed to %s.",
			"Ava's OKR doc lists a commitment to %s.",
			"A follow-up email from Ava confirms she committed to %s.",
		}
		rows := []struct{ field, object, from, to string }{
			{"shipping the Q3 report by the end of the month", "ship-q3-report", "2026-07", ""},
			{"mentoring a new hire through onboarding", "mentor-new-hire", "2026-05", ""},
			{"reviewing a pending PR by Friday", "review-pr-by-friday", "2026-06", ""},
			{"paying off her credit card balance this quarter", "pay-off-credit-card", "2026-04", ""},
			{"running a half marathon in the fall", "run-half-marathon", "2026-03", ""},
		}
		var regular, extra []fact
		for _, r := range rows {
			regular = append(regular, gen1(tmpl, r.field, r.object, r.from, r.to)...)
			extra = append(extra, gen1(tmplExtra, r.field, r.object, r.from, r.to)...)
		}
		out = append(out, familySpec{
			predicate:    "committed_to",
			regular:      regular,
			extraHeldOut: extra,
			contradiction: [4]fact{
				mk("Ava committed in a March 2026 planning doc to cutting over the Postgres migration by the end of March.", "migrate-postgres-end-of-march", "2026-03", ""),
				mk("Ava's March 2026 sprint plan pins the Postgres migration cutover to end-of-month.", "migrate-postgres-end-of-march", "2026-03", ""),
				mk("In a Slack thread that same week, Ava committed to holding the Postgres migration cutover until April so QA has more time.", "migrate-postgres-delay-to-april", "2026-03", ""),
				mk("Ava's March 2026 message to the on-call channel commits to an April cutover for the Postgres migration instead.", "migrate-postgres-delay-to-april", "2026-03", ""),
			},
			pairWhy: "Ava commits to two different cutover timings for the same Postgres migration within the same week.",
		})
	}

	// deadline_on -- object is the due date itself. Contradiction: two
	// different dates given for the same performance-review deadline.
	{
		tmpl := []string{
			"Ava's deadline for %s is %s.",
			"A calendar reminder shows Ava's %s due %s.",
			"Ava's manager set a deadline for %s of %s.",
			"Ava noted in her planner that %s is due %s.",
		}
		tmplExtra := []string{
			"Ava's project tracker shows %s due %s.",
			"In a status email, Ava confirmed %s is due %s.",
			"Ava's manager's meeting notes list %s as due %s.",
			"A reminder in Ava's task app flags %s as due %s.",
		}
		rows := []struct{ thing, date, object, from, to string }{
			{"the Q3 report", "2026-07-31", "2026-07-31", "2026-07", ""},
			{"her tax filing", "2026-04-15", "2026-04-15", "2026-04", ""},
			{"her passport renewal", "2026-09-01", "2026-09-01", "2026-09", ""},
			{"the vendor contract renewal", "2026-05-20", "2026-05-20", "2026-05", ""},
			{"the security audit response", "2026-06-10", "2026-06-10", "2026-06", ""},
		}
		var regular, extra []fact
		for _, r := range rows {
			regular = append(regular, gen2(tmpl, r.thing, r.date, r.object, r.from, r.to)...)
			extra = append(extra, gen2(tmplExtra, r.thing, r.date, r.object, r.from, r.to)...)
		}
		out = append(out, familySpec{
			predicate:    "deadline_on",
			regular:      regular,
			extraHeldOut: extra,
			contradiction: [4]fact{
				mk("Ava's HR portal lists her performance review as due 2026-03-15.", "2026-03-15", "2026-03", ""),
				mk("The HR system's March 2026 task list still shows Ava's performance review due 2026-03-15.", "2026-03-15", "2026-03", ""),
				mk("Ava's manager verbally moved the performance review deadline to 2026-03-29 during a 1:1.", "2026-03-29", "2026-03", ""),
				mk("Ava's own calendar entry, updated after the 1:1, now shows the performance review due 2026-03-29.", "2026-03-29", "2026-03", ""),
			},
			pairWhy: "Two sources name different due dates for the same performance-review deadline in the same month -- only one can be the real deadline.",
		})
	}

	// relates_to -- contradiction: who is Ava's manager as of Feb 2026.
	{
		tmpl := []string{
			"Ava's %s is %s.",
			"In an email signature, Ava listed her %s as %s.",
			"Ava introduced her %s, %s, at the team offsite.",
			"Ava's emergency contact form names her %s as %s.",
		}
		tmplExtra := []string{
			"Ava's benefits form lists her %s as %s.",
			"In a team icebreaker, Ava mentioned her %s, %s.",
			"Ava's holiday card list includes her %s, %s.",
			"A hospital intake form names Ava's %s as %s.",
		}
		rows := []struct{ relation, name, object, from, to string }{
			{"spouse", "Dan Standardo", "dan-standardo", "2015-06", ""},
			{"sister", "Lily Chen", "lily-chen", "1994-03", ""},
			{"best friend", "Theo Kim", "theo-kim", "2010-09", ""},
			{"accountant", "Renee Ortiz", "renee-ortiz", "2019-01", ""},
			{"therapist", "Dr. Novak", "dr-novak", "2021-10", ""},
		}
		var regular, extra []fact
		for _, r := range rows {
			regular = append(regular, gen2(tmpl, r.relation, r.name, r.object, r.from, r.to)...)
			extra = append(extra, gen2(tmplExtra, r.relation, r.name, r.object, r.from, r.to)...)
		}
		out = append(out, familySpec{
			predicate:    "relates_to",
			regular:      regular,
			extraHeldOut: extra,
			contradiction: [4]fact{
				mk("As of February 2026, Ava's org chart still lists Priya Ram as her manager.", "priya-ram", "2026-02", ""),
				mk("Ava's February 2026 1:1 calendar series is still booked under Priya Ram as her manager.", "priya-ram", "2026-02", ""),
				mk("Ava told a colleague in February 2026 that Marcus Webb is now her manager.", "marcus-webb", "2026-02", ""),
				mk("Ava's February 2026 expense approvals started routing to Marcus Webb as her new manager.", "marcus-webb", "2026-02", ""),
			},
			pairWhy: "Two sources name a different person as Ava's manager for the same month -- she reports to only one manager at a time.",
		})
	}

	// belongs_to_project -- contradiction: still on the Beacon redesign
	// or fully reassigned to Project Anchorpoint.
	{
		tmpl := []string{
			"Ava belongs to %s.",
			"Ava's team roster lists her under %s.",
			"In a project kickoff doc, Ava is named a contributor to %s.",
			"Ava's calendar shows recurring standups for %s.",
		}
		tmplExtra := []string{
			"Ava's status report is filed under %s.",
			"A retro doc credits Ava as a contributor to %s.",
			"Ava's calendar invite series is titled after %s.",
			"In a resourcing spreadsheet, Ava is allocated to %s.",
		}
		rows := []struct{ field, object, from, to string }{
			{"Project Lighthouse", "project-lighthouse", "2024-02", "2024-11"},
			{"Project Meridian", "project-meridian", "2024-12", "2025-06"},
			{"the Aurora migration", "aurora-migration", "2025-07", "2025-12"},
			{"the Beacon redesign", "beacon-redesign", "2026-01", "2026-02"},
			{"Operation Tidewater", "operation-tidewater", "2026-01", ""},
		}
		var regular, extra []fact
		for _, r := range rows {
			regular = append(regular, gen1(tmpl, r.field, r.object, r.from, r.to)...)
			extra = append(extra, gen1(tmplExtra, r.field, r.object, r.from, r.to)...)
		}
		out = append(out, familySpec{
			predicate:    "belongs_to_project",
			regular:      regular,
			extraHeldOut: extra,
			contradiction: [4]fact{
				mk("As of February 2026, Ava is still listed as a core contributor to the Beacon redesign.", "beacon-redesign", "2026-02", ""),
				mk("The Beacon redesign's February 2026 contributor list still carries Ava's name.", "beacon-redesign", "2026-02", ""),
				mk("A February 2026 org update reassigned Ava fully to Project Anchorpoint, replacing her Beacon redesign work.", "project-anchorpoint", "2026-02", ""),
				mk("Ava's February 2026 calendar dropped the Beacon standup and added Project Anchorpoint's in its place.", "project-anchorpoint", "2026-02", ""),
			},
			pairWhy: "One source keeps Ava on the Beacon redesign in February 2026 while another has her fully reassigned off it to Project Anchorpoint the same month.",
		})
	}

	// said -- contradiction: what Ava said about the migration ship date.
	{
		tmpl := []string{
			"In the %s, Ava said %s.",
			"During the %s meeting, Ava said %s.",
			"Ava's email after the %s stated %s.",
			"Ava was quoted in the %s retro notes saying %s.",
		}
		tmplExtra := []string{
			"Meeting notes from the %s record Ava saying %s.",
			"A colleague's recap of the %s quotes Ava saying %s.",
			"Ava's follow-up Slack message after the %s says %s.",
			"The %s summary attributes to Ava the comment that %s.",
		}
		rows := []struct{ context, quote, object, from, to string }{
			{"Tuesday standup", "the deploy window should move to Thursday", "deploy-window-thursday", "2026-05", ""},
			{"sprint planning session", "she'd rather use feature flags than a hard cutover", "prefer-feature-flags", "2026-05", ""},
			{"on-call review", "the on-call rotation needs a fourth person", "on-call-needs-fourth", "2026-04", ""},
			{"vendor sync", "the vendor's SLA response times are unacceptable", "vendor-sla-unacceptable", "2026-03", ""},
			{"quarterly review", "the Q2 roadmap review went well", "q2-roadmap-review-went-well", "2026-06", ""},
		}
		var regular, extra []fact
		for _, r := range rows {
			regular = append(regular, gen2(tmpl, r.context, r.quote, r.object, r.from, r.to)...)
			extra = append(extra, gen2(tmplExtra, r.context, r.quote, r.object, r.from, r.to)...)
		}
		out = append(out, familySpec{
			predicate:    "said",
			regular:      regular,
			extraHeldOut: extra,
			contradiction: [4]fact{
				mk("In the March 2026 planning meeting, Ava said the Postgres migration would ship by end of March.", "migration-ships-end-of-march", "2026-03", ""),
				mk("Ava's March 2026 planning-doc summary quotes her saying the Postgres migration ships by end of March.", "migration-ships-end-of-march", "2026-03", ""),
				mk("In a March 2026 Slack message, Ava said the Postgres migration would slip to April.", "migration-ships-april", "2026-03", ""),
				mk("Ava's March 2026 status update to the on-call channel says the Postgres migration is slipping to April.", "migration-ships-april", "2026-03", ""),
			},
			pairWhy: "Ava is quoted giving two different ship dates for the same Postgres migration within the same month.",
		})
	}

	// costs (shard tier) -- contradiction: two different invoiced amounts
	// for the same Q3 offsite venue.
	{
		tmpl := []string{
			"%s costs %s.",
			"Ava's expense report lists %s at %s.",
			"The invoice for %s shows %s.",
			"Ava's monthly budget line for %s is %s.",
		}
		tmplExtra := []string{
			"Ava's receipt for %s shows %s.",
			"A finance dashboard entry for %s lists %s.",
			"Ava's reimbursement request for %s cites %s.",
			"The vendor confirmation email for %s states %s.",
		}
		rows := []struct{ item, amountText, object, from, to string }{
			{"Ava's AWS bill", "$312.00", "312.00-usd", "2026-04", ""},
			{"Ava's conference ticket for the Systems Summit", "$899.00", "899.00-usd", "2026-05", ""},
			{"Ava's therapy session copay", "$40.00", "40.00-usd", "2026-03", ""},
			{"Ava's gym membership", "$65.00", "65.00-usd", "2026-01", ""},
			{"Ava's domain renewal for avastandar.do", "$18.00", "18.00-usd", "2026-02", ""},
		}
		var regular, extra []fact
		for _, r := range rows {
			regular = append(regular, gen2(tmpl, r.item, r.amountText, r.object, r.from, r.to)...)
			extra = append(extra, gen2(tmplExtra, r.item, r.amountText, r.object, r.from, r.to)...)
		}
		out = append(out, familySpec{
			predicate:    "costs",
			regular:      regular,
			extraHeldOut: extra,
			contradiction: [4]fact{
				mk("The vendor's original invoice for the Q3 offsite venue lists a cost of $3,200.00.", "3200.00-usd", "2026-07", ""),
				mk("Ava forwarded the vendor's original Q3 offsite invoice showing $3,200.00.", "3200.00-usd", "2026-07", ""),
				mk("The finance team's reconciled ledger shows the Q3 offsite venue actually cost $4,750.00.", "4750.00-usd", "2026-07", ""),
				mk("Ava's expense reconciliation email cites the finance ledger's $4,750.00 figure for the Q3 offsite venue.", "4750.00-usd", "2026-07", ""),
			},
			pairWhy: "The vendor's original invoice and finance's reconciled ledger disagree on what the same Q3 offsite venue actually cost.",
		})
	}

	return out
}

// labelers cycles labeling provenance across the corpus the way T1.13's
// real dual-labeling process would: two independent model labelers plus a
// human reviewer.
var labelers = []string{"model-a", "model-b", "human-reviewer"}

// heldOutFactPositions are the within-"regular"-block indices (0-based,
// over the original fixed 20-entry order: 5 facts x 4 original phrasings)
// marked held-out by T1.14's original scheme -- one phrasing each from
// facts 1, 2, 3, and 5 of the regular set, leaving fact 4 entirely visible
// for tuning. Unchanged by T1.32: every T1.32 "extra" span (all 20 of
// them, at positions 20..39 in the per-family layout main() builds) is
// ALSO held out, additively -- see familySpec's doc comment.
var heldOutFactPositions = map[int]bool{1: true, 6: true, 11: true, 16: true}

type contradictionPairOut struct {
	ID     string `yaml:"id"`
	Family string `yaml:"family"`
	SpanA  string `yaml:"span_a"`
	SpanB  string `yaml:"span_b"`
	Why    string `yaml:"why"`
}

func main() {
	families := buildFamilies()

	if err := os.MkdirAll(labelsDir, 0o755); err != nil {
		log.Fatalf("gen_corpus: mkdir %s: %v", labelsDir, err)
	}
	// Clear previously generated label files so a regeneration after
	// removing a fact doesn't leave an orphaned file behind for
	// VerifyManifest to later flag as an unpinned addition.
	if err := clearYAML(labelsDir); err != nil {
		log.Fatalf("gen_corpus: clear %s: %v", labelsDir, err)
	}

	var heldOut []string
	var pairs []contradictionPairOut

	for _, fam := range families {
		if len(fam.regular) != 20 {
			log.Fatalf("gen_corpus: family %s has %d regular spans, want 20", fam.predicate, len(fam.regular))
		}
		if len(fam.extraHeldOut) != 20 {
			log.Fatalf("gen_corpus: family %s has %d extra held-out spans, want 20", fam.predicate, len(fam.extraHeldOut))
		}

		// all = 20 regular (positions 0-19) + 20 T1.32 extra (positions
		// 20-39, all held out) + 4 contradiction (positions 40-43, never
		// held out) = 44.
		all := append([]fact{}, fam.regular...)
		all = append(all, fam.extraHeldOut...)
		all = append(all, fam.contradiction[:]...)
		if len(all) != 44 {
			log.Fatalf("gen_corpus: family %s has %d spans, want 44 (20 regular + 20 T1.32 extra + 4 contradiction)", fam.predicate, len(all))
		}

		pairID := fmt.Sprintf("ava-%s-conflict", fam.predicate)
		for i, f := range all {
			r := record{}
			r.Span = f.span
			r.Expected.Predicate = fam.predicate
			r.Expected.Object = f.object
			r.Expected.ValidFrom = f.validFrom
			r.Expected.ValidTo = f.validTo
			r.Labeler = labelers[i%len(labelers)]
			r.Adjudicated = i%len(labelers) == len(labelers)-1

			switch {
			case i == 40 || i == 41:
				r.ContradictionPairID = pairID
				r.ContradictionRole = "a"
				r.Adjudicated = true
			case i == 42 || i == 43:
				r.ContradictionPairID = pairID
				r.ContradictionRole = "b"
				r.Adjudicated = true
			}

			// Held out: T1.14's original 4 positions within the regular
			// block (i < 20), plus T1.32's entire extra block (20 <= i <
			// 40) -- purely additive over the original scheme.
			if (i < 20 && heldOutFactPositions[i]) || (i >= 20 && i < 40) {
				heldOut = append(heldOut, f.span)
			}

			name := fmt.Sprintf("ava-%s-%02d.yaml", fam.predicate, i+1)
			writeRecord(filepath.Join(labelsDir, name), r)
		}

		pairs = append(pairs, contradictionPairOut{
			ID:     pairID,
			Family: fam.predicate,
			SpanA:  fam.contradiction[0].span,
			SpanB:  fam.contradiction[2].span,
			Why:    fam.pairWhy,
		})
	}

	sort.Strings(heldOut)
	writeYAML(splitPath, struct {
		HeldOut []string `yaml:"held_out"`
	}{HeldOut: heldOut})

	sort.Slice(pairs, func(i, j int) bool { return pairs[i].ID < pairs[j].ID })
	writeYAML(contraPath, struct {
		Pairs []contradictionPairOut `yaml:"pairs"`
	}{Pairs: pairs})

	if err := eval.WriteManifest(labelsDir, manifestPath); err != nil {
		log.Fatalf("gen_corpus: WriteManifest: %v", err)
	}

	fmt.Printf("gen_corpus: wrote %d families x 44 spans, %d held-out, %d contradiction pairs\n", len(families), len(heldOut), len(pairs))
}

func clearYAML(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func writeRecord(path string, r record) {
	writeYAML(path, r)
}

func writeYAML(path string, v any) {
	b, err := yaml.Marshal(v)
	if err != nil {
		log.Fatalf("gen_corpus: marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Fatalf("gen_corpus: write %s: %v", path, err)
	}
}
