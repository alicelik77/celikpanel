# guest (root): lab load standing for Boston (unzip and zip absent). Lab only.
DEBIAN_FRONTEND=noninteractive apt-get remove -y -q zip unzip 2>&1 | tail -2
dpkg-query -W -f '${Package} ${db:Status-Abbrev}\n' zip unzip 2>&1 | tr '\n' ';'; echo
