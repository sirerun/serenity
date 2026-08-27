# Serenity, and what it owes gbrain

Podcast script. Two voices, about 14 minutes at a conversational pace (~2,100 words).
HOST is the interviewer. MAINTAINER is the person building Serenity.
Working title caveat: "Serenity" collides with SerenityOS; the name is decided before launch.

---

**COLD OPEN**

MAINTAINER: Here's the moment that made me build this. An agent I trusted booked a vendor call for me. Perfectly competent. Except six weeks earlier I had decided, in writing, that we would not add vendors that quarter. The decision existed. Nothing checked the plan against it.

HOST: So the memory was there and the judgment was there, and neither one stopped the action.

MAINTAINER: Right. The memory could tell you what I'd said. It couldn't tell you what I'd decided, and it definitely couldn't stop anything.

HOST: That's the show today. Serenity, a personal memory system for people who run agents, and gbrain, the project it descends from. Let's start with the lineage, because you're unusually explicit about it.

---

**SEGMENT 1: WHAT GBRAIN GOT RIGHT**

MAINTAINER: gbrain is Garry Tan's open-source agent brain. It's TypeScript on Bun, and it made one decision that I think is the most important design choice in this whole category: your knowledge lives in a git repository of markdown files, and the database is a cache you can throw away.

HOST: Say more about why that matters, because "markdown in git" sounds almost too simple.

MAINTAINER: It sounds simple until you list what it buys you. Disaster recovery is `git clone` and rebuild. Syncing between machines is `git push` and `git pull`. Your audit history is `git log`. Privacy granularity is a gitignore. And when the machine gets a fact wrong, you open the file and fix it. No admin panel, no support ticket.

HOST: And gbrain enforces that, it's not just a convention?

MAINTAINER: It's enforced. There's an invariant test that wipes the derived tables and proves the rebuild is byte-identical, and a CI gate against any code path that writes knowledge to the database without writing the file first. I kept both of those verbatim. Not the idea, the actual test discipline.

HOST: What else did you keep?

MAINTAINER: Three things. First, the wire protocol. gbrain froze something called MEMORY_VERBS version one: five verbs, `recall`, `remember`, `entity`, `synthesize`, `forget`, with a self-describing envelope and a conformance suite you can run against any server. Serenity is a conformant MEMORY_VERBS server. If you have agents talking to gbrain today, they keep working.

HOST: So it's not a fork.

MAINTAINER: Not a fork, not a rewrite, and not a competitor to its thesis. It's a separate codebase in Go that honors the same contracts. Second thing I kept: the retrieval architecture. Hybrid lexical plus vector search fused with reciprocal rank fusion, multi-query expansion, a four-layer dedup, typed graph edges extracted without a model, and synthesis that returns a cited answer plus a statement of what the brain doesn't know. I ported that pattern for pattern. gbrain proved it; I had no reason to respec it.

HOST: And third?

MAINTAINER: The fence format. On a gbrain entity page, facts and takes live inside HTML-comment fences as append-only markdown tables, with strikethrough for things that were superseded or forgotten. That's a genuinely good encoding: humans can read it, git can diff it, and nothing ever silently disappears. Serenity's claims fence is a direct descendant.

---

**SEGMENT 2: WHERE THE SUCCESSOR BET STARTS**

HOST: So if you kept the substrate, the protocol, the retrieval, and the file format, what's actually new?

MAINTAINER: The unit of memory. gbrain's atomic unit is the page: compiled truth about an entity, plus a timeline, plus those fact and take fences. Serenity's atomic unit is the claim. Subject, predicate, object, with provenance, a confidence, a validity window, and a supersession chain.

HOST: Give me a concrete one.

MAINTAINER: "Alice Tan works at Acme." Confidence 0.92, valid from June 2025, observed in source e42 at span 3, state active. And below it, struck through, "Alice Tan works at Initech," valid 2023 to June 2025, superseded by the row above.

HOST: gbrain has facts with confidence and validity dates too. What's the difference?

