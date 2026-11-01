source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab4/env.sh
head -12 /var/tmp/a114r4/singleR.out | cut -c1-300
b1 api a6-r main GET /api/v1/panel/update/check --label update-check --limit 3000 | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("status"), d.get("body"))'
bash $H/gq.sh a6-r main "tail -3 /etc/hosts; curl -sS https://celikpanel.net/releases/latest.txt; ls /etc/ca-certificates/trust-source/anchors/; grep -c BEGIN /etc/ssl/certs/ca-certificates.crt; journalctl -u celikpanel-panel -u celikpanel-agent --no-pager --since -10min | grep -iE 'update|release' | tail -8 | cut -c1-300"
