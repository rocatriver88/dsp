#!/usr/bin/env bash
# scripts/qa/lib.sh
# Shared helpers for biz QA harness scripts. Source after e2e-env.sh.
set -euo pipefail

# Persistent state dir per run. Reused across step scripts via exported path.
QA_STATE_DIR="${QA_STATE_DIR:-$(mktemp -d -t biz-qa-XXXXXX)}"
export QA_STATE_DIR

log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*" >&2; }
fail() { log "FAIL: $*"; exit 1; }
step_start() { log "---- $* ----"; }
step_ok() { log "[ok] $*"; }

# curl_json METHOD PATH [BODY_JSON] [API_KEY]
#   Hits ${DSP_E2E_API}${PATH}. Appends an HTTP status suffix so callers
#   can distinguish body from status. Output format:
#     <body>\n<status>
curl_json() {
  local method="$1"; shift
  local path="$1"; shift
  local body="${1:-}"
  local key="${2:-}"
  local args=(-sS -X "$method" -w '\n%{http_code}' -H "Content-Type: application/json")
  [ -n "$key" ] && args+=(-H "X-API-Key: $key")
  [ -n "$body" ] && args+=(--data "$body")
  curl "${args[@]}" "${DSP_E2E_API}${path}"
}

# curl_admin METHOD PATH [BODY_JSON]
#   Hits ${DSP_E2E_ADMIN_API}${PATH} with the admin token header.
curl_admin() {
  local method="$1"; shift
  local path="$1"; shift
  local body="${1:-}"
  local args=(-sS -X "$method" -w '\n%{http_code}'
    -H "Content-Type: application/json"
    -H "X-Admin-Token: ${DSP_E2E_ADMIN_TOKEN}")
  [ -n "$body" ] && args+=(--data "$body")
  curl "${args[@]}" "${DSP_E2E_ADMIN_API}${path}"
}

# assert_status EXPECTED RESPONSE
#   Splits `curl_json`/`curl_admin` output on the trailing newline,
#   compares status, echoes body on success, fails on mismatch.
assert_status() {
  local expected="$1"
  local resp="$2"
  local status="${resp##*$'\n'}"
  local body="${resp%$'\n'*}"
  if [ "$status" != "$expected" ]; then
    log "expected status $expected, got $status"
    log "body: $body"
    return 1
  fi
  printf '%s' "$body"
}

# _json_reader returns the name of a working JSON extractor: "jq" or
# "python". Cached on first call. Empty string if neither is available
# (callers should check and fail loudly).
_json_reader() {
  if [ -n "${_QA_JSON_READER:-}" ]; then
    printf '%s' "$_QA_JSON_READER"
    return
  fi
  if have_tool jq; then
    _QA_JSON_READER=jq
  elif have_tool python; then
    _QA_JSON_READER=python
  elif have_tool python3; then
    _QA_JSON_READER=python3
  else
    _QA_JSON_READER=
  fi
  printf '%s' "$_QA_JSON_READER"
}

# json_field BODY FIELD
#   Extracts a top-level JSON field (no nesting). Returns empty if
#   missing. Uses jq when available, falls back to python's stdlib
#   json module so hosts without jq still work. FIELD may be a top-
#   level key name (e.g. "code", "id", "api_key"); nested paths are
#   NOT supported — use the full extractor via a subshell if needed.
json_field() {
  local body="$1"
  local field="$2"
  local reader
  reader="$(_json_reader)"
  case "$reader" in
    jq)
      printf '%s' "$body" | jq -r ".${field} // empty"
      ;;
    python|python3)
      printf '%s' "$body" | "$reader" -c "
import json, sys
try:
    d = json.loads(sys.stdin.read() or '{}')
except Exception:
    sys.exit(0)
if isinstance(d, dict):
    v = d.get('${field}')
    if v is None:
        sys.exit(0)
    print(v)
"
      ;;
    *)
      fail "no JSON reader available: install jq or python"
      ;;
  esac
}

# json_array_field BODY INDEX FIELD
#   Extract a field from an element of a top-level JSON array.
#   Example: json_array_field "$body" 0 id  -> prints the first elem's id.
#   Returns empty on missing. Used by scripts that hit list endpoints.
json_array_field() {
  local body="$1"
  local idx="$2"
  local field="$3"
  local reader
  reader="$(_json_reader)"
  case "$reader" in
    jq)
      printf '%s' "$body" | jq -r ".[${idx}].${field} // empty"
      ;;
    python|python3)
      printf '%s' "$body" | "$reader" -c "
import json, sys
try:
    d = json.loads(sys.stdin.read() or '[]')
except Exception:
    sys.exit(0)
if isinstance(d, list) and len(d) > ${idx}:
    v = d[${idx}].get('${field}') if isinstance(d[${idx}], dict) else None
    if v is None:
        sys.exit(0)
    print(v)
"
      ;;
    *)
      fail "no JSON reader available"
      ;;
  esac
}

# json_array_find_field BODY MATCH_KEY MATCH_VALUE OUT_FIELD
#   Scan a top-level JSON array; for the first element where
#   element[MATCH_KEY] == MATCH_VALUE, print element[OUT_FIELD].
#   Used for "find the entry with email=X and return its id" flows.
json_array_find_field() {
  local body="$1"
  local mk="$2"
  local mv="$3"
  local of="$4"
  local reader
  reader="$(_json_reader)"
  case "$reader" in
    jq)
      printf '%s' "$body" | jq -r --arg mk "$mk" --arg mv "$mv" --arg of "$of" \
        '[.[] | select(.[$mk] == $mv)][0][$of] // empty'
      ;;
    python|python3)
      printf '%s' "$body" | "$reader" -c "
import json, sys
try:
    d = json.loads(sys.stdin.read() or '[]')
except Exception:
    sys.exit(0)
mk, mv, of = '${mk}', '${mv}', '${of}'
if isinstance(d, list):
    for item in d:
        if isinstance(item, dict) and str(item.get(mk)) == mv:
            v = item.get(of)
            if v is not None:
                print(v)
            break
"
      ;;
    *)
      fail "no JSON reader available"
      ;;
  esac
}

# save_state NAME VALUE
save_state() { printf '%s' "$2" > "$QA_STATE_DIR/$1"; }

# load_state NAME  (exits with error if missing)
load_state() {
  local f="$QA_STATE_DIR/$1"
  [ -f "$f" ] || fail "state missing: $1 (expected at $f)"
  cat "$f"
}

# have_tool NAME -> 0 if tool on PATH, 1 otherwise.
have_tool() { command -v "$1" >/dev/null 2>&1; }

# redis_cmd ... -> runs redis-cli against the biz redis, using host client
# if installed else docker compose exec fallback.
redis_cmd() {
  if have_tool redis-cli; then
    redis-cli -h "${DSP_E2E_REDIS_HOST}" -p "${DSP_E2E_REDIS_PORT}" \
      -a "${DSP_E2E_REDIS_PASSWORD}" --no-auth-warning "$@"
  else
    # docker compose exec requires stdin in a non-tty context, use -T
    docker compose exec -T redis redis-cli \
      -a "${DSP_E2E_REDIS_PASSWORD}" --no-auth-warning "$@"
  fi
}

# ch_query SQL -> posts SQL to the ClickHouse HTTP interface.
# Uses basic auth via URL user:password@host form.
ch_query() {
  local sql="$1"
  local url="${DSP_E2E_CH_HTTP}/?database=default"
  curl -sS \
    -u "${DSP_E2E_CH_USER}:${DSP_E2E_CH_PASSWORD}" \
    --data-binary "$sql" \
    "$url"
}
