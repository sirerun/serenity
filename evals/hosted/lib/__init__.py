"""T23.43 hosted semantic-retrieval qualification: corpus + harness library.

Every module here is pure Python with no network access at import time.
Network calls only happen inside eval_embeddings.py's --live path, gated by
an explicit manifest (docs/launch/hosted-completion/qualification.example.json).
"""
