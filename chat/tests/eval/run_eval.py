"""Score the chat agent over the golden question set.

Drives the stateless POST /api/chat/answer, so a run stores no conversations. Costs money.
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import signal
import statistics
import sys
import time
from collections import defaultdict
from dataclasses import asdict, dataclass, field
from datetime import UTC, datetime
from pathlib import Path
from types import FrameType
from typing import Any

import httpx
import yaml

HERE = Path(__file__).resolve().parent
# parents[2] is the repo root locally and / in the container, where docs/ is mounted at /docs.
DEMO_DIR = Path(os.getenv("EVAL_DEMO_DIR", HERE.parents[2] / "docs" / "demo"))
SET_DIRS = (DEMO_DIR, HERE / "fixtures")
QUESTIONS = HERE / "questions.yaml"
THRESHOLDS = HERE / "thresholds.yaml"
RESULTS_DIR = HERE / "results"

ANSWER_TIMEOUT_SECONDS = 180.0
RETRIES_ON_429 = 5


@dataclass
class Scores:
    recall: float | None
    precision: float | None
    tools: bool | None
    substr: bool | None
    violations: list[str]
    refusal_ok: bool | None

    @property
    def passed(self) -> bool:
        return (
            self.recall in (None, 1.0)
            and self.tools is not False
            and self.substr is not False
            and not self.violations
            and self.refusal_ok is not False
        )


@dataclass
class Result:
    id: str
    set: str
    category: str
    language: str
    question: str
    run: int
    answer: str = ""
    citations: list[str] = field(default_factory=list)
    tool_calls: list[dict[str, Any]] = field(default_factory=list)
    model: str = ""
    usage: dict[str, int] = field(default_factory=dict)
    latency_ms: int = 0
    wall_ms: int = 0
    error: str | None = None
    scores: Scores | None = None


def score(
    q: dict[str, Any], citations: list[str], tool_calls: list[dict[str, Any]], text: str
) -> Scores:
    expected = {c.lower() for c in q.get("expect_citations") or []}
    actual = {c.lower() for c in citations}
    hit = len(expected & actual)

    # Nothing required means nothing to recall; a refusal's citations are judged below instead.
    recall = hit / len(expected) if expected else None
    # An empty citation list claims nothing, so precision is undefined rather than zero.
    precision = hit / len(actual) if expected and actual else None

    called = {c.get("name") for c in tool_calls}
    wanted = set(q.get("expect_tools_any") or [])
    # Nothing required: every turn now retrieves first (08.7), so a call is not a miss.
    tools = bool(called & wanted) if wanted else None

    lowered = text.lower()
    contains = q.get("expect_contains_any") or []
    substr = any(s.lower() in lowered for s in contains) if contains else None
    violations = [s for s in q.get("must_not_contain") or [] if s.lower() in lowered]

    # A correct refusal may name the table it checked; anything else cited is suspect.
    allowed = expected | {c.lower() for c in q.get("allow_citations") or []}
    refusal_ok = (actual <= allowed and not violations) if q["category"] == "refusal" else None
    return Scores(recall, precision, tools, substr, violations, refusal_ok)


def select(questions: list[dict[str, Any]], args: argparse.Namespace) -> list[dict[str, Any]]:
    return [
        q
        for q in questions
        if (not args.set or q["set"] in args.set)
        and (not args.category or q["category"] in args.category)
        and (not args.id or q["id"] in args.id)
    ]


class Snapshots:
    """Ingests each demo set once and deletes everything this run created."""

    def __init__(self, http: httpx.Client, label: str) -> None:
        self.http = http
        self.label = label
        self.ids: dict[str, str] = {}

    def ingest(self, name: str) -> str:
        root = next((d / name for d in SET_DIRS if (d / name).is_dir()), None)
        if root is None:
            raise RuntimeError(f"set {name!r} not found in {', '.join(map(str, SET_DIRS))}")
        files = [
            {"path": str(p.relative_to(root)), "content": p.read_text()}
            for p in sorted(root.rglob("*"))
            # A fixture's README is for people; ingested, it would add a diagnostic.
            if p.suffix in {".md", ".toml"} and p.relative_to(root) != Path("README.md")
        ]
        body = {"name": name, "sourceLabel": self.label, "files": files}
        response = self.http.post("/api/v1/ingest", json=body)
        response.raise_for_status()
        sid: str = response.json()["snapshot"]["id"]
        self.ids[name] = sid
        return sid

    def cleanup(self) -> list[str]:
        # Also swept by label, which catches an ingest that landed after an interrupt.
        ids = set(self.ids.values())
        try:
            listed = self.http.get("/api/v1/snapshots").json().get("snapshots") or []
            ids |= {s["id"] for s in listed if s.get("sourceLabel") == self.label}
        except httpx.HTTPError as exc:
            print(f"warning: could not list snapshots for cleanup: {exc}", file=sys.stderr)

        failed = []
        for sid in ids:
            try:
                status = self.http.delete(f"/api/v1/snapshots/{sid}").status_code
                if status not in (204, 404):
                    failed.append(f"{sid} ({status})")
            except httpx.HTTPError as exc:
                failed.append(f"{sid} ({exc})")
        return failed


async def ask(
    chat: httpx.AsyncClient,
    q: dict[str, Any],
    snapshot_id: str,
    run: int,
    gate: asyncio.Semaphore,
    expected_model: str | None,
) -> Result:
    result = Result(q["id"], q["set"], q["category"], q["language"], q["question"], run)
    body = {"snapshotId": snapshot_id, "question": q["question"], "language": q["language"]}
    async with gate:
        started = time.perf_counter()
        try:
            for attempt in range(RETRIES_ON_429 + 1):
                response = await chat.post("/api/chat/answer", json=body)
                if response.status_code != 429 or attempt == RETRIES_ON_429:
                    break
                await asyncio.sleep(2**attempt)
            response.raise_for_status()
            data = response.json()
        except httpx.HTTPError as exc:
            detail = exc.response.text[:300] if isinstance(exc, httpx.HTTPStatusError) else ""
            result.error = f"{type(exc).__name__}: {exc} {detail}".strip()
            return result
        finally:
            result.wall_ms = round((time.perf_counter() - started) * 1000)

    result.answer = data["text"]
    result.citations = data["citations"]
    result.tool_calls = data["toolCalls"]
    result.model = data["model"]
    result.usage = data.get("usage") or {}
    result.latency_ms = data["latencyMs"]
    if expected_model and result.model != expected_model:
        # The service picks the model; another one's answers would be scored under the wrong name.
        raise SystemExit(
            f"the chat service answered with {result.model!r}, not {expected_model!r}; "
            f"restart it with LLM_MODEL={expected_model}"
        )
    result.scores = score(q, result.citations, result.tool_calls, result.answer)
    print(f"  {'pass' if result.scores.passed else 'FAIL'}  {q['id']} (run {run})", flush=True)
    return result


async def run_all(
    args: argparse.Namespace, selected: list[dict[str, Any]], snapshot_ids: dict[str, str]
) -> list[Result]:
    gate = asyncio.Semaphore(args.concurrency)
    async with httpx.AsyncClient(base_url=args.chat_url, timeout=ANSWER_TIMEOUT_SECONDS) as chat:
        ready = await chat.get("/readyz")
        if ready.status_code != 200:
            raise SystemExit(f"chat service is not ready: {ready.status_code} {ready.text[:200]}")
        tasks = [
            ask(chat, q, snapshot_ids[q["set"]], run, gate, args.model)
            for run in range(1, args.repeat + 1)
            for q in selected
        ]
        return list(await asyncio.gather(*tasks))


def mean(values: list[float]) -> float | None:
    return statistics.fmean(values) if values else None


def fmt(value: float | None) -> str:
    return "--" if value is None else f"{value:.2f}"


def summarise(results: list[Result]) -> dict[str, Any]:
    scored = [r for r in results if r.scores]

    def row(rs: list[Result]) -> dict[str, Any]:
        s = [r.scores for r in rs if r.scores]
        return {
            "n": len(rs),
            "recall": mean([x.recall for x in s if x.recall is not None]),
            "precision": mean([x.precision for x in s if x.precision is not None]),
            "tools": mean([float(x.tools) for x in s if x.tools is not None]),
            "substr": mean([float(x.substr) for x in s if x.substr is not None]),
            "violations": sum(len(x.violations) for x in s),
        }

    by_category: dict[str, list[Result]] = defaultdict(list)
    by_question: dict[str, list[Result]] = defaultdict(list)
    for r in results:
        by_category[r.category].append(r)
        by_question[r.id].append(r)

    refusals = [r.scores.refusal_ok for r in scored if r.scores and r.scores.refusal_ok is not None]
    per_question = {}
    for qid, rs in by_question.items():
        recalls = [r.scores.recall for r in rs if r.scores and r.scores.recall is not None]
        per_question[qid] = {
            "runs": len(rs),
            "passes": sum(1 for r in rs if r.scores and r.scores.passed),
            "errors": sum(1 for r in rs if r.error),
            "recall_mean": mean(recalls),
            "recall_stdev": statistics.pstdev(recalls) if len(recalls) > 1 else 0.0,
        }

    return {
        "categories": {c: row(rs) for c, rs in sorted(by_category.items())},
        "overall": row(results),
        "refusal_accuracy": mean([float(x) for x in refusals]),
        "violations": [
            {"id": r.id, "run": r.run, "strings": r.scores.violations}
            for r in scored
            if r.scores and r.scores.violations
        ],
        "errors": [{"id": r.id, "run": r.run, "error": r.error} for r in results if r.error],
        "per_question": per_question,
        "tokens_in": sum(r.usage.get("input_tokens", 0) for r in results),
        "tokens_out": sum(r.usage.get("output_tokens", 0) for r in results),
        "mean_wall_ms": mean([float(r.wall_ms) for r in results]),
    }


def report(summary: dict[str, Any], repeat: int, wall_seconds: float) -> None:
    print(
        f"\n{'category':<14}{'n':>4}{'recall':>9}{'precision':>11}"
        f"{'tools':>8}{'substr':>8}{'violations':>12}"
    )

    def line(name: str, row: dict[str, Any]) -> None:
        print(
            f"{name:<14}{row['n']:>4}{fmt(row['recall']):>9}{fmt(row['precision']):>11}"
            f"{fmt(row['tools']):>8}{fmt(row['substr']):>8}{row['violations']:>12}"
        )

    for name, row in summary["categories"].items():
        line(name, row)
    line("overall", summary["overall"])
    print(f"\nrefusal accuracy: {fmt(summary['refusal_accuracy'])}")

    if summary["violations"]:
        print("\nVIOLATIONS")
        for v in summary["violations"]:
            print(f"  {v['id']} (run {v['run']}): {', '.join(v['strings'])}")
    if summary["errors"]:
        print("\nERRORS")
        for e in summary["errors"]:
            print(f"  {e['id']} (run {e['run']}): {e['error']}")

    if repeat > 1:
        print(f"\n{'question':<44}{'passes':>8}{'recall':>9}{'spread':>8}")
        for qid, q in sorted(summary["per_question"].items()):
            print(
                f"{qid:<44}{q['passes']:>4}/{q['runs']:<3}{fmt(q['recall_mean']):>9}"
                f"{fmt(q['recall_stdev']):>8}"
            )

    minutes, seconds = divmod(round(wall_seconds), 60)
    mean_s = (summary["mean_wall_ms"] or 0) / 1000
    print(
        f"\ntokens: {summary['tokens_in']:,} in / {summary['tokens_out']:,} out"
        f"    wall: {minutes}m{seconds:02d}s    mean {mean_s:.1f}s/question"
    )


def check_thresholds(summary: dict[str, Any], thresholds: dict[str, Any]) -> list[str]:
    missed = []
    recall = summary["overall"]["recall"]
    # None when the selection expects no citations; that is not a failure to cite.
    if recall is not None and recall < (floor := thresholds["overall_citation_recall"]):
        missed.append(f"overall citation recall {recall:.2f} < {floor}")
    refusal = summary["refusal_accuracy"]
    if refusal is not None and refusal < thresholds["refusal_accuracy"]:
        missed.append(f"refusal accuracy {refusal:.2f} < {thresholds['refusal_accuracy']}")
    violations = summary["overall"]["violations"]
    if violations > thresholds["max_violations"]:
        missed.append(f"violations {violations} > {thresholds['max_violations']}")
    if summary["errors"]:
        missed.append(f"{len(summary['errors'])} question(s) failed to get an answer")
    return missed


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--set", action="append", help="demo set; repeatable")
    p.add_argument("--category", action="append", help="category; repeatable")
    p.add_argument("--id", action="append", help="question id; repeatable")
    p.add_argument("--model", help="fail if the chat service answers with a different model")
    p.add_argument("--repeat", type=int, default=1)
    # The service caps turns at MAX_CONCURRENT_TURNS; more only collects 429s.
    p.add_argument("--concurrency", type=int, default=2)
    p.add_argument("--out", type=Path, help="results file (default: results/<timestamp>.json)")
    p.add_argument("--chat-url", default=os.getenv("EVAL_CHAT_URL", "http://localhost:8090"))
    p.add_argument("--backend-url", default=os.getenv("EVAL_BACKEND_URL", "http://localhost:8080"))
    p.add_argument(
        "--token",
        default=os.getenv("EVAL_API_TOKEN", "relviz-dev-token-not-for-production"),
    )
    return p.parse_args()


def _terminate(signum: int, frame: FrameType | None) -> None:
    raise KeyboardInterrupt


def main() -> int:
    args = parse_args()
    if args.repeat < 1 or args.concurrency < 1:
        raise SystemExit("--repeat and --concurrency must be at least 1")

    selected = select(yaml.safe_load(QUESTIONS.read_text()), args)
    if not selected:
        raise SystemExit("no questions match the selection")
    thresholds = yaml.safe_load(THRESHOLDS.read_text())

    # SIGTERM (make, timeout) takes the same cleanup path as ctrl-c.
    signal.signal(signal.SIGTERM, _terminate)

    started_at = datetime.now(UTC)
    stamp = started_at.strftime("%Y%m%dT%H%M%SZ")
    headers = {"Authorization": f"Bearer {args.token}"} if args.token else {}
    started = time.perf_counter()

    with httpx.Client(base_url=args.backend_url, headers=headers, timeout=60.0) as http:
        snapshots = Snapshots(http, label=f"eval-{stamp}-{os.getpid()}")
        try:
            for name in sorted({q["set"] for q in selected}):
                print(f"ingesting {name}", flush=True)
                snapshots.ingest(name)
            print(f"asking {len(selected)} question(s) x {args.repeat}", flush=True)
            results = asyncio.run(run_all(args, selected, snapshots.ids))
        finally:
            failed = snapshots.cleanup()
            if failed:
                print(f"warning: snapshots not deleted: {', '.join(failed)}", file=sys.stderr)

    wall = time.perf_counter() - started
    summary = summarise(results)
    report(summary, args.repeat, wall)

    out = args.out or RESULTS_DIR / f"{stamp}.json"
    out.parent.mkdir(parents=True, exist_ok=True)
    payload = {
        "startedAt": started_at.isoformat(),
        "wallSeconds": round(wall, 1),
        "args": {
            k: str(v) if isinstance(v, Path) else v for k, v in vars(args).items() if k != "token"
        },
        "thresholds": thresholds,
        "summary": summary,
        "results": [asdict(r) for r in results],
    }
    out.write_text(json.dumps(payload, indent=2, ensure_ascii=False))
    print(f"results: {out}")

    missed = check_thresholds(summary, thresholds)
    for m in missed:
        print(f"THRESHOLD MISSED: {m}", file=sys.stderr)
    return 1 if missed else 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print("\ninterrupted", file=sys.stderr)
        sys.exit(130)
