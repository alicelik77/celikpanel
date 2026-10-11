#!/usr/bin/env python3
"""set11: in the evidence's own host-side files (tools/, host/, <platform>/<run>/host/; never a driver file, which its
SHA256SUMS covers), the local scratch-folder prefix becomes <scratchpad>, the repository checkout path <repo> and the
user's temporary folder <user-temp>; every file changed is listed in host/scrubbed-files.txt. usage: scrub.py EVIDENCE"""
import os, re, sys
E = sys.argv[1]
# Intake 2026-10-11: the Windows user name and the scratch-folder identifiers that stood in this file as run are
# replaced by <windows-user> and <session-scratch-dir>; the rules below are therefore illustrative, not runnable as is.
# As run, the single-backslash patterns also missed JSON-escaped (double-backslash) and file:/// forms and skipped
# browser/ folders; those occurrences were replaced by hand at intake (README "Corrections after intake").
SCRATCH = "AppData/Local/Temp/claude/<session-scratch-dir>/scratchpad"
RULES = [
    (re.compile(r"(?:/mnt/c|/c|C:)[/\\]Users[/\\]<windows-user>[/\\]" + SCRATCH.replace("/", r"[/\\]")), "<scratchpad>"),
    (re.compile(r"(?:/mnt/c|/c|C:)[/\\]<repo-parent-dir>[/\\]celikpanel"), "<repo>"),
    (re.compile(r"(?:/mnt/c|/c|C:)[/\\]Users[/\\]<windows-user>[/\\]AppData[/\\]Local[/\\]Temp"), "<user-temp>"),
]
changed = []
for directory, _, names in os.walk(E):
    rel = os.path.relpath(directory, E).replace(os.sep, "/")
    if "/driver" in "/" + rel or "/browser" in "/" + rel:
        continue
    if not (rel.startswith("tools") or rel.startswith("host") or rel.endswith("/host")):
        continue
    for name in names:
        path = os.path.join(directory, name)
        try:
            text = open(path, encoding="utf-8").read()
        except UnicodeDecodeError:
            continue
        new = text
        for pattern, replacement in RULES:
            new = pattern.sub(replacement, new)
        if new != text:
            with open(path, "w", encoding="utf-8", newline="") as stream:
                stream.write(new)
            changed.append(os.path.relpath(path, E).replace(os.sep, "/"))
with open(os.path.join(E, "host", "scrubbed-files.txt"), "w", encoding="utf-8", newline="\n") as stream:
    stream.write("# set11: files whose local scratch-folder prefix became <scratchpad>, repository path <repo>, user temp "
                 "folder <user-temp> (scrub.py); no driver file is among them\n" + "\n".join(sorted(changed)) + "\n")
print(len(changed), "files changed")
