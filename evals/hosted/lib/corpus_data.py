"""T23.43 frozen corpus content: literal, reviewable case/fact data.

This module is the single source of truth a human reviewer reads to freeze
T23.43's corpus (docs/launch/hosted-completion/embedding-eval.md records the
resulting corpus_hash). corpus_gen.py turns build_corpus() into
evals/hosted/corpus.json and evals/hosted/facts.json; nothing here calls a
network or a model -- every fact/query pair is authored text.

Composition (T23.43.md step 1, exact required counts):
  40 paraphrases       (>=20 with zero content-word overlap vs their fact)
  20 names/entities
  20 preferences/negation/near-duplicates
  10 multilingual positive cases
   5 temporal positive cases
   5 expected-empty (forgotten/expired) cases
 100 total; 95 positive cases each have exactly one expected_fact_id.

No customer data: every subject name, company, and scenario below is
invented for this corpus.
"""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass(frozen=True)
class Fact:
    id: str
    type: str
    text: str


@dataclass(frozen=True)
class Case:
    id: str
    category: str
    query: str
    expected_fact_id: str | None
    distractor_fact_ids: tuple[str, ...]
    subject_tokens: tuple[str, ...] = ()  # excluded from content-overlap scoring
    language: str = "en"
    isolated_filler_fact_id: str | None = None  # empty-case brains only
    notes: str = ""


# ---------------------------------------------------------------------------
# Filler pool: generic facts used as ambient distractor noise. Never any
# case's expected target.
# ---------------------------------------------------------------------------

FILLERS: list[Fact] = [
    Fact("filler-01", "fact", "Nadia keeps her houseplants alive with a strict watering spreadsheet."),
    Fact("filler-02", "fact", "Otto collects vintage subway maps from cities he has never visited."),
    Fact("filler-03", "fact", "Petra bakes sourdough bread every other weekend."),
    Fact("filler-04", "fact", "Quang repairs broken umbrellas as a hobby."),
    Fact("filler-05", "fact", "Rina practices calligraphy with a fountain pen each evening."),
    Fact("filler-06", "fact", "Salim keeps a running list of palindromes he finds in the wild."),
    Fact("filler-07", "fact", "Tea Okonkwo knits scarves for the animal shelter's fundraiser."),
    Fact("filler-08", "fact", "Ulf photographs cloud formations from his rooftop."),
    Fact("filler-09", "fact", "Vera memorizes a new poem every month."),
    Fact("filler-10", "fact", "Wale restores old bicycles for neighborhood kids."),
]

# ---------------------------------------------------------------------------
# Paraphrase category: 13 topic triads (39 cases) + 1 standalone (40 total).
# Each triad shares one topic template across 3 subjects; the other two
# triad members are each case's scored distractors -- same topic, different
# subject and value, so a model that only captures topic (not the correct
# subject binding) ranks the wrong member just as high.
# ---------------------------------------------------------------------------

