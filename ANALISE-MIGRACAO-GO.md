# Loobby API — Viabilidade de migração para Golang

**Contexto:** deploy alvo = Oracle `VM.Standard.E2.1.Micro` (1 OCPU AMD EPYC, **1 GB RAM**, boot volume 47 GB). Postgres externo (gerenciado). Motivação declarada: **caber em 1 GB**.

**Data:** 28/07/2026 · **Commit base:** `81c068f`

---

## Veredito

**Migração viável, com perfil de risco acima da média** — mas provavelmente desnecessária para o problema que você quer resolver.

Duas conclusões independentes:

1. **O código é atipicamente fácil de portar.** O projeto já está escrito num estilo que mapeia quase 1:1 para Go: entidades planas, FKs por UUID, `save()` explícito, zero grafo de objetos. A maior dor de qualquer migração Spring→Go (desmontar JPA) praticamente não existe aqui.

2. **O gargalo de 1 GB tem soluções de 1–2 dias.** A reescrita custa ~6 semanas full-time. Antes de pagar isso, vale gastar dois dias nas alternativas — em especial trocar o Firebase Admin SDK, que é de longe a dependência mais pesada do projeto.

**Recomendação:** sequência A → B → medir (§5). Se couber com folga, o Go vira decisão de preferência — legítima, e executável sem pressa via *strangler* (§7). Se não couber, Go é o alvo certo e o plano está em §6.

---

## 1. Retrato do projeto

| Métrica | Valor |
|---|---|
| Linhas de Kotlin | **6.124** (118 arquivos) |
| Endpoints REST | **53** em 8 controllers |
| Entidades JPA | 17 |
| Repositórios | 15 |
| Queries `@Query` customizadas | **3** |
| Anotações de relacionamento JPA (`@OneToMany`/`@ManyToOne`/`@JoinColumn`) | **2 no projeto inteiro** |
| Testes | **1** (`contextLoads`) |

**Stack:** Kotlin 2.3.10 · Spring Boot 3.5.6 · JPA/Hibernate · Postgres · Spring Security OAuth2 Resource Server (JWT HS256) · Caffeine · Firebase Admin SDK 9.3.0 · Google API Client · Resend (via `java.net.http` cru)

### LOC por módulo

| Módulo | LOC | Natureza |
|---|---|---|
| `events` (+ `teams`) | 1.771 | Regra de negócio densa — RSVP, times, reservas |
| `notifications` | 1.119 | FCM, 2 crons, dispatch log, debouncer |
| `auth` | 1.045 | JWT, Google, bcrypt, verify-email, reset |
| `users` | 621 | Perfil, avatar, feed |
| `groups` | 539 | CRUD + convites + membros |
| `games` | 517 | Proxy RAWG + cache + circuit breaker |
| `common` | 251 | Erros, email, storage |
| `config` | 207 | Security, JWT, web |
| `legal` | 43 | HTML estático |

---

## 2. Por que este código é fácil de portar

Este é o achado mais relevante da análise. A migração Spring→Go costuma ser cara por causa do Hibernate: lazy loading, dirty checking implícito, cascatas em nível de ORM, grafos de objetos. **Nada disso está em uso aqui.**

Evidências:

- **2 anotações de relacionamento no projeto inteiro** (um único `@OneToOne(LAZY)` em `UserCredentialsEntity`). Todo o resto usa `UUID` cru como FK: `event.groupId`, `event.ownerId`, `rsvp.userId`.
- **Cascatas são do banco, não do JPA.** O schema já declara `on delete cascade` nas FKs, e os comentários no código confirmam a intenção (`// on delete cascade no banco remove os players`). Isso sobrevive intacto à migração.
- **`save()` é sempre explícito.** `updateEvent` muta os campos e chama `eventRepository.save(event)` — não depende de auto-flush do contexto de persistência. Esse é o padrão em todo o codebase.
- **Só 3 `@Query` customizadas.** O resto são métodos derivados (`findByEventIdOrderByCreatedAtAsc`), que viram ~43 queries SQL escritas à mão. Trabalho mecânico e de baixo risco — `sqlc` gera o Go tipado a partir do SQL.
- **`open-in-view: false`** já está setado. Sem lazy loading vazando para a camada de view.

Na prática, as entidades já são structs planas com um repositório por cima. É exatamente a forma que o código Go teria.

---

## 3. Orçamento de memória em 1 GB

