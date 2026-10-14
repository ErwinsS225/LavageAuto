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
//
// La fiche est chargée avec son entreprise (jamais par identifiant seul), la
// remise repasse par le plafond du rôle à chaque modification, et le statut
// « annulée » exige la permission dédiée : modifier une fiche n'est pas
// annuler un lavage.
// ---------------------------------------------------------------------------

// getOwnedOrder charge une fiche et vérifie qu'elle appartient bien à
// l'entreprise. Renvoie (nil, nil) si elle n'existe pas ou appartient à une
// autre organisation.
func (a *App) getOwnedOrder(ctx context.Context, id, orgID string) (*WashOrder, error) {
	row := a.Pool.QueryRow(ctx, `
		SELECT `+washOrderColumns+` FROM wash_orders
		WHERE id = $1 AND organization_id = $2`, id, orgID)
	o, err := scanWashOrder(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
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
	if forced := scopedStationID(user); forced != "" && forced != order.StationID {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}
	if order.Status == "canceled" {
		writeError(w, http.StatusConflict, "conflict", "Cette fiche est annulée : elle n'est plus modifiable.", nil)
		return
	}

	// Une fiche déjà encaissée est figée sur le plan financier : ni remise
	// rétroactive, ni retour en atelier après coup. Restent modifiables les
	// champs purement descriptifs (client, plaque, note, laveur, catégorie).
	paidTotal := a.paidForOrder(r, order.ID, order.OrganizationID)
	encaissement := paidTotal > 0

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
	if body.Notes != "" {
		add("notes", body.Notes)
	}

	// Remise : re-plafonnée à chaque écriture (l'ancien code ne vérifiait le
	// plafond qu'à la création, un caissier pouvait donc le contourner ici).
	if body.DiscountXOF > 0 && body.DiscountXOF != order.DiscountXOF {
		if encaissement {
			writeError(w, http.StatusConflict, "conflict",
				"Cette fiche porte un encaissement : la remise ne peut plus être modifiée. Annulez d'abord le paiement.", nil)
			return
		}
		subtotal := order.SubtotalXOF
		if maxDiscount := a.maxDiscountFor(r, user, order.StationID, subtotal); body.DiscountXOF > maxDiscount {
			writeError(w, http.StatusBadRequest, "validation_error",
				fmt.Sprintf("Remise refusée : plafond de votre profil (%d FCFA).", maxDiscount),
				map[string]string{"discount_xof": "Plafond de remise dépassé"})
			return
		}
		if strings.TrimSpace(body.DiscountReason) == "" {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Une remise doit être justifiée.", map[string]string{"discount_reason": "Motif requis"})
			return
		}
		newTotal := subtotal - body.DiscountXOF
		if newTotal < 0 {
			newTotal = 0
		}
		add("discount_xof", body.DiscountXOF)
		add("discount_reason", body.DiscountReason)
		add("total_xof", newTotal)
		a.addAudit(ctx, order.OrganizationID, order.StationID, user.FullName,
			"discount.granted", "wash_order", id,
			map[string]any{"amount_xof": body.DiscountXOF, "reason": body.DiscountReason, "granted_by_role": user.Role})
	}

	// Statut : seuls des transitions explicites sont acceptées.
	if body.Status != "" && body.Status != order.Status {
		if encaissement && body.Status != "completed" {
			writeError(w, http.StatusConflict, "conflict",
				"Cette fiche porte un encaissement : son statut ne peut plus revenir en arrière.", nil)
			return
		}
		switch body.Status {
		case "pending", "in_progress":
			// retour en atelier : autorisé
		case "completed":
			_, _, due, err := a.soldeFiche(r, id, deref(user.OrganizationID))
			if err != nil {
				writeAPIError(w, err)
				return
			}
			if due > 0 {
				writeError(w, http.StatusConflict, "conflict",
					fmt.Sprintf("Il reste %d FCFA à encaisser avant de clore cette fiche.", due), nil)
				return
			}
			add("completed_at", "now()")
		case "canceled":
			if !hasPermission(user, "wash:cancel") {
				writeError(w, http.StatusForbidden, "forbidden",
					"L'annulation d'un lavage nécessite l'accord du gérant.", nil)
				return
			}
			if strings.TrimSpace(body.DiscountReason) == "" && strings.TrimSpace(deref(order.CancelReason)) == "" {
				writeError(w, http.StatusBadRequest, "validation_error",
					"Une annulation doit être motivée.", map[string]string{"reason": "Motif requis"})
				return
			}
		default:
			writeError(w, http.StatusBadRequest, "validation_error", "Statut inconnu.",
				map[string]string{"status": "Statut invalide"})
			return
		}
		if body.Status == "completed" {
			setClauses = append(setClauses, "status = 'completed'", "completed_at = COALESCE(completed_at, now())")
		} else {
			add("status", body.Status)
		}
	}

	// Changement de laveur : permission dédiée, compte vérifié dans l'entreprise.
	if body.EmployeeID != "" && body.EmployeeID != deref(order.EmployeeID) {
		if !hasPermission(user, "wash:assign") {
			writeError(w, http.StatusForbidden, "forbidden",
				"L'affectation d'un laveur nécessite les droits du gérant.", nil)
			return
		}
		var name, role string
		if err := a.Pool.QueryRow(ctx, `
			SELECT full_name, role FROM users
			WHERE id = $1 AND organization_id = $2 AND is_active
			  AND ($3 = '' OR station_id = $3)`,
			body.EmployeeID, deref(user.OrganizationID), order.StationID).Scan(&name, &role); err != nil {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Laveur introuvable.", map[string]string{"employee_id": "Laveur introuvable"})
			return
		}
		add("employee_id", body.EmployeeID)
		add("employee_name", name)
	}
	// Catégorie : le référentiel de la station, comme à la création. Le nom est
	// recopié — la fiche reste explicable même après un renommage.
	if body.VehicleCategoryID != "" && body.VehicleCategoryID != deref(order.VehicleCategoryID) {
		name, err := a.nomCategorieStation(ctx, order.StationID, body.VehicleCategoryID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Catégorie de véhicule inconnue ou non activée pour cette station.", nil)
			return
		}
		add("vehicle_category_id", body.VehicleCategoryID)
		add("vehicle_category_name", name)
	}

	// Usage commercial : second référentiel. Modifier le couple (catégorie,
	// usage) d'une fiche doit rester possible tant qu'elle n'est pas annulée,
	// mais la combinaison doit être déclarée par la station.
	if body.VehicleUsageID != "" && body.VehicleUsageID != deref(order.VehicleUsageID) {
		name, err := a.nomUsageStation(ctx, order.StationID, body.VehicleUsageID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Usage commercial inconnu ou non activé pour cette station.", nil)
			return
		}
		categorie := body.VehicleCategoryID
		if categorie == "" {
			categorie = deref(order.VehicleCategoryID)
		}
		if categorie != "" {
			actifs, err := a.combinaisonsActives(ctx, order.StationID)
			if err == nil && len(actifs) > 0 && !actifs[body.VehicleUsageID+":"+categorie] {
				writeError(w, http.StatusBadRequest, "validation_error",
					"Cette combinaison véhicule × usage n'est pas proposée par la station.", nil)
				return
			}
		}
		add("vehicle_usage_id", body.VehicleUsageID)
		add("vehicle_usage_name", name)
	}

	// Changement de véhicule : le tarif suit. Une fiche encaissée dont le
	// total bougerait est refusée plutôt que corrigée en silence.
	categorieApres := deref(order.VehicleCategoryID)
	if body.VehicleCategoryID != "" {
		categorieApres = body.VehicleCategoryID
	}
	usageApres := deref(order.VehicleUsageID)
	if body.VehicleUsageID != "" {
		usageApres = body.VehicleUsageID
	}
	if categorieApres != deref(order.VehicleCategoryID) || usageApres != deref(order.VehicleUsageID) {
		lignes := a.loadItems(r, id, order.OrganizationID)
		if len(lignes) > 0 {
			nouvelles, nouveauSubtotal, err := a.recalculerLignes(ctx, order, lignes, categorieApres, usageApres)
			if err != nil {
				if manque, ok := estPrixRequis(err); ok {
					writeError(w, http.StatusConflict, "price_required",
						"« "+manque.Service+" » n'a pas de tarif pour cette combinaison : "+
							"la fiche garde son prix actuel.",
						map[string]string{"items": "Prestation sans tarif"})
					return
				}
				writeAPIError(w, err)
				return
			}
			nouveauTotal := nouveauSubtotal - order.DiscountXOF
			if nouveauTotal < 0 {
				nouveauTotal = 0
			}
			if encaissement && nouveauTotal != order.TotalXOF {
				writeError(w, http.StatusConflict, "conflict",
					"Ce changement de véhicule modifie le montant d'une fiche déjà encaissée : "+
						"annulez le paiement pour la corriger.", nil)
				return
			}
			for _, ligne := range nouvelles {
				if _, err := a.Pool.Exec(ctx, `
					UPDATE wash_order_items
					SET unit_price_xof = $1, line_total_xof = $2, price_source = $3
					WHERE id = $4 AND organization_id = $5`,
					ligne.UnitPrice, ligne.LineTotal, ligne.PriceSource, ligne.ItemID, order.OrganizationID); err != nil {
					writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
					return
				}
			}
			add("subtotal_xof", nouveauSubtotal)
			add("total_xof", nouveauTotal)
		}
	}

	if len(setClauses) > 0 {
		args = append(args, id, deref(user.OrganizationID))
		query := `UPDATE wash_orders SET ` + strings.Join(setClauses, ", ") +
			fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d", len(args)-1, len(args))
		if _, err := a.Pool.Exec(ctx, query, args...); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la mise à jour.", nil)
			return
		}
	}

	a.addAudit(ctx, order.OrganizationID, order.StationID, user.FullName,
		"wash_order.updated", "wash_order", id, map[string]any{"actor_role": user.Role})

	updated, err := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	paid := a.paidForOrder(r, updated.ID, updated.OrganizationID)
	items := a.loadItems(r, id, deref(user.OrganizationID))
	labels := []string{}
	for _, it := range items {
		labels = append(labels, it.ServiceName)
	}
	label := "Lavage"
	if len(labels) > 0 {
		label = joinComma(labels)
	}
	writeJSON(w, http.StatusOK, orderToJSON(updated, paid, items, label, nil))
}

