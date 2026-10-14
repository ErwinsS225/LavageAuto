package main

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Stock de la station : consommables, seuils, mouvements.
//
// La règle qui tient tout le fichier : **« je ne sais pas encore » n'est pas
// « zéro »**. Une station neuve qui n'a pas compté ses produits a
// `quantity_known = false` ; l'écran affiche alors « non renseigné », aucune
// alerte de rupture n'est produite, et un mouvement ne fabrique pas de chiffre
// tant que rien n'a été compté. C'est la différence entre un stock honnête et
// un stock qui affiche « 0 flacon » parce que personne n'a ouvert l'armoire.
//
// Un seuil d'alerte sans quantité connue ne déclenche donc rien : il est
// conservé pour le jour où quelqu'un comptera.
// ---------------------------------------------------------------------------

// categorieStockModele : une ligne du référentiel de consommables.
type categorieStockModele struct {
	Code  string
	Nom   string
	Ordre int
}

// referentielCategoriesStock : ce qu'une station de lavage consomme
// réellement. Comme les moyens de paiement, la liste est posée par station et
// peut être renommée localement.
var referentielCategoriesStock = []categorieStockModele{
	{Code: "produits_lavage", Nom: "Produits de lavage", Ordre: 10},
	{Code: "produits_finition", Nom: "Produits de finition (cire, polish)", Ordre: 20},
	{Code: "consommables", Nom: "Consommables (chiffons, éponges)", Ordre: 30},
	{Code: "entretien_materiel", Nom: "Entretien du matériel", Ordre: 40},
	{Code: "carburant", Nom: "Carburant et lubrifiants", Ordre: 50},
	{Code: "eau_traitement", Nom: "Traitement de l'eau", Ordre: 60},
	{Code: "pieces_detachees", Nom: "Pièces détachées", Ordre: 70},
	{Code: "fournitures_bureau", Nom: "Fournitures de bureau et reçus", Ordre: 80},
	{Code: "protection", Nom: "Équipements de protection (gants, bottes)", Ordre: 90},
	{Code: "autre", Nom: "Autre", Ordre: 900},
}

// equiperCategoriesStock amorce les catégories d'une station : chaque ligne
// manquante est créée, aucune n'est écrasée (un renommage local survit à un
// redémarrage).
func (a *App) equiperCategoriesStock(ctx context.Context, orgID, stationID string) error {
	for _, modele := range referentielCategoriesStock {
		if _, err := a.Pool.Exec(ctx, `
			INSERT INTO stock_categories (id, organization_id, station_id, code, name, sort_order, is_active)
			VALUES ($1,$2,$3,$4,$5,$6,true)
			ON CONFLICT (station_id, code) DO NOTHING`,
			newID("stc"), orgID, stationID, modele.Code, modele.Nom, modele.Ordre); err != nil {
			return err
		}
	}
	return nil
}

