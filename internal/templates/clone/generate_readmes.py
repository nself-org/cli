#!/usr/bin/env python3
"""Render app-clone Getting started commands from their manifests."""
import json
from pathlib import Path

for folder in sorted(Path(__file__).parent.glob("*-clone")):
    manifest = json.loads((folder / "manifest.json").read_text())
    path = folder / "README.md"
    prose = path.read_text().split("## Getting started", 1)[0]
    prose = "\n".join(
        line for line in prose.splitlines() if "Flutter starter" not in line
    ).rstrip()
    commands = "\n".join(manifest["next_steps"])
    path.write_text(
        prose + "\n\n## Getting started\n\n"
        "<!-- Generated from manifest.json by generate_readmes.py. -->\n\n"
        "```bash\n" + commands + "\n```\n"
    )
