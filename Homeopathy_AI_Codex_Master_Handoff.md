# Homeopathy AI — Codex/Astra Project Handoff & Full Conversation Summary

**Prepared for:** Continuing development/research in Codex / GPT-6 Astra  
**Date:** 2026-10-05  
**Status:** Canonical working handoff  
**Primary next phase:** End-to-end POC — Source Ingestion to Citation-Grounded Answers
**Deployment:** ARM Mac, 48 GB RAM (up to 38 GB available for AI). App/OCR/database/storage in Docker Compose; existing native LM Studio recommended for local inference. Strict-container CPU alternative remains available.
**OCR preference:** Tesseract baseline; compare alternatives only if scan quality warrants it (PRD section 57)

**Scope correction — 2026-10-05:** The user requires an end-to-end POC on an Apple Silicon (ARM) Mac: source ingestion → OCR/review → indexing with PostgreSQL FTS + pgvector → questions → researched answers with verifiable citations. Ingestion is the first milestone, not the complete POC. PRD section 58 defines the complete scope and ARM inference options.

**Review update — 2026-10-05:** Go replaces the previous Python API/worker choice at the user's request. shadcn-vue and the supporting Go libraries are recommendations pending discussion. The [POC PRD](Homeopathy_AI_Source_Ingestion_POC_PRD.md) is authoritative for implementation details; its section 56 defines revision, publication, and verification requirements. This handoff preserves product context and future direction.

---

# 1. How to use this file

Treat this document as the working continuation context for the **Homeopathy AI research product**.

Do not restart the product strategy from zero.

The immediate development focus is:

> Build a trustworthy source-ingestion POC that can discover/import authentic classical homeopathic sources, preserve provenance, extract/OCR pages, map printed pages, create structured chunks, run QA, allow human review, and publish only approved sources, index them with PostgreSQL FTS + pgvector, and answer questions with verifiable passage/page citations.

The product is intentionally being built **evidence-first**.

The first milestone is a reliable, traceable source corpus. Completion requires questions, evidence synthesis, citations, and the original-page viewer.

---

# 2. Product Vision

Homeopathy AI is intended to become a:

> **Global AI research + clinical-reference assistant for homeopathic practitioners and students.**

It must not be positioned as a generic chatbot.

The core experience should eventually be:

```text
User asks a normal practitioner/research question
        ↓
System finds appropriate evidence
        ↓
System answers clearly
        ↓
Every substantive claim links to exact evidence
        ↓
User can inspect the original source passage/page
```

The platform should eventually combine:

- classical homeopathic literature;
- modern clinical research;
- Indian homeopathic research;
- international research;
- general medical/safety information;
- structured remedy knowledge;
- condition/topic knowledge;
- citation-linked AI research workflows.

India is a major part of the corpus, but the product should be global rather than India-only.

---

# 3. Core Trust Principle

The product must keep evidence types separate.

## 3.1 Classical reference

Statements made in historical/classical homeopathic works.

Example framing:

> “Hering describes…”

## 3.2 Clinical research

What trials, systematic reviews, observational studies, etc. actually report.

Example framing:

> “A randomized trial reported…”

## 3.3 Product information

What official product records or labels declare.

## 3.4 Safety/guideline information

Current authoritative safety, interaction, referral, and general medical information.

## 3.5 Critical rules

The system must never:

- convert classical textual claims into modern clinical efficacy claims;
- transfer evidence between similarly named but materially different preparations;
- infer safety from the absence of a materia-medica mention;
- present absence of evidence as proof that no study exists;
- invent bibliographic information;
- silently merge incompatible editions;
- generate unsupported patient-specific prescriptions;
- let fluent language override source evidence.

The working philosophy is:

> **Evidence first, fluent answer second.**

---

# 4. Original MVP Vision

The initial product milestone remains:

> **Question → evidence-backed answer → exact citation/source**

The required local POC research interface should include:

1. Chat/search input
2. Answer
3. Citation/evidence cards
4. Source viewer / source details panel

Example target interaction:

```text
User:
What does Hering say about Nux vomica and constipation?

System:
Hering describes ...

Source:
Hering, Guiding Symptoms of Our Materia Medica
Volume ...
Page ...

[Read original passage]
```

The citation must open the relevant source passage/page rather than being a generic reference at the end of the answer.

---

# 5. What We Decided NOT to Build First

Do not start with:

- full repertorization;
- automated prescribing;
- patient diagnosis;
- constitutional analysis;
- case management;
- automated remedy selection;
- voice assistant;
- mobile app;
- browser extension;
- community/forum;
- complex multi-agent architecture;
- personalized AI memory;
- large clinical decision-support features;
- deep research agents;
- thousands of books immediately.

These are later-stage possibilities.

---

# 6. Existing Starting Point

A small proof of concept already exists conceptually/previously as:

```text
source gathering
    ↓
embeddings
    ↓
LLM Q&A
```

The project must not restart from zero.

The evolution is toward:

```text
User question
    ↓
Query understanding/classification
    ↓
Hybrid retrieval
    ├── lexical / BM25-style retrieval
    └── vector / semantic retrieval
    ↓
Candidate passages
    ↓
Reranking
    ↓
Evidence set
    ↓
LLM answer generation
    ↓
Citation validation
    ↓
Answer + evidence cards + source viewer
```

However, before building this retrieval/answer flow, the source foundation must be reliable.

---

# 7. Key Realization from This Conversation

The immediate problem is not:

> “How do we make the chatbot answer?”

The immediate problem is:

> **“How do we reliably feed authentic homeopathic sources into the system?”**

Therefore the project order was changed to:

```text
PHASE 1
Source Management + Trusted Source Ingestion

PHASE 2
Document processing + page extraction

PHASE 3
Source QA / page verification

PHASE 4
Indexing + embeddings

PHASE 5
Hybrid retrieval

PHASE 6
Answer + citation engine

PHASE 7
User-facing research interface
```

---

# 8. Manual Feeding vs Automated Ingestion — Final Decision

The user should **not manually copy/paste books or individual passages**.

The correct model is:

> **Curated trusted-source registry → automated discovery/import → automated processing → automatic QA → human review only at trust-critical checkpoints → publish**

The system should automate repetitive processing but not blindly scrape the internet.

---

# 9. Trusted-Source Ingestion Philosophy

Do NOT build:

```text
search the public internet
→ download anything containing “homeopathy”
→ embed it
```

Build:

```text
approved repositories
        ↓
approved authors / works
        ↓
specific source record / edition
        ↓
automated import
        ↓
processing
        ↓
QA
        ↓
human approval
        ↓
publish
```

Important distinction:

> **Discovery may become automatic. Publication remains controlled.**

---

# 10. Trusted Source Registry

The system needs an internal registry for approved source repositories.

Initial examples identified:

- Wellcome Collection
- National Library of Medicine Digital Collections
- University of Michigan Homeopathy Collection
- Internet Archive / Medical Heritage Library
- Library of Congress
- Gallica
- HathiTrust
- other verified institutional/archive collections later

Each repository record should include:

```text
repository_id
name
domain
trust_tier
connector_type
metadata_support
ocr_support
page_image_support
rights_metadata_support
base_url
terms_url
enabled
```

Connector types may include:

- API
- IIIF
- direct-download
- custom adapter
- manual-only

---

# 11. Source Connectors

Repository-specific code must be isolated behind adapters.

Example:

```text
/connectors

wellcome.go
nlm.go
internet_archive.go
michigan.go
loc.go
gallica.go
```

Each connector should expose a common interface conceptually:

```go
type SourceConnector interface {
    Search(ctx context.Context, query SearchQuery) ([]SourceRecord, error)
    GetRecord(ctx context.Context, externalID string) (SourceRecord, error)
    GetMetadata(ctx context.Context, externalID string) (SourceMetadata, error)
    GetFiles(ctx context.Context, externalID string) ([]RemoteFile, error)
    GetRights(ctx context.Context, externalID string) (RightsStatement, error)
}
```

All connectors normalize data into one internal shape such as:

```json
{
  "repository": "repository-key",
  "external_id": "123",
  "title": "...",
  "author": "...",
  "year": 1891,
  "edition": "...",
  "volume": "...",
  "language": "English",
  "source_url": "...",
  "document_url": "...",
  "ocr_url": null,
  "rights_statement": "...",
  "identifiers": {}
}
```

The downstream ingestion pipeline should not care which repository produced the source.

---

# 12. POC Connector Strategy

Do NOT implement many connectors at first.

POC-1 should support:

1. **One automated trusted repository connector**
2. **Manual PDF upload**

Both paths must feed into the same ingestion pipeline.

Recommended first automated repository candidate:

- Wellcome Collection

Alternative trusted repository may be selected if implementation access is better.

---

# 13. Source Authenticity / Provenance Validation

The system should automatically check:

## Repository

Is the source from an approved repository?

## Author

Does metadata match the expected author?

## Title

Does normalized title match the expected work?

## Publication year

Is it plausible / consistent?

## Volume

Is the volume identified and complete?

## Edition

Must be stored distinctly rather than silently merged.

## File checksum

Every downloaded/uploaded original file should receive a SHA-256 checksum.

This allows the system to prove:

> A later answer came from this exact file of this exact edition.

---

# 14. Deduplication

The same work may exist in multiple repositories.

The system must distinguish:

```text
WORK
Clarke — Dictionary of Practical Materia Medica
        │
        ├── Edition A
        │    ├── Scan A / Repository 1
        │    └── Scan B / Repository 2
        │
        └── Edition B
             └── Scan C
```

Deduplication should occur at multiple levels:

### Exact file duplicate

Use SHA-256.

### Probable edition duplicate

Compare normalized:

- author
- title
- edition
- publisher
- year
- volume

### Work duplicate

Compare canonical author + canonical work.

