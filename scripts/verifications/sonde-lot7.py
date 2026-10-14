#!/usr/bin/env python3
"""Sonde WashGuard du lot 7 : personnel, matériel, stock, règles de commande.

    python3 scripts/verifications/sonde-lot7.py [base_url]

La sonde crée **sa propre entreprise** (comme `sonde-onboarding.py`) et la met
en route de bout en bout : profil, horaires, catégories, prestation tarifée,
activation, caisse. Elle n'a donc besoin d'aucune donnée de démonstration :
lancer l'API avec `REGISTER_MAX_PER_HOUR` suffisamment haut (une inscription).

Ce qu'elle éprouve, dans l'ordre du parcours :

  1. le parcours — les trois étapes facultatives du lot 7 (équipe, matériel,
     règles) et l'activation qui n'en dépend pas ;
  2. l'équipe — fiche de poste (intitulé, arrivée, horaires, situation),
     cloisonnement de la rémunération, situation inventée refusée ;
  3. le matériel — référentiel, ajout par modèle, ajout libre, retrait ;
  4. le stock — quantité inconnue, sortie refusée, inventaire, seuil, perte
     motivée, historique des mouvements ;
  5. les règles de la station — chaque interrupteur est activé puis éprouvé sur
     l'opération qu'il gouverne (création, remise, acompte, annulation) ;
  6. le cycle de vie — les sept états, contrôle qualité, transition interdite,
     fiche close qui ne repart pas, plafond de fiches par laveur.

Aucune dépendance : uniquement la bibliothèque standard.
"""
import json
import sys
import urllib.error
import urllib.request
import uuid

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8099") + "/api/v1"
RESULTS = []

# Le corps exact que construit le formulaire « entreprise » : l'étape ne se
# déclare terminée que sur des informations complètes (l'écran le dit).
CORPS_ENTREPRISE = {
    "trade_name": "Lavage Sonde 7",
    "legal_name": "LAVAGE SONDE 7 SARL U",
    "activity_type": "lavage manuel",
    "activity_description": "Lavage à la main, nettoyage intérieur, polissage.",
    "activity_started_on": "2020-06-01",
    "whatsapp": "+225 07 00 00 00 07",
    "website": "https://sonde7.example",
    "billing_name": "LAVAGE SONDE 7 SARL U",
    "billing_address": "Rue des Jardins, Plateau",
    "billing_email": "facturation@sonde7.example",
    "billing_phone": "+225 27 21 00 00 07",
    "legal_mentions": "SARL U au capital de 1 000 000 FCFA",
    "registry": {
        "rccm": "CI-ABJ-2020-B-54321",
        "tax_id": "1926547K",
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
            "full_name": "Propriétaire Sonde 7",
            "phone": "+225 27 00 00 00 07",
            "email": f"sonde7-{uuid.uuid4().hex[:8]}@test.local",
            "password": "sonde12345",
            "station_name": f"Station {nom}",
            "city": "Abidjan",
        })


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


def article_nomme(fiche_stock, nom):
    for article in (fiche_stock or {}).get("items", []):
        if article.get("name") == nom:
            return article
    return {}


