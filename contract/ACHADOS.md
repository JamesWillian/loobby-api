# Achados da baseline de contrato

Os itens 1 a 21 saíram dos golden de `contract/golden/`, capturados de
`https://api.loobby.app` a partir de 08/09/2026. Os itens 22 em diante saíram da
porta dos módulos restantes para Go (novembro de 2026), medidos contra o Kotlin
rodando localmente (`contract/local/`). Tudo aqui é **medido**, não inferido do
código — exceto os §28, §29 e §32, que vêm da leitura do código Kotlin e estão
marcados assim.

**Leia primeiro: §22 (falha de segurança em produção), §23 e §24 (exclusão de
conta não funciona).**

Cada item é uma forma concreta do Go quebrar os apps em silêncio.

---

## 1. Três precisões de timestamp na mesma resposta

| Campo | Dígitos de fração | Exemplo medido |
|---|---|---|
| `scheduledDatetime` | 0 | `2026-09-15T16:14:35Z` |
| `createdAt` | 6 | `2026-09-08T16:14:36.211055Z` |
| `timestamp` (envelope de erro) | 9 | `2026-09-08T16:14:32.551538700Z` |

O `time.Time` do Go serializa como **RFC3339Nano, que remove zeros à direita**.
Nenhuma das três formas sai correta no default: `.211055` viraria `.211055`, mas
`.211050` viraria `.21105`, e `.500000` viraria `.5`.

E o lado Kotlin também é variável: o `ISO_INSTANT` do Jackson emite 0, 3, 6 ou 9
dígitos **conforme o valor**, não conforme o campo. Ou seja, a tabela acima é uma
amostra de 1 por campo, não uma garantia de formato fixo.

**Ação:** não confiar no marshal default dos dois lados. Formatar explicitamente
com layout fixo e **coletar mais amostras** antes de fechar o layout de cada campo
— principalmente valores cujos microssegundos terminem em zero.

## 2. `pricePerPlayer` preserva a escala de `numeric(12,2)`

Enviado `"25.50"`, devolvido **`25.50`** — com o zero à direita.

```json
"sport":{"durationMinutes":90,"arena":"Quadra Contract","pricePerPlayer":25.50,...}
```

`float64` no Go emite `25.5`. **Ação:** `shopspring/decimal`, e serializar como
número JSON (não string) preservando a escala.

## 3. `rating` muda de escala conforme a ORIGEM do dado, não o endpoint

> **Corrigido em 23/10/2026.** A versão anterior desta seção dizia que a escala
> dependia do endpoint (`0.0` na busca, `0.00` no detalhe). Errado: o próprio
> `GET /games/{id}` devolve as duas.

Medido em produção, pedindo o detalhe do mesmo jogo duas vezes seguidas, para três
jogos que ninguém tinha aberto ainda (257, 64911, 21482):

| Chamada | Origem no Kotlin | `rating` |
|---|---|---|
| 1ª | RAWG, na hora (o jogo não tinha descrição no cache) | `0.0` |
| 2ª | tabela `games` do Postgres | `0.00` |

Byte a byte, é a **única** diferença entre as duas respostas.

O mecanismo: do RAWG, o valor passa por `JsonNode.decimalValue()` — um double
convertido para BigDecimal, que sai como o `Double.toString` (`0.0`, `4.5`, `4.38`).
Do cache, vem da coluna `rating numeric(4,2)`, que fixa duas casas (`0.00`, `4.50`).
A busca sempre responde pelo primeiro caminho.

**Ação para o Go:** montar a resposta fresca com o que veio do RAWG, sem reler do
banco, e a do cache com o texto da coluna. Feito em `api-go/internal/games`, e
travado num teste que reproduz as duas chamadas de produção, na ordem.

## 4. Arrays vazios e nulos seguem duas convenções opostas

| Campo | Forma medida | Ocorrências |
|---|---|---|
| `players` (time sem jogadores) | `[]` | 4 |
| `confirmedAvatars` | `[null]` | 3 |

