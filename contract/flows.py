#!/usr/bin/env python3
"""
Fluxos felizes que a producao nunca gravou: cadastro, verificacao de email,
reset de senha, troca de senha, exclusao de conta e login com Google.

A suite principal (capture.py) roda contra producao e por isso para na porta de
cada um deles: o token chega por email, a exclusao destroi a conta de teste, o
Google exige um ID token de verdade. Aqui eles rodam inteiros, contra o ambiente
LOCAL de contract/local/ -- onde o Resend e o Google sao falsos e o banco e
descartavel. NUNCA aponte isto para producao: ele cria e apaga contas.

    contract/local/oracle.sh compare      # o jeito normal: os dois lados + compare.py

    python3 contract/flows.py --base-url http://127.0.0.1:18081 --out /tmp/cap-go \\
        --mailbox http://127.0.0.1:18100 --side go --google-key google.key \\
        --db-url postgres://postgres@127.0.0.1:54329/loobby_go

Grava golden no mesmo formato do capture.py (nomes `fluxo-*`), para o compare.py
comparar um lado com o outro. Precisa de banco recem-criado: os emails das contas
sao fixos, para as duas capturas serem comparaveis.

--db-url e --google-key sao opcionais; sem eles, os cenarios que dependem de
"viajar no tempo" (token vencido, espera de 3 minutos) e os do Google sao pulados.
"""
import argparse
import base64
import hashlib
import hmac
import json
import os
import re
import subprocess
import sys
import tempfile
import time
import urllib.parse
import urllib.request

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from capture import Capture, UUID_ANYWHERE_RE  # noqa: E402

SENHA = "senha-do-fluxo-1"
SENHA_RESET = "senha-do-fluxo-2"
SENHA_TROCADA = "senha-do-fluxo-3"
EMAIL_A = "fluxo-a@teste.local"
EMAIL_C = "fluxo-c@teste.local"
EMAIL_MAIUSCULO = "Fluxo-Maiusculo@Teste.Local"

WEB_CLIENT_ID = "web-local.apps.googleusercontent.com"
IOS_CLIENT_ID = "ios-local.apps.googleusercontent.com"


def b64url(raw):
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