// assurerCategoriesStock couvre les stations existantes (démarrage).
func (a *App) assurerCategoriesStock(ctx context.Context) error {
	rows, err := a.Pool.Query(ctx, `SELECT id, organization_id FROM stations`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type station struct{ id, orgID string }
	stations := []station{}
	for rows.Next() {
		var s station
		if err := rows.Scan(&s.id, &s.orgID); err != nil {
			return err
		}
		stations = append(stations, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, s := range stations {
		if err := a.equiperCategoriesStock(ctx, s.orgID, s.id); err != nil {
			return err
		}
	}
	return nil
}

// articleStock : un produit suivi par la station.
type articleStock struct {
	ID            string   `json:"id"`
	StationID     string   `json:"station_id"`
	CategoryCode  string   `json:"category_code"`
	CategoryName  string   `json:"category_name"`
	Name          string   `json:"name"`
	InternalCode  string   `json:"internal_code"`
	Unit          string   `json:"unit"`
	QuantityKnown bool     `json:"quantity_known"`
	Quantity      float64  `json:"quantity"`
	UnitCostXOF   int      `json:"unit_cost_xof"`
	MinQuantity   *float64 `json:"min_quantity"`
	Supplier      string   `json:"supplier"`
	Location      string   `json:"location"`
	ExpiresOn     *string  `json:"expires_on"`
	Notes         string   `json:"notes"`
	IsActive      bool     `json:"is_active"`
	ValueXOF      int      `json:"value_xof"`
	// État calculé, jamais deviné :
	//   « inconnu »  : quantité non renseignée — ce n'est pas une rupture ;
	//   « rupture »  : quantité connue ≤ 0 ;
	//   « seuil »    : quantité connue ≤ seuil déclaré ;
	//   « peremption » : date de péremption atteinte ou proche (30 jours) ;
	//   « ok »       : le reste.
	State         string `json:"state"`
	StateLabel    string `json:"state_label"`
	ExpiresSoon   bool   `json:"expires_soon"`
	UpdatedAt     string `json:"updated_at"`
	CreatedAt     string `json:"created_at"`
	Mouvements    int    `json:"movements_count"`
	CreatedByName string `json:"created_by_name"`
}

const articleStockColumns = `s.id, s.station_id, s.category_code, s.name, s.internal_code, s.unit,
	s.quantity_known, s.quantity::float8, s.unit_cost_xof, s.min_quantity::float8, s.supplier,
	s.location, s.expires_on::text, s.notes, s.is_active, s.updated_at::text, s.created_at::text,
	s.created_by_name, COALESCE(c.name, ''),
	(SELECT count(*) FROM stock_movements m WHERE m.item_id = s.id)`

// etatArticleStock calcule l'état d'un article à partir de faits : la quantité
// connue, le seuil déclaré et la date de péremption. Aucune alerte n'est
// produite sur une quantité inconnue.
func etatArticleStock(a articleStock, aujourdhui time.Time) (string, string) {
	if a.ExpiresOn != nil && strings.TrimSpace(*a.ExpiresOn) != "" {
		if jour, err := time.Parse("2006-01-02", strings.TrimSpace(*a.ExpiresOn)); err == nil {
			if !jour.AddDate(0, 0, 30).After(aujourdhui) {
				return "peremption", "Péremption proche"
			}
		}
	}
	if !a.QuantityKnown {
		return "inconnu", "Quantité non renseignée"
	}
	if a.Quantity <= 0 {
		return "rupture", "Rupture"
	}
	if a.MinQuantity != nil && a.Quantity <= *a.MinQuantity {
		return "seuil", "Sous le seuil"
	}
	return "ok", "En stock"
}

func scanArticleStock(row interface{ Scan(dest ...any) error }) (articleStock, error) {
	var a articleStock
	var expires *string
	var categorie string
	if err := row.Scan(&a.ID, &a.StationID, &a.CategoryCode, &a.Name, &a.InternalCode, &a.Unit,
		&a.QuantityKnown, &a.Quantity, &a.UnitCostXOF, &a.MinQuantity, &a.Supplier,
		&a.Location, &expires, &a.Notes, &a.IsActive, &a.UpdatedAt, &a.CreatedAt, &a.CreatedByName,
		&categorie, &a.Mouvements); err != nil {
		return a, err
	}
	a.ExpiresOn = expires
	a.CategoryName = categorie
	if a.CategoryName == "" {
		a.CategoryName = "Sans catégorie"
	}
	if a.QuantityKnown {
		a.ValueXOF = int(a.Quantity * float64(a.UnitCostXOF))
	}
	etat, libelle := etatArticleStock(a, time.Now())
	a.State = etat
	a.StateLabel = libelle
	a.ExpiresSoon = etat == "peremption"
	return a, nil
}

// articlesDeLaStation liste les articles d'une station.
func (a *App) articlesDeLaStation(ctx context.Context, stationID string) ([]articleStock, error) {
	rows, err := a.Pool.Query(ctx, `SELECT `+articleStockColumns+`
		FROM stock_items s LEFT JOIN stock_categories c
		  ON c.station_id = s.station_id AND c.code = s.category_code
		WHERE s.station_id = $1
		ORDER BY s.is_active DESC, c.sort_order, s.name`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []articleStock{}
	for rows.Next() {
		article, err := scanArticleStock(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, article)
	}
	return items, rows.Err()
}

// ---------------------------------------------------------------------------
// GET /api/v1/stations/{id}/stock
// ---------------------------------------------------------------------------

func (a *App) handleStockList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}

	categories := []map[string]any{}
	rows, err := a.Pool.Query(r.Context(), `
		SELECT code, name, sort_order, is_active FROM stock_categories
		WHERE station_id = $1 ORDER BY sort_order, name`, stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	for rows.Next() {
		var code, nom string
		var ordre int
		var actif bool
		if err := rows.Scan(&code, &nom, &ordre, &actif); err != nil {
			continue
		}
		categories = append(categories, map[string]any{
			"code": code, "name": nom, "sort_order": ordre, "is_active": actif,
		})
	}
	rows.Close()

	items, err := a.articlesDeLaStation(r.Context(), stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	// Le résumé sépare trois choses qu'on confond trop souvent :
	//   - ce qui manque vraiment (rupture / sous le seuil, quantités connues) ;
	//   - ce qui n'a jamais été compté (à faire, mais pas une alerte) ;
	//   - la valeur du stock, calculée **uniquement** sur les quantités connues.
	enRupture, sousSeuil, inconnus, valeur := 0, 0, 0, 0
	for _, item := range items {
		if !item.IsActive {
			continue
		}
		switch item.State {
		case "rupture":
			enRupture++
		case "seuil":
			sousSeuil++
		case "inconnu":
			inconnus++
		}
		valeur += item.ValueXOF
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"station_id": stationID,
		"categories": categories,
		"items":      items,
		"summary": map[string]any{
			"articles": len(enServiceArticles(items)), "rupture": enRupture, "sous_seuil": sousSeuil,
			"quantite_inconnue": inconnus, "valeur_xof": valeur,
		},
	})
}

func enServiceArticles(items []articleStock) []articleStock {
	actifs := make([]articleStock, 0, len(items))
	for _, item := range items {
		if item.IsActive {
			actifs = append(actifs, item)
		}
	}
	return actifs
}

// ---------------------------------------------------------------------------
// POST /api/v1/stations/{id}/stock/items
// ---------------------------------------------------------------------------

func (a *App) handleStockItemCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var body struct {
		CategoryCode  string   `json:"category_code"`
		Name          string   `json:"name"`
		InternalCode  string   `json:"internal_code"`
		Unit          string   `json:"unit"`
		QuantityKnown *bool    `json:"quantity_known"`
		Quantity      *float64 `json:"quantity"`
		UnitCostXOF   *int     `json:"unit_cost_xof"`
		MinQuantity   *float64 `json:"min_quantity"`
		Supplier      string   `json:"supplier"`
		Location      string   `json:"location"`
		ExpiresOn     string   `json:"expires_on"`
		Notes         string   `json:"notes"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	nom := strings.TrimSpace(body.Name)
	if nom == "" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Le nom de l'article est obligatoire.", map[string]string{"name": "Nom obligatoire"})
		return
	}

	codeCategorie := strings.TrimSpace(body.CategoryCode)
	if codeCategorie != "" {
		var existe bool
		_ = a.Pool.QueryRow(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM stock_categories WHERE station_id = $1 AND code = $2)`,
			stationID, codeCategorie).Scan(&existe)
		if !existe {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Catégorie de stock inconnue.", map[string]string{"category_code": "Catégorie inconnue"})
			return
		}
	}

	if body.Quantity != nil && *body.Quantity < 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Une quantité ne peut pas être négative : pour un manquant, enregistrez une perte.",
			map[string]string{"quantity": "Quantité invalide"})
		return
	}
	if body.MinQuantity != nil && *body.MinQuantity < 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Le seuil d'alerte ne peut pas être négatif.", map[string]string{"min_quantity": "Seuil invalide"})
		return
	}
	cout := 0
	if body.UnitCostXOF != nil {
		if *body.UnitCostXOF < 0 {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Le coût unitaire ne peut pas être négatif.", map[string]string{"unit_cost_xof": "Montant invalide"})
			return
		}
		cout = *body.UnitCostXOF
	}
	expiration, err := dateOuNil(body.ExpiresOn, "expires_on")
	if err != nil {
		writeAPIError(w, err)
		return
	}

	// La quantité n'est « connue » que si elle a été donnée. Sans quantité, la
	// réponse de l'écran est « je ne sais pas encore » — et elle est conservée
	// comme telle, pas remplacée par 0.
	connue := body.Quantity != nil
	if body.QuantityKnown != nil {
		connue = *body.QuantityKnown && body.Quantity != nil
	}
	quantite := 0.0
	if connue {
		quantite = *body.Quantity
	}

	id := newID("sti")
	if _, err := a.Pool.Exec(r.Context(), `
		INSERT INTO stock_items (id, organization_id, station_id, category_code, name, internal_code,
		  unit, quantity_known, quantity, unit_cost_xof, min_quantity, supplier, location, expires_on,
		  notes, is_active, created_by_name)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,true,$16)`,
		id, deref(user.OrganizationID), stationID, codeCategorie, nom,
		strings.TrimSpace(body.InternalCode), orDefault(strings.TrimSpace(body.Unit), "unité"),
		connue, quantite, cout, body.MinQuantity, strings.TrimSpace(body.Supplier),
		strings.TrimSpace(body.Location), expiration, strings.TrimSpace(body.Notes),
		user.FullName); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'ajout de l'article.", nil)
		return
	}

	// Un article dont la quantité est connue dès la création a son mouvement
	// d'entrée : le stock est justifié par un fait, pas par une saisie directe.
	if connue && quantite > 0 {
		if _, err := a.Pool.Exec(r.Context(), `
			INSERT INTO stock_movements (id, organization_id, station_id, item_id, kind, delta,
			  quantity_after, known_after, unit_cost_xof, reason, created_by, created_by_name)
			VALUES ($1,$2,$3,$4,'entree',$5,$5,true,$6,'Stock initial',$7,$8)`,
			newID("stm"), deref(user.OrganizationID), stationID, id, quantite, cout, user.ID,
			user.FullName); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
	}

	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"stock.item_created", "stock_item", id,
		map[string]any{"name": nom, "quantity_known": connue})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": nom, "quantity_known": connue})
}