`confirmedAvatars` é um array **contendo o literal `null`**, não um array vazio nem
um `null`. Vem de `List<String?>?` onde o usuário confirmado não tem avatar.

**Ação:** `[]*string` (slice de ponteiro) para `confirmedAvatars`, inicializado
como slice vazio e não nil para `players`. Um `[]string` no Go emitiria `[""]`.

## 5. "Não encontrado" responde 400, não 404

Nenhuma das 47 respostas foi 404. Distribuição medida:

| Status | Ocorrências | Casos |
|---|---|---|
| 200 | 37 | |
| 204 | 3 | `devices/register`, `devices/{token}`, `rsvps/me` (DELETE) |
| 400 | 4 | credenciais inválidas · grupo inexistente · time inexistente · body inválido |
| 401 | 2 | sem `Authorization` · refresh token inválido |
| 500 | 1 | ver §6 |

`Group not found` e `Team not found` chegam como `IllegalArgumentException` e o
`ApiExceptionHandler` mapeia para **400**. Credencial errada também: **400**, não 401.

**Ação:** o Go precisa reproduzir 400 nesses casos. É contrato, mesmo sendo
discutível — os apps em produção podem depender disso. Trocar para 404/401
é uma decisão de produto, separada da migração, e com os apps versionados.

## 6. ~~Bug real~~ CORRIGIDO: `ResponseStatusException` virava 500

**Sintoma capturado:** `GET /games/{id}` com id inexistente devolvia 500.

```json
{"status":500,"error":"Internal Server Error","message":"Unexpected error",...}
```

**Causa:** não era o `RawgClient` — ele trata o 404 do RAWG corretamente e devolve
`null`, e o `GameService` levanta `ResponseStatusException(NOT_FOUND)` como deveria.
O culpado era o `@ExceptionHandler(Exception::class)` do `ApiExceptionHandler`:
`ResponseStatusException` **é** uma `Exception`, e o `ExceptionHandlerExceptionResolver`
roda antes do `ResponseStatusExceptionResolver` do Spring. O catch-all vencia sempre
e descartava o status declarado na exceção.

Como `ResponseStatusException` é lançada em **26 pontos** do projeto, o bug nunca
foi só de games.

**Correção:** handler dedicado para `ResponseStatusException`, que respeita o
`statusCode` e usa `ex.reason` como mensagem.

### Alcance

Não muda (já capturavam a exceção localmente para renderizar HTML):
`GET /auth/verify-email` · `GET /auth/reset-password` · `POST /auth/reset-password`

Não muda (nunca lança — usa `return` para não revelar se o e-mail existe):
`POST /auth/forgot-password`. **A proteção contra enumeration segue intacta.**

Passam a responder o status certo, todos hoje em 500:

| Endpoint | Antes | Depois |
|---|---|---|
| `GET /games/{id}` inexistente | 500 | 404 |
| `POST /auth/resend-verification` | 500 | 404 · 400 · **429** (rate limit) |
| `POST /auth/google` | 500 | 400 · 404 · 401 |
| `PUT /users/me/password` | 500 | 400 · 404 · **401** (senha atual errada) |
| `DELETE /users/me` | 500 | **401** (senha errada) · 404 |

> **O golden `0NN-games-details-inexistente.json` está desatualizado de propósito.**
> Ele registra os 500 da produção, que ainda roda o código antigo. Regrave a
> baseline **depois do deploy** — é o momento em que a suíte confirma que a
> correção chegou lá. Os outros quatro endpoints da tabela não têm golden ainda
> (ver "Pendente de cobertura").

## 7. Envelope de erro

Formato único, estável nos 7 erros capturados:

```json
{"timestamp":"<iso>","status":<int>,"error":"<reason phrase>",
 "message":"<msg>","path":"<path>","fieldErrors":null}
```

