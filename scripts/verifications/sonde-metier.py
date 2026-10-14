#!/usr/bin/env python3
"""Sonde métier WashGuard : caisse, remises, soldes, plafonds de formule.

    python3 scripts/verifications/sonde-metier.py [base_url]

À lancer sur une API démarrée avec les données de démonstration
(`SEED_DEMO=true`) : elle se connecte avec owner@demo.local / demo1234, puis
avec les comptes caissier et laveur pour éprouver les plafonds et les refus.

Aucune dépendance : uniquement la bibliothèque standard.
"""
import json
import os
import sys
import urllib.error
import urllib.request
import uuid

BASE = (sys.argv[1] if len(sys.argv) > 1
        else "http://127.0.0.1:" + os.environ.get("API_PORT", "8080")) + "/api/v1"
RESULTS = []


class Client:
    def __init__(self):
        self.cookie = None

    def call(self, method, path, body=None):
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
                return resp.status, json.loads(payload or "null")
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
        })
        return code == 201, payload


def check(name, ok, detail=""):
    RESULTS.append((name, ok, detail))
    print(f"{'OK ' if ok else 'KO '} {name}" + (f" — {detail}" if detail else ""), flush=True)


def main():
    owner = Client()
    ok, _ = owner.login("owner@demo.local")
    if not ok:
        print("connexion du propriétaire de démonstration impossible (SEED_DEMO ?)")
        return 1

    _, stations = owner.call("GET", "/stations")
    station = stations[0]["id"] if isinstance(stations, list) and stations else ""
    _, catalogue = owner.call("GET", f"/services?station_id={station}")
    service = catalogue["items"][0]

    # --- Session de caisse ouverte ---------------------------------------
    code, session = owner.call("POST", "/cash/sessions", {"station_id": station, "opening_float_xof": 20000})
    if code == 409:
        # La sonde tourne sur une base déjà utilisée : on clôture la session
        # ouverte pour repartir d'un compteur à zéro (sinon les soldes
        # théoriques mélangent deux campagnes).
        _, courant = owner.call("GET", f"/cash/current?station_id={station}")
        courant = courant or {}
        ouverte = courant.get("session")
        if not isinstance(ouverte, dict):
            ouverte = courant.get("open")
        if isinstance(ouverte, dict) and ouverte.get("id"):
            _, detail = owner.call("GET", f"/cash/sessions/{ouverte['id']}")
            solde = (detail or {}).get("expected_cash_xof", 0)
            code_cloture, _ = owner.call("POST", f"/cash/sessions/{ouverte['id']}/close",
                                         {"counted_cash_xof": solde, "note": "Clôture par la sonde"})
            print(f"    (session précédente clôturée : http={code_cloture})", flush=True)
        code, session = owner.call("POST", "/cash/sessions", {"station_id": station, "opening_float_xof": 20000})
    check("ouverture d'une session de caisse", code == 201, f"http={code}")
    code, again = owner.call("POST", "/cash/sessions", {"station_id": station, "opening_float_xof": 1000})
    check("impossible d'ouvrir deux sessions sur la même station", code == 409, f"http={code}")

    # --- Fiche soldée d'un coup ------------------------------------------
    code, order = owner.call("POST", "/wash-orders", {
        "station_id": station, "customer_name": "Client Sonde", "status": "pending",
        "items": [{"service_id": service["id"], "quantity": 1}],
    })
    check("création d'une fiche au prix du catalogue", order.get("total_xof") == service["price_xof"],
          f"total={order.get('total_xof')} attendu={service['price_xof']}")

    code, payment = owner.call("POST", f"/wash-orders/{order['id']}/payments", {
        "amount_xof": order["total_xof"], "method": "especes",
    })
    check("encaissement complet accepté", code == 200, f"http={code}")
    check("la fiche passe terminée quand le solde est nul",
          payment["order"]["status"] == "completed", f"statut={payment['order']['status']}")

    # --- Paiement partiel : la fiche reste ouverte ------------------------
    code, order2 = owner.call("POST", "/wash-orders", {
        "station_id": station, "customer_name": "Client Partiel", "status": "pending",
        "items": [{"service_id": service["id"], "quantity": 1}],
    })
    part = max(1, order2["total_xof"] // 2)
    code, partial = owner.call("POST", f"/wash-orders/{order2['id']}/payments", {"amount_xof": part})
    check("un acompte n'éteint pas la fiche",
          code == 200 and partial["order"]["status"] != "completed",
          f"http={code} statut={partial['order'].get('status')} reste={partial.get('due_xof')}")
    check("le reste à payer est recalculé", partial.get("due_xof") == order2["total_xof"] - part,
          f"reste={partial.get('due_xof')} attendu={order2['total_xof'] - part}")

    code, too_much = owner.call("POST", f"/wash-orders/{order2['id']}/payments",
                                {"amount_xof": order2["total_xof"] * 3})
    check("un encaissement supérieur au solde est refusé", code == 400, f"http={code}")

    # Un montant nul ou absent ne doit pas solder la fiche à la place du
    # caissier : le refus est le même que sur /cash/payments.
    for montant in (0, -part):
        code, zero = owner.call("POST", f"/wash-orders/{order2['id']}/payments",
                                {"amount_xof": montant, "method": "especes"})
        check(f"un encaissement de {montant} FCFA est refusé", code == 400, f"http={code}")
    _, apres_refus = owner.call("GET", f"/wash-orders/{order2['id']}")
    # Le détail d'une fiche est renvoyé sous la clé « order ».
    reste = apres_refus["order"]["due_xof"]
    check("le solde de la fiche est inchangé après un montant refusé",
          reste == order2["total_xof"] - part,
          f"reste={reste} attendu={order2['total_xof'] - part}")

    # --- Caisse : les compteurs suivent la méthode ------------------------
    _, cash = owner.call("GET", f"/cash/current?station_id={station}")
    session = cash["session"]
    expected_cash = 20000 + order["total_xof"] + part
    check("les espèces alimentent le compteur d'espèces",
          session["cash_in_xof"] == order["total_xof"] + part, f"cash_in={session['cash_in_xof']}")
    check("les espèces n'alimentent plus les « autres » encaissements",
          session["other_in_xof"] == 0, f"other_in={session['other_in_xof']}")
    check("le solde théorique suit les encaissements",
          session["expected_cash_xof"] == expected_cash,
          f"theorique={session['expected_cash_xof']} attendu={expected_cash}")

    # --- Mobile money puis annulation -------------------------------------
    code, cashPay = owner.call("POST", f"/wash-orders/{order2['id']}/payments",
                               {"amount_xof": order2["total_xof"] - part, "method": "mobile_money"})
    check("complément en mobile money accepté", code == 200, f"http={code}")
    _, cash = owner.call("GET", f"/cash/current?station_id={station}")
    mobile_after = cash["session"]["mobile_money_xof"]
    check("le mobile money alimente son propre compteur",
          mobile_after == order2["total_xof"] - part, f"mobile={mobile_after}")

    _, payments = owner.call("GET", f"/cash/payments?station_id={station}")
    pay_id = [p for p in payments["items"] if p["method"] == "mobile_money"][0]["id"]
    code, voided = owner.call("POST", f"/cash/payments/{pay_id}/void", {"reason": "Erreur de saisie à l'encaissement"})
    check("annulation d'un encaissement avec motif", code == 200 and voided["status"] == "voided", f"http={code}")
    _, cash = owner.call("GET", f"/cash/current?station_id={station}")
    check("l'annulation redescend le compteur mobile money",
          cash["session"]["mobile_money_xof"] == 0, f"mobile={cash['session']['mobile_money_xof']}")
    code, voided_no_reason = owner.call("POST", f"/cash/payments/{pay_id}/void", {"reason": ""})
    check("une annulation sans motif est refusée", code in (400, 409), f"http={code}")

    # --- Remise : plafond du caissier -------------------------------------
    cashier = Client()
    ok, _ = cashier.login("cashier@demo.local")
    if ok:
        _, me = cashier.call("GET", "/auth/me")
        station_of_cashier = me["station"]["id"] if me.get("station") else station
        code, denied = cashier.call("POST", "/wash-orders", {
            "station_id": station_of_cashier, "customer_name": "Remise géante",
            "discount_xof": 500000, "discount_reason": "Trop de remise",
            "items": [{"service_id": service["id"], "quantity": 1}],
        })
        check("une remise au-delà du plafond du caissier est refusée", code == 400, f"http={code}")

        code, small = cashier.call("POST", "/wash-orders", {
            "station_id": station_of_cashier, "customer_name": "Petite remise",
            "discount_xof": 500, "discount_reason": "Client fidèle",
            "items": [{"service_id": service["id"], "quantity": 1}],
        })
        check("une remise dans le plafond passe", code == 201, f"http={code}")
        if code == 201:
            code, boosted = cashier.call("PATCH", f"/wash-orders/{small['id']}", {"discount_xof": 400000})
            check("le plafond s'applique aussi à la modification", code == 400, f"http={code}")
            code, canceled = cashier.call("POST", f"/wash-orders/{small['id']}/cancel", {"reason": "Test"})
            check("un caissier n'annule pas une fiche (403 attendu)", code == 403, f"http={code}")

    # --- Laveur : terminer un lavage non encaissé --------------------------
    washer = Client()
    ok, _ = washer.login("washer@demo.local")
    if ok:
        code, order3 = owner.call("POST", "/wash-orders", {
            "station_id": station, "customer_name": "Client Laveur", "status": "pending",
            "items": [{"service_id": service["id"], "quantity": 1}],
        })
        _, employees = owner.call("GET", "/employees/options")
        opts = employees.get("options") if isinstance(employees, dict) else None
        if not isinstance(opts, list) or not all(isinstance(e, dict) for e in opts):
            print("DEBUG /employees/options →", json.dumps(employees, ensure_ascii=False)[:300], flush=True)
            opts = []
        washer_id = next((e["id"] for e in opts if e.get("role") == "washer"), "")
        code, affected = owner.call("POST", f"/wash-orders/{order3['id']}/assign", {"employee_id": washer_id})
        check("affectation d'un laveur", code == 200, f"http={code}")
        _, tasks = washer.call("GET", "/washer/tasks")
        task_ids = [t["id"] for t in tasks["items"]]
        check("le laveur voit sa tâche", order3["id"] in task_ids, f"tâches={len(task_ids)}")
        code, refusal = washer.call("POST", f"/washer/tasks/{order3['id']}/status", {"status": "completed"})
        check("terminer un lavage non encaissé est refusé", code == 409, f"http={code}")
        code, start = washer.call("POST", f"/washer/tasks/{order3['id']}/status", {"status": "in_progress"})
        check("le laveur démarre son lavage", code == 200, f"http={code}")
        code, incident = washer.call("POST", f"/washer/tasks/{order3['id']}/incident",
                                     {"incident_note": "Rayure constatée à l'avant droit"})
        check("signalement d'incident enregistré", code == 201, f"http={code}")
        if code == 201:
            check("l'incident est visible sur la fiche",
                  incident.get("incident_note") == "Rayure constatée à l'avant droit",
                  f"note={incident.get('incident_note')}")

    # --- Dépense liée à la caisse -----------------------------------------
    _, cash = owner.call("GET", f"/cash/current?station_id={station}")
    before_expenses = cash["session"]["expenses_xof"]
    code, expense = owner.call("POST", "/expenses", {
        "station_id": station, "label": "Produits de lavage", "category": "produits",
        "amount_xof": 3000, "method": "especes",
    })
    check("dépense en espèces enregistrée", code == 201, f"http={code}")
    _, cash = owner.call("GET", f"/cash/current?station_id={station}")
    check("la dépense sort de la caisse",
          cash["session"]["expenses_xof"] == before_expenses + 3000,
          f"depenses={cash['session']['expenses_xof']} avant={before_expenses}")

    # --- Formule : plafond de stations ------------------------------------
    code, second = owner.call("POST", "/stations", {"name": "Station Bis"})
    check("la formule d'essai limite à une station (403 attendu)", code == 403, f"http={code}")

    # --- Rapports : cohérence des totaux -----------------------------------
    code, overview = owner.call("GET", f"/reports/overview?from=2020-01-01&to=2030-01-01")
    totals = overview.get("totals", {}) if isinstance(overview, dict) else {}
    _, wash_orders = owner.call("GET", f"/wash-orders?station_id={station}&from=2020-01-01&to=2030-01-01")
    revenue = sum(o["total_xof"] for o in wash_orders["items"] if o["status"] == "completed")
    check("le chiffre d'affaires du rapport correspond aux fiches terminées",
          totals.get("revenue_xof") == revenue,
          f"rapport={totals.get('revenue_xof')} fiches={revenue}")
    check("aucun produit cartésien : les dépenses ne gonflent pas le CA",
          totals.get("revenue_xof", 0) < 10_000_000, f"CA={totals.get('revenue_xof')}")

    # --- Objectif du mois --------------------------------------------------
    code, goal = owner.call("GET", f"/reports/dashboard?station_id={station}")
    check("le tableau de bord répond", code == 200, f"http={code}")


    print()
    failed = [name for name, ok_, _ in RESULTS if not ok_]
    print(f"{len(RESULTS) - len(failed)}/{len(RESULTS)} contrôles au vert")
    for name in failed:
        print("  KO :", name)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
