package main

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// Lot 8 — récapitulatif, activation transactionnelle, tableau de bord.
//
// Ces tests portent sur ce que l'on ne peut pas vérifier « à l'œil » : le fait
// qu'une activation refusée **n'écrive rien**, qu'une activation réussie laisse
// une trace unique, et qu'un tableau de bord sans commande n'invente aucun
// montant. Ils interrogent aussi la base directement (`b.pool`) : c'est le seul
// moyen de constater une absence d'écriture.
// ---------------------------------------------------------------------------

// clesMontants parcourt une réponse JSON et rend les chemins des clés qui
// portent un montant. Aucun écran d'arrivée ne doit en contenir avant la
// première fiche — un zéro affiché ressemble à un résultat, et un résultat
// inventé est exactement ce que le produit s'interdit.
func clesMontants(valeur any, chemin string, trouves *[]string) {
	switch noeud := valeur.(type) {
	case map[string]any:
		for clef, enfant := range noeud {
			// `definitions` est le lexique des indicateurs (« Chiffre d'affaires :
			// total des lavages terminés ») : il nomme des montants sans en
			// porter aucun.
			if clef == "definitions" {
				continue
			}
			suivant := chemin + "." + clef
			// Les compteurs d'articles (`stock_items_out`…) ne sont pas des
			// montants ; ce sont les clés `_xof`, `amount`, `montant` et
			// `revenue` qui en portent un.
			if len(clef) >= 4 && clef[len(clef)-4:] == "_xof" ||
				clef == "amount" || clef == "montant" || clef == "revenue" {
				*trouves = append(*trouves, suivant)
			}
			clesMontants(enfant, suivant, trouves)
		}
	case []any:
		for index, enfant := range noeud {
			clesMontants(enfant, chemin+"["+itoa(index)+"]", trouves)
		}
	}
}

// montantsDans : les chemins des montants d'une réponse.
func montantsDans(reponse map[string]any) []string {
	trouves := []string{}
	clesMontants(reponse, "", &trouves)
	return trouves
}

// codesBloquants : les codes des prérequis manquants listés par le serveur.
func codesBloquants(bloc map[string]any) map[string]bool {
	codes := map[string]bool{}
	if activation, ok := bloc["activation"].(map[string]any); ok {
		items, _ := activation["blockers"].([]any)
		for _, brut := range items {
			item, _ := brut.(map[string]any)
			codes[texte(item, "code")] = true
		}
	}
	return codes
}

