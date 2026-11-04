# guest (root): write the lab Flask app into a site's app/src AS THE SITE'S OWN ACCOUNT (customer upload stand-in,
# like panel-runtime.sh UPLOAD=local). usage: g_pyapp.sh SITEUSER MODE   MODE: good | badreq-e | badreq-url | brokenapp. Lab only.
U=$1; MODE=$2; H=$(getent passwd $U | cut -d: -f6); D=$H/app/src
runuser -u $U -- mkdir -p $D
w() { runuser -u $U -- sh -c "cat > '$D/$1'"; }
case $MODE in
good)
  printf 'from flask import Flask\napp = Flask(__name__)\n\n@app.get("/")\ndef index():\n    return "lab flask app ok\\n"\n' | w app.py
  printf 'flask==3.1.2\n' | w requirements.txt ;;
badreq-e)   printf 'flask==3.1.2\n-e ./local-pkg\n' | w requirements.txt ;;
badreq-url) printf '# lab\nflask==3.1.2\nhttps://example.com/pkg-1.0-py3-none-any.whl\n' | w requirements.txt ;;
brokenapp)
  printf 'flask==3.1.2\n' | w requirements.txt
  printf 'raise RuntimeError("lab: this app is broken on purpose")\n' | w app.py ;;
esac
echo "## $D ($MODE)"; ls -la $D; cat $D/requirements.txt
