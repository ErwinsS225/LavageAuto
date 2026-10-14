package main

import (
	"testing"
)

// ===========================================================================
// Lot 5 — prestations, options et grille tarifaire.
//
// Ce que ces tests démontrent, dans l'ordre où l'exploitant le vit :
//
//  1. un compte neuf reçoit un catalogue **à tarifer** : aucun prix n'est
//     inventé à sa place ;
//  2. une prestation sans tarif ne se vend pas (`409 price_required`) ;
//  3. le tarif se résout par ordre de précision — contrat client > prestation ×
//     véhicule × usage > prestation × véhicule > prestation × usage > tarif du
//     catalogue ;
//  4. la provenance du prix (`price_source`) accompagne l'aperçu et la fiche ;
//  5. deux règles aussi précises se signalent (`409 price_rule_conflict`) et
//     l'enregistrement de la grille exige un motif ;
//  6. les options se facturent par lavage, la gratuité vaut zéro, et le
//     réglage `allow_generic_price` retire le repli du catalogue ;
//  7. aucune grille ne franchit la frontière d'une entreprise.
// ===========================================================================

// prestationDuModele retrouve, dans le catalogue de la station, la prestation
// issue d'un modèle de référence.
func (b *banc) prestationDuModele(c *client, station, modele string) map[string]any {
	c.t.Helper()
	code, catalogue := c.appel("GET", "/services?station_id="+station, nil)
	if code != 200 {
		c.t.Fatalf("GET /services : http=%d %v", code, catalogue)
	}
	for _, brut := range catalogue["items"].([]any) {
		prestation, _ := brut.(map[string]any)
		if texte(prestation, "template_code") == modele {
			return prestation
		}
	}
	c.t.Fatalf("prestation %q absente du catalogue de la station", modele)
	return nil
}

// reglePrix pose une règle tarifaire et vérifie le code attendu.
func (b *banc) reglePrix(c *client, corps map[string]any, attendu int) map[string]any {
	c.t.Helper()
	code, rep := c.appel("POST", "/price-rules", corps)
	if code != attendu {
		c.t.Fatalf("POST /price-rules (attendu %d) : http=%d %v", attendu, code, rep)
	}
	return rep
}

// fiche crée une fiche de lavage et rend sa réponse — le prix y est calculé par
// le serveur, jamais envoyé par le client.
func (b *banc) fiche(c *client, corps map[string]any, attendu int) map[string]any {
	c.t.Helper()
	code, rep := c.appel("POST", "/wash-orders", corps)
	if code != attendu {
		c.t.Fatalf("POST /wash-orders (attendu %d) : http=%d %v", attendu, code, rep)
	}
	return depaqueter(rep)
}

// depaqueter : la fiche est renvoyée sous « order » par les routes de fiche.
// Le banc accepte les deux formes plutôt que de figer la couche de transport.
func depaqueter(rep map[string]any) map[string]any {
	if interieur, ok := rep["order"].(map[string]any); ok {
		return interieur
	}
	return rep
}

// ligneUnique rend la première ligne d'une fiche.
func (b *banc) ligneUnique(c *client, fiche map[string]any) map[string]any {
	c.t.Helper()
	items, _ := fiche["items"].([]any)
	if len(items) == 0 {
		c.t.Fatalf("la fiche n'a aucune ligne : %v", fiche)
	}
	ligne, _ := items[0].(map[string]any)
	return ligne
}

// celluleMatrice retrouve une case (prestation, véhicule) dans la matrice.
func (b *banc) celluleMatrice(c *client, matrice map[string]any, serviceID, categorie string) map[string]any {
	c.t.Helper()
	lignes, _ := matrice["services"].([]any)
	for _, brut := range lignes {
		ligne, _ := brut.(map[string]any)
		if texte(ligne, "service_id") != serviceID {
			continue
		}
		for _, brutCase := range ligne["cells"].([]any) {
			celle, _ := brutCase.(map[string]any)
			if texte(celle, "category_code") == categorie {
				return celle
			}
		}
	}
	c.t.Fatalf("case (%s, %s) absente de la matrice", serviceID, categorie)
	return nil
}

