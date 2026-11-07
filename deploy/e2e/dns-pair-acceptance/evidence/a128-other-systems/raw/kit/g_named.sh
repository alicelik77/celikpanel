journalctl -u celikpanel-agent -o cat --no-pager | grep -nE "BIND|bind zone|named" | cut -c1-1500 | tail -15
echo; dpkg -S /usr/lib/systemd/system/named.service; echo rc $?; dpkg -S /lib/systemd/system/named.service; echo rc $?; dpkg -S named.service; ps -eo pid,lstart,args | grep -E "[a]pt|[d]pkg" | head