_PARAPHRASE_TRIADS = [
    # (topic key, hard: bool, query_template, [(subject, fact_template_fill), ...])
    (
        "commute", False, "How does {name} get to the office?",
        [
            ("Amara", "{name} commutes to the office by bicycle every Monday."),
            ("Baptiste", "{name} commutes to the office by train every Tuesday."),
            ("Chidi", "{name} commutes to the office by carpool every Wednesday."),
        ],
    ),
    (
        "pet", True, "Which creature shares {name}'s home?",
        [
            ("Delphine", "{name} keeps a cat named Mimir as a pet."),
            ("Esteban", "{name} keeps a dog named Bruno as a pet."),
            ("Fenna", "{name} keeps a parrot named Kiwi as a pet."),
        ],
    ),
    (
        "hobby", False, "What does {name} do on weekends?",
        [
            ("Grzegorz", "{name} spends weekends restoring vintage typewriters."),
            ("Haruto", "{name} spends weekends brewing small-batch soy sauce."),
            ("Ingrid", "{name} spends weekends mapping abandoned rail lines."),
        ],
    ),
    (
        "morning-drink", True, "What is {name}'s first beverage of the day?",
        [
            ("Jokull", "{name} starts every morning with a cup of chicory coffee."),
            ("Kwame", "{name} starts every morning with a cup of ginger tea."),
            ("Liora", "{name} starts every morning with a cup of yerba mate."),
        ],
    ),
    (
        "instrument", True, "How does {name} unwind musically at night?",
        [
            ("Mateus", "{name} practices the cello for an hour each evening."),
            ("Noor", "{name} practices the marimba for an hour each evening."),
            ("Oksana", "{name} practices the accordion for an hour each evening."),
        ],
    ),
    (
        "reading", True, "What has {name} been absorbed in lately, story-wise?",
        [
            ("Priya", "{name} is currently reading a novel about arctic explorers."),
            ("Quentin", "{name} is currently reading a novel about deep-sea cartography."),
            ("Rosalind", "{name} is currently reading a novel about forgotten board games."),
        ],
    ),
    (
        "garden", True, "What is thriving outside {name}'s window this season?",
        [
            ("Sten", "{name} grows chili peppers on the balcony."),
            ("Tanvir", "{name} grows dwarf citrus trees on the balcony."),
            ("Urszula", "{name} grows heirloom tomatoes on the balcony."),
        ],
    ),
    (
        "relocation", False, "What city does {name} live in now?",
        [
            ("Viraj", "{name} relocated to a new city, Cebu, for a role last spring."),
            ("Wren", "{name} relocated to a new city, Ljubljana, for a role last spring."),
            ("Xochitl", "{name} relocated to a new city, Accra, for a role last spring."),
        ],
    ),
    (
        "cooking", False, "What does {name} cook every Sunday?",
        [
            ("Yusuf", "{name} makes a signature lamb tagine every Sunday for friends."),
            ("Zanele", "{name} makes a signature mushroom risotto every Sunday for friends."),
            ("Aoife", "{name} makes a signature potato farl every Sunday for friends."),
        ],
    ),
    (
        "language-learning", True, "What new skill is {name} quietly building on their own?",
        [
            ("Bilal", "{name} is teaching themself Icelandic using flashcards."),
            ("Cosima", "{name} is teaching themself Tagalog using flashcards."),
            ("Dov", "{name} is teaching themself Xhosa using flashcards."),
        ],
    ),
    (
        "volunteer", False, "What kind of shelter does {name} help out at?",
        [
            ("Fatoumata", "{name} volunteers weekly at a local animal shelter."),
            ("Giedre", "{name} volunteers weekly at a local homeless shelter."),
            ("Hamid", "{name} volunteers weekly at a local wildlife shelter."),
        ],
    ),
    (
        "exercise", True, "What does {name} do for exercise before starting the workday?",
        [
            ("Rustam", "{name} goes for a run along the river every morning before work."),
            ("Sunniva", "{name} goes for a swim at the lake every morning before work."),
            ("Tobias", "{name} goes for a row on the canal every morning before work."),
        ],
    ),
    (
        "radio", False, "What kind of radio programs does {name} enjoy?",
        [
            ("Vasilis", "{name} always listens to old radio dramas while cleaning the kitchen."),
            ("Winnie", "{name} always listens to radio call-in quizzes while cleaning the kitchen."),
            ("Xander", "{name} always listens to overnight radio jazz while cleaning the kitchen."),
        ],
    ),
]

_PARAPHRASE_STANDALONE = Case(
    id="para-40",
    category="paraphrase",
    query="Which section of the newspaper does Yolanda start with?",
    expected_fact_id="para-40-fact",
    distractor_fact_ids=("filler-01", "filler-02"),
    subject_tokens=("yolanda",),
)
_PARAPHRASE_STANDALONE_FACT = Fact(
    "para-40-fact", "fact",
    "Yolanda reads yesterday's newspaper backwards, starting from the sports page.",
)


def _build_paraphrase() -> tuple[list[Case], list[Fact]]:
    cases: list[Case] = []
    facts: list[Fact] = []
    for topic_idx, (topic, hard, query_tmpl, entries) in enumerate(_PARAPHRASE_TRIADS, start=1):
        member_ids = [f"para-{topic_idx:02d}{letter}" for letter in "abc"]
        for slot, (subject, fact_tmpl) in enumerate(entries):
            fid = f"{member_ids[slot]}-fact"
            facts.append(Fact(fid, "fact", fact_tmpl.format(name=subject)))
        for slot, (subject, _fact_tmpl) in enumerate(entries):
            others = [f"{mid}-fact" for i, mid in enumerate(member_ids) if i != slot]
            cases.append(
                Case(
                    id=member_ids[slot],
                    category="paraphrase",
                    query=query_tmpl.format(name=subject),
                    expected_fact_id=f"{member_ids[slot]}-fact",
                    distractor_fact_ids=tuple(others),
                    subject_tokens=(subject.lower(),),
                    notes=f"topic={topic} hard={hard}",
                )
            )
    facts.append(_PARAPHRASE_STANDALONE_FACT)
    cases.append(_PARAPHRASE_STANDALONE)
    assert len(cases) == 40, f"expected 40 paraphrase cases, got {len(cases)}"
    return cases, facts


