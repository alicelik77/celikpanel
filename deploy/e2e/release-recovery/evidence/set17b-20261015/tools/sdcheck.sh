#!/bin/bash
ps -p 1 -o comm=
systemctl is-system-running 2>&1
ls -d /root/celikpanel-release-recovery-lab 2>&1
stat -fc %T /sys/fs/cgroup