class Flow(Capture):
    def __init__(self, args):
        super().__init__(args.base_url, args.out)
        self.mailbox = args.mailbox.rstrip("/")
        self.side = args.side
        self.google_key = args.google_key
        self.psql = args.psql
        self.db_url = args.db_url
        self.jwt_secret = args.jwt_secret
        self.patterns = []   # (regex, troca) para textos com um trecho aleatorio
        self.skipped = []

    # ---------- normalizacao ----------

    def normalize(self, v):
        if isinstance(v, str) and v not in self.ids:
            for rx, repl in self.patterns:
                if rx.match(v):
                    return rx.sub(repl, v)
        return super().normalize(v)

    def scrub(self, text):
        """Texto livre (HTML): tira o que muda a cada execucao."""
        text = text.replace(self.base, "<base-url>")
        return UUID_ANYWHERE_RE.sub("<uuid>", text)

    def page(self, name, method, path, body=None, **kw):
        """record() para resposta HTML. O corpo de uma pagina e comparado byte a
        byte, e o formulario de reset carrega o token: troca por marcador."""
        self.record(name, method, path, body, **kw)
        p = os.path.join(self.out, name + ".json")
        g = json.load(open(p, encoding="utf-8"))
        if g["response"].get("body_raw"):
            g["response"]["body_raw"] = self.scrub(g["response"]["body_raw"])
            self._write(p, g)

    @staticmethod
    def _write(path, golden):
        with open(path, "w", encoding="utf-8") as f:
            json.dump(golden, f, indent=2, ensure_ascii=False, sort_keys=False)
            f.write("\n")

    # ---------- o que nao e a API ----------

    def call(self, method, path, body=None, token=None, auth=True):
        """Requisicao de preparacao: nao vira golden, mas precisa dar certo."""
        status, _, blob = self.request(method, path, body, auth=auth, token=token)
        if status not in (200, 201, 204):
            sys.exit(f"preparacao falhou: {method} {path} -> {status} {blob[:300]!r}")
        return json.loads(blob) if blob else None

    def mail(self, name, to):
        """Grava como golden o que o Resend falso recebeu para um destinatario:
        quantos emails ate agora e o conteudo do ultimo. Devolve o token do link."""
        self.n += 1
        url = f"{self.mailbox}/emails?lado={self.side}&to={urllib.parse.quote(to)}"
        with urllib.request.urlopen(url, timeout=10) as r:
            box = json.load(r)
        last = box["last"] or {}
        html = last.get("html", "")
        m = re.search(r"token=([0-9a-f-]{36})", html)
        body = {"count": box["count"]}
        for key in ("from", "to", "subject", "auth", "invalido"):
            if key in last:
                body[key] = last[key]
        if "html" in last:
            body["html"] = self.scrub(html)
        self._write(os.path.join(self.out, name + ".json"), {
            "request": {"method": "MAILBOX", "path": f"/emails?to={to}", "body": None},
            "response": {"status": 200, "headers": {}, "body_raw": None, "body": body},
        })
        print(f"  ok  {name + '.json':<44} MAILBOX {to:<51} {box['count']} email(s)")
        return m.group(1) if m else None

    def sql(self, statement):
        subprocess.run([self.psql, self.db_url, "-q", "-v", "ON_ERROR_STOP=1", "-c", statement],
                       check=True, capture_output=True)

    def skip(self, what, why):
        self.skipped.append(f"{what}: {why}")
        print(f"  --  pulado: {what} ({why})")

    def google_token(self, key=None, header=None, **claims):
        """ID token como o Google emite, assinado (RS256) pela chave do ambiente local."""
        now = int(time.time())
        payload = {
            "iss": "https://accounts.google.com", "azp": WEB_CLIENT_ID, "aud": WEB_CLIENT_ID,
            "email_verified": True, "iat": now, "exp": now + 3600,
        }
        payload.update(claims)
        payload = {k: v for k, v in payload.items() if v is not None}
        head = {"alg": "RS256", "kid": "fake-google-key", "typ": "JWT"}
        head.update(header or {})
        signing = (b64url(json.dumps(head, separators=(",", ":")).encode()) + "."
                   + b64url(json.dumps(payload, separators=(",", ":")).encode()))
        sig = subprocess.run(["openssl", "dgst", "-sha256", "-sign", key or self.google_key],
                             input=signing.encode(), check=True, capture_output=True).stdout
        return signing + "." + b64url(sig)

    def loobby_token(self, sub, typ, expires_in):
        """JWT do proprio Loobby (HS256) com validade a escolher -- para os
        cenarios de refresh token vencido."""
        now = int(time.time())
        head = b64url(b'{"alg":"HS256"}')
        claims = {"sub": sub, "exp": now + expires_in, "type": typ, "iat": now + expires_in - 1296000}
        signing = head + "." + b64url(json.dumps(claims, separators=(",", ":")).encode())
        mac = hmac.new(self.jwt_secret.encode(), signing.encode(), hashlib.sha256).digest()
        return signing + "." + b64url(mac)


def anon(c, label):
    """Conta anonima nova: devolve (token, id)."""
    a = c.call("POST", "/auth/anonymous", {}, auth=False)
    c.remember(label, a["userId"])
    return a["accessToken"], str(a["userId"])


