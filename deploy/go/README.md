# Go em produção

## Como está (desde 16/11/2026): só o Go, com nginx na mesma máquina

O AWS (nginx + Kotlin) foi desligado e `api.loobby.app` aponta direto para a
máquina da Oracle Cloud (`163.176.222.85`, Ubuntu 24.04, usuário `ubuntu`):

```
usuário ─▶ nginx :443 (Let's Encrypt) ─ HTTP 127.0.0.1:8080 ─▶ Go (systemd) ─▶ Supabase (pooler 6543)
```

| O quê | Onde |
|---|---|
| Binário / serviço | `/opt/loobby/api`, `loobby-api.service` (este diretório) |
| Variáveis | `/etc/loobby/api.env` (`root:loobby 0640`), **sem** `TLS_*`: o Go escuta só em `127.0.0.1:8080` |
| Firebase | `/etc/loobby/loobby-fcm.json` |
| Uploads | `/var/lib/loobby/uploads` |
| nginx | `nginx-api.loobby.app.conf` → `/etc/nginx/sites-available/api.loobby.app` |
| Certificado | `/etc/letsencrypt/live/api.loobby.app/`, renovado pelo `certbot.timer` (webroot `/var/www/certbot`, recarrega o nginx) |
| Firewall | `iptables` libera 22, 80 e 443 (`netfilter-persistent save`) |

O `api.env` segue o modelo comentado em `api.env.example`. Dois pontos:

- **`DB_URL`** no formato do pgx, com usuário e senha **escapados** dentro da URL (a
  senha tem `@`) e sem `DB_USERNAME`/`DB_PASSWORD`:
  `postgres://USUARIO:SENHA_ESCAPADA@aws-1-sa-east-1.pooler.supabase.com:6543/postgres?sslmode=require&default_query_exec_mode=simple_protocol`.
  O pooler em modo transação (6543) não suporta os prepared statements nomeados
  que o pgx usa por padrão; o `simple_protocol` é o equivalente ao
  `prepareThreshold=0` do JDBC, que **não** pode ir na URL do pgx. A suíte de
  contrato passa inteira nesse modo.
- `NOTIFICATIONS_SCHEDULER_ENABLED=true`: os lembretes agendados são do Go.

**O Supabase do plano gratuito pausa por inatividade.** Em 16/11 o projeto estava
`INACTIVE` e o Go não subia (`tenant/user ... not found` no pooler). Conferir com
`supabase projects list`.

## Publicar uma versão nova

O `deploy.sh` usa `ssh` sem `-i`, então a chave entra por um agente:

```bash
eval "$(ssh-agent -s)" && ssh-add ~/Workspace/SSH/jammes-server/ssh-key-jammes-server.key
deploy/go/deploy.sh ubuntu@163.176.222.85
```

Roda os testes, compila para a arquitetura da máquina (detectada por `uname -m`),
troca o binário e reinicia. Só considera no ar quando o processo escreve
`servidor no ar` no log; se não escrever em ~15 s, **volta o binário anterior**.
Depois, de qualquer lugar:

```bash
curl -s https://api.loobby.app/actuator/health   # {"status":"UP"}
```

## Preparar uma máquina do zero

Nos comandos, `MAQUINA` é `usuario@ip-da-maquina`.

**1. Usuário e diretórios.**

```bash
ssh MAQUINA 'sudo useradd --system --no-create-home --shell /usr/sbin/nologin loobby
             sudo install -d -m 0755 /opt/loobby
             sudo install -d -m 0750 -o root -g loobby /etc/loobby'
```

**2. Variáveis.** Copie `deploy/go/api.env.example` para `/etc/loobby/api.env` na
máquina (dono `root:loobby`, modo `0640`) e preencha. O `JWT_SECRET` precisa ser o
de produção: com outro, os tokens já emitidos deixam de valer e os apps levam 401.

**3. Credenciais do Firebase.**

```bash
scp secrets/loobby-fcm.json MAQUINA:/tmp/
ssh MAQUINA 'sudo install -m 0640 -o root -g loobby /tmp/loobby-fcm.json /etc/loobby/ && rm -f /tmp/loobby-fcm.json'
```

Sem ele (e com `NOTIFICATIONS_ENABLED=true`) o serviço **não sobe**. A máquina
também precisa sair para a internet por HTTPS: Supabase, `api.resend.com`,
`www.googleapis.com`, `oauth2.googleapis.com`, `fcm.googleapis.com` e `api.rawg.io`.

**4. Firewall, nas duas camadas.** Na Security List (ou NSG) da subnet, entrada TCP
80 e 443 de qualquer origem. No sistema, porque as imagens Ubuntu da Oracle vêm
com o `iptables` recusando tudo exceto SSH:

```bash
ssh MAQUINA 'sudo iptables -I INPUT -p tcp --dport 80 -j ACCEPT
             sudo iptables -I INPUT -p tcp --dport 443 -j ACCEPT
             sudo netfilter-persistent save'
```

**5. nginx e certificado.** `nginx-api.loobby.app.conf` vai em
`/etc/nginx/sites-available/api.loobby.app`, com link em `sites-enabled`. O
`server` da porta 443 só sobe com o certificado; na primeira vez, emita-o com o
`server` da porta 80 sozinho no ar:

```bash
sudo certbot certonly --webroot -w /var/www/certbot -d api.loobby.app \
     --deploy-hook 'systemctl reload nginx'
```

O `certbot.timer` renova sozinho e o `--deploy-hook` recarrega o nginx a cada
renovação.

**6. Serviço.**

```bash
scp deploy/go/loobby-api.service MAQUINA:/tmp/
ssh MAQUINA 'sudo install -m 0644 /tmp/loobby-api.service /etc/systemd/system/
             sudo systemctl daemon-reload && sudo systemctl enable loobby-api'
deploy/go/deploy.sh MAQUINA        # primeira publicação; sobe o serviço
```

O desenho anterior — nginx no AWS na frente do Kotlin, e o Go numa segunda máquina
por TLS mútuo (`deploy/nginx/`, `deploy/certs/`) — ficou na tag `arquivo/v1`.
