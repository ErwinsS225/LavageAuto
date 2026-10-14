package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Fiches de lavage (création).
//
// Le prix vient toujours du catalogue de l'entreprise : le poste de caisse ne
// peut pas fixer un tarif (l'ancien code honorait `unit_price_xof` envoyé par
// le navigateur). La remise est plafonnée par le réglage de la station.
// ---------------------------------------------------------------------------

// orderItemInput prestation saisie par le poste de caisse. `unit_price_xof`
// n'est accepté que comme information d'affichage : il est ignoré au calcul.
// En revanche `options` (identifiants et quantités) est une demande légitime :
// c'est la station qui décide du supplément, pas le navigateur.
type orderItemInput struct {
	ServiceID    string           `json:"service_id"`
	Quantity     int              `json:"quantity"`
	UnitPriceXOF int              `json:"unit_price_xof"`
	Options      []optionDemandee `json:"options"`
}

// washOrderDraft corps de la requête de création de fiche.
type washOrderDraft struct {
	StationID         string           `json:"station_id"`
	CustomerID        string           `json:"customer_id"`
	CustomerName      string           `json:"customer_name"`
	VehicleCategoryID string           `json:"vehicle_category_id"`
	VehicleUsageID    string           `json:"vehicle_usage_id"`
	Plate             string           `json:"plate"`
	EmployeeID        string           `json:"employee_id"`
	DiscountXOF       int              `json:"discount_xof"`
	DiscountReason    string           `json:"discount_reason"`
	Notes             string           `json:"notes"`
	Status            string           `json:"status"`
	Items             []orderItemInput `json:"items"`
}

// pricedItem prestation après résolution du prix : le montant retenu **et** la
// raison de ce montant, pour que la fiche reste explicable.
//
// `OptionsTotal` porte le supplément d'options de **toute la ligne** (par unité
// de prestation, multiplié par la quantité) ; `UnitPrice` reste le prix de la
// prestation seule.
type pricedItem struct {
	ServiceID    string
	ServiceName  string
	UnitPrice    int
	Quantity     int
	LineTotal    int
	PriceSource  string
	Options      []optionRetenue
	OptionsTotal int
}

// maxDiscountFor : plafond de remise de l'appelant. Le gérant et le
// propriétaire ne sont pas limités ; le plafond du caissier vient des réglages
// de la station (1000 FCFA par défaut) ; un laveur n'accorde pas de remise.
func (a *App) maxDiscountFor(r *http.Request, user *User, stationID string, subtotal int) int {
	switch user.Role {
	case "cashier":
		var settingsRaw []byte
		_ = a.Pool.QueryRow(r.Context(), `SELECT settings FROM stations WHERE id = $1`, stationID).Scan(&settingsRaw)
		settings := map[string]any{}
		parseJSONB(settingsRaw, &settings)
		if v, ok := settings["max_cashier_discount_xof"].(float64); ok && v >= 0 {
			return int(v)
		}
		return 1000
	case "washer":
		return 0
	default:
		return subtotal // gérant / propriétaire / superadmin : pas de plafond
	}
}

