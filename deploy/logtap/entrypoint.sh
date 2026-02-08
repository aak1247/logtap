#!/bin/sh
set -eu

GEOIP_DIR="${GEOIP_DIR:-/data/geoip}"
CITY_MMDB="${GEOIP_DIR}/GeoLite2-City.mmdb"
ASN_MMDB="${GEOIP_DIR}/GeoLite2-ASN.mmdb"

MAXMIND_LICENSE_KEY="${MAXMIND_LICENSE_KEY:-}"
MAXMIND_CITY_EDITION_ID="${MAXMIND_CITY_EDITION_ID:-GeoLite2-City}"
MAXMIND_ASN_EDITION_ID="${MAXMIND_ASN_EDITION_ID:-GeoLite2-ASN}"

mkdir -p "${GEOIP_DIR}"

download_mmdb() {
  edition_id="$1"
  out="$2"
  tmp="${out}.tar.gz"

  curl -fsSL "https://download.maxmind.com/app/geoip_download?edition_id=${edition_id}&license_key=${MAXMIND_LICENSE_KEY}&suffix=tar.gz" -o "${tmp}"
  mmdb_path="$(tar -tzf "${tmp}" | awk '/\.mmdb$/ {print; exit}')"
  [ -n "${mmdb_path}" ] || return 1
  tar -xzf "${tmp}" -C "${GEOIP_DIR}" "${mmdb_path}"
  rm -f "${tmp}"
  mv "${GEOIP_DIR}/${mmdb_path}" "${out}"
  rm -rf "${GEOIP_DIR}/$(echo "${mmdb_path}" | cut -d/ -f1)" 2>/dev/null || true
}

if [ -n "${MAXMIND_LICENSE_KEY}" ]; then
  if [ ! -f "${CITY_MMDB}" ]; then
    download_mmdb "${MAXMIND_CITY_EDITION_ID}" "${CITY_MMDB}" || true
  fi
  if [ ! -f "${ASN_MMDB}" ]; then
    download_mmdb "${MAXMIND_ASN_EDITION_ID}" "${ASN_MMDB}" || true
  fi
fi

if [ -z "${GEOIP_CITY_MMDB:-}" ] && [ -f "${CITY_MMDB}" ]; then
  export GEOIP_CITY_MMDB="${CITY_MMDB}"
fi
if [ -z "${GEOIP_ASN_MMDB:-}" ] && [ -f "${ASN_MMDB}" ]; then
  export GEOIP_ASN_MMDB="${ASN_MMDB}"
fi

exec /gateway

