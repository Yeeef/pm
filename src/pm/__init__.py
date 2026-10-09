"""pm: the project records CLI (cli.py) and site renderer (render.py), with the records, Beads and site code they share."""

from importlib.metadata import version
from pathlib import Path

__version__ = version("pm")  # the one source is pm/pyproject.toml


def prompt(name: str) -> str:
    """prompts/<name>.txt without its final newline: a model prompt or hook text that Go pm embeds too (assets.go)."""
    return (Path(__file__).resolve().parent / "prompts" / f"{name}.txt").read_text(encoding="utf-8").removesuffix("\n")