MAINTAINER: Two things. One, in gbrain, staleness is handled at ranking time: recency decay and source-tier boosts when you search. In Serenity, confidence decays in the data, with a half-life per predicate family. A bank balance has a one-day half-life. A job title, ninety days. A preference, a year. The staleness is a property of the claim, not the query.

HOST: And two?

MAINTAINER: Contradictions. gbrain measures them: it has a contradiction probe with a temporal verdict enum, and the operator applies the fixes. Serenity detects them structurally, on subject and predicate, and routes every conflict through a human decision. That leads to the part I care about most.

HOST: Which is not memory.

MAINTAINER: Which is not memory. gbrain's own v0 design document has a section it deferred, that it called the "intelligence compiler": treat every fact as a first-class claim with source span, entity links, validity window, confidence, and contradiction status. Serenity is me building the thing gbrain named and set aside. But the wedge, the reason to switch, is one layer above that.

---

**SEGMENT 3: PRECEPTS, AND THE DIRECTION PROTOCOL**

HOST: Let's go back to your cold open. The vendor call.

MAINTAINER: Serenity has four epistemic layers, and each one has different authority. A Source is raw material: an email, a file, a transcript. Immutable. An Observation is what a model extracted from one source span: not yet believed. A Claim is what reconciliation accepted. And a Precept is a record of human judgment: a decision, a constraint, an intent, or an open question.

HOST: And precepts are different in kind.

MAINTAINER: They're immutable. You never edit one, you only supersede it. They carry the alternatives you rejected and why, and the condition under which you'd revisit. And here's the security invariant: no ingest path, no extraction, no model call can create or alter a precept. Only a human, through an explicit disposition. We attack that in the test suite with a corpus of malicious emails that try to fabricate one.

HOST: So the precept store is where your "no new vendors" decision would have lived.

MAINTAINER: Yes, and precepts aren't a format I invented. They're dira entries. dira is a git-native ledger of decisions with a schema, a CLI, and a plan checker of its own. Serenity's precept store *is* a dira ledger, vendored at a pinned commit, and the dira command line reads it unmodified.

HOST: And the checking?

MAINTAINER: That's the second new protocol, DIRECTION. An agent runtime calls `brief` at the start of a session and gets an attention-budgeted packet: standing precepts, current intents, relevant entities with their claim heads, and open blocking questions. Then before it executes anything, it calls `check_plan`.

HOST: What does `check_plan` return?

MAINTAINER: A schema verdict, per applicable constraint. Pass, or violated, with the precept id, the stored reasoning, and the revisit condition, verbatim. The agent gets an explanation it can act on, not a flag. And there's a detail I'm stubborn about: a plan that matches zero constraints returns `no_applicable_constraints`, never a bare pass, so the caller can tell "checked and clean" from "nothing was checked."

HOST: How does matching work? This sounds like the place where a model quietly decides everything.

MAINTAINER: Two stages, and the first has no model at all. Constraints apply to a small closed set of actions: start a project, deploy to production, spend over an amount, contact a new party, schedule outside hours. If the agent sends structured actions, matching is deterministic and works offline. If it only sends free text, a cheap model classifies the text into that action set first, and the classification rides along in the verdict so you can audit it. With no model available, free text comes back `unverified`. Explicit. Never a silent pass.

HOST: And the vendor call?

MAINTAINER: `contact_new_party`. Violated. Precept dec-0031: no new vendors this quarter. Why not: budget freeze. Revisit if: the Series B closes. The agent stops and asks.

---

**SEGMENT 4: DISPOSITION, AND EARNING AUTOMATION**

HOST: You keep saying "human decision." A lot of these systems promise the opposite: zero clerical work. Are you asking people to review a queue?

MAINTAINER: I am, and I'm being honest about the cost. The third protocol is DISPOSITION. Every consequential change lands in a queue: a contradiction between two claims, shown as A versus B with provenance; a draft precept; a request to do something with side effects; a captured ramble that needs filing. You accept, edit-and-accept, reject with a note, or defer.

HOST: That's the part people abandon.

