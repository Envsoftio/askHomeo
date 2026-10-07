#!/usr/bin/env python3
"""Run saved release questions and export evidence for a human scoring pass.

Draft questions are allowed only with --smoke. A release run requires reviewed
gold passage IDs and expected points for every answerable question.
"""

import argparse
import json
import os
import pathlib
import statistics
import time
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
DATASET = ROOT / "evaluation/questions.draft.json"


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


def validate(cases, smoke):
    groups = {name: sum(c["category"] == name for c in cases) for name in ("single_source", "comparison", "author_edition", "unsupported")}
    expected = {"single_source": 10, "comparison": 10, "author_edition": 5, "unsupported": 5}
    if groups != expected or len({c["id"] for c in cases}) != 30:
        raise ValueError(f"Question distribution must be 10/10/5/5 with unique IDs: {groups}")
    if not smoke:
        for c in cases:
            if c.get("review_status") != "reviewed":
                raise ValueError(f"{c['id']} has no recorded human review")
            if c["category"] != "unsupported" and (not c.get("gold_chunk_ids") or not c.get("expected_points")):
                raise ValueError(f"{c['id']} needs gold passages and expected points")
    return groups


def wait_for_job(base, token, job_id, timeout):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        job = request(base, token, "/research/answer-jobs/" + job_id)
        if job["status"] in ("finished", "failed"):
            return job
        time.sleep(3)
    raise TimeoutError(f"Answer job {job_id} did not finish within {timeout}s")


def run_case(base, token, case, sources, timeout):
    wanted = case["source_titles"]
    selected = []
    for title in wanted:
        matches = [s for s in sources if s["title"] == title and s["status"] == "published" and not s.get("superseded")]
        if len(matches) != 1:
            raise ValueError(f"{case['id']}: expected one active published source named {title!r}, got {len(matches)}")
        selected.append(matches[0]["id"])
    began = time.monotonic()
    queued = request(base, token, "/research/answer-jobs", {"question": case["question"], "mode": case.get("mode", "quick"), "source_ids": selected})
    job = wait_for_job(base, token, queued["job_id"], timeout)
    result = {"id": case["id"], "category": case["category"], "held_out": case["held_out"], "question": case["question"], "job_id": job["id"], "job_status": job["status"], "elapsed_seconds": round(time.monotonic() - began, 2), "error": job.get("error_message")}
    if job["status"] != "finished":
        return result
    answer = request(base, token, "/research/answers/" + job["answer_id"])
    claims = request(base, token, "/research/answers/" + job["answer_id"] + "/claims")["claims"]
    citation_records = {}
    citation_integrity = True
    for citation in answer["citations"]:
        record = request(base, token, "/citations/" + citation["id"])
        citation_records[citation["label"]] = record
        citation_integrity &= record["id"] == citation["id"] and record["title"] == citation["title"] and record["author"] == citation["author"] and record["scan_position"] == citation["scan_position"] and len(record["pdf_sha256"]) == 64
    for claim in claims:
        if claim["decision"] != "supported":
            continue
        if not claim["supports"]:
            citation_integrity = False
        for span in claim["supports"]:
            record = citation_records.get(span["label"])
            citation_integrity &= bool(record) and record["chunk_id"] == span["chunk_id"] and record["passage"][span["excerpt_start"]:span["excerpt_end"]] == span["excerpt"]
    retrieved = [e["chunk_id"] for e in answer["evidence"] if e.get("chunk_id")]
    gold = case.get("gold_chunk_ids", [])
    displayed_claims = [claim["claim"] for claim in claims if claim["decision"] == "supported"]
    if not displayed_claims and answer["status"] in ("answered", "partial"):
        displayed_claims = [answer["answer"]]
    result.update({"answer_id": job["answer_id"], "answer_status": answer["status"], "answer": answer["answer"], "omitted_claim_count": answer["omitted_claim_count"], "retrieved_chunk_ids": retrieved, "citations": answer["citations"], "citation_records": citation_records, "citation_integrity": citation_integrity, "claims": claims, "displayed_claims": displayed_claims, "gold_evidence_recall_top_10": (len(set(gold) & set(retrieved[:10])) / len(set(gold))) if gold else None, "expected_points": case.get("expected_points", []), "human_claim_review": None, "human_point_coverage": None})
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default="http://127.0.0.1:8088", help="Local frontend URL or API origin")
    parser.add_argument("--dataset", type=pathlib.Path, default=DATASET)
    parser.add_argument("--output", type=pathlib.Path, default=ROOT / "evaluation/results.json")
    parser.add_argument("--smoke", action="store_true", help="Permit draft questions; report cannot pass release gates")
    parser.add_argument("--case", action="append", help="Run only named case IDs")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--timeout", type=int, default=900)
    args = parser.parse_args()
    cases = json.loads(args.dataset.read_text())["cases"]
    groups = validate(cases, args.smoke or args.dry_run)
    if args.case:
        cases = [c for c in cases if c["id"] in args.case]
        if len(cases) != len(set(args.case)):
            raise ValueError("Unknown or duplicate case ID")
    if args.dry_run:
        print(json.dumps({"groups": groups, "selected": len(cases), "reviewed": sum(c.get("review_status") == "reviewed" for c in cases)}, indent=2))
        return
    token = os.getenv("API_TOKEN", "")
    sources = request(args.base, token, "/sources")
    results = []
    for case in cases:
        print(f"{case['id']}: {case['question']}", flush=True)
        results.append(run_case(args.base, token, case, sources, args.timeout))
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps({"dataset": str(args.dataset), "smoke": args.smoke, "results": results}, indent=2) + "\n")
    recalls = [r["gold_evidence_recall_top_10"] for r in results if r.get("gold_evidence_recall_top_10") is not None]
    print(json.dumps({"completed": len(results), "failed_jobs": sum(r["job_status"] == "failed" for r in results), "mean_recall_top_10": statistics.mean(recalls) if recalls else None, "median_elapsed_seconds": statistics.median(r["elapsed_seconds"] for r in results) if results else None, "human_claim_support": "pending review", "human_expected_point_coverage": "pending review"}, indent=2))


if __name__ == "__main__":
    main()
