# ING-01 — Safe public source fetching and document locations

**Status:** Implemented. Fixture checks, Go suite and frontend build passed on 2026-10-10. No live URL check was run.

**User outcome:** An administrator can preview a public HTTP or HTTPS source link safely and understand its actual format, final address, transport and extraction readiness before starting import. The resulting original location can be represented without pretending it is a PDF page.

**PRD basis:** Sections 36 and 40.1–40.6. This is the shared foundation for the first complete HTML/TXT file and URL intake in ING-02. Keep the existing PDF upload/import, review, rights, publication, index and saved citation flow working.

## Complete feature slice

1. Add an administrator-only URL preview flow in the existing Sources UI and Go API. Accept explicit HTTP and HTTPS. Show requested/final URL, redirect/transport change, detected type, byte size and a short inert text sample. Show clear outcomes for unsupported type, empty/error/login page, TLS failure, size limit and blocked destination. Preserve the entered URL on failure. Preview must not create a source or mark it ready.
2. Build one shared server-side fetcher for previews and later imports. Bound redirects, request time, decompressed bytes and ports. Validate the initial URL, every redirect and the resolved address at connect time; reject credentials in URLs and private, loopback, link-local, metadata and other non-public destinations. Never forward application cookies or authorization. Honor explicit HTTP, never silently downgrade requested HTTPS after TLS failure, and require an explicit recorded allowance for HTTPS-to-HTTP redirects.
3. Define a durable non-PDF original/document locator that can identify a saved asset, revision and section or text span. Keep PDF page/citation IDs and response fields valid. Add only schema needed by this implemented preview/location feature; explain why existing PDF-only columns cannot hold it. Raw HTML must not be served as executable same-origin content.
4. Reuse the shared fetcher for direct PDF URL acquisition where compatible, preserving the existing 250 MiB PDF limit and rights gates. The preview and PDF path must use the same network checks. Do not silently broaden ALTO OCR, DOI metadata or archive connectors.

## Acceptance

- An authorized administrator previews a public HTTP-only page and a public HTTPS page. The response identifies format and final address; nothing is imported or published by preview alone. A supported direct PDF link still imports through the existing background review flow.
- Private IPs, DNS rebinding, disallowed ports, credential-bearing URLs and redirect escapes fail before a network connection to the blocked destination. A TLS error never causes automatic HTTP retry. HTTP content is visibly labeled as unencrypted acquisition.
- A saved PDF citation still resolves to its original page after the schema change. A non-PDF locator represents a section/span without fabricated page numbers. Unknown structure or missing text remains a visible blocker for ING-02.
- Focused fetch/redirect fixtures, affected Go checks and the frontend build pass. Record any live URL checks separately from fixture results and do not claim that preview alone completes HTML/TXT/XML ingestion.

**Following slice:** ING-02 delivers upload and URL intake for HTML/TXT through extraction review, publication, compatible indexing, Quick/Deep retrieval and reopening exact original citations. Backups/restore and operational alerts in PROD-01 remain deferred while this research core work proceeds.

## Implementation notes

- `POST /api/v1/sources/preview-url` is administrator-only and read-only. It accepts `{ "url": "...", "allow_https_to_http_redirect": false }`. A preview reads at most 10 MiB of decompressed content and reports the requested/final URL, transport, detected format, byte count, sample and readiness. A larger document reports the preview limit; direct PDF import retains its 250 MiB limit.
- Preview and direct PDF URL import use `internal/safefetch`. Explicit HTTP remains HTTP, TLS failures do not retry over HTTP, and an HTTPS-to-HTTP redirect requires the preview request's explicit allowance. Direct PDF imports never allow that downgrade. DOI metadata and archive connector policies are unchanged; DOI PDF downloads retain HTTPS-only eligibility while using the same fetcher.
- Migration 037 adds revision-scoped `document_locations` for a PDF page, section or text span. Existing PDF page IDs and citation response fields remain intact. `sources` and `pages` contain required PDF-only paths, hashes and page indices, so non-PDF locations live in the separate table with no invented page number. Future ING-02 intake will populate non-PDF assets and locations. Raw HTML has no serving route.
- Fixtures cover blocked URLs, redirect escape, mixed public/private DNS answers, TLS failure, byte limits and explicit downgrade policy. All migrations applied to a disposable PostgreSQL database; a non-PDF text span with no PDF page and a saved PDF citation resolving to page index 0 were checked there. The disposable database was removed. No live URL result or HTML/TXT/XML ingestion is claimed.
