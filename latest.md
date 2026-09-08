# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.283 |
| store_sources | 10000 | 27.220 |
| chunk_extract | 10000 | 0.745 |
| reconcile | 10000 | 5.278 |
| write_claims | 10000 | 0.904 |
| index | 10000 | 10.147 |
| embed | 20050 | 5.671 |
| **Measured total** | | **50.248** |

Previous run: 58.763 seconds. Change: -14.49%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
