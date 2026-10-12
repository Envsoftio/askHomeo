# PAPER-DEV-05 — Hybrid library preparation and question-triggered paper discovery

Date: 2026-10-13  
Status: Confirmed direction; implementation and acceptance pending.  
Authority: [Roadmap section 43.4](../Homeopathy_AI_Product_Requirements_and_Roadmap.md#434-confirmed-hybrid-preparation-strategy--2026-10-13).  
Dependencies: PAPER-DEV-01 discovery/acquisition/evidence contracts and existing durable research jobs. Book structure remains owned by ING-06.

## User outcome

A researcher can use the prepared library and discover additional papers from a question without manually restarting the workflow in Find papers. Core books and frequently used papers are processed in advance. Missing papers are prepared on demand within declared scope and limits; eligible saved revisions are reused. References appear promptly, with a clear distinction between discovered publications and usable answer evidence.

## Baseline and scope

Find papers already provides provider search, saved references and authorized acquisition into the shared review pipeline. Ask currently uses reviewed, published sources with compatible READY indexes. Question-triggered discovery, acquisition requests and continuation after preparation are pending; this task must not claim that the existing saved-search provenance alone implements them.

1. Define and persist the research scope, provider/query/filter choices and finite paper, request, byte, time and cost limits before discovery. Make library-only versus additional-paper discovery visible. Do not widen selected sources or providers silently. Retain provider-specific rate limits and optional-provider failure isolation.
2. Reuse existing work identifiers, source assets, processing revisions, acquisition jobs and saved-search provenance. Reuse a matching eligible saved revision/index without unnecessary downloads or embeddings. Conflicting identities, editions, versions and changed content follow existing review/revision rules; title similarity alone cannot merge works.
3. Connect the question to bounded discovery and preparation jobs. Users may search/save within their permissions; a user without ingestion privileges can request preparation for an authorized role, not invoke administrator operations. A queued acquisition or successful parse does not approve rights, text or publication.
4. Persist progress and actionable failures across restart, cancellation and retry. Show discovered, saved, unavailable, acquiring, awaiting review, indexing and eligible counts separately. Missing configuration, provider outages, denied access, unsupported extraction and empty results remain distinct states.
5. Continue answering only from the selected eligible evidence. Pending references may be displayed as references, never as supporting findings. With no eligible evidence, show the preparation/review or unavailable state. With partial evidence, state coverage limits. Once additional sources become eligible, offer continuation as a new saved answer run; preserve earlier answers and exact citations.
6. Save question-to-search-to-acquisition-to-source/revision lineage, limits and coverage with the answer. Recheck rights/access, publication, notices and index compatibility when resuming and before retrieval. Do not substitute a refreshed revision into a historical citation.
7. Retain extraction limitations for tables, images, formulas and supplements. Review must expose the saved original and missing content. A Markdown editor/preview is a candidate improvement, not proof of structured table extraction or image interpretation; rendering and richer extraction need their own verified implementation scope.

## Acceptance

- Prepared-library case answers using eligible saved revisions without a provider fetch or redundant processing.
- A new permitted paper follows question → discovery → saved reference → authorized acquisition → review → publication → compatible index → new answer → exact saved-original citation. Record each stage; human review is not simulated as real approval.
- A mixed case reuses an existing source and prepares a missing one without duplicates; the initial limited answer and later expanded answer retain separate provenance.
- Metadata-only, rights-pending, unreviewed, disabled, stale/incompatible and inaccessible sources never enter answer evidence. Verify researcher/admin authorization, selected-empty scope and cross-user isolation.
- Provider outage, no candidate, wrong article, extraction failure, cancellation, lease expiry and retry retain visible reasons and do not cause duplicate jobs/assets or silent scope expansion. Concurrent questions requesting the same paper preserve user-owned provenance while reusing shared eligible content.
- Browser checks cover progress, pending-review state, partial evidence, retry/cancel and reopening both saved answers/citations. Run meaningful provider/recovery/authorization tests with a disposable database and preserve PDF/HTML/TXT/JATS/ALTO behavior.
- Record latency/cost, actual asset/revision IDs and live versus fixture evidence. New paid-model evaluation requires its applicable numerical budget; synthetic answers are not human-reviewed research-quality results.

## Boundaries and completion evidence

No blanket acquisition of all papers, arbitrary book crawling, review bypass, broad OpenAlex acquisition, CMS, alerts or paywall bypass. Reuse the current schema where practical; any necessary migration is additive with the next available number. PAPER-DEV-01's remaining pilot/quality acceptance and ING-06's book requirements remain open independently.

At creation only the decision and task were documented. No runtime implementation, migration, browser test, live acquisition or model evaluation was performed for this task. Update this file, roadmap and README with actual behavior and verification when implemented.