# ---------------------------------------------------------------------------
# Name/entity category: 20 cases, confusable-name triads/pairs.
# ---------------------------------------------------------------------------

_NAME_TRIADS = [
    (
        "Who leads operations in {slot}?",
        [
            ("Warsaw", "Katarzyna Wroblewski leads the Warsaw distribution hub."),
            ("Krakow", "Katarzyna Wozniak leads the Krakow distribution hub."),
            ("Gdansk", "Katarzyna Wysocka leads the Gdansk distribution hub."),
        ],
    ),
    (
        "Which Meridian Freight branch manages {slot} shipping routes?",
        [
            ("Arctic", "Meridian Freight Norway handles Arctic shipping routes."),
            ("Pacific coastal", "Meridian Freight Chile handles Pacific coastal shipping routes."),
            ("East African", "Meridian Freight Kenya handles East African shipping routes."),
        ],
    ),
    (
        "Who handles {slot} law matters in Lagos?",
        [
            ("tax", "Consultant Femi Adeyemi specializes in tax law for the Lagos office."),
            ("labor", "Consultant Ronke Adeyemi specializes in labor law for the Lagos office."),
            ("contract", "Consultant Tunde Adeyemi specializes in contract law for the Lagos office."),
        ],
    ),
    (
        "Which drone model offers a {slot}-minute flight time?",
        [
            ("40", "The AeroLite 200 drone has a 40-minute flight time."),
            ("25", "The AeroLite 400 drone has a 25-minute flight time."),
            ("15", "The AeroLite Mini drone has a 15-minute flight time."),
        ],
    ),
    (
        "Who is keynoting about {slot} restoration?",
        [
            ("coral reef", "Dr. Beatriz Souza is keynoting on coral reef restoration at the marine summit."),
            ("mangrove", "Dr. Camila Souza is keynoting on mangrove restoration at the marine summit."),
            ("seagrass", "Dr. Diana Souza is keynoting on seagrass restoration at the marine summit."),
        ],
    ),
    (
        "Who owns the {slot} on Cedar Street?",
        [
            ("bakery", "Youssef Haddad owns the bakery on Cedar Street."),
            ("bookstore", "Marwan Haddad owns the bookstore on Cedar Street."),
            ("pharmacy", "Samira Haddad owns the pharmacy on Cedar Street."),
        ],
    ),
]

_NAME_EXTRA = [
    Case(
        id="name-07a",
        category="name_entity",
        query="Who backed the solar-panel recycling startup's seed round?",
        expected_fact_id="name-07a-fact",
        distractor_fact_ids=("name-07b-fact", "filler-03"),
        subject_tokens=("lin", "wei-chen"),
    ),
    Case(
        id="name-07b",
        category="name_entity",
        query="Who backed the rival solar-panel recycling startup's seed round?",
        expected_fact_id="name-07b-fact",
        distractor_fact_ids=("name-07a-fact", "filler-04"),
        subject_tokens=("lin", "mei-hua"),
    ),
]
_NAME_EXTRA_FACTS = [
    Fact("name-07a-fact", "fact", "Investor Lin Wei-Chen backed the seed round for a solar-panel recycling startup."),
    Fact("name-07b-fact", "fact", "Investor Lin Mei-Hua backed the seed round for a rival solar-panel recycling startup."),
]


def _build_name_entity() -> tuple[list[Case], list[Fact]]:
    cases: list[Case] = []
    facts: list[Fact] = []
    for topic_idx, (query_tmpl, entries) in enumerate(_NAME_TRIADS, start=1):
        member_ids = [f"name-{topic_idx:02d}{letter}" for letter in "abc"]
        for slot, (slot_word, fact_text) in enumerate(entries):
            facts.append(Fact(f"{member_ids[slot]}-fact", "fact", fact_text))
        for slot, (slot_word, fact_text) in enumerate(entries):
            others = [f"{mid}-fact" for i, mid in enumerate(member_ids) if i != slot]
            subject_name = fact_text.split(" is keynoting")[0].split(" leads")[0].split(" handles")[0].split(
                " specializes"
            )[0].split(" owns")[0].split(" has ")[0]
            cases.append(
                Case(
                    id=member_ids[slot],
                    category="name_entity",
                    query=query_tmpl.format(slot=slot_word),
                    expected_fact_id=f"{member_ids[slot]}-fact",
                    distractor_fact_ids=tuple(others),
                    subject_tokens=tuple(w.lower() for w in subject_name.split()),
                )
            )
    cases.extend(_NAME_EXTRA)
    facts.extend(_NAME_EXTRA_FACTS)
    assert len(cases) == 20, f"expected 20 name/entity cases, got {len(cases)}"
    return cases, facts


