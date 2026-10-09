# Local source research POC

Product planning: [Product requirements and development direction](Homeopathy_AI_Product_Requirements_and_Roadmap.md) records the general-question and future case-analysis workflows, current POC boundaries, and guidance for task creation and architecture design. Use it alongside the [POC PRD](Homeopathy_AI_Source_Ingestion_POC_PRD.md) and [master handoff](Homeopathy_AI_Codex_Master_Handoff.md).

## Run

1. Copy `.env.example` to `.env`, set a random `API_TOKEN` of at least 24 characters, and set `OPENROUTER_API_KEY`. Keep the file private.
2. The local example uses OpenRouter for both chat and embeddings. No local model server is needed.
3. Run `docker compose --profile dev up --build` from this directory and open <http://127.0.0.1:8088>. Sign in with the administrator or reviewer token from `.env`.

For local database inspection, open Adminer at <http://127.0.0.1:8089>. Select **PostgreSQL** and use server `db`, username `homeopath`, database `homeopath`, and the `POSTGRES_PASSWORD` from `.env` (or `localdev` if unset). Adminer starts only with the `dev` Compose profile.

For an existing `.env`, configure both hosted endpoints:

```dotenv
CHAT_PROVIDER=openrouter
EMBEDDING_PROVIDER=openrouter
OPENROUTER_API_KEY=replace-with-your-openrouter-api-key
CHAT_MODEL=nvidia/nemotron-3-ultra-550b-a55b:free
EMBEDDING_MODEL=baai/bge-m3
EMBEDDING_INPUT_STYLE=plain
EMBEDDING_REVISION=openrouter-baai-bge-m3-unpinned
EMBEDDING_DIMENSIONS=1024
CHAT_MAX_TOKENS=4096
CHAT_REASONING_EFFORT=none
MODEL_REQUEST_TIMEOUT_SECONDS=300
```

After adding your real key, run `docker compose --profile dev up -d --build api worker` to apply the settings. If the database already contains sources indexed with Nomic, reindex each published source using **Sources → Reindex** (or `POST /api/v1/sources/{id}/reindex` as an administrator) and wait for READY before asking questions. The worker batches hosted embeddings during reindexing.

The free Nemotron endpoint can be temporarily overloaded or rate limited. The app retries transient chat responses and shows a provider-busy state in Activity when a question must be retried. `CHAT_REASONING_EFFORT=none` leaves the output budget for the evidence table; another model may need a different effort setting or no reasoning setting. A short answer may still take a few minutes because citation checks call the model after retrieval.

`CHAT_PROVIDER` and `EMBEDDING_PROVIDER` are independent. Change `CHAT_MODEL` to any OpenRouter chat model ID to compare LLMs without changing the search index; `nvidia/nemotron-3-ultra-550b-a55b:free` is only an example. Both the API and worker need the same settings, and the OpenRouter key stays in their server-side environment. If you change the embedding model, provider, or input style, set its matching `EMBEDDING_DIMENSIONS` and a new `EMBEDDING_REVISION`, then reindex prepared sources before asking questions. An existing Nomic index cannot be searched with another embedding model.

