# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.348 |
| store_sources | 10000 | 33.132 |
| chunk_extract | 10000 | 0.935 |
| reconcile | 10000 | 7.470 |
| write_claims | 10000 | 0.992 |
| index | 10000 | 14.551 |
| embed | 20050 | 8.472 |
| **Measured total** | | **65.901** |

Previous run: 63.772 seconds. Change: +3.34%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
