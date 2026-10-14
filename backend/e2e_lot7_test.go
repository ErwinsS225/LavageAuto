package main

import (
	"fmt"
	"testing"
	"time"
)

// ===========================================================================
// Lot 7 — étapes 11, 12 et 13 : équipe, matériel et stock, règles de commande.
//
// Ce que ces tests démontrent, dans l'ordre où l'exploitant le vit :
//
//  1. l'équipe se décrit au-delà du compte (fonction, date d'arrivée, horaires,
//     situation) — et **la rémunération ne se lit pas sans permission dédiée** :
//     un caissier qui consulte l'équipe ne voit aucun montant de paie ;
//  2. le matériel s'ajoute à partir du référentiel, sans obligation de
//     l'utiliser, et **rien ne disparaît** : un équipement retiré reste dans
//     l'historique, avec ses dates et son coût ;
//  3. le stock distingue **« non renseigné » de « zéro »** : aucune alerte de
//     rupture n'est produite sur une quantité inconnue, et une sortie ne peut
//     pas se faire sur un stock qui n'a jamais été compté ;
//  4. une quantité ne s'écrit pas à la main : elle bouge par un mouvement, qui
//     laisse une trace (inventaire, entrée, perte motivée) ;
//  5. le cycle de vie d'une fiche est un graphe appliqué par le serveur : les
//     transitions interdites sont refusées, et les règles de la station
//     (contrôle qualité, laveur obligatoire, file d'attente, acomptes, clients
//     de passage, remise subordonnée au paiement) décident — pas l'interface ;
//  6. tout cela reste cloisonné par entreprise.
// ===========================================================================

// stationActive prépare un espace activé, avec sa station et une prestation
// tarifée : le point de départ de tous les tests de ce lot.
func stationActive(t *testing.T) (*banc, *client, string) {
	t.Helper()
	b := nouveauBanc(t)
	proprietaire := b.client()
	proprietaire.inscrire("Station Lot 7")
	proprietaire.activer()
	station, _, _ := b.creerStationEtPrestation(proprietaire)
	return b, proprietaire, station
}

// ---------------------------------------------------------------------------
// 1. L'équipe : décrite, cloisonnée, et la paie réservée
// ---------------------------------------------------------------------------

