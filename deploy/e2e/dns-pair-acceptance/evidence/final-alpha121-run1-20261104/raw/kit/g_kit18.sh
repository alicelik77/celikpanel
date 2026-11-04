# guest (root): LAB KIT INSTRUMENTATION for item 18 only (prints what the checks see; changes no verdict). Lab only.
cd /root/kit/deploy/e2e/sandbox-escape
python3 - <<'P'
t=open("run.sh").read()
t=t.replace('	findmnt -rn -o SOURCE,TARGET | grep -q "990002" && mark 18 FAIL "the other site is mounted somewhere"',
'	echo "ITEM18 bind unit state: $(systemctl is-active celikpanel-sandbox@${UNIT_GO%.service}.service) / unit $(systemctl is-active $UNIT_GO)"\n	findmnt -rn -o SOURCE,TARGET,FSTYPE,PROPAGATION | grep -E "99000|celikpanel-sandbox" | sed "s/^/ITEM18 mount: /"\n	journalctl -u "celikpanel-sandbox@${UNIT_GO%.service}.service" -o short-precise --since "@$STARTED" --no-pager | tail -8 | sed "s/^/ITEM18 bind journal: /"\n	findmnt -rn -o SOURCE,TARGET | grep -q "990002" && mark 18 FAIL "the other site is mounted somewhere"',1)
t=t.replace('	[ -e "/run/celikpanel-sandbox/$UNIT_GO" ] && mark 18 FAIL',
'	sleep 2; ls -laR "/run/celikpanel-sandbox/$UNIT_GO" 2>&1 | head -8 | sed "s/^/ITEM18 after stop: /"; findmnt -rn -o SOURCE,TARGET | grep -E "celikpanel-sandbox" | sed "s/^/ITEM18 mount after stop: /"; systemctl is-active celikpanel-sandbox@${UNIT_GO%.service}.service | sed "s/^/ITEM18 bind unit after stop: /"\n	[ -e "/run/celikpanel-sandbox/$UNIT_GO" ] && mark 18 FAIL',1)
open("run.sh","w").write(t)
P
grep -c ITEM18 run.sh
./run.sh quick 2>&1 | grep -E "ITEM18|^item 18|^item 9|^(PASS|FAIL)$"
