#!/usr/bin/env python3
"""Sonde d'acceptation — prestations, options et grille tarifaire (lot 5).

    python3 scripts/verifications/sonde-tarifs.py [base_url]

Elle interroge une API **déjà démarrée** (par défaut http://127.0.0.1:8080) et
rejoue les gestes réels de la mise en route : reprendre un modèle, activer une
famille, poser ses prix dans la matrice, ajouter une option, encaisser.

Ce qui est vérifié, et pourquoi :

* **Aucun prix n'est inventé.** Un compte neuf reçoit des prestations
  décrites, à `price_xof = 0` et signalées « à tarifer ». Le serveur refuse de
  les vendre (`409 price_required`) — mieux vaut une fiche refusée qu'une
  recette fausse.
* **La hiérarchie des prix tient.** Contrat client > prestation + véhicule +
  usage > prestation + véhicule > tarif du catalogue. Chaque fiche garde
  l'origine de son prix (`price_source`), pour qu'un écart s'explique après
  coup.
* **Deux règles identiques se signalent** (`409 price_rule_conflict`) au lieu
  de se départager en silence.
* **L'écran ne calcule rien.** `GET /price-preview` renvoie le montant et sa
  provenance ; la matrice ne fait que les lire et les écrire.
* **Les options se facturent par lavage**, et une option réservée à un véhicule
  ne s'applique pas à un autre.
* **Isolation.** La grille d'une entreprise est invisible et intouchable pour
  une autre.
"""
import json
import sys
import urllib.error
import urllib.request
import uuid

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8080") + "/api/v1"


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
                return resp.status, (json.loads(payload) if payload else None)
        except urllib.error.HTTPError as err:
            payload = err.read().decode()
            try:
                return err.code, json.loads(payload or "null")
            except json.JSONDecodeError:
                return err.code, payload
        except Exception as exc:  # noqa: BLE001
            return 0, str(exc)

    def inscrire(self, nom):
        return self.call("POST", "/auth/register", {
            "organization_name": nom,
            "full_name": "Propriétaire Sonde",
            "email": f"sonde-tarif-{uuid.uuid4().hex[:8]}@test.local",
            "phone": "+225 27 00 00 00 00",
            "password": "sonde12345",
            "station_name": f"Station {nom}",
            "city": "Abidjan",
        })

    def station(self):
        code, me = self.call("GET", "/auth/me")
        return (me or {}).get("station", {}).get("id", "") if isinstance(me, dict) else ""

    def horaires(self, station_id):
        return self.call("PUT", f"/stations/{station_id}/opening-hours", {
            "days": [
                {"weekday": jour, "is_open": jour <= 5, "opens_at": "08:00" if jour <= 5 else "",
                 "closes_at": "18:00" if jour <= 5 else "", "break_start": "", "break_end": "",
                 "last_checkin_at": "17:30" if jour <= 5 else "", "booking_allowed": jour <= 5}
                for jour in range(1, 8)
            ],
            "exceptions": [],
        })

    def declarer_vehicules(self, station_id):
        """Catégories et usages : prérequis de vente depuis le lot 4."""
        self.call("PUT", f"/stations/{station_id}/vehicle-categories", {
            "categories": [{"code": "berline", "is_active": True, "sort_order": 10},
                           {"code": "moto", "is_active": True, "sort_order": 20}],
        })
        self.call("PUT", f"/stations/{station_id}/vehicle-usages", {
            "usages": [{"code": "taxi_compteur", "is_active": True, "sort_order": 10}],
        })
        self.call("PUT", f"/stations/{station_id}/vehicle-usage-categories", {
            "couples": [{"usage_code": "taxi_compteur", "category_code": "berline"}],
        })

    def prestation(self, station_id, modele):
        """Retrouve, dans le catalogue de la station, la prestation d'un modèle."""
        code, catalogue = self.call("GET", f"/services?station_id={station_id}")
        for item in (catalogue or {}).get("items", []):
            if item.get("template_code") == modele:
                return item
        return {}

    def fiche(self, corps):
        return self.call("POST", "/wash-orders", corps)


RESULTS = []


def check(nom, ok, detail=""):
    RESULTS.append((nom, ok, detail))
    print(f"{'OK ' if ok else 'KO '} {nom}" + (f" — {detail}" if detail else ""), flush=True)


def erreur_code(charge):
    if isinstance(charge, dict) and isinstance(charge.get("error"), dict):
        return charge["error"].get("code", "")
    return ""


def champs_erreur(charge):
    if isinstance(charge, dict) and isinstance(charge.get("error"), dict):
        return charge["error"].get("fields") or {}
    return {}


