grep -E "^202|^409|status_url|GONE|deletion_incomplete|== 6" /var/tmp/a129o/logs/C6tok-r9.log | sed -E "s/(FTPPassword|password)\"?: ?\"[^\"]*\"/:REDACTED/Ig" | cut -c1-420
