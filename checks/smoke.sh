#!/usr/bin/env bash
# Smoke tests for drohnenwetter.de
# Usage: ./checks/smoke.sh [base_url]
# Exits 0 if all checks pass, 1 otherwise.

BASE="${1:-https://drohnenwetter.de}"
PASS=0
FAIL=0

ok()   { echo "  PASS  $1"; ((PASS++)); }
fail() { echo "  FAIL  $1"; ((FAIL++)); }

check_status() {
  local label="$1" url="$2" expected="${3:-200}"
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" --max-time 15 "$url")
  if [[ "$status" == "$expected" ]]; then
    ok "$label (HTTP $status)"
  else
    fail "$label — expected $expected, got $status ($url)"
  fi
}

check_body() {
  local label="$1" url="$2" pattern="$3"
  local body
  body=$(curl -s --max-time 15 "$url")
  if echo "$body" | grep -q "$pattern"; then
    ok "$label"
  else
    fail "$label — pattern '$pattern' not found in response ($url)"
  fi
}

check_json() {
  local label="$1" url="$2"
  local body
  body=$(curl -s --max-time 15 "$url")
  if echo "$body" | python3 -m json.tool > /dev/null 2>&1; then
    ok "$label (valid JSON)"
  else
    fail "$label — response is not valid JSON ($url)"
  fi
}

echo "Smoke tests → $BASE"
echo "-------------------------------------------"

# 1. Health endpoint
check_status "/health" "$BASE/health" 200
check_body   "/health returns 'ok'" "$BASE/health" "ok"

# 2. Homepage loads
check_status "/ homepage" "$BASE/" 200
check_body   "/ contains search form" "$BASE/" "address"

# 3. zone-info (Berlin Mitte) — tests DiPUL API
check_status "/zone-info Berlin" "$BASE/zone-info?lat=52.5200&lon=13.4050" 200
check_json   "/zone-info returns JSON" "$BASE/zone-info?lat=52.5200&lon=13.4050"

# 4. Full results (Berlin address) — tests HERE + weather APIs
RESULT_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --max-time 30 \
  -X POST "$BASE/results" \
  -d "address=Alexanderplatz+1%2C+Berlin")
if [[ "$RESULT_STATUS" == "200" ]]; then
  ok "/results POST (full weather lookup)"
else
  fail "/results POST — expected 200, got $RESULT_STATUS"
fi

# 5. Static assets
check_status "/favicon.ico" "$BASE/favicon.ico" 200
check_status "/robots.txt"  "$BASE/robots.txt"  200

echo "-------------------------------------------"
echo "Results: $PASS passed, $FAIL failed"

[[ "$FAIL" -eq 0 ]]