Duplicate works are allowed.

Identical files should not be stored repeatedly unless explicitly required.

---

# 15. Rights / Licensing Requirement

Online availability must NOT automatically mean:

> safe for commercial ingestion or redistribution.

Store:

```text
rights_status
rights_statement_raw
rights_source
rights_jurisdiction
rights_checked_at
commercial_use_status
review_status
reviewer_notes
```

Suggested status values:

```text
public_domain
licensed
restricted
needs_review
unknown
```

Repository metadata can be imported, but the system must not invent additional legal permissions.

---

# 16. Initial Classical Corpus

**Prepared starter set:** Nash, *Leaders in Homoeopathic Therapeutics* (1899), and Farrington, *A Clinical Materia Medica* (1890, second edition), both from Wellcome. Original PDFs, raw catalogue/IIIF metadata, page maps, rights evidence, and checksums are saved locally. Use [the ingestion guide](data/starter-corpus/README.md) and [manifest](data/starter-corpus/manifest.json); PRD section 62 records verification. Both PDFs include a generated cover sheet and scanned body pages: never equate PDF position with printed page, or mistake cover-sheet text for a usable book text layer. These sources are ready for import/processing, not pre-approved publication.

Do not start with thousands of books.

Target Classical Corpus V1: approximately 8–10 important works.

Suggested initial set:

### Hahnemann
- Organon of Medicine
- Materia Medica Pura
- Chronic Diseases

### Hering
- Guiding Symptoms of Our Materia Medica

### Kent
- Repertory of the Homoeopathic Materia Medica

### Clarke
- A Dictionary of Practical Materia Medica

### Farrington
- A Clinical Materia Medica

### Boericke
- Pocket Manual of Homoeopathic Materia Medica

### Nash
- Leaders in Homoeopathic Therapeutics

Do not ingest all ten before testing.

POC-1 validation target:

```text
2–3 works
1 automated trusted repository
1 manual upload path
```

---

# 17. Architecture Decision

The user explicitly requested:

- Vue for the frontend
- not a monolith
- but also not excessive microservices

Final architectural choice:

> **Modular monolith + separate background ingestion worker**

This means:

- one repository / monorepo;
- shared domain models;
- strong module boundaries;
- Go HTTP API runs as the HTTP/API process;
- ingestion worker runs as a separate process;
- PostgreSQL is shared;
- PostgreSQL-backed River is proposed for background job coordination;
- MinIO/S3 stores immutable source assets;
- selected modules can later be extracted if scaling requires it.

Avoid early service sprawl such as:

```text
source-service
document-service
ocr-service
page-service
chunk-service
rights-service
metadata-service
```

These should be backend modules, not independently deployed business services in POC-1.

---

# 18. Recommended Runtime Stack

## Frontend

- Vue 3 + TypeScript + Vite
- Vue Router
- Tailwind CSS
- **Proposed:** shadcn-vue for components; UI choice remains open for discussion
- TanStack Table for source/review tables if shadcn-vue is selected
- Pinia only for shared client state; keep API fetching in a dedicated service layer

## Backend

- **Confirmed by user:** Go for the API and background worker
- **Recommended libraries:** net/http + chi, pgx + sqlc, Goose SQL migrations
- Standard-library HTTP client for repository connectors, with deadlines
- OpenAPI contract shared with the frontend; generate TypeScript API types
- Pin supported Go and dependency versions when scaffolding

## Worker / Jobs

- **Proposed:** River backed by PostgreSQL, with a separate Go worker process
- Enqueue jobs in the same database transaction as the ingestion record
- Redis is unnecessary for this proposal; add it only for a demonstrated requirement
- River owns queue delivery/retries; ingestion_jobs owns application progress and links to the queue job

## Database / Storage

- PostgreSQL + pgvector
- Install pgvector in the PostgreSQL container from the foundation milestone; embeddings and hybrid retrieval are required within this POC
- Local embedding and answer-generation providers, with pinned model versions; ARM inference choice and model size are detailed for the 48 GB ARM Mac with existing LM Studio (PRD section 58)
- S3-compatible object storage; MinIO remains the local development candidate

## Document Processing

Go owns workflow orchestration, provenance, review, and publication. PDF/OCR engines sit behind provider interfaces.

- **Proposed first spike:** Poppler command-line tools for PDF inspection/text/rendering and Tesseract for OCR
- Execute tools as bounded child processes inside the worker container, with explicit arguments, deadlines, resource limits, and temporary directories
- Preserve engine versions, configuration, language data versions, and output artifacts
- Benchmark on representative historical scans before committing to an engine
- POC OCR must run locally in Docker. A local Python processing adapter remains an option if benchmarking justifies it; hosted OCR is outside the POC baseline.

Conceptual interface (domain types defined during implementation):

```go
type OCRProvider interface {
    ExtractPage(ctx context.Context, page PageInput) (OCRResult, error)
}
```

Library choices above are recommendations, not additional user-confirmed decisions. See the review notes for tradeoffs and official references.

