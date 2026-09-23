# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.346 |
| store_sources | 10000 | 32.329 |
| chunk_extract | 10000 | 0.979 |
| reconcile | 10000 | 7.217 |
| write_claims | 10000 | 0.951 |
| index | 10000 | 13.946 |
| embed | 20050 | 8.005 |
| **Measured total** | | **63.772** |

Previous run: 66.030 seconds. Change: -3.42%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