def cadastro(c):
    print("\n-- cadastro + verificacao de email --")
    a = c.record("fluxo-cadastro-anonimo", "POST", "/auth/anonymous", {}, auth=False, expect=200)
    if not a or "accessToken" not in a:
        sys.exit("nao consegui nem criar uma conta anonima; o servidor esta no ar?")
    ta, ida = a["accessToken"], str(a["userId"])
    c.remember("a", ida)
    c.record("fluxo-cadastro-me-anonimo", "GET", "/users/me", token=ta, expect=200)

    # Uma violacao so: com mais de uma, a ORDEM do fieldErrors muda a cada
    # chamada no proprio Kotlin (ACHADOS §31).
    c.record("fluxo-cadastro-email-invalido", "POST", "/auth/register",
             {"email": "nao-e-email", "password": SENHA}, token=ta, expect=400)
    c.record("fluxo-cadastro-senha-de-80", "POST", "/auth/register",
             {"email": EMAIL_A, "password": "x" * 80}, token=ta, expect=400)

    reg = c.record("fluxo-cadastro-register", "POST", "/auth/register",
                   {"email": EMAIL_A, "password": SENHA}, token=ta, expect=200)
    if not reg or "accessToken" not in reg:
        sys.exit("cadastro falhou -- o banco precisa estar recem-criado (oracle.sh reset)")
    ta = reg["accessToken"]
    t1 = c.mail("fluxo-cadastro-email-enviado", EMAIL_A)
    c.record("fluxo-cadastro-me-nao-verificado", "GET", "/users/me", token=ta, expect=200)
    c.record("fluxo-cadastro-register-de-novo", "POST", "/auth/register",
             {"email": "outro@teste.local", "password": SENHA}, token=ta, expect=400)
    c.record("fluxo-cadastro-reenvio-cedo-demais", "POST", "/auth/resend-verification", {}, token=ta, expect=429)

    # Cada login de conta nao verificada manda um email novo, e o token anterior morre.
    c.record("fluxo-cadastro-login-nao-verificado", "POST", "/auth/login",
             {"email": EMAIL_A, "password": SENHA}, auth=False, expect=200)
    t2 = c.mail("fluxo-cadastro-email-do-login", EMAIL_A)
    c.page("fluxo-cadastro-verificar-token-antigo", "GET", f"/auth/verify-email?token={t1}", auth=False, expect=400)

    if c.db_url:
        c.sql(f"UPDATE user_credentials SET email_verification_expires_at = now() - interval '1 hour' "
              f"WHERE email = '{EMAIL_A}'")
        c.page("fluxo-cadastro-verificar-token-vencido", "GET", f"/auth/verify-email?token={t2}",
               auth=False, expect=400)
        # com o envio anterior "ha mais de 3 minutos", o reenvio passa
        c.record("fluxo-cadastro-reenvio", "POST", "/auth/resend-verification", {}, token=ta, expect=200)
        t2 = c.mail("fluxo-cadastro-email-do-reenvio", EMAIL_A)
    else:
        c.skip("token de verificacao vencido e reenvio", "sem --db-url")

    c.page("fluxo-cadastro-verificar", "GET", f"/auth/verify-email?token={t2}", auth=False, expect=200)
    c.page("fluxo-cadastro-verificar-de-novo", "GET", f"/auth/verify-email?token={t2}", auth=False, expect=400)
    c.record("fluxo-cadastro-me-verificado", "GET", "/users/me", token=ta, expect=200)
    c.record("fluxo-cadastro-reenvio-ja-verificado", "POST", "/auth/resend-verification", {}, token=ta, expect=400)
    c.record("fluxo-cadastro-login-verificado", "POST", "/auth/login",
             {"email": EMAIL_A, "password": SENHA}, auth=False, expect=200)
    c.mail("fluxo-cadastro-sem-email-no-login-verificado", EMAIL_A)

    # Email que ja e de outra conta: a restricao unica do banco responde, e sai 500.
    tx, _ = anon(c, "x")
    c.record("fluxo-cadastro-email-ja-usado", "POST", "/auth/register",
             {"email": EMAIL_A, "password": SENHA}, token=tx, expect=500)
    c.record("fluxo-cadastro-me-depois-do-500", "GET", "/users/me", token=tx, expect=200)
    return ta, ida


