#!/usr/bin/env python3
"""Sonde WashGuard du lot 8 : récapitulatif, activation, tableau de bord.

    python3 scripts/verifications/sonde-lot8.py [base_url]

La sonde crée **sa propre entreprise** et la conduit jusqu'à la première fiche.
Elle n'a besoin d'aucune donnée de démonstration ; lancer l'API avec un
`REGISTER_MAX_PER_HOUR` suffisant (une inscription).

Ce qu'elle éprouve, dans l'ordre du parcours :

  1. le récapitulatif — les treize étapes, le bloc `setup`, les comptages
     réels du catalogue, les prérequis manquants, les réglages facultatifs,
     et **aucun montant** ;
  2. le refus d'activation — nommé (`error.fields` + `error.details.blockers`),
     précis, et **sans écriture** : ni état, ni trace, ni instantané ;
  3. le chemin jusqu'à l'activation — ce que chaque étape satisfaite retire de
     la liste des obstacles, puis l'activation et son idempotence ;
  4. le tableau de bord — avant la première vente : des états honnêtes et
     aucun montant inventé ; après la première fiche : les chiffres réels (dont
     la distinction entre encaissé et reste à encaisser) ;
  5. les rôles — le caissier n'obtient aucun bloc d'administration, le laveur
     ni tableau de bord ni récapitulatif, et son poste ne porte aucun montant.

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

# Le corps exact que construit le formulaire « entreprise » : l'étape ne se
# déclare terminée que sur des informations complètes (l'écran le dit).
CORPS_ENTREPRISE = {
    "trade_name": "Lavage Sonde 8",
    "legal_name": "LAVAGE SONDE 8 SARL U",
    "activity_type": "lavage manuel",
    "activity_description": "Lavage à la main, nettoyage intérieur, polissage.",
    "activity_started_on": "2020-06-01",
    "whatsapp": "+225 07 00 00 00 08",
    "website": "https://sonde8.example",
    "billing_name": "LAVAGE SONDE 8 SARL U",
    "billing_address": "Rue des Jardins, Plateau",
    "billing_email": "facturation@sonde8.example",
    "billing_phone": "+225 27 21 00 00 08",
    "legal_mentions": "SARL U au capital de 1 000 000 FCFA",
    "registry": {
        "rccm": "CI-ABJ-2020-B-54322",
        "tax_id": "1926548K",
        "cnps_employer_number": "",
        "legal_form": "SARL U",
        "legal_representative": "Awa Koné",
        "fiscal_regime": "",
    },
}

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
    jours = []
    for weekday in range(1, 8):
        ouvre, ferme, pause_debut, pause_fin = JOURS[weekday]
        jours.append({
            "weekday": weekday,
            "is_open": ouvre != "",
            "opens_at": ouvre,
            "closes_at": ferme,
            "break_start": pause_debut,
            "break_end": pause_fin,
            "last_checkin_at": "17:30" if ferme == "18:00" else ("13:00" if ferme else ""),
            "booking_allowed": True,
        })
    return {"days": jours, "exceptions": []}


class Client:
    def __init__(self, nom="sonde"):
        self.nom = nom
        self.cookie = None
        self.bearer = None

    def call(self, method, path, body=None):
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(BASE + path, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        if self.cookie:
            req.add_header("Cookie", self.cookie)
        if self.bearer:
            req.add_header("Authorization", "Bearer " + self.bearer)
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

    def inscrire(self, nom):
        return self.call("POST", "/auth/register", {
            "organization_name": nom,
            "full_name": "Propriétaire Sonde 8",
            "phone": "+225 27 00 00 00 08",
            "email": f"sonde8-{uuid.uuid4().hex[:8]}@test.local",
            "password": "sonde12345",
            "station_name": f"Station {nom}",
            "city": "Abidjan",
        })

    def connecter(self, email, mot_de_passe):
        return self.call("POST", "/auth/login", {"email": email, "password": mot_de_passe})


def check(name, ok, detail=""):
    RESULTS.append((name, ok, detail))
    print(f"{'OK ' if ok else 'KO '} {name}" + (f" — {detail}" if detail else ""), flush=True)


def erreur(payload):
    """Code d'erreur applicatif, quelle que soit l'enveloppe."""
    if isinstance(payload, dict):
        for cle in ("error", "code", "erreur"):
            valeur = payload.get(cle)
            if isinstance(valeur, str):
                return valeur
            if isinstance(valeur, dict):
                return valeur.get("code") or valeur.get("error") or ""
    return ""


def bloc_erreur(payload):
    """Le contenu de `error`, pour lire ses détails."""
    if isinstance(payload, dict) and isinstance(payload.get("error"), dict):
        return payload["error"]
    return {}


def cles_montants(valeur, chemin=""):
    """Les chemins des clés qui portent un montant (`*_xof`)."""
    trouves = []
    if isinstance(valeur, dict):
        for cle, enfant in valeur.items():
            suivant = f"{chemin}.{cle}" if chemin else cle
            if cle.endswith("_xof"):
                trouves.append(suivant)
            trouves += cles_montants(enfant, suivant)
    elif isinstance(valeur, list):
        for index, enfant in enumerate(valeur):
            trouves += cles_montants(enfant, f"{chemin}[{index}]")
    return trouves


def codes_obstacles(setup):
    return {point.get("code") for point in ((setup or {}).get("activation") or {}).get("blockers", [])}


def main():
    proprietaire = Client("propriétaire")
    code, inscription = proprietaire.inscrire("Sonde Lot 8")
    check("inscription d'une entreprise neuve", code == 201, f"http={code} {erreur(inscription)}")
    if code != 201:
        return 1

    _, me = proprietaire.call("GET", "/auth/me")
    station = (me or {}).get("station", {}).get("id", "") if isinstance(me, dict) else ""
    check("la station de référence est connue", bool(station), f"station={station}")
    if not station:
        return 1

    # -----------------------------------------------------------------------
    # 1. Le récapitulatif : ce qui existe, ce qui manque, aucun montant
    # -----------------------------------------------------------------------
    code, recap = proprietaire.call("GET", "/onboarding/summary")
    check("le récapitulatif est servi", code == 200, f"http={code} {erreur(recap)}")
    if code != 200:
        return 1

    etapes = {e.get("code"): e for e in (recap or {}).get("steps", [])}
    check("le récapitulatif décrit les treize étapes", len(etapes) == 13, f"étapes={len(etapes)}")
    check("l'identité de l'entreprise figure au récapitulatif",
          (recap.get("organization") or {}).get("name") == "Sonde Lot 8",
          str((recap.get("organization") or {}).get("name")))
    check("le récapitulatif réutilise l'avancement du parcours",
          (recap.get("progress") or {}).get("total") == 8
          and (recap.get("progress") or {}).get("percent") is not None,
          str(recap.get("progress")))

    setup = recap.get("setup") or {}
    check("le bloc de mise en route est présent", bool(setup), f"clés={sorted(setup)[:6]}")
    check("le bloc dit pour quel profil il a été composé",
          setup.get("role") == "owner"
          and (setup.get("can") or {}).get("manage_settings") is True
          and (setup.get("can") or {}).get("manage_organization") is True
          and (setup.get("can") or {}).get("see_amounts") is True,
          f"rôle={setup.get('role')} can={setup.get('can')}")

    activation = setup.get("activation") or {}
    check("un compte neuf n'est pas activé",
          activation.get("state") == "pending" and activation.get("active") is False,
          f"état={activation.get('state')} active={activation.get('active')}")
    check("un compte neuf ne peut pas être activé",
          activation.get("can_activate") is False and activation.get("blockers_count") == 3,
          f"can_activate={activation.get('can_activate')} obstacles={activation.get('blockers_count')}")
    obstacles = codes_obstacles(setup)
    check("les obstacles sont exactement horaires, catégories et tarifs",
          obstacles == {"horaires", "categories", "tarifs"}, str(sorted(obstacles)))
    check("le catalogue de référence satisfait déjà « prestations » et « moyens »",
          "prestations" not in obstacles and "moyens" not in obstacles, str(sorted(obstacles)))

    prerequis = setup.get("prerequisites") or []
    check("les huit prérequis d'activation sont listés",
          len(prerequis) == 8 and all(p.get("required") is True for p in prerequis)
          and all(p.get("title") and p.get("hint") for p in prerequis),
          f"{len(prerequis)} prérequis")
    check("chaque obstacle dit quoi corriger et où",
          all(point.get("hint") and point.get("screen") and point.get("title")
              for point in (activation.get("blockers") or [])),
          str([point.get("code") for point in activation.get("blockers") or []]))

    catalogue = setup.get("catalogue") or {}
    check("le catalogue est présent mais sans prix",
          catalogue.get("services_active", 0) > 0 and catalogue.get("services_priced") == 0
          and catalogue.get("services_sellable", 0) > 0,
          f"prestations={catalogue.get('services_active')} vendables={catalogue.get('services_sellable')} "
          f"tarifées={catalogue.get('services_priced')}")
    check("aucune catégorie de véhicule n'est active à l'inscription",
          catalogue.get("categories_active") == 0, f"catégories={catalogue.get('categories_active')}")
    check("les espèces sont actives par défaut",
          catalogue.get("payment_methods_active", 0) >= 1,
          f"moyens={catalogue.get('payment_methods_active')}")
    check("les comptages ne portent aucun montant", not cles_montants(catalogue), str(sorted(catalogue)))

    heures = setup.get("hours") or {}
    jour = heures.get("today") or {}
    check("les horaires sont constatés vides, sans libellé tronqué",
          heures.get("days_open") == 0 and jour.get("declared") is False
          and heures.get("label") == "Aucune journée d'ouverture déclarée"
          and jour.get("state") == "closed_today" and bool(jour.get("day_label")),
          f"jours={heures.get('days_open')} libellé={heures.get('label')!r} "
          f"jour={jour.get('day_label')!r} état={jour.get('state')!r}")

    activite = setup.get("activity") or {}
    check("aucune fiche n'existe sur un compte neuf",
          activite.get("orders_total") == 0 and activite.get("has_activity") is False
          and activite.get("last_order_label") == "Aucune fiche enregistrée",
          f"fiches={activite.get('orders_total')} libellé={activite.get('last_order_label')!r}")
    readiness = setup.get("readiness") or {}
    check("l'écran d'arrivée a des phrases honnêtes avant la première vente",
          readiness.get("no_orders") is True and readiness.get("catalog_ready") is False
          and any("Aucune commande enregistrée" in phrase for phrase in readiness.get("statements") or [])
          and any("pas encore activée" in phrase for phrase in readiness.get("statements") or []),
          f"{len(readiness.get('statements') or [])} phrase(s)")

    facultatifs = {reglage.get("code"): reglage for reglage in setup.get("optional_settings") or []}
    attendus = {"usages", "options", "equipe", "materiel", "commandes", "capacites", "localisation"}
    check("les réglages facultatifs sont nommés et marqués facultatifs",
          attendus <= set(facultatifs)
          and all(reglage.get("required") is False for reglage in facultatifs.values())
          and all(reglage.get("state") in ("done", "empty") for reglage in facultatifs.values()),
          str({code: reglage.get("state") for code, reglage in facultatifs.items()}))
    check("matériel et localisation sont constatés absents, sans être bloquants",
          facultatifs.get("materiel", {}).get("state") == "empty"
          and facultatifs.get("localisation", {}).get("state") == "empty",
          f"matériel={facultatifs.get('materiel', {}).get('state')} "
          f"localisation={facultatifs.get('localisation', {}).get('state')}")

    blocs = setup.get("blocks") or []
    raccourcis = setup.get("shortcuts") or []
    check("le propriétaire reçoit les blocs d'administration et ses raccourcis",
          {"station", "catalogue"} <= {bloc.get("code") for bloc in blocs}
          and {"nouvelle_fiche", "caisse", "rapports"} <= {r.get("code") for r in raccourcis},
          f"blocs={sorted(bloc.get('code') for bloc in blocs)} "
          f"raccourcis={sorted(r.get('code') for r in raccourcis)}")
    check("chaque bloc d'administration dit où il mène et ce qu'il exige",
          all(bloc.get("href") and bloc.get("permission") for bloc in blocs), "")

    montants = cles_montants(setup)
    check("le récapitulatif ne contient aucun montant", not montants, str(montants[:5]))
    trace = recap.get("activated_by") or {}
    check("aucune trace d'activation sur un compte neuf",
          not trace.get("activated_at") and not trace.get("activated_by_name") and not trace.get("snapshot"),
          f"trace={ {cle: trace.get(cle) for cle in ('activated_at', 'activated_by_name')} }")

    # -----------------------------------------------------------------------
    # 2. Le refus d'activation : nommé, précis, sans écriture
    # -----------------------------------------------------------------------
    code, refus = proprietaire.call("POST", "/onboarding/activate")
    check("l'activation d'un compte incomplet est refusée (409 onboarding_incomplete)",
          code == 409 and erreur(refus) == "onboarding_incomplete",
          f"http={code} code={erreur(refus)}")
    contenu = bloc_erreur(refus)
    champs = contenu.get("fields") or {}
    details = contenu.get("details") or {}
    liste_obstacles = details.get("blockers") or []
    check("le refus liste les manques, champ par champ",
          len(champs) == 3 and len(liste_obstacles) == 3 and set(champs) == obstacles,
          f"fields={sorted(champs)} blockers={sorted(p.get('code') for p in liste_obstacles)}")
    check("chaque obstacle du refus dit quoi corriger et où",
          all(point.get("hint") and point.get("screen") and point.get("title") and point.get("detail")
              for point in liste_obstacles),
          str([point.get("code") for point in liste_obstacles]))
    check("le refus se déclare non validé et n'annonce pas d'activation",
          details.get("validated") is False and not contenu.get("message", "").startswith("Station activée"),
          f"validated={details.get('validated')}")

    _, apres_refus = proprietaire.call("GET", "/onboarding")
    _, recap_refus = proprietaire.call("GET", "/onboarding/summary")
    trace_refus = (recap_refus or {}).get("activated_by") or {}
    check("un refus ne change ni l'état ni la trace d'activation",
          (apres_refus or {}).get("state") == "pending"
          and not trace_refus.get("activated_at") and not trace_refus.get("snapshot")
          and ((recap_refus or {}).get("setup") or {}).get("activation", {}).get("active") is False,
          f"état={(apres_refus or {}).get('state')} trace={bool(trace_refus.get('activated_at'))}")

    # -----------------------------------------------------------------------
    # 3. Le chemin jusqu'à l'activation
    # -----------------------------------------------------------------------
    # L'étape « identité » est déjà satisfaite par l'inscription (nom, téléphone
    # et ville sont demandés au premier écran) : les obstacles restants se
    # comptent donc sans elle.
    check("l'identité de l'entreprise est acquise dès l'inscription",
          "entreprise" not in obstacles and "espace" not in obstacles
          and "station" not in obstacles, str(sorted(obstacles)))
    code, profil = proprietaire.call("PATCH", "/organizations/current", CORPS_ENTREPRISE)
    check("le profil complet de l'entreprise s'enregistre", code == 200)
    precedents = len(obstacles)
    for libelle, appel in (
        ("les horaires d'ouverture",
         lambda: proprietaire.call("PUT", f"/stations/{station}/opening-hours", corps_horaires())),
        ("les catégories de véhicules",
         lambda: proprietaire.call("PUT", f"/stations/{station}/vehicle-categories", {
             "categories": [{"code": "berline", "is_active": True, "sort_order": 10},
                            {"code": "moto", "is_active": True, "sort_order": 20}]})),
    ):
        appel()
        _, etape_recap = proprietaire.call("GET", "/onboarding/summary")
        restants = len(codes_obstacles(etape_recap.get("setup") or {}))
        check(f"{libelle} font reculer la liste des obstacles",
              restants < precedents, f"{precedents} → {restants}")
        precedents = restants

    _, recap_avant_prix = proprietaire.call("GET", "/onboarding/summary")
    setup_avant = recap_avant_prix.get("setup") or {}
    check("il ne reste que la grille tarifaire",
          codes_obstacles(setup_avant) == {"tarifs"},
          str(sorted(codes_obstacles(setup_avant))))
    obstacle_tarifs = next((point for point in setup_avant.get("missing") or []
                            if point.get("code") == "tarifs"), {})
    check("l'obstacle « tarifs » dit ce qu'il attend et pour quoi",
          "prix" in (obstacle_tarifs.get("hint") or "")
          and "prix" in (obstacle_tarifs.get("detail") or ""),
          f"{obstacle_tarifs.get('detail')!r}")
    check("les horaires déclarés sont constatés, et la journée se calcule",
          (setup_avant.get("hours") or {}).get("days_open") == 6
          and "6 jours d'ouverture" in ((setup_avant.get("hours") or {}).get("label") or "")
          and (setup_avant.get("hours") or {}).get("today", {}).get("day_label") != "",
          f"jours={(setup_avant.get('hours') or {}).get('days_open')} "
          f"libellé={(setup_avant.get('hours') or {}).get('label')!r}")

    code, prestation = proprietaire.call("POST", "/services", {
        "station_id": station, "name": "Lavage complet berline", "duration_minutes": 45,
    })
    service = prestation.get("id", "") if isinstance(prestation, dict) else ""
    check("une prestation est créée pour être tarifée", code == 201 and bool(service),
          f"http={code} {erreur(prestation)}")
    code, grille = proprietaire.call("PUT", "/price-rules/matrix", {
        "station_id": station,
        "reason": "Grille d'ouverture de la station",
        "generic_prices": [{"service_id": service, "price_xof": 3000}],
    })
    check("la grille tarifaire est enregistrée", code in (200, 201), f"http={code} {erreur(grille)}")

    _, recap_pret = proprietaire.call("GET", "/onboarding/summary")
    setup_pret = recap_pret.get("setup") or {}
    check("la grille satisfaite ouvre l'activation",
          (setup_pret.get("activation") or {}).get("can_activate") is True
          and not codes_obstacles(setup_pret),
          f"can_activate={(setup_pret.get('activation') or {}).get('can_activate')} "
          f"obstacles={sorted(codes_obstacles(setup_pret))}")
    check("le catalogue compte la prestation tarifée et vendable",
          (setup_pret.get("catalogue") or {}).get("services_sellable_priced", 0) >= 1
          and (setup_pret.get("catalogue") or {}).get("categories_active", 0) == 2,
          f"vendables tarifées={(setup_pret.get('catalogue') or {}).get('services_sellable_priced')} "
          f"catégories={(setup_pret.get('catalogue') or {}).get('categories_active')}")

    code, active = proprietaire.call("POST", "/onboarding/activate")
    check("l'activation aboutit et se dit activée",
          code == 200 and active.get("activated") is True and active.get("already_active") is False
          and active.get("state") == "active",
          f"http={code} état={(active or {}).get('state')} message={erreur(active)}")
    instantane = (active or {}).get("snapshot") or {}
    check("l'activation enregistre ce qu'elle a constaté",
          len(instantane.get("prerequisites") or []) == 8 and bool(instantane.get("catalogue"))
          and instantane.get("by_name") == "Propriétaire Sonde 8"
          and instantane.get("opening_days") == 6
          and isinstance(instantane.get("optional_remaining"), list),
          f"prérequis={len(instantane.get('prerequisites') or [])} "
          f"jours={instantane.get('opening_days')} restants={instantane.get('optional_remaining')}")
    check("l'instantané d'activation ne porte aucun montant",
          not cles_montants(instantane), str(cles_montants(instantane)[:5]))

    code, rappel = proprietaire.call("POST", "/onboarding/activate")
    check("une seconde activation est idempotente",
          code == 200 and rappel.get("already_active") is True and rappel.get("state") == "active",
          f"http={code} déjà={rappel.get('already_active')} message={rappel.get('message')}")

    _, recap_active = proprietaire.call("GET", "/onboarding/summary")
    trace = recap_active.get("activated_by") or {}
    check("la trace d'activation est relisible",
          bool(trace.get("activated_at")) and trace.get("activated_by_name") == "Propriétaire Sonde 8"
          and len((trace.get("snapshot") or {}).get("prerequisites") or []) == 8,
          f"par={trace.get('activated_by_name')!r} le={trace.get('activated_at')!r}")
    check("l'état de l'entreprise est passé à « active »",
          recap_active.get("state") == "active"
          and ((recap_active.get("setup") or {}).get("activation") or {}).get("state_label")
          == "Espace en exploitation",
          f"état={recap_active.get('state')}")

    # -----------------------------------------------------------------------
    # 4. Le tableau de bord : des états honnêtes, puis des chiffres réels
    # -----------------------------------------------------------------------
    code, tableau = proprietaire.call("GET", "/reports/dashboard")
    check("le tableau de bord est servi", code == 200, f"http={code} {erreur(tableau)}")
    setup_bord = tableau.get("setup") or {}
    check("le tableau de bord porte le même bloc de mise en route",
          setup_bord.get("role") == "owner"
          and (setup_bord.get("station") or {}).get("id") == station,
          f"rôle={setup_bord.get('role')} station={(setup_bord.get('station') or {}).get('id')}")
    check("aucune fiche n'est encore enregistrée, et l'écran le dit",
          (setup_bord.get("activity") or {}).get("orders_total") == 0
          and (setup_bord.get("readiness") or {}).get("no_orders") is True
          and tableau.get("no_activity") is True
          and "aucune fiche" in (tableau.get("no_activity_note") or "").lower(),
          f"fiches={(setup_bord.get('activity') or {}).get('orders_total')} "
          f"no_activity={tableau.get('no_activity')} note={(tableau.get('no_activity_note') or '')[:60]!r}")
    retires = sorted(bloc for bloc in
                     ("today", "series", "employees", "recent", "top_services", "payment_methods",
                      "goal", "alerts") if bloc in tableau)
    check("les blocs financiers sont retirés tant qu'aucune fiche n'existe",
          not retires, f"encore servis={retires}")
    # `definitions` est le lexique du rapport : il **nomme** les clés de montants
    # (`revenue_xof`, `due_xof`…) pour que l'écran puisse légender ses colonnes.
    # Il n'en porte aucune valeur, il est donc exclu du balayage — tout le reste
    # de la réponse doit être exempt de montant.
    corps = {cle: valeur for cle, valeur in tableau.items() if cle != "definitions"}
    check("aucun montant n'est servi avant la première fiche",
          not cles_montants(corps), str(cles_montants(corps)[:5]))
    check("l'état de la caisse, lui, reste lisible",
          "cash" in tableau and tableau.get("cash") is None and "definitions" in tableau,
          f"caisse={tableau.get('cash')}")
    check("le catalogue prêt est constaté, pas supposé",
          (setup_bord.get("readiness") or {}).get("catalog_ready") is True
          and (setup_bord.get("catalogue") or {}).get("services_sellable_priced", 0) >= 1,
          f"catalog_ready={(setup_bord.get('readiness') or {}).get('catalog_ready')}")
    check("le bloc de mise en route du tableau de bord ne porte aucun montant",
          not cles_montants(setup_bord), str(cles_montants(setup_bord)[:5]))
    check("il reste des réglages facultatifs, et ils sont annoncés comme tels",
          any(reglage.get("state") == "empty" for reglage in setup_bord.get("optional_settings") or [])
          and (setup_bord.get("activation") or {}).get("active") is True,
          str([r.get("code") for r in setup_bord.get("optional_settings") or []
               if r.get("state") == "empty"]))

    code, session = proprietaire.call("POST", "/cash/sessions",
                                      {"station_id": station, "opening_float_xof": 20000})
    check("la caisse s'ouvre pour encaisser", code == 201, f"http={code} {erreur(session)}")

    code, fiche = proprietaire.call("POST", "/wash-orders", {
        "station_id": station,
        "items": [{"service_id": service, "quantity": 1}],
    })
    check("la première fiche est acceptée après activation",
          code == 201 and fiche.get("total_xof") == 3000,
          f"http={code} total={(fiche or {}).get('total_xof')} {erreur(fiche)}")
    fiche_id = fiche.get("id", "") if isinstance(fiche, dict) else ""

    _, tableau_apres = proprietaire.call("GET", "/reports/dashboard")
    setup_apres = tableau_apres.get("setup") or {}
    jour_apres = tableau_apres.get("today") or {}
    check("le tableau de bord compte la fiche créée",
          (setup_apres.get("activity") or {}).get("orders_total") == 1
          and (setup_apres.get("activity") or {}).get("has_activity") is True
          and (setup_apres.get("activity") or {}).get("orders_open") == 1,
          f"fiches={(setup_apres.get('activity') or {}).get('orders_total')} "
          f"ouvertes={(setup_apres.get('activity') or {}).get('orders_open')}")
    check("la fiche non soldée laisse le chiffre d'affaires à zéro et un reste à encaisser réel",
          jour_apres.get("revenue_xof") == 0 and jour_apres.get("payments_xof") == 0
          and jour_apres.get("due_xof") == 3000,
          str({cle: jour_apres.get(cle) for cle in ("revenue_xof", "payments_xof", "due_xof")}))

    # -----------------------------------------------------------------------
    # 5. Les rôles
    # -----------------------------------------------------------------------
    jeton = uuid.uuid4().hex[:6]
    caissier = Client("caissier")
    email_caissier = f"sonde8-caissier-{jeton}@test.local"
    code, caissier_cree = proprietaire.call("POST", "/employees", {
        "full_name": f"Caissier Sonde {jeton}", "email": email_caissier, "role": "cashier",
        "station_id": station,
    })
    mot_de_passe = caissier_cree.get("temporary_password", "") if isinstance(caissier_cree, dict) else ""
    check("un caissier est créé avec son mot de passe temporaire",
          code == 201 and bool(mot_de_passe), f"http={code}")
    code, _ = caissier.connecter(email_caissier, mot_de_passe)
    check("la session du caissier s'ouvre", code == 200 and bool(caissier.cookie), f"http={code}")

    code, tableau_caissier = caissier.call("GET", "/reports/dashboard")
    check("le caissier voit le tableau de bord du jour", code == 200, f"http={code}")
    setup_caissier = tableau_caissier.get("setup") or {}
    check("le caissier n'obtient aucun bloc d'administration",
          setup_caissier.get("blocks") == [], str(setup_caissier.get("blocks")))
    check("le caissier n'obtient pas les droits d'administration",
          (setup_caissier.get("can") or {}).get("manage_settings") is False
          and (setup_caissier.get("can") or {}).get("manage_organization") is False
          and (setup_caissier.get("can") or {}).get("see_amounts") is True,
          str(setup_caissier.get("can")))
    raccourcis_caissier = {r.get("code") for r in setup_caissier.get("shortcuts") or []}
    check("le caissier garde ses raccourcis d'exploitation, sans les rapports",
          {"caisse", "nouvelle_fiche"} <= raccourcis_caissier and "rapports" not in raccourcis_caissier,
          str(sorted(raccourcis_caissier)))
    check("le récapitulatif de mise en route reste fermé au caissier",
          caissier.call("GET", "/onboarding/summary")[0] == 403, "")

    laveur = Client("laveur")
    email_laveur = f"sonde8-laveur-{jeton}@test.local"
    code, laveur_cree = proprietaire.call("POST", "/employees", {
        "full_name": f"Laveur Sonde {jeton}", "email": email_laveur, "role": "washer",
        "station_id": station, "job_title": "Laveur automobile",
    })
    mot_de_passe_laveur = laveur_cree.get("temporary_password", "") if isinstance(laveur_cree, dict) else ""
    laveur_id = laveur_cree.get("id", "") if isinstance(laveur_cree, dict) else ""
    check("un laveur est créé avec sa fiche de poste",
          code == 201 and bool(mot_de_passe_laveur) and bool(laveur_id), f"http={code}")
    code, _ = laveur.connecter(email_laveur, mot_de_passe_laveur)
    check("la session du laveur s'ouvre", code == 200 and bool(laveur.cookie), f"http={code}")

    check("le laveur n'obtient pas le tableau de bord (403)",
          laveur.call("GET", "/reports/dashboard")[0] == 403, "")
    check("le laveur n'obtient pas le récapitulatif (403)",
          laveur.call("GET", "/onboarding/summary")[0] == 403, "")

    code, affectation = proprietaire.call("PATCH", f"/wash-orders/{fiche_id}", {"employee_id": laveur_id})
    check("la fiche est affectée au laveur", code == 200, f"http={code} {erreur(affectation)}")
    code, taches = laveur.call("GET", "/washer/tasks")
    items = (taches or {}).get("items") if isinstance(taches, dict) else []
    check("le laveur retrouve sa tâche, sans aucun montant",
          code == 200 and bool(items) and not cles_montants(items),
          f"http={code} tâches={len(items or [])} montants={cles_montants(items)[:3]}")

    echecs = [nom for nom, ok, _ in RESULTS if not ok]
    print(f"\n{len(RESULTS) - len(echecs)}/{len(RESULTS)} vérifications passent.")
    if echecs:
        print("Échecs : " + " · ".join(echecs))
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
