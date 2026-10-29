pkill -f '/b1/run.sh' ; sleep 1; pgrep -af 'run.sh' | grep -v pgrep | head -3; echo killed
