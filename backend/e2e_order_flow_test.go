package main

import (
	"testing"
)

// ===========================================================================
// Lot 7 — le cycle de vie d'une fiche et les règles de la station.
//
// Les règles ne sont pas des cases à cocher décoratives : chacune est lue à
// l'endroit où le serveur décide. Ces tests les activent une par une et
// vérifient que le refus arrive **du serveur**, avec un motif exploitable.
// ===========================================================================

// reglerCommandes applique des règles à une station et vérifie la réponse.
func reglerCommandes(t *testing.T, c *client, station string, corps map[string]any) {
	t.Helper()
	if code, rep := c.appel("PUT", "/stations/"+station+"/order-rules", corps); code != 200 {
		t.Fatalf("PUT /stations/%s/order-rules : http=%d %v", station, code, rep)
	}
}

// ficheEnAttente crée une fiche « reçue, en attente » et rend l'identifiant.
func ficheEnAttente(t *testing.T, b *banc, c *client, station, service string) string {
	t.Helper()
	rep := b.fiche(c, map[string]any{
		"station_id": station, "customer_name": "Client Cycle", "status": "pending",
		"items": []map[string]any{{"service_id": service, "quantity": 1}},
	}, 201)
	return texte(rep, "id")
}

func TestCycleDeVieDuneFicheEtTransitionsInterdites(t *testing.T) {
	b, proprietaire, station := stationActive(t)
	service := b.serviceTarife(proprietaire, station)
	fiche := ficheEnAttente(t, b, proprietaire, station, service)

	// Le référentiel des statuts est exposé : l'écran n'invente pas ses
	// libellés, il les lit.
	code, regles := proprietaire.appel("GET", "/stations/"+station+"/order-rules", nil)
	if code != 200 {
		t.Fatalf("GET /order-rules : http=%d %v", code, regles)
	}
	if len(regles["statuses"].([]any)) != 7 {
		t.Errorf("sept états de cycle de vie attendus : %v", regles["statuses"])
	}

	// Reçue → en cours → contrôle qualité → prête.
	for _, cible := range []string{"in_progress", "quality_check", "ready"} {
		code, rep := proprietaire.appel("PATCH", "/wash-orders/"+fiche, map[string]any{"status": cible})
		if code != 200 {
			t.Fatalf("transition vers %s : http=%d %v", cible, code, rep)
		}
		if texte(rep, "status") != cible {
			t.Errorf("statut attendu %s, obtenu %q", cible, texte(rep, "status"))
		}
	}

	// Une fiche prête ne redevient pas « reçue » : le graphe ne le permet pas.
	code, refus := proprietaire.appel("PATCH", "/wash-orders/"+fiche, map[string]any{"status": "pending"})
	if code != 409 || codeErreur(refus) != "transition_forbidden" {
		t.Errorf("transition interdite attendue (409 transition_forbidden) : http=%d %v", code, refus)
	}
	// Un statut inventé est refusé sur son champ.
	code, inconnu := proprietaire.appel("PATCH", "/wash-orders/"+fiche, map[string]any{"status": "terminee"})
	if code != 400 {
		t.Errorf("statut inconnu refusé attendu (400) : http=%d %v", code, inconnu)
	}
	// Un litige sans motif n'apprend rien au gérant qui le lira.
	code, litige := proprietaire.appel("PATCH", "/wash-orders/"+fiche, map[string]any{"status": "disputed"})
	if code != 400 {
		t.Errorf("litige non motivé refusé attendu (400) : http=%d %v", code, litige)
	}
	code, motif := proprietaire.appel("PATCH", "/wash-orders/"+fiche,
		map[string]any{"status": "disputed", "dispute_reason": "Rayure sur l'aile avant droite"})
	if code != 200 {
		t.Fatalf("litige motivé : http=%d %v", code, motif)
	}
	if texte(motif, "status") != "disputed" {
		t.Errorf("statut attendu disputed, obtenu %q", texte(motif, "status"))
	}

	// Un caissier annule ? Non : annuler un lavage demande la permission
	// dédiée, et le caissier ne l'a pas.
	caissier := b.creerEmploye(proprietaire, station, "cashier")
	code, refusAnnulation := caissier.appel("PATCH", "/wash-orders/"+fiche,
		map[string]any{"status": "canceled", "reason": "Client parti"})
	if code != 403 {
		t.Errorf("un caissier n'annule pas une fiche (403) : http=%d %v", code, refusAnnulation)
	}

	// Annulation motivée par le propriétaire, puis plus rien n'est possible.
	code, annulee := proprietaire.appel("PATCH", "/wash-orders/"+fiche,
		map[string]any{"status": "canceled", "reason": "Client parti sans attendre"})
	if code != 200 {
		t.Fatalf("annulation motivée : http=%d %v", code, annulee)
	}
	code, terminale := proprietaire.appel("PATCH", "/wash-orders/"+fiche, map[string]any{"status": "in_progress"})
	if code != 409 {
		t.Errorf("une fiche annulée est terminale (409) : http=%d %v", code, terminale)
	}
}

