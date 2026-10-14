package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Lot 9 — ONB-9.1 : batterie d'isolation entre entreprises.
//
// Chaque entité née des lots 1 à 8 est attaquée depuis une **autre** entreprise,
// en lecture comme en écriture. Deux choses sont vérifiées à chaque fois :
//
//  1. la réponse ne livre rien (`404` — la ressource n'existe pas pour ce
//     compte) et n'accepte rien (`403` quand la permission manque) ;
//  2. la ressource de la victime est **intacte** après l'attaque : une tentative
//     refusée qui aurait modifié quelque chose serait pire qu'un `200`.
//
// Les deux entreprises sont activées : sans cela, les gardes d'onboarding
// répondraient `409` avant le contrôle d'isolation, et le test passerait pour
// une mauvaise raison.
// ---------------------------------------------------------------------------

// ressourcesVictime : ce qu'une entreprise met en route, avec les identifiants
// que l'attaquant va essayer d'utiliser.
type ressourcesVictime struct {
	station, service, option, regle, moyen string
	equipement, article, employe, fiche    string
	session, invitation                    string
}

// TestIsolationDesEntitesDesLots1A8 : la batterie complète.
func TestIsolationDesEntitesDesLots1A8(t *testing.T) {
	b := nouveauBanc(t)

	victime := b.client()
	victime.inscrire("Entreprise Victime")
	victime.remplirProfilEntreprise()
	victime.declarerHoraires()
	victime.declarerCategories("berline")
	victime.assurerPrestationTarifee()
	victime.activer()

	station, service, _ := b.creerStationEtPrestation(victime)
	r := ressourcesVictime{station: station, service: service}

	// --- les ressources de la victime -------------------------------------
	code, option := victime.appel("POST", "/service-options", map[string]any{
		"station_id": station, "service_id": service, "name": "Cire de finition", "price_xof": 1500,
	})
	if code != 201 {
		t.Fatalf("création de l'option : http=%d %v", code, option)
	}
	r.option = texte(option, "id")

	code, regle := victime.appel("POST", "/price-rules", map[string]any{
		"station_id": station, "service_id": service, "category_code": "berline",
		"price_xof": 3500, "reason": "Tarif berline de la victime",
	})
	if code != 201 {
		t.Fatalf("création de la règle de prix : http=%d %v", code, regle)
	}
	r.regle = texte(regle, "id")

	code, moyen := victime.appel("POST", "/payment-methods", map[string]any{
		"station_id": station, "name": "Wave de la victime", "family": "mobile_money",
	})
	if code != 201 {
		t.Fatalf("création du moyen de paiement : http=%d %v", code, moyen)
	}
	r.moyen = texte(moyen, "id")

	code, equipement := victime.appel("POST", "/stations/"+station+"/equipment", map[string]any{
		"name": "Aspirateur de la victime", "quantity": 1,
	})
	if code != 201 {
		t.Fatalf("création de l'équipement : http=%d %v", code, equipement)
	}
	r.equipement = texte(equipement, "id")

	code, article := victime.appel("POST", "/stations/"+station+"/stock/items", map[string]any{
		"name": "Shampoing de la victime", "unit": "litre", "quantity_known": true, "quantity": 12,
	})
	if code != 201 {
		t.Fatalf("création de l'article de stock : http=%d %v", code, article)
	}
	r.article = texte(article, "id")

	employe := b.creerEmploye(victime, station, "washer")
	code, ficheEmploye := victime.appel("GET", "/employees", nil)
	if code != 200 {
		t.Fatalf("GET /employees : http=%d", code)
	}
	for _, brut := range listeDe(ficheEmploye, "items") {
		personne, _ := brut.(map[string]any)
		if texte(personne, "email") == employe.email {
			r.employe = texte(personne, "id")
		}
	}
	if r.employe == "" {
		t.Fatal("l'employé de la victime est introuvable")
	}

	invitation := b.creerInvitation(victime, map[string]any{
		"station_id": station, "email": fmt.Sprintf("invite-%d@test.local", time.Now().UnixNano()),
		"role": "cashier",
	})
	r.invitation = texte(invitation, "id")

	code, session := victime.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 10000,
	})
	if code != 201 {
		t.Fatalf("ouverture de caisse : http=%d %v", code, session)
	}
	r.session = texte(session, "id")

	code, fiche := victime.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("création de la fiche : http=%d %v", code, fiche)
	}
	r.fiche = texte(fiche, "id")

	// --- l'attaquant : une entreprise voisine, elle aussi activée ----------
	attaquant := b.client()
	attaquant.inscrire("Entreprise Attaquante")
	attaquant.remplirProfilEntreprise()
	attaquant.declarerHoraires()
	attaquant.declarerCategories("berline")
	attaquant.assurerPrestationTarifee()
	attaquant.activer()
	if attaquant.orgID == victime.orgID {
		t.Fatal("les deux entreprises doivent être différentes")
	}

	// --- la batterie -------------------------------------------------------
	tentatives := []struct {
		quoi            string
		methode, chemin string
		corps           map[string]any
	}{
		{"lire le profil de la station", "GET", "/stations/" + r.station + "/profile", nil},
		{"modifier la station", "PATCH", "/stations/" + r.station, map[string]any{"trade_name": "Détournée"}},
		{"réécrire les horaires", "PUT", "/stations/" + r.station + "/opening-hours",
			map[string]any{"days": []map[string]any{}}},
		{"réécrire les catégories acceptées", "PUT", "/stations/" + r.station + "/vehicle-categories",
			map[string]any{"categories": []map[string]any{}}},
		{"réécrire les usages", "PUT", "/stations/" + r.station + "/vehicle-usages",
			map[string]any{"usages": []map[string]any{}}},
		{"lire les règles de commande", "GET", "/stations/" + r.station + "/order-rules", nil},
		{"réécrire les règles de commande", "PUT", "/stations/" + r.station + "/order-rules",
			map[string]any{"auto_close_on_payment": false}},
		{"lire les réglages de caisse", "GET", "/stations/" + r.station + "/cash-settings", nil},
		{"réécrire les réglages de caisse", "PUT", "/stations/" + r.station + "/cash-settings",
			map[string]any{"max_cashier_discount_xof": 999999}},
		{"modifier la prestation", "PATCH", "/services/" + r.service, map[string]any{"name": "Détournée"}},
		{"désactiver la prestation", "DELETE", "/services/" + r.service, nil},
		{"réécrire les compatibilités", "PUT", "/services/" + r.service + "/compatibilities",
			map[string]any{"categories": []string{"moto"}}},
		{"modifier l'option", "PATCH", "/service-options/" + r.option, map[string]any{"price_xof": 1}},
		{"désactiver l'option", "DELETE", "/service-options/" + r.option, nil},
		{"modifier la règle de prix", "PATCH", "/price-rules/" + r.regle, map[string]any{"price_xof": 1}},
		{"désactiver la règle de prix", "DELETE", "/price-rules/" + r.regle, nil},
		{"modifier le moyen de paiement", "PATCH", "/payment-methods/" + r.moyen, map[string]any{"name": "Détourné"}},
		{"désactiver le moyen de paiement", "DELETE", "/payment-methods/" + r.moyen, nil},
		{"lire le parc de matériel", "GET", "/stations/" + r.station + "/equipment", nil},
		{"modifier l'équipement", "PATCH", "/equipment/" + r.equipement, map[string]any{"name": "Détourné"}},
		{"ajouter du matériel chez la victime", "POST", "/stations/" + r.station + "/equipment",
			map[string]any{"name": "Matériel intrus"}},
		{"lire le stock", "GET", "/stations/" + r.station + "/stock", nil},
		{"modifier l'article", "PATCH", "/stock/items/" + r.article, map[string]any{"name": "Détourné"}},
		{"lire les mouvements", "GET", "/stock/items/" + r.article + "/movements", nil},
		{"sortir du stock chez la victime", "POST", "/stock/items/" + r.article + "/movements",
			map[string]any{"kind": "sortie", "quantity": 1, "reason": "intrusion"}},
		{"ajouter un article chez la victime", "POST", "/stations/" + r.station + "/stock/items",
			map[string]any{"name": "Article intrus", "unit": "pièce"}},
		{"modifier la fiche de poste", "PATCH", "/employees/" + r.employe, map[string]any{"job_title": "Détourné"}},
		{"réinitialiser le mot de passe", "POST", "/employees/" + r.employe + "/reset-password", nil},
		{"modifier la rémunération", "PATCH", "/employees/" + r.employe,
			map[string]any{"commission_mode": "percent", "commission_percent": 90}},
		{"révoquer l'invitation", "POST", "/invitations/" + r.invitation + "/revoke", nil},
		{"lire la session de caisse", "GET", "/cash/sessions/" + r.session, nil},
		{"clôturer la session de caisse", "POST", "/cash/sessions/" + r.session + "/close",
			map[string]any{"counted_cash_xof": 0}},
		{"réouvrir la session de caisse", "POST", "/cash/sessions/" + r.session + "/reopen", nil},
		{"lire la fiche", "GET", "/wash-orders/" + r.fiche, nil},
		{"modifier la fiche", "PATCH", "/wash-orders/" + r.fiche, map[string]any{"notes": "Détournée"}},
		{"annuler la fiche", "POST", "/wash-orders/" + r.fiche + "/cancel", map[string]any{"reason": "intrusion"}},
		{"encaisser sur la fiche", "POST", "/wash-orders/" + r.fiche + "/payments",
			map[string]any{"amount_xof": 100, "method": "especes"}},
		{"lire le reçu", "GET", "/wash-orders/" + r.fiche + "/receipt", nil},
		{"ouvrir une caisse dans la station de la victime", "POST", "/cash/sessions",
			map[string]any{"station_id": r.station, "opening_float_xof": 1000}},
		{"enregistrer une fiche dans la station de la victime", "POST", "/wash-orders",
			map[string]any{"station_id": r.station,
				"items": []map[string]any{{"service_id": r.service, "quantity": 1}}}},
		{"créer un employé chez la victime", "POST", "/employees",
			map[string]any{"full_name": "Intrus", "email": fmt.Sprintf("intrus-%d@test.local", time.Now().UnixNano()),
				"role": "cashier", "station_id": r.station}},
		{"créer une prestation chez la victime", "POST", "/services",
			map[string]any{"station_id": r.station, "name": "Prestation intruse"}},
		{"créer une invitation chez la victime", "POST", "/invitations",
			map[string]any{"station_id": r.station, "email": fmt.Sprintf("intrus2-%d@test.local", time.Now().UnixNano()),
				"role": "cashier"}},
		{"réécrire la grille tarifaire de la victime", "PUT", "/price-rules/matrix",
			map[string]any{"station_id": r.station, "reason": "intrusion",
				"generic_prices": []map[string]any{{"service_id": r.service, "price_xof": 1}}}},
		{"créer une station chez la victime", "POST", "/stations",
			map[string]any{"name": "Station intruse", "city": "Abidjan"}},
	}

	for _, tentative := range tentatives {
		code, rep := attaquant.appel(tentative.methode, tentative.chemin, tentative.corps)
		if code == 403 || code == 404 {
			continue
		}
		t.Errorf("%s : attendu 403 ou 404, obtenu http=%d %v", tentative.quoi, code, resume(rep))
	}

	// --- les listes de l'attaquant ne contiennent rien de la victime --------
	listes := []struct {
		nom, chemin string
	}{
		{"les prestations", "/services"},
		{"les options", "/service-options"},
		{"les règles de prix", "/price-rules"},
		{"les moyens de paiement", "/payment-methods"},
		{"les fiches", "/wash-orders"},
		{"les sessions de caisse", "/cash/sessions"},
	}
	for _, liste := range listes {
		code, corps := attaquant.appel("GET", liste.chemin, nil)
		if code != 200 {
			t.Errorf("GET %s (attaquant) : http=%d", liste.chemin, code)
			continue
		}
		for _, identifiant := range []string{r.service, r.option, r.regle, r.moyen, r.fiche, r.session} {
			if identifiant == "" {
				continue
			}
			if chaineContient(corps, identifiant) {
				t.Errorf("%s de l'attaquant contiennent « %s »", liste.nom, identifiant)
			}
		}
	}

	// Le filtre par station de la victime ne doit rien ouvrir non plus.
	for _, chemin := range []string{
		"/services?station_id=" + r.station,
		"/price-rules/matrix?station_id=" + r.station,
		"/payment-methods?station_id=" + r.station,
		"/wash-orders?station_id=" + r.station,
	} {
		code, corps := attaquant.appel("GET", chemin, nil)
		if code == 200 && chaineContient(corps, r.service) {
			t.Errorf("GET %s a livré les données de la victime : %v", chemin, resume(corps))
		}
		if code != 200 && code != 404 {
			t.Errorf("GET %s : attendu 200 (vide) ou 404, obtenu http=%d", chemin, code)
		}
	}

	// --- un `organization_id` fabriqué côté client ne change rien -----------
	code, rep := attaquant.appel("POST", "/services", map[string]any{
		"station_id": r.station, "organization_id": victime.orgID, "name": "Prestation fabriquée",
	})
	if code == 200 || code == 201 {
		t.Errorf("un organization_id fabriqué a été accepté : http=%d %v", code, resume(rep))
	}

	// --- et rien n'a bougé chez la victime ---------------------------------
	verifierIntacte(t, victime, r)

	// --- contrôle de validité : la batterie ne doit pas être vide ----------
	//
	// Un `404` peut vouloir dire deux choses : « cette ressource ne t'appartient
	// pas » (ce qu'on veut) ou « cette route n'existe pas » (ce qui rendrait le
	// contrôle inutile). On rejoue donc chaque ligne **depuis la victime** :
	// elle a le droit, donc tout `404`/`405` restant dénonce une route absente
	// et, partant, une vérification creuse. Les modifications faites ici portent
	// sur la victime elle-même, après la vérification d'intégrité : sans effet
	// sur ce que le test a prouvé plus haut.
	//
	// Le distinguo est net : une route inconnue est refusée par le routeur avant
	// tout handler, en texte brut. Un refus métier, lui, est un JSON
	// `{"error": {...}}`. C'est donc la **forme** de la réponse qui tranche.
	for _, tentative := range tentatives {
		code, rep := victime.appel(tentative.methode, tentative.chemin, tentative.corps)
		if _, texteBrut := rep["_brut"]; texteBrut {
			t.Errorf("contrôle vide sur « %s » : %s %s n'atteint aucun handler (http=%d, réponse brute du routeur)",
				tentative.quoi, tentative.methode, tentative.chemin, code)
			continue
		}
		if code == 405 {
			t.Errorf("contrôle vide sur « %s » : %s %s n'accepte pas la méthode que le test emploie",
				tentative.quoi, tentative.methode, tentative.chemin)
			continue
		}
		// Une lecture de la propriétaire doit aboutir. Un `404` de type
		// `receipt_not_issued` (« la fiche n'est pas soldée ») prouve que le
		// handler est bien passé ; un `not_found` générique, non.
		if tentative.methode == "GET" && code != 200 && codeErreur(rep) == "not_found" {
			t.Errorf("« %s » : la lecture doit aboutir pour la propriétaire, http=%d %v",
				tentative.quoi, code, resume(rep))
		}
	}
}

