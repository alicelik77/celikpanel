python3 -c '
import json; b=json.load(open("/var/tmp/a123r1/logs/preview-pgdg18-d12.json"))
print({k:v for k,v in b.items() if k not in ("installs","notable")})
for n in b.get("notable") or []: print(json.dumps(n))'
