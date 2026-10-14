#!/usr/bin/env python3
"""Sonde d'acceptation — catégories de véhicules et usages commerciaux (lot 4).

    python3 scripts/verifications/sonde-vehicules.py [base_url]

Elle interroge une API **déjà démarrée** (par défaut http://127.0.0.1:8080) et
rejoue les corps de requête exacts des deux écrans de la mise en route
(« Catégories de véhicules », « Usages commerciaux »), puis relit les données
par les routes de consultation.

Ce qui est vérifié, et pourquoi :

* **Le référentiel est amorcé et rejouable.** Deux lectures séparées donnent le
  même nombre d'entrées : un amorçage rejoué ne duplique rien.
* **Une catégorie et un usage sont deux référentiels distincts.** Passer un
  usage comme catégorie est refusé : sinon une berline « taxi compteur »
  deviendrait une catégorie de plus, et la caisse ne saurait plus compter les
  berlines.
* **Désactiver ne détruit rien.** Une fiche passée garde le nom qu'elle
  portait, et l'écran de caisse ne propose plus la catégorie désactivée.
* **Isolation.** Une autre entreprise ne voit ni ne modifie ces réglages.
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
            "email": f"sonde-veh-{uuid.uuid4().hex[:8]}@test.local",
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
                {"weekday": jour, "is_open": True, "opens_at": "08:00", "closes_at": "18:00",
                 "break_start": "", "break_end": "", "last_checkin_at": "17:30",
                 "booking_allowed": True}
                for jour in range(1, 6)
            ] + [
                {"weekday": jour, "is_open": False, "opens_at": "", "closes_at": "",
                 "break_start": "", "break_end": "", "last_checkin_at": "",
                 "booking_allowed": False}
                for jour in (6, 7)
            ],
            "exceptions": [],
        })


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


def main():
    # 1. Une entreprise neuve : le référentiel est là, rien n'est activé pour
    #    elle. C'est l'état honnête d'une station qui vient d'ouvrir son espace.
    patron = Client()
    code, inscription = patron.inscrire("Sonde Véhicules")
    if code != 201:
        print("inscription impossible :", code, inscription)
        return 1
    station = patron.station()
    check("inscription et station de référence", bool(station), f"station={station}")

    code, catalogue = patron.call("GET", f"/vehicle-catalogue?station_id={station}")
    categories = (catalogue or {}).get("categories", []) if isinstance(catalogue, dict) else []
    usages = (catalogue or {}).get("usages", []) if isinstance(catalogue, dict) else []
    check("le référentiel des véhicules est amorcé", code == 200 and len(categories) >= 10,
          f"{len(categories)} catégorie(s), http={code}")
    check("le référentiel des usages est amorcé", len(usages) >= 15, f"{len(usages)} usage(s)")
    familles = (catalogue or {}).get("familles_categories", []) if isinstance(catalogue, dict) else []
    check("les catégories sont regroupées par famille physique",
          "Véhicules légers" in familles and "Poids lourds" in familles,
          ", ".join(familles[:3]) + (f" (+{len(familles) - 3})" if len(familles) > 3 else ""))

    # 2. Rien n'est activé tant que l'exploitant ne l'a pas décidé, et la
    #    relecture est stable (un amorçage rejoué ne duplique pas).
    code, avant = patron.call("GET", f"/vehicle-categories?station_id={station}")
    check("aucune catégorie n'est activée par défaut",
          code == 200 and (avant or {}).get("total") == 0, f"total={(avant or {}).get('total')}")
    code, apres = patron.call("GET", f"/vehicle-catalogue?station_id={station}")
    check("le catalogue reste identique à la seconde lecture",
          len((apres or {}).get("categories", [])) == len(categories),
          f"{len(categories)} puis {len((apres or {}).get('categories', []))}")

    # 3. L'écran « catégories » : activation en bloc, telle que le formulaire
    #    l'envoie (un tableau de couples code / actif / nom local / ordre).
    # « gbaka » est un **usage** (transport public), la catégorie correspondante
    # est « minibus » : c'est exactement la distinction que cette sonde vérifie.
    a_activer = [c["code"] for c in categories if c["code"] in
                 ("citadine", "berline", "suv", "moto", "minibus", "camion")]
    corps = {"categories": [
        {"code": c["code"], "is_active": c["code"] in a_activer,
         "name": "Voiture de ville" if c["code"] == "berline" else "",
         "sort_order": (i + 1) * 10}
        for i, c in enumerate(categories)
    ]}
    code, enregistre = patron.call("PUT", f"/stations/{station}/vehicle-categories", corps)
    check("activation des catégories acceptée", code == 200, f"http={code}")
    check("le serveur renvoie le nombre de catégories actives",
          (enregistre or {}).get("actives") == len(a_activer),
          f"actives={(enregistre or {}).get('actives')} attendu={len(a_activer)}")

    code, listees = patron.call("GET", f"/vehicle-categories?station_id={station}&actives=1")
    actives = (listees or {}).get("items", [])
    check("la caisse ne reçoit que les catégories activées",
          len(actives) == len(a_activer), f"{len(actives)} renvoyée(s)")
    berline = next((c for c in actives if c["code"] == "berline"), {})
    check("un nom local remplace le nom du référentiel",
          berline.get("name") == "Voiture de ville" and berline.get("name_from_template") is False,
          berline.get("name", ""))
    autre = next((c for c in actives if c["code"] == "suv"), {})
    check("sans renommage, le nom du référentiel est conservé",
          autre.get("name") == "SUV / 4x4" and autre.get("name_from_template") is True,
          autre.get("name", ""))

    # 4. Une catégorie propre à la station (hors référentiel).
    code, cree = patron.call("POST", "/vehicle-categories", {
        "station_id": station, "name": "Tricycle de livraison",
        "family": "Deux-roues et assimilés", "description": "Tricycle motorisé de livraison.",
    })
    check("création d'une catégorie propre à la station", code == 200, f"http={code}")
    perso = next((c for c in (cree or {}).get("items", [])
                  if c.get("name") == "Tricycle de livraison"), {})
    check("la catégorie créée porte un code local et reste distincte du référentiel",
          perso.get("kind") == "personnalisee" and str(perso.get("code", "")).startswith("perso_"),
          perso.get("code", ""))

    # Renommage d'une entrée du référentiel : c'est le libellé local qui change,
    # jamais le référentiel commun.
    code, _ = patron.call("PATCH", f"/vehicle-categories/{perso.get('code')}", {
        "station_id": station, "name": "Tricycle (livraison)", "is_active": True,
    })
    code, relu = patron.call("GET", f"/vehicle-categories?station_id={station}&actives=1")
    renomme = next((c for c in (relu or {}).get("items", []) if c.get("code") == perso.get("code")), {})
    check("le renommage est relu", renomme.get("name") == "Tricycle (livraison)",
          renomme.get("name", ""))

    # 4 bis. Ré-enregistrer la liste **sans** nom ne doit pas effacer un
    # renommage local : l'écran d'activation coche des entrées, il ne réécrit
    # pas les libellés que la station a choisis.
    code, _ = patron.call("PUT", f"/stations/{station}/vehicle-categories", {
        "categories": [{"code": c["code"], "is_active": c["code"] in a_activer,
                        "sort_order": i + 1}
                       for i, c in enumerate(categories)],
    })
    code, relu = patron.call("GET", f"/vehicle-categories?station_id={station}&actives=1")
    inchange = next((c for c in (relu or {}).get("items", []) if c.get("code") == "berline"), {})
    check("un ré-enregistrement sans nom conserve le renommage local",
          inchange.get("name") == "Voiture de ville", inchange.get("name", ""))

    # Rétablir le libellé du référentiel : nom vide → le modèle reprend la main.
    code, _ = patron.call("PATCH", "/vehicle-categories/berline",
                          {"station_id": station, "name": ""})
    code, relu = patron.call("GET", f"/vehicle-categories?station_id={station}&actives=1")
    retablie = next((c for c in (relu or {}).get("items", []) if c.get("code") == "berline"), {})
    check("un nom vide rétablit le libellé du référentiel",
          retablie.get("name") == "Berline" and retablie.get("name_from_template") is True,
          f"{retablie.get('name')} (référentiel={retablie.get('name_from_template')})")
    # On remet le libellé local pour la suite : la fiche doit porter ce nom-là.
    patron.call("PATCH", "/vehicle-categories/berline",
                {"station_id": station, "name": "Voiture de ville"})

    # 5. L'écran « usages » : activation puis combinaisons véhicule × usage.
    codes_usages = [u["code"] for u in usages if u["code"] in
                    ("personnel", "taxi_compteur", "livraison", "gbaka",
                     "taxi_communal", "moto_taxi", "flotte")]
    code, usages_actives = patron.call("PUT", f"/stations/{station}/vehicle-usages", {
        "usages": [{"code": u["code"], "is_active": u["code"] in codes_usages,
                   "sort_order": (i + 1) * 10}
                   for i, u in enumerate(usages)],
    })
    check("activation des usages acceptée", code == 200, f"http={code}")
    check("le serveur renvoie le nombre d'usages actifs",
          (usages_actives or {}).get("actifs") == len(codes_usages),
          f"actifs={(usages_actives or {}).get('actifs')} attendu={len(codes_usages)}")

    couples = [
        {"usage_code": "taxi_compteur", "category_code": "berline"},
        {"usage_code": "taxi_compteur", "category_code": "citadine"},
        {"usage_code": "taxi_communal", "category_code": "berline"},
        {"usage_code": "livraison", "category_code": "suv"},
        {"usage_code": "moto_taxi", "category_code": "moto"},
        {"usage_code": "gbaka", "category_code": "minibus"},
        # La catégorie personnalisée : le couple doit être accepté comme les autres.
        {"usage_code": "livraison", "category_code": perso.get("code")},
    ]
    code, enregistres = patron.call("PUT", f"/stations/{station}/vehicle-usage-categories",
                                    {"couples": couples})
    check("les combinaisons véhicule × usage sont enregistrées",
          code == 200 and (enregistres or {}).get("total") == len(couples),
          f"http={code} total={(enregistres or {}).get('total')}")

    code, fusion = patron.call("GET", f"/vehicle-category-options?station_id={station}")
    options = fusion or {}
    check("la caisse reçoit catégories **et** usages en un appel",
          code == 200 and len(options.get("categories", [])) == len(a_activer) + 1
          and len(options.get("usages", [])) == len(codes_usages),
          f"{len(options.get('categories', []))} catégorie(s), {len(options.get('usages', []))} usage(s)")

    # Un couple qui cite une catégorie non activée est refusé, avec la raison.
    code, refus = patron.call("PUT", f"/stations/{station}/vehicle-usage-categories",
                              {"couples": [{"usage_code": "taxi_compteur",
                                            "category_code": "camion_remorque"}]})
    check("un couple avec une catégorie non activée est refusé",
          code == 400 and erreur_code(refus) == "validation_error" and bool(champs_erreur(refus)),
          f"http={code} champs={list(champs_erreur(refus).keys())}")

    # 6. Une fiche de lavage : catégorie **et** usage, deux libellés conservés.
    patron.horaires(station)
    # La prestation se compose ici, se tarife dans la grille (motif compris).
    code, presta = patron.call("POST", "/services", {
        "station_id": station, "name": "Lavage complet",
        "duration_minutes": 45,
    })
    if isinstance(presta, dict) and presta.get("id"):
        patron.call("PUT", "/price-rules/matrix", {
            "station_id": station, "reason": "Grille d'acceptation",
            "generic_prices": [{"service_id": presta["id"], "price_xof": 5000}],
        })
    code, _ = patron.call("PUT", "/organizations/current", {"phone": "+225 27 00 00 00 00", "city": "Abidjan"})
    code, activation = patron.call("POST", "/onboarding/activate")
    check("activation de la station une fois le parcours rempli", code == 200,
          f"http={code} manquants={list(champs_erreur(activation).keys())}")

    code, prestations = patron.call("GET", f"/services?station_id={station}")
    service_id = ((presta or {}).get("id") or ((prestations or {}).get("items") or [{}])[0].get("id", ""))
    code, fiche = patron.call("POST", "/wash-orders", {
        "station_id": station, "customer_name": "Client Taxi", "plate": "1234 AB 01",
        "vehicle_category_id": "berline", "vehicle_usage_id": "taxi_compteur",
        "items": [{"service_id": service_id, "quantity": 1}],
    })
    commande = (fiche or {}).get("order", fiche) if isinstance(fiche, dict) else {}
    check("la fiche accepte une catégorie **et** un usage", code == 201,
          f"http={code}")
    check("la fiche conserve les deux libellés",
          commande.get("vehicle_category_name") == "Voiture de ville"
          and commande.get("vehicle_usage_name") == "Taxi compteur",
          f"catégorie={commande.get('vehicle_category_name')} usage={commande.get('vehicle_usage_name')}")

    # Un usage passé comme catégorie est refusé : ce sont deux référentiels.
    code, melange = patron.call("POST", "/wash-orders", {
        "station_id": station, "vehicle_category_id": "taxi_compteur",
        "items": [{"service_id": service_id, "quantity": 1}],
    })
    check("un usage n'est pas une catégorie",
          code == 400 and erreur_code(melange) == "validation_error",
          f"http={code} code={erreur_code(melange)}")

    # Un couple non déclaré est refusé aussi (Moto × Taxi compteur).
    code, couple_refuse = patron.call("POST", "/wash-orders", {
        "station_id": station, "vehicle_category_id": "moto", "vehicle_usage_id": "taxi_compteur",
        "items": [{"service_id": service_id, "quantity": 1}],
    })
    check("un couple non déclaré est refusé",
          code == 400 and erreur_code(couple_refuse) == "validation_error",
          f"http={code} code={erreur_code(couple_refuse)}")

    # 7. Désactivation : la fiche passée reste lisible, la caisse ne propose
    #    plus la catégorie.
    code, _ = patron.call("PUT", f"/stations/{station}/vehicle-categories", {
        "categories": [{"code": c["code"], "is_active": c["code"] != "berline",
                        "name": "", "sort_order": (i + 1) * 10}
                       for i, c in enumerate(categories)],
    })
    check("désactivation d'une catégorie acceptée", code == 200, f"http={code}")

    code, relue = patron.call("GET", f"/wash-orders/{commande.get('id')}")
    ancienne = (relue or {}).get("order", {}) if isinstance(relue, dict) else {}
    check("une fiche passée garde le nom de sa catégorie désactivée",
          code == 200 and ancienne.get("vehicle_category_name") == "Voiture de ville"
          and ancienne.get("vehicle_usage_name") == "Taxi compteur",
          f"catégorie={ancienne.get('vehicle_category_name')}")

    code, proposees = patron.call("GET", f"/vehicle-category-options?station_id={station}")
    ids = [c.get("id") for c in (proposees or {}).get("categories", [])]
    check("la catégorie désactivée n'est plus proposée à la caisse",
          "berline" not in ids, f"{len(ids)} catégorie(s) proposée(s)")

    # 8. Isolation : une autre entreprise ne voit ni ne modifie ces réglages.
    concurrent = Client()
    code, _ = concurrent.inscrire("Sonde Véhicules Concurrente")
    station_concurrente = concurrent.station()
    code, ses_categories = concurrent.call("GET", f"/vehicle-categories?station_id={station}")
    check("les catégories d'une autre entreprise sont refusées",
          code in (403, 404), f"http={code}")
    code, _ = concurrent.call("PUT", f"/stations/{station}/vehicle-categories",
                              {"categories": [{"code": "berline", "is_active": True}]})
    check("activer les catégories d'une autre entreprise est refusé",
          code in (403, 404), f"http={code}")
    code, ses_propres = concurrent.call("GET", f"/vehicle-categories?station_id={station_concurrente}&actives=1")
    check("chaque entreprise a bien ses propres réglages",
          code == 200 and (ses_propres or {}).get("total") == 0,
          f"total={(ses_propres or {}).get('total')}")

    # 9. L'étape de mise en route dit la vérité : sans catégorie, on voit qu'il
    #    en manque — la station concurrente n'en a activé aucune.
    code, onboard = concurrent.call("GET", "/onboarding")
    etapes = {e.get("code"): e for e in (onboard or {}).get("steps", [])}
    check("l'étape « Catégories de véhicules » est présente et obligatoire",
          etapes.get("categories", {}).get("required") is True
          and etapes.get("categories", {}).get("done") is False,
          f"done={etapes.get('categories', {}).get('done')}")
    check("l'étape « Usages commerciaux » est présente et facultative",
          etapes.get("usages", {}).get("required") is False,
          f"required={etapes.get('usages', {}).get('required')}")
    check("les formulaires des deux étapes sont annoncés",
          etapes.get("categories", {}).get("form") == "vehicules"
          and etapes.get("usages", {}).get("form") == "usages",
          f"{etapes.get('categories', {}).get('form')} / {etapes.get('usages', {}).get('form')}")
    manquants = [m.get("code") for m in (onboard or {}).get("missing", [])]
    check("l'activation est bloquée sans catégorie active",
          (onboard or {}).get("can_activate") is False and "categories" in manquants,
          f"manquants={manquants}")

    total = len(RESULTS)
    gagnants = sum(1 for _, ok, _ in RESULTS if ok)
    print(f"\n{gagnants}/{total} contrôle(s) au vert")
    return 0 if gagnants == total else 1


if __name__ == "__main__":
    sys.exit(main())