// TestRecapitulatifDecritLaStationSansRienInventer vérifie ONB-8.1/8.3 : le
// récapitulatif dit ce que contient la station, ce qui manque, et vers quel
// écran aller — sans un seul montant.
func TestRecapitulatifDecritLaStationSansRienInventer(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Recapitulatif")

	code, summary := c.appel("GET", "/onboarding/summary", nil)
	if code != 200 {
		t.Fatalf("GET /onboarding/summary : http=%d %v", code, summary)
	}

	// Le parcours : treize étapes, dont trois facultatives.
	etapes, _ := summary["steps"].([]any)
	if len(etapes) != 13 {
		t.Errorf("le récapitulatif doit décrire les 13 étapes du parcours, reçu %d", len(etapes))
	}

	org := sousMap(summary, "organization")
	if texte(org, "name") != "Entreprise Recapitulatif" {
		t.Errorf("l'identité de l'entreprise doit figurer au récapitulatif : %v", org)
	}

	setup := sousMap(summary, "setup")
	if len(setup) == 0 {
		t.Fatalf("le récapitulatif doit porter le bloc `setup` : %v", summary)
	}
	activation := sousMap(setup, "activation")
	if etat := texte(activation, "state"); etat != "pending" {
		t.Errorf("un compte neuf est en attente d'activation, reçu %q", etat)
	}
	if actif, _ := activation["active"].(bool); actif {
		t.Error("une entreprise neuve ne doit pas être active")
	}
	if activable, _ := activation["can_activate"].(bool); activable {
		t.Error("une entreprise neuve ne peut pas être activable : aucun prérequis n'est satisfait")
	}
	if bloqueurs := entier(activation["blockers_count"]); bloqueurs == 0 {
		t.Error("l'activation doit annoncer les éléments qui manquent")
	}

	// Le catalogue : présent, non tarifé — le compte neuf reçoit les modèles.
	catalogue := sousMap(setup, "catalogue")
	if entier(catalogue["services_active"]) == 0 {
		t.Error("le catalogue de référence doit être présent au récapitulatif")
	}
	if entier(catalogue["services_priced"]) != 0 {
		t.Errorf("aucun prix ne doit apparaître sur un compte neuf : %v", catalogue)
	}
	if entier(catalogue["payment_methods_active"]) == 0 {
		t.Error("les espèces doivent être actives par défaut : sans moyen de paiement, la caisse ne peut rien encaisser")
	}
	if entier(catalogue["categories_active"]) != 0 {
		t.Errorf("aucune catégorie de véhicule n'est active à l'inscription : %v", catalogue)
	}

	// Les horaires, l'activité : dits sans détour.
	heures := sousMap(setup, "hours")
	if entier(heures["days_open"]) != 0 || texte(heures, "label") == "" {
		t.Errorf("les horaires doivent être constatés vides, avec un libellé : %v", heures)
	}
	activite := sousMap(setup, "activity")
	if total := entier(activite["orders_total"]); total != 0 {
		t.Errorf("aucune fiche n'existe sur un compte neuf, reçu %d", total)
	}
	if aActivite, _ := activite["has_activity"].(bool); aActivite {
		t.Error("un compte neuf n'a aucune activité à présenter")
	}
	readiness := sousMap(setup, "readiness")
	if phrases, _ := readiness["statements"].([]any); len(phrases) == 0 {
		t.Error("l'écran d'arrivée doit avoir des phrases honnêtes à afficher avant la première vente")
	}

	// Les réglages facultatifs : nommés, et signalés comme facultatifs.
	supplementaires := map[string]bool{}
	items, _ := setup["optional_settings"].([]any)
	for _, brut := range items {
		item, _ := brut.(map[string]any)
		supplementaires[texte(item, "code")] = true
		if obligatoire, _ := item["required"].(bool); obligatoire {
			t.Errorf("un réglage facultatif ne peut pas être obligatoire : %v", item)
		}
	}
	for _, attendu := range []string{"usages", "options", "equipe", "materiel", "commandes", "capacites", "localisation"} {
		if !supplementaires[attendu] {
			t.Errorf("le réglage facultatif « %s » doit figurer au récapitulatif", attendu)
		}
	}

	// Aucun montant, nulle part : ni zéro, ni « à titre d'indication ».
	if montants := montantsDans(setup); len(montants) > 0 {
		t.Errorf("le récapitulatif ne doit contenir aucun montant avant la première fiche : %v", montants)
	}
}

