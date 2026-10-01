#!/usr/bin/env bash
set -euo pipefail

image=${1:?image reference required}
temp=$(mktemp -d)
container="axlr-smoke-$$"
cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  # The non-root container creates private state directories owned by its UID.
  sudo -n rm -rf -- "$temp" || rm -rf -- "$temp"
}
trap cleanup EXIT

mkdir -p "$temp/state"
chmod 755 "$temp"
chmod 777 "$temp/state"
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$temp/tls.key" -out "$temp/tls.crt" -days 1 -subj '/CN=localhost' -addext 'subjectAltName=DNS:localhost' >/dev/null 2>&1
printf '{"version":1,"entries":[]}\n' > "$temp/principals.json"
printf 'smoke-placeholder\n' > "$temp/model-key"
chmod 644 "$temp/tls.key" "$temp/tls.crt" "$temp/principals.json" "$temp/model-key"
cat > "$temp/service.json" <<'JSON'
{
  "api_listen": "0.0.0.0:8443",
  "probe_listen": "127.0.0.1:8081",
  "workspace": "/workspace",
  "state_dir": "/state",
  "server_cert_file": "/smoke/tls.crt",
  "server_key_file": "/smoke/tls.key",
  "client_ca_file": "/smoke/tls.crt",
  "principals_file": "/smoke/principals.json",
  "model_api_key_file": "/smoke/model-key",
  "kmp": {"endpoint":"127.0.0.1:65534","server_name":"kmp.invalid","tls_dir":"/smoke","command":"/usr/local/bin/kmp-mcp"},
  "made": {"endpoint":"127.0.0.1:65533","server_name":"made.invalid","tls_dir":"/smoke","command":"/usr/local/bin/made-mcp"}
}
JSON
docker run -d --name "$container" -p 127.0.0.1::8443 --mount "type=bind,src=$temp,dst=/smoke,readonly" --mount "type=bind,src=$temp/state,dst=/state" "$image" --config=/smoke/service.json >/dev/null
ready=0
for _ in $(seq 1 30); do
  if docker exec "$container" /usr/local/bin/axlr-serve --probe=livez >/dev/null 2>&1; then ready=1; break; fi
  sleep 1
done
if [[ "$ready" != 1 ]]; then docker logs "$container"; exit 1; fi
if docker exec "$container" /usr/local/bin/axlr-serve --probe=readyz; then
  echo 'readyz passed without engines' >&2
  exit 1
fi
port=$(docker port "$container" 8443/tcp | sed -n 's/.*://p')
status=$(curl --insecure --silent --output /dev/null --write-out '%{http_code}' "https://127.0.0.1:$port/v1/tools" || true)
if [[ "$status" != 000 ]]; then
  echo "API accepted a request without a client certificate: $status" >&2
  exit 1
fi
echo 'image smoke passed: livez, failing readyz, client certificate required'