`fieldErrors` sai como **`null` explícito**, não omitido — confirma a regra de não
usar `omitempty` em nenhum campo de response.

---

## 8. O schema real diverge das migrations do repo

`contract/schema.sql` (dump da produção, 08/09/2026) contra
`src/main/resources/db/migration/`. O dump é a fonte de verdade; as migrations não.

### Os enums do `V1__init.sql` estão órfãos

| | `V1__init.sql` | Produção |
|---|---|---|
| `events.event_type` | `event_type` (enum) | **`text`** |
| `event_rsvps.status` | `rsvp_status` (enum) | **`text`** |

Os `CREATE TYPE` existem no banco, mas **nenhuma coluna os usa**. E o
`rsvp_status` da produção nem bate com o do repo: tem `('YES','NO','MAYBE_RESERVE')`,
enquanto o Kotlin usa `YES/NO/MAYBE/RESERVE`. Como a coluna é `text`, isso nunca
quebrou — o tipo é vestigial.

**Ação para o Go:** mapear as duas colunas como `string`/`text`. Declarar enum no
`sqlc` ia gerar tipo Postgres que a coluna não tem.

## 9. Há regra de negócio em triggers do banco

Dois triggers que **o Go herda de graça — e não pode reimplementar**:

```sql
-- AFTER INSERT ON groups
insert into group_members (id, group_id, user_id)
values (gen_random_uuid(), NEW.id, NEW.owner_id)
on conflict (group_id, user_id) do nothing;

-- AFTER INSERT ON events
insert into event_rsvps (event_id, user_id, status)
values (new.id, new.owner_id, 'YES');
```

Isso explica um dado da baseline que eu não tinha explicado: o evento recém-criado
já volta com `confirmedCount: 1` e `rsvpStatus: "YES"` **antes de qualquer chamada
ao endpoint de RSVP**. Não é o `EventService` — é o `trg_auto_rsvp_owner`.

**O risco concreto:** alguém lê o `createEvent` do Kotlin em Go, não encontra
insert de RSVP, conclui que falta e "corrige". O `fn_auto_rsvp_owner` **não tem
`on conflict`**, então o insert explícito colide com o do trigger e estoura a PK
`(event_id, user_id)`. O de grupos é idempotente e perdoaria; o de eventos não.

## 10. `user_feed` é uma VIEW, e ela filtra mais do que parece

```sql
CREATE VIEW public.user_feed WITH (security_invoker='on') AS ...
  UNION ALL de (grupos do usuário) e (eventos com RSVP do usuário)
  WHERE e.is_instant = true          -- <- só eventos instantâneos entram no feed
  ORDER BY sort_bucket, scheduled_datetime
```

Três coisas que só o dump conta:

1. **Só eventos com `is_instant = true`** aparecem no feed. Evento de grupo não entra.
2. **A ordenação é da view** (`sort_bucket`: pendente=1, grupo=2, terminado=3, depois
   por data). O `UserFeedRepository` faz `select * from user_feed` **sem `ORDER BY`** —
   ele depende da ordem da view. Em Go, ou se replica a view, ou o `ORDER BY` volta
   explícito na query, senão a ordem do feed vira indefinida.
3. A view expõe `scheduled_datetime`, coluna que a `UserFeedEntity` **não mapeia** —
   por isso ela não aparece na resposta de `/users/feed`.

---

## 11. ~~`AccessDeniedException` também vira 500~~ CORRIGIDO

Medido com o 2º usuário tentando mexer no grupo de outro dono:

| Cenário | Esperado | Medido |
|---|---|---|
| `PATCH /groups/{id}` sem ser dono | 403 | **500** |
| `DELETE /groups/{id}` sem ser dono | 403 | **500** |

**Não é falha de segurança.** A autorização funciona: o `GroupService` levanta
`AccessDeniedException` e a operação é bloqueada — o grupo sobreviveu ao DELETE
e só foi apagado depois, pelo dono legítimo. O que está errado é o status.

