# Revue de code — `ai-rag-agent`

**Date :** 7 juin 2026  
**Scope :** ensemble du dépôt (~92 fichiers Go, ~11 300 lignes, 28 fichiers de tests)  
**Note globale : 6,5 / 10**

---

## Résumé exécutif

`ai-rag-agent` est un service RAG (Retrieval-Augmented Generation) Go bien documenté, orienté évaluation, avec une récupération hybride (BM25 lexical + embeddings vectoriels), un moteur lexical enfichable (Bleve, Tantivy, BM25 RAM f4kvs), une API HTTP Gin, un adaptateur Plan 9 / 9P optionnel, et un harnais d'évaluation solide.

Les points forts sont la documentation, l'abstraction des moteurs lexicaux, la tuning domaine légal, et le harnais d'évaluation. Les faiblesses principales pour une mise en production sont l'absence d'authentification, des scans linéaires sur le corpus, quelques bugs d'intégration, et une dette technique legacy (TF-IDF inutilisé).

---

## Table des matières

1. [Structure du projet](#1-structure-du-projet)
2. [Note par domaine](#2-note-par-domaine)
3. [Bugs identifiés (P0)](#3-bugs-identifiés-p0)
4. [Sécurité (P1)](#4-sécurité-p1)
5. [Performance et scalabilité (P2)](#5-performance-et-scalabilité-p2)
6. [Qualité de code (P3)](#6-qualité-de-code-p3)
7. [Tests et CI/CD](#7-tests-et-cicd)
8. [Documentation](#8-documentation)
9. [Liste priorisée des améliorations](#9-liste-priorisée-des-améliorations)
10. [Verdict final](#10-verdict-final)

---

## 1. Structure du projet

```
agent/          — Serveur HTTP, ingestion, chunking, ranking, LLM, 9P (~70 .go)
client/         — CLI : ingest, benchmark, eval récupération/génération
lexical/        — Abstraction moteurs lexicaux (Bleve, Tantivy, RAM BM25)
model/          — Types partagés (LegalDocument, Chunk, RetrieveResponse)
internal/f4kvs/ — Wrapper CGO autour du FFI Rust f4kvs
eval/           — Datasets JSONL gold, fixtures, script RAGAS Python
scripts/        — Bash : eval, lexical compare, release builds
docs/           — Notes d'architecture, quickstart Docker, checklist release
```

**Flux de données (correctement implémenté et documenté) :**

```
POST /ingest
  → normalisation HTML→Markdown
  → chunking AST goldmark
  → embeddings optionnels
  → index lexical + store f4kvs

GET /search | GET /retrieve
  → réécriture de requête (optionnelle)
  → récupération hybride BM25 + vecteurs
  → fusion RRF / pondérée
  → boost légal + reranking
  → génération LLM SSE (streaming)
```

---

## 2. Note par domaine

| Domaine | Note | Commentaire |
|---------|------|-------------|
| Architecture générale | 7/10 | Layering propre transport → Agent → pipeline ; moteurs lexicaux enfichables |
| Documentation | 9/10 | README exhaustif, eval README, docs/, CONTRIBUTING — exceptionnel pour un prototype |
| Tests | 6/10 | 28 fichiers, bon coverage chunking/ranking ; LLM, CORS, delete, finalize non couverts |
| Sécurité | 3/10 | Aucune auth, endpoints destructeurs publics, prompts exposés via SSE |
| Performance | 5/10 | Scans O(n) acceptables en prototype, bloquants à l'échelle |
| Qualité de code | 6/10 | Legacy TF-IDF, état global mutable, mélange FR/EN, timeout manquants |
| CI/CD | 7/10 | Pipeline solide ; limité sans token f4kvs privé |
| Ops / déploiement | 6/10 | Dockerfile sans Tantivy, pas de healthz, pas de rate limiting |

---

## 3. Bugs identifiés (P0)

### 3.1 `min_score` non appliqué dans le chemin multi-requêtes

`rankChunks` (`agent/rank.go:156-163`) applique correctement un filtre `min_score`. `rankChunksMulti` (`agent/rank_multi.go`), utilisé par défaut pour `legal-demo` via la réécriture de requête, ne l'applique **jamais**. Des résultats de faible confiance atteignent le LLM.

**Correction suggérée :** appliquer le même filtre `min_score` dans `rankChunksMulti` après fusion RRF.

---

### 3.2 `POST /finalize` retourne un body vide

`agent/finalize.go` appelle `ragAgent.Finalize()` sans écrire de réponse JSON. Gin retourne HTTP 200 avec un body vide. Le client `Finalize()` ne peut pas distinguer succès et erreur silencieuse.

**Correction suggérée :** ajouter `c.JSON(http.StatusOK, gin.H{"status": "ok"})`.

---

### 3.3 `extractHeadings` rend le document entier à chaque heading

`agent/chunk.go:54-64` rend le **document entier** (pas juste le nœud heading) pour extraire le texte d'un heading. Fonction non utilisée dans le chemin chaud actuel, mais incorrecte et coûteuse si appelée.

---

## 4. Sécurité (P1)

| Risque | Sévérité | Localisation |
|--------|----------|--------------|
| Aucune authentification API | **Haute** | `agent/main.go` routes |
| `POST /reset` public (wiping complet) | **Haute** | `agent/reset.go` |
| `POST /ingest` public (contenu arbitraire) | **Haute** | `agent/ingest.go` |
| Prompts complets exposés dans les métadonnées SSE | **Moyenne** | `agent/llm_generate.go:212-224` |
| Aucune limite de taille sur le body d'ingest | **Moyenne** | `agent/ingest.go` |
| 9P sans authentification, avec opérations mutantes | **Moyenne** | `agent/p9fs/` |

**Recommandations prioritaires :**
- Ajouter un middleware d'API key (header `Authorization: Bearer <token>`) sur toutes les routes mutantes.
- Retirer ou mettre derrière un flag `RAG_DEBUG_SSE_METADATA` l'exposition des prompts dans les événements SSE.
- Ajouter `MaxBytesReader` sur le body de `/ingest` (ex. 10 MB).

---

## 5. Performance et scalabilité (P2)

| Bottleneck | Complexité | Impact | Localisation |
|------------|------------|--------|--------------|
| Recherche vectorielle | O(chunks) par requête | Fort à >10k chunks | `vector_search.go:42` |
| BM25 RAM (f4kvs) | O(chunks) par requête | Fort | `f4kvs_ram.go:102` |
| Boost articles légaux | O(chunks) par requête | Modéré | `rank_legal.go:46` |
| Suppression document | O(chunks) par suppression | Faible (rare) | `delete_document.go:47` |
| Mutex global sur `Agent` | Ingest bloque toutes les recherches | Modéré sous charge | `service.go:38-41` |
| HyDE + réécriture requête | +2–3 appels LLM par requête faible | Latence non bornée | `hyde.go`, `query_rewrite.go` |

**Recommandations :**
- À partir de ~20k chunks : ajouter un index ANN (HNSW via `hnswlib-go` ou `faiss`) pour la recherche vectorielle.
- Ajouter un index secondaire `doc_id → []chunk_id` pour éviter les scans complets à la suppression et au boost.
- Séparer le mutex en read/write lock (`sync.RWMutex`) pour permettre des recherches concurrentes.
- Limiter le nombre de tours HyDE (ex. max 1 tentative) et borner le timeout LLM.

---

## 6. Qualité de code (P3)

### 6.1 État global mutable dans `package main`

`chunkStore`, `lexicalBackend`, `documentTFIDFs`, `globalIDF`, `llmConfig`, `statsState`, `ragAgent` sont des variables de package dans `agent/stores.go`, `agent/bleve.go`, etc. Cela complique les tests d'intégration, le raisonnement sur la concurrence, et tout futur usage multi-tenant.

**Recommandation :** regrouper en un struct `Server` injecté via les handlers Gin.

---

### 6.2 Legacy TF-IDF inutilisé

`documentTFIDFs` et `globalIDF` sont mis à jour à chaque ingest mais **jamais consultés** dans le pipeline de récupération principal (Bleve/Tantivy/vecteurs). `POST /finalize` est documenté comme "legacy".

**Recommandation :** retirer le code TF-IDF ou l'isoler derrière un build tag `//go:build tfidf`.

---

### 6.3 Mélange français / anglais

Logs, messages d'erreur API, et chaînes de statut mélangent les deux langues (`agent/ingest.go:16`, `agent/main.go:63`). 

**Recommandation :** adopter l'anglais pour tout le code (logs, erreurs API) ; le français peut rester dans les fixtures eval et la documentation.

---

### 6.4 Timeout manquant sur le streaming LLM

`agent/llm_generate.go:227-269` utilise `http.DefaultClient` sans timeout pour les appels de génération en streaming. Une réponse LLM bloquée tient une goroutine indéfiniment.

**Recommandation :** créer un client HTTP dédié avec `ResponseHeaderTimeout` (ex. 60s) et propager le `context` de la requête HTTP.

---

### 6.5 Propagation du contexte HTTP manquante

`context.Background()` est utilisé pour les appels LLM (`llm_generate.go:227`). Si le client HTTP se déconnecte, l'appel LLM continue jusqu'à complétion.

**Recommandation :** passer `c.Request.Context()` dans tous les appels LLM/embedding.

---

### 6.6 Dockerfile sans le tag `tantivy`

`Dockerfile:32` build l'agent sans `-tags tantivy`. L'image Docker ne peut pas utiliser `-lexical-engine=tantivy`, contrairement au build local via `make agent`.

**Correction :**
```dockerfile
RUN CGO_ENABLED=1 go build -tags tantivy -o /src/bin/agent ./agent
```

---

### 6.7 Typo dans le client

`ConstitionFrancaise` (manque un 't') dans `client/constitution-francaise.go` et `client/main.go:28`.

---

### 6.8 Comptage de tokens inexact

`countTokens` dans `agent/chunk.go:196-200` compte les mots (whitespace split), pas les vrais tokens LLM. Le paramètre `MaxTokens: 600` est donc une approximation grossière qui peut sous-chunker ou sur-chunker.

**Recommandation :** utiliser `tiktoken-go` ou une approximation calibrée (1 token ≈ 0.75 mots pour le français).

---

## 7. Tests et CI/CD

### 7.1 Couverture des tests

| Zone | État |
|------|------|
| Chunking, splitting légal, snippets | Bon (>36 fonctions de test) |
| Ranking, fusion RRF, reranking légal | Bon |
| Réécriture de requête, HyDE parsing | Bon |
| Moteurs lexicaux (Bleve, f4kvs RAM) | Bon |
| Handlers HTTP ingest/retrieve | Partiel |
| `llm_complete.go`, streaming LLM | **Absent** |
| `cors.go` | **Absent** |
| Handler `delete_document` | **Absent** |
| Réponse handler `finalize` | **Absent** |
| `rankChunksMulti` + `min_score` | **Absent** |
| Ingest concurrent + search | **Absent** |
| Build Docker | **Absent** |

### 7.2 CI

- **Job `check`** : fmt, vet, test-lite, golangci-lint, build client — toujours exécuté.
- **Job `build-and-test`** : conditionné au secret `F4KVS_REPO_TOKEN` — le cœur du package agent n'est pas couvert en CI publique.
- **Workflow `eval`** : matrice bleve/tantivy/f4kvs, gate recall 0.65, upload artefacts — bonne régression.

**Manquant :**
- Workflow de release (pas de push d'image Docker, pas de binaires publiés automatiquement).
- Scan de sécurité (`govulncheck`, `Trivy` sur l'image Docker).
- Tests de génération LLM et RAGAS dans l'éval CI.

---

## 8. Documentation

**Points forts :**
- `README.md` — architecture, contrat API, guide 9P, comparaison moteurs, quickstart eval, variables d'environnement. Niveau production.
- `CONTRIBUTING.md` — clair sur l'accès f4kvs privé.
- `eval/README.md` — format gold, métriques, grille A/B/C.

**Manques :**
- Pas de spec OpenAPI/Swagger pour l'API HTTP.
- Pas de diagramme d'architecture (le README est uniquement en prose).
- Variables d'environnement documentées uniquement dans le README, pas dans `.env.example`.

---

## 9. Liste priorisée des améliorations

### P0 — Bugs bloquants

1. **Appliquer `min_score` dans `rankChunksMulti`** (`agent/rank_multi.go`).
2. **Ajouter une réponse JSON à `POST /finalize`** (`agent/finalize.go`).

### P1 — Sécurité

3. **Authentification API** : ajouter un middleware `Authorization: Bearer` sur les routes mutantes.
4. **Retirer ou conditionner l'exposition des prompts** dans les métadonnées SSE (`llm_generate.go:212-224`).
5. **Limiter la taille du body ingest** (`MaxBytesReader`, ex. 10 MB).

### P2 — Performance

8. **Index ANN pour la recherche vectorielle** (HNSW) à partir de ~20k chunks.
9. **Index secondaire `doc_id → []chunk_id`** pour supprimer les scans O(n) à la suppression et au boost.
10. **`sync.RWMutex`** sur `Agent` pour permettre des recherches concurrentes.
11. **Borner le timeout LLM** sur les chemins HyDE et réécriture de requête.

### P3 — Qualité de code

12. **Supprimer le code TF-IDF legacy** (`documentTFIDFs`, `globalIDF`, `POST /finalize` IDF path).
13. **Propager le contexte HTTP** (`c.Request.Context()`) dans tous les appels LLM/embedding.
14. **Ajouter le tag `tantivy` dans le Dockerfile**.
15. **Regrouper l'état global** en un struct `Server` injecté.
16. **Corriger `countTokens`** : utiliser une approximation de tokens LLM, pas un word count.
17. **Corriger la typo** `ConstitionFrancaise` → `ConstitutionFrancaise`.
18. **Uniformiser la langue** (anglais) dans les logs et messages d'erreur API.

### P4 — CI/CD et ops

19. **Ajouter un endpoint `/healthz`** pour les liveness/readiness probes Kubernetes/Docker.
20. **Workflow de release** : build multiplateforme + push image Docker automatisé.
21. **Scan de sécurité CI** : `govulncheck` + `Trivy` sur l'image Docker.
22. **Spec OpenAPI** générée (ex. via `swag` ou `ogen`).
23. **Tests d'intégration HTTP** sans f4kvs (mock store) pour couvrir CORS, delete, finalize en CI publique.
24. **Diagramme d'architecture** dans la documentation.

---

## 10. Verdict final

**Note : 6,5 / 10**

| Critère | Note |
|---------|------|
| Architecture & design | 7/10 |
| Documentation | 9/10 |
| Sécurité | 3/10 |
| Tests | 6/10 |
| Performance | 5/10 |
| Qualité de code | 6/10 |
| CI/CD & ops | 6/10 |

**Prototype de recherche bien conçu**, avec une documentation et un harnais d'évaluation de niveau production. Les fondations architecturales sont solides (layering propre, moteurs lexicaux enfichables, pipeline hybride). Pour passer en production, les trois chantiers prioritaires sont : **sécurité** (authentification + limites), **correction des bugs d'intégration** (min_score, finalize), et **scalabilité** (index ANN, mutex RW, scans O(n)).
