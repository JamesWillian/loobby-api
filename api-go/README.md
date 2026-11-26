# api-go — a Loobby API

Roda na máquina da Oracle Cloud, atrás de um nginx na mesma máquina
(`../deploy/go/README.md`). Substituiu a API Kotlin, que foi arquivada na tag
`arquivo/v1`.

## Estado

**Os 53 endpoints da v1** respondem igual ao Kotlin nos 168 cenários de
`../contract/golden-local/`, menos nos seis da exclusão de conta que o Go corrigiu
(abaixo). Desde 16/11/2026 o Go é o único backend em produção. As rotas novas não
têm referência no Kotlin e são testadas só do lado Go: `/app/config` e as 19 rotas
`/v2` das gameplays, a API da 2.0 (ver "Rotas da 2.0: gameplays").

Onde o Go difere do Kotlin **de propósito**:

- recusa refresh token forjado (`../contract/ACHADOS.md` §22);
- conserta a exclusão de conta (§23 e §24): a segunda exclusão em diante e a de
  conta Google funcionam, e a troca de senha de conta Google diz que ela não tem
  senha. São os únicos consertos que mudam respostas: seis cenários do golden
  foram regravados do Go;
- grava evento e detalhes numa transação só;
- sorteia outro username quando o do anônimo colide (§28).

O resto, inclusive os defeitos, é reproduzido.

## Pacotes

| Pacote | O quê |
|---|---|
| `cmd/api` | `main`: config, pool, módulos, servidor, shutdown gracioso |
| `internal/server` | a cadeia HTTP: headers do Spring → filtro de autenticação → precedência de rotas → handler |
| `internal/config` | env vars — **as mesmas** do `application.yaml` do Kotlin |
| `internal/db` | pool `pgx`, transações e o código gerado pelo `sqlc` |
| `migrations`, `cmd/migrate` | as migrações do banco (goose) e o comando que as aplica — ver "Migrações" |
| `internal/httpx` | envelope de erro, `Instant`, leitura de corpo como o Jackson, UUID como o Java, validação |
| `internal/jackson`, `internal/validation`, `internal/kotlin` | números, mensagens e `trim()` **como** o Jackson, o Hibernate Validator e o Kotlin — testados contra os originais |
| `internal/auth` | tokens HS256, filtro (o Spring Security do lado Go), login, cadastro, e-mail, reset, Google |
| `internal/users`, `groups`, `events`, `teams`, `games`, `legal`, `files`, `notifications`, `health` | os módulos |
| `internal/appconfig` | `GET /app/config` (pública): versão mínima de cada app e o aviso mostrado ao abrir. Primeira rota que não veio do Kotlin; muda-se editando `Current` e publicando |
| `internal/gameplays` | a 2.0: gameplays do grupo e únicas, participação, votação com desempate, comentários, encerramento, Histórico, o match com as bibliotecas e os jobs e pushes delas — ver "Rotas da 2.0" |
| `internal/library` | a 2.0: a biblioteca de jogos da pessoa (com o "tenho o jogo") e a do grupo — ver "Rotas da 2.0: bibliotecas" |
| `internal/platform` | as 5 plataformas da 2.0 e a tradução das plataformas do RAWG para elas |
| `internal/email`, `internal/password`, `internal/ids` | Resend, BCrypt e identificadores |
| `internal/contracttest`, `internal/dbtest` | apoio aos testes |

## Rodar

Variáveis: lista comentada em `../deploy/go/api.env.example`. Obrigatórias:
`DB_URL` (JDBC ou formato pgx), `JWT_SECRET` (32 bytes ou mais),
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

Sem `TLS_*`, o servidor fala HTTP puro e **só no loopback** — é o modo de
produção, atrás do nginx da mesma máquina (`../deploy/go/README.md`). Com
`TLS_CERT_FILE`, `TLS_KEY_FILE` e `TLS_CLIENT_CA_FILE` (os três juntos), exige
TLS mútuo e escuta em todas as interfaces, como no desenho anterior. `BIND_HOST`
sobrepõe o host nos dois casos.

## Testes

```bash
go test ./...                                                                # sem banco: os de banco se pulam
TEST_DATABASE_URL=postgres://postgres@127.0.0.1:54329/loobby go test ./...   # tudo
```

