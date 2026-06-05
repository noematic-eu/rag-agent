# Embeddings

An embedding model maps text to a dense vector in high-dimensional space.
Cosine similarity between query and chunk vectors approximates semantic relevance.

Local stacks often use Ollama with models such as nomic-embed-text or mxbai-embed-large.
Disabling embeddings falls back to BM25-only retrieval, which is faster but weaker on paraphrases.
