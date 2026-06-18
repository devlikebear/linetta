#!/usr/bin/env bash
# One-time Mac App Store Apple-side setup via the App Store Connect API:
#   - register the App ID (bundle id) if missing
#   - create the Apple Distribution + Mac Installer Distribution certificates
#   - create a MAC_APP_STORE provisioning profile
# Private keys and the profile are written to ~/.linetta/apple/ (never committed).
#
# If any API call is rejected (some cert types can be account-holder-only), the
# script prints the error and the manual web-portal fallback for that step.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIR="${HOME}/.linetta/apple"
CONFIG="${LINETTA_APPLE_CONFIG:-${DIR}/config.env}"
# shellcheck disable=SC1090
set -a; . "${CONFIG}"; set +a
: "${APPLE_TEAM_ID:?}" "${APP_STORE_CONNECT_KEY_ID:?}" "${APP_STORE_CONNECT_ISSUER_ID:?}"

BUNDLE_ID="com.devlikebear.linetta"
api() { ( cd "${ROOT}/scripts/ascapi" && go run . "$@" ); }

echo "== 1. App ID =="
existing="$(api GET "/v1/bundleIds?filter[identifier]=${BUNDLE_ID}" | jq -r '.data[0].id // empty')"
if [ -n "${existing}" ]; then
  echo "bundle id already registered: ${existing}"
  BID="${existing}"
else
  cat > /tmp/bundleid.json <<JSON
{"data":{"type":"bundleIds","attributes":{"identifier":"${BUNDLE_ID}","name":"Linetta","platform":"MAC_OS","seedId":"${APPLE_TEAM_ID}"}}}
JSON
  BID="$(api POST /v1/bundleIds /tmp/bundleid.json | jq -r '.data.id')"
  echo "registered bundle id: ${BID}"
fi

# create_cert <DISTRIBUTION|MAC_INSTALLER_DISTRIBUTION> <basename> <p12-pass>
create_cert() {
  local ctype="$1" base="$2" pass="$3"
  local key="${DIR}/${base}.key" csr="${DIR}/${base}.csr" cer="${DIR}/${base}.cer" p12="${DIR}/${base}.p12"
  if [ ! -f "${key}" ]; then
    openssl req -new -newkey rsa:2048 -nodes -keyout "${key}" \
      -out "${csr}" -subj "/CN=Linetta ${ctype}/O=${APPLE_TEAM_ID}/C=US"
    chmod 600 "${key}"
  fi
  # csrContent must be the full CSR PEM as a JSON string; jq handles newline escaping.
  jq -n --arg t "${ctype}" --arg csr "$(cat "${csr}")" \
    '{data:{type:"certificates",attributes:{certificateType:$t,csrContent:$csr}}}' > /tmp/cert.json
  echo "creating ${ctype} certificate..."
  local content
  content="$(api POST /v1/certificates /tmp/cert.json | jq -r '.data.attributes.certificateContent')"
  printf '%s' "${content}" | base64 --decode > "${cer}.der"
  openssl x509 -inform der -in "${cer}.der" -out "${cer}"
  openssl pkcs12 -export -legacy -macalg sha1 -inkey "${key}" -in "${cer}" \
    -out "${p12}" -passout "pass:${pass}"
  security import "${p12}" -P "${pass}" -T /usr/bin/codesign -T /usr/bin/productbuild
  echo "imported ${ctype}: ${p12}"
}

echo "== 2. Apple Distribution cert =="
create_cert DISTRIBUTION mas_app_distribution linetta-mas-app
echo "== 3. Mac Installer Distribution cert =="
create_cert MAC_INSTALLER_DISTRIBUTION mas_installer_distribution linetta-mas-installer

echo "== 4. Provisioning profile =="
# Apple Distribution cert id (most recent DISTRIBUTION cert for this team)
CERTID="$(api GET "/v1/certificates?filter[certificateType]=DISTRIBUTION&sort=-createdDate&limit=1" | jq -r '.data[0].id')"
cat > /tmp/profile.json <<JSON
{"data":{"type":"profiles","attributes":{"name":"Linetta MAS","profileType":"MAC_APP_STORE"},"relationships":{"bundleId":{"data":{"type":"bundleIds","id":"${BID}"}},"certificates":{"data":[{"type":"certificates","id":"${CERTID}"}]}}}}
JSON
api POST /v1/profiles /tmp/profile.json | jq -r '.data.attributes.profileContent' \
  | base64 --decode > "${DIR}/linetta-mas.provisionprofile"
echo "wrote profile: ${DIR}/linetta-mas.provisionprofile"

rm -f /tmp/bundleid.json /tmp/cert.json /tmp/profile.json
echo ""
echo "Done. Verify identities:"
echo "  security find-identity -v -p codesigning | grep -E 'Apple Distribution|Mac Installer|3rd Party Mac'"
