# Suíte de contrato — Fase 0 da migração para Go

Congela o comportamento observável da API Kotlin para servir de critério de aceite
da reescrita em Go. O projeto tem 1 teste (`contextLoads`) e 3 apps em produção;
esta suíte é a rede de segurança.

## Arquivos

| Arquivo | O quê |
|---|---|
| `ENDPOINTS.md` | inventário dos 53 endpoints + auth por rota |
| `capture.py` | executa a suíte e grava os golden |
| `golden/` | baseline capturada da API Kotlin |
| `ACHADOS.md` | **leia isto** — as divergências que quebrariam os apps |

## Uso

Precisa de `.env` na raiz (ignorado pelo git):

```
LOOBBY_BASE_URL=https://api.loobby.app
LOOBBY_TEST_EMAIL=...
LOOBBY_TEST_PASSWORD=...
```

```bash
python3 contract/capture.py                                    # baseline (Kotlin)
python3 contract/capture.py --base-url http://localhost:8081 --out contract/go
diff -ru contract/golden contract/go                           # critério de aceite
```

Flags, todas fora do default porque deixam rastro na produção:

| Flag | Efeito |
|---|---|
| `--include-uploads` | multipart de avatar e imagem de grupo, mais `GET /files/**` |
| `--include-email` | `forgot-password` e `resend-verification` — mandam e-mail de verdade |
| `--keep` | não apaga os recursos criados |

**Use as mesmas flags nos dois lados** ao comparar Kotlin e Go, senão o `diff`
acusa arquivos ausentes em vez de diferenças de comportamento.

`--include-uploads` está atrás de flag porque o `FileStorageService.store()` nomeia
o arquivo como `<epochMillis>.<ext>` e **nunca apaga o anterior**: cada execução
deixaria um arquivo permanente no disco do servidor, sem rota de exclusão pela API.

## As divergências abertas são a checklist de deploy

Hoje a suíte acusa **7 status fora do esperado**. Não é ruído: cada um é um
endpoint que as correções no working tree consertam, e que a produção ainda não
recebeu. As expectativas já apontam para o comportamento pós-deploy.

| Divergência | Hoje | Pós-deploy | Correção |
|---|---|---|---|
| `games-details-inexistente` | 500 | 404 | ACHADOS §6 |
| `grupo-patch-sem-permissao` | 500 | 403 | ACHADOS §11 |
| `grupo-delete-sem-permissao` | 500 | 403 | ACHADOS §11 |
| `senha-nova-curta` | 500 | 400 | ACHADOS §6 |
| `senha-confirmacao-diferente` | 500 | 400 | ACHADOS §6 |
| `senha-atual-errada` | 500 | 401 | ACHADOS §6 |
| `excluir-conta-senha-errada` | 500 | 401 | ACHADOS §6 |

**A lista zerar é o sinal de que o deploy pegou.** Depois disso, regrave a baseline
(`rm contract/golden/*.json && python3 contract/capture.py --include-uploads`) —
e a partir daí qualquer divergência nova é regressão de verdade.

## Como a suíte se comporta

Roda **contra produção**, por decisão explícita. Para isso:

- só cria e apaga recursos próprios (grupo, eventos, times, device token), num
  `finally` que roda mesmo se a suíte falhar no meio;
- nunca toca em dados preexistentes;
- deixa de fora, por padrão, tudo que tem efeito externo (e-mail) ou destrói a
  conta de teste (`DELETE /users/me`).

Respostas binárias (imagens de `/files/**`) não vão para `body_raw` — bytes
não-UTF-8 viram `\ufffd` na decodificação, e comparar a string mutilada não provaria
nada. Elas viram `body_binary: {bytes, sha256}`, que é o que precisa bater.

O corpo cru de cada resposta textual é gravado **literalmente** em `body_raw`. Isso não é
redundância: `0.00` vira `0` num Go com `float64`, e só o texto cru denuncia. Ao
lado, `body` guarda uma visão normalizada — ids gerados, timestamps e JWTs viram
placeholders — para que duas execuções deem diff limpo.
