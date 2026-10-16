#!/usr/bin/env python3
"""set17: replace host paths and the operator's user and machine names with <redacted> in every text file of the
evidence directory (PNG files are not touched); print one line per changed file with the counts. The names come from
the environment (SCRUB_USER, SCRUB_MACHINE), so this file names neither.  usage: scrub.py DIR [--dry]"""
import os, re, sys
from pathlib import Path
user, machine = os.environ["SCRUB_USER"], os.environ["SCRUB_MACHINE"]
RULES = [
    ("windows-user-path", re.compile(r"[A-Za-z]:(?:\\|\|/)+Users(?:\\|\|/)+[^\s\"',;)\]}]*")),
    ("msys-user-path", re.compile(r"(?<![A-Za-z])/(?:mnt/)?c/Users/[^\s\"',;)\]}]*")),
    ("repo-path", re.compile(r"(?:[A-Za-z]:|/mnt/c|/c)(?:\\|\|/)+CELIKBROS PROJECTS[^\"',;)\]}\n]*")),
    ("user-name", re.compile(r"\b" + re.escape(user) + r"\b", re.I)),
    ("machine-name", re.compile(r"\b" + re.escape(machine) + r"\b", re.I)),
]
root = Path(sys.argv[1]); dry = "--dry" in sys.argv
total = 0
for path in sorted(root.rglob("*")):
    if not path.is_file() or path.suffix.lower() in (".png",):
        continue
    try:
        text = path.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        print(f"binary-skipped {path.relative_to(root)}"); continue
    counts = {}
    for name, rule in RULES:
        text, n = rule.subn("<redacted>", text)
        if n: counts[name] = n
    if counts:
        total += 1
        print(f"{'would-change' if dry else 'changed'} {path.relative_to(root)} {counts}")
        if not dry:
            path.write_text(text, encoding="utf-8", newline="")
print(f"files {'to change' if dry else 'changed'}: {total}")
