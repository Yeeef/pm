"""The list of tests expected to fail on Go pm names only tests that exist, so a renamed or removed test leaves it."""

from __future__ import annotations

import ast
from pathlib import Path

from conftest import GO_EXPECTED_FAILURES, go_expected_failures


def test_every_go_expected_failure_names_a_test_function_of_its_file():
    here = GO_EXPECTED_FAILURES.parent
    missing = []
    for entry in sorted(go_expected_failures()):
        file, sep, test = entry.partition("::")
        func = test.split("[", 1)[0]
        path = here / file
        names = ({n.name for n in ast.walk(ast.parse(path.read_text())) if isinstance(n, ast.FunctionDef)}
                 if sep and path.is_file() else set())
        if func not in names:
            missing.append(entry)
    assert not missing, f"{GO_EXPECTED_FAILURES.name} names tests that do not exist; remove them: {missing}"
