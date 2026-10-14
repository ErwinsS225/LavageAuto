package main

import (
	"strings"
	"testing"
)

// ===========================================================================
// Lot 6 — caisse, moyens de paiement et reçus.
//
// Ce que ces tests démontrent, dans l'ordre où le comptoir le vit :
//
//  1. une station neuve reçoit le référentiel des moyens de paiement, les
//     espèces seules étant actives : on encaisse le premier jour sans rien
//     configurer, et rien d'autre n'est accepté d'office ;
//  2. un moyen désactivé est **refusé** (`409 payment_method_disabled`) ; un
//     moyen à référence ne passe pas sans sa référence ;
//  3. le moyen déclaré est recopié sur l'encaissement, et c'est sa **famille**
//     qui décide du compteur de caisse (renommer ne change aucun total) ;
//  4. un paiement **différé** n'est pas un encaissement : la fiche reste due,
//     aucun compteur ne bouge, aucun reçu n'est émis ;
//  5. le reçu reçoit un numéro unique, une seule fois, quand la fiche est
//     soldée — et n'imprime que les mentions réellement saisies ;
//  6. les paramètres de caisse sont appliqués : rôles autorisés, écart toléré
//     avant justification écrite, annulation ouverte ou fermée, espèces hors
//     session ;
//  7. aucun moyen de paiement ni aucune configuration ne franchit la frontière
//     d'une entreprise.
// ===========================================================================

// moyenParCode retrouve un moyen de paiement déclaré par son code de référentiel.
func (b *banc) moyenParCode(c *client, station, code string) map[string]any {
	c.t.Helper()
	statut, rep := c.appel("GET", "/payment-methods?station_id="+station, nil)
	if statut != 200 {
		c.t.Fatalf("GET /payment-methods : http=%d %v", statut, rep)
	}
	for _, brut := range rep["items"].([]any) {
		moyen, _ := brut.(map[string]any)
		if texte(moyen, "code") == code {
			return moyen
		}
	}
	c.t.Fatalf("moyen de paiement %q absent du référentiel de la station", code)
	return nil
}

// activerMoyen ouvre un moyen de paiement dans les paramètres de la station.
func (b *banc) activerMoyen(c *client, station, code string) map[string]any {
	c.t.Helper()
	moyen := b.moyenParCode(c, station, code)
	statut, rep := c.appel("PATCH", "/payment-methods/"+texte(moyen, "id"), map[string]any{
		"station_id": station, "is_active": true,
	})
	if statut != 200 {
		c.t.Fatalf("activation du moyen %q : http=%d %v", code, statut, rep)
	}
	return rep
}

// payer encaisse une fiche (route de la caisse) et rend le code HTTP et la
// réponse — la fiche y est enveloppée sous « order ».
func (c *client) payer(ficheID string, corps map[string]any) (int, map[string]any) {
	c.t.Helper()
	return c.appel("POST", "/wash-orders/"+ficheID+"/payments", corps)
}

// solde relit le solde d'une fiche comme l'écran de caisse : sommes reçues et
// reste dû. C'est la mesure qui compte — un paiement différé ne doit pas la
// faire bouger.
func (b *banc) solde(c *client, ficheID string) (paid, due int) {
	c.t.Helper()
	statut, rep := c.appel("GET", "/wash-orders/"+ficheID, nil)
	if statut != 200 {
		c.t.Fatalf("GET /wash-orders/%s : http=%d %v", ficheID, statut, rep)
	}
	fiche := depaqueter(rep)
	return entier(fiche["paid_xof"]), entier(fiche["due_xof"])
}

// sessionOuverte relit la session de caisse ouverte de la station.
func (b *banc) sessionOuverte(c *client, station string) map[string]any {
	c.t.Helper()
	statut, rep := c.appel("GET", "/cash/current?station_id="+station, nil)
	if statut != 200 {
		c.t.Fatalf("GET /cash/current : http=%d %v", statut, rep)
	}
	session, _ := rep["session"].(map[string]any)
	if session == nil {
		c.t.Fatalf("aucune session de caisse ouverte : %v", rep)
	}
	return session
}

