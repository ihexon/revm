#!/bin/bash
set -euo pipefail

REVM="./out/bin/revm"
TOTAL=20
PASS=0
FAIL=0

for i in $(seq 1 $TOTAL); do
    ID="test-run-$i"
    echo "=== Run $i/$TOTAL (id=$ID) ==="

    rm -rf "/tmp/$ID"

    OUTFILE="/tmp/revm-test-$i.log"
    timeout 30 $REVM docker --id "$ID" --log-level debug >"$OUTFILE" 2>&1 || true
    EXIT_CODE=${PIPESTATUS[0]:-$?}

    if grep -q "podman API proxy ready" "$OUTFILE"; then
        echo "  PASS (podman ready)"
        PASS=$((PASS + 1))
    elif grep -q "all guest services are ready" "$OUTFILE"; then
        echo "  PASS (all services ready)"
        PASS=$((PASS + 1))
    elif grep -q "ssh ready" "$OUTFILE"; then
        echo "  PASS (ssh ready)"
        PASS=$((PASS + 1))
    elif grep -q "Power down" "$OUTFILE"; then
        echo "  PASS (clean power down)"
        PASS=$((PASS + 1))
    else
        echo "  FAIL (exit=$EXIT_CODE)"
        FAIL=$((FAIL + 1))
        grep -E "ERRO|WARN|panic|fatal|Error|error" "$OUTFILE" | head -5 || true
        echo "  (last 3 lines):"
        tail -3 "$OUTFILE"
    fi

    rm -rf "/tmp/$ID"
    rm -f "$OUTFILE"

    # Give the system time to fully clean up krun resources
    sleep 2
done

echo ""
echo "================================"
echo "Results: $PASS/$TOTAL passed, $FAIL/$TOTAL failed"
echo "================================"

if [ $FAIL -eq 0 ]; then
    echo "ALL TESTS PASSED"
    exit 0
else
    echo "SOME TESTS FAILED"
    exit 1
fi
