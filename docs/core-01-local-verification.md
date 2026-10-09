# CORE-01 local verification — 2026-10-09

Scope: local Docker stack only. This is a provider and first-source smoke test, not staging acceptance or a scored research-quality evaluation.

## Source and preparation

- User-supplied PDF: *Pocket Manual of Homoeopathic Materia Medica*, William Boericke, eighth edition, Boericke & Runyon, New York, 1922. The original file has 1,136 PDF pages and SHA-256 `963ef178cf3f608b85f5cfd5320770532526e2008982fdeb5fd53c2d79f68a9c`.
- To bound time and cost, the local test used a temporary first-24-page excerpt, SHA-256 `a1995ab76a2d263f1f113a070a9f6feeaa638aafe9b7d0bd9abc59efd9023d27`. The extra workspace copy was removed after testing. It is not a full-book ingestion test.
- Local source ID: `046b43f2-4379-491b-bc89-c667a6fe2723`. Metadata records the edition and excerpt. The rights decision is restricted to local testing; staging or public use needs a fresh rights review.
- Ingested 24 pages. Automatic QA initially flagged ten pages; blank/title scans were excluded and four text scans were visually sampled and accepted with OCR caveats. After review: 18 checked text pages, zero unresolved suspect/unclassified/missing pages.
- Published and indexed 87 eligible passages. Active index status: READY with `BAAI/bge-m3`, 1,024 dimensions, plain input, revision `deepinfra-BAAI-bge-m3-plain-v1`.
- After the smoke test, the local excerpt source was disabled to exclude it from real Ask searches while retaining the audit record. Its saved citation links will be unavailable until an administrator restores the source.

## Answer checks

All three jobs explicitly selected the local excerpt source.

| Check | Job ID | Result | Citation resolution |
| --- | --- | --- | --- |
| Quick: eighth-edition changes | `07b0a0f5-03c5-466c-8168-dac34643b085` | Finished, `partial`; two supported claims | E1 opened original excerpt PDF page 4 |
| Deep: same question | `9549e903-11b3-4176-ab6e-b563fe3a6ebd` | Finished, `partial`; four supported claims | E1/E3/E4 opened original excerpt PDF pages 4/8/4 |
| Quick: supposed 2025 trial | `361bb549-0d69-4eba-a9dd-79ade1ae4e0f` | Finished, `insufficient_evidence` | No citation |

The first Quick attempt failed because the model's grouped quotation check did not satisfy the verifier's JSON contract. The worker was stopped before retry; DeepInfra GLM-5.3 JSON mode was enabled and tested. The retry finished. Saved Quick and Deep answers were reopened through the API; no model regeneration was needed. The citation API returned checksum-validated source passages and PDF page links.

## Recorded usage and limits

- User-authorized maximum for this local test: US$0.50.
- Chat: 11 calls, 9,690 prompt tokens, 1,427 completion tokens, US$0.008318125 provider-reported estimated cost. This includes the failed Quick attempt and retry.
- Embeddings: 12 calls, 8,354 input tokens. DeepInfra did not return an estimated cost for these calls, so storage and reports correctly show unknown cost. The token volume is small, but the report does not claim a billed total.
- The model-call table and admin answer report retain per-attempt outcome, model, tokens, duration, request ID when returned, and cost origin. No keys, prompts, or response bodies are stored there.
- The full 1,136-page PDF, staging deployment, rights for wider use, and the reviewed evaluation remain unverified. Some accepted excerpt pages retain OCR errors; this smoke test does not establish clinical reliability.
- The US$0.50 ceiling was monitored between test steps; the application does not yet enforce a hard per-job dollar limit. Output tokens, request timeouts and retry counts bound individual operations.

## Staging acceptance still required

1. Back up the staging database and PDF assets together before deploying migration `034_model_calls.sql`; retain the previous image and non-secret settings for rollback.
2. Configure the same non-secret API/worker models and an explicit embedding revision on staging. Keep the DeepInfra key server-side and verify both processes receive it.
3. Review the full source's exact asset, acquisition and wider-use rights before publication. The local excerpt's rights decision must not be reused as staging approval.
4. Import and review the selected staging source, wait for a compatible READY index, then repeat Quick, Deep, unsupported-question, citation-opening and saved-answer checks. Reconcile provider-reported usage and unknown costs in the admin report.
