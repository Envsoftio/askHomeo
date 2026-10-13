# ING-07 — Standalone OCR, PDF parsing and visual retrieval comparison

Date: 2026-10-13  
Status: Backlog; task specification only. Script implementation, corpus selection, runs and adoption decision are pending.  
Priority: Evaluate before integrating new document-processing dependencies.  
Authority: User requested a detailed task for later implementation: test the tools with a script, measure workflow improvement, then decide whether to adopt them. See [roadmap section 43.5](../Homeopathy_AI_Product_Requirements_and_Roadmap.md#435-document-processing-comparison-before-adoption--2026-10-13).  
Related work: [PAPER-DEV-01](PAPER-DEV-01.md), [PAPER-DEV-05](PAPER-DEV-05.md), [ING-06](ING-06.md), [evaluation contract](../evaluation/README.md).

## Decision and user outcome

Determine whether Unstructured, Surya, or ColPali-family visual retrieval can make our source preparation and research workflow faster and more accurate. The primary extraction outcome is lower human correction time per correctly accepted page, without increased unnoticed critical errors. Retrieval quality is a separate outcome.

Build an isolated, reproducible command-line experiment first. A useful result may be adopt selectively, retain the current implementation, or defer for inadequate evidence/license/hardware. Installing a package or successfully processing one PDF is not evidence of improvement. This task does not authorize production integration or reduced publication/review controls.

## Baseline to inspect when implementation starts

- `apps/backend/internal/ingest/worker.go`: native PDF text extraction, Ghostscript fallback, Tesseract fallback, extraction provenance and page persistence.
- `apps/backend/internal/pdfocr/ocr.go`: rendering, OCR retries and quality warnings.
- `apps/backend/internal/ingest/textcheck.go`: fresh OCR comparison. The principal word-agreement calculation does not establish reading order or punctuation fidelity.
- `apps/backend/internal/document/jats.go`: structured article extraction; suitable JATS should not be rasterized just to use a new parser.
- `apps/backend/internal/passage/split.go`: exact Unicode passage spans.
- `apps/backend/internal/httpapi/research.go`: existing dense and PostgreSQL lexical retrieval with reciprocal-rank fusion. Do not describe hybrid retrieval as a missing feature.
- `evaluation/README.md`, `scripts/evaluate_poc.py`: existing source-pinned evaluation and human-review requirements.

Record the repository commit and worktree state for the baseline. Reinspect actual behavior: this specification is not a frozen description of future code. Prefer a thin standalone Go harness around existing extraction functions over copying/reimplementing baseline logic. If access requires a refactor, keep it behavior-preserving and test parity; benchmark scripts must not require a live production database.

## Scope and stages

### A. Reproducible standalone runner

Proposed entry point: `scripts/benchmark_document_processing.py`, with helpers under `scripts/document_benchmark/` and a synthetic/example manifest under `evaluation/fixtures/document_benchmark/`. Final paths may follow repository conventions; document any change.

Implement explicit subcommands or equivalent modes:

- `doctor`: report tool/package/model availability, versions, selected device, RAM and runtime prerequisites without downloading weights or invoking providers.
- `validate`: check manifest, input hashes, page ranges, rights/model-use records, gold schemas, output destination and limits without inference.
- `extract`: run the selected baseline/candidate extraction configurations.
- `review-pack`: generate local comparison artifacts and a worksheet for human corrections/timing.
- `score`: compare outputs with reviewed gold and summarize quality, failures and resources.
- `retrieve`: separate opt-in visual retrieval experiment, after extraction benchmarking is usable.
- `report`: generate a reproducible JSON/CSV summary and a readable HTML report with adoption recommendations/evidence gaps.

Illustrative CLI contract (not commands that exist today):

```sh
python3 scripts/benchmark_document_processing.py doctor --device cpu
python3 scripts/benchmark_document_processing.py validate --manifest /private/path/corpus.json
python3 scripts/benchmark_document_processing.py extract --manifest /private/path/corpus.json --engines baseline,unstructured,surya --device cpu --output /private/path/run-001
python3 scripts/benchmark_document_processing.py review-pack --run /private/path/run-001
python3 scripts/benchmark_document_processing.py score --run /private/path/run-001 --gold /private/path/reviewed-gold.json
python3 scripts/benchmark_document_processing.py retrieve --manifest /private/path/corpus.json --queries /private/path/reviewed-queries.json --device cpu --output /private/path/retrieval-001
python3 scripts/benchmark_document_processing.py report --run /private/path/run-001
```

Resolve options and resource caps into an immutable run manifest before starting. Use isolated, pinned environments; do not add heavy Python/model dependencies to the production Go API image. Separate CPU and accelerated dependency configurations where necessary. Document explicit model preparation/download steps, download sizes where available, required license decisions and offline execution afterward. No silent network fallback or auto-installation during a scored run.

### B. Corpus and gold labels

Start with a 10–20-page smoke corpus for runner mechanics. Then propose 200–300 representative pages across multiple documents for the diagnostic comparison; finalize strata and sample sizes before scoring. These are experiment sizes, not sufficient evidence for general release claims.

Required strata: clean digital papers, mixed text/scan PDFs, historical low-quality scans, multi-column prose, tables with headers/units/footnotes, figures/captions, and repertory/materia medica pages with meaningful abbreviations/typography. Add multilingual pages only where relevant and label language explicitly. Include currently passing pages as well as suspected failures, blank pages, rotation/cropping problems and unsupported content. Do not select only examples on which a candidate looks good.

Manifest fields must include:

- Dataset version; document ID; local asset path and SHA-256; work/edition/version and acquisition provenance; approved uses and permission basis.
- Explicit zero-based PDF page indexes, printed labels when known, page strata and language; no fabricated printed labels.
- Development versus held-out split assigned by whole document (group editions/duplicates where leakage is possible); held-out material excluded from tuning.
- Optional expected regions/tables, gold-file references, reviewer identity/date/status and an explicit distinction between synthetic, provisional and human-reviewed labels.
- For retrieval: fixed corpus membership, query ID/text, selected source scope, relevant page and passage/region IDs, answerable/unsupported status and reviewer records.

Use locally available permitted copies; no bulk acquisition or upload to hosted services by default. Keep source PDFs, page renders, OCR, gold and reports outside version control unless cleared for sharing. Commit only synthetic redistributable fixtures and non-sensitive documentation. Gold transcription must be checked against original pixels. Native text, JATS and model outputs can assist annotation but are not automatically ground truth; do not assume JATS/PDF versions match.

### C. Extraction experiment matrix

| Configuration | Question answered |
|---|---|
| Current extraction plus its existing QA | What quality, failures and preparation cost do we have today? |
| Unstructured `fast` on eligible text PDFs | Does its partitioning improve useful structure versus the native baseline? |
| Unstructured `hi_res` with explicit table settings | Does layout/table extraction improve complex pages enough to justify cost? |
| Surya pinned OCR/layout configuration | Does alternate OCR/order reconstruction reduce meaningful errors and correction work? |
| Selective routing policy, tuned only on development data | Can we capture benefits without processing every page with every engine? |

Pin package versions, model revisions/checksums, OCR languages, render DPI, preprocessing, extraction strategy, precision, concurrency and seeds where supported. Record the actual strategy/device used, including fallback. Unsupported candidates must be reported explicitly rather than silently replaced. Compare the same source pages; preserve each engine's raw output alongside the normalized comparison format. Report native settings first; any tuned configuration must be separately labelled.

The normalized format must preserve asset/page identity, block IDs/types, reading order, text, image dimensions, bounding boxes and their coordinate system, table cells/header links/spans/units/footnotes when available, confidence with its engine-specific meaning, warnings and coverage gaps. Missing geometry or structure stays null/unsupported. Preserve exact raw-to-normalized mappings where feasible; inability to support our locators is an integration risk to report. Do not let aggressive normalization hide lost punctuation, numbers, negation or typography.

### D. Runtime controls and measurement

CPU is the default experiment. Explicitly select any CUDA or Apple Metal/MPS run; integrated-GPU measurements must not be labelled CPU-only. No rented GPU or paid API without a separately agreed numerical spending limit. Current task creation authorizes no execution or spending.

Before running, set page/document limits, per-page and total deadlines, output-byte caps and conservative concurrency. Process candidates in resource-isolated/sequential trials for fair comparison; do not benchmark competing engines simultaneously on the same hardware. Record CPU model/cores, RAM, OS, acceleration/device/VRAM if used, precision, warm/cold start, model-load time, render time, inference time, end-to-end time, peak memory and failures. Record sampling limitations for process-tree memory and unknown costs as unknown.

Cache by asset hash, page, engine/model revision and full effective configuration. Distinguish cached hits from measured inference. Support interruption/resume and atomic per-page writes; retries have attempt counts and finite limits. A run with failed/missing pages is partial, with denominators shown. Preserve failure outcomes in summaries instead of scoring only successes. No external service transmission or telemetry of corpus content; disable optional telemetry and document residual runtime network requirements.

### E. Human review and scoring

Generate a local HTML comparison pack with original page/region, candidate output, reading-order overlay and table view, plus a CSV/JSON worksheet for correction time, accepted/corrected/rejected outcome, critical errors and comments. Escape extracted content and render it inertly; no scripts/remote fetches from document content. Region views must retain access to the full page.

Blind engine labels and counterbalance page/candidate order where practical. Record annotation and timing protocol; repeated exposure to the same page can artificially lower correction time. Keep gold creation time separate from candidate correction time. Have a second reviewer adjudicate a sampled subset and critical disagreements where available; report limitations if only one reviewer is available. Model judgments cannot substitute for human acceptance labels.

Report paired results and sample sizes overall and per stratum:

- Character/word error rates against reviewed transcriptions, with explicit Unicode/whitespace normalization rules and separate punctuation-sensitive checks.
- Missing/duplicated text and coverage; reading-order correctness using reviewed region order.
- Critical-token errors: negation, decimal points, inequality signs, units, result values, remedy abbreviations and source-specific notation. Define error severity before scoring.
- Table cell content and header association accuracy; merged cells, missing footnotes and unsupported tables separately.
- Review minutes per accepted page, rejection rate, automated runtime, peak resources and total accepted-page cost with declared reviewer-rate assumptions if monetized.
- If simulating reduced-review routing: false acceptance among pages marked low-uncertainty, review referral rate and audited sample size per class. Do not equate engine confidence or engine agreement with verified correctness.

Use document-level paired uncertainty estimates where feasible, avoiding a claim of independent samples for adjacent pages. Missing gold means unscored, not passed. Zero observed critical errors is not proof of zero risk. Keep an error gallery of representative failures and wins linked to local source regions.

### F. Separate ColPali-family retrieval experiment

This is an optional second stage of the same evaluation task, not an OCR competitor. Extraction comparison can yield a decision even if visual retrieval remains explicitly pending.

Recheck maintained inference libraries and exact checkpoint licenses before choosing models. Compare a pinned ColPali-family checkpoint against our existing lexical+dense retrieval baseline using the same frozen eligible source set. Reuse existing retrieval logic in an isolated harness/disposable environment where feasible; identify approximations and do not call a generic replacement our production baseline.

Use reviewed queries (initial proposal: 80–120 across multiple documents, including unsupported queries) and report page Recall@10 separately from passage/region Recall@10, ranking quality, visual-only relevant hits, query latency, indexing time and storage. Map baseline passages to pages for fair page-level comparisons. Compare (1) baseline, (2) visual reranking of baseline candidates and (3) independent visual candidates fused with baseline ranks. Reranking alone cannot recover pages absent from the original candidate set. Avoid combining incompatible raw model scores; record the fusion rule and parameters.

Record page-image rendering, model/preprocessor revision, vector count/dimensions/precision and corpus hashes. Query embedding latency remains part of serving cost even with precomputed page embeddings. Use bounded local scoring before adding a vector database. A retrieved page is not an exact supporting quotation; report whether it resolves to reviewed passages/regions. Similarity thresholds alone do not establish correct abstention. No generated clinical answers are required in this task; answer-quality experiments require the existing evaluation contract and any applicable spend limit.

## License and upstream checkpoints

The following are research observations from 2026-10-13, to reverify against the exact selected versions before execution; they do not approve use:

- Unstructured OSS and its commercial platform are distinct. Pin OSS strategies/dependencies; do not attribute platform benchmark claims to the local package. [Repository](https://github.com/Unstructured-IO/unstructured), [partitioning](https://docs.unstructured.io/open-source/core-functionality/partitioning), [chunk provenance](https://docs.unstructured.io/open-source/core-functionality/chunking).
- Surya's current code is Apache-2.0, but weights have separate modified OpenRAIL terms. Inspect output/share-alike, attribution, commercial thresholds and competing-service restrictions for this experiment and subsequent corpus use; a revenue threshold alone is not blanket permission. If suitability is unresolved, mark Surya deferred and continue eligible baselines. [Repository](https://github.com/datalab-to/surya), [model license](https://huggingface.co/datalab-to/surya-ocr-2/blob/main/LICENSE).
- ColPali maintainers currently recommend Sentence Transformers for new integrations. Backbone and adapter licenses differ; do not rely on a repository/model badge alone. [Original article](https://huggingface.co/blog/manu/colpali), [maintainer guidance](https://github.com/illuin-tech/colpali), [ColPali model card](https://huggingface.co/vidore/colpali-v1.3), [ColQwen2 card](https://huggingface.co/vidore/colqwen2-v1.0), [ColQwen2.5 card](https://huggingface.co/vidore/colqwen2.5-v0.2).

Record code and weight versions/licenses, dependencies, permitted input/output uses and deployment constraints in the experiment report. No model is selected for adoption by this task.

## Decision gates and deliverables

Before held-out scoring, finalize numerical quality/non-inferiority margins, runtime/resource limits and the minimum worthwhile review-time saving. A proposed starting target is at least 30% lower median review time per accepted page on the intended strata, with no unacceptable critical-error regression. This is a proposal, not a user-approved release threshold. If the sample cannot support the agreed conclusion, recommend more evidence rather than acceptance. Account for rejected pages and work shifted to other stages so lower correction time is not misleading.

Deliver:

1. Standalone runner, pinned dependencies, example manifest, documented commands and meaningful synthetic tests.
2. Private, hashed experiment manifests, raw/normalized outputs, review packs and reviewed gold references.
3. JSON/CSV metrics and HTML comparison report with failures, per-layout results, hardware, cold/warm timing, costs and uncertainty.
4. Decision table per engine and page class: adopt selectively / retain baseline / defer, with evidence, license constraints and remaining risks.
5. A proposed follow-up integration task only if justified, identifying adapter, provenance, reviewer UI, citation, resource and regression work. Keep implementation approval separate from a promising benchmark.

## Acceptance checklist

- [ ] `doctor`/`validate` work without inference or implicit downloads; invalid hashes/pages/configuration fail clearly.
- [ ] Baseline behavior is faithful to the inspected repository, with parity evidence for any harness/refactor.
- [ ] Baseline, Unstructured and Surya adapters are implemented/tested or explicitly deferred with reasons; unsupported optional engines do not invalidate completed comparisons.
- [ ] Synthetic tests cover output contracts, coordinate/page numbering, timeout/failure reporting, cache invalidation, resume, scoring with missing gold, denominator accounting and inert report rendering.
- [ ] A bounded real-document smoke run records actual hardware, artifacts, versions and limitations; fixture success is not quality acceptance.
- [ ] Reviewed diagnostic/held-out comparisons and human timing are completed, or remain visibly pending without fabricated scores.
- [ ] Optional visual-retrieval stage has separate evidence and status; page recall is never reported as passage or answer correctness.
- [ ] Reports provide a defensible adoption/defer decision without changing live corpus, database, publication policy or production dependencies.
- [ ] Completion documentation distinguishes runner implementation, actual experiments, human review and any remaining adoption decision.

## Non-goals and creation evidence

No production API/worker/UI integration, schema migration, corpus rewrite, bulk download, automatic rights/text approval, publication-policy change, replacement embedding model, GPU purchase, paid provider run, or assertion of clinical/research accuracy. Existing PAPER-DEV-01, PAPER-DEV-05 and ING-06 acceptance remains independent.

At task creation only this specification and its roadmap link were added. No benchmark script, dependency installation, model download, document processing, human review or measured comparison has been performed under ING-07.