def reset_de_senha(c):
    print("\n-- reset de senha --")
    # O email chega com espacos e maiusculas: a rota apara e poe em minusculas.
    c.record("fluxo-reset-pedido", "POST", "/auth/forgot-password",
             {"email": "  " + EMAIL_A.upper() + " "}, auth=False, expect=200)
    token = c.mail("fluxo-reset-email-enviado", EMAIL_A)
    c.record("fluxo-reset-pedido-de-novo", "POST", "/auth/forgot-password",
             {"email": EMAIL_A}, auth=False, expect=200)
    c.mail("fluxo-reset-sem-segundo-email", EMAIL_A)
    c.record("fluxo-reset-email-desconhecido", "POST", "/auth/forgot-password",
             {"email": "ninguem@teste.local"}, auth=False, expect=200)
    c.mail("fluxo-reset-nada-para-desconhecido", "ninguem@teste.local")

    c.page("fluxo-reset-formulario", "GET", f"/auth/reset-password?token={token}", auth=False, expect=200)
    c.page("fluxo-reset-senha-curta", "POST", "/auth/reset-password",
           {"token": token, "password": "abc"}, auth=False, expect=400)
    c.page("fluxo-reset-trocar", "POST", "/auth/reset-password",
           {"token": token, "password": SENHA_RESET}, auth=False, expect=200)
    c.page("fluxo-reset-token-ja-usado", "POST", "/auth/reset-password",
           {"token": token, "password": SENHA_RESET}, auth=False, expect=400)
    c.page("fluxo-reset-formulario-token-usado", "GET", f"/auth/reset-password?token={token}",
           auth=False, expect=400)
    c.record("fluxo-reset-login-senha-antiga", "POST", "/auth/login",
             {"email": EMAIL_A, "password": SENHA}, auth=False, expect=400)
    c.record("fluxo-reset-login-senha-nova", "POST", "/auth/login",
             {"email": EMAIL_A, "password": SENHA_RESET}, auth=False, expect=200)

    if c.db_url:
        # usado o token, a espera de 3 minutos deixa de valer: um pedido novo ja manda email
        c.record("fluxo-reset-segundo-pedido", "POST", "/auth/forgot-password",
                 {"email": EMAIL_A}, auth=False, expect=200)
        token = c.mail("fluxo-reset-segundo-email", EMAIL_A)
        c.sql(f"UPDATE user_credentials SET password_reset_expires_at = now() - interval '1 minute' "
              f"WHERE email = '{EMAIL_A}'")
        c.page("fluxo-reset-formulario-link-vencido", "GET", f"/auth/reset-password?token={token}",
               auth=False, expect=400)
        c.page("fluxo-reset-trocar-link-vencido", "POST", "/auth/reset-password",
               {"token": token, "password": SENHA}, auth=False, expect=400)
    else:
        c.skip("link de reset vencido", "sem --db-url")

    # O cadastro grava o email como digitado; o pedido de reset procura em
    # minusculas. Conta criada com maiuscula no email nunca recebe o link.
    tm, _ = anon(c, "maiusculo")
    c.record("fluxo-reset-cadastro-com-maiuscula", "POST", "/auth/register",
             {"email": EMAIL_MAIUSCULO, "password": SENHA}, token=tm, expect=200)
    c.mail("fluxo-reset-maiuscula-email-de-verificacao", EMAIL_MAIUSCULO)
    c.record("fluxo-reset-pedido-com-maiuscula", "POST", "/auth/forgot-password",
             {"email": EMAIL_MAIUSCULO}, auth=False, expect=200)
    c.mail("fluxo-reset-maiuscula-nao-recebe", EMAIL_MAIUSCULO)
    c.record("fluxo-reset-login-email-em-minusculas", "POST", "/auth/login",
             {"email": EMAIL_MAIUSCULO.lower(), "password": SENHA}, auth=False, expect=400)


def troca_de_senha(c, ta):
    print("\n-- troca de senha --")
    c.record("fluxo-senha-igual-a-atual", "PUT", "/users/me/password",
             {"currentPassword": SENHA_RESET, "newPassword": SENHA_RESET, "confirmPassword": SENHA_RESET},
             token=ta, expect=400)
    c.record("fluxo-senha-trocar", "PUT", "/users/me/password",
             {"currentPassword": SENHA_RESET, "newPassword": SENHA_TROCADA, "confirmPassword": SENHA_TROCADA},
             token=ta, expect=200)
    c.record("fluxo-senha-login-antiga", "POST", "/auth/login",
             {"email": EMAIL_A, "password": SENHA_RESET}, auth=False, expect=400)
    c.record("fluxo-senha-login-nova", "POST", "/auth/login",
             {"email": EMAIL_A, "password": SENHA_TROCADA}, auth=False, expect=200)
    # o token emitido antes da troca continua valendo
    c.record("fluxo-senha-token-antigo-ainda-vale", "GET", "/users/me", token=ta, expect=200)
    tn, _ = anon(c, "anonimo")
    c.record("fluxo-senha-conta-anonima", "PUT", "/users/me/password",
             {"currentPassword": "qualquer", "newPassword": SENHA, "confirmPassword": SENHA},
             token=tn, expect=404)


