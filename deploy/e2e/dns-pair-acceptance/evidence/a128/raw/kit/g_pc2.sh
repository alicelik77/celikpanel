stat -c '%A %U:%G %n' /var/cache/celikpanel /var/cache/celikpanel/pagecache /var/cache/celikpanel/pagecache/* /var/cache/celikpanel/pagecache/*/* 2>&1 | head
ps -o user= -C nginx | sort | uniq -c
systemctl show celikpanel-agent -p UMask -p Group -p User -p CacheDirectory 2>&1
grep -rn "var/cache/celikpanel" /etc/systemd/system/celikpanel* /usr/lib/systemd/system/celikpanel* /etc/tmpfiles.d 2>/dev/null | head
journalctl --since -10min -o cat --no-pager | grep -i "pagecache\|page cache" | tail -5 | cut -c1-250
