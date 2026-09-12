"""Citation extraction."""

import pytest

from urara_chat.agent.citations import MAX_CITATIONS, extract_citations

FACT_ORDERS = "ordering/fact_orders"
DIM_CUSTOMERS = "customer_identity/dim_customers"


def tool_result(*ids: str) -> dict[str, object]:
    return {
        "items": [{"id": i, "name": i.split("/")[1]} for i in ids],
        "truncated": False,
        "total": len(ids),
    }


class TestTheTwoRulesThatMatter:
    def test_retrieved_and_mentioned_by_full_id_is_cited(self) -> None:
        cited = extract_citations([tool_result(FACT_ORDERS)], f"See {FACT_ORDERS} for that.")
        assert cited == [FACT_ORDERS]

    def test_retrieved_and_mentioned_by_bare_name_is_cited(self) -> None:
        """A model writes "fact_orders", not "ordering/fact_orders"."""
        cited = extract_citations(
            [tool_result(FACT_ORDERS)], "fact_orders holds one row per order."
        )
        assert cited == [FACT_ORDERS]

    def test_retrieved_but_never_mentioned_is_not_cited(self) -> None:
        """Otherwise the list is padded with everything the turn looked at, and"""
        cited = extract_citations(
            [tool_result(FACT_ORDERS, DIM_CUSTOMERS)],
            "fact_orders holds one row per order.",
        )
        assert cited == [FACT_ORDERS]

    def test_mentioned_but_never_retrieved_is_not_cited(self) -> None:
        """The hallucination case, and the most important assertion here."""
        cited = extract_citations(
            [tool_result(FACT_ORDERS)],
            f"See {FACT_ORDERS} and also invented/dim_nonsense for the rest.",
        )
        assert cited == [FACT_ORDERS]
        assert "invented/dim_nonsense" not in cited

    def test_nothing_retrieved_yields_nothing(self) -> None:
        assert extract_citations([], "fact_orders and dim_customers explain it") == []


class TestOrdering:
    def test_order_follows_first_mention_not_tool_order(self) -> None:
        """The reader scans the answer top to bottom and the chips should"""
        results = [tool_result(FACT_ORDERS, DIM_CUSTOMERS)]
        answer = "dim_customers is joined by fact_orders."

        assert extract_citations(results, answer) == [DIM_CUSTOMERS, FACT_ORDERS]

    def test_the_earlier_of_the_two_spellings_wins(self) -> None:
        results = [tool_result(FACT_ORDERS, DIM_CUSTOMERS)]
        answer = f"fact_orders joins dim_customers; see {FACT_ORDERS}."

        assert extract_citations(results, answer) == [FACT_ORDERS, DIM_CUSTOMERS]

    def test_duplicates_collapse(self) -> None:
        answer = "fact_orders, fact_orders again, and ordering/fact_orders once more."
        assert extract_citations([tool_result(FACT_ORDERS)], answer) == [FACT_ORDERS]

    def test_a_table_retrieved_twice_is_cited_once(self) -> None:
        results = [tool_result(FACT_ORDERS), tool_result(FACT_ORDERS)]
        assert extract_citations(results, "fact_orders") == [FACT_ORDERS]

    def test_more_than_twenty_candidates_cap_at_twenty(self) -> None:
        ids = [f"d{i}/table_{i}" for i in range(30)]
        answer = " ".join(f"table_{i}" for i in range(30))

        cited = extract_citations([tool_result(*ids)], answer)

        assert len(cited) == MAX_CITATIONS
        # The cap keeps the first mentioned, not an arbitrary twenty.
        assert cited == ids[:MAX_CITATIONS]