func TestMoyensPaiementAmorcesEtDesactivesRefuses(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Moyens")
	c.activer()
	station, service, prix := b.creerStationEtPrestation(c)

	// 1. Le référentiel est posé, les espèces seules sont actives.
	statut, referentiel := c.appel("GET", "/payment-methods?station_id="+station, nil)
	if statut != 200 {
		t.Fatalf("GET /payment-methods : http=%d %v", statut, referentiel)
	}
	if total := entier(referentiel["total"]); total != 9 {
		t.Errorf("le référentiel doit compter 9 moyens, obtenu %d", total)
	}
	if actifs := entier(referentiel["actifs"]); actifs != 1 {
		t.Errorf("seules les espèces doivent être actives, obtenu %d moyen(s) actif(s)", actifs)
	}
	especes := b.moyenParCode(c, station, "especes")
	if !vrai(especes, "is_active") || texte(especes, "family") != "cash" {
		t.Errorf("les espèces doivent être actives et de famille « cash » : %v", especes)
	}
	orange := b.moyenParCode(c, station, "orange_money")
	if vrai(orange, "is_active") {
		t.Errorf("Orange Money ne doit pas être actif par défaut : %v", orange)
	}
	if !vrai(orange, "requires_reference") {
		t.Errorf("un moyen mobile money doit exiger une référence : %v", orange)
	}
	if statut, rep := c.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 20_000,
	}); statut != 201 {
		t.Fatalf("ouverture de caisse : http=%d %v", statut, rep)
	}

	// 2. Un moyen désactivé est refusé, explicitement, et la fiche reste due.
	fiche := b.fiche(c, map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	}, 201)
	ficheID := texte(fiche, "id")
	statut, refus := c.payer(ficheID, map[string]any{
		"amount_xof": prix, "payment_method_id": texte(orange, "id"), "reference": "TRX-1",
	})
	enveloppe, _ := refus["error"].(map[string]any)
	if statut != 409 || texte(enveloppe, "code") != "payment_method_disabled" {
		t.Fatalf("un moyen désactivé doit être refusé (409 payment_method_disabled) : http=%d %v", statut, refus)
	}
	if paid, due := b.solde(c, ficheID); paid != 0 || due != prix {
		t.Fatalf("la fiche doit rester due après un refus (encaissé=%d, dû=%d)", paid, due)
	}

	// 3. Activé sans référence : refusé (un mobile money sans numéro de
	//    transaction n'est pas vérifiable) ; avec référence : accepté.
	orange = b.activerMoyen(c, station, "orange_money")
	statut, refus = c.payer(ficheID, map[string]any{
		"amount_xof": prix, "payment_method_id": texte(orange, "id"),
	})
	if statut != 400 {
		t.Fatalf("une référence manquante doit être refusée (400) : http=%d %v", statut, refus)
	}
	statut, encaisse := c.payer(ficheID, map[string]any{
		"amount_xof": prix, "payment_method_id": texte(orange, "id"), "reference": "OM-4471",
	})
	if statut != 200 {
		t.Fatalf("encaissement mobile money : http=%d %v", statut, encaisse)
	}
	if s := texte(depaqueter(encaisse), "status"); s != "completed" {
		t.Errorf("la fiche soldée doit être terminée, statut obtenu %q", s)
	}

	// 4. Le compteur alimenté est celui de la **famille**, pas du libellé.
	session := b.sessionOuverte(c, station)
	if v := entier(session["mobile_money_xof"]); v != prix {
		t.Errorf("mobile money encaissé : attendu %d, obtenu %d", prix, v)
	}
	if v := entier(session["cash_in_xof"]); v != 0 {
		t.Errorf("les espèces ne doivent pas bouger : obtenu %d", v)
	}
	if v := entier(session["other_in_xof"]); v != 0 {
		t.Errorf("le mobile money ne doit pas tomber dans « autres encaissements » : obtenu %d", v)
	}
	if v := entier(session["payment_count"]); v != 1 {
		t.Errorf("un seul paiement attendu dans la session : obtenu %d", v)
	}

	// 5. Le moyen déclaré est recopié sur l'encaissement (le journal du
	//    comptoir affiche « Orange Money », pas « mobile_money »).
	statut, journal := c.appel("GET", "/cash/payments?station_id="+station, nil)
	if statut != 200 {
		t.Fatalf("GET /cash/payments : http=%d %v", statut, journal)
	}
	items, _ := journal["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("le journal des encaissements est vide : %v", journal)
	}
	premier, _ := items[0].(map[string]any)
	if nom := texte(premier, "payment_method_name"); nom != "Orange Money" {
		t.Errorf("le moyen déclaré doit être recopié : obtenu %q", nom)
	}
	if famille := texte(premier, "method"); famille != "mobile_money" {
		t.Errorf("la famille canonique doit rester la référence comptable : obtenu %q", famille)
	}

	// 6. On ne désactive pas le dernier moyen actif : la caisse resterait muette.
	if statut, rep := c.appel("PATCH", "/payment-methods/"+texte(orange, "id"), map[string]any{
		"station_id": station, "is_active": false,
	}); statut != 200 {
		t.Errorf("Orange Money doit pouvoir être désactivé quand les espèces restent actives : http=%d %v", statut, rep)
	}
	if statut, rep := c.appel("PATCH", "/payment-methods/"+texte(especes, "id"), map[string]any{
		"station_id": station, "is_active": false,
	}); statut != 409 {
		t.Errorf("désactiver le dernier moyen actif doit être refusé (409) : http=%d %v", statut, rep)
	}

	// 7. La configuration des moyens est réservée au gérant : un caissier lit la
	//    liste mais ne la modifie pas.
	caissier := b.creerEmploye(c, station, "cashier")
	if statut, rep := caissier.appel("GET", "/payment-methods?station_id="+station, nil); statut != 200 {
		t.Errorf("un caissier doit pouvoir lire les moyens de paiement : http=%d %v", statut, rep)
	}
	if statut, rep := caissier.appel("PATCH", "/payment-methods/"+texte(especes, "id"), map[string]any{
		"station_id": station, "is_active": true,
	}); statut != 403 {
		t.Errorf("un caissier ne doit pas pouvoir modifier les moyens de paiement : http=%d %v", statut, rep)
	}
}

