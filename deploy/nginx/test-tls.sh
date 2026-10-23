#!/usr/bin/env bash
# Teste de ponta a ponta da topologia de producao, rodando local:
#
#   cliente -> nginx (config REAL) --TLS mutuo--> Go (binario REAL) -> Postgres
#                                 \--HTTP-------> Kotlin (falso)
#
# Prova que: a resposta do Go atravessando o nginx e IDENTICA ao golden gravado
# da producao; o Go recusa quem nao tem o certificado do nginx; Go fora do ar
# cai no Kotlin so para GET/HEAD; Kotlin fora do ar nao entra em laco; e o kill
# switch tambem troca o esquema (https -> http).
#
#     TEST_DATABASE_URL=postgres://postgres@127.0.0.1:54329/loobby deploy/nginx/test-tls.sh
#
# Requer: go, nginx, python3, curl, openssl, e um Postgres com contract/schema.sql
# aplicado (ver api-go/README.md). O Go so precisa do banco para subir.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
NGX="$ROOT/deploy/nginx"
NGINX="${NGINX:-$(command -v nginx || echo /opt/homebrew/bin/nginx)}"
OPENSSL="${OPENSSL:-$(ls /opt/homebrew/opt/openssl@3/bin/openssl 2>/dev/null || command -v openssl)}"
: "${TEST_DATABASE_URL:?defina TEST_DATABASE_URL (Postgres com contract/schema.sql aplicado)}"
P_KOTLIN=18080 P_GO=18443 P_PROXY=18090

WORK="$(mktemp -d)"
PIDS=()
cleanup() {
  [[ -f "$WORK/nginx.pid" ]] && kill "$(cat "$WORK/nginx.pid")" 2>/dev/null || true
  for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null || true; done
  rm -rf "$WORK"
}
trap cleanup EXIT
FAIL=0
ok()   { printf '  ok   %s\n' "$1"; }
fail() { printf '  FAIL %s\n' "$1"; FAIL=1; }

echo "-- preparando: binario Go, certificados, backends --"
(cd "$ROOT/api-go" && go build -o "$WORK/api" ./cmd/api)
OPENSSL="$OPENSSL" "$ROOT/deploy/certs/gen-certs.sh" "$WORK/certs" >/dev/null
C="$WORK/certs"

cat > "$WORK/kotlin.py" <<'PY'
import http.server, sys
class H(http.server.BaseHTTPRequestHandler):
    def _r(self):
        self.send_response(200)
        self.send_header("X-Backend", "kotlin")
        self.send_header("Content-Length", "0")
        self.end_headers()
    do_GET = do_HEAD = do_POST = do_PUT = do_PATCH = do_DELETE = do_OPTIONS = _r
    def log_message(self, *a): pass
http.server.ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
PY
python3 "$WORK/kotlin.py" "$P_KOTLIN" & PID_KOTLIN=$!; PIDS+=("$PID_KOTLIN"); disown

DB_URL="$TEST_DATABASE_URL" JWT_SECRET=segredo-de-teste RAWG_API_KEY=chave-de-teste PORT="$P_GO" BIND_HOST=127.0.0.1 \
TLS_CERT_FILE="$C/go-server.crt" TLS_KEY_FILE="$C/go-server.key" TLS_CLIENT_CA_FILE="$C/ca.crt" \
  "$WORK/api" > "$WORK/go.log" 2>&1 & PID_GO=$!; PIDS+=("$PID_GO"); disown

# curl direto no Go, se identificando como o nginx
go_direct() {
  curl -s -o /dev/null -w '%{http_code}' --max-time 3 \
    --cacert "$C/ca.crt" --resolve "loobby-go:$P_GO:127.0.0.1" "$@" "https://loobby-go:$P_GO/legal/privacy" || true
}
for _ in $(seq 1 100); do
  [[ "$(go_direct --cert "$C/nginx-client.crt" --key "$C/nginx-client.key")" == 200 ]] && break; sleep 0.1
done
if [[ "$(go_direct --cert "$C/nginx-client.crt" --key "$C/nginx-client.key")" != 200 ]]; then
  echo "o Go nao subiu:"; cat "$WORK/go.log"; exit 1
fi

# nginx com os arquivos REAIS do repo; so enderecos e caminhos trocados
sed -e "s#127.0.0.1:8080#127.0.0.1:$P_KOTLIN#" -e "s#IP_DA_MAQUINA_NOVA:8443#127.0.0.1:$P_GO#" \
  "$NGX/loobby-upstreams.conf" > "$WORK/upstreams.conf"
sed "s#/etc/nginx/loobby/#$C/#" "$NGX/loobby-proxy.conf" > "$WORK/proxy.conf"
cp "$NGX/loobby-routes.conf" "$WORK/routes.conf"
cat > "$WORK/nginx.conf" <<CONF
worker_processes 1;
pid $WORK/nginx.pid;
error_log $WORK/error.log warn;
events { worker_connections 64; }
http {
    access_log off;
    include $WORK/upstreams.conf;
    include $WORK/routes.conf;
    server {
        listen 127.0.0.1:$P_PROXY;
        include $NGX/loobby-fallback.conf;
        location / { include $WORK/proxy.conf; }
    }
}
CONF
"$NGINX" -t -q -p "$WORK" -c "$WORK/nginx.conf"
"$NGINX" -p "$WORK" -c "$WORK/nginx.conf"
reload() { "$NGINX" -p "$WORK" -c "$WORK/nginx.conf" -s reload; sleep 0.6; }