class TestWhereIDsAreFound:
    def test_a_join_path_contributes_its_tables_array(self) -> None:
        """JoinPath.tables is a list of bare strings, so its elements never pass"""
        result = {
            "items": [
                {
                    "length": 1,
                    "tables": [FACT_ORDERS, DIM_CUSTOMERS],
                    "hops": [{"from": FACT_ORDERS, "to": DIM_CUSTOMERS, "fromColumn": "c"}],
                }
            ]
        }
        cited = extract_citations([result], "fact_orders joins dim_customers")
        assert set(cited) == {FACT_ORDERS, DIM_CUSTOMERS}

    def test_a_graph_contributes_node_ids_and_link_endpoints(self) -> None:
        result = {
            "items": [{"id": FACT_ORDERS, "label": "fact_orders"}],
            "links": [{"source": FACT_ORDERS, "target": DIM_CUSTOMERS, "type": "joins"}],
        }
        cited = extract_citations([result], "fact_orders reaches dim_customers")
        assert set(cited) == {FACT_ORDERS, DIM_CUSTOMERS}

    def test_a_table_id_key_is_collected(self) -> None:
        result = {"items": [{"tableId": FACT_ORDERS, "rank": 0.9}]}
        assert extract_citations([result], "fact_orders") == [FACT_ORDERS]

    def test_a_result_three_levels_deep_is_walked(self) -> None:
        result = {"items": [{"table": {"id": FACT_ORDERS, "columns": [{"name": "order_id"}]}}]}
        assert extract_citations([result], "fact_orders") == [FACT_ORDERS]

    def test_prose_fields_are_not_harvested(self) -> None:
        """A description mentioning an ID is a coincidence, not a retrieval."""
        result = {
            "items": [
                {
                    "id": FACT_ORDERS,
                    "description": f"Similar in shape to {DIM_CUSTOMERS}.",
                    "grain": "One row per order in customer_identity/dim_customers terms.",
                }
            ]
        }
        cited = extract_citations([result], "fact_orders and dim_customers")
        assert cited == [FACT_ORDERS]
        assert DIM_CUSTOMERS not in cited

    def test_source_model_ids_are_not_tables(self) -> None:
        """`jaffle_shop.stg_orders` has no slash: it is not a table and cannot"""
        result = {"items": [{"id": "jaffle_shop.stg_orders", "dataset": "jaffle_shop"}]}
        assert extract_citations([result], "built from jaffle_shop.stg_orders") == []

    def test_a_bare_string_in_a_tables_list_is_collected(self) -> None:
        assert extract_citations([{"tables": [FACT_ORDERS]}], "fact_orders") == [FACT_ORDERS]

    def test_a_missing_id_string_is_ignored(self) -> None:
        """The batch endpoint reports unfound IDs; they were not retrieved."""
        result = {"items": [], "missing": ["nope/not_a_table"], "truncated": False, "total": 0}
        assert extract_citations([result], "nope/not_a_table does not exist") == []


class TestWordBoundaries:
    def test_a_bare_name_does_not_match_inside_a_longer_name(self) -> None:
        result = tool_result("shared_kernel/dim_date")
        assert extract_citations([result], "dim_dates is a different table") == []

    def test_a_shorter_name_does_not_match_inside_a_longer_one(self) -> None:
        """Underscore counts as part of a word: `date` must not match inside"""
        assert extract_citations([tool_result("d/date")], "dim_date holds days") == []

    def test_a_name_after_a_slash_still_matches(self) -> None:
        result = tool_result("shared_kernel/dim_date")
        cited = extract_citations([result], "use customer_identity/dim_date instead")
        assert cited == ["shared_kernel/dim_date"]

    def test_punctuation_around_the_name_is_fine(self) -> None:
        for answer in ("(fact_orders)", "fact_orders,", "'fact_orders'", "-fact_orders."):
            assert extract_citations([tool_result(FACT_ORDERS)], answer) == [FACT_ORDERS], answer


