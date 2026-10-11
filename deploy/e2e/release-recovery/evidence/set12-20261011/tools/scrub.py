#!/usr/bin/env python3
"""set12: in the evidence's own files outside the drivers' run directories (`*/*/driver/`, covered by their SHA256SUMS)
and outside the run copies' harness diffs (source text), the local scratch-folder prefix becomes <scratchpad>, the
repository checkout path <repo>, the user's temporary folder <user-temp>, the Windows user name <windows-user>, the
session scratch identifiers <session-scratch-dir> and the checkout's parent folder <repo-parent-dir>; plain,
JSON-escaped (doubled backslashes) and file:/// forms. Every file changed is listed in host/scrubbed-files.txt.
The constants below are written in their scrubbed form after the run (the script is illustrative as kept).
usage: scrub.py EVIDENCE"""
import os, re, sys
E = sys.argv[1]
USER = os.environ.get("SCRUB_USER", "<windows-user>")
SESSION = os.environ.get("SCRUB_SESSION", "<session-scratch-dir>")
PARENT = os.environ.get("SCRUB_PARENT", "<repo-parent-dir>")
SEP = r"(?:/|\\{1,2})"
RULES = [
    (re.compile(r"(?:/mnt/c|/c|C:)" + SEP + "Users" + SEP + re.escape(USER) + SEP + "AppData" + SEP + "Local" + SEP + "Temp" + SEP
                + "claude" + SEP + "c--" + re.escape(PARENT.replace(" ", "-")) + "-celikpanel" + SEP + re.escape(SESSION) + SEP + "scratchpad", re.I), "<scratchpad>"),
    (re.compile(r"(?:/mnt/c|/c|C:)" + SEP + "Users" + SEP + re.escape(USER) + SEP + "AppData" + SEP + "Local" + SEP + "Temp", re.I), "<user-temp>"),
    (re.compile(r"file:///C:/" + re.escape(PARENT.replace(" ", "%20")) + "/celikpanel", re.I), "file:///<repo>"),
    (re.compile(r"(?:/mnt/c|/c|C:)" + SEP + re.escape(PARENT) + SEP + "celikpanel", re.I), "<repo>"),
    (re.compile(re.escape(SESSION)), "<session-scratch-dir>"),
    (re.compile("c--" + re.escape(PARENT.replace(" ", "-")) + "-celikpanel"), "<session-scratch-dir>"),
    (re.compile(re.escape(PARENT)), "<repo-parent-dir>"),
    (re.compile(r"(?<![A-Za-z])" + re.escape(USER) + r"(?![A-Za-z])", re.I), "<windows-user>"),
]
changed = []
for directory, dirs, names in os.walk(E):
    rel = os.path.relpath(directory, E).replace(os.sep, "/")
    if "/driver" in "/" + rel or rel.startswith("harness-run-copy"):
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
os.makedirs(os.path.join(E, "host"), exist_ok=True)
with open(os.path.join(E, "host", "scrubbed-files.txt"), "w", encoding="utf-8", newline="\n") as stream:
    stream.write("# set12: files whose local paths and names became <scratchpad>, <repo>, <user-temp>, <windows-user>, "
                 "<session-scratch-dir>, <repo-parent-dir> (tools/scrub.py); no driver file is among them\n" + "\n".join(sorted(changed)) + "\n")
print(len(changed), "files changed")
