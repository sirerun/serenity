# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.311 |
| store_sources | 10000 | 28.723 |
| chunk_extract | 10000 | 0.981 |
| reconcile | 10000 | 6.026 |
| write_claims | 10000 | 1.009 |
| index | 10000 | 12.161 |
| embed | 20050 | 7.069 |
| **Measured total** | | **56.280** |

Previous run: 64.117 seconds. Change: -12.22%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
