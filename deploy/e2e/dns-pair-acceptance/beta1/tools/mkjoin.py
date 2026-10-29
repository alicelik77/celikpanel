# Writes join request bodies from the saved code without printing it. usage: mkjoin.py CODEFILE OUTDIR
import json, sys
code = open(sys.argv[1]).read().strip()
plain = code.replace("-", "")
alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
last = plain[-1]
wrong = plain[:-1] + alphabet[(alphabet.index(last) + 1) % 32] if last in alphabet else plain[:-1] + "A"
fmt = lambda p: "-".join(p[i:i+4] for i in range(0, 24, 4))
out = sys.argv[2]
bodies = {
  "wrong-code": {"address": "https://192.0.3.10:2083", "code": fmt(wrong)},
  "unreachable": {"address": "https://192.0.3.99:2083", "code": code},
  "wrong-server": {"address": "https://192.0.3.1:2083", "code": code},
  "source-mismatch": {"address": "https://192.0.3.10:2083", "code": code, "public_ipv4": "192.0.3.12"},
  "correct": {"address": "https://192.0.3.10:2083", "code": code},
}
for name, body in bodies.items():
    with open(f"{out}/join-{name}.json", "w") as f:
        json.dump(body, f)
print("written", sorted(bodies), "code_shape", len(plain), code.count("-"))
