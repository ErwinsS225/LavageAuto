#!/usr/bin/env python3
"""Suspension d'entreprise : l'accès est coupé immédiatement, sans révocation.

    python3 scripts/verifications/sonde-suspension.py [base_url]

La sonde crée une entreprise jetable, vérifie l'accès, bascule son statut en
`suspended` directement en base (c'est l'action d'un administrateur), puis
contrôle que :

* le profil (`/auth/me`) et une route métier répondent 403 ;
* le code d'erreur est bien `organization_suspended` (et non un 401 confus) ;
* la réactivation rend l'accès **sans nouvelle connexion** : la session n'est
  pas révoquée, seule la vérification du statut coupe l'accès.

Accès à la base : la commande SQL est lue dans la variable d'environnement
`WASHGUARD_PSQL` (elle doit lire le SQL sur l'entrée standard) ; par défaut
`psql "$DATABASE_URL"`. Dans un bac à sable sans client `psql`, on peut
fournir un petit script équivalent.
"""
import json
import os
import shlex
import subprocess
import sys
import urllib.error
import urllib.request
import uuid

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8099") + "/api/v1"
cookie = None


def call(method, path, body=None):
    global cookie
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if cookie:
        req.add_header("Cookie", cookie)
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            if resp.headers.get("Set-Cookie"):
                cookie = resp.headers["Set-Cookie"].split(";")[0]
            return resp.status, json.loads(resp.read().decode() or "null")
    except urllib.error.HTTPError as err:
        try:
            return err.code, json.loads(err.read().decode() or "null")
        except json.JSONDecodeError:
            return err.code, ""


def run_sql(sql):
    """Exécute une requête SQL de service via la commande configurée.

    Les variables d'environnement de la commande sont développées
    (`psql "$DATABASE_URL"` fonctionne sans passer par un shell).
    """
    command = os.environ.get("WASHGUARD_PSQL", 'psql "$DATABASE_URL"')
    subprocess.run(shlex.split(os.path.expandvars(command)), input=sql, text=True, check=True)


def main():
    status, payload = call("POST", "/auth/register", {
        "organization_name": "Entreprise Suspendue",
        "full_name": "Propriétaire Suspendu",
        "email": f"sup-{uuid.uuid4().hex[:8]}@test.local",
        "password": "suspendu12345",
        "station_name": "Station Suspendue",
    })
    if status != 201 or not isinstance(payload, dict) or "user" not in payload:
        print("inscription refusée :", status, payload)
        return 1
    org_id = payload["user"]["organization_id"]
    print("inscription :", status, "— entreprise :", org_id, flush=True)

    code, _ = call("GET", "/auth/me")
    print("accès avant suspension :", code, flush=True)

    run_sql(f"UPDATE organizations SET status = 'suspended', is_active = false WHERE id = '{org_id}';")
    code, body = call("GET", "/auth/me")
    inner = body.get("error", {}) if isinstance(body, dict) else {}
    print("accès après suspension :", code, inner.get("code"), "—", inner.get("message"))
    ok = code == 403 and inner.get("code") == "organization_suspended"

    # Les routes métier sont coupées aussi, pas seulement le profil.
    code2, _ = call("GET", "/wash-orders")
    print("liste des lavages après suspension :", code2)
    ok = ok and code2 == 403

    # Réactivation : l'accès revient sans nouvelle connexion.
    run_sql(f"UPDATE organizations SET status = 'active', is_active = true WHERE id = '{org_id}';")
    code3, _ = call("GET", "/auth/me")
    print("accès après réactivation :", code3)
    ok = ok and code3 == 200

    print("\n" + ("1/1 contrôle au vert" if ok else "0/1 : la suspension ne coupe pas l'accès"))
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
