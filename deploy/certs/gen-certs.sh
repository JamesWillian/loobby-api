#!/usr/bin/env bash
# Gera a CA privada e os dois certificados do TLS mutuo entre o nginx (AWS) e o
# Go (maquina nova).
#
#     deploy/certs/gen-certs.sh <diretorio-de-saida>
#
# RODE NA SUA MAQUINA, NAO NUM SERVIDOR. A chave da CA (ca.key) e o que permite
# emitir certificados que o Go aceita; ela nao deve estar nem no AWS nem na
# maquina nova. Guarde-a fora dos dois (gerenciador de senhas, cofre).
#
# Saida:
#   ca.crt / ca.key                 CA privada (10 anos)        -- ca.key fica com voce
#   go-server.crt / go-server.key   certificado do Go           -> maquina nova
#   nginx-client.crt / .key         certificado do nginx        -> AWS
#   ca.crt                          tambem vai para os DOIS servidores
#
# Os certificados das pontas valem 2 anos. Para renovar, rode de novo com a mesma
# CA:  REUSE_CA=1 deploy/certs/gen-certs.sh <mesmo-diretorio>
set -euo pipefail

OUT="${1:?uso: $0 <diretorio-de-saida>}"
OPENSSL="${OPENSSL:-openssl}"
CA_DAYS=3650 LEAF_DAYS=730

mkdir -p "$OUT"
cd "$OUT"
umask 077

cnf() { # $1=arquivo $2=secao com as extensoes
  cat > "$1" <<CNF
[req]
distinguished_name = dn
prompt = no
[dn]
# o LibreSSL do macOS recusa secao vazia; o -subj de cada comando sobrescreve
CN = loobby
[ca]
basicConstraints = critical, CA:TRUE, pathlen:0
keyUsage = critical, keyCertSign, cRLSign
subjectKeyIdentifier = hash
[server]
basicConstraints = critical, CA:FALSE
keyUsage = critical, digitalSignature
extendedKeyUsage = serverAuth
subjectAltName = DNS:loobby-go
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid
[client]
basicConstraints = critical, CA:FALSE
keyUsage = critical, digitalSignature
extendedKeyUsage = clientAuth
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid
CNF
}
cnf ext.cnf

key() { "$OPENSSL" ecparam -name prime256v1 -genkey -noout -out "$1" 2>/dev/null; }

if [[ "${REUSE_CA:-0}" == "1" ]]; then
  [[ -f ca.key && -f ca.crt ]] || { echo "REUSE_CA=1 mas ca.key/ca.crt nao existem em $OUT"; exit 1; }
  echo "reaproveitando a CA existente"
else
  [[ -e ca.key ]] && { echo "$OUT/ca.key ja existe -- use REUSE_CA=1 para renovar so as pontas"; exit 1; }
  key ca.key
  "$OPENSSL" req -x509 -new -key ca.key -sha256 -days "$CA_DAYS" \
    -subj "/CN=Loobby strangler CA" -config ext.cnf -extensions ca -out ca.crt
fi

leaf() { # $1=nome-do-arquivo $2=CN $3=secao
  key "$1.key"
  "$OPENSSL" req -new -key "$1.key" -subj "/CN=$2" -config ext.cnf -out "$1.csr"
  "$OPENSSL" x509 -req -in "$1.csr" -CA ca.crt -CAkey ca.key -CAcreateserial \
    -days "$LEAF_DAYS" -sha256 -extfile ext.cnf -extensions "$3" -out "$1.crt" 2>/dev/null
  rm -f "$1.csr"
}
leaf go-server    loobby-go    server
leaf nginx-client loobby-nginx client
rm -f ext.cnf ca.srl

# Confere a cadeia antes de entregar.
"$OPENSSL" verify -CAfile ca.crt go-server.crt nginx-client.crt >/dev/null
chmod 600 ./*.key
chmod 644 ./*.crt

echo "certificados em $OUT:"
for f in ca go-server nginx-client; do
  printf '  %-17s %s\n' "$f.crt" "$("$OPENSSL" x509 -in "$f.crt" -noout -enddate | cut -d= -f2)"
done
echo
echo "  maquina nova (Go) : ca.crt, go-server.crt, go-server.key"
echo "  AWS (nginx)       : ca.crt, nginx-client.crt, nginx-client.key"
echo "  so com voce       : ca.key   <- nao copie para servidor nenhum"
