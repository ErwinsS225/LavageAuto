package main

import (
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
)

// CashSession représente une session de caisse
type CashSession struct {
	ID                string            `json:"id"`
	StationID         string            `json:"station_id"`
	OpeningBalanceXOF int               `json:"opening_balance_xof"`
	ClosingBalanceXOF *int              `json:"closing_balance_xof,omitempty"`
	IsOpen            bool              `json:"is_open"`
	OpenedAt          pgtype.Timestamp  `json:"opened_at"`
	ClosedAt          *pgtype.Timestamp `json:"closed_at,omitempty"`
	OpenedBy          string            `json:"opened_by"`
	ClosedBy          *string           `json:"closed_by,omitempty"`
	ClosingNotes      *string           `json:"closing_notes,omitempty"`
}

// Requêtes caisse
type OuvrirSessionRequest struct {
	StationID         string `json:"station_id" validate:"required,uuid"`
	OpeningBalanceXOF int    `json:"opening_balance_xof" validate:"required,min=0"`
}

type FermerSessionRequest struct {
	ClosingBalanceXOF int     `json:"closing_balance_xof" validate:"required,min=0"`
	Notes             *string `json:"notes,omitempty"`
}

type AnnulerPaiementRequest struct {
	Reason string `json:"reason" validate:"required,min=5"`
}

