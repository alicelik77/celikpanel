# alpha.125 extra reads on p5 main: EN/TR sentences in the served web bundle; the owner-value report in full; compression on secret-word paths with large answers. Lab only (read-only).
export CELL=p5 IP=192.0.3.190 Z=p5.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C5extra-$CELL.log) 2>&1
echo "== EN/TR sentences in the served web bundle $(ts)"
gshn main 'W=$(dirname "$(find /opt/celikpanel /usr/share/celikpanel /var/lib/celikpanel -name "app-*.js" -path "*assets*" 2>/dev/null | head -1)"); echo "web assets $W"; for s in "is a static site: it serves files" "statik bir site: dosyaları olduğu gibi" "already exists, so nothing was changed" "posta kutusu zaten var" "the update waits for it (up to a minute)" "güncelleme onu bekler (en çok bir dakika)"; do printf "  %-48s %s file(s)\n" "$s" "$(grep -rlF "$s" "$W" 2>/dev/null | wc -l)"; done'
echo "== the Postfix page report after the owner set their own values $(ts)"
adm GET /api/v1/mail/passwordless --label passwordless | python3 -c 'import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); print(d["status"], json.dumps(b.get("enforcement"), indent=1)); print("state", b.get("state"))'
echo "== compression on secret-word paths with answers over 1 KiB $(ts)"
S=/var/tmp/cp-beta1/state/p5/main.json
read IP2 PORT CK < <(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d["ip"], d.get("panel_port") or 1923, d["cookie"])' $S)
for p in /api/v1/system-databases /api/v1/users /api/v1/subscriptions /api/v1/dkim/keys /api/v1/domains/1/mail/accounts; do
  curl -sk -m 60 -o /tmp/b -D /tmp/h -H "Accept-Encoding: gzip" -H "Cookie: celikpanel_session=$CK" https://$IP2:$PORT$p
  printf '  GET %-40s %s | %s | wire %s B\n' "$p" "$(head -1 /tmp/h | tr -d '\r')" "$(grep -i '^content-encoding:' /tmp/h | tr -d '\r' || echo 'content-encoding: -')" "$(stat -c %s /tmp/b)"
done; rm -f /tmp/b /tmp/h
