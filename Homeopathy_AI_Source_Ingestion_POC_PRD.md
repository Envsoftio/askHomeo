# Homeopathy AI
# Product Requirements Document — Source Ingestion to Citation-Grounded Answers

**Version:** 1.5  
**Status:** Reviewed POC specification; library choices proposed for discussion  
**Primary phase:** End-to-end POC — Source Ingestion to Citation-Grounded Answers  
**Frontend:** Vue 3 + TypeScript  
**Backend:** Go (confirmed)  
**Architecture:** Modular monolith + separate background worker  
**Database:** PostgreSQL + pgvector  
**Queue:** PostgreSQL + River (proposed)  
**Object storage:** MinIO locally / S3-compatible storage later  
**POC retrieval:** PostgreSQL Full Text Search + pgvector

**Scope correction — 2026-10-05:** The user requires an end-to-end POC on an Apple Silicon (ARM) Mac: source ingestion → OCR/review → indexing with PostgreSQL FTS + pgvector → questions → researched answers with verifiable citations. Ingestion is the first milestone, not the complete POC. PRD section 58 defines the complete scope and ARM inference options.

**Review update — 2026-10-05:** Go replaces the previous Python API/worker choice at the user's request. shadcn-vue and the supporting Go libraries are recommendations pending discussion. The [POC PRD](Homeopathy_AI_Source_Ingestion_POC_PRD.md) is authoritative for implementation details; its section 56 defines revision, publication, and verification requirements. The companion handoff preserves product context and future direction.

**Literature research update — 2026-10-10:** Product roadmap section 42 and this PRD section 64 define source categories and structured repertory/materia medica research. Deliver classification and filters in ING-02, then ING-06 before broader collection. These are requirements, not implemented capabilities.

---

# 1. Executive Summary

Homeopathy AI is a planned AI research and clinical-reference product for homeopathic practitioners and students.

This POC allows users to ask natural-language questions about imported, reviewed sources and receive synthesized answers linked to exact supporting passages. Research coverage is limited to the indexed corpus; broader clinical research integrations remain later work.

The first implementation milestone is ingestion; the complete deliverable includes the research question interface and citation-grounded answer engine.

The first POC must prove that authentic homeopathic literature can be acquired, normalized, processed, reviewed, and published into a trustworthy internal corpus while preserving exact provenance down to the edition, source asset, page, and chunk.

POC-1 therefore delivers a:

> **Trusted Source Ingestion + Hybrid Retrieval + Citation-Grounded Research Assistant**

The system will support:

- trusted-source discovery from approved repositories;
- manual PDF upload;
- bibliographic/provenance metadata;
- immutable source-file storage;
- SHA-256 verification;
- PDF/text extraction;
- OCR fallback;
- printed-page mapping;
- structure/remedy-heading detection;
- page-safe chunking;
- automatic QA;
- human review;
- approval;
- controlled publishing;
- local embeddings and pgvector + full-text retrieval;
- source-grounded answer synthesis with claim-level citations;
- citation-linked original-page viewing and insufficient-evidence responses.

Published sources with a ready index are searchable by the POC answer engine.

---

# 2. Problem Statement

A citation-grounded AI product is only as trustworthy as its source corpus.

Simply collecting PDFs, creating embeddings, and allowing an LLM to answer introduces several failure modes:

- wrong edition;
- uncertain provenance;
- incorrect page numbers;
- poor OCR;
- duplicated documents;
- missing volumes;
- mixed source types;
- rights uncertainty;
- fabricated citations;
- inability to reconstruct the original evidence.

The product therefore needs a dedicated ingestion foundation before retrieval and answer generation.

---

# 3. POC Goal

The POC must demonstrate this complete workflow:

```text
Trusted source / PDF
        ↓
Source record
        ↓
Original asset stored
        ↓
Provenance recorded
        ↓
Pages extracted
        ↓
Text extracted or OCR performed
        ↓
Printed pages mapped
        ↓
Text normalized
        ↓
Chunks created
        ↓
Automated QA
        ↓
Human review
        ↓
Approval
        ↓
Publication
        ↓
Embedding + full-text indexing
        ↓
Question → hybrid retrieval → evidence pack
        ↓
Answer synthesis → citation validation
        ↓
Answer + citations + original-page viewer
```

For every published chunk, the system must support reverse traceability:

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
SHA-256
```

---

# 4. POC Success Definition

POC-1 is successful when:

1. One source can be uploaded manually.
2. One source can be imported from one approved external repository connector.
3. Both paths enter the same downstream ingestion pipeline.
4. Original source assets are immutable.
5. The system extracts page-level text.
6. OCR is used when required.
7. Printed page numbering can be reviewed/corrected.
8. Page-safe chunks are generated.
9. Automatic QA detects common ingestion problems.
10. A reviewer can inspect source/page issues.
11. Only approved sources can become `PUBLISHED`.
12. Every published chunk retains exact provenance.
13. PostgreSQL FTS and pgvector retrieve evidence for questions over the approved corpus.
14. Answers cite supporting passages and open the correct original page.
15. Insufficient evidence produces an explicit limitation or abstention, not an unsupported answer.
16. The full local workflow passes the evaluation gates in section 58.

---

# 5. Non-Goals

POC-1 does not include:

- public internet deployment (the local research question interface is required);
- remedy recommendation;
- prescribing;
- patient records;
- diagnosis;
- repertorization;
- case management;
- constitutional analysis;
- research synthesis agents;
- PubMed ingestion;
- CCRH ingestion;
- CTRI ingestion;
- clinical-trial evaluation;
- standalone vector-database administration UI;
- deep research mode;
- graph database;
- complex knowledge graph;
- many independent microservices;
- multi-tenant billing;
- mobile apps.

---

# 6. Product Principles

## 6.1 Evidence First

Source integrity is more important than fluent AI output.

## 6.2 Provenance Is First-Class

Metadata and source identity must survive all downstream processing.

## 6.3 Original Assets Are Immutable

Never modify imported source files.

## 6.4 Derived Data Is Regeneratable

OCR, normalized text, chunks, embeddings, and indexes can be rebuilt.

## 6.5 Unknown Means Unknown

Never invent missing metadata.

## 6.6 Publication Is Controlled

Automated discovery/import must not equal automatic inclusion in the production corpus.

## 6.7 Modular, Not Over-Distributed

Use clean module boundaries inside a shared backend codebase and split runtime processes only when technically justified.

---

# 7. Intended Users

The POC includes internal source administration and an authenticated research question interface. ADMIN and REVIEWER users can ask questions; public account onboarding is out of scope.

Primary roles:

## ADMIN

Can:

- configure trusted repositories;
- create/edit works and editions;
- discover/import sources;
- upload PDFs;
- retry ingestion;
- edit metadata;
- resolve review issues;
- approve/reject;
- publish/disable sources.

## REVIEWER

Can:

- inspect sources;
- inspect pages;
- review OCR/extracted text;
- correct page mapping;
- resolve assigned issues;
- mark pages reviewed;
- approve review items where permissions allow.

---

# 8. Initial Corpus Strategy

**Starter pack prepared:** [Ingestion guide](data/starter-corpus/README.md), [machine-readable manifest](data/starter-corpus/manifest.json), and [offline verifier](scripts/verify_starter_corpus.py). Two original PDFs and provenance snapshots are present locally. Start with Nash (1899), then Farrington (1890, second edition). See section 62 for verified details and remaining publication checks.

Do not import thousands of works initially.

Target Classical Corpus V1 later:

- Hahnemann — Organon of Medicine
- Hahnemann — Materia Medica Pura
- Hahnemann — Chronic Diseases
- Hering — Guiding Symptoms of Our Materia Medica
- Kent — Repertory of the Homoeopathic Materia Medica
- Clarke — A Dictionary of Practical Materia Medica
- Farrington — A Clinical Materia Medica
- Boericke — Pocket Manual of Homoeopathic Materia Medica
- Nash — Leaders in Homoeopathic Therapeutics

## 8.1 Classical book acquisition register (checked 2026-10-07)

These are **candidate source files**, not published/READY app sources. A catalogue record describes an edition; a DOI or catalogue link alone supplies no page evidence. Download each scan, record its source URL, edition, rights statement, retrieval date, byte size and SHA-256, then run page/OCR QA and the normal rights and publication workflow. Keep each volume as a separately identified asset with its own printed-page map. Repository availability and rights can change; recheck them before reuse, especially for client-facing distribution. The Wellcome items below currently show a Public Domain Mark; Internet Archive's “possible copyright status” is a repository indication, not a legal determination for every jurisdiction.

| Work and exact edition | Repository record / readable source | PDF or text acquisition | Coverage / ingestion note |
|---|---|---|---|
| Hahnemann, *Organon of Medicine*, R. E. Dudgeon translation of the **fifth** German edition (1849) | [Wellcome record](https://wellcomecollection.org/works/efq8ru6u) | [PDF](https://iiif.wellcomecollection.org/pdf/b33290192) | Do not label this as the sixth edition. |
| Hahnemann, *Materia Medica Pura*, Dudgeon/Hughes translation (1880–1881) | [Wellcome record](https://wellcomecollection.org/works/ukrrfv6p) | [Volume 1 PDF](https://iiif.wellcomecollection.org/pdf/b28121612_0002); [volume 2 PDF](https://iiif.wellcomecollection.org/pdf/b28121612_0001) | The repository's IIIF identifiers are **not** in volume-number order; retain the labels shown here. |
| Hahnemann, *Chronic Diseases*, C. J. Hempel translation (1845–1846) | [Wellcome record](https://wellcomecollection.org/works/y22eqanb) | [Vol. 1](https://iiif.wellcomecollection.org/pdf/b29326515_0001), [vol. 2](https://iiif.wellcomecollection.org/pdf/b29326515_0002), [vol. 3](https://iiif.wellcomecollection.org/pdf/b29326515_0003), [vol. 5](https://iiif.wellcomecollection.org/pdf/b29326515_0005) PDFs | **Vol. 4 is absent from this digitized copy.** Mark the series incomplete until another edition-matched scan is found. |
| Hering, *The Guiding Symptoms of Our Materia Medica* (1879–1891) | [Wellcome record](https://wellcomecollection.org/works/buvsvkee) | [Digitized item PDF](https://iiif.wellcomecollection.org/pdf/b21058386) | Catalogue describes a ten-volume series, but this linked item has 538 scans. Identify its volume from the title page; do not treat it as the complete set. |
| Kent, *Repertory of the Homoeopathic Materia Medica* (1897–1899) | [Wellcome record](https://wellcomecollection.org/works/aguw48nw) | [Volume 1 PDF](https://iiif.wellcomecollection.org/pdf/b20416234_001); [volume 2 PDF](https://iiif.wellcomecollection.org/pdf/b20416234_002) | Original issue in parts, bound as two volumes. |
| Clarke, *A Dictionary of Practical Materia Medica* (1900–1902) | [University of Pennsylvania's volume guide](https://onlinebooks.library.upenn.edu/webbin/book/lookupid?key=olbp78922) | [Vol. I A–H PDF](https://archive.org/download/adictionaryprac04clargoog/adictionaryprac04clargoog.pdf); [vol. II/1 I–Pen PDF](https://archive.org/download/adictionaryprac03clargoog/adictionaryprac03clargoog.pdf); [vol. II/2 Pen–Z PDF](https://archive.org/download/adictionaryprac01unkngoog/adictionaryprac01unkngoog.pdf) | Use [the last volume's item page](https://archive.org/details/adictionaryprac01unkngoog) if its direct PDF endpoint is temporarily unavailable. Do not substitute Wellcome's 1925 issue silently: Wellcome marks that scan “In copyright.” |
| Farrington, *A Clinical Materia Medica*, second edition (1890) | [Wellcome record](https://wellcomecollection.org/works/hh24qeg4) | [PDF](https://iiif.wellcomecollection.org/pdf/b21118838) | Already downloaded and indexed; see section 62 and the starter-corpus manifest. |
| Boericke, *Pocket Manual of Homoeopathic Materia Medica*, third revised edition with Oscar E. Boericke's repertory (1906) | [Internet Archive item](https://archive.org/details/pocketmanualhom00boergoog); [Open Library edition record](https://openlibrary.org/books/OL6971411M/Pocket_manual_of_homoeopathic_materia_medica) | [Scan PDF](https://archive.org/download/pocketmanualhom00boergoog/pocketmanualhom00boergoog.pdf); [OCR text](https://archive.org/download/pocketmanualhom00boergoog/pocketmanualhom00boergoog_djvu.txt) | Use the PDF for page citations; OCR text alone requires alignment to scan pages. Do not confuse this edition with the later ninth edition. |
| Nash, *Leaders in Homoeopathic Therapeutics* (1899) | [Wellcome record](https://wellcomecollection.org/works/sh3awcy7) | [PDF](https://iiif.wellcomecollection.org/pdf/b20409497) | Already downloaded and indexed; see section 62 and the starter-corpus manifest. |

Additional historical candidates with page-addressable scans:

| Work and edition | Record | PDF | Note |
|---|---|---|---|
| Carroll Dunham, *Lectures on Materia Medica* (1878) | [Wellcome record](https://wellcomecollection.org/works/n898yxk3) | [PDF](https://iiif.wellcomecollection.org/pdf/b2105020x) | 419-page work; Wellcome Public Domain Mark. |
| W. A. Dewey, *Essentials of Homoeopathic Therapeutics* (1895) | [Wellcome record](https://wellcomecollection.org/works/kwgr5kjj) | [PDF](https://iiif.wellcomecollection.org/pdf/b21114742) | 266-page work; Wellcome Public Domain Mark. |

To download a linked PDF manually, open the PDF link in a browser and save it, or from a terminal run `curl -L --fail --output organon-1849.pdf 'https://iiif.wellcomecollection.org/pdf/b33290192'`. Confirm that the saved file opens and that its title page matches the listed edition. For Internet Archive items, use **Download Options → PDF** on the item page if a direct link fails. Prefer the original scan for immutable citations; use repository OCR/plain text only after matching it to scan pages. None of these candidates becomes answer evidence until ingestion, automatic page QA, rights approval, publication and indexing complete.

POC validation target:

```text
2–3 real works
1 automated trusted-source connector
1 manual PDF upload path
```

---

# 9. Trusted Repository Model

Initial repository candidates:

- Wellcome Collection
- NLM Digital Collections
- University of Michigan Homeopathy Collection
- Internet Archive / Medical Heritage Library
- Library of Congress
- Gallica
- HathiTrust

Only one connector is required for POC-1.

Repository entity:

```text
repository_id
name
domain
repository_type
trust_tier
connector_type
metadata_supported
ocr_supported
page_images_supported
rights_metadata_supported
base_url
terms_url
enabled
created_at
updated_at
```

Connector types:

```text
api
iiif
direct_download
custom_adapter
manual_only
```

---

# 10. Source Connector Contract

Repository adapters must normalize remote records into a common internal shape.

Conceptual interface:

```go
type SourceConnector interface {
    Search(ctx context.Context, query SearchQuery) ([]SourceRecord, error)
    GetRecord(ctx context.Context, externalID string) (SourceRecord, error)
    GetMetadata(ctx context.Context, externalID string) (SourceMetadata, error)
    GetFiles(ctx context.Context, externalID string) ([]RemoteFile, error)
    GetRights(ctx context.Context, externalID string) (RightsStatement, error)
}
```

Normalized record example:

```json
{
  "repository": "wellcome",
  "external_id": "abc123",
  "title": "Example title",
  "author": "Example author",
  "year": 1891,
  "edition": "Example edition",
  "volume": "Volume 1",
  "language": "English",
  "source_url": "https://...",
  "document_url": "https://...",
  "ocr_url": null,
  "rights_statement": "...",
  "identifiers": {}
}
```

Connector code must not write directly to final domain tables.

Connector output goes through the application/ingestion layer.

---

# 11. Architecture

## 11.1 Architecture Style

Use:

> **Modular monolith + independent worker process**

Do not build a single tightly coupled codebase with all logic in controllers.

Do not build many independently deployed microservices.

Use one monorepo with one backend domain model.

Run:

- frontend;
- API;
- worker;
- PostgreSQL + pgvector;
- MinIO;

as separate runtime containers.

---

# 12. Runtime Architecture

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

# 13. Technology Stack

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

# 14. Monorepo Layout

```text
homeopathy-ai/
│
├── apps/
│   ├── frontend/
│   └── backend/
│
├── infrastructure/
│   ├── docker/
│   └── compose/
│
├── docs/
│   ├── architecture/
│   ├── source-policy/
│   └── api/
│
├── docker-compose.yml
├── .env.example
└── README.md
```

Backend:

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

Frontend:

```text
apps/frontend/
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