O Postgres dos testes é o do ambiente local: `../contract/local/oracle.sh up`
cria o banco `loobby` e aplica as migrações que faltam (rode de novo depois de
criar uma). O `TestContratoIgualAoKotlin` e os testes de `migrations` criam bancos
próprios no mesmo servidor.

Três camadas:

- **`cmd/api` — `TestContratoIgualAoKotlin`.** Sobe o servidor inteiro, com a
  fiação do `main` e um banco só dele, passa a suíte de contrato completa
  (`capture.py` + `flows.py`, 168 cenários) e exige zero diferenças contra
  `contract/golden-local`. Precisa de `python3`, `openssl` e `psql`.
- **`internal/server`** — rotas públicas, precedência de rotas, e `legal` e `games`
  byte a byte contra os golden de produção.
- **cada pacote** — o que a suíte não alcança: conversões do corpo, UUID, senha,
  mensagens de token, verificador do Google, refresh forjado, falha do Resend,
  uploads, FCM contra servidor falso, registro de envios.

## Queries: `sqlc`

SQL escrito à mão em `db/queries/`, Go gerado em `internal/db/sqlc/` — **nunca
edite o gerado**. O schema são as migrações (`migrations/`), lidas na ordem; o
sqlc ignora os trechos Down.

```bash
sqlc generate   # depois de mudar uma query ou criar uma migração
sqlc diff       # confere que o gerado está em dia
```

O gerado inclui um tipo `RsvpStatus` com `MAYBE_RESERVE`: é o enum órfão da
produção (`contract/ACHADOS.md` §8). Nenhuma coluna o usa — `status` é `text`.

## Migrações: goose

O schema muda por migrações em `migrations/` (`NNNNN_nome.sql`, no formato do
goose), aplicadas pelo `cmd/migrate`. Elas **não vão no deploy**: rodam antes, da
sua máquina, e o código que depende delas é publicado depois.

```bash
go run ./cmd/migrate status     # só lê: o que já foi aplicado e o que falta
go run ./cmd/migrate up         # aplica o que falta
```

O banco vem de `MIGRATE_DATABASE_URL`, no formato do pgx. Na produção é a conexão
de **sessão** do Supabase (porta 5432); o pooler em modo transação (6543), o do
`DB_URL` da API, é recusado. Para a senha não ficar no histórico do shell, cole a
URL num `read` silencioso:

```bash
read -rs MIGRATE_DATABASE_URL && export MIGRATE_DATABASE_URL
```

| Migração | O quê |
|---|---|
| `00001_baseline.sql` | o schema da produção de antes do goose |
| `00002_gameplays.sql` | as seis tabelas da 2.0 (`gameplays`, `gameplay_participants`, `gameplay_games`, `gameplay_votes`, `gameplay_comments`, `gameplay_comment_reports`), ao lado das da v1, que não mudam |
| `00003_contas_excluidas.sql` | troca a `UNIQUE` do `users.username` por um índice único que ignora o vazio, para caber mais de uma conta excluída (`../contract/ACHADOS.md` §23). Não depende de código novo: pode rodar antes ou depois do deploy |
| `00004_bibliotecas.sql` | as bibliotecas de jogos da 2.0, `user_library` e `group_library`. Tem que rodar **antes** do deploy do código que as usa: o detalhe de toda gameplay lê a `user_library` (o match) |

A `00001_baseline.sql` é o schema da produção de antes do goose (o dump de
08/09/2026, conferido de novo em 21/11/2026). Num banco vazio ela cria tudo; na
produção ela é marcada como aplicada **uma vez**, sem rodar — e até lá o `up` na
produção é recusado:

```bash
go run ./cmd/migrate baseline   # produção, só na primeira vez
```

O histórico fica em `goose.goose_db_version`, fora do `public`.

**Toda tabela nova no `public`** precisa, na mesma migração, de duas coisas que a
produção tem e os bancos locais não:

- `ALTER TABLE ... ENABLE ROW LEVEL SECURITY`. Os privilégios padrão do Supabase
  dão ao `anon` e ao `authenticated` acesso total a toda tabela nova; com RLS e sem
  policy, a API de dados do Supabase não vê nada — como nas 14 tabelas de hoje.
- `GRANT SELECT, INSERT, UPDATE, DELETE` ao papel `loobby_backend`, que tem isso em
  todas as tabelas de hoje e não entra nos privilégios padrão. Ele só existe no
  Supabase, então o GRANT confere antes (o corpo tem `;`, daí o `StatementBegin`):

