# nginx do strangler

O roteamento Kotlin → Go, rota a rota. Hoje há um nginx 1.28.3 na frente da API
(AWS, `54.94.194.109`), fazendo TLS e proxy para o Kotlin. Estes arquivos se
**encaixam** nele; não o substituem.

| Arquivo | Contexto | Muda quando |
|---|---|---|
| `loobby-upstreams.conf` | `http {}` | raramente — endereço dos dois processos |
| `loobby-routes.conf` | `http {}` | **a cada módulo migrado** — é o estado da migração |
| `test-routing.sh` | — | junto com o `routes.conf`: cada rota nova ganha casos |

## A regra

**Rotear por (método, caminho) exatos, nunca por prefixo.** Tudo que não casa vai
para o Kotlin — inclusive as variações das rotas migradas: método errado, barra no
fim, subpath inexistente, preflight CORS. O Go só recebe o que implementa, e o
comportamento de erro dessas variações continua sendo o do Kotlin.

Um `location /legal/` mandaria `POST /legal/privacy` para o Go (405), quando o
Kotlin responde outra coisa — e o cliente perceberia a troca.

## Primeira integração

Eu não tenho a config atual do servidor, então isto é o roteiro, não o diff. O
momento de risco é a troca do `proxy_pass`: por isso ele acontece **com o kill
switch ligado**, sem mandar nada para o Go ainda.

1. **Subir o Go em `:8081`** com as mesmas variáveis de ambiente do Kotlin
   (`DB_URL`, `JWT_SECRET` …) e conferir direto, sem nginx:
   `curl -s localhost:8081/actuator/health` → `{"status":"UP"}`.

2. **Copiar os dois `.conf` para o contexto `http {}`** — em muitas distros,
   `/etc/nginx/conf.d/` já é incluído ali. Ajustar os endereços no upstreams.

3. **Ligar o kill switch antes do primeiro reload:** no `loobby-routes.conf`,
   linha `go`, trocar `loobby-go` por `loobby-kotlin`. Assim a plumbing entra em
   produção sem mudar comportamento nenhum.

4. **No `server {}` de `api.loobby.app`**, trocar o `proxy_pass` fixo por:
   ```nginx
   proxy_set_header Host $host;          # se ainda não existir
   proxy_pass http://$loobby_backend;
   ```
   Mantenha os demais `proxy_set_header` que já existirem.

5. `nginx -t && nginx -s reload`, e provar que nada mudou:
   ```bash
   python3 contract/capture.py --include-uploads --out contract/depois
   python3 contract/compare.py contract/golden contract/depois   # 0 diferenças
   ```

6. **Desligar o kill switch** (`loobby-kotlin` → `loobby-go` na linha `go`),
   `nginx -t && nginx -s reload`, e repetir o passo 5. Zero diferenças = `legal`
   está no Go e nenhum cliente consegue perceber.

Uma mudança por vez entre baselines: se o deploy do PR #1 (correções de status)
e o cutover de uma rota caírem no mesmo intervalo, o `compare` mistura as duas
causas. Regrave a baseline depois de cada uma.

## Kill switch

Na linha `go` do segundo `map`, `loobby-go` → `loobby-kotlin`, e
`nginx -s reload`. Todas as rotas migradas voltam ao Kotlin de uma vez, sem
editar a lista.

Medido no teste: logo após o reload, o worker antigo e o novo aceitam conexões
juntos por alguns milissegundos. Na prática o rollback é imediato.

## Migrar a próxima rota

1. Uma linha nova no primeiro `map` do `loobby-routes.conf`.
2. Casos novos no `test-routing.sh` — a rota e as variações que devem **ficar** no Kotlin.
3. `deploy/nginx/test-routing.sh` verde.
4. Cutover pelos passos 5 e 6 acima.

O teste sobe o nginx com o `routes.conf` real e dois backends falsos; não precisa
de banco, Kotlin nem Go. Requer `nginx` e `python3` no `PATH`.
