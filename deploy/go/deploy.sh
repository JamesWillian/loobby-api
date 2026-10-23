#!/usr/bin/env bash
# Publica o Go na maquina nova: testa, compila para a arquitetura dela, troca o
# binario e reinicia. Se o servico nao ficar de pe, volta o binario anterior.
#
#     deploy/go/deploy.sh usuario@ip-da-maquina-nova
#
# Pressupoe a maquina ja preparada (deploy/go/README.md, passos 1 a 6).
set -euo pipefail

TARGET="${1:?uso: $0 usuario@ip-da-maquina-nova}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

echo "-- testes --"
(cd "$ROOT/api-go" && go vet ./... && go test ./...)

arch="$(ssh "$TARGET" uname -m)"
case "$arch" in
  x86_64)        GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) echo "arquitetura nao suportada: $arch"; exit 1 ;;
esac

echo "-- build linux/$GOARCH --"
BUILD="$(mktemp -d)"; trap 'rm -rf "$BUILD"' EXIT
(cd "$ROOT/api-go" && GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o "$BUILD/api" ./cmd/api)

echo "-- envio e troca --"
scp -q "$BUILD/api" "$TARGET:/tmp/loobby-api.new"
ssh "$TARGET" 'bash -s' <<'REMOTE'
set -euo pipefail
sudo install -m 0755 -o root -g root /tmp/loobby-api.new /opt/loobby/api.new
rm -f /tmp/loobby-api.new
[ -f /opt/loobby/api ] && sudo cp -p /opt/loobby/api /opt/loobby/api.prev
sudo mv /opt/loobby/api.new /opt/loobby/api

since="$(date '+%Y-%m-%d %H:%M:%S')"
sudo systemctl restart loobby-api
# "active" logo apos o restart nao basta: com Restart=on-failure, um processo que
# morre ao conectar no banco aparece ativo entre uma tentativa e outra. So conta
# como no ar quem escreveu "servidor no ar" no log desde o restart.
for _ in $(seq 1 15); do
  if sudo journalctl -u loobby-api --since "$since" --no-pager -o cat | grep -q 'servidor no ar'; then
    sleep 2
    if systemctl is-active --quiet loobby-api; then echo "servico no ar"; exit 0; fi
  fi
  sleep 1
done

echo "!! o servico nao ficou de pe -- ultimas linhas do log:"
sudo journalctl -u loobby-api --since "$since" --no-pager -o cat | tail -15
if [ -f /opt/loobby/api.prev ]; then
  echo "!! revertendo para o binario anterior"
  sudo cp -p /opt/loobby/api.prev /opt/loobby/api
  sudo systemctl restart loobby-api
fi
exit 1
REMOTE

echo
echo "Publicado. Confira a partir do AWS (deploy/go/README.md, passo 7)."