A causa é a do §6: `AccessDeniedException` não tem handler dedicado, então cai no
`@ExceptionHandler(Exception::class)` e vira 500 "Unexpected error". A correção do
§6 cobriu só `ResponseStatusException`; esta é uma exceção diferente e continua
caindo no catch-all.

**Consequência prática:** os apps não conseguem distinguir "você não tem permissão"
de "o servidor quebrou". E, sem corrigir, o Go precisa **replicar um 500 em falha de
autorização** para manter paridade.

**Correção aplicada:** handler dedicado para `AccessDeniedException` no
`ApiExceptionHandler`, devolvendo 403 com o mesmo envelope. Três pontos de
lançamento afetados: `GroupService`, `GroupMemberService`, `EventService`.

> Os golden `grupo-patch-sem-permissao` e `grupo-delete-sem-permissao` estão
> desatualizados de propósito, como o do §6: registram os 500 que a produção ainda
> responde. Regrave depois do deploy.

## 12. `RawgUnavailableException` virava 500 — CORRIGIDO

Terceira instância do mesmo padrão. `RawgUnavailableException` é uma
`RuntimeException` sem `@ResponseStatus`, e o `GameService.search` não a capturava —
então RAWG fora do ar (ou circuit breaker aberto) devolvia **500** em
`GET /games/search`.

Diferente de `getDetails`, a busca **não tem cache local para servir de fallback**:
se o RAWG está fora, não há o que devolver. O cliente precisa distinguir
"indisponibilidade temporária de terceiro" de "o servidor quebrou" — para poder
tentar de novo em vez de tratar como erro definitivo.

**Correção aplicada:** `GameService.search` traduz para
`ResponseStatusException(SERVICE_UNAVAILABLE)` com mensagem genérica. A mensagem
técnica do RAWG fica no log, fora da resposta.

**Não tem golden** e é difícil ter: exigiria derrubar o RAWG ou abrir o circuit
breaker de propósito (5 falhas consecutivas). Fica registrado aqui como
comportamento esperado, não coberto pela suíte.

---

## 13. `avatarUrl: null` mantém o avatar; quem limpa é `""`

O comentário no `UsersService.updateUserProfile` diz *"avatarUrl também pode ser
null → remove avatar"*. **O código faz o contrário:**

```kotlin
req.avatarUrl?.let { newAvatarUrl ->
    user.avatarUrl = newAvatarUrl.trim().ifBlank { null }
}
```

O `?.let` pula quando é null. Quem apaga o avatar é **string vazia**, via o
`ifBlank { null }` de dentro. Mesma semântica vale para `displayname`.

Confirmado na captura: `PATCH /users/me` com `{"avatarUrl": ""}` devolve
`avatarUrl: null`.

**Ação:** no Go, `*string` distinguindo os três estados — ausente/null = não
alterar, `""` = limpar, valor = definir. Um `string` simples colapsa null e `""`
no mesmo zero value e passa a apagar o avatar em toda requisição que omita o campo.

## 14. Uploads acumulam arquivos sem rota de exclusão

`FileStorageService.store()` nomeia como `<epochMillis>.<ext>` sob
`uploads/<subfolder>/<ownerId>/` e **não apaga o anterior**. Trocar de avatar dez
vezes deixa dez arquivos; só o último é referenciado. Nenhum endpoint apaga.

Não é regressão da migração — é dívida existente, e o mesmo problema existiria em
Go. Mas pesa mais no destino pretendido (boot volume de 47 GB), e a suíte de
contrato seria uma fonte a mais de lixo, por isso `--include-uploads` está fora
do default.

---

## 15. `POST /auth/google` com token malformado: 400 com `message: null`

```json
{"timestamp":"...","status":400,"error":"Bad Request","message":null,
 "path":"/auth/google","fieldErrors":null}
```

