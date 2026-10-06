# /// script
# requires-python = ">=3.12"
# dependencies = ["pytest>=8", "markdown-it-py>=3", "mdit-py-plugins>=0.4", "pyyaml>=6"]
# ///
"""Run the harness tests: uv run tests/run.py [pytest args]."""

import sys
from pathlib import Path

import pytest

sys.exit(pytest.main([str(Path(__file__).resolve().parent), "-q", "-p", "no:cacheprovider", *sys.argv[1:]]))
