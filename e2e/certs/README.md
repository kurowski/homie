# e2e test certificates

Static TLS fixtures used exclusively by `go test -tags=e2e ./e2e/...`.

The e2e harness stands up a tiny nginx sidecar that pretends to be
`github.com` and `raw.githubusercontent.com` on a private Docker
network. These certs let it speak HTTPS to the test containers
without `curl -k` shenanigans.

- `ca.crt` — self-signed root. Installed into each distro container's
  trust store at test run time.
- `server.crt` — leaf with SANs for both impersonated hostnames,
  signed by the CA. Mounted into nginx.
- `server.key` — leaf private key. Mounted into nginx.

**Why a private key is fine to commit:** the cert is only trusted by
test containers that explicitly install our CA. Public clients
(browsers, real curl, real git) reject it. It is useless outside the
e2e harness.

To regenerate (e.g. on rotation or to add a hostname), run `./gen.sh`.
Currently valid through ~April 2126.
