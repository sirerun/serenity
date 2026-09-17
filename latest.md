# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.341 |
| store_sources | 10000 | 32.219 |
| chunk_extract | 10000 | 1.008 |
| reconcile | 10000 | 7.608 |
| write_claims | 10000 | 1.002 |
| index | 10000 | 14.052 |
| embed | 20050 | 8.030 |
| **Measured total** | | **64.261** |

Previous run: 55.681 seconds. Change: +15.41%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
