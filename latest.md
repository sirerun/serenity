# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.354 |
| store_sources | 10000 | 27.696 |
| chunk_extract | 10000 | 1.002 |
| reconcile | 10000 | 5.831 |
| write_claims | 10000 | 1.000 |
| index | 10000 | 11.744 |
| embed | 20050 | 6.909 |
| **Measured total** | | **54.537** |

Previous run: 50.248 seconds. Change: +8.53%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
