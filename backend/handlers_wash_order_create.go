package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Fiches de lavage (création).
// ---------------------------------------------------------------------------

// orderItemInput prestation saisie par le poste de caisse.
type orderItemInput struct {
	ServiceID    string `json:"service_id"`
	Quantity     int    `json:"quantity"`
	UnitPriceXOF int    `json:"unit_price_xof"`
}

// washOrderDraft corps de la requête de création de fiche.
type washOrderDraft struct {
	StationID         string           `json:"station_id"`
	CustomerID        string           `json:"customer_id"`
	CustomerName      string           `json:"customer_name"`
	VehicleCategoryID string           `json:"vehicle_category_id"`
	Plate             string           `json:"plate"`
	EmployeeID        string           `json:"employee_id"`
	DiscountXOF       int              `json:"discount_xof"`
	DiscountReason    string           `json:"discount_reason"`
	Notes             string           `json:"notes"`
	Status            string           `json:"status"`
	Items             []orderItemInput `json:"items"`
}

// pricedItem prestation après résolution du prix dans le catalogue.
type pricedItem struct {
	ServiceID   string
	ServiceName string
	UnitPrice   int
	Quantity    int
	LineTotal   int
}

func (a *App) handleWashOrderCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body washOrderDraft
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	ctx := r.Context()
	stationID := body.StationID
	if stationID == "" {
		stationID = deref(user.StationID)
	}

	// Sous-total à partir du catalogue (prix réel côté base).
	subtotal := 0
	items := []pricedItem{}
	for _, it := range body.Items {
		qty := it.Quantity
		if qty <= 0 {
			qty = 1
		}
		name := "Prestation"
		price := it.UnitPriceXOF
		if it.ServiceID != "" {
			var dbPrice int
			err := a.Pool.QueryRow(ctx,
				`SELECT name, price_xof FROM services WHERE id = $1`, it.ServiceID,
			).Scan(&name, &dbPrice)
			if err == nil && price == 0 {
				price = dbPrice
			}
		}
		if price == 0 {
			price = 2000
		}
		line := price * qty
		subtotal += line
		items = append(items, pricedItem{ServiceID: it.ServiceID, ServiceName: name, UnitPrice: price, Quantity: qty, LineTotal: line})
	}

	// Plafond de remise du caissier (réglage de la station).
	discount := body.DiscountXOF
	if user.Role == "cashier" && discount > 0 {
		var settingsRaw []byte
		_ = a.Pool.QueryRow(ctx, `SELECT settings FROM stations WHERE id = $1`, stationID).Scan(&settingsRaw)
		maxDiscount := 1000
		settings := map[string]any{}
		parseJSONB(settingsRaw, &settings)
		if v, ok := settings["max_cashier_discount_xof"].(float64); ok {
			maxDiscount = int(v)
		}
		if discount > maxDiscount {
			writeError(w, http.StatusBadRequest, "validation_error",
				fmt.Sprintf("La remise demandée (%d FCFA) dépasse le plafond autorisé pour le caissier (%d FCFA). Une remise exceptionnelle nécessite l'accord du gérant.", discount, maxDiscount),
				map[string]string{"discount_xof": "Plafond de remise caissier dépassé"})
			return
		}
	}

	total := subtotal - discount
	if total < 0 {
		total = 0
	}

	// --- Persistance -------------------------------------------------------

	// Référence : L-AAAAMMJJ-NNNN (compteur du jour).
	today := businessDay(time.Now())
	var seq int
	_ = a.Pool.QueryRow(ctx, `
		SELECT count(*) FROM wash_orders WHERE station_id = $1 AND business_day = $2`,
		stationID, today).Scan(&seq)
	reference := fmt.Sprintf("L-%s-%04d", strings.ReplaceAll(today, "-", ""), seq+1)

	// Véhicule + employé.
	vehicleName := "Voiture"
	if body.VehicleCategoryID != "" {
		_ = a.Pool.QueryRow(ctx, `SELECT name FROM vehicle_categories WHERE id = $1`,
			body.VehicleCategoryID).Scan(&vehicleName)
	}
	employeeName := "Non attribué"
	employeeID := body.EmployeeID
	if user.Role == "washer" && employeeID == "" {
		employeeID = user.ID
	}
	if employeeID != "" {
		_ = a.Pool.QueryRow(ctx, `SELECT full_name FROM users WHERE id = $1`, employeeID).Scan(&employeeName)
	}
	customerName := body.CustomerName
	if customerName == "" {
		customerName = "Client de passage"
	}
	status := body.Status
	if status == "" {
		status = "in_progress"
	}

	id := newID("ord")
	var customerPtr *string
	if body.CustomerID != "" {
		customerPtr = &body.CustomerID
	}
	var vehiclePtr *string
	if body.VehicleCategoryID != "" {
		vehiclePtr = &body.VehicleCategoryID
	}
	var employeePtr *string
	if employeeID != "" {
		employeePtr = &employeeID
	}

	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO wash_orders (`+washOrderColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,NULL,NULL,NULL,NULL,$18,$19,now(),now(),NULL,NULL,$20)`,
		id, deref(user.OrganizationID), stationID, reference,
		customerPtr, customerName, vehiclePtr, vehicleName,
		body.Plate, employeePtr, employeeName, status,
		subtotal, discount, body.DiscountReason, total, body.Notes,
		user.ID, user.FullName, today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création de la fiche.", nil)
		return
	}

	for i, it := range items {
		itemID := fmt.Sprintf("%s-i%d", id, i)
		_, _ = tx.Exec(ctx, `
			INSERT INTO wash_order_items (id, wash_order_id, service_id, service_name, unit_price_xof, quantity, line_total_xof)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			itemID, id, it.ServiceID, it.ServiceName, it.UnitPrice, it.Quantity, it.LineTotal)
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	a.addAudit(ctx, deref(user.OrganizationID), stationID, user.FullName,
		"wash_order.created", "wash_order", id,
		map[string]any{"total_xof": total, "discount_xof": discount})
	if discount > 0 {
		a.addAudit(ctx, deref(user.OrganizationID), stationID, user.FullName,
			"discount.granted", "wash_order", id,
			map[string]any{"amount_xof": discount,
				"reason":          orDefault(body.DiscountReason, "Remise accordée"),
				"granted_by_role": user.Role})
	}

	row := a.Pool.QueryRow(ctx, `SELECT `+washOrderColumns+` FROM wash_orders WHERE id = $1`, id)
	o, err := scanWashOrder(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	o.Items = a.loadItems(r, id)
	writeJSON(w, http.StatusCreated, o)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

