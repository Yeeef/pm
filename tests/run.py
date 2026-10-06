"""Run the pm tests in the package environment: uv run --project pm python pm/tests/run.py [pytest args]."""

import sys
from pathlib import Path

import pytest

sys.exit(pytest.main([str(Path(__file__).resolve().parent), "-q", "-p", "no:cacheprovider", *sys.argv[1:]]))