def refresh_vencido(c, ida):
    print("\n-- refresh token vencido --")
    if not c.jwt_secret:
        return c.skip("refresh token vencido", "sem --jwt-secret")
    tn, idn = anon(c, "anonimo-refresh")
    # Anonimo nao tem como logar de novo: refresh vencido, mas assinado, e renovado.
    c.record("fluxo-refresh-vencido-anonimo", "POST", "/auth/refresh",
             {"refreshToken": c.loobby_token(idn, "refresh", -3600)}, auth=False, expect=200)
    c.record("fluxo-refresh-vencido-conta-com-senha", "POST", "/auth/refresh",
             {"refreshToken": c.loobby_token(ida, "refresh", -3600)}, auth=False, expect=401)
    c.record("fluxo-refresh-access-vencido-de-anonimo", "POST", "/auth/refresh",
             {"refreshToken": c.loobby_token(idn, "access", -3600)}, auth=False, expect=401)
    c.record("fluxo-refresh-com-access-token", "POST", "/auth/refresh",
             {"refreshToken": tn}, auth=False, expect=400)


def exclusao_de_conta(c, ta, ida):
    print("\n-- exclusao de conta --")
    tb, idb = anon(c, "b")
    soon = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time() + 3 * 86400))

    def instant(name, token):
        e = c.call("POST", "/events/instant", {
            "eventType": "GAMEPLAY", "name": name, "scheduledDatetime": soon,
            "gameplay": {"gameName": "Jogo do Fluxo"}}, token=token)
        return str(e["id"])

    # O que a conta A deixa para tras:
    g1 = str(c.call("POST", "/groups", {"name": "fluxo-grupo-com-outro-membro"}, token=ta)["id"])
    c.remember("grupo-transferido", g1)
    c.call("POST", f"/groups/{g1}/members", token=tb)
    g2 = str(c.call("POST", "/groups", {"name": "fluxo-grupo-so-dela"}, token=ta)["id"])
    c.remember("grupo-apagado", g2)
    e1 = instant("fluxo-evento-com-outro-confirmado", ta)
    c.remember("evento-transferido", e1)
    c.call("PUT", f"/events/{e1}/rsvps", {"status": "YES", "isPaid": False, "obs": "vou"}, token=tb)
    e2 = instant("fluxo-evento-so-dela", ta)
    c.remember("evento-apagado", e2)
    e3 = instant("fluxo-evento-do-outro", tb)
    c.remember("evento-do-b", e3)
    c.call("PUT", f"/events/{e3}/rsvps", {"status": "YES", "isPaid": True, "obs": "levo a bola"}, token=ta)
    e4 = str(c.call("POST", f"/groups/{g1}/events", {
        "eventType": "SPORT", "name": "fluxo-evento-de-grupo", "scheduledDatetime": soon,
        "sport": {"durationMinutes": 60, "arena": "Quadra", "pricePerPlayer": "10.00",
                  "maxPlayers": 10, "acceptReserve": False}}, token=ta)["id"])
    c.remember("evento-de-grupo", e4)
    c.record("fluxo-exclusao-rsvps-antes", "GET", f"/events/{e3}/rsvps", token=tb, expect=200)

    refresh_a = c.call("POST", "/auth/login", {"email": EMAIL_A, "password": SENHA_TROCADA}, auth=False)["refreshToken"]

    tn, _ = anon(c, "anonimo-exclusao")
    c.record("fluxo-exclusao-conta-anonima", "DELETE", "/users/me", {"password": "qualquer"}, token=tn, expect=404)
    c.record("fluxo-exclusao", "DELETE", "/users/me", {"password": SENHA_TROCADA}, token=ta, expect=200)

    # O que sobra da conta: o registro esvaziado, e os tokens que ja existiam.
    c.record("fluxo-exclusao-login-depois", "POST", "/auth/login",
             {"email": EMAIL_A, "password": SENHA_TROCADA}, auth=False, expect=400)
    c.record("fluxo-exclusao-me-depois", "GET", "/users/me", token=ta, expect=200)
    c.record("fluxo-exclusao-refresh-depois", "POST", "/auth/refresh",
             {"refreshToken": refresh_a}, auth=False, expect=200)
    c.record("fluxo-exclusao-de-novo", "DELETE", "/users/me", {"password": SENHA_TROCADA}, token=ta, expect=404)

    # O que aconteceu com o que era dela.
    c.record("fluxo-exclusao-grupo-transferido", "GET", f"/groups/{g1}", token=tb, expect=200)
    c.record("fluxo-exclusao-membros-do-grupo", "GET", f"/groups/{g1}/members", token=tb, expect=200)
    c.record("fluxo-exclusao-grupo-apagado", "GET", f"/groups/{g2}", token=tb, expect=400)
    c.record("fluxo-exclusao-evento-transferido", "GET", f"/events/{e1}", token=tb, expect=200)
    c.record("fluxo-exclusao-evento-apagado", "GET", f"/events/{e2}", token=tb, expect=400)
    c.record("fluxo-exclusao-evento-de-grupo", "GET", f"/events/{e4}", token=tb, expect=200)
    c.record("fluxo-exclusao-rsvps-depois", "GET", f"/events/{e3}/rsvps", token=tb, expect=200)
    c.record("fluxo-exclusao-sorteio-ignora-excluido", "POST", f"/events/{e3}/teams/auto-generate",
             {"teamCount": 2, "includeReserves": True}, token=tb, expect=200)
    c.record("fluxo-exclusao-feed-do-outro", "GET", "/users/feed", token=tb, expect=200)

    # O email fica livre para um cadastro novo.
    tr, _ = anon(c, "reaproveita-email")
    c.record("fluxo-exclusao-email-liberado", "POST", "/auth/register",
             {"email": EMAIL_A, "password": SENHA}, token=tr, expect=200)

    # A SEGUNDA conta a ser excluida: users.username e UNIQUE e a exclusao grava
    # username "" -- so cabe uma (ACHADOS §23).
    tc, _ = anon(c, "c")
    c.record("fluxo-exclusao-2a-cadastro", "POST", "/auth/register",
             {"email": EMAIL_C, "password": SENHA}, token=tc, expect=200)
    c.record("fluxo-exclusao-2a-conta", "DELETE", "/users/me", {"password": SENHA}, token=tc, expect=500)
    c.record("fluxo-exclusao-2a-conta-continua-existindo", "POST", "/auth/login",
             {"email": EMAIL_C, "password": SENHA}, auth=False, expect=200)


