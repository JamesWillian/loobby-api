# api-go — o lado Go da Loobby API

Roda ao lado do Kotlin durante o *strangler*, atrás do nginx, que encaminha rota a
rota (`../deploy/nginx/`). Os dois compartilham banco e `JWT_SECRET`, então cada
rota é migrável e reversível isoladamente.

Ordem: `legal → games → files → notifications → groups → users → events → auth`.

## Estado

| Módulo | Situação |
|---|---|
| fundação (config, pool, JWT, envelope de erro, headers do Spring) | ✅ |
| `legal` — 3 rotas | ✅ implementado e igual aos golden · **aguardando cutover no nginx** |
| demais | — |

## Pacotes

| Pacote | O quê |
|---|---|
| `cmd/api` | `main`: config, pool, servidor, shutdown gracioso |
| `internal/server` | **a cadeia HTTP completa** — fica fora do `main` para os testes exercitarem a mesma fiação que vai para produção |
| `internal/config` | env vars — **as mesmas** do `application.yaml` do Kotlin |
| `internal/db` | pool `pgx`, 5 conexões |
| `internal/httpx` | envelope de erro, `Instant`, `Marshal` e `SpringHeaders` |
| `internal/auth` | middleware JWT HS256 |
| `internal/health` | `GET /actuator/health` |
| `internal/legal` | `GET /legal/{privacy,account-deletion,child-safety}` |
| `internal/contracttest` | compara uma resposta Go com o golden do Kotlin |

## Rodar

Mesmas variáveis do Kotlin: `DB_URL` (JDBC ou formato pgx), `JWT_SECRET`, e
`DB_USERNAME`/`DB_PASSWORD` se a URL não trouxer credenciais. Porta: `PORT`
(o nginx espera `8081`).

```bash
go test ./...
PORT=8081 go run ./cmd/api
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o api ./cmd/api
```

## Como cada módulo é provado

`internal/contracttest` carrega um golden de `../contract/golden/` e exige que a
resposta Go bata em **status, headers de contrato (nos dois sentidos) e corpo**:
texto byte a byte, binário por sha256. Um módulo está pronto para cutover quando
os testes dele em `internal/server` passam contra os golden das suas rotas.

Os testes têm dentes — cada mutação abaixo foi aplicada e pega:

- `Content-Type` no formato que o Go farejaria (`text/html; charset=utf-8`)
- `Cache-Control` diferente
- esquecer o `SpringHeaders` na cadeia
- `X-XSS-Protection: 1; mode=block` em vez do `0` do Spring Security 6
- o `\n` que o `json.Encoder` acrescenta
- `/actuator/health` como `application/json`

## Armadilhas do lado Go, todas travadas em teste

**O `encoding/json` padrão não serializa como o Jackson.** O `Encoder` acrescenta
`\n` no fim (`{"status":"UP"}\n`), e o `Marshal` escapa HTML (`"Tom & Jerry"` →
`"Tom & Jerry"`). Use sempre `httpx.Marshal` / `httpx.WriteJSON`.

**O timestamp não é o default do Go.** O `ISO_INSTANT` do Jackson agrupa a fração
em 0, 3, 6 ou 9 dígitos; o `RFC3339Nano` corta *todos* os zeros (`.500` → `.5`).
Daí o tipo `httpx.Instant`.

**Todo response leva os headers do Spring Security**, inclusive o 401 que o
filtro devolve antes de qualquer controller. Por isso `SpringHeaders` é o
middleware mais externo e envolve o JWT. Ver `contract/ACHADOS.md` §16.

**O `Content-Type` é literal.** `/legal/*` é `text/html;charset=UTF-8` — sem
espaço, `UTF-8` maiúsculo. O health é `application/vnd.spring-boot.actuator.v3+json`.

**Refresh token é aceito como access token.** O Kotlin não checa o claim `type`
(medido: `/users/me` com o refresh no `Bearer` → 200). O Go replica por padrão;
`ACCEPT_REFRESH_AS_ACCESS=false` endurece, mas é mudança de comportamento.

## Duas cópias dos HTML legais

O `go:embed` não alcança fora do módulo, então durante o strangler os documentos
existem aqui (`internal/legal/static/`) e em `../src/main/resources/legal/`.
`legal_test.go` falha se as cópias divergirem. Depois do cutover, apague o
`LegalController` e a cópia Kotlin juntos — o teste passa a se pular.
