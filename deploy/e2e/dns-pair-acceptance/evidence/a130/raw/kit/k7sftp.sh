# case 7: extra SFTP/FTP logins of site1. usage: k7sftp.sh state | add LOGIN | vsftpd | pw yes|no|clear. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/hdr.sh
exec > >(tee -a $LOGS/C7sftp-$S.log) 2>&1
nup n_sftp.sh >/dev/null
lst() { admn main GET /api/v1/domains/$S1/access/logins --label logins | python3 -c '
import json,sys; d=json.load(sys.stdin); b=d.get("body") or ""
try: j=json.loads(b)
except Exception: print(d["status"], b[:300]); sys.exit()
st=j.get("state") or {}
print(d["status"], "warnings", json.dumps(j.get("warnings") or st.get("warnings")), "| keys", sorted(j.keys())[:20])
print("   ", json.dumps({k:v for k,v in j.items() if k not in ("logins",)})[:900])
for l in (j.get("logins") or st.get("logins") or []): print("    login", json.dumps(l)[:300])'; }
case $1 in
state)
  echo "== 7.state $(ts)"
  gshn main 'sshd -T 2>/dev/null | grep -iE "^(passwordauthentication|kbdinteractiveauthentication|usepam|authenticationmethods) " | tr "\n" " "; echo; grep -rhE "^\s*(PasswordAuthentication|KbdInteractiveAuthentication)" /etc/ssh/sshd_config /etc/ssh/sshd_config.d/*.conf 2>/dev/null; command -v vsftpd || echo "vsftpd: not installed"'
  lst ;;
pw)
  echo "== 7.pw $2 (lab drop-in /etc/ssh/sshd_config.d/01-lab-password.conf) $(ts)"
  if [ "$2" = clear ]; then gshn main 'rm -f /etc/ssh/sshd_config.d/01-lab-password.conf; systemctl reload ssh 2>/dev/null || systemctl reload sshd; sshd -T | grep -i "^passwordauthentication"'
  else gshn main "echo \"PasswordAuthentication $2\" > /etc/ssh/sshd_config.d/01-lab-password.conf; systemctl reload ssh 2>/dev/null || systemctl reload sshd; sshd -T | grep -i '^passwordauthentication'"; fi
  lst ;;
vsftpd)
  echo "== 7.vsftpd from Components $(ts)"
  admn main GET /api/v1/managed-services --label ms | python3 -c '
import json,sys; d=json.load(sys.stdin); b=json.loads(d["body"]); rows=b if isinstance(b,list) else (b.get("services") or b.get("items") or [])
[print("  row", json.dumps({k:r.get(k) for k in ("id","name","is_installed","status","package")})[:300]) for r in rows if "ftp" in str(r.get("id")).lower()]'
  installc main ${2:-vsftpd}
  lst ;;
add)
  LOGIN=$2; echo "== 7.add $LOGIN $(ts)"
  mkcred $LOGIN
  admn main POST /api/v1/domains/$S1/access/logins "{\"login\":\"$LOGIN\",\"folder\":\"/public_html\",\"password\":\"$(pw $LOGIN)\"}" --label login-add --timeout 300 | sbrief 900
  gshn main "id $LOGIN; getent passwd $LOGIN | cut -d: -f3-7; systemctl is-active celikpanel-ftp-$LOGIN.service; findmnt -n /run/celikpanel-ftp/$LOGIN/files | cut -c1-160"
  pw $LOGIN | timeout 200 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_sftp.sh $IP $LOGIN"
  gshn main "R=\$(ls -d /var/www/celikpanel/subscriptions/*/sites/$S1/public_html | head -1); ls -ln \$R/lab-up.txt \$R/lab-ftps.txt 2>&1; stat -c %u \$R; rm -f \$R/lab-up.txt \$R/lab-ftps.txt"
  admn main DELETE /api/v1/domains/$S1/access/logins/$LOGIN --label login-del --timeout 300 | sbrief 400
  gshn main "id $LOGIN 2>&1 | tail -1; systemctl is-active celikpanel-ftp-$LOGIN.service 2>&1; grep -c $LOGIN /etc/ssh/sshd_config.d/60-celikpanel-file-access.conf 2>/dev/null; grep -rc $LOGIN /etc/vsftpd* 2>/dev/null | head -3"
  echo "-- after removal:"; pw $LOGIN | timeout 120 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_sftp.sh $IP $LOGIN" | head -8
  ;;
esac
echo "== k7sftp $S $1 done $(ts)"