// vrai : un booléen de réponse JSON (absent = false).
func vrai(rep map[string]any, cle string) bool {
	valeur, _ := rep[cle].(bool)
	return valeur
}

// clientDeTest crée un client de passage pour tester les contrats.
func (b *banc) clientDeTest(c *client, station, nom string) string {
	c.t.Helper()
	code, rep := c.appel("POST", "/customers", map[string]any{
		"station_id": station, "full_name": nom, "phone": "+225 07 11 22 33 44",
	})
	if code != 201 {
		c.t.Fatalf("création du client %q : http=%d %v", nom, code, rep)
	}
	return texte(rep, "id")
}

// ===========================================================================
// Tests
// ===========================================================================

// Un catalogue neuf est « à tarifer » : l'aperçu de prix dit ce qui manque au
// lieu d'afficher un montant que personne n'a décidé.
func TestCatalogueATariferEtApercuSansPrix(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Catalogue")
	c.activer()

	code, stations := c.tableau("GET", "/stations", nil)
	if code != 200 || len(stations) == 0 {
		t.Fatalf("GET /stations : http=%d", code)
	}
	premiere, _ := stations[0].(map[string]any)
	station := texte(premiere, "id")

	// Le catalogue de référence est visible et complet, mais sans aucun prix.
	code, modeles := c.appel("GET", "/service-templates?station_id="+station, nil)
	if code != 200 {
		t.Fatalf("GET /service-templates : http=%d %v", code, modeles)
	}
	referentiel, _ := modeles["templates"].([]any)
	if len(referentiel) < 15 {
		t.Errorf("le référentiel doit proposer au moins 15 modèles, obtenu %d", len(referentiel))
	}
	for _, brut := range referentiel {
		modele, _ := brut.(map[string]any)
		if prix := entier(modele["price_xof"]); prix != 0 {
			t.Errorf("un modèle ne doit porter aucun prix : %v", modele)
		}
	}

	// Le catalogue de la station est équipé des 18 prestations, à tarifer.
	code, catalogue := c.appel("GET", "/services?station_id="+station+"&filtre=a_tarifer", nil)
	if code != 200 {
		t.Fatalf("GET /services (à tarifer) : http=%d %v", code, catalogue)
	}
	if total := entier(catalogue["total"]); total == 0 {
		t.Errorf("le catalogue neuf doit être équipé et à tarifer : %v", catalogue)
	}

	prestation := b.prestationDuModele(c, station, "lavage_exterieur")
	service := texte(prestation, "id")
	if !vrai(prestation, "needs_price") {
		t.Errorf("une prestation sans règle ni tarif doit être signalée à tarifer : %v", prestation)
	}

	// L'aperçu de prix est honnête : 200 + « needs_price », pas un montant.
	code, apercu := c.appel("GET", "/price-preview?station_id="+station+"&service_id="+service, nil)
	if code != 200 {
		t.Fatalf("GET /price-preview : http=%d %v", code, apercu)
	}
	if !vrai(apercu, "needs_price") {
		t.Errorf("l'aperçu doit dire qu'aucun prix n'est défini : %v", apercu)
	}
	if prix := entier(apercu["price_xof"]); prix != 0 {
		t.Errorf("aucun prix ne doit être inventé : obtenu %d", prix)
	}

	// Et la vente est refusée tant que le prix manque : le catalogue décrit ce
	// qui est fait, il ne fixe pas le prix.
	code, refus := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 409 {
		t.Fatalf("vendre une prestation sans tarif doit être refusé (409) : http=%d %v", code, refus)
	}
	enveloppe, _ := refus["error"].(map[string]any)
	if texte(enveloppe, "code") != "price_required" {
		t.Errorf("code attendu « price_required » : %v", enveloppe)
	}
	// La grille est la seule route qui écrit un prix : le catalogue refuse.
	code, refus = c.appel("PATCH", "/services/"+service, map[string]any{"price_xof": 3000})
	if code != 400 {
		t.Errorf("un prix hors grille doit être refusé (400) : http=%d %v", code, refus)
	}
	code, refus = c.appel("POST", "/services", map[string]any{
		"station_id": station, "name": "Lavage hors grille", "price_xof": 2500,
	})
	if code != 400 {
		t.Errorf("une prestation ne se crée pas avec un prix (400) : http=%d %v", code, refus)
	}
}

