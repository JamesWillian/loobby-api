#!/usr/bin/env bash
# Sobe o Go na sua maquina com tudo que ele precisa -- Postgres com o schema de
# producao e servicos externos falsos -- sem tocar em producao. Ver
# contract/local/README.md.
#
#   contract/local/oracle.sh up        sobe tudo (idempotente)
#   contract/local/oracle.sh reset     banco do Go zerado + seed, e reinicia o Go
#   contract/local/oracle.sh compare   reset + captura do Go + compare.py contra contract/golden-local/
#   contract/local/oracle.sh go        recompila e reinicia so o Go
#   contract/local/oracle.sh status    o que esta no ar
#   contract/local/oracle.sh down      derruba tudo
#
# O Kotlin, que gravou o contract/golden-local/, foi arquivado: a versao deste
# script que o sobe ao lado do Go (e o `record`) ficou na tag arquivo/v1.
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
G_PORT=${G_PORT:-18081}
F_PORT=${F_PORT:-18100}

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

# migrate_ <nome>: aplica as migracoes que faltam (api-go/cmd/migrate).
MIGRATE_BUILT=
migrate_() {
  if [ -z "$MIGRATE_BUILT" ]; then
    (cd "$ROOT/api-go" && go build -o "$WORK/loobby-migrate" ./cmd/migrate)
    MIGRATE_BUILT=1
  fi
  MIGRATE_DATABASE_URL="postgres://postgres@127.0.0.1:$PG_PORT/$1" "$WORK/loobby-migrate" up >/dev/null
}

# mkdb <nome> [seed]: recria o banco, com as migracoes.
mkdb() {
  # FORCE: derruba as conexoes de um Go que ainda nao terminou de parar
  psql_ -c "DROP DATABASE IF EXISTS $1 WITH (FORCE)" -c "CREATE DATABASE $1"
  migrate_ "$1"
  if [ "${2:-}" = seed ]; then psql_ -d "$1" -f "$HERE/seed.sql"; fi
}

db_exists() { [ "$(psql_ -tA -c "SELECT 1 FROM pg_database WHERE datname='$1'")" = 1 ]; }

# db_up <nome> [seed]: leva o banco a ultima migracao. O que nao existe -- ou
# que vem do tempo em que o schema saia do dump, sem o historico do goose -- e
# recriado: aqui nada e dado de verdade.
db_up() {
  if db_exists "$1" && migrate_ "$1" 2>/dev/null; then return 0; fi
  mkdb "$1" "${2:-}"
}

dbs_up() {
  db_up loobby # o banco dos testes Go (TEST_DATABASE_URL)
  db_up loobby_go seed
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

# ---------------------------------------------------------------- go

go_stop() { kill_port "$G_PORT"; }

go_up() {
  listening "$G_PORT" && return 0
  (cd "$ROOT/api-go" && go build -o "$WORK/loobby-go" ./cmd/api)
  (
    export JWT_SECRET=segredo-local-de-teste-com-mais-de-32-bytes-0123456789
    export DB_URL=jdbc:postgresql://127.0.0.1:$PG_PORT/loobby_go DB_USERNAME=postgres DB_PASSWORD=x
    export PORT=$G_PORT APP_BASE_URL=$G_URL
    export RESEND_API_KEY=fake-resend-key RESEND_BASE_URL=$F_URL/resend-go
    export RAWG_API_KEY=fake-rawg-key RAWG_BASE_URL=$F_URL/rawg
    export GOOGLE_WEB_CLIENT_ID=web-local.apps.googleusercontent.com
    export GOOGLE_IOS_CLIENT_ID=ios-local.apps.googleusercontent.com
    export GOOGLE_ANDROID_CLIENT_ID=android-local.apps.googleusercontent.com
    export GOOGLE_CERTS_URL=$F_URL/google/v3/certs
    export FILE_STORAGE_LOCATION=$WORK/go-uploads
    export NOTIFICATIONS_ENABLED=false
    nohup "$WORK/loobby-go" >"$WORK/go.log" 2>&1 &
  )
  wait_http "$G_URL/actuator/health" 40 || { echo "o Go NAO subiu:"; tail -30 "$WORK/go.log"; exit 1; }
}

# ---------------------------------------------------------------- comandos

up() {
  pg_up
  fakes_up
  dbs_up
  go_up
  status
}

reset() {
  pg_up
  fakes_up
  go_stop
  mkdb loobby_go seed
  rm -rf "$WORK/go-uploads"
  curl -s -o /dev/null -X DELETE "$F_URL/emails"
  go_up
}

# Credenciais que o capture.py e o flows.py leem. Ficam aqui, porque o
# capture.py regrava nelas o refresh token do 2o usuario -- e nunca no .env da
# raiz, que e o de producao.
env_file() {
  cat >"$WORK/env-go" <<EOF
LOOBBY_BASE_URL=$G_URL
LOOBBY_TEST_EMAIL=user@loobby.app
LOOBBY_TEST_PASSWORD=$SENHA_LOCAL
LOOBBY_TEST2_EMAIL=contract-suite@loobby.app
LOOBBY_TEST2_PASSWORD=$SENHA_LOCAL
LOOBBY_TEST2_USER_ID=61d1ba14-2a43-4e31-a6b8-2ed31cadbcdf
EOF
}

capture() {
  local out=$WORK/cap-go
  rm -rf "$out"
  env_file
  (
    cd "$ROOT"
    env -i PATH="$PATH" HOME="$HOME" python3 contract/capture.py \
      --env-file "$WORK/env-go" --out "$out" --include-uploads >"$WORK/capture-go.log" 2>&1
    env -i PATH="$PATH" HOME="$HOME" python3 contract/flows.py \
      --base-url "$G_URL" --out "$out" --mailbox "$F_URL" --side go \
      --google-key "$WORK/google.key" --psql "$PG_BIN/psql" \
      --db-url "postgres://postgres@127.0.0.1:$PG_PORT/loobby_go" >"$WORK/flows-go.log" 2>&1
  ) || { echo "a captura falhou:"; tail -30 "$WORK/capture-go.log" "$WORK/flows-go.log"; exit 1; }
}

# O mesmo criterio do TestContratoIgualAoKotlin, com o Go no ar para inspecionar.
compare() {
  reset
  capture
  echo "== contract/golden-local x Go local =="
  python3 "$ROOT/contract/compare.py" "$ROOT/contract/golden-local" "$WORK/cap-go"
}

status() {
  for item in "postgres:$PG_PORT" "fakes:$F_PORT" "go:$G_PORT"; do
    if listening "${item#*:}"; then echo "${item%%:*} no ar em 127.0.0.1:${item#*:}"; else echo "${item%%:*} PARADO"; fi
  done
  echo "pasta de trabalho: $WORK"
}

down() {
  go_stop
  kill_port "$F_PORT"
  if [ -d "$WORK/pgdata" ]; then "$PG_BIN/pg_ctl" -D "$WORK/pgdata" -m fast stop >/dev/null 2>&1 || true; fi
  status
}

case "${1:-}" in
  up) up ;;
  reset) reset; status ;;
  compare) compare ;;
  go) pg_up; fakes_up; dbs_up; go_stop; go_up; status ;;
  status) status ;;
  down) down ;;
  *) sed -n '2,17p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
