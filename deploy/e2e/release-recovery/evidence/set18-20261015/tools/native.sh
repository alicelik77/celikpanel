#!/bin/bash
# set18 (run note H1): a read-only native reading of lab s18-b over the lab's own SSH key (the driver's API reads fail
# after the certificate handover). usage: native.sh LABEL
lab=/var/tmp/cp-release-drill-s18-b
kh=$(ls $lab/cells/*/ssh-known-hosts | head -1)
out=/var/tmp/cp-set18/native-$1.txt
[ -e $out ] && { echo "refusing: $out exists"; exit 2; }
{ echo "# set18 read-only native reading ($1) at $(date -u +%FT%TZ)"
ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=yes -o HashKnownHosts=no -o UserKnownHostsFile=$kh -i $lab/key -p 5319 celik@127.0.0.1 'sudo /bin/bash -s' <<'R'
set +e
echo "== hostname"; hostnamectl --static 2>&1; hostname -f 2>&1
echo "== panel TLS (127.0.0.1:2083): SNI panel.set18.test, then no SNI"; echo | timeout 10 openssl s_client -connect 127.0.0.1:2083 -servername panel.set18.test 2>/dev/null | openssl x509 -noout -subject -issuer -dates -ext subjectAltName 2>&1
echo | timeout 10 openssl s_client -connect 127.0.0.1:2083 -noservername 2>/dev/null | openssl x509 -noout -subject -issuer 2>&1
echo "== submission TLS (127.0.0.1:587)"; echo | timeout 10 openssl s_client -starttls smtp -connect 127.0.0.1:587 -servername mail.set18.test 2>/dev/null | openssl x509 -noout -subject -issuer -dates -ext subjectAltName 2>&1
echo "== imaps TLS (127.0.0.1:993)"; echo | timeout 10 openssl s_client -connect 127.0.0.1:993 -servername mail.set18.test 2>/dev/null | openssl x509 -noout -subject -issuer -dates 2>&1
echo "== certbot lineages"; ls -la /etc/letsencrypt/live 2>&1; for f in /etc/letsencrypt/renewal/*.conf; do echo "-- $f"; grep -E '^(server|no_verify_ssl|authenticator|webroot_path|account) ' "$f"; done 2>&1
echo "== certbot timer"; systemctl is-enabled certbot.timer 2>&1; systemctl is-active certbot.timer 2>&1
echo "== postfix"; postconf -h myhostname smtpd_tls_security_level smtpd_tls_cert_file 2>&1
echo "== units"; for u in nginx postfix dovecot mariadb celikpanel-panel celikpanel-agent set18-lab-resolver set18-lab-pebble set18-lab-guard; do echo "$u $(systemctl is-active $u 2>&1)"; done
echo "== listeners"; ss -ltn 2>/dev/null | awk 'NR>1{print $4}' | sort -u | tr '\n' ' '; echo
echo "== nft tables"; nft list tables 2>&1
echo "== lab table counters"; nft list table ip set18lab 2>&1 | grep -E 'counter' | sed 's/^ *//'
echo "== 443 block log"; journalctl -k --no-pager -o short-iso | grep -F set18-lab-443-block | tail -n 20
echo "== pebble orders"; journalctl -u set18-lab-pebble --no-pager -o short-iso | grep -E 'Issued|order|authz|validat' | tail -n 30
echo "== resolver questions by name/type"; python3 -I -c 'import json,collections; c=collections.Counter((d.get("as"),d.get("qname"),d.get("qtype"),d.get("rcode")) for d in map(json.loads, open("/root/celikpanel-release-recovery-lab/set18-lab/resolver-queries.jsonl")) if d.get("event")=="query"); [print(n,*k) for k,n in sorted(c.items(), key=lambda x:-x[1])[:40]]'
R
echo "# rc=$?"; } > $out 2>&1
wc -l $out
