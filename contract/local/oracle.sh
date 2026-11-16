#!/usr/bin/env bash
# Sobe o Kotlin e o Go lado a lado, na sua maquina, com tudo que eles precisam
# -- Postgres com o schema de producao e servicos externos falsos -- para
# comparar um com o outro sem tocar em producao. Ver contract/local/README.md.
#
#   contract/local/oracle.sh up        sobe tudo (idempotente)
#   contract/local/oracle.sh reset     bancos zerados + mesmo seed nos dois lados, e reinicia Kotlin e Go
#   contract/local/oracle.sh compare   reset + captura dos dois lados + compare.py
#   contract/local/oracle.sh record    compare e, se nao houver diferenca, regrava contract/golden-local/
#   contract/local/oracle.sh go        recompila e reinicia so o Go
#   contract/local/oracle.sh status    o que esta no ar
#   contract/local/oracle.sh down      derruba tudo
#
# Nada daqui e segredo: chaves, senha e JWT_SECRET sao valores de mentira que so
# existem neste ambiente.
set -euo pipefail
export LC_ALL=C # exigencia do Postgres no macOS

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
HERE=$ROOT/contract/local
WORK=${LOOBBY_ORACLE_DIR:-${TMPDIR:-/tmp}/loobby-oracle}
WORK=${WORK%/}
PG_BIN=${PG_BIN:-/opt/homebrew/opt/postgresql@17/bin}
PG_PORT=${PG_PORT:-54329}
K_PORT=${K_PORT:-18080}
G_PORT=${G_PORT:-18081}
F_PORT=${F_PORT:-18100}

K_URL=http://127.0.0.1:$K_PORT
G_URL=http://127.0.0.1:$G_PORT
F_URL=http://127.0.0.1:$F_PORT
SENHA_LOCAL=senha-local-123 # a senha das duas contas do seed.sql

mkdir -p "$WORK"

psql_() { "$PG_BIN/psql" -h 127.0.0.1 -p "$PG_PORT" -U postgres -q -v ON_ERROR_STOP=1 "$@"; }
listening() { lsof -ti "tcp:$1" -sTCP:LISTEN >/dev/null 2>&1; }
kill_port() {
  local pid
  pid=$(lsof -ti "tcp:$1" -sTCP:LISTEN 2>/dev/null | head -1 || true)
  [ -n "$pid" ] && kill "$pid" 2>/dev/null || true
  for _ in $(seq 1 60); do listening "$1" || return 0; sleep 0.25; done
}
wait_http() { # url, tentativas de 0,5 s
  for _ in $(seq 1 "$2"); do
    curl -s -o /dev/null --max-time 2 "$1" && return 0
    sleep 0.5
  done
  return 1
}

# As variaveis que os dois lados leem igual.
common_env() {
  export JWT_SECRET=segredo-local-de-teste-com-mais-de-32-bytes-0123456789
  export DB_USERNAME=postgres DB_PASSWORD=x
  export RESEND_API_KEY=fake-resend-key
  export RAWG_API_KEY=fake-rawg-key RAWG_BASE_URL=$F_URL/rawg
  export GOOGLE_WEB_CLIENT_ID=web-local.apps.googleusercontent.com
  export GOOGLE_IOS_CLIENT_ID=ios-local.apps.googleusercontent.com
  export GOOGLE_ANDROID_CLIENT_ID=android-local.apps.googleusercontent.com
  export NOTIFICATIONS_ENABLED=false
}

# ---------------------------------------------------------------- postgres

pg_up() {
  if [ ! -d "$WORK/pgdata" ]; then
    "$PG_BIN/initdb" -U postgres --auth=trust -E UTF8 --locale=C -D "$WORK/pgdata" >/dev/null
  fi
  if ! "$PG_BIN/pg_isready" -h 127.0.0.1 -p "$PG_PORT" >/dev/null 2>&1; then
    "$PG_BIN/pg_ctl" -D "$WORK/pgdata" -l "$WORK/pg.log" -w \
      -o "-p $PG_PORT -k '' -c listen_addresses=127.0.0.1" start >/dev/null
  fi
}

