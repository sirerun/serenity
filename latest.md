# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.366 |
| store_sources | 10000 | 31.951 |
| chunk_extract | 10000 | 0.954 |
| reconcile | 10000 | 6.995 |
| write_claims | 10000 | 0.950 |
| index | 10000 | 13.741 |
| embed | 20050 | 7.914 |
| **Measured total** | | **62.872** |

Previous run: 64.261 seconds. Change: -2.16%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