func TestEquipeDecriteEtRemunerationReservee(t *testing.T) {
	b, proprietaire, station := stationActive(t)

	code, creation := proprietaire.appel("POST", "/employees", map[string]any{
		"full_name":               "Awa Koné",
		"email":                   fmt.Sprintf("awa-%d@test.local", time.Now().UnixNano()),
		"role":                    "cashier",
		"station_id":              station,
		"job_title":               "Caissière principale",
		"hired_on":                "2026-03-02",
		"work_schedule":           "lun–sam, 7h–16h",
		"employment_status":       "active",
		"commission_mode":         "per_wash",
		"commission_per_wash_xof": 250,
	})
	if code != 201 {
		t.Fatalf("création de l'employée : http=%d %v", code, creation)
	}

	// La fiche d'équipe porte bien ce que l'exploitant a saisi.
	code, liste := proprietaire.appel("GET", "/employees", nil)
	if code != 200 {
		t.Fatalf("GET /employees : http=%d %v", code, liste)
	}
	fiche := employeNomme(t, liste, "Awa Koné")
	if texte(fiche, "job_title") != "Caissière principale" ||
		texte(fiche, "hired_on") != "2026-03-02" ||
		texte(fiche, "work_schedule") != "lun–sam, 7h–16h" {
		t.Errorf("les informations d'équipe ne sont pas relues : %v", fiche)
	}
	if texte(fiche, "employment_status") != "active" {
		t.Errorf("situation attendue active, obtenu %q", texte(fiche, "employment_status"))
	}
	if paie, ok := fiche["compensation"].(map[string]any); !ok || entier(paie["per_wash_xof"]) != 250 {
		t.Errorf("le propriétaire doit voir la rémunération qu'il a saisie : %v", fiche["compensation"])
	}

	// Le caissier, lui, peut voir l'équipe (il affecte un laveur) mais **pas**
	// les montants de rémunération.
	caissier := b.client()
	code, connexion := caissier.appel("POST", "/auth/login", map[string]any{
		"email":    texte(creation, "email"),
		"password": texte(creation, "temporary_password"),
	})
	if code != 200 {
		t.Fatalf("connexion du caissier : http=%d %v", code, connexion)
	}
	caissier.me = connexion
	if user, ok := connexion["user"].(map[string]any); ok {
		caissier.role, _ = user["role"].(string)
		caissier.orgID, _ = user["organization_id"].(string)
	}

	code, vueCaissier := caissier.appel("GET", "/employees/options", nil)
	if code != 200 {
		t.Fatalf("le caissier doit voir les options d'équipe : http=%d %v", code, vueCaissier)
	}
	// /employees est protégé par la permission de gestion : le caissier n'y a
	// pas accès du tout. C'est la réponse du serveur, pas celle du navigateur.
	if code, refus := caissier.appel("GET", "/employees", nil); code != 403 {
		t.Errorf("un caissier ne gère pas l'équipe : http=%d %v", code, refus)
	}
	// Il ne peut pas davantage fixer une rémunération, même en connaissant
	// l'identifiant de sa propre fiche.
	code, refus := caissier.appel("PATCH", "/employees/"+texte(creation, "id"),
		map[string]any{"commission_per_wash_xof": 5000})
	if code != 403 {
		t.Errorf("la rémunération est réservée : http=%d %v", code, refus)
	}

	// Un gérant (rôle manager) modifie la fiche, mais pas la paie : la
	// permission dédiée est portée par le propriétaire.
	gerant := b.creerEmploye(proprietaire, station, "manager")
	code, maj := gerant.appel("PATCH", "/employees/"+texte(creation, "id"),
		map[string]any{"job_title": "Chef de caisse", "employment_status": "on_leave"})
	if code != 200 {
		t.Fatalf("un gérant décrit son équipe : http=%d %v", code, maj)
	}
	code, refusPaie := gerant.appel("PATCH", "/employees/"+texte(creation, "id"),
		map[string]any{"commission_mode": "percent", "commission_percent": 10})
	if code != 403 {
		t.Errorf("un gérant sans permission dédiée ne fixe pas les salaires : http=%d %v", code, refusPaie)
	}

	code, relu := proprietaire.appel("GET", "/employees", nil)
	if code != 200 {
		t.Fatalf("GET /employees : http=%d", code)
	}
	après := employeNomme(t, relu, "Awa Koné")
	if texte(après, "job_title") != "Chef de caisse" || texte(après, "employment_status") != "on_leave" {
		t.Errorf("la mise à jour n'est pas relue : %v", après)
	}

	// Une situation inconnue est refusée **champ par champ**.
	code, invalide := proprietaire.appel("PATCH", "/employees/"+texte(creation, "id"),
		map[string]any{"employment_status": "en_vacances"})
	if code != 400 {
		t.Errorf("situation inconnue refusée attendue (400) : http=%d %v", code, invalide)
	}
}

// employeNomme retrouve une fiche d'employé par son nom.
func employeNomme(t *testing.T, liste map[string]any, nom string) map[string]any {
	t.Helper()
	items, _ := liste["items"].([]any)
	for _, brut := range items {
		fiche, _ := brut.(map[string]any)
		if texte(fiche, "full_name") == nom {
			return fiche
		}
	}
	t.Fatalf("employé %q absent de la liste", nom)
	return nil
}

// ---------------------------------------------------------------------------
// 2. Matériel : référentiel, ajout guidé, retrait conservé
// ---------------------------------------------------------------------------