---

# 19. High-Level Runtime Architecture

```text
Vue admin → REST → Go API
                    ├── PostgreSQL + pgvector (domain data, FTS, vectors, River queue)
                    ├── local embedding / answer model runtime
                    └── S3-compatible object storage

Go worker ← River queue in PostgreSQL
    ├── PDF extraction / OCR provider
    ├── page mapping / normalization / chunking / QA
    ├── embedding provider → PostgreSQL + pgvector
    ├── PostgreSQL (progress + versioned results)
    └── object storage (originals + derived artifacts)
```

---

# 20. Backend Module Boundaries

```text
apps/backend/
├── cmd/
│   ├── api/main.go
│   └── worker/main.go
├── internal/
│   ├── httpapi/
│   ├── sources/
│   ├── connectors/
│   ├── ingestion/
│   ├── extraction/
│   ├── pages/
│   ├── chunking/
│   ├── review/
│   ├── publishing/
│   ├── storage/
│   └── database/
├── queries/
├── migrations/
├── tests/
├── go.mod
└── go.sum
```

Both entry points share domain/application packages. Keep business rules out of HTTP handlers and queue adapters.

POC modules: retrieval, answer engine, citation engine, evaluation. Reranking is an optional measured improvement.

---

# 21. Frontend Structure

Use feature-based Vue modules:

```text
frontend/
│
├── src/
│   ├── modules/
│   │   ├── dashboard/
│   │   ├── sources/
│   │   ├── discovery/
│   │   ├── ingestion/
│   │   ├── review/
│   │   └── repositories/
│   ├── components/
│   ├── stores/
│   ├── services/
│   ├── router/
│   ├── types/
│   └── utils/
└── tests/
```

Do not put business logic directly inside Vue components.

---

# 22. Canonical Data Model

Important hierarchy:

```text
Work
  ↓
Edition
  ↓
Source Record
  ↓
Source Asset
  ↓
Page
  ↓
Chunk
```

## Work

The intellectual work itself.

Example:

> Guiding Symptoms of Our Materia Medica

## Edition

The exact published edition/volume.

## Source Record

The record in an external repository.

## Source Asset

The exact PDF/OCR/page image/metadata file imported.

## Page

The exact physical/scan page.

## Chunk

A search/retrieval unit tied to one exact page.

---

# 23. Database Tables

These conceptual tables are extended by PRD section 56 with source, processing revision, publication, and asset-scoped rights entities. Implement that complete model.

## works

Suggested fields:

```text
id UUID
canonical_title
author_name
author_normalized
original_language
work_type
first_publication_year
description
created_at
updated_at
```

Possible `work_type` values:

```text
materia_medica
repertory
organon
therapeutics
proving
clinical_reference
other
```

## editions

```text
id UUID
work_id UUID
title_as_printed
edition_statement
publisher
publication_place
publication_year
volume_label
language
source_type
source_tier
rights_status
rights_statement_raw
rights_source
rights_jurisdiction
rights_review_status
rights_checked_at
ingestion_status
created_at
updated_at
```

## source_records

```text
id UUID
edition_id UUID
repository_id UUID
external_id
source_url
original_scan_url
metadata_json
accessed_at
is_preferred_source
created_at
```

## source_assets

```text
id UUID
edition_id
source_record_id
asset_type
storage_path
original_url
mime_type
file_size
sha256
downloaded_at
```

Asset types:

```text
pdf
ocr
page_image
metadata
```

## pages

```text
id UUID
edition_id
source_asset_id
scan_page_index
pdf_page_index
printed_page_label
printed_page_number
chapter
section
remedy_heading
text_raw
text_normalized
extraction_method
ocr_confidence
page_image_path
source_page_url
review_status
created_at
updated_at
```

## chunks

```text
id UUID
edition_id
page_id
chunk_index
text_exact
text_search
start_character
end_character
heading_path
remedy_normalized
chapter_normalized
token_count
source_type
language
created_at
```

POC rule:

> A chunk must never cross a physical page boundary.

## ingestion_jobs

```text
id UUID
edition_id
job_type
status
current_step
progress_percent
started_at
completed_at
error_code
error_message
retry_count
metadata_json
```

## review_issues

```text
id UUID
edition_id
page_id nullable
issue_type
severity
message
detected_by
status
resolution_note
resolved_by
resolved_at
```

Example issue types:

```text
ocr_low_confidence
page_number_uncertain
missing_metadata
duplicate_source
missing_page
sequence_break
rights_unknown
structure_uncertain
file_corrupt
author_mismatch
title_mismatch
```

## remedy_aliases

```text
id
canonical_name
alias
alias_type
language
verified
```

Example:

```text
Nux vomica
Nux-v.
Nux vom.
Nux v.
```

This table is useful now for structure detection and later for hybrid retrieval.

---

# 24. Critical Page-Number Rule

Never assume:

```text
PDF page == printed page
```

Store separately:

```text
scan_page_index
pdf_page_index
printed_page_label
printed_page_number
```

Example:

```text
PDF page index:   37
Scan page index:  38
Printed page:     21
```

The system should infer sequences where possible:

```text
PDF 37 → printed 21
PDF 38 → printed 22
PDF 39 → printed 23
```

and propose an offset with confidence.

Human review can then confirm.

---

# 25. Original vs Processed Text

Always preserve:

```text
text_raw
```

separately from:

```text
text_normalized
```

`text_raw` is the source extraction/OCR output.

`text_normalized` may include:

- Unicode normalization;
- whitespace cleanup;
- line-break cleanup;
- hyphenation repair;
- repeated header/footer handling;
- invalid control-character removal.

Normalization must not silently rewrite historical terminology.

---

# 26. Chunking Rules

Initial chunking:

```text
Target:
250–450 words

Overlap:
50–80 words

Boundary:
never cross physical page

Preferred split:
paragraph
section
remedy heading
```

Keep:

```text
text_exact
```

for evidence display.

Use:

```text
text_search
```

later for retrieval enrichment.

Do not add metadata to `text_exact`.

---

# 27. Source Lifecycle

Explicit source state machine:

```text
DRAFT
   ↓
DISCOVERED
   ↓
IMPORTED
   ↓
PROCESSING
   ↓
NEEDS_REVIEW
   ↓
APPROVED
   ↓
PUBLISHED
```

Alternative terminal/error states:

```text
PROCESSING_FAILED
REJECTED
DISABLED
```

Only `PUBLISHED` sources will later be used by production retrieval. POC publication does not require indexing. Manual uploads may skip DISCOVERED; see PRD section 56 for revision and transition rules.

---

# 28. Ingestion Pipeline

A source import should execute approximately:

```text
01 Acquire document
02 Verify file
03 Generate checksum
04 Inspect PDF
05 Extract embedded text
06 Determine OCR requirement
07 Generate page records
08 Extract/OCR text per page
09 Normalize text
10 Detect printed page numbers
11 Detect headings
12 Detect probable remedy headings
13 Generate page-safe chunks
14 Run QA rules
15 Generate review issues
16 Mark NEEDS_REVIEW
```

Embedding generation follows source approval and is required before the end-to-end POC is complete.

---

# 29. Automated QA

Minimum automated QA checks:

```text
file readable
non-zero pages
metadata present
author present
title present
publication information present or flagged
checksum generated
text extraction coverage
OCR coverage
page-number consistency
missing-page anomalies
duplicate detection
rights status present
page records created
chunk records created
```

Results:

```text
QA PASS
QA WARNING
QA FAIL
```

---

# 30. Human Review

Human review is retained only for trust-critical checkpoints.

The reviewer should verify:

```text
correct work
correct author
correct edition
complete volume
correct or acceptable provenance
rights information recorded
printed page mapping plausible
OCR/extraction quality acceptable
critical QA issues resolved
```

The user should not have to read the whole book.

---

# 31. POC Admin Screens

Group the screens below under **Ask**, **Sources**, and **Review**, with secondary **Settings**. They are not separate top-level navigation items. Follow PRD section 60 for short nontechnical guidance, next actions, and compact visual design.

The Vue POC should include:

1. Dashboard
2. Sources
3. Discover Sources
4. Add / Upload Source
5. Source Details
6. Processing Status
7. Review Queue
8. Page Review
9. Trusted Repositories

An authenticated local research question interface is required in this POC, alongside administration. Public deployment is not required.

---

# 32. Page Review UX

Suggested screen:

```text
┌────────────────────────┬─────────────────────────┐
│ Original Page          │ Extracted Text          │
│                        │                         │
│ page image/PDF         │ raw/normalized toggle   │
│                        │                         │
└────────────────────────┴─────────────────────────┘

Printed Page:   [421]

Chapter:        [...]

Section:        [...]

Remedy:         [Nux vomica]

Extraction:
OCR / Embedded

Confidence:
...

Issues:
...

[Save]
[Mark Reviewed]

[Previous] [Next]
```

---

# 33. Processing UI

Show asynchronous pipeline progress:

```text
Downloading            ✓
Verifying              ✓
Extracting             ✓
OCR                     ✓
Mapping pages           Running
Detecting structure     Pending
Chunking                Pending
QA                      Pending
```

Polling is acceptable for the first POC. Follow PRD section 61: persistent cross-screen Activity, real progress, completion notices, actionable errors, automatic-retry visibility, stale-connection warnings, and recovery after reload/restart. Never leave failed or stalled jobs displaying an endless spinner.

---

# 34. Authentication / Roles

POC can use basic admin authentication.

Roles:

```text
ADMIN
REVIEWER
```

ADMIN:

- configuration;
- import;
- approve;
- publish;
- disable;
- manage repositories.

REVIEWER:

- inspect source/page;
- correct metadata/page mapping;
- resolve review issues;
- approve review tasks where allowed.

