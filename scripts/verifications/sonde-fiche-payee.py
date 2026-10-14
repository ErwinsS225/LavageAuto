#!/usr/bin/env python3
"""Sonde « fiche encaissée » : une fiche payée ne se réécrit plus.

    python3 scripts/verifications/sonde-fiche-payee.py [base_url]

Vérifie qu'une fiche portant un encaissement refuse une nouvelle remise, un
retour en atelier et une annulation, tout en laissant modifiables les champs
purement descriptifs (plaque, note, client, laveur, catégorie).
"""
import json
import os
import sys
import urllib.error
import urllib.request

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


def check(name, ok, detail=""):
    RESULTS.append((name, ok, detail))
    print(f"{'OK ' if ok else 'KO '} {name}" + (f" — {detail}" if detail else ""), flush=True)


def code_of(payload):
    if isinstance(payload, dict):
        err = payload.get("error")
        if isinstance(err, dict):
            return err.get("code")
    return None


def code_of_erreur(payload, cle="receipt"):
    """Champ fautif renvoyé par le serveur, s'il y en a un."""
    if isinstance(payload, dict):
        err = payload.get("error")
        if isinstance(err, dict):
            champs = err.get("fields") or err.get("details") or {}
            if isinstance(champs, dict):
                return champs.get(cle)
    return None


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

    # La session de caisse peut déjà être ouverte par une campagne précédente.
    code, _ = owner.call("POST", "/cash/sessions", {"station_id": station, "opening_float_xof": 20000})
    check("session de caisse disponible", code in (201, 409), f"http={code}")

    # --- Fiche encaissée en totalité --------------------------------------
    code, order = owner.call("POST", "/wash-orders", {
        "station_id": station, "customer_name": "Client Verrou", "status": "pending",
        "items": [{"service_id": service["id"], "quantity": 1}],
    })
    check("création d'une fiche", code == 201, f"http={code}")
    code, paid = owner.call("POST", f"/wash-orders/{order['id']}/payments", {
        "amount_xof": order["total_xof"], "method": "especes",
    })
    check("encaissement complet", code == 200, f"http={code}")

    # 1. Plus de remise rétroactive sur une fiche payée.
    code, denied = owner.call("PATCH", f"/wash-orders/{order['id']}",
                              {"discount_xof": 500, "discount_reason": "Remise après coup"})
    check("remise refusée sur une fiche encaissée", code == 409, f"http={code} {code_of(denied)}")

    # 2. Plus de retour en atelier : le statut est figé.
    code, denied = owner.call("PATCH", f"/wash-orders/{order['id']}", {"status": "pending"})
    check("retour en atelier refusé sur une fiche encaissée", code == 409, f"http={code} {code_of(denied)}")

    # 3. Les champs descriptifs restent modifiables (plaque, note, client).
    code, updated = owner.call("PATCH", f"/wash-orders/{order['id']}",
                               {"notes": "Note ajoutée après encaissement", "plate": "AA-999-ZZ"})
    check("champs descriptifs encore modifiables", code == 200, f"http={code}")

    code, detail = owner.call("GET", f"/wash-orders/{order['id']}")
    fiche = detail.get("order", {}) if isinstance(detail, dict) else {}
    check("la note est bien enregistrée",
          code == 200 and (fiche.get("notes") or "") == "Note ajoutée après encaissement",
          f"http={code} notes={fiche.get('notes')}")

    # 4. Annulation d'une fiche payée : toujours refusée (déjà couvert, on
    #    vérifie la non-régression après l'ajout du verrou).
    code, denied = owner.call("POST", f"/wash-orders/{order['id']}/cancel", {"reason": "Test sonde"})
    check("annulation d'une fiche encaissée toujours refusée", code == 409, f"http={code}")

    # ---------------------------------------------------------------------
    # Lot 6 : moyen déclaré, reçu, paiement différé.
    # ---------------------------------------------------------------------
    code, moyens = owner.call("GET", f"/payment-methods?station_id={station}")
    items = (moyens or {}).get("items", []) if isinstance(moyens, dict) else {}
    orange = next((m for m in items if m.get("code") == "orange_money"), {})
    differe = next((m for m in items if m.get("code") == "differe"), {})
    check("le référentiel des moyens est posé pour la station",
          code == 200 and len(items) == 9,
          f"http={code} total={len(items)}")

    # La sonde décide de son état de départ : les moyens mobiles et le différé
    # sont remis inactifs, pour que le contrôle dise la même chose à chaque
    # exécution (les campagnes précédentes les ont activés).
    for moyen in (orange, differe):
        if moyen.get("id"):
            owner.call("PATCH", f"/payment-methods/{moyen.get('id')}", {"is_active": False})

    # Un moyen déclaré mais inactif ne s'utilise pas : la caisse propose
    # seulement ce que la station a activé, et le serveur refuse le reste.
    code, autres = owner.call("GET", f"/payment-methods?station_id={station}&actives=1")
    actifs = [m.get("code") for m in (autres or {}).get("items", [])]
    check("seuls les moyens actifs sont proposés à la caisse",
          code == 200 and "orange_money" not in actifs and "especes" in actifs,
          f"actifs={actifs}")

    code, fiche3 = owner.call("POST", "/wash-orders", {
        "station_id": station, "customer_name": "Client Référence", "status": "pending",
        "items": [{"service_id": service["id"], "quantity": 1}],
    })
    code, inactif = owner.call("POST", f"/wash-orders/{fiche3['id']}/payments", {
        "amount_xof": 500, "payment_method_id": orange.get("id"), "reference": "TX-1",
    })
    check("un moyen désactivé ne peut pas encaisser",
          code == 409, f"http={code} code={code_of(inactif)}")

    code, _ = owner.call("PATCH", f"/payment-methods/{orange.get('id')}",
                         {"is_active": True, "name": "Orange Money CI"})
    # Le moyen déclaré exige une référence : le serveur refuse sans elle et
    # désigne le champ fautif, au lieu d'inventer un numéro de transaction.
    code, refus = owner.call("POST", f"/wash-orders/{fiche3['id']}/payments", {
        "amount_xof": 500, "payment_method_id": orange.get("id"),
    })
    check("un moyen à référence refuse un encaissement sans référence",
          code == 400 and code_of_erreur(refus, "reference"),
          f"http={code} champs={code_of_erreur(refus, 'reference')}")

    code, partiel = owner.call("POST", f"/wash-orders/{fiche3['id']}/payments", {
        "amount_xof": 500, "payment_method_id": orange.get("id"), "reference": "OM-2026-0001",
    })
    code, detail3 = owner.call("GET", f"/wash-orders/{fiche3['id']}")
    reglement = ((detail3 or {}).get("payments") or [{}])[0] if isinstance(detail3, dict) else {}
    check("le libellé du moyen déclaré est recopié sur l'encaissement",
          code == 200 and reglement.get("method") == "mobile_money"
          and (reglement.get("payment_method_name") or "") == "Orange Money CI"
          and partiel.get("due_xof") == fiche3["total_xof"] - 500,
          f"famille={reglement.get('method')} libellé={reglement.get('payment_method_name')} "
          f"dû={partiel.get('due_xof')}")

    # --- Reçu de la fiche soldée -----------------------------------------
    code, charge = owner.call("GET", f"/wash-orders/{order['id']}/receipt")
    recu = (charge or {}).get("receipt", {}) if isinstance(charge, dict) else {}
    numero = recu.get("number", "")
    check("la fiche soldée porte un numéro de reçu",
          code == 200 and len(numero) >= 12 and numero.split("-")[-1].isdigit(),
          f"http={code} numéro={numero}")

    code, rejoue = owner.call("POST", f"/wash-orders/{order['id']}/receipt")
    numero2 = (rejoue or {}).get("receipt", {}).get("number", "") if isinstance(rejoue, dict) else ""
    check("réémettre un reçu ne change pas son numéro",
          code in (200, 201) and numero2 == numero, f"http={code} numéro={numero2}")

    mentions = recu.get("missing_mentions", [])
    check("les mentions fiscales absentes ne sont pas inventées",
          recu.get("tax_id", "") == "" and any("fiscal" in m for m in mentions),
          f"identifiant fiscal={recu.get('tax_id')!r} manquantes={mentions}")
    check("le reçu ne reprend que des encaissements réels",
          recu.get("paid_xof") == fiche.get("total_xof")
          and len(recu.get("payments", [])) == 1,
          f"payé={recu.get('paid_xof')} règlements={len(recu.get('payments', []))}")

    # --- Paiement différé : un engagement n'est pas un encaissement -------
    code, _ = owner.call("PATCH", f"/payment-methods/{differe.get('id')}", {"is_active": True})
    check("le moyen différé peut être activé", code == 200, f"http={code}")

    _, ouverte = owner.call("GET", f"/cash/current?station_id={station}")
    avant = (ouverte or {}).get("session") or {}
    code, fiche2 = owner.call("POST", "/wash-orders", {
        "station_id": station, "customer_name": "Client Différé", "status": "pending",
        "items": [{"service_id": service["id"], "quantity": 1}],
    })
    code, engagement = owner.call("POST", f"/wash-orders/{fiche2['id']}/payments", {
        "amount_xof": fiche2["total_xof"], "payment_method_id": differe.get("id"),
        "reference": "Bon de commande entreprise",
    })
    check("un paiement différé s'enregistre", code == 200, f"http={code}")

    code, detail2 = owner.call("GET", f"/wash-orders/{fiche2['id']}")
    suite = (detail2 or {}).get("order", {}) if isinstance(detail2, dict) else {}
    check("un différé ne solde pas la fiche",
          code == 200 and suite.get("paid_xof", 0) == 0
          and suite.get("due_xof", 0) == fiche2["total_xof"]
          and suite.get("status") != "completed",
          f"payé={suite.get('paid_xof')} dû={suite.get('due_xof')} état={suite.get('status')}")

    code, refuse = owner.call("GET", f"/wash-orders/{fiche2['id']}/receipt")
    check("une fiche non soldée n'a pas de reçu",
          code == 404 and code_of(refuse) == "receipt_not_issued",
          f"http={code} code={code_of(refuse)}")

    _, apres = owner.call("GET", f"/cash/current?station_id={station}")
    maintenant = (apres or {}).get("session") or {}
    check("un différé ne bouge aucun compteur de caisse",
          maintenant.get("cash_in_xof") == avant.get("cash_in_xof")
          and maintenant.get("payment_count") == avant.get("payment_count"),
          f"espèces={maintenant.get('cash_in_xof')} (avant {avant.get('cash_in_xof')})")

    total = len(RESULTS)
    verts = sum(1 for _, ok, _ in RESULTS if ok)
    print(f"\n{verts}/{total} contrôles au vert")
    return 0 if verts == total else 1


if __name__ == "__main__":
    sys.exit(main())
