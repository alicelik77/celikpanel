# usage: insum.py LOG NAME  -> one-line summary of a getsh_install.py run
import json, sys
t = open(sys.argv[1]).read(); i = t.find("{")
try:
    d = json.loads(t[i:])
except Exception:
    print(sys.argv[2], "INSTALL OUTPUT NOT JSON:", t[-1500:]); sys.exit()
x = d.get("terminal", {})
print(sys.argv[2], "terminal rc", x.get("rc"), x.get("seconds"), "problem", x.get("problem"), "restart_required", x.get("restart_required"),
      "postinstall rc", d.get("postinstall", {}).get("rc"))
print("  tail:", x.get("tail", "")[-600:].replace("\n", " | "))
