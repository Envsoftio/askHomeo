# Evaluation dataset and live runner

CORE-02 has no committed question fixture. Build the dataset from a complete source that has passed the intended environment's acquisition, page, content, rights, publication and index reviews. Do not reuse the CORE-01 excerpt or copy its passage IDs into a full-source dataset.

## Dataset contract

Pass a JSON file to the runner with a nonempty `version`, a `sources` object, and a `cases` array. Each source key identifies one real source and records its `id`, `source_asset_id`, `processing_revision_id`, and `index_run_id`. PDF pins use `pdf_sha256`; HTML/TXT pins set `document_format` to `html` or `txt` and use `asset_sha256`. A linked page should also pin `collection_snapshot_id`. Record the work, edition, acquisition route and rights evidence alongside these pins. Obtain the source fields from `GET /api/v1/sources/{id}` and the active run ID from `GET /api/v1/sources/{id}/index-status`. Reprocessing, replacement or reindexing needs a new dataset version and review of affected gold labels.

Each case needs a unique `id`, `category` (`single_source`, `comparison`, `author_edition`, or `unsupported`), real source-specific `question`, nonempty `source_keys`, `held_out` boolean, and `mode` (`quick` or `deep`). A reviewed case also needs `review_status: "reviewed"`, `reviewer`, `reviewed_at`, and, unless it is unsupported, confirmed `gold_chunk_ids` and `expected_points`. Confirm each gold passage against its saved PDF scan or HTML/TXT original and check its context. For unsupported cases, confirm the selected source scope cannot answer the question. Keep held-out cases away from tuning.

The worksheet tool accepts the same dataset with source pins and suggests candidate passages. Run it from `apps/backend` with `DATABASE_URL` set and explicit `-input`, `-output` and `-markdown` paths. Its JSON and Markdown outputs can contain private source text; keep them outside version control. Suggestions are never gold labels until a person checks them.

## Run against an environment

Use `python3 scripts/evaluate_poc.py --dataset /path/to/reviewed.json --dry-run` to validate the manifest without network or model calls. `--smoke` permits an incomplete category mix for a small diagnostic; it does **not** permit unreviewed cases in a live run. `--case ID` limits the selected cases. A scored release run has no fixed question count but requires all four categories, held-out cases, a complete run, and human claim and expected-point judgments.

For a live run, set `EVALUATION_API_BASE` to the staging or production HTTPS origin and `EVALUATION_API_TOKEN` to an administrator token, then pass `--dataset` and `--output`. Agree on the source and numerical spending limit first. The runner checks pinned source and active index identity before jobs and before each case. It writes a private checkpoint as soon as an answer job ID is returned. Use `--resume` with the same dataset, selection and output after interruption; the runner polls saved jobs instead of submitting them again. A new run uses a new output path.

Review original-page citations, retrieved evidence, supported claims, expected-point coverage, saved-answer reopening, job failures and provider-reported usage. Unknown cost stays unknown. Generated reports contain source passages and model output; keep them outside version control. `python3 scripts/score_evaluation.py /path/to/results.json` refuses a smoke or incomplete report. A small passing set does not establish broad quality.

The runner and index-status API changes need no schema migration or reindex. Full-source and staging acceptance are still pending until a permitted source and live review are completed.


## Planned linked-book evaluation extension — ING-06 F

The dataset contract and runner above currently assume PDFs; they do not yet validate repertory HTML or casebook collections. [ING-06 F](../tasks/ING-06.md#f-integrated-evaluation-and-release-evidence) must add format-neutral per-asset hashes, collection manifest pins, adapter versions, structured gold records and multiple exact supporting locations, while retaining PDF compatibility. Roadmap section 42.8 defines the required measured release gates. Do not use fake PDF hashes or page positions to fit HTML into the old contract.

The [Homeoint inspection manifest](fixtures/homeoint-inspection-2026-10-10.json) is metadata from public linked-page inspection, not runner input, human-reviewed extraction gold, a rights decision or evidence that the application supports these sites. It contains no book body text. Raw source snapshots and future answer outputs remain outside version control unless cleared for sharing. The recorded HTTP byte hashes identify inspected snapshots, not publisher authenticity. Live re-fetches may differ and require new pins/review.

Required cases include nested and `br`-separated rubric rows, ordinary/italic/unknown notation, source-convention support, cross-reference-with-list versus reference-only, complete paginated lists, ambiguous aliases and negated/unsupported membership; casebook multi-patient/sequence/unsuccessful-treatment distinctions, shared commentary and source-attributed outcomes; mixed-category questions; scope/access/revision isolation; and saved citations after source change. For both Quick/Deep report retrieval recall/ranking, list completeness, supported claims, expected aspects, abstention, latency and cost, with per-layout sample sizes and held-out labels. Retain the inherited Recall@10 ≥ 0.90, human claim support ≥ 0.95 and expected-point coverage ≥ 0.90 thresholds; predeclare additional layout/list-completeness targets before the run; unconfigured targets mean pending acceptance. See ING-06 F for ownership rather than treating this as implemented tooling.
