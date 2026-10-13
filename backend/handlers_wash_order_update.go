package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Fiches de lavage : mise à jour et affectation.
// ---------------------------------------------------------------------------

// getOwnedOrder charge une fiche et vérifie qu'elle appartient bien à l'appelant.
// Renvoie (nil, nil) si la fiche n'existe pas ou appartient à une autre organisation.
func (a *App) getOwnedOrder(ctx context.Context, id, orgID string) (*WashOrder, error) {
	row := a.Pool.QueryRow(ctx, `SELECT `+washOrderColumns+` FROM wash_orders WHERE id = $1`, id)
	o, err := scanWashOrder(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if orgID != "" && o.OrganizationID != orgID {
		return nil, nil
	}
	return o, nil
}

// PATCH /wash-orders/:id — mise à jour des champs de la fiche.
func (a *App) handleWashOrderUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")
	order, err := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	if err != nil || order == nil {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}

	var body washOrderDraft
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	setClauses := []string{}
	args := []any{}
	add := func(column string, value any) {
		args = append(args, value)
		setClauses = append(setClauses, column+fmt.Sprintf(" = $%d", len(args)))
	}
	if body.CustomerName != "" {
		add("customer_name", body.CustomerName)
	}
	if body.Plate != "" {
		add("plate", body.Plate)
	}
	if body.DiscountXOF > 0 {
		newTotal := order.SubtotalXOF - body.DiscountXOF
		if newTotal < 0 {
			newTotal = 0
		}
		add("discount_xof", body.DiscountXOF)
		add("discount_reason", body.DiscountReason)
		add("total_xof", newTotal)
	}
	if body.Notes != "" {
		add("notes", body.Notes)
	}
	if body.Status != "" {
		add("status", body.Status)
	}
	if body.EmployeeID != "" {
		var name string
		_ = a.Pool.QueryRow(ctx, `SELECT full_name FROM users WHERE id = $1`, body.EmployeeID).Scan(&name)
		if name == "" {
			name = "Non attribué"
		}
		add("employee_id", body.EmployeeID)
		add("employee_name", name)
	}

	if len(setClauses) > 0 {
		args = append(args, id)
		query := `UPDATE wash_orders SET ` + strings.Join(setClauses, ", ") +
			fmt.Sprintf(" WHERE id = $%d", len(args))
		if _, err := a.Pool.Exec(ctx, query, args...); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la mise à jour.", nil)
			return
		}
	}

	a.addAudit(ctx, order.OrganizationID, order.StationID, user.FullName,
		"wash_order.updated", "wash_order", id, nil)

	updated, err := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	updated.Items = a.loadItems(r, id)
	writeJSON(w, http.StatusOK, updated)
}

// POST /wash-orders/:id/assign — affectation d'un laveur (réservée au staff).
func (a *App) handleWashOrderAssign(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user.Role == "washer" {
		writeError(w, http.StatusForbidden, "forbidden", "Action non autorisée pour ce profil.", nil)
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	order, err := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	if err != nil || order == nil {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}

	var body struct {
		EmployeeID string `json:"employee_id"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	employeeName := "Non attribué"
	var employeePtr *string
	if body.EmployeeID != "" {
		var name, role string
		err := a.Pool.QueryRow(ctx,
			`SELECT full_name, role FROM users WHERE id = $1`, body.EmployeeID).Scan(&name, &role)
		if err != nil || role != "washer" {
			writeError(w, http.StatusBadRequest, "validation_error", "Laveur introuvable.",
				map[string]string{"employee_id": "Laveur introuvable"})
			return
		}
		employeeName = name
		employeePtr = &body.EmployeeID
	}

	if _, err := a.Pool.Exec(ctx,
		`UPDATE wash_orders SET employee_id = $1, employee_name = $2 WHERE id = $3`,
		employeePtr, employeeName, id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'affectation.", nil)
		return
	}

	a.addAudit(ctx, order.OrganizationID, order.StationID, user.FullName,
		"wash_order.assigned", "wash_order", id, map[string]any{
			"employee_id":      body.EmployeeID,
			"employee_name":    employeeName,
			"assigned_by_role": user.Role,
		})

	updated, _ := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	updated.Items = a.loadItems(r, id)
	writeJSON(w, http.StatusOK, updated)
}
