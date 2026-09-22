# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.358 |
| store_sources | 10000 | 33.420 |
| chunk_extract | 10000 | 1.021 |
| reconcile | 10000 | 7.564 |
| write_claims | 10000 | 0.991 |
| index | 10000 | 14.430 |
| embed | 20050 | 8.245 |
| **Measured total** | | **66.030** |

Previous run: 56.280 seconds. Change: +17.32%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