# mkdb <nome> [seed]: recria o banco com o schema de producao. O pg_trgm nao vem
# no dump, e o "CREATE SCHEMA public" dele ja existe num banco novo (ACHADOS §20).
mkdb() {
  # FORCE: o Kotlin leva um instante para soltar as conexoes depois de parar
  psql_ -c "DROP DATABASE IF EXISTS $1 WITH (FORCE)" -c "CREATE DATABASE $1"
  psql_ -d "$1" -c "CREATE EXTENSION pg_trgm SCHEMA public"
  grep -v '^CREATE SCHEMA public;$' "$ROOT/contract/schema.sql" | psql_ -d "$1" >/dev/null
  if [ "${2:-}" = seed ]; then psql_ -d "$1" -f "$HERE/seed.sql"; fi
}

db_exists() { [ "$(psql_ -tA -c "SELECT 1 FROM pg_database WHERE datname='$1'")" = 1 ]; }

dbs_up() {
  db_exists loobby || mkdb loobby # o banco dos testes Go (TEST_DATABASE_URL)
  db_exists loobby_kotlin || mkdb loobby_kotlin seed
  db_exists loobby_go || mkdb loobby_go seed
}

# ---------------------------------------------------------------- fakes

fakes_up() {
  if [ ! -f "$WORK/google.key" ]; then
    openssl req -x509 -newkey rsa:2048 -nodes -days 3650 -subj "/CN=fake-google" \
      -keyout "$WORK/google.key" -out "$WORK/google.crt" >/dev/null 2>&1
  fi
  listening "$F_PORT" && return 0
  (nohup python3 "$HERE/fakes.py" "$F_PORT" --google-cert "$WORK/google.crt" >"$WORK/fakes.log" 2>&1 &)
  wait_http "$F_URL/emails" 20 || { echo "os servicos falsos NAO subiram:"; tail -20 "$WORK/fakes.log"; exit 1; }
}

# ---------------------------------------------------------------- kotlin

# O Kotlin roda de uma copia de trabalho separada (git worktree), no commit
# atual, com um patch que SO existe ali: enderecos do Resend e das chaves do
# Google configuraveis, para apontarem para os servicos falsos. O codigo do
# repositorio nao e alterado.
kotlin_tree() {
  [ -d "$WORK/kotlin" ] && return 0
  git -C "$ROOT" worktree add --detach "$WORK/kotlin" HEAD >/dev/null 2>&1
  git -C "$WORK/kotlin" apply "$HERE/kotlin-oracle.patch"
}

kotlin_stop() {
  kill_port "$K_PORT"
  pkill -f "$WORK/kotlin.*bootRun" 2>/dev/null || true
}

kotlin_up() {
  listening "$K_PORT" && return 0
  kotlin_tree
  (
    cd "$WORK/kotlin"
    common_env
    export DB_URL=jdbc:postgresql://127.0.0.1:$PG_PORT/loobby_kotlin
    export SERVER_PORT=$K_PORT APP_BASE_URL=$K_URL
    export RESEND_BASE_URL=$F_URL/resend-kotlin GOOGLE_CERTS_URL=$F_URL/google/v1/certs
    # idioma e fuso da imagem de producao (eclipse-temurin: en_US, UTC)
    export JAVA_TOOL_OPTIONS='-Duser.language=en -Duser.country=US -Duser.timezone=UTC'
    nohup ./gradlew bootRun --console=plain >"$WORK/kotlin.log" 2>&1 &
  )
  echo "subindo o Kotlin (a primeira vez compila; pode levar alguns minutos)..."
  wait_http "$K_URL/actuator/health" 600 || { echo "o Kotlin NAO subiu:"; tail -30 "$WORK/kotlin.log"; exit 1; }
}

# ---------------------------------------------------------------- go

go_stop() { kill_port "$G_PORT"; }

go_up() {
  listening "$G_PORT" && return 0
  (cd "$ROOT/api-go" && go build -o "$WORK/loobby-go" ./cmd/api)
  (
    common_env
    export DB_URL=jdbc:postgresql://127.0.0.1:$PG_PORT/loobby_go
    export PORT=$G_PORT APP_BASE_URL=$G_URL
    export RESEND_BASE_URL=$F_URL/resend-go GOOGLE_CERTS_URL=$F_URL/google/v3/certs
    export FILE_STORAGE_LOCATION=$WORK/go-uploads
    nohup "$WORK/loobby-go" >"$WORK/go.log" 2>&1 &
  )
  wait_http "$G_URL/actuator/health" 40 || { echo "o Go NAO subiu:"; tail -30 "$WORK/go.log"; exit 1; }
}