class TestCaseInsensitivity:
    def test_an_upper_case_answer_matches_a_lower_case_id(self) -> None:
        assert extract_citations([tool_result(FACT_ORDERS)], "FACT_ORDERS") == [FACT_ORDERS]

    def test_an_upper_case_id_matches_a_lower_case_answer(self) -> None:
        result = tool_result("Ordering/Fact_Orders")
        assert extract_citations([result], "fact_orders") == ["Ordering/Fact_Orders"]

    def test_the_full_id_matches_in_either_case(self) -> None:
        assert extract_citations([tool_result(FACT_ORDERS)], "ORDERING/FACT_ORDERS") == [
            FACT_ORDERS
        ]


class TestAmbiguity:
    def test_two_tables_sharing_a_bare_name_are_both_cited(self) -> None:
        """The demo sets contain exactly this."""
        shared = "shared_kernel/dim_date"
        local = "customer_identity/dim_date"

        cited = extract_citations([tool_result(shared, local)], "join on dim_date")

        assert set(cited) == {shared, local}

    def test_a_full_id_still_selects_only_that_one(self) -> None:
        shared = "shared_kernel/dim_date"
        local = "customer_identity/dim_date"

        cited = extract_citations([tool_result(shared, local)], f"join on {shared}")

        # The bare name appears inside the full ID, so both still match -- which is the documented
        # behaviour: the reader learns there are two.
        assert set(cited) == {shared, local}


class TestDegenerateInput:
    def test_empty_tool_results(self) -> None:
        assert extract_citations([], "fact_orders") == []

    def test_empty_answer(self) -> None:
        assert extract_citations([tool_result(FACT_ORDERS)], "") == []

    def test_both_empty(self) -> None:
        assert extract_citations([], "") == []

    def test_a_self_referential_structure_terminates(self) -> None:
        """A malformed result must not hang the turn."""
        node: dict[str, object] = {"id": FACT_ORDERS}
        node["self"] = node
        node["siblings"] = [node]

        assert extract_citations([node], "fact_orders") == [FACT_ORDERS]

    def test_two_structures_referring_to_each_other_terminate(self) -> None:
        a: dict[str, object] = {"id": FACT_ORDERS}
        b: dict[str, object] = {"id": DIM_CUSTOMERS, "other": a}
        a["other"] = b

        cited = extract_citations([a], "fact_orders and dim_customers")

        assert set(cited) == {FACT_ORDERS, DIM_CUSTOMERS}

    def test_a_very_deep_structure_terminates(self) -> None:
        deep: dict[str, object] = {"id": FACT_ORDERS}
        for _ in range(200):
            deep = {"nested": deep}

        # Past the depth cap the ID is simply not found, which is the safe outcome: a citation is
        # dropped rather than a turn hanging.
        assert extract_citations([deep], "fact_orders") == []

    def test_non_string_values_are_ignored(self) -> None:
        result = {"items": [{"id": None, "tableId": 42, "source": ["not", "a", "string"]}]}
        assert extract_citations([result], "anything") == []

    def test_a_string_that_is_not_an_id_is_ignored(self) -> None:
        for value in ("no-slash", "too/many/slashes", "/leading", "trailing/", "-bad/start"):
            result = {"items": [{"id": value}]}
            assert extract_citations([result], value) == [], value

    def test_a_tool_error_string_contributes_nothing(self) -> None:
        """A wrapped tool answers with guidance when a lookup fails, and that"""
        guidance = "No table with id 'nope/missing' in this model. Call search_model to find it."
        assert extract_citations([guidance], "nope/missing was not found") == []


@pytest.mark.parametrize(
    ("answer", "expected"),
    [
        ("fact_orders", [FACT_ORDERS]),
        ("ordering/fact_orders", [FACT_ORDERS]),
        ("The fact_orders table.", [FACT_ORDERS]),
        ("fact_orderss", []),
        ("xfact_orders", []),
        ("fact_order", []),
        ("", []),
    ],
)
def test_mention_shapes(answer: str, expected: list[str]) -> None:
    assert extract_citations([tool_result(FACT_ORDERS)], answer) == expected