```sql
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'loobby_backend') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON public.minha_tabela TO loobby_backend;
  END IF;
END $$;
-- +goose StatementEnd
```

Os dois são conferidos em teste (`migrations`): uma tabela do `public` sem RLS
falha o `TestTodaTabelaDoPublicTemRLS`, e o `TestGrantAoLoobbyBackend` cria o
papel no servidor de teste para conferir o GRANT das tabelas da 2.0.

## Rotas da 2.0: gameplays

A 2.0 transforma o Loobby numa ferramenta de gameplays online entre amigos. É uma
API ao lado da v1, não em cima dela: rotas em `/v2`, tabelas da `00002`, nada da
v1 lido ou escrito (eventos e RSVPs seguem para os apps 1.3 e 1.3.1). As tabelas
compartilhadas são `users`, `groups`, `group_members`, `games` e `device_tokens`.
O código é o `internal/gameplays`.

Uma gameplay é **do grupo** (vista pelos membros) ou **única**, sem grupo (vista
por quem entrou pelo convite, `$-` + 8 caracteres). Por enquanto tudo é privado: a
gameplay pública está pausada. Modos: `VOTING` (votação), `FIXED` (jogo definido)
e `FREE` (livre). Plataforma, uma por enquanto, guardada como lista em JSON para o
crossplay: `PC`, `PLAYSTATION`, `XBOX`, `SWITCH`, `MOBILE`.

| Rota | Quem pode | O quê |
|---|---|---|
| `POST /v2/groups/{groupId}/gameplays` | membro não anônimo | cria gameplay do grupo (201) |
| `POST /v2/gameplays` | não anônimo | cria única, já com o convite (201) |
| `GET /v2/groups/{groupId}/gameplays?status=active\|ended&before=&limit=` | membro | ativas; ou encerradas, da mais recente, em páginas (`before` = a última da página anterior) |
| `GET /v2/me/gameplays` | qualquer | o Lobby: as ativas dos grupos da pessoa e as únicas em que entrou |
| `GET /v2/me/history?before=&limit=` | qualquer | o Histórico: as encerradas em que a pessoa marcou Vou, da mais recente, em páginas como as encerradas do grupo |
| `GET /v2/gameplays/{id}` | quem tem acesso | detalhe: participantes, jogos, votos por rodada, resultado, o match (`owners`) e `permissions` (os botões de quem lê) |
| `PATCH /v2/gameplays/{id}` | criador ou dono do grupo (única: criador) | nome, descrição, horário, plataforma, jogo do FIXED; `voting.finalists` e `voting.multiVote` só o criador, antes do 1º voto |
| `POST /v2/gameplays/{id}/end` | criador ou dono do grupo (única: criador) | Encerrar |
| `GET /v2/invites/{code}` | qualquer | prévia da única |
| `POST /v2/invites/{code}` | qualquer, até anônimo | entra na única como Pendente |
| `PUT /v2/gameplays/{id}/participants/me` | não anônimo | `{"status": "GOING" \| "NOT_GOING" \| "PENDING"}` |
| `DELETE /v2/gameplays/{id}/participants/me` | participante da única, menos o criador | sai da única (204) |
| `POST /v2/gameplays/{id}/games` | membro não anônimo (única: criador) | `{"gameId"?, "name"}`: jogo do catálogo ou digitado |
| `DELETE /v2/gameplays/{id}/games/{gameId}` | idem | remove o jogo da votação e os votos nele |
| `PUT /v2/gameplays/{id}/votes/me` | não anônimo que não marcou Não vou | `{"votes": [ids dos jogos da votação]}`; vazia tira o voto |
| `POST /v2/gameplays/{id}/tiebreak` | criador | Desempatar |
| `GET /v2/gameplays/{id}/comments?before=&limit=` | quem tem acesso | comentários, mais novos primeiro |
| `POST /v2/gameplays/{id}/comments` | não anônimo | `{"body"}`, 1 a 1000 caracteres (201) |
| `DELETE /v2/gameplays/{id}/comments/{commentId}` | autor; dono do grupo; na única, o criador | apaga (204) |
| `POST /v2/gameplays/{id}/comments/{commentId}/report` | quem tem acesso, até anônimo | denuncia; `{"reason"}` opcional (204) |

