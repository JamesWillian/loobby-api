#!/usr/bin/env python3
"""
Servicos externos falsos para rodar Kotlin e Go localmente, lado a lado
(contract/local/README.md). Sem dependencias fora da stdlib e do `openssl`.

    python3 contract/local/fakes.py PORTA --google-cert google.crt

  POST   /resend-<lado>/emails     Resend: guarda o email e responde 200
  GET    /emails?lado=&to=         quantos emails um destinatario recebeu, e o ultimo
  DELETE /emails                   esvazia a caixa (entre duas execucoes)
  GET    /rawg/games?search=&page= RAWG: busca (dados reconstruidos dos golden)
  GET    /rawg/games/<id>          RAWG: detalhe, ou 404
  GET    /google/v1/certs          chave do "Google" como certificado PEM (o Kotlin le assim)
  GET    /google/v3/certs          a mesma chave como JWK (o Go le assim)

<lado> e so um rotulo (kotlin, go): cada servidor recebe uma RESEND_BASE_URL
diferente, e assim os emails de um nao se misturam com os do outro.
"""
import argparse
import base64
import json
import os
import re
import subprocess
import threading
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
EMAILS, LOCK = [], threading.Lock()
GOOGLE = {"pem": None, "jwk": None}


def golden(name):
    path = os.path.join(ROOT, "contract", "golden", name + ".json")
    return json.loads(json.load(open(path, encoding="utf-8"))["response"]["body_raw"])


def to_rawg(item):
    """Resposta da API -> o que o RAWG teria mandado (nomes de campo e id numerico)."""
    rename = {"backgroundImage": "background_image", "descriptionRaw": "description_raw"}
    out = {rename.get(k, k): v for k, v in item.items()}
    out["id"] = int(out["id"]) if str(out["id"]).isdigit() else out["id"]
    return out


SEARCH = golden("games-search")
DETAIL = golden("games-details")
RESULTS = [to_rawg(r) for r in SEARCH["results"]]
DETAILS = {str(DETAIL["id"]): to_rawg(DETAIL)}
for r in RESULTS:
    DETAILS.setdefault(str(r["id"]), dict(
        r, description_raw=f"Descricao de {r['name']}",
        genres=[{"id": 4, "name": "Action", "slug": "action"}], platforms=None))


def load_google(cert_path):
    """Le o certificado e prepara as duas formas em que a chave e servida."""
    pem = open(cert_path, encoding="ascii").read()
    modulus = subprocess.run(
        ["openssl", "x509", "-in", cert_path, "-noout", "-modulus"],
        check=True, capture_output=True, text=True).stdout.strip().split("=", 1)[1]
    n = base64.urlsafe_b64encode(bytes.fromhex(modulus)).rstrip(b"=").decode()
    GOOGLE["pem"] = {"fake-google-key": pem}
    GOOGLE["jwk"] = {"keys": [{
        "kid": "fake-google-key", "kty": "RSA", "alg": "RS256", "use": "sig", "n": n, "e": "AQAB"}]}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def send(self, status, obj, cache=None):
        body = json.dumps(obj).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        if cache:
            self.send_header("Cache-Control", cache)
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        m = re.match(r"^/resend-(\w+)/emails$", self.path)
        if not m:
            return self.send(404, {"error": "rota"})
        raw = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        try:
            body = json.loads(raw)
        except Exception as e:
            # O Kotlin monta o JSON por concatenacao: se quebrar, fica registrado.
            with LOCK:
                EMAILS.append({"lado": m.group(1), "invalido": raw.decode("utf-8", "replace")})
            return self.send(422, {"message": f"JSON invalido: {e}"})
        with LOCK:
            EMAILS.append({"lado": m.group(1), "auth": self.headers.get("Authorization"), **body})
            n = len(EMAILS)
        self.send(200, {"id": f"fake-{n}"})

    def do_DELETE(self):
        if self.path != "/emails":
            return self.send(404, {"error": "rota"})
        with LOCK:
            EMAILS.clear()
        self.send(200, {"count": 0})

    def do_GET(self):
        u = urllib.parse.urlparse(self.path)
        q = urllib.parse.parse_qs(u.query)
        if u.path == "/emails":
            lado, to = q.get("lado", [""])[0], q.get("to", [""])[0]
            with LOCK:
                hits = [e for e in EMAILS if e["lado"] == lado and (not to or to in e.get("to", []))]
            return self.send(200, {"count": len(hits), "last": hits[-1] if hits else None})
        if u.path == "/rawg/games":
            if int(q.get("page", ["1"])[0]) > 50:
                return self.send(404, {"detail": "Invalid page."})
            return self.send(200, {"count": SEARCH["count"], "next": None, "previous": None, "results": RESULTS})
        m = re.match(r"^/rawg/games/(.+)$", u.path)
        if m:
            d = DETAILS.get(urllib.parse.unquote(m.group(1)))
            return self.send(200, d) if d else self.send(404, {"detail": "Not found."})
        if u.path == "/google/v1/certs" and GOOGLE["pem"]:
            return self.send(200, GOOGLE["pem"], cache="public, max-age=3600")
        if u.path == "/google/v3/certs" and GOOGLE["jwk"]:
            return self.send(200, GOOGLE["jwk"], cache="public, max-age=3600")
        self.send(404, {"error": "rota"})


def main():
    p = argparse.ArgumentParser()
    p.add_argument("port", type=int)
    p.add_argument("--google-cert", help="certificado X.509 (PEM) da chave que assina os ID tokens falsos")
    args = p.parse_args()
    if args.google_cert:
        load_google(args.google_cert)
    ThreadingHTTPServer(("127.0.0.1", args.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
