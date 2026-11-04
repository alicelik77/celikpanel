# guest (root, read only): built guide chunks served by the panel. Lab only.
D=/opt/celikpanel/web; ls $D | head; 
for p in admin.en customer.en reseller.en; do f=$(find $D -name "$p*.js" -printf '%P\n' | head -1); echo "$p chunk: ${f:-MISSING} http $(curl -sk -o /tmp/c.js -w '%{http_code} %{content_type} %{size_download}' https://127.0.0.1:1923/$f)"; done
for c in getting-started server-setup team-members; do f=$(find $D -name "$c.en*.js" -printf '%P\n' | head -1); echo "chapter $c: ${f:-MISSING} http $(curl -sk -o /dev/null -w '%{http_code} %{content_type} %{size_download}' https://127.0.0.1:1923/$f)"; done
echo "guide js chunks: $(find $D -name '*.js' | grep -cE '/(admin|customer|reseller)\.(en|tr)')  chapter chunks: $(find $D -name '*.js' | grep -cE '(getting-started|server-setup|team-members|dns|mail)\.(en|tr)')"
