# Inventário do contrato — Loobby API

Os 53 endpoints em 8 controllers, mais 2 rotas de infra. **Todos estão
implementados em Go** (`api-go/`) e respondem igual ao Kotlin nos 166 cenários de
`contract/golden-local/`. O que ainda falta é o cutover no nginx, módulo a módulo
(`deploy/nginx/README.md`): hoje o painel liga só `legal` e `games`.

Legenda de auth (de `config/security/SecurityConfig.kt`):
`pub` = `permitAll` · `jwt` = `authenticated`

Legenda de cobertura:

| | |
|---|---|
| ☑ | gravado da **produção** (`contract/golden/`, 73 cenários) |
| ▣ | em produção só o caminho de erro; o fluxo completo está gravado do Kotlin **local** (`contract/golden-local/`, `fluxo-*`) |

Os ▣ são os endpoints cujo caminho feliz exige um token que chega por e-mail, um ID
token do Google, ou destrói a conta de teste. Ver "Cobertura" no `ACHADOS.md`.

## auth — `AuthController` (10)

| ✓ | Método | Rota | Auth | Notas de captura |
|---|---|---|---|---|
| ☑ | POST | `/auth/anonymous` | pub | ponto de entrada: gera o JWT usado no resto da suíte |
| ▣ | POST | `/auth/register` | jwt | exige o token do anônimo · e-mail já usado dá 500 (ACHADOS §26) |
| ☑ | POST | `/auth/login` | pub | gravar também 401 (senha errada) |
| ▣ | POST | `/auth/google` | pub | 3 client ids · tolera 5 min de relógio |
| ☑ | POST | `/auth/refresh` | pub | vencido de anônimo é renovado · **Go recusa token forjado (ACHADOS §22)** |
| ▣ | GET | `/auth/verify-email` | pub | resposta HTML, não JSON |
| ▣ | POST | `/auth/resend-verification` | jwt | espera de 3 min → 429 |
| ▣ | POST | `/auth/forgot-password` | pub | sempre 200 · procura o e-mail em minúsculas (ACHADOS §26) |
| ▣ | GET | `/auth/reset-password` | pub | HTML (formulário) |
| ▣ | POST | `/auth/reset-password` | pub | recebe JSON, responde HTML |

## users — `UsersController` (6)

| ✓ | Método | Rota | Auth | Notas de captura |
|---|---|---|---|---|
| ☑ | GET | `/users/me` | jwt | |
| ☑ | PATCH | `/users/me` | jwt | campos nulos: conferir se saem como `null` ou somem |
| ☑ | POST | `/users/me/avatar` | jwt | multipart |
| ☑ | GET | `/users/feed` | jwt | `user_feed` existe só no Supabase, não nas migrations |
| ▣ | PUT | `/users/me/password` | jwt | conta Google: 404 (ACHADOS §24) |
| ▣ | DELETE | `/users/me` | jwt | **só a 1ª exclusão funciona (ACHADOS §23)** |

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
| ☑ | acesso a recurso de outro dono | 403 (a produção gravou 500 antes da correção — ACHADOS §11) |
| ▣ | conflito (restrição única do banco) | **500** — `fluxo-cadastro-email-ja-usado`, `fluxo-exclusao-2a-conta` |
| ▣ | refresh token **vencido** (≠ inválido) | 200 para anônimo, 401 para conta com senha |
