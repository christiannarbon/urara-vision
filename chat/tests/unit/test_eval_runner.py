"""The eval runner's scoring and threshold gate, without a model."""

import sys
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "eval"))

import run_eval  # noqa: E402

THRESHOLDS = {"overall_citation_recall": 0.93, "refusal_accuracy": 1.0, "max_violations": 0}


def question(**over: Any) -> dict[str, Any]:
    base: dict[str, Any] = {
        "id": "q1",
        "category": "lookup",
        "expect_citations": [],
        "expect_tools_any": [],
        "expect_contains_any": [],
        "must_not_contain": [],
    }
    return base | over


def scored(q: dict[str, Any], citations: list[str], text: str = "answer") -> run_eval.Result:
    result = run_eval.Result(q["id"], "set", q["category"], "EN", "question?", 1, answer=text)
    result.citations = citations
    result.scores = run_eval.score(q, citations, [], text)
    return result


class TestScore:
    def test_no_expected_citations_means_recall_is_not_scored(self) -> None:
        assert run_eval.score(question(), ["a/b"], [], "x").recall is None

    def test_a_refusal_may_cite_an_allowed_table(self) -> None:
        q = question(category="refusal", allow_citations=["a/b"])
        assert run_eval.score(q, ["a/b"], [], "x").refusal_ok is True

    def test_a_refusal_citing_anything_else_fails(self) -> None:
        q = question(category="refusal", allow_citations=["a/b"])
        assert run_eval.score(q, ["a/c"], [], "x").refusal_ok is False


class TestThresholds:
    def test_a_selection_with_no_citation_expectations_passes(self) -> None:
        results = [scored(question(category="refusal"), [])]
        summary = run_eval.summarise(results)

        assert summary["overall"]["recall"] is None
        assert run_eval.check_thresholds(summary, THRESHOLDS) == []

    def test_low_recall_still_fails(self) -> None:
        results = [scored(question(expect_citations=["a/b", "a/c"]), ["a/b"])]
        missed = run_eval.check_thresholds(run_eval.summarise(results), THRESHOLDS)
        assert missed == ["overall citation recall 0.50 < 0.93"]
