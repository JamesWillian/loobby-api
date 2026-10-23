# Inventário do contrato — Loobby API (Kotlin/Spring)

Base para a **Fase 0** da migração para Go: cada linha precisa de pelo menos um
golden JSON gravado contra a API Kotlin rodando, antes de qualquer linha de Go.

**Commit base:** `81c068f` · 53 endpoints em 8 controllers + 2 rotas de infra.

**Cobertura atual: 44/53 completos + 7 parciais (só erro) = 51/53 com contrato capturado** (+ `/actuator/health`), em 72 golden — veja
`ACHADOS.md` para o que a captura revelou. Os 14 abertos estão listados no fim,
cada um com o motivo.

Legenda de auth (de `config/security/SecurityConfig.kt`):
`pub` = `permitAll` · `jwt` = `authenticated`

Legenda de cobertura: ☑ coberto · **◐ só o caminho de erro** · ☐ sem golden

Os ◐ são os endpoints cujo caminho feliz exige um token que chega por e-mail, ou
destrói a conta de teste. Ficam pendentes até o cutover das fases 2 (`auth`) e 3
(`users`) — que são as **últimas** da ordem do strangler, então há semanas de folga.
O que **não** pode acontecer: alterar essas rotas no Kotlin antes de gravar o golden
do caminho feliz. Se elas mudarem, a referência vira ficção.

## auth — `AuthController` (10)

| ✓ | Método | Rota | Auth | Notas de captura |
|---|---|---|---|---|
| ☑ | POST | `/auth/anonymous` | pub | ponto de entrada: gera o JWT usado no resto da suíte |
| ◐ | POST | `/auth/register` | jwt | exige token anônimo válido |
| ☑ | POST | `/auth/login` | pub | gravar também 401 (senha errada) |
| ◐ | POST | `/auth/google` | pub | 3 client IDs; mockar o verificador ou gravar só o caminho de erro |
| ☑ | POST | `/auth/refresh` | pub | gravar 401 de refresh expirado (regressão de `d635718`) |
| ◐ | GET | `/auth/verify-email` | pub | resposta HTML, não JSON |
| ☐ | POST | `/auth/resend-verification` | jwt | cooldown de 3 min → gravar o 429/erro |
| ☐ | POST | `/auth/forgot-password` | pub | efeito colateral: Resend |
| ◐ | GET | `/auth/reset-password` | pub | HTML (formulário) |
| ◐ | POST | `/auth/reset-password` | pub | form-encoded, não JSON |

## users — `UsersController` (6)

| ✓ | Método | Rota | Auth | Notas de captura |
|---|---|---|---|---|
| ☑ | GET | `/users/me` | jwt | |
| ☑ | PATCH | `/users/me` | jwt | campos nulos: conferir se saem como `null` ou somem |
| ☑ | POST | `/users/me/avatar` | jwt | multipart |
| ☑ | GET | `/users/feed` | jwt | `user_feed` existe só no Supabase, não nas migrations |
| ◐ | PUT | `/users/me/password` | jwt | |
| ◐ | DELETE | `/users/me` | jwt | destrutivo: rodar por último no seed |

## groups — `GroupController` (11)

| ✓ | Método | Rota | Auth |
|---|---|---|---|
| ☑ | POST | `/groups` | jwt |
| ☑ | GET | `/groups` | jwt |
| ☑ | GET | `/groups/{id}` | jwt |
| ☑ | GET | `/groups/invite/{code}` | jwt |
| ☑ | PATCH | `/groups/{groupId}` | jwt |
| ☑ | POST | `/groups/{groupId}/image` | jwt |
| ☑ | DELETE | `/groups/{groupId}` | jwt |
| ☑ | POST | `/groups/{groupId}/members` | jwt |
| ☑ | GET | `/groups/{groupId}/members` | jwt |
| ☑ | DELETE | `/groups/{groupId}/members/me` | jwt |
| ☑ | DELETE | `/groups/{groupId}/members/{memberId}` | jwt |

## events — `EventController` (11)