# ---------------------------------------------------------------------------
# Preference/negation/near-duplicate category: 20 cases.
# ---------------------------------------------------------------------------

_PREF_TRIADS = [
    (
        "Who prefers coffee to tea?",
        [
            ("Bram", "Bram prefers coffee over tea in the mornings."),
            ("Corin", "Corin prefers tea over coffee in the mornings."),
            ("Dara", "Dara avoids both coffee and tea, drinking only water."),
        ],
    ),
    (
        "Who prefers meetings in the morning?",
        [
            ("Elin", "Elin prefers morning meetings and avoids anything after 4pm."),
            ("Farrukh", "Farrukh prefers afternoon meetings and avoids anything before 10am."),
            ("Gunnar", "Gunnar has no meeting-time preference and accepts any slot."),
        ],
    ),
    (
        "Who avoids shellfish specifically?",
        [
            ("Halima", "Halima does not eat shellfish but eats other seafood."),
            ("Imre", "Imre does not eat any seafood at all, shellfish included."),
            ("Jovan", "Jovan eats shellfish but avoids other seafood."),
        ],
    ),
    (
        "Who always chooses a window seat?",
        [
            ("Kaveh", "Kaveh prefers window seats and never books an aisle seat."),
            ("Leilani", "Leilani prefers aisle seats and never books a window seat."),
            ("Milos", "Milos has no seat preference and takes whatever is available."),
        ],
    ),
    (
        "Who prefers email to phone calls?",
        [
            ("Nour", "Nour prefers email over phone calls for work matters."),
            ("Omar", "Omar prefers phone calls over email for work matters."),
            ("Paloma", "Paloma refuses phone calls entirely, email only, no exceptions."),
        ],
    ),
    (
        "Who currently trains at dawn?",
        [
            ("Reza", "Reza used to prefer the gym in the evening but now trains at dawn."),
            ("Soraya", "Reza's colleague Soraya still trains in the evening, unchanged."),
            ("Tamar", "Tamar trains at dawn on weekdays and in the evening on weekends."),
        ],
    ),
]

_PREF_EXTRA = [
    Case(
        id="pref-07a",
        category="preference",
        query="Who adds chili to almost every meal?",
        expected_fact_id="pref-07a-fact",
        distractor_fact_ids=("pref-07b-fact", "filler-05"),
        subject_tokens=("ugne",),
    ),
]
_PREF_EXTRA_B_CASE = Case(
    id="pref-07b",
    category="preference",
    query="Who avoids chili entirely?",
    expected_fact_id="pref-07b-fact",
    distractor_fact_ids=("pref-07a-fact", "filler-06"),
    subject_tokens=("vikram",),
)
_PREF_EXTRA_FACTS = [
    Fact("pref-07a-fact", "fact", "Ugne likes spicy food and adds chili to almost everything."),
    Fact("pref-07b-fact", "fact", "Vikram dislikes spicy food and avoids chili entirely."),
]


