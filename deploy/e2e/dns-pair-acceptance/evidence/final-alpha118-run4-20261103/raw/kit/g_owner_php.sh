# OWNER ACTION (lab), as the panel's APP_EXTENSIONS_MISSING guidance says: the system's package manager.
DEBIAN_FRONTEND=noninteractive apt-get install -y -q php8.4-curl php8.4-gd php8.4-mysql php8.4-bcmath 2>&1 | tail -3
systemctl is-active php8.4-fpm; php8.4 -m | grep -iE '^(curl|gd|pdo_mysql|mysqli|bcmath|intl|zip)$' | tr '\n' ' '
