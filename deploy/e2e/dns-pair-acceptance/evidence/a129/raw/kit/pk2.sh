pkill -f runPF.sh; pkill -f infra-up.sh; pkill -f "wait-ssh beta1-lab"; sleep 1; pgrep -af "runPF|infra-up" | grep -v pgrep | wc -l
