# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.540 |
| store_sources | 10000 | 33.627 |
| chunk_extract | 10000 | 0.984 |
| reconcile | 10000 | 7.338 |
| write_claims | 10000 | 1.037 |
| index | 10000 | 14.199 |
| embed | 20050 | 8.073 |
| **Measured total** | | **65.798** |

Previous run: 61.708 seconds. Change: +6.63%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