---

# 15. Domain Model

Canonical hierarchy:

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

Intellectual work independent of edition.

## Edition

Specific publication/volume.

## Source Record

External repository catalog record.

## Source Asset

Imported/downloaded file.

## Page

Physical/scan page extracted from source asset.

## Chunk

Retrieval unit tied to one page.

---

# 16. Database Schema Requirements

These are conceptual field lists. Section 56.2 adds required source/revision/publication entities and constraints; section 56.3 defines asset-scoped rights decisions. Implement them together. Use timezone-aware timestamps for instants.

## 16.1 `repositories`

```text
id UUID PK
name TEXT
domain TEXT
repository_type TEXT
trust_tier INT
connector_type TEXT
metadata_supported BOOLEAN
ocr_supported BOOLEAN
page_images_supported BOOLEAN
rights_metadata_supported BOOLEAN
base_url TEXT
terms_url TEXT
enabled BOOLEAN
created_at TIMESTAMP
updated_at TIMESTAMP
```

## 16.2 `works`

```text
id UUID PK
canonical_title TEXT
author_name TEXT
author_normalized TEXT
original_language TEXT
work_type TEXT
first_publication_year INT NULL
description TEXT NULL
created_at TIMESTAMP
updated_at TIMESTAMP
```

Allowed `work_type` initially:

```text
materia_medica
repertory
organon
therapeutics
proving
clinical_reference
other
```

## 16.3 `editions`

```text
id UUID PK
work_id UUID FK
title_as_printed TEXT
edition_statement TEXT NULL
publisher TEXT NULL
publication_place TEXT NULL
publication_year INT NULL
volume_label TEXT NULL
language TEXT
source_type TEXT
source_tier INT
rights_status TEXT
rights_statement_raw TEXT NULL
rights_source TEXT NULL
rights_jurisdiction TEXT NULL
rights_review_status TEXT
rights_checked_at TIMESTAMP NULL
ingestion_status TEXT
created_at TIMESTAMP
updated_at TIMESTAMP
```

## 16.4 `source_records`

```text
id UUID PK
edition_id UUID FK
repository_id UUID FK NULL
external_id TEXT NULL
source_url TEXT NULL
original_scan_url TEXT NULL
metadata_json JSONB
accessed_at TIMESTAMP
is_preferred_source BOOLEAN
created_at TIMESTAMP
```

## 16.5 `source_assets`

```text
id UUID PK
edition_id UUID FK
source_record_id UUID FK NULL
asset_type TEXT
storage_path TEXT
original_url TEXT NULL
mime_type TEXT
file_size BIGINT
sha256 TEXT
downloaded_at TIMESTAMP
created_at TIMESTAMP
```

Allowed asset types:

```text
pdf
ocr
page_image
metadata
other
```

## 16.6 `pages`

```text
id UUID PK
edition_id UUID FK
source_asset_id UUID FK
scan_page_index INT
pdf_page_index INT
printed_page_label TEXT NULL
printed_page_number INT NULL
chapter TEXT NULL
section TEXT NULL
remedy_heading TEXT NULL
text_raw TEXT NULL
text_normalized TEXT NULL
extraction_method TEXT
ocr_confidence FLOAT NULL
page_image_path TEXT NULL
source_page_url TEXT NULL
review_status TEXT
created_at TIMESTAMP
updated_at TIMESTAMP
```

## 16.7 `chunks`

```text
id UUID PK
edition_id UUID FK
page_id UUID FK
chunk_index INT
text_exact TEXT
text_search TEXT
start_character INT NULL
end_character INT NULL
heading_path TEXT NULL
remedy_normalized TEXT NULL
chapter_normalized TEXT NULL
token_count INT
source_type TEXT
language TEXT
created_at TIMESTAMP
```

## 16.8 `ingestion_jobs`

```text
id UUID PK
edition_id UUID FK
job_type TEXT
status TEXT
current_step TEXT
progress_percent INT
started_at TIMESTAMP NULL
completed_at TIMESTAMP NULL
error_code TEXT NULL
error_message TEXT NULL
retry_count INT DEFAULT 0
metadata_json JSONB
created_at TIMESTAMP
updated_at TIMESTAMP
```

## 16.9 `review_issues`

```text
id UUID PK
edition_id UUID FK
page_id UUID FK NULL
issue_type TEXT
severity TEXT
message TEXT
detected_by TEXT
status TEXT
resolution_note TEXT NULL
resolved_by UUID NULL
resolved_at TIMESTAMP NULL
created_at TIMESTAMP
```

Issue types:

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

## 16.10 `remedy_aliases`

```text
id UUID PK
canonical_name TEXT
alias TEXT
alias_type TEXT
language TEXT
verified BOOLEAN
created_at TIMESTAMP
```

## 16.11 `audit_logs`

```text
id UUID PK
actor_id UUID NULL
action TEXT
entity_type TEXT
entity_id UUID
before_json JSONB NULL
after_json JSONB NULL
created_at TIMESTAMP
```

---

# 17. Rights Status Model

Allowed values:

```text
public_domain
licensed
restricted
needs_review
unknown
```

The system must also store raw rights statements and source.

Rights uncertainty must create a blocking publication issue; see section 56.3.

The application must never infer commercial reuse permission solely from online availability.

---

# 18. Source Lifecycle State Machine

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

Error/alternate states:

```text
PROCESSING_FAILED
REJECTED
DISABLED
```

Rules:

- `PUBLISHED` requires `APPROVED`.
- `APPROVED` requires no unresolved blocking review issues.
- `DISABLED` sources must not participate in later production retrieval.
- reprocessing must not modify the original source asset.

---

# 19. Manual Upload Flow

## User flow

```text
Sources
→ Add Source
→ Upload PDF
```

Required metadata:

```text
Author
Title
Edition statement
Publication year
Volume
Language
Source type
Original source URL if available
Repository if known
Rights status
Rights statement
```

Processing:

```text
upload
↓
validate file
↓
store immutable asset
↓
SHA-256
↓
create work/edition/source asset
↓
create ingestion job
↓
worker pipeline
```

Manual upload and connector import must converge into the same processing pipeline.

---

# 20. Trusted Repository Discovery Flow

```text
Sources
→ Discover Sources
→ Choose repository
→ Search
→ Review normalized results
→ Import selected source
```

Result card/table should display:

```text
Title
Author
Year
Edition
Volume
Repository
Rights summary
Available assets
```

Admin chooses `Import`.

No bulk auto-publication in POC-1.

---

# 21. Duplicate Detection

## Exact duplicate

Compare:

```text
SHA-256
```

## Probable edition duplicate

Compare normalized:

```text
author
title
edition
publication_year
publisher
volume
```

## Work duplicate

Compare:

```text
canonical author
canonical title
```

Behavior:

- identical asset: reject or reuse existing asset;
- probable edition duplicate: flag for review;
- same work/different edition: allowed.

---

# 22. Ingestion Pipeline

```text
01 Acquire document
02 Validate file
03 Generate SHA-256
04 Store immutable asset
05 Inspect PDF
06 Extract embedded text
07 Determine OCR requirement
08 Create page records
09 Extract/OCR each page
10 Normalize text
11 Detect printed page labels/numbers
12 Analyze page-number sequence
13 Detect headings
14 Detect likely remedy headings
15 Generate page-safe chunks
16 Run automatic QA
17 Create review issues
18 Set source to NEEDS_REVIEW
```

Embedding generation is a required downstream indexing stage in this end-to-end POC.

---

# 23. PDF / OCR Rules

Prefer existing embedded text when:

- text exists;
- extraction quality passes basic heuristics.

Trigger OCR when:

- no text layer exists;
- extracted text is near-empty;
- extracted text is clearly corrupted.

OCR implementation must be behind an interface.

Each page must store:

```text
extraction_method
ocr_confidence
```

Possible methods:

```text
embedded_text
repository_ocr
local_ocr
external_ocr
manual
```

---

# 24. Text Preservation

Store:

```text
text_raw
```

and:

```text
text_normalized
```

separately.

Normalization may include:

- Unicode normalization;
- whitespace normalization;
- control-character removal;
- conservative dehyphenation;
- line-break cleanup;
- repetitive header/footer handling.

Do not overwrite `text_raw`.

---

# 25. Page Mapping

The system must distinguish:

```text
scan_page_index
pdf_page_index
printed_page_label
printed_page_number
```

Automatic mapping should use detected printed page numbers plus sequence analysis.

Example:

```text
PDF 37 → 21
PDF 38 → 22
PDF 39 → 23
PDF 40 → 24
```

System may propose:

```text
offset: +16
confidence: high
```

Reviewer can accept/correct.

Do not automatically overwrite uncertain values.

---

# 26. Page Sequence QA

Detect anomalies such as:

```text
21
22
23
25
```

Possible missing printed page 24.

Detect likely recognition errors:

```text
55
56
12
58
```

Create review issues rather than silently repairing uncertain data.

---

# 27. Structure Detection

POC structure extraction is enrichment only.

Attempt:

- chapter heading;
- section heading;
- remedy heading.

Each detection should be associated with confidence where practical.

Incorrect structure detection must not invalidate source provenance.

---

# 28. Chunking

Rules:

```text
target size:
250–450 words

overlap:
50–80 words

hard boundary:
one physical page

preferred boundaries:
paragraph
section
remedy heading
```

Each chunk must contain:

```text
page_id
edition_id
text_exact
text_search
```

`text_exact` is the evidence representation.

`text_search` is a search-friendly representation.

Do not insert generated metadata into `text_exact`.

---

# 29. Automated QA Requirements

Minimum checks:

1. File opens successfully.
2. Page count > 0.
3. SHA-256 exists.
4. Author exists or issue created.
5. Title exists or issue created.
6. Publication metadata is present or flagged.
7. Rights status exists or issue created.
8. Text extraction coverage calculated.
9. OCR coverage calculated.
10. Empty/failed page extraction identified.
11. Page-number sequence analyzed.
12. Missing-page anomalies flagged.
13. Duplicate asset detection performed.
14. Page records created.
15. Chunk records created.

QA result categories:

```text
PASS
WARNING
FAIL
```

---

# 30. Blocking vs Non-Blocking Issues

Blocking examples:

```text
file_corrupt
wrong_author
wrong_work
missing_provenance
critical_extraction_failure
explicit_rights_restriction
```

Non-blocking warning examples:

```text
some_low_confidence_ocr
uncertain_section_heading
minor_page_number_gap
missing_optional_metadata
```

An admin may resolve/acknowledge non-blocking issues.

Blocking issues must prevent approval.

---

# 31. Page Review Screen

Required UI:

```text
┌────────────────────────┬─────────────────────────┐
│ Original Page          │ Extracted Text          │
│                        │                         │
│ image/PDF rendering    │ raw / normalized tabs   │
│                        │                         │
└────────────────────────┴─────────────────────────┘

Printed Page:   [421]
Chapter:        [...]
Section:        [...]
Remedy:         [...]

Extraction:     OCR / Embedded
Confidence:     ...

Open Issues:
...

[Save]
[Mark Reviewed]

[Previous] [Next]
```

The user must be able to move between pages quickly.

---

# 32. Source Detail Screen

Section 60 defines the user-facing labels and layout. Show only the current next action prominently; place technical metadata in a disclosure.

Display:

```text
Work
Edition
Author
Publisher
Year
Volume
Language
Repository
Source URL
Rights
File SHA-256
Page count
OCR percentage
Mapped-page percentage
Chunk count
Open QA issues
Current status
```

Actions:

```text
Review Pages
Resolve Issues
Approve
Reject
Reprocess
Publish
Disable
```

Show actions conditionally based on current state.

---

# 33. Dashboard

Present a compact next-action summary within Sources, not a separate large dashboard. Prioritize sources needing review and sources ready for questions; secondary counts can be collapsed.

Display:

```text
Total sources
Published
Needs review
Processing
Failed
Total pages
Open review issues
Recent ingestion activity
```

---

# 34. Sources List

Columns:

```text
Title
Author
Edition
Volume
Repository
Pages
Status
QA
Updated
```

Filters:

```text
status
author
repository
rights_status
work_type
```

Search:

```text
title
author
```

---

# 35. Review Queue

Filters:

```text
Rights
OCR
Page Mapping
Metadata
Duplicates
Structure
Processing Errors
```

Each issue should link directly to the relevant source/page.

---

# 36. Processing Status Screen

The internal stages below map to short user-facing labels in section 60. Include automatic embedding preparation from section 59 after publication.

Show pipeline steps:

```text
Downloading             Complete
File verification       Complete
Text extraction         Complete
OCR                     Complete
Page mapping            Running
Structure detection     Pending
Chunking                Pending
QA                      Pending
```

Use polling in POC.

---

# 37. API Requirements

## Source CRUD

```http
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

```http
GET /api/v1/discovery/search
```

Parameters:

```text
repository
query
author
title
year
```

## Repository Import

```http
POST /api/v1/imports/repository
```

Body:

```json
{
  "repository": "wellcome",
  "external_id": "..."
}
```

## Manual Import

```http
POST /api/v1/imports/upload
```

multipart:

```text
file
metadata
```

## Jobs

```http
GET  /api/v1/jobs
GET  /api/v1/jobs/{id}
POST /api/v1/jobs/{id}/retry
```

Example response:

```json
{
  "id": "...",
  "status": "processing",
  "current_step": "page_extraction",
  "progress_percent": 53
}
```

## Pages

```http
GET   /api/v1/sources/{source_id}/pages
GET   /api/v1/pages/{page_id}
PATCH /api/v1/pages/{page_id}
POST  /api/v1/pages/{page_id}/review
```

## Review Issues

```http
GET   /api/v1/review/issues
PATCH /api/v1/review/issues/{id}
```

## Repositories

```http
GET    /api/v1/repositories
POST   /api/v1/repositories
PATCH  /api/v1/repositories/{id}
```

---

# 38. Background Job Requirements

Jobs must be:

- restartable;
- idempotent where practical;
- progress-reporting;
- failure-aware;
- associated with source/edition.

Job steps should update:

```text
status
current_step
progress_percent
error_code
error_message
retry_count
```

A failed job must move the source into a clear failure state.

---

# 39. Reprocessing

The admin must be able to re-run:

- extraction;
- OCR;
- normalization;
- page mapping;
- structure detection;
- chunking;
- QA.

Do not re-upload the source asset for reprocessing.

Original file remains immutable.

Derived records/artifacts must be versioned. Never replace a published revision in place; see section 56.

---

# 40. Processing Versions

Track software versions for:

```text
extractor_version
ocr_version
normalizer_version
page_mapper_version
chunker_version
qa_rules_version
```

Purpose:

- reproducibility;
- regression analysis;
- future reprocessing.

---

# 41. Audit Requirements

Audit at least:

- source created/imported;
- metadata edited;
- rights edited;
- page number corrected;
- review issue resolved;
- source approved;
- source rejected;
- source published;
- source disabled;
- reprocessing triggered.

Audit record:

```text
actor
action
entity
before
after
timestamp
```

---

# 42. Authentication and Authorization

POC can use simple credentials/session/JWT.

Minimum roles:

```text
ADMIN
REVIEWER
```

All admin APIs require authentication.

Authorization must be enforced server-side.

---

# 43. Security Requirements

- PDF/file-type validation
- upload-size limits
- safe filename handling
- path traversal prevention
- no executable handling
- malware-scan integration hook
- source assets outside web root
- signed/authorized file URLs
- API authorization
- environment secret management
- parameterized database access
- rate limiting on discovery/import
- external download timeouts
- external file-size limits
- external URL allowlisting through repository connectors

---

# 44. Observability

Structured job logs:

```text
job_id
source_id
edition_id
step
duration_ms
status
```

Metrics:

```text
ingestion_duration
page_count
ocr_page_count
failed_page_count
qa_warning_count
qa_failure_count
worker_failure_count
```

Full APM is optional for POC.

---

# 45. Testing

## Unit

- author normalization
- title normalization
- metadata validation
- source-state transitions
- checksum generation
- page-number sequence detection
- chunk page-boundary enforcement
- rights validation

## Integration

- manual upload → MinIO
- source asset → page records
- page extraction → normalized text
- pages → chunks
- QA → review issues
- connector → normalized record
- publication gate

## End-to-End

```text
discover/upload
→ import
→ process
→ review
→ approve
→ publish
```

---

# 46. Test Fixtures

Include small fixtures for:

- text-native PDF;
- scanned PDF;
- front matter with Roman numerals;
- PDF page offset;
- missing page;
- duplicate file;
- low-text page;
- intentionally corrupted PDF.

Do not use full books in every CI test.

---

# 47. POC Acceptance Criteria

## Ingestion

- manual PDF upload works;
- one trusted connector import works.

## Provenance

System stores:

- author;
- title;
- edition;
- publication info;
- repository;
- source URL;
- rights;
- immutable asset;
- SHA-256.

## Processing

System:

- extracts pages;
- extracts/OCRs text;
- stores raw/normalized text;
- attempts printed-page mapping;
- creates page-safe chunks.

## QA

System can flag:

- extraction problems;
- unknown rights;
- duplicate assets;
- page sequence anomalies;
- missing required metadata.

## Review

Reviewer can:

- inspect source page;
- inspect extracted text;
- correct printed page;
- update important metadata;
- resolve issues.

## Publishing

- blocking issues prevent approval;
- only approved sources may publish;
- disabled sources are excluded later.

## Traceability

Every published chunk resolves to exact original asset + checksum.

## Research answers

The question UI, hybrid retrieval, synthesized answers, citation viewer, and evaluation gates in section 58 are required for POC acceptance.

---

# 48. Development Milestones

## M1 — Foundation

Include admin authentication and server-side role checks in the initial vertical slice.

- monorepo
- Docker Compose
- Vue scaffold
- Go HTTP API scaffold
- PostgreSQL + pgvector
- Goose SQL migrations
- River queue in PostgreSQL
- Go/River worker
- MinIO
- health checks
- `.env.example`

## M2 — Domain / Source Registry

- repository model
- work model
- edition model
- source record model
- source asset model
- state machine
- source CRUD
- initial Vue Sources UI

## M3 — Manual PDF Vertical Slice

- create source metadata
- upload PDF
- validate
- store in MinIO
- SHA-256
- create source asset
- create ingestion job
- processing status screen

This is the first critical milestone.

## M4 — Page Extraction

- inspect PDF
- create page rows
- extract embedded text
- OCR abstraction
- raw text
- normalized text

## M5 — Page Mapping / Chunking

- printed-page detection
- page-sequence analysis
- manual correction
- structure detection
- chunk generation

## M6 — QA / Review

- QA rule engine
- review issue model
- review queue
- page viewer
- page metadata correction

## M7 — First Repository Connector

- repository search
- record preview
- normalized metadata
- file acquisition
- rights metadata import
- route into common ingestion pipeline

## M8 — Approval / Publishing

- approval gates
- publish
- disable
- audit log
- reprocessing
- ingestion milestone demo

---

## M9 — Indexing and Hybrid Retrieval

- PostgreSQL pgvector extension and full-text index
- local embedding provider and versioned embedding records
- automatic publish-to-embedding workflow, resumable batches, honest progress and recovery (section 59)
- indexing status, retry, and atomic index activation
- metadata filters, lexical/vector retrieval, rank fusion
- source-scoped retrieval tests

## M10 — Answers and Citation Viewer

- guided, compact question/source/review UI with copy and visual checks from section 60
- question UI and Go research API
- local answer provider, evidence packs, synthesis and abstention
- deterministic citation resolution and support verification
- citation cards opening exact passages and original pages

## M11 — End-to-End Evaluation

- representative questions and manually checked reference passages
- full import-to-answer demo on the ARM Mac
- correctness, citation, abstention, latency and memory results
- fixes for failures; POC is complete only after section 58 acceptance gates pass

---

# 49. Deployment for POC

Run the application processes, storage, database, and OCR locally with Docker Compose. Recommended inference uses the user’s existing native LM Studio on the ARM Mac; a strict all-container CPU profile remains an option. No hosted application service or cloud OCR is required; see section 58.

Use Docker Compose:

```text
frontend
api
worker
postgres-with-pgvector
minio
model-runtime (optional strict-container CPU profile; native LM Studio recommended)
```

Local startup goal:

```bash
docker compose up
```

Avoid Kubernetes in POC. Multiple containers do not require splitting the backend into independent business microservices. Keep the Go API and Go worker in one modular backend codebase.

Install Poppler and Tesseract in the worker image and call them as bounded subprocesses. A separate OCR HTTP service is unnecessary for the baseline. Persist PostgreSQL and object storage in named volumes; include health checks and migration/bootstrap steps. Bind user-facing ports to localhost and keep database/storage traffic on the Compose network unless explicitly needed for development.

Target CPU-only operation first. Pin images, language data, and tool versions; verify native linux/arm64 image support on the user's Apple Silicon Mac. Download dependencies/model data during setup and cache them locally. Manual upload and OCR must work without internet after setup; external repository discovery/import still requires internet.

---

# 50. Developer Architecture Rules

1. Connector modules never directly persist final domain records.
2. Vue components do not contain business rules.
3. Go HTTP API route handlers remain thin.
4. Domain/application services own workflow logic.
5. Original assets are immutable.
6. Derived artifacts are regeneratable.
7. Each page belongs to an exact edition/source asset.
8. Each chunk belongs to an exact page.
9. Unknown metadata is never invented.
10. Rights uncertainty produces a review issue.
11. Only `PUBLISHED` sources become available for retrieval.
12. Background jobs must be restartable.
13. Do not add infrastructure without an immediate POC requirement.
14. Do not introduce microservices merely for conceptual separation.
15. Keep retrieval/citation needs compatible with today's schema.

---

# 51. POC — Hybrid Retrieval

Within this POC, after ingestion/review:

```text
PostgreSQL FTS
+
pgvector
+
metadata filters
+
RRF fusion
+
reranking
```

Explicit source/author constraints must be applied before semantic ranking.

Example:

```text
“What does Hering say about Nux vomica?”
```

must search Hering's approved sources rather than other authors.

---

# 52. POC — Citation-Grounded Answering

Required POC flow:

```text
Question
↓
Query parser
↓
Hybrid retrieval
↓
Reranking
↓
Evidence pack
↓
LLM answer with [[E1]], [[E2]]
↓
Citation validator
↓
Backend resolves IDs to bibliographic records
↓
Answer + evidence cards + source viewer
```

Important:

> The LLM must not generate bibliographic citations itself.

---

# 53. POC Source Viewer

The POC user-facing research UI should let a citation open:

- exact source;
- exact edition;
- exact printed page;
- exact passage;
- original scan/page image.

Quoted passages must come from stored evidence, not from model memory.

---

# 54. First Coding Task

Development should begin with this exact vertical slice:

> Scaffold the monorepo and Docker Compose environment with Vue 3 + TypeScript, Go HTTP API, PostgreSQL + pgvector, River, and MinIO. Add SQL schema migrations and sqlc queries for repositories, works, editions, sources, source records/assets, processing revisions, ingestion jobs, pages, chunks, review issues, rights decisions, publications, and audit logs. Include authentication and server-side role checks. Then implement manual PDF upload end-to-end: create metadata → validate/store immutable file → calculate SHA-256 → create ingestion job → expose job state in the Vue admin interface.

Do not implement a remote repository connector until this manual vertical slice works.

---

# 55. Definition of Done

The POC is complete when a demonstration can show:

```text
Trusted source
↓
Import / upload
↓
Immutable original file
↓
Provenance + rights
↓
Page extraction
↓
Text / OCR
↓
Printed page mapping
↓
Page-safe chunks
↓
QA
↓
Human review
↓
Approval
↓
Published source
↓
Embedding + full-text indexing
↓
Question → hybrid retrieval → evidence pack
↓
Answer with validated citations
↓
Open original passage/page
```

Every resulting citation must trace back to an exact chunk, revision, page, original asset, and checksum. The POC must also pass section 58 evaluation gates; ingestion alone is not completion.

---

# 56. Review Decisions and Implementation Clarifications — 2026-10-05

This section refines the earlier conceptual schema and acceptance criteria. Apply these requirements when implementing migrations and APIs.

## 56.1 Confirmed choices and proposals

**Confirmed:** Go backend; retain Vue 3 + TypeScript; deliver ingestion through citation-grounded answers in one local ARM Mac POC, with pgvector and a separate worker.

**Proposed for discussion:** shadcn-vue, chi, pgx + sqlc, Goose, River, and Poppler/Tesseract. These have not been approved as final library choices. Pin compatible versions after a small working spike.

shadcn-vue is a community-led Vue port of shadcn/ui. Components live in the application codebase, giving us control over the page review layout, evidence cards, and eventual research interface. That also makes us responsible for maintaining those components. Its table guide composes TanStack Table; filtering, pagination, selection, and loading/error states need application integration. PrimeVue supplies a more integrated DataTable and can reduce initial admin-screen assembly. Recommendation: shadcn-vue if custom product UI is the priority. Keep Vue; changing to React solely for shadcn is unnecessary. See [shadcn-vue introduction](https://shadcn-vue.com/docs/introduction), [table guide](https://www.shadcn-vue.com/docs/components/data-table), and [PrimeVue DataTable](https://primevue.org/datatable/).

Go suits the API, connectors, jobs, and transactional publication workflow. Use [chi](https://github.com/go-chi/chi) for net/http-compatible routing, [pgx](https://github.com/jackc/pgx) for PostgreSQL access, [sqlc](https://sqlc.dev/) for typed query code, and [Goose](https://github.com/pressly/goose) for SQL migrations. These are separate responsibilities, not separate services.

[River](https://riverqueue.com/docs) stores jobs in PostgreSQL and supports transactional enqueueing. This lets source creation and job submission commit together without a separate Redis deployment. Run migrations for both the application schema and River's schema. No paid River feature is a POC requirement.

PDF/OCR quality must be measured separately from backend language. Trial [Poppler](https://poppler.freedesktop.org/) and [Tesseract](https://tesseract-ocr.github.io/tessdoc/Command-Line-Usage.html) behind adapters. Evaluate a local Python adapter only if representative scans justify it. Hosted OCR is outside the local Docker POC baseline. Record dependency licenses and deployment constraints when selecting processing engines.

## 56.2 Source identity and immutable revisions

An edition is bibliographic identity; it must not be the processing/publication identity shared by every scan.

- Add `sources(id, edition_id, source_record_id, primary_asset_id, status, published_revision_id, created_at, updated_at)`. A source represents one imported scan/document. `/sources/{id}` addresses this entity.
- Create a source record for manual uploads too, with nullable repository/external IDs and uploader-supplied provenance. Missing URLs are allowed; do not invent them.
- Add `processing_revisions(id, source_id, status, component_versions_json, configuration_json, created_at, completed_at)`. Store engine/model/language-data versions and configuration hashes.
- Pages, chunks, jobs, and review issues belong to a revision. An edition's former ingestion status is a derived summary, not the lifecycle authority.
- Page IDs identify pages within a revision. Enforce uniqueness of `(processing_revision_id, source_asset_id, pdf_page_index)` for PDFs and `(page_id, chunk_index)` for chunks. Enforce matching source, edition, asset, and revision relationships.
- Add `publications(id, source_id, processing_revision_id, approved_by, approved_at, published_by, published_at, rights_decision_id)` and keep the corresponding metadata/page-label snapshot immutable.
- Reprocessing creates a candidate revision. The existing published revision stays active until a replacement passes review. Failed reprocessing must not remove the previous publication.
- Corrections after approval create a new candidate revision and require fresh approval. Switch the active publication atomically; retain old published evidence IDs for authorized citation resolution.
- Identical bytes may share an object-storage blob by checksum while retaining distinct source records and provenance. Do not collapse different repository records into one bibliographic identity.

## 56.3 Publication and rights gates

For POC-1, `APPROVED → PUBLISHED` means available in the reviewed corpus. It does not imply embeddings exist. Track indexing separately within this POC. Q&A eligibility requires an active publication and READY index for the selected embedding model; see section 58. Manual upload can enter `IMPORTED` from `DRAFT` without `DISCOVERED`.

Evaluate approval and publication gates server-side in a transaction against the current candidate revision. Block unresolved critical QA issues and incomplete required provenance. Record the reviewer and revision approved. Use optimistic concurrency to reject edits/publication based on stale revisions.

Edition-level rights fields are only summaries. Add an asset/source-scoped `rights_decisions` record containing the raw statement, evidence URL or artifact, intended use, jurisdiction, decision (`allowed`, `denied`, `needs_review`), reviewer, timestamp, and restrictions. Do not treat the string `public_domain` or `licensed` alone as approval. Unresolved `unknown`, `needs_review`, or applicable restrictions block publication. Internal processing permission and permission to display/redistribute scans must be evaluated separately. A generic issue acknowledgement cannot bypass the rights gate.

Disabling a source removes it from retrieval immediately and gates asset access as appropriate. Historical audit records remain retained under the access policy.

## 56.4 Exact text and page coordinates

Use zero-based `pdf_page_index` and `scan_page_index` internally; display one-based scan positions to users. Preserve printed labels verbatim, including Roman numerals and unnumbered pages. A single global offset is a proposal, not a reliable mapping rule across inserts or numbering restarts.

For the initial implementation, `text_exact` must equal a substring of the revision's immutable `text_raw`. Define offsets as zero-based Unicode code-point offsets with an exclusive end; Go byte offsets and JavaScript UTF-16 indices must not be used interchangeably. Store `text_search` separately. OCR text is an extracted transcription, not a guaranteed exact transcription of the scan; always make the original page available for checking. Manual transcription corrections require a new text/revision record and audit history.

OCR confidence is nullable and engine-specific. Missing confidence is not zero or perfect confidence. Store the provider's scale and aggregation method. Preserve word/line bounding boxes when available; passage highlighting is deferred unless reliable coordinates exist.

## 56.5 Reliable jobs and bounded processing

- Treat queue delivery as at-least-once. Use unique step keys per revision/page, checkpoints, and retry-safe writes; do not promise exactly-once execution.
- Persist the source/revision and enqueue work transactionally. Object storage is outside that transaction: track staged uploads and clean up unreferenced objects after a grace period.
- Separate transient download/provider failures from permanent corrupt-file failures. Use bounded retries, backoff, timeouts, cancellation, and clear exhausted-retry status.
- Bound OCR concurrency separately from download concurrency. Kill timed-out child processes and clean temporary files on success/failure.
- Check connector URLs and redirects against allowed hosts; block private/loopback/link-local destinations, including after DNS resolution. Never interpolate downloaded paths into shell commands.
- Require authentication in the first vertical slice. Use server-side ADMIN/REVIEWER checks, bounded pagination, consistent errors, and upload/import idempotency keys.

## 56.6 Verification added to the POC definition of done

Use 2–3 works with representative text-native and scanned pages. Record page counts, selected processing engines, runtime, and review findings; do not claim corpus-wide accuracy from a small sample.

1. Every published chunk passes substring/offset validation and resolves to its immutable revision, page, original asset, and SHA-256.
2. Killing and restarting the worker mid-document creates no duplicate pages/chunks and resumes from persisted progress.
3. Reprocessing a published source leaves existing evidence links intact; a failed candidate does not replace the active publication.
4. Publication is rejected for unresolved rights, blocking QA, stale approval, or a non-admin actor. Disabling removes retrieval eligibility.
5. Run automatic checks on every text page and require a reviewer decision for flagged, missing-text, or unclassified pages before publication. Record the decision and rationale. The former fixed 20-page manual sample is superseded by this automatic QA plus exception-review workflow; evaluate page-label accuracy and OCR quality on representative fixtures and release questions.
6. Blank/title/illustration pages can legitimately have no chunks. Every physical page must have a recorded outcome; missing text on content pages must be reviewed.
7. Add fixtures for numbering restarts, multi-column pages, duplicate job delivery, and concurrent review/publication, alongside the existing fixtures.

## 56.7 Next implementation slice

First scaffold Vue + Go API/worker + PostgreSQL/pgvector + object storage with authentication and a local LM Studio provider adapter. Implement manual upload, immutable storage/checksum, transactional job creation, and a real status screen. Add one-page extraction/OCR validation next. Then implement embeddings, hybrid retrieval, and citation-grounded Q&A as required POC milestones. Dashboards and extra connectors remain lower priority. Library selection and the OCR spike should precede a full scaffold.

---

# 57. Local OCR Recommendation — 2026-10-05

**User requirement:** run the full POC locally in Docker. **User preference:** Tesseract. **Recommendation:** use Tesseract as the initial OCR engine and validate it on actual source pages before claiming adequate accuracy.

| Option | Role in this POC | Tradeoff |
|---|---|---|
| Tesseract | Default CPU OCR engine for printed book scans | Needs good image preparation and layout settings; difficult columns and damaged print require review |
| OCRmyPDF | Optional searchable-PDF derivative using Tesseract | Workflow tool, not a different recognition engine; adds a Python runtime and PDF tooling |
| PaddleOCR | First alternative to benchmark if layout or recognition quality fails | Local neural OCR/document structure tools; more runtime/model dependencies |
| Kraken | Specialist alternative for historical typography | Designed for historical/non-Latin material; suitable models or training may be needed |

See [Tesseract quality guidance](https://tesseract-ocr.github.io/tessdoc/ImproveQuality.html), [OCRmyPDF introduction](https://ocrmypdf.readthedocs.io/en/latest/introduction.html), [PaddleOCR](https://github.com/PaddlePaddle/PaddleOCR), and [Kraken documentation](https://kraken.re/main/index.html). These are candidates, not a measured accuracy ranking for our corpus.

Baseline pipeline:

1. Inspect the PDF and extract usable embedded text with Poppler; OCR only pages that need it.
2. Render scanned pages at approximately 300 DPI initially; test higher resolution for small type. Upscaling cannot recover missing scan detail.
3. Correct rotation/skew and test conservative noise/background cleanup, preserving originals and processing parameters.
4. Run Tesseract with the relevant language model. Compare `tessdata_fast` with `tessdata_best` on the same pages; do not assume the accuracy/runtime tradeoff without measurement. See [model documentation](https://tesseract-ocr.github.io/tessdoc/Data-Files.html).
5. Use automatic page segmentation as the baseline; test single-column modes only on suitable pages. Never apply a single-block mode indiscriminately to multi-column books.
6. Preserve page-level text and TSV/hOCR coordinates/confidence. Treat coordinates as belonging to the processed image and retain its transform to the original scan.
7. Run QA and human review before approval. Store any searchable PDF from OCRmyPDF as a derived asset; keep the uploaded PDF immutable.

Compare engines on the same manually transcribed sample: clean print, faded print, skewed pages, small type, and columns. Measure character/word error rates, remedy-name errors, reading order, printed-page labels, seconds per page, and peak memory. Select any replacement based on this evidence. Do not add multiple OCR engines to the baseline merely because they are available.

---

# 58. End-to-End Research POC and ARM Mac Runtime — 2026-10-05

## 58.1 Scope and meaning of researched answers

The user explicitly requires the complete path from source ingestion to asking questions and receiving researched text with citations. Ingestion, pgvector/FTS indexing, retrieval, answer generation, and citation viewing are required parts of this POC. Earlier ingestion-only completion criteria are superseded.

“Researched” means the system searches the imported, approved corpus, synthesizes relevant evidence across passages/sources where available, attributes claims, preserves disagreements, and states gaps. It does not imply a live web search, exhaustive literature review, or clinical efficacy established by historical texts. Show corpus coverage and source types in the UI. Broader modern research connectors remain later work; user-uploaded material must retain its evidence category.

## 58.2 Hardware and deployment

Confirmed hardware: Apple Silicon ARM Mac, 48 GB unified memory; user can spare up to 38 GB for AI; LM Studio is already installed.

Recommended development layout:

```text
Docker Compose (native linux/arm64 builds)
  frontend: Vue research UI + source administration
  api: Go, retrieval, evidence selection, answers, citation resolution
  worker: Go, Poppler/Tesseract, chunking, QA, embedding jobs
  postgres: PostgreSQL + pgvector + full-text search + River
  object-storage: original PDFs, scans, derived artifacts

