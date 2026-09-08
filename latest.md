# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 10050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.589 |
| store_sources | 10000 | 34.367 |
| chunk_extract | 10000 | 1.026 |
| reconcile | 10000 | 7.673 |
| write_claims | 10000 | 122.538 |
| index | 10000 | 10.496 |
| embed | 10050 | 3.334 |
| **Measured total** | | **180.023** |

Previous run: 179.031 seconds. Change: +0.55%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
