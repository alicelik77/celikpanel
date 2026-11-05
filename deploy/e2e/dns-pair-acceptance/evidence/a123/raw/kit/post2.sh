# after wave 2a: u22r and d13 BIND/PDNS identity, host facts, systemd features, sandbox probe. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit/lib.sh
source $A/probe.sh
for c in ${CELLS:-u22r d13}; do CELL=$c; T=$B/t-$c; mkdir -p $T
  ( echo "==== $c $(ts)"; GQT=60 gq g_bindid3.sh; GQT=60 gq g_facts.sh; GQT=60 gq g_sdfeat.sh; probe
    adm GET /api/v1/setup/components --label comp > $T/c.json; python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]); print("  setup dns_engines", json.dumps(b.get("dns_engines"))[:900])' $T/c.json
  ) 2>&1 | tee $LOGS/post-$c.log
done