Advanced enterprise identity is not needed yet.

---

# 35. Audit Trail

Log important changes:

```text
source imported
metadata changed
page number corrected
rights changed
source approved
source rejected
source published
source disabled
source reprocessed
```

Store:

```text
actor
action
entity
before
after
timestamp
```

---

# 36. Reprocessing / Versioning

The original source file must never be mutated.

Improved OCR, page mapping, normalization, structure detection, or chunking should be rerunnable.

Store processing component versions:

```text
extractor
ocr_engine
normalizer
page_mapper
chunker
```

Example:

```json
{
  "extractor": "1.0.0",
  "normalizer": "1.1.0",
  "chunker": "1.0.0"
}
```

Unpublished derived artifacts can be regenerated. Published revisions must remain addressable for citation integrity; see PRD section 56.

Original source assets are immutable.

---

# 37. Core API Requirements

## Sources

```text
GET    /api/v1/sources
GET    /api/v1/sources/{id}
POST   /api/v1/sources
PATCH  /api/v1/sources/{id}
POST   /api/v1/sources/{id}/approve
POST   /api/v1/sources/{id}/reject
POST   /api/v1/sources/{id}/publish
POST   /api/v1/sources/{id}/disable
POST   /api/v1/sources/{id}/reprocess
```

## Discovery

```text
GET /api/v1/discovery/search
```

Supported query parameters:

```text
repository
query
author
title
year
```

## Repository import

```text
POST /api/v1/imports/repository
```

Example:

```json
{
  "repository": "wellcome",
  "external_id": "..."
}
```

## Manual upload

```text
POST /api/v1/imports/upload
```

multipart:

```text
PDF
metadata
```

## Jobs

```text
GET  /api/v1/jobs
GET  /api/v1/jobs/{id}
POST /api/v1/jobs/{id}/retry
```

## Pages

```text
GET   /api/v1/sources/{source_id}/pages
GET   /api/v1/pages/{page_id}
PATCH /api/v1/pages/{page_id}
```

## Review

```text
GET   /api/v1/review/issues
PATCH /api/v1/review/issues/{id}
POST  /api/v1/pages/{id}/review
```

---

# 38. Security Requirements

At minimum:

- validate uploaded file types;
- limit upload size;
- protect against malicious filenames/path traversal;
- do not execute uploaded files;
- malware-scan hook;
- store assets outside the public web root;
- signed/authorized asset URLs;
- authentication for admin APIs;
- server-side permission checks;
- environment-based secret management;
- ORM/query parameterization;
- rate limiting on discovery/import endpoints.

---

# 39. Observability

Structured logs should include:

```text
job_id
source_id
edition_id
step
duration
status
```

Minimum metrics:

```text
ingestion duration
pages processed
pages requiring OCR
failed pages
QA warnings
QA failures
worker failures
```

A full observability platform is not required for POC-1.

---

# 40. Testing Strategy

## Unit tests

- metadata normalization;
- title normalization;
- author normalization;
- checksum generation;
- page sequence detection;
- chunk boundaries;
- rights-state validation;
- state transitions.

## Integration tests

- connector → normalized source;
- upload → asset storage;
- asset → pages;
- pages → chunks;
- processing → review queue.

## E2E

```text
discover source
→ import
→ process
→ review
→ approve
→ publish
```

Use small fixtures rather than full books for regular CI:

- text-native PDF;
- scanned PDF;
- roman-numeral front matter;
- printed-page offset;
- missing page;
- duplicate PDF.

---

# 41. POC Acceptance Criteria

POC-1 is done when:

## Source ingestion

Admin can:

- discover/import one source from one trusted repository;
- manually upload one PDF.

## Provenance

System records:

- author;
- title;
- edition;
- publication information;
- repository;
- source URL;
- rights information;
- SHA-256.

## Processing

System:

- extracts pages;
- extracts embedded text or performs OCR;
- creates page records;
- attempts printed-page mapping;
- creates chunks.

## QA

System identifies at least:

- low extraction coverage;
- missing metadata;
- page sequence anomalies;
- duplicate files;
- unknown rights.

## Review

Admin can:

- inspect page image;
- inspect raw/normalized text;
- correct printed page;
- edit important metadata;
- resolve review issue.

## Publishing

Only approved sources can become `PUBLISHED`.

## Research answers

POC completion additionally requires hybrid retrieval, source-grounded answers, verified citations, and the evaluation gates in PRD section 58.

## Traceability

Every published chunk must resolve backwards to:

```text
chunk
↓
page
↓
edition
↓
work
↓
source record
↓
repository
↓
original asset
↓
checksum
```

---

# 42. Development Milestones

## Milestone 1 — Foundation

- monorepo;
- Docker Compose;
- Vue;
- Go HTTP API;
- PostgreSQL + pgvector;
- River queue in PostgreSQL;
- MinIO;
- migrations;
- admin authentication.

## Milestone 2 — Source Registry

- works;
- editions;
- repositories;
- source records;
- assets;
- source CRUD;
- source state machine.

