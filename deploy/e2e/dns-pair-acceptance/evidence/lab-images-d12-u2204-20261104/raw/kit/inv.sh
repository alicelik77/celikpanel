#!/bin/bash
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
echo "== date"; date -u
echo "== qemu processes"; ps -eo pid,etimes,rss,args | grep -E 'qemu-system' | grep -v grep
echo "== other lab processes"; ps -eo pid,etimes,args | grep -E 'beta1|lab.py|curl|wget|gpg|sha(256|512)sum|build-dist|npm|go build|python3' | grep -v grep | head -30
echo "== mounts under lab roots"; grep -E '/var/tmp' /proc/mounts
echo "== images"; ls -la /var/tmp/cp-v3n28/images/ 2>&1
echo "== image root subdirs/partials"; find /var/tmp/cp-v3n28/images -mindepth 1 -maxdepth 2 \( -type d -o -name '*.part' -o -name '*.tmp' -o -name '*SUMS*' -o -name '*.gpg' \) -printf '%y %s %TY-%Tm-%Td %TH:%TM %p\n' 2>&1
echo "== cp-v3n28 top"; ls -la /var/tmp/cp-v3n28/ 2>&1
echo "== beta1 lab nodes"; find /var/tmp/cp-beta1/lab -maxdepth 2 -mindepth 1 -printf '%TY-%Tm-%Td %TH:%TM %p\n' 2>&1 | sort | tail -30
echo "== var/tmp top"; ls -la /var/tmp/ 2>&1
echo "== a118r1"; ls -la /var/tmp/a118r1 2>&1 | head -50
echo "== taps/bridge"; ip -br link | grep -E 'cpb1|tap|cp' 
echo "== disk"; df -h /var/tmp / 2>&1
echo "== recent large files in /var/tmp (last 1 day, >50M)"; find /var/tmp /tmp /root -xdev -type f -size +50M -mtime -1 -printf '%s %TY-%Tm-%Td %TH:%TM %p\n' 2>/dev/null | sort -k3 | tail -30