Com Postgres externo, o orçamento inteiro da VM fica para a aplicação.

```
Total da VM                              1.024 MB
  – SO (Ubuntu/Oracle Linux minimal)       ~150 MB
  ────────────────────────────────────────────────
  Disponível para a aplicação              ~875 MB
```

### Spring Boot hoje (RSS estimado)

| Componente | Estimativa |
|---|---|
| Heap (ativo, com `MaxRAMPercentage` default) | 120–200 MB |
| **Metaspace** (Spring + Hibernate + Security + **Firebase Admin/gRPC/protobuf/Guava**) | **110–150 MB** |
| Code cache (JIT) | 30–60 MB |
| Thread stacks (Tomcat 200 + pools gRPC do Firebase) | 20–60 MB |
| GC, buffers diretos, overhead nativo | 40–80 MB |
| **Total realista** | **450–650 MB** |

**Cabe?** Sim, mas com 225–425 MB de folga e exposto ao OOM killer nos picos: upload multipart de avatar, job noturno de notificações (`dailyMorningJob` às 08:00 varre eventos + membros + dispara FCM em lote), e o job de lembrete que roda a cada 5 minutos.

### Equivalente em Go

| Métrica | Kotlin/Spring | Go | Fator |
|---|---|---|---|
| RSS em regime | 450–650 MB | **25–45 MB** | **~13x** |
| Startup | 8–20 s (core burstable) | ~30–80 ms | ~200x |
| Imagem Docker | ~450 MB (`jre-alpine` + jar) | ~20 MB (distroless/scratch) | ~22x |
| Artefato | jar + JRE | binário estático único | — |

O ganho é real e grande. A questão é se ele justifica o custo agora.

---

## 4. O elefante: Firebase Admin SDK

`com.google.firebase:firebase-admin:9.3.0` arrasta gRPC, protobuf, Guava, `google-http-client` e uma pilha de `google-cloud-*`. É a dependência mais pesada do projeto por uma margem confortável — e é usada para exatamente **uma** coisa: enviar push via FCM.

O que ela faz por você poderia ser substituído por:

1. Obter um access token OAuth2 a partir do service account JSON (`google-auth-library-oauth2-http`, ordens de grandeza mais leve — ou até um JWT assinado à mão).
2. `POST https://fcm.googleapis.com/v1/projects/{projectId}/messages:send`

**Superfície de mudança:** `FcmSender.kt` (124 LOC) e `FirebaseConfig.kt` (123 LOC). **Ganho estimado:** 80–150 MB entre metaspace e heap, mais a eliminação dos pools de threads do gRPC.

Isso sozinho pode resolver o problema de 1 GB. É a intervenção de melhor relação custo/benefício disponível.

---

## 5. Alternativas antes de reescrever

Em ordem de custo crescente.

### A. Tuning da JVM — algumas horas

```bash
JAVA_TOOL_OPTIONS="-Xmx256m -Xss512k -XX:MaxMetaspaceSize=160m \
  -XX:+UseSerialGC -XX:TieredStopAtLevel=1 \
  -XX:ReservedCodeCacheSize=64m -XX:+ExitOnOutOfMemoryError"
```

Em `application.yaml`:

```yaml
server:
  tomcat:
    threads:
      max: 40          # default é 200
spring:
  datasource:
    hikari:
      maximum-pool-size: 5   # importante: DB externo, cada conexão tem custo dos dois lados
      minimum-idle: 2
```

`UseSerialGC` é a escolha certa num único core — G1 mantém estruturas e threads que não se pagam aqui. `TieredStopAtLevel=1` corta o C2, reduzindo code cache e CPU de compilação em troca de pico de throughput (irrelevante nesta escala).

**RSS esperado:** 280–380 MB. Adicione 2 GB de swap no boot volume de 47 GB como rede de segurança para páginas frias.

### B. Remover o Firebase Admin SDK — ~1 dia

Ver §4. Ganho de 80–150 MB tocando 2 arquivos.

### C. AppCDS — ~2 horas

Spring Boot 3.3+ suporta nativamente. Reduz startup em 30–40% e compartilha metadados de classe. Ganho de 20–40 MB e um boot bem mais rápido — relevante num core burstable, onde 20 s de startup é dor real em cada deploy.

### D. GraalVM Native Image — dias, risco médio-alto

