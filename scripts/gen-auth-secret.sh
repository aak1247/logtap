#!/usr/bin/env bash
set -euo pipefail

# Generate AUTH_SECRET for logtap.
# Output: base64 (URL-safe, no padding), decoded length >= 32 bytes.
#
# Usage:
#   ./scripts/gen-auth-secret.sh
#   export AUTH_SECRET="$(./scripts/gen-auth-secret.sh)"

bytes="${1:-32}"
if [[ "${bytes}" == "-h" || "${bytes}" == "--help" ]]; then
  echo "Usage: $0 [bytes]"
  echo "Default bytes: 32"
  exit 0
fi

if ! [[ "${bytes}" =~ ^[0-9]+$ ]] || (( bytes < 32 )); then
  echo "error: bytes must be an integer >= 32" >&2
  exit 1
fi

if command -v openssl >/dev/null 2>&1; then
  # URL-safe base64 without padding.
  openssl rand "${bytes}" | openssl base64 -A | tr '+/' '-_' | tr -d '='
  echo
  exit 0
fi

if command -v python3 >/dev/null 2>&1; then
  python3 - "$bytes" <<'PY'
import base64, os, sys
n = int(sys.argv[1])
print(base64.urlsafe_b64encode(os.urandom(n)).decode().rstrip("="))
PY
  exit 0
fi

echo "error: need openssl or python3 to generate secret" >&2
exit 1

