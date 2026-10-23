# Respostas reais de GET /games/{id}, gravadas da produção

Capturadas em 23/10/2026 de `https://api.loobby.app`, para os jogos 257 e 21482,
em duas chamadas seguidas a cada um:

| Arquivo | Chamada | Origem do dado no Kotlin |
|---|---|---|
| `<id>-fresco.json` | 1ª | RAWG, na hora (o jogo ainda não tinha descrição no cache) |
| `<id>-cache.json` | 2ª | tabela `games` do Postgres |

A única diferença entre as duas, byte a byte, é o rating: `0.0` e `0.00`. O
primeiro vem de `JsonNode.decimalValue()` sobre o double do RAWG; o segundo, da
coluna `numeric(4,2)`. O teste `TestGamesFrescoECacheIdenticosAProducao`
exige que o Go reproduza as duas respostas, na mesma ordem de chamadas.

São respostas brutas da API (sem normalização), e não passam pelo
`contract/capture.py`: o caminho "fresco" exige um jogo que ninguém abriu ainda,
e por isso não dá para regravar a cada execução da suíte.
