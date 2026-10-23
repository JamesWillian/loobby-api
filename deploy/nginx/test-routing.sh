#!/usr/bin/env bash
# Prova o roteamento do strangler ANTES de ir para producao.
#
# Sobe o nginx com o loobby-routes.conf REAL e dois backends falsos que so dizem
# quem sao, e confere para qual lado cada requisicao vai. Nao precisa de banco,
# Kotlin nem Go rodando. Cada rota nova no routes.conf deve ganhar casos aqui.
#
#     deploy/nginx/test-routing.sh
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
NGINX="${NGINX:-$(command -v nginx || echo /opt/homebrew/bin/nginx)}"
P_KOTLIN=18080 P_GO=18081 P_PROXY=18090

WORK="$(mktemp -d)"
PIDS=()
cleanup() {
  [[ -f "$WORK/nginx.pid" ]] && kill "$(cat "$WORK/nginx.pid")" 2>/dev/null || true
  for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null || true; done
  rm -rf "$WORK"
}
trap cleanup EXIT

cat > "$WORK/backend.py" <<'PY'
import http.server, sys
name, port = sys.argv[1], int(sys.argv[2])
class H(http.server.BaseHTTPRequestHandler):
    def _r(self):
        self.send_response(200)
        self.send_header("X-Backend", name)
        self.send_header("X-Seen-Uri", self.path)
        self.send_header("X-Seen-Host", self.headers.get("Host", ""))
        self.send_header("Content-Length", "0")
        self.end_headers()
    do_GET = do_HEAD = do_POST = do_PUT = do_PATCH = do_DELETE = do_OPTIONS = _r
    def log_message(self, *a): pass
http.server.ThreadingHTTPServer(("127.0.0.1", port), H).serve_forever()
PY
python3 "$WORK/backend.py" kotlin "$P_KOTLIN" & PIDS+=($!); disown
python3 "$WORK/backend.py" go "$P_GO" & PIDS+=($!); disown

# Espera os backends responderem diretamente. Esperar so o nginx nao basta: ele
# responde 502 enquanto o backend sobe, e um 502 tambem "responde".
for port in "$P_KOTLIN" "$P_GO"; do
  for _ in $(seq 1 100); do
    curl -s -o /dev/null "http://127.0.0.1:$port/" && break; sleep 0.05
  done
done

# Upstreams de teste com os MESMOS nomes do loobby-upstreams.conf; o arquivo de
# rotas e o de producao, sem copia.
cat > "$WORK/nginx.conf" <<CONF
worker_processes 1;
pid $WORK/nginx.pid;
error_log $WORK/error.log warn;
events { worker_connections 64; }
http {
    access_log off;
    upstream loobby-kotlin { server 127.0.0.1:$P_KOTLIN; }
    upstream loobby-go     { server 127.0.0.1:$P_GO; }
    include $HERE/loobby-routes.conf;
    server {
        listen 127.0.0.1:$P_PROXY;
        location / {
            proxy_set_header Host \$host;
            proxy_pass http://\$loobby_backend;
        }
    }
}
CONF

"$NGINX" -t -p "$WORK" -c "$WORK/nginx.conf" -q
"$NGINX" -p "$WORK" -c "$WORK/nginx.conf"
for _ in $(seq 1 50); do
  curl -s -o /dev/null "http://127.0.0.1:$P_PROXY/" 2>/dev/null && break; sleep 0.1
done

FAIL=0
expect() { # $1=backend esperado, $2..=args do curl; o ultimo e o caminho
  local want="$1"; shift
  local route="${*: -1}"; set -- "${@:1:$#-1}"
  local got; got="$(curl -s -o /dev/null -D - "$@" "http://127.0.0.1:$P_PROXY$route" \
    | tr -d '\r' | awk -F': ' 'tolower($1)=="x-backend"{print $2}')"
  if [[ "$got" == "$want" ]]; then printf '  ok   %-7s %s %s\n' "$got" "$*" "$route"
  else printf '  FAIL esperava %s, veio "%s": %s %s\n' "$want" "$got" "$*" "$route"; FAIL=1; fi
}