func TestMaterielDuReferentielEtRetraitConserve(t *testing.T) {
	_, proprietaire, station := stationActive(t)
	base := "/stations/" + station + "/equipment"

	code, referentiel := proprietaire.appel("GET", base, nil)
	if code != 200 {
		t.Fatalf("GET %s : http=%d %v", base, code, referentiel)
	}
	modeles, _ := referentiel["templates"].([]any)
	if len(modeles) < 10 {
		t.Fatalf("le référentiel d'équipements doit être amorcé : %d modèle(s)", len(modeles))
	}

	// Ajout à partir d'un modèle : le nom est repris, la famille aussi.
	code, cree := proprietaire.appel("POST", base, map[string]any{
		"template_code": "jet_haute_pression", "quantity": 2, "state": "neuf",
		"acquired_on": "2026-02-10", "cost_xof": 450000,
	})
	if code != 201 {
		t.Fatalf("ajout d'un équipement : http=%d %v", code, cree)
	}
	jet := texte(cree, "id")

	// Ajout libre : un matériel qui n'est pas au référentiel s'ajoute quand
	// même, avec son nom.
	code, libre := proprietaire.appel("POST", base, map[string]any{
		"name": "Remorque de dépannage", "quantity": 1, "state": "bon",
	})
	if code != 201 {
		t.Fatalf("ajout d'un matériel hors référentiel : http=%d %v", code, libre)
	}
	// Un ajout sans nom ni modèle est refusé sur le champ.
	if code, refus := proprietaire.appel("POST", base, map[string]any{"quantity": 1}); code != 400 {
		t.Errorf("un équipement sans nom doit être refusé (400) : http=%d %v", code, refus)
	}

	code, relu := proprietaire.appel("GET", base, nil)
	if code != 200 {
		t.Fatalf("GET %s : http=%d", base, code)
	}
	items, _ := relu["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("deux équipements attendus, %d obtenu(s)", len(items))
	}
	resume, _ := relu["summary"].(map[string]any)
	if entier(resume["en_service"]) != 2 || entier(resume["valeur_xof"]) != 450000 {
		t.Errorf("résumé du parc : %v", resume)
	}

	// Retrait : l'équipement quitte le parc en service, sans disparaître.
	if code, maj := proprietaire.appel("PATCH", "/equipment/"+jet,
		map[string]any{"is_active": false, "state": "hors_service", "notes": "Compresseur HS"}); code != 200 {
		t.Fatalf("retrait d'un équipement : http=%d %v", code, maj)
	}
	code, après := proprietaire.appel("GET", base, nil)
	if code != 200 {
		t.Fatalf("GET %s : http=%d", base, code)
	}
	if len(après["items"].([]any)) != 2 {
		t.Errorf("un équipement retiré reste dans l'historique : %v", après["items"])
	}
	resume = après["summary"].(map[string]any)
	if entier(resume["en_service"]) != 1 || entier(resume["retires"]) != 1 {
		t.Errorf("le parc en service ne compte plus l'équipement retiré : %v", resume)
	}
	// Le retiré garde ses dates et son coût : c'est ce qui en fait un
	// historique plutôt qu'une liste.
	for _, brut := range après["items"].([]any) {
		fiche, _ := brut.(map[string]any)
		if texte(fiche, "id") == jet {
			if texte(fiche, "acquired_on") != "2026-02-10" || entier(fiche["cost_xof"]) != 450000 {
				t.Errorf("l'équipement retiré a perdu son historique : %v", fiche)
			}
			if texte(fiche, "state") != "hors_service" {
				t.Errorf("état attendu hors_service, obtenu %q", texte(fiche, "state"))
			}
		}
	}

	// Isolation : une autre entreprise ne voit ni ne modifie ce matériel.
	autre := nouveauBanc(t).client()
	autre.inscrire("Voisine Lot 7")
	if code, _ := autre.appel("PATCH", "/equipment/"+jet, map[string]any{"name": "Volé"}); code != 404 {
		t.Errorf("le matériel d'une autre entreprise est introuvable (404) : http=%d", code)
	}
}

// ---------------------------------------------------------------------------
// 3 et 4. Stock : « je ne sais pas encore » n'est pas zéro
// ---------------------------------------------------------------------------

