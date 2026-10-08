#!/bin/sh
# Runs the overlay test that dumps the machine view of every spell.
set -e
HERE=$(cd "$(dirname "$0")" && pwd)
REPO=${REPO:-/home/user/meuRPG}
cat > "$HERE/overlay.json" <<J
{"Replace":{"$REPO/backend/internal/rules/zz_spells_dump_test.go":"$HERE/zz_spells_dump_test.go"}}
J
cd "$REPO/backend"
COV_OUT="$HERE/../spells-machine.json" go test -overlay "$HERE/overlay.json" -run TestCoverageDumpSpells -count=1 ./internal/rules/
