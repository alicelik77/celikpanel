#!/bin/bash
# read only, on a guest: journal checks over every boot. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
J() { journalctl -b all -q --no-pager -o short-iso "$@" | grep -vE ' sudo\[[0-9]+\]: | (celik|root) : PWD=|/root/q\.sh'; }
echo "== $(cat /etc/hostname): boots $(journalctl --list-boots --no-pager | wc -l)"
for p in 'milter service|Milter.*(not found|refused|cannot)' 'FQDN' 'fail-closed|ambiguous|agent_mutations_held|refusing durable' 'FOREIGN KEY' 'unsupported map type: lmdb' "can.t lock|cannot lock|could not lock"; do
  n=$(J | grep -ciE "$p"); echo "   [$p]: $n"; [ "$n" != 0 ] && J | grep -iE "$p" | head -3 | cut -c1-240
done
n=$(J -u celikpanel-panel | grep -c '\[500\]'); echo "   [panel 500]: $n"; [ "$n" != 0 ] && J -u celikpanel-panel | grep '\[500\]' | head -3 | cut -c1-240
echo "   err-priority lines of celikpanel units: $(journalctl -b all -q -p err -u 'celikpanel*' | wc -l)"; journalctl -b all -q -p err -u 'celikpanel*' -o short-iso | head -4 | cut -c1-240
echo "   [systemd is not ready]: $(J | grep -c 'systemd is not ready')"; J | grep 'systemd is not ready' | head -3 | cut -c1-240
echo "   [held_by= / server_setup_host_busy]: $(J | grep -cE 'held_by=|server_setup_host_busy')"; J | grep -E 'held_by=|server_setup_host_busy' | head -4 | cut -c1-260
echo "   per boot: boot start | panel starts | 'Stopping celikpanel-panel' | wait line | finished line | timeout lines"
for b in $(journalctl --list-boots --no-pager -q | awk '$1 ~ /^-?[0-9]+$/ {print $1}'); do
  B() { journalctl -b $b -q -o short-iso --no-pager "$@"; }
  bs=$(B | head -1 | cut -c12-19)
  ns=$(B -u celikpanel-panel | grep -c 'Starting CelikPanel Backend')
  st=$(B | grep -c 'Stopping celikpanel-panel')
  w=$(B -u celikpanel-panel | grep -m1 'startup: this server is still starting' | cut -c12-19)
  f=$(B -u celikpanel-panel | grep -m1 'finished starting after' | sed -E 's/^.{11}(.{8}).*(finished starting after [^;]*).*/\1 \2/')
  to=$(B -u celikpanel-panel | grep -c 'was still starting after')
  echo "   boot $b: $bs | starts $ns | stopping $st | wait ${w:--} | ${f:--} | timeout $to"
done
echo "   failed units: [$(systemctl --failed --no-legend --no-pager | cut -c1-100 | paste -sd';')]"
