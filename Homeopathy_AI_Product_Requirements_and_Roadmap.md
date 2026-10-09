# Homeopathy AI — Product Requirements and Development Direction

Date: 2026-10-08  
Status: Consolidated product requirements, scope assessment, source catalogue, competitor review, public website requirements, and task-ready roadmap. India-first launch and all seven product modules are confirmed direction; sequencing, collection targets, pricing, and release dates remain proposals unless explicitly identified otherwise. No future feature is marked implemented by this document.

## 1. Purpose and document authority

Capture the product discussion through 2026-10-08, including the seven-module scope audit, India-first launch decision, international ambition, 56-source collection catalogue, hosted production models with pluggable development inference, and inherited POC UX. Preserve the existing clarification that **a patient case is optional**. Future work must support both general knowledge questions and a later structured case workflow using a shared, source-grounded knowledge foundation.

Read this document alongside:

- [POC PRD](Homeopathy_AI_Source_Ingestion_POC_PRD.md): implementation requirements and acceptance gates for the current POC, especially sections 5, 56, 58–61, and 63.
- [Master handoff](Homeopathy_AI_Codex_Master_Handoff.md): original product vision, evidence categories, architecture direction, and historical decisions.
- [README](README.md): documented operating behavior, configuration, and current limitations.

This document extends product direction beyond the POC. It does not move all future features into the current POC or override its provenance, publication, authorization, or evaluation requirements. Later dated implementation checkpoints qualify older scaffolding instructions; inspect the code and tests before creating tasks for capabilities that may already exist.

Requirement status is explicit below:

- **Confirmed direction:** the user's requested two entry points, all seven modules, preparation of data for all seven from day one where feasible, India as the first market, fast incremental delivery, hosted production inference, development provider choice, and reuse of the POC UX direction.
- **Inherited requirement:** constraints already established in the POC PRD and handoff.
- **Proposed scope:** features from the supplied strategy discussion to refine into future releases.
- **Hypothesis/open decision:** commercial assumptions, sequencing, and architecture details that still need validation.

The conversation and earlier strategy text are the source of commercial proposals. Sections 13–24 consolidate the scope and data discussion. Sections 25–30 add the AnswerThis review, future workspace requirements, and public website plan. Competitor interface observations are distinguished from public marketing claims; market sizes, performance guarantees, security certifications, and customer counts have not been independently verified. Source links reflect the research performed in this conversation; they are not blanket commercial-use permissions or evidence that files have been imported.