def _build_preference() -> tuple[list[Case], list[Fact]]:
    cases: list[Case] = []
    facts: list[Fact] = []
    for topic_idx, (query, entries) in enumerate(_PREF_TRIADS, start=1):
        member_ids = [f"pref-{topic_idx:02d}{letter}" for letter in "abc"]
        for slot, (subject, fact_text) in enumerate(entries):
            facts.append(Fact(f"{member_ids[slot]}-fact", "fact", fact_text))
        target_slot = 0  # the query targets the triad's first (affirmative) member
        others = [f"{mid}-fact" for i, mid in enumerate(member_ids) if i != target_slot]
        subject_name = entries[target_slot][0]
        cases.append(
            Case(
                id=member_ids[target_slot],
                category="preference",
                query=query,
                expected_fact_id=f"{member_ids[target_slot]}-fact",
                distractor_fact_ids=tuple(others),
                subject_tokens=(subject_name.lower(),),
            )
        )
    # Each triad above only yields 1 case (6 cases); add the other two
    # members as explicit standalone cases so every triad contributes to
    # the 20-count and every triad member is exercised at least once
    # across the corpus's positive cases.
    for topic_idx, (_query, entries) in enumerate(_PREF_TRIADS, start=1):
        member_ids = [f"pref-{topic_idx:02d}{letter}" for letter in "abc"]
        for slot in (1, 2):
            subject, fact_text = entries[slot]
            others = [f"{mid}-fact" for i, mid in enumerate(member_ids) if i != slot]
            cases.append(
                Case(
                    id=f"{member_ids[slot]}-q",
                    category="preference",
                    query=_paraphrase_pref_query(fact_text, subject),
                    expected_fact_id=f"{member_ids[slot]}-fact",
                    distractor_fact_ids=tuple(others),
                    subject_tokens=(subject.lower(),),
                )
            )
    cases.extend(_PREF_EXTRA)
    cases.append(_PREF_EXTRA_B_CASE)
    facts.extend(_PREF_EXTRA_FACTS)
    assert len(cases) == 20, f"expected 20 preference cases, got {len(cases)}"
    return cases, facts


def _paraphrase_pref_query(fact_text: str, subject: str) -> str:
    """A short direct query about subject's own stated preference -- used
    for the 2nd/3rd triad members so every authored preference fact is
    exercised by some case, not only the triad's first member."""
    return f"What is {subject}'s stated preference in this area?"


# ---------------------------------------------------------------------------
# Multilingual category: 10 cases, English facts queried in another
# language (cross-lingual retrieval).
# ---------------------------------------------------------------------------

_MULTILINGUAL = [
    ("ml-01", "es", "¿Cuál es el destino de vacaciones favorito de Amina?",
     "Amina's favorite holiday destination is the Azores.", ("amina",)),
    ("ml-02", "fr", "Quel genre de chien Bjorn a-t-il adopté ?",
     "Bjorn adopted a rescue greyhound named Sable.", ("bjorn",)),
    ("ml-03", "de", "Was unterrichtet Chiara donnerstagabends?",
     "Chiara teaches pottery classes on Thursday evenings.", ("chiara",)),
    ("ml-04", "pt", "O que Dawit conserta como negócio paralelo?",
     "Dawit repairs antique clocks as a side business.", ("dawit",)),
    ("ml-05", "sw", "Elif hujitolea kufanya nini kwa familia za wakimbizi?",
     "Elif volunteers translating documents for refugee families.", ("elif",)),
    ("ml-06", "it", "Cosa alleva Faisal sul tetto del suo appartamento?",
     "Faisal keeps bees on his apartment rooftop.", ("faisal",)),
    ("ml-07", "nl", "Wat voor studio runt Greta?",
     "Greta runs a small letterpress printing studio.", ("greta",)),
    ("ml-08", "pl", "Czym zajmuje się Hiroshi w weekendy?",
     "Hiroshi grows bonsai trees as a weekend practice.", ("hiroshi",)),
    ("ml-09", "tr", "Ines müze için neyi restore ediyor?",
     "Ines restores old sailing maps for a maritime museum.", ("ines",)),
    ("ml-10", "id", "Jamal membuat musik apa dengan nama panggung?",
     "Jamal composes ambient music under a stage name.", ("jamal",)),
]


def _build_multilingual() -> tuple[list[Case], list[Fact]]:
    cases: list[Case] = []
    facts: list[Fact] = []
    n = len(_MULTILINGUAL)
    for i, (cid, lang, query, fact_text, subject_tokens) in enumerate(_MULTILINGUAL):
        fid = f"{cid}-fact"
        facts.append(Fact(fid, "fact", fact_text))
    for i, (cid, lang, query, fact_text, subject_tokens) in enumerate(_MULTILINGUAL):
        prev_fid = f"{_MULTILINGUAL[(i - 1) % n][0]}-fact"
        next_fid = f"{_MULTILINGUAL[(i + 1) % n][0]}-fact"
        cases.append(
            Case(
                id=cid,
                category="multilingual",
                query=query,
                expected_fact_id=f"{cid}-fact",
                distractor_fact_ids=(prev_fid, next_fid),
                subject_tokens=subject_tokens,
                language=lang,
            )
        )
    assert len(cases) == 10, f"expected 10 multilingual cases, got {len(cases)}"
    return cases, facts


