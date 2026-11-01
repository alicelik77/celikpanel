#!/bin/bash
# mksite.sh CELL NODE SITE NS-ZONE IP1 IP2 : a static site with mailboxes alice@ and bob@ (owner API). Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/env.sh
source $H/common.sh
CELL=$1; N=$2; S=$3; Z=$4; I1=$5; I2=$6; T=/var/tmp/a114r3
infra delegate $S ns1.$Z $I1 ns2.$Z $I2 > /dev/null
b1 api $CELL $N POST /api/v1/domains/create "{\"domain\":\"$S\",\"project_type\":\"static\",\"ssl_type\":\"none\"}" --label site-create --limit 4000 | brief
SID=$(domain_id $S $N); echo "   site id $SID"
python3 - $CELL <<'PY'
import json, secrets, sys, pathlib
p = pathlib.Path(f"/var/tmp/cp-beta1/state/{sys.argv[1]}")
for who in ("alice", "bob"):
    f = p / f"mbx-{who}.json"
    if not f.exists():
        f.write_text(json.dumps({"password": secrets.token_urlsafe(18)})); f.chmod(0o600)
PY
for who in alice bob; do
  python3 -c 'import json,sys; print(json.dumps({"address": sys.argv[2]+"@"+sys.argv[3], "password": json.load(open(sys.argv[1]))["password"], "quota_mb": 1024}))' /var/tmp/cp-beta1/state/$CELL/mbx-$who.json $who $S > $T/mbx-$CELL.json
  b1 api $CELL $N POST /api/v1/domains/$SID/mail/accounts @$T/mbx-$CELL.json --label mailbox-$who --limit 2000 | brief
  rm -f $T/mbx-$CELL.json
done