MAINTAINER: Which is why the queue has structural hygiene. Grouped items, so one decision clears twenty similar ones. Bulk defer. An escape hatch so the queue can never hold you hostage. Expiry is auto-defer, never auto-decline, and after three deferrals an item is parked, not resurfaced forever. And there are service levels: if the median item is older than three days, your morning briefing tells you.

HOST: But it's still all manual.

MAINTAINER: At install, yes. That's gbrain's invariant too: auto-supersession never applies. Serenity's twist is that automation is earned, per connector and per predicate family, and it's measured on your dispositions. A cell like "email, works_at" can unlock automatic resolution once enough of your decisions have gone one way, spread over enough days and enough sources that one bad connector day can't promote it. Every automatic action is still logged as a disposition, sampled back into your queue, and one reversal by you demotes the cell to zero.

HOST: So the thresholds aren't constants.

MAINTAINER: They're a policy object, and the shipped defaults come out of a calibration milestone, with the sweep data published. The numbers in the design document are priors. The evidence replaces them before launch.

---

**SEGMENT 5: SWITCHING, AND WHAT'S NOT THERE**

HOST: If someone runs gbrain today, what does switching look like?

MAINTAINER: One command: `serenity import --from-gbrain`. It reads the repo directly: pages become entity pages, every row in a facts or takes fence becomes exactly one claim with its validity window, visibility, and provenance preserved, and the struck-through rows become superseded or retracted claims with the pointers intact. There's a field-level round-trip test on a public fixture brain, so "lossless" is a test, not an adjective.

HOST: Fully lossless?

MAINTAINER: Representation-level, yes. Some things are translations by nature. gbrain's take weights and source tiers become initial confidences; its fact kinds map onto Serenity's controlled predicates. Every claim that came through one of those mappings is flagged for review rather than presented as settled. I'd rather name the asymmetry than hide it.

HOST: And what's deliberately missing?

MAINTAINER: No hosted service, nothing phones home, egress is only the model keys you configure. Single user. No autonomous side effects: drafts yes, effects only through the queue. No GUI at launch: it's a daemon and a CLI, and the CLI is the first conformant client of all three protocols, so the daemon has no privileged back door. A desktop app comes later, and it'll talk to the daemon over the same public wire as everyone else.

HOST: Why open source, and why open protocols specifically?

MAINTAINER: Because the protocols are the product. Any approval client can implement DISPOSITION. Any agent runtime can implement DIRECTION. Changes go through a public RFC process in the repo, additive forever, with conformance fixtures anyone can run. And there's a kill criterion written into the design: if real users don't keep exercising plan-check and conflict review, the protocol surface stops growing until they do.

---

**CLOSE**

HOST: Give me the one-sentence version.

MAINTAINER: gbrain proved that your memory belongs in your repo, compiled by agents and queried over a frozen protocol. Serenity keeps all of that and adds the thing it measured but didn't model: belief. Which claims are held true, at what confidence, superseded by what. And on top of belief, judgment: what you decided, why, and an agent that gets stopped before it violates it.

HOST: Where do people find it?

MAINTAINER: The design document is RFC 0001 in the repository, and it credits gbrain in the first paragraph. The name is a working title, so watch the repo rather than the word. And if you want to try the wedge first: run `serenity check` against a plan and a precept, and see it refuse.

HOST: Thanks for coming on.

MAINTAINER: Thanks for having me.

---

**PRODUCTION NOTES**

- Runtime estimate: 13 to 15 minutes at 150 words per minute.
- Every factual claim about gbrain traces to its repository at commit d35c9c9e441e: the file-first invariant and CI gate, MEMORY_VERBS v1 and its conformance suite, the facts and takes fences (`src/core/facts-fence.ts`, `takes-fence.ts`), the contradiction probe and temporal verdicts, the retrieval stack, the deferred "intelligence compiler" section of its v0 document.
- Every claim about Serenity traces to RFC 0001 v2.2 (`docs/rfc/0001-serenity.md`). Features described in the future tense are in the plan (`docs/plan.md`), not shipped; at recording time only the M0 substrate exists.
- If the name changes before recording, replace "Serenity" throughout and drop the working-title line in the close.