The [OpenRouter chat](https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request) and [embeddings](https://openrouter.ai/docs/api/api-reference/embeddings/create-embeddings) endpoints both use the same API key. The example model IDs are [Nemotron 3 Ultra (free)](https://openrouter.ai/nvidia/nemotron-3-ultra-550b-a55b:free) and [bge-m3](https://openrouter.ai/baai/bge-m3).

`CHAT_PROVIDER` and `EMBEDDING_PROVIDER` accept `lmstudio`, `openrouter`, `deepinfra`, or `openai`. For another OpenAI-compatible service, use `openai` with the corresponding `CHAT_BASE_URL`/`CHAT_API_KEY` or `EMBEDDING_BASE_URL`/`EMBEDDING_API_KEY`. `AI_PROVIDER` and `AI_BASE_URL` remain fallbacks for existing configurations. `CHAT_MAX_TOKENS` controls the output limit, `CHAT_REASONING_EFFORT` is optional for reasoning models, `MODEL_REQUEST_TIMEOUT_SECONDS` sets the provider request timeout (120 seconds by default), and `CHAT_PROMPT_SUFFIX` is available for a model-specific suffix if needed; the application does not append one automatically. `EMBEDDING_INPUT_STYLE` can be `plain` or `prefixed` for embedding models with different query/document input conventions. If no revision is provided, the app records a provider/model-specific `unpinned` marker.

## Production model provider

Copy `production.env.example` to `production.env` on the server, replace its credentials, and run `docker compose --env-file production.env up --build -d`. This example uses OpenRouter over HTTPS for both chat and embeddings, with no LM Studio process on the server. Set the exact `CHAT_MODEL` ID you want to evaluate. The key is passed only to the API and worker containers, never to the browser.

Set `OPENROUTER_API_KEY` and keep `CHAT_PROVIDER` and `EMBEDDING_PROVIDER` as `openrouter`; `AI_BASE_URL` is not needed. `POSTGRES_PORT`, `API_PORT`, and `FRONTEND_PORT` set distinct host ports. PostgreSQL and the API bind to localhost; `FRONTEND_BIND_ADDR` also defaults to localhost for use behind an HTTPS reverse proxy. Use `docker compose --env-file production.env config --quiet` to check the file before starting services.

The production embedding model produces 1,024-dimensional vectors and is incompatible with the development Nomic index. For an existing database, call `POST /api/v1/sources/{id}/reindex` as an administrator for every published source after switching providers, and wait until all sources are READY before asking questions. New sources are indexed with the configured model when published. Do not mix sources prepared under the two embedding models in one answer. Run the held-out evaluation with human citation review before treating the hosted setup as release-ready.

The Compose UI port is bound to localhost. A public deployment also needs an HTTPS reverse proxy, backups for the PostgreSQL volume and PDF assets, and server-managed credentials. Use a URL-safe random `POSTGRES_PASSWORD` so it can be interpolated into the database URL. The sample Compose defaults remain for local development only.

Compose starts PostgreSQL, the Go API, the ingestion/embedding worker, and the Vue UI. Docker sets the backend project root and internal database URL; they do not need entries in `.env`. PDF assets are saved in `data/runtime/assets`; the database uses the `pgdata` volume. The starter corpus is mounted read-only. For local testing, the browser uses a long-lived HttpOnly session cookie that renews on authenticated requests and is cleared by Sign out. The proxy does not grant administrator access automatically. Set `ADMIN_NAME` and `REVIEWER_NAME` to identify the two default users. For more people, set `AUTH_PRINCIPALS_JSON` as shown in `.env.example`, with a stable UUID and distinct token for each; when that list is set, its tokens replace the two default sign-in tokens. Reviewer answer jobs and Activity are private to that reviewer; administrators can inspect all jobs.

In **Sources**, search Internet Archive by book title or author, inspect a catalogue record and its listed PDFs, then choose **Download for review**. The app saves the PDF and catalogue link and queues normal page and rights checks. A repository record may have no eligible PDF or no rights statement; the admin must check rights before publishing. You can also enter a DOI, upload a PDF, or paste a direct public HTTPS PDF link. Title and author are optional for PDF intake: the app reads the opening pages and uses OCR when needed to fill available details, including edition, publication, repository and catalogue URL when it finds them. Review the detected details before publication; unreadable fields stay marked for correction. User-entered values take priority. Uploads and linked PDFs have a 250 MB limit. Linked PDFs are downloaded to local storage and enter the same page-reading queue as uploads; a catalogue page or DOI landing page is not a direct PDF link. DOI lookup fills a reference from Crossref (or DataCite when Crossref has no record) without storing a PDF. A direct DOI PDF import is offered when Crossref reports an HTTPS PDF link and a supported Creative Commons licence for the matching version. The connector also recognizes the verified PLOS ONE printable PDF route for eligible `10.1371/journal.pone.*` DOIs. Other DOIs without a usable route remain reference-only until a permitted PDF is uploaded or linked. Reference-only records are never searched or cited as page evidence. **Delete reference** removes a saved DOI reference from the list; when a PDF source is already linked, its DOI provenance and source remain stored.

For imported PDFs, the worker extracts embedded text or uses Tesseract for scanned pages, then creates reviewable passages and automatic page checks. Each import records an edition snapshot, acquisition record, checksum-bound PDF asset, and processing revision with Ghostscript/Tesseract versions and a configuration hash. In **Review**, an administrator corrects source details, and an administrator or reviewer can check pages, printed labels, and rights. Rights decisions are append-only and attributed to the signed-in person. Only an administrator publishes; the publication records the reviewer and publisher identities, decision and metadata/page-label snapshots. Passage preparation starts afterward. An admin can reprocess a published source from its saved PDF. The candidate receives fresh page QA and rights review while the current publication remains searchable. The old citation IDs still open their original scans after the candidate index becomes READY and replaces it.

In **Ask**, choose all prepared sources or select specific prepared sources. The selected source IDs are checked by the API, applied to both vector and text search, and saved with the answer. Questions about a prepared book or paper can return a short overview grounded in its opening pages. A direct question about an acronym returns its expansion when a prepared passage spells it out. “Who is/was” questions about a prepared source author return a short identity answer when an indexed page names the person. These answers are saved with a citation to the original page. The app says when it lacks verified biographical information about another person; it does not improvise from unrelated passages. Questions about a work's specific claims continue through passage search. A DOI reference without processed full text cannot support these answers.

Questions asking how two prepared authors are related compare their author pages and check whether either indexed work names the other. The answer distinguishes a documented connection through writings from an unverified personal or family relationship, with citations to each supporting scan.

Ask saves a durable question job and shows its real status. Open **Activity** to follow answer and source preparation work, reopen a finished answer, or retry a failed question. General answers include persisted per-claim support decisions and exact supporting excerpts in the optional research trail. A partial answer means some draft claims were excluded; only checked claims are displayed. An admin can disable a published source with a reason and restore it after review. Disabled sources are excluded from new answers and citation access.

Administrators can open **Evaluation report** beside any question in Activity and download its JSON. The report includes the exact submitted question, saved system answer, selected sources, search queries, ranked candidate passages and retrieval scores, citations, claim checks, model and prompt revisions, and structured job stage logs with durations and failure codes. Its counts and citation coverage describe the retrieval and checking process; they are diagnostics, not an accuracy score. Older questions retain their saved answers and searches, while stage logs begin with questions submitted after this feature was added. The report API is restricted to administrators.

PDF assets are stored locally only after the user chooses to analyze a source. Internet Archive book discovery is available. The [release evaluation](evaluation/README.md) has a draft 30-question set and runner, but its human-reviewed gold passages and scores are pending. Reprocessing currently represents a candidate as a linked source row; the stricter single-source/multiple-revision identity in the PRD remains open. Broader literature search belongs to the later research phase.

## Checks

Run `go test ./...` in `apps/backend` and `npm run build` in `apps/frontend`. Disposable PostgreSQL integration checks for ingestion, provenance, rights history, replacement, source disable/restore, and exhausted job recovery are available. Set `PROVENANCE_TEST_DATABASE_URL` to a migrated disposable database for `TestReprocessPreservesPublishedCitationIntegration`, `TestRightsDecisionHistoryIntegration`, `TestActivateReplacementIntegration`, `TestDisableAndRestoreCitationIntegration`, and `TestExhaustedSourceJobBecomesFailedIntegration`. An isolated ingestion check is available with `INTAKE_TEST_DATABASE_URL` set to a disposable database URL: `go test ./internal/ingest -run TestManualPDFIngestionIntegration -v`.

To check the real DOI route in a disposable database, set both `INTAKE_TEST_DATABASE_URL` and `LIVE_DOI_TEST=1`, then run `go test ./internal/ingest -run TestLiveDOIImportIntegration -v`. This downloads the PLOS ONE PDF for DOI `10.1371/journal.pone.0134657` and verifies that the worker creates reviewable pages. Crossref retraction metadata is checked during DOI lookup; retracted records remain visible but cannot be imported or published for answers.
