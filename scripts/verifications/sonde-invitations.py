#!/usr/bin/env python3
"""Sonde d'acceptation — invitations : rejoindre une entreprise existante.

Elle interroge une API **déjà démarrée** (par défaut http://127.0.0.1:8080) :
aucune dépendance, uniquement la bibliothèque standard.

    python3 scripts/verifications/sonde-invitations.py [base_url]

Le parcours complet est rejoué : une entreprise en exploitation invite une
caissière, transmet le lien, et l'invitée crée son compte. Ce qui compte ici
n'est pas seulement que ça marche, mais que ça ne donne **rien de plus** que ce
qui a été accordé : le code ne révèle ni identifiant ni données, l'invitée ne
devient pas propriétaire, et l'invitation ne sert qu'une fois.
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

    def inscrire(self, nom, email=None):
        """Inscription classique : l'entreprise est créée par le compte."""
        self.email = email or f"sonde-inv-{uuid.uuid4().hex[:8]}@test.local"
        return self.call("POST", "/auth/register", {
            "organization_name": nom,
            "full_name": "Propriétaire Sonde",
            "email": self.email,
            "phone": "+225 27 00 00 00 00",
            "password": "sonde12345",
            "station_name": f"Station {nom}",
            "city": "Abidjan",
        })

    def mettre_en_route(self, station_id):
        """Déclare catégories + horaires + prestation tarifée, puis active.

        Sans catégorie active, le serveur refuse l'activation : une station doit
        dire ce qu'elle lave avant d'ouvrir (ONB-8.2).
        """
        self.call("PUT", f"/stations/{station_id}/opening-hours", {
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
        self.call("PUT", f"/stations/{station_id}/vehicle-categories", {
            "categories": [{"code": "berline", "is_active": True, "sort_order": 10},
                           {"code": "moto", "is_active": True, "sort_order": 20}],
        })
        # Le catalogue se compose, la grille décide : la prestation naît « à
        # tarifer », le prix se pose dans la grille avec son motif.
        code, service = self.call("POST", "/services", {
            "station_id": station_id, "name": "Lavage simple", "duration_minutes": 30,
        })
        if isinstance(service, dict) and service.get("id"):
            self.call("PUT", "/price-rules/matrix", {
                "station_id": station_id, "reason": "Grille d'acceptation",
                "generic_prices": [{"service_id": service["id"], "price_xof": 2500}],
            })
        return self.call("POST", "/onboarding/activate")


RESULTS = []


def check(nom, ok, detail=""):
    RESULTS.append((nom, ok, detail))
    print(f"{'OK ' if ok else 'KO '} {nom}" + (f" — {detail}" if detail else ""), flush=True)


def code_erreur(charge):
    if isinstance(charge, dict) and isinstance(charge.get("error"), dict):
        return charge["error"].get("code", "")
    return ""


def main():
    # 1. L'entreprise invitante : en exploitation, sinon elle ne peut rien gérer.
    patron = Client()
    code, inscription = patron.inscrire("Sonde Invitante")
    if code != 201:
        print("inscription impossible :", inscription)
        return 1
    station = (inscription.get("station") or {}).get("id", "")
    org = (inscription.get("user") or {}).get("organization_id", "")
    code, etat = patron.mettre_en_route(station)
    check("l'entreprise invitante est en exploitation", code == 200 and etat.get("state") == "active",
          f"http={code} état={etat.get('state') if isinstance(etat, dict) else etat}")
    check("l'inscription par défaut crée bien un espace", bool(org) and bool(station), f"org={bool(org)}")

    # 2. L'invitation d'une caissière.
    email_invitee = f"invitee-{uuid.uuid4().hex[:8]}@test.local"
    code, invitation = patron.call("POST", "/invitations", {"email": email_invitee, "role": "cashier"})
    jeton, code_court = (invitation or {}).get("token", ""), (invitation or {}).get("code", "")
    check("une invitation est créée avec un lien et un code",
          code == 201 and bool(jeton) and len(code_court) == 8,
          f"http={code} code={code_court}")
    check("l'API annonce qu'aucun message n'est envoyé",
          "n'envoie" in (invitation or {}).get("message", ""), (invitation or {}).get("message", "")[:60])

    # 3. L'aperçu public : de quoi reconnaître l'employeur, rien de plus.
    inconnu = Client()
    code, apercu = inconnu.call("GET", f"/invitations/{code_court.lower()}")
    check("un code se saisit indifféremment en minuscules", code == 200, f"http={code}")
    check("l'aperçu nomme l'entreprise et le rôle",
          (apercu or {}).get("organization_name") == "Sonde Invitante"
          and (apercu or {}).get("role") == "cashier"
          and (apercu or {}).get("role_label") == "Caissier",
          f"{apercu.get('organization_name') if isinstance(apercu, dict) else apercu} / "
          f"{apercu.get('role_label') if isinstance(apercu, dict) else ''}")
    intrus = [cle for cle in ("id", "organization_id", "station_id", "user_id", "token")
              if isinstance(apercu, dict) and cle in apercu]
    check("l'aperçu ne livre aucun identifiant", not intrus, f"champs exposés : {intrus}")

    code, refus = inconnu.call("GET", "/invitations/ZZZZZZZZ")
    check("un code inventé ne révèle rien (404)", code == 404 and code_erreur(refus) == "invitation_unknown",
          f"http={code} code={code_erreur(refus)}")

    # 4. Un compte existant n'est pas rattaché de force : le refus est explicite.
    #    Ce contrôle précède l'inscription de l'invitée, sinon l'invitation
    #    serait déjà consommée et le serveur répondrait « invitation_used ».
    code, refus = patron.call("POST", "/auth/register", {
        "invitation_token": jeton, "full_name": "Déjà Là",
        "email": patron.email, "password": "sonde12345",
    })
    check("un compte existant n'est pas rattaché (409 invitation_requires_login)",
          code == 409 and code_erreur(refus) == "invitation_requires_login",
          f"http={code} code={code_erreur(refus)}")

    # 5. L'invitée rejoint l'entreprise.
    invitee = Client()
    code, me = invitee.call("POST", "/auth/register", {
        "invitation_token": jeton, "full_name": "Caissière Invitée",
        "email": email_invitee, "password": "sonde12345",
    })
    utilisateur = (me or {}).get("user") or {}
    check("l'inscription par invitation aboutit",
          code == 201 and utilisateur.get("role") == "cashier",
          f"http={code} rôle={utilisateur.get('role')}")
    check("la réponse annonce l'invitation acceptée", (me or {}).get("invitation_accepted") is True,
          str((me or {}).get("invitation_accepted")))
    check("l'invitée appartient à l'entreprise invitante",
          utilisateur.get("organization_id") == org, f"{utilisateur.get('organization_id')} vs {org}")
    check("l'invitée n'est pas propriétaire", utilisateur.get("role") != "owner", str(utilisateur.get("role")))
    check("l'invitée ne passe pas par la mise en route", not (me or {}).get("onboarding"),
          "aucun parcours à faire")

    # 6. Elle travaille tout de suite : l'entreprise est déjà configurée.
    code, taches = invitee.call("GET", "/washer/tasks")
    check("le poste de travail répond à l'invitée", code in (200, 403),
          f"http={code} (403 = rôle non laveur, l'essentiel est l'absence d'erreur serveur)")
    code, caisse = invitee.call("GET", f"/cash/current?station_id={station}")
    check("la caissière lit la caisse de son entreprise", code == 200, f"http={code}")

    # 7. L'invitation ne sert qu'une fois.
    seconde = Client()
    code, refus = seconde.call("POST", "/auth/register", {
        "invitation_token": jeton, "full_name": "Deuxième Personne",
        "email": f"seconde-{uuid.uuid4().hex[:8]}@test.local", "password": "sonde12345",
    })
    check("une invitation déjà utilisée est refusée (409)",
          code == 409 and code_erreur(refus) == "invitation_used", f"http={code} code={code_erreur(refus)}")

    # 8. La liste de l'entreprise montre l'état réel.
    code, liste = patron.call("GET", "/invitations")
    items = (liste or {}).get("items", []) if isinstance(liste, dict) else []
    acceptee = next((i for i in items if i.get("id") == (invitation or {}).get("id")), {})
    check("la liste marque l'invitation « Acceptée »",
          acceptee.get("state") == "accepted" and acceptee.get("state_label") == "Acceptée",
          f"état={acceptee.get('state_label')}")
    check("le jeton n'est jamais relu", "token" not in acceptee and "token_hash" not in acceptee,
          f"champs : {sorted(acceptee)[:8]}")
    check("le rôle du propriétaire est intact",
          ((patron.call("GET", "/auth/me")[1] or {}).get("user") or {}).get("role") == "owner",
          "owner")

    # 9. Cloisonnement : une autre entreprise ne voit ni ne révoque cette invitation.
    voisine = Client()
    code, _ = voisine.inscrire("Sonde Voisine")
    check("la seconde entreprise s'inscrit", code == 201, f"http={code}")
    code, liste = voisine.call("GET", "/invitations")
    check("elle ne voit aucune invitation étrangère", not (liste or {}).get("items", []),
          f"{len((liste or {}).get('items', []))} invitation(s)")
    code, _ = voisine.call("POST", f"/invitations/{(invitation or {}).get('id')}/revoke")
    check("elle ne peut pas révoquer l'invitation d'autrui (404)", code == 404, f"http={code}")

    print()
    failed = [nom for nom, ok, _ in RESULTS if not ok]
    print(f"{len(RESULTS) - len(failed)}/{len(RESULTS)} contrôles au vert")
    for nom in failed:
        print("  KO :", nom)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
