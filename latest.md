# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.355 |
| store_sources | 10000 | 32.528 |
| chunk_extract | 10000 | 0.972 |
| reconcile | 10000 | 7.106 |
| write_claims | 10000 | 0.936 |
| index | 10000 | 14.042 |
| embed | 20050 | 8.177 |
| **Measured total** | | **64.117** |

Previous run: 62.872 seconds. Change: +1.98%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
