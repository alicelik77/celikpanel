systemctl show postfix -p Type -p MainPID -p RemainAfterExit -p SubState -p FragmentPath | cat
systemctl list-unit-files 'postfix*' --no-legend | cat; systemctl list-units 'postfix*' --all --no-legend --plain | cat
dpkg -l postfix | tail -1 | cut -c1-80
