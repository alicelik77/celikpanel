command -v mysql mariadb; ls -la $(command -v mysql); mariadb --version
echo "select count(*) from tgt_ar_test_copy_76c5.celikpanel_install_ownership;" | mysql --batch --skip-column-names 2>&1 | od -c | head -8
