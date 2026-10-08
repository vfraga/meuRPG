#!/bin/sh
# usage: run.sh <name> <TestName> <package dir under backend/internal>
# Runs an overlay test file (kept outside the repo) and writes its output to ../<name>.json
set -e
NAME=$1; TEST=$2; PKG=${3:-rules}
HERE=$(cd "$(dirname "$0")" && pwd)
REPO=${REPO:-/home/user/meuRPG}
cat > "$HERE/overlay-$NAME.json" <<J
{"Replace":{"$REPO/backend/internal/$PKG/zz_${NAME}_dump_test.go":"$HERE/zz_${NAME}_dump_test.go"}}
J
cd "$REPO/backend"
COV_OUT="$HERE/../$NAME.json" go test -overlay "$HERE/overlay-$NAME.json" -run "$TEST" -count=1 ./internal/$PKG/
