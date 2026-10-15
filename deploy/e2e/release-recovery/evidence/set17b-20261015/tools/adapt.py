import re, sys
from pathlib import Path
d = Path(sys.argv[1])
J = "<redacted>"
for p in sorted(d.iterdir()):
    if p.suffix not in (".sh", ".py", ".ps1") or p.name in ("adapt.py", "mk-tool.py"):
        continue
    t = p.read_text(encoding="utf-8")
    o = t
    t = t.replace("source <redacted>", f"source {J}/env.sh")
    t = re.sub(r"/var/tmp/cp-set17(?!b)", "/var/tmp/cp-set17b", t)
    t = re.sub(r"^# set17(?!b)", "# set17b", t, flags=re.M)
    t = t.replace("Set17.Power", "Set17b.Power").replace("-Namespace Set17 ", "-Namespace Set17b ")
    if p.name == "env.sh":
        t = re.sub(r"^J=.*$", f"J={J}", t, flags=re.M)
        t = re.sub(r"^C=.*$", "C=ecd2a5f088d676f7cc8a8e571ed3c903ea951f3e", t, flags=re.M)
    t = t.replace("\r\n", "\n")
    if t != o:
        p.write_text(t, encoding="utf-8", newline="\n")
        print("adapted", p.name)
