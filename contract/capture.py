#!/usr/bin/env python3
"""
Captura os golden de contrato da Loobby API (Fase 0 da migracao para Go).

Roda contra a API Kotlin para gerar a baseline, e depois contra a Go para
comparar. Sem dependencias fora da stdlib.

    python3 contract/capture.py                      # captura a baseline
    python3 contract/capture.py --base-url http://localhost:8081 --out contract/go
    python3 contract/capture.py --include-email      # inclui endpoints que mandam email

O corpo cru de cada resposta e gravado literalmente. Isso e o ponto: `0.00`
serializado pelo Jackson vira `0` num Go ingenuo com float64, e so o texto cru
denuncia isso. A visao `normalized` ao lado troca valores volateis (ids gerados,
timestamps, JWTs) por placeholders, para que duas execucoes deem diff limpo.
"""

import argparse
import hashlib
import json
import os
import re
import sys
import urllib.error
import urllib.request
import uuid
from datetime import datetime, timedelta, timezone
from decimal import Decimal

UUID_RE = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$", re.I)
ISO_RE = re.compile(r"^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}")
JWT_RE = re.compile(r"^ey[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$")
UUID_ANYWHERE_RE = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", re.I)
# Username de anonimo: "user_" + 6 hex (AuthService.generateRandomUsername).
ANON_USER_RE = re.compile(r"^user_[0-9a-f]{6}$")
# inviteCode e aleatorio, mas o FORMATO e contrato: grupo e $-XXXXXX, evento e
# $-XXXX-XXXX. Normaliza o valor e preserva a forma.
INVITE_RE = re.compile(r"^\$-[A-Z0-9]+(?:-[A-Z0-9]+)*$")
# /files/<subfolder>/<ownerId>/<epochMillis>.<ext> — o epoch muda a cada upload
FILE_URL_RE = re.compile(r"^/files/([^/]+)/([0-9a-f-]{36})/(\d+)(\.\w+)$", re.I)

# Headers que fazem parte do contrato. O resto (Date, servidor, trace) e ruido.
KEPT_HEADERS = (
    "content-type", "location", "www-authenticate", "allow",
    # Escritos pelo Spring Security em TODA resposta, inclusive no 401 que sai do
    # filtro antes de qualquer controller. Navegador aplica X-Frame-Options e
    # nosniff; cliente/CDN respeita cache. Sao contrato tanto quanto o corpo.
    "cache-control", "pragma", "expires",
    "x-content-type-options", "x-xss-protection", "x-frame-options",
    "strict-transport-security",
    # .cors(withDefaults()) emite tres linhas Vary separadas.
    "vary",
)

TEXTUAL = ("json", "text/", "xml", "javascript", "x-www-form-urlencoded")


def is_textual(content_type, blob=b""):
    ct = (content_type or "").lower()
    if any(t in ct for t in TEXTUAL):
        return True
    if ct:
        return False
    # Sem Content-Type (ex.: o 403 "Invalid CORS request" do preflight): e texto
    # se for UTF-8 valido. Imagem nunca passa nesse teste.
    try:
        blob.decode("utf-8")
        return True
    except UnicodeDecodeError:
        return False

# Estes golden vao para o git. Nenhum segredo pode entrar neles: o valor concreto
# de um token nao e contrato (e aleatorio), so a presenca e o tipo da chave sao.
SECRET_KEYS = ("password", "accessToken", "refreshToken", "idToken", "newPassword", "currentPassword")
SECRET_RAW_RE = re.compile(r'("(?:%s)"\s*:\s*)"[^"]*"' % "|".join(SECRET_KEYS))


def redact_raw(text):
    return SECRET_RAW_RE.sub(r'\1"<redigido>"', text)


def redact_body(v):
    if isinstance(v, dict):
        return {k: ("<redigido>" if k in SECRET_KEYS else redact_body(x)) for k, x in v.items()}
    if isinstance(v, list):
        return [redact_body(x) for x in v]
    return v


def kept_headers(headers):
    """Headers do contrato, com multiplos valores juntados na ordem recebida.
    dict(HTTPMessage) guardaria so o ultimo Vary dos tres que o Spring emite."""
    if not headers:
        return {}
    out = {}
    for name in KEPT_HEADERS:
        values = headers.get_all(name) if hasattr(headers, "get_all") else (
            [headers[name]] if name in headers else None)
        if values:
            out[name] = ", ".join(values)
    return out


