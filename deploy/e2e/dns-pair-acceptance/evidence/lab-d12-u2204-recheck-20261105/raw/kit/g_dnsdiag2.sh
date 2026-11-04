# guest (root, read-only): why 01-dns failed. Lab only.
grep PRETTY /etc/os-release; systemctl --version | head -1
journalctl -u celikpanel-agent --since "-20 min" --no-pager -o short-precise | grep -iE 'dns|bind|named|error|fail|refus|identity|vendor' | tail -60
echo "-- named"; systemctl is-active named bind9 2>&1 | tr '\n' ' '; echo; dpkg -l bind9 2>/dev/null | grep -E '^(ii|rc)' | awk '{print $1,$2,$3}'
systemctl show named.service -p Id -p Names -p FragmentPath -p DropInPaths -p UnitFileState -p LoadState 2>&1
ls -la /lib /usr/lib/systemd/system/named.service /lib/systemd/system/named.service 2>&1 | head
dpkg -S named.service 2>&1 | head; cat /lib/systemd/system/named.service 2>/dev/null
journalctl -u named --since "-20 min" --no-pager | tail -15
echo "-- apt"; tail -12 /var/log/apt/history.log 2>/dev/null
