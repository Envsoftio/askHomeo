#!/usr/bin/env python3
"""Score a fully reviewed POC evaluation result; refuse incomplete release data."""

import argparse
import json
import pathlib
import statistics

ROOT = pathlib.Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("results", type=pathlib.Path, nargs="?", default=ROOT / "evaluation/results.json")
    args = parser.parse_args()
    report = json.loads(args.results.read_text())
    rows = report["results"]
    if report.get("smoke") or len(rows) != 30 or len({r["id"] for r in rows}) != 30:
        raise ValueError("Release scoring requires all 30 unique non-smoke cases")
    if any(r["job_status"] != "finished" for r in rows):
        raise ValueError("Some answer jobs failed; inspect the saved errors")
    answerable = [r for r in rows if r["category"] != "unsupported"]
    unsupported = [r for r in rows if r["category"] == "unsupported"]
    if any(r.get("gold_evidence_recall_top_10") is None for r in answerable):
        raise ValueError("Gold passage IDs are missing")
    if any(r.get("human_claim_review") is None or r.get("human_point_coverage") is None for r in rows):
        raise ValueError("Human claim and expected-point reviews are missing")
    if any(len(r["human_claim_review"]) != len(r.get("displayed_claims", [])) for r in rows):
        raise ValueError("Every displayed claim needs exactly one human support decision")
    if any(not r.get("citation_integrity") for r in rows):
        raise ValueError("At least one saved citation or excerpt failed resolution")
    claim_decisions = [decision for row in rows for decision in row["human_claim_review"]]
    if any(decision not in (True, False) for decision in claim_decisions):
        raise ValueError("Human claim decisions must be boolean")
    if not claim_decisions:
        raise ValueError("No displayed claims were reviewed")
    points = [covered for row in answerable for covered in row["human_point_coverage"]]
    if any(point not in (True, False) for point in points) or not points:
        raise ValueError("Expected-point coverage must be reviewed as booleans")
    if any(len(row["human_point_coverage"]) != len(row["expected_points"]) for row in answerable):
        raise ValueError("Every expected point needs a coverage decision")
    recall = statistics.mean(r["gold_evidence_recall_top_10"] for r in answerable)
    claim_support = sum(claim_decisions) / len(claim_decisions)
    coverage = sum(points) / len(points)
    unsupported_safe = all(r["answer_status"] == "insufficient_evidence" or r.get("human_unsupported_safe") is True for r in unsupported)
    metrics = {"evidence_recall_top_10": recall, "human_claim_support": claim_support, "expected_point_coverage": coverage, "unsupported_safe": unsupported_safe, "citation_integrity": True, "median_latency_seconds": statistics.median(r["elapsed_seconds"] for r in rows), "gates_passed": recall >= .90 and claim_support >= .95 and coverage >= .90 and unsupported_safe}
    print(json.dumps(metrics, indent=2))


if __name__ == "__main__":
    main()
