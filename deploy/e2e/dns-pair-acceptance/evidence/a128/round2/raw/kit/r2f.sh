# alpha.128 round 2: SFTP confinement smoke with password sign-in allowed on the lab servers (lab load: a drop-in that
# sorts before cloud-init's PasswordAuthentication no), Debian 13 and Ubuntu 24.04; removed afterwards. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/R2sftp-$CELL.log) 2>&1
for n in main backup; do gshn $n 'printf "# a128 lab load\nPasswordAuthentication yes\n" > /etc/ssh/sshd_config.d/00-a128-lab.conf; sshd -t && systemctl reload ssh 2>/dev/null || systemctl reload sshd; sshd -T | grep -i "^passwordauthentication"'; done
bash $A/r2sftp.sh
for n in main backup; do gshn $n 'rm -f /etc/ssh/sshd_config.d/00-a128-lab.conf; systemctl reload ssh 2>/dev/null || systemctl reload sshd; sshd -T | grep -i "^passwordauthentication"'; done