// Handlers caisse
func (a *App) ouvrirSessionCaisse(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !a.stationActive(r.Context(), *sess.OrganizationID) {
		repondreErreur(w, http.StatusConflict, "onboarding_incomplete", "Station non activée.")
		return
	}
	corps, ok := decoderCorps[OuvrirSessionRequest](w, r)
	if !ok {
		return
	}
	// Vérifier qu'aucune session n'est ouverte
	var dejaOuverte bool
	if err := a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM cash_sessions WHERE station_id = $1 AND is_open = true)`, corps.StationID).Scan(&dejaOuverte); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de vérifier l'état de la caisse.")
		return
	}
	if dejaOuverte {
		repondreErreur(w, http.StatusConflict, "session_already_open", "Une session est déjà ouverte.")
		return
	}
	// Créer la session
	var nouvelID string
	if err := a.db.QueryRow(r.Context(), `INSERT INTO cash_sessions (station_id, opening_balance_xof, is_open, opened_by) VALUES ($1,$2,true,$3) RETURNING id`, corps.StationID, corps.OpeningBalanceXOF, sess.UserID).Scan(&nouvelID); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "create_failed", err.Error())
		return
	}
	ecrireJSON(w, http.StatusCreated, map[string]string{"id": nouvelID})
}

func (a *App) fermerSessionCaisse(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	sessionID := cheminParam(r, "/api/v1/cash/sessions/")
	corps, ok := decoderCorps[FermerSessionRequest](w, r)
	if !ok {
		return
	}
	// Marquer comme fermée
	_, err := a.db.Exec(r.Context(), `UPDATE cash_sessions SET is_open = false, closing_balance_xof = $1, closed_by = $2, closed_at = NOW(), closing_notes = $3 WHERE id = $4`, corps.ClosingBalanceXOF, sess.UserID, corps.Notes, sessionID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "close_failed", err.Error())
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (a *App) rouvrirSessionCaisse(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	sessionID := cheminParam(r, "/api/v1/cash/sessions/")
	_, err := a.db.Exec(r.Context(), `UPDATE cash_sessions SET is_open = true, closed_at = null, closed_by = null, closing_balance_xof = null WHERE id = $1`, sessionID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "reopen_failed", err.Error())
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (a *App) annulerPaiement(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	paiementID := cheminParam(r, "/api/v1/cash/payments/")
	corps, ok := decoderCorps[AnnulerPaiementRequest](w, r)
	if !ok {
		return
	}
	// Marquer le paiement comme void (jamais supprimé)
	_, err := a.db.Exec(r.Context(), `UPDATE payments SET status = 'voided', void_reason = $1, voided_by = $2, voided_at = NOW() WHERE id = $3`, corps.Reason, sess.UserID, paiementID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "void_failed", err.Error())
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (a *App) sessionActuelleCaisse(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	stationID := r.URL.Query().Get("station_id")
	var session CashSession
	err := a.db.QueryRow(r.Context(), `SELECT id, station_id, opening_balance_xof, closing_balance_xof, is_open, opened_at, closed_at, opened_by, closed_by, closing_notes FROM cash_sessions WHERE station_id = $1 AND is_open = true`, stationID).Scan(&session.ID, &session.StationID, &session.OpeningBalanceXOF, &session.ClosingBalanceXOF, &session.IsOpen, &session.OpenedAt, &session.ClosedAt, &session.OpenedBy, &session.ClosedBy, &session.ClosingNotes)
	if err != nil {
		repondreErreur(w, http.StatusNotFound, "no_active_session", "Aucune session ouverte.")
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]CashSession{"session": session})
}

func (a *App) listerSessionsCaisse(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	stationID := r.URL.Query().Get("station_id")
	rows, err := a.db.Query(r.Context(), `SELECT id, station_id, opening_balance_xof, closing_balance_xof, is_open, opened_at, closed_at, opened_by, closed_by, closing_notes FROM cash_sessions WHERE station_id = $1 ORDER BY opened_at DESC`, stationID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de charger les sessions de caisse.")
		return
	}
	defer rows.Close()
	sessions := []CashSession{}
	for rows.Next() {
		var s CashSession
		if err := rows.Scan(&s.ID, &s.StationID, &s.OpeningBalanceXOF, &s.ClosingBalanceXOF, &s.IsOpen, &s.OpenedAt, &s.ClosedAt, &s.OpenedBy, &s.ClosedBy, &s.ClosingNotes); err == nil {
			sessions = append(sessions, s)
		}
	}
	ecrireJSON(w, http.StatusOK, map[string][]CashSession{"sessions": sessions})
}

func (a *App) listerPaiementsCaisse(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	stationID := r.URL.Query().Get("station_id")
	rows, err := a.db.Query(r.Context(), `SELECT id, order_id, amount_xof, method,
   CASE WHEN status = 'voided' THEN true ELSE false END,
   void_reason, cash_session_id, created_at, created_by
   FROM payments WHERE order_id IN (SELECT id FROM wash_orders WHERE station_id = $1)
   ORDER BY created_at DESC LIMIT 100`, stationID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de charger les paiements.")
		return
	}
	defer rows.Close()
	paiements := []Payment{}
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.WashOrderID, &p.AmountXOF, &p.Method, &p.IsVoid, &p.VoidReason, &p.CashSessionID, &p.CreatedAt, &p.CreatedBy); err != nil {
			continue
		}
		paiements = append(paiements, p)
	}
	ecrireJSON(w, http.StatusOK, map[string][]Payment{"payments": paiements})
}

// Enregistrement des routes caisse
func (a *App) enregistrerRoutesCaisse(rt *routeur) {
	rt.proteger("GET", "/api/v1/cash/current", "cash:view", a.sessionActuelleCaisse)
	rt.proteger("GET", "/api/v1/cash/sessions", "cash:view", a.listerSessionsCaisse)
	rt.proteger("POST", "/api/v1/cash/sessions", "cash:manage", a.ouvrirSessionCaisse)
	rt.proteger("POST", "/api/v1/cash/sessions/", "cash:manage", a.fermerSessionCaisse)  // /sessions/{id}/close
	rt.proteger("POST", "/api/v1/cash/sessions/", "cash:manage", a.rouvrirSessionCaisse) // /sessions/{id}/reopen
	rt.proteger("GET", "/api/v1/cash/payments", "cash:view", a.listerPaiementsCaisse)
	rt.proteger("POST", "/api/v1/cash/payments/", "cash:manage", a.annulerPaiement) // /payments/{id}/void
}

// Fonction utilitaire mustAtoi (conversion string->int sécurisée)
func mustAtoi(s string) int {
	var res int
	fmt.Sscanf(s, "%d", &res)
	return res
}
