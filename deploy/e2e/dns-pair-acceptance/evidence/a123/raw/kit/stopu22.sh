pkill -f 'CELL=u22' 2>/dev/null; for p in $(pgrep -f 'singleX.sh'); do grep -qa 'CELL=u22' /proc/$p/environ 2>/dev/null && kill $p && echo "stopped singleX u22 pid $p"; done
pgrep -af singleX | cut -c1-100