def main():
    proprietaire = Client("propriétaire")
    code, inscription = proprietaire.inscrire("Sonde Lot 7")
    check("inscription d'une entreprise neuve", code == 201, f"http={code} {erreur(inscription)}")
    if code != 201:
        return 1

    _, me = proprietaire.call("GET", "/auth/me")
    station = (me or {}).get("station", {}).get("id", "") if isinstance(me, dict) else ""
    check("la station de référence est connue", bool(station), f"station={station}")
    if not station:
        return 1

    # -----------------------------------------------------------------------
    # 1. Le parcours : les trois étapes du lot 7 sont facultatives
    # -----------------------------------------------------------------------
    _, parcours = proprietaire.call("GET", "/onboarding")
    etapes = {e.get("code"): e for e in (parcours or {}).get("steps", [])}
    check("le parcours compte treize étapes depuis le lot 7",
          len(etapes) == 13, f"étapes={len(etapes)}")
    check("les étapes équipe, matériel et règles sont proposées",
          {"equipe", "materiel", "commandes"} <= set(etapes),
          str(sorted(set(etapes) & {"equipe", "materiel", "commandes"})))
    check("aucune de ces trois étapes n'est obligatoire à l'activation",
          all(etapes.get(c, {}).get("required") is False
              for c in ("equipe", "materiel", "commandes")),
          str({c: etapes.get(c, {}).get("required") for c in ("equipe", "materiel", "commandes")}))

    # Mise en route minimale : de quoi activer, pas plus.
    proprietaire.call("PATCH", "/organizations/current", CORPS_ENTREPRISE)
    proprietaire.call("PUT", f"/stations/{station}/opening-hours", corps_horaires())
    proprietaire.call("PUT", f"/stations/{station}/vehicle-categories", {
        "categories": [{"code": "berline", "is_active": True, "sort_order": 10},
                       {"code": "moto", "is_active": True, "sort_order": 20}],
    })
    code, prestation = proprietaire.call("POST", "/services", {
        "station_id": station, "name": "Lavage complet berline", "duration_minutes": 45,
    })
    service = prestation.get("id", "") if isinstance(prestation, dict) else ""
    proprietaire.call("PUT", "/price-rules/matrix", {
        "station_id": station,
        "reason": "Grille d'ouverture de la station",
        "generic_prices": [{"service_id": service, "price_xof": 3000}],
    })

    _, parcours = proprietaire.call("GET", "/onboarding")
    check("l'activation est possible sans équipe, matériel ni stock",
          parcours.get("can_activate") is True and parcours.get("progress", {}).get("percent") == 100,
          f"percent={parcours.get('progress', {}).get('percent')} "
          f"manque={[m.get('code') for m in parcours.get('missing', [])]}")
    code, active = proprietaire.call("POST", "/onboarding/activate")
    check("l'activation aboutit", code == 200 and active.get("state") == "active",
          f"http={code} état={active.get('state') if isinstance(active, dict) else None}")

    # La caisse : la sonde encaisse elle-même, elle ouvre donc sa session.
    code, session = proprietaire.call("POST", "/cash/sessions",
                                      {"station_id": station, "opening_float_xof": 20000})
    check("ouverture d'une session de caisse", code == 201, f"http={code} {erreur(session)}")

    jeton = uuid.uuid4().hex[:6]

    # -----------------------------------------------------------------------
    # 2. L'équipe
    # -----------------------------------------------------------------------
    email = f"sonde-lot7-{jeton}@test.local"
    code, employe = proprietaire.call("POST", "/employees", {
        "full_name": f"Awa Sonde {jeton}", "email": email, "role": "washer",
        "station_id": station, "job_title": "Laveur automobile",
        "hired_on": "2026-03-01", "work_schedule": "Lundi au samedi, 7 h – 18 h",
        "employment_status": "active",
    })
    check("création d'un employé avec sa fiche de poste", code == 201, f"http={code} {erreur(employe)}")
    employe_id = employe.get("id", "") if isinstance(employe, dict) else ""

    code, liste = proprietaire.call("GET", "/employees")
    fiche = next((e for e in ((liste or {}).get("items") or []) if e.get("id") == employe_id), {})
    check("le poste, l'arrivée, les horaires et la situation sont relus",
          fiche.get("job_title") == "Laveur automobile"
          and fiche.get("hired_on") == "2026-03-01"
          and fiche.get("work_schedule") == "Lundi au samedi, 7 h – 18 h"
          and fiche.get("employment_status") == "active",
          str({cle: fiche.get(cle) for cle in
               ("job_title", "hired_on", "work_schedule", "employment_status")}))

    code, situation = proprietaire.call("PATCH", f"/employees/{employe_id}",
                                        {"employment_status": "en_vacances"})
    check("une situation inventée est refusée (400 attendu)", code == 400,
          f"http={code} {erreur(situation)}")

    code, _ = proprietaire.call("PATCH", f"/employees/{employe_id}",
                               {"employment_status": "on_leave"})
    _, liste = proprietaire.call("GET", "/employees")
    fiche = next((e for e in ((liste or {}).get("items") or []) if e.get("id") == employe_id), {})
    check("situation « En congé » acceptée et relue",
          code == 200 and fiche.get("employment_status") == "on_leave",
          f"http={code} situation={fiche.get('employment_status')}")

    # Le laveur se connecte avec son mot de passe provisoire : c'est avec ses
    # droits que les refus doivent tomber.
    laveur = Client("laveur")
    provisoire = employe.get("temporary_password", "") if isinstance(employe, dict) else ""
    if provisoire:
        laveur.call("POST", "/auth/login", {"email": email, "password": provisoire})
    code, refus = laveur.call("GET", "/employees")
    check("un laveur n'accède pas à la liste du personnel (403 attendu)",
          code == 403, f"http={code} {erreur(refus)}")
    # Le congé ne coupe pas l'accès : c'est une situation RH, pas une sanction.
    # Seule la désactivation du compte (`is_active`) ferme la porte.
    code_conge, relu_conge = laveur.call("GET", "/auth/me")
    check("un employé en congé garde son accès au portail",
          code_conge == 200 and relu_conge.get("user", {}).get("id") == employe_id,
          f"http={code_conge}")
    code, annulation = laveur.call("PATCH", f"/wash-orders/inconnue", {"status": "canceled"})
    check("un laveur ne peut pas annuler une fiche (403 ou 404 attendu)",
          code in (403, 404), f"http={code}")

    # -----------------------------------------------------------------------
    # 3. Le matériel
    # -----------------------------------------------------------------------
    base = f"/stations/{station}/equipment"
    code, parc = proprietaire.call("GET", base, None)
    modeles = (parc or {}).get("templates") or []
    check("le référentiel d'équipements est proposé",
          code == 200 and len(modeles) >= 10, f"http={code} modèles={len(modeles)}")

    code, jet = proprietaire.call("POST", base, {
        "template_code": "jet_haute_pression", "quantity": 2, "state": "neuf",
        "acquired_on": "2026-02-10", "cost_xof": 450000,
    })
    check("ajout d'un matériel à partir du référentiel", code == 201, f"http={code} {erreur(jet)}")

    code, libre = proprietaire.call("POST", base, {
        "name": f"Remorque sonde {jeton}", "quantity": 1, "state": "bon",
    })
    check("ajout libre d'un matériel hors référentiel", code == 201, f"http={code} {erreur(libre)}")
    remorque = libre.get("id", "") if isinstance(libre, dict) else ""

    code, _ = proprietaire.call("POST", base, {"quantity": 1})
    check("un matériel sans nom ni modèle est refusé (400 attendu)", code == 400, f"http={code}")

    code, retire = proprietaire.call("PATCH", f"/equipment/{remorque}",
                                     {"is_active": False, "retired_on": "2026-09-30"})
    check("un matériel retiré reste dans l'historique", code == 200, f"http={code} {erreur(retire)}")
    _, parc = proprietaire.call("GET", base, None)
    fiche_materiel = next((e for e in ((parc or {}).get("items") or []) if e.get("id") == remorque), {})
    resume = (parc or {}).get("summary") or {}
    check("le matériel retiré n'est plus compté en service",
          fiche_materiel.get("is_active") is False and resume.get("retires") == 1,
          f"actif={fiche_materiel.get('is_active')} retirés={resume.get('retires')}")

    # -----------------------------------------------------------------------
    # 4. Le stock
    # -----------------------------------------------------------------------
    base_stock = f"/stations/{station}/stock"
    code, stock = proprietaire.call("GET", base_stock, None)
    categories = (stock or {}).get("categories") or []
    check("les catégories de consommables sont proposées",
          code == 200 and len(categories) >= 5, f"http={code} catégories={len(categories)}")

    nom_article = f"Savon sonde {jeton}"
    code, article = proprietaire.call("POST", base_stock + "/items", {
        "name": nom_article, "category_code": "produits_lavage", "unit": "bidon",
        "min_quantity": 4,
    })
    check("déclaration d'un article sans quantité", code == 201, f"http={code} {erreur(article)}")
    article_id = article.get("id", "") if isinstance(article, dict) else ""
    check("« je ne sais pas encore » n'est pas « zéro »",
          isinstance(article, dict) and article.get("quantity_known") is False,
          f"quantity_known={article.get('quantity_known') if isinstance(article, dict) else None}")

    def mouvement(corps, attendu, libelle):
        code, rep = proprietaire.call("POST", f"/stock/items/{article_id}/movements", corps)
        check(libelle, code == attendu, f"http={code} {erreur(rep)}")
        return rep

    mouvement({"kind": "sortie", "quantity": 1}, 409,
              "sortie impossible sur un stock jamais compté")

    inventaire = mouvement({"kind": "inventaire", "quantity": 3}, 201,
                           "l'inventaire compte le stock")
    check("la quantité après inventaire est celle comptée",
          inventaire.get("quantity_after") == 3,
          f"après={inventaire.get('quantity_after') if isinstance(inventaire, dict) else None}")

    _, stock = proprietaire.call("GET", base_stock, None)
    fiche_article = article_nomme(stock, nom_article)
    check("3 unités sous un seuil de 4 déclarent l'alerte",
          fiche_article.get("state") == "seuil", f"état={fiche_article.get('state')}")

    code, direct = proprietaire.call("PATCH", f"/stock/items/{article_id}", {"quantity": 12})
    check("une quantité ne s'écrit pas à la main (409 attendu)",
          code == 409 and erreur(direct) == "stock_movement_required",
          f"http={code} {erreur(direct)}")

    mouvement({"kind": "entree", "quantity": 6, "unit_cost_xof": 3500}, 201,
              "entrée de marchandise")
    mouvement({"kind": "perte", "quantity": 1}, 400, "une perte sans motif est refusée")
    mouvement({"kind": "perte", "quantity": 1, "reason": "Bidon percé"}, 201,
              "perte motivée enregistrée")

    code, mouvements = proprietaire.call("GET", f"/stock/items/{article_id}/movements", None)
    check("l'historique explique la quantité courante (3 + 6 − 1 = 8)",
          code == 200 and len((mouvements or {}).get("items") or []) == 3,
          f"http={code} mouvements={len((mouvements or {}).get('items') or [])}")

    _, stock = proprietaire.call("GET", base_stock, None)
    fiche_article = article_nomme(stock, nom_article)
    check("la quantité relue est celle du dernier mouvement",
          fiche_article.get("quantity") == 8, f"quantité={fiche_article.get('quantity')}")

    # Les étapes facultatives passent à « terminées » : le parcours suit l'état
    # réel de la station, il ne coche pas sur parole.
    _, parcours = proprietaire.call("GET", "/onboarding")
    etapes = {e.get("code"): e for e in (parcours or {}).get("steps", [])}
    check("les étapes équipe, matériel et stock passent à terminées",
          etapes.get("equipe", {}).get("done") is True
          and etapes.get("materiel", {}).get("done") is True
          and etapes.get("commandes", {}).get("done") is True,
          str({c: etapes.get(c, {}).get("done") for c in ("equipe", "materiel", "commandes")}))

    # -----------------------------------------------------------------------
    # 5. Les règles de la station et le cycle de vie
    # -----------------------------------------------------------------------
    regles = f"/stations/{station}/order-rules"

    def appliquer(**valeurs):
        corps = {
            "allow_walk_in": True, "require_washer": False, "quality_check_required": False,
            "release_requires_payment": True, "allow_partial_payments": True,
            "require_cancel_reason": True, "max_open_orders_per_washer": 0,
        }
        corps.update(valeurs)
        return proprietaire.call("PUT", regles, corps)

    code, actuelles = proprietaire.call("GET", regles, None)
    check("les règles de la station sont lisibles",
          code == 200 and len((actuelles or {}).get("statuses") or []) == 7,
          f"http={code} statuts={len((actuelles or {}).get('statuses') or [])}")

    def nouvelle_fiche(extra=None):
        corps = {"station_id": station, "customer_name": f"Client sonde {jeton}",
                 "status": "pending", "items": [{"service_id": service, "quantity": 1}]}
        if extra:
            corps.update(extra)
        return proprietaire.call("POST", "/wash-orders", corps)

    # a) Le client de passage.
    code, _ = appliquer(allow_walk_in=False)
    check("les règles s'enregistrent", code == 200, f"http={code}")
    code, refus = nouvelle_fiche({"customer_id": ""})
    check("clients de passage refusés ⇒ 400 walk_in_not_allowed",
          code == 400 and erreur(refus) == "walk_in_not_allowed", f"http={code} {erreur(refus)}")

    # b) Le laveur obligatoire.
    appliquer(require_washer=True)
    code, refus = nouvelle_fiche()
    check("laveur obligatoire ⇒ 400 washer_required",
          code == 400 and erreur(refus) == "washer_required", f"http={code} {erreur(refus)}")

    # c) Contrôle qualité obligatoire, remise subordonnée au paiement, acompte
    #    refusé : trois règles qui doivent toutes mordre.
    appliquer(quality_check_required=True, release_requires_payment=True,
              allow_partial_payments=False)
    code, fiche_ordre = nouvelle_fiche()
    check("fiche créée sous les règles de la station", code == 201, f"http={code} {erreur(fiche_ordre)}")
    fiche_id = fiche_ordre.get("id", "") if isinstance(fiche_ordre, dict) else ""
    if not fiche_id:
        return 1

    code, saute = proprietaire.call("PATCH", f"/wash-orders/{fiche_id}", {"status": "ready"})
    check("contrôle qualité obligatoire ⇒ passage direct en « prête » refusé",
          code == 409 and erreur(saute) == "quality_check_required", f"http={code} {erreur(saute)}")

    proprietaire.call("PATCH", f"/wash-orders/{fiche_id}", {"status": "in_progress"})
    code, qualite = proprietaire.call("PATCH", f"/wash-orders/{fiche_id}", {"status": "quality_check"})
    check("le passage par le contrôle qualité est accepté", code == 200, f"http={code} {erreur(qualite)}")
    code, prete = proprietaire.call("PATCH", f"/wash-orders/{fiche_id}", {"status": "ready"})
    check("« prête » après contrôle qualité", code == 200, f"http={code} {erreur(prete)}")

    total = (fiche_ordre.get("total_xof") or 0)
    acompte = max(1, total // 2)
    code, refus = proprietaire.call("POST", f"/wash-orders/{fiche_id}/payments",
                                    {"amount_xof": acompte, "method": "especes"})
    check("acompte refusé sur la fiche ⇒ 409 partial_payment_disabled",
          code == 409 and erreur(refus) == "partial_payment_disabled", f"http={code} {erreur(refus)}")
    code, refus = proprietaire.call("POST", "/cash/payments",
                                    {"station_id": station, "wash_order_id": fiche_id,
                                     "amount_xof": acompte, "method": "especes"})
    check("acompte refusé aussi au guichet ⇒ 409 partial_payment_disabled",
          code == 409 and erreur(refus) == "partial_payment_disabled", f"http={code} {erreur(refus)}")

    code, refus = proprietaire.call("PATCH", f"/wash-orders/{fiche_id}", {"status": "completed"})
    check("remise subordonnée au paiement ⇒ 409 payment_required",
          code == 409 and erreur(refus) == "payment_required", f"http={code} {erreur(refus)}")

    code, encaisse = proprietaire.call("POST", f"/wash-orders/{fiche_id}/payments",
                                       {"amount_xof": total, "method": "especes"})
    check("la fiche se règle en une fois quand les acomptes sont refusés",
          code == 200, f"http={code} {erreur(encaisse)}")
    code, livree = proprietaire.call("PATCH", f"/wash-orders/{fiche_id}", {"status": "completed"})
    check("la remise du véhicule est acceptée une fois la fiche soldée",
          code == 200, f"http={code} {erreur(livree)}")

    # d) Une fiche soldée ne s'annule pas : c'est le contrôle qualité de la
    #    caisse, pas une case à cocher.
    code, annulee = proprietaire.call("PATCH", f"/wash-orders/{fiche_id}",
                                      {"status": "canceled", "reason": "Erreur de saisie"})
    check("une fiche encaissée ne s'annule pas (409 attendu)", code == 409,
          f"http={code} {erreur(annulee)}")

    # e) Annulation d'une fiche non encaissée : motif obligatoire, état final.
    apply_code, _ = appliquer(require_cancel_reason=True)
    code, deuxieme = nouvelle_fiche()
    deuxieme_id = deuxieme.get("id", "") if isinstance(deuxieme, dict) else ""
    code, sans_motif = proprietaire.call("PATCH", f"/wash-orders/{deuxieme_id}", {"status": "canceled"})
    check("annulation sans motif refusée", code == 400, f"http={code} {erreur(sans_motif)}")
    code, annulee = proprietaire.call("PATCH", f"/wash-orders/{deuxieme_id}",
                                      {"status": "canceled", "reason": "Client parti"})
    check("annulation motivée acceptée", code == 200, f"http={code} {erreur(annulee)}")
    code, apres = proprietaire.call("PATCH", f"/wash-orders/{deuxieme_id}", {"status": "in_progress"})
    check("une fiche annulée ne repart pas (409 attendu)", code == 409, f"http={code} {erreur(apres)}")

    # f) Le plafond de fiches par laveur : la règle se vérifie là où le laveur
    #    est affecté, et se libère à l'annulation.
    appliquer(require_cancel_reason=False, max_open_orders_per_washer=1)
    code, premiere = nouvelle_fiche({"employee_id": employe_id})
    check("première fiche affectée au laveur", code == 201, f"http={code} {erreur(premiere)}")
    code, trop = nouvelle_fiche({"employee_id": employe_id})
    check("plafond atteint ⇒ 409 washer_queue_full",
          code == 409 and erreur(trop) == "washer_queue_full", f"http={code} {erreur(trop)}")
    if isinstance(premiere, dict) and premiere.get("id"):
        proprietaire.call("PATCH", f"/wash-orders/{premiere['id']}",
                          {"status": "canceled", "reason": "Doublon sonde"})
        code, libere = nouvelle_fiche({"employee_id": employe_id})
        check("l'annulation libère la place du laveur", code == 201, f"http={code} {erreur(libere)}")

    # g) Cloisonnement : l'entreprise voisine ne voit rien de cette station.
    voisine = Client("voisine")
    voisine.inscrire("Sonde Lot 7 Voisine")
    code, _ = voisine.call("GET", base, None)
    check("le matériel d'une autre entreprise est invisible (404 attendu)",
          code == 404, f"http={code}")

    print()
    failed = [name for name, ok_, _ in RESULTS if not ok_]
    print(f"{len(RESULTS) - len(failed)}/{len(RESULTS)} contrôles au vert")
    for name in failed:
        print("  KO :", name)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
