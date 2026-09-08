# Achados da baseline de contrato

Extraídos dos 47 golden em `contract/golden/`, capturados de `https://api.loobby.app`
em 08/09/2026 (commit `81c068f`). Tudo aqui é **medido**, não inferido do código.

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

## 3. `rating` sai com escalas diferentes conforme o endpoint

| Endpoint | Valor |
|---|---|
| `GET /games/search` | `0.0` |
| `GET /games/{id}` | `0.00` |

**Não é inconsistência aleatória — é o caminho dos dados.** O `search` monta a
resposta a partir da `GameEntity` recém-upsertada em memória, cujo `rating` veio de
`node.path("rating").decimalValue()` — a escala literal do RAWG. O `getDetails` lê
da coluna `rating numeric(4, 2)` (`V2__games_cache.sql:24`), que força escala 2.

**Ação:** replicar os dois caminhos separadamente. Um Go que normalize tudo pela
mesma origem acerta um endpoint e quebra o outro.

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

## Pendente de cobertura

Não entraram na baseline, com o motivo:

| Endpoint | Motivo |
|---|---|
| `POST /auth/register` | precisa do fluxo com token anônimo |
| `POST /auth/google` | precisa de `idToken` real do Google |
| `GET /auth/verify-email`, `GET`/`POST /auth/reset-password` | HTML + token de e-mail |
| `POST /auth/forgot-password`, `/auth/resend-verification` | disparam e-mail — atrás de `--include-email` |
| `POST /users/me/avatar`, `POST /groups/{id}/image` | multipart |
| `GET /files/**` | depende de upload |
| `DELETE /users/me` | destrói a conta de teste |
| `POST /groups/{id}/members`, `DELETE .../members/{id}` | precisam de um 2º usuário |

Os quatro últimos grupos exigem uma segunda conta descartável, criada via
`POST /auth/anonymous`. É o próximo incremento do harness.
