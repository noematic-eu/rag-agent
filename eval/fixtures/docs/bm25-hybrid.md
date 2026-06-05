# BM25 and Hybrid Search

BM25 is a probabilistic ranking function used for lexical retrieval.
It scores term frequency and inverse document frequency without neural networks.

Hybrid search merges BM25 results with vector search using weighted scores or reciprocal rank fusion (RRF).
A typical fusion weight gives roughly sixty percent weight to the vector branch and forty percent to BM25.