Mac host (recommended local inference option)
  LM Studio: embedding model + answer model via local HTTP API
```

This proposal keeps inference local but outside Linux containers to use native Apple Silicon acceleration. It is an explicit exception to an all-processes-in-containers deployment, not a claim that LM Studio runs inside Compose. Preserve a CPU-only model-runtime container profile if strict containerization is required; do not assume Apple GPU passthrough for an ordinary Linux inference container. Docker Model Runner is another local Docker-managed option, but its macOS inference engines run on the host rather than inside a container. See [Docker Model Runner](https://docs.docker.com/ai/model-runner/).

LM Studio supports [chat and embedding API endpoints](https://lmstudio.ai/docs/developer/openai-compat) and [Apple Silicon](https://lmstudio.ai/docs/app/system-requirements). Use provider interfaces in Go so the backend does not depend on LM Studio-specific retrieval or document-chat features. The application owns ingestion, retrieval, and citations.

Proposed configuration, verify actual port/model IDs during setup:

```text
AI_BASE_URL=http://host.docker.internal:1234/v1
CHAT_MODEL=<loaded-answer-model-id>
EMBEDDING_MODEL=<loaded-embedding-model-id>
```

Use Docker Desktop's host address from containers, not container-local localhost. Verify server binding and authentication from the API/worker containers; allow only necessary local access. The Vue browser calls Go, not the model server directly. See [Docker networking](https://docs.docker.com/desktop/features/networking/) and [LM Studio server](https://lmstudio.ai/docs/developer/core/server).

Treat 38 GB as a ceiling, not a target allocation. Initial planning budget: roughly 20–26 GB for model processes including context/cache, 6–8 GB for Docker, and the remainder for macOS/browser/headroom. These are estimates to measure, not model memory guarantees. Limit generation to one request initially and reduce OCR/embedding concurrency if memory pressure rises. Persist and cache model files; document startup readiness and fail clearly if LM Studio is unavailable. No hosted fallback or automatic cloud upload.

## 58.3 Initial model candidates

Recommend benchmarking [Qwen3-14B GGUF Q4_K_M](https://huggingface.co/Qwen/Qwen3-14B-GGUF) for answer synthesis, initially with an 8K context limit and a bounded evidence pack. This is a practical baseline, not a claim that it is the best available model. Increase model size/context only if measured answer support or completeness improves within memory/latency limits.

For embeddings, benchmark [Qwen3-Embedding-0.6B](https://huggingface.co/Qwen/Qwen3-Embedding-0.6B) using a compatible locally served artifact. Verify LM Studio/runtime support, query instructions, output dimensions, pooling, and normalization with real API calls before locking it in. The embedding model and answer model serve different purposes and must have separate identifiers/configuration.

Pin model artifact/revision, quantization, tokenizer, prompt/template, and runtime version. Disable silent truncation: split embedding inputs to fit the embedding model and build answer context within the model token budget. Store the selected vector dimension in the schema only after verification. Re-embed into a new index version when the embedding configuration changes; never compare embeddings from incompatible models.

## 58.4 Indexing and retrieval

Enable `CREATE EXTENSION IF NOT EXISTS vector` in a migration against an ARM-compatible PostgreSQL image containing pgvector. pgvector stores/searches embeddings; it does not generate them or answer questions. For 2–3 works, start with exact vector search plus a GIN full-text index; add HNSW only if measured latency requires it. PostgreSQL full-text ranking is not BM25. See [pgvector documentation](https://github.com/pgvector/pgvector).

Add:

- `embedding_configs(id, model_id, model_revision, dimensions, distance_metric, preprocessing_version)`.
- `index_runs(id, publication_id, embedding_config_id, status, expected_chunk_count, indexed_chunk_count, error, timestamps)`.
- `chunk_embeddings(chunk_id, embedding_config_id, embedding, input_hash, created_at)`, unique on chunk/config; enforce dimensions and immutable chunk identity.
- Indexed full-text representation of `text_search`, with tested remedy alias expansion that never modifies `text_exact`.

Track `PENDING/RUNNING/READY/FAILED` indexing independently from approval. Activate a complete index version atomically; failed rebuilds leave the previous active publication/index usable. Retrieval requires the source to remain enabled, the publication to be active, and the selected index to be READY. Enforce these filters in SQL on both retrieval paths, including explicit author/edition/language constraints. Recheck eligibility before returning an answer.

Question → metadata constraints → query embedding + lexical query → retrieve both candidate sets → reciprocal-rank fusion → deduplicate overlapping evidence → bounded evidence pack. Start with up to 20 candidates per path and 6–10 final passages, tune on the evaluation set. These are starting parameters. A local reranker is optional after baseline measurement; rank fusion is required.

## 58.5 Answer and citation contract

Add authenticated endpoints:

```http
POST /api/v1/research/questions
GET  /api/v1/research/answers/{id}
GET  /api/v1/citations/{id}
GET  /api/v1/sources/{id}/index-status
POST /api/v1/sources/{id}/reindex
```

Question request includes the question and optional source/author filters. Only ADMIN can reindex. Responses include `answer_id`, `status` (`answered`, `partial`, `insufficient_evidence`), answer text, evidence limitations, and server-resolved citation records. Provider failures are explicit errors, not evidence abstentions.

Persist answer runs, model/prompt/index versions, retrieved evidence IDs/scores, and claim-to-evidence links. Each citation resolves to the immutable publication, chunk, original passage, work/author/edition, printed page if known, scan position, original asset/checksum, and authorized viewer link. If a printed page is unknown, show scan position explicitly rather than guessing.

Give the model opaque evidence IDs and quoted text. Treat source text as data, never instructions. The model must synthesize only supported claims, distinguish authors/source types, and identify disagreements and missing evidence. Conversation history is not source evidence. Never allow the model to invent bibliographic fields, URLs, or citation IDs.

Before displaying a final answer:

1. Validate citation IDs against the supplied evidence set and current authorization/publication state.
2. Check direct quotations against stored evidence and require citations for substantive source claims.
3. Run a separate support-check pass against cited passages; remove/regenerate unsupported claims once, then return a partial answer or abstain. An LLM support check is fallible and does not replace human evaluation.
4. Build bibliographic labels and viewer links in Go from stored records.

Buffer the final answer until checks complete for the initial POC. A progress indicator can show retrieval/generation/validation. The UI displays the answer, numbered inline citations, source cards, and original-page/text side panel.

## 58.6 End-to-end acceptance gates

Build 30 manually reviewed questions before tuning: 10 single-source, 10 multi-passage/comparison, 5 author/edition constrained, and 5 deliberately unsupported. Keep a held-out subset for final evaluation. Gold records contain required supporting passages, expected claims, and abstention behavior.

Proposed POC release gates:

- 100% of displayed citation IDs resolve to the correct immutable source page and passage; zero fabricated bibliographic values or quote mismatches.
- 100% exclusion of disabled/unapproved sources and adherence to explicit source/author filters in dedicated fixtures.
- At least 90% recall of required gold evidence within the top 10 fused passages across answerable questions; report both aggregate and per-question results.
- At least 95% of substantive generated claims supported by their cited passages in human review, and at least 90% coverage of expected answer points on answerable questions. Abstaining on everything cannot pass.
- All five unsupported questions avoid unsupported substantive answers and explain the corpus limitation.
- Demonstrate upload/import → OCR/review → publication → embeddings → question → synthesized answer → click citation → original page, including a multi-source question.
- Record warm/cold retrieval/generation latency, peak memory and swap behavior on the user's Mac; avoid out-of-memory failures. Set the user-facing latency target after the first measured model run.
- Repeat citation resolution after reprocessing and index replacement. Old citations remain stable and authorized; new answers use the active approved index.

These are acceptance targets, not achieved results. The repository now contains a running local API, worker, UI, PostgreSQL migrations, and automated tests. The scored human-reviewed release set, OCR quality measurements, citation-stability demonstration after a live replacement, and Mac latency/memory results are still required before calling the full POC accepted.

---

# 59. Explicit Embedding Creation Workflow — 2026-10-05

Embedding creation is a required working POC feature, not a manual setup step or future task. Section 58 defines the storage/retrieval contract; this section defines the trigger, progress, and recovery behavior.

1. After a reviewed source is published, create its initial `index_run` and enqueue preparation in the publication transaction. An idempotency key for publication + embedding configuration prevents duplicate runs from repeated clicks. A successful publication alone does not mean the source is ready for questions.
2. Confirm that the configured local embedding model is available. Snapshot the immutable publication/chunk set, model configuration, and expected passage count. If no eligible text passages exist, stop with an actionable review issue rather than mark the source ready.
3. Read `text_search` for each passage, enforce model token limits, apply versioned document preprocessing, and send bounded batches to the embedding provider. Preserve the original evidence text. Any needed chunk splitting must preserve page/offset provenance and occur before freezing the publication; never silently truncate or modify published chunks during indexing.
4. Validate every returned vector: correct response/input association, dimensions, finite values, and suitability for the selected distance metric. Write embeddings to pgvector and persist checkpoints after successful batches. A timeout or partial response must not mark unprocessed passages complete.
5. Build/check the full-text representation for the same eligible passage set. Record completed/total passages and recoverable errors. Do not average unlike OCR and embedding stages into a fabricated overall percentage.
6. Confirm all expected passages have valid vectors and lexical records. Atomically mark the index READY and activate the matching publication/index pair, subject to existing review/rights/source-enabled gates. Only then display “Ready to ask”.
7. On failure, retain completed work and allow an authorized retry of incomplete batches. Repeated delivery must not duplicate embeddings. Model/configuration changes create a separate index version; ordinary retry uses the same snapshot. Continue serving a previous ready version during an update and clearly label that behavior.
8. When a question arrives, create its query embedding using the same compatible embedding configuration, with the model's query-specific instructions. Use it with keyword search to retrieve passages. Creating document embeddings is not model training, and the UI must not describe it as training.

The source status API returns publication/index readiness, active and candidate versions, current preparation step, completed/total passages, a user-safe failure reason, and authorized next actions. Initial indexing starts automatically; `reindex` is for retries or deliberate updates. The backend derives allowed actions; UI hints do not replace permission checks.

Acceptance checks: publish triggers preparation automatically; restarting a worker resumes without duplicate vectors; bad/partial model responses cannot produce READY; a failed update leaves prior answers available; a question uses the matching embedding configuration. A visible source journey must show preparation progressing to a real successful question and citation.

---

# 60. Guided, Compact POC User Experience — 2026-10-05

## 60.1 User requirement and navigation

The POC must explain what the user should do, what the system is doing, and what happens next in short, nontechnical language. A new user should complete the source-to-answer journey without reading the PRD or receiving a developer walkthrough. The interface should feel polished, calm, and compact.

Use three primary destinations: **Ask**, **Sources**, and **Review**. Put repository configuration, model connection settings, and diagnostics under secondary **Settings**. Upload, discovery, processing, source details, and page review remain screens within these destinations, not nine competing top-level navigation items. Use a small next-action summary instead of a large metric dashboard.

Each source has a persistent journey strip:

```text
Add source → Read pages → Review → Prepare for questions → Ask
```

Mark the current step, finished steps, and any action needed using text and icons. Users can leave and return without restarting. Accepted background work must continue without an open browser while the local runtime remains available; see section 61 for upload, shutdown, and recovery behavior. Keep an “Ask about this source” action when ready; other ready sources remain usable while new ones process.

## 60.2 Guided copy and next actions

Use these as initial product copy; adapt counts and actions to actual state and permissions. Headings are short; helper text is normally one sentence and never more than two short sentences. One primary action per step, with secondary actions visually quieter.

| Step/state | Heading | Helper text | Primary action |
|---|---|---|---|
| No sources | Add your first source | Upload a book or find one in an approved collection. | Add source |
| Upload | Choose a PDF | Add the original file. We’ll read its pages and keep it for checking references. | Upload PDF |
| Source details | Check the book details | Confirm the title and author. Leave anything you don’t know blank. | Save details |
| Reading | Reading the pages | We’re turning the pages into searchable text. You can leave this screen and return later. | View progress |
| Review needed | A few items need checking | Check the highlighted items against the original pages. | Review items |
| Ready for publication | Make this source available | The required checks are complete. Continue to prepare this source for questions. | Prepare for questions |
| Preparing | Preparing for questions | We’re organizing the passages so the system can find useful evidence. | View progress |
| Ready | Ready to ask | This source is now available for answers with page references. | Ask about this source |
| Question | Ask your sources | Ask a question. Each reference lets you check the original passage. | Ask |
| Answer | Answer from your sources | Open a reference to see the passage behind the answer. | Source references are inline actions |

“Prepare for questions” calls the authorized publish action and starts indexing automatically; it does not bypass approval. If approval is still needed, use “Approve source” with “Confirm the source checks before making it available.” REVIEWER users see “Waiting for admin approval” when applicable. Blocking issues show what must be resolved and link to the relevant item.

Book details may be saved as an incomplete draft. Missing required provenance can still block approval later; do not make users invent data to complete an upload.

## 60.3 Explain system behavior without exposing implementation details

Use “Reading scanned pages” instead of OCR, “Passages” instead of chunks, and “Preparing for questions” instead of embedding/indexing in the main workflow. An optional “How it works” disclosure can say: “The system uses both wording and meaning to find passages, then writes an answer with references you can check.”

Show actual progress, for example “120 of 300 pages read” or “Preparing passages: 240 of 600”. Show waiting/running/finished states when counts are unavailable; do not invent progress or time estimates. During questions show “Finding relevant passages”, “Writing the answer”, and “Checking references” only when those operations actually run. These labels describe processing, not a guarantee of correctness.

Keep model names, vector dimensions, checksums, raw errors, and internal IDs in a collapsed “Technical details” area for administrators. Important source identity, evidence type, rights decisions, and limitations remain visible because they affect interpretation and user decisions.

## 60.4 Recovery and empty states

Apply the persistent Activity and failure-communication requirements in section 61 to every state below.

- Local model unavailable: “The local AI connection is unavailable. Open LM Studio and start its server, then try again.” Provide “Check connection” and a short setup guide. Distinguish connection failure from a missing model when the backend can identify it.
- Preparation failed: “We couldn’t finish preparing this source. Your file and completed work are saved.” Offer authorized “Try again” and details.
- No ready sources: “Prepare a source before asking questions.” Link to its next required step; preserve any typed question.
- Some sources still processing: show “Searching 2 ready sources · 1 still preparing”. Do not silently imply all uploaded material was searched.
- Updating an existing source: “Updating this source. Answers still use the previous reviewed version.”
- Insufficient evidence: “I couldn’t find enough support in the selected sources.” Offer “View sources” or “Change sources”; do not confuse this with a server error.
- Partial answer: clearly identify what the selected sources support and what remains unanswered.

Preserve form input, source selection, and the current question across recoverable failures. Avoid success notifications for merely queued work.

## 60.5 Visual and interaction direction

Use the proposed Vue + shadcn-vue foundation with consistent typography, spacing, buttons, and form patterns. Prefer a neutral background, one restrained accent color, subtle borders, and clear status labels. Keep content readable with a roughly 65–80-character answer line length. Avoid oversized hero sections, giant cards for every field, nested panels, decorative charts, or excessive animation.

Sources use a compact readable list/table: title/author, status, progress or next action. Advanced filters and metadata open on demand. The Ask screen prioritizes the question field, a concise source selector, and the answer. Example questions must fit available sources rather than advertise unavailable coverage.

Citations show author/work, edition or volume when known, and page label. Clicking a citation opens a side panel with the stored passage and original page. Preserve the answer and reading position. Page review uses a resizable scan/text split view; stack or tab these panes on narrow screens. Do not squeeze a full desktop table onto a small screen.

Require visible keyboard focus, labeled inputs, readable contrast, status text in addition to color, and accessible progress/error announcements. Respect reduced-motion preferences and avoid announcing every polling update. Keep enough spacing for comfortable clicking without making the screen bulky.

## 60.6 UI acceptance checks

- A first-time POC user can add a source, identify the next action, review issues, observe automatic preparation, ask a question, and open its citation without a developer explanation.
- Empty, loading, blocked, failure, retry, partial-answer, and ready states contain a short explanation and an appropriate next action.
- Technical vocabulary is absent from the primary journey unless explained in an optional disclosure; published and ready-to-answer states remain distinct.
- Verify the full journey at representative laptop/desktop widths and a narrow viewport, including long titles, long answers, and multi-source citations. No overlapping controls or page-wide horizontal overflow.
- Verify keyboard completion of the main journey, panel focus/close behavior, and preservation of the question when opening citations.
- Validate UI progress against real worker/indexing state; no simulated success or prematurely enabled “Ready to ask”.

These are implementation and visual review requirements. No UI implementation or usability validation is claimed by this document update.

---

# 61. Background Activity and Failure Communication — 2026-10-05

## 61.1 Visibility throughout the application

Every user-relevant background operation must have a visible, persistent status: source download, upload completion, page reading, quality checks, search preparation/embeddings, updates, and answer generation/reference checks. Include automatically triggered jobs, not only tasks started by a button. Group internal subtasks under the corresponding source or question to avoid notification noise.

Provide a compact **Activity** control in the shared app header with running-task and action-needed counts. Its panel shows the source/question, current step, real progress when available, last update, and next action. Relevant source rows and question screens show the same server-backed status. Do not require users to remain on the processing screen.

Use plain-language states: **Waiting**, **Working**, **Retrying**, **Needs your attention**, **Finished**, **Couldn’t finish**, and **Cancelled** where cancellation is supported. Indicate waiting for a busy local model separately from a failure. Completed steps and saved checkpoints remain visible when a later step fails.

Show a brief completion notice and retain it in Activity. Failures and blocked work also appear persistently beside the affected source/question until resolved. Dismissing a notice only marks it read; it does not fix the task or clear its failure. Deduplicate notices per event; never show a new toast on every poll. Group repeated retries into one activity item. No email or operating-system notification integration is required for this POC.

## 61.2 Useful error messages

Each failure message must state, concisely:

- What could not finish and which source/question it affects.
- The known cause, or that the cause is not yet known.
- What is saved and what remains unavailable, only when verified.
- Whether the system will retry automatically or needs user action.
- An appropriate next action, such as **Try again**, **Check connection**, **Review pages**, or **Choose another file**.

Example when the worker has confirmed saved checkpoints and a scheduled retry:

> **Source preparation interrupted**  
> The local AI connection was lost. Completed passages are saved. We’ll retry shortly.  
> **Check connection**

Example when all automatic retries are exhausted:

> **We couldn’t finish preparing this source**  
> Completed work is saved, but this source is not ready for questions. Check the local AI connection, then try again.  
> **Try again**

Example when an answer request fails:

> **We couldn’t finish this answer**  
> The local AI service stopped responding. Your question is still here.  
> **Try again**

Messages must reflect the actual condition. Never claim an upload was saved before storage is confirmed, promise a retry that is not scheduled, or report missing evidence when a service failed. Show unavailable capabilities without blocking unrelated working features. Keep safe diagnostic details and an error reference in an expandable area; do not expose secrets or stack traces in the main UI.

## 61.3 Durable status, disconnects, and local runtime limits

Persist operation state, stage, timestamps, heartbeat, progress counters, retry attempts/next retry, safe error code/message, and allowed actions. Store durable activity events with stable IDs and per-user read state; enforce existing source/question permissions. Write meaningful state transitions and their activity events atomically. Reconcile queue/worker failures into application state so a crashed job cannot remain “Working” indefinitely.

Use polling for the POC: target roughly 3-second refresh while visible work is active, back off when idle/disconnected, and reconcile immediately on reconnect, focus, or page reload. Provide a paginated activity API and mark-read action. On a healthy connection, stage changes and failures should appear within two active polling cycles after the backend records them. Worker heartbeats and per-step timeouts must detect stalled work; a long OCR operation must continue heartbeating even without new completed pages.

If the browser cannot reach the API, show a persistent connection banner: “Connection lost. Progress may be out of date. We’ll reconnect automatically.” Display the last successful update and do not infer that background work has stopped. After recovery, fetch authoritative states and missed events without duplicate notices.

Work continues when the user navigates away or closes the browser **after upload/request acceptance**, while Docker, required local services, and the Mac remain running. Uploads still transferring may be interrupted by closing the browser. Mac sleep, stopped containers, or stopped LM Studio can pause or interrupt work; do not promise otherwise. Resume eligible work from saved checkpoints when services return. Browser closure does not itself cancel an accepted answer job; persist its outcome so it can be reopened from Activity. A completed answer remains subject to citation access controls.

## 61.4 Acceptance checks

- Start ingestion or embedding work, navigate away, then reopen/reload: status, progress, and the correct next action remain available.
- Stop LM Studio during embeddings and answer generation; show accurate interruption/retry/failure states, preserve completed work/questions, and recover without duplicate jobs.
- Stop the worker or simulate a stale heartbeat; detect the interrupted task within the configured timeout instead of displaying an endless spinner.
- Disconnect the API/browser connection; display stale-state guidance, recover status after reconnect, and avoid duplicate notifications.
- Exercise invalid PDFs, storage failures, exhausted retries, and partial indexing failures; verify each message names the affected task and a valid recovery action.
- Close and reopen the browser after a task completes or fails; its durable activity result is still discoverable.
- Verify that completion notices do not fire for merely queued work, dismissed errors stay unresolved until actual recovery, and unauthorized users cannot see other users’ question activity.

These requirements extend sections 36, 38, 58–60. They are specification requirements, not claims of implemented notifications or tested recovery.

---

# 62. Verified Starter Corpus — 2026-10-05

The starter list is now backed by downloaded files, repository snapshots, checksums, and page maps. [Manifest](data/starter-corpus/manifest.json) is the machine-readable import seed; [guide](data/starter-corpus/README.md) explains how to use it.

| Priority | Book and repository record | Edition / publisher | PDF pages / scans | Intended first use |
|---|---|---|---|---|
| 1 | [Nash — Leaders in Homoeopathic Therapeutics, 1899](https://wellcomecollection.org/works/sh3awcy7) | Edition number unstated; Boericke & Tafel, Philadelphia | 399 / 398 | Manual PDF import and first question/citation |
| 2 | [Farrington — A Clinical Materia Medica, 1890](https://wellcomecollection.org/works/hh24qeg4) | Second edition; Hahnemann Publishing House, Philadelphia | 779 / 778 | Wellcome connector import and second-author retrieval |

The downloaded PDFs are under `data/starter-corpus/originals/`. Metadata includes exact download URLs, source/work/item IDs, author and contributor records, sizes, SHA-256 hashes, repository rights statements, and retrieval timestamps. Both records carry a Public Domain Mark; app-level rights/provenance approval remains pending. Nash's edition number and both volume labels remain null rather than invented.

**Verified technical finding:** only the generated Wellcome cover sheet has embedded text. The remaining book pages are scans. Use per-page text checks and either canvas-linked repository ALTO OCR or Tesseract. Whole-book repository OCR must not become citation evidence without page boundaries.

Each PDF adds one generated metadata page before the IIIF scans. The manifest/page maps explicitly store `pdf_page_index = scan_page_index + 1` for this acquisition, with zero-based indices. This does not determine printed page labels. Exclude PDF index 0 from book-text retrieval. Sampled title/content pages were visually checked; the full-book sequence, OCR, and printed labels still require ingestion QA.

Start QA with Nash PDF index 49 / printed page 39 and Farrington PDF index 49 / printed page 45. Their source images and page-specific OCR samples are saved under `qa/`. Seed questions refer to those inspected pages; a full scored evaluation set remains implementation work.

Verification command from repository root:

```bash
python3 scripts/verify_starter_corpus.py
```

The offline check passed for both sources: SHA-256, byte sizes, PDF signatures, and page-map structure. PDFs opened successfully in a PDF parser; all pages were checked for embedded-text presence. This verifies preparation/integrity, not publication eligibility or OCR accuracy. Ready-for-import is not Ready-to-ask: processing, review, publication, embedding generation, and Q&A remain normal application steps.

Keep the existing 2–3-work POC target; these two books satisfy the minimum starting corpus. Add a third only if the evaluation needs a different layout or source type.

---

# 63. Research Workspace and Multi-Source Roadmap — 2026-10-06

## 63.1 Product outcome and boundaries

The product is a source-led research workspace. A user should be able to ask a broad or narrow question, see how it was investigated, read a synthesis that distinguishes agreement, disagreement, uncertainty, and missing evidence, and open each supporting passage in its original document. The design reference is the inspected [AnswerThis GLP-1 canvas](https://app.answerthis.io/canvas/379e9970021d70ff): four visible searches across three literature databases, a source comparison table, ten linked citations, and follow-up tasks. These are observed interaction patterns, not a quality guarantee or a requirement to copy its exact interface.

Keep two evidence domains explicit. **Historical library research** answers what named authors such as Nash and Farrington wrote. **Contemporary evidence research** will answer questions about modern studies only after rights-aware paper acquisition, study-type labeling, and claim-level verification exist. Never merge a historical remedy claim into a modern efficacy conclusion. The local model receives approved source passages only; there is no hosted answer or embedding fallback. External discovery services may receive a search query and return bibliographic metadata, but source text is not submitted to them for generation.

The current POC remains a research aid, not a clinical decision or treatment tool. Answer arbitrary user questions conversationally, but identify when the indexed corpus cannot support the requested topic and suggest a useful narrower or different research path. An answer that cites nothing is not a researched answer.

## 63.2 Current implementation and immediate source expansion

Nash and Farrington are the first two operating sources. The starter pack contains Farrington's 1890 second edition PDF, page map, checksum, catalogue snapshot, and repository rights statement. Farrington uses the same retry-safe ingest → automatic page QA → flagged-page review → rights decision → publication → embedding workflow. Its 778 scans, rather than Nash's 398, drive progress and publication checks. The user sees each book's preparation state separately. A published book is searchable only after its own complete READY index is active.

The first cross-source gate is a question that retrieves and cites at least one exact, verified passage from each book, with author names attached to the relevant claims. Source filters and author/edition constraints must prevent wrong-author retrieval. Citation viewers must retain the original scan and stable source metadata. The UI should show corpus coverage, including which books were ready for a given answer. More books are not an automatic proxy for depth: add a third source only to cover a measured question-set gap.

## 63.3 Deep research workflow

Offer a quick answer for focused questions and a deeper research run for multi-part or comparison questions. A deep run records: the original question, a small bounded set of focused subquestions/searches, filters and sources searched, retrieved candidates and scores, included/excluded evidence, an evidence table, model and prompt revisions, verification results, and the final answer. Searches should be visible to the user; do not simulate counts or progress. Set strict budgets for number of searches, passages, tokens, time, and retries.

For each search, perform lexical plus vector retrieval with reciprocal-rank fusion, diversify by source when comparing authors, and deduplicate overlapping passages. Test local reranking only if the held-out evaluation shows a retrieval gap. Extract an evidence table with one row per potential claim: exact passage ID, source/author/edition, page or scan, evidence category, relation to the question, and limitations or contradiction. The answer model writes from this table and must not create citation IDs or bibliographic data. Check each substantive claim against its own cited passage. Unsupported claims are removed; return a partial answer or a clear insufficient-evidence result when needed. Verification failures and model/index errors remain distinct statuses.

The answer should start with a direct, friendly synthesis; then explain important comparisons, uncertainty, and what to inspect next. Length follows supported complexity rather than a fixed five-sentence template. Show an evidence/source table and citation-to-page side panel. Follow-up suggestions should be grounded in actual gaps, such as comparing a named second author or locating a missing type of study.

## 63.4 Contemporary literature phase

After the historical cross-source gate, add metadata discovery through PubMed, OpenAlex, and/or Semantic Scholar with documented API limits, dates, deduplication by DOI/PMID, and reproducible query logs. Label original trials, systematic reviews, observational studies, commentaries, letters, and historical works. Prefer primary studies for numerical trial claims; if only an abstract or secondary summary is available, disclose that limit. Ingest full text only when access and reuse rights permit it, preserve the original PDF/version, and cite exact pages or sections. A bibliography link alone does not establish claim support.

Systematic-review mode is a later, stricter workflow: protocol/question criteria, saved search strings and dates, inclusion/exclusion decisions, duplicate handling, study table, and human review. A broad narrative answer must never claim to be an exhaustive systematic review merely because it cites several papers.

## 63.5 Models, evaluation, and release gates

Keep embedding and answer model IDs/revisions separate. The local answer model is now `qwen/qwen3-14b` (Q4_K_M); the embedding model remains `text-embedding-nomic-embed-text-v1.5`. The 14B model was verified through LM Studio's API on real cross-author and insufficient-evidence questions. A reversed comparison in an initial answer prompted per-sentence citation checks before the switch. Further model changes require API/model-ID verification, measured supported-claim coverage, latency and memory, and a rollback path. Changing the embedding model requires a new complete index, never mixed vector comparisons. Increasing model size alone is not an acceptance criterion.

Before calling the cross-source POC complete, extend the section 58 gold set with at least ten Nash/Farrington comparison questions and five questions outside corpus scope. Report evidence recall, claim support, expected-point coverage, citation resolution, abstentions, latency, and peak memory per question. The section 58 citation and authorization gates continue to apply. A passing demonstration includes: import Farrington, complete automatic QA and necessary human checks, publish, reach READY, ask a cross-author question, inspect citations in both original scans, and reload the saved answer. Maintain regression checks for Nash and for an unsupported question.

Implementation should proceed in independently runnable slices: (1) second-source ingestion and status, (2) cross-source retrieval and answer attribution, (3) persisted research trail and evidence table, (4) bounded multi-search deep mode, (5) literature discovery and rights-aware modern paper ingestion. Each slice is verified against real database and local-model behavior before expanding the next.

## 63.6 Historical implementation checkpoint — 2026-10-06

The current code supports Farrington import from the checked starter corpus, source-specific scan maps and page counts, automatic page QA with a crop fallback for sparse OCR, index-page exclusion for the inspected Farrington edition, and independent embedding readiness. Farrington has been published after its flagged pages and rights were checked; its embedding job records real progress toward READY. Ask performs a real local-model answer from ready sources; a question naming one author restricts retrieval to that author, and a Nash/Farrington comparison searches each ready book separately. Quick mode searches once per source. Detailed mode plans two additional focused local-model queries and records all actual searches. Selected evidence passages, citation use, search scopes and candidate counts are saved with answered results; search records are also saved for insufficient-evidence results. Inline citation labels survive answer reload. The UI displays the search trail, selected passages, citations, and original scans. This is a bounded historical research slice; contemporary literature discovery, claim-level verification, and a full scored comparison set remain future work.

Both first-source indexes reached READY (Nash 1,120 passages; Farrington 2,608). A detailed Lachesis throat question returned citations to both authors after six searches; the saved answer and all four citation scans resolved. The local answer model is now Qwen3 14B (Q4_K_M), as described in section 63.5. The implementation retries one malformed plan and checks cited sentences for unsupported names, quotes, and claims; these checks can still reject a valid paraphrase or miss a wrong attribution. The next model milestone is the evidence-first claim pipeline in section 63.7, benchmarked on the section 58/63.5 gold set with human review of each claim and scan. The present detailed mode does not yet establish AnswerThis-scale literature coverage or systematic review quality.

Claim verification now handles partial answers: after one repair attempt, each cited sentence is checked against its own passage. Supported sentences can be returned while unsupported or uncited sentences are omitted. The response records the omission count and shows it in the UI. Insufficient-evidence results also retain the retrieved candidate passages so the reader can inspect the scans and refine the question. For example, a “bad mood” run retained the Nash and Hepar passages but omitted a model claim about Ignatia and Nux vomica whose citation actually described Mercurius. The product must never promise an answer for every search term; a term outside the indexed books or a claim without matching evidence still requires abstention.

Still required: complete and score the held-out cross-source evaluation, add an evidence comparison table and claim-to-evidence record, implement bounded subquestion planning and gap-aware follow-ups, then add modern-literature discovery and rights-aware full-text ingestion. Benchmark a larger local answer model only against the same evaluation questions and hardware measurements. Do not describe historical two-book synthesis as a modern systematic review.

## 63.7 Citation reliability upgrade

The current answer contract accepts prose with inline evidence labels and then tries to verify the labels. This is insufficient for reliable attribution: a model can cite a real passage that shares symptom words but discusses a different remedy. In a local Qwen3 4B check, a request for an exact quote supporting an Ignatia/Nux vomica anxiety claim returned “anxiety and restlessness” from a Mercurius passage. The quote was real, but the attribution was false. A single answer-wide support verdict or a vocabulary-overlap threshold cannot resolve this.

Implement the next citation slice as an evidence-first claim pipeline:

1. Ask the local answer model for a bounded array of atomic claims, each with one or more supplied evidence IDs and a short supporting excerpt copied from each cited passage. Render prose only from accepted claims. Never let the model supply source metadata or URLs.
2. In Go, normalize scan whitespace and line-break hyphenation for matching, then require every excerpt to be an actual span of its cited immutable chunk. Store the exact chunk offsets, page/source/publication IDs, and index/model/prompt versions with the claim. A matching excerpt is necessary but not proof of support.
3. Check each claim against the local context around its excerpt. Remedy names, authors, negation, comparison targets, and temporal qualifiers must refer to the same subject in that passage. Use a separate local-model support check on **one claim and its cited span/context at a time**, with explicit supported/unsupported/uncertain results. Reject unknown IDs and contradictory or ambiguous results.
4. Persist each claim and its verification decision. Return `answered`, `partial`, or `insufficient_evidence` according to accepted claim coverage. Show omitted or uncertain claims and the reason in the research trail; keep retrieved passages available for manual inspection. A failed model call is an error, not insufficient evidence.
5. Evaluate the change on the held-out section 58 set before replacing the current path. Include the real “bad mood” wrong-remedy citation, “headache” OCR-wrap paraphrase, multi-author comparisons, unsupported modern-trials prompts, negated claims, adjacent-remedy paragraphs, and claims that combine two passages. Measure supported-claim precision, expected-point coverage, citation/span resolution, abstentions, and latency. Benchmark a stronger **local** answer model against the same cases before choosing it.

The release decision is based on human review of claim support and usefulness, not the fraction of questions that receive an answer. Unsupported questions must still abstain. No source text is sent to a hosted generation or verification service.

---

## 63.8 Source intake implementation checkpoint — 2026-10-06

The app now accepts administrator-uploaded PDFs from outside the Nash/Farrington starter pack. Uploads are size bounded, checked as readable PDFs, hashed, stored under their SHA-256 name, and linked to a queued ingestion job in one database transaction. The local worker reads text-native pages with Ghostscript, uses Tesseract for scanned pages, creates page-safe chunks, and enters the existing automatic QA and review workflow. The source screen collects provenance fields; review allows metadata and printed-page corrections before publication. Original uploaded pages are rendered from the saved PDF for review and citation access. An isolated PostgreSQL integration test covers HTTP upload, worker ingestion, and page-image delivery. Docker Compose now defines database, API, worker, and UI services; the images build and an API/UI smoke check passed.

This is the manual-upload route for general source intake. Repository discovery/import beyond the two fixed starter records is still open. The separate edition, asset, and processing-revision model, stable citation replacement across reprocessing, recorded claim-level verification, durable answer jobs and Activity, and scored release evaluation also remain open acceptance work. The historical-source POC gate remains separate from later modern-literature discovery.

## 63.9 DOI-first reference intake checkpoint — 2026-10-06

The source screen now accepts a DOI or doi.org URL. The API normalizes the DOI, retrieves bibliographic metadata from Crossref, falls back to DataCite for an unregistered Crossref DOI, and saves a deduplicated reference without a PDF. Reference-only records do not enter retrieval or support page citations. A direct PDF import is offered when Crossref deposits a public HTTPS PDF link and a supported Creative Commons licence for that same content version, with the licence start date reached. A narrow PLOS ONE connector supplies the verified printable PDF route for eligible `10.1371/journal.pone.*` DOIs whose Crossref record lacks a direct PDF link. Downloads are size bounded and reject private network destinations and unsafe redirects. The imported PDF then follows the existing checksum, automatic QA, rights review, publication, and embedding path. If no eligible direct PDF is available, the user can still upload a permitted copy. Published citation panels now expose the external source URL or DOI alongside the saved page. This is DOI lookup by identifier, not topical literature discovery or proof that every DOI supplies full text.

The original DOI import example, `10.1371/journal.pone.0118440`, was found to be retracted by PLOS in 2020 ([publisher notice](https://doi.org/10.1371/journal.pone.0232415)). It remains visible as a historical reference but must not be published or cited as evidence. DOI intake now reads Crossref's `updated-by` retraction metadata, records the notice link, and blocks PDF import and publication for a flagged record. The live import check uses `10.1371/journal.pone.0134657` instead. Retraction checks for manually uploaded PDFs and sources outside Crossref still need a general integrity workflow.

## 63.10 PDF-link intake and DOI reference removal — 2026-10-07

The source screen now accepts a direct public HTTPS PDF URL plus title, author, edition/volume, publication, repository, catalogue URL and rights statement. The API rejects private destinations and unsafe redirects, bounds downloads to 250 MB, stores the exact PDF origin URL and file checksum, then queues the existing ingestion and review workflow. This is direct URL import, not automated repository search. A repository landing page or DOI URL must be resolved to a PDF link first. The reviewer can inspect both the catalogue record and the original PDF link. Uploads share the 250 MB bound.

Saved DOI references can be deleted from the source list. A reference with no linked source is deleted from the database; one with an imported source is hidden from the list while its DOI record, retraction guard and source remain intact. Re-entering the DOI restores its visibility. Removing the reference does not delete the downloaded PDF or published citations. The Go tests and Vue build pass; an end-to-end linked-PDF download through a running local stack remains to be exercised.

## 63.11 Automatic PDF metadata at intake — 2026-10-07

Manual upload and direct PDF-link import no longer require the user to type a title or author first. Intake reads opening-page text and uses bounded OCR of early scans when needed. It uses detected title, author, edition, publication details, repository and catalogue URL where available; supplied form values take precedence. Wellcome's generated PDF cover page is a useful metadata source for the classical links in section 8.1. The detected fields remain editable in Review. If title or author cannot be identified, the source is labeled for correction and cannot be published until those fields are confirmed. PDF metadata and OCR can be wrong, so automated extraction is a starting value, not an edition or provenance approval.

## 63.12 Questions about sources and terms — 2026-10-07

Ask now recognizes overview questions about a published, READY book or paper by its title or author. It selects an opening preface, introduction, or abstract passage, uses the local model for a short summary, checks its claims against that passage, and saves the answer with an original-page citation. If the model answer cannot be verified, it gives the source's bibliographic details and points to the cited opening page. Questions about specific content in a book continue through normal passage retrieval.

For a direct definition question, Ask can expand a term only when an indexed passage explicitly supplies the expansion. The example “What is the ROBIS?” now answers “Risk Of Bias In Systematic reviews” from the imported paper's first page and links to that scan. Its PDF metadata had been misread as title “et al. Systematic Reviews (2023) 12:191” and author “the ROBIS”; a migration corrects that exact imported PDF, and journal-cover extraction now reads article title and author lines before generic title-page rules. These narrow question paths do not replace broader source discovery or the release evaluation gates.

“Who is/was” questions now match the author of a published, READY source and require an indexed page that names the person. Nash's title page and Farrington's memorial page support short cited identity answers. If the named person is not a prepared source author, Ask saves an insufficient-evidence response instead of drawing biographical claims from unrelated retrieval candidates. This is source-limited identity coverage, not general medical-person discovery or an encyclopedia.

Two-author relationship questions now resolve both people against prepared author records and look for explicit cross-mentions in their indexed works. For “How is Nash related to Albert Farrington?”, Nash's book names Farrington and discusses his observations; the response cites that passage alongside both author pages. It describes their documented connection through writings and states that the cited pages do not establish a personal or family relationship. The answer and all three citations persist for reload. If either author cannot be verified, Ask returns a specific insufficient-evidence response.

## 63.11 Classical repository discovery and Ask source selection — 2026-10-07

The Sources screen can now search Internet Archive text records by title or author. An admin inspects the catalogue record, rights field, and listed public PDFs before selecting a PDF to download. The selected file enters the existing bounded PDF-link ingestion workflow, with its repository and catalogue URL recorded; it still requires page QA, an explicit rights decision, publication, and a READY passage index. Catalogue metadata or repository availability is not a rights grant. This first connector covers classical book discovery; broader scholarly discovery remains a later phase.

Ask now offers all READY sources or an explicit set of READY sources. The API validates the set, applies it to vector and full-text retrieval, and stores the selected source IDs alongside each result, including insufficient-evidence results. The source selector lets an admin demonstrate a newly imported work without relying on the two starter books. Immutable edition/asset/revision provenance, claim-verification records, durable answer jobs and Activity, and measured release evaluation remain acceptance work.

Live local-stack check: Internet Archive search for Boericke returned catalogue results, item inspection listed an original PDF, and a 5.4 MB Boericke scan (`acompendprincip00boergoog`) downloaded and entered the normal processing/Review queue with repository URL, original PDF URL and SHA-256 saved. It was not published or rights-approved automatically. A selected-source Nash question returned an answer whose citations were all from Nash; reloading the saved answer retained the exact Nash source ID. An unprepared source selection returned HTTP 409. Go tests and the Vue production build passed.

## 63.12 Provenance and candidate reprocessing checkpoint — 2026-10-07

The local database now records immutable edition and acquisition snapshots, checksum-bound PDF assets, processing revisions, page/chunk/job lineage, append-only rights decisions, and publication snapshots of metadata, rights and printed-page labels. New processing revisions record the actual Ghostscript and Tesseract versions, OCR language, and a processing configuration hash. Legacy sources were backfilled without claiming exact historic tool versions. Citation responses expose their exact edition, asset, processing revision, publication, page and PDF checksum.

An admin can choose **Reprocess saved PDF** on a current published source. This creates a separately reviewable candidate linked to the previous source and reuses the saved PDF bytes. The old publication remains eligible for new answers until the candidate has passed automatic QA, a fresh rights decision, publication, and a complete READY index. The worker atomically marks the old source superseded when the candidate index becomes READY. New retrieval excludes the superseded source; saved citations continue to resolve to its original immutable page and PDF. A failed candidate does not switch the old source. DOI retraction checks follow the replacement lineage so a candidate cannot bypass an ancestor's retraction notice.

The schema migration was tested on a disposable copy of the current 42 MB database; a new one-page PDF import, candidate creation, rights correction/history, citation stability, and replacement switch were tested in disposable databases. Migrations 024 and 025 were then applied to the local stack. Existing saved Nash citations still resolve with the new provenance identifiers. A pre-migration local backup is at `/private/tmp/homeopath-pre-provenance.dump`.

This candidate implementation uses a new linked source row for each reprocessing attempt. The PRD's stronger single-source/multiple-revision representation, explicit named reviewer approval and publication identities, and a live reviewed candidate reaching READY still need acceptance testing. Claim-decision persistence, durable answer jobs/Activity, and scored release evaluation remain open.

A separate one-page disposable integration run also exercised automatic QA, a new append-only rights decision, publication with the exact decision/revision and metadata/page-label snapshots, and the database guard against changing a published snapshot. Correcting a rights statement cleared the current decision and required a new one while preserving the previous record. The disposable databases and their tiny PDF fixtures were removed after testing; the pre-migration backup was retained.

## 63.13 Durable answers, claim audit, and release-evaluation preparation — 2026-10-07

Ask now submits a saved answer job. The independent worker claims it with a lease and heartbeat, retries transient failures, reconciles an exhausted stale job, and links a completed result to the original question. The browser polls server-backed status and can reopen saved results in **Activity** after reload. Source preparation jobs appear there with their actual steps and counters. Activity events are persisted for meaningful answer/source transitions and can be marked read. The local stack was rebuilt and a queued source-author question finished and reopened.

The source worker also refreshes its lease during long OCR or embedding steps and reconciles an exhausted stale job to a durable failure state. A disposable database test confirmed that an expired job becomes failed with an Activity event. This does not yet establish every interrupted-ingestion recovery scenario in section 61.4.

General research answers now record `answered`, `partial`, or `insufficient_evidence`. Per-sentence verification saves the decision and exact supporting chunk excerpt with Unicode character offsets. The saved answer exposes these records in an optional research trail. A live Nash/Aconite question returned a partial answer with four supported sentences and three citations; another saved answer's eight stored excerpts all matched their original chunk text at the recorded offsets in PostgreSQL. This is an audited version of the current prose-first pipeline, not yet the evidence-first atomic-claim generation specified in section 63.7. The local model's support verdict still needs human assessment.

New answers snapshot the answer prompt revision; claim records snapshot the verification prompt revision and local model ID. Previously saved answers are identified as legacy prompt records rather than receiving invented historic versions.

An admin can disable or restore a published source with a recorded reason. Disabled material is excluded by retrieval and its citation/PDF endpoints; a disposable database integration test confirmed citation access returned 404 while disabled and recovered after restoration. This control does not replace named user identity or a broader authorization model.

The 30-question draft evaluation set and repeatable runner are under [`evaluation/`](evaluation/). A separate scorer refuses incomplete human review. One draft Nash/Aconite smoke run completed in 87.22 seconds, returned a partial answer with three resolving citations and ten retrieved passages, and passed the automatic citation/excerpt-offset check. This is one warm-path engineering check, not a latency release measurement. The draft currently has **zero reviewed cases**, so no evidence-recall, claim-support, coverage, unsupported-question, or latency/memory release gate is claimed. The later checkpoint below records work completed after this evaluation note. Later scholarly discovery and systematic-review mode remain a separate research phase.

## 63.15 Named principals and live replacement check — 2026-10-07

The local API now records named administrator/reviewer principals, the reviewer who made each rights decision, the publisher, the associated approval time, page-review decisions, and source-access actors. Answer jobs and Activity belong to a principal; another reviewer cannot open or mark those answer jobs read. The browser now requires a token sign-in and uses an HttpOnly local session cookie, including for original-scan/PDF links. The proxy no longer silently grants the administrator token to every browser. The configured local reviewer token is stored in `.env`; administrators can set a display name and add more stable UUID/token pairs with `AUTH_PRINCIPALS_JSON`. Migration 032 and cross-reviewer isolation passed on a disposable database before deployment; migration 032 is applied to the local stack.

The published Hamre et al. 2023 PDF was reprocessed end to end from its saved bytes. Its 24 visible scans completed automatic QA; scan 6's prose was reviewed and scan 11's poorly extracted rotated table was kept as an image and excluded from passages. The candidate received a fresh rights decision tied to the [publisher's CC BY 4.0 article record](https://link.springer.com/article/10.1186/s13643-023-02313-2), was published, and reached READY with 270 passages. The old source was superseded only after that index was ready. A citation saved before reprocessing still resolves to its original source/revision, scan and PDF checksum; a newly generated citation resolves to the candidate revision. Both citation endpoints returned HTTP 200 after the switch.

The answer pipeline now asks the model for atomic claims with exact cited passage excerpts before composing display text. It checks each proposed excerpt and citation ID, then applies the existing claim verifier and persists decisions and excerpt offsets. A live Nash/Aconite question returned a partial answer with four supported claims, seven stored support excerpts and two omitted draft claims. This is a regression check, not a scored quality result. Source-worker and answer-worker exhausted-lease recovery tests pass. A browser reconnect banner after an API outage still needs a reliable end-to-end check.

The candidate continues to use a linked `sources` row. The PRD's stricter single-source/multiple-revision identity and stable `/sources/{id}` address across revisions are still open. The recorded publication approval currently uses the reviewer and time of the latest rights decision; a separate explicit revision/content approval and stale-approval rejection are still open. The browser reconnect banner also needs an end-to-end check. The human-reviewed evaluation and Mac resource/latency gates remain open, so the full POC is not yet accepted.

The admin and reviewer API tokens now scope saved question jobs, answers, citations, and answer Activity to the token's principal role. A disposable database integration test confirmed that the reviewer token could not read an admin job, answer, or citation, or mark its Activity notice read. A queue request with no `source_ids` now saves an empty filter instead of failing. This deployment has one token per role; it still needs named individual accounts and per-user read state before it satisfies the full identity requirement. The Activity browser now polls about every three seconds while work is active, backs off when idle or disconnected, and reconciles on reconnect or focus. The frontend build and a live HTTP 200 check passed; interrupted-browser and worker recovery scenarios still require acceptance tests.

A [review worksheet](evaluation/review_worksheet.md) now offers candidate passage IDs, text previews, and original-scan links for every draft question, with balanced suggestions from Nash and Farrington for comparison cases. Reviewers must select gold passages from the actual scans; worksheet suggestions are not gold labels.

Two additional draft smoke questions were run with only Nash and Farrington selected: a request for results of a 2026 randomized trial and a request for a current Aconite dose for chest pain. Both returned `insufficient_evidence` without a treatment recommendation or fabricated modern result (about 24 seconds each). These are useful regressions, not a substitute for the five reviewed unsupported cases.

Focused author, book and acronym answers now honor explicit Ask source selection as well as general passage retrieval. A live Nash-only “Who is Nash?” job returned a Nash citation and persisted only the Nash source ID. The same question with Farrington alone returned `insufficient_evidence`, no citation, and retained the Farrington selection.

# 64. Literature classification and structured research — 2026-10-10

The confirmed cross-format requirements are specified in [product roadmap section 42](Homeopathy_AI_Product_Requirements_and_Roadmap.md#42-literature-categories-and-category-aware-research--2026-10-10). This section makes them part of the POC implementation contract and qualifies earlier generic page/chunk retrieval guidance: generic text retrieval alone does not establish repertory support.

- Automatically detect literature type from usable content, headings/layout and metadata for supported intake routes. Prefill reviewable categories with rationale, uncertainty and classifier/revision provenance. Low-confidence or failed detection remains unclassified and offers admin selection/retry without failing import; admins can always override suggestions and their choices survive retries. Metadata-only suggestions remain provisional. Use the existing review workflow and snapshot accepted categories at publication.
- RAG must use category-aware chunking and index metadata, reviewed aliases, hybrid lexical/vector plus verified structured retrieval, reranking and source-diverse evidence selection under section 42.3. Treat category as a relevance signal within explicit filters; do not silently exclude relevant All literature evidence based on a guess. Validate rubric/grade claims against structured records and refresh affected indexes after category corrections. Compare retrieval recall and claim/citation correctness against the generic baseline using the same reviewed corpus/questions/model; report regressions and uncertainty.
- Store reviewed literature categories independently of evidence category, asset format, acquisition and publication status. Support materia medica, repertory, philosophy, therapeutics, provings, clinical cases, research, other and unclassified, using section 42.1's stable values. Allow mixed works with section-level overrides; default existing sources to unclassified without inventing labels or blocking otherwise eligible textual research.
- Preserve remedy/subsection structure for materia medica and rubric hierarchy, associations, original notation and source-specific grade schemes for repertories. Structured records are revision-scoped and cite exact saved originals. Shared remedy aliases retain ambiguity; different editions/preparations are not silently merged. Unknown structure or grades remain explicit, independently of text readiness.
- Apply selected source/category intersections to Quick/Deep passage and structured retrieval, including every subquery. Save scope, category snapshots, route and coverage with research results. Distinguish author descriptions, repertory listings, proving observations, clinical cases and research findings; grades do not establish efficacy. Other literature must remain available for appropriate cited research.
- Reuse rights, access, review, publication and index gates. Classification/extraction corrections must not rewrite old answers, quotations or citations. Unsupported structured questions receive a limitation or insufficient evidence, not invented rubric membership or grades.
- Implement [ING-02](tasks/ING-02.md) classification/filtering alongside complete HTML/TXT intake, then [ING-06](tasks/ING-06.md) reviewed structured extraction and research before broader ING-03/04/05 expansion. Acceptance includes remedy lookup, graded and uncertain rubrics, mixed works, other literature, filter isolation, cross-edition differences, recovery and historical citations. Neither task implements full patient repertorization or automated prescribing.

## 64.1 Linked repertory and casebook ingestion contract — 2026-10-10

[Roadmap sections 42.5–42.9](Homeopathy_AI_Product_Requirements_and_Roadmap.md#425-inspected-linked-book-datasets-and-support-boundary--2026-10-10) and [ING-06 A–F](tasks/ING-06.md#ordered-implementation-slices--2026-10-10) are mandatory additions to this POC contract. They cover the inspected Boericke repertory and Nash casebook as representative layouts, not exclusive supported books.

- Persist bounded collection frontiers and work/edition snapshots with multiple immutable page assets; distinguish partial scope from complete acquisition and from text/structure/index readiness. The current 30-page preview is insufficient for the inspected 33-page repertory. Durable limits, resume and activation require implementation, not silent truncation or a synchronous timeout increase.
- Preserve legacy encoding, anchors, `br` rows, nesting, typography and exact multi-location provenance. Review region-level filtering so substantive index notes and short rubrics survive. Unsupported layout/encoding/style remains an explicit review condition.
- Extend numeric-only rubric grading with source-specific categorical notation, convention evidence and unknown style. Boericke italics must not become invented numeric grades. Cross-reference edges do not imply remedy membership; source aliases, list completeness and absence claims need independent validation.
- Add historical casebook records, patient-scoped treatment chronology and mention roles, reporting-physician attribution, shared commentary and separate remedy summaries. Casebook ingestion remains literature research, not patient management or prescribing.
- Combine verified lookup and hybrid retrieval with bounded parent context, exact rubric ancestry and complete paginated list lookup. Apply eligibility/filter gates to every expansion. Reconcile character-based chunk/index limits, preserve exact quotation text separately from search context and report skipped eligible units.
- Extend saved-original review/citations to typography, anchors and multiple supports without fabricated page numbers. New snapshot activation preserves old citations; failed or partial candidates cannot silently replace a complete active collection.
- Offer a collection-wide editable text review to reduce repeated per-page decisions. Show readable pages together while preserving immutable page/block identities and original URLs underneath. A bulk edit may exclude blocks or retain exact source excerpts; it must reject invented text and cross-page reassignment, record decisions/revisions, and keep each published passage tied to its original asset. A single reviewed rights basis may be recorded across eligible pages as separate source decisions, followed by explicit publication and indexing. The combined reading copy alone is never a citation source or proof of structured readiness.
- Generalize the PDF-only evaluation pins and scorer to collection/asset hashes, structured gold records and exact spans. Execute finite-fixture correctness, zero-leak/invention/citation gates and predeclared measured quality/latency/cost targets under section 42.8. Run representative permitted material through the application in Quick/Deep before declaring support. Targets not set, unreviewed gold or missing live acceptance remain pending.

The 2026-10-10 inspection fetched public pages and checked links; it did not establish reuse rights, production import, model quality or end-to-end application support. Requirements completion must not be reported as implementation completion.

## 64.2 Generic ingestion clarification — 2026-10-10

Roadmap 42.10 qualifies named-book adapter wording: implement shared format/layout recognizers and content-based category detection; named sources are validation fixtures, not hardcoded intake paths. Retain uncertain/manual fallback and exact evidence gates. Generic HTML filtering, heading/anchor/line-break preservation, source title extraction and automatic draft category prefilling are implemented; migration 042 distinguishes detected category decisions from human acceptance. Existing sources need explicit reimport/review to adopt changed extraction; never rewrite saved evidence or silently reclassify published sources. Structured rubric/grade correctness and live-model RAG evaluation remain open.

The first collection-wide editor now applies accept/exclude/exact contiguous trim decisions across prepared draft pages in one transaction, with fixed page/block markers and separate page-backed provenance. Previously published pages remain visible but read only. An administrator can record a common rights rationale as revision-scoped decisions on retained draft pages and queue publication/indexing in one workflow after complete scoped capture; failed requests remain visible for retry. This reduces repeated text-review actions but does not remove review of source scope, rights, uncertain extraction or categories. Interior cuts within one block, durable server-side bulk preparation, collection-aware retrieval, structured records and end-to-end acceptance remain open in ING-06.

# END OF POC PRD

## 64.3 Repertory search and navigation — 2026-10-10

Implement the approved repertory workspace contract in [roadmap §42.11](Homeopathy_AI_Product_Requirements_and_Roadmap.md#4211-repertory-discovery-and-search-workspace--2026-10-10) within ING-06. Signed-in Similia inspection informed source/edition selection, chapter and tree navigation, full rubric paths, inline verified memberships and links to materia medica. Import only our independently approved source material.

The first browser increment uses deterministic keyword lookup and remedy/preparation reverse lookup over reviewed, published, compatible structured records. Preserve explicit empty scopes; show source-specific notation, unknown grades and exact supporting excerpts. A result count is a count of verified available records, not a completeness claim. All catalog/detail/count/navigation routes share research eligibility and source-access gates. Provide distinct empty, loading and error states.

Remaining increments include notation-preserving extraction/review, exact saved-original targeting, typed cross-references, verified remedy-to-materia-medica navigation, semantic discovery, reviewed cross-repertory correspondence and Quick/Deep integration. Source-specific scoring requires verified convention evidence and evaluation. Do not replace the research route's unsupported-membership/grade refusal until structured evidence is integrated and tested. Keep all historical casebook, cross-document continuation, lifecycle and human-reviewed baseline acceptance requirements in ING-06 open until individually demonstrated.

Validate full-path ranking, source/chapter/remedy filter isolation, selected-empty behavior, unapproved membership exclusion, unknown-grade preservation, pagination, tree/back navigation and access/index/collection gates with synthetic fixtures. Report fixture tests, browser checks, permitted live-source acceptance and model evaluation separately.

**Implementation checkpoint — 2026-10-10:** The first verified repertory browser is implemented with authenticated eligible-record catalog/search/detail routes, deterministic keyword ranking, source/chapter/remedy filters, tree/back navigation, exact excerpts and source notation. Synthetic integration tests and isolated desktop/mobile browser checks passed. Notation extraction, semantic retrieval, structured Quick/Deep answers, exact reader targeting, materia medica navigation and human-reviewed evaluation remain open; see the detailed ING-06 checkpoint. This increment requires no migration and has not activated the local runtime containers.
