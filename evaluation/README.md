# Evaluation dataset and live runner

CORE-02 has no committed question fixture. Build the dataset from a complete source that has passed the intended environment's acquisition, page, content, rights, publication and index reviews. Do not reuse the CORE-01 excerpt or copy its passage IDs into a full-source dataset.

## Dataset contract

Pass a JSON file to the runner with a nonempty `version`, a `sources` object, and a `cases` array. Each source key identifies one real source and records its `id`, `pdf_sha256`, `source_asset_id`, `processing_revision_id`, and `index_run_id`. Record the work, edition, acquisition route and rights evidence alongside these pins. Obtain the source fields from `GET /api/v1/sources/{id}` and the active run ID from `GET /api/v1/sources/{id}/index-status`. Reprocessing or reindexing needs a new dataset version and review of affected gold labels.

Each case needs a unique `id`, `category` (`single_source`, `comparison`, `author_edition`, or `unsupported`), real source-specific `question`, nonempty `source_keys`, `held_out` boolean, and `mode` (`quick` or `deep`). A reviewed case also needs `review_status: "reviewed"`, `reviewer`, `reviewed_at`, and, unless it is unsupported, confirmed `gold_chunk_ids` and `expected_points`. Confirm each gold passage on the original PDF page and check its context. For unsupported cases, confirm the selected source scope cannot answer the question. Keep held-out cases away from tuning.

The worksheet tool accepts the same dataset with source pins and suggests candidate passages. Run it from `apps/backend` with `DATABASE_URL` set and explicit `-input`, `-output` and `-markdown` paths. Its JSON and Markdown outputs can contain private source text; keep them outside version control. Suggestions are never gold labels until a person checks them.

## Run against an environment

Use `python3 scripts/evaluate_poc.py --dataset /path/to/reviewed.json --dry-run` to validate the manifest without network or model calls. `--smoke` permits an incomplete category mix for a small diagnostic; it does **not** permit unreviewed cases in a live run. `--case ID` limits the selected cases. A scored release run has no fixed question count but requires all four categories, held-out cases, a complete run, and human claim and expected-point judgments.

For a live run, set `EVALUATION_API_BASE` to the staging or production HTTPS origin and `EVALUATION_API_TOKEN` to an administrator token, then pass `--dataset` and `--output`. Agree on the source and numerical spending limit first. The runner checks pinned source and active index identity before jobs and before each case. It writes a private checkpoint as soon as an answer job ID is returned. Use `--resume` with the same dataset, selection and output after interruption; the runner polls saved jobs instead of submitting them again. A new run uses a new output path.

Review original-page citations, retrieved evidence, supported claims, expected-point coverage, saved-answer reopening, job failures and provider-reported usage. Unknown cost stays unknown. Generated reports contain source passages and model output; keep them outside version control. `python3 scripts/score_evaluation.py /path/to/results.json` refuses a smoke or incomplete report. A small passing set does not establish broad quality.

The runner and index-status API changes need no schema migration or reindex. Full-source and staging acceptance are still pending until a permitted source and live review are completed.