func TestMoyensPaiementIsolesParEntreprise(t *testing.T) {
	b := nouveauBanc(t)
	voisine := b.client()
	voisine.inscrire("Entreprise Voisine")
	voisine.activer()
	stationVoisine, _, _ := b.creerStationEtPrestation(voisine)
	moyenVoisin := b.moyenParCode(voisine, stationVoisine, "virement")

	c := b.client()
	c.inscrire("Entreprise Isolee")
	c.activer()
	station, service, prix := b.creerStationEtPrestation(c)
	if statut, rep := c.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 10_000,
	}); statut != 201 {
		t.Fatalf("ouverture de caisse : http=%d %v", statut, rep)
	}

	// Le moyen d'une autre entreprise est introuvable : l'encaissement est
	// refusé, la fiche reste due.
	fiche := b.fiche(c, map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	}, 201)
	statut, rep := c.payer(texte(fiche, "id"), map[string]any{
		"amount_xof": prix, "payment_method_id": texte(moyenVoisin, "id"),
	})
	if statut != 400 {
		t.Fatalf("un moyen d'une autre entreprise doit être refusé (400) : http=%d %v", statut, rep)
	}
	if _, due := b.solde(c, texte(fiche, "id")); due != prix {
		t.Errorf("la fiche doit rester due de %d, obtenu %d", prix, due)
	}

	// Sa modification ou sa désactivation par un tiers ne franchit pas la
	// frontière non plus.
	if statut, rep := c.appel("DELETE",
		"/payment-methods/"+texte(moyenVoisin, "id")+"?station_id="+station, nil); statut != 404 {
		t.Errorf("un moyen d'une autre entreprise doit être introuvable (404) : http=%d %v", statut, rep)
	}
	if vrai(b.moyenParCode(voisine, stationVoisine, "virement"), "is_active") {
		t.Error("le moyen de l'entreprise voisine ne doit pas avoir été touché")
	}
}