// verifierIntacte relit les ressources de la victime : après une batterie
// d'attaques toutes refusées, elles doivent être exactement celles d'avant.
func verifierIntacte(t *testing.T, victime *client, r ressourcesVictime) {
	t.Helper()

	code, fiche := victime.appel("GET", "/wash-orders/"+r.fiche, nil)
	if code != 200 {
		t.Fatalf("la fiche de la victime est illisible : http=%d", code)
	}
	details, _ := fiche["order"].(map[string]any)
	if statut := texte(details, "status"); statut != "pending" && statut != "in_progress" {
		t.Errorf("la fiche de la victime a changé d'état : %q", statut)
	}
	if total := entier(details["total_xof"]); total <= 0 {
		t.Errorf("le total de la fiche de la victime a été touché : %d", total)
	}
	if solde := entier(details["paid_xof"]); solde != 0 {
		t.Errorf("la fiche de la victime a été encaissée par un tiers : %d", solde)
	}

	session := b_requete(t, victime, "GET", "/cash/sessions/"+r.session)
	if statut := texte(session, "status"); statut != "open" {
		t.Errorf("la session de caisse de la victime n'est plus ouverte : %q", statut)
	}

	prestation := b_requete(t, victime, "GET", "/services")
	if !chaineContient(prestation, r.service) {
		t.Error("la prestation de la victime a disparu")
	}

	reglages := b_requete(t, victime, "GET", "/stations/"+r.station+"/cash-settings")
	if valeur, ok := reglages["max_cashier_discount_xof"]; ok {
		if plafond := entier(valeur); plafond == 999999 {
			t.Error("les réglages de caisse de la victime ont été modifiés")
		}
	}

	moyens := b_requete(t, victime, "GET", "/payment-methods?station_id="+r.station)
	if !chaineContient(moyens, r.moyen) {
		t.Error("le moyen de paiement de la victime a disparu")
	}

	regles := b_requete(t, victime, "GET", "/stations/"+r.station+"/order-rules")
	if actif, ok := regles["auto_close_on_payment"].(bool); ok && !actif {
		t.Error("les règles de commande de la victime ont été modifiées")
	}

	parc := b_requete(t, victime, "GET", "/stations/"+r.station+"/equipment")
	if !chaineContient(parc, r.equipement) {
		t.Error("l'équipement de la victime a disparu")
	}

	stock := b_requete(t, victime, "GET", "/stations/"+r.station+"/stock")
	if !chaineContient(stock, r.article) {
		t.Error("l'article de stock de la victime a disparu")
	}
}

