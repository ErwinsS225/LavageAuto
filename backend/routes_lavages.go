package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Statuts des commandes de lavage (7 états)
const (
	StatutCommandeCreee       = "created"
	StatutCommandeEnCours     = "in_progress"
	StatutCommandeTerminee    = "completed"
	StatutCommandeAnnulee     = "cancelled"
	StatutCommandeEncaissement = "paid"
	StatutCommandeEnAttente   = "pending"
	StatutCommandeRetard      = "delayed"
)

// transitionsAutorisees — seuls changements de statut valides
var transitionsAutorisees = map[string][]string{
	StatutCommandeCreee:       {StatutCommandeEnCours, StatutCommandeAnnulee, StatutCommandeEnAttente},
	StatutCommandeEnCours:     {StatutCommandeTerminee, StatutCommandeRetard, StatutCommandeAnnulee},
	StatutCommandeTerminee:    {StatutCommandeEncaissement},
	StatutCommandeEnAttente:   {StatutCommandeEnCours, StatutCommandeAnnulee},
	StatutCommandeRetard:      {StatutCommandeEnCours, StatutCommandeTerminee, StatutCommandeAnnulee},
	StatutCommandeAnnulee:     {},
	StatutCommandeEncaissement: {},
}

// Structures de données
type WashOrder struct {
	ID                string    `json:"id"`
	StationID         string    `json:"station_id"`
	VehicleID         string    `json:"vehicle_id"`
	PackageID         string    `json:"package_id"`
	AssignedToID      *string   `json:"assigned_to_id,omitempty"`
	CustomerName      string    `json:"customer_name"`
	CustomerPhone     *string   `json:"customer_phone,omitempty"`
	Notes             *string   `json:"notes,omitempty"`
	UnitPriceXOF      int       `json:"unit_price_xof"`
	LineTotalXOF      int       `json:"line_total_xof"`
	DueXOF            int       `json:"due_xof"`
	PriceSource       string    `json:"price_source"`
	PriceSourceLabel  string    `json:"price_source_label"`
	Options           interface{} `json:"options,omitempty"`
	Status            string    `json:"status"`
	ReceiptNumber     *string   `json:"receipt_number,omitempty"`
	CancelledReason   *string   `json:"cancelled_reason,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         *time.Time `json:"updated_at,omitempty"`
	CreatedBy         string    `json:"created_by"`
}

type Payment struct {
	ID                string    `json:"id"`
	WashOrderID       string    `json:"wash_order_id"`
	AmountXOF         int       `json:"amount_xof"`
	Method            string    `json:"method"`
	IsVoid            bool      `json:"is_void"`
	VoidReason        *string   `json:"void_reason,omitempty"`
	CashSessionID     *string   `json:"cash_session_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	CreatedBy         string    `json:"created_by"`
}

// Requêtes
type CreerWashOrderRequest struct {
	StationID         string                 `json:"station_id" validate:"required,uuid"`
	VehicleID         string                 `json:"vehicle_id" validate:"required,uuid"`
	PackageID         string                 `json:"package_id" validate:"required,uuid"`
	CustomerName      string                 `json:"customer_name" validate:"required,min=2"`
	CustomerPhone     *string                `json:"customer_phone,omitempty"`
	Notes             *string                `json:"notes,omitempty"`
	Options           map[string]interface{} `json:"options,omitempty"`
}

type AjouterPaiementRequest struct {
	AmountXOF         int    `json:"amount_xof" validate:"required,min=1"`
	Method            string `json:"method" validate:"required,oneof=cash mobile card deferred"`
}

type AnnulerWashOrderRequest struct {
	Reason            string `json:"reason" validate:"required,min=2"`
}

type AffecterWashOrderRequest struct {
	AssignedToID      string `json:"assigned_to_id" validate:"required,uuid"`
}

// transitionAutorisee — vérification de validité de changement de statut
func transitionAutorisee(ancien, nouveau string) bool {
	autres, ok := transitionsAutorisees[ancien]
	if !ok {
		return false
	}
	for _, possible := range autres {
		if possible == nouveau {
			return true
		}
	}
	return false
}

// cloturerFicheSiSoldee — clôture automatique si la commande est payée en totalité
func (a *App) cloturerFicheSiSoldee(ctx context.Context, commandeID string, userID string) error {
	var totalPaye, dueXof int
	var stationID string
	a.db.QueryRow(ctx, `SELECT COALESCE(SUM(amount_xof),0) FROM payments WHERE wash_order_id = $1 AND is_void = false`, commandeID).Scan(&totalPaye)
	a.db.QueryRow(ctx, `SELECT due_xof, station_id FROM wash_orders WHERE id = $1`, commandeID).Scan(&dueXof, &stationID)
	if totalPaye >= dueXof {
		_, err := a.db.Exec(ctx, `UPDATE wash_orders SET status = $1, updated_by = $2, updated_at = NOW() WHERE id = $3`,
			StatutCommandeEncaissement, userID, commandeID)
		if err != nil {
			return err
		}
		return a.genererRecu(ctx, stationID, commandeID)
	}
	return nil
}