// TestActivationRefuseeNommeChaquePrerequis : le refus est précis. Le test
// parcourt le chemin réel — entreprise, horaires, catégories, tarifs — et
// vérifie qu'à chaque étape l'obstacle annoncé est le bon.
func TestActivationRefuseeNommeChaquePrerequis(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Prerequis")

	// 1. Tout manque : le refus doit nommer les prérequis dans `fields` (pour
	//    les formulaires) **et** dans `details.blockers` (pour l'écran).
	code, refus := c.appel("POST", "/onboarding/activate", nil)
	if code != 409 {
		t.Fatalf("l'activation doit être refusée sur un compte neuf : http=%d %v", code, refus)
	}
	if codeErreur(refus) != "onboarding_incomplete" {
		t.Errorf("code d'erreur attendu « onboarding_incomplete », reçu %q", codeErreur(refus))
	}
	erreur := sousMap(refus, "error")
	if champs := sousMap(erreur, "fields"); len(champs) == 0 {
		t.Error("le refus doit lister les éléments manquants, champ par champ")
	}
	details := sousMap(erreur, "details")
	bloqueurs, _ := details["blockers"].([]any)
	if len(bloqueurs) == 0 {
		t.Fatalf("le refus doit détailler les prérequis manquants : %v", details)
	}
	codes := map[string]bool{}
	for _, brut := range bloqueurs {
		item, _ := brut.(map[string]any)
		codes[texte(item, "code")] = true
		if texte(item, "hint") == "" || texte(item, "screen") == "" {
			t.Errorf("chaque obstacle doit dire quoi corriger et où : %v", item)
		}
	}
	for _, attendu := range []string{"entreprise", "horaires", "categories", "tarifs"} {
		if !codes[attendu] {
			t.Errorf("le refus doit nommer « %s » : %v", attendu, codes)
		}
	}
	// Les prestations, elles, ne manquent pas : le compte neuf reçoit les
	// modèles de référence. Ce qui manque, c'est leur prix (ci-dessus).
	if codes["prestations"] {
		t.Errorf("le catalogue de référence satisfait déjà l'étape « prestations » : %v", codes)
	}

	// 2. L'identité de l'entreprise : le blocage disparaît, les autres restent.
	c.remplirProfilEntreprise()
	_, parcours := c.appel("GET", "/onboarding", nil)
	if point := etapeOnboarding(parcours, "entreprise"); point == nil || point["done"] != true {
		t.Errorf("l'étape « entreprise » doit être satisfaite après saisie : %v", point)
	}

	// 3. Les horaires.
	c.declarerHoraires()
	_, parcours = c.appel("GET", "/onboarding", nil)
	if point := etapeOnboarding(parcours, "horaires"); point == nil || point["done"] != true {
		t.Errorf("l'étape « horaires » doit être satisfaite après saisie : %v", point)
	}

	// 4. Les catégories de véhicules.
	c.declarerCategories()
	_, parcours = c.appel("GET", "/onboarding", nil)
	if point := etapeOnboarding(parcours, "categories"); point == nil || point["done"] != true {
		t.Errorf("l'étape « catégories » doit être satisfaite après activation : %v", point)
	}

	// 5. Les tarifs : le catalogue est livré **sans prix**, donc l'activation
	//    reste refusée — et le refus nomme l'étape « tarifs ».
	code2, refus := c.appel("POST", "/onboarding/activate", nil)
	if code2 != 409 {
		t.Fatalf("un catalogue sans prix ne doit pas pouvoir être activé : http=%d %v", code2, refus)
	}
	if codeErreur(refus) != "onboarding_incomplete" {
		t.Errorf("code d'erreur attendu « onboarding_incomplete », reçu %q", codeErreur(refus))
	}
	blocRefus := sousMap(sousMap(sousMap(refus, "error"), "details"), "setup")
	if codes := codesBloquants(blocRefus); !codes["tarifs"] {
		t.Errorf("le refus doit nommer l'étape « tarifs » : %v", codes)
	} else if len(codes) != 1 {
		t.Errorf("seul « tarifs » doit encore manquer : %v", codes)
	}

	// Les trois étapes facultatives ne bloquent pas : l'équipe, le matériel et
	// les règles de commande restent à faire après l'ouverture (ONB-8.2).
	supplementaires, _ := blocRefus["optional_settings"].([]any)
	if len(supplementaires) == 0 {
		t.Error("le refus doit aussi rappeler ce qui reste facultatif")
	}

	// 6. La grille tarifaire : l'activation passe.
	c.assurerPrestationTarifee()
	code, active := c.appel("POST", "/onboarding/activate", nil)
	if code != 200 {
		t.Fatalf("l'activation doit aboutir une fois les prérequis satisfaits : http=%d %v", code, active)
	}
	if deja, _ := active["already_active"].(bool); deja {
		t.Error("la première activation ne peut pas être « déjà active »")
	}
	if etat := b.etatOnboarding(c.orgID); etat != "active" {
		t.Errorf("l'état enregistré doit passer à « active », reçu %q", etat)
	}
	// La réponse de l'activation porte déjà l'état complet du parcours.
	_, apres := c.appel("GET", "/onboarding", nil)

	// 7. La trace : un instantané, un auteur, une seule ligne de journal.
	var auteur, nom string
	var statut, snapshot string
	if err := b.pool.QueryRow(context.Background(), `
		SELECT onboarding_activated_by, onboarding_activated_by_name,
		       COALESCE(onboarding_completed_at::text, ''), onboarding_snapshot::text
		FROM organizations WHERE id = $1`, c.orgID).Scan(&auteur, &nom, &statut, &snapshot); err != nil {
		t.Fatalf("lecture de la trace d'activation : %v", err)
	}
	if auteur == "" || nom == "" || statut == "" {
		t.Errorf("l'activation doit laisser un auteur et une date : %q %q %q", auteur, nom, statut)
	}
	if len(snapshot) < 2 || snapshot == "{}" {
		t.Errorf("l'instantané d'activation doit être enregistré : %q", snapshot)
	}
	var lignes int
	_ = b.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_logs WHERE organization_id = $1 AND action = 'onboarding.activated'`,
		c.orgID).Scan(&lignes)
	if lignes != 1 {
		t.Errorf("l'activation doit être journalisée une seule fois, reçu %d ligne(s)", lignes)
	}

	// 8. Les trois étapes facultatives restent atteignables après l'ouverture :
	//    une station s'active sans équipe ni matériel, et peut les décrire
	//    ensuite. Leur état est *constaté*, pas déclaré :
	//      - « équipe » est satisfaite par le compte du propriétaire lui-même ;
	//      - « commandes » est satisfaite par les règles par défaut, qui
	//        s'appliquent toujours — ce qui compte est de savoir lesquelles ;
	//      - « matériel » reste à faire tant que rien n'a été déclaré.
	_, apres = c.appel("GET", "/onboarding", nil)
	attendus := map[string]bool{"equipe": true, "materiel": false, "commandes": true}
	for codeEtape, doitEtreFait := range attendus {
		point := etapeOnboarding(apres, codeEtape)
		if point == nil {
			t.Fatalf("l'étape « %s » doit rester dans le parcours après activation", codeEtape)
		}
		if obligatoire, _ := point["required"].(bool); obligatoire {
			t.Errorf("l'étape « %s » est facultative : elle ne doit pas bloquer l'activation", codeEtape)
		}
		if fait, _ := point["done"].(bool); fait != doitEtreFait {
			t.Errorf("l'étape « %s » : « fait » attendu = %v, reçu %v (%v)",
				codeEtape, doitEtreFait, fait, point["detail"])
		}
		if texte(point, "detail") == "" {
			t.Errorf("l'étape « %s » doit constater ce qui existe, pas afficher un état sans justification", codeEtape)
		}
	}

	// 9. Réactiver est idempotent : même réponse, aucune nouvelle trace.
	code, rappel := c.appel("POST", "/onboarding/activate", nil)
	if code != 200 {
		t.Fatalf("une seconde activation doit répondre 200 : http=%d %v", code, rappel)
	}
	if deja, _ := rappel["already_active"].(bool); !deja {
		t.Error("la seconde activation doit se reconnaître comme « déjà active »")
	}
	_ = b.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_logs WHERE organization_id = $1 AND action = 'onboarding.activated'`,
		c.orgID).Scan(&lignes)
	if lignes != 1 {
		t.Errorf("une activation répétée ne doit pas ajouter de ligne au journal, reçu %d", lignes)
	}
}

