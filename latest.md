# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 20050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.326 |
| store_sources | 10000 | 32.833 |
| chunk_extract | 10000 | 1.015 |
| reconcile | 10000 | 7.334 |
| write_claims | 10000 | 0.982 |
| index | 10000 | 14.257 |
| embed | 20050 | 8.251 |
| **Measured total** | | **64.997** |

Previous run: 54.537 seconds. Change: +19.18%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
