export S=u22 IPU=192.0.3.221
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/hdr.sh
exec > >(tee -a $LOGS/C4mail-$S.log) 2>&1
D=site1.$Z; nup n_swaks.sh >/dev/null
echo "== 4.12 box2 again, $(ts) (password changed at 16:05:31)"; pw box2 | timeout 90 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_swaks.sh $MIP 587 box2@$D box1@$D box2@$D"
gshn main "journalctl -u postfwd --since -8min -o cat --no-pager | grep -E \"user=box2|reload|HUP|respawn|uninitialized|DELRATE\" | sed -E \"s/client=.*(hits=)/\1/\" | cut -c1-220 | tail -14"
