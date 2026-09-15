# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.299 |
| store_sources | 10000 | 34.129 |
| chunk_extract | 10000 | 0.753 |
| reconcile | 10000 | 8.225 |
| write_claims | 10000 | 0.826 |
| index | 10000 | 17.155 |
| embed | 20050 | 8.596 |
| **Measured total** | | **69.982** |

Previous run: 65.798 seconds. Change: +6.36%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
