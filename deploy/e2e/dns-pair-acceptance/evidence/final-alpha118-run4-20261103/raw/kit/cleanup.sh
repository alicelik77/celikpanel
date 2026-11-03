source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/env.sh
df -h /var/tmp | tail -1; du -sh $B /var/tmp/cp-beta1/lab/cells/a118 /var/tmp/cp-beta1/lab/cells/u117 2>/dev/null
bash $A/portalF.sh down | tail -1
lab ssh beta1-lab labnet -- "sudo -n sh -c 'userdel -r rbk 2>/dev/null; rm -f /root/rbkkey /root/rbkkey.pub /root/labchk.sh /root/nrb.sh; echo labnet-cleaned'"
for c in a118 a118d; do python3 -I $L/lab.py stop $c main 2>/dev/null | tr -d '\n '; echo; done
python3 -I $L/lab.py stop beta1-lab labnet --keep | tr -d '\n '; echo
rm -rf /var/tmp/cp-beta1/state/a118 /var/tmp/cp-beta1/state/u117 /var/tmp/cp-beta1/state/a118d /var/tmp/cp-beta1/evidence/a118d /var/tmp/cp-beta1/lab/cells/a118d /var/tmp/cp-beta1/evidence/a118 /var/tmp/cp-beta1/evidence/u117 /var/tmp/cp-beta1/lab/cells/a118 /var/tmp/cp-beta1/lab/cells/u117 /var/tmp/cp-beta1/lab/cells/a118c
rm -rf $B /var/tmp/cp-pair-accept/dist/$C118-acceptance-license /var/tmp/cp-pair-accept/dist/$C117-acceptance-license /var/tmp/cp-pair-accept/dist/02542ef2432a12bfb090166b04f86876b99eca96-acceptance-license
echo "qemu left: $(pgrep -af qemu-system | wc -l)"; pgrep -af 'a118|u117|beta1.py|lab.py|serve.py' | grep -v cleanup | cut -c1-120
ls /var/tmp/cp-beta1/state /var/tmp/cp-beta1/lab/cells; ls /var/tmp/cp-pair-accept/dist; df -h /var/tmp | tail -1
