# CORE-02 — Establish a fresh-source evaluation baseline

**Status:** Ready for implementation. Live source acceptance and paid runs depend on a permitted source, reviewed rights, and a new explicit spending limit.

**User outcome:** A reviewer can take one complete, permitted source through intake and page review, prepare a small set of source-specific questions with reviewed evidence, and run a diagnostic that shows citation and answer failures. The diagnostic must not be presented as release-quality evidence.

**Related requirements:** ASK-02, ASK-03, ASK-05, ASK-07, TRUST-01–03; PRD sections 21.1, 22, 36.1–36.4, 38 and 39. CORE-01 provider and local smoke work is the technical starting point; its staging acceptance remains separate.

## Observed baseline and gap

- [CORE-01's local verification](../docs/core-01-local-verification.md) used a temporary 24-page excerpt of the user-supplied Boericke PDF. The full 1,136-page asset, staging workflow, wider-use rights and research quality were not verified. The excerpt source was disabled after testing.
- [`scripts/evaluate_poc.py`](../scripts/evaluate_poc.py) requires the full 10/10/5/5, 30-case distribution even with `--smoke`. It resolves sources by title, so duplicate editions can be ambiguous. The default `evaluation/questions.draft.json` is absent from this checkout.
- [`scripts/score_evaluation.py`](../scripts/score_evaluation.py) is the release scorer and requires reviewed gold and human claim/point judgments. A small diagnostic must not weaken those gates.
- The first full-source intake and an evaluation dataset tied to exact source revisions are still missing.

## In scope

1. **Prepare one real source.** Choose a complete PDF with a recorded acquisition route, exact work/edition, file checksum and rights decision for the intended environment. The user-supplied Boericke PDF is a candidate, not preapproved for staging or broader use. Exercise the existing upload, extraction/OCR, uncertain-page review, content and rights gates, publication and compatible READY index. Record sampled page-to-PDF mapping and any OCR defects; fix blockers found in this workflow or create focused follow-up tasks for defects outside this slice.
2. **Create the first reviewed cases.** Add a versioned `evaluation/` dataset and short reviewer instructions. Start with 3–5 questions grounded in the selected source, including an answerable question and an unsupported question. Store the exact source asset and processing/index revision with each case or dataset manifest. Use the existing evaluation worksheet tool to suggest passages and expected points, but require a person to confirm gold passage IDs, expected points, source scope and review status. Keep unreviewed suggestions visibly unreviewed.
3. **Make the runner useful for a small baseline.** Allow `--smoke` and `--dry-run` to validate and run a selected small dataset. Preserve strict 30-case distribution and human-review requirements for release mode and the release scorer. Resolve the intended source by stable identity and verify its current asset/revision before paid jobs; reject missing, superseded, inaccessible or drifted sources with a clear error. Preserve explicit source selection for each answer job and export citation integrity, retrieved passages, statuses, latency and human-review fields. Report unknown or unreviewed measures as unknown.
4. **Run and review the diagnostic.** After a numerical budget is agreed, run the small set against the prepared source in the intended environment. Open cited original PDF pages, reopen saved answers, review supported claims and expected-point coverage, and record recurring retrieval/OCR/model failures and actual reported usage. Preserve the diagnostic output and a concise evidence record without secrets or private source text in the repository.

## Out of scope

- Completing or relaxing the 30-case release contract: 10 single-source, 10 comparison/multi-passage, 5 author/edition-constrained and 5 unsupported cases, with held-out examples and human review.
- Claiming cross-source or difficult-scan performance from one source; broader representative-corpus intake and release scoring follow this slice.
- Reusing old Nash/Farrington IDs or labels, bulk ingestion, automatic rights approval, model/reranker experiments, production rollout or a new analytics interface.

## Data, access and change boundaries

- Store dataset definitions and review instructions in `evaluation/`. Keep generated results and any source extracts out of version control unless specifically cleared for sharing. Do not put API keys, private source text or provider responses containing private data in a committed fixture.
- Prefer existing source, citation and answer APIs. Any schema or UI change should be limited to a blocker found during the full-source flow and documented with its migration/reindex effects. A change to embedding model, dimensions, input style or revision requires a new compatible index; preserve existing citation lineage.
- The prior US$0.50 authorization covered the local CORE-01 test only. A CORE-02 live run needs its own budget and permitted source decision. Record unknown embedding cost as unknown; do not infer a billed total from chat cost.

## Acceptance examples and failure cases

- A complete permitted source passes review, is published and reaches a compatible READY index. At least one reviewed page maps to the correct original PDF page; flagged OCR pages have an explicit accept/correct/exclude decision.
- A 3–5-case smoke dataset dry-runs, including at least one reviewed answerable case with gold passage and expected points and one unsupported case. `--smoke --case <id>` runs only that case. A normal release run and scorer reject the small dataset.
- The runner rejects an incorrect source checksum/revision, duplicate or missing source, unready index and unknown case ID before submitting an answer job. It never silently substitutes another edition with the same title.
- Answerable output includes resolving original-page citations and reviewable claims; an unsupported case reports insufficient evidence or is marked as a failure for review. Saved answers reopen without another model call. Failed jobs, missing citation data and unreviewed judgments remain visible in the export.
- The completion record names the source ID, asset checksum, processing/index revision, page-review decisions, dataset version, case/job IDs, reviewed findings, provider-reported usage and known/unknown costs. It explicitly states whether local and staging acceptance passed.

## Verification and release

- Add focused offline checks for small-dataset validation, source identity/revision mismatch, case filtering and release-gate preservation. Run the existing evaluation scripts in dry-run mode; run affected Go checks or the frontend build only if those layers change.
- Use a fresh complete source for manual acceptance after rights and budget are settled. The local excerpt's smoke result is historical evidence, not the CORE-02 acceptance fixture.
- Keep the dataset and runner change reversible. Before any staging schema migration, back up the database and PDF assets together; retain the previous application image and non-secret configuration. Never delete an existing source or index as a shortcut.

**Following slice:** Expand to the representative clean book, difficult scan and papers; complete the versioned 30-case set, held-out review and release thresholds in PRD section 38.