## Milestone 3 — Manual Upload

- PDF upload;
- immutable object storage;
- SHA-256;
- metadata;
- processing-job creation.

## Milestone 4 — Worker / Extraction

- Go/River worker;
- PDF inspection;
- page extraction;
- text extraction;
- OCR provider abstraction;
- normalization.

## Milestone 5 — Page Mapping + Chunking

- printed page detection;
- sequence validation;
- page mapping;
- conservative structure detection;
- page-safe chunks.

## Milestone 6 — Review

- QA engine;
- review issues;
- review queue;
- page viewer;
- page/metadata correction.

## Milestone 7 — Trusted Repository Connector

- search;
- preview;
- normalize metadata;
- import;
- acquire source asset.

## Milestone 8 — Approval / Publishing

- approval gates;
- publish;
- disable;
- reprocessing;
- audit history.

Milestone 8 completes ingestion only. Required milestones 9–11 add pgvector/FTS indexing, citation-grounded Q&A, and end-to-end evaluation; see PRD sections 48 and 58.

---

# 43. POC — Hybrid Retrieval

Embedding creation is required and starts automatically after publication, with resumable batches and visible “Preparing for questions” progress. See PRD section 59 for the complete workflow and readiness gates.

After the ingestion milestone works, build within the same POC:

```text
PostgreSQL full-text search
+
pgvector semantic search
+
metadata filtering
+
RRF fusion
+
reranking
```

Explicit author/source constraints must override semantic relevance.

Example:

> “What does Hering say…”

must restrict retrieval to Hering rather than returning another author merely because their passage scores better semantically.

---

# 44. POC Query Understanding

Within the POC, parse each question into a structured representation such as:

```json
{
  "question_type": "source_specific",
  "authors": ["Hering"],
  "works": [],
  "remedies": ["Nux vomica"],
  "topics": ["constipation"],
  "explicit_source_constraint": true
}
```

Initial categories:

```text
remedy
source_specific
topic
comparison
negative_or_uncertain
```

---

# 45. POC Retrieval Pipeline

Target:

```text
Question
   ↓
Query parser
   ↓
Metadata constraints
   ↓
┌────────────────┐       ┌────────────────┐
│ Lexical Search │       │ Vector Search  │
│ top N          │       │ top N          │
└───────┬────────┘       └───────┬────────┘
        │                        │
        └──────────┬─────────────┘
                   ↓
             RRF Fusion
                   ↓
              Reranker
                   ↓
             Evidence Set
```

Hybrid retrieval is important because classical literature contains:

- exact remedy names;
- abbreviations;
- old spellings;
- unusual phrases;
- rubric terminology;
- source-specific wording.

---

# 46. POC Evidence Pack

The LLM should receive labeled evidence, not anonymous passages.

Example:

```text
[E1]

Author:
Constantine Hering

Work:
The Guiding Symptoms of Our Materia Medica

Volume:
...

Printed page:
...

Source type:
Classical materia medica

Passage:
"..."
```

---

# 47. POC Citation Mechanism

Critical rule:

> **The LLM must not invent bibliographic citations.**

The LLM should output only internal evidence IDs:

```text
Hering describes ... [[E1]]
```

The backend resolves:

```text
E1
→ chunk
→ page
→ edition
→ work
→ repository
```

The citation displayed to the user is generated from trusted database records.

---

# 48. POC Citation Validation

Two layers:

## Structural

Deterministic checks:

- evidence ID exists;
- chunk exists;
- page exists;
- edition exists;
- source metadata exists;
- no fabricated evidence IDs.

## Support validation

Check whether:

```text
claim → cited passage
```

is actually supported.

Unsupported claims should be:

- removed;
- regenerated once;
- or explicitly marked as insufficient evidence.

---

# 49. POC Source Viewer

Expected UX:

```text
┌───────────────────────────┬──────────────────────────────┐
│ ANSWER                    │ ORIGINAL SOURCE              │
│                           │                              │
│ Hering describes... [1]  │ Guiding Symptoms            │
│                           │ Volume ...                   │
│ Evidence                  │ Page ...                     │
│ [1] Hering, p...          │ scanned page                │
│                           │ original passage             │
└───────────────────────────┴──────────────────────────────┘
```

The quoted source passage must come directly from stored source text/page data, not be regenerated by the LLM.

---

# 50. POC Evaluation Dataset

Start with 30 manually reviewed questions as specified in PRD section 58; expand toward 100 after the initial evaluation.

Categories:

- source-specific;
- remedy description;
- symptom/topic;
- comparisons;
- exact terminology/spelling;
- multi-source questions;
- negative/insufficient evidence;
- later research questions.

Each should have:

```text
question
question_type
expected_sources
gold_page_ids
gold_passage_ids
acceptable_supporting_pages
expected_answer_notes
should_abstain
difficulty
reviewer_notes
```

Evaluate retrieval independently from answer generation.

---

