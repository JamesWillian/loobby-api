# api-go — o lado Go da Loobby API

Roda numa máquina separada, atrás do nginx do AWS, que encaminha rota a rota
(`../deploy/`). Os dois lados compartilham banco e `JWT_SECRET`, então cada rota é
migrável e reversível isoladamente.

Ordem: `legal → games → files → notifications → groups → users → events → auth`.

## Estado

**Os 53 endpoints estão implementados**, e o Go responde igual ao Kotlin nos 166
cenários de `../contract/golden-local/`. O que falta é o cutover no nginx, módulo
a módulo (`../deploy/nginx/README.md`): o painel liga hoje `legal` e `games`.

Onde o Go difere do Kotlin **de propósito**: recusa refresh token forjado
(`../contract/ACHADOS.md` §22), grava evento e detalhes numa transação só e sorteia
outro username quando o do anônimo colide (§28). O resto — inclusive os defeitos —
é reproduzido.

## Pacotes

| Pacote | O quê |
|---|---|
| `cmd/api` | `main`: config, pool, módulos, servidor, shutdown gracioso |
| `internal/server` | a cadeia HTTP: headers do Spring → filtro de autenticação → precedência de rotas → handler |
| `internal/config` | env vars — **as mesmas** do `application.yaml` do Kotlin |
| `internal/db` | pool `pgx`, transações e o código gerado pelo `sqlc` |
| `internal/httpx` | envelope de erro, `Instant`, leitura de corpo como o Jackson, UUID como o Java, validação |
| `internal/jackson`, `internal/validation`, `internal/kotlin` | números, mensagens e `trim()` **como** o Jackson, o Hibernate Validator e o Kotlin — testados contra os originais |
| `internal/auth` | tokens HS256, filtro (o Spring Security do lado Go), login, cadastro, e-mail, reset, Google |
| `internal/users`, `groups`, `events`, `teams`, `games`, `legal`, `files`, `notifications`, `health` | os módulos |
| `internal/email`, `internal/password`, `internal/ids` | Resend, BCrypt e identificadores |
| `internal/contracttest`, `internal/dbtest` | apoio aos testes |

## Rodar

Mesmas variáveis do Kotlin (lista comentada em `../deploy/go/api.env.example`).
Obrigatórias: `DB_URL` (JDBC ou formato pgx), `JWT_SECRET` (32 bytes ou mais),
`RAWG_API_KEY`, `RESEND_API_KEY` e os três `GOOGLE_*_CLIENT_ID`. Com
`NOTIFICATIONS_ENABLED=true` (o padrão), o arquivo de credenciais do Firebase
também precisa existir.

O jeito mais curto de ver tudo funcionando é o ambiente local, que sobe o Go em
`http://127.0.0.1:18081` com banco e serviços externos falsos:

```bash
../contract/local/oracle.sh up
```

**Nunca aponte um Go de teste para o `DB_URL` do Supabase**: cadastro e exclusão
de conta seriam escritas em produção.

Com `TLS_CERT_FILE`, `TLS_KEY_FILE` e `TLS_CLIENT_CA_FILE` (os três juntos), o
servidor exige TLS mútuo e escuta em todas as interfaces — é o modo de produção
(`../deploy/go/README.md`). Sem eles, HTTP puro e **só loopback**: HTTP sem TLS
nunca fica exposto na rede. `BIND_HOST` sobrepõe o host nos dois casos.

## Testes

```bash
go test ./...                                                                # sem banco: os de banco se pulam
TEST_DATABASE_URL=postgres://postgres@127.0.0.1:54329/loobby go test ./...   # tudo
```

O Postgres dos testes é o do ambiente local (`../contract/local/oracle.sh up`
cria o banco `loobby` com o schema de produção — que precisa do `pg_trgm`, que o
dump não traz: `contract/ACHADOS.md` §20).

Três camadas:

- **`cmd/api` — `TestContratoIgualAoKotlin`.** Sobe o servidor inteiro, com a
  fiação do `main` e um banco só dele, passa a suíte de contrato completa
  (`capture.py` + `flows.py`, 166 cenários) e exige zero diferenças contra
  `contract/golden-local`. Precisa de `python3`, `openssl` e `psql`.
- **`internal/server`** — rotas públicas, precedência de rotas, e `legal` e `games`
  byte a byte contra os golden de produção.
- **cada pacote** — o que a suíte não alcança: conversões do corpo, UUID, senha,
  mensagens de token, verificador do Google, refresh forjado, falha do Resend,
  uploads, FCM contra servidor falso, registro de envios.

## Queries: `sqlc`