def load_env(path=".env"):
    if not os.path.exists(path):
        sys.exit(f"faltando {path} — veja contract/README.md")
    for line in open(path, encoding="utf-8"):
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            k, v = line.split("=", 1)
            os.environ.setdefault(k.strip(), v.strip())


class Capture:
    def __init__(self, base_url, out_dir):
        self.base = base_url.rstrip("/")
        self.out = out_dir
        self.token = None
        self.token2 = None
        self.ids = {}        # nome logico -> id real, para placeholders estaveis
        self.cleanup = []    # LIFO
        self.n = 0
        self.failures = []
        os.makedirs(self.out, exist_ok=True)

    # ---------- placeholders ----------

    def remember(self, label, value):
        """Registra um id gerado para que ele vire <id:label> nos golden."""
        if value:
            self.ids[str(value)] = f"<id:{label}>"
        return value

    def normalize(self, v):
        if isinstance(v, dict):
            return {k: self.normalize(x) for k, x in v.items()}
        if isinstance(v, list):
            return [self.normalize(x) for x in v]
        if isinstance(v, Decimal):
            # preserva o texto literal: 0.00 != 0.0 != 0
            return {"$num": format(v, "f")}
        if isinstance(v, str):
            if v in self.ids:
                return self.ids[v]
            if JWT_RE.match(v):
                return "<jwt>"
            if UUID_RE.match(v):
                return "<uuid>"
            if ISO_RE.match(v):
                return "<timestamp>"
            if v.startswith(self.base):
                return "<base-url>" + v[len(self.base):]
            m = FILE_URL_RE.match(v)
            if m:
                owner = self.ids.get(m.group(2), "<uuid>")
                return f"/files/{m.group(1)}/{owner}/<epoch>{m.group(4)}"
            if ANON_USER_RE.match(v):
                return "user_<hex6>"
            if INVITE_RE.match(v):
                return re.sub(r"[A-Z0-9]", "X", v)
            # UUID embutido em string maior (ex.: "path" do envelope de erro)
            return UUID_ANYWHERE_RE.sub(lambda u: self.ids.get(u.group(0), "<uuid>"), v)
        return v

    # ---------- http ----------

    def request(self, method, path, body=None, auth=True, raw_body=None, ctype="application/json", token=None, extra_headers=None):
        url = self.base + path
        data = None
        if raw_body is not None:
            data = raw_body if isinstance(raw_body, bytes) else raw_body.encode()
        elif body is not None:
            data = json.dumps(body).encode()
        req = urllib.request.Request(url, data=data, method=method)
        if data is not None:
            req.add_header("Content-Type", ctype)
        for k, v in (extra_headers or {}).items():
            req.add_header(k, v)
        bearer = token or self.token
        if auth and bearer:
            req.add_header("Authorization", "Bearer " + bearer)
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                return r.status, r.headers, r.read()
        except urllib.error.HTTPError as e:
            return e.code, e.headers, e.read()
        except Exception as e:
            return 0, {}, f"<erro de transporte: {e}>".encode()

    def record(self, name, method, path, body=None, auth=True, expect=None,
               raw_body=None, ctype="application/json", token=None, extra_headers=None):
        """Executa e grava o golden. Devolve o corpo parseado (ou None)."""
        self.n += 1
        status, headers, blob = self.request(method, path, body, auth, raw_body, ctype, token, extra_headers)
        content_type = headers.get("Content-Type", "") if headers else ""

        # Corpo binario (imagem etc.) nao sobrevive a decodificacao UTF-8: guardar
        # a string mutilada nao provaria nada num diff. Guarda tamanho + sha256,
        # que e o que de fato precisa bater entre Kotlin e Go.
        binary = bool(blob) and not is_textual(content_type, blob)
        if binary:
            text = ""
            digest = {"bytes": len(blob), "sha256": hashlib.sha256(blob).hexdigest()}
        else:
            text = blob.decode("utf-8", "replace") if isinstance(blob, bytes) else blob
            digest = None

        parsed = None
        if text and "json" in content_type.lower():
            try:
                parsed = json.loads(text, parse_float=Decimal)
            except json.JSONDecodeError:
                parsed = None

        golden = {
            "request": {
                "method": method,
                "path": re.sub(r"[0-9a-f-]{36}", "{uuid}", path, flags=re.I),
                "body": redact_body(self.normalize(json.loads(json.dumps(body), parse_float=Decimal))) if body else None,
            },
            "response": {
                "status": status,
                "headers": kept_headers(headers),
                "body_raw": redact_raw(text),
                "body": redact_body(self.normalize(parsed)) if parsed is not None else None,
            },
        }
        if digest:
            golden["response"]["body_raw"] = None
            golden["response"]["body_binary"] = digest
        elif not text:
            golden["response"]["body_empty"] = True

        fname = f"{name}.json"
        with open(os.path.join(self.out, fname), "w", encoding="utf-8") as f:
            json.dump(golden, f, indent=2, ensure_ascii=False, sort_keys=False)
            f.write("\n")

        ok = expect is None or status in (expect if isinstance(expect, (list, tuple)) else [expect])
        mark = "ok " if ok else "XX "
        if not ok:
            self.failures.append(f"{name}: esperava {expect}, veio {status}")
        print(f"  {mark}{fname:<44} {method:<6} {path[:52]:<52} {status}")
        return parsed

    def defer(self, name, method, path):
        self.cleanup.append((name, method, path))

    def run_cleanup(self):
        """Apaga o que a suite criou -- e grava o golden, porque os DELETE de
        evento e de grupo sao endpoints do contrato como qualquer outro."""
        if not self.cleanup:
            return
        print("\n-- limpeza (recursos criados por esta execucao, gravada como golden) --")
        for name, method, path in reversed(self.cleanup):
            self.record(name, method, path, expect=[200, 204])


