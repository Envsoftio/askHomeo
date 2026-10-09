#!/usr/bin/env python3
"""Run pinned evaluation cases and save evidence for a human review pass."""

import argparse
import hashlib
import json
import os
import pathlib
import statistics
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

CATEGORIES = ("single_source", "comparison", "author_edition", "unsupported")


def request(base, token, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(base.rstrip("/") + "/api/v1" + path, data=data, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=30) as response:
            return json.load(response)
    except urllib.error.HTTPError as exc:
        raise RuntimeError(f"{path}: HTTP {exc.code}: {exc.read().decode()[:500]}") from exc
    except (urllib.error.URLError, TimeoutError) as exc:
        raise RuntimeError(f"{path}: request failed: {exc}") from exc


def validate_base(base):
    parsed = urllib.parse.urlparse(base)
    if not parsed.netloc or parsed.username or parsed.password or parsed.query or parsed.fragment or parsed.path not in ("", "/"):
        raise ValueError("--base must be an API origin without credentials, path, query or fragment")
    if parsed.scheme == "https":
        return
    if parsed.scheme == "http" and parsed.hostname in ("localhost", "127.0.0.1", "::1"):
        return
    raise ValueError("Remote evaluation APIs require HTTPS")


def valid_uuid(value):
    try:
        return str(uuid.UUID(value)) == value
    except (ValueError, TypeError, AttributeError):
        return False


def validate(dataset, smoke, require_review=False):
    if not isinstance(dataset, dict) or not isinstance(dataset.get("cases"), list):
        raise ValueError("Dataset needs a cases array")
    cases = dataset["cases"]
    if not cases:
        raise ValueError("Dataset has no cases")
    if not isinstance(dataset.get("version"), str) or not dataset["version"].strip():
        raise ValueError("Dataset needs a version")
    sources = dataset.get("sources")
    if not isinstance(sources, dict) or not sources:
        raise ValueError("Dataset needs pinned sources")
    pinned_ids = []
    for key, source in sources.items():
        if not isinstance(source, dict):
            raise ValueError(f"Source {key} needs an identity record")
        sha = source.get("pdf_sha256")
        if not isinstance(sha, str) or len(sha) != 64 or any(c not in "0123456789abcdef" for c in sha):
            raise ValueError(f"Source {key} needs a lowercase SHA-256 checksum")
        for field in ("id", "source_asset_id", "processing_revision_id", "index_run_id"):
            value = source.get(field)
            if value is not None and not valid_uuid(value):
                raise ValueError(f"Source {key} has invalid {field}")
        if not smoke and any(not source.get(field) for field in ("id", "source_asset_id", "processing_revision_id", "index_run_id")):
            raise ValueError(f"Source {key} lacks a complete pinned identity")
        if source.get("id"):
            pinned_ids.append(source["id"])
    if len(pinned_ids) != len(set(pinned_ids)):
        raise ValueError("Each source key must refer to a distinct source ID")
    ids = []
    groups = dict.fromkeys(CATEGORIES, 0)
    for case in cases:
        if not isinstance(case, dict) or not isinstance(case.get("id"), str) or not case["id"]:
            raise ValueError("Every case needs an ID")
        cid = case["id"]
        ids.append(cid)
        category = case.get("category")
        if category not in CATEGORIES:
            raise ValueError(f"{cid}: invalid category")
        groups[category] += 1
        if not isinstance(case.get("question"), str) or not case["question"].strip():
            raise ValueError(f"{cid}: question is empty")
        keys = case.get("source_keys")
        if not isinstance(keys, list) or not keys or any(not isinstance(key, str) for key in keys) or len(keys) != len(set(keys)) or any(key not in sources for key in keys):
            raise ValueError(f"{cid}: source_keys must name distinct pinned sources")
        if case.get("mode", "quick") not in ("quick", "deep"):
            raise ValueError(f"{cid}: invalid mode")
        if not isinstance(case.get("held_out"), bool):
            raise ValueError(f"{cid}: held_out must be boolean")
        if case.get("review_status", "unreviewed") not in ("unreviewed", "reviewed"):
            raise ValueError(f"{cid}: invalid review_status")
        gold = case.get("gold_chunk_ids", [])
        points = case.get("expected_points", [])
        if not isinstance(gold, list) or any(not valid_uuid(v) for v in gold) or len(gold) != len(set(gold)):
            raise ValueError(f"{cid}: invalid gold_chunk_ids")
        if not isinstance(points, list) or any(not isinstance(v, str) or not v.strip() for v in points):
            raise ValueError(f"{cid}: invalid expected_points")
        if not smoke or require_review:
            if case.get("review_status") != "reviewed" or not case.get("reviewer") or not case.get("reviewed_at"):
                raise ValueError(f"{cid} has no recorded human review")
            if category != "unsupported" and (not gold or not points):
                raise ValueError(f"{cid} needs gold passages and expected points")
    if len(ids) != len(set(ids)):
        raise ValueError("Case IDs must be unique")
    if not smoke and any(count == 0 for count in groups.values()):
        raise ValueError(f"Reviewed release dataset needs every question category: {groups}")
    return groups


def select_cases(cases, requested):
    if not requested:
        return cases
    if len(requested) != len(set(requested)) or set(requested) - {c["id"] for c in cases}:
        raise ValueError("Unknown or duplicate case ID")
    return [c for c in cases if c["id"] in requested]


def preflight_sources(base, token, dataset, cases):
    """Check every source before queuing any paid answer job."""
    wanted = {key for case in cases for key in case["source_keys"]}
    selected = {}
    for key in sorted(wanted):
        pinned = dataset["sources"][key]
        if any(not pinned.get(field) for field in ("id", "source_asset_id", "processing_revision_id", "index_run_id")):
            raise ValueError(f"Source {key} is not pinned; complete source review before running jobs")
        source = request(base, token, "/sources/" + pinned["id"])
        for field in ("id", "pdf_sha256", "source_asset_id", "processing_revision_id"):
            if source.get(field) != pinned[field]:
                raise ValueError(f"Source {key} {field} drifted: expected {pinned[field]}, got {source.get(field)}")
        if source.get("published_revision_id") != pinned["processing_revision_id"]:
            raise ValueError(f"Source {key} published revision drifted")
        if source.get("status") != "published" or source.get("rights_status") != "allowed" or source.get("superseded"):
            raise ValueError(f"Source {key} is not active, published, and rights allowed")
        index = request(base, token, "/sources/" + pinned["id"] + "/index-status")
        if index.get("active_status", "").lower() != "ready" or not index.get("active_matches_config") or index.get("active_run_id") != pinned["index_run_id"] or index.get("active_processing_revision_id") != pinned["processing_revision_id"]:
            raise ValueError(f"Source {key} lacks the pinned, compatible READY index")
        selected[key] = pinned["id"]
    return selected


def wait_for_job(base, token, job_id, timeout):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        job = request(base, token, "/research/answer-jobs/" + job_id)
        if job["status"] in ("finished", "failed"):
            return job
        time.sleep(3)
    raise TimeoutError(f"Answer job {job_id} did not finish within {timeout}s")


def add_model_usage(base, token, result):
    try:
        report = request(base, token, "/admin/answer-jobs/" + result["job_id"] + "/evaluation")
        result["model_calls"] = report["model_calls"]
        result["model_usage_summary"] = report["model_usage_summary"]
    except RuntimeError as exc:
        result["model_usage_summary"] = None
        result["model_usage_error"] = str(exc)


def run_case(base, token, case, selected, source_pins, timeout, checkpoint, prior=None):
    source_ids = [selected[key] for key in case["source_keys"]]
    pins_by_id = {selected[key]: source_pins[key] for key in case["source_keys"]}
    began = time.monotonic()
    result = prior or {"id": case["id"], "category": case["category"], "held_out": case["held_out"], "review_status": case.get("review_status", "unreviewed"), "question": case["question"], "selected_source_ids": source_ids, "job_status": "not_submitted", "human_claim_review": None, "human_point_coverage": None, "human_unsupported_safe": None}
    if not result.get("job_id"):
        queued = request(base, token, "/research/answer-jobs", {"question": case["question"], "mode": case.get("mode", "quick"), "source_ids": source_ids})
        result.update({"job_id": queued["job_id"], "job_status": "queued"})
        checkpoint(result)
    try:
        job = wait_for_job(base, token, result["job_id"], timeout)
    except TimeoutError as exc:
        result.update({"job_status": "timed_out", "error": str(exc), "elapsed_seconds": round(time.monotonic() - began, 2)})
        return result
    result.update({"job_id": job["id"], "job_status": job["status"], "elapsed_seconds": round(time.monotonic() - began, 2), "error": job.get("error_message")})
    if job["status"] != "finished":
        add_model_usage(base, token, result)
        return result
    answer = request(base, token, "/research/answers/" + job["answer_id"])
    claims = request(base, token, "/research/answers/" + job["answer_id"] + "/claims")["claims"]
    citation_records = {}
    citation_integrity = answer["status"] == "insufficient_evidence" or bool(answer["citations"])
    for citation in answer["citations"]:
        record = request(base, token, "/citations/" + citation["id"])
        citation_records[citation["label"]] = record
        pin = pins_by_id.get(record["source_id"])
        citation_integrity &= bool(pin) and record["id"] == citation["id"] and record["title"] == citation["title"] and record["author"] == citation["author"] and record["scan_position"] == citation["scan_position"] and record["pdf_sha256"] == pin["pdf_sha256"] and record["source_asset_id"] == pin["source_asset_id"] and record["processing_revision_id"] == pin["processing_revision_id"]
    for claim in claims:
        if claim["decision"] != "supported":
            continue
        if not claim["supports"]:
            citation_integrity = False
        for span in claim["supports"]:
            record = citation_records.get(span["label"])
            citation_integrity &= bool(record) and record["chunk_id"] == span["chunk_id"] and record["passage"][span["excerpt_start"]:span["excerpt_end"]] == span["excerpt"]
    retrieved = [e["chunk_id"] for e in answer["evidence"] if e.get("chunk_id")]
    gold = case.get("gold_chunk_ids", []) if case.get("review_status") == "reviewed" else []
    displayed_claims = [claim["claim"] for claim in claims if claim["decision"] == "supported"]
    if not displayed_claims and answer["status"] in ("answered", "partial"):
        displayed_claims = [answer["answer"]]
    result.update({"answer_id": job["answer_id"], "answer_status": answer["status"], "answer": answer["answer"], "omitted_claim_count": answer["omitted_claim_count"], "retrieved_chunk_ids": retrieved, "retrieved_evidence": answer["evidence"], "citations": answer["citations"], "citation_records": citation_records, "citation_integrity": citation_integrity, "claims": claims, "displayed_claims": displayed_claims, "gold_evidence_recall_top_10": (len(set(gold) & set(retrieved[:10])) / len(set(gold))) if gold else None, "expected_points": case.get("expected_points", [])})
    add_model_usage(base, token, result)
    return result


def write_report(path, report):
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(prefix=".evaluation-", suffix=".json", dir=path.parent)
    try:
        with os.fdopen(descriptor, "w") as stream:
            os.chmod(temporary, 0o600)
            json.dump(report, stream, indent=2)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default=os.getenv("EVALUATION_API_BASE"), help="Staging or production API origin; required for live runs")
    parser.add_argument("--dataset", type=pathlib.Path, required=True, help="Versioned dataset built from reviewed, real sources")
    parser.add_argument("--output", type=pathlib.Path, help="Private result file; required for live runs")
    parser.add_argument("--smoke", action="store_true", help="Small diagnostic; live cases still require human review")
    parser.add_argument("--case", action="append", help="Run only named case IDs")
    parser.add_argument("--dry-run", action="store_true", help="Validate offline without submitting jobs")
    parser.add_argument("--timeout", type=int, default=900)
    parser.add_argument("--resume", action="store_true", help="Resume jobs saved in an existing result file")
    args = parser.parse_args()
    dataset_bytes = args.dataset.read_bytes()
    dataset = json.loads(dataset_bytes)
    dataset_sha256 = hashlib.sha256(dataset_bytes).hexdigest()
    groups = validate(dataset, args.smoke, require_review=not args.dry_run)
    cases = select_cases(dataset["cases"], args.case)
    if args.dry_run:
        pending = sorted({key for case in cases for key in case["source_keys"] if any(not dataset["sources"][key].get(field) for field in ("id", "source_asset_id", "processing_revision_id", "index_run_id"))})
        print(json.dumps({"version": dataset["version"], "groups": groups, "selected_case_ids": [c["id"] for c in cases], "reviewed": sum(c.get("review_status") == "reviewed" for c in cases), "sources_pending_identity": pending}, indent=2))
        return
    if args.output is None:
        parser.error("--output is required for a live run")
    if not args.base:
        parser.error("--base or EVALUATION_API_BASE is required for a live run")
    validate_base(args.base)
    token = os.getenv("EVALUATION_API_TOKEN") or os.getenv("API_TOKEN", "")
    if not token:
        parser.error("EVALUATION_API_TOKEN or API_TOKEN is required for a live run")
    selected = preflight_sources(args.base, token, dataset, cases)
    report = {"dataset_version": dataset["version"], "dataset_sha256": dataset_sha256, "dataset": str(args.dataset), "smoke": args.smoke, "case_ids": [case["id"] for case in dataset["cases"]], "selected_case_ids": [case["id"] for case in cases], "source_pins": {key: dataset["sources"][key] for key in selected}, "results": []}
    if args.output.exists():
        if not args.resume:
            raise ValueError(f"{args.output} already exists; use --resume or choose a new output")
        previous = json.loads(args.output.read_text())
        for field in ("dataset_sha256", "smoke", "case_ids", "selected_case_ids", "source_pins"):
            if previous.get(field) != report[field]:
                raise ValueError(f"Cannot resume: {field} changed")
        report = previous
    elif args.resume:
        raise ValueError(f"Cannot resume: {args.output} does not exist")
    rows = {row["id"]: row for row in report["results"]}

    def checkpoint(row):
        rows[row["id"]] = row
        report["results"] = [rows[case["id"]] for case in cases if case["id"] in rows]
        write_report(args.output, report)

    for case in cases:
        prior = rows.get(case["id"])
        if prior and prior.get("job_status") in ("finished", "failed"):
            continue
        print(f"{case['id']}: {case['question']}", flush=True)
        try:
            preflight_sources(args.base, token, dataset, [case])
            result = run_case(args.base, token, case, selected, dataset["sources"], args.timeout, checkpoint, prior)
        except (RuntimeError, TimeoutError, KeyError, ValueError) as exc:
            result = rows.get(case["id"]) or prior or {"id": case["id"], "category": case["category"], "held_out": case["held_out"], "review_status": case.get("review_status", "unreviewed"), "human_claim_review": None, "human_point_coverage": None, "human_unsupported_safe": None}
            result.update({"job_status": "interrupted" if result.get("job_id") else "not_submitted", "error": str(exc)})
        checkpoint(result)
        if result["job_status"] in ("not_submitted", "interrupted", "timed_out"):
            break
    results = report["results"]
    recalls = [r["gold_evidence_recall_top_10"] for r in results if r.get("gold_evidence_recall_top_10") is not None]
    elapsed = [r["elapsed_seconds"] for r in results if r.get("elapsed_seconds") is not None]
    print(json.dumps({"completed": len(results), "failed_jobs": sum(r["job_status"] == "failed" for r in results), "mean_recall_top_10": statistics.mean(recalls) if recalls else None, "median_elapsed_seconds": statistics.median(elapsed) if elapsed else None, "human_claim_support": None, "human_expected_point_coverage": None}, indent=2))


if __name__ == "__main__":
    main()