// b_requete : un appel dont le résultat doit être un objet JSON lisible.
func b_requete(t *testing.T, c *client, methode, chemin string) map[string]any {
	t.Helper()
	code, rep := c.appel(methode, chemin, nil)
	if code != 200 {
		t.Fatalf("%s %s (victime) : http=%d %v", methode, chemin, code, resume(rep))
	}
	return rep
}

// TestManagerNePeutPasUsurperLeProprietaire : les pouvoirs qui n'appartiennent
// qu'au propriétaire restent fermés au gérant, même s'il administre la station.
func TestManagerNePeutPasUsurperLeProprietaire(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Gérant")
	patron.remplirProfilEntreprise()
	patron.declarerHoraires()
	patron.declarerCategories("berline")
	patron.assurerPrestationTarifee()
	station, _, _ := b.creerStationEtPrestation(patron)

	gerant := b.creerEmploye(patron, station, "manager")
	laveur := b.creerEmploye(patron, station, "washer")
	code, ficheEmploye := patron.appel("GET", "/employees", nil)
	if code != 200 {
		t.Fatalf("GET /employees : http=%d", code)
	}
	laveurID := ""
	for _, brut := range listeDe(ficheEmploye, "items") {
		personne, _ := brut.(map[string]any)
		if texte(personne, "email") == laveur.email {
			laveurID = texte(personne, "id")
		}
	}
	if laveurID == "" {
		t.Fatal("le laveur est introuvable dans la liste")
	}

	interdits := []struct {
		quoi            string
		methode, chemin string
		corps           map[string]any
	}{
		{"activer la station", "POST", "/onboarding/activate", nil},
		{"créer une station", "POST", "/stations", map[string]any{"name": "Station du gérant", "city": "Abidjan"}},
		{"demander un changement d'abonnement", "POST", "/organizations/subscription/request",
			map[string]any{"plan_code": "pro"}},
		{"fixer une rémunération", "PATCH", "/employees/" + laveurID,
			map[string]any{"commission_mode": "percent", "commission_percent": 50}},
		{"entrer dans l'espace plateforme", "GET", "/platform/overview", nil},
	}
	for _, tentative := range interdits {
		code, rep := gerant.appel(tentative.methode, tentative.chemin, tentative.corps)
		if code != 403 {
			t.Errorf("%s : un gérant doit recevoir 403, obtenu http=%d %v",
				tentative.quoi, code, resume(rep))
		}
	}

	// Ce qu'il a le droit de faire reste ouvert : administrer la station.
	code, rep := gerant.appel("PUT", "/stations/"+station+"/order-rules",
		map[string]any{"auto_close_on_payment": true})
	if code != 200 {
		t.Errorf("un gérant administre les règles de sa station : http=%d %v", code, resume(rep))
	}

	// Le laveur, lui, reste un laveur : rien de l'administration ne lui est
	// ouvert, et sa fiche de poste encore moins.
	code, rep = laveur.appel("PATCH", "/employees/"+laveurID, map[string]any{"job_title": "Chef"})
	if code != 403 {
		t.Errorf("un laveur ne modifie pas sa fiche de poste : http=%d %v", code, resume(rep))
	}
}

