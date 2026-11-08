# lab: remove the unused lab 131 build (superseded by the integrated head)
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/env.sh
C=$(cat $B/c131); rm -rf /var/tmp/cp-pair-accept/dist/$C-acceptance-license $B/portal-v0.1.0-alpha.131 $B/repo-v0.1.0-alpha.131 $B/tools-$C /var/tmp/a130o/portalnet/site/releases/v0.1.0-alpha.131
lab ssh beta1-lab labnet -- "sudo -n rm -rf /root/a114-portal/site/releases/v0.1.0-alpha.131; sudo -n ls /root/a114-portal/site/releases"
du -sh /var/tmp/a130o /var/tmp/cp-pair-accept /var/tmp/cp-beta1 2>/dev/null; df -h / | tail -1
