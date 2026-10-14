#!/usr/bin/env python3
"""Sonde d'acceptation — les formulaires de la mise en route écrivent en base.

Elle interroge une API **déjà démarrée** (par défaut http://127.0.0.1:8080) :
aucune dépendance, uniquement la bibliothèque standard.

    python3 scripts/verifications/sonde-onboarding.py [base_url]

Elle rejoue exactement les corps de requête que construisent les quatre
formulaires du parcours de mise en route (entreprise, station, horaires,
prestations) et relit ensuite les données par les routes de consultation : ce
qu'un écran enregistre doit être ce qu'un autre écran relit. Autrement dit elle
vérifie la chaîne complète *formulaire → base → relecture*, pas seulement le
code de retour.

Contrôles : enregistrement et relecture de l'identité commerciale, de la
facturation et des mentions administratives ; distinction « non renseigné » /
« non » ; horaires validés jour par jour ; tarifs saisis par l'exploitant ;
cloisonnement entre entreprises (404) ; activation refusée tant qu'un prérequis
manque, acceptée ensuite.
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
            "email": f"sonde-onb-{uuid.uuid4().hex[:8]}@test.local",
            "phone": "+225 27 00 00 00 00",
            "password": "sonde12345",
            "station_name": f"Station {nom}",
            "city": "Abidjan",
        })


RESULTS = []


def check(nom, ok, detail=""):
    RESULTS.append((nom, ok, detail))
    print(f"{'OK ' if ok else 'KO '} {nom}" + (f" — {detail}" if detail else ""), flush=True)


def erreur_code(charge):
    """Le code applicatif d'une réponse d'erreur (contrat du front)."""
    if isinstance(charge, dict) and isinstance(charge.get("error"), dict):
        return charge["error"].get("code", "")
    return ""


def champs_erreur(charge):
    """Les champs fautifs : c'est ce que le front affiche au bon endroit."""
    if isinstance(charge, dict) and isinstance(charge.get("error"), dict):
        return charge["error"].get("fields") or {}
    return {}


# ---------------------------------------------------------------------------
# Le corps exact que construit le formulaire « entreprise ».
# ---------------------------------------------------------------------------
CORPS_ENTREPRISE = {
    "trade_name": "Lavage du Plateau",
    "legal_name": "LAVAGE DU PLATEAU SARL U",
    "activity_type": "lavage manuel",
    "activity_description": "Lavage à la main, nettoyage intérieur, polissage.",
    "activity_started_on": "2019-03-15",
    "whatsapp": "+225 07 07 07 07 07",
    "website": "https://lavageduplateau.example",
    "billing_name": "LAVAGE DU PLATEAU SARL U",
    "billing_address": "Rue des Jardins, Plateau",
    "billing_email": "facturation@lavageduplateau.example",
    "billing_phone": "+225 27 20 20 20 20",
    "legal_mentions": "SARL U au capital de 1 000 000 FCFA",
    "registry": {
        "rccm": "CI-ABJ-2019-B-12345",
        "tax_id": "1926547K",
        "cnps_employer_number": "",
        "legal_form": "SARL U",
        "legal_representative": "Awa Koné",
        "fiscal_regime": "",
    },
}

# ---------------------------------------------------------------------------
# Le corps exact que construit le formulaire « station » : capacités à trois
# états (oui / non / non renseigné) et localisation.
# ---------------------------------------------------------------------------
CORPS_STATION = {
    "name": "Lavage du Plateau",
    "code": "LDP",
    "phone": "+225 27 21 21 21 21",
    "city": "Abidjan",
    "country": "Côte d'Ivoire",
    "commune": "Cocody",
    "district_area": "Angré 7e tranche",
    "street": "Boulevard Latrille",
    "landmark": "En face de la pharmacie du carrefour",
    "latitude": "5.3489",
    "longitude": "-3.9861",
    "billing_address": "",
    "site": {
        "wash_types": ["lavage manuel", "nettoyage intérieur"],
        "bays": 3,
        "simultaneous_vehicles": 2,
        "has_vacuum": True,
        "has_drying": False,
        # Volontairement « non renseigné » : la relecture doit rendre None, pas
        # False. Confondre les deux ferait afficher « pas de detailing » à une
        # station qui n'a simplement pas répondu.
        "has_detailing": None,
        "has_waiting_room": True,
        "has_storage": None,
        "has_generator": False,
        "water_source": "raccordement + forage",
        "water_reserve_liters": 2000,
        "power_supply": "réseau CIE",
        "notes": "Piste couverte, accès poids lourds impossible.",
        "constraints": "Coupures fréquentes en saison des pluies.",
    },
}

