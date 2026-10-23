#!/usr/bin/env python3
"""
Compara duas capturas da suite de contrato. E o criterio de aceite do strangler:
depois de mover uma rota do Kotlin para o Go, a captura nova tem que ser
indistinguivel da baseline.

    python3 contract/compare.py contract/golden contract/depois

NAO use `diff -ru` para isso: duas capturas do MESMO Kotlin ja diferem em dezenas
de arquivos, porque o body_raw carrega UUIDs, timestamps e codigos aleatorios
reais. Medido: 42 arquivos diferentes entre duas execucoes seguidas.

O que e comparado, por golden:
  - status
  - headers de contrato (os de KEPT_HEADERS)
  - corpo:
      JSON       -> a visao normalizada (`body`). Preserva chaves, ordem, null vs
                    ausente, [] vs [null] e o texto literal dos numeros (0.00 vs 0).
      texto      -> body_raw byte a byte (HTML de /legal, "Invalid CORS request")
      binario    -> sha256

Limite conhecido: a normalizacao troca timestamps por "<timestamp>", entao a
PRECISAO da fracao (0/3/6/9 digitos) nao e comparada aqui -- ela varia por valor
ate dentro do proprio Kotlin. Quem trava isso e o teste httpx.TestInstantGrouping
do lado Go.

Goldens de games dependem do conteudo do RAWG, que muda com o tempo (rating,
novos jogos). Para eles a comparacao e estrutural: chaves, tipos e o FORMATO dos
numeros (#.## vs #.#), nao os valores -- assim pega a armadilha do 0.0 vs 0.00
sem alarme falso quando o RAWG atualiza uma nota.
"""
import glob
import json
import os
import re
import sys

EXTERNAL_DATA = re.compile(r"^games-")


def shape(v):
    """Estrutura sem valores: para dados externos que mudam com o tempo."""
    if isinstance(v, dict):
        if set(v) == {"$num"}:
            return {"$num": re.sub(r"\d", "#", v["$num"])}
        return {k: shape(x) for k, x in v.items()}
    if isinstance(v, list):
        # o tamanho da lista do RAWG muda; compara o formato dos elementos
        return [shape(v[0])] if v else []
    if isinstance(v, str):
        return "<str>"
    return v  # bool, int, None: tipo e valor


def response_view(path):
    g = json.load(open(path, encoding="utf-8"))
    r = g["response"]
    view = {"status": r["status"], "headers": r.get("headers", {})}
    if r.get("body_binary"):
        view["body"] = {"sha256": r["body_binary"]["sha256"]}
    elif r.get("body") is not None:
        body = r["body"]
        if EXTERNAL_DATA.match(os.path.basename(path)):
            body = shape(body)
        view["body"] = body
    else:
        view["body"] = r.get("body_raw") or ""
    return view


def first_difference(a, b, where=""):
    if type(a) != type(b):
        return where or "(raiz)", a, b
    if isinstance(a, dict):
        if list(a) != list(b):
            return where + " [chaves/ordem]", list(a), list(b)
        for k in a:
            d = first_difference(a[k], b[k], f"{where}.{k}")
            if d:
                return d
    elif isinstance(a, list):
        if len(a) != len(b):
            return where + " [tamanho]", len(a), len(b)
        for i, (x, y) in enumerate(zip(a, b)):
            d = first_difference(x, y, f"{where}[{i}]")
            if d:
                return d
    elif a != b:
        return where or "(raiz)", a, b
    return None


def main():
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    base, other = sys.argv[1:]
    names = sorted(os.path.basename(p) for p in glob.glob(os.path.join(base, "*.json")))
    if not names:
        sys.exit(f"nenhum golden em {base}")

    problems = []
    for n in names:
        o = os.path.join(other, n)
        if not os.path.exists(o):
            problems.append((n, "ausente na captura nova", "", ""))
            continue
        a, b = response_view(os.path.join(base, n)), response_view(o)
        for part in ("status", "headers", "body"):
            d = first_difference(a[part], b[part], part)
            if d:
                problems.append((n,) + d)
                break

    extra = sorted(set(os.path.basename(p) for p in glob.glob(os.path.join(other, "*.json"))) - set(names))
    for n in extra:
        problems.append((n, "existe so na captura nova", "", ""))

    for n, where, x, y in problems:
        print(f"DIFERE {n}\n   em: {where}\n   base: {str(x)[:160]}\n   nova: {str(y)[:160]}")
    print(f"\n{len(names)} golden comparados, {len(problems)} com diferenca")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
