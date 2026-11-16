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

echo "-- internas: ficam no Kotlin --"
expect kotlin -X GET  /actuator/health
expect kotlin -X GET  /internal/whoami

echo "-- o backend recebe a URI original, com query string --"
seen="$(curl -s -o /dev/null -D - "http://127.0.0.1:$P_PROXY/legal/privacy?utm=x" | tr -d '\r' | awk -F': ' 'tolower($1)=="x-seen-uri"{print $2}')"
if [[ "$seen" == "/legal/privacy?utm=x" ]]; then echo "  ok   uri preservada: $seen"
else echo "  FAIL uri chegou como: $seen"; FAIL=1; fi

echo "-- modulos ainda no Kotlin, segundo o painel do arquivo real --"
U=54f29b67-cb8b-4435-8b72-43c17f2f4b8f
side_of() { awk -v m="$1" '$1==m && ($2=="go;" || $2=="kotlin;") {gsub(";","",$2); print $2}' "$HERE/loobby-routes.conf"; }
for pair in "files:POST /users/me/avatar" "notifications:POST /devices/register" "groups:GET /groups" \
            "users:GET /users/feed" "events:POST /events/instant" "teams:GET /events/$U/teams" "auth:POST /auth/login"; do
  mod="${pair%%:*}"; req="${pair#*:}"
  expect "$(side_of "$mod")" -X "${req%% *}" "${req#* }"
done

echo "-- ensaio geral: todos os modulos ligados no painel --"
sed -E 's/^(    [a-z]+ +)kotlin;$/\1go;/' "$HERE/loobby-routes.conf" | sed -E 's/^(    default +)go;$/\1kotlin;/' > "$WORK/routes-all.conf"
sed -i.bak "s#$HERE/loobby-routes.conf#$WORK/routes-all.conf#" "$WORK/nginx.conf"
"$NGINX" -t -p "$WORK" -c "$WORK/nginx.conf" -q
"$NGINX" -p "$WORK" -c "$WORK/nginx.conf" -s reload
streak=0
for _ in $(seq 1 200); do
  backend="$(curl -s -o /dev/null -D - -X POST "http://127.0.0.1:$P_PROXY/auth/login" | tr -d '\r' | awk -F': ' 'tolower($1)=="x-backend"{print $2}')"
  if [[ "$backend" == "go" ]]; then streak=$((streak+1)); else streak=0; fi
  [[ $streak -ge 10 ]] && break
  sleep 0.02
done
T=$U/teams/61d1ba14-2a43-4e31-a6b8-2ed31cadbcdf
for req in \
  "GET /files/avatars/$U/1789488875123.png" "HEAD /files/groups/$U/1.webp" "POST /users/me/avatar" "POST /groups/$U/image" \
  "POST /devices/register" "DELETE /devices/fMEP0vJqS0:APA91bH-abc_123" "DELETE /devices/fMEP0vJqS0%3AAPA91bH" \
  "POST /groups" "GET /groups" "GET /groups/$U" "PATCH /groups/$U" "DELETE /groups/$U" \
  "GET /groups/invite/\$-2ISBVV" "GET /groups/invite/%24-2ISBVV" "POST /groups/$U/members" "GET /groups/$U/members" \
  "DELETE /groups/$U/members/me" "DELETE /groups/$U/members/$U" \
  "GET /users/me" "PATCH /users/me" "DELETE /users/me" "GET /users/feed" "PUT /users/me/password" \
  "POST /groups/$U/events" "GET /groups/$U/events" "POST /events/instant" "GET /events/$U" "PUT /events/$U" "DELETE /events/$U" \
  "GET /events/invite/\$-AB12-CD34" "PUT /events/$U/rsvps" "GET /events/$U/rsvps" "GET /events/$U/rsvps/me" "DELETE /events/$U/rsvps/me" \
  "POST /events/$U/teams" "GET /events/$U/teams" "POST /events/$U/teams/auto-generate" "PUT /events/$T" "DELETE /events/$T" \
  "POST /events/$T/players" "PUT /events/$T/players/$U" "DELETE /events/$T/players/$U" \
  "POST /auth/anonymous" "POST /auth/register" "POST /auth/login" "POST /auth/google" "POST /auth/refresh" \
  "POST /auth/resend-verification" "POST /auth/forgot-password" "POST /auth/reset-password" \
  "GET /auth/verify-email?token=$U" "GET /auth/reset-password?token=$U"; do
  if [[ "${req%% *}" == HEAD ]]; then expect go -I "${req#* }"; else expect go -X "${req%% *}" "${req#* }"; fi
done

echo "-- ensaio geral: variacoes ficam no Kotlin --"
for req in \
  "GET /files/avatars/$U/1.gif" "GET /files/outra/$U/1.png" "GET /files/avatars/$U/" "GET /files" "POST /files/avatars/$U/1.png" \
  "GET /users/me/avatar" "GET /devices/register" "DELETE /devices/" "DELETE /devices/a%2Fb" \
  "PUT /groups" "GET /groups/" "GET /groups/xyz" "GET /groups/1-1-1-1-1" "GET /groups/invite/" "GET /groups/invite/a%2Fb" \
  "DELETE /groups/$U/members" "GET /groups/$U/members/me" \
  "POST /users/me" "GET /users/me/" "GET /users/me/password" "GET /users" \
  "GET /events/instant" "POST /events/$U" "GET /events" "DELETE /events/$U/rsvps" "PUT /events/$U/rsvps/me" \
  "GET /events/$U/teams/auto-generate" "GET /events/$T" "GET /events/$T/players" "POST /events/$T/players/$U" \
  "GET /auth/login" "POST /auth/verify-email" "POST /auth/login/" "POST /auth/logout" "GET /auth" \
  "GET /actuator/health" "GET /nada"; do
  expect kotlin -X "${req%% *}" "${req#* }"
done
expect kotlin -X OPTIONS -H "Origin: https://x.com" -H "Access-Control-Request-Method: POST" /auth/login
HERE_ROUTES="$WORK/routes-all.conf"

echo "-- kill switch: \"go loobby-kotlin\" devolve tudo ao Kotlin --"
# sed -E com classes POSIX: o sed do macOS nao entende \s nem \+, e uma troca
# que nao acontece faria este teste medir a config errada em silencio.
sed -E 's/^([[:space:]]*go[[:space:]]+)loobby-go;/\1loobby-kotlin;/' \
  "$HERE_ROUTES" > "$WORK/routes-killed.conf"
if cmp -s "$HERE_ROUTES" "$WORK/routes-killed.conf"; then
  echo "  FAIL a troca do kill switch nao aconteceu -- o formato da linha \"go\" mudou?"; FAIL=1
else
  sed -i.bak "s#$HERE_ROUTES#$WORK/routes-killed.conf#" "$WORK/nginx.conf"
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
  expect kotlin -X POST /auth/login
  expect kotlin -X GET /users/me
fi

echo
if [[ $FAIL -eq 0 ]]; then echo "roteamento OK"; else echo "roteamento com FALHAS"; exit 1; fi
