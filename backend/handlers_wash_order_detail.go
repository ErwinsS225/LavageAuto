package main

import (
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Fiches de lavage (détail).
//
// La fiche est lue avec son entreprise : une fiche d'une autre entreprise est
// introuvable. Les montants encaissés viennent des paiements valides.
// ---------------------------------------------------------------------------

func (a *App) handleWashOrderDetail(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")

	row := a.Pool.QueryRow(r.Context(), `
		SELECT `+washOrderColumns+` FROM wash_orders
		WHERE id = $1 AND organization_id = $2`, id, deref(user.OrganizationID))
	o, err := scanWashOrder(row)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Fiche de lavage introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != o.StationID {
		writeError(w, http.StatusNotFound, "not_found", "Fiche de lavage introuvable.", nil)
		return
	}

	// Paiements rattachés. Le moyen **déclaré** par la station est remonté avec
	// l'encaissement : la fiche montre « Orange Money », pas seulement la
	// famille comptable.
	prows, _ := a.Pool.Query(r.Context(), `
		SELECT id, method, amount_xof, reference, note, paid_at, received_by_name, status, void_reason,
		       payment_method_id, payment_method_name
		FROM payments WHERE wash_order_id = $1 AND organization_id = $2
		ORDER BY paid_at`, id, deref(user.OrganizationID))
	payments := []map[string]any{}
	methods := []string{}
	paid := 0
	if prows != nil {
		defer prows.Close()
		for prows.Next() {
			var pID, method, reference, note, receivedBy, status string
			var amount int
			var paidAt time.Time
			var voidReason, methodID, methodName *string
			if err := prows.Scan(&pID, &method, &amount, &reference, &note, &paidAt, &receivedBy,
				&status, &voidReason, &methodID, &methodName); err != nil {
				continue
			}
			payments = append(payments, map[string]any{
				"id": pID, "method": method, "amount_xof": amount,
				"reference": reference, "note": note,
				"paid_at":          paidAt.Format(timeLayout),
				"received_by_name": receivedBy, "status": status, "void_reason": voidReason,
				"payment_method_id": methodID, "payment_method_name": deref(methodName),
			})
			// Un paiement différé figure au détail (c'est un engagement du
			// client) mais ne compte pas comme somme reçue : le reste dû
			// affiché doit rester vrai.
			if status == "valid" && method != "differe" {
				methods = append(methods, method)
				paid += amount
			}
		}
	}

	items := a.loadItems(r, o.ID, o.OrganizationID)
	labels := []string{}
	for _, it := range items {
		labels = append(labels, it.ServiceName)
	}
	servicesLabel := "Lavage"
	if len(labels) > 0 {
		servicesLabel = joinComma(labels)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"order":    orderToJSON(o, paid, items, servicesLabel, methods),
		"payments": payments,
	})
}

// formatTimePtr formate un timestamp nullable en RFC3339 ou null.
func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(timeLayout)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// joinComma relie des chaînes par une virgule.
func joinComma(parts []string) string {
	return strings.Join(parts, ", ")
}