// ---------------------------------------------------------------------------
// PATCH /api/v1/stock/items/{id}
// ---------------------------------------------------------------------------

func (a *App) handleStockItemUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	article, err := a.articleDeLEntreprise(r.Context(), deref(user.OrganizationID), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Article introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != article.StationID {
		writeError(w, http.StatusNotFound, "not_found", "Article introuvable.", nil)
		return
	}

	var body struct {
		CategoryCode  *string  `json:"category_code"`
		Name          *string  `json:"name"`
		InternalCode  *string  `json:"internal_code"`
		Unit          *string  `json:"unit"`
		QuantityKnown *bool    `json:"quantity_known"`
		Quantity      *float64 `json:"quantity"`
		UnitCostXOF   *int     `json:"unit_cost_xof"`
		MinQuantity   *float64 `json:"min_quantity"`
		Supplier      *string  `json:"supplier"`
		Location      *string  `json:"location"`
		ExpiresOn     *string  `json:"expires_on"`
		Notes         *string  `json:"notes"`
		IsActive      *bool    `json:"is_active"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	clauses := []string{"updated_at = now()"}
	args := []any{id}
	ajoute := func(sql string, valeur any) {
		args = append(args, valeur)
		clauses = append(clauses, sql+" = $"+itoa(len(args)))
	}
	if body.Name != nil {
		nom := strings.TrimSpace(*body.Name)
		if nom == "" {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Le nom de l'article est obligatoire.", map[string]string{"name": "Nom obligatoire"})
			return
		}
		ajoute("name", nom)
	}
	if body.CategoryCode != nil {
		code := strings.TrimSpace(*body.CategoryCode)
		if code != "" {
			var existe bool
			_ = a.Pool.QueryRow(r.Context(),
				`SELECT EXISTS (SELECT 1 FROM stock_categories WHERE station_id = $1 AND code = $2)`,
				article.StationID, code).Scan(&existe)
			if !existe {
				writeError(w, http.StatusBadRequest, "validation_error",
					"Catégorie de stock inconnue.", map[string]string{"category_code": "Catégorie inconnue"})
				return
			}
		}
		ajoute("category_code", code)
	}
	if body.InternalCode != nil {
		ajoute("internal_code", strings.TrimSpace(*body.InternalCode))
	}
	if body.Unit != nil {
		ajoute("unit", orDefault(strings.TrimSpace(*body.Unit), "unité"))
	}
	if body.UnitCostXOF != nil {
		if *body.UnitCostXOF < 0 {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Le coût unitaire ne peut pas être négatif.", map[string]string{"unit_cost_xof": "Montant invalide"})
			return
		}
		ajoute("unit_cost_xof", *body.UnitCostXOF)
	}
	if body.MinQuantity != nil {
		// Un seuil à zéro ou vide est effacé : « aucun seuil » et « seuil nul »
		// ne veulent pas dire la même chose.
		if *body.MinQuantity <= 0 {
			clauses = append(clauses, "min_quantity = NULL")
		} else {
			ajoute("min_quantity", *body.MinQuantity)
		}
	}
	if body.Supplier != nil {
		ajoute("supplier", strings.TrimSpace(*body.Supplier))
	}
	if body.Location != nil {
		ajoute("location", strings.TrimSpace(*body.Location))
	}
	if body.ExpiresOn != nil {
		expiration, err := dateOuNil(*body.ExpiresOn, "expires_on")
		if err != nil {
			writeAPIError(w, err)
			return
		}
		ajoute("expires_on", expiration)
	}
	if body.Notes != nil {
		ajoute("notes", strings.TrimSpace(*body.Notes))
	}
	if body.IsActive != nil {
		ajoute("is_active", *body.IsActive)
	}

	// Quantité : elle ne s'écrit pas directement. Une correction passe par un
	// mouvement (inventaire ou ajustement) pour laisser une trace lisible.
	// Si l'écran veut seulement dire « je ne sais pas encore », il le déclare.
	if body.Quantity != nil || body.QuantityKnown != nil {
		connue := article.QuantityKnown
		if body.QuantityKnown != nil {
			connue = *body.QuantityKnown
		}
		if body.Quantity != nil {
			if *body.Quantity < 0 {
				writeError(w, http.StatusBadRequest, "validation_error",
					"Une quantité ne peut pas être négative : pour un manquant, enregistrez une perte.",
					map[string]string{"quantity": "Quantité invalide"})
				return
			}
			// Toute quantité qui diffère de celle enregistrée est une
			// correction : elle passe par un mouvement, pour laisser une trace.
			if *body.Quantity != article.Quantity {
				writeError(w, http.StatusConflict, "stock_movement_required",
					"Une quantité se corrige par un mouvement de stock (inventaire, entrée, perte), jamais par une écriture directe.",
					map[string]string{"quantity": "Mouvement de stock requis"})
				return
			}
			connue = true
		}
		ajoute("quantity_known", connue)
		if !connue {
			// « Je ne sais pas encore » : la quantité repart à zéro, mais elle
			// n'est plus présentée comme un chiffre.
			clauses = append(clauses, "quantity = 0")
		}
	}

	args = append(args, deref(user.OrganizationID))
	requete := `UPDATE stock_items SET ` + strings.Join(clauses, ", ") +
		` WHERE id = $1 AND organization_id = $` + itoa(len(args))
	if _, err := a.Pool.Exec(r.Context(), requete, args...); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'enregistrement.", nil)
		return
	}

	a.addAudit(r.Context(), deref(user.OrganizationID), article.StationID, user.FullName,
		"stock.item_updated", "stock_item", id, map[string]any{"name": article.Name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) articleDeLEntreprise(ctx context.Context, orgID, id string) (articleStock, error) {
	return scanArticleStock(a.Pool.QueryRow(ctx, `SELECT `+articleStockColumns+`
		FROM stock_items s LEFT JOIN stock_categories c
		  ON c.station_id = s.station_id AND c.code = s.category_code
		WHERE s.id = $1 AND s.organization_id = $2`, id, orgID))
}

// ---------------------------------------------------------------------------
// Mouvements de stock
// ---------------------------------------------------------------------------

// POST /api/v1/stock/items/{id}/movements
//
// Un mouvement est la seule façon de faire bouger une quantité. Chaque type a
// sa règle, et le résultat est écrit noir sur blanc : quantité après le
// mouvement, et si cette quantité est connue.
func (a *App) handleStockMovementCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	article, err := a.articleDeLEntreprise(r.Context(), deref(user.OrganizationID), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Article introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != article.StationID {
		writeError(w, http.StatusNotFound, "not_found", "Article introuvable.", nil)
		return
	}

	var body struct {
		Kind        string   `json:"kind"`
		Quantity    *float64 `json:"quantity"`
		UnitCostXOF *int     `json:"unit_cost_xof"`
		Reason      string   `json:"reason"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	kind := strings.TrimSpace(body.Kind)
	types := map[string]string{
		"entree":     "Réception d'une commande",
		"sortie":     "Consommation",
		"inventaire": "Comptage physique",
		"ajustement": "Correction justifiée",
		"perte":      "Casse, vol ou produit périmé",
	}
	if _, ok := types[kind]; !ok {
		writeError(w, http.StatusBadRequest, "validation_error", "Type de mouvement inconnu.",
			map[string]string{"kind": "Valeurs acceptées : entree, sortie, inventaire, ajustement, perte"})
		return
	}
	if body.Quantity == nil {
		writeError(w, http.StatusBadRequest, "validation_error",
			"La quantité du mouvement est obligatoire.", map[string]string{"quantity": "Quantité obligatoire"})
		return
	}
	quantite := *body.Quantity
	if quantite < 0 || (kind == "inventaire" && quantite < 0) {
		writeError(w, http.StatusBadRequest, "validation_error",
			"La quantité du mouvement doit être positive ; le sens est donné par le type.",
			map[string]string{"quantity": "Quantité invalide"})
		return
	}
	if quantite == 0 && kind != "inventaire" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Un mouvement de quantité nulle n'apprend rien.", map[string]string{"quantity": "Quantité invalide"})
		return
	}
	// Une correction et une perte doivent être motivées : sans motif, personne
	// ne peut relire l'écart plus tard.
	if (kind == "ajustement" || kind == "perte") && strings.TrimSpace(body.Reason) == "" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Un ajustement ou une perte doit être motivé.", map[string]string{"reason": "Motif requis"})
		return
	}
	// Un inventaire sur un article non compté est légitime : c'est justement la
	// façon de le compter. Une entrée, une sortie ou une perte, non — on ne
	// retire pas d'un stock qu'on n'a jamais compté sans le dire.
	if !article.QuantityKnown && (kind == "sortie" || kind == "perte" || kind == "ajustement") {
		writeError(w, http.StatusConflict, "stock_quantity_unknown",
			"La quantité de cet article n'est pas connue : faites d'abord un inventaire, ou enregistrez une entrée.",
			map[string]string{"quantity_known": "Inventaire requis"})
		return
	}

	cout := article.UnitCostXOF
	if body.UnitCostXOF != nil && *body.UnitCostXOF >= 0 {
		cout = *body.UnitCostXOF
	}

	// Calcul du résultat, type par type.
	var delta float64
	après := article.Quantity
	connueAprès := article.QuantityKnown
	switch kind {
	case "inventaire":
		après = quantite
		connueAprès = true
		delta = 0
	case "entree":
		après = article.Quantity + quantite
		connueAprès = true
		delta = quantite
	case "sortie", "perte":
		après = article.Quantity - quantite
		connueAprès = true
		delta = -quantite
	case "ajustement":
		après = article.Quantity + quantite
		connueAprès = true
		delta = quantite
	}
	if après < 0 {
		writeError(w, http.StatusConflict, "stock_quantity_negative",
			"Le stock ne peut pas devenir négatif : vérifiez la quantité disponible ou corrigez par un inventaire.",
			map[string]string{"quantity": "Stock insuffisant"})
		return
	}

	ctx := r.Context()
	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	mouvementID := newID("stm")
	if _, err := tx.Exec(ctx, `
		INSERT INTO stock_movements (id, organization_id, station_id, item_id, kind, delta, counted,
		  quantity_after, known_after, unit_cost_xof, reason, created_by, created_by_name)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		mouvementID, deref(user.OrganizationID), article.StationID, id, kind, delta,
		compte(kind, quantite), après, connueAprès, cout, strings.TrimSpace(body.Reason),
		user.ID, user.FullName); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	// Le coût unitaire d'une entrée met à jour la valeur de référence de
	// l'article : sinon le stock vaudrait toujours le prix de la première
	// commande.
	if kind == "entree" && body.UnitCostXOF != nil && *body.UnitCostXOF > 0 {
		if _, err := tx.Exec(ctx, `UPDATE stock_items SET unit_cost_xof = $2 WHERE id = $1`, id, cout); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE stock_items SET quantity_known = $2, quantity = $3, updated_at = now()
		WHERE id = $1`, id, connueAprès, après); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	a.addAudit(ctx, deref(user.OrganizationID), article.StationID, user.FullName,
		"stock.movement", "stock_item", id,
		map[string]any{"kind": kind, "quantity": quantite, "reason": strings.TrimSpace(body.Reason)})

	writeJSON(w, http.StatusCreated, map[string]any{
		"id": mouvementID, "kind": kind, "kind_label": types[kind],
		"quantity_after": après, "quantity_known": connueAprès,
	})
}