// La hiérarchie des prix, du plus général au plus précis, et la provenance
// renvoyée à l'écran.
func TestHierarchieDesPrixEtProvenance(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Hierarchie")
	c.declarerCategories("berline", "moto")
	c.declarerUsages("taxi_compteur:berline")
	c.activer()
	station, service, _ := b.creerStationEtPrestation(c)

	// Repli du catalogue.
	c.tariferCatalogue(station, service, 3000)
	code, apercu := c.appel("GET", "/price-preview?station_id="+station+"&service_id="+service, nil)
	if code != 200 || entier(apercu["price_xof"]) != 3000 || texte(apercu, "price_source") != "service_generic" {
		t.Fatalf("repli du catalogue : http=%d %v", code, apercu)
	}

	// Prestation × véhicule : plus précis que le repli.
	b.reglePrix(c, map[string]any{
		"station_id": station, "service_id": service, "category_code": "berline", "price_xof": 2500,
		"reason": "Tarif berline",
	}, 201)
	code, apercu = c.appel("GET", "/price-preview?station_id="+station+"&service_id="+service+"&category_code=berline", nil)
	if code != 200 || entier(apercu["price_xof"]) != 2500 || texte(apercu, "price_source") != "service_category" {
		t.Fatalf("prestation × véhicule : http=%d %v", code, apercu)
	}

	// Prestation × véhicule × usage : plus précis encore.
	b.reglePrix(c, map[string]any{
		"station_id": station, "service_id": service, "category_code": "berline",
		"usage_code": "taxi_compteur", "price_xof": 2000, "reason": "Tarif taxi",
	}, 201)
	code, apercu = c.appel("GET", "/price-preview?station_id="+station+"&service_id="+service+
		"&category_code=berline&usage_code=taxi_compteur", nil)
	if code != 200 || entier(apercu["price_xof"]) != 2000 {
		t.Fatalf("prestation × véhicule × usage : http=%d %v", code, apercu)
	}

	// Contrat client : le plus précis, même sans usage déclaré.
	clientID := b.clientDeTest(c, station, "Flotte Taxi Express")
	b.reglePrix(c, map[string]any{
		"station_id": station, "service_id": service, "customer_id": clientID,
		"price_xof": 1800, "reason": "Contrat flotte",
	}, 201)
	code, apercu = c.appel("GET", "/price-preview?station_id="+station+"&service_id="+service+
		"&category_code=berline&customer_id="+clientID, nil)
	if code != 200 || entier(apercu["price_xof"]) != 1800 || texte(apercu, "price_source") != "client_contract" {
		t.Fatalf("contrat client : http=%d %v", code, apercu)
	}

	// La fiche porte le prix résolu et sa provenance, jamais un total envoyé
	// par le client.
	fiche := b.fiche(c, map[string]any{
		"station_id": station, "customer_id": clientID, "vehicle_category_id": "berline",
		"items": []map[string]any{{"service_id": service, "quantity": 2, "unit_price_xof": 1}},
	}, 201)
	if total := entier(fiche["total_xof"]); total != 3600 {
		t.Errorf("total attendu 2 × 1800 = 3600, obtenu %d", total)
	}
	ligne := b.ligneUnique(c, fiche)
	if prix := entier(ligne["unit_price_xof"]); prix != 1800 {
		t.Errorf("le prix unitaire doit venir de la grille (1800), obtenu %d", prix)
	}
	if source := texte(ligne, "price_source"); source != "client_contract" {
		t.Errorf("la provenance du prix doit accompagner la ligne : obtenu %q", source)
	}

	// Un discriminant absent rend la règle inapplicable : sans client, la
	// grille retombe sur véhicule × usage.
	fiche = b.fiche(c, map[string]any{
		"station_id": station, "vehicle_category_id": "berline", "vehicle_usage_id": "taxi_compteur",
		"items": []map[string]any{{"service_id": service, "quantity": 1}},
	}, 201)
	if total := entier(fiche["total_xof"]); total != 2000 {
		t.Errorf("sans client, la grille doit retomber sur véhicule × usage (2000) : obtenu %d", total)
	}
}

