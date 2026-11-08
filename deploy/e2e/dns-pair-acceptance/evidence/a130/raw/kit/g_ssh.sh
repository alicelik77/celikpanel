# guest (root): SSH lab preparation and reads for the password sign-in check. usage: prep | setpw (password on stdin) | read | ownedit | ownrevert | restore. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
D=/etc/ssh/sshd_config.d; O=/root/sshd-orig
case $1 in
prep) mkdir -p $O; [ -f $O/saved ] || { cp -a $D/. $O/ 2>/dev/null; touch $O/saved; }
  echo 'PasswordAuthentication yes' > $D/50-cloud-init.conf; echo 'PasswordAuthentication no' > $D/60-cloudimg-settings.conf
  id labpw >/dev/null 2>&1 || useradd -m -s /bin/bash -G sudo labpw
  systemctl reload ssh 2>/dev/null || systemctl reload sshd; ls -la $D ;;
setpw) read -r P; echo "labpw:$P" | chpasswd && echo "password set for labpw" ;;
read) ls -la $D; for f in $D/*; do echo "--- $f"; cat $f; done; sshd -T 2>/dev/null | grep -E '^(passwordauthentication|kbdinteractiveauthentication|permitrootlogin|pubkeyauthentication) '; systemctl is-active ssh ssh.socket 2>&1 | tr '\n' ' '; echo ;;
ownedit) echo '# owner note' >> $D/00-celikpanel.conf; tail -2 $D/00-celikpanel.conf ;;
ownrevert) sed -i '/^# owner note$/d' $D/00-celikpanel.conf; cat $D/00-celikpanel.conf ;;
restore) rm -f $D/*; cp -a $O/. $D/; rm -f $D/saved; userdel -r labpw 2>/dev/null; systemctl reload ssh 2>/dev/null || systemctl reload sshd; ls -la $D; sshd -T | grep -E '^passwordauthentication ' ;;
esac
case $1 in
chgrp) chgrp root /etc/ssh/sshd_config.d/00-celikpanel.conf && ls -la /etc/ssh/sshd_config.d/00-celikpanel.conf ;;
esac
