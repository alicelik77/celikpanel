for p in go2A.sh go2R.sh opsR.sh opsA.sh pair2.sh go1.sh waitf.sh single3.sh; do pkill -f "lab3/$p" && echo "stopped $p"; done
pkill -f 'owner.py a5-' && echo "stopped owner.py"
sleep 2; pgrep -af 'lab3/' | grep -v stopall | cut -c1-200
