# Ambiente local: o Go com serviços externos falsos

Para rodar a API sem tocar em produção — inclusive os fluxos que a produção não
deixa (cadastro, e-mail, exclusão de conta, Google).

```bash
contract/local/oracle.sh up        # Postgres, serviços falsos e o Go (:18081)
contract/local/oracle.sh compare   # banco zerado, captura do Go, compare.py contra contract/golden-local/
contract/local/oracle.sh down
```

Precisa de `postgresql@17` (Homebrew), Go, `python3` e `openssl`. Tudo fica numa
pasta de trabalho fora do repositório (`$LOOBBY_ORACLE_DIR`, padrão
`$TMPDIR/loobby-oracle`); nada daqui é segredo.

| Arquivo | O quê |
|---|---|
| `oracle.sh` | sobe, zera, compara e derruba o ambiente |
| `fakes.py` | Resend (guarda os e-mails), RAWG (dados dos golden) e as chaves do "Google" |
| `seed.sql` | as duas contas de teste e o grupo que a suíte espera encontrar |

O Postgres deste ambiente também serve aos testes Go, no banco `loobby`:

```bash
TEST_DATABASE_URL=postgres://postgres@127.0.0.1:54329/loobby go test ./...   # em api-go/
```

O nome `oracle` vem de quando o Kotlin rodava aqui ao lado do Go, como o oráculo
que gravou o `contract/golden-local/`. O Kotlin foi arquivado: a versão do script
que o sobe, com o `record` que regravava a referência, ficou na tag `arquivo/v1`.