def tiny_png():
    """PNG 1x1 valido, ~70 bytes. Montado na hora em vez de um blob base64
    para que dê para conferir que é mesmo um PNG bem-formado."""
    import struct
    import zlib

    def chunk(typ, data):
        body = typ + data
        return struct.pack(">I", len(data)) + body + struct.pack(">I", zlib.crc32(body) & 0xFFFFFFFF)

    return (b"\x89PNG\r\n\x1a\n"
            + chunk(b"IHDR", struct.pack(">IIBBBBB", 1, 1, 8, 2, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(b"\x00\xff\x00\x00"))
            + chunk(b"IEND", b""))


def multipart(field, filename, content_type, payload):
    """Monta um corpo multipart/form-data na mao (a stdlib nao tem helper)."""
    boundary = "----contract" + uuid.uuid4().hex
    body = (
        f"--{boundary}\r\n"
        f'Content-Disposition: form-data; name="{field}"; filename="{filename}"\r\n'
        f"Content-Type: {content_type}\r\n\r\n"
    ).encode() + payload + f"\r\n--{boundary}--\r\n".encode()
    return f"multipart/form-data; boundary={boundary}", body


def update_env(key, value, path=".env"):
    """Rotaciona uma linha do .env preservando o resto do arquivo."""
    lines = open(path, encoding="utf-8").read().splitlines()
    out, done = [], False
    for line in lines:
        if line.startswith(key + "="):
            out.append(f"{key}={value}")
            done = True
        else:
            out.append(line)
    if not done:
        out.append(f"{key}={value}")
    open(path, "w", encoding="utf-8").write("\n".join(out) + "\n")


def user2_token(c):
    """Token do 2o usuario. Tenta /auth/refresh primeiro -- de proposito, porque
    enquanto o email dele nao for verificado, TODO login dispara um email real
    (EmailVerificationService.sendVerificationEmail nao tem rate limit). O refresh
    token e rotacionado a cada uso e volta pro .env.

    O refresh vale 15 dias. Se a suite ficar mais que isso sem rodar, ele expira e
    o unico caminho e logar com a senha guardada -- o que pode disparar UM email.
    Verificar o email da conta de teste uma vez elimina esse custo para sempre."""
    rt = os.environ.get("LOOBBY_TEST2_REFRESH")
    if rt:
        status, _, body = c.request("POST", "/auth/refresh", {"refreshToken": rt}, auth=False)
        if status == 200:
            d = json.loads(body)
            update_env("LOOBBY_TEST2_REFRESH", d["refreshToken"])
            return d["accessToken"]

    email = os.environ.get("LOOBBY_TEST2_EMAIL")
    password = os.environ.get("LOOBBY_TEST2_PASSWORD")
    if not (email and password):
        print("  -- 2o usuario ausente do .env; rode contract/bootstrap_user2.py")
        return None

    print("  !! refresh do 2o usuario ausente ou expirado (vale 15 dias) -- relogando.")
    print("     Se o email da conta ainda nao foi verificado, isto dispara 1 email de verificacao.")
    status, _, body = c.request("POST", "/auth/login",
                                {"email": email, "password": password}, auth=False)
    if status != 200:
        print(f"  -- login do 2o usuario falhou ({status}); cenarios de membership pulados")
        return None
    d = json.loads(body)
    update_env("LOOBBY_TEST2_REFRESH", d["refreshToken"])
    return d["accessToken"]


def future_iso(days=7):
    return (datetime.now(timezone.utc) + timedelta(days=days)).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def run(c, args):
    email = os.environ["LOOBBY_TEST_EMAIL"]
    password = os.environ["LOOBBY_TEST_PASSWORD"]

    # ---------------- auth ----------------
    print("\n-- auth --")
    auth = c.record("auth-login", "POST", "/auth/login",
                    {"email": email, "password": password}, auth=False, expect=200)
    if not auth or "accessToken" not in auth:
        sys.exit("login falhou; sem token nao da para seguir")
    c.token = auth["accessToken"]
    refresh = auth["refreshToken"]
    me_id = str(auth["userId"])
    c.remember("me", me_id)

    c.record("auth-login-senha-errada", "POST", "/auth/login",
             {"email": email, "password": "senha-errada-de-proposito"}, auth=False, expect=400)  # nao 401
    c.record("auth-login-body-invalido", "POST", "/auth/login",
             {"email": "nao-e-email", "password": "x"}, auth=False, expect=400)
    c.record("auth-refresh", "POST", "/auth/refresh", {"refreshToken": refresh}, auth=False, expect=200)
    c.record("auth-refresh-invalido", "POST", "/auth/refresh",
             {"refreshToken": "token.que.nao.existe"}, auth=False, expect=[400, 401])
    c.record("auth-anonymous", "POST", "/auth/anonymous", {}, auth=False, expect=[200, 201])

    # ---------------- erros transversais ----------------
    print("\n-- erros transversais --")
    c.record("erro-401-sem-token", "GET", "/users/me", auth=False, expect=401)
    c.record("erro-nao-encontrado", "GET", "/groups/00000000-0000-0000-0000-000000000000", expect=400)  # nao 404

    # ---------------- users ----------------
    print("\n-- users --")
    c.record("users-me", "GET", "/users/me", expect=200)
    c.record("users-me-patch", "PATCH", "/users/me", {"displayname": "Contract Baseline"}, expect=200)
    c.record("users-feed", "GET", "/users/feed", expect=200)

    # ---------------- groups ----------------
    print("\n-- groups --")
    g = c.record("groups-create", "POST", "/groups", {"name": "contract-baseline-group"}, expect=[200, 201])
    gid = c.remember("group", g and g.get("id"))
    if gid:
        c.defer("groups-delete", "DELETE", f"/groups/{gid}")
    invite = g and g.get("inviteCode")

    c.record("groups-list", "GET", "/groups", expect=200)
    if gid:
        c.record("groups-get", "GET", f"/groups/{gid}", expect=200)
        c.record("groups-patch", "PATCH", f"/groups/{gid}", {"name": "contract-baseline-renomeado"}, expect=200)
        c.record("groups-members-list", "GET", f"/groups/{gid}/members", expect=200)
    if invite:
        c.record("groups-by-invite", "GET", f"/groups/invite/{invite}", expect=200)

    # ---------------- membership + permissao (2o usuario) ----------------
    print("\n-- membership + permissao (2o usuario) --")
    t2 = c.token2 = user2_token(c)
    u2 = os.environ.get("LOOBBY_TEST2_USER_ID")
    if t2 and gid and u2:
        c.remember("user2", u2)
        c.record("membros-entrar-como-user2", "POST", f"/groups/{gid}/members",
                 token=t2, expect=[200, 201, 204])
        c.record("membros-lista-com-dois", "GET", f"/groups/{gid}/members", expect=200)

        # 403 depois do deploy do handler de AccessDeniedException; producao ainda
        # responde 500 ate la. Divergencia esperada, some sozinha. Ver ACHADOS.md §11.
        c.record("grupo-patch-sem-permissao", "PATCH", f"/groups/{gid}",
                 {"name": "invasao"}, token=t2, expect=403)
        c.record("grupo-delete-sem-permissao", "DELETE", f"/groups/{gid}",
                 token=t2, expect=403)
        c.record("membros-remover-pelo-dono", "DELETE", f"/groups/{gid}/members/{u2}",
                 expect=[200, 204])
        c.record("membros-remover-nao-membro", "DELETE", f"/groups/{gid}/members/{u2}",
                 expect=400)

        c.record("membros-entrar-de-novo", "POST", f"/groups/{gid}/members",
                 token=t2, expect=[200, 201, 204])
        c.record("membros-sair-eu-mesmo", "DELETE", f"/groups/{gid}/members/me",
                 token=t2, expect=[200, 204])

    # ---------------- events ----------------
    print("\n-- events --")
    ev = None
    if gid:
        ev = c.record("events-create-sport", "POST", f"/groups/{gid}/events", {
            "eventType": "SPORT",
            "name": "contract-baseline-sport",
            "description": None,
            "scheduledDatetime": future_iso(),
            "sport": {
                "durationMinutes": 90,
                "arena": "Quadra Contract",
                # o ponto sensivel: numeric(12,2) precisa voltar como 0.00 / 25.50
                "pricePerPlayer": "25.50",
                "maxPlayers": 10,
                "acceptReserve": True,
            },
        }, expect=[200, 201])
    eid = c.remember("event", ev and ev.get("id"))
    if eid:
        c.defer("events-delete", "DELETE", f"/events/{eid}")

    gp = c.record("events-create-instant-gameplay", "POST", "/events/instant", {
        "eventType": "GAMEPLAY",
        "name": "contract-baseline-gameplay",
        "scheduledDatetime": future_iso(1),
        "gameplay": {"gameName": "Contract Test Game"},
    }, expect=[200, 201])
    gpid = c.remember("event-instant", gp and gp.get("id"))
    if gpid:
        c.defer("events-instant-delete", "DELETE", f"/events/{gpid}")

    if gid:
        c.record("events-list-do-grupo", "GET", f"/groups/{gid}/events", expect=200)
    if eid:
        c.record("events-get", "GET", f"/events/{eid}", expect=200)
        c.record("events-update", "PUT", f"/events/{eid}",
                 {"name": "contract-baseline-sport-v2"}, expect=200)
        ic = ev.get("inviteCode") if ev else None
        if ic:
            c.record("events-by-invite", "GET", f"/events/invite/{ic}", expect=200)

        # rsvps
        print("\n-- rsvps --")
        c.record("rsvp-me-vazio", "GET", f"/events/{eid}/rsvps/me", expect=[200, 404])
        c.record("rsvp-upsert", "PUT", f"/events/{eid}/rsvps",
                 {"status": "YES", "isPaid": False, "obs": None}, expect=[200, 201])
        c.record("rsvp-list", "GET", f"/events/{eid}/rsvps", expect=200)
        c.record("rsvp-me", "GET", f"/events/{eid}/rsvps/me", expect=200)

        # teams
        print("\n-- teams --")
        t = c.record("teams-create", "POST", f"/events/{eid}/teams",
                     {"name": "Time A", "color": "#FF0000"}, expect=[200, 201])
        tid = c.remember("team", t and t.get("id"))
        c.record("teams-list", "GET", f"/events/{eid}/teams", expect=200)
        if tid:
            c.record("teams-update", "PUT", f"/events/{eid}/teams/{tid}",
                     {"name": "Time A prime", "color": "#00FF00"}, expect=200)
            c.record("teams-add-player", "POST", f"/events/{eid}/teams/{tid}/players",
                     {"userId": me_id, "role": "atacante"}, expect=[200, 201])
            c.record("teams-update-player", "PUT", f"/events/{eid}/teams/{tid}/players/{me_id}",
                     {"role": "goleiro"}, expect=200)
            c.record("teams-remove-player", "DELETE", f"/events/{eid}/teams/{tid}/players/{me_id}",
                     expect=[200, 204])
            # antes do auto-generate: ele faz deleteByEventId e leva os times junto
            c.record("teams-delete", "DELETE", f"/events/{eid}/teams/{tid}", expect=[200, 204])
            c.record("teams-delete-inexistente", "DELETE", f"/events/{eid}/teams/{tid}", expect=400)
        c.record("teams-auto-generate", "POST", f"/events/{eid}/teams/auto-generate",
                 {"teamCount": 2, "includeReserves": True}, expect=[200, 201])
        c.record("rsvp-me-delete", "DELETE", f"/events/{eid}/rsvps/me", expect=[200, 204])

    # ---------------- games ----------------
    print("\n-- games (proxy RAWG) --")
    s = c.record("games-search", "GET", "/games/search?q=zelda&page=1", expect=200)
    first = None
    if s and isinstance(s.get("results"), list) and s["results"]:
        first = s["results"][0].get("id")
    c.record("games-search-cache-hit", "GET", "/games/search?q=zelda&page=1", expect=200)
    if first:
        c.record("games-details", "GET", f"/games/{first}", expect=200)
    # 404 depois do deploy do fix no ApiExceptionHandler; producao ainda responde
    # 500 ate la, entao esta divergencia e esperada e some sozinha. Ver ACHADOS.md §6.
    c.record("games-details-inexistente", "GET", "/games/id-que-nao-existe-9999999", expect=404)

    # ---------------- devices ----------------
    print("\n-- devices --")
    tok = "contract-baseline-fake-fcm-token"
    c.record("devices-register", "POST", "/devices/register",
             {"token": tok, "platform": "ANDROID"}, expect=[200, 201, 204])
    c.record("devices-delete", "DELETE", f"/devices/{tok}", expect=[200, 204])

    # ---------------- legal + infra (publicos) ----------------
    print("\n-- legal + infra --")
    for slug in ("privacy", "account-deletion", "child-safety"):
        c.record(f"legal-{slug}", "GET", f"/legal/{slug}", auth=False, expect=200)
    c.record("actuator-health", "GET", "/actuator/health", auth=False, expect=200)
    # O Kotlin nao configura CORS nenhum: todo preflight e recusado com 403 e o
    # corpo literal "Invalid CORS request". Browser de outra origem nao consegue
    # chamar a API com requisicao nao-simples.
    c.record("cors-preflight-recusado", "OPTIONS", "/legal/privacy", auth=False,
             extra_headers={"Origin": "https://exemplo.com", "Access-Control-Request-Method": "GET"},
             expect=403)

    # ---------------- gated: efeitos externos ----------------
    # ---------------- caminhos de erro do auth/users ----------------
    # Os caminhos FELIZES destes endpoints exigem token vindo por email, ou destroem
    # a conta de teste -- ficam pendentes ate o cutover das fases 2 e 3 (ver
    # ENDPOINTS.md). Os de erro, abaixo, sao seguros e cobrem o que mais importa
    # para o Go: status e envelope.
    print("\n-- caminhos de erro (auth + users) --")

    c.record("register-ja-registrado", "POST", "/auth/register",
             {"email": email, "password": "qualquer-uma"}, expect=400)
    # Token malformado estoura na biblioteca do Google como IllegalArgumentException
    # sem mensagem -> 400 com "message": null. O 401 de GoogleAuthService:81 exige um
    # JWT bem-formado que o verificador rejeite -- nao coberto. Ver ACHADOS.md §15.
    c.record("google-token-malformado", "POST", "/auth/google",
             {"idToken": "nao.e.um.id.token"}, auth=False, expect=400)
    c.record("verify-email-token-invalido", "GET",
             "/auth/verify-email?token=00000000-0000-0000-0000-000000000000",
             auth=False, expect=400)
    c.record("reset-password-form-token-invalido", "GET",
             "/auth/reset-password?token=00000000-0000-0000-0000-000000000000",
             auth=False, expect=400)
    c.record("reset-password-token-invalido", "POST", "/auth/reset-password",
             {"token": "00000000-0000-0000-0000-000000000000", "password": "novaSenha123"},
             auth=False, expect=400)

    # changePassword valida NA ORDEM: tamanho, confirmacao, diferenca, e so entao
    # compara a senha atual -- e so grava depois disso. Senha errada nao altera nada.
    # Os quatro abaixo esperam o comportamento POS-DEPLOY do fix de
    # ResponseStatusException (ACHADOS.md §6). A producao ainda responde 500.
    if c.token2:
        c.record("senha-nova-curta", "PUT", "/users/me/password",
                 {"currentPassword": "irrelevante", "newPassword": "abc",
                  "confirmPassword": "abc"}, token=c.token2, expect=400)
        c.record("senha-confirmacao-diferente", "PUT", "/users/me/password",
                 {"currentPassword": "irrelevante", "newPassword": "umaSenhaBoa1",
                  "confirmPassword": "outraSenhaBoa2"}, token=c.token2, expect=400)
        c.record("senha-atual-errada", "PUT", "/users/me/password",
                 {"currentPassword": "senha-que-nao-e-a-dele", "newPassword": "umaSenhaBoa1",
                  "confirmPassword": "umaSenhaBoa1"}, token=c.token2, expect=401)
        # deleteAccount tambem valida a senha antes de qualquer mutacao.
        c.record("excluir-conta-senha-errada", "DELETE", "/users/me",
                 {"password": "senha-que-nao-e-a-dele"}, token=c.token2, expect=401)

    # ---------------- gated: uploads ----------------
    # Atras de flag porque FileStorageService.store() nomeia o arquivo como
    # <epochMillis>.<ext> e NUNCA apaga o anterior: cada execucao deixaria um
    # arquivo permanente no disco da VM, sem rota de exclusao pela API.
    if args.include_uploads:
        print("\n-- uploads (deixam arquivo no disco da producao) --")
        ctype, payload = multipart("file", "contract.png", "image/png", tiny_png())

        prof = c.record("avatar-upload", "POST", "/users/me/avatar",
                        raw_body=payload, ctype=ctype, expect=200)
        url = prof.get("avatarUrl") if prof else None
        if url:
            # /files/** e publico e so existe depois de um upload: cobre os dois de uma vez
            c.record("files-servir-avatar", "GET", url, auth=False, expect=200)

        # Restaura o estado: avatarUrl volta a null, senao o golden de users-me
        # muda entre execucoes. Cuidado: null significa "nao alterar" -- quem limpa
        # e string vazia (updateUserProfile faz trim().ifBlank { null }).
        c.record("avatar-limpar", "PATCH", "/users/me", {"avatarUrl": ""}, expect=200)

        if gid:
            ctype, payload = multipart("file", "grupo.png", "image/png", tiny_png())
            c.record("grupo-imagem-upload", "POST", f"/groups/{gid}/image",
                     raw_body=payload, ctype=ctype, expect=200)

        # Erro: tipo nao permitido (store() so aceita jpeg/png/webp)
        ctype, payload = multipart("file", "contract.txt", "text/plain", b"nao sou imagem")
        c.record("avatar-tipo-invalido", "POST", "/users/me/avatar",
                 raw_body=payload, ctype=ctype, expect=400)

    if args.include_email:
        print("\n-- email (Resend dispara de verdade) --")
        c.record("auth-forgot-password", "POST", "/auth/forgot-password",
                 {"email": email}, auth=False, expect=[200, 204])
        c.record("auth-resend-verification", "POST", "/auth/resend-verification", {}, expect=[200, 204, 429])


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--base-url", default=None, help="default: LOOBBY_BASE_URL do .env")
    p.add_argument("--out", default="contract/golden")
    p.add_argument("--include-email", action="store_true",
                   help="inclui forgot-password e resend-verification (mandam email de verdade)")
    p.add_argument("--include-uploads", action="store_true",
                   help="inclui os multipart; deixa arquivo permanente no disco do servidor")
    p.add_argument("--keep", action="store_true", help="nao apaga os recursos criados")
    args = p.parse_args()

    load_env()
    base = args.base_url or os.environ.get("LOOBBY_BASE_URL")
    c = Capture(base, args.out)
    print(f"capturando de {c.base} -> {c.out}/")
    try:
        run(c, args)
    finally:
        if not args.keep:
            c.run_cleanup()

    print(f"\n{c.n} golden gravados em {c.out}/")
    if c.failures:
        print(f"\n{len(c.failures)} status fora do esperado (o golden foi gravado mesmo assim):")
        for f in c.failures:
            print("  - " + f)
    # Nunca falha o processo: a captura de baseline registra a realidade,
    # inclusive quando ela diverge do que eu esperava lendo o codigo.
    return 0


if __name__ == "__main__":
    sys.exit(main())
