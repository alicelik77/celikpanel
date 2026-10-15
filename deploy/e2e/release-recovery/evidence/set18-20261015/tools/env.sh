# set18: shared names (sourced)
R=/var/tmp/cp-set18
L=$R/logs
J=<redacted>
REPO='/mnt/c/CELIKBROS PROJECTS/celikpanel'
C=5d54aba1e9c93357fa0c19d3b0487e27384acd01
G() { git -c safe.directory='*' -C "$REPO" "$@"; }