// genererRecu — reçu unique avec verrouillage PostgreSQL (idempotent)
func (a *App) genererRecu(ctx context.Context, stationID, commandeID string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var compteurActuel, prefixe string
	err = tx.QueryRow(ctx, `SELECT receipt_counter, receipt_prefix FROM stations WHERE id = $1 FOR UPDATE`, stationID).Scan(&compteurActuel, &prefixe)
	if err != nil {
		return err
	}
	compteurActuel = fmt.Sprintf("%d", mustAtoi(compteurActuel)+1)
	tx.Exec(ctx, `UPDATE stations SET receipt_counter = $1 WHERE id = $2`, compteurActuel, stationID)
	numeroRecu := fmt.Sprintf("%s-%s-%04d", prefixe, time.Now().Format("20060102"), mustAtoi(compteurActuel))
	_, err = tx.Exec(ctx, `UPDATE wash_orders SET receipt_number = $1 WHERE id = $2`, numeroRecu, commandeID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Handlers
func (a *App) creerWashOrder(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !a.stationActive(r.Context(), *sess.OrganizationID) {
		repondreErreur(w, http.StatusConflict, "onboarding_incomplete", "Station non activée.")
		return
	}
	corps, ok := decoderCorps[CreerWashOrderRequest](w, r)
	if !ok {
		return
	}
	if err := a.verifierAppartenanceStation(r.Context(), corps.StationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station inconnue.")
		return
	}
	// Calculer prix via moteur Lot 3
	prix, err := a.calculerPrixLavage(r.Context(), corps.StationID, corps.VehicleID, corps.PackageID, corps.Options)
	if err != nil {
		repondreErreur(w, http.StatusBadRequest, "pricing_error", err.Error())
		return
	}
	// Insérer la commande
	var nouvelID string
	err = a.db.QueryRow(r.Context(), `INSERT INTO wash_orders (station_id, vehicle_id, package_id, customer_name, customer_phone, notes, unit_price_xof, line_total_xof, due_xof, price_source, price_source_label, options, status, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`,
		corps.StationID, corps.VehicleID, corps.PackageID, corps.CustomerName, corps.CustomerPhone, corps.Notes, prix["unit_price"], prix["total"], prix["total"], prix["source"], prix["source_label"], corps.Options, StatutCommandeCreee, sess.UserID).Scan(&nouvelID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "creation_failed", err.Error())
		return
	}
	ecrireJSON(w, http.StatusCreated, map[string]string{"id": nouvelID})
}

func (a *App) ajouterPaiementWashOrder(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	commandeID := cheminParam(r, "/api/v1/wash-orders/")
	corps, ok := decoderCorps[AjouterPaiementRequest](w, r)
	if !ok {
		return
	}
	// Vérifier session de caisse pour paiement cash
	var sessionOuverte string
	if corps.Method == "cash" {
		a.db.QueryRow(r.Context(), `SELECT id FROM cash_sessions WHERE station_id = (SELECT station_id FROM wash_orders WHERE id = $1) AND is_open = true`, commandeID).Scan(&sessionOuverte)
		if sessionOuverte == "" {
			repondreErreur(w, http.StatusConflict, "no_open_cash_session", "Session de caisse requise pour paiement en espèces.")
			return
		}
	}
	// Enregistrer paiement
	var nouvelID string
	a.db.QueryRow(r.Context(), `INSERT INTO payments (wash_order_id, amount_xof, method, cash_session_id, created_by) VALUES ($1,$2,$3,$4,$5) RETURNING id`, commandeID, corps.AmountXOF, corps.Method, sessionOuverte, sess.UserID).Scan(&nouvelID)
	// Clôturer automatiquement si soldé
	go a.cloturerFicheSiSoldee(r.Context(), commandeID, sess.UserID)
	ecrireJSON(w, http.StatusCreated, map[string]string{"id": nouvelID})
}

func (a *App) annulerWashOrder(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	commandeID := cheminParam(r, "/api/v1/wash-orders/")
	corps, ok := decoderCorps[AnnulerWashOrderRequest](w, r)
	if !ok {
		return
	}
	var ancienStatut string
	a.db.QueryRow(r.Context(), `SELECT status FROM wash_orders WHERE id = $1`, commandeID).Scan(&ancienStatut)
	if !transitionAutorisee(ancienStatut, StatutCommandeAnnulee) {
		repondreErreur(w, http.StatusConflict, "transition_forbidden", "Annulation impossible.")
		return
	}
	a.db.Exec(r.Context(), `UPDATE wash_orders SET status = $1, cancelled_reason = $2, updated_by = $3 WHERE id = $4`, StatutCommandeAnnulee, corps.Reason, sess.UserID, commandeID)
	ecrireJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (a *App) affecterWashOrder(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	commandeID := cheminParam(r, "/api/v1/wash-orders/")
	corps, ok := decoderCorps[AffecterWashOrderRequest](w, r)
	if !ok {
		return
	}
	a.db.Exec(r.Context(), `UPDATE wash_orders SET assigned_to_id = $1, updated_by = $2 WHERE id = $3`, corps.AssignedToID, sess.UserID, commandeID)
	ecrireJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (a *App) listerWashOrders(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	stationID := r.URL.Query().Get("station_id")
	rows, _ := a.db.Query(r.Context(), `SELECT id, station_id, vehicle_id, package_id, customer_name, status, due_xof, created_at FROM wash_orders WHERE station_id = $1 ORDER BY created_at DESC`, stationID)
	defer rows.Close()
	commandes := []WashOrder{}
	for rows.Next() {
		var cmd WashOrder
		if err := rows.Scan(&cmd.ID, &cmd.StationID, &cmd.VehicleID, &cmd.PackageID, &cmd.CustomerName, &cmd.Status, &cmd.DueXOF, &cmd.CreatedAt); err == nil {
			commandes = append(commandes, cmd)
		}
	}
	ecrireJSON(w, http.StatusOK, map[string][]WashOrder{"wash_orders": commandes})
}

func (a *App) lireWashOrder(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	commandeID := cheminParam(r, "/api/v1/wash-orders/")
	var cmd WashOrder
	a.db.QueryRow(r.Context(), `SELECT * FROM wash_orders WHERE id = $1`, commandeID).Scan(&cmd.ID, &cmd.StationID, &cmd.VehicleID, &cmd.PackageID, &cmd.AssignedToID, &cmd.CustomerName, &cmd.CustomerPhone, &cmd.Notes, &cmd.UnitPriceXOF, &cmd.LineTotalXOF, &cmd.DueXOF, &cmd.PriceSource, &cmd.PriceSourceLabel, &cmd.Options, &cmd.Status, &cmd.ReceiptNumber, &cmd.CreatedAt, &cmd.UpdatedAt, &cmd.CreatedBy)
	// Charger les paiements
	paiements := []Payment{}
	rows, _ := a.db.Query(r.Context(), `SELECT * FROM payments WHERE wash_order_id = $1`, commandeID)
	defer rows.Close()
	for rows.Next() {
		var p Payment
		rows.Scan(&p.ID, &p.WashOrderID, &p.AmountXOF, &p.Method, &p.IsVoid, &p.VoidReason, &p.CashSessionID, &p.CreatedAt, &p.CreatedBy)
		paiements = append(paiements, p)
	}
	ecrireJSON(w, http.StatusOK, map[string]interface{}{"order": cmd, "payments": paiements})
}

func (a *App) mettreAJourWashOrder(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	commandeID := cheminParam(r, "/api/v1/wash-orders/")
	var corps map[string]interface{}
	json.NewDecoder(r.Body).Decode(&corps)
	var ancienStatut string
	a.db.QueryRow(r.Context(), `SELECT status FROM wash_orders WHERE id = $1`, commandeID).Scan(&ancienStatut)
	if nouveau, ok := corps["status"].(string); ok && !transitionAutorisee(ancienStatut, nouveau) {
		repondreErreur(w, http.StatusConflict, "transition_forbidden", "Changement de statut invalide.")
		return
	}
	// Mise à jour dynamique
	modifs := []string{"updated_at = NOW()", "updated_by = $1"}
	vals := []interface{}{sess.UserID}
	idx := 2
	for k, v := range corps {
		modifs = append(modifs, fmt.Sprintf("%s = $%d", k, idx))
		vals = append(vals, v)
		idx++
	}
	requete := fmt.Sprintf("UPDATE wash_orders SET %s WHERE id = $%d", strings.Join(modifs, ", "), idx)
	vals = append(vals, commandeID)
	a.db.Exec(r.Context(), requete, vals...)
	ecrireJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// Enregistrement des routes
func (a *App) enregistrerRoutesLavages(rt *routeur) {
	rt.proteger("GET", "/api/v1/wash-orders", "wash:view", a.listerWashOrders)
	rt.proteger("GET", "/api/v1/wash-orders/", "wash:view", a.lireWashOrder)

	rt.proteger("POST", "/api/v1/wash-orders", "wash:create", a.creerWashOrder)
	rt.proteger("PATCH", "/api/v1/wash-orders/", "wash:manage", a.mettreAJourWashOrder)
	rt.proteger("POST", "/api/v1/wash-orders/", "wash:manage", a.affecterWashOrder)    // /wash-orders/{id}/assign
	rt.proteger("POST", "/api/v1/wash-orders/", "wash:manage", a.annulerWashOrder)       // /wash-orders/{id}/cancel
	rt.proteger("POST", "/api/v1/wash-orders/", "wash:create", a.ajouterPaiementWashOrder) // /wash-orders/{id}/payments
}