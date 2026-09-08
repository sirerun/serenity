# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 10050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.329 |
| store_sources | 10000 | 28.647 |
| chunk_extract | 10000 | 0.955 |
| reconcile | 10000 | 6.484 |
| write_claims | 10000 | 122.652 |
| index | 10000 | 9.208 |
| embed | 10050 | 2.647 |
| **Measured total** | | **170.921** |

Previous run: 180.023 seconds. Change: -5.06%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
