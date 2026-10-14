package main

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Reçus : le justificatif remis au client.
//
// Principes appliqués ici :
//   - un reçu n'est émis que lorsque la fiche est **soldée** (aucun reste à
//     payer). Un paiement différé est un engagement, pas un encaissement : il
//     ne solde rien ;
//   - le numéro est attribué **une seule fois** et ne change plus (index unique
//     par station) ;
//   - rien n'est inventé : ni numéro fiscal, ni raison sociale, ni mention
//     commerciale. Ce qui apparaît vient des paramètres saisis par une personne
//     habilitée, et si un champ est vide, la ligne n'est pas imprimée ;
//   - la pièce est un justificatif **numérique** (JSON/CSV imprimable). Le rendu
//     papier et le logo imprimé restent à faire (V2).
// ---------------------------------------------------------------------------

// recu : ce que l'écran de caisse et l'export reçoivent.
type recu struct {
	Number        string           `json:"number"`
	IssuedAt      string           `json:"issued_at"`
	StationName   string           `json:"station_name"`
	StationCity   string           `json:"station_city"`
	StationPhone  string           `json:"station_phone"`
	LegalName     string           `json:"legal_name"`
	TaxID         string           `json:"tax_id"`
	LogoURL       string           `json:"logo_url"`
	Footer        string           `json:"footer"`
	OrderRef      string           `json:"order_reference"`
	BusinessDay   string           `json:"business_day"`
	CustomerName  string           `json:"customer_name"`
	Plate         string           `json:"plate"`
	CategoryName  string           `json:"vehicle_category_name"`
	EmployeeName  string           `json:"employee_name"`
	Lines         []map[string]any `json:"lines"`
	SubtotalXOF   int              `json:"subtotal_xof"`
	DiscountXOF   int              `json:"discount_xof"`
	DiscountLabel string           `json:"discount_reason"`
	TotalXOF      int              `json:"total_xof"`
	PaidXOF       int              `json:"paid_xof"`
	DueXOF        int              `json:"due_xof"`
	Payments      []map[string]any `json:"payments"`
	// MentionsVides : la liste des mentions absentes — l'écran les annonce comme
	// « non renseignées » au lieu de laisser croire à un oubli d'impression.
	MentionsVides []string `json:"missing_mentions"`
}

// numeroRecuCandidat construit un numéro lisible : PRÉFIXE-AAAAMMJJ-NNNN.
func numeroRecuCandidat(prefixe, jour string, sequence int) string {
	return prefixe + "-" + strings.ReplaceAll(jour, "-", "") + "-" + pad4(sequence)
}

// definirNumeroRecu attribue un numéro à une fiche si elle n'en a pas encore.
//
// Le numéro est `PRÉFIXE-AAAAMMJJ-NNNN`, séquentiel par station et par jour.
// La séquence repart du **plus grand numéro déjà émis**, jamais d'un `count(*)`
// — sinon un reçu supprimé ferait réattribuer son numéro à une autre fiche
// (la leçon de `references.go`). L'index unique `(station_id, receipt_number)`
// reste le dernier rempart, et un verrou sur la station sérialise deux
// encaissements simultanés.
func (a *App) definirNumeroRecu(ctx context.Context, stationID, jour, prefixe, orderID string) (string, error) {
	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`SELECT 1 FROM stations WHERE id = $1 FOR UPDATE`, stationID); err != nil {
		return "", err
	}
	var existant string
	if err := tx.QueryRow(ctx,
		`SELECT receipt_number FROM wash_orders WHERE id = $1`, orderID).Scan(&existant); err != nil {
		return "", err
	}
	if existant != "" {
		// Numéro déjà attribué : un reçu garde le sien, même si on le réédite.
		return existant, nil
	}

	var maxNum *int
	if err := tx.QueryRow(ctx, `
		SELECT max(NULLIF(regexp_replace(receipt_number, '^.*-', ''), '')::int)
		FROM wash_orders
		WHERE station_id = $1 AND receipt_number LIKE $2`,
		stationID, prefixe+"-"+strings.ReplaceAll(jour, "-", "")+"-%").Scan(&maxNum); err != nil {
		return "", err
	}
	suivant := 1
	if maxNum != nil {
		suivant = *maxNum + 1
	}
	numero := numeroRecuCandidat(prefixe, jour, suivant)
	if _, err := tx.Exec(ctx, `
		UPDATE wash_orders SET receipt_number = $1, receipt_issued_at = now()
		WHERE id = $2 AND receipt_number = ''`, numero, orderID); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return numero, nil
}

