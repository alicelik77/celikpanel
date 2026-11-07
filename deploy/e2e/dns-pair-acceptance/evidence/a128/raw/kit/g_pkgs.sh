dpkg-query -W -f '${Package} ${db:Status-Abbrev}\n' zip unzip less nano rsync vim-tiny file 2>&1 | tr '\n' ';'; echo
