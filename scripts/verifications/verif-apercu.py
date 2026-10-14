#!/usr/bin/env python3
"""Simule le navigateur d'aperçu : aucun cookie conservé, session par jeton.

Le client envoie le jeton reçu au corps de la réponse d'authentification en
`Authorization: Bearer` — exactement ce que fait src/lib/api.ts en développement.
"""
import json, sys, urllib.request, urllib.error, time

BASE = "http://127.0.0.1:3000/api/v1"
HOST = "sbx-ggfaa7663rgmskcb.arena.site"

def appel(method, path, body=None, jeton="", cookies=""):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Host", HOST)
    req.add_header("Content-Type", "application/json")
    if jeton:
        req.add_header("Authorization", f"Bearer {jeton}")
    if cookies:
        req.add_header("Cookie", cookies)
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            brut = r.read().decode()
            return r.status, (json.loads(brut) if brut else None), r.headers
    except urllib.error.HTTPError as e:
        brut = e.read().decode()
        try:
            return e.code, json.loads(brut or "null"), e.headers
        except json.JSONDecodeError:
            return e.code, brut, e.headers

ok = []
def check(nom, cond, detail=""):
    ok.append(bool(cond))
    print(f"{'OK ' if cond else 'KO '} {nom}" + (f" — {detail}" if detail else ""))

# 1. Inscription d'une entreprise neuve : la réponse porte le jeton de session.
email = f"apercu{int(time.time())}@exemple.ci"
code, rep, _ = appel("POST", "/auth/register", {
    "full_name": "Awa Koné", "email": email, "password": "MotDePasse2026!",
    "organization_name": "Lavage Aperçu", "station_name": "Station Cocody",
    "city": "Abidjan", "phone": "+225 07 00 00 00 00",
})
check("inscription d'une entreprise neuve", code == 201, f"http={code}")
jeton = (rep or {}).get("session_token", "")
check("la réponse porte le jeton de session (hors production)", len(jeton) >= 32,
      f"{len(jeton)} caractères")

# 2. Les requêtes suivantes n'envoient QUE ce jeton (aucun cookie).
code, me, _ = appel("GET", "/auth/me", jeton=jeton)
check("la session est reconnue sans cookie", code == 200 and me.get("user", {}).get("email") == email,
      f"http={code}")

code, onboard, _ = appel("GET", "/onboarding", jeton=jeton)
etapes = [e.get("code") for e in (onboard or {}).get("steps", [])]
check("l'état de la mise en route est lisible", code == 200 and len(etapes) == 13,
      f"http={code} étapes={len(etapes)}")
check("les treize étapes sont celles annoncées",
      etapes == ["espace", "entreprise", "station", "horaires", "categories", "usages",
                 "prestations", "options", "tarifs", "moyens", "equipe", "materiel",
                 "commandes"], str(etapes))

# 3. Un compte neuf encaisse dès le premier jour : les espèces sont actives.
station = (me or {}).get("station", {}).get("id", "")
code, moyens, _ = appel("GET", f"/payment-methods?station_id={station}", jeton=jeton)
items = (moyens or {}).get("items", [])
actifs = [m.get("code") for m in items if m.get("is_active")]
check("le référentiel des moyens est posé", code == 200 and len(items) == 9,
      f"http={code} total={len(items)}")
check("les espèces sont actives d'emblée", actifs == ["especes"], f"actifs={actifs}")

# 4. Le parcours se termine : horaires, catégories, tarif, puis activation.
code, _, _ = appel("PATCH", "/organizations/current",
                   {"trade_name": "Lavage Aperçu", "billing_phone": "+225 07 00 00 00 00"}, jeton=jeton)
check("la fiche d'entreprise s'enregistre par le jeton", code == 200, f"http={code}")

code, _, _ = appel("POST", "/services", {"station_id": station, "name": "Lavage complet",
                                         "duration_minutes": 30}, jeton=jeton)
check("une prestation se crée (sans prix inventé)", code == 201, f"http={code}")

services = appel("GET", f"/services?station_id={station}", jeton=jeton)[1]
service_id = ((services or {}).get("items") or [{}])[0].get("id", "")
code, _, _ = appel("PUT", "/price-rules/matrix",
                   {"station_id": station, "reason": "Grille d'ouverture",
                    "generic_prices": [{"service_id": service_id, "price_xof": 3000}]}, jeton=jeton)
check("la grille tarifaire s'enregistre", code == 200, f"http={code}")

code, _, _ = appel("PUT", f"/stations/{station}/vehicle-categories",
                   {"categories": [{"code": "berline", "is_active": True, "sort_order": 10}]}, jeton=jeton)
check("une catégorie de véhicule s'active", code == 200, f"http={code}")

# 5. Sans jeton, la porte reste fermée.
code, _, _ = appel("GET", "/onboarding")
check("sans session, la mise en route reste fermée (401)", code == 401, f"http={code}")
code, _, _ = appel("GET", "/onboarding", jeton="jeton-invente")
check("un jeton inventé ne vaut rien (401)", code == 401, f"http={code}")

# 6. Le cookie continue de fonctionner (voie normale, hors aperçu).
_, _, entetes = appel("POST", "/auth/login", {"email": email, "password": "MotDePasse2026!"})
cookie = ""
for ligne in entetes.get_all("Set-Cookie") or []:
    if ligne.startswith("washguard_session="):
        cookie = ligne.split(";")[0]
code, _, _ = appel("GET", "/auth/me", cookies=cookie)
check("le cookie de session fonctionne toujours", code == 200 and bool(cookie), f"http={code}")

# 7. Déconnexion : la session tombe des deux côtés.
appel("POST", "/auth/logout", jeton=jeton)
code, _, _ = appel("GET", "/auth/me", jeton=jeton)
check("après déconnexion, le jeton ne vaut plus rien (401)", code == 401, f"http={code}")

print(f"\n{sum(ok)}/{len(ok)} contrôles au vert")
sys.exit(0 if all(ok) else 1)