func (a *App) handleWashOrderCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body washOrderDraft
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	ctx := r.Context()
	stationID, err := a.resolveStation(ctx, user, body.StationID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if err := a.checkPlanLimit(r, user, limitWashOrdersMonth); err != nil {
		writeAPIError(w, err)
		return
	}
	if len(body.Items) == 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Ajoutez au moins une prestation à la fiche.",
			map[string]string{"items": "Aucune prestation"})
		return
	}

	// Client : vérifié dans l'entreprise (jamais celui d'un voisin).
	var customerPtr *string
	orgID := deref(user.OrganizationID)
	customerName := body.CustomerName
	if body.CustomerID != "" {
		var foundName string
		err := a.Pool.QueryRow(ctx, `
			SELECT full_name FROM customers
			WHERE id = $1 AND organization_id = $2 AND is_active`,
			body.CustomerID, deref(user.OrganizationID)).Scan(&foundName)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "Client introuvable.", nil)
			return
		}
		customerPtr = &body.CustomerID
		if customerName == "" {
			customerName = foundName
		}
	}
	if customerName == "" {
		customerName = "Client de passage"
	}

	// --- Le véhicule et son usage d'abord : le prix en dépend -------------

	// Catégorie de véhicule : le référentiel de la station, jamais un nom libre.
	// Le nom est recopié sur la fiche : une fiche ancienne doit rester lisible
	// même si la catégorie est renommée ou désactivée plus tard.
	vehicleName := ""
	var vehiclePtr *string
	if body.VehicleCategoryID != "" {
		nom, err := a.nomCategorieStation(ctx, stationID, body.VehicleCategoryID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Catégorie de véhicule inconnue ou non activée pour cette station.",
				map[string]string{"vehicle_category_id": "Catégorie inconnue"})
			return
		}
		vehicleName, vehiclePtr = nom, &body.VehicleCategoryID
	}

	// Usage commercial : second référentiel, indépendant du premier. Une
	// berline « taxi compteur » reste une berline — c'est ce qui permet de
	// compter les berlines **et** de savoir lesquelles étaient des taxis.
	usageName := ""
	var usagePtr *string
	if body.VehicleUsageID != "" {
		nom, err := a.nomUsageStation(ctx, stationID, body.VehicleUsageID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Usage commercial inconnu ou non activé pour cette station.",
				map[string]string{"vehicle_usage_id": "Usage inconnu"})
			return
		}
		// Le couple (catégorie, usage) doit être déclaré : sinon la caisse
		// enregistrerait une combinaison que la station n'a pas reconnue.
		if body.VehicleCategoryID != "" {
			actifs, err := a.combinaisonsActives(ctx, stationID)
			if err == nil && len(actifs) > 0 &&
				!actifs[body.VehicleUsageID+":"+body.VehicleCategoryID] {
				writeError(w, http.StatusBadRequest, "validation_error",
					"Cette combinaison véhicule × usage n'est pas proposée par la station.",
					map[string]string{"vehicle_usage_id": "Combinaison non déclarée"})
				return
			}
		}
		usageName, usagePtr = nom, &body.VehicleUsageID
	}

	// --- Prix : le serveur seul décide ------------------------------------
	//
	// Le poste de caisse envoie des choix (prestation, véhicule, usage, client,
	// options) ; la hiérarchie — contrat, prestation × véhicule × usage,
	// prestation × véhicule, tarif du catalogue — est appliquée ici et nulle
	// part ailleurs. Une prestation sans tarif utilisable est refusée : mieux
	// vaut une fiche non créée qu'une recette fausse.
	subtotal := 0
	items := []pricedItem{}
	moment := time.Now()
	for _, it := range body.Items {
		if it.ServiceID == "" {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Prestation inconnue.", map[string]string{"items": "service_id manquant"})
			return
		}
		service, err := scanService(a.Pool.QueryRow(ctx, `
			SELECT `+serviceColumnsSansAlias+`
			FROM services
			WHERE id = $1 AND organization_id = $2 AND station_id = $3 AND is_active`,
			it.ServiceID, deref(user.OrganizationID), stationID))
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Prestation introuvable dans votre catalogue.",
				map[string]string{"items": "Prestation inconnue", "service_id": it.ServiceID})
			return
		}
		// Une prestation sur devis ne se vend pas au comptoir : il faut d'abord
		// la chiffrer.
		if service.IsQuoteOnly {
			writeError(w, http.StatusConflict, "price_required",
				"« "+service.Name+" » se chiffre sur devis : établissez le montant avant de l'encaisser.",
				map[string]string{"items": "Prestation sur devis", "service_id": service.ID})
			return
		}
		prix, err := a.resolvePrice(ctx, stationID, service,
			body.VehicleCategoryID, body.VehicleUsageID, body.CustomerID, moment)
		if err != nil {
			if manque, ok := estPrixRequis(err); ok {
				writeError(w, http.StatusConflict, "price_required",
					"« "+manque.Service+" » n'a pas encore de tarif pour cette combinaison. "+
						"Renseignez-le dans la grille tarifaire avant de l'encaisser.",
					map[string]string{"items": "Prestation sans tarif", "service_id": service.ID})
				return
			}
			writeAPIError(w, err)
			return
		}
		options, optionsTotalUnitaires, err := a.chargerOptions(ctx, stationID, service.ID,
			body.VehicleCategoryID, it.Options)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		qty := it.Quantity
		if qty <= 0 {
			qty = 1
		}
		// Les options se facturent comme la prestation : par lavage.
		optionsTotal := optionsTotalUnitaires * qty
		line := prix.Montant*qty + optionsTotal
		subtotal += line
		items = append(items, pricedItem{
			ServiceID: service.ID, ServiceName: service.Name,
			UnitPrice: prix.Montant, Quantity: qty, LineTotal: line,
			PriceSource: prix.Source, Options: options, OptionsTotal: optionsTotal,
		})
	}

	discount := body.DiscountXOF
	if discount < 0 {
		discount = 0
	}
	if maxDiscount := a.maxDiscountFor(r, user, stationID, subtotal); discount > maxDiscount {
		message := fmt.Sprintf(
			"La remise demandée (%d FCFA) dépasse le plafond autorisé (%d FCFA). Passez par un gérant si la remise est justifiée.",
			discount, maxDiscount)
		if maxDiscount == 0 {
			message = "Une remise ne peut être accordée que par le gérant ou le propriétaire."
		}
		writeError(w, http.StatusBadRequest, "validation_error", message,
			map[string]string{"discount_xof": "Plafond de remise dépassé"})
		return
	}
	total := subtotal - discount
	if total < 0 {
		total = 0
	}

	// Employé : doit appartenir à l'entreprise ; un laveur s'affecte lui-même.
	employeeID := body.EmployeeID
	if user.Role == "washer" && employeeID == "" {
		employeeID = user.ID
	}
	employeeName := "Non attribué"
	var employeePtr *string
	if employeeID != "" {
		var name, role string
		err := a.Pool.QueryRow(ctx, `
			SELECT full_name, role FROM users
			WHERE id = $1 AND organization_id = $2 AND is_active`,
			employeeID, deref(user.OrganizationID)).Scan(&name, &role)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Employé introuvable.", map[string]string{"employee_id": "Employé introuvable"})
			return
		}
		employeeName = name
		employeePtr = &employeeID
	}

	status := body.Status
	if status == "" {
		status = "in_progress"
	}
	if status != "pending" && status != "in_progress" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Une nouvelle fiche démarre « en attente » ou « en cours ».",
			map[string]string{"status": "Statut invalide"})
		return
	}

	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer tx.Rollback(ctx)

	// Référence unique par entreprise : FICHE-2026-0001.
	reference, err := nextReference(ctx, tx, "wash_orders", "reference",
		fmt.Sprintf("FICHE-%d", time.Now().Year()), deref(user.OrganizationID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur de numérotation.", nil)
		return
	}

	today := businessDay(time.Now())
	id := newID("ord")
	if _, err := tx.Exec(ctx, `
		INSERT INTO wash_orders (`+washOrderColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,
		        NULL,NULL,NULL,NULL,$20,$21,now(),now(),NULL,NULL,$22)`,
		id, deref(user.OrganizationID), stationID, reference,
		customerPtr, customerName, vehiclePtr, vehicleName,
		usagePtr, usageName,
		body.Plate, employeePtr, employeeName, status,
		subtotal, discount, body.DiscountReason, total, body.Notes,
		user.ID, user.FullName, today); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création de la fiche.", nil)
		return
	}

	for i, it := range items {
		itemID := fmt.Sprintf("%s-i%d", id, i)
		// Le prix retenu est recopié avec sa provenance et le détail des
		// options : si la grille change demain, cette fiche reste explicable.
		instantane, _ := json.Marshal(it.Options)
		if _, err := tx.Exec(ctx, `
			INSERT INTO wash_order_items (id, wash_order_id, organization_id, service_id, service_name,
			  unit_price_xof, quantity, line_total_xof, price_source, options, options_total_xof)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11)`,
			itemID, id, orgID, it.ServiceID, it.ServiceName, it.UnitPrice, it.Quantity,
			it.LineTotal, it.PriceSource, string(instantane), it.OptionsTotal); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'enregistrement des prestations.", nil)
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	a.addAudit(ctx, deref(user.OrganizationID), stationID, user.FullName,
		"wash_order.created", "wash_order", id,
		map[string]any{"reference": reference, "total_xof": total, "discount_xof": discount, "actor_role": user.Role})
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
	// Réponse normalisée : le poste de caisse reçoit le reste à payer calculé,
	// comme sur les autres routes de fiche.
	o.Items = a.loadItems(r, id, orgID)
	writeJSON(w, http.StatusCreated, orderToJSON(o, 0, o.Items, "", nil))
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
