# Document Chunking

Chunking splits long documents into retrieval units of roughly four hundred to eight hundred tokens.
Structure-aware chunking respects Markdown headings, code blocks, and tables.

Overlap between adjacent chunks (ten to twenty percent) reduces the chance that an answer span sits on a boundary.
Each chunk stores metadata: document id, chunk id, section path, and corpus tag.
