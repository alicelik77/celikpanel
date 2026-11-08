source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
df -h / | tail -1; du -sh /var/tmp 2>/dev/null
lab ssh beta1-lab labnet -- "sudo -n sh -c 'systemctl stop a128-git 2>/dev/null; ip addr del 192.0.2.62/24 dev lab0 2>/dev/null; rm -rf /srv/git/app.git /root/gitwork /root/git-tls-key.pem /root/git-tls.pem; echo git-down'"
bash $A/portalF.sh down | tail -1
for n in main backup; do python3 -I $L/lab.py stop r8 $n 2>/dev/null | tr -d '\n '; echo; done
python3 -I $L/lab.py stop beta1-lab labnet --keep | tr -d '\n '; echo
for c in r8 f8; do rm -rf /var/tmp/cp-beta1/state/$c /var/tmp/cp-beta1/lab/cells/$c /var/tmp/cp-beta1/evidence/$c; done
for d in 0c7aefe8613c9fd5fd34799ec36b991d894c16cd a48a12e3ed174b9f8fc9aece98a27d946a8a8052 91482b38612d91d9432e96250f7b5c0ef0ee497d 2e1790160d051a2dbe9ac1e1cce50f5e01d396b6 6e2e1af46ec3ba30551fcf2880819537cce656b0; do rm -rf /var/tmp/cp-pair-accept/dist/$d-acceptance-license; done
rm -rf /var/tmp/a128r1 /var/tmp/a128-phase1.out /var/tmp/a128-phase1b.out
echo "qemu left: $(pgrep -af qemu-system | grep -v pgrep | wc -l)"; pgrep -af qemu-system | grep -o "name [^ ]*"
ls /var/tmp/cp-beta1/lab/cells; df -h / | tail -1; du -sh /var/tmp 2>/dev/null; free -m | sed -n 2p