# ---------------------------------------------------------------- comandos

up() {
  pg_up
  fakes_up
  dbs_up
  kotlin_up
  go_up
  status
}

reset() {
  pg_up
  fakes_up
  kotlin_stop
  go_stop
  mkdb loobby_kotlin seed
  mkdb loobby_go seed
  rm -rf "$WORK/go-uploads" "$WORK/kotlin/uploads"
  curl -s -o /dev/null -X DELETE "$F_URL/emails"
  kotlin_up
  go_up
}

# Credenciais que o capture.py e o flows.py leem. Um arquivo por lado, porque o
# capture.py regrava nele o refresh token do 2o usuario -- e nunca no .env da
# raiz, que e o de producao.
env_file() { # lado, url
  cat >"$WORK/env-$1" <<EOF
LOOBBY_BASE_URL=$2
LOOBBY_TEST_EMAIL=user@loobby.app
LOOBBY_TEST_PASSWORD=$SENHA_LOCAL
LOOBBY_TEST2_EMAIL=contract-suite@loobby.app
LOOBBY_TEST2_PASSWORD=$SENHA_LOCAL
LOOBBY_TEST2_USER_ID=61d1ba14-2a43-4e31-a6b8-2ed31cadbcdf
EOF
}

capture_side() { # lado, url, banco
  local out=$WORK/cap-$1
  rm -rf "$out"
  env_file "$1" "$2"
  (
    cd "$ROOT"
    env -i PATH="$PATH" HOME="$HOME" python3 contract/capture.py \
      --env-file "$WORK/env-$1" --out "$out" --include-uploads >"$WORK/capture-$1.log" 2>&1
    env -i PATH="$PATH" HOME="$HOME" python3 contract/flows.py \
      --base-url "$2" --out "$out" --mailbox "$F_URL" --side "$1" \
      --google-key "$WORK/google.key" --psql "$PG_BIN/psql" \
      --db-url "postgres://postgres@127.0.0.1:$PG_PORT/$3" >"$WORK/flows-$1.log" 2>&1
  ) || { echo "a captura do lado $1 falhou:"; tail -30 "$WORK/capture-$1.log" "$WORK/flows-$1.log"; exit 1; }
}

compare() {
  reset
  capture_side kotlin "$K_URL" loobby_kotlin
  capture_side go "$G_URL" loobby_go
  echo "== Kotlin local x Go local =="
  python3 "$ROOT/contract/compare.py" "$WORK/cap-kotlin" "$WORK/cap-go"
}

# A captura do Kotlin vira a referencia que o `go test` usa (contract/golden-local).
# So regrava quando os dois lados concordam: uma referencia que o Go ja nao
# iguala deixaria o teste vermelho sem ninguem ter mexido no Go.
record() {
  compare
  rm -f "$ROOT"/contract/golden-local/*.json
  mkdir -p "$ROOT/contract/golden-local"
  cp "$WORK"/cap-kotlin/*.json "$ROOT/contract/golden-local/"
  echo "contract/golden-local/ regravado: $(ls "$ROOT/contract/golden-local" | wc -l | tr -d ' ') golden"
}

status() {
  for item in "postgres:$PG_PORT" "fakes:$F_PORT" "kotlin:$K_PORT" "go:$G_PORT"; do
    if listening "${item#*:}"; then echo "${item%%:*} no ar em 127.0.0.1:${item#*:}"; else echo "${item%%:*} PARADO"; fi
  done
  echo "pasta de trabalho: $WORK"
}

down() {
  kotlin_stop
  go_stop
  kill_port "$F_PORT"
  if [ -d "$WORK/pgdata" ]; then "$PG_BIN/pg_ctl" -D "$WORK/pgdata" -m fast stop >/dev/null 2>&1 || true; fi
  status
}

case "${1:-}" in
  up) up ;;
  reset) reset; status ;;
  compare) compare ;;
  record) record ;;
  go) pg_up; fakes_up; dbs_up; go_stop; go_up; status ;;
  status) status ;;
  down) down ;;
  *) sed -n '2,14p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
