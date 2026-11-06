# guest (root, read-only): what the joined server ran for api6.test's deletion (three DELETE presses at 13:26:06-13:26:17). Lab only.
journalctl --since 13:26:00 --until 13:27:00 -o short-iso --no-pager | grep -E "api6" | cut -c1-260
echo "userdel api6_test: $(journalctl --since 13:26:00 --until 13:27:00 -o cat --no-pager | grep -c "delete user 'api6_test'")  domain deletion pending lines: $(journalctl -u celikpanel-panel --since 13:26:00 --until 13:27:00 -o cat --no-pager | grep -c 'domain deletion pending for api6.test')"