// Deux règles de même spécificité ne laissent pas le serveur choisir à la place
// de l'exploitant : il refuse, en nommant la règle en cause.
func TestRegleEnConflitEtMotifObligatoire(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Conflit")
	c.declarerCategories("berline")
	c.activer()
	station, service, _ := b.creerStationEtPrestation(c)

	b.reglePrix(c, map[string]any{
		"station_id": station, "service_id": service, "category_code": "berline",
		"price_xof": 2500, "reason": "Première grille",
	}, 201)

	// Deuxième règle, même spécificité : conflit signalé, pas de remplacement
	// silencieux.
	code, refus := c.appel("POST", "/price-rules", map[string]any{
		"station_id": station, "service_id": service, "category_code": "berline",
		"price_xof": 2700, "reason": "Deuxième grille",
	})
	if code != 409 {
		t.Fatalf("un conflit de règles doit être refusé (409) : http=%d %v", code, refus)
	}
	enveloppe, _ := refus["error"].(map[string]any)
	if texte(enveloppe, "code") != "price_rule_conflict" {
		t.Errorf("code attendu « price_rule_conflict » : %v", enveloppe)
	}
	champs, _ := enveloppe["fields"].(map[string]any)
	if texte(champs, "price_rule_id") == "" {
		t.Errorf("le conflit doit nommer la règle existante : %v", enveloppe)
	}

	// Un motif est exigé partout où un prix s'écrit : sans lui, aucune trace
	// exploitable ne resterait.
	code, refus = c.appel("POST", "/price-rules", map[string]any{
		"station_id": station, "service_id": service, "category_code": "moto", "price_xof": 1500,
	})
	if code != 400 {
		t.Fatalf("une règle sans motif doit être refusée (400) : http=%d %v", code, refus)
	}
	code, refus = c.appel("PUT", "/price-rules/matrix", map[string]any{
		"station_id": station, "generic_prices": []map[string]any{{"service_id": service, "price_xof": 3000}},
	})
	if code != 400 {
		t.Fatalf("la grille sans motif doit être refusée (400) : http=%d %v", code, refus)
	}

	// La matrice se relit avec la règle active et le tarif de repli.
	code, matrice := c.appel("GET", "/price-rules/matrix?station_id="+station, nil)
	if code != 200 {
		t.Fatalf("GET /price-rules/matrix : http=%d %v", code, matrice)
	}
	celle := b.celluleMatrice(c, matrice, service, "berline")
	if prix := entier(celle["price_xof"]); prix != 2500 {
		t.Errorf("la case berline doit porter 2500 : obtenu %d", prix)
	}
	if source := texte(celle, "source"); source != "service_category" {
		t.Errorf("la case doit citer la règle qui l'alimente : %v", celle)
	}
	if vide := texte(celle, "rule_id"); vide == "" {
		t.Errorf("une case tarifée doit porter l'identifiant de sa règle : %v", celle)
	}
	// Le tarif de repli du catalogue est visible dans la matrice, ligne par
	// ligne : la grille dit ce qui s'applique quand aucune case n'est remplie.
	repli := 0
	for _, brut := range matrice["services"].([]any) {
		ligne, _ := brut.(map[string]any)
		if texte(ligne, "service_id") == service {
			repli = entier(ligne["generic_price_xof"])
		}
	}
	if repli != 3000 {
		t.Errorf("le repli du catalogue doit apparaître dans la matrice (3000) : obtenu %d", repli)
	}
}

