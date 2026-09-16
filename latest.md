# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.324 |
| store_sources | 10000 | 28.072 |
| chunk_extract | 10000 | 0.957 |
| reconcile | 10000 | 5.914 |
| write_claims | 10000 | 0.970 |
| index | 10000 | 12.442 |
| embed | 20050 | 7.003 |
| **Measured total** | | **55.681** |

Previous run: 69.982 seconds. Change: -20.44%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