O `idToken` malformado estoura dentro da biblioteca do Google como
`IllegalArgumentException` **sem mensagem**, e o `handleIllegalArgumentOrState`
repassa `ex.message` — que é null. O cliente recebe um erro sem nenhuma informação.

**Ação para o Go:** o campo `message` precisa sair como **`null` explícito**, não
como `""` nem omitido. É o caso mais extremo da regra de não usar `omitempty`:
aqui o valor null é o contrato inteiro.

**Não coberto:** o `ResponseStatusException(UNAUTHORIZED, "Token do Google inválido")`
de `GoogleAuthService:81` só é alcançado por um JWT **bem-formado** que o verificador
rejeite. Exigiria forjar um JWT com estrutura válida e assinatura/audience inválidas —
viável, mas não feito.

---

## 16. Headers do Spring Security e do CORS em TODA resposta

A primeira baseline guardava só 4 headers numa whitelist e deixou estes de fora.
Corrigido em 05/10/2026; todos os golden foram regravados com eles.

```
X-Content-Type-Options: nosniff
X-XSS-Protection: 0                      <- default do Spring Security 6
X-Frame-Options: DENY
Vary: Origin
Vary: Access-Control-Request-Method
Vary: Access-Control-Request-Headers
Cache-Control: no-cache, no-store, max-age=0, must-revalidate
Pragma: no-cache
Expires: 0
```

Saem **inclusive no 401** que o filtro de autenticação devolve antes de qualquer
controller, e no 400 do `ApiExceptionHandler`. Os três últimos só aparecem quando
o handler não definiu cache: `/legal/*` manda `max-age=3600, public` e aí o Spring
não acrescenta `Pragma` nem `Expires`. Não há HSTS — o TLS termina no nginx.

**Ação para o Go:** um middleware mais externo que todos, inclusive que o de JWT.
Feito em `api-go/internal/httpx/springheaders.go`.

## 17. Não existe configuração de CORS: todo preflight é recusado

`.cors(withDefaults())` sem nenhum `CorsConfigurationSource`. Medido:

| Requisição | Resposta |
|---|---|
| `OPTIONS` com `Origin` + `Access-Control-Request-Method` | **403**, corpo `Invalid CORS request`, sem `Content-Type` |
| o mesmo, com `Origin: https://api.loobby.app` | **403** |
| `OPTIONS` sem `Origin` | **401** — cai na autenticação |

**Consequência de produto:** nenhum navegador em outra origem consegue fazer
requisição não-simples à API (com `Authorization`, ou JSON no corpo). Se existe ou
vai existir um cliente web servido de outro domínio, ele não funciona hoje.

**Ação para o Go:** durante o strangler, preflight vai para o Kotlin (o nginx só
encaminha ao Go os métodos que ele implementa). O `SpringHeaders` replica o 403
para quando o Go atender sozinho — com golden e teste.

## 18. Mais cinco 500 da mesma família do §6 — NÃO corrigidos

Exceções-padrão do Spring MVC também não são `ResponseStatusException`, então o
catch-all do `ApiExceptionHandler` as engole. Medido com token válido:

| Requisição | Hoje | Deveria |
|---|---|---|
| rota inexistente (`NoResourceFoundException`) | 500 | 404 |
| método errado (`HttpRequestMethodNotSupportedException`) | 500 | 405 |
| query param obrigatório faltando | 500 | 400 |
| JSON malformado (`HttpMessageNotReadableException`) | 500 | 400 |
| path var que não é UUID (`MethodArgumentTypeMismatchException`) | 500 | 400 |

Os três primeiros implementam `ErrorResponse` (Spring 6) e carregam o status
certo — o catch-all só precisaria respeitar `ErrorResponse.statusCode`. Os dois
últimos precisam de handler explícito.

**Para o strangler isto não bloqueia nada:** como o nginx roteia por (método,
caminho) exatos, as requisições malformadas continuam no Kotlin, e a paridade se
mantém sozinha. Mas são cinco 500 em produção agora, e a correção cabe no PR #1,
que é exatamente sobre esta família.