// TestActivationRefuseeNEcritRien : le refus ne laisse aucune écriture — ni
// état, ni date, ni instantané, ni journal. C'est la raison d'être de la
// transaction (ONB-8.6 : « rollback complet si une étape d'activation échoue »).
func TestActivationRefuseeNEcritRien(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Rollback")

	if code, _ := c.appel("POST", "/onboarding/activate", nil); code != 409 {
		t.Fatalf("l'activation doit être refusée : http=%d", code)
	}

	var etat, debut, fin, auteur, nom, snapshot string
	if err := b.pool.QueryRow(context.Background(), `
		SELECT onboarding_state, COALESCE(onboarding_started_at::text, ''),
		       COALESCE(onboarding_completed_at::text, ''), onboarding_activated_by,
		       onboarding_activated_by_name, onboarding_snapshot::text
		FROM organizations WHERE id = $1`, c.orgID).
		Scan(&etat, &debut, &fin, &auteur, &nom, &snapshot); err != nil {
		t.Fatalf("lecture de l'entreprise : %v", err)
	}
	if etat != "pending" {
		t.Errorf("un refus ne doit pas changer l'état, reçu %q", etat)
	}
	if debut != "" || fin != "" {
		t.Errorf("un refus ne doit pas dater l'activation : début=%q fin=%q", debut, fin)
	}
	if auteur != "" || nom != "" {
		t.Errorf("un refus ne doit attribuer l'activation à personne : %q %q", auteur, nom)
	}
	if snapshot != "{}" {
		t.Errorf("un refus ne doit rien enregistrer comme constaté : %q", snapshot)
	}
	var lignes int
	_ = b.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_logs WHERE organization_id = $1 AND action = 'onboarding.activated'`,
		c.orgID).Scan(&lignes)
	if lignes != 0 {
		t.Errorf("un refus ne doit pas laisser de trace d'activation, reçu %d", lignes)
	}
}

// TestTableauDeBordSansActiviteNAfficheAucunMontant : avant la première fiche,
// l'écran d'arrivée raconte la station et n'avance aucun chiffre. Après la
// première fiche, les montants reviennent — ce sont ceux du jour, réels.
func TestTableauDeBordSansActiviteNAfficheAucunMontant(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Tableau")
	c.activer()

	code, tableau := c.appel("GET", "/reports/dashboard", nil)
	if code != 200 {
		t.Fatalf("GET /reports/dashboard : http=%d %v", code, tableau)
	}
	setup := sousMap(tableau, "setup")
	if len(setup) == 0 {
		t.Fatalf("le tableau de bord doit porter le bloc de mise en route : %v", tableau)
	}
	activite := sousMap(setup, "activity")
	if total := entier(activite["orders_total"]); total != 0 {
		t.Errorf("aucune fiche n'existe encore, reçu %d", total)
	}
	if sans, _ := tableau["no_activity"].(bool); !sans {
		t.Errorf("le tableau de bord doit se déclarer sans activité : %v", tableau["no_activity_note"])
	}
	// Le cœur de l'exigence : pas un seul montant dans la réponse.
	if montants := montantsDans(tableau); len(montants) > 0 {
		t.Errorf("aucun montant ne doit être présenté avant la première fiche : %v", montants)
	}
	for _, interdit := range []string{"today", "series", "goal", "payment_methods", "top_services", "employees"} {
		if _, present := tableau[interdit]; present {
			t.Errorf("le bloc « %s » ne doit pas être servi sans activité", interdit)
		}
	}
	// La caisse, elle, reste décrite : « aucune session ouverte » est un fait
	// d'exploitation, et c'est là qu'on ouvre la caisse du jour.
	if _, present := tableau["cash"]; !present {
		t.Error("l'état de la caisse doit rester lisible même sans activité")
	}

	// Le catalogue, lui, est décrit : c'est ce que l'exploitant peut encore
	// régler, et c'est utile dès la première minute.
	catalogue := sousMap(setup, "catalogue")
	if entier(catalogue["services_priced"]) == 0 {
		t.Error("le tableau de bord doit constater le catalogue tarifé au moment de l'activation")
	}
	if entier(catalogue["payment_methods_active"]) == 0 {
		t.Error("le tableau de bord doit constater les moyens de paiement actifs")
	}
	heures := sousMap(setup, "hours")
	if entier(heures["days_open"]) == 0 {
		t.Error("le tableau de bord doit constater les journées d'ouverture déclarées")
	}
	if _, present := heures["today"]; !present {
		t.Error("le tableau de bord doit dire si la station est ouverte maintenant")
	}

	// La première fiche : les montants reviennent, parce qu'ils existent.
	station, service, prix := b.creerStationEtPrestation(c)
	code, fiche := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("la création d'une fiche doit être acceptée après activation : http=%d %v", code, fiche)
	}
	if entier(fiche["total_xof"]) != prix {
		t.Errorf("le prix de la fiche doit venir du barème (%d), reçu %v", prix, fiche["total_xof"])
	}

	code, tableau = c.appel("GET", "/reports/dashboard", nil)
	if code != 200 {
		t.Fatalf("GET /reports/dashboard après la première fiche : http=%d", code)
	}
	if sans, _ := tableau["no_activity"].(bool); sans {
		t.Error("le tableau de bord ne doit plus se déclarer sans activité")
	}
	today := sousMap(tableau, "today")
	if _, present := today["revenue_xof"]; !present {
		t.Errorf("les montants du jour doivent revenir dès la première fiche : %v", today)
	}
	if total := entier(sousMap(sousMap(tableau, "setup"), "activity")["orders_total"]); total != 1 {
		t.Errorf("le tableau de bord doit compter la fiche créée, reçu %d", total)
	}
}

// TestTableauDeBordRespecteLesRoles : le caissier n'obtient pas les blocs
// d'administration, le laveur n'obtient pas le tableau de bord du tout (donc
// aucun montant), et un profil sans accès aux paiements ne reçoit aucun bloc
// financier — même si, aujourd'hui, le filtre est une ceinture de sécurité.
func TestTableauDeBordRespecteLesRoles(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Roles")
	c.activer()

	station := c.statID
	caissier := b.creerEmploye(c, station, "cashier")
	laveur := b.creerEmploye(c, station, "washer")

	code, tableau := caissier.appel("GET", "/reports/dashboard", nil)
	if code != 200 {
		t.Fatalf("le caissier doit voir le tableau de bord : http=%d %v", code, tableau)
	}
	setup := sousMap(tableau, "setup")
	blocs, _ := setup["blocks"].([]any)
	codes := map[string]bool{}
	for _, brut := range blocs {
		bloc, _ := brut.(map[string]any)
		codes[texte(bloc, "code")] = true
		if texte(bloc, "permission") == "" || texte(bloc, "href") == "" {
			t.Errorf("un bloc proposé doit dire où il mène : %v", bloc)
		}
	}
	if len(codes) != 0 {
		t.Errorf("un caissier n'administre rien : aucun bloc ne doit lui être proposé, reçu %v", codes)
	}
	can := sousMap(setup, "can")
	if gestion, _ := can["manage_settings"].(bool); gestion {
		t.Error("un caissier ne gère pas les réglages de la station")
	}
	// Ses raccourcis, eux, existent : encaisser est son métier.
	raccourcis, _ := setup["shortcuts"].([]any)
	if len(raccourcis) == 0 {
		t.Error("un caissier doit garder des raccourcis d'exploitation")
	}
	for _, brut := range raccourcis {
		raccourci, _ := brut.(map[string]any)
		if texte(raccourci, "code") == "rapports" {
			t.Error("un caissier n'a pas accès aux rapports : le raccourci ne doit pas être proposé")
		}
	}

	// Le propriétaire, lui, voit les blocs d'administration.
	_, tableauPatron := c.appel("GET", "/reports/dashboard", nil)
	blocsPatron, _ := sousMap(tableauPatron, "setup")["blocks"].([]any)
	if len(blocsPatron) == 0 {
		t.Error("le propriétaire doit voir les blocs d'administration")
	}

	// Le laveur : aucun montant, parce qu'aucun accès au tableau de bord.
	code, refus := laveur.appel("GET", "/reports/dashboard", nil)
	if code != 403 {
		t.Fatalf("le laveur ne doit pas lire le tableau de bord : http=%d %v", code, refus)
	}

	// Et le récapitulatif de mise en route ne lui est pas ouvert non plus :
	// il expose la configuration de l'entreprise.
	if code, _ := laveur.appel("GET", "/onboarding/summary", nil); code != 403 {
		t.Errorf("le laveur ne doit pas lire le récapitulatif : http=%d", code)
	}

	// Son propre poste, en revanche, lui est ouvert — sans un seul montant :
	// ni le prix du client, ni la recette de la station.
	stationFiche, serviceFiche, _ := b.creerStationEtPrestation(c)
	code, fiche := c.appel("POST", "/wash-orders", map[string]any{
		"station_id":  stationFiche,
		"employee_id": texte(sousMap(laveur.me, "user"), "id"),
		"items":       []map[string]any{{"service_id": serviceFiche, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("attribution d'une fiche au laveur : http=%d %v", code, fiche)
	}
	code, taches := laveur.appel("GET", "/washer/tasks", nil)
	if code != 200 {
		t.Fatalf("GET /washer/tasks : http=%d %v", code, taches)
	}
	items, _ := taches["items"].([]any)
	if len(items) == 0 {
		t.Fatal("le laveur doit voir la fiche qui lui est attribuée")
	}
	if montants := montantsDansTaches(items); len(montants) > 0 {
		t.Errorf("le poste laveur ne doit exposer aucun montant : %v", montants)
	}
}

// montantsDansTaches : les clés en `_xof` d'une liste de tâches du laveur.
func montantsDansTaches(items []any) []string {
	trouves := []string{}
	var parcourir func(valeur any, chemin string)
	parcourir = func(valeur any, chemin string) {
		switch noeud := valeur.(type) {
		case map[string]any:
			for clef, enfant := range noeud {
				suivant := chemin + "." + clef
				if len(clef) >= 4 && clef[len(clef)-4:] == "_xof" {
					trouves = append(trouves, suivant)
				}
				parcourir(enfant, suivant)
			}
		case []any:
			for index, enfant := range noeud {
				parcourir(enfant, chemin+"["+itoa(index)+"]")
			}
		}
	}
	parcourir(items, "")
	return trouves
}

// TestSansMontantsRetireLesBlocsFinanciers : la fonction de retrait est pure ;
// on l'éprouve directement, sans passer par la base.
func TestSansMontantsRetireLesBlocsFinanciers(t *testing.T) {
	reponse := map[string]any{
		"date": "2026-01-01", "station_id": "st_1",
		"today":           map[string]any{"revenue_xof": 0},
		"series":          []any{map[string]any{"revenue_xof": 0}},
		"cash":            map[string]any{"expected_xof": 0},
		"employees":       []any{map[string]any{"revenue_xof": 0}},
		"top_services":    []any{map[string]any{"service_name": "Lavage", "revenue_xof": 0}},
		"payment_methods": []any{map[string]any{"method": "especes", "amount_xof": 0}},
		"goal":            map[string]any{"target_xof": 0},
		"recent":          []any{map[string]any{"type": "payment", "amount_xof": 0}},
		"setup":           map[string]any{"catalogue": map[string]any{"services_active": 3}},
		"definitions":     map[string]any{},
	}
	propre := sansMontants(reponse)
	if montants := montantsDans(propre); len(montants) > 0 {
		t.Errorf("aucun montant ne doit survivre au retrait : %v", montants)
	}
	if masque, _ := propre["amounts_hidden"].(bool); !masque {
		t.Error("le retrait doit être annoncé, pas silencieux")
	}
	if texte(propre, "date") != "2026-01-01" || len(sousMap(propre, "setup")) == 0 {
		t.Error("le retrait ne doit toucher que les montants : le reste de la réponse est conservé")
	}
	if _, present := reponse["today"]; !present {
		t.Error("la fonction ne doit pas modifier la réponse d'origine")
	}

	// Un tableau de bord sans activité le dit différemment : ce n'est pas un
	// droit, c'est un constat.
	vide := sansMontantsDuJour(reponse)
	if sans, _ := vide["no_activity"].(bool); !sans {
		t.Error("un tableau de bord sans fiche doit se déclarer sans activité")
	}
	if _, present := vide["amounts_hidden"]; present {
		t.Error("« sans activité » n'est pas « profil sans droits » : la réponse ne doit pas prétendre le contraire")
	}
}