def login_com_google(c):
    print("\n-- login com Google --")
    if not c.google_key:
        return c.skip("login com Google", "sem --google-key")
    c.patterns += [
        (re.compile(r"^(fulana_da_si|fluxosemnome|fluxo_a)_[0-9a-f]{4}$"), r"\1_<hex4>"),
    ]
    foto = "https://exemplo.com/foto.png"

    def login(golden, expect, **claims):
        return c.record(golden, "POST", "/auth/google", {"idToken": c.google_token(**claims)},
                        auth=False, expect=expect)

    g = login("fluxo-google-conta-nova", 200, sub="google-sub-1", email="fluxo.google@teste.local",
              name="Fulana da Silva Sauro", picture=foto)
    if g and "accessToken" in g:
        tg = g["accessToken"]
        c.remember("google-1", g["userId"])
        c.record("fluxo-google-me", "GET", "/users/me", token=tg, expect=200)
        # mesma conta Google de novo, por outro app e com outra foto: entra, e a foto fica
        login("fluxo-google-de-novo", 200, sub="google-sub-1", email="fluxo.google@teste.local",
              name="Outro Nome", picture="https://exemplo.com/outra.png", aud=IOS_CLIENT_ID, azp=IOS_CLIENT_ID)
        c.record("fluxo-google-me-de-novo", "GET", "/users/me", token=tg, expect=200)
        # conta Google nao tem senha: nao troca senha nem consegue se excluir
        c.record("fluxo-google-trocar-senha", "PUT", "/users/me/password",
                 {"currentPassword": "qualquer", "newPassword": SENHA, "confirmPassword": SENHA},
                 token=tg, expect=404)
        c.record("fluxo-google-excluir-conta", "DELETE", "/users/me", {"password": "qualquer"}, token=tg, expect=404)
        c.record("fluxo-google-reenviar-verificacao", "POST", "/auth/resend-verification", {}, token=tg, expect=404)

    # sem nome e sem foto: o nome vem do email; a foto entra no primeiro login que a trouxer
    g2 = login("fluxo-google-sem-nome-nem-foto", 200, sub="google-sub-2", email="fluxo.sem.nome@teste.local")
    if g2 and "accessToken" in g2:
        c.remember("google-2", g2["userId"])
        c.record("fluxo-google-me-sem-foto", "GET", "/users/me", token=g2["accessToken"], expect=200)
        login("fluxo-google-ganha-foto", 200, sub="google-sub-2", email="fluxo.sem.nome@teste.local", picture=foto)
        c.record("fluxo-google-me-com-foto", "GET", "/users/me", token=g2["accessToken"], expect=200)

    # email que ja e de uma conta com senha: nasce outra conta, sem ligacao com a primeira
    g3 = login("fluxo-google-email-de-conta-com-senha", 200, sub="google-sub-3", email=EMAIL_A, name="Fluxo A")
    if g3 and "accessToken" in g3:
        c.remember("google-3", g3["userId"])

    login("fluxo-google-sem-email", 400, sub="google-sub-4", name="Sem Email")
    login("fluxo-google-audiencia-de-outro-app", 401, sub="google-sub-1", email="fluxo.google@teste.local",
          aud="outro-app.apps.googleusercontent.com")
    login("fluxo-google-emissor-errado", 401, sub="google-sub-1", email="fluxo.google@teste.local",
          iss="https://accounts.exemplo.com")
    login("fluxo-google-vencido", 401, sub="google-sub-1", email="fluxo.google@teste.local",
          iat=int(time.time()) - 7200, exp=int(time.time()) - 3600)
    # a biblioteca do Google tolera 5 minutos de diferenca de relogio
    login("fluxo-google-vencido-ha-4-minutos", 200, sub="google-sub-1", email="fluxo.google@teste.local",
          iat=int(time.time()) - 3840, exp=int(time.time()) - 240)

    with tempfile.TemporaryDirectory() as tmp:
        other = os.path.join(tmp, "outra.key")
        subprocess.run(["openssl", "genrsa", "-out", other, "2048"], check=True, capture_output=True)
        c.record("fluxo-google-assinado-por-outra-chave", "POST", "/auth/google",
                 {"idToken": c.google_token(key=other, sub="google-sub-1", email="fluxo.google@teste.local")},
                 auth=False, expect=401)


