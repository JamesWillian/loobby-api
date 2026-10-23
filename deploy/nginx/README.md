# nginx do strangler

O roteamento Kotlin → Go, rota a rota. Hoje há um nginx 1.28.3 rodando direto no
servidor do AWS (`54.94.194.109`), fazendo TLS e proxy para o Kotlin em Docker. O
Go roda em **outra máquina** (`deploy/go/README.md`), alcançada por HTTPS com TLS
mútuo. Estes arquivos se **encaixam** no nginx existente; não o substituem.

| Arquivo | Onde incluir | Muda quando |
|---|---|---|
| `loobby-upstreams.conf` | `http {}` | raramente — endereços e esquema dos dois lados |
| `loobby-routes.conf` | `http {}` | **a cada módulo migrado** — é o estado da migração |
| `loobby-proxy.conf` | `location / {}` de `api.loobby.app` | raramente |
| `loobby-fallback.conf` | `server {}` de `api.loobby.app` | raramente |
| `test-routing.sh` | — | junto com o `routes.conf`: cada rota nova ganha casos |
| `test-tls.sh` | — | prova a topologia inteira, local |

## A regra

**Rotear por (método, caminho) exatos, nunca por prefixo.** Tudo que não casa vai
para o Kotlin — inclusive as variações das rotas migradas: método errado, barra no
fim, subpath inexistente, preflight CORS. O Go só recebe o que implementa, e o
comportamento de erro dessas variações continua sendo o do Kotlin.

O casamento é pela URI **crua** (`$request_uri`), não pela normalizada (`$uri`).
O nginx normaliza o `$uri` — decodifica `%XX`, junta barras —, então
`/legal//privacy` e `/legal/%70rivacy` casariam com a rota do Go, embora o
Kotlin as trate de outro jeito. Pela URI crua, só a forma canônica vai para o Go;
o resto fica no Kotlin. Em games, o id aceita só letras, dígitos, `-` e `_`.

## As redes de segurança

- **Kill switch.** Na linha `go` do segundo `map` do `loobby-routes.conf`,
  `loobby-go` → `loobby-kotlin`, e `nginx -s reload`: todas as rotas migradas
  voltam ao Kotlin de uma vez, inclusive o esquema (HTTPS → HTTP).
- **Fallback automático.** Se o Go não responder (máquina fora, rede, firewall),
  o nginx desiste em 3 s e repete a requisição no Kotlin — **só para GET e HEAD**.
  Um POST que talvez já tenha sido gravado pelo Go não é repetido; ele recebe 502.

## Primeira integração

Eu não tenho a config atual do servidor, então isto é o roteiro, não o diff. O
momento de risco é trocar o `proxy_pass`: por isso ele acontece **com o kill
switch ligado**, sem mandar nada para o Go ainda.

1. **Certificados do nginx** (gerados em `deploy/certs/gen-certs.sh`):
   ```bash
   scp ~/loobby-certs/{ca.crt,nginx-client.crt,nginx-client.key} usuario@AWS:/tmp/
   ssh usuario@AWS 'sudo install -d -m 0750 /etc/nginx/loobby
                    sudo install -m 0644 /tmp/ca.crt /tmp/nginx-client.crt /etc/nginx/loobby/
                    sudo install -m 0600 /tmp/nginx-client.key /etc/nginx/loobby/
                    rm -f /tmp/ca.crt /tmp/nginx-client.*'
   ```
   O processo mestre do nginx roda como root e lê a chave na carga da config.

2. **Go no ar na máquina nova** — `deploy/go/README.md`, passos 1 a 7. O passo 7
   confere do AWS que o Go responde e mede a latência entre as máquinas.

3. **Os quatro `.conf`, em dois lugares diferentes:**
   - `loobby-upstreams.conf` e `loobby-routes.conf` → `/etc/nginx/conf.d/`, que
     na maioria das distros já é incluído no `http {}`. No upstreams, ajuste o
     endereço do Kotlin (o mesmo do `proxy_pass` atual) e o IP da máquina nova.
   - `loobby-proxy.conf` e `loobby-fallback.conf` → `/etc/nginx/loobby/`.
     **Nunca no `conf.d/`:** lá eles seriam incluídos no `http {}`, onde
     `proxy_pass` e `location` não são permitidos, e o `nginx -t` falharia.

4. **Kill switch ligado antes do primeiro reload:** no `loobby-routes.conf`, linha
   `go`, troque `loobby-go` por `loobby-kotlin`.

5. **No `server {}` de `api.loobby.app`:**
   ```nginx
   include /etc/nginx/loobby/loobby-fallback.conf;   # no server, fora do location
   location / {
       include /etc/nginx/loobby/loobby-proxy.conf;  # no lugar do proxy_pass atual
       # os proxy_set_header que já existiam continuam aqui
   }
   ```
   Replique os `proxy_set_header` extras também no `loobby-fallback.conf`.

6. `sudo nginx -t && sudo nginx -s reload`, e prove que nada mudou:
   ```bash
   python3 contract/capture.py --include-uploads --out contract/depois
   python3 contract/compare.py contract/golden contract/depois   # 0 diferenças
   ```

7. **Desligue o kill switch** (`loobby-kotlin` → `loobby-go` na linha `go`),
   `nginx -t && nginx -s reload`, e repita o passo 6. Zero diferenças = `legal`
   está no Go, numa máquina separada, e nenhum cliente consegue perceber.

Uma mudança por vez entre baselines: se o deploy do PR #1 e o cutover de uma rota
caírem no mesmo intervalo, o `compare` mistura as duas causas.

## Migrar a próxima rota

1. Uma linha nova no primeiro `map` do `loobby-routes.conf`.
2. Casos novos no `test-routing.sh` — a rota e as variações que devem **ficar** no Kotlin.
3. `deploy/nginx/test-routing.sh` verde.
4. Publicar o Go (`deploy/go/deploy.sh`) e fazer o cutover pelos passos 6 e 7.

## Testes

```bash
deploy/nginx/test-routing.sh     # só o roteamento; sem banco, Kotlin nem Go
TEST_DATABASE_URL=postgres://postgres@127.0.0.1:54329/loobby deploy/nginx/test-tls.sh
```

O `test-tls.sh` sobe o binário Go de verdade com TLS mútuo atrás do nginx com
estes arquivos, e um Kotlin falso. Ele precisa de um Postgres com o schema
aplicado, só para o Go conseguir subir (ver `api-go/README.md`).
