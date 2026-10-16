# set18b: shared names (sourced)
R=/var/tmp/cp-set18b
L=$R/logs
J=<redacted>
REPO='/mnt/c/CELIKBROS PROJECTS/celikpanel'
C=6b363ac4680146f856642cffdab0b57662f6159c
G() { git -c safe.directory='*' -C "$REPO" "$@"; }
