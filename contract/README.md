# Suíte de contrato — Fase 0 da migração para Go

Congela o comportamento observável da API Kotlin para servir de critério de aceite
da reescrita em Go. O projeto tem 1 teste (`contextLoads`) e 3 apps em produção;
esta suíte é a rede de segurança.

## Arquivos

| Arquivo | O quê |
|---|---|
| `ENDPOINTS.md` | inventário dos 53 endpoints + auth por rota |
| `capture.py` | executa a suíte e grava os golden |
| `compare.py` | compara duas capturas — **o critério de aceite** |
| `golden/` | baseline capturada da API Kotlin |
| `bootstrap_user2.py` | cria, uma vez, a 2ª conta de teste |
| `ACHADOS.md` | **leia isto** — as divergências que quebrariam os apps |

## Uso

Precisa de `.env` na raiz (ignorado pelo git):

```
LOOBBY_BASE_URL=https://api.loobby.app
LOOBBY_TEST_EMAIL=...
LOOBBY_TEST_PASSWORD=...
```

```bash
python3 contract/capture.py --include-uploads                        # baseline
python3 contract/capture.py --include-uploads --out contract/depois  # depois de uma mudança
python3 contract/compare.py contract/golden contract/depois          # 0 diferenças = paridade
```

### Durante o strangler, a suíte roda contra o proxy

Não contra o Go direto. A suíte começa com `/auth/login`, e `auth` é o **último**
módulo a migrar — o Go sozinho não consegue nem autenticar. Rodando contra
produção (o nginx), cada requisição vai para o lado que o `deploy/nginx/loobby-routes.conf`
manda, e a invariante do strangler vira um comando:

> **mover uma rota do Kotlin para o Go não pode mudar a baseline.**

Antes do cutover, os testes Go (`api-go/internal/server`) já comparam o módulo
contra os golden sem precisar de servidor nem banco.

### Por que não `diff -ru`

Duas capturas do **mesmo** Kotlin diferem em dezenas de arquivos — medido: 42 de
73 —, porque o `body_raw` carrega UUIDs, timestamps e códigos de convite reais.
O `compare.py` compara status, headers de contrato e a visão normalizada do
corpo, e foi validado nos dois sentidos: duas capturas do Kotlin dão **zero**
diferenças, e mutações como `25.50`→`25.5`, `null`→chave ausente, `[null]`→`[]` ou
um byte no HTML legal são todas pegas.

Goldens de `games-*` dependem do conteúdo do RAWG, que muda com o tempo; para eles
a comparação é estrutural — chaves, tipos e o **formato** dos números (`#.##` vs
`#.#`), não os valores. Pega o `0.0` vs `0.00` sem alarme falso quando o RAWG
atualiza uma nota.

A precisão da fração dos timestamps (0/3/6/9 dígitos) **não** é comparada aqui —
varia por valor até dentro do Kotlin. Quem trava isso é o
`httpx.TestInstantGrouping`, do lado Go.

Flags, todas fora do default porque deixam rastro na produção:

| Flag | Efeito |
|---|---|
| `--include-uploads` | multipart de avatar e imagem de grupo, mais `GET /files/**` |
| `--include-email` | `forgot-password` e `resend-verification` — mandam e-mail de verdade |
| `--keep` | não apaga os recursos criados |

**Use as mesmas flags nas duas capturas**, senão o `compare` acusa golden ausente
em vez de diferença de comportamento.

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

**Uma mudança por vez entre baselines.** Se o deploy destas correções e o cutover
de uma rota para o Go caírem no mesmo intervalo, o `compare` mistura as duas
causas e não dá para saber qual mexeu no quê.

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

Os nomes dos golden são estáveis (`legal-privacy.json`, não `052-legal-privacy.json`):
com numeração sequencial, uma seção pulada desalinhava todos os arquivos seguintes.

O corpo cru de cada resposta textual é gravado **literalmente** em `body_raw`. Isso não é
redundância: `0.00` vira `0` num Go com `float64`, e só o texto cru denuncia. Ao
lado, `body` guarda uma visão normalizada — ids gerados, timestamps e JWTs viram
placeholders — para que duas execuções deem diff limpo.
