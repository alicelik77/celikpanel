source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
bash $A/portalF.sh down | tail -1
for c in d12v u22u d12u aru d12f u22f arf; do python3 -I $L/lab.py stop $c main 2>/dev/null | tr -d "\n "; echo " $c"; rm -rf /var/tmp/cp-beta1/state/$c /var/tmp/cp-beta1/lab/cells/$c /var/tmp/cp-beta1/evidence/$c; done
python3 -I $L/lab.py stop beta1-lab labnet --keep | tr -d "\n "; echo
for c in 0c7aefe8613c9fd5fd34799ec36b991d894c16cd 818a03e36db19421f6898c6ca53aaaea4c6b5cea 2ac7e5624db3c73357863ffba559f8085317298e 2b8114325036a218174e5f26144951f748287b06; do rm -rf /var/tmp/cp-pair-accept/dist/$c-acceptance-license; done
rm -rf /var/tmp/a129o
echo "qemu left: $(pgrep -af qemu-system | grep -v pgrep | wc -l)"; ls /var/tmp/cp-beta1/state /var/tmp/cp-beta1/lab/cells 2>/dev/null | tr "\n" " "; echo; df -h / | tail -1; free -m | sed -n 2p