// Les options se facturent par lavage : elles s'ajoutent au prix de la
// prestation, et une option inconnue est refusée.
func TestOptionsFactureesParLavage(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Options")
	c.activer()
	station, service, _ := b.creerStationEtPrestation(c)
	c.tariferCatalogue(station, service, 3000)

	// Une option de station (cire) et une option réservée à cette prestation.
	code, cire := c.appel("POST", "/service-options", map[string]any{
		"station_id": station, "name": "Cire de finition", "price_xof": 500,
		"duration_minutes": 10, "is_active": true,
	})
	if code != 201 {
		t.Fatalf("création de l'option cire : http=%d %v", code, cire)
	}
	code, tapis := c.appel("POST", "/service-options", map[string]any{
		"station_id": station, "service_id": service, "name": "Shampouinage tapis",
		"price_xof": 1500, "duration_minutes": 15, "is_active": true,
	})
	if code != 201 {
		t.Fatalf("création de l'option tapis : http=%d %v", code, tapis)
	}
	code, options := c.appel("GET", "/service-options?station_id="+station+"&service_id="+service, nil)
	if code != 200 {
		t.Fatalf("GET /service-options : http=%d %v", code, options)
	}
	if total := entier(options["total"]); total != 2 {
		t.Errorf("la station doit proposer 2 options, obtenu %d", total)
	}

	// La fiche facture prestation + options, ligne par ligne.
	fiche := b.fiche(c, map[string]any{
		"station_id": station,
		"items": []map[string]any{{
			"service_id": service, "quantity": 1,
			"options": []map[string]any{
				{"option_id": texte(cire, "id"), "quantity": 1},
				{"option_id": texte(tapis, "id"), "quantity": 1},
			},
		}},
	}, 201)
	ligne := b.ligneUnique(c, fiche)
	if total := entier(ligne["line_total_xof"]); total != 5000 {
		t.Errorf("ligne attendue 3000 + 500 + 1500 = 5000, obtenue %d", total)
	}
	if suppléments := entier(ligne["options_total_xof"]); suppléments != 2000 {
		t.Errorf("suppléments attendus 2000, obtenus %d", suppléments)
	}
	if v := entier(fiche["total_xof"]); v != 5000 {
		t.Errorf("total de la fiche : attendu 5000, obtenu %d", v)
	}

	// Une option d'une autre prestation est refusée : la fiche ne facture pas
	// ce que la station ne propose pas pour ce lavage.
	code, autre := c.appel("POST", "/services", map[string]any{
		"station_id": station, "name": "Lavage moto", "duration_minutes": 20,
	})
	if code != 201 {
		t.Fatalf("création d'une seconde prestation : http=%d %v", code, autre)
	}
	c.tariferCatalogue(station, texte(autre, "id"), 1500)
	code, refus := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items": []map[string]any{{
			"service_id": texte(autre, "id"), "quantity": 1,
			"options": []map[string]any{{"option_id": texte(tapis, "id"), "quantity": 1}},
		}},
	})
	if code != 400 {
		t.Fatalf("une option non proposée pour cette prestation doit être refusée (400) : http=%d %v", code, refus)
	}

	// Désactiver une option la retire de la vente sans l'effacer : les fiches
	// passées continuent de la citer.
	if code, rep := c.appel("PATCH", "/service-options/"+texte(cire, "id"), map[string]any{
		"is_active": false,
	}); code != 200 {
		t.Fatalf("désactivation de l'option cire : http=%d %v", code, rep)
	}
	code, apercu := c.appel("GET", "/price-preview?station_id="+station+"&service_id="+service, nil)
	if code != 200 {
		t.Fatalf("aperçu après désactivation : http=%d %v", code, apercu)
	}
	for _, brut := range apercu["options"].([]any) {
		option, _ := brut.(map[string]any)
		if texte(option, "id") == texte(cire, "id") {
			t.Errorf("une option désactivée ne doit plus être proposée : %v", option)
		}
	}
	// Et elle se recrée sous le même nom une fois active à nouveau.
	if code, rep := c.appel("PATCH", "/service-options/"+texte(cire, "id"), map[string]any{
		"is_active": true,
	}); code != 200 {
		t.Fatalf("réactivation de l'option cire : http=%d %v", code, rep)
	}
	code, apercu = c.appel("GET", "/price-preview?station_id="+station+"&service_id="+service, nil)
	if code != 200 {
		t.Fatalf("aperçu final : http=%d %v", code, apercu)
	}
	trouvee := false
	for _, brut := range apercu["options"].([]any) {
		option, _ := brut.(map[string]any)
		if texte(option, "id") == texte(cire, "id") {
			trouvee = true
		}
	}
	if !trouvee {
		t.Errorf("l'option réactivée doit être de nouveau proposée : %v", apercu)
	}
}