func TestRegleControleQualiteObligatoire(t *testing.T) {
	b, proprietaire, station := stationActive(t)
	service := b.serviceTarife(proprietaire, station)
	reglerCommandes(t, proprietaire, station, map[string]any{"quality_check_required": true})

	fiche := ficheEnAttente(t, b, proprietaire, station, service)
	if code, rep := proprietaire.appel("PATCH", "/wash-orders/"+fiche,
		map[string]any{"status": "in_progress"}); code != 200 {
		t.Fatalf("démarrage du lavage : http=%d %v", code, rep)
	}

	// On ne saute pas le contrôle : la fiche ne peut pas être déclarée prête
	// depuis l'atelier.
	code, refus := proprietaire.appel("PATCH", "/wash-orders/"+fiche, map[string]any{"status": "ready"})
	if code != 409 || codeErreur(refus) != "quality_check_required" {
		t.Errorf("contrôle qualité obligatoire : http=%d %v", code, refus)
	}

	// Le passage par le contrôle qualité ouvre la suite.
	if code, rep := proprietaire.appel("PATCH", "/wash-orders/"+fiche,
		map[string]any{"status": "quality_check"}); code != 200 {
		t.Fatalf("contrôle qualité : http=%d %v", code, rep)
	}
	if code, rep := proprietaire.appel("PATCH", "/wash-orders/"+fiche, map[string]any{"status": "ready"}); code != 200 {
		t.Fatalf("véhicule prêt après contrôle : http=%d %v", code, rep)
	}

	// Le laveur suit le même chemin : son portail connaît le contrôle qualité.
	laveur := b.creerEmploye(proprietaire, station, "washer")
	deuxieme := ficheEnAttente(t, b, proprietaire, station, service)
	if code, rep := proprietaire.appel("POST", "/wash-orders/"+deuxieme+"/assign",
		map[string]any{"employee_id": texte(laveur.me["user"].(map[string]any), "id")}); code != 200 {
		t.Fatalf("affectation du laveur : http=%d %v", code, rep)
	}
	if code, rep := laveur.appel("POST", "/washer/tasks/"+deuxieme+"/status",
		map[string]any{"status": "in_progress"}); code != 200 {
		t.Fatalf("démarrage par le laveur : http=%d %v", code, rep)
	}
	if code, rep := laveur.appel("POST", "/washer/tasks/"+deuxieme+"/status",
		map[string]any{"status": "ready"}); code != 400 {
		t.Errorf("le laveur ne déclare pas un véhicule prêt (400) : http=%d %v", code, rep)
	}
	if code, rep := laveur.appel("POST", "/washer/tasks/"+deuxieme+"/status",
		map[string]any{"status": "quality_check"}); code != 200 {
		t.Errorf("le laveur déclare la fin du lavage : http=%d %v", code, rep)
	}
}

