python3 -c "import json; d=json.load(open(\"/var/tmp/cp-beta1/evidence/aru/0152-main-post-domains-4-copy.json\")); print({k:str(v)[:400] for k,v in d.items()})"
