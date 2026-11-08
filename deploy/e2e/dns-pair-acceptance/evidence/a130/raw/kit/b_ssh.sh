# B lane item 4: SSH password sign-in on Ubuntu 24.04 (cloud-init-like drop-ins). usage: b_ssh.sh PART... Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/b_lib.sh
exec > >(tee -a $LOGS/B4ssh-$CELL.log) 2>&1
P=/api/v1/security/ssh-password-login
status() { admn $N GET $P --label ssh-status > $T/ssh.json; python3 -c '
import json,sys; d=json.load(open(sys.argv[1])); b=json.loads(d["body"]) if d["body"].startswith("{") else {}
print("  status", d["status"], json.dumps({k:b.get(k) for k in ("state","reason","detail","unit","unit_active","password_authentication","kbd_interactive_authentication","can_turn_off","can_turn_on")}, ensure_ascii=False)[:600])
print("  deciders", json.dumps(b.get("deciders"))[:500]); print("  drop_in", json.dumps(b.get("drop_in"))); print("  key_accounts", json.dumps(b.get("key_accounts")))' $T/ssh.json; }
sha() { python3 -c 'import json,sys; b=json.loads(json.load(open(sys.argv[1]))["body"]); print((b.get("drop_in") or {}).get("sha256",""))' $T/ssh.json; }
change() { status >/dev/null; admn $N POST $P "{\"action\":\"$1\",\"expected_drop_in_sha256\":\"$(sha)\"}" --label ssh-$1 --timeout 120 | sbrief 900; }
pwtry() { nup n_pwssh.sh >/dev/null; pw labpw | timeout 60 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_pwssh.sh $BIP labpw"; }
keytry() { timeout 30 python3 -I $L/lab.py ssh $CELL $N -- "echo KEY-LOGIN-OK as \$(id -un)"; }
for part in "$@"; do case $part in
prep) echo "== prep $(ts)"; grun $N g_ssh.sh prep; mkcred labpw; pw labpw | timeout 60 python3 -I $L/lab.py ssh $CELL $N -- "sudo -n tee /root/g_ssh.sh >/dev/null" < /dev/null; gput g_ssh.sh $N; pw labpw | timeout 60 python3 -I $L/lab.py ssh $CELL $N -- "sudo -n bash /root/g_ssh.sh setpw"; grun $N g_ssh.sh read ;;
status) echo "== status $(ts)"; status ;;
pw) echo "== password login from labnet $(ts)"; pwtry ;;
key) echo "== key login $(ts)"; keytry ;;
off) echo "== turn off $(ts)"; change turn_off; grun $N g_ssh.sh read; status ;;
on) echo "== turn on $(ts)"; change turn_on; grun $N g_ssh.sh read; status ;;
ownedit) echo "== owner edits 00-celikpanel.conf $(ts)"; grun $N g_ssh.sh ownedit; status; change turn_on; change turn_off; grun $N g_ssh.sh ownrevert ;;
peerhide) echo "== lab: the peer inspector Include hidden (stand-in for the fix) $(ts)"; grun $N g_sshpeer.sh hide; status ;;
peerback) echo "== lab: the peer inspector Include back $(ts)"; grun $N g_sshpeer.sh restore; status ;;
chgrp) echo "== lab: the drop-in to root:root (stand-in for the writer fix) $(ts)"; grun $N g_ssh.sh chgrp; status ;;
restore) echo "== restore the VM's sshd config $(ts)"; grun $N g_ssh.sh restore ;;
esac; done
echo "== b_ssh $* done $(ts)"
