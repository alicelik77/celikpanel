source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/env.sh
df -h / | tail -1; du -sh /var/tmp 2>/dev/null
bash $A/portalF.sh down | tail -1
for n in main backup; do python3 -I $L/lab.py stop r7 $n 2>/dev/null | tr -d '\n '; echo; done
python3 -I $L/lab.py stop f7 main 2>/dev/null | tr -d '\n '; echo
python3 -I $L/lab.py stop beta1-lab labnet --keep | tr -d '\n '; echo
for c in r7 f7 a118; do rm -rf /var/tmp/cp-beta1/state/$c /var/tmp/cp-beta1/lab/cells/$c /var/tmp/cp-beta1/evidence/$c; done
for d in 14a90360cdabf5972345f93307dd988ef4d0770d 4ee915bb85f24d1c15c3eb28cebe905bf6137ee8 3ac4745f2681e747c20c627bc9b2ecc00b45200c 111b45f28fb60e531097ef6ba20e0d10e837ffcd 50910894e4f20ab6d1ba85b860c4cb6cc6ca0294 b25b1b717fa00b34fee5e77f8aec3c07c9dd41dd; do rm -rf /var/tmp/cp-pair-accept/dist/$d-acceptance-license; done
rm -rf /var/tmp/a128r1 /var/tmp/a114r6
echo "qemu left: $(pgrep -af qemu-system | grep -v pgrep | wc -l)"; pgrep -af qemu-system | grep -o "name [^ ]*"
ls /var/tmp/cp-beta1/lab/cells; df -h / | tail -1; du -sh /var/tmp 2>/dev/null; free -m | sed -n 2p