func TestDiffereNEstPasUnEncaissement(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Differe")
	c.activer()
	station, service, prix := b.creerStationEtPrestation(c)
	if statut, rep := c.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 50_000,
	}); statut != 201 {
		t.Fatalf("ouverture de caisse : http=%d %v", statut, rep)
	}

	fiche := b.fiche(c, map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	}, 201)
	ficheID := texte(fiche, "id")

	// Un caissier n'accorde pas de crédit : la permission lui manque.
	caissier := b.creerEmploye(c, station, "cashier")
	statut, rep := caissier.payer(ficheID, map[string]any{
		"amount_xof": prix, "method": "differe", "reference": "PROMESSE-1",
	})
	if statut != 403 {
		t.Fatalf("un caissier ne doit pas pouvoir différer un paiement (403) : http=%d %v", statut, rep)
	}

	// Le gérant peut l'enregistrer — comme engagement, pas comme encaissement.
	gerant := b.creerEmploye(c, station, "manager")
	statut, rep = gerant.payer(ficheID, map[string]any{
		"amount_xof": prix, "method": "differe", "reference": "PROMESSE-1",
	})
	if statut != 200 {
		t.Fatalf("un gérant doit pouvoir enregistrer un paiement différé : http=%d %v", statut, rep)
	}
	if s := texte(depaqueter(rep), "status"); s == "completed" {
		t.Errorf("une fiche différée ne doit pas être terminée : statut %q", s)
	}

	// Aucun compteur de caisse n'a bougé, et la fiche reste due.
	session := b.sessionOuverte(c, station)
	for _, cle := range []string{"cash_in_xof", "mobile_money_xof", "other_in_xof", "payment_count"} {
		if v := entier(session[cle]); v != 0 {
			t.Errorf("« %s » doit rester à 0 après un différé : obtenu %d", cle, v)
		}
	}
	if paid, due := b.solde(c, ficheID); paid != 0 || due != prix {
		t.Errorf("une fiche différée doit rester due de %d (encaissé=%d, dû=%d)", prix, paid, due)
	}

	// Aucun reçu n'est émis : rien n'a été payé.
	if statut, rep := c.appel("GET", "/wash-orders/"+ficheID+"/receipt", nil); statut != 404 {
		t.Errorf("un différé ne doit pas émettre de reçu (404 attendu) : http=%d %v", statut, rep)
	}
	if statut, rep := c.appel("POST", "/wash-orders/"+ficheID+"/receipt", nil); statut != 409 {
		t.Errorf("un reçu ne s'émet pas sur une fiche non soldée (409 attendu) : http=%d %v", statut, rep)
	}

	// Le rapport ne compte pas le différé dans les encaissements, mais le
	// montre à part.
	statut, rapport := c.appel("GET", "/reports/overview?station_id="+station, nil)
	if statut != 200 {
		t.Fatalf("GET /reports/overview : http=%d %v", statut, rapport)
	}
	totaux, _ := rapport["totals"].(map[string]any)
	if v := entier(totaux["payments_xof"]); v != 0 {
		t.Errorf("les encaissements de la période doivent être nuls : obtenu %d", v)
	}
	if v := entier(totaux["deferred_xof"]); v != prix {
		t.Errorf("les engagements différés doivent apparaître (%d) : obtenu %d", prix, v)
	}
	if v := entier(totaux["other_xof"]); v != 0 {
		t.Errorf("un différé ne doit pas tomber dans « autres encaissements » : obtenu %d", v)
	}

	// Le client règle finalement : la fiche est soldée, le reçu est émis.
	statut, rep = gerant.payer(ficheID, map[string]any{"amount_xof": prix, "method": "especes"})
	if statut != 200 {
		t.Fatalf("règlement effectif : http=%d %v", statut, rep)
	}
	if s := texte(depaqueter(rep), "status"); s != "completed" {
		t.Errorf("la fiche réglée doit être terminée : statut %q", s)
	}
	statut, repRecu := c.appel("GET", "/wash-orders/"+ficheID+"/receipt", nil)
	if statut != 200 {
		t.Fatalf("le reçu doit exister après règlement : http=%d %v", statut, repRecu)
	}
	recu, _ := repRecu["receipt"].(map[string]any)
	numero := texte(recu, "number")
	if !strings.HasPrefix(numero, "REC-") {
		t.Errorf("numéro de reçu inattendu : %q", numero)
	}
	// Un reçu ne change pas de numéro : on le réémet, il garde le sien.
	statut, repRecu = c.appel("POST", "/wash-orders/"+ficheID+"/receipt", nil)
	if statut != 201 {
		t.Fatalf("réémission du reçu : http=%d %v", statut, repRecu)
	}
	relu, _ := repRecu["receipt"].(map[string]any)
	if texte(relu, "number") != numero {
		t.Errorf("le numéro de reçu a changé : %q puis %q", numero, texte(relu, "number"))
	}
	// Le reçu n'atteste que des sommes réellement reçues.
	if v := entier(recu["paid_xof"]); v != prix {
		t.Errorf("sommes reçues sur le reçu : attendu %d, obtenu %d", prix, v)
	}
	if v := entier(recu["due_xof"]); v != 0 {
		t.Errorf("le reçu ne doit pas porter de reste dû : obtenu %d", v)
	}
	if reglements, _ := recu["payments"].([]any); len(reglements) != 1 {
		t.Errorf("le reçu ne doit lister que le règlement effectif : %v", recu["payments"])
	}
}