// TestCaissierSansPermissionDePrix : le caissier encaisse, il ne tarife pas —
// et ce n'est pas l'écran qui décide, c'est le serveur.
func TestCaissierSansPermissionDePrix(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Caisse")
	patron.remplirProfilEntreprise()
	patron.declarerHoraires()
	patron.declarerCategories("berline")
	patron.assurerPrestationTarifee()
	patron.activer()
	station, service, _ := b.creerStationEtPrestation(patron)
	caissier := b.creerEmploye(patron, station, "cashier")

	interdits := []struct {
		quoi            string
		methode, chemin string
		corps           map[string]any
	}{
		{"créer une prestation", "POST", "/services",
			map[string]any{"station_id": station, "name": "Prestation du caissier"}},
		{"modifier une prestation", "PATCH", "/services/" + service, map[string]any{"name": "Détournée"}},
		{"désactiver une prestation", "DELETE", "/services/" + service, nil},
		{"créer une option", "POST", "/service-options",
			map[string]any{"station_id": station, "service_id": service, "name": "Option du caissier",
				"price_xof": 100}},
		{"poser un prix", "POST", "/price-rules",
			map[string]any{"station_id": station, "service_id": service, "price_xof": 1,
				"reason": "Prix du caissier"}},
		{"réécrire la grille", "PUT", "/price-rules/matrix",
			map[string]any{"station_id": station, "reason": "grille du caissier",
				"generic_prices": []map[string]any{{"service_id": service, "price_xof": 1}}}},
		{"modifier un moyen de paiement", "PATCH", "/payment-methods/moyen-inexistant",
			map[string]any{"name": "Détourné"}},
		{"modifier les réglages de la station", "PUT", "/stations/" + station + "/cash-settings",
			map[string]any{"max_cashier_discount_xof": 999999}},
	}
	for _, tentative := range interdits {
		code, rep := caissier.appel(tentative.methode, tentative.chemin, tentative.corps)
		if code != 403 && code != 404 {
			t.Errorf("%s : attendu 403 ou 404, obtenu http=%d %v", tentative.quoi, code, resume(rep))
		}
	}

	// Il lit le catalogue (il en a besoin pour vendre) et il encaisse.
	code, rep := caissier.appel("GET", "/services?station_id="+station, nil)
	if code != 200 {
		t.Errorf("un caissier doit pouvoir lire le catalogue : http=%d %v", code, resume(rep))
	}
	code, rep = caissier.appel("GET", "/price-preview?station_id="+station+"&service_id="+service, nil)
	if code != 200 {
		t.Errorf("un caissier doit pouvoir lire un prix (sans le changer) : http=%d %v", code, resume(rep))
	}
	code, rep = caissier.appel("POST", "/cash/sessions",
		map[string]any{"station_id": station, "opening_float_xof": 10000})
	if code != 201 {
		t.Fatalf("le caissier ouvre la caisse : http=%d %v", code, resume(rep))
	}
	code, rep = caissier.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("le caissier enregistre un lavage : http=%d %v", code, resume(rep))
	}
}