// compte : la quantité comptée n'a de sens que pour un inventaire.
func compte(kind string, quantite float64) *float64 {
	if kind != "inventaire" {
		return nil
	}
	return &quantite
}

// GET /api/v1/stock/items/{id}/movements — l'historique d'un article.
func (a *App) handleStockMovements(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	article, err := a.articleDeLEntreprise(r.Context(), deref(user.OrganizationID), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Article introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != article.StationID {
		writeError(w, http.StatusNotFound, "not_found", "Article introuvable.", nil)
		return
	}
	libelles := map[string]string{
		"entree": "Réception", "sortie": "Consommation", "inventaire": "Inventaire",
		"ajustement": "Ajustement", "perte": "Perte",
	}
	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, kind, delta::float8, counted::float8, quantity_after::float8, known_after,
		       unit_cost_xof, reason, created_by_name, created_at::text
		FROM stock_movements WHERE item_id = $1 ORDER BY created_at DESC, id DESC LIMIT 100`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var mid, kind, motif, auteur, cree string
		var delta float64
		var compte, apresValeur *float64
		var cout int
		var connue bool
		if err := rows.Scan(&mid, &kind, &delta, &compte, &apresValeur, &connue, &cout, &motif, &auteur, &cree); err != nil {
			continue
		}
		// `apresValeur` reste nul quand la quantité n'était pas connue : le
		// mouvement a eu lieu, la quantité n'a pas été inventée pour autant.
		items = append(items, map[string]any{
			"id": mid, "kind": kind, "kind_label": libelles[kind], "delta": delta,
			"counted": compte, "quantity_after": apresValeur, "quantity_known": connue,
			"unit_cost_xof": cout, "reason": motif, "created_by_name": auteur, "created_at": cree,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"item_id": id, "item_name": article.Name, "unit": article.Unit, "items": items,
		"total": len(items),
	})
}
