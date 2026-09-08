# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 10050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.322 |
| store_sources | 10000 | 33.571 |
| chunk_extract | 10000 | 1.016 |
| reconcile | 10000 | 7.343 |
| write_claims | 10000 | 123.020 |
| index | 10000 | 10.465 |
| embed | 10050 | 3.294 |
| **Measured total** | | **179.031** |

First recorded run: regression comparison is not available; the four-hour ceiling is enforced.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