# ---------------------------------------------------------------------------
# Temporal category: 5 cases.
# ---------------------------------------------------------------------------

_TEMPORAL = [
    ("temp-01", "When is the Q3 product review happening?",
     "The Q3 product review is scheduled for the third Tuesday of September 2026."),
    ("temp-02", "By when must the office lease be renewed?",
     "The office lease renewal deadline is January 15, 2027."),
    ("temp-03", "When does the annual security audit start?",
     "The annual security audit begins the first Monday of November 2026."),
    ("temp-04", "When does the next onboarding cohort begin?",
     "The new hire onboarding cohort starts on 2026-10-05."),
    ("temp-05", "When do the new data-retention rules take effect?",
     "The data-retention policy changes take effect on 2027-03-01."),
]


def _build_temporal() -> tuple[list[Case], list[Fact]]:
    cases: list[Case] = []
    facts: list[Fact] = []
    n = len(_TEMPORAL)
    for cid, _query, fact_text in _TEMPORAL:
        facts.append(Fact(f"{cid}-fact", "fact", fact_text))
    for i, (cid, query, _fact_text) in enumerate(_TEMPORAL):
        d1 = f"{_TEMPORAL[(i - 1) % n][0]}-fact"
        d2 = f"{_TEMPORAL[(i + 1) % n][0]}-fact"
        cases.append(
            Case(
                id=cid,
                category="temporal",
                query=query,
                expected_fact_id=f"{cid}-fact",
                distractor_fact_ids=(d1, d2),
            )
        )
    assert len(cases) == 5, f"expected 5 temporal cases, got {len(cases)}"
    return cases, facts


# ---------------------------------------------------------------------------
# Expected-empty category: 5 cases, each an isolated disposable brain
# containing only one unrelated filler fact -- no current fact should ever
# satisfy the query. See docs/launch/hosted-completion/embedding-eval.md
# "Known limits" for why fixture mode approximates "forgotten/expired" as
# "never authored" rather than exercising the real forget/expiry pipeline
# (out of T23.43's owned paths).
# ---------------------------------------------------------------------------

_EMPTY = [
    ("empty-01", "What was the name of the pet David gave up for adoption last year?", "filler-07"),
    ("empty-02", "Which apartment did Maya live in before her 2023 move?", "filler-08"),
    ("empty-03", "What was the old vendor contract number before it was superseded?", "filler-09"),
    ("empty-04", "What temporary badge code was issued during the office renovation?", "filler-10"),
    ("empty-05", "What was the original launch date before it was rescheduled?", "filler-01"),
]


def _build_empty() -> list[Case]:
    cases = [
        Case(
            id=cid,
            category="empty",
            query=query,
            expected_fact_id=None,
            distractor_fact_ids=(),
            isolated_filler_fact_id=filler_id,
        )
        for cid, query, filler_id in _EMPTY
    ]
    assert len(cases) == 5, f"expected 5 empty cases, got {len(cases)}"
    return cases


def build_corpus() -> tuple[list[Case], list[Fact]]:
    """Returns (cases, facts): 100 cases total (95 positive + 5 empty), and
    every fact referenced by any case's expected_fact_id/distractor_fact_ids
    plus the filler pool.
    """
    all_cases: list[Case] = []
    all_facts: list[Fact] = list(FILLERS)

    for builder in (_build_paraphrase, _build_name_entity, _build_preference, _build_multilingual, _build_temporal):
        cases, facts = builder()
        all_cases.extend(cases)
        all_facts.extend(facts)

    all_cases.extend(_build_empty())

    assert len(all_cases) == 100, f"expected 100 total cases, got {len(all_cases)}"
    positive = [c for c in all_cases if c.category != "empty"]
    assert len(positive) == 95, f"expected 95 positive cases, got {len(positive)}"

    fact_ids = {f.id for f in all_facts}
    dup = len(fact_ids) != len(all_facts)
    assert not dup, "duplicate fact id in corpus"
    for c in all_cases:
        if c.expected_fact_id is not None:
            assert c.expected_fact_id in fact_ids, f"{c.id}: missing expected fact {c.expected_fact_id}"
        for d in c.distractor_fact_ids:
            assert d in fact_ids, f"{c.id}: missing distractor fact {d}"
        if c.isolated_filler_fact_id is not None:
            assert c.isolated_filler_fact_id in fact_ids, f"{c.id}: missing filler {c.isolated_filler_fact_id}"

    return all_cases, all_facts
