for p in trackA.sh trackR.sh trackN.sh opsA4.sh opsR4.sh opsN4.sh shotsA.sh pair2.sh single3.sh waitf.sh delwatch.sh; do pkill -f "lab4/$p" && echo "stopped $p"; done
pkill -f 'owner.py a6-' && echo "stopped owner.py"
sleep 2; pgrep -af 'lab4/' | grep -v stopall | cut -c1-200
