# guest (root): write lab Python apps into a site's app/src AS THE SITE'S OWN ACCOUNT (customer upload stand-in,
# like panel-runtime.sh UPLOAD=local). usage: g_pyapp2.sh SITEUSER MODE
# MODE: flask | flask-broken | fastapi. Lab only.
U=$1; MODE=$2; H=$(getent passwd $U | cut -d: -f6); D=$H/app/src
runuser -u $U -- mkdir -p $D
w() { runuser -u $U -- sh -c "cat > '$D/$1'"; }
case $MODE in
flask)
  printf 'from flask import Flask\napp = Flask(__name__)\n\n@app.get("/")\ndef index():\n    return "lab flask app ok\\n"\n' | w app.py
  printf 'flask==3.1.2\n' | w requirements.txt ;;
flask-broken)
  printf 'raise RuntimeError("lab: this module is broken on purpose")\n' | w app.py ;;
fastapi)
  printf 'from fastapi import FastAPI\napp = FastAPI()\n\n@app.get("/")\ndef index():\n    return {"lab": "fastapi app ok"}\n' | w main.py
  printf 'fastapi==0.115.6\n' | w requirements.txt ;;
esac
echo "## $D ($MODE)"; ls -la $D; cat $D/requirements.txt 2>/dev/null; ls -d $H/app/venv-* 2>/dev/null
