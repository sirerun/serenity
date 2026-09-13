# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.371 |
| store_sources | 10000 | 31.278 |
| chunk_extract | 10000 | 0.967 |
| reconcile | 10000 | 6.950 |
| write_claims | 10000 | 0.962 |
| index | 10000 | 13.298 |
| embed | 20050 | 7.881 |
| **Measured total** | | **61.708** |

Previous run: 64.997 seconds. Change: -5.06%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
