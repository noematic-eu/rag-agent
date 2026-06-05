# Evaluating RAG Systems

Retrieval metrics include Recall at k, mean reciprocal rank (MRR), and nDCG.
Generation metrics such as faithfulness and answer relevancy are often computed with frameworks like RAGAS.

Hold the corpus, embedding model, and chat model constant when comparing two RAG implementations.
Use a gold question set with expected document or chunk identifiers for reproducible grading.