60–120 MB de RSS, startup sub-segundo, sem reescrever nada. Porém:

- O build precisa de 4–8 GB de RAM — tem que rodar no CI, nunca na micro.
- Hibernate tem suporte oficial via Spring AOT, e funciona.
- **O Firebase Admin SDK é o problema conhecido** em native image (reflexão pesada + gRPC). Pode virar um poço de tempo.

Só considere **depois** de (B). Sem `firebase-admin`, o native image fica bem mais plausível.

### E. Migrar para Go

Ganho garantido de ~13x. Custo em §6.

---

## 6. Plano e estimativa da migração para Go

### Stack recomendada

| Camada | Escolha | Nota |
|---|---|---|
| Router | `net/http` (Go 1.22+ patterns) ou `chi` | Stdlib basta para 53 rotas |
| Banco | `pgx/v5` + **`sqlc`** | Gera Go tipado a partir do SQL — análogo mais próximo dos repositórios atuais |
| Migrations | `goose` | Substitui o Flyway desabilitado |
| JWT | `golang-jwt/jwt/v5` | HS256, **mesmo segredo** |
| Senhas | `golang.org/x/crypto/bcrypt` | **Compatível com os hashes `$2a$` do Spring** |
| Google ID token | `google.golang.org/api/idtoken` | Uma função, valida os 3 client IDs |
| FCM | `firebase.google.com/go/v4` ou HTTP v1 direto | SDK oficial existe e é bem mais leve que o Java |
| Cron | `robfig/cron/v3` | Suporta timezone (`America/Sao_Paulo`) |
| Cache | `jellydator/ttlcache` | Substitui o Caffeine |
| Validação | `go-playground/validator/v10` | Substitui o Bean Validation |
| Decimal | `shopspring/decimal` | **Obrigatório** — ver §8 |

### Esforço por fase

| # | Fase | Escopo | Dias-pessoa | Risco |
|---|---|---|---|---|
| 0 | **Rede de segurança** | Capturar testes de contrato contra a API Kotlin rodando: 53 endpoints, JSON golden, casos de erro | **3–4** | **Bloqueante** |
| 1 | Fundação | `main`, router, pool pgx, config, middleware JWT, mapper de erros, validação | 2–3 | Baixo |
| 2 | Auth | login/register/refresh/google/anonymous, bcrypt, verify-email, reset-password, Resend | 4–5 | Baixo-Médio |
| 3 | Users + Groups | perfil, upload de avatar, feed, membros, convites | 3–4 | Baixo |
| 4 | Events + Teams | maior módulo: RSVP, auto-generate de times, reservas, permissões | **6–7** | **Alto** |
| 5 | Games/RAWG | client HTTP, cache TTL, circuit breaker, cache em Postgres | 2 | Baixo |
| 6 | Notifications | FCM, 2 crons, dispatch log anti-duplicata, debouncer | 4–5 | Médio |
| 7 | Files, Legal, estáticos | multipart, `go:embed`, servir `/files` | 1 | Baixo |
| 8 | Deploy | binário + systemd (ou Docker distroless), CI com cross-compile `linux/amd64` | 1–2 | Baixo |
| | **Total** | | **26–33 dias** | |

**Calendário:** ~6 semanas full-time. Em regime de noites e fins de semana (~10h/semana), 5–6 meses. Some ~50% se for seu primeiro projeto Go em produção.

**Volume de código:** espere 9.000–11.000 LOC em Go. Go é mais verboso — tratamento de erro explícito, SQL à mão, sem DI mágica — mas o resultado é mais direto de ler.

---

## 7. Continuidade: por que dá para migrar sem big bang

Três fatos tornam a migração incremental e reversível:

1. **JWT HS256 com o mesmo `JWT_SECRET`** → tokens emitidos pelo Kotlin continuam válidos no Go. **Zero logout forçado.**
2. **Bcrypt do Spring gera `$2a$`**, formato que `golang.org/x/crypto/bcrypt` valida. **Zero reset de senha.**
3. **Schema aplicado à mão com `ddl-auto: validate`** → o Go só precisa ler as mesmas tabelas. **Zero migração de dados.**

Isso viabiliza o padrão **strangler**: nginx na frente, rotas migradas uma por uma, rollback por rota. E a conta Always Free dá **2× E2.1.Micro** — dá para rodar Kotlin numa e Go na outra durante a transição.

Ordem sugerida (do mais isolado para o mais acoplado):