// paidForOrder : total des encaissements valides d'une fiche.
// paidForOrder : sommes réellement reçues sur une fiche. Un paiement différé
// est un engagement, il n'entre donc pas dans le total encaissé.
func (a *App) paidForOrder(r *http.Request, orderID, orgID string) int {
	var paid int
	_ = a.Pool.QueryRow(r.Context(), `
		SELECT COALESCE(sum(amount_xof), 0) FROM payments
		WHERE wash_order_id = $1 AND organization_id = $2 AND status = 'valid'
		  AND method <> 'differe'`,
		orderID, orgID).Scan(&paid)
	return paid
}

// POST /wash-orders/:id/assign — affectation d'un laveur.
func (a *App) handleWashOrderAssign(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")
	order, err := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	if err != nil || order == nil {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != order.StationID {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}
	if order.Status == "canceled" || order.Status == "completed" {
		writeError(w, http.StatusConflict, "conflict",
			"Une fiche annulée ou terminée ne peut plus être réaffectée.", nil)
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
		err := a.Pool.QueryRow(ctx, `
			SELECT full_name, role FROM users
			WHERE id = $1 AND organization_id = $2 AND is_active
			  AND ($3 = '' OR station_id = $3)`,
			body.EmployeeID, deref(user.OrganizationID), order.StationID).Scan(&name, &role)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", "Laveur introuvable.",
				map[string]string{"employee_id": "Laveur introuvable"})
			return
		}
		employeeName = name
		employeePtr = &body.EmployeeID
	}

	if _, err := a.Pool.Exec(ctx, `
		UPDATE wash_orders SET employee_id = $1, employee_name = $2
		WHERE id = $3 AND organization_id = $4`,
		employeePtr, employeeName, id, deref(user.OrganizationID)); err != nil {
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
	items := a.loadItems(r, id, deref(user.OrganizationID))
	paid := a.paidForOrder(r, id, order.OrganizationID)
	writeJSON(w, http.StatusOK, orderToJSON(updated, paid, items, "", nil))
}