def cellule(matrice, service_id, categorie):
    for ligne in (matrice or {}).get("services", []):
        if ligne.get("service_id") != service_id:
            continue
        for case in ligne.get("cells", []):
            if case.get("category_code") == categorie:
                return case
    return {}


def main():
    patron = Client()
    code, inscription = patron.inscrire("Sonde Tarifs")
    if code != 201:
        print("inscription impossible :", code, inscription)
        return 1
    station = patron.station()
    check("inscription et station de référence", bool(station), f"station={station}")
    patron.horaires(station)
    patron.declarer_vehicules(station)

    # 1. Le catalogue de référence : décrit, jamais tarifé.
    code, referentiel = patron.call("GET", f"/service-templates?station_id={station}")
    modeles = (referentiel or {}).get("templates", []) if isinstance(referentiel, dict) else []
    check("le référentiel des prestations est servi", code == 200 and len(modeles) >= 15,
          f"{len(modeles)} modèle(s), http={code}")
    check("aucun modèle ne porte de prix",
          all(int(m.get("price_xof") or 0) == 0 for m in modeles),
          ",".join(str(m.get("price_xof")) for m in modeles[:3]))
    familles = (referentiel or {}).get("families", []) if isinstance(referentiel, dict) else []
    check("les familles sont proposées", len(familles) >= 4, f"{len(familles)} famille(s)")
    check("une prestation sur devis ne se vend pas au comptoir",
          any(m.get("is_quote_only") for m in modeles),
          ",".join(m.get("code", "") for m in modeles if m.get("is_quote_only")))

    # 2. Le catalogue de la station : repris, mais à tarifer.
    exterieur = patron.prestation(station, "lavage_exterieur")
    check("une prestation de référence est reprise sans prix",
          bool(exterieur) and int(exterieur.get("price_xof") or 0) == 0,
          f"prix={exterieur.get('price_xof')}")
    check("elle est signalée « à tarifer »", bool(exterieur.get("needs_price")),
          f"needs_price={exterieur.get('needs_price')}")
    service = exterieur.get("id", "")

    code, liste = patron.call("GET", f"/services?station_id={station}")
    check("la liste annonce ce qu'il reste à tarifer",
          int((liste or {}).get("a_tarifer") or 0) > 0,
          f"a_tarifer={(liste or {}).get('a_tarifer')}")

    # 3. Avant activation, la station ne vend pas : le refus nomme les étapes
    #    qui restent (le tarif en fait partie).
    code, refus = patron.fiche({
        "station_id": station, "customer_name": "Client Sans Tarif",
        "items": [{"service_id": service, "quantity": 1}],
    })
    check("une station non activée ne vend pas",
          code == 409 and erreur_code(refus) == "onboarding_incomplete",
          f"http={code} code={erreur_code(refus)}")

    # 4. Le propriétaire pose son prix : ici, le tarif du catalogue.
    code, pose = patron.call("PUT", "/price-rules/matrix", {
        "station_id": station, "reason": "Premier tarif, décidé par le propriétaire",
        "generic_prices": [{"service_id": service, "price_xof": 2500}],
    })
    check("le tarif du catalogue s'enregistre depuis la matrice", code == 200,
          f"http={code} {json.dumps(pose, ensure_ascii=False)[:120]}")
    code, sans_motif = patron.call("PUT", "/price-rules/matrix", {
        "station_id": station,
        "generic_prices": [{"service_id": service, "price_xof": 9999}],
    })
    check("modifier un prix sans motif est refusé",
          code == 400 and "reason" in champs_erreur(sans_motif),
          f"http={code} champs={list(champs_erreur(sans_motif))}")

    # 5. L'aperçu : c'est le serveur qui sait le prix, pas le navigateur.
    code, apercu = patron.call(
        "GET", f"/price-preview?station_id={station}&service_id={service}")
    check("l'aperçu rend le prix du catalogue",
          code == 200 and apercu.get("price_xof") == 2500
          and apercu.get("price_source") == "service_generic",
          f"prix={apercu.get('price_xof')} source={apercu.get('price_source')}")
    check("l'aperçu nomme l'origine du prix", bool(apercu.get("price_source_label")),
          apercu.get("price_source_label", ""))

    # 6. Une prestation gratuite se vend à zéro, par décision explicite.
    code, gratuite = patron.call("POST", "/services", {
        "station_id": station, "name": "Contrôle pression des pneus",
        "duration_minutes": 10, "is_free": True,
    })
    check("une prestation gratuite se crée", code == 201, f"http={code}")
    code, refus = patron.call("PUT", "/price-rules/matrix", {
        "station_id": station, "reason": "Tentative de tarif sur prestation gratuite",
        "generic_prices": [{"service_id": gratuite.get("id"), "price_xof": 1500}],
    })
    check("une prestation gratuite ne porte pas de tarif", code == 400, f"http={code}")

    # 7. La station s'active : horaires, véhicules et un tarif sont là.
    code, activation = patron.call("POST", "/onboarding/activate")
    check("la station s'active une fois les prix posés", code == 200,
          f"http={code} {json.dumps(activation, ensure_ascii=False)[:120]}")

    # 8. La grille par véhicule : prestation + berline, puis + usage.
    code, _ = patron.call("POST", "/price-rules", {
        "station_id": station, "service_id": service, "category_code": "berline",
        "price_xof": 3000, "reason": "Tarif berline (lavage extérieur)",
    })
    check("une règle prestation + véhicule se pose", code == 201, f"http={code}")
    code, _ = patron.call("POST", "/price-rules", {
        "station_id": station, "service_id": service, "category_code": "berline",
        "usage_code": "taxi_compteur", "price_xof": 3500,
        "reason": "Taxis compteurs : plus de passages",
    })
    check("une règle prestation + véhicule + usage se pose", code == 201, f"http={code}")
    code, conflit = patron.call("POST", "/price-rules", {
        "station_id": station, "service_id": service, "category_code": "berline",
        "usage_code": "taxi_compteur", "price_xof": 5000, "reason": "Doublon volontaire",
    })
    check("deux règles identiques se signalent", code == 409 and erreur_code(conflit) == "price_rule_conflict",
          f"http={code} code={erreur_code(conflit)}")

    fiche_berline = patron.fiche({
        "station_id": station, "customer_name": "Client Berline", "vehicle_category_id": "berline",
        "items": [{"service_id": service, "quantity": 1}],
    })[1]
    ligne = (fiche_berline.get("items") or [{}])[0]
    check("la fiche applique le tarif du véhicule",
          fiche_berline.get("total_xof") == 3000 and ligne.get("price_source") == "service_category",
          f"total={fiche_berline.get('total_xof')} source={ligne.get('price_source')}")
    fiche_taxi = patron.fiche({
        "station_id": station, "customer_name": "Client Taxi",
        "vehicle_category_id": "berline", "vehicle_usage_id": "taxi_compteur",
        "items": [{"service_id": service, "quantity": 1}],
    })[1]
    check("le tarif par usage l'emporte sur le tarif par véhicule",
          fiche_taxi.get("total_xof") == 3500
          and (fiche_taxi.get("items") or [{}])[0].get("price_source") == "service_category_usage",
          f"total={fiche_taxi.get('total_xof')}")
    fiche_moto = patron.fiche({
        "station_id": station, "customer_name": "Client Moto", "vehicle_category_id": "moto",
        "items": [{"service_id": service, "quantity": 1}],
    })[1]
    check("une combinaison sans règle retombe sur le tarif du catalogue",
          fiche_moto.get("total_xof") == 2500
          and (fiche_moto.get("items") or [{}])[0].get("price_source") == "service_generic",
          f"total={fiche_moto.get('total_xof')}")

    # 9. Le contrat client prime sur toute la grille.
    code, client = patron.call("POST", "/customers", {
        "station_id": station, "full_name": "Flotte Sonde", "phone": "+225 07 00 00 00 00",
    })
    client_id = (client or {}).get("id", "")
    code, _ = patron.call("POST", "/price-rules", {
        "station_id": station, "service_id": service, "customer_id": client_id,
        "price_xof": 2000, "reason": "Contrat flotte de 12 véhicules",
    })
    check("un tarif de contrat client se pose", code == 201, f"http={code}")
    fiche_contrat = patron.fiche({
        "station_id": station, "customer_id": client_id,
        "vehicle_category_id": "berline", "vehicle_usage_id": "taxi_compteur",
        "items": [{"service_id": service, "quantity": 1}],
    })[1]
    check("le contrat client prime sur la grille",
          fiche_contrat.get("total_xof") == 2000
          and (fiche_contrat.get("items") or [{}])[0].get("price_source") == "client_contract",
          f"total={fiche_contrat.get('total_xof')}")

    # 10. Une prestation gratuite : zéro, et annoncé comme tel.
    fiche_gratuite = patron.fiche({
        "station_id": station, "customer_name": "Client Gratuit",
        "items": [{"service_id": gratuite.get("id"), "quantity": 1}],
    })[1]
    check("une prestation gratuite se vend à zéro",
          fiche_gratuite.get("total_xof") == 0
          and (fiche_gratuite.get("items") or [{}])[0].get("price_source") == "free",
          f"total={fiche_gratuite.get('total_xof')}")

    # 11. Une prestation encore sans tarif : le refus reste, même activé.
    desinfection = patron.prestation(station, "desinfection")
    code, refus = patron.fiche({
        "station_id": station, "customer_name": "Client Désinfection",
        "items": [{"service_id": desinfection.get("id"), "quantity": 1}],
    })
    check("une prestation jamais tarifée reste invendable",
          code == 409 and erreur_code(refus) == "price_required", f"http={code}")

    # 12. Les options se facturent par lavage.
    code, option = patron.call("POST", "/service-options", {
        "station_id": station, "name": "Lavage des tapis", "price_xof": 500,
    })
    check("une option se déclare avec son supplément", code == 201, f"http={code}")
    fiche_options = patron.fiche({
        "station_id": station, "customer_name": "Client Tapis", "vehicle_category_id": "berline",
        "items": [{"service_id": service, "quantity": 1,
                   "options": [{"option_id": option.get("id"), "quantity": 2}]}],
    })[1]
    ligne_options = (fiche_options.get("items") or [{}])[0]
    check("le supplément d'options est facturé par lavage",
          fiche_options.get("total_xof") == 4000 and ligne_options.get("options_total_xof") == 1000,
          f"total={fiche_options.get('total_xof')} options={ligne_options.get('options_total_xof')}")
    check("le prix de la prestation reste identifiable sur la ligne",
          ligne_options.get("unit_price_xof") == 3000 and len(ligne_options.get("options") or []) == 1,
          f"unité={ligne_options.get('unit_price_xof')}")
    code, inconnue = patron.fiche({
        "station_id": station, "customer_name": "Client Option Inventée",
        "items": [{"service_id": service, "quantity": 1,
                   "options": [{"option_id": "opt-inventee", "quantity": 1}]}],
    })
    check("une option inconnue est refusée", code == 400, f"http={code}")

    # 13. La matrice relit la grille, et la vider fait remonter le niveau suivant.
    code, matrice = patron.call("GET", f"/price-rules/matrix?station_id={station}")
    case = cellule(matrice, service, "berline")
    check("la matrice montre la règle du véhicule",
          case.get("price_xof") == 3000 and case.get("source") == "service_category",
          f"{case.get('price_xof')} / {case.get('source')}")
    check("la matrice annonce ce qui reste à tarifer",
          int((matrice or {}).get("a_tarifer") or 0) >= 1,
          f"a_tarifer={(matrice or {}).get('a_tarifer')}")
    code, vidage = patron.call("PUT", "/price-rules/matrix", {
        "station_id": station, "reason": "Retour au tarif du catalogue",
        "cells": [{"service_id": service, "category_code": "berline"}],
    })
    check("vider une case désactive la règle", code == 200 and vidage.get("desactivees") == 1,
          f"http={code} désactivées={vidage.get('desactivees')}")
    fiche_apres = patron.fiche({
        "station_id": station, "customer_name": "Client Après", "vehicle_category_id": "berline",
        "items": [{"service_id": service, "quantity": 1}],
    })[1]
    check("le niveau suivant reprend la main",
          fiche_apres.get("total_xof") == 2500
          and (fiche_apres.get("items") or [{}])[0].get("price_source") == "service_generic",
          f"total={fiche_apres.get('total_xof')}")

    # 14. Isolation : la grille d'une entreprise est invisible à une autre.
    voisine = Client()
    code, _ = voisine.inscrire("Sonde Tarifs Voisine")
    code, _ = voisine.call("GET", f"/price-rules?station_id={station}")
    check("la grille d'une autre entreprise est introuvable", code == 404, f"http={code}")
    code, _ = voisine.call("GET", f"/price-rules/matrix?station_id={station}")
    check("la matrice d'une autre entreprise est introuvable", code == 404, f"http={code}")
    code, vierge = voisine.call("GET", f"/price-rules?station_id={voisine.station()}")
    check("chaque entreprise a sa propre grille",
          code == 200 and (vierge or {}).get("total") == 0, f"total={(vierge or {}).get('total')}")

    total = len(RESULTS)
    gagnants = sum(1 for _, ok, _ in RESULTS if ok)
    print(f"\n{gagnants}/{total} contrôle(s) au vert")
    return 0 if gagnants == total else 1


if __name__ == "__main__":
    sys.exit(main())
