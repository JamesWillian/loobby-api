# Go na máquina nova

```
usuário ─▶ nginx (AWS) ─ HTTP ─────────────▶ Kotlin (Docker, mesmo servidor)
                      └─ HTTPS + TLS mútuo ─▶ Go (máquina nova, systemd) ─▶ Supabase
```

O caminho AWS → máquina nova atravessa a internet levando os tokens de login dos
usuários. Ele é protegido em três camadas, e **a primeira sozinha já basta**:

1. **TLS mútuo.** O Go só completa a conexão com quem apresentar o certificado
   do nginx, assinado por uma CA privada nossa. Sem ele, a conexão é recusada
   antes de qualquer requisição — mesmo se as camadas 2 e 3 estiverem erradas.
2. **Firewall da Oracle Cloud**: porta 8443 aberta só para o IP do AWS.
3. **Firewall do sistema da máquina nova**: a mesma regra.

## Preparar a máquina nova (uma vez)

Nos comandos, `NOVA` é `usuario@ip-da-maquina-nova` e `IP_AWS` é `54.94.194.109`.

**1. Certificados — na sua máquina, não num servidor.**

```bash
deploy/certs/gen-certs.sh ~/loobby-certs
```

A `ca.key` fica só com você (gerenciador de senhas). Ela emite certificados que o
Go aceita; não pode estar no AWS nem na máquina nova.

**2. Usuário e diretórios.**

```bash
ssh NOVA 'sudo useradd --system --no-create-home --shell /usr/sbin/nologin loobby
          sudo install -d -m 0755 /opt/loobby
          sudo install -d -m 0750 -o root -g loobby /etc/loobby /etc/loobby/tls'
```

**3. Certificados do Go.**

```bash
scp ~/loobby-certs/{ca.crt,go-server.crt,go-server.key} NOVA:/tmp/
ssh NOVA 'sudo install -m 0644 -o root -g loobby /tmp/ca.crt /tmp/go-server.crt /etc/loobby/tls/
          sudo install -m 0640 -o root -g loobby /tmp/go-server.key /etc/loobby/tls/
          rm -f /tmp/ca.crt /tmp/go-server.crt /tmp/go-server.key'
```

**4. Variáveis.** Copie `deploy/go/api.env.example` para `/etc/loobby/api.env` na
máquina nova (dono `root:loobby`, modo `0640`) e preencha. Banco e `JWT_SECRET`
são **os mesmos do Kotlin** — com outro segredo, todo usuário numa rota migrada
leva 401.

**5. Firewall, nas duas camadas.**

- **Oracle Cloud:** na Security List (ou NSG) da subnet da máquina, regra de
  entrada TCP `8443` com origem `IP_AWS/32`. Nunca `0.0.0.0/0`.
- **Sistema operacional.** As imagens Ubuntu da Oracle costumam vir com `iptables`
  recusando tudo exceto SSH:
  ```bash
  ssh NOVA 'sudo iptables -I INPUT -p tcp -s IP_AWS/32 --dport 8443 -j ACCEPT
            sudo netfilter-persistent save'
  ```
  No Oracle Linux (firewalld):
  ```bash
  ssh NOVA "sudo firewall-cmd --permanent --add-rich-rule='rule family=ipv4 source address=IP_AWS/32 port port=8443 protocol=tcp accept'
            sudo firewall-cmd --reload"
  ```

Confira se o IP do AWS é um **Elastic IP**. Se não for, ele muda quando a instância
para e volta, e o firewall passa a bloquear o nginx. Isso falha de forma segura (o
TLS continua protegendo, e GET/HEAD caem no Kotlin sozinhos), mas derruba os POST
das rotas migradas.

**6. Serviço.**

```bash
scp deploy/go/loobby-api.service NOVA:/tmp/
ssh NOVA 'sudo install -m 0644 /tmp/loobby-api.service /etc/systemd/system/
          sudo systemctl daemon-reload && sudo systemctl enable loobby-api'
deploy/go/deploy.sh NOVA        # primeira publicação; sobe o serviço
```

**7. Conferir a partir do AWS** — depois de instalar os certificados do nginx
(`deploy/nginx/README.md`, passo 1):

```bash
curl --cacert /etc/nginx/loobby/ca.crt \
     --cert /etc/nginx/loobby/nginx-client.crt --key /etc/nginx/loobby/nginx-client.key \
     --resolve loobby-go:8443:IP_DA_MAQUINA_NOVA \
     -w '\nconexao %{time_connect}s  tls %{time_appconnect}s  total %{time_total}s\n' \
     https://loobby-go:8443/actuator/health
```

Esperado: `{"status":"UP"}`. E da sua máquina, **sem** os certificados, o mesmo
comando tem que falhar no handshake — é a prova de que só o nginx entra.

**Olhe o `conexao`.** Ele é uma ida-e-volta de rede, e toda requisição migrada vai
pagá-lo a mais. Entre AWS São Paulo e Oracle São Paulo/Vinhedo, fica em poucos
milissegundos. Se passar de ~20 ms, a máquina nova está longe — e na Oracle, a
região de uma conta Always Free não muda depois de criada. Melhor descobrir agora
do que depois da primeira rota migrada.

## Publicar uma versão nova

```bash
deploy/go/deploy.sh NOVA
```

Roda os testes, compila para a arquitetura da máquina (detectada por `uname -m`),
troca o binário e reinicia. Só considera no ar quando o processo escreve
`servidor no ar` no log; se não escrever em ~15 s, **volta o binário anterior**.

## O que foi testado e o que não foi

**Testado localmente, com nginx e Go de verdade** (`deploy/nginx/test-tls.sh`):
a resposta do Go atravessando o nginx por TLS mútuo é idêntica ao golden de
produção; o Go recusa conexão sem certificado, com o certificado errado e em HTTP
puro; Go fora do ar faz GET/HEAD caírem no Kotlin e devolve 502 para POST; o kill
switch troca também o esquema. Cada uma dessas verificações foi quebrada de
propósito para confirmar que o teste falha.

**Não testado:** o `loobby-api.service` e o `deploy.sh` não rodaram num Linux (esta
máquina é macOS; o `deploy.sh` passou no `shellcheck`). Os comandos de firewall
também não foram executados.