Planning navigation: [competitor evidence](#25-answerthis-review-and-adoption-decisions), [research workspace](#26-future-research-workspace-requirements), [landing-page-message-and-content](#27-public-website-positioning-and-homepage-requirements), [public sitemap](#28-public-website-pages-and-acquisition-workflows), [development tasks](#29-competitor-derived-development-backlog-and-delivery-order), [release acceptance](#30-acceptance-measurement-and-maintenance-for-the-new-direction).

Latest technical discussion: [UI-to-AI-search design](#31-technical-direction-from-ui-to-ai-search) and [day-one-data-for-all-seven-modules](#32-day-one-data-coverage-across-all-seven-modules). These sections qualify earlier sequencing: collect across every module from the outset; feature implementation can remain incremental. No exclusive choice between classical literature and paper research is required for data collection.

Latest confirmed development decisions: [stack, authentication, naming and task delivery](#33-confirmed-stack-authentication-and-development-conventions). Section 33 confirms shadcn-vue and Go-based Authboss, optional Google login, Handlebars email templates, and the user's preference for a simple first launch, concrete names, complete feature tasks and focused verification. It supersedes earlier descriptions of the UI library as undecided and the proposed Better Auth Node.js service.

Launch decisions updated 2026-10-09: [public registration, ZeptoMail, research completion and admin analytics](#34-launch-access-background-research-and-admin-analytics). Research must continue after the user leaves, persist its result and send an answer-ready email. Private user uploads are future scope.

## 2. Product understanding

Build an AI-powered homeopathy knowledge, learning, and research platform that can eventually support practitioner case analysis. Users should be able to ask, learn, compare, research, or analyze a case with traceable sources.

The durable product asset is the curated and appropriately licensed corpus, structured knowledge, exact provenance, citation engine, and useful workflows. Model providers can change without redefining the product.

The platform has two connected product areas:

| Area | Primary outcome | Audiences |
|---|---|---|
| Knowledge, learning, and research | Answer questions, explain concepts, compare sources, and investigate literature | Students, teachers, researchers, practitioners |
| Practitioner case workspace | Structure a case, explore repertory matches and materia medica, and inspect supporting literature | Practitioners; educational case studies for students and teachers |

These areas share evidence services but have different inputs, permissions, domain models, and evaluation needs. A research user must never need to create a patient record to ask a question.

## 3. Confirmed entry point A: Ask without a case

The user explicitly requires that someone with a general query can ask it and receive a relevant answer from the platform's RAG and knowledge base.

```text
General question
  → understand the question and explicit source constraints
  → search eligible knowledge-base content
  → collect relevant supporting passages
  → synthesize and check supported claims
  → answer with citations, original passages, and coverage limitations
```

“General” means a natural-language question within the platform's knowledge scope; it does not imply an unrestricted internet chatbot or guaranteed knowledge of every subject.

| ID | Requirement | Observable acceptance behavior |
|---|---|---|
| ASK-01 | Asking is independent of case entry | A signed-in user can submit a question with no case or patient identifier. |
| ASK-02 | Use the approved, accessible knowledge base | Only eligible published sources with a compatible READY index support answers. |
| ASK-03 | Respect question and source scope | Explicit author/work constraints and selected sources apply to all retrieval and specialized answer paths. |
| ASK-04 | Support useful question types | Evaluate concepts, remedy descriptions, symptoms/topics, rubric explanations, source summaries, and comparisons against reviewed examples. |
| ASK-05 | Make answers inspectable | Substantive source-derived claims link to supporting passages and original pages where available. |
| ASK-06 | Handle missing or conflicting evidence | State corpus limits, preserve disagreements, and return partial answers or insufficient evidence when necessary. |
| ASK-07 | Preserve work | Saved questions retain source selections, answer status, citations, and support records and obey access permissions. |

Example questions for future acceptance fixtures:

- “What does Kent say about this symptom?”
- “Compare how these two remedies are described in materia medica.”
- “Explain this repertory rubric.”
- “Summarize this prepared book or paper.”
- “What research is available on this topic?”

These are target question categories, not promises that the present corpus contains the necessary sources. Today, research availability means material in the prepared corpus. Broader literature discovery is a separate future capability and must be labeled as such.

Conversational follow-ups are proposed future scope: if introduced, retain the question context and source filters while retrieving and checking evidence again for new claims.

## 4. Confirmed entry point B: Analyze a case — future product scope

```text
Case
  → structured symptoms
  → repertory analysis
  → candidate remedies
  → materia medica comparison
  → relevant literature and historical cases, when available
  → evidence and citations
  → practitioner review
```

This is the requested product direction. Case management, repertorization, and remedy recommendations remain outside the current POC under PRD section 5. Their inclusion here establishes future requirements, not authorization to bypass POC completion gates.

| ID | Future requirement | Acceptance intent for later specification |
|---|---|---|
| CASE-01 | Capture and summarize a case | Preserve the original narrative and distinguish supplied facts from extracted interpretations. |
| CASE-02 | Extract structured symptoms | Link each extracted item to its case text; allow practitioner correction and confirmation before downstream analysis. Missing details remain unknown. |
| CASE-03 | Map symptoms to repertory rubrics | Show the exact repertory, edition, rubric path, mapping rationale, and unresolved ambiguity. |
| CASE-04 | Produce reproducible repertory analysis | Define and version the scoring method, source grades, selected rubrics, and practitioner weighting; preserve the inputs for each run. |
| CASE-05 | Present candidate remedies for review | Explain candidates through the selected rubrics and source evidence; do not represent rankings as probabilities of clinical effectiveness. |
| CASE-06 | Compare materia medica | Provide side-by-side source-supported descriptions, similarities, differences, and gaps. |
| CASE-07 | Connect literature and historical cases | Explain relevance and evidence type; keep historical narratives separate from clinical study findings. |
| CASE-08 | Record practitioner review | Preserve edits, accepted/rejected suggestions, and analysis history. The AI output is not an autonomous diagnosis or prescription. |
| CASE-09 | Support case history and follow-up | Proposed later scope: version observations and comparisons across visits with appropriate patient-data access controls. |

Structured repertorization requires a validated repertory data model and usable content rights. Searching prose with embeddings alone does not establish accurate rubric membership, remedy grades, or scoring.

## 5. Shared trust requirements

These are inherited constraints for both entry points:

| ID | Requirement |
|---|---|
| TRUST-01 | Preserve traceability from claim and supporting excerpt to chunk, page, processing revision, asset checksum, edition, work, and acquisition record. |
| TRUST-02 | Resolve citations from stored evidence identifiers and trusted metadata. Never let generated bibliography text serve as citation verification. |
| TRUST-03 | Check both citation existence and whether the cited passage supports the claim. Retain verification decisions and exact excerpts. |
| TRUST-04 | Keep classical reference, clinical research, product information, and safety/guideline information distinguishable. |
| TRUST-05 | Do not convert historical descriptions into established efficacy claims, infer safety from silence, or transfer findings between materially different preparations. |
| TRUST-06 | Keep original assets and published evidence immutable; reprocessing must preserve authorized resolution of earlier citations. |
| TRUST-07 | Require rights, provenance, page QA, content approval, and publication gates before retrieval eligibility. Source discovery is not publication. |
| TRUST-08 | Report search coverage honestly. “Not found in our sources” does not mean “no study exists.” |
| TRUST-09 | Reference-only DOI/catalogue records are discovery metadata, not full-text passage evidence. |
| TRUST-10 | Source disablement, access restrictions, and integrity/retraction controls must govern retrieval and source viewing. |

Citation coverage and automated verification counts are diagnostics, not proof of answer accuracy or study quality. Clinical evidence appraisal needs its own defined method and human-reviewed evaluation.

## 6. Current documented baseline and remaining POC work

The baseline below combines earlier documentation with the code-level assessment performed in this conversation. The assessment inspected the frontend, backend routes, migrations, ingestion and answer pipelines, tests, README, and specifications. It included a frontend build and backend test run, but did not validate a running deployment or independently score live model answers. It is a dated observation, not a new test run performed while editing this document.

| Capability | Documented position |
|---|---|
| Source intake | PDF upload, direct HTTPS PDF import, Internet Archive discovery, DOI metadata lookup, and selected eligible DOI-to-PDF imports are described as implemented. |
| Source preparation | Extraction/OCR, automatic page QA, review, rights decisions, publication, and indexing are described as implemented. |
| General Ask | Hybrid retrieval, source selection, limited specialized question paths, saved answers, claim support records, and original-page citations are documented. |
| Background work | Durable source/answer jobs, Activity, retries, and recovery mechanisms are documented; some interruption/reconnect acceptance scenarios remain open. |
| Provenance/reprocessing | Asset and revision lineage and historical citations exist; reprocessing uses linked candidate source rows rather than the specified stable source identity across revisions. |
| Publication approval | Named actors are recorded; separate revision/content approval and stale-approval rejection remain open in the latest checkpoint. |
| Identity | Token-based principals and isolation are documented. Individual account lifecycle and per-user state need reconciliation before a broader multi-user release. |
| Quality acceptance | Documentation describes a draft evaluation, but the audit checkout lacked the linked `evaluation/` directory. Human-reviewed gold evidence, scored release evaluation, and resource/latency gates remain unverified. |
| Broader product | Full case workflows, structured repertorization, broad scholarly discovery, learning tools, and institutional collaboration are future scope. |

Read PRD sections 56, 58.6, 61.4, and the latest section 63 checkpoints before converting these gaps into tasks. Some identity descriptions in the checkpoints differ; verify actual behavior rather than assuming either full account support or only two fixed users.

Verification observed during the conversation:

- `npm run build` in `apps/frontend` passed, including TypeScript checking.
- `go test ./...` in `apps/backend` did not fully pass. `TestStarterFarringtonUsesItsOwnPageMap` lacked `data/starter-corpus/manifest.json`; `TestNashSampleALTO` lacked `data/starter-corpus/qa/nash-page-0049.alto.xml`.
- Archive, DOI, HTTP API, and model-client packages passed in that run.
- Database/live-provider integration acceptance was not established by that run; these checks require their designated disposable database and, where applicable, live-provider configuration.
- The README linked `evaluation/README.md`, which was absent in the reviewed checkout, and reported human-reviewed gold passages/scores as pending.
- No implementation was changed during the scope audit. This document update also does not repair those missing assets or certify a release.

The user considers the POC development milestone done for now. Record that milestone separately from technical acceptance: **Research foundation POC — implementation milestone closed by product owner; acceptance verification outstanding.** Existing revision identity, approval, recovery, and evaluation gaps should become visible tasks rather than being silently marked complete.

## 7. Proposed audience capabilities

| Audience | Proposed features beyond shared Ask |
|---|---|
| Student | Source-grounded tutor, case-study learning, repertory and materia medica explanation, citations, exam/learning mode. |
| Teacher | Teaching examples, source comparisons, educational case review, and later shared learning resources. |
| Practitioner | Case structuring, repertorization, candidate comparison, saved cases, and follow-up analysis. |
| Researcher | Literature discovery, paper/PDF analysis, study comparison, evidence tables, citation verification, research questions, manuscript and case-report assistance. |
| Institution | Shared libraries/projects, private knowledge bases, membership administration, usage controls, and later SSO and integrations. |

Systematic-review assistance must retain search strategies, databases, dates, screening decisions, and coverage limitations; a generated summary must not be labeled a completed systematic review.

## 8. Architecture direction for future design

Preserve the established Vue 3 + TypeScript frontend, Go backend, PostgreSQL with full-text search and pgvector, and separate API/worker processes. Continue the modular-monolith direction; these product areas do not by themselves justify separate microservices.

Current README configuration allows chat and embedding providers to vary independently, including hosted providers. Treat older local-only model examples as historical configuration context. Do not hard-code a model, provider, API key, or embedding dimension into the product requirements.

The following are proposed logical boundaries, not final service or table definitions:

| Module | Responsibility |
|---|---|
| Sources and ingestion | Discovery, acquisition, immutable assets, metadata, rights, OCR, QA, revisions, publication. |
| Retrieval and evidence | Query scope, lexical/vector search, source eligibility, evidence packs, citation resolution, claim support. |
| Questions and answers | General Ask, saved question runs, later conversation context, answer states, user-visible results. |
| Structured homeopathic knowledge | Remedy identities/aliases, versioned rubric hierarchies, remedy grades, source mappings. |
| Case workspace | Case narratives, confirmed symptoms, analysis runs, practitioner decisions, later follow-ups. |
| Research workspace | Discovery records, saved papers, evidence tables, projects, reference exports. |
| Identity and workspaces | Users, roles, membership, access scope, private/shared content, later entitlements. |
| Jobs and audit | Durable processing, progress, retries, model/prompt revisions, operational diagnostics. |

Architecture constraints for later designs:

- Reuse the evidence and citation services across Ask, learning, research, and case analysis.
- General questions must not have a required dependency on case or patient records.
- Keep case-input provenance separate from literature evidence: a patient narrative is not a published source.
- Apply authorization to retrieval candidates, saved answers, exports, and citation/asset access, not just screens.
- Keep private case and institutional material out of the shared corpus unless an explicit permitted publication workflow exists.
- Before real patient-data workflows, define retention, deletion, audit access, and what content each configured model provider receives.
- Version repertory data, scoring, confirmed case inputs, evidence selections, and model/prompt configuration so analysis can be inspected and compared.
- Preserve embedding compatibility checks and explicit reindexing when the embedding configuration changes.
- Use bounded, durable jobs for expensive analysis with actionable failure states.
- Check existing storage and queue implementations before selecting replacements. Earlier MinIO/River proposals are not proof those technologies are currently deployed.

Future architecture deliverables should include a domain model, access matrix, workflow/state diagrams, API contracts, migration strategy, and short decision records for unresolved choices.

## 9. Proposed development sequence and task families

This earlier task-family grouping remains useful for organizing work. The more specific seven-module sequence in section 15 is the latest planning recommendation. Neither is a committed schedule. India-first and eventual coverage of all seven modules are confirmed; the exact pilot audience and release thresholds still need validation.

| Stage / task family | Intended outcome | Dependencies / exit evidence |
|---|---|---|
| P0 — POC acceptance | Establish a trustworthy working baseline | Reconcile current code with PRD gaps; complete revision/approval and recovery checks; human-reviewed retrieval, support, abstention, and performance evaluation. |
| P1 — General Ask quality | Make case-free questions useful across the intended categories | ASK-01–07 and TRUST-01–10 evaluated on representative source-specific, comparison, ambiguous, and unsupported queries. |
| P2 — Research and learning | Add focused learning and research workflows | Validated user needs, source rights, reliable Ask; connectors and metadata-only/full-text distinctions tested. |
| P3 — Structured repertory and case prototype | Validate the complete case-to-evidence workflow | Licensed/usable repertory data, defined scoring, practitioner-reviewed fixtures, case-data controls, and CASE-01–08. |
| P4 — Institutional and commercial readiness | Support shared workspaces and sustainable plans | Verified isolation, account administration, usage economics, onboarding, and operational requirements. |

The next task-planning session should identify the smallest useful vertical slice and reuse implemented capabilities rather than rebuilding the original scaffold.

Every implementation task should record:

1. Requirement IDs and user outcome.
2. Current behavior verified in code and the exact gap.
3. In-scope behavior, dependencies, and explicit exclusions.
4. Data/API/UI effects, migration and access implications.
5. Observable acceptance criteria, relevant fixtures, and evaluation needs.
6. Evidence of completion and documentation updates.

## 10. Commercial context — hypotheses to validate

The user confirmed **India as the initial market**. BHMS students, teachers, practitioners, researchers, clinics, colleges, and the CCRH-related research ecosystem are potential audiences. These are possible acquisition/customer channels, not established partnerships or guaranteed demand.

The longer-term ambition includes Europe, the UK, Africa, the Americas, and worldwide availability. Germany, Brazil, France, and international English-speaking audiences were earlier expansion proposals; their order is not confirmed and must not override the India-first decision. Validate demand, content rights, localization, and applicable product requirements before fixing a country sequence.

Three proposed landing-page messages are AI for Homeopaths, AI for Homeopathy Students, and AI for Homeopathy Researchers. Their purpose is to learn which workflows attract sustained use and payment.

Proposed individual pricing from the supplied discussion:

| Plan | India monthly / annual | International monthly / annual |
|---|---|---|
| Free | Limited free access | Limited free access |
| Student | ₹199 / ₹1,499 | $5.99 / $49 |
| Practitioner | ₹999 / ₹9,999 | $19.99 / $199 |
| Researcher | ₹1,499 / ₹14,999 | $29.99 / $299 |
| Power | ₹2,499 / ₹24,999 | $49.99 / $499 |

The discussion also proposes clinic, college, university, and research-department contracts, with shared libraries and administration as institutional value. A 7–14 day trial was suggested. None of these prices, tier inclusions, usage limits, or trial terms is finalized; measure model/OCR/storage costs and willingness to pay before turning them into billing requirements. “Unlimited AI” is not a committed entitlement.

Named competitor benchmarks for later research include RadarOpus, Complete Dynamics, Synthesis-based products, MacRepertory, HomeoQuest, India-focused AI tools, and general-purpose AI assistants. AnswerThis has now been reviewed as a research-workflow and marketing reference; see section 25. This document makes no verified claims about the other named competitors' current features, prices, or user bases.

## 11. Open decisions for future planning

- Which audience and use case should lead the first release after POC acceptance?
- What corpus breadth and answer-quality thresholds are necessary for that release?
- Which repertory editions can be used, and how will rubric structure and remedy grades be validated?
- What scoring method and practitioner review process should the case prototype use?
- Which research connectors and evidence-appraisal methods should come first?
- Which educational features have demonstrated demand?
- What account, organization, case-data, and provider-data policies are required for the intended deployment?
- Which languages, usage limits, prices, and institutional capabilities should be tested first?

These decisions should become scoped discovery tasks or architecture decisions. They do not block documenting or improving the already-required case-free Ask workflow.

## 12. Instructions for future development sessions

Read this document, the POC PRD, and the README before task generation or architecture changes. Preserve the two independent entry points. Keep source grounding shared and mandatory. Distinguish working code, documented behavior, unverified acceptance, and proposed scope in progress reports.

Do not infer that this roadmap makes the POC complete, that every proposed feature must ship at once, or that commercial hypotheses are confirmed requirements. Do not copy environment credentials or patient information into planning documents, task descriptions, fixtures, or architecture examples.

## 13. Confirmed decisions and scope boundaries from this conversation

| ID | Decision | Development consequence |
|---|---|---|
| DIR-01 | Launch in India first; retain global ambition | Prioritize Indian literature, pilot users, usable English workflows, and INR commercial experiments. Do not build every country's deployment requirements before the India research pilot. |
| DIR-02 | Cover all seven modules one by one | Maintain explicit requirements and acceptance criteria for each module; release in useful increments. |
| DIR-03 | Reach the market quickly without unnecessary data or technical hurdles | Reuse the POC, integrate a small number of dependable connectors, and collect data continuously. Do not make all 56 sources mandatory integrations. |
| DIR-04 | Use hosted model providers in production | Production chat and embeddings use configured hosted endpoints; credentials remain server-side. |
| DIR-05 | Keep development inference pluggable | Developers can choose LM Studio, hosted providers, or a mixed chat/embedding setup without changing domain workflows. |
| DIR-06 | Use the UX direction discussed for the POC | Extend the compact Vue workflow, visible progress, preserved work, and citation side panel. |
| DIR-07 | Support questions independently of cases | Case or patient creation is never required for general research, learning, or comparison. |
| DIR-08 | Collect public data for all modules | Use a shared corpus plus module-specific structures; public reading availability, automated access, and commercial ingestion rights are recorded separately. |
| DIR-09 | Preserve provenance and evidence distinctions | Repertory scores, historical descriptions, case observations, clinical studies, and modern guidelines must not be conflated. |

The requested minimum source catalogue is interpreted as at least 50 sources **across the seven modules**, not 50 per module. The catalogue below contains 56 works, collections, repositories, services, and reference frameworks. These are not 56 independent clinical studies, nor 56 completed imports. A chatbot and citation assistant reuse the shared knowledge base; they do not require a separate corpus of generated medical conversations.

Speed comes from a narrow usable slice, automated metadata handling, structured full text, and reuse of existing components. It does not require a model-training project, microservices rewrite, universal web crawler, or ingestion of the entire internet. Maintain existing trust checks with focused exception review instead of adding repetitive approval prompts for routine, authorized collection.

## 14. Seven-module coverage audit

Status reflects the code-level assessment in this conversation, with the verification limitations in section 6. “Partial” means useful related behavior exists; it does not mean a dedicated complete module exists.

| Module | Current coverage | Missing capability |
|---|---|---|
| Repertory analysis | No dedicated structured implementation. Imported repertory prose could be searched as a document. | Rubric hierarchy, symptom mapping, remedy membership/grades, user weighting, deterministic scoring, edition-specific provenance, and suitable data rights. |
| Case research | Ordinary questions can search prepared sources. No dedicated case workspace was found. | Published-case extraction/search, structured case intake, anonymization, timelines, literature matching, saved case workspaces, review and follow-up controls. |
| Paper search | Partial: Crossref/DataCite DOI lookup, selected eligible PDF import, uploads, public HTTPS PDF links, and Internet Archive discovery. | Topic-based biomedical search, filters, deduplication, related papers, saved searches, alerts, and broader permitted full-text discovery. |
| Materia medica comparison | Partial: prepared authors/remedy descriptions can be compared in cited prose. | Structured comparison tables, normalized names, symptom/modality categories, edition differences, disagreements, and completeness evaluation. |
| Clinical decision support | Not implemented. Existing answer prompts restrict medical advice. | Defined clinical intended use, contemporary evidence, clinician-reviewed workflows, safety evaluation, governance, and applicable product assessment. |
| AI chatbot | Partial: natural-language questions, quick/detailed modes, selected sources, saved answers, retries, Activity. | Persistent threads, resolved follow-up context, controlled memory, fresh grounding of new claims, and project organization. |
| Citation/research assistant | Strongest implemented foundation: hybrid retrieval, synthesis, claim/excerpt records, scan-linked citations, provenance and research trails. | Independent accuracy evaluation, bibliography exports, reference-manager interoperability, structured evidence appraisal, and systematic research workflow support. |

Do not count these existing features as broader capabilities:

- “Detailed research” is bounded deeper search over prepared sources, not wider internet or medical-literature search.
- A known DOI lookup is not topic-based paper discovery.
- Saved question history is not multi-turn conversation.
- Embedding search over a repertory is not reproducible repertorization.
- A verified quote supports attribution to a document, not the scientific truth of its claim.
- Automated claim-check diagnostics are not independently measured accuracy.
- A bibliography record or abstract is not reviewed full-text evidence.
- A small historical corpus is not an exhaustive literature review.

### 14.1 Existing assets to preserve

- PDF acquisition, text extraction/OCR, page-level QA, review, rights decisions, and publication.
- Edition/acquisition/asset lineage, checksums, processing revisions, and original scans.
- Full-text and vector retrieval with eligible-source filtering and embedding compatibility checks.
- Durable answer jobs, retries, saved answers, Activity, and named actors.
- Claim-to-excerpt records, partial answers, abstention, and original-page citation inspection.
- Source disable/restore and replacement/reprocessing behavior that preserves earlier evidence.
- Evaluation reports recording questions, searches, candidates, citations, model/prompt revisions, and job diagnostics.

Inspect [API routes](apps/backend/internal/httpapi/api.go), [research engine](apps/backend/internal/httpapi/research.go), [claim pipeline](apps/backend/internal/httpapi/evidence_claims.go), [answer jobs](apps/backend/internal/httpapi/answer_jobs.go), [model configuration](apps/backend/internal/localllm/client.go), [frontend](apps/frontend/src/App.vue), and [migrations](apps/backend/migrations) before creating replacement tasks.

## 15. Incremental release plan for all seven modules

This is the recommended sequence, not a staffing estimate or fixed delivery date. Work can overlap when dependencies permit. A marketable research beta need not wait for clinical functionality or for all sources to be integrated.

**Latest clarification:** section 32 establishes day-one data preparation across all seven modules. The increments below order feature delivery, not permission to begin collecting each category. Whether public launch must wait for a working feature in all seven remains a separate release-scope decision; data coverage alone does not imply complete functionality.

| Increment | Outcome | First usable scope | Exit evidence |
|---|---|---|---|
| 0 — Close out the POC | Reproducible baseline | Restore missing fixtures/evaluation assets; reconcile documentation; exercise import → review → publish → READY → answer → citation → reload. | Appropriate automated checks pass and a human-reviewed benchmark reports actual results and known gaps. |
| 1 — Citation/research assistant | Inspectable, reusable research results | Existing cited answers, saved research organization, bibliography export, reliable source opening. | Citations/export fields resolve from stored metadata; missing metadata remains explicit. |
| 2 — AI chatbot | Grounded ongoing conversation | Persistent threads, source filters, grounded follow-ups, retry/reopen, preserved input. | Follow-up pronouns and source constraints resolve correctly; new factual claims retrieve supporting evidence. |
| 3 — Paper search | Discover and analyze literature | PubMed/Europe PMC topic search, Crossref enrichment, filters, deduplication, save/import. | Search → save → eligible full text → question → evidence → export works end to end. |
| 4 — Materia medica comparison | Structured source comparison | Selected remedies/authors/editions; symptoms, modalities, similarities, differences, gaps. | Each substantive cell has evidence; omissions and disagreements survive synthesis. |
| 5 — Repertory analysis | Reproducible rubric analysis | One reviewed repertory edition, rubric search, user-confirmed mappings, explicit weights and grades. | Expert fixtures verify rubric membership and calculations; repeat runs produce the same result. |
| 6 — Case research | Search and compare published cases | Published-case extraction, timeline, similarity explanation, link to studies and materia medica. Add private practitioner case workspace as a separate expansion. | Reported facts link to case text; concurrent treatment and incomplete follow-up are visible; private inputs obey access rules. |
| 7 — Clinical decision support | Clinician-facing evidence and safety support | Guideline lookup, evidence summaries, safety/referral reference, explicit applicability and gaps. | Exact functionality is clinically reviewed and evaluated; applicable India requirements assessed before patient-specific recommendations. |

After increments 1–3, recruit a small India pilot and assess repeat use, research usefulness, citation correctness, response time, and willingness to pay. The exact audience mix is open; practitioners, BHMS students/teachers, and researchers are candidates. Extend to the remaining modules without marketing them as available before they work.

Production identity, access isolation, monitoring, backup restoration, and cost controls are a parallel launch workstream. Institutional SSO, large collaboration suites, broad localization, and international hosting expansion can follow demonstrated demand.

### 15.1 Module-specific behavior requirements

| ID | Requirement |
|---|---|
| CIT-01 | Generate citation exports from stored DOI/PMID/edition/author/title metadata, with an explicit unknown state; never invent missing bibliographic fields. |
| CIT-02 | Support a focused initial export format such as BibTeX or RIS, then reference-manager interoperability and richer research exports. Format priority remains open. |
| CIT-03 | Preserve query, databases/sources, run date, filters, selected evidence, claim checks, and answer lineage. |
| CHAT-01 | Store conversations and messages with owner/workspace scope, effective source selection, and evidence-linked answer runs. |
| CHAT-02 | Resolve follow-ups while rechecking source eligibility; prior model output is context, not a primary evidence source. |
| CHAT-03 | Support missing-context clarification, recoverable failure, reopen, and retry without losing the user's work. |
| PAPER-01 | Search by topic, remedy, author, condition and identifiers; support available date, study-type, and access filters without fabricating unavailable fields. |
| PAPER-02 | Deduplicate manifestations of a paper and keep preprint, accepted manuscript, correction, and version-of-record distinctions. |
| PAPER-03 | Save useful metadata immediately; acquire full text only through a permitted available route. Show abstract-only and full-text states distinctly. |
| PAPER-04 | Add saved searches, alerts, related-paper expansion, and screening workflows after the first search/import slice. |
| MM-01 | Normalize remedy aliases without collapsing different substances/preparations or losing the original label. |
| MM-02 | Compare explicit remedies/authors/editions and expose source-specific symptoms, modalities, similarities, differences, and missing coverage. |
| REP-01 | Preserve repertory edition, rubric hierarchy/path, remedy membership, source grades, typography interpretation, and exact provenance. |
| REP-02 | AI may suggest mappings; a user confirms ambiguous symptom-to-rubric choices before scoring. |
| REP-03 | Compute scores in deterministic code from versioned inputs, never as unexplained LLM rankings. |
| REP-04 | Explain each candidate's contribution by rubric/grade/weight; rankings are not probabilities of treatment success. |
| CASE-10 | Keep published-case discovery separate from storing identifiable private patient records. |
| CASE-11 | Extract presentation, supplied diagnosis/investigations, timeline, interventions, concurrent treatment, outcomes, adverse events, and follow-up; preserve missingness. |
| CASE-12 | Explain why a case is similar and where it differs; an observed improvement does not establish causality. |
| CDS-01 | Define intended users, inputs, outputs, and clinical influence before implementing clinical recommendations. |
| CDS-02 | Keep guideline-based information and safety context distinct from historical remedy descriptions. |
| CDS-03 | Expose guideline version, jurisdiction, evidence quality, applicability, and uncertainty; do not infer safety from absent reports. |
| CDS-04 | Patient-specific diagnosis, treatment selection, potency/dose guidance, and prescribing are separate scope requiring further evidence and assessment, not implied by reference lookup. |

## 16. Data collection strategy and initial targets

### 16.1 Collection principles

Use one common acquisition/provenance pipeline, with module-specific extraction afterward. Keep the following decisions separate:

The latest user direction is to prepare useful seed data for **all seven modules from day one where feasible**, rather than select only classical-book research or paper research. Section 32 defines the module coverage matrix and readiness measures. The numbered questions below apply per source and do not require completing one module's collection before another starts.

1. Is a source publicly discoverable/readable?
2. Does it offer a supported automated retrieval method?
3. Can the specific material be stored, processed, and displayed in the intended commercial product?
4. Is it appropriate evidence for this question or module?

Publicly readable does not mean public domain. A historic original, modern edition, translation, transcription, scan, and website wrapper can have different reuse conditions. Government publication also does not automatically mean unrestricted reproduction. API availability does not grant rights to every linked paper.

Use two practical routes:

- **Ingestion route:** acquire clearly reusable or appropriately permitted content, record the basis, run the existing quality/publication process, and index it.
- **Discovery/reference route:** retain permitted bibliographic metadata and source links where full-text access or commercial reuse is unresolved. Continue collecting elsewhere; do not block the entire release on that item.

Do not invent permission or bypass paywalls, CAPTCHAs, or access controls. Prefer official APIs, repository exports, and user-authorized/manual import where supported. Uncertain rights or extraction create an item-level exception, not a requirement to stop all collection. Do not email rights holders or institutions unless separately authorized to send those messages.

### 16.2 Initial corpus targets

These are proposed starting quantities, not compulsory release gates or completed counts.

| Collection | Starting target | Primary candidates |
|---|---|---|
| Classical literature | 8–12 exact editions | Existing Nash/Farrington, Kent, Boericke, Allen, historical case texts. |
| Structured repertory | One well-checked edition | Kent or Boericke; inspect OOREP data before rebuilding from OCR. |
| Paper discovery | 500 relevant deduplicated records | PubMed, Crossref, Europe PMC, Indian research portals. |
| Reusable full-text research | 100–200 relevant papers | Compatible PMC OA and publisher/repository versions. |
| Published cases | 50–100 extractable reports/narratives | Eligible modern reports and separately labelled historical cases. |
| Clinical reference | 20–30 documents or external references for chosen topics | ICMR, MoHFW, WHO, NCDC and safety resources. |
| Evaluation fixtures | 50 realistic questions/tasks, expanding with modules | Clinician/domain-reviewed examples, including unsupported and misleading questions. |

Question coverage, source quality, and valid reuse matter more than raw volume. Do not mix planned quantities with the POC's documented historical corpus counts, or report these targets as ingested data.

### 16.3 Acquisition and processing flow

```text
Discover
 → normalize identifiers and deduplicate
 → record evidence type and access/reuse basis
 → acquire eligible original asset or retain discovery record
 → extract structured XML/HTML, PDF text, or OCR
 → attach provenance and quality checks
 → extract module-specific structures
 → review exceptions and required publication decisions
 → index/publish eligible evidence
 → evaluate retrieval and update coverage gaps
```

- Query both `homeopathy` and `homoeopathy`; expand with remedy aliases, conditions, author names, and MeSH vocabulary.
- Deduplicate with DOI/PMID and version identifiers, then normalized title/author/year. Books additionally require edition and volume identity.
- Prefer structured XML/HTML, then embedded PDF text, then OCR. Preserve a stable original representation and text offsets/sections for non-PDF citations; do not fabricate printed page numbers.
- Preserve tables, negation, comparative direction, rubric hierarchy, and typography used for grades. These are common places where ordinary text extraction loses meaning.
- Record DOI, PMID, PMCID or other identifiers where present, title, authors, publication date, work/edition/version, evidence category, original URL, acquisition time, asset hash, licence/basis, processing revision, and retraction/correction status.
- Review uncertain OCR, ambiguous rubric grades, suspect metadata, unsupported licence assumptions, and missing case fields. Keep bulk metadata automation and established source-level rules efficient.
- Keep publication approval and review requirements inherited from the POC; automation must not silently turn discovery into searchable approved evidence.
- Recheck relevant retraction/correction and source-access status on a defined schedule; preserve earlier answer lineage while indicating changed source status.

### 16.4 Evidence categories and structured extraction

| Evidence/data type | Extract | Do not infer |
|---|---|---|
| Classical materia medica | Remedy names, author/edition, descriptions, modalities, cited passages | Modern clinical effectiveness from a historical statement. |
| Repertory | Rubric path, remedy IDs, source grades, original notation and source location | Grades from font-lost OCR, or treatment probabilities from scores. |
| Published case report | Presentation, chronology, investigations, interventions, concurrent care, outcomes, follow-up | Causation from improvement or missing details from typical practice. |
| Trial/study | Design, population, comparator, intervention/preparation, endpoints, results, uncertainty, limitations | Findings for other preparations/populations or unreported results. |
| Systematic review | Search scope/date, included studies, synthesis, certainty and limitations | Exhaustiveness of the app's own search from the review's title. |
| Trial registration | Protocol, dates, status, outcomes planned, linked publication/results | Completion or benefit merely from registration. |
| Guideline/safety reference | Issuer, version/date, jurisdiction, recommendation context, safety information | Local applicability or endorsement of homeopathic claims. |
| Bibliographic metadata | Identifiers, titles, authors, links, licence assertions where supplied | Full-text support or verified efficacy. |

Study quality and licensing are independent: a reusable article can be weak evidence, and a high-quality reference may be link-only. Retrieve positive, negative, and inconclusive findings without suppressing disagreement.

## 17. Public source catalogue — 56 resources

Catalogue compiled from this conversation's research on 2026-10-08. Sources include individual works, broad collections, discovery services, terminology datasets, and reporting frameworks. Their inclusion is a collection recommendation, not a completed integration, exhaustive rights audit, or clinical endorsement. Access quotas, licence terms, URLs, and service availability must be checked when implementing a connector.

**Route notation:** Text/scan = discoverable reading material; choose the exact edition and reuse basis. API/metadata = programmatic discovery/structured records, not automatic full-text rights. Reference/link = useful external reference; ingestion requires applicable permissions. Module abbreviations: REP repertory; CASE case research; PAPER paper search; MM materia medica; CDS clinical support; CHAT chatbot; CITE citation/research assistant. CHAT and CITE reuse eligible material throughout the catalogue.

### 17.1 Classical repertory, materia medica, and case works

| # | Source | Main modules / use | Collection route and qualification |
|---|---|---|---|
| 1 | [Kent — Repertory of the Homoeopathic Materia Medica](https://www.homeoint.org/books/kentrep/kentpref.htm) | REP: hierarchy, remedy entries | Text/scan; preserve grade typography and edition. |
| 2 | [Oscar Boericke — Repertory](https://www.homeoint.org/books4/boerirep/index.htm) | REP: condition/rubric indexing | Text/scan; distinguish from William Boericke's materia medica. |
| 3 | [Boger — General Analysis](https://www.homeoint.org/books5/bogergena/index.htm) | REP, MM: general rubrics/comparison | Text/scan; preserve original structure. |
| 4 | [Knerr — Repertory of Hering's Guiding Symptoms](https://homeoint.org/hering/index.php) | REP: detailed rubric structure | Text/scan; identify exact work within combined site. |
| 5 | [Bidwell — How to Use the Repertory](https://www.homeoint.org/site/deepak/bidwell1.htm) | REP, CASE: methodology/worked examples | Historical teaching text, not a modern validated scoring standard. |
| 6 | [William Boericke — Homoeopathic Materia Medica](https://www.homeoint.org/books/boericmm/) | MM: remedy descriptions/comparisons | Text/scan with edition identification. |
| 7 | [Kent — Lectures on Homoeopathic Materia Medica](https://www.homeoint.org/books3/kentmm/index.htm) | MM: author/remedy comparison | Text/scan. |
| 8 | [H. C. Allen — Keynotes and Characteristics](https://www.homeoint.org/books/allkeyn/index.htm) | MM: keynotes/comparative descriptions | Text/scan. |
| 9 | [Nash — Leaders in Homoeopathic Therapeutics](https://www.homeoint.org/books2/nashtherap/intro.htm) | MM: extends existing POC direction | Text/scan; reconcile with already prepared edition. |
| 10 | [Farrington — A Clinical Materia Medica](https://archive.org/details/64250520R.nlm.nih.gov) | MM: cross-author comparison | Digitized book; verify exact edition against POC source. |
| 11 | [Kent — Clinical Cases](https://www.homeoint.org/books3/kentclin/index.htm) | CASE: historical narratives | Extract reported observations with historical labels. |
| 12 | [Nash — The Testimony of the Clinic](https://www.homeoint.org/books5/nashtestimony/index.htm) | CASE: historical narratives | Reported outcomes are not causal proof. |

Several Homeoint entries were discoverable in indexed search results but direct retrieval failed during this review. They are discovery leads, not a verified functioning bulk-download integration. Prefer suitable original scans from archives where possible. Transcription/translation permissions are separate from the age of the underlying work.

### 17.2 India-focused collections and a structured repertory candidate

| # | Source | Main modules / use | Collection route and qualification |
|---|---|---|---|
| 13 | [AYUSH Research Portal](https://arp.ayush.gov.in/) | PAPER, CASE: Indian research discovery | Search records and linked publications. |
| 14 | [Indian Journal of Research in Homoeopathy](https://www.ijrh.org/journal/) | PAPER, CASE, MM: papers, reports, reviews, provings | Public articles; site displays CC BY-NC-SA 4.0. Commercial full-text use needs an appropriate basis. |
| 15 | [CCRH Archives on Homoeopathy](https://aoh.ccrhlibrary.in/) | REP, MM, CASE, PAPER: books/journals/grey literature | Repository records and available files; item-level rights. |
| 16 | [CCRH Homoeopathic Pathogenic Trial portal information](https://www.ccrhindia.ayush.gov.in/node/202?language_content_entity=en) | MM, REP: proving/symptom research | Official portal description links to HPT; actual access/retrieval needs checking. |
| 17 | [CCRH Clinical Verification programme](https://ccrhindia.ayush.gov.in/research-activities/clinical-verification?language_content_entity=en) | PAPER, MM: study discovery | Programme information and cited publications; appraise underlying studies. |
| 18 | [CCRH Annual Reports](https://ccrhindia.ayush.gov.in/index.php/publications/annual-reports) | PAPER: projects/publication discovery | Public PDFs; administrative statements are not clinical evidence. |
| 19 | [Homoeopathic Pharmacopoeia of India — Volume V](https://www.pcimh.gov.in/WriteReadData/RTF1984/HPIVOLV.pdf) | MM: substance identity and quality standards | Public reference PDF; establish reuse basis and current applicability. |
| 20 | [Clinical Trials Registry–India](https://www.ctri.nic.in/Clinicaltrials/gsearch.php) | PAPER: registrations/protocol matching | Public search; manual import initially if automated access is restricted. |
| 21 | [Shodhganga](https://shodhganga.inflibnet.ac.in/) | PAPER, CASE: Indian theses | Item-specific rights; automated access was blocked during research. |
| 22 | [OOREP](https://www.oorep.com/) / [project repository](https://github.com/nondeterministic/oorep) | REP: structured data candidate | Inspect available data, provenance, and dataset licence separately from software licence. |

IJRH free reading access is not blanket commercial-ingestion permission. Keep it discoverable while selecting compatible articles or obtaining an appropriate permission. OOREP may save extraction work, but availability of usable exports, completeness, grading accuracy, and reuse rights were not established by the catalogue review. CTRI, HPT, and Shodhganga do not have to block the initial API integrations.

### 17.3 Literature, trials, metadata, and digital libraries

| # | Source | Main modules / use | Collection route and qualification |
|---|---|---|---|
| 23 | [PubMed / NCBI E-utilities](https://www.nlm.nih.gov/dataguide/eutilities/utilities.html) | PAPER, CITE: biomedical discovery | API metadata and available abstracts; preserve abstract rights and evidence scope. |
| 24 | [PMC Open Access Subset](https://pmc.ncbi.nlm.nih.gov/tools/openftlist/) | PAPER, CASE, CITE: full text | Approved retrieval services; compatible article licences only. |
| 25 | [Europe PMC](https://europepmc.org/RestfulWebService) | PAPER, CASE: discovery/OA text | REST API; OA XML where available. |
| 26 | [Crossref](https://www.crossref.org/documentation/retrieve-metadata/rest-api/) | PAPER, CITE: DOI metadata/references/updates | Existing partial integration; extend rather than duplicate. |
| 27 | [OpenAlex](https://help.openalex.org/) | PAPER: related literature/authors/citation relationships | API/data under current service terms. |
| 28 | [DOAJ](https://doaj.org/terms/) | PAPER: OA journal/article discovery | Metadata; follow each article's licence. |
| 29 | [Unpaywall](https://unpaywall.org/products/api) | PAPER: OA versions by DOI | Locator API; acquire from linked host under its terms. |
| 30 | [DataCite](https://datacite.org/) | PAPER, CITE: datasets/research objects | DOI metadata; existing partial integration. |
| 31 | [Semantic Scholar API](https://api.semanticscholar.org/api-docs/) | PAPER: related papers/citation graph | Current API terms, key requirements and quotas apply. |
| 32 | [CORE](https://core.ac.uk/services/api) | PAPER: repository metadata/available text | API access plan and document-specific rights. |
| 33 | [ClinicalTrials.gov](https://clinicaltrials.gov/data-api/api) | PAPER, CDS: protocols/status/posted results | Structured API; registration alone is not results evidence. |
| 34 | [WHO ICTRP](https://www.who.int/tools/clinical-trials-registry-platform) | PAPER: cross-registry discovery | Public search; bulk access may require arrangements. |
| 35 | [Internet Archive](https://archive.org/developers/) | REP, MM, CASE: historical originals | Existing discovery connector; select suitable downloadable items. |
| 36 | [NLM Digital Collections](https://collections.nlm.nih.gov/) | MM, CASE: medical books/provenance | Catalogue and digitized items; automated access route needs checking. |
| 37 | [Wellcome Collection](https://wellcomecollection.org/collections) | MM, CASE: historical medical literature | Catalogue API and digital items with stated licences. |
| 38 | [DUT Open Scholar](https://openscholar.dut.ac.za/) | PAPER, CASE: homeopathy dissertations/research | Repository records and available theses; appraise quality separately. |

PMC distinguishes its reusable subset from other freely readable content and specifies permitted automated retrieval services. Prefer structured XML through those services where appropriate rather than scraping article pages. Europe PMC and PubMed overlap; deduplicate rather than count the same publication as independent evidence. Digital libraries may hold the same book/edition.

### 17.4 Clinical reference, safety, and evidence appraisal

| # | Source | Main modules / use | Collection route and qualification |
|---|---|---|---|
| 39 | [ICMR Standard Treatment Workflows](https://www.icmr.gov.in/standard-treatment-workflows-stws) | CDS: India-specific reference | Public PDFs; reference/link until reuse basis confirmed. |
| 40 | [MoHFW Standard Treatment Guidelines](https://www.clinicalestablishments.mohfw.gov.in/en/standard-treatment-guidelines) | CDS: Indian clinical pathways | Versioned guideline links/PDFs; rights and currency checks. |
| 41 | [WHO Guidelines](https://www.who.int/publications/who-guidelines) | CDS: clinical/public-health evidence | Publication-specific licence; applicability review. |
| 42 | [NICE Guidance](https://www.nice.org.uk/guidance) | CDS: comparison/reference | Link first; India and AI reuse require appropriate terms/permission. |
| 43 | [MedlinePlus](https://medlineplus.gov/) | CHAT, CDS: plain-language explanations | Selected resources; distinguish third-party copyrighted content. |
| 44 | [NCCIH — Homeopathy](https://www.nccih.nih.gov/health/homeopathy) | CHAT, CDS: limitations/safety context | Public reference; include contrary evidence and limitations. |
| 45 | [Cochrane Evidence](https://www.cochrane.org/evidence) | PAPER, CDS: systematic-review findings | Public summaries; full-text access/reuse varies. |
| 46 | [National Centre for Disease Control](https://ncdc.mohfw.gov.in/) | CDS: Indian infectious-disease guidance | Official documents; retain issue/version dates. |
| 47 | [NHM IMNCI resources](https://www.nhm.gov.in/images/pdf/programmes/child-health/guidelines/IMNCI-Module-2023-For-Medical-Officers/IMNCI-Facilitator-Guide-Medical-Officer-2023.pdf) | CDS: child-health assessment/referral context | Public training document; clinician-reviewed extraction. |
| 48 | [Pharmacovigilance Programme of India](https://ipc.gov.in/PvPI/pv_home.html) | CDS: safety communications/reporting | Public resources, not access to individual patient reports. |
| 49 | [JBI Critical Appraisal Tools](https://jbi.global/critical-appraisal-tools) | PAPER, CITE: study-quality assessment | Reference checklists; reuse terms apply; not a clinical dataset. |
| 50 | [DailyMed](https://dailymed.nlm.nih.gov/dailymed/) | CDS: label/safety/concomitant-medication context | Structured US labels; not Indian prescribing guidance or proof of efficacy. |

An [ICMR workflow volume](https://www.icmr.gov.in/icmrobject/uploads/Documents/1725444938_stw_manual_v1_stw_i.pdf) explicitly reserves reproduction rights. [NICE reuse conditions](https://www.nice.org.uk/reusing-our-content/nice-uk-open-content-licence) include specific AI/content-use provisions. Link-only references remain useful while eligible content is ingested elsewhere. Contemporary guideline sources are not endorsements of historical homeopathic claims.

### 17.5 Terminology, reporting structure, and integrity

| # | Source | Main modules / use | Collection route and qualification |
|---|---|---|---|
| 51 | [MeSH](https://www.nlm.nih.gov/mesh/meshhome.html) | PAPER, CHAT: synonyms/search expansion | Structured vocabulary; preserve version. |
| 52 | [WHO ICD-11](https://icd.who.int/browse/2026-01/mms/en) | CASE, CDS: condition organization | Classification/API under WHO terms; not a remedy-mapping dataset. |
| 53 | [PubChem](https://pubchem.ncbi.nlm.nih.gov/) | MM: substance identity/synonyms | Structured records with source provenance; distinguish preparation/dilution. |
| 54 | [CARE Checklist](https://www.care-statement.org/checklist) | CASE: extraction/report structure | Reporting framework, not patient data or efficacy assessment. |
| 55 | [EQUATOR Network](https://www.equator-network.org/) | PAPER, CASE, CITE: reporting standards | Framework discovery/reference; not a full-text corpus. |
| 56 | [Crossref Retraction Watch data](https://www.crossref.org/documentation/retrieve-metadata/retraction-watch/) | CITE, PAPER: integrity updates | Data/feed integration under its terms; corrections/retractions affect evidence eligibility. |

### 17.6 Initial connector priorities and module coverage

| Priority | Work | Reason |
|---|---|---|
| First | Preserve existing Internet Archive, PDF/URL, Crossref/DataCite intake | Already implemented foundations reduce delivery time. |
| First | PubMed topic search and Europe PMC/PMC eligible full-text resolution | Broad discovery plus structured text without unnecessary OCR. |
| First | Curated classical editions and one repertory dataset investigation | Supports comparison and structured repertory work. |
| Next | AYUSH/IJRH/CCRH discovery and selected permitted imports | India relevance without assuming universal bulk access or commercial permissions. |
| Next | Unpaywall and retraction updates | Better full-text resolution and ongoing integrity. |
| Later | OpenAlex, Semantic Scholar, CORE, additional libraries, registries and alerts | Expand coverage in response to measured gaps. |

| Module | Initial source IDs | Additional data/work |
|---|---|---|
| REP | 1–5, 15, 22 | Reviewed grade parsing, rubric/alias schema, deterministic scoring fixtures. |
| CASE | 11–15, 20–25, 33–34, 38, 54 | Structured case extraction; private case inputs stay separate. |
| PAPER | 13–21, 23–38, 45, 49, 55–56 | Connector records, deduplication, screening and appraisal. |
| MM | 3, 6–10, 15–17, 19, 35–38, 53 | Canonical remedy identities and source-backed comparison fields. |
| CDS | 20, 23–25, 33–34, 39–50, 52 | Current applicability and clinical review; data alone does not validate CDS. |
| CHAT | Eligible content across all groups | Conversation state and human-authored evaluation questions, not a separate medical chat training corpus. |
| CITE | Provenance across all groups; 23, 26, 30, 49, 54–56 | Bibliographic resolution, exports, support records, integrity updates. |

## 18. Model-provider and technical delivery requirements

### 18.1 Confirmed environment matrix

| Environment | Chat inference | Embeddings | Index consequence |
|---|---|---|---|
| Production | Hosted provider | Hosted provider | Production corpus indexed under its explicit embedding configuration. |
| Development, cloud | Hosted provider | Hosted provider | May use a compatible development copy/configuration of the production index. |
| Development, local | LM Studio | LM Studio | Use a separately compatible local index where the embedding model differs. |
| Development, mixed | Either supported option | Either supported option | Chat choice is independent; retrieval embeddings must match the selected index. |

The inspected client already supports independently configured chat and embedding providers, including LM Studio, OpenRouter, DeepInfra, and an OpenAI-compatible configuration. Preserve and extend that abstraction instead of creating parallel research implementations. This observation does not guarantee every provider/model combination has passed quality evaluation.

Relevant existing configuration includes `CHAT_PROVIDER`, `CHAT_MODEL`, `CHAT_BASE_URL`, `CHAT_API_KEY`, `CHAT_REVISION`, `EMBEDDING_PROVIDER`, `EMBEDDING_MODEL`, `EMBEDDING_BASE_URL`, `EMBEDDING_API_KEY`, `EMBEDDING_DIMENSIONS`, `EMBEDDING_REVISION`, `EMBEDDING_INPUT_STYLE`, provider-specific server credentials, `CHAT_MAX_TOKENS`, and `MODEL_REQUEST_TIMEOUT_SECONDS`. Earlier `AI_PROVIDER`/`AI_BASE_URL` values are fallbacks; inspect current implementation before changing configuration names.

| ID | Requirement | Acceptance behavior |
|---|---|---|
| MODEL-01 | Keep provider selection configuration-driven | Hosted/local development switching does not change ingestion, citation, or domain logic. |
| MODEL-02 | Separate chat and embedding configurations | A chat-model change does not force reindexing; an incompatible embedding change cannot query an old index. |
| MODEL-03 | Validate production inference configuration | Explicit production configuration requires suitable hosted endpoints and server-side credentials; startup errors are actionable and redact secrets. |
| MODEL-04 | Preserve reproducibility | Save model, provider, prompt, embedding configuration/revision, and relevant answer/index lineage. |
| MODEL-05 | Bound operational cost and latency | Apply request timeouts, bounded retries, usage limits, and per-run token/cost/latency observation where supported. |
| MODEL-06 | Make provider failures recoverable | Durable jobs distinguish transient failure, exhausted retries, configuration errors, and insufficient evidence. |
| MODEL-07 | Keep credentials out of clients and artifacts | No keys in browser bundles, exports, logs, examples, test fixtures, or planning documents. |
| MODEL-08 | Evaluate provider/model changes | Compare supported claims, completeness, abstention, latency and cost on the same representative tasks. |
| MODEL-09 | Control private-data transmission | Before private cases, define exactly which case text/excerpts each provider receives, with retention and access controls. |

Do not silently fail over embeddings to another model. Any future chat fallback must preserve provider-data rules and record the actual model used. No specific hosted model, paid plan, free-tier model, or throughput promise was selected by this discussion.

### 18.2 Architecture and deployment scope

Keep the modular Go backend, Vue 3/TypeScript frontend, PostgreSQL full-text search/pgvector, separate API/worker processes, and durable jobs. Extend through logical modules, schema migrations, and connector interfaces. Neither seven product modules nor international ambition by themselves require microservices, a graph database, a new vector database, or model fine-tuning.

Current configuration and production examples provide a starting point, not deployment certification. The research launch workstream should cover HTTPS, production session behavior, server-managed credentials, database and original-asset backups with a tested restore, monitoring, rate/usage limits, and capacity/cost measurements. Current local filesystem assets can remain during an appropriate small deployment; move to object storage when deployment or durability requirements justify it, without assuming earlier MinIO/S3 proposals are already implemented.

A broader service also needs account lifecycle, suitable roles, and access isolation for saved work. Distinguish existing token principals from full customer accounts, and reviewer answer isolation from organization/tenant isolation. The observed local session implementation should be reviewed for production cookie and lifecycle requirements rather than presented as production identity readiness.

## 19. UX requirements inherited from the POC

The user asked to use the UI direction already discussed. [POC PRD section 60](Homeopathy_AI_Source_Ingestion_POC_PRD.md) is the detailed UX baseline. It calls for a polished, calm, compact interface with plain-language next actions, not a large metric dashboard or many competing top-level screens.

Vue remains the frontend. shadcn-vue is now confirmed by the user in section 33; that decision is not evidence that the current app already uses it or that a wholesale component rewrite is required. Keep the visual and interaction direction while making focused improvements.

### 19.1 Navigation and workspaces

| Destination | Intended experience |
|---|---|
| Ask | Primary question/conversation view with source selection and contextual task choices such as Search papers, Compare remedies, and Research a case. |
| Sources | Compact library of books, papers, collections, import/discovery, and real preparation state. |
| Review | Authorized corpus-maintenance work: metadata, pages, rights, quality, publication. Keep ordinary research users out of admin chores. |
| Activity | Preserve current durable-job history, reopen, and retry; exact final placement may remain secondary. |
| Settings | Provider connections, repository settings, account/workspace settings and diagnostics under appropriate permissions. |
| Focused tools | Repertory and comparison workspaces opened in context, preserving the originating question and evidence access. |

Seven modules do not require seven equal top-level navigation items. Research users should consume a prepared shared library without having to operate its ingestion pipeline. Import and Review remain available for authorized users and private research workflows.

Section 26 refines this structure after the AnswerThis review: retain Ask, present Sources as the user-facing Library, add project organization incrementally, and keep Review/Activity/technical Settings secondary and role-appropriate. Cases, Repertory and Drafts become destinations only when their workflows are ready. This is an evolution of the POC shell, not a requirement to create all navigation items immediately.

### 19.2 Interaction requirements

| ID | Requirement |
|---|---|
| UX-01 | Explain what to do, what is happening, and what comes next with short nontechnical text and one clear primary action. |
| UX-02 | Preserve the question, draft case, selected sources, and reading position across recoverable failures and citation inspection. |
| UX-03 | Show honest queued/processing/review/ready/failed states based on actual jobs; accepted work is not completed work. |
| UX-04 | For source preparation show Add source → Read pages → Review → Prepare for questions → Ask. Other prepared sources remain usable during processing. |
| UX-05 | Use a readable answer width, neutral background, one restrained accent, subtle borders, consistent spacing, and compact lists/tables. |
| UX-06 | Open citations in a side panel with excerpt, author/work, edition/volume when known, and original page or structured source location. |
| UX-07 | Keep page review as a scan/text split view; stack/tab on narrow screens. Avoid page-wide horizontal scrolling; allow appropriate contained scrolling for comparison tables. |
| UX-08 | Explain empty, loading, blocked, retry, partial-answer, insufficient-evidence, and ready states with an appropriate next action. |
| UX-09 | Hide technical diagnostics and detailed research trails behind optional controls while keeping supporting sources easy to inspect. |
| UX-10 | Provide labelled inputs, keyboard focus, usable contrast, text status labels, accessible progress/errors, and reduced-motion support. |
| UX-11 | Example questions must match available corpus coverage; unfinished modules should not advertise working capability. |
| UX-12 | Show historical, abstract-only, full-text research, guideline, and case-report evidence types clearly without turning the main flow into a compliance checklist. |

Usability acceptance: a first-time user completes the relevant task without a developer walkthrough, opens evidence and returns without losing context, and recovers from a representative failure. Test laptop and narrow/mobile layouts, long source names, long answers, keyboard use, and multiple citations.

### 19.3 India pilot localization

Recommended initial scope: English interface and answers, mobile-friendly workflows, familiar domain terminology, and INR pricing experiments. Test Hindi/Hinglish input handling on representative questions before advertising support. If translated answers are introduced, label them, preserve original evidence, and evaluate meaning, negation, remedy names and rubric mappings. Full multilingual OCR and terminology coverage are future capabilities, not implied by a model's general language ability.

## 20. India launch and subsequent international expansion

### 20.1 Product positioning

Proposed initial promise:

> An AI research assistant for exploring homeopathic literature and contemporary scientific evidence, with inspectable sources and explicit evidence limitations.

The product should support learning, comparison, literature research, and clinician review. It should not promise cure, guaranteed remedy selection, or established effectiveness based on historical books or individual cases. [NCCIH's homeopathy reference](https://www.nccih.nih.gov/health/homeopathy) reports little evidence supporting homeopathy for a specific health condition; include such evidence limitations alongside positive, negative, and inconclusive research.

Pilot with a small India cohort and measure useful completed research tasks, repeat use, citation inspection, error reports, latency, and cost. Pricing in section 10 remains an experiment. Do not present prospective colleges, clinics, CCRH-related users, or government bodies as partners or endorsers.

### 20.2 India-specific scope work

Research features and clinical functionality can progress as separate release workstreams. Before patient-specific clinical recommendations, specify exact intended use and assess applicable Indian medical-software requirements against the actual function, not merely a “research” disclaimer. See the [CDSCO medical-device framework](https://www.cdsco.gov.in/opencms/opencms/en/Medical-Device-Diagnostics/Medical-Device-Diagnostics/).

For private patient workflows, define authorization, patient-data handling, retention/deletion, provider transmission, audit access, and applicable privacy requirements before ingesting real records. Review India's current [data-protection framework](https://www.meity.gov.in/content/digital-personal-data-protection-act-2023-dpdp-act) and commencement/applicability details when implementing those workflows; this document does not give a legal determination or assert all obligations have the same effective date. Relevant practitioner obligations can be researched through the [National Commission for Homoeopathy](https://www.nch.org.in/).

These are scoped product tasks. They do not require postponing ordinary public-literature discovery, citation features, or the case-free research beta.

### 20.3 International discussion retained for later planning

| Region | Finding from the conversation | Future planning action |
|---|---|---|
| EU/EEA | Software qualification/classification depends on intended function; clinical functionality may fall within medical-device rules. | Assess intended use with [European Commission software guidance](https://health.ec.europa.eu/system/files/2025-06/mdcg_2019_11_en.pdf) and other applicable current requirements. |
| UK | Clinical purpose and how outputs influence decisions must be clearly defined. | Use [MHRA intended-purpose guidance](https://www.gov.uk/government/publications/crafting-an-intended-purpose-in-the-context-of-software-as-a-medical-device-samd/crafting-an-intended-purpose-in-the-context-of-software-as-a-medical-device-samd). |
| United States | Citations/explainability alone do not establish a non-device CDS exemption. | Assess functions against [FDA CDS guidance](https://www.fda.gov/regulatory-information/search-fda-guidance-documents/clinical-decision-support-software). |
| South Africa | AI/ML medical-device requirements exist; Africa is not one approval jurisdiction. | Review [SAHPRA guidance](https://www.sahpra.org.za/document/regulatory-requirements-of-artificial-intelligence-and-machine-learning-ai-ml-enabled-medical-devices/) for that market. |
| Brazil | Software-as-medical-device requirements exist; the Americas are not one approval jurisdiction. | Review [ANVISA's software guidance](https://www.gov.br/anvisa/pt-br/assuntos/noticias-anvisa/2022/software-como-dispositivo-medico-perguntas-e-respostas/) and current updates. |
| Other countries | No blanket global clearance follows from an India launch or one regional assessment. | Select countries using demand and assess local claims, privacy, content rights and localization. |

International deployment must map the complete data flow, including model providers, application logs, support tools, and backups. An EU database region alone does not settle cross-border processing. [EDPB transfer guidance](https://www.edpb.europa.eu/sme/be-compliant/international-data-transfers_en) informs later EU planning. Before expansion, reassess commercial content territories, language-specific OCR/retrieval quality, customer support, payment, and operational requirements. This is retained future context, not a prerequisite to collecting the initial India research corpus.

## 21. Task-ready development backlog

These entries are candidate epics/tasks for future sessions, not claims of work started or tickets created. Recheck the live implementation before execution. Requirement IDs refer to this document. Split large entries into small vertical slices with their own acceptance evidence.

| Task ID | Deliverable | Dependencies | Acceptance evidence |
|---|---|---|---|
| BASE-01 | Reconcile code, README and PRD against this baseline | None | Explicit implemented/partial/missing inventory; older contradictory checkpoints qualified. |
| BASE-02 | Restore reproducible starter fixtures or an explicit acquisition setup | BASE-01 | The two observed missing-fixture tests run successfully without silently skipping required checks. |
| BASE-03 | Restore/create referenced evaluation assets | BASE-01 | Documented runner and reviewed fixture locations exist; README links resolve. |
| BASE-04 | Exercise the real source-to-answer workflow | BASE-02, BASE-03 | Import, review, rights, publish, READY, cross-source answer, citation scans, reload and unsupported query verified. |
| BASE-05 | Reconcile revision identity and content-approval gaps | BASE-01 | Task specification for stable source/revision identity, stale approval rejection and preserved citations; fix only confirmed gaps. |
| BASE-06 | Run disposable database recovery/access/provenance checks | BASE-01 | Relevant integration cases pass; retries, disable/restore, replacement, authorization and ownership behavior recorded. |
| DATA-01 | Implement a source collection manifest | Existing provenance | Exact work/edition, source URL, identifiers, module tags, evidence category, retrieval route, rights basis, priority and ingestion state recorded. |
| DATA-02 | Prepare classical seed corpus | DATA-01 | Selected 8–12 editions or initial smaller useful set acquired, reviewed and cited correctly; no duplicate edition inflation. |
| DATA-03 | Investigate one structured repertory source | DATA-01 | OOREP/other candidate assessed for export, licence, provenance, coverage and grade fidelity; one edition chosen. |
| DATA-04 | Define evidence-type and access-state metadata | DATA-01 | Metadata-only, abstract-only, full text, historical, case, trial, review and guideline states represented and used in UI/retrieval. |
| DATA-05 | Implement structured text acquisition | DATA-04 | Eligible XML/HTML can produce stable section/offset citations without invented pages; PDF/OCR fallback remains. |
| DATA-06 | Define India-specific manual/reference intake | DATA-01 | AYUSH/CCRH/IJRH/CTRI records saved under appropriate access/reuse routes; blocked bulk access does not block other sources. |
| DATA-07 | Normalize entities and terminology | DATA-02, DATA-04 | Remedy aliases, conditions and bibliographic identifiers linked without conflating distinct entities/preparations. |
| DATA-08 | Add correction/retraction refresh | Bibliographic connectors | Updated status affects eligible evidence and is surfaced on existing research where appropriate. |
| RES-01 | Add saved research organization | Existing saved answers | User can save/reopen questions, sources and findings with access isolation. |
| RES-02 | Add bibliographic export | CIT-01 | Chosen format round-trips through a reference reader; metadata and source identifiers remain correct. |
| CHAT-DEV-01 | Add persistent threads | CHAT-01, account scope | Create/reopen thread, preserve ordering and source choices, enforce ownership. |
| CHAT-DEV-02 | Ground conversational follow-ups | CHAT-DEV-01 | Pronoun/context and changed-source examples pass; unsupported follow-ups abstain. |
| PAPER-DEV-01 | Add PubMed topic search | DATA-04 | Query/filter/paginate/save works; results retain query/source/time and API failure state. |
| PAPER-DEV-02 | Add Europe PMC/PMC full-text resolution | PAPER-DEV-01, DATA-05 | Eligible full text enters existing review/index flow; unavailable items stay reference-only. |
| PAPER-DEV-03 | Deduplicate/enrich research records | Existing Crossref/DataCite, PAPER-DEV-01 | DOI/PMID/version links prevent repeated publications from appearing as separate evidence. |
| PAPER-DEV-04 | Add saved search/alert workflow | Search stable | Later slice stores query/filters and reports genuinely new matches with provenance. |
| MM-DEV-01 | Build cited comparison table | DATA-02, DATA-07 | User selects remedies/authors; each substantive cell has support; disagreements and gaps are explicit. |
| MM-DEV-02 | Evaluate comparison completeness | MM-DEV-01 | Reviewer fixtures check both sides, modality direction, negation, names, edition and omitted expected points. |
| REP-DEV-01 | Import versioned rubric/grade structures | DATA-03 | Sampled hierarchy/membership/grades match the original; ambiguous extraction is flagged. |
| REP-DEV-02 | Implement confirmed symptom-to-rubric mapping | REP-DEV-01 | Suggested mappings show exact paths and source; user edits/accepts ambiguity. |
| REP-DEV-03 | Implement deterministic scoring and explanation | REP-DEV-02 | Known inputs produce expected scores; versioned weights/grades and per-rubric contributions inspectable. |
| CASE-DEV-01 | Extract published cases | DATA-04, eligible case corpus | CASE-11 fields link to text; concurrent treatment, missingness and follow-up preserved. |
| CASE-DEV-02 | Search and compare published cases | CASE-DEV-01 | Similarity rationale and differences visible; historical cases distinguished from modern reports. |
| CASE-DEV-03 | Design private practitioner workspace | CASE-01–09, identity/privacy decisions | Separate case-input provenance, access matrix, retention/deletion, provider transmission and review workflow specified. |
| CASE-DEV-04 | Implement private case workflow and later follow-up | CASE-DEV-03, relevant REP/MM slices | Original input preserved, extracted facts confirmed, private data not leaked into shared retrieval; history inspectable. |
| CDS-DEV-01 | Define clinical intended use and assessment | CDS-01 | Exact users, outputs, influence, evidence needs and India applicability documented before recommendations. |
| CDS-DEV-02 | Add guideline/safety reference retrieval | CDS-DEV-01, selected reference corpus | Issuer/version/jurisdiction and applicability displayed; reference-only items are not represented as full text. |
| CDS-DEV-03 | Evaluate clinician-facing decision support | CDS-DEV-02 | Clinician-reviewed safety, evidence and failure cases pass agreed criteria; recommendation scope remains explicit. |
| MODEL-DEV-01 | Verify environment configuration matrix | MODEL-01–03 | Hosted/local/mixed development configurations behave correctly; production validates hosted setup. |
| MODEL-DEV-02 | Add operational model controls | MODEL-04–09 | Redacted errors, bounded retries, actual lineage, cost/latency and private-data rules verified. |
| UX-DEV-01 | Extend navigation/workspace structure | Existing POC UX | Ask/Sources/Review and secondary controls remain compact; tools preserve context. |
| UX-DEV-02 | Test usability and accessibility | Each shipped slice | First-time user, keyboard, mobile, long content, progress and recovery checks pass. |
| INDIA-01 | Test English and optional Hindi/Hinglish behavior | Reviewed question set | Supported language coverage measured; translations labelled and originals available. |
| LAUNCH-01 | Implement customer account lifecycle and scope | Product pilot scope | Roles, session behavior, ownership and source/answer/export access tested; organization isolation when offered. |
| LAUNCH-02 | Verify production operations | Deployment choice | HTTPS, credentials, rate limits, monitoring, backup and original-asset restore exercised. |
| LAUNCH-03 | Measure load and unit economics | Hosted model setup | Observed latency/concurrency and cost per workflow support proposed usage limits/pricing. |
| LAUNCH-04 | Run India research pilot | Increments 1–3 and minimum launch readiness | Real users complete discovery → evidence → saved/exported result; issues and repeat-use findings recorded. |
| EXPAND-01 | Choose next country and module investments | Pilot findings | Demand, language, content rights, product claims and operating requirements justify sequence. |

### 21.1 Minimum task specification

Use this template when creating an issue or development task:

```text
Task ID and title:
User outcome:
Related requirement IDs:
Observed current behavior and code references:
Exact gap:
In scope:
Out of scope:
Dependencies and data sources/editions:
Access, rights, privacy and provider implications (only where relevant):
Data model/API/UI changes:
Migration or reindex implications:
Acceptance examples and failure cases:
Verification method and required fixtures:
Release/rollback implications:
Completion evidence and documentation updates:
```

No ticket should use “add AI” or “support research” as its only acceptance criterion. Require an observable user workflow and evidence of correct behavior. Low-impact reversible changes do not need artificial testing bureaucracy; choose checks proportional to the actual risk and behavior changed.

## 22. Evaluation and release evidence

### 22.1 Quality measures

Measure retrieval recall against reviewed passages, supported-claim precision, expected-point completeness, correct author/edition attribution, citation resolution, abstention on out-of-scope questions, performance/cost and operational recovery. Report each metric with its dataset, method, reviewed examples and limitations. Do not substitute candidate citation coverage for correctness.

The inherited PRD calls for a scored held-out cross-source set, including at least ten Nash/Farrington comparison questions and five outside-corpus questions. Preserve those requirements while growing toward the proposed 50-task product set. Documentation's draft 30-question evaluation and proposed future 50-task coverage are not completed benchmark results.

| Module | Representative failure cases to include |
|---|---|
| CITE | Wrong page/edition, invalid export field, disabled/retracted source, citation after replacement. |
| CHAT | Ambiguous pronoun, changed source scope, user correction, unsupported prior answer repeated as fact. |
| PAPER | Duplicate DOI, preprint/version mismatch, abstract-only record, missing full text, API timeout/quota, retraction. |
| MM | Reversed comparison, mixed remedy names, unsupported modality, one-sided retrieval, conflicting sources. |
| REP | Wrong hierarchy, grade lost by OCR, alias collision, contradictory mappings, changed weighting/scoring version. |
| CASE | Missing timeline, concurrent conventional treatment, absent follow-up, similarity mistaken for efficacy, private-data leakage. |
| CDS | Outdated/nonlocal guideline, unsupported recommendation, omitted important safety context, historical claim presented as current evidence. |

### 22.2 Research beta release evidence

- User can discover a paper, save it, acquire eligible evidence, ask a grounded question, open support, and export a useful result.
- Prepared sources remain usable during background ingestion; failures preserve work and explain recovery.
- The human-reviewed quality report identifies strengths and limits of the selected corpus/model combination.
- Account/source/answer/citation/export access rules behave correctly for the offered sharing model.
- Production operations and backup restoration are exercised, and model costs/latencies support the pilot.
- Commercial content-use basis exists for ingested/displayed material; unresolved items remain useful discovery links where permitted.
- UI passes the focused usability checks in section 19.

Do not expand to exhaustive testing without a reason after appropriate checks pass. Conversely, do not mark missing fixture tests, unrun integrations, or unreviewed model answers as successful acceptance.

## 23. Open decisions and explicitly deferred work

### 23.1 Still open

- Exact first India customer segment, recruitment channel and pilot cohort.
- Final hosted providers/models, commercial plans, quotas, fallback behavior and model-data terms.
- Which exact book/repertory editions to ingest and which structured repertory export is usable.
- Final repertory scoring method, practitioner weighting defaults and ambiguity-review design.
- Per-item commercial permissions for noncommercial or unclear sources, including suitable alternatives.
- Initial paper filters, citation export format and breadth of evidence appraisal.
- Numerical quality, latency and cost thresholds; define before interpreting benchmark results.
- Customer account/organization scope for beta versus institutional release.
- English-only launch versus measured Hindi/Hinglish support, and later localization order.
- Prices, usage limits, trial duration and institutional packaging; earlier figures are hypotheses.
- Exact clinical intended use, patient-data workflow scope and required India assessment.
- Staffing, budget, delivery dates, and international expansion sequence.

These choices do not reopen confirmed decisions: India first, seven modules incrementally, independent case-free Ask, hosted production inference, pluggable development inference, and POC UX reuse.

### 23.2 Deferred until justified

- Training/fine-tuning a proprietary medical model or building a separate synthetic medical-chat corpus.
- Rewriting the application as microservices or changing frontend framework solely for a UI library.
- Requiring all 56 source integrations before any launch.
- Exhaustive systematic-review claims without recorded search/screening methods.
- Autonomous diagnosis/prescribing or unvalidated potency/dose recommendations.
- Full clinic EHR, institutional SSO, mobile-native apps, broad multilingual OCR, and global deployment expansion as first-release prerequisites.
- Treating public source availability, open-source software, or government association as endorsement or unrestricted dataset permission.

## 24. Discussion traceability and maintenance rules

| Discussion finding | Where captured |
|---|---|
| User considers POC done for now; acceptance still incomplete | Sections 6, 14, 22 |
| Complete seven-part coverage and missing capabilities | Sections 14–15 |
| Existing ingestion/citation/provenance assets worth preserving | Sections 5–6, 14.1 |
| Build passed; missing backend fixtures and evaluation folder | Section 6; BASE-02–04 |
| India-first correction to earlier international ambition | Sections 10, 13, 20 |
| At least 50 public sources across all modules | Section 17, numbered 1–56 |
| Sources are not all commercially ingestible or independent evidence | Sections 16–17 |
| Low-friction collection and few first integrations | Sections 16.3, 17.6 |
| Initial corpus/evaluation quantity proposals | Sections 16.2, 22 |
| Hosted production models and local/cloud development flexibility | Section 18 |
| Preserve Go/Vue/PostgreSQL/worker architecture | Sections 8, 18 |
| Preserve compact POC UI and usable mobile/keyboard flows | Section 19 |
| Case optional; published cases separate from private patients | Sections 3–4, 15, 16.4 |
| Deterministic repertory scores and user-confirmed mapping | Sections 15.1, 21 |
| Citation support is not scientific validation | Sections 5, 14, 16.4, 22 |
| Research beta before completion of every clinical feature | Sections 15, 20, 22 |
| EU/UK/US/Africa/Americas considerations retained for later | Section 20.3 |
| Existing pricing/audience/institution hypotheses preserved | Sections 7, 10, 23 |
| Future task generation with stable identifiers and acceptance | Section 21 |
| AnswerThis app inspection and public-page review, including limitations | Section 25 |
| Unified research workspace, evidence tables, library, reader and drafts | Section 26 |
| India-first landing-page value proposition, original copy and demo | Section 27 |
| Feature/use-case pages, pricing, guides, trust and acquisition | Section 28 |
| New task IDs, dependencies, staged rollout and acceptance | Sections 29–30 |
| UI-to-AI architecture, hybrid retrieval, provider interfaces and bounded workflows | Section 31 |
| Day-one data for every module, shared datasets and separate feature readiness | Section 32 |

Maintenance rules:

1. Update the relevant status row when a task ships; attach completion evidence rather than appending contradictory claims.
2. Preserve original assets, source IDs, exact editions, and evaluation lineage through changes.
3. Record source verification dates and actual ingestion state in the collection manifest; this static catalogue is not a live inventory.
4. Recheck current API, licence, clinical guidance and regulatory details when implementing the corresponding work. Dated external research is not permanent clearance.
5. Do not overwrite unrelated user work or copy credentials/private patient information into this document or future tasks.
6. Keep requirements, proposals, observed implementation and verified acceptance distinct.
7. Use this roadmap with the POC PRD and README; newer explicit user decisions take precedence over older proposals.

### Change log

- **2026-10-08, technical and data clarification:** Recorded the UI-to-AI-search discussion and the user's request for day-one data coverage across all seven modules. Added shared data layers, retrieval/citation design, provider portability, module readiness and task candidates in sections 31–32. Qualified earlier sequential collection interpretations; no requirement to choose classical literature instead of paper research. All-seven feature availability at public launch remains a separate decision.

- **2026-10-08:** Consolidated the seven-module scope audit, observed POC verification results, India-first decision, complete 56-source catalogue, collection strategy, production/development model matrix, POC UX requirements, task-ready backlog, release evidence, and retained international expansion context. Preserved the prior case-optional product model, audience proposals, architecture boundaries and commercial hypotheses. This update changes planning documentation only; it does not implement the roadmap or certify the POC.
- **2026-10-08, competitor and website expansion:** Added sections 25–30 after the AnswerThis app and public-site review. Captured adoption decisions, detailed workspace behavior, a proposed homepage and public sitemap, original draft copy, demo and acquisition flows, task dependencies, and acceptance criteria. Preserved all seven modules and the 56-source catalogue. No product code, billing, publication, or external account changes are implied.

## 25. AnswerThis review and adoption decisions

### 25.1 Review scope, evidence and limitations

Review date: **2026-10-08**. The user requested detailed inspection of AnswerThis as a reference for our application and subsequently requested these findings and landing-page recommendations in this document.

- **Direct app observation:** research composer/modes/settings, project selector, saved answer, source table, methodology tab, citation/export controls, library/import controls, global search, paper reader/chat, writer entry screen, MCP connections and upgrade entry points. Existing content was inspected; new-answer quality was not benchmarked.
- **Public-page review:** homepage and every distinct public destination in its main navigation/footer: eight feature/result destinations, three audience pages, Enterprise, Careers, Guide, Blog, Pricing, Contact, Terms and Affiliate. The register below records their canonical destinations, including redirects.
- **Additional public reading:** all four guide articles shown on the guide index; citation generator, paraphraser, research-gap preview and researcher/university directory entry pages. The six initially visible blog links were attempted; three returned readable article text and three did not return usable content in the browsing tool.
- **Limits:** this is not a recursive crawl of every blog archive, citation-style variant, individual researcher/university profile, external job listing, social network or payment/scheduling destination. Paid systematic review and several exports were gated. Integrations, payments, new uploads and generated extraction columns were not exercised. Public capability/security claims are descriptions by the vendor, not independently validated results.
- **Visual limit on the follow-up review:** the public homepage text was available through web retrieval, but the signed-in browser subsequently redirected the homepage to the application. Public-site content analysis is therefore stronger than responsive visual testing; no claim is made that every public page was visually tested at every device size.

The review informs original requirements. Do not copy their branding, screenshots, testimonials, prose or claimed partnerships into our product. References below are product-design evidence, not additions to the section 17 ingestion catalogue.

### 25.2 Public-page coverage and decisions

| Reviewed destination | Useful pattern observed | Decision for our public site |
|---|---|---|
| [Homepage](https://answerthis.io/) | Outcome-led hero, demonstration, workflow narrative, trust explanation, FAQ and repeated CTA | Adopt the clear journey with our own product evidence and shorter, domain-specific copy. |
| [AI Writer destination](https://answerthis.io/ai/ai-essay-writer) | The linked writer page redirects to a limited public essay tool | Prefer a cited research-brief example; label a preview separately from the full product. |
| [Library](https://answerthis.io/library) | Organization, reference imports and source-to-draft story | Show how saved sources and notes support a project; publish integration claims only after they work. |
| [Search results](https://answerthis.io/search-result) | Search leads into extraction, notes and writing | Demonstrate the complete search-to-evidence workflow rather than a search box alone. |
| [Literature-review preview](https://answerthis.io/literature-review-generator) | Bounded abstract-based preview with examples and explicit limits | Start with a reviewed sample; later offer a small live preview with clear scope and cost limits. |
| [Research gaps](https://answerthis.io/research-gaps) | A dedicated page explains the research-planning outcome | Later feature page for candidate gaps within a stated corpus; avoid universal novelty claims. |
| [Citation mapping](https://answerthis.io/citation-map) | Explains connections among papers | Defer graph marketing until graph functionality is implemented. |
| [Bibliometrics](https://answerthis.io/bibliometric-analysis) | Trends and author/journal activity presented as research tools | Later advanced-research page; citation activity must not stand in for study validity. |
| [Diagrams](https://answerthis.io/diagram) | Connects visual outputs to research deliverables | Initially demonstrate supported comparison tables; defer generated charts and slides. |
| [Pharma/medical sciences](https://answerthis.io/pharma-medical-sciences) | Audience-specific workflows and institutional CTA | Create a practitioner research page with our actual intended scope. |
| [Academia/education](https://answerthis.io/academia-educations) | Education-specific research outcomes | Create separate student/teacher and researcher examples, using one reusable page template. |
| [Industry/consulting](https://answerthis.io/industry-consultings) | Deliverable-oriented audience messaging | Adapt the focus on outputs; do not prioritize a generic consulting audience for India beta. |
| [Enterprise](https://answerthis.io/enterprise) | Deeper workflow explanation and sales/demo route | Add a simple college/research-team enquiry route first; defer a large enterprise feature suite. |
| [Careers](https://careers.answerthis.io/) | Company mission and recruiting destination | Optional when hiring; not a launch dependency. |
| [Guide index](https://answerthis.io/guide) | Task-based help near acquisition paths | Provide short guides tied to the first useful workflows. |
| [Blog index](https://answerthis.io/blog) | Educational content grouped by research task | Publish a small number of useful, reviewed domain guides; avoid a large generic content programme initially. |
| [Pricing](https://answerthis.io/our-pricing) | Audience/plan comparisons and billing-period controls | Clear INR pricing and usage definitions; exact prices remain section 10 hypotheses. |
| [Contact](https://answerthis.io/contact-us) | Support and institutional enquiries share a visible route | Require only the information needed to respond; institution/phone can be optional. |
| [Terms](https://answerthis.io/terms) | A dedicated policy destination | Write terms for our actual service, entity and data handling; do not reuse competitor legal text. |
| [Affiliate](https://answerthis.io/affiliate) | Referral explanation and promotional resources | Consider an educator/referral pilot after retention and unit economics are known. |

The homepage and public pricing material contain broad scale, accuracy and compliance claims; app entitlements and public pages also differ in places. We will maintain one internal record of our own published capabilities, prices and limits instead of treating competitor numbers as a reliable commercial specification.

Additional references read:

- [Literature review guide](https://answerthis.io/guide/answerthis-literature-review-tool), [welcome guide](https://answerthis.io/guide/welcome-to-answerthis), [data handling guide](https://answerthis.io/guide/answerthis-data-handling-guide), and [PDF chat guide](https://answerthis.io/guide/using-chat-with-pdfs-in-answerthis): useful models for task-specific onboarding and explicit source-access explanations.
- [Citation generator](https://answerthis.io/citation-generator): identifier/manual metadata input, format selection and a path into the full research app. Our first public utility could reuse existing bibliographic metadata services; formatting a reference must remain distinct from checking claim support.
- [Paraphraser](https://answerthis.io/ai/ai-paraphraser) and [gap preview](https://answerthis.io/ai/research-gap-finder): low-friction acquisition tools, but generic uncited generation is a weak first demonstration of our source-grounded value.
- [Researchers](https://answerthis.io/researchers) and [universities](https://answerthis.io/universities): directory-based discovery. Defer large directory/SEO projects; later prioritize a useful catalogue of our actual sources and editions.
- Readable blog examples included [sample-size planning](https://answerthis.io/blog/how-to-calculate-sample-size), [evidence-based practice](https://answerthis.io/blog/evidence-based-practice-steps-and-tools), and [checking AI references](https://answerthis.io/blog/how-to-check-references-for-ai-fabricated-citations). The useful acquisition pattern is practical education connected to an appropriate tool, not copying their medical or statistical content.

### 25.3 App patterns to adopt, adapt or defer

| Pattern observed in app | Decision | Related future requirement |
|---|---|---|
| One composer with task modes and tailored examples | Adopt; reveal only usable modes | WS-01–03 |
| Attach uploads or existing library context | Adopt within actual permissions and readiness | WS-04–05 |
| Source groups, access depth, date/type filters | Adopt supported filters; hide unavailable metadata filters | WS-05–06 |
| Length, tone and citation-style controls | Adopt a small initial choice set | WS-07 |
| Answer beside sources, plus methodology tab | Highest priority; extend existing citations/research trail | WS-08–10 |
| Custom evidence-table columns | Adopt fixed domain templates before free-form columns | WS-11–12 |
| Contextual follow-ups and saved work | Adopt with fresh retrieval and scope preservation | WS-13–14 |
| Project library, folders, tags and import controls | Adopt incrementally | WS-15–16 |
| Paper Read/Chat modes with notes | Adopt once document processing state is consistent | WS-17 |
| AI Writer handoff and export menu | Begin with editable research briefs and focused exports | WS-18 |
| Global search across answers, papers and drafts | Add after durable objects exist | WS-19 |
| Normal/maximum effort and usage display | Adapt as clear depth/cost options; core evidence inspection stays available | WS-20 |
| Citation graphs, bibliometrics, diagrams, presentations | Later advanced-research scope | ADV tasks |
| Full systematic-review pipeline | Later dedicated workflow with human screening/appraisal | ADV-01 |
| MCP and institutional integrations | Later, demand-led extension | ADV-04 |

Direct app references: [research workspace](https://app.answerthis.io/ask-answerthis?style=auto), [library](https://app.answerthis.io/ask-answerthis?tab=library&view=papers), [writer entry](https://app.answerthis.io/documents), [systematic-review entry](https://app.answerthis.io/slr), [MCP connections](https://app.answerthis.io/connections/mcp). These may require sign-in and are not promises of public access.

Observed weaknesses to avoid: a cramped wide evidence grid in a split view; many task choices competing on the home screen; hidden navigation labels; and inconsistent document readiness. In one inspected example, a PDF rendered in the reader but Chat requested a usable abstract or PDF. This is a single observed inconsistency, not a general failure-rate finding. Our processing state should distinguish asset availability from extraction/index readiness and explain recovery accurately.

## 26. Future research workspace requirements

### 26.1 Information architecture and user journeys

Recommended default: **research-first, case-optional**, with source-linked answers and prominent domain tools. This refines the prior recommendation without changing the confirmed two-entry-point product model. Exact labels can be tested with India pilot users.

| Area | Behavior and placement |
|---|---|
| Ask / New Research | Primary destination: composer, context selection, task choices and recent work. |
| Projects | Groups related threads, sources, evidence tables and briefs; a quick question need not start with project creation. |
| Library | User-facing name for prepared Sources plus saved research references; shared corpus and private collections are distinguishable. |
| Cases | Introduced with published-case research; private practitioner cases have separate access and data handling. |
| Repertory | Dedicated structured tool when ready; also reachable from Ask or a confirmed case workflow. |
| Drafts | Added when editable briefs exist; initially may be a project tab rather than a top-level item. |
| Review / Activity / Settings | Role-appropriate secondary tools; ordinary users do not see corpus-administration chores or provider diagnostics. |

Desktop: labelled compact sidebar, readable main content, collapsible/resizable evidence pane. Narrow screens: answer/evidence tabs and contained table scrolling. Avoid seven equally prominent launch destinations and avoid requiring a dashboard before a useful question.

Core journeys:

1. **Knowledge question:** select prepared sources → ask → inspect a cited passage → follow up → save a research note.
2. **Paper research:** topic/filters → search results → save selected records → access eligible abstracts/full text → build an evidence table → export a brief and bibliography.
3. **Materia medica:** select remedies and authors/editions → compare supported dimensions → inspect differing passages → save/export comparison.
4. **Repertory:** enter/choose symptoms → confirm rubric mappings → choose explicit weights → calculate → inspect contributions and associated materia medica.
5. **Case research:** choose published cases or separately authorized private case input → confirm extracted facts → inspect similarities and differences → review evidence and preserve notes.

### 26.2 Detailed functional requirements

| ID | Requirement and observable acceptance |
|---|---|
| WS-01 | One composer supports general Ask without a patient record. A user reaches a usable prompt from the primary navigation in one action. |
| WS-02 | Task modes select a real workflow with appropriate inputs and output schema. Asking for paper search must not silently return a generic corpus answer. Unavailable modes are hidden or clearly labelled planned outside the primary task row. |
| WS-03 | Example prompts change with task and available data. Selecting an example populates the composer, allowing inspection/editing before a model run. Examples identify the selected source set where relevant. |
| WS-04 | Users can attach permitted files or select library documents/collections. Display selected scope, access and preparation status before submission. Pending attachments do not silently become usable evidence. |
| WS-05 | Separate source origin, content access and processing readiness: classical text/paper/case/guideline; metadata/abstract/full text; queued/extracted/indexed/failed. These are independent fields rather than one overloaded badge. |
| WS-06 | A settings panel exposes available sources, date ranges, study types, language and access filters. Preserve filters in saved runs, disclose unsupported filters and provide reset. Citation counts or journal ranking are optional discovery metadata, never a clinical-validity score. |
| WS-07 | Start with concise/detailed outputs, plain/academic language, and a small supported citation-style set. Defaults require no configuration. Length targets must not force unsupported filler or remove material qualifications. |
| WS-08 | Results show readable answers with inline citations. Selecting a citation opens the supporting passage and original location without losing scroll position, draft input or filters. Evidence viewing is not blocked by a deeper-generation quota. |
| WS-09 | Evidence pane contains Sources, Evidence Table and Research Trail. Resize/collapse/full-width actions preserve state; keyboard and narrow-screen access work. Technical diagnostics stay separate from the readable trail. |
| WS-10 | Research Trail records actual query/rewrites, connectors, effective filters, timestamps, retrieved/deduplicated/selected counts, exclusions and failures when known. Never generate a plausible search history. Snapshot run inputs and relevant source revisions for later inspection. |
| WS-11 | Evidence tables start from fixed reviewed templates. Every substantive extracted cell includes a passage link, source revision and origin (abstract/full text/user input). Missing, conflicting, unavailable and failed extraction are distinguishable. |
| WS-12 | Later custom columns have a name, extraction question and expected value type. Extraction is scoped to selected rows; cost/progress/cancel/retry are visible. A failed row can be retried without charging for or replacing successful rows unnecessarily. User corrections retain the original value and edit history. |
| WS-13 | Follow-up suggestions reflect the current task, such as compare studies, explain a rubric, inspect disagreement or create a brief. Clicking a suggestion keeps explicit context; every new substantive claim is grounded again. |
| WS-14 | Saved threads retain ordered messages, effective source selection and immutable run references. Reopening preserves work. A new run after changing sources is labelled as new; earlier results are not silently rewritten. |
| WS-15 | Projects collect references to sources, threads, notes, tables and drafts without duplicating original files. Moving an item or creating a collection does not expand sharing permissions. Basic rename/archive/search can precede complex collaboration. |
| WS-16 | Library supports search, list/table view, tags, folders and import status. Save bibliographic records without demanding full text. Start with a focused BibTeX/RIS import/export path; add Zotero/Mendeley sync only after import behavior and permissions are tested. |
| WS-17 | Document reader supports original page/section navigation and source-scoped questions. Private highlights/notes retain stable anchors and source revisions. If extraction or indexing is unavailable, show the specific processing step and recovery; a readable PDF alone does not imply RAG readiness. |
| WS-18 | Turn an answer/table into an editable research brief with linked citations and bibliography. Editing claims marks their support as needing recheck where appropriate. Exports retain evidence scope, references and missing-evidence notices. Start with Markdown plus one citation format; add DOCX/PDF when output fidelity is verified. |
| WS-19 | Search saved papers, answers, notes and drafts with object-type filters and accessible keyboard invocation. Results enforce user/project access and open the matching object; no cross-user title/snippet leakage. |
| WS-20 | Display remaining usage and explain quick versus deeper research before expensive work. Depth changes retrieval/extraction budget, not whether citations are inspectable. Technical model/provider choice remains in appropriate settings; failures preserve work and reflect actual usage. |

### 26.3 Domain table schemas and evidence behavior

| Module/template | Initial fields | Required distinctions |
|---|---|---|
| Paper evidence | Identifier/title/year; design; population; sample size; intervention/preparation; comparator; outcomes; follow-up; limitations | Reported values versus reviewer interpretation; abstract versus full text; study/version identity |
| Materia medica | Remedy; source author/work/edition; symptom; location; modality; differentiating description; supporting passage | Separate authors' descriptions; preserve negation and direction; missing coverage is not absence of a symptom |
| Published case | Presentation; reported assessment; timeline; intervention; concurrent care; outcomes; adverse events; follow-up | Reported association versus causal inference; historical versus modern case; unreported values |
| Repertory | Edition; rubric path; remedy membership; source grade; user weight; score contribution | Imported grades versus user settings; deterministic calculation versus AI mapping suggestion |
| Clinical references | Issuer; publication/update date; jurisdiction; population; recommendation passage; applicability notes | Guideline versus historical text; evidence interpretation versus patient-specific advice |

Each table cell can have multiple support links. A summary must retain contradictions instead of averaging them into an invented consensus. Imported/AI-extracted values must be reviewable. Evidence category labels should be visible at source and cell level without adding intrusive checklists to ordinary reading.

### 26.4 Implementation boundaries and shared objects

Reuse Go APIs, Vue components, PostgreSQL, workers, existing source/page/claim lineage and provider abstractions. Begin with additive changes and recheck current implementation before creating new services. No microservice, vector-store replacement or frontend rewrite is required by this design.

Logical objects to map to existing models: Project, Collection, ResearchThread, Message, ResearchRun, SavedReference, EvidenceTable, ExtractionColumn, ExtractionCell, Annotation and ResearchBrief. These are design concepts, not instructions to create duplicate tables. Each persisted object needs ownership, timestamps, applicable source/run references and deletion/archive behavior. Private case input remains separate from shared corpus content.

Run lifecycle: draft → queued → searching/retrieving → extracting/checking → complete/partial/failed/cancelled. Steps can differ by task. Store actual worker progress and idempotency information; do not simulate a completed search during model latency. Retry resumes or creates a clearly identified run. Reopening a result must not trigger paid regeneration.

Provider portability remains section 18's requirement: hosted inference in production, hosted/LM Studio choice in development, and consistent request/response contracts. Persist actual provider/model lineage where appropriate; never put credentials or private prompts into public analytics or URLs.

### 26.5 Advanced capabilities retained for later planning

- **Systematic review:** protocol/question, eligibility criteria, reproducible searches, deduplication, title/abstract and full-text screening, exclusion reasons, extraction, human appraisal and reporting. Add dual reviewers/conflict resolution when the audience needs them. A narrative literature summary is not a completed systematic review.
- **Research gaps:** propose unresolved questions supported by the retrieved literature; record search coverage/date and contradictory findings. A sparse corpus cannot establish worldwide novelty.
- **Citation graph and bibliometrics:** related-paper navigation, publication trends and author networks based on available bibliographic links, with coverage and identity-resolution limits.
- **Charts, slides and writing assistance:** generate from reviewed tables and linked sources. Generic paraphrasing, plagiarism/AI detection claims and presentation generation are not beta prerequisites.
- **MCP/API/institutional connections:** scoped permissions, revocation, audit and private-data controls; implement after an actual integration need is identified.

## 27. Public website positioning and homepage requirements

### 27.1 Our value proposition and message hierarchy

**Proposed positioning:** a source-linked homeopathy research workspace for students, educators, researchers and practitioners, launching in India. Help users find relevant material, understand what it says, compare sources and retain a usable research output.

The public site should communicate five concrete values:

1. **Find the passage behind an answer:** source, author, edition and original location are inspectable.
2. **Study classical literature alongside research:** distinguish historical descriptions, case reports, trials and guidance.
3. **Compare without losing attribution:** see which source supports each difference, similarity or gap.
4. **Keep research together:** questions, papers, notes and drafts belong to a continuing project.
5. **Start with a question:** no patient case or technical setup is required for general research.

These are target product benefits. Publish each as available only after the supporting workflow passes acceptance. Do not advertise all seven modules as shipped merely because the roadmap includes them. Avoid cure/outcome promises, invented scale metrics, blanket accuracy guarantees, unverified endorsements and unsupported security labels. This is a concise content rule, not a new approval layer for routine development.

The existing price/audience proposals remain hypotheses. Research-first is the recommended homepage emphasis; later audience pages can lead with practitioner, student or researcher tasks without changing the shared product.

### 27.2 Original draft hero and CTA copy

Use this as the copywriting starting point, adapting to release readiness:

| Element | Proposed copy / behavior |
|---|---|
| Eyebrow | **For homeopathy study and research** |
| Headline | **Explore homeopathy literature. Follow every source.** |
| Research beta subheading | **Ask questions across our prepared library, inspect the passages behind answers, and keep your findings together. A focused research workspace for students, teachers and practitioners.** |
| Expanded subheading after paper search/comparison ship | **Search classical texts and research papers, compare remedy descriptions, and build cited research notes in one workspace.** |
| Primary CTA during invitation pilot | **Request beta access** |
| Primary CTA when self-service beta works | **Start researching**; use **Try free** only when real free entitlements are clearly defined |
| Secondary CTA | **Explore a sample answer** |
| Short reassurance | **Start with a question. No patient case required.** |
| Intended-use note near demo/FAQ | **For learning and research. Review original sources and use professional judgment for clinical decisions.** |

Avoid generic AI model logos as the central promise. Hosted provider choice and LM Studio compatibility belong in development/admin documentation, not the customer hero. The CTA text must match its destination: beta request, working signup or sample, never an unexpected checkout.

### 27.3 Homepage sequence and content specification

| Order | Section | Content and action | Evidence/readiness needed |
|---|---|---|---|
| 1 | Header | Product, Who it's for, Sources & Evidence, Pricing, Guides; Sign in; one primary CTA. Mobile menu retains labels. | Every visible link resolves to a useful route or section. |
| 2 | Hero | Copy above with a real research-workspace screenshot or small sample preview. | Current UI capture with safe demo content; no invented functionality. |
| 3 | Try a sample | Three tasks: explain a passage, inspect a paper summary, compare source descriptions when available. | Reviewed source-linked examples, clearly marked saved demonstrations. |
| 4 | Research frustrations | Briefly address scattered books/papers, difficult source checking and repeated note-taking. | Domain language validated with pilot users; no unsupported time-savings percentage. |
| 5 | How it works | Ask or choose sources → inspect evidence → compare/save → export. | Each step links to a real screen or labelled preview. |
| 6 | Product walkthrough | Show answer + passage, evidence table, project library and a cited brief. | Only shipped screens appear under available capabilities. |
| 7 | Seven-part direction | Cards for all seven modules with Available/Beta/Planned status and concise outcomes. Group planned modules below available features. | Shared capability registry, updated per release. |
| 8 | Sources and evidence | Explain source categories, exact editions, access depth and coverage. Link to catalogue/method page. | Actual corpus inventory; metadata discovery count kept separate from full-text count. |
| 9 | Audience paths | Student/teacher, researcher and practitioner cards with one relevant example each. | Dedicated content or meaningful sections; case entry remains optional. |
| 10 | Trust through demonstration | Explain how a citation opens an original passage and where uncertainty is shown. | A working example; no claim that traceability proves clinical effectiveness. |
| 11 | Pilot stories | Optional short feedback about research usability, with permission and context. | Real quotes/participants; omit section until available. |
| 12 | Pricing preview | INR plan/usage summary or beta-access terms, link to full details. | Approved commercial configuration; no fabricated scarcity or discounts. |
| 13 | FAQ | Answer practical coverage, citation, access, privacy and availability questions. | Matches actual product behavior and provider arrangements. |
| 14 | Final CTA and footer | Repeat the primary action; link contact, guides, terms, privacy, data handling and source policy. | Functional routes and contact path. |

Keep the first release compact: homepage sections can satisfy several proposed feature/use-case destinations before separate pages are justified. A full sitemap is a growth plan, not a condition to launch the pilot.

### 27.4 Demo specification and sample prompts

First version: a static or locally interactive, reviewed answer with real citations and original-source access. This gives visitors a useful demonstration without a paid model call, login or upload. Clearly state **Sample result — prepared from the listed sources**, its preparation date and source set.

Sample prompts are candidates, not verified coverage claims:

- “Explain this passage from the selected Organon edition.”
- “Compare the modalities described for Nux vomica and Pulsatilla in these selected materia medica sources.”
- “Summarize the methods and limitations of this open-access paper.”
- “Show how this selected repertory rubric contributes to the score.” — only when repertory analysis ships.

Select the actual examples from reviewed material in our library. A comparison needs support for both remedies and a visible source constraint. Use no real patient details or unsupported treatment-success statements. Mark omissions and abstract-only findings.

Interaction acceptance: choose an example → read output → open at least two different source passages → return without losing position → choose Start researching. Preserve example/task selection across signup using a safe identifier; do not put private free-text questions in query strings. If a later live preview is added, disclose its scope, remaining runs, retention and whether results transfer into the account. A failed generation should not consume a successful-run allowance.

### 27.5 Draft FAQ requirements

| Question | Answer content required |
|---|---|
| Who is this for? | Students, teachers, researchers and practitioners using the shipped learning/research workflows. |
| Do I need to enter a patient case? | No. General questions and paper research are independent workflows. |
| Which books and papers are included? | Link actual titles/editions and search coverage; distinguish prepared full text from discoverable records. |
| Can I see where an answer came from? | Demonstrate citations and supporting passages; explain missing-source/insufficient-evidence states. |
| Does a citation mean a treatment is proven? | Explain that source support and strength of clinical evidence are different questions. |
| Can I upload my own documents? | State supported formats, limits, preparation steps, access and reuse responsibilities for the release. |
| Can I use repertory or case analysis? | Display current availability and intended users; link planned capabilities honestly. |
| Is this a diagnosis or prescribing service? | State the actual research scope; do not imply autonomous clinical care. |
| Are my documents private? | Describe actual account access, provider processing, retention and deletion with a data-handling link. |
| Which languages work? | State evaluated language coverage; do not infer multilingual support from model marketing. |
| What is included in free/paid access? | Explain searches, deeper runs, document limits, exports, reset period and billing terms clearly. |
| Can a college or research team use it? | Link an enquiry route and distinguish offered collaboration from future plans. |

### 27.6 Visual, accessibility and performance requirements

- Use the same calm visual language as the POC: neutral background, one restrained accent, readable typography, consistent spacing and subtle borders. The product screenshot should be more prominent than decorative stock photography.
- Show a legible research task and citation interaction above or just below the initial viewport. Do not shrink the whole application into an unreadable image.
- Mobile layouts stack text and evidence demonstrations, maintain clear touch targets, and avoid page-wide horizontal scrolling. Use a contained scroller or simplified rows for comparison previews.
- Use semantic headings, labelled forms, keyboard-operable menus/accordions, visible focus, meaningful alternative text and reduced-motion behavior. Target WCAG 2.2 AA during implementation; this document is not a compliance certification.
- Avoid autoplay video as the only explanation. Provide a lightweight poster, optional playback, captions/transcript and a text walkthrough. Lazy-load below-fold media.
- Proposed performance targets: mobile LCP ≤2.5 seconds, INP ≤200 ms and CLS ≤0.1 at the 75th percentile once sufficient field data exists. Before traffic exists, record repeatable lab checks on agreed devices/network settings; do not treat a single Lighthouse score as field proof.
- Prefer rendered/prerendered public content, descriptive titles and canonical routes. Keep private app pages, research results and account data out of public indexing. Reuse the current frontend where practical; a marketing-site framework migration is not required.

## 28. Public website pages and acquisition workflows

### 28.1 Proposed sitemap with release priorities

Paths below are proposed routes, not files or pages already implemented. **Launch** means required for the offered public beta experience; **as feature ships** means publish with working capability; **later** means demand-led expansion.

| Route | Purpose and core content | CTA | Timing |
|---|---|---|---|
| `/` | Value proposition, sample, workflow, audience, evidence, FAQ | Start researching / Request beta access | Launch |
| `/product` | Concise capability overview and visible release status for all seven modules | Explore sample | Launch; can begin as homepage anchor |
| `/features/research-assistant` | Ask, grounded follow-ups, source inspection and saved research | Try a source-linked question | Launch |
| `/features/paper-search` | Search scope, filters, access states, saving and full-text preparation | Search papers | As feature ships |
| `/features/materia-medica` | Source/edition-aware comparisons and inspectable table cells | Open sample comparison | As feature ships |
| `/features/repertory` | Rubric selection, confirmed mappings, versioned grades and scoring explanation | Explore rubric example | As feature ships |
| `/features/case-research` | Published-case research first; clearly separate future private-case handling | Explore a published case | As feature ships |
| `/features/clinical-support` | Exact intended use, supporting evidence and clinician-review boundaries | Learn about scope / appropriate demo | Only with evaluated offered scope |
| `/features/citations` | Passage support, bibliography formatting and exports | Inspect a citation | Launch when workflow is ready |
| `/features/library` | Sources, projects, notes, imports and preparation states | Build a research collection | As feature ships |
| `/for/students-and-teachers` | Source reading, concepts, comparisons and cited learning notes | Try a learning example | Launch or homepage section first |
| `/for/researchers` | Paper discovery, evidence matrices, research briefs and methodology | Explore research workflow | Launch or homepage section first |
| `/for/practitioners` | Literature questions and comparisons; separate research from patient-specific support | Explore practitioner research | Launch or homepage section first |
| `/institutions` | College/research-team pilot, actual collaboration offer, onboarding | Request a team demo | Simple enquiry at launch; deeper content later |
| `/sources` | Actual source catalogue, editions, evidence categories, access and update information | Browse sources / inspect example | Launch |
| `/how-evidence-works` | Retrieve → cite → inspect → review; limitations and correction reporting | Open evidence demo | Launch |
| `/pricing` | India INR pricing or explicit invitation-beta terms, quotas and billing FAQ | Correct signup or contact flow | Launch |
| `/guides` | Task-based getting-started instructions | Complete one task | Launch with a small useful set |
| `/research-notes` | Reviewed educational articles and product examples | Relevant sample or workflow | Later; first articles can live under Guides |
| `/about` | Product purpose, actual team/entity and contact | Contact | Launch; compact page |
| `/contact` | Support, source correction and institutional enquiries | Submit enquiry | Launch |
| `/privacy`, `/terms`, `/data-handling` | Accurate service, provider/data and user-rights information | Relevant contact or settings path | Launch |
| `/source-policy` | Content sourcing, permissions, attribution and correction/takedown contact | Report a source issue | Launch; can be a Sources section |
| `/tools/citation-formatter` | Small metadata-based public utility with editable references | Continue research in app | Later, after bibliography functionality |
| `/partners` | Optional educator/referral programme and clear terms | Apply / enquire | Later |

Do not create thin near-duplicate pages for every condition, remedy, city or language. A dedicated page needs a useful domain example, distinct outcome and working next step. No remedy page should present an unsupported efficacy claim to attract search traffic.

### 28.2 Reusable feature and audience page templates

**Feature page:** specific user outcome → current availability → real screen/sample → 3–4 steps → input/source requirements → output example → limitations and FAQ → relevant CTA. Include a concise link to the adjacent workflow, such as Paper Search → Library → Evidence Table → Brief.

**Audience page:** name the audience's actual task → show one relevant example → explain the output they keep → provide source-inspection proof → link 2–3 relevant workflows → CTA. Student copy should focus on learning and attribution, researcher copy on comparison and reproducibility, practitioner copy on literature access and review. Institution branding/logos require a real permitted relationship.

**Guide page:** state the task and prerequisites, show a small reproducible example, explain output and failure recovery, link original sources when discussing evidence, and record author/reviewer/update date. Keep screenshots consistent with the shipped UI.

Suggested first guides: ask without a case; inspect an answer's citation; understand abstract versus full text; organize a research project; compare materia medica sources when available; export a bibliography. These directly support activation instead of expanding the first-release feature set.

### 28.3 Pricing and conversion requirements

- India-first display in INR; separate monthly price from annual total and show applicable tax treatment and renewal terms accurately. Section 10 prices remain proposals until unit economics and pilot demand support them.
- Start with the smallest viable plan structure; student/practitioner/researcher personas do not automatically require separate billing products. A limited free/pilot tier and one paid research tier can be evaluated before a complex plan matrix.
- Define what consumes usage: quick question, deeper search, extraction row/document, OCR pages, storage and exports where applicable. Explain reset timing and failed-run handling. Do not advertise unlimited expensive inference without a sustainable defined policy.
- All tiers that display an answer should allow users to inspect its cited evidence. Paid differentiation can be greater capacity, deeper workflows, larger private collections or team features when available.
- Pricing page, upgrade dialog and backend entitlements use the same plan configuration. Show total payable before checkout. A public CTA must not describe a paid checkout as free access.
- Preserve safe task/example context through registration. Return the user to the intended workflow instead of a generic onboarding dashboard.
- Institution enquiries ask for name, email, organization if relevant, approximate need and message; phone is optional. Do not ask visitors to send patient records in an enquiry form. Show success, failure/retry and an alternative contact route.

### 28.4 Public trust content and claim maintenance

Maintain a small capability/content record for each feature: status, release/version, evidence link, supported scope, responsible owner and last-reviewed date. Reuse it in the app, homepage, feature pages, pricing and guides. Before publishing copy, verify the claim against this record.

| Claim/content type | Minimum supporting record |
|---|---|
| Available feature | Working end-to-end acceptance example and current release |
| Corpus size | Dated inventory with unit defined: works, editions, records or accessible full texts |
| Connector logo | Working connector and appropriate use of the logo; source access is not a partnership |
| Citation quality | Evaluated measure, dataset, version and limitations; avoid absolute guarantees |
| Time saved | Measured task/user comparison with method; otherwise describe the workflow benefit without numbers |
| Testimonials | Permission, accurate context and actual wording; no invented participants or treatment outcomes |
| Privacy/security | Actual access controls, provider terms/settings, storage and retention behavior |
| Collaboration/institutional offer | Current sharing/role behavior and offered support; enquiry interest alone is not a shipped feature |
| Language support | Reviewed examples and measured coverage for the advertised language |

Publish an understandable data flow: user input/documents → our storage/retrieval → selected processing/model providers → saved output. Describe actual deletion, retention and training-use arrangements. Do not call server-side provider processing end-to-end encrypted merely because transport and storage encryption are present. This adds clarity to the offered service without requiring an enterprise certification project for the research beta.

## 29. Competitor-derived development backlog and delivery order

This extends section 21. IDs below are task candidates, not created tickets or completed work. Reuse existing tasks where outcomes overlap; new IDs identify narrower slices rather than requiring duplicate implementation. Dependencies can be satisfied by existing code once verified.

### 29.1 Workspace implementation tasks

| Task ID | Deliverable / requirement | Dependencies or existing task | Acceptance evidence |
|---|---|---|---|
| WORK-01 | Research shell, labelled navigation and task composer; WS-01–03 | UX-DEV-01, existing Ask | Case-free question, task-specific example, preserved input and usable narrow-screen flow. |
| WORK-02 | Source/context/settings panel; WS-04–07 | DATA-04, existing source eligibility | Supported filters persist into run; unavailable/private/unready sources handled explicitly. |
| WORK-03 | Answer/evidence pane and Research Trail; WS-08–10 | Existing citations, CIT-03 | Citation opens original passage; actual search metadata visible; resize/tab preserves context. |
| WORK-04 | Persistent context and follow-ups; WS-13–14 | CHAT-DEV-01–02 | Reload and changed-scope follow-up work; prior generated text is not treated as source evidence. |
| WORK-05 | Projects and library organization; WS-15–16 | RES-01, LAUNCH-01 | Save paper/answer/table into project, reopen and search; ownership/isolation preserved. |
| WORK-06 | Fixed paper extraction table; WS-11 | PAPER-DEV-01–03, DATA-04 | Reviewed cells link to actual abstract/full text; missing/conflicting values preserved. |
| WORK-07 | Materia medica table presentation | MM-DEV-01–02, WORK-03 | Two-sided comparison with author/edition attribution and usable mobile presentation. |
| WORK-08 | Custom extraction columns; WS-12 | WORK-06, MODEL-DEV-02 | Typed schema, scoped rows, progress, per-row failure/retry and support links. |
| WORK-09 | Reader, annotations and document chat; WS-17 | Existing original-asset reader, WORK-04 | Stable anchors; private notes; processing/index mismatch gives actionable state. |
| WORK-10 | Research brief and export; WS-18 | RES-02, CIT-01–02 | Editable brief preserves citations; changed claim support marked; export opens correctly. |
| WORK-11 | Search across saved work; WS-19 | WORK-04–05, relevant draft objects | Paper/answer/note/draft filters work without access leakage. |
| WORK-12 | Depth and usage controls; WS-20 | MODEL-DEV-02, LAUNCH-03 | Cost/limit explained before run; retry billing correct; source inspection still available. |
| WORK-13 | Specialist workspace consistency | REP/CASE/CDS development tasks, WORK-03 | Shared evidence interaction reused while each module keeps its validated domain model. |

### 29.2 Public website and growth tasks

| Task ID | Deliverable | Dependencies | Acceptance evidence |
|---|---|---|---|
| SITE-01 | Capability registry and original copy inventory | BASE-01, section 27 | Every public capability has status, scope and proof; planned features visibly distinct. |
| SITE-02 | Responsive homepage and shared public layout | SITE-01, existing design system | Correct hero/CTA, readable sample, working links and keyboard/mobile navigation. |
| SITE-03 | Reviewed interactive sample | Available corpus, WORK-03 or equivalent | Two citations open real sources; sample/date/scope labelled; no model call for static demo. |
| SITE-04 | Sources and evidence-method pages | DATA-01, DATA-04, SITE-03 | Real editions/access categories, coverage date, example and source-correction route. |
| SITE-05 | Reusable feature pages | SITE-01–02, corresponding shipped workflow | Each page has a distinct task, actual example, current status and working CTA. |
| SITE-06 | Audience pages | SITE-02–03 | Student/researcher/practitioner copy differs meaningfully; preserves case-optional entry. |
| SITE-07 | Pricing and signup handoff | LAUNCH-01, LAUNCH-03, chosen pilot offer | INR and totals correct; entitlements match backend; example context survives registration. |
| SITE-08 | Contact and institution enquiry | Operating contact destination | Validation, success/failure and spam controls; optional phone; no patient-data request. |
| SITE-09 | About, terms, privacy and data-handling content | Actual entity/provider/storage choices | No placeholder entity/jurisdiction; statements match implementation; footer routes resolve. |
| SITE-10 | Initial task guides | Relevant shipped workflows | A new user completes the documented task; screenshots and availability accurate. |
| SITE-11 | Public SEO, accessibility and performance | SITE-02–10 as published | Titles/canonical/sitemap, private-route exclusion, keyboard/mobile checks and measured loading behavior. |
| SITE-12 | Minimal funnel measurement | Privacy choices, SITE-02–07 | Events record stage and safe example IDs; no prompt/document/patient content in analytics. |
| SITE-13 | Public citation-formatting utility | CIT-01–02, SITE-12 | Metadata/manual input, missing-field handling and reference export; clear formatting-only scope. |
| SITE-14 | Pilot evidence and stories | LAUNCH-04 | Permissioned research-usability feedback, measured activation/return use; no invented proof. |
| SITE-15 | Educator/referral experiment | SITE-14, viable unit economics | Clear terms, attribution and measured acquisition quality; not a beta dependency. |

### 29.3 Later advanced-research tasks

| Task ID | Candidate scope | Start condition |
|---|---|---|
| ADV-01 | Systematic-review protocol, screening, exclusion log, extraction and appraisal | Core paper workflow reliable and pilot demand demonstrated; define methodology and reviewer roles first. |
| ADV-02 | Corpus-bounded research-gap exploration | Search coverage, deduplication and evidence tables reliable. |
| ADV-03 | Citation graph, bibliometrics and reviewed-data charts | Bibliographic linkage/coverage adequate and users need these outputs. |
| ADV-04 | MCP/API, reference-manager sync and institutional integration | Concrete customer workflow plus scoped access/revocation specification. |
| ADV-05 | Team editing, advanced drafts, presentation exports | Saved briefs, access rules and export fidelity established. |

### 29.4 Recommended execution sequence

1. **Baseline and reusable shell:** existing BASE tasks plus WORK-01–03. Prepare SITE-01 and original homepage content alongside this work.
2. **Demonstrate value publicly:** SITE-02–04 and SITE-08–11 in proportion to the offered pilot. Use a reviewed static sample first. A waitlist can precede self-service signup; paid plans wait for actual entitlements and billing readiness.
3. **Preserve research:** WORK-04–05 and WORK-10, using existing citation/export tasks. Connect signup/usage and measure activation through SITE-07/12 when the app is ready.
4. **Paper and comparison value:** paper connectors, WORK-06–07 and reader work; publish corresponding feature pages. Run the India research pilot after the existing section 15 increments 1–3 acceptance, without waiting for all advanced features.
5. **Remaining domain modules:** follow section 15's repertory → published-case → clinically reviewed support sequence. Published-case extraction may proceed earlier if data is ready; private patient workflows remain a separate expansion.
6. **Demand-led depth:** custom columns, advanced reviews, graphs, teams, integrations and referral programmes after repeat use and economics justify them.

This sequence preserves all seven modules, source collection, provider flexibility and international ambition. It does not require all public pages, all 56 sources, a custom model, a new architecture or a full enterprise suite before launch. Staffing and dates remain open.

## 30. Acceptance, measurement and maintenance for the new direction

### 30.1 End-to-end acceptance scenarios

| Scenario | Expected evidence |
|---|---|
| Visitor explores sample without signup | Source-linked sample works on laptop/mobile, distinguishes saved demo from live search, and exposes no private data. |
| Visitor starts research | CTA leads to the stated access/signup flow; selected safe example/task is retained; no patient record required. |
| User asks and checks evidence | Answer references resolve; original passages are readable; returning preserves question and position. |
| User searches papers with limited full text | Metadata saves successfully; abstract-only status is clear; unavailable full text does not become invented extraction. |
| User creates comparison | Substantive cells cite source passages; both sides and disagreements/missingness are represented. |
| User saves/reopens project | Threads, source references, notes and briefs persist without automatic rerun or access expansion. |
| A document is readable but not indexed | UI explains readiness accurately; offers the applicable preparation/retry action; does not falsely say full text was used. |
| Provider/connector fails | Honest partial/failed state, preserved inputs and bounded retry; successful sources remain inspectable. |
| Feature not yet released | Marketing labels it planned; app avoids a misleading working action; no checkout implies entitlement. |
| User exports a brief | Bibliography and support links match the saved run; format opens correctly and preserves material qualifications. |
| User reaches privacy/pricing/help | Public statements match actual release, terms and plan configuration; links work without needing app access. |

Use section 22 quality checks for answer correctness, source fidelity and clinical scope. Test only the behavior affected by each slice, with deeper checks for access, billing, evidence integrity and patient-data functionality. A documentation-only edit needs structural/content checks, not a new application test suite.

### 30.2 Pilot and acquisition measurements

Measure the user journey: landing visit → sample opened → source inspected → access/signup completed → first supported answer → source inspected in app → research saved/exported → return visit. Inspect drop-off by audience page and safe example ID. Avoid capturing raw queries, documents, clinical inputs or sensitive URLs in marketing analytics.

Candidate metrics: sample-to-signup conversion, signup-to-first-supported-answer activation, source-inspection rate, successful saves/exports, repeat use within an agreed pilot window, retrieval/support failure rate, median and tail latency, cost per completed workflow, and paid willingness/retention. Define denominators, observation windows and targets before claiming improvement. Source clicks indicate engagement, not answer correctness; evaluate correctness separately.

Use the pilot to decide whether the homepage should emphasize learning, literature research or practitioner reference. Do not change the product into a generic writing tool merely because an uncited demo produces more clicks.

### 30.3 Maintenance and open choices

- Update capability status and linked public copy as part of each release; include new screenshots and guide revisions when behavior changes.
- Maintain competitor review date and observation level. Recheck external pricing and claims when relevant; do not treat this review as a permanent feature/entitlement inventory.
- Keep proposed copy separate from published commitments. Final brand/domain, audience emphasis, plan terms, sample corpus, citation-format priority and numerical pilot targets remain open.
- Record first-launch pages versus later pages in actual tasks; use the section 21.1 task template and IDs from section 29.
- Preserve earlier source catalogue, module requirements and POC evidence. This update adds design and acquisition direction; it does not change implementation status, certify the POC, publish a site or mark any future task complete.

## 31. Technical direction from UI to AI search

### 31.1 Status and architecture recommendation

This section records the technical discussion following the competitor review. Retaining the existing stack and provider flexibility follows the established product direction. Specific interfaces, schemas, retrieval methods and transport choices below are **implementation recommendations to verify against current code**, not claims that they are already implemented or independently approved architecture decisions.

Keep Vue, Go, PostgreSQL and existing ingestion workers. Build a shared research engine behind all seven workflows, using existing source, revision, passage and citation infrastructure. The principal investment is data quality, retrieval, structured domain knowledge and reliable evidence presentation. Models remain replaceable components.

Start with a modular application and durable workers. Do not require microservices, a new frontend framework, a separate vector database or a proprietary trained model before launch. Evaluate existing retrieval/storage capability and measured workload before introducing additional infrastructure.

### 31.2 UI and API contract

| UI component | Responsibility | Backend contract |
|---|---|---|
| Research composer | Question, task, selected sources, attachments | Validated task input and explicit source constraints |
| Answer view | Readable response, citations, follow-ups | Structured answer blocks, citation IDs and answer status |
| Evidence panel | Supporting excerpts and original pages | Stable evidence IDs, source revision and page/section location |
| Evidence table | Paper extraction and remedy comparison | Typed rows/cells with support links and missing/conflicting states |
| Research trail | Actual searches, filters and limits | Persisted run events and effective configuration |
| Projects/library | Saved threads, documents, notes and results | Owned durable objects with access-filtered queries |
| Repertory workspace | Rubric confirmation, weights and contributions | Structured rubric records and deterministic scoring output |

The frontend should not recover citation relationships or domain-table structure by parsing arbitrary model-generated Markdown. Define a versioned application response schema for narrative blocks, citations, evidence, coverage warnings and run status. Markdown can remain a presentation/export format. Validate model output before converting it into application records.

Recommended interaction: create a durable research run, return its identifier, then use **server-sent events (SSE)** for progress and answer streaming. Record enough state to reopen or reconnect without starting another generation. Provide a state-fetch/polling fallback if the stream disconnects. Distinguish provisional streamed text from completed checked output; an interrupted stream must not appear as a fully verified answer.

Long searches, document preparation and extraction run in workers. The browser reports actual progress, preserves inputs on failure, and reopens completed results without paid regeneration. See WS-08–20 for detailed user behavior.

### 31.3 Backend responsibilities

| Internal module | Responsibility |
|---|---|
| Research orchestration | Choose bounded workflow, execute steps, enforce budgets, persist run status |
| Search and retrieval | Search eligible text, bibliographic records and structured data |
| Ingestion | Acquire permitted content, parse/OCR, preserve originals, prepare indexes |
| Evidence and citations | Resolve original locations, bind claims to passages and check support |
| Domain tools | Remedy aliases, source-aware comparison, rubric mapping and scoring |
| Provider adapters | Generation, embeddings, extraction, optional reranking and vision capabilities |
| Accounts/projects/usage | Ownership, source eligibility, saved work, limits and accounting |

PostgreSQL holds durable records and job state as appropriate to the existing implementation. Original files use the existing file/object-storage abstraction; production storage selection remains an operational decision. Public, shared and private material must keep their access boundaries through retrieval, caches, exports and saved results.

### 31.4 Three search types with different contracts

| Search type | Searches | Example | Important boundary |
|---|---|---|---|
| Knowledge search | Prepared passages from books, papers, cases and guidelines | Explain a description in a selected work | Only eligible prepared content supports full-text answers |
| Literature discovery | External bibliographic services | Find studies on a topic | A discovered record may contain metadata or an abstract only |
| Structured domain search | Remedy entities, rubric hierarchy, memberships and grades | List remedies under an exact rubric | Prose similarity cannot establish authoritative grade/membership |

The UI can combine these searches, but the backend must retain their distinct outputs and evidence depth. A discovered paper becomes a full-text knowledge source only after permitted acquisition and successful preparation. A bibliographic reference can be saved before full text is available. Failed access is an item-level state, not a reason to invent a paper's content or stop unrelated collection.

### 31.5 Hybrid retrieval and answer pipeline

Recommended retrieval combines lexical search for exact names/phrases with semantic search for paraphrases. Structured filters enforce owner, publication/index readiness, selected works/editions, evidence category and access scope. Merge and deduplicate candidates, then rerank where measured improvement justifies its latency/cost.

```text
Question + task + selected context
  → resolve intent, entities and explicit constraints
  → lexical + semantic retrieval within eligible source scope
  → merge and deduplicate candidates
  → rerank and assemble an evidence set
  → expand passages with needed surrounding context
  → generate an evidence-linked answer or structured extraction
  → validate references and check claim support
  → return answer, evidence, actual research trail and limitations
```

Explicit author/edition constraints apply to every retrieval and generation step. Query expansion may add aliases but must not silently widen selected source scope. Store actual query transformations for inspection. Access controls must constrain candidate retrieval as well as final rendering.

Comparison workflows retrieve evidence for each remedy/source side independently, then check coverage before synthesis. Do not let a strong result set for one remedy hide missing coverage for another. New conversational claims require new retrieval/support checks; previous model answers are context, not original evidence.

Exact ranking algorithms, candidate counts, embedding dimensions, reranker choice and latency budgets remain open until tested on our material. Compare lexical-only, semantic-only and combined approaches using the same reviewed tasks. Assess relevance, coverage, source constraint fidelity, negation and original-passage support—not fluency alone.

### 31.6 Structure-aware preparation

| Source type | Preferred unit | Context to preserve |
|---|---|---|
| Materia medica | Remedy section and coherent symptom/modality group | Author, edition, remedy, body location, negation and modality direction |
| Repertory | Rubric path with remedy memberships and grades | Edition, hierarchy, typography/grade interpretation and source location |
| Research paper | Section-aware passages with separately represented tables | Study identity, section, table headers, populations and qualifications |
| Published case | Presentation, intervention, timeline and outcomes | Reported facts, concurrent care, missingness and follow-up |
| Clinical reference | Recommendation with its qualifying context | Issuer/version, population, jurisdiction and applicability |

Uniform character-count chunking alone is insufficient. Small retrievable units can refer to parent sections for context expansion. Every unit retains stable source/revision identity and original page, section or offset. Source changes and index changes must not silently break historical citations.

“Better with movement” and “worse with movement” can be semantically close while having opposite meaning. Preserve polarity and negation in preparation and evaluation; embeddings alone must not decide equivalence. Numeric table extraction likewise needs row/column context, not an isolated number.

### 31.7 Provider portability and model roles

Define replaceable capabilities for text generation, embeddings, structured extraction, optional reranking, and vision/OCR assistance where necessary. One vendor may serve several roles, but interfaces should not assume identical endpoint formats or model features across providers.

| Environment | Intended behavior |
|---|---|
| Development | LM Studio, hosted services or a mixture, selected through configuration |
| Production | Hosted services with bounded retries/timeouts, budgets and observability |
| Evaluation | Pinned provider/model/configuration and corpus/index versions for comparison |

Benchmark lower-cost models for routing and straightforward extraction; use stronger synthesis only where it improves measured results. Exact providers/models remain open. Record actual lineage, token/cost accounting where available and redacted errors. Provider-specific behavior stays in adapters; unavailable capabilities should fail clearly rather than silently change the workflow.

**Embedding compatibility is a separate concern:** a new embedding model/version or vector dimension generally requires a compatible new index and re-embedding. Record embedding model, preprocessing/chunking version and index identity. Build and validate replacements before switching; preserve rollback and old answer lineage. Changing the generation provider must not implicitly invalidate or silently mix embedding indexes.

### 31.8 Citation validation and deterministic domain work

Give the model only evidence IDs provided by the application. Resolve bibliography fields from stored metadata. Reject unknown IDs and avoid invented authors, page numbers, dates or identifiers.

Use two checks:

1. **Deterministic reference validation:** evidence exists, belongs to the permitted run/source set, resolves to the stored revision/location and is eligible for the user.
2. **Claim-support assessment:** the cited passage supports the actual statement, including scope, qualifiers, negation and numbers. A real reference can still be misused. Unsupported statements should be revised, qualified, removed or reported as insufficient evidence.

Apply the same mechanism to table cells. Preserve “not reported,” ambiguity and disagreement. A support check is itself fallible; evaluate it and do not turn its output into a guarantee of scientific validity.

For repertory analysis, AI suggests language interpretations and rubric candidates. Users confirm ambiguous mappings. Versioned code calculates scores from confirmed rubric memberships, source grades and chosen weights. Return per-rubric contributions. The model may explain this result but must not replace the calculation with an unexplained ranking or treatment-success probability.

### 31.9 Bounded workflows and first integration slice

Start with explicit workflows: answer from sources; discover papers and summarize available evidence; compare remedies; extract an evidence table; score confirmed repertory inputs. Each has allowed tools, maximum work/budget, stop conditions, recoverable failure states and a defined output schema. Broad autonomous agents are not a prerequisite.

The first integration slice remains question → hybrid retrieval → cited answer → evidence inspection → saved run. It establishes reusable infrastructure **while data collection proceeds across all seven modules**. It does not require choosing classical literature at the expense of paper research. Add more adaptive search loops only when they improve evaluated research usefulness within cost and time budgets.

## 32. Day-one data coverage across all seven modules

### 32.1 Latest user direction and release interpretation

The user clarified: **data should be provided for all seven sections from day one, if feasible**. Record this as the collection objective. Do not restrict the initial data programme to either classical books/remedy comparison or paper discovery/literature research.

Three separate milestones must be tracked:

- **Data acquired or discoverable:** an exact source is registered and accessible through an appropriate route.
- **Data usable by the module:** content is prepared or structured to the required quality with provenance and applicable access.
- **Feature ready:** the UI, retrieval/domain logic, persistence and acceptance examples work end to end.

Day-one scope means useful seed coverage in every category, not a promise of exhaustive content or ingestion within one calendar day. Feasibility depends on the selected editions, access routes and extraction quality. Start acquisition workstreams together; a difficult source must not block unrelated categories. These are workstreams, not a requirement to use multiple agents or add infrastructure.

**Release recommendation:** aim for a narrow useful capability in each section as its minimum feature target, then deepen incrementally. The user has requested all-seven data coverage; they have not separately fixed a requirement that public launch wait for complete functionality in all seven. If that becomes a launch condition, update the milestone explicitly. The earlier research-beta option remains available and must show honest capability status.

### 32.2 Day-one collection and usability matrix

| Module | Initial data foundation | What makes it usable | First narrow capability / readiness evidence |
|---|---|---|---|
| Repertory analysis | One usable edition with rubric hierarchy, remedy membership and grades | Structured import, verified sample mappings/grades and exact source references | Rubric lookup first; deterministic scoring only after calculation fixtures pass |
| Case research | Curated public historical narratives and modern case reports | Extracted presentation, interventions, concurrent care, outcomes and follow-up with missingness | Search and compare published cases; historical/modern labels preserved |
| Paper search | A small set of bibliographic connectors plus selected accessible full texts | Search, deduplication, saved identifiers and explicit metadata/abstract/full-text states | Find, save and inspect records; analyze full text only when prepared |
| Materia medica comparison | Complementary works covering an overlapping remedy set | Normalized remedy aliases, author/edition identity and coherent passage support | Source-specific comparison with evidence for both sides |
| Clinical decision support | Selected current guideline, safety and referral references | Issuer/version/jurisdiction, evidence category and retrievable applicable passages | Cited reference lookup initially; patient-specific recommendations require separate validation |
| AI chatbot | Shared eligible material across all above categories | Task routing, source constraints, grounded conversation and saved context | Questions/follow-ups within measured coverage, with explicit gaps |
| Citation/research assistant | Bibliographic metadata and original locations for every source | Claim/cell-to-passage links, metadata formatting and research-run lineage | Inspect source, save result and export supported references |

Use section 17's existing 56-resource catalogue as the acquisition candidate pool. The catalogue is not proof that a module is covered. Each row above needs actual selected sources and usable records. Chatbot and citation assistance do not need independent duplicate corpora; they consume shared evidence services.

### 32.3 Three connected data layers

1. **Original sources:** PDF/scans/XML/HTML or supported structured exports. Record origin, exact edition/version, retrieval date, identifiers, access/reuse basis and original-asset identity.
2. **Searchable evidence:** extracted passages, original locations, evidence types, quality/readiness and lexical/vector indexes. These support grounded answers and source inspection.
3. **Structured domain records:** remedy entities/aliases, rubric paths/memberships/grades, study characteristics, case timelines and guideline recommendations. Link each source-derived field back to the appropriate evidence revision.

A single source may support multiple modules; use module tags and relationships instead of copying source files. A study containing a case report may contribute to paper discovery, case research, chatbot answers and citations. Materia medica can support comparisons and explanatory context for repertory results, but cannot invent repertory grades.

Minimum coverage record per module: selected source IDs/editions, intended question/task set, evidence/access category, acquired amount, usable amount, structure/extraction status, supporting citations, evaluation examples, blocker/alternative and owner. Count raw files, discoverable records, ready passages and validated structured entries separately.

### 32.4 Collection execution and exceptions

- Begin with one structured repertory, a small complementary materia medica collection, sufficiently complete published cases, a few literature connectors/selected full texts, and a focused clinical-reference collection.
- Retain section 16.2 quantity targets as proposals; useful tested coverage across categories takes precedence over reaching a large count in one category.
- Attach metadata/provenance during intake so every category can support chatbot and citation workflows as it becomes ready.
- Prefer usable API/XML/HTML/structured exports over OCR when available. Repertory data needs grade/hierarchy fidelity; a bag of searchable PDF passages does not satisfy that requirement.
- Keep discovery-only records useful when full text is unavailable. For unclear access/reuse or poor extraction, record an item-level exception and select an alternative where possible.
- Start case collection with published material. Private patient records are not required for day-one data coverage and should not enter a shared research corpus.
- Clinical reference freshness needs an update route, version and review date. Reference retrieval is an initial capability; it does not establish readiness for diagnosis, treatment selection or prescribing.
- Reflect actual readiness in the UI and landing-page capability registry. All seven can be explained in the product overview; unavailable actions must remain clearly planned rather than appear functional.

### 32.5 Technical and data task candidates

These extend existing DATA, MODEL, WORK and domain tasks; verify current code and combine overlapping work rather than duplicating it.

| Task ID | Deliverable | Dependency / acceptance |
|---|---|---|
| TECH-01 | Versioned answer/evidence/table API schema | Existing citations + WORK-01–03; frontend renders stable evidence relationships without parsing arbitrary Markdown |
| TECH-02 | Durable research-run streaming/reconnect | Existing jobs + WORK-04; reconnect/reload does not create a duplicate paid run; incomplete versus checked output distinguishable |
| TECH-03 | Hybrid retrieval evaluation and implementation | Prepared seed material; compare lexical/semantic/combined results on exact-name, paraphrase, scope and negation fixtures |
| TECH-04 | Structure-aware passage preparation | Existing ingestion + DATA-05; section context, table meaning and original anchors survive extraction/chunking |
| TECH-05 | Balanced comparison retrieval | TECH-03 + MM-DEV-01; each side has explicit coverage and missing evidence is not filled by generation |
| TECH-06 | Provider capability contracts and index compatibility | MODEL-DEV-01–02; hosted/local matrix works and incompatible embedding/index combinations are rejected |
| TECH-07 | Reference and support-validation pipeline | Existing trust/citation behavior; unknown IDs rejected, unsupported claims/cells handled, quality measured |
| TECH-08 | Bounded workflow orchestration | Existing jobs; allowed steps/budgets, stop/cancel/retry behavior and actual trail recorded |
| COVER-01 | Seven-module collection manifest | DATA-01; all seven mapped to exact candidate sources, intended uses, readiness and alternatives |
| COVER-02 | Acquire/prepare all-category seed set | COVER-01 + DATA-02–06; evidence for each module recorded without requiring all 56 integrations |
| COVER-03 | Validate module-usable data | COVER-02 + relevant domain tasks; source/grade/cell samples and original locations reviewed; gaps visible |
| COVER-04 | Publish coverage/readiness view | COVER-03 + SITE-01; acquired data, usable data and available features are separate; no planned count presented as completed |
| COVER-05 | Choose public-launch feature milestone | Actual coverage and pilot findings; explicitly record research beta versus all-seven minimal functionality, with corresponding scope and dependencies |

Completion of this documentation update does not acquire data, implement the technical design or settle the remaining launch milestone. Future work must report measured coverage and working behavior, while preserving the confirmed goal of collecting across all seven from the outset.

## 33. Confirmed stack, authentication and development conventions

Decision date: 2026-10-08. This section records the user's decisions from the technology discussion. These are requirements for future implementation, not claims of completed code. The priority is a simple first launch that takes less development time. Retain the seven-module product direction and data-collection scope; do not interpret this as a requirement to complete every future module before launch.

### 33.1 Stack decisions and scope

| Area | Decision | Implementation boundary |
|---|---|---|
| Frontend | Keep Vue 3, TypeScript and Vite | Reuse the current app and working interaction patterns |
| UI components | Use shadcn-vue with Tailwind CSS | Adopt incrementally for forms, buttons, dialogs, menus, tabs and panels; no wholesale rewrite prerequisite |
| Navigation | Use Vue Router as screens are separated | Clear URLs for Login, Ask, Sources, Projects and Settings |
| Rich tables | TanStack Table when sorting/filtering/selection justify it | Ordinary tables do not need an extra abstraction |
| Shared frontend state | Pinia only when needed across screens | Keep API calls in a clear service layer; avoid duplicating server state |
| Backend | Keep Go API and Go workers | Authentication belongs inside the existing Go backend |
| Database/search | Keep PostgreSQL, full-text search and pgvector | No additional vector database for launch |
| Authentication | Use Authboss, a Go library | Integrate its account modules; do not create a Better Auth/Node.js service or build a complete authentication framework ourselves |
| Social login | Implement Google login behind configuration | Whether to enable it at first launch remains undecided |
| Email templates | Use Handlebars templates | Render from Go; no Node.js runtime/service requirement |
| AI and processing | Keep provider flexibility and existing durable workers | Reuse current capabilities before adding libraries or infrastructure |
| File storage | Keep existing storage integration initially | Production object-storage provider remains an operational decision |

The user accepted the remaining discussed stack for now. Conditional libraries above should be introduced with a feature that needs them. Earlier proposals for River, MinIO or other replacements are not newly approved requirements. The current frontend package inspection showed Vue with custom CSS; the approved UI dependencies still need implementation.

### 33.2 Authentication and account flow

Use [Authboss](https://github.com/aarondl/authboss) inside the Go API for registration, password login, email confirmation, password recovery and the Google login integration. Authboss supplies reusable account-flow modules; we still implement storage integration, Vue screens, email delivery and application access rules. Verify the selected version and its integration requirements during the first feature; this decision does not imply a completed compatibility check.

```text
Vue + shadcn-vue
  → existing Go API: accounts, sessions and application features
  → PostgreSQL: users, session/account records and application data

Go account flow → Handlebars email rendering → ZeptoMail
Optional Google login → Go callback → application session
```

Initial account requirements:

- **Register and verify email:** public registration; anyone can create an account without an invitation. Name, email and password → account creation → verification email → confirmation → access to protected app features. Allow the account/verification actions needed to complete registration before verification.
- **Sign in and sign out:** email/password → browser session → intended app page; logout ends that session. Use secure HTTP-only cookies in production and server-side session records with expiry and revocation. Keep the browser and API under the same website origin where practical; JWT infrastructure is not a launch requirement.
- **Recover password:** request reset → generic response → temporary, single-use emailed link → new password → invalidate existing sessions. Preserve the distinction between changing a password while signed in and recovering a forgotten one.
- **Google login:** implement the provider flow and account-linking rules, but gate availability with a server-side setting such as `GOOGLE_LOGIN_ENABLED`. Default to disabled until explicitly enabled. When disabled, hide the button and reject login-start/callback routes; missing Google credentials must not prevent email/password login or application startup. Require credentials and valid callback configuration when enabled. Do not merge accounts solely because an unverified email matches.
- **Account settings:** support name/password changes and session revocation as concrete feature slices; advanced identity features are not prerequisites for first launch.
- **Access rules:** the Go backend enforces ownership and user/reviewer/admin permissions on APIs, saved work, sources and assets. Student, teacher, practitioner and researcher are profile/audience choices, not self-granted reviewer or administrator permissions.
- **Existing POC ownership:** the user confirms no real users have been created yet. Inspect any seeded principals and saved POC work before account setup; if retained records exist, explicitly map their ownership to the intended accounts. If none exist, no user-migration feature is needed. Do not silently reassign records or leave production access dependent on shared development tokens.

Use library-supported password handling and provider validation. Apply bounded login/reset attempts, cookie/CSRF protections and token expiry as part of the relevant feature, rather than creating a separate large security project. ZeptoMail is the only selected email delivery provider; account/domain configuration and credentials remain to be supplied. No additional application service is required for email.

### 33.3 Handlebars email templates

The user explicitly selected **Handlebars** for transactional email templates. Use clear `.hbs` file names such as `verify_email.hbs` and `reset_password.hbs`. Keep templates together in an obvious location such as `internal/email/templates/` and pass small named values such as `user_name`, `verification_url`, `reset_url` and `expires_in_minutes`.

- Render templates in the Go backend using a maintained Handlebars-compatible library. The exact Go library remains to be selected after checking required syntax, escaping and maintenance; do not claim full JavaScript Handlebars compatibility without verification.
- Keep initial templates simple: shared branding, short explanation, one primary link/action, expiry information and a readable fallback URL. Supply a plain-text alternative alongside HTML.
- Escape user-controlled values, construct action links from the configured application URL, and keep passwords and unnecessary personal data out of emails and logs.
- Integrate rendering and delivery with Authboss's mailer interface. Reuse the existing worker/job approach where suitable for email retries; do not add a new queue service just for email.
- A failed send or resend should produce an actionable app state. Verification and reset behaviour must not depend on a decorative or complex template system.

### 33.4 Clear table, API and file names

Names must be concrete, readable and development-friendly. Use familiar product words rather than vague abstractions or unnecessarily technical labels. Use the same word for the same concept across UI, API and database; preserve meaningful domain distinctions such as source revision and citation.

**Confirmed clarification — 2026-10-09:** create only database tables needed by an implemented feature. Inspect and reuse or extend existing tables where their purpose fits before adding new ones. Do not create a table for every screen, API endpoint, processing step or hypothetical future feature. Each proposed new table must have a short, concrete justification in its feature task describing the records it stores and why existing storage is insufficient; no separate design document is required.

Keep necessary relationships, ownership, constraints, durable job state and history intact. Fewer tables is a simplicity goal, not a reason to combine unrelated data into a generic catch-all table. Do not duplicate stored data or introduce analytics summary tables when queries over existing records are sufficient; add such storage only for a demonstrated need. The table names below and the record categories in section 34.4 are illustrative, not a mandatory list of tables to create.

Columns, API paths and file names must be equally straightforward: prefer `status`, `finished_at`, `error_message` and `cost_amount` over opaque abbreviations or elaborate technical labels. Make units and meaning explicit where needed, such as `duration_ms` and `cost_currency`. Use recognizable feature names such as `questions`, `answers`, `login` and `password_reset`; avoid unnecessary terminology such as “orchestrator,” “entity” or “facade” in names that can directly describe the feature. Follow normal Go, Vue and SQL conventions without adding jargon.

| Item | Naming convention / examples |
|---|---|
| Database tables | Plural snake_case: `users`, `user_sessions`, `user_login_accounts`, `email_verifications`, `password_resets`, `questions`, `answers`, `sources`, `source_pages`, `projects`, `project_members` |
| Database columns | Explicit snake_case: `user_id`, `source_id`, `created_at`, `email_verified_at`, `expires_at` |
| Authentication APIs | Clear actions under `/api/v1/auth`: `/register`, `/login`, `/logout`, `/verify-email`, `/forgot-password`, `/reset-password` |
| Product APIs | Resource-based paths: `POST /api/v1/questions`, `GET /api/v1/answers/{id}`, `/api/v1/projects/{id}/questions` |
| Vue files | `LoginPage.vue`, `RegisterPage.vue`, `ResetPasswordPage.vue`, `QuestionForm.vue`, `AnswerView.vue`, `SourcePanel.vue` |
| Go files | `login.go`, `registration.go`, `password_reset.go`, `sessions.go`, `questions.go`, `source_access.go` |
| Functions | Purpose-specific names in the language's normal style: `saveQuestion`, `getAnswer`, `checkSourceAccess` |

These examples guide design; they are not a finalized schema or an instruction to rename working tables indiscriminately. Reconcile existing records and Authboss integration before migrations. Avoid generic names such as `entity_manager`, `process_data` and `workflow_handler` when a concrete feature name is available. Do not add wrapper layers solely to rename standard library interfaces.

### 33.5 Tasks follow the app flow

Every development task should deliver one concrete, usable feature through the necessary frontend, API, backend, database and worker/email changes. Include all relevant layers within the task; do not split ordinary implementation into disconnected frontend-only and database-only tickets. A feature need not change every layer.

Task size is defined by a demonstrable user outcome: “build authentication” is too broad, while “add an email input” is too small. Keep prerequisites explicit and order tasks along the actual user journey. Recheck current implementation and combine overlapping backlog entries before creating work; older tables in this document may describe epics that still need splitting.

| Example feature task | Complete flow / acceptance outcome |
|---|---|
| Register and verify email | Vue form → Go/Authboss → account storage → Handlebars email → confirmation result → protected-access rule |
| Sign in and sign out | Login form → session creation → authorized Go request → logout → subsequent request denied |
| Recover a forgotten password | Request form → temporary email link → reset form → password update → previous sessions revoked |
| Continue with Google when enabled | Configuration → visible login action → provider callback → verified identity/linking → session; disabled mode remains functional without credentials |
| Preserve retained POC work, if any | Inspect seeded/test records → explicitly map retained ownership → saved work remains accessible only to its authorized user; skip migration when no records exist |
| Save a question to a project | Project selector → API validation → ownership check → database write → visible saved question |

Each task needs a short statement of the user outcome, dependencies, affected layers, and a concise manual acceptance checklist. Keep technical substeps inside that feature task. Select a small first-launch set instead of adding optional providers, enterprise SSO, elaborate account administration or unrelated infrastructure.

### 33.6 Documentation and verification preferences

- Update this existing PRD when decisions or scope change. Do not create unnecessary planning documents, duplicate specifications or test files.
- The user will perform broader manual testing. Provide short, practical acceptance steps with each feature rather than generating a large automated suite by default.
- Run proportionate build/type checks and focused functionality or code-integrity checks. Reuse relevant existing checks when useful; avoid repeated broad suites without a reason.
- For authentication and access changes, concentrate on the affected behaviour: expired/reused verification or reset links, disabled Google login, logout/session revocation, and another user's records remaining inaccessible. A small targeted automated check is appropriate only when it materially verifies such behaviour; do not add tests merely to mirror implementation.
- This preference qualifies earlier test-heavy delivery examples without marking evidence integrity, ownership or other acceptance requirements satisfied. Report what was actually checked and what remains for manual verification.
- Documentation-only updates require content/structure checks, not application tests. This update creates no implementation, installs no dependency and activates no login provider.

Remaining implementation choices: exact Go Handlebars library, session/storage integration details, first-launch Google activation and final launch feature milestone. ZeptoMail is confirmed in section 34. Authboss, shadcn-vue, the Go-only application backend, clear naming, feature-sized tasks and focused verification are confirmed decisions.

## 34. Launch access, background research and admin analytics

Decision date: 2026-10-09. The user confirmed the requirements below. Implementation details described as recommendations remain subject to checking the current code. This section supersedes the invite-only pilot suggestion and earlier open email-provider choice; it does not mark features implemented.

### 34.1 Confirmed launch boundaries

| Area | Confirmed requirement |
|---|---|
| Registration | Anyone can create an account; retain email verification and ordinary-user defaults |
| Email | ZeptoMail only, with Handlebars templates rendered by Go |
| Existing users | No real users created yet; map any retained seeded/test ownership explicitly if such records exist |
| Shared library | Ordinary users consume prepared content; reviewers/admins manage shared content according to their permissions |
| Uploads | Admin source intake is in scope; ordinary-user private knowledge uploads are future scope and unavailable in this version |
| Usage | Per-user limits, bounded retries, saved-answer reuse and per-query research/cost accounting |
| Admin visibility | Query/answer history, RAG diagnostics and cost analytics with useful charts, filters and per-query detail |
| Background work | Accepted research continues after navigation or leaving the site; persist the result, notify in-app and email when ready |
| Deployment | One initial deployment with HTTPS, database/file backups and a checked restore procedure |
| UI consistency | Reusable page layouts, form errors, loading states, empty states, notifications and citation panels using shadcn-vue |

“Private uploads” means a future user can upload their own documents and ask questions over that separate, private knowledge collection. Uploading must not publish their material to the shared library. Future design must enforce ownership through files, retrieval, answers and citations, with no implicit cross-user sharing. Do not add the upload feature or its infrastructure as a first-launch dependency.

### 34.2 Durable research is a core launch requirement

The browser observes research; it does not keep research alive. Closing the page, navigating elsewhere, losing connectivity or logging out must not cancel an accepted job. On return, the user sees the same saved job and its latest state or completed answer. Completion remains subject to provider availability and bounded retries; exhausted failures must be visible rather than falsely marked ready.

```text
Submit question
  → validate access and reserve usage allowance
  → save question, selected sources and durable job together
  → return saved job identifier
  → Go worker searches eligible sources
  → builds evidence, generates answer and checks citations
  → saves answer, research details and usage/cost records
  → commits completed status and pending notification together
  → email worker sends answer-ready message through ZeptoMail

User returns → fetch saved job/result → display without generating again
```

Implementation requirements:

- Use existing PostgreSQL-backed answer jobs and Go workers as the starting point. Inspect gaps before choosing replacements; this requirement does not mandate Redis, microservices or a new queue library.
- Save accepted inputs durably before acknowledging submission. Use a submission identifier so duplicate clicks or retries of the same submission return the same job. An intentional new question or rerun creates a new run; do not deduplicate unrelated submissions merely because their text matches.
- Reserve/check per-user allowance atomically before accepting work, so simultaneous submissions cannot bypass limits. Settle recorded usage on completion/failure and release unused reservations. Reopening an answer incurs no new generation quota or model call.
- Keep work independent of browser request cancellation. Record actual stages such as queued, searching sources, generating answer, checking citations, retrying, completed and failed; map these labels to existing stored states where possible.
- Persist recoverable stage outputs, source/evidence identifiers, attempts and failure details. Workers claim jobs with time-limited ownership, renew it while active and recover abandoned work after a crash. Prevent a superseded worker from publishing stale output.
- Recheck applicable source/access eligibility as work proceeds and before exposing saved results. A saved job must not bypass later source restrictions.
- Retry transient failures with delays and a bounded attempt/time/cost budget. Permanent failures and exhausted retries surface with preserved inputs and an actionable state. An insufficient-evidence result is distinct from an infrastructure failure and must be labelled accurately.
- Save one authoritative result per run and create its completion notification in the same database transaction. Separate email retry from answer generation: a failed email must never rerun research or hide a completed answer.
- Use streaming/SSE only for updates, with state-fetch/polling fallback. A disconnected stream is not a failed job; partial streamed text is not the final checked answer.
- Minimize repeated paid calls after worker failure by persisting completed steps and provider request identifiers. External calls can succeed just before a crash/timeout; exactly-once provider execution or billing cannot be assumed. Record uncertain attempts and use provider idempotency/reconciliation where available.

The current code contains answer jobs, worker leases/heartbeats, retry states and saved-answer references. This is an implementation starting point, not proof that all requirements above or their interruption scenarios are already satisfied.

### 34.3 Answer-ready notifications and ZeptoMail

Use ZeptoMail for verification, password recovery and answer-ready emails. Render `.hbs` templates in Go and send the resulting email through the configured ZeptoMail API; do not assume ZeptoMail's hosted templates execute Handlebars. See the [official ZeptoMail API documentation](https://www.zoho.com/zeptomail/help/api-home.html).

- Add an `answer_ready.hbs` template with a short completion message and a link to the saved result. The link requires normal account access and preserves the destination across login; do not email the full query, private source excerpts or answer by default.
- Create an in-app completion notification and a durable email record for each completed run. Recommended initial behaviour: email every completed run, independent of unreliable browser-presence detection. A later notification preference can refine this without affecting job completion.
- Send only to the account's verified email. Track pending, accepted-by-provider and failed delivery states separately; API acceptance does not prove inbox delivery. Retain provider message identifiers and update delivery/bounce status when supported by configured provider events.
- Use a unique notification key per run/type, bounded email retries and attempt records. Suppress known duplicate sends; ambiguous network outcomes need reconciliation where possible rather than a promise of exactly-once email delivery.
- Show the answer immediately once saved, even if email is delayed or fails. Surface delivery failures to admins without changing the answer's completion state.
- Configure the verified sender/domain, correct ZeptoMail account region/endpoint and server-side credentials during deployment. Do not add a second email provider or another application service.

### 34.4 Query history, RAG diagnostics and cost accounting

Track every submitted research query and generated answer with its user, run identifier and timestamps. Accepted runs, rejected/limited requests, retries and deliberate reruns must be distinguishable. Raw user content belongs in access-controlled application records, not general infrastructure logs or marketing analytics.

| Record | Required detail |
|---|---|
| Question/run | User, original question, effective source selection, workflow, submitted/started/finished times, status, linked answer and rerun relationship |
| Retrieval (RAG) | Effective searches/filters, eligible source revisions, retrieved passage IDs and scores, selected evidence, reranking when used, timing and coverage gaps |
| Answer checks | Persisted answer, citations, supporting passages, claim-support outcomes and insufficient/conflicting evidence states |
| Model calls | Run/stage/attempt, provider/model, request ID when available, input/output and other reported token categories, latency and outcome |
| Cost | Reported charge when available; otherwise estimate from recorded usage and versioned prices, currency, pricing date and cost status |
| Notification | In-app status, email attempts, provider ID and available delivery/failure state |

Record observable processing steps and evidence decisions; “RAG analysis” does not mean storing hidden model chain-of-thought. Include query embeddings, generation, reranking, verification and retry calls where used. Preserve cost records for failed or cancelled work too. Keep shared corpus-ingestion costs separate from per-question costs unless an explicit allocation method is defined.

Missing provider usage or charges must show as unknown/pending, not zero. Distinguish provider-reported amounts from estimates; historical totals must use the recorded price basis rather than silently changing with current prices. Reconcile attempts using stable identifiers to avoid counting the same call twice. If displaying another currency, retain the original currency and conversion basis.

Admin dashboard requirements:

- Summary cards: query count, active users, queued/running/completed/failed runs, total recorded cost, average cost per run and completion time; show unknown-cost counts.
- Charts: queries and cost over time, cost by provider/model or research stage, completion/failure trend and per-user usage. Define denominators and time zones consistently; do not combine different currencies without an explicit conversion.
- Filters: date range, user, run status, provider/model and workflow; apply filters consistently across cards, charts and detail tables.
- Query table: user, question preview, time, status, duration, usage, cost and answer link. Open a query to inspect its answer, retrieved/selected evidence, citation checks, call/attempt breakdown and notification state.
- Restrict analytics and detailed user-query inspection to authorized admins; reviewers do not automatically gain access to all private query history. Audit detailed-content access and establish retention/deletion handling before public rollout.
- Charts can use one Vue-compatible chart library selected when implementing this feature. Do not add an external analytics platform as a prerequisite.

### 34.5 Deployment, reusable UI and focused acceptance

One initial deployment may contain the Vue app, Go API, Go workers and PostgreSQL; a single deployment does not mean processing inside a browser request. Configure HTTPS and backups for both database records and original files, stored separately from the running deployment. Verify a restore into an isolated environment and prevent restored pending jobs from sending real email or making paid provider calls during the exercise. Backup frequency, retention and acceptable recovery time remain operational choices.

Use shared shadcn-vue patterns for page layout, validation, loading/empty/error states, notifications and citation panels. Research pages must include persistent status, saved-result navigation and a clear indication that work continues after leaving. Admin analytics should reuse existing table/filter/page patterns.

Deliver this as concrete features such as: submit and reopen background research; recover interrupted research; receive answer-ready email; inspect a query's evidence and cost; filter admin usage/cost charts. Each task includes its necessary UI, API, storage and worker changes. Avoid one oversized “robust pipeline and analytics” ticket.

Focused acceptance should exercise: leave/close the browser after submission and return to the saved answer; restart a worker mid-run; repeat the same submission; fail a provider call; delay/fail email while the answer remains available; verify user isolation; and reconcile recorded calls/costs with an example run. Reuse existing checks and provide concise manual steps rather than generating broad test suites. This documentation update performs none of these runtime checks and changes no application code.