backend() { # imprime "kotlin" se o falso respondeu, senao "go:<status>"
  local h; h="$(curl -s -o /dev/null -D - --max-time 10 "$@" | tr -d '\r')"
  if grep -qi '^x-backend: kotlin' <<<"$h"; then echo kotlin
  else echo "go:$(awk 'NR==1{print $2}' <<<"$h")"; fi
}

echo "-- Go atras do nginx, por TLS mutuo: identico ao golden de producao --"
python3 - "$ROOT" "$P_PROXY" <<'PY' || FAIL=1
import json, sys, urllib.request
root, port = sys.argv[1], sys.argv[2]
sys.path.insert(0, f"{root}/contract")
from capture import kept_headers
bad = 0
for slug in ("privacy", "account-deletion", "child-safety"):
    g = json.load(open(f"{root}/contract/golden/legal-{slug}.json"))["response"]
    with urllib.request.urlopen(f"http://127.0.0.1:{port}/legal/{slug}", timeout=10) as r:
        status, headers, body = r.status, r.headers, r.read().decode()
    diffs = []
    if status != g["status"]: diffs.append(f"status {status} != {g['status']}")
    got = kept_headers(headers)
    for k in sorted(set(got) | set(g["headers"])):
        if got.get(k) != g["headers"].get(k): diffs.append(f"{k}: {got.get(k)!r} != {g['headers'].get(k)!r}")
    if body != g["body_raw"]: diffs.append(f"corpo difere ({len(body)} vs {len(g['body_raw'])} chars)")
    if headers.get("X-Backend"): diffs.append("respondido pelo Kotlin falso, nao pelo Go")
    print(f"  {'ok  ' if not diffs else 'FAIL'} /legal/{slug} via nginx == golden" + "".join(f"\n         {d}" for d in diffs))
    bad |= bool(diffs)
sys.exit(1 if bad else 0)
PY
[[ "$(backend "http://127.0.0.1:$P_PROXY/users/me")" == kotlin ]] && ok "rota nao migrada continua no Kotlin" || fail "rota nao migrada"

echo "-- o Go so fala com quem tem o certificado do nginx --"
[[ "$(go_direct)" == 000 ]] && ok "sem certificado: handshake recusado" || fail "sem certificado foi aceito"
[[ "$(go_direct --cert "$C/go-server.crt" --key "$C/go-server.key")" == 000 ]] \
  && ok "certificado do servidor usado como cliente: recusado" || fail "certificado do servidor aceito como cliente"
code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "http://127.0.0.1:$P_GO/legal/privacy" || true)"
[[ "$code" != 200 ]] && ok "HTTP puro na porta do Go: recusado ($code)" || fail "HTTP puro aceito"

echo "-- kill switch tambem troca o esquema (https -> http) --"
sed -i.bak -E 's/^([[:space:]]*go[[:space:]]+)loobby-go;/\1loobby-kotlin;/' "$WORK/routes.conf"; reload
[[ "$(backend "http://127.0.0.1:$P_PROXY/legal/privacy")" == kotlin ]] && ok "kill switch: legal volta ao Kotlin" || fail "kill switch"
mv "$WORK/routes.conf.bak" "$WORK/routes.conf"; reload
[[ "$(backend "http://127.0.0.1:$P_PROXY/legal/privacy")" == go:200 ]] && ok "kill switch desligado: legal volta ao Go" || fail "religar o Go"

echo "-- Go fora do ar --"
kill "$PID_GO"; sleep 0.3
[[ "$(backend "http://127.0.0.1:$P_PROXY/legal/privacy")" == kotlin ]] && ok "GET cai no Kotlin automaticamente" || fail "GET sem fallback"
[[ "$(backend -I "http://127.0.0.1:$P_PROXY/legal/privacy")" == kotlin ]] && ok "HEAD cai no Kotlin automaticamente" || fail "HEAD sem fallback"
# rota de escrita fingindo estar migrada: NAO pode ser repetida no Kotlin
sed -i.bak 's#^    default  kotlin;#    default  kotlin;\n    "~^POST /legal/privacy$"  go;#' "$WORK/routes.conf"; reload
[[ "$(backend -X POST "http://127.0.0.1:$P_PROXY/legal/privacy")" == go:502 ]] \
  && ok "POST com o Go fora: 502, sem repetir no Kotlin" || fail "POST foi repetido no Kotlin"
mv "$WORK/routes.conf.bak" "$WORK/routes.conf"; reload

echo "-- Kotlin fora do ar --"
kill "$PID_KOTLIN"; sleep 0.3
start=$(date +%s)
[[ "$(backend "http://127.0.0.1:$P_PROXY/users/me")" == go:502 ]] && ok "502 sem laco de fallback ($(( $(date +%s) - start ))s)" || fail "Kotlin fora"

echo
if [[ $FAIL -eq 0 ]]; then echo "topologia de producao OK"; else echo "topologia com FALHAS"; tail -5 "$WORK/error.log"; exit 1; fi
