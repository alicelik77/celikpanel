# guest (root): lab load - the site owner changes the Express app to the common default listen (all addresses, its own port). $1 = domain id. Lab only.
D=$(ls -d /var/www/celikpanel/subscriptions/*/sites/$1/public_html | head -1)
sed -i "s/app.listen(port, process.env.HOST || '127.0.0.1', /app.listen(port, /" $D/server.js; grep -n "listen" $D/server.js