// TestLimitesDeFormuleAppliquees : la formule Essai annonce une station et
// trois comptes. Le serveur doit le faire respecter, sinon le plafond n'est
// qu'une phrase sur un écran d'abonnement.
func TestLimitesDeFormuleAppliquees(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Plafond")

	// L'inscription a déjà créé la station de l'entreprise : la deuxième est de
	// trop pour la formule Essai.
	code, rep := patron.appel("POST", "/stations", map[string]any{
		"name": "Deuxième station", "city": "Abidjan",
	})
	if code != 403 || codeErreur(rep) != "plan_limit" {
		t.Fatalf("une station de plus que la formule doit être refusée : http=%d %v", code, resume(rep))
	}
	erreur, _ := rep["error"].(map[string]any)
	if message := texte(erreur, "message"); !strings.Contains(message, "Essai") || !strings.Contains(message, "Abonnement") {
		t.Errorf("le refus doit nommer la formule et la sortie : %q", message)
	}

	// Deux comptes de plus, puis le quatrième refusé (trois au total avec le
	// propriétaire).
	station, _, _ := b.creerStationEtPrestation(patron)
	b.creerEmploye(patron, station, "washer")
	b.creerEmploye(patron, station, "cashier")

	code, rep = patron.appel("POST", "/employees", map[string]any{
		"full_name": "Quatrième compte", "email": fmt.Sprintf("plafond-%d@test.local", time.Now().UnixNano()),
		"role": "washer", "station_id": station,
	})
	if code != 403 || codeErreur(rep) != "plan_limit" {
		t.Errorf("un compte de plus que la formule doit être refusé : http=%d %v", code, resume(rep))
	}

	// Une invitation qui dépasserait le plafond est refusée **avant** d'être
	// émise : inviter puis refuser à l'acceptation serait un piège.
	code, rep = patron.appel("POST", "/invitations", map[string]any{
		"station_id": station, "email": fmt.Sprintf("surplus-%d@test.local", time.Now().UnixNano()),
		"role": "cashier",
	})
	if code != 403 || codeErreur(rep) != "plan_limit" {
		t.Errorf("une invitation au-delà du plafond doit être refusée : http=%d %v", code, resume(rep))
	}

	// Les lectures et l'exploitation restent ouvertes : un plafond atteint
	// n'enferme pas l'entreprise qui travaille déjà.
	if code, rep := patron.appel("GET", "/stations", nil); code != 200 {
		t.Errorf("la liste des stations reste lisible : http=%d %v", code, resume(rep))
	}
}

