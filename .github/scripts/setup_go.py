#!/usr/bin/env python3
"""Install the Go that go.mod names, for this runner, and put it first on PATH.

    setup_go.py

It downloads go.dev's archive of go.mod's `go` version for this OS and architecture, checks it against the sha256
go.dev lists for that file, unpacks it under $RUNNER_TEMP and appends its bin to $GITHUB_PATH; the step's output
`go-version` is the version. actions/setup-go does the same and then copies the unpacked tree into the runner's tool
cache, which takes 12 s of its 14-20 s on macos-14 and serves no later run: each job runs on a fresh runner.
Any mismatch fails the step.
"""
from __future__ import annotations

import hashlib
import json
import os
import platform
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def curl(url: str, *args: str) -> str:
    return subprocess.run(["curl", "-fsSL", "--retry", "3", "--max-time", "300", *args, url], capture_output=True,
                          text=True, check=True).stdout


def main() -> int:
    match = re.search(r"^go (\d+\.\d+\.\d+)$", (ROOT / "go.mod").read_text(), re.M)
    if not match:
        sys.exit("setup_go.py: go.mod names no go version of the form go X.Y.Z")
    version = match.group(1)
    goos = {"Darwin": "darwin", "Linux": "linux"}[platform.system()]
    goarch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64"}[platform.machine()]
    name = f"go{version}.{goos}-{goarch}.tar.gz"
    releases = json.loads(curl("https://go.dev/dl/?mode=json&include=all"))
    sums = [f["sha256"] for rel in releases for f in rel["files"] if f["filename"] == name]
    if len(sums) != 1:
        sys.exit(f"setup_go.py: go.dev lists {len(sums)} files named {name}")
    temp = Path(os.environ["RUNNER_TEMP"])
    archive = temp / name
    curl(f"https://dl.google.com/go/{name}", "-o", str(archive))
    digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    if digest != sums[0]:
        sys.exit(f"setup_go.py: {name} has sha256 {digest}, go.dev lists {sums[0]}")
    subprocess.run(["tar", "-xzf", str(archive), "-C", str(temp)], check=True)  # unpacks to $RUNNER_TEMP/go
    archive.unlink()
    goroot = temp / "go"
    got = subprocess.run([str(goroot / "bin" / "go"), "env", "GOVERSION"], capture_output=True, text=True,
                         check=True).stdout.strip()
    if got != f"go{version}":
        sys.exit(f"setup_go.py: the unpacked Go reports {got}, not go{version}")
    with open(os.environ["GITHUB_PATH"], "a") as f:
        f.write(f"{goroot / 'bin'}\n")
    with open(os.environ["GITHUB_OUTPUT"], "a") as f:
        f.write(f"go-version={version}\n")
    print(f"Go {version} at {goroot}, {name} sha256 {sums[0]}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