func TestStockInconnuNestPasZero(t *testing.T) {
	_, proprietaire, station := stationActive(t)
	base := "/stations/" + station + "/stock"

	code, stock := proprietaire.appel("GET", base, nil)
	if code != 200 {
		t.Fatalf("GET %s : http=%d %v", base, code, stock)
	}
	categories, _ := stock["categories"].([]any)
	if len(categories) < 5 {
		t.Fatalf("les catégories de consommables doivent être amorcées : %d", len(categories))
	}

	// Article déclaré sans quantité : « je ne sais pas encore ».
	code, cree := proprietaire.appel("POST", base+"/items", map[string]any{
		"name": "Savon mousse 5 L", "category_code": "produits_lavage", "unit": "bidon",
		"min_quantity": 4, // seuil déclaré…
	})
	if code != 201 {
		t.Fatalf("création d'un article : http=%d %v", code, cree)
	}
	article := texte(cree, "id")
	if connu, _ := cree["quantity_known"].(bool); connu {
		t.Errorf("sans quantité saisie, l'article ne doit pas être « connu » : %v", cree)
	}

	code, relu := proprietaire.appel("GET", base, nil)
	if code != 200 {
		t.Fatalf("GET %s : http=%d", base, code)
	}
	fiche := articleNomme(t, relu, "Savon mousse 5 L")
	// …mais aucune rupture n'est annoncée : le seuil ne s'applique pas à une
	// quantité que personne n'a comptée.
	if texte(fiche, "state") != "inconnu" {
		t.Errorf("état attendu « inconnu », obtenu %q", texte(fiche, "state"))
	}
	resume, _ := relu["summary"].(map[string]any)
	if entier(resume["rupture"]) != 0 {
		t.Errorf("un stock non renseigné n'est pas une rupture : %v", resume)
	}
	if entier(resume["quantite_inconnue"]) != 1 {
		t.Errorf("le résumé doit compter les articles non comptés : %v", resume)
	}

	// Une sortie sur un stock jamais compté est refusée : on ne retire pas
	// d'une quantité qu'on n'a pas.
	code, refus := proprietaire.appel("POST", "/stock/items/"+article+"/movements",
		map[string]any{"kind": "sortie", "quantity": 1})
	if code != 409 || codeErreur(refus) != "stock_quantity_unknown" {
		t.Errorf("sortie sur stock non compté refusée attendue : http=%d %v", code, refus)
	}

	// L'inventaire est justement la façon de le compter.
	code, inventaire := proprietaire.appel("POST", "/stock/items/"+article+"/movements",
		map[string]any{"kind": "inventaire", "quantity": 3})
	if code != 201 {
		t.Fatalf("inventaire : http=%d %v", code, inventaire)
	}
	if connu, _ := inventaire["quantity_known"].(bool); !connu {
		t.Errorf("après inventaire, la quantité est connue : %v", inventaire)
	}
	if valeur := nombre(inventaire["quantity_after"]); valeur != 3 {
		t.Errorf("quantité attendue 3, obtenu %v", valeur)
	}

	code, relu = proprietaire.appel("GET", base, nil)
	if code != 200 {
		t.Fatalf("GET %s : http=%d", base, code)
	}
	fiche = articleNomme(t, relu, "Savon mousse 5 L")
	// 3 ≤ seuil de 4 : l'alerte est maintenant légitime.
	if texte(fiche, "state") != "seuil" {
		t.Errorf("état attendu « seuil » après inventaire, obtenu %q", texte(fiche, "state"))
	}

	// Une quantité ne s'écrit pas à la main : elle bouge par un mouvement.
	code, direct := proprietaire.appel("PATCH", "/stock/items/"+article, map[string]any{"quantity": 12})
	if code != 409 || codeErreur(direct) != "stock_movement_required" {
		t.Errorf("écriture directe d'une quantité refusée attendue : http=%d %v", code, direct)
	}

	// Une entrée fait remonter le stock, une perte doit être motivée.
	if code, entree := proprietaire.appel("POST", "/stock/items/"+article+"/movements",
		map[string]any{"kind": "entree", "quantity": 6, "unit_cost_xof": 3500}); code != 201 {
		t.Fatalf("entrée de stock : http=%d %v", code, entree)
	}
	code, perteSansMotif := proprietaire.appel("POST", "/stock/items/"+article+"/movements",
		map[string]any{"kind": "perte", "quantity": 1})
	if code != 400 {
		t.Errorf("une perte doit être motivée : http=%d %v", code, perteSansMotif)
	}
	if code, perte := proprietaire.appel("POST", "/stock/items/"+article+"/movements",
		map[string]any{"kind": "perte", "quantity": 1, "reason": "Bidon percé"}); code != 201 {
		t.Fatalf("perte motivée : http=%d %v", code, perte)
	}

	// L'historique explique la quantité actuelle : 3 (inventaire) + 6 − 1 = 8.
	code, mouvements := proprietaire.appel("GET", "/stock/items/"+article+"/movements", nil)
	if code != 200 {
		t.Fatalf("historique des mouvements : http=%d %v", code, mouvements)
	}
	if len(mouvements["items"].([]any)) != 3 {
		t.Errorf("trois mouvements attendus, %v", mouvements["items"])
	}
	code, final := proprietaire.appel("GET", base, nil)
	if code != 200 {
		t.Fatalf("GET %s : http=%d", base, code)
	}
	fiche = articleNomme(t, final, "Savon mousse 5 L")
	if nombre(fiche["quantity"]) != 8 {
		t.Errorf("quantité attendue 8, obtenu %v", fiche["quantity"])
	}
	// La valeur du stock ne compte que ce qui est connu : 8 × 3500.
	if entier(fiche["value_xof"]) != 28000 {
		t.Errorf("valeur attendue 28000, obtenu %v", fiche["value_xof"])
	}

	// Le stock ne franchit pas la frontière d'une entreprise.
	voisine := nouveauBanc(t).client()
	voisine.inscrire("Voisine Stock")
	if code, _ := voisine.appel("GET", base, nil); code == 200 {
		t.Errorf("la station d'une autre entreprise reste inaccessible")
	}
}

// articleNomme retrouve un article de stock par son nom.
func articleNomme(t *testing.T, rep map[string]any, nom string) map[string]any {
	t.Helper()
	for _, brut := range rep["items"].([]any) {
		article, _ := brut.(map[string]any)
		if texte(article, "name") == nom {
			return article
		}
	}
	t.Fatalf("article %q absent du stock", nom)
	return nil
}

// nombre lit un nombre JSON (float64 dans le protocole).
func nombre(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return -1
}
