#!/usr/bin/env python3
"""Sonde d'acceptation WashGuard — garanties d'accès et de contrat d'API.

Elle interroge une API **déjà démarrée** (par défaut http://127.0.0.1:8099) :
aucune dépendance, uniquement la bibliothèque standard.

    python3 scripts/verifications/sonde-api.py [base_url]

Chaque contrôle affiche OK (comportement attendu) ou KO (défaut constaté) et
la sonde se termine par un code de sortie non nul en cas d'échec, ce qui la
rend utilisable en intégration continue une fois un serveur lancé.

Contrôles : cloisonnement entre entreprises (404), permissions par rôle (403),
prix issus du catalogue, contrats de réponse des routes principales.
"""
import json
import sys
import urllib.error
import urllib.request
import uuid

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8099") + "/api/v1"


class Client:
    def __init__(self):
        self.cookie = None

    def call(self, method, path, body=None, raw=False):
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(BASE + path, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        if self.cookie:
            req.add_header("Cookie", self.cookie)
        try:
            with urllib.request.urlopen(req, timeout=20) as resp:
                payload = resp.read().decode()
                if resp.headers.get("Set-Cookie"):
                    self.cookie = resp.headers["Set-Cookie"].split(";")[0]
                return resp.status, (payload if raw else json.loads(payload or "null"))
        except urllib.error.HTTPError as err:
            payload = err.read().decode()
            try:
                return err.code, json.loads(payload or "null")
            except json.JSONDecodeError:
                return err.code, payload
        except Exception as exc:  # noqa: BLE001
            return 0, str(exc)

    def login(self, email, password="demo1234"):
        code, payload = self.call("POST", "/auth/login", {"email": email, "password": password})
        return code == 200, payload

    def register(self, name):
        code, payload = self.call("POST", "/auth/register", {
            "organization_name": name,
            "full_name": "Propriétaire Sonde",
            "email": f"sonde-{uuid.uuid4().hex[:8]}@test.local",
            "password": "sonde12345",
            "station_name": f"Station {name}",
            "phone": "+225 27 00 00 00 00",
            "city": "Abidjan",
        })
        return code == 201, payload


def mettre_en_route(client, station_id, prix=2500, categories=("berline", "moto")):
    """Complète la mise en route d'une entreprise neuve, puis l'active.

    Depuis l'onboarding, une station fraîche n'a **ni horaires, ni catégorie, ni
    tarif** : l'application n'invente rien, et le serveur refuse d'exploiter
    (`onboarding_incomplete`) tant que l'exploitant n'a pas déclaré ce qu'il
    lave, quand il ouvre et à quel prix. Les contrôles de cloisonnement se font
    donc sur des entreprises exploitables, comme en production.
    """
    client.call("PUT", f"/stations/{station_id}/opening-hours", {
        "days": [
            {"weekday": jour, "is_open": True, "opens_at": "08:00", "closes_at": "18:00",
             "break_start": "", "break_end": "", "last_checkin_at": "", "booking_allowed": True}
            for jour in range(1, 6)
        ] + [
            {"weekday": jour, "is_open": False, "opens_at": "", "closes_at": "",
             "break_start": "", "break_end": "", "last_checkin_at": "", "booking_allowed": False}
            for jour in (6, 7)
        ],
        "exceptions": [],
    })
    client.call("PUT", f"/stations/{station_id}/vehicle-categories", {
        "categories": [{"code": code, "is_active": True, "sort_order": (i + 1) * 10}
                       for i, code in enumerate(categories)],
    })
    # La prestation naît « à tarifer » : le prix se pose dans la grille
    # tarifaire, avec son motif (c'est la seule route qui écrit un tarif).
    code, service = client.call("POST", "/services", {
        "station_id": station_id, "name": "Lavage simple", "duration_minutes": 30,
    })
    if isinstance(service, dict) and service.get("id"):
        client.call("PUT", "/price-rules/matrix", {
            "station_id": station_id, "reason": "Grille d'acceptation",
            "generic_prices": [{"service_id": service["id"], "price_xof": prix}],
        })
    code, etat = client.call("POST", "/onboarding/activate")
    return (service if isinstance(service, dict) else {}), etat


RESULTS = []


def check(name, ok, detail=""):
    RESULTS.append((name, ok, detail))
    print(f"{'OK ' if ok else 'KO '} {name}" + (f" — {detail}" if detail else ""), flush=True)


def ignore(name, detail=""):
    """Un contrôle qui n'a pas lieu d'être sur cette base.

    Exemple : les rôles de démonstration. Une base vierge (`SEED_DEMO=false`)
    n'a pas de caissier de démonstration ; ce n'est pas une régression, et ce
    n'est pas non plus un succès : la ligne le dit, et elle n'entre pas dans le
    décompte.
    """
    print(f"·   {name} — ignoré" + (f" : {detail}" if detail else ""), flush=True)


def main():
    # --- Deux entreprises distinctes -------------------------------------
    victim = Client()
    ok, payload = victim.register("Entreprise Victime")
    if not ok:
        print("impossible de créer l'entreprise victime :", payload)
        return 1
    victim_org = payload["user"]["organization_id"]
    victim_station = payload["station"]["id"]
    victim_user = payload["user"]["id"]

    attacker = Client()
    ok, payload = attacker.register("Entreprise Curieuse")
    if not ok:
        print("impossible de créer l'entreprise curieuse :", payload)
        return 1
    attacker_station = payload["station"]["id"]
    mettre_en_route(attacker, attacker_station)

    # La station créée à l'inscription doit exiger une caisse ouverte avant
    # d'encaisser les espèces — c'était l'inverse sur ce seul chemin.
    code, stations = victim.call("GET", "/stations")
    station_creee = next((st for st in stations if st["id"] == victim_station), {}) if isinstance(stations, list) else {}
    reglages = station_creee.get("settings") or {}
    check("la station créée à l'inscription exige une caisse ouverte",
          reglages.get("require_open_cash_session") is True,
          f"http={code} réglages={sorted(reglages)}")

    # La victime termine sa mise en route : horaires déclarés et prestation
    # tarifée par l'exploitant. Avant cela, l'exploitation est refusée.
    code, refus = victim.call("POST", "/wash-orders", {
        "station_id": victim_station, "customer_name": "Client Trop Tôt",
        "items": [{"service_id": "", "quantity": 1}],
    })
    check("une station non configurée ne vend pas (409 onboarding_incomplete)",
          code == 409 and (refus.get("error") or {}).get("code") == "onboarding_incomplete",
          f"http={code} {(refus.get('error') or {}).get('code')}")

    # Le catalogue part équipé (18 modèles de référence) mais **non tarifé** :
    # une inscription ne doit jamais porter un prix que l'exploitant n'a pas
    # décidé. On contrôle donc l'absence de tarif, pas l'absence de prestation.
    code, avant = victim.call("GET", f"/services?station_id={victim_station}")
    services = avant.get("items", []) if isinstance(avant, dict) else []
    invente = [p for p in services if (p.get("price_xof") or 0) > 0]
    check("aucun tarif n'est inventé à l'inscription",
          bool(services) and not invente,
          f"http={code} services={len(services)} avec_prix={len(invente)}")

    service, activation = mettre_en_route(victim, victim_station)
    check("la mise en route déclarée par l'exploitant aboutit",
          bool(service.get("id")) and activation.get("state") == "active",
          f"prix={service.get('price_xof')} état={activation.get('state')}")

    # Espèces sans caisse ouverte : refusé, et la fiche reste intacte.
    code, sans_caisse = victim.call("POST", "/wash-orders", {
        "station_id": victim_station, "customer_name": "Client Sans Caisse",
        "items": [{"service_id": service["id"], "quantity": 1}],
    })
    if code == 201:
        code, refus = victim.call("POST", f"/wash-orders/{sans_caisse['id']}/payments",
                                  {"amount_xof": sans_caisse["total_xof"], "method": "especes"})
        check("espèces sans caisse ouverte : encaissement refusé",
              code == 409 and (refus.get("error") or {}).get("code") == "cash_session_required",
              f"http={code} {refus.get('error', {}).get('code')}")
        victim.call("POST", f"/wash-orders/{sans_caisse['id']}/cancel",
                    {"reason": "Fiche de sonde sans caisse"})

    # fiche de lavage chez la victime — avec un prix client volontairement faux
    code, order = victim.call("POST", "/wash-orders", {
        "station_id": victim_station,
        "customer_name": "Client Victime",
        "status": "pending",
        "items": [{"service_id": service["id"], "quantity": 1, "unit_price_xof": 1}],
    })
    order_id = order.get("id") if isinstance(order, dict) else None
    check("la victime peut créer une fiche", code == 201 and order_id, f"http={code} {order if code != 201 else ''}")
    if order_id:
        # Le prix attendu est celui que le **serveur** applique (l'aperçu), pas
        # celui que l'écran a envoyé : c'est tout l'enjeu de la grille.
        _, apercu = victim.call(
            "GET", f"/price-preview?station_id={victim_station}&service_id={service['id']}")
        applique = (apercu or {}).get("price_xof")
        check("le prix vient du serveur, pas du poste client",
              applique is not None and order.get("total_xof") == applique
              and order.get("total_xof") != 1,
              f"total={order.get('total_xof')} appliqué={applique}")
        check("une fiche naît avec un reste à payer calculé",
              order.get("due_xof") == applique, f"due={order.get('due_xof')} appliqué={applique}")

    # --- Cloisonnement : lecture ------------------------------------------
    code, _ = attacker.call("GET", f"/wash-orders/{order_id}")
    check("une autre entreprise ne peut pas lire la fiche (404 attendu)", code == 404, f"http={code}")

    code, body = attacker.call("GET", f"/wash-orders?station_id={victim_station}")
    items = body.get("items", []) if isinstance(body, dict) else []
    check("une autre entreprise ne voit pas les lavages d'une station qui n'est pas la sienne",
          code != 201 and not items, f"http={code} items={len(items)}")

    code, body = attacker.call("GET", f"/cash/current?station_id={victim_station}")
    check("une autre entreprise ne peut pas lire la caisse d'une autre station",
          not (isinstance(body, dict) and body.get("session")), f"http={code}")

    code, body = attacker.call("GET", f"/cash/payments?station_id={victim_station}")
    items = body.get("items", []) if isinstance(body, dict) else []
    check("une autre entreprise ne voit pas les encaissements d'une autre station",
          not items, f"http={code} items={len(items)}")

    code, body = attacker.call("GET", f"/services?station_id={victim_station}")
    items = body.get("items", []) if isinstance(body, dict) else []
    check("une autre entreprise ne voit pas le catalogue d'une autre station",
          not items, f"http={code} items={len(items)}")

    code, _ = attacker.call("GET", f"/employees/{victim_user}/activity")
    check("une autre entreprise ne peut pas lire l'activité d'un employé (404 attendu)", code == 404, f"http={code}")

    # --- Cloisonnement : écriture ----------------------------------------
    code, body = attacker.call("POST", "/employees", {
        "station_id": victim_station, "full_name": "Intrus", "email": f"intrus-{uuid.uuid4().hex[:6]}@test.local",
    })
    check("impossible de créer un employé dans la station d'une autre entreprise", code == 404, f"http={code}")

    code, body = attacker.call("POST", "/cash/sessions", {"station_id": victim_station, "opening_float_xof": 1000})
    check("impossible d'ouvrir une caisse dans une autre entreprise", code == 404, f"http={code}")

    code, body = attacker.call("POST", "/expenses", {
        "station_id": victim_station, "label": "Dépense intruse", "amount_xof": 5000, "category": "autre",
    })
    check("impossible d'enregistrer une dépense chez une autre entreprise", code == 404, f"http={code}")

    code, body = attacker.call("POST", "/customers", {
        "station_id": victim_station, "full_name": "Client intrus",
    })
    check("impossible de créer un client chez une autre entreprise", code == 404, f"http={code}")

    code, body = attacker.call("POST", "/wash-orders", {
        "station_id": victim_station, "customer_name": "Fiche intruse", "status": "pending",
        "items": [{"service_id": "", "quantity": 1}],
    })
    check("impossible de créer une fiche chez une autre entreprise", code == 404, f"http={code}")

    # --- Permissions -------------------------------------------------------
    code, _ = attacker.call("GET", "/commissions")
    check("les commissions sont réservées (403 attendu pour une sonde propriétaire ? non : 200 propriétaire)",
          code == 200, f"http={code} (propriétaire)")

    # caissier de démonstration
    cashier = Client()
    ok, payload = cashier.login("cashier@demo.local")
    if ok:
        check("le caissier de démonstration peut ouvrir une session", True, "")
    else:
        ignore("caissier de démonstration",
               "aucune donnée de démonstration sur cette base : les contrôles de rôle du "
               "caissier sont couverts par sonde-metier.py, qui exige SEED_DEMO=true")
    if ok:
        code, _ = cashier.call("GET", "/commissions")
        check("un caissier ne lit pas les commissions (403 attendu)", code == 403, f"http={code}")

        code, _ = cashier.call("GET", "/employees")
        check("un caissier ne lit pas les fiches employés (403 attendu)", code == 403, f"http={code}")

        code, _ = cashier.call("PATCH", "/organizations/current", {"name": "Renommée par le caissier"})
        check("un caissier ne modifie pas l'entreprise (403 attendu)", code == 403, f"http={code}")

        code, body = cashier.call("GET", "/stations")
        station_id = ""
        if isinstance(body, dict):
            items = body.get("items") or []
            station_id = items[0]["id"] if items else ""
        elif isinstance(body, list):
            station_id = body[0]["id"] if body else ""
        if station_id:
            code, _ = cashier.call("PATCH", f"/stations/{station_id}", {"settings": {"max_cashier_discount_xof": 999999}})
            check("un caissier ne modifie pas les réglages de la station (403 attendu)", code == 403, f"http={code}")

    # laveur de démonstration
    washer = Client()
    ok, _ = washer.login("washer@demo.local")
    if not ok:
        ignore("laveur de démonstration", "aucune donnée de démonstration sur cette base")
    if ok:
        code, _ = washer.call("POST", "/expenses", {"label": "Dépense du laveur", "amount_xof": 100, "category": "autre"})
        check("un laveur ne peut pas enregistrer de dépense (403 attendu)", code == 403, f"http={code}")
        code, _ = washer.call("GET", "/reports/overview?from=2020-01-01&to=2030-01-01")
        check("un laveur ne lit pas les rapports (403 attendu)", code == 403, f"http={code}")

    # --- Contrat d'API -----------------------------------------------------
    owner = Client()
    ok, _ = owner.login("owner@demo.local")
    if not ok:
        ignore("contrat d'API (propriétaire de démonstration)",
               "aucune donnée de démonstration sur cette base")
    if ok:
        code, body = owner.call("GET", "/stations")
        check("GET /stations renvoie un tableau (contrat front)", isinstance(body, list), f"http={code} type={type(body).__name__}")

        code, body = owner.call("GET", "/employees/options")
        check("GET /employees/options renvoie « options »", isinstance(body, dict) and "options" in body,
              f"http={code} clés={list(body)[:4] if isinstance(body, dict) else body}")

        code, body = owner.call("GET", "/cash/current")
        check("GET /cash/current renvoie {open,last,movements}",
              isinstance(body, dict) and {"open", "last", "movements"} <= set(body), f"http={code} clés={list(body)[:5] if isinstance(body,dict) else body}")

        code, body = owner.call("GET", "/reports/overview?from=2020-01-01&to=2030-01-01")
        check("GET /reports/overview renvoie totals/by_service/by_employee/by_method",
              isinstance(body, dict) and {"totals", "by_service", "by_employee", "by_method"} <= set(body),
              f"http={code} clés={list(body)[:8] if isinstance(body, dict) else body}")

        code, body = owner.call("GET", f"/commissions?from=2020-01-01&to=2030-01-01")
        check("GET /commissions renvoie period/settings/totals/items",
              isinstance(body, dict) and {"period", "totals", "items"} <= set(body),
              f"http={code} clés={list(body)[:5] if isinstance(body, dict) else body}")

    # --- Espace laveur -----------------------------------------------------
    if ok:
        code, _ = washer.call("GET", "/washer/tasks")
        check("GET /washer/tasks répond (pas 500)", code == 200, f"http={code}")
        code, body = washer.call("GET", "/washer/me/performance")
        check("GET /washer/me/performance renvoie washes_today/commission_month_xof",
              isinstance(body, dict) and {"washes_today", "commission_month_xof"} <= set(body),
              f"http={code} clés={list(body)[:5] if isinstance(body, dict) else body}")

    # --- Règles métier -----------------------------------------------------
    code, body = attacker.call("GET", "/platform/overview")
    check("l'espace plateforme est refusé à un propriétaire (403 attendu)", code == 403, f"http={code}")

    # --- Espace plateforme (compte annoncé par le README) ------------------
    # Le seed crée admin@washguard.app hors production : sans lui, /administration
    # n'est atteignable par personne sur une base neuve.
    if ok:
        platform = Client()
        connecte, _ = platform.login("admin@washguard.app", "admin1234")
        check("le compte plateforme du README se connecte", connecte, "admin@washguard.app")
        if connecte:
            for chemin, cle in (("/platform/overview", None),
                                ("/platform/organizations", "items"),
                                ("/platform/plans", "items"),
                                ("/platform/audit", "items")):
                code, body = platform.call("GET", chemin)
                forme = isinstance(body, dict) and (cle is None or cle in body)
                check(f"GET {chemin} répond au support", code == 200 and forme,
                      f"http={code} clés={list(body)[:4] if isinstance(body, dict) else body}")

    print()
    failed = [name for name, ok_, _ in RESULTS if not ok_]
    print(f"{len(RESULTS) - len(failed)}/{len(RESULTS)} contrôles au vert")
    for name in failed:
        print("  KO :", name)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