// Gratuité et repli du catalogue : deux règles de sécurité, pas deux détails.
func TestGratuiteEtRepliDuCatalogue(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Gratuite")
	c.activer()
	station, service, _ := b.creerStationEtPrestation(c)
	c.tariferCatalogue(station, service, 3000)

	// Une prestation déclarée gratuite se vend à zéro, quel que soit le tarif
	// du catalogue : la décision commerciale prime sur la grille.
	code, gratuit := c.appel("PATCH", "/services/"+service, map[string]any{
		"is_free": true,
	})
	if code != 200 {
		t.Fatalf("marquage « gratuit » : http=%d %v", code, gratuit)
	}
	code, apercu := c.appel("GET", "/price-preview?station_id="+station+"&service_id="+service, nil)
	if code != 200 || entier(apercu["price_xof"]) != 0 || texte(apercu, "price_source") != "free" {
		t.Fatalf("une prestation gratuite doit valoir 0 en « free » : http=%d %v", code, apercu)
	}
	fiche := b.fiche(c, map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	}, 201)
	if total := entier(fiche["total_xof"]); total != 0 {
		t.Errorf("une prestation gratuite ne se facture pas : obtenu %d", total)
	}

	// Retirer la gratuité ne ressuscite pas un prix : la prestation repasse
	// « à tarifer » — le serveur n'invente pas un montant que la gratuité avait
	// effacé.
	if code, rep := c.appel("PATCH", "/services/"+service, map[string]any{"is_free": false}); code != 200 {
		t.Fatalf("retrait de la gratuité : http=%d %v", code, rep)
	}
	code, apercu = c.appel("GET", "/price-preview?station_id="+station+"&service_id="+service, nil)
	if code != 200 || !vrai(apercu, "needs_price") {
		t.Fatalf("après une gratuité retirée, la prestation doit être à tarifer : http=%d %v", code, apercu)
	}
	// La grille reprend la main : c'est elle, et elle seule, qui repose un prix.
	c.tariferCatalogue(station, service, 3000)
	code, apercu = c.appel("GET", "/price-preview?station_id="+station+"&service_id="+service, nil)
	if code != 200 || entier(apercu["price_xof"]) != 3000 {
		t.Fatalf("le tarif du catalogue doit reprendre : http=%d %v", code, apercu)
	}

	// Le repli du catalogue peut être retiré par la station : chaque véhicule
	// doit alors avoir son tarif, et rien n'est deviné.
	code, rep := c.appel("PATCH", "/stations/"+station, map[string]any{
		"settings": map[string]any{"allow_generic_price": false},
	})
	if code != 200 {
		t.Fatalf("retrait du repli générique : http=%d %v", code, rep)
	}
	code, refus := c.appel("GET", "/price-preview?station_id="+station+"&service_id="+service+"&category_code=berline", nil)
	if code != 200 || !vrai(refus, "needs_price") {
		t.Fatalf("sans repli, un véhicule sans tarif doit être signalé : http=%d %v", code, refus)
	}
	code, venteRefusee := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station, "vehicle_category_id": "berline",
		"items": []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 409 {
		t.Fatalf("sans repli ni règle, la vente doit être refusée (409) : http=%d %v", code, venteRefusee)
	}

	// Une règle par véhicule rétablit la vente, au prix décidé.
	c.declarerCategories("berline")
	b.reglePrix(c, map[string]any{
		"station_id": station, "service_id": service, "category_code": "berline",
		"price_xof": 4200, "reason": "Grille berline sans repli",
	}, 201)
	fiche = b.fiche(c, map[string]any{
		"station_id": station, "vehicle_category_id": "berline",
		"items": []map[string]any{{"service_id": service, "quantity": 1}},
	}, 201)
	if total := entier(fiche["total_xof"]); total != 4200 {
		t.Errorf("la fiche doit porter le tarif de la règle (4200) : obtenu %d", total)
	}
}

