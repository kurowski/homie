#!/usr/bin/env bash
# Regenerate the e2e TLS chain. Run from this directory.
#
# Produces:
#   ca.crt       — self-signed root, installed into each test container's
#                  trust store at run time
#   server.crt   — leaf cert with SANs for github.com and
#                  raw.githubusercontent.com (the hostnames the e2e
#                  nginx sidecar impersonates), signed by the CA above
#   server.key   — leaf private key, mounted into nginx
#
# Validity: 36500 days (~100y). These certs are fixture data — they only
# matter inside the per-run docker network and are useless against real
# github.com because no public client trusts our CA.
set -euo pipefail

cd "$(dirname "$0")"

# 1. Root CA.
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
  -keyout ca.key -out ca.crt -days 36500 -nodes \
  -subj "/CN=homie-e2e-ca"

# 2. Leaf key + CSR.
openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
  -keyout server.key -out server.csr -nodes \
  -subj "/CN=github.com"

# 3. Sign the leaf with the CA, attaching SANs for both vhosts.
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out server.crt -days 36500 \
  -extfile <(printf "subjectAltName=DNS:github.com,DNS:raw.githubusercontent.com\nextendedKeyUsage=serverAuth\n")

# 4. Tidy — we don't commit the CA key, CSR, or serial.
rm -f ca.key ca.srl server.csr
