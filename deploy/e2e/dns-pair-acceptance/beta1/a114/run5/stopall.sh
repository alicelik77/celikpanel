for p in trackA.sh trackR.sh trackN.sh opsA5.sh opsE5.sh shotsA.sh pair2.sh single3.sh waitf.sh delwatch.sh; do pkill -f "lab5/$p" && echo "stopped $p"; done
pkill -f 'owner.py a6-' && echo "stopped owner.py"
sleep 2; pgrep -af 'lab5/' | grep -v stopall | cut -c1-200