# 51. Long-Term Research Integrations

Only after classical retrieval/citations are reliable should the system add:

- PubMed
- PMC
- Europe PMC
- ClinicalTrials.gov
- CTRI
- CCRH
- IJRH
- AYUSH Research Portal
- other appropriate journals/databases

These should use the same source/provenance principles.

---

# 52. Current Priority

Build the local question interface after ingestion, review, and indexing; it is required for POC completion.

The next engineering goal is:

> **Successfully ingest one real trusted classical source from repository/file through the complete processing + review workflow while retaining source → edition → page → chunk traceability.**

Then ingest a second source.

Then add the first automated repository connector.

Then implement hybrid retrieval and citation-grounded answers, evaluate them, and freeze the end-to-end POC baseline.

---

# 53. Exact Immediate Development Order

Start development in this order:

```text
1. Create monorepo
2. Add Docker Compose
3. Scaffold Vue 3 + TypeScript
4. Scaffold Go HTTP API
5. Configure PostgreSQL + pgvector + Goose
6. Configure PostgreSQL + River
7. Configure MinIO
8. Implement repository/work/edition/source-asset models
9. Implement source CRUD
10. Implement manual PDF ingestion
11. Implement processing jobs
12. Implement page extraction
13. Implement source/page review UI
14. Implement printed-page mapping
15. Implement page-safe chunk generation
16. Implement QA engine
17. Implement first trusted repository connector
18. Implement approval/publishing
19. Validate with 2–3 real classical works
20. Index approved sources with embeddings + full-text search
21. Implement hybrid retrieval and evidence packs
22. Implement answer synthesis, citation checks, and source viewer
23. Evaluate retrieval, answer support, abstention, and ARM performance
24. Freeze end-to-end POC baseline
```

Ingestion, retrieval, and citation-grounded Q&A are stages of one POC. They are not deferred to separate future POCs.

---

# 54. Instructions for Codex / Astra

When continuing this project:

1. Do not redesign the product from scratch.
2. Treat source provenance and page-level traceability as hard requirements.
3. Keep the architecture modular but avoid unnecessary microservices.
4. Frontend must use Vue 3 + TypeScript.
5. Backend must use Go, as requested by the user on 2026-10-05. Library choices remain recommendations until discussed.
6. Start with manual PDF ingestion before implementing external connectors.
7. Use one codebase with separate API and background worker runtime processes.
8. Preserve original source assets unchanged.
9. Derived data must be regeneratable.
10. Never infer unknown bibliographic or rights metadata.
11. Never allow unapproved sources into production retrieval.
12. Keep the POC RAG/citation requirements in mind while designing schema today.
13. Do not add prescribing or clinical workflows in this POC.
14. Prefer working code and incremental vertical slices over speculative architecture.

---

# 55. Recommended First Codex Task

The next concrete coding task should be:

> Scaffold the POC repository with Vue 3 + TypeScript frontend, Go HTTP API backend, PostgreSQL + pgvector, River, and MinIO using Docker Compose. Implement the initial SQL schema migrations and sqlc queries for repositories, works, editions, sources, source records/assets, processing revisions, ingestion jobs, pages, chunks, review issues, rights decisions, publications, and audit logs. Include authentication and server-side role checks. Then implement a vertical slice for manual PDF upload: create the source/edition metadata, store the immutable file in MinIO, compute SHA-256, create an ingestion job, and expose processing status in the Vue admin UI.

Do not implement external source connectors until the manual vertical slice works end-to-end.

---

# 56. Product Principle to Preserve

The durable product asset is:

> **curated knowledge + exact provenance + page-level evidence**

not the current LLM.

The system should ultimately feel like:

> **a research tool that happens to speak conversationally**

rather than:

> a chatbot that happens to cite things.

---

## Current implementation checkpoint — 2026-10-07

The running Compose stack has manual PDF upload, HTTPS PDF import, Internet Archive discovery, DOI reference/import when an eligible PDF is available, automatic page QA, review and publication, local hybrid retrieval, answer jobs/Activity, saved citations, and an evidence-first claim pipeline with stored excerpt offsets and decisions. Browser sign-in now requires a configured administrator or reviewer token; the browser proxy no longer grants the administrator token to every user. The API records named reviewers/publishers and isolates reviewer answer Activity. A separate reviewer token was generated into the local `.env` for testing. See the README for configuration.

One published 2023 Hamre et al. paper was reprocessed from saved PDF bytes, reviewed, published, and indexed to READY with 270 passages. Old and new citations both resolve after the switch. The implementation still uses a linked candidate `sources` row, so the PRD's strict stable source identity with multiple revisions is open. Publication approval currently derives from the latest rights decision; a separate revision/content approval gate is still needed. The evaluation worksheet has not yet been human reviewed or scored, and the browser reconnect banner needs an end-to-end check. Do not report the full POC as accepted until those gates pass. The PRD section 63.15 has the exact verification record.

# END OF CODEX / ASTRA HANDOFF