// construireRecu assemble le justificatif d'une fiche : identité de la station,
// lignes de la fiche, encaissements réels, et mentions telles qu'elles ont été
// saisies.
func (a *App) construireRecu(ctx context.Context, orgID, orderID string) (*recu, error) {
	order, err := a.getOwnedOrder(ctx, orderID, orgID)
	if err != nil || order == nil {
		return nil, errNotFound
	}

	var (
		numero     string
		emisLe     *time.Time
		nomStation string
		ville      string
		telephone  string
		legalName  string
		taxID      string
		logoURL    string
		footer     string
	)
	err = a.Pool.QueryRow(ctx, `
		SELECT o.receipt_number, o.receipt_issued_at,
		       s.name, s.city, s.phone,
		       COALESCE(s.settings ->> 'receipt_legal_name', ''),
		       COALESCE(s.settings ->> 'receipt_tax_id', ''),
		       COALESCE(s.settings ->> 'receipt_logo_url', ''),
		       COALESCE(s.settings ->> 'receipt_footer', '')
		FROM wash_orders o
		JOIN stations s ON s.id = o.station_id
		WHERE o.id = $1 AND o.organization_id = $2`, orderID, orgID).
		Scan(&numero, &emisLe, &nomStation, &ville, &telephone, &legalName, &taxID, &logoURL, &footer)
	if err != nil {
		return nil, err
	}

	lignes := []map[string]any{}
	rows, err := a.Pool.Query(ctx, `
		SELECT service_name, vehicle_category_name, quantity, unit_price_xof, total_xof
		FROM wash_order_items WHERE wash_order_id = $1 ORDER BY created_at, service_name`, orderID)
	if err == nil {
		for rows.Next() {
			var nom, categorie string
			var quantite, unitaire, total int
			if err := rows.Scan(&nom, &categorie, &quantite, &unitaire, &total); err != nil {
				continue
			}
			lignes = append(lignes, map[string]any{
				"service_name": nom, "vehicle_category_name": categorie,
				"quantity": quantite, "unit_price_xof": unitaire, "total_xof": total,
			})
		}
		rows.Close()
	}

	reglements := []map[string]any{}
	// Seuls les encaissements réels figurent sur le reçu : un paiement différé
	// engage le client, il ne constate pas de somme reçue.
	var totalPaye int
	payRows, err := a.Pool.Query(ctx, `
		SELECT amount_xof, method, payment_method_name, reference, paid_at, received_by_name
		FROM payments
		WHERE wash_order_id = $1 AND status = 'valid' AND method <> 'differe'
		ORDER BY paid_at`, orderID)
	if err == nil {
		for payRows.Next() {
			var montant int
			var methode, moyen, reference, agent string
			var quand time.Time
			if err := payRows.Scan(&montant, &methode, &moyen, &reference, &quand, &agent); err != nil {
				continue
			}
			libelle := moyen
			if strings.TrimSpace(libelle) == "" {
				libelle = methodLabelFR(methode)
			}
			totalPaye += montant
			reglements = append(reglements, map[string]any{
				"amount_xof": montant, "method": methode, "method_label": libelle,
				"reference": reference, "paid_at": quand.Format(timeLayout), "received_by_name": agent,
			})
		}
		payRows.Close()
	}

	mentionsVides := []string{}
	for _, champ := range []struct {
		libelle string
		valeur  string
	}{
		{"raison sociale", legalName},
		{"identifiant fiscal", taxID},
		{"bas de reçu", footer},
		{"logo", logoURL},
		{"téléphone de la station", telephone},
	} {
		if strings.TrimSpace(champ.valeur) == "" {
			mentionsVides = append(mentionsVides, champ.libelle)
		}
	}

	recuEmis := ""
	if emisLe != nil {
		recuEmis = emisLe.Format(timeLayout)
	}
	return &recu{
		Number: numero, IssuedAt: recuEmis,
		StationName: nomStation, StationCity: ville, StationPhone: telephone,
		LegalName: legalName, TaxID: taxID, LogoURL: logoURL, Footer: footer,
		OrderRef: order.Reference, BusinessDay: order.BusinessDay,
		CustomerName: order.CustomerName, Plate: order.Plate,
		CategoryName: order.VehicleCategoryName,
		EmployeeName: order.EmployeeName,
		Lines:        lignes,
		SubtotalXOF:  order.SubtotalXOF, DiscountXOF: order.DiscountXOF, DiscountLabel: order.DiscountReason,
		TotalXOF: order.TotalXOF, PaidXOF: totalPaye,
		DueXOF:        maxInt(order.TotalXOF-totalPaye, 0),
		Payments:      reglements,
		MentionsVides: mentionsVides,
	}, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// GET /wash-orders/{id}/receipt — le justificatif s'il a été émis.
func (a *App) handleReceiptRead(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	// La portée de la station est vérifiée sur la fiche elle-même : un caissier
	// rattaché ne lit pas le reçu d'une autre station.
	order, err := a.getOwnedOrder(r.Context(), id, deref(user.OrganizationID))
	if err != nil || order == nil {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != order.StationID {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}
	recu, err := a.construireRecu(r.Context(), deref(user.OrganizationID), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}
	if recu.Number == "" {
		writeError(w, http.StatusNotFound, "receipt_not_issued",
			"Cette fiche n'a pas encore de reçu : elle n'est pas soldée.",
			map[string]string{"receipt": "Reçu non émis"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"receipt": recu})
}

// POST /wash-orders/{id}/receipt — émission (idempotente).
//
// L'émission automatique a lieu à l'encaissement du solde ; cette route sert à
// émettre un reçu sur une fiche soldée avant la mise à jour, ou à le relire
// après un échec d'affichage. Elle n'attribue jamais deux numéros.
func (a *App) handleReceiptIssue(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusConflict, "conflict", "Cette fiche est annulée.", nil)
		return
	}
	_, _, due, err := a.soldeFiche(r, id, deref(user.OrganizationID))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if due > 0 {
		writeError(w, http.StatusConflict, "order_not_settled",
			"Le reçu est émis quand la fiche est entièrement payée : il reste "+
				itoa(due)+" FCFA à encaisser.",
			map[string]string{"receipt": "Reste à payer"})
		return
	}

	prefixe := "REC"
	if parametres, err := a.lireParametresCaisse(ctx, order.StationID); err == nil {
		prefixe = parametres.ReceiptPrefix
	}
	numero, err := a.definirNumeroRecu(ctx, order.StationID, order.BusinessDay, prefixe, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur d'émission du reçu.", nil)
		return
	}
	a.addAudit(ctx, deref(user.OrganizationID), order.StationID, user.FullName,
		"receipt.issued", "wash_order", id, map[string]any{"receipt_number": numero})

	recu, err := a.construireRecu(ctx, deref(user.OrganizationID), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"receipt": recu})
}

// emettreRecuAuto : appelée dans la transaction d'encaissement quand la fiche
// vient d'être soldée. Un échec ne doit pas annuler l'encaissement : le reçu
// reste émettable par la route dédiée, et l'exploitation garde son argent
// enregistré.
func (a *App) emettreRecuAuto(ctx context.Context, orgID, stationID, orderID, jour, acteur string) {
	prefixe := "REC"
	if parametres, err := a.lireParametresCaisse(ctx, stationID); err == nil {
		prefixe = parametres.ReceiptPrefix
	}
	numero, err := a.definirNumeroRecu(ctx, stationID, jour, prefixe, orderID)
	if err != nil {
		return
	}
	a.addAudit(ctx, orgID, stationID, acteur, "receipt.issued", "wash_order", orderID,
		map[string]any{"receipt_number": numero, "mode": "encaissement du solde"})
}
