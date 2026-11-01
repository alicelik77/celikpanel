#!/bin/bash
# read only, on a mail guest: DKIM key/table state for a domain, rspamd's log
# lines for given Message-IDs, milter warnings on every boot.
# usage: guestmail.sh DOMAIN [MSGID...]   Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
D=$1; shift
conf=$(ls /etc/opendkim.conf /etc/opendkim/opendkim.conf 2>/dev/null | head -1)
kt=$(awk '$1=="KeyTable"{print $2}' $conf | sed 's/^refile://'); st=$(awk '$1=="SigningTable"{print $2}' $conf | sed 's/^refile://')
echo "opendkim conf $conf keytable=$kt signingtable=$st"
echo "keytable lines for $D: $(grep -c "$D" ${kt#*:} 2>/dev/null)"; grep "$D" ${kt#*:} 2>/dev/null | cut -c1-160
echo "signingtable lines for $D: $(grep -c "$D" ${st#*:} 2>/dev/null)"; grep "$D" ${st#*:} 2>/dev/null | cut -c1-160
echo "key files mentioning $D:"; find /etc/opendkim /var/lib/opendkim /etc/celikpanel* -path "*$D*" 2>/dev/null | head;
for id in "$@"; do
  echo "rspamd lines for $id:"
  { journalctl -u rspamd -b all -q --no-pager 2>/dev/null; cat /var/log/rspamd/rspamd.log 2>/dev/null; } | grep -F "$id" | grep -oE '\((default|local)[^)]*\)[^,]*|rspamd_task_write_log.{0,60}|mid: <[^>]*>|\[[-0-9.]+/[0-9.]+\]' | head -4
done
echo "milter warnings, all boots: $(journalctl -b all -q --no-pager | grep -ciE 'milter service|Milter.*(not found|refused|cannot)')"
journalctl -b all -q --no-pager -o short-iso | grep -iE 'milter service|Milter.*(not found|refused|cannot)' | head -3 | cut -c1-200
echo "boots: $(journalctl --list-boots --no-pager 2>/dev/null | wc -l)"
echo "milters: smtpd=$(postconf -h smtpd_milters) non_smtpd=$(postconf -h non_smtpd_milters)"; postconf -M submission/inet smtps/inet smtp/inet 2>/dev/null | awk '{print $1,$2,$5}'
