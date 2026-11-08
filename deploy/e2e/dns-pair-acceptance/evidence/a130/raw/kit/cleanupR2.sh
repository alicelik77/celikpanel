source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
lab ssh beta1-lab labnet -- "sudo -n sh -c 'systemctl stop a128-git 2>/dev/null; ip addr del 192.0.2.62/24 dev lab0 2>/dev/null; rm -rf /srv/git/app.git /root/gitwork /root/git-tls-key.pem /root/git-tls.pem /root/git-smart.py /root/tok-adm.txt; echo git-down'"
bash $A/portalF.sh down | tail -1
for n in main backup; do python3 -I $L/lab.py stop r9 $n 2>/dev/null | tr -d '\n '; echo; done
python3 -I $L/lab.py stop f9 main 2>/dev/null | tr -d '\n '; echo
python3 -I $L/lab.py stop beta1-lab labnet --keep | tr -d '\n '; echo
for c in r9 f9; do rm -rf /var/tmp/cp-beta1/state/$c /var/tmp/cp-beta1/lab/cells/$c /var/tmp/cp-beta1/evidence/$c; done
for f in $B/out-build*.out; do c=$(sed -n 's/^lab bump commit //p;s/^commit //p' $f | tail -1); [ -n "$c" ] && rm -rf /var/tmp/cp-pair-accept/dist/$c-acceptance-license; done
rm -rf /var/tmp/cp-pair-accept/dist/0c7aefe8613c9fd5fd34799ec36b991d894c16cd-acceptance-license /var/tmp/a130o /var/tmp/a128-web
echo "qemu left: $(pgrep -af qemu-system | grep -v pgrep | wc -l)"; pgrep -af qemu-system | grep -o "name [^ ]*"
df -h / | tail -1; free -m | sed -n 2p
