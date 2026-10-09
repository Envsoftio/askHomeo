# PROD-01 — Production deployment, recovery and operational controls

**Status:** Deferred after an initial application slice. The user reports that deployment already works, wants no deployment changes, and has deferred backups/restore and operational alerts while source ingestion and source-grounded answering take priority. Browser sessions and a database readiness endpoint are implemented locally; B2 connectivity and production acceptance remain unverified.

**User outcome:** Administrators can operate the hosted research app over HTTPS with durable private originals, recoverable data, bounded usage, and actionable incident visibility.

**PRD basis:** Sections 18.2 (production operations), 33.4 (storage justification) and 41 (production delivery priority), plus LAUNCH-02. The new B2 storage implementation is a dependency; CORE-02 local evaluation is not a prerequisite implementation task.

## Implementation record — first application slice

- Added `browser_sessions` with hashed opaque tokens, 12-hour absolute expiry and server-side revocation on sign-out. Browser cookies no longer contain administrator/reviewer bearer tokens. Existing bearer API access remains available. Production cookie and same-origin enforcement are implemented behind `APP_ENV=production` plus a valid HTTPS `PUBLIC_ORIGIN`; those settings have not been applied to the user's working deployment.
- Added `/api/v1/ready` for a short database check while `/api/v1/health` stays process-only. The existing deployment health check and scripts were not changed.
- Added `go run ./cmd/check-b2` as a read-only bucket access diagnostic. All required B2 settings are present in local `.env`; the local check could not reach B2 from this network-restricted sandbox, so key permissions and bucket reachability are still unverified.
- Focused HTTP/session, server-command and storage-configuration checks pass. The broader Go suite reached the existing B2 round-trip test, which cannot bind its local test listener in this sandbox. No production migration, backup, restore, quota or alert work is claimed by this slice.

## Current code baseline

- `docker-compose.prod.yml`, `.github/workflows/deploy-production.yml` and `scripts/deploy-prod.sh` are the user's working deployment path. This implementation does not change those files. Production recovery, backup and usage controls still need separate application and operations work.
- Private B2 storage, checksum-addressed objects and resumable PDF migration code exist, but live bucket configuration, migration coverage and isolated restore are unverified. The legacy assets volume remains mounted read-only for migration.
- `/api/v1/health` is used as a container check. There is no separate dependency readiness contract, backup schedule or operator alert path. The worker has no active health signal in Compose.
- Browser sessions now use revocable database records in local code. Production cookie enforcement still needs the public-origin setting in the running app; full customer accounts remain a later task.
- Answer and ingestion jobs already persist status, retries and leases. Model-call records capture reported usage, but there is no durable admission budget or per-principal quota before paid jobs enter the queue.

## Implementation order

1. **Production access.** Make pilot sessions expiring and server-revocable without exposing bearer tokens as cookie values. Keep administrator/reviewer permissions and existing saved-work ownership intact. Validate the configured public HTTPS origin before enabling secure browser cookies and same-origin write checks. Leave the existing deployment pipeline unchanged.
2. **B2 cutover and recovery.** Add a cutover preflight that reports referenced PDF checksums, verified B2 locations and missing objects without changing source or citation identity. Run the existing resumable migration only after a coordinated database/local-assets backup. Implement scheduled encrypted PostgreSQL backups and an inventory or recoverable snapshot of referenced originals; alert on failure. Provide an isolated restore command and runbook that checks asset checksums, source/citation access and database consistency before any restored deployment serves traffic. Record retention, key rotation and recovery objectives as configuration or operator decisions.
3. **Operational signals.** Split process liveness from database/B2 readiness. Publish structured, secret-redacted API/worker logs and a small set of observable counters or checks for API failures, worker heartbeat/expired leases, oldest queued job, B2 failures, temporary disk pressure and backup age/failure. Wire actionable alerts to the configured operator destination; a dependency outage must not trigger destructive restarts.
4. **Bounded paid work.** Check durable per-principal request/job quotas and provider spending reservations before queueing ingestion or answer work. Bound concurrent uploads/processing and include retries in the same accounting. Persist reservations and actual usage across process restarts; report provider costs that are unavailable as unknown. Return a clear limit/retry response to the UI and API caller.

Build the remaining application work in the existing Go API/worker, Vue UI and PostgreSQL as needed. Leave the working deployment files untouched as requested. Add tables only when the current records cannot hold the required durable state, and explain each new table in the implementation change. Keep credentials and private source text out of logs, backup manifests and committed fixtures.

## Deliverables

1. Ship secure revocable pilot sessions and production-origin validation in the Go API. Keep the existing deployed network and secret configuration unchanged; verify the application settings against it before enabling production cookie enforcement.
2. Complete private B2 cutover with the provided migration command. Preserve every asset checksum, source identity, revision and saved citation. Configure bucket-scoped credentials, retention and key rotation. Remove dependence on shared writable PDF storage and provision bounded temporary processing capacity.
3. Implement automated PostgreSQL backups plus a matching original-object inventory, encrypted backup storage, retention, scheduled execution and failure alerts. Provide a restore command/runbook that restores a consistent database and asset set to a replacement deployment without overwriting the active service. Record recovery time/data-loss objectives before enabling the schedule.
4. Add production readiness checks for database/storage dependencies, structured redacted logs and metrics/alerts for API errors, worker liveness/lease expiry, queue age, storage failures, disk/temp pressure and backup failures. Separate liveness from readiness so outages do not trigger destructive restart loops.
5. Enforce per-principal request/job quotas, upload and processing concurrency limits, and provider spending limits before paid work is queued. Expose actionable retry/limit messages, persist accounting across restarts, and include retries in consumption. Preserve unknown provider costs as unknown. Do not rely on in-memory counters across replicas.

## Production acceptance

An administrator imports a permitted PDF in the deployed service, completes review/publication/indexing, opens a source-backed answer and its original citation, and reopens it after API/worker replacement without a shared writable asset volume. A signed-out, expired or revoked browser session cannot be reused; unauthorized and disabled-source reads remain blocked. Dependency outages surface recoverable failures; quotas prevent additional work beyond configured limits, including after restart and retry. An isolated restore recovers saved answers and their original PDFs with matching checksums. Alerts reach the configured operator destination, and the documented rollback preserves post-cutover assets.

Use focused automated checks and deployment acceptance as part of this delivery. This is not a standalone local-validation or evaluation task. Do not equate mocked checks with live release readiness. Research-quality and rights gates remain required for public launch.

## Configuration and release boundaries

Production hostname, bucket, secret source, operator alert destination, retention/recovery objectives and numerical spending limits must be supplied before their dependent application and operations checks. Do not invent credentials or increase paid limits. Back up before B2 cutover, retain prior images and originals, and record migration counts, restored asset integrity, access checks and remaining launch blockers without secrets or private source text. Production acceptance stays pending until the actual host, bucket, backup destination and alert destination have been exercised. The existing deployment workflow and scripts are outside this implementation scope at the user's request.

Account registration/recovery, broader roles and tenant isolation are a subsequent production feature slice; this task does not declare the existing token principals to be full customer accounts. HTML/TXT/XML intake follows the PRD section 40 end-to-end slices.

**Resume after the research core:** finish B2 cutover verification, backup/restore, alerts and durable usage controls before claiming production operations acceptance. The implemented session/readiness slice remains available; deferral does not mark the ticket complete.
