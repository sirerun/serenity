# Search canonical facts

`serenity --root /path/to/brain search "your question"` searches raw source text,
entity summaries and derived canonical claim text. After inbox approval, a human
assertion is searchable even when its wording never appeared in a raw source.
With no embedding model configured, the command reports keyword-only search.

Canonical claim snippets put the fact first and retain its subject, predicate,
confidence, actor and review qualifier. Their references identify derived search
entries; the canonical claim ID and files remain the source of truth. Search does
not create another canonical fact or rewrite a source.

Current fence facts and resolved shard heads are indexed. Numbered shard segments
belong to one family, including when only numbered segments remain. Superseded,
retracted, expired and future claims do not become current search evidence.

The local owner can search private canonical facts. Remote recall and provider
embedding/composition exclude private and index-only evidence. Native claims with
a nonempty source digest require that source's canonical metadata to exist before
remote/provider disclosure. Human assertions without a machine source digest keep
their explicit human attribution. Native legacy claims with no visibility marker
retain the existing single-principal shared default; imported claims retain their
explicit visibility requirement.

Eligibility is checked against canonical files on each request. Cached rows with
changed text, identity, attribution, confidence, validity or privacy cannot authorize
stale disclosure. A deleted claim is not returned through its old search entry.
After changing canonical files, run `serenity sync` to rebuild their search entries;
until then the old entry may be withheld rather than showing outdated evidence.
An index rebuild preserves inbox decisions. Do not delete `.serenity` to refresh
search.

Raw sources are still source evidence, distinct from current claims. Retracting a
claim does not erase the original source text or claim history. This repair covers
canonical claim projection and disclosure; it does not reinterpret every raw
source mention as a currently believed fact.
