python3 - <<"P"
import json,re
d=json.load(open("/var/tmp/a130o/logs/install-d12v-main.log"))
print({k:(str(v)[:300]) for k,v in d.items() if k not in ("terminal","transcript")})
t=d.get("terminal") or d.get("transcript") or ""
print(str(t)[-1500:])
P
