"""Content-word overlap: an honest, computed check for T23.43's "at least
20 paraphrases lack useful content-word overlap" requirement, rather than a
hand-set flag nobody re-verifies. corpus_gen.py calls content_overlap() on
every paraphrase case and refuses to freeze the corpus if fewer than 20 come
out empty (see corpus_gen.MIN_ZERO_OVERLAP_PARAPHRASES).
"""

from __future__ import annotations

import re
import unicodedata

# Deliberately small and English-only: this repo's "lacks content-word
# overlap" requirement only applies to the English-language paraphrase
# category (T23.43.md step 1); multilingual cases are scored separately and
# never run through this function.
_STOPWORDS = frozenset(
    """
    a an the this that these those is are was were be been being
    do does did doing have has had having
    i you he she it we they me him her us them my your his its our their
    mine yours hers ours theirs myself yourself himself herself itself
    ourselves yourselves themselves
    and or but if then else when while as of at by for with about
    against between into through during before after above below to from
    up down in out on off over under again further once here there all
    any both each few more most other some such no nor not only own
    same so than too very can will just don should now
    what which who whom
    does do did
    """.split()
)


def tokenize(text: str) -> list[str]:
    """Lowercase, strip accents/punctuation, split on whitespace."""
    normalized = unicodedata.normalize("NFKD", text)
    ascii_only = normalized.encode("ascii", "ignore").decode("ascii")
    return re.findall(r"[a-z0-9]+", ascii_only.lower())


def content_words(text: str, extra_stop: frozenset[str] = frozenset()) -> set[str]:
    """Tokens with English stopwords (and any caller-supplied extra
    stopwords, e.g. the subject's own name -- a disambiguating name
    naturally repeats between a fact and its paraphrase query and is not
    the "content word overlap" this check is measuring) removed.
    """
    return {t for t in tokenize(text) if t not in _STOPWORDS and t not in extra_stop}


def content_overlap(query: str, fact_text: str, subject_tokens: frozenset[str] = frozenset()) -> set[str]:
    """Content words shared between query and fact_text, excluding
    subject_tokens (see content_words). Empty return means a genuine
    paraphrase: the query cannot be answered by lexical overlap alone.
    """
    return content_words(query, subject_tokens) & content_words(fact_text, subject_tokens)
