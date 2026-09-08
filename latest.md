# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.292 |
| store_sources | 10000 | 28.930 |
| chunk_extract | 10000 | 0.873 |
| reconcile | 10000 | 6.331 |
| write_claims | 10000 | 0.772 |
| index | 10000 | 14.028 |
| embed | 20050 | 7.537 |
| **Measured total** | | **58.763** |

Previous run: 56.439 seconds. Change: +4.12%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