echo "-- migradas: vao para o Go --"
expect go     -X GET  /legal/privacy
expect go     -I      /legal/privacy
expect go     -X GET  /legal/account-deletion
expect go     -X GET  /legal/child-safety
expect go     -X GET  "/legal/privacy?utm=x"

echo "-- variacoes das migradas: ficam no Kotlin --"
expect kotlin -X POST    /legal/privacy
expect kotlin -X OPTIONS -H "Origin: https://x.com" -H "Access-Control-Request-Method: GET" /legal/privacy
expect kotlin -X GET  /legal/privacy/
expect kotlin -X GET  /legal/privacy-v2
expect kotlin -X GET  /legal/
expect kotlin -X GET  /LEGAL/privacy
# formas nao canonicas: o $uri normalizado casaria; a URI crua, nao
expect kotlin -X GET  /legal//privacy
expect kotlin -X GET  /legal/%70rivacy

echo "-- games: vao para o Go --"
expect go     -X GET  "/games/search?q=zelda&page=1"
expect go     -I      "/games/search?q=zelda"
expect go     -X GET  /games/search
expect go     -X GET  /games/479836
expect go     -X GET  /games/zelda-2
expect go     -X GET  "/games/479836?x=1"

echo "-- variacoes de games: ficam no Kotlin --"
expect kotlin -X POST /games/search
expect kotlin -X GET  /games/search/
expect kotlin -X GET  /games/
expect kotlin -X GET  /games
expect kotlin -X GET  /games/479836/screenshots
expect kotlin -X GET  /games/a%2Fb
expect kotlin -X GET  /games/a.b
expect kotlin -X GET  --path-as-is /games/..

echo "-- nao migradas e internas: ficam no Kotlin --"
expect kotlin -X GET  /users/me
expect kotlin -X GET  /actuator/health
expect kotlin -X GET  /internal/whoami

echo "-- o backend recebe a URI original, com query string --"
seen="$(curl -s -o /dev/null -D - "http://127.0.0.1:$P_PROXY/legal/privacy?utm=x" | tr -d '\r' | awk -F': ' 'tolower($1)=="x-seen-uri"{print $2}')"
if [[ "$seen" == "/legal/privacy?utm=x" ]]; then echo "  ok   uri preservada: $seen"
else echo "  FAIL uri chegou como: $seen"; FAIL=1; fi

echo "-- kill switch: \"go loobby-kotlin\" devolve tudo ao Kotlin --"
# sed -E com classes POSIX: o sed do macOS nao entende \s nem \+, e uma troca
# que nao acontece faria este teste medir a config errada em silencio.
sed -E 's/^([[:space:]]*go[[:space:]]+)loobby-go;/\1loobby-kotlin;/' \
  "$HERE/loobby-routes.conf" > "$WORK/routes-killed.conf"
if cmp -s "$HERE/loobby-routes.conf" "$WORK/routes-killed.conf"; then
  echo "  FAIL a troca do kill switch nao aconteceu -- o formato da linha \"go\" mudou?"; FAIL=1
else
  sed -i.bak "s#$HERE/loobby-routes.conf#$WORK/routes-killed.conf#" "$WORK/nginx.conf"
  "$NGINX" -p "$WORK" -c "$WORK/nginx.conf" -s reload
  # Logo apos o reload, o worker antigo e o novo aceitam conexoes ao mesmo tempo
  # por alguns milissegundos -- o nginx so manda o antigo sair depois que o novo
  # sobe. Um "kotlin" isolado nao prova nada; exige 10 seguidos.
  streak=0
  for _ in $(seq 1 200); do
    backend="$(curl -s -o /dev/null -D - "http://127.0.0.1:$P_PROXY/legal/privacy" | tr -d '\r' | awk -F': ' 'tolower($1)=="x-backend"{print $2}')"
    if [[ "$backend" == "kotlin" ]]; then streak=$((streak+1)); else streak=0; fi
    [[ $streak -ge 10 ]] && break
    sleep 0.02
  done
  expect kotlin -X GET /legal/privacy
  expect kotlin -X GET /legal/child-safety
  expect kotlin -X GET "/games/search?q=zelda"
  expect kotlin -X GET /games/479836
fi

echo
if [[ $FAIL -eq 0 ]]; then echo "roteamento OK"; else echo "roteamento com FALHAS"; exit 1; fi
