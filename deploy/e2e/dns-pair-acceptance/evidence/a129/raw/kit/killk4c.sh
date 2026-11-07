for p in $(pgrep -f "k4c.sh"); do kill $p; done; sleep 1; pgrep -af "k4c" | grep -v pgrep | wc -l