SQL escrito à mão em `db/queries/`, Go gerado em `internal/db/sqlc/` — **nunca
edite o gerado**. O schema é o dump da produção (`../contract/schema.sql`), não
as migrations do Kotlin, que divergiram dela.

```bash
sqlc generate   # depois de mudar uma query
sqlc diff       # confere que o gerado está em dia
```

O gerado inclui um tipo `RsvpStatus` com `MAYBE_RESERVE`: é o enum órfão da
produção (`contract/ACHADOS.md` §8). Nenhuma coluna o usa — `status` é `text`.

## Como cada módulo é provado

`internal/contracttest` carrega um golden de `../contract/golden/` e exige que a
resposta Go bata em **status, headers de contrato (nos dois sentidos) e corpo**:
texto byte a byte, binário por sha256. Em `internal/server`, cada módulo roda
pela cadeia completa — headers do Spring, JWT, handler, banco.

Os testes foram checados aplicando defeitos de propósito: cada um dos abaixo foi
introduzido e pego.

- `Content-Type` no formato do Go; `Cache-Control` diferente; `X-XSS-Protection` antigo
- esquecer o `SpringHeaders`; o `\n` do `json.Encoder`; health como `application/json`
- TLS: aceitar cliente sem certificado
- numeric lido pelo protocolo binário; jsonb do cache sem re-escrever como o Jackson
- resposta fresca de games relida do banco; `page` sem hexadecimal; trim do Go no lugar do do Java
- o `FixEscapes` desligado; a chave do RAWG sem ser removida do erro
- assunto do e-mail de reset; `obs` não limpo na exclusão de conta; tolerância de relógio do Google
- refresh de anônimo lido sem conferir a assinatura

## Armadilhas do lado Go — todas travadas em teste

**O `encoding/json` não escreve como o Jackson.** O `Encoder` acrescenta `\n`; o
`Marshal` escapa `&`, `<`, `>`, U+2028 e U+2029 e usa hexa minúsculo nos
controles. Use sempre `httpx.Marshal` / `httpx.WriteJSON`.

**Números seguem as regras do Java, não as do Go.** O `internal/jackson` reproduz
o `Double.toString` (inclusive a regra que faz `Double.MIN_VALUE` sair `4.9E-324`)
e o `BigDecimal.toString`. O gabarito, `internal/jackson/testdata/oracle.jsonl`,
foi gerado pelo **Jackson 2.19.2 de verdade** — 3.067 casos, inclusive 3.026
doubles. Para regerar, ver o cabeçalho de `testdata/JacksonOracle.java`.

**O pgx perde a escala do zero.** Pelo protocolo binário, `numeric(4,2)` `0.00`
chega como `"0"` — e `0.00` é a nota de boa parte dos jogos. O
`db.RegisterTypes` faz o numeric chegar como o texto do Postgres. **Todo pool
precisa passar por `db.Connect`**; um pool aberto por fora leria `0` sem erro.

**O `sqlc` erra a nulidade de expressões.** Com cast (`rating::text`) ele gera
`string` e um NULL quebra a leitura; com `NULLIF` gera `bool`. Prefira colunas
puras e resolva o tipo no `sqlc.yaml`, como no numeric.

**O timestamp não é o default do Go.** O `ISO_INSTANT` agrupa a fração em 0, 3, 6
ou 9 dígitos; o `RFC3339Nano` corta todos os zeros. Daí o `httpx.Instant`.

**O `path` do envelope de erro é a URI crua**, sem decodificar
(`httpx.RequestPath`) — o `r.URL.Path` do Go já vem decodificado.

**Refresh token é aceito como access token**, como no Kotlin.
`ACCEPT_REFRESH_AS_ACCESS=false` endurece, mas é mudança de comportamento.

**Mensagens de erro do `net/http` trazem a URL inteira**, com a query string — e a
chave do RAWG vai nela. O `games.RawgClient` corta antes de logar.

## games: dois comportamentos que parecem bug e são contrato

**O rating muda de escala conforme a origem.** Do RAWG, `0.0`; do cache, `0.00`.
A resposta fresca é montada com o que veio do RAWG, nunca relida do banco
(`contract/ACHADOS.md` §3).

**Parâmetros malformados dão 500.** `q` ausente ou vazio, `page=abc`: o Kotlin
responde 500 "Unexpected error", e o Go também, até o Kotlin ser corrigido
(`contract/ACHADOS.md` §18 e §21).

## Duas cópias dos HTML legais

O `go:embed` não alcança fora do módulo, então durante o strangler os documentos
existem aqui (`internal/legal/static/`) e em `../src/main/resources/legal/`.
`legal_test.go` falha se as cópias divergirem. Depois do cutover, apague o
`LegalController` e a cópia Kotlin juntos.