def main():
    p = argparse.ArgumentParser(description=__doc__.strip().split("\n")[0])
    p.add_argument("--base-url", required=True)
    p.add_argument("--out", required=True)
    p.add_argument("--mailbox", required=True, help="endereco do contract/local/fakes.py")
    p.add_argument("--side", required=True, help="rotulo deste servidor na caixa de email (kotlin, go)")
    p.add_argument("--google-key", help="chave privada (PEM) que assina os ID tokens falsos")
    p.add_argument("--db-url", help="banco deste servidor, para os cenarios de token vencido")
    p.add_argument("--psql", default="psql")
    p.add_argument("--jwt-secret", default="segredo-local-de-teste-com-mais-de-32-bytes-0123456789",
                   help="JWT_SECRET do ambiente local, para os cenarios de refresh vencido")
    args = p.parse_args()

    host = urllib.parse.urlparse(args.base_url).hostname
    if host not in ("127.0.0.1", "localhost", "::1"):
        sys.exit(f"flows.py so roda contra o ambiente local; {host} nao e esta maquina")

    c = Flow(args)
    print(f"fluxos em {c.base} -> {c.out}/")
    ta, ida = cadastro(c)
    reset_de_senha(c)
    troca_de_senha(c, ta)
    refresh_vencido(c, ida)
    login_com_google(c)
    exclusao_de_conta(c, ta, ida)

    print(f"\n{c.n} golden gravados em {c.out}/")
    for s in c.skipped:
        print("  pulado -- " + s)
    if c.failures:
        print(f"\n{len(c.failures)} status fora do esperado (o golden foi gravado mesmo assim):")
        for f in c.failures:
            print("  - " + f)
    return 0


if __name__ == "__main__":
    sys.exit(main())