# ---------------------------------------------------------------------------
# Le corps exact que construit le formulaire « horaires ».
# ---------------------------------------------------------------------------
JOURS = {
    1: ("08:00", "18:00", "12:00", "13:30"),
    2: ("08:00", "18:00", "12:00", "13:30"),
    3: ("08:00", "18:00", "12:00", "13:30"),
    4: ("08:00", "18:00", "12:00", "13:30"),
    5: ("08:00", "18:00", "12:00", "13:30"),
    6: ("08:00", "14:00", "", ""),
    7: ("", "", "", ""),
}


def corps_horaires():
    days = []
    for weekday in range(1, 8):
        ouvre, ferme, pause_debut, pause_fin = JOURS[weekday]
        days.append({
            "weekday": weekday,
            "is_open": ouvre != "",
            "opens_at": ouvre,
            "closes_at": ferme,
            "break_start": pause_debut,
            "break_end": pause_fin,
            # La dernière réception doit tomber dans les heures d'ouverture du
            # jour : le serveur refuserait 17:30 pour un samedi qui ferme à 14:00.
            "last_checkin_at": "17:30" if ferme == "18:00" else ("13:00" if ferme else ""),
            "booking_allowed": True,
        })
    return {
        "days": days,
        "exceptions": [{
            "day": "2026-12-25",
            "is_open": False,
            "opens_at": "",
            "closes_at": "",
            "label": "Noël",
        }],
    }