## 19. `/actuator/health` não é `application/json`

```
Content-Type: application/vnd.spring-boot.actuator.v3+json
```

O primeiro handler Go da Fase 1 respondia `application/json` — teria ido para
produção assim. Pego ao comparar com o golden.

---

## 20. O banco depende da extensão `pg_trgm`, que o dump não registra

O índice `games_name_trgm_idx` usa `gin_trgm_ops`, da extensão `pg_trgm`. O dump
do schema (`--schema=public`) não inclui o `CREATE EXTENSION`, então aplicar
`contract/schema.sql` num Postgres limpo falha na linha do índice.

**Ação:** `CREATE EXTENSION pg_trgm SCHEMA public` antes do schema — o
`api-go/README.md` traz o passo a passo do banco local de testes.

**Hoje:** o dump virou a migração de base (`api-go/migrations/00001_baseline.sql`),
que já cria a extensão.

## 21. A busca de jogos trata parâmetros de jeitos que só medindo

Medido em produção em 23/10/2026. Pelo nginx, toda `GET /games/search` vai para
o Go, com qualquer query string — então tudo isto é contrato:

| Requisição | Kotlin |
|---|---|
| `q` ausente, `q=` ou `q=%20` | **500** "Unexpected error" (ver §18) |
| `q=%C2%A0` (espaço não-separável) | 200 — o `@NotBlank` usa o `trim()` do Java, que não remove U+00A0 |
| `q=mario&q=kart` | 200 — busca `"mario,kart"` |
| `page=abc` | **500** |
| `page=` | 200, página 1 (o vazio cai no default) |
| `page=0x2` | 200, **página 2** — o Spring lê `0x`/`#` como hexadecimal |
| `page=%202%20` | 200, página 2 — espaços são removidos |
| `page=2&page=3` | 200, página 2 — vale o primeiro |
| `page=&page=2` | **500** — o primeiro vazio vira null num `Int` |
| `page=999999` | **500** (503 depois do PR #1) |

O último tem um efeito colateral, este **deduzido do código**: o 404 que o RAWG
devolve para página além do fim conta como falha no circuit breaker da busca
(`searchGames` trata qualquer `RestClientException` como falha). Cinco clientes
paginando além do fim abrem o circuito por 2 minutos — e a busca fica fora para
todo mundo. O Go reproduz o comportamento; corrigir é decisão para os dois lados.

---

## 22. FALHA DE SEGURANÇA: `/auth/refresh` renova anônimo sem conferir a assinatura

Medido no Kotlin local em 15/11/2026. Quando o refresh token é recusado (vencido
**ou inválido**), o `refreshExpiredAnonymousOrThrow` lê o token com
`SignedJWT.parse`, que **não confere assinatura**, e, se o `sub` for de um usuário
anônimo, devolve tokens novos e válidos.

Basta saber o id de um usuário anônimo — e o id aparece para qualquer membro do
mesmo grupo, em `/groups/{id}/members` — para fabricar um "refresh token"
assinado com qualquer segredo e **entrar na conta dele**.

**No Go está corrigido, e é a única diferença deliberada de comportamento:** a
renovação de anônimo só vale para token assinado por este servidor; só a validade
é relevada (`TestRefreshForjadoDeAnonimoERecusado`). O caso legítimo — anônimo há
mais de 15 dias sem abrir o app — continua funcionando, e é idêntico nos dois
lados (`fluxo-refresh-vencido-anonimo`). `ALLOW_UNSIGNED_ANON_REFRESH=true`
restaura o comportamento do Kotlin; só faz sentido se o `JWT_SECRET` for trocado
e houver anônimos com token antigo a preservar.

**A produção continua vulnerável até o cutover de `auth`** (ou até corrigir o
Kotlin: conferir a assinatura antes de ler o `sub`).

## 23. Só UMA conta consegue ser excluída

`DELETE /users/me` grava `username = ""` no usuário excluído, e `users.username`
tem restrição `UNIQUE`. A primeira exclusão funciona; **da segunda em diante, 500**
— a transação é desfeita e a conta continua existindo, com login e tudo. Medido
nos dois lados (`fluxo-exclusao-2a-conta`).

Se alguém já excluiu a conta em produção, ninguém mais consegue. É exigência das
lojas (a página `/legal/account-deletion` promete a exclusão).

O Go reproduz o 500, de propósito: consertar exige decidir o que gravar no lugar
(`NULL` com a coluna anulável, ou um nome único por conta excluída) e os times e
sorteios hoje reconhecem excluído por `username` vazio. **Decisão sua**, para os
dois lados ou logo depois do cutover de `users`.

## 24. Conta Google não consegue se excluir nem trocar senha

`DELETE /users/me` e `PUT /users/me/password` procuram a linha de
`user_credentials`, que conta Google não tem: **404 "Usuário não encontrado."**
(`fluxo-google-excluir-conta`). Mesmo sem o §23, quem entrou com Google não tem
como excluir a própria conta pela API.

## 25. Conta excluída continua "viva"

Medido depois da exclusão (`fluxo-exclusao-*`), igual nos dois lados:

- o access token antigo continua valendo, e `GET /users/me` responde 200 com
  `username: ""`, `displayname: "[deleted]"`, `emailVerified: true`;
- **o refresh token antigo ainda emite tokens novos** — nada é revogado;
- o usuário segue como membro dos grupos e nos RSVPs, como `[deleted]`;
- grupo com outro membro passa para ele; sem outro membro, some. Evento
  instantâneo passa para o confirmado mais antigo, ou some. Mas **evento de grupo
  fica com `ownerId` do excluído**;
- o e-mail fica livre para um cadastro novo.

## 26. E-mail: gravado como digitado, procurado de três jeitos

- **Cadastro com e-mail já usado: 500**, não um erro de negócio — quem responde é
  a restrição única do banco (`fluxo-cadastro-email-ja-usado`).
- O cadastro grava o e-mail **como digitado**; o login compara exato; e o
  `forgot-password` procura em **minúsculas**. Quem se cadastrou como
  `Fulano@Gmail.com` loga só com essa grafia e **nunca recebe o link de reset**
  (`fluxo-reset-maiuscula-nao-recebe`).
- Entrar com Google usando o e-mail de uma conta com senha cria **outra conta**,
  sem ligação com a primeira.

## 27. Cada login de conta não verificada manda um e-mail — e depende do Resend

`POST /auth/login` de conta não verificada gera um token novo e manda outro e-mail
**a cada login** (sem espera), e o link anterior deixa de valer. Se o Resend
falhar, o login responde **500**, embora a senha esteja certa. No cadastro é
igual: a conta é criada, mas a resposta é 500.

O reenvio manual tem espera de 3 minutos (429). O horário do último envio não é
gravado: é deduzido de `validade − 24 h`.

## 28. Três consertos no Go que o cliente não percebe

*Lido no código Kotlin, não medido.*

Onde o Kotlin tem um defeito cujo conserto não muda nenhuma resposta bem-sucedida:

- **Criação de evento numa transação.** No Kotlin o evento é gravado antes de os
  detalhes (`sport`/`gameplay`) serem validados; um detalhe inválido responde 400
  e deixa um evento órfão no banco. No Go, valida antes e grava junto.
- **Código de convite.** A checagem de unicidade do Kotlin compara o código sem o
  prefixo `$-` com a coluna que o guarda com prefixo — nunca encontra nada. No Go
  compara o código inteiro.
- **Username de anônimo repetido.** `user_` + 6 hexadecimais pode colidir; o
  Kotlin responde 500, o Go sorteia outro.

O `EmailService` do Kotlin também monta o JSON do Resend por concatenação (aspas
no assunto ou no destinatário quebrariam o envio); o Go usa um codificador.

## 29. Rotas sem checagem de dono

*Lido no código Kotlin, não medido.*

Paridade mantida, mas vale saber: **todas as rotas de times** (`/events/{id}/teams/**`)
e `DELETE /devices/{token}` aceitam qualquer usuário autenticado. Quem conhece o
id de um evento apaga os times dele; quem conhece um token de push o desregistra.

## 30. `displayname` em branco no `PATCH /users/me` dá 500

O código trata `""` como "limpar" e grava `null`, mas a coluna é `NOT NULL`. Para
`avatarUrl` o mesmo caminho funciona (§13).

## 31. O que o Spring faz com requisição malformada — tudo medido, tudo reproduzido

Da família do §18. O Go responde igual; os testes citados travam cada item.

| Requisição | Resposta |
|---|---|
| rota que não existe, método errado, `{id}` que não é UUID, arquivo de `/files` que não existe | **500** `Unexpected error` |
| corpo que não é JSON, `Content-Type` errado, campo obrigatório ausente, enum desconhecido | **500** |
| upload sem a parte `file`, ou acima de 1 MB | **500** |
| token ruim — até em rota pública | **401** com o motivo no `WWW-Authenticate` |
| `HEAD` numa rota pública de `GET`, sem token | **401** (a lista de rotas públicas casa por método exato) |
| caminho com `//`, `;`, `%2e`, `%2f`... | **401** `Bearer` (o firewall do Spring Security) |

Detalhes que só aparecem medindo: o Jackson converte escalares entre si (`123`
num campo texto, `"90"` e `90.7` num inteiro, `1` num booleano, o **índice** num
enum); o `UUID.fromString` aceita `1-1-1-1-1`; a senha passa de 72 bytes e o
BCrypt devolve 400 com a mensagem dele; as mensagens de validação seguem o
`Accept-Language`; e, com mais de uma violação, **a ordem do `fieldErrors` muda a
cada chamada** no próprio Kotlin (o Go usa a ordem de declaração).
Testes: `internal/httpx`, `internal/auth`, `internal/files`, `internal/server`.

## 32. Jobs de notificação rodam mesmo com o envio desligado

*Lido no código Kotlin, não medido.*

No Kotlin, com `NOTIFICATIONS_ENABLED=false` os jobs agendados continuam rodando
e registram como enviado o que não enviaram. Com dois processos no mesmo banco,
um lado desligado engoliria os avisos do outro. No Go os jobs só rodam com o
envio ligado, e `NOTIFICATIONS_SCHEDULER_ENABLED=false` os desliga à parte — é o
recomendado enquanto o Kotlin estiver no ar.

---

## Cobertura: o que foi medido onde

Os golden de `contract/golden/` vêm da **produção** e não têm os caminhos felizes
de cadastro, verificação de e-mail, reset e troca de senha, exclusão de conta e
login com Google — o token chega por e-mail, a exclusão destrói a conta de teste,
o Google exige um ID token de verdade.

Esses fluxos foram medidos no **ambiente local** (`contract/local/`, Kotlin no
commit `4c22311` com Resend, RAWG e Google falsos) e estão em
`contract/golden-local/`: 166 cenários, os 73 da suíte mais 93 dos fluxos
(`contract/flows.py`). Kotlin e Go dão **zero diferenças** neles, e o `go test`
compara o Go com essa gravação a cada execução.

O que continua sem medição nenhuma:

| O quê | Por quê |
|---|---|
| os fluxos felizes **em produção** | os mesmos motivos de sempre; a referência é o Kotlin local |
| envio real de push (FCM) e de e-mail (Resend) | só contra servidores falsos |
| ID token emitido pelo Google de verdade | idem — a chave é do ambiente local |
| RAWG fora do ar pela API (§12) | exigiria derrubar o RAWG |
| o que o Tomcat recusa antes do Spring (URL inválida, cabeçalho gigante) | fica no Kotlin até ele ser desligado |