Toda ação sobre a gameplay responde com o detalhe inteiro, já atualizado.

**Regras, todas conferidas no servidor.** Cada escrita trava a linha da gameplay
(`FOR UPDATE`), o que serializa votos, limite de jogos e desempate.

- **Ativa** = não encerrada e `scheduled_at` + 24 h ainda no futuro. Ela encerra
  pelo botão ou sozinha, 24 h depois do horário marcado. Encerrada é só leitura;
  apagar e denunciar comentário continuam valendo.
- **Criação:** quem cria entra como Vou. O horário aceita até 5 min no passado,
  para "começar agora". O modo não muda depois.
- **Votação:** até 20 jogos; repetido é o mesmo jogo do catálogo ou o mesmo nome,
  sem diferenciar maiúsculas. Jogo digitado vale, sem capa (a capa vem do cache
  `games`). Voto único, ou múltiplo (um voto por jogo); finalistas (1 a 20) e voto
  múltiplo travam no primeiro voto. Votar estando Pendente passa para Vou; com Não
  vou, é recusado; passar para Pendente ou Não vou apaga os votos da rodada aberta.
  O voto é sempre visível.
- **Resultado:** finalistas são os N mais votados com pelo menos 1 voto; um empate
  na última vaga leva todos os empatados.
- **Desempate:** só com 1 finalista e empate no 1º lugar. Abre a rodada seguinte só
  com os empatados, sempre com 1 voto por pessoa; a anterior congela (votos e lista
  de jogos). Se empatar de novo, o criador pode abrir outra rodada.
- **Anônimo** entra em grupos e únicas e vê tudo; não responde, não vota, não
  comenta, não mexe nos jogos e não cria. Pode denunciar.
- **Saída do grupo** (sair ou ser removido): a pessoa perde a participação e os
  votos abertos nas gameplays ativas daquele grupo.
- **Exclusão de conta:** votos, comentários, denúncias e participações saem; a
  gameplay do grupo que a pessoa criou passa para o dono do grupo, e a única, para
  o participante não anônimo mais antigo (sem nenhum, ela é apagada). A biblioteca
  da pessoa sai; o jogo que ela pôs na biblioteca de um grupo fica, com
  `addedBy: null`.
- **Listas** (Lobby, gameplays do grupo e Histórico): na votação, `leading` são os
  finalistas da rodada aberta, e `tie` diz se é empate no 1º lugar (com 1
  finalista). Enquanto o desempate não tem voto, valem os da rodada anterior: o
  desempate só abre porque houve empate.
- **Histórico:** só Vou, porque o status congela no encerramento. A gameplay do
  grupo só aparece enquanto a pessoa é membro, já que o detalhe dá 404 a quem
  saiu; se ela voltar, a gameplay volta, porque a saída só apaga a participação
  nas ativas. O anônimo não responde, então não tem Histórico. Os eventos da v1
  não entram.
- **Match:** no detalhe, `voting.games[].owners` e `game.owners` são os
  participantes com Vou que têm o jogo na biblioteca, marcado numa plataforma da
  gameplay, na ordem dos participantes. O jogo digitado não está no catálogo e
  vem com a lista vazia. É o único jeito de ver algo da biblioteca de outra
  pessoa.

**Erros.** O envelope é o da v1, mas os status são os corretos e as mensagens vêm
em português, prontas para o app mostrar: 400 para corpo ilegível (`Corpo da
requisição inválido`) ou validação (`Dados inválidos`, com `fieldErrors`), 403 sem
permissão, 404 quando não existe ou não há acesso (inclusive id que não é UUID) e
409 para conflito de estado (encerrada, votação travada, desempate aberto, 20
jogos). Rota que não existe continua 500, como na v1.

**Pushes e jobs.** O `data` de cada push traz `type`, `gameplayId` e, na do grupo,
`groupId`.

| Tipo | Quando | Quem recebe |
|---|---|---|
| `GAMEPLAY_NEW_IN_GROUP` | na criação | membros do grupo, menos o criador |
| `GAMEPLAY_TIEBREAK_OPENED` | ao Desempatar | quem marcou Vou, menos o criador |
| `GAMEPLAY_REMINDER` | 1 h antes (job de 5 em 5 min) | quem marcou Vou |
| `GAMEPLAY_TODAY_PENDING` | todo dia às 8h, America/Sao_Paulo | quem ainda não respondeu, menos anônimos |