// Aucune grille ne franchit la frontière d'une entreprise.
func TestGrilleIsoleeParEntreprise(t *testing.T) {
	b := nouveauBanc(t)
	voisine := b.client()
	voisine.inscrire("Entreprise Grille Voisine")
	voisine.activer()
	stationVoisine, serviceVoisin, _ := b.creerStationEtPrestation(voisine)

	c := b.client()
	c.inscrire("Entreprise Grille Locale")
	c.activer()
	station, service, _ := b.creerStationEtPrestation(c)
	c.tariferCatalogue(station, service, 2000)

	// Une règle posée sur la grille voisine ne s'applique pas ici.
	code, refus := c.appel("GET", "/price-preview?station_id="+station+"&service_id="+serviceVoisin, nil)
	if code != 404 && code != 400 {
		t.Errorf("une prestation d'une autre entreprise doit être introuvable : http=%d %v", code, refus)
	}
	code, refus = c.appel("POST", "/price-rules", map[string]any{
		"station_id": stationVoisine, "service_id": service, "price_xof": 1000, "reason": "Tentative",
	})
	if code != 403 && code != 404 {
		t.Errorf("écrire dans la grille d'une autre station doit être refusé : http=%d %v", code, refus)
	}

	// La grille locale reste intacte : 2000, appliqué à la fiche.
	code, apercu := c.appel("GET", "/price-preview?station_id="+station+"&service_id="+service, nil)
	if code != 200 || entier(apercu["price_xof"]) != 2000 {
		t.Fatalf("la grille locale ne doit pas bouger : http=%d %v", code, apercu)
	}
	fiche := b.fiche(c, map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	}, 201)
	if total := entier(fiche["total_xof"]); total != 2000 {
		t.Errorf("prix appliqué à la fiche : attendu 2000, obtenu %d", total)
	}
}

// Le référentiel des prestations est proposé à l'adoption : adopter un modèle
// crée la prestation sans prix, jamais avec un prix supposé.
func TestAdoptionDUnModeleSansPrix(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Adoption")

	code, stations := c.tableau("GET", "/stations", nil)
	if code != 200 || len(stations) == 0 {
		t.Fatalf("GET /stations : http=%d", code)
	}
	premiere, _ := stations[0].(map[string]any)
	station := texte(premiere, "id")

	code, adoption := c.appel("POST", "/services", map[string]any{
		"station_id": station, "template_code": "ceramique",
	})
	if code != 201 {
		t.Fatalf("adoption du modèle « céramique » : http=%d %v", code, adoption)
	}
	if prix := entier(adoption["price_xof"]); prix != 0 {
		t.Errorf("un modèle adopté ne porte aucun prix : obtenu %d", prix)
	}
	if !vrai(adoption, "is_quote_only") && !vrai(adoption, "needs_price") {
		t.Errorf("la céramique se vend sur devis ou à tarifer, jamais à un prix supposé : %v", adoption)
	}
	if template := texte(adoption, "template_code"); template != "ceramique" {
		t.Errorf("l'origine du modèle doit rester lisible : obtenu %q", template)
	}
	if texte(adoption, "id") == "" {
		t.Error("la prestation adoptée doit avoir un identifiant")
	}
}
