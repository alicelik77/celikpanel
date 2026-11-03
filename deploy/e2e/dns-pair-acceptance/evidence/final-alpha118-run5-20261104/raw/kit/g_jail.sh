# guest (root, read only): jail tools present for subscription $1, sshd config, packages. Lab only.
J=/var/www/celikpanel/subscriptions/$1/sites
echo "## jail $J"; for t in zip unzip nano vi vim rsync less bash; do p=$(ls -d $J/usr/bin/$t $J/bin/$t 2>/dev/null | head -1); echo "$t: ${p:-MISSING}"; done
grep -nE "Match User|ChrootDirectory|ForceCommand" /etc/ssh/sshd_config.d/60-celikpanel-file-access.conf 2>&1 | head -8; sshd -t && echo "sshd -t ok"
dpkg -l zip unzip nano vim-tiny rsync less 2>/dev/null | grep -E "^(ii|un)" | awk '{print $1, $2, $3}'
