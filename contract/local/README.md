# Ambiente local: Kotlin e Go lado a lado

Para comparar os dois sem tocar em produção — e para rodar os fluxos que a
produção não deixa (cadastro, e-mail, exclusão de conta, Google).

```bash
contract/local/oracle.sh up        # Postgres, serviços falsos, Kotlin (:18080) e Go (:18081)
contract/local/oracle.sh compare   # bancos zerados, captura dos dois lados, compare.py
contract/local/oracle.sh down
```

Precisa de `postgresql@17` (Homebrew), JDK, Go, `python3` e `openssl`. Tudo fica
numa pasta de trabalho fora do repositório (`$LOOBBY_ORACLE_DIR`, padrão
`$TMPDIR/loobby-oracle`); nada daqui é segredo.

| Arquivo | O quê |
|---|---|
| `oracle.sh` | sobe, zera, compara e derruba o ambiente |
| `fakes.py` | Resend (guarda os e-mails), RAWG (dados dos golden) e as chaves do "Google" |
| `seed.sql` | as duas contas de teste e o grupo que a suíte espera encontrar |
| `kotlin-oracle.patch` | o que muda no Kotlin **só aqui**: endereço do Resend e das chaves do Google configuráveis |

O Kotlin roda de uma cópia separada (`git worktree`) no commit atual, com o patch
aplicado; o código do repositório não é alterado. Depois de `down`, a cópia some
com `git worktree remove <pasta>/kotlin`.

O Postgres deste ambiente também serve aos testes Go, no banco `loobby`:

```bash
TEST_DATABASE_URL=postgres://postgres@127.0.0.1:54329/loobby go test ./...   # em api-go/
```

`oracle.sh record` regrava `contract/golden-local/` a partir do Kotlin — só se o
Go der zero diferenças, para a referência nunca ficar à frente do Go.

As portas 18080 e 18081 são as mesmas dos testes do nginx: derrube o ambiente
antes de rodar `deploy/nginx/test-*.sh`.