func TestRecuNumerotationEtMentionsSaisies(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Recu")
	c.activer()
	station, service, prix := b.creerStationEtPrestation(c)
	if statut, rep := c.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 30_000,
	}); statut != 201 {
		t.Fatalf("ouverture de caisse : http=%d %v", statut, rep)
	}

	encaisser := func() map[string]any {
		t.Helper()
		fiche := b.fiche(c, map[string]any{
			"station_id": station,
			"items":      []map[string]any{{"service_id": service, "quantity": 1}},
		}, 201)
		ficheID := texte(fiche, "id")
		if statut, rep := c.payer(ficheID, map[string]any{
			"amount_xof": prix, "method": "especes",
		}); statut != 200 {
			t.Fatalf("encaissement espèces : http=%d %v", statut, rep)
		}
		statut, repRecu := c.appel("GET", "/wash-orders/"+ficheID+"/receipt", nil)
		if statut != 200 {
			t.Fatalf("reçu de la fiche : http=%d %v", statut, repRecu)
		}
		recu, _ := repRecu["receipt"].(map[string]any)
		return recu
	}

	premier := encaisser()
	if numero := texte(premier, "number"); !strings.HasSuffix(numero, "-0001") {
		t.Errorf("le premier reçu du jour doit être 0001 : %q", numero)
	}
	second := encaisser()
	if numero := texte(second, "number"); !strings.HasSuffix(numero, "-0002") {
		t.Errorf("le deuxième reçu du jour doit être 0002 : %q", numero)
	}

	// Rien n'est inventé : les mentions absentes sont annoncées comme telles.
	mentions := func(recu map[string]any) []string {
		liste := []string{}
		for _, brut := range recu["missing_mentions"].([]any) {
			if libelle, ok := brut.(string); ok {
				liste = append(liste, libelle)
			}
		}
		return liste
	}
	if texte(premier, "legal_name") != "" || texte(premier, "tax_id") != "" {
		t.Errorf("aucune mention fiscale ne doit être inventée : %v", premier)
	}
	if !contient(mentions(premier), "identifiant fiscal") {
		t.Errorf("l'absence d'identifiant fiscal doit être annoncée : %v", mentions(premier))
	}

	// L'exploitant saisit son identité de reçu : elle apparaît telle quelle.
	statut, reglages := c.appel("PUT", "/stations/"+station+"/cash-settings", map[string]any{
		"receipt_prefix":     "lav",
		"receipt_legal_name": "SARL LAVAGE EXPRESS",
		"receipt_tax_id":     "CI-ABJ-2026-A-12345",
		"receipt_footer":     "Merci de votre confiance",
	})
	if statut != 200 {
		t.Fatalf("enregistrement des paramètres de caisse : http=%d %v", statut, reglages)
	}
	relus, _ := reglages["settings"].(map[string]any)
	if texte(relus, "receipt_prefix") != "lav" {
		t.Errorf("le préfixe saisi doit être conservé : %v", relus)
	}

	troisieme := encaisser()
	if numero := texte(troisieme, "number"); !strings.HasPrefix(numero, "lav-") {
		t.Errorf("le préfixe saisi doit apparaître dans le numéro : %q", numero)
	}
	if texte(troisieme, "legal_name") != "SARL LAVAGE EXPRESS" ||
		texte(troisieme, "tax_id") != "CI-ABJ-2026-A-12345" {
		t.Errorf("le reçu doit porter les mentions saisies : %v", troisieme)
	}
	if contient(mentions(troisieme), "raison sociale") || contient(mentions(troisieme), "identifiant fiscal") {
		t.Errorf("les mentions saisies ne doivent plus être signalées absentes : %v", mentions(troisieme))
	}
	if texte(troisieme, "footer") != "Merci de votre confiance" {
		t.Errorf("le bas de reçu doit être repris : %v", troisieme)
	}

	// Une mention vidée disparaît du reçu : elle n'est remplacée par rien.
	if statut, rep := c.appel("PUT", "/stations/"+station+"/cash-settings", map[string]any{
		"receipt_legal_name": "",
		"receipt_tax_id":     "",
	}); statut != 200 {
		t.Fatalf("vidage des mentions fiscales : http=%d %v", statut, rep)
	}
	quatrieme := encaisser()
	if texte(quatrieme, "legal_name") != "" || texte(quatrieme, "tax_id") != "" {
		t.Errorf("une mention vidée ne doit pas réapparaître : %v", quatrieme)
	}
	if !contient(mentions(quatrieme), "raison sociale") {
		t.Errorf("une mention vidée doit être annoncée absente : %v", mentions(quatrieme))
	}
}