Os dois agendados usam o `notification_dispatch_log` (`reference_id` = id da
gameplay) e só rodam com `NOTIFICATIONS_ENABLED` e
`NOTIFICATIONS_SCHEDULER_ENABLED`, como os da v1. O job de encerramento roda de 5
em 5 min **sempre**: ele só carimba o `ended_at` (as consultas já tratam como
encerrada a que passou do prazo).

## Rotas da 2.0: bibliotecas

O código é o `internal/library`, e as tabelas são as da `00004`. As duas
bibliotecas só aceitam jogo do catálogo, pelo id do RAWG (o de `GET /games/{id}`).
O que ainda não está no cache `games` é buscado no RAWG; o que ele não conhece, ou
com o RAWG fora do ar, dá 404, como no `GET /games/{id}`. O app adiciona a partir
da busca, que já grava cada resultado no cache.

| Rota | Quem pode | O quê |
|---|---|---|
| `GET /v2/me/library` | qualquer, até anônimo | a biblioteca da pessoa, pelo nome do jogo |
| `PUT /v2/me/library/{gameId}` | qualquer, até anônimo | `{"ownedPlatforms": [...]}`: adiciona o jogo, ou troca o "tenho o jogo"; lista vazia = só na biblioteca |
| `DELETE /v2/me/library/{gameId}` | qualquer, até anônimo | tira o jogo (204, mesmo se ele não estava lá) |
| `GET /v2/groups/{groupId}/library` | membro, até anônimo | a biblioteca do grupo, pelo nome do jogo |
| `PUT /v2/groups/{groupId}/library/{gameId}` | membro não anônimo | adiciona; repetir não muda nada, nem o `addedBy` |
| `DELETE /v2/groups/{groupId}/library/{gameId}` | membro não anônimo | tira (204, mesmo se ele não estava lá) |

Um item da biblioteca da pessoa. O do grupo não tem `ownedPlatforms` nem
`updatedAt`, e tem `addedBy`: o usuário, ou `null` se a conta foi excluída.

```json
{"gameId": "3498", "name": "Grand Theft Auto V", "coverUrl": "https://media.rawg.io/...",
 "availablePlatforms": ["PC", "PLAYSTATION", "XBOX"], "ownedPlatforms": ["PC"],
 "addedAt": "2026-11-26T01:00:00Z", "updatedAt": "2026-11-26T01:00:00Z"}
```

- **`availablePlatforms`** são as plataformas em que o jogo existe, tiradas do
  `games.platforms` do RAWG: `pc`, `macos` e `linux` são `PC`; `playstation*`,
  `ps-vita` e `psp`, `PLAYSTATION`; `xbox*`, `XBOX`; `nintendo-switch*`, `SWITCH`;
  `ios` e `android`, `MOBILE`. Sem lista no RAWG, ou só com plataformas retrô, vem
  `[]`, e o app oferece as 5.
- **`ownedPlatforms`** aceita qualquer uma das 5, sem repetir, mesmo fora da lista
  do RAWG, que às vezes vem incompleta. Sai na ordem das 5.
- **Limites:** 500 jogos por pessoa e 100 por grupo (409). Mudar o que já está lá
  continua valendo.
- **Quem vê:** a biblioteca da pessoa, só ela. Os outros veem apenas o match, no
  detalhe das gameplays que dividem com ela.
- **Erros:** os da v2. 400 para corpo ilegível ou `ownedPlatforms` faltando ou
  inválida (`fieldErrors`), 403 para quem não é membro ou é anônimo, 404 para o
  grupo e o jogo (inclusive o id que não é só dígitos) e 409 para a biblioteca
  cheia.

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
- exclusão de conta:
  - a `UNIQUE` do username de volta;
  - as credenciais do Google mantidas;
  - a senha sem conferir, ou passada para trás pelo Google;
  - anônimo conseguindo se excluir;
  - o 404 de volta na troca de senha de conta Google

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
respondia 500 "Unexpected error", e o Go mantém, porque é o contrato dos apps 1.3
(`contract/ACHADOS.md` §18 e §21).

## HTML legais

Ficam em `internal/legal/static/`, embutidos no binário; a cópia do Kotlin saiu
com ele. Mudar um texto legal é publicar a API.