// ---------------------------------------------------------------------------
// Aides locales
// ---------------------------------------------------------------------------

// listeDe lit une liste dans une réponse (`{"items": […]}` ou tableau direct).
func listeDe(reponse map[string]any, cle string) []any {
	if liste, ok := reponse[cle].([]any); ok {
		return liste
	}
	if liste, ok := reponse["_tableau"].([]any); ok {
		return liste
	}
	return nil
}

// chaineContient : la réponse contient-elle cette valeur, où que ce soit ?
// Sert à prouver qu'aucune liste ne livre un identifiant d'une autre
// entreprise — la forme de la réponse n'importe pas.
func chaineContient(valeur any, cherche string) bool {
	if cherche == "" {
		return false
	}
	switch noeud := valeur.(type) {
	case string:
		return noeud == cherche
	case []any:
		for _, enfant := range noeud {
			if chaineContient(enfant, cherche) {
				return true
			}
		}
	case map[string]any:
		for _, enfant := range noeud {
			if chaineContient(enfant, cherche) {
				return true
			}
		}
	}
	return false
}

// resume : de quoi comprendre un échec sans noyer la sortie du test.
func resume(reponse map[string]any) string {
	if erreur, ok := reponse["error"].(map[string]any); ok {
		return fmt.Sprintf("%v (%v)", erreur["code"], erreur["message"])
	}
	if brut, ok := reponse["_brut"].(string); ok && len(brut) > 120 {
		return brut[:120] + "…"
	}
	partiel := map[string]any{}
	for _, cle := range []string{"code", "message", "id", "name", "status", "state"} {
		if valeur, ok := reponse[cle]; ok {
			partiel[cle] = valeur
		}
	}
	return fmt.Sprintf("%v", partiel)
}