def main():
    # 1. Une entreprise neuve : c'est le point de départ du parcours.
    proprietaire = Client()
    code, inscription = proprietaire.inscrire("Sonde Formulaires")
    check("inscription d'une entreprise neuve", code == 201 and bool(proprietaire.cookie),
          f"http={code}")

    code, me = proprietaire.call("GET", "/auth/me")
    station_id = (me or {}).get("station", {}).get("id", "") if isinstance(me, dict) else ""
    check("la station de référence est connue", code == 200 and bool(station_id), f"http={code}")

    code, onboard = proprietaire.call("GET", "/onboarding")
    etapes = {e.get("code"): e for e in (onboard or {}).get("steps", [])}
    formulaires = {c: e.get("form") for c, e in etapes.items() if e.get("form")}
    check("les étapes exposent le formulaire à afficher",
          formulaires == {"entreprise": "entreprise", "station": "station",
                          "horaires": "horaires", "categories": "vehicules",
                          "usages": "usages", "prestations": "prestations",
                          "options": "options", "tarifs": "tarifs",
                          "moyens": "moyens"},
          json.dumps(formulaires, ensure_ascii=False))
    check("le parcours compte dix étapes, dans l'ordre",
          [e.get("code") for e in (onboard or {}).get("steps", [])] ==
          ["espace", "entreprise", "station", "horaires", "categories", "usages",
           "prestations", "options", "tarifs", "moyens"],
          str([e.get("code") for e in (onboard or {}).get("steps", [])]))
    check("activation impossible avant remplissage",
          onboard.get("can_activate") is False and onboard.get("state") != "active",
          f"can_activate={onboard.get('can_activate')}")
    check("l'étape station signale ce qui reste facultatif",
          "facultatif" in (etapes.get("station", {}).get("detail", "") or "").lower()
          or etapes.get("station", {}).get("detail", "").startswith("Manque"),
          etapes.get("station", {}).get("detail", ""))

    # 2. Formulaire « entreprise » : enregistrement puis relecture.
    code, _ = proprietaire.call("PATCH", "/organizations/current", CORPS_ENTREPRISE)
    check("PATCH /organizations/current accepte le corps du formulaire", code == 200, f"http={code}")

    code, fiche = proprietaire.call("GET", "/organizations/current")
    org = (fiche or {}).get("organization", {}) if isinstance(fiche, dict) else {}
    attendus = {cle: CORPS_ENTREPRISE[cle] for cle in
                ("trade_name", "legal_name", "activity_type", "activity_started_on",
                 "whatsapp", "billing_email", "billing_phone", "legal_mentions")}
    ecarts = {cle: (org.get(cle), valeur) for cle, valeur in attendus.items() if org.get(cle) != valeur}
    check("la fiche d'entreprise relue est celle enregistrée", not ecarts,
          json.dumps(ecarts, ensure_ascii=False)[:200])

    registre = {f.get("field_code"): f for f in (fiche or {}).get("registry", [])}
    check("le registre administratif est renvoyé avec ses états",
          len(registre) >= 6 and registre.get("rccm", {}).get("status") == "provided"
          and registre.get("rccm", {}).get("value") == CORPS_ENTREPRISE["registry"]["rccm"],
          f"{len(registre)} mention(s)")
    check("une mention vide reste « Non renseigné »",
          registre.get("cnps_employer_number", {}).get("status") == "empty"
          and registre.get("cnps_employer_number", {}).get("status_label") == "Non renseigné",
          registre.get("cnps_employer_number", {}).get("status_label", ""))

    # 3. Un corps invalide est refusé champ par champ — le front s'en sert pour
    #    placer ses messages sous les bons champs.
    code, refus = proprietaire.call("PATCH", "/organizations/current",
                                    {"billing_email": "pas-un-email", "activity_started_on": "15/03/2020"})
    champs = champs_erreur(refus)
    check("un e-mail et une date invalides sont refusés champ par champ",
          code == 400 and erreur_code(refus) == "validation_error"
          and {"billing_email", "activity_started_on"} <= set(champs),
          f"http={code} champs={sorted(champs)}")

    # 4. Formulaire « station » : localisation et capacités.
    code, profil = proprietaire.call("GET", f"/stations/{station_id}/profile")
    jours = (profil or {}).get("days", []) if isinstance(profil, dict) else []
    check("la fiche station renvoie les sept jours",
          code == 200 and len(jours) == 7 and [j.get("weekday") for j in jours] == list(range(1, 8)),
          f"http={code} jours={len(jours)}")

    code, _ = proprietaire.call("PATCH", f"/stations/{station_id}", CORPS_STATION)
    check("PATCH /stations/{id} accepte localisation et capacités", code == 200, f"http={code}")

    code, profil = proprietaire.call("GET", f"/stations/{station_id}/profile")
    station = (profil or {}).get("station", {}) if isinstance(profil, dict) else {}
    site = (profil or {}).get("site", {}) if isinstance(profil, dict) else {}
    check("la localisation relue est celle enregistrée",
          station.get("commune") == "Cocody" and station.get("district_area") == "Angré 7e tranche"
          and station.get("landmark") == CORPS_STATION["landmark"]
          and station.get("latitude", "").startswith("5.3489")
          and station.get("longitude", "").startswith("-3.9861"),
          f"lat={station.get('latitude')} lon={station.get('longitude')}")
    check("« non renseigné » reste distinct de « non »",
          site.get("has_vacuum") is True and site.get("has_drying") is False
          and site.get("has_detailing") is None and site.get("has_storage") is None
          and site.get("has_generator") is False,
          f"aspiration={site.get('has_vacuum')} séchage={site.get('has_drying')} detailing={site.get('has_detailing')}")
    check("les capacités déclarées sont relues",
          site.get("bays") == 3 and site.get("simultaneous_vehicles") == 2
          and site.get("wash_types") == CORPS_STATION["site"]["wash_types"]
          and site.get("water_reserve_liters") == 2000,
          f"pistes={site.get('bays')} types={site.get('wash_types')}")

    # 5. Formulaire « horaires » : accepté, puis refusé jour par jour.
    code, enregistres = proprietaire.call("PUT", f"/stations/{station_id}/opening-hours", corps_horaires())
    jours_rendus = (enregistres or {}).get("days", []) if isinstance(enregistres, dict) else []
    check("PUT /opening-hours accepte la semaine type",
          code == 200 and len(jours_rendus) == 7
          and sum(1 for j in jours_rendus if j.get("is_open")) == 6,
          f"http={code} ouverts={sum(1 for j in jours_rendus if j.get('is_open'))}")
    check("la journée particulière est enregistrée",
          len((enregistres or {}).get("exceptions", [])) == 1,
          str((enregistres or {}).get("exceptions", []))[:80])

    faux = corps_horaires()
    for jour in faux["days"]:
        if jour["weekday"] == 2:  # mardi : fermeture avant ouverture, pause dehors
            jour["closes_at"] = "07:00"
            jour["break_start"] = "19:00"
            jour["break_end"] = "19:30"
    code, refus = proprietaire.call("PUT", f"/stations/{station_id}/opening-hours", faux)
    champs = champs_erreur(refus)
    check("des horaires incohérents sont refusés sur le jour concerné",
          code == 400 and erreur_code(refus) == "validation_error" and "mardi" in champs,
          f"http={code} champs={sorted(champs)}")

    code, profil = proprietaire.call("GET", f"/stations/{station_id}/profile")
    jours_relus = {j.get("weekday"): j for j in (profil or {}).get("days", [])}
    check("la semaine enregistrée reste intacte après un refus",
          jours_relus.get(1, {}).get("opens_at") == "08:00"
          and jours_relus.get(1, {}).get("break_start") == "12:00"
          and jours_relus.get(7, {}).get("is_open") is False
          and jours_relus.get(1, {}).get("last_checkin_at") == "17:30",
          f"lundi={jours_relus.get(1, {}).get('opens_at')}-{jours_relus.get(1, {}).get('closes_at')}")

    code, onboard = proprietaire.call("GET", "/onboarding")
    etapes = {e.get("code"): e for e in (onboard or {}).get("steps", [])}
    check("l'étape horaires passe à terminée",
          etapes.get("horaires", {}).get("done") is True,
          etapes.get("horaires", {}).get("detail", ""))

    # 6. Formulaire « prestations » : la prestation se compose (elle naît « à
    # tarifer ») ; le prix se pose dans la grille tarifaire, avec son motif.
    code, prestation = proprietaire.call("POST", "/services", {
        "station_id": station_id,
        "name": "Lavage complet berline",
        "duration_minutes": 45,
    })
    service_id = (prestation or {}).get("id", "") if isinstance(prestation, dict) else ""
    check("la prestation est créée sans prix inventé",
          code == 201 and bool(service_id) and (prestation or {}).get("price_xof") == 0,
          f"http={code} prix={(prestation or {}).get('price_xof')}")

    code, _ = proprietaire.call("PUT", "/price-rules/matrix", {
        "station_id": station_id,
        "reason": "Grille d'ouverture de la station",
        "generic_prices": [{"service_id": service_id, "price_xof": 3000}],
    })
    code2, liste = proprietaire.call("GET", f"/services?station_id={station_id}")
    tarifs = {s.get("id"): s.get("price_xof") for s in (liste or {}).get("items", [])}
    check("le tarif posé dans la grille est celui relu",
          code == 200 and tarifs.get(service_id) == 3000,
          f"http={code} prix={tarifs.get(service_id)}")

    # 6 bis. Catégories de véhicules : prérequis depuis le lot 4 (on ne vend pas
    # sans dire ce qu'on lave). L'écran complet est couvert par
    # `sonde-vehicules.py` ; ici, une seule catégorie suffit pour activer.
    code, _ = proprietaire.call("PUT", f"/stations/{station_id}/vehicle-categories", {
        "categories": [{"code": "berline", "is_active": True, "sort_order": 10},
                       {"code": "moto", "is_active": True, "sort_order": 20}],
    })
    code, actives = proprietaire.call("GET", f"/vehicle-categories?station_id={station_id}&actives=1")
    check("les catégories de véhicules activées sont relues",
          code == 200 and (actives or {}).get("total") == 2,
          f"http={code} total={(actives or {}).get('total')}")

    # 6 ter. Moyens de paiement : la station naît avec le référentiel posé et
    # seules les espèces actives. Le moyen est un **id** : c'est lui qui décide
    # la famille comptable, l'exigence de référence et le compteur de caisse.
    code, moyens = proprietaire.call("GET", f"/payment-methods?station_id={station_id}")
    items = (moyens or {}).get("items", []) if isinstance(moyens, dict) else []
    actifs = [m for m in items if m.get("is_active")]
    check("la station naît avec son référentiel de moyens de paiement",
          code == 200 and len(items) == 9 and len(actifs) == 1 and actifs[0].get("code") == "especes",
          f"http={code} total={len(items)} actifs={[m.get('code') for m in actifs]}")

    orange = next((m for m in items if m.get("code") == "orange_money"), {})
    code, _ = proprietaire.call("PATCH", f"/payment-methods/{orange.get('id')}",
                                {"is_active": True, "name": "Orange Money CI"})
    code, relus = proprietaire.call("GET", f"/payment-methods?station_id={station_id}&actives=1")
    codes = [m.get("code") for m in (relus or {}).get("items", [])]
    nomme = next((m for m in (relus or {}).get("items", []) if m.get("code") == "orange_money"), {})
    check("un moyen activé et renommé est relu tel quel",
          code == 200 and set(codes) == {"especes", "orange_money"} and nomme.get("name") == "Orange Money CI",
          f"http={code} actifs={codes} nom={nomme.get('name')}")

    code, onboard = proprietaire.call("GET", "/onboarding")
    etapes = {e.get("code"): e for e in (onboard or {}).get("steps", [])}
    check("l'étape moyens de paiement passe à terminée",
          etapes.get("moyens", {}).get("done") is True,
          etapes.get("moyens", {}).get("detail", ""))

    # La caisse ne peut pas désactiver son dernier moyen : sans moyen actif,
    # plus rien ne s'encaisse. Le refus doit être explicite.
    code, refus = proprietaire.call("DELETE", f"/payment-methods/{orange.get('id')}")
    check("un moyen peut être retiré tant qu'un autre reste actif", code == 200, f"http={code}")
    code, seul = proprietaire.call("GET", f"/payment-methods?station_id={station_id}&actives=1")
    dernier = ((seul or {}).get("items") or [{}])[0]
    code, refus = proprietaire.call("DELETE", f"/payment-methods/{dernier.get('id')}")
    check("le dernier moyen actif ne se désactive pas",
          code == 409 and bool(dernier.get("id")), f"http={code} code={erreur_code(refus)}")

    # Paramètres de caisse : lus, écrits, validés — et cloisonnés.
    code, params = proprietaire.call("GET", f"/stations/{station_id}/cash-settings")
    reglages = (params or {}).get("settings", {}) if isinstance(params, dict) else {}
    check("les paramètres de caisse sont lus avec leurs clés",
          code == 200 and len(reglages) == 11 and "receipt_prefix" in reglages,
          f"http={code} clés={len(reglages)}")

    code, _ = proprietaire.call("PUT", f"/stations/{station_id}/cash-settings", {
        "receipt_prefix": "LDP",
        "receipt_footer": "Merci de votre confiance",
        "discrepancy_tolerance_xof": 500,
        "allowed_roles": ["owner", "manager", "cashier"],
        "allow_void": True,
        "default_float_xof": 25000,
    })
    code, params = proprietaire.call("GET", f"/stations/{station_id}/cash-settings")
    ecrits = (params or {}).get("settings", {})
    check("les paramètres de caisse enregistrés sont ceux relus",
          code == 200 and ecrits.get("receipt_prefix") == "LDP"
          and ecrits.get("discrepancy_tolerance_xof") == 500
          and ecrits.get("allowed_roles") == ["owner", "manager", "cashier"]
          and ecrits.get("default_float_xof") == 25000,
          f"http={code} préfixe={ecrits.get('receipt_prefix')} tolérance={ecrits.get('discrepancy_tolerance_xof')}")

    code, refus = proprietaire.call("PUT", f"/stations/{station_id}/cash-settings",
                                    {"allowed_roles": ["patron"]})
    check("un rôle inconnu est refusé dans les paramètres de caisse",
          code == 400 and "allowed_roles" in champs_erreur(refus),
          f"http={code} champs={sorted(champs_erreur(refus))}")

    # 7. Activation : le serveur revérifie tout, puis la station exploite.
    code, onboard = proprietaire.call("GET", "/onboarding")
    check("toutes les étapes obligatoires sont satisfaites",
          onboard.get("can_activate") is True and onboard.get("progress", {}).get("percent") == 100,
          f"percent={onboard.get('progress', {}).get('percent')} manque={[m.get('code') for m in onboard.get('missing', [])]}")

    code, active = proprietaire.call("POST", "/onboarding/activate")
    check("l'activation aboutit", code == 200 and active.get("state") == "active",
          f"http={code} état={active.get('state')}")

    # 8. Cloisonnement : une autre entreprise ne lit ni ne modifie ces données.
    voisine = Client()
    code, _ = voisine.inscrire("Sonde Voisine")
    check("la seconde entreprise s'inscrit", code == 201, f"http={code}")

    code, _ = voisine.call("GET", f"/stations/{station_id}/profile")
    check("la fiche d'une autre entreprise est introuvable (404)", code == 404, f"http={code}")

    code, _ = voisine.call("PATCH", f"/stations/{station_id}", {"city": "Yamoussoukro"})
    check("la station d'une autre entreprise ne se modifie pas (404)", code == 404, f"http={code}")

    code, _ = voisine.call("PUT", f"/stations/{station_id}/opening-hours", corps_horaires())
    check("les horaires d'une autre entreprise ne s'écrivent pas (404)", code == 404, f"http={code}")

    code, _ = voisine.call("PUT", f"/stations/{station_id}/cash-settings", {"receipt_prefix": "VOISIN"})
    check("les paramètres de caisse d'une autre entreprise ne s'écrivent pas (404)",
          code == 404, f"http={code}")

    # 9. La donnée enregistrée est bien celle relue après activation.
    code, profil = proprietaire.call("GET", f"/stations/{station_id}/profile")
    check("la fiche reste lisible après activation",
          code == 200 and (profil or {}).get("station", {}).get("code") == "LDP",
          f"http={code}")

    print()
    failed = [nom for nom, ok, _ in RESULTS if not ok]
    print(f"{len(RESULTS) - len(failed)}/{len(RESULTS)} contrôles au vert")
    for nom in failed:
        print("  KO :", nom)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