```
legal → games → files → notifications → groups → users → events → auth
```

Deixe `auth` por último: enquanto os dois lados validam o mesmo JWT, é o componente que menos precisa mudar e o mais caro de errar.

---

## 8. Armadilhas de paridade — onde clientes quebram em silêncio

Você tem apps iOS, Android e web em produção (3 client IDs Google + FCM). O JSON precisa bater. Estes são os pontos concretos que encontrei no código:

| Campo / tipo | Jackson (hoje) | Go ingênuo | Correção |
|---|---|---|---|
| `pricePerPlayer: BigDecimal` (`numeric(12,2)`) | `0.00` — preserva a escala | `0` com `float64` | Usar `shopspring/decimal`; **validar contra o app real** |
| `description: String?` = null | `"description": null` | chave omitida com `omitempty` | Ponteiro **sem** `omitempty` |
| `confirmedAvatars: List<String?>?` | `null` ou array | slice nil → `null`, vazio → `[]` | Decidir explicitamente qual dos dois o app espera |
| `scheduledDatetime: Instant` | ISO-8601, precisão variável | RFC3339 com nanossegundos | Formatar explicitamente com layout fixo |
| `isInstant` | já usa `@get:JsonProperty("isInstant")` — contorno de quirk Kotlin/Jackson | tag de struct explícita | Conferir o nome exato que o app consome |
| `EventType` / `RsvpStatus` | nome do enum (`"GAMEPLAY"`) | string | Direto, sem risco |

**Regra prática:** no Go, não use `omitempty` em nenhum campo de response. Jackson emite `null`; `omitempty` some com a chave. Cliente que faz `json["campo"]` quebra.

**A fase 0 não é opcional.** Reescrever uma API em produção com um único teste `contextLoads` é o cenário em que isso dá errado. Grave as respostas reais da API Kotlin e use como suíte de aceitação do Go — é o que transforma "espero que esteja igual" em "está provado que está igual".

---

## 9. Achados incidentais

Coisas que a análise revelou e que valem atenção independentemente da decisão:

- **`EmailService` monta JSON por interpolação de string.** O `html` é escapado à mão, mas `subject` e `to` **não são**. Um `displayName` com aspas duplas quebra o payload — e o `subject` é montado a partir de dados do usuário em vários fluxos. Vale corrigir hoje, no Kotlin, independente da migração. (No Go isso desaparece de graça com `encoding/json`.)

- **`user_feed` não existe nas migrations.** É uma entidade `@Immutable` lida por query nativa, sobre uma tabela/view criada manualmente no banco — não está em `V1__init.sql` nem em `V2__games_cache.sql`. Precisa ser versionada antes de migrar, senão o lado Go não tem como saber o que ler.

- **Flyway está desabilitado** e o `V2` traz o comentário *"APLIQUE ESTE SCRIPT MANUALMENTE"*. Combinado com `ddl-auto: validate`, um schema desalinhado derruba a aplicação no boot. Bom momento para adotar migrations de verdade.

- **O `Dockerfile` roda `gradle bootJar` dentro do build.** Isso **não vai funcionar** na E2.1.Micro — o Gradle sozinho quer 1–2 GB. Independente da linguagem escolhida, o build precisa sair da VM: CI ou máquina local com push da imagem pronta.

- **Estado em memória impede escala horizontal.** `PeerRsvpDebouncer` e o circuit breaker do `RawgClient` guardam estado no processo. Com 2 instâncias, ambos se comportam de forma inconsistente. Não é regressão da migração — é dívida existente, e o mesmo problema existiria em Go.

---

## 10. Próximos passos sugeridos

**Esta semana (~2 dias, reversível):**

1. Medir o RSS real: `jcmd <pid> VM.native_memory summary` com NMT ligado. Substituir as estimativas da §3 por números.
2. Aplicar o tuning da JVM (§5.A) e medir de novo.
3. Se ainda estiver apertado, remover o Firebase Admin SDK (§4). Sozinho, provavelmente resolve.

**Se couber com folga:** a migração para Go vira preferência técnica, não necessidade. Continua sendo uma boa ideia — startup, deploy e custo operacional melhoram muito — mas dá para fazer por strangler, sem prazo e sem risco.

**Se não couber:** Go é o alvo certo. Comece pela fase 0 (testes de contrato) antes de escrever qualquer linha de produção.