func TestReglesDeCommandeAppliqueesParLeServeur(t *testing.T) {
	b, proprietaire, station := stationActive(t)
	service := b.serviceTarife(proprietaire, station)

	// --- Clients de passage refusés ---------------------------------------
	reglerCommandes(t, proprietaire, station, map[string]any{"allow_walk_in": false})
	code, refus := b.ficheCode(proprietaire, map[string]any{
		"station_id": station, "customer_name": "Passage", "status": "pending",
		"items": []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 400 || codeErreur(refus) != "walk_in_not_allowed" {
		t.Errorf("client de passage refusé attendu : http=%d %v", code, refus)
	}
	// Avec un client enregistré, la fiche passe.
	code, client := proprietaire.appel("POST", "/customers", map[string]any{
		"station_id": station, "full_name": "Société Kossi", "phone": "+225 07 11 22 33 44",
	})
	if code != 201 {
		t.Fatalf("création du client : http=%d %v", code, client)
	}
	avecClient := b.fiche(proprietaire, map[string]any{
		"station_id": station, "customer_id": texte(client, "id"), "customer_name": "Société Kossi",
		"status": "pending", "items": []map[string]any{{"service_id": service, "quantity": 1}},
	}, 201)
	reglerCommandes(t, proprietaire, station, map[string]any{"allow_walk_in": true})

	// --- Laveur obligatoire ------------------------------------------------
	reglerCommandes(t, proprietaire, station, map[string]any{"require_washer": true})
	code, refus = b.ficheCode(proprietaire, map[string]any{
		"station_id": station, "customer_name": "Sans laveur", "status": "pending",
		"items": []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 400 || codeErreur(refus) != "washer_required" {
		t.Errorf("laveur obligatoire : http=%d %v", code, refus)
	}
	laveur := b.creerEmploye(proprietaire, station, "washer")
	laveurID := texte(laveur.me["user"].(map[string]any), "id")

	// --- File d'attente : une fiche en cours par laveur ---------------------
	reglerCommandes(t, proprietaire, station,
		map[string]any{"require_washer": false, "max_open_orders_per_washer": 1})
	premiere := b.fiche(proprietaire, map[string]any{
		"station_id": station, "customer_name": "Première", "status": "pending",
		"employee_id": laveurID,
		"items":       []map[string]any{{"service_id": service, "quantity": 1}},
	}, 201)
	code, pleine := b.ficheCode(proprietaire, map[string]any{
		"station_id": station, "customer_name": "Deuxième", "status": "pending",
		"employee_id": laveurID,
		"items":       []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 409 || codeErreur(pleine) != "washer_queue_full" {
		t.Errorf("file d'attente pleine : http=%d %v", code, pleine)
	}
	// Une autre position se libère : la fiche livrée ne compte plus.
	if code, rep := proprietaire.appel("PATCH", "/wash-orders/"+texte(premiere, "id"),
		map[string]any{"status": "canceled", "reason": "Client parti"}); code != 200 {
		t.Fatalf("libération de la file : http=%d %v", code, rep)
	}
	if code, rep := b.ficheCode(proprietaire, map[string]any{
		"station_id": station, "customer_name": "Troisième", "status": "pending",
		"employee_id": laveurID,
		"items":       []map[string]any{{"service_id": service, "quantity": 1}},
	}); code != 201 {
		t.Errorf("la file se libère : http=%d %v", code, rep)
	}
	reglerCommandes(t, proprietaire, station, map[string]any{"max_open_orders_per_washer": 0})

	// --- Acomptes interdits -------------------------------------------------
	if code, rep := proprietaire.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 20000,
	}); code != 201 && code != 409 {
		t.Fatalf("ouverture de caisse : http=%d %v", code, rep)
	}
	reglerCommandes(t, proprietaire, station, map[string]any{"allow_partial_payments": false})
	code, acompte := proprietaire.appel("POST", "/wash-orders/"+texte(avecClient, "id")+"/payments",
		map[string]any{"amount_xof": entier(avecClient["total_xof"]) - 500, "method": "especes"})
	if code != 409 || codeErreur(acompte) != "partial_payment_disabled" {
		t.Errorf("acompte refusé attendu : http=%d %v", code, acompte)
	}
	reglerCommandes(t, proprietaire, station, map[string]any{"allow_partial_payments": true})
	if code, rep := proprietaire.appel("POST", "/wash-orders/"+texte(avecClient, "id")+"/payments",
		map[string]any{"amount_xof": entier(avecClient["total_xof"]) - 500, "method": "especes"}); code != 200 {
		t.Fatalf("acompte accepté quand la règle le permet : http=%d %v", code, rep)
	}
}

func TestRemiseDuVehiculeSubordonneeAuPaiement(t *testing.T) {
	b, proprietaire, station := stationActive(t)
	service := b.serviceTarife(proprietaire, station)
	fiche := ficheEnAttente(t, b, proprietaire, station, service)

	// Par défaut, un véhicule ne se remet pas impayé : c'est la règle qui
	// empêche d'inscrire une fiche en « livrée » parce qu'on a coché une étape.
	code, refus := proprietaire.appel("PATCH", "/wash-orders/"+fiche, map[string]any{"status": "completed"})
	if code != 409 || codeErreur(refus) != "payment_required" {
		t.Errorf("remise impayée refusée attendue : http=%d %v", code, refus)
	}

	// Une station qui facture ses clients plus tard peut l'autoriser : la
	// règle change le comportement, elle ne le décrit pas seulement.
	reglerCommandes(t, proprietaire, station, map[string]any{"release_requires_payment": false})
	code, livre := proprietaire.appel("PATCH", "/wash-orders/"+fiche,
		map[string]any{"status": "completed"})
	if code != 200 {
		t.Fatalf("remise autorisée sans paiement quand la règle le permet : http=%d %v", code, livre)
	}
	if texte(livre, "status") != "completed" {
		t.Errorf("statut attendu completed, obtenu %q", texte(livre, "status"))
	}

	// Une fiche livrée ne retourne pas en atelier… sauf litige, qui est un acte
	// commercial distinct.
	code, retour := proprietaire.appel("PATCH", "/wash-orders/"+fiche, map[string]any{"status": "in_progress"})
	if code != 409 {
		t.Errorf("une fiche livrée ne revient pas en atelier (409) : http=%d %v", code, retour)
	}
	code, litige := proprietaire.appel("PATCH", "/wash-orders/"+fiche,
		map[string]any{"status": "disputed", "dispute_reason": "Client conteste la finition"})
	if code != 200 {
		t.Errorf("un litige reste ouvert après livraison : http=%d %v", code, litige)
	}
}

func TestReglesDeCommandeCloisonnees(t *testing.T) {
	_, proprietaire, station := stationActive(t)
	voisine := nouveauBanc(t).client()
	voisine.inscrire("Voisine Règles")

	// Les règles d'une station ne se lisent ni ne s'écrivent de l'extérieur.
	code, refus := voisine.appel("GET", "/stations/"+station+"/order-rules", nil)
	if code == 200 {
		t.Errorf("les règles d'une autre entreprise restent fermées : http=%d %v", code, refus)
	}
	code, refus = voisine.appel("PUT", "/stations/"+station+"/order-rules",
		map[string]any{"require_washer": true})
	if code == 200 {
		t.Errorf("les règles d'une autre entreprise ne s'écrivent pas : http=%d %v", code, refus)
	}

	// Et la modification par le propriétaire est tracée.
	code, regles := proprietaire.appel("GET", "/stations/"+station+"/order-rules", nil)
	if code != 200 {
		t.Fatalf("GET /order-rules : http=%d %v", code, regles)
	}
	if _, ok := regles["rules"].(map[string]any); !ok {
		t.Fatalf("les règles doivent être renvoyées : %v", regles)
	}
	if labels, ok := regles["labels"].(map[string]any); !ok || len(labels) < 5 {
		t.Errorf("chaque réglage doit avoir son libellé affichable : %v", regles["labels"])
	}
}

func TestOnboardingCouvreLesEtapesOnzeDouzeTreize(t *testing.T) {
	b := nouveauBanc(t)
	proprietaire := b.client()
	proprietaire.inscrire("Parcours Lot 7")

	code, parcours := proprietaire.appel("GET", "/onboarding", nil)
	if code != 200 {
		t.Fatalf("GET /onboarding : http=%d %v", code, parcours)
	}
	etapes, _ := parcours["steps"].([]any)
	if len(etapes) != 13 {
		t.Fatalf("treize étapes attendues, %d obtenue(s)", len(etapes))
	}
	for _, attendu := range []struct{ code, form string }{
		{"equipe", "equipe"}, {"materiel", "materiel"}, {"commandes", "commandes"},
	} {
		etape := etapeOnboarding(parcours, attendu.code)
		if etape == nil {
			t.Fatalf("étape %q absente du parcours", attendu.code)
		}
		if texte(etape, "form") != attendu.form {
			t.Errorf("étape %q : formulaire attendu %q, obtenu %q", attendu.code, attendu.form, texte(etape, "form"))
		}
		// Les trois étapes sont facultatives : une station tenue par son
		// propriétaire, sans matériel déclaré, doit pouvoir ouvrir.
		if requis, _ := etape["required"].(bool); requis {
			t.Errorf("l'étape %q ne doit pas bloquer l'activation", attendu.code)
		}
	}

	// L'étape « règles » n'est jamais vide : les règles par défaut existent,
	// et l'écran doit dire lesquelles s'appliquent.
	regles := etapeOnboarding(parcours, "commandes")
	if texte(regles, "detail") == "" {
		t.Errorf("l'étape « commandes » doit résumer les règles en vigueur : %v", regles["detail"])
	}
	if done, _ := regles["done"].(bool); !done {
		t.Errorf("les règles par défaut s'appliquent : l'étape ne peut pas rester ouverte")
	}

	// Après avoir déclaré du matériel, l'étape le constate.
	proprietaire.remplirProfilEntreprise()
	proprietaire.declarerHoraires()
	proprietaire.declarerCategories()
	proprietaire.assurerPrestationTarifee()
	station := proprietaire.statID
	if code, rep := proprietaire.appel("POST", "/stations/"+station+"/equipment",
		map[string]any{"template_code": "aspirateur", "quantity": 1}); code != 201 {
		t.Fatalf("ajout d'un équipement : http=%d %v", code, rep)
	}
	if code, rep := proprietaire.appel("POST", "/stations/"+station+"/stock/items",
		map[string]any{"name": "Chiffons microfibre", "category_code": "consommables"}); code != 201 {
		t.Fatalf("ajout d'un article : http=%d %v", code, rep)
	}
	code, parcours = proprietaire.appel("GET", "/onboarding", nil)
	if code != 200 {
		t.Fatalf("GET /onboarding : http=%d", code)
	}
	materiel := etapeOnboarding(parcours, "materiel")
	if done, _ := materiel["done"].(bool); !done {
		t.Errorf("l'étape « matériel » doit constater ce qui est déclaré : %v", materiel["detail"])
	}
	if texte(materiel, "detail") == "" {
		t.Errorf("le constat du matériel doit être explicite : %v", materiel["detail"])
	}
	// Et l'activation reste possible : ces étapes ne bloquent pas.
	if code, rep := proprietaire.appel("POST", "/onboarding/activate", nil); code != 200 {
		t.Fatalf("activation avec les étapes 11 à 13 facultatives : http=%d %v", code, rep)
	}
}

// ficheCode crée une fiche et rend le couple (code, réponse) : c'est le test
// qui juge, pas l'assistant.
func (b *banc) ficheCode(c *client, corps map[string]any) (int, map[string]any) {
	c.t.Helper()
	return c.appel("POST", "/wash-orders", corps)
}
