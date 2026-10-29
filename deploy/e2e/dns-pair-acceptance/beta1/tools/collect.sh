# usage: collect CELL NODE  -> journals and versions into the cell evidence (redacted by the driver)
collect() {
  $B sh $1 $2 "journalctl -u celikpanel-panel -u celikpanel-agent -u named -u bind9 -u pdns --no-pager -o short-iso -b all | grep -vE 'metrics' | tail -n 400" --label journal-$2 --limit 120000 > /dev/null
  $B sh $1 $2 "cat /opt/celikpanel/release.commit 2>/dev/null; /opt/celikpanel/bin/panel --version 2>&1 | head -2; dpkg-query -W bind9 pdns-server nginx mariadb-server 2>/dev/null; systemctl is-active celikpanel-panel celikpanel-agent named; nft list tables" --label versions-$2 --limit 4000 > /dev/null
  echo collected $1 $2
}
