# Synthetic cached-model import timing

Messages: 10000. Claims: 10000. Vectors: 10050. Cache hits: 10000. Live model calls: 0.

| Stage | Items | Seconds |
| --- | ---: | ---: |
| poll | 10000 | 0.529 |
| store_sources | 10000 | 33.098 |
| chunk_extract | 10000 | 0.989 |
| reconcile | 10000 | 7.271 |
| write_claims | 10000 | 0.946 |
| index | 10000 | 10.395 |
| embed | 10050 | 3.211 |
| **Measured total** | | **56.439** |

Previous run: 170.921 seconds. Change: -66.98%.

Corpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.