func TestParametresCaisseAppliques(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Parametres")
	c.activer()
	station, service, prix := b.creerStationEtPrestation(c)

	statut, rep := c.appel("GET", "/stations/"+station+"/cash-settings", nil)
	if statut != 200 {
		t.Fatalf("GET /cash-settings : http=%d %v", statut, rep)
	}
	defauts, _ := rep["settings"].(map[string]any)
	if !vrai(defauts, "require_open_cash_session") || !vrai(defauts, "allow_void") {
		t.Errorf("les valeurs par défaut doivent préserver l'existant : %v", defauts)
	}
	if v := entier(defauts["discrepancy_tolerance_xof"]); v != 0 {
		t.Errorf("aucune tolérance d'écart n'est supposée par défaut : obtenu %d", v)
	}
	if v := entier(defauts["max_cashier_discount_xof"]); v != 1000 {
		t.Errorf("le plafond de remise du caissier existant doit être repris : obtenu %d", v)
	}

	// 1. Rôles autorisés : la caisse ne s'ouvre qu'à qui la station l'a dit.
	statut, rep = c.appel("PUT", "/stations/"+station+"/cash-settings", map[string]any{
		"allowed_roles": []string{"manager"}, "discrepancy_tolerance_xof": 500,
		"receipt_prefix": "REC",
	})
	if statut != 200 {
		t.Fatalf("PUT /cash-settings : http=%d %v", statut, rep)
	}
	caissier := b.creerEmploye(c, station, "cashier")
	statut, refus := caissier.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 10_000,
	})
	enveloppe, _ := refus["error"].(map[string]any)
	if statut != 403 || texte(enveloppe, "code") != "cash_role_forbidden" {
		t.Fatalf("un rôle non autorisé ne doit pas ouvrir la caisse (403 cash_role_forbidden) : http=%d %v", statut, refus)
	}

	gerant := b.creerEmploye(c, station, "manager")
	if statut, rep := gerant.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 10_000,
	}); statut != 201 {
		t.Fatalf("le gérant autorisé doit pouvoir ouvrir la caisse : http=%d %v", statut, rep)
	}
	sessionID := texte(b.sessionOuverte(c, station), "id")

	// 2. Écart hors tolérance : la clôture exige un motif écrit.
	statut, refus = gerant.appel("POST", "/cash/sessions/"+sessionID+"/close", map[string]any{
		"counted_cash_xof": 12_000, // 2 000 FCFA de plus que le théorique
	})
	if statut != 400 {
		t.Fatalf("un écart hors tolérance sans motif doit être refusé (400) : http=%d %v", statut, refus)
	}
	statut, cloture := gerant.appel("POST", "/cash/sessions/"+sessionID+"/close", map[string]any{
		"counted_cash_xof": 12_000, "note": "Erreur de rendu de monnaie, justifiée",
	})
	if statut != 200 {
		t.Fatalf("clôture justifiée : http=%d %v", statut, cloture)
	}
	if v := entier(cloture["difference_xof"]); v != 2_000 {
		t.Errorf("écart de clôture : attendu 2000, obtenu %d", v)
	}

	// 3. Annulations fermées : le gérant ne peut plus contre-passer, et le
	//    message renvoie aux paramètres.
	if statut, rep := c.appel("PUT", "/stations/"+station+"/cash-settings", map[string]any{
		"allow_void": false,
	}); statut != 200 {
		t.Fatalf("fermeture des annulations : http=%d %v", statut, rep)
	}
	if statut, rep := c.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 5_000,
	}); statut != 201 {
		t.Fatalf("réouverture d'une session : http=%d %v", statut, rep)
	}
	fiche := b.fiche(c, map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	}, 201)
	ficheID := texte(fiche, "id")
	statut, encaisse := c.payer(ficheID, map[string]any{"amount_xof": prix, "method": "especes"})
	if statut != 200 {
		t.Fatalf("encaissement : http=%d %v", statut, encaisse)
	}
	// L'identifiant de l'encaissement se relit dans le journal, comme l'écran
	// de caisse le fait pour proposer l'annulation.
	statut, journal := c.appel("GET", "/cash/payments?station_id="+station, nil)
	if statut != 200 {
		t.Fatalf("GET /cash/payments : http=%d %v", statut, journal)
	}
	items, _ := journal["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("journal vide : %v", journal)
	}
	ligne, _ := items[0].(map[string]any)
	paiement := texte(ligne, "id")

	statut, refus = c.appel("POST", "/cash/payments/"+paiement+"/void", map[string]any{
		"reason": "Client s'est trompé de montant",
	})
	enveloppe, _ = refus["error"].(map[string]any)
	if statut != 409 || texte(enveloppe, "code") != "void_not_allowed" {
		t.Fatalf("une annulation fermée par les paramètres doit être refusée (409 void_not_allowed) : http=%d %v", statut, refus)
	}

	// 4. Annulations rouvertes : la contrepassation repasse et redescend le
	//    compteur d'espèces.
	if statut, rep := c.appel("PUT", "/stations/"+station+"/cash-settings", map[string]any{
		"allow_void": true,
	}); statut != 200 {
		t.Fatalf("réouverture des annulations : http=%d %v", statut, rep)
	}
	statut, annule := c.appel("POST", "/cash/payments/"+paiement+"/void", map[string]any{
		"reason": "Client s'est trompé de montant",
	})
	if statut != 200 {
		t.Fatalf("annulation : http=%d %v", statut, annule)
	}
	session := b.sessionOuverte(c, station)
	if v := entier(session["cash_in_xof"]); v != 0 {
		t.Errorf("les espèces encaissées doivent redescendre à 0 : obtenu %d", v)
	}
	if v := entier(session["payment_count"]); v != 0 {
		t.Errorf("le compteur de paiements doit redescendre à 0 : obtenu %d", v)
	}
	if paid, due := b.solde(c, ficheID); paid != 0 || due != prix {
		t.Errorf("une annulation rouvre le solde de la fiche (encaissé=%d, dû=%d)", paid, due)
	}

	// 5. Espèces hors session : la règle se relâche, et l'encaissement passe
	//    sans caisse ouverte.
	if statut, rep := c.appel("PUT", "/stations/"+station+"/cash-settings", map[string]any{
		"require_open_cash_session": false,
	}); statut != 200 {
		t.Fatalf("assouplissement de la règle de caisse : http=%d %v", statut, rep)
	}
	// La session ouverte est celle rouverte à l'étape 3 : on la clôture pour
	// vérifier l'encaissement hors session.
	sessionID = texte(b.sessionOuverte(c, station), "id")
	if statut, rep := c.appel("POST", "/cash/sessions/"+sessionID+"/close", map[string]any{
		"counted_cash_xof": 5_000,
	}); statut != 200 {
		t.Fatalf("clôture de la session courante : http=%d %v", statut, rep)
	}
	statut, sessionCourante := c.appel("GET", "/cash/current?station_id="+station, nil)
	if statut != 200 {
		t.Fatalf("GET /cash/current : http=%d %v", statut, sessionCourante)
	}
	if ouvert, _ := sessionCourante["open"].(bool); ouvert {
		t.Fatalf("la session devait être clôturée : %v", sessionCourante)
	}
	seconde := b.fiche(c, map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	}, 201)
	statut, encaisse = c.payer(texte(seconde, "id"), map[string]any{"amount_xof": prix, "method": "especes"})
	if statut != 200 {
		t.Fatalf("sans règle de session, les espèces doivent passer : http=%d %v", statut, encaisse)
	}
}
