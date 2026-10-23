#!/usr/bin/env python3
"""
Cria, UMA VEZ, a segunda conta de teste usada pelos cenarios de membership e 403.

    python3 contract/bootstrap_user2.py

Fluxo: POST /auth/anonymous -> POST /auth/register. Grava as credenciais e o
refresh token no .env. A suite nunca cria conta: ela so le o que esta ali.

Custo, uma vez so: o /auth/register dispara um email de verificacao real (via
Resend) para o endereco escolhido. Nas execucoes seguintes a suite usa
/auth/refresh, que nao envia nada -- por isso o refresh token e guardado.
"""
import json
import os
import secrets
import sys
import urllib.error
import urllib.request

ENV = ".env"
EMAIL_DEFAULT = "contract-suite@loobby.app"


def load_env():
    env = {}
    for line in open(ENV, encoding="utf-8"):
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            k, v = line.split("=", 1)
            env[k.strip()] = v.strip()
    return env


def post(base, path, body, token=None):
    req = urllib.request.Request(base + path, data=json.dumps(body).encode(), method="POST")
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return r.status, json.loads(r.read().decode())
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def main():
    env = load_env()
    if env.get("LOOBBY_TEST2_EMAIL"):
        sys.exit(f"ja existe LOOBBY_TEST2_EMAIL no {ENV} — nada a fazer.\n"
                 f"Refresh expirado NAO pede conta nova: a suite reloga sozinha com a\n"
                 f"senha guardada. So remova as linhas LOOBBY_TEST2_* se a conta foi apagada.")

    base = env["LOOBBY_BASE_URL"].rstrip("/")
    email = os.environ.get("USER2_EMAIL", EMAIL_DEFAULT)
    password = secrets.token_urlsafe(18)

    print(f"base : {base}")
    print(f"email: {email}")

    status, anon = post(base, "/auth/anonymous", {})
    if status != 200:
        sys.exit(f"/auth/anonymous falhou: {status} {anon}")
    print(f"  anonimo criado: {anon['username']} ({anon['userId']})")

    status, reg = post(base, "/auth/register",
                       {"email": email, "password": password},
                       token=anon["accessToken"])
    if status != 200:
        sys.exit(f"/auth/register falhou: {status} {reg}\n"
                 f"O usuario anonimo {anon['userId']} ficou orfao.")
    print(f"  registrado: {reg['username']} ({reg['userId']})")
    print("  1 email de verificacao enviado (esperado, so desta vez)")

    with open(ENV, "a", encoding="utf-8") as f:
        f.write("\n# 2a conta de teste — criada por contract/bootstrap_user2.py\n")
        f.write("# A suite usa o refresh token para nao logar (login dispara email\n")
        f.write("# enquanto o endereco nao for verificado) e rotaciona a linha abaixo.\n")
        f.write(f"LOOBBY_TEST2_EMAIL={email}\n")
        f.write(f"LOOBBY_TEST2_PASSWORD={password}\n")
        f.write(f"LOOBBY_TEST2_USER_ID={reg['userId']}\n")
        f.write(f"LOOBBY_TEST2_REFRESH={reg['refreshToken']}\n")
    print(f"\ncredenciais gravadas em {ENV} (ignorado pelo git)")


if __name__ == "__main__":
    main()