| ✓ | Método | Rota | Auth | Notas de captura |
|---|---|---|---|---|
| ☑ | POST | `/groups/{groupId}/events` | jwt | 3 `EventType`: GAMEPLAY, SPORT, PARTY |
| ☑ | POST | `/events/instant` | jwt | conferir o nome exato de `isInstant` no JSON |
| ☑ | GET | `/groups/{groupId}/events` | jwt | ordenação importa |
| ☑ | GET | `/events/{eventId}` | jwt | **`pricePerPlayer` → `0.00` vs `0`** |
| ☑ | GET | `/events/invite/{code}` | jwt | |
| ☑ | PUT | `/events/{eventId}` | jwt | |
| ☑ | DELETE | `/events/{eventId}` | jwt | cascata é do banco |
| ☑ | PUT | `/events/{eventId}/rsvps` | jwt | dispara notificação (debouncer) |
| ☑ | GET | `/events/{eventId}/rsvps` | jwt | **`confirmedAvatars`: `null` ou `[]`?** |
| ☑ | GET | `/events/{eventId}/rsvps/me` | jwt | gravar o caso "sem RSVP" |
| ☑ | DELETE | `/events/{eventId}/rsvps/me` | jwt | |

## events/teams — `EventTeamController` (8)

| ✓ | Método | Rota | Auth |
|---|---|---|---|
| ☑ | POST | `/events/{eventId}/teams` | jwt |
| ☑ | GET | `/events/{eventId}/teams` | jwt |
| ☑ | PUT | `/events/{eventId}/teams/{teamId}` | jwt |
| ☑ | DELETE | `/events/{eventId}/teams/{teamId}` | jwt |
| ☑ | POST | `/events/{eventId}/teams/{teamId}/players` | jwt |
| ☑ | PUT | `/events/{eventId}/teams/{teamId}/players/{userId}` | jwt |
| ☑ | DELETE | `/events/{eventId}/teams/{teamId}/players/{userId}` | jwt |
| ☑ | POST | `/events/{eventId}/teams/auto-generate` | jwt |

## games — `GameController` (2)

**Go: implementado e igual à produção** (`api-go/internal/games`): a busca e o
detalhe do cache idênticos aos golden, e o detalhe vindo do RAWG idêntico a
respostas gravadas da produção. Aguardando cutover.

| ✓ | Método | Rota | Auth | Notas de captura |
|---|---|---|---|---|
| ☑ | GET | `/games/search` | jwt | proxy RAWG: gravar hit de cache, miss e circuito aberto |
| ☑ | GET | `/games/{id}` | jwt | idem |

## notifications — `DeviceTokenController` (2)

| ✓ | Método | Rota | Auth |
|---|---|---|---|
| ☑ | POST | `/devices/register` | jwt |
| ☑ | DELETE | `/devices/{token}` | jwt |

## legal — `LegalController` (3)

**Go: implementado e igual aos golden** (`api-go/internal/legal`). Aguardando
cutover no nginx — ver `deploy/nginx/README.md`.

| ✓ | Método | Rota | Auth | Notas |
|---|---|---|---|---|
| ☑ | GET | `/legal/privacy` | pub | `text/html;charset=UTF-8`, `max-age=3600, public` |
| ☑ | GET | `/legal/account-deletion` | pub | idem |
| ☑ | GET | `/legal/child-safety` | pub | idem |

## infra (fora dos controllers)

| ✓ | Método | Rota | Auth | Notas |
|---|---|---|---|---|
| ☑ | GET | `/files/**` | pub | estático de `./uploads`; conferir `Content-Type` |
| ☑ | GET | `/actuator/health` | pub | usado por healthcheck de deploy |

## Erros — capturar transversalmente

O `ApiExceptionHandler` é contrato tanto quanto os 200. Gravar pelo menos um
exemplo de cada forma de `ApiErrorResponse`:

Status **medidos**, não os que o código sugere — ver `ACHADOS.md` §5.

| ✓ | Cenário | Status real |
|---|---|---|
| ☑ | body inválido (Bean Validation) | 400 |
| ☑ | sem `Authorization` | 401 |
| ☑ | refresh token inválido | 401 |
| ☑ | credenciais erradas | **400**, não 401 |
| ☑ | id inexistente (grupo, time) | **400**, não 404 |
| ☑ | acesso a recurso de outro dono | **500**, não 403 — ver ACHADOS §11 |
| ☐ | conflito (`DataIntegrityViolation`) | ? — não reproduzido |
| ☐ | refresh token **expirado** (≠ inválido) | ? — regressão de `d635718`, precisa de token velho |
