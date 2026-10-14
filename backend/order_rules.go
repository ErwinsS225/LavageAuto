package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Règles de fonctionnement des commandes, et cycle de vie d'une fiche.
//
// Deux moitiés, une seule idée : **une règle affichée est une règle appliquée**.
// Chaque réglage ci-dessous est lu à l'endroit exact où il décide — création
// d'une fiche, encaissement, changement de statut, annulation. Aucun n'est
// décoratif : le formateur du parcours ne propose pas d'interrupteur qui ne
// change rien au comportement du serveur.
//
// Les règles vivent dans `stations.settings` (clé `order_rules`), comme les
// paramètres de caisse : une station, une configuration.
// ---------------------------------------------------------------------------

// reglesCommandes : ce que l'exploitant décide de sa façon de travailler.
type reglesCommandes struct {
	// Accepte-t-on un véhicule sans fiche client (passage, dépannage) ? Quand
	// la règle est désactivée, une fiche sans client enregistré est refusée.
	AllowWalkIn bool `json:"allow_walk_in"`
	// Toute fiche doit-elle avoir un laveur affecté dès sa création ?
	RequireWasher bool `json:"require_washer"`
	// Le passage par « contrôle qualité » est-il obligatoire avant « prête » ?
	QualityCheckRequired bool `json:"quality_check_required"`
	// La remise du véhicule est-elle subordonnée au paiement complet ?
	ReleaseRequiresPayment bool `json:"release_requires_payment"`
	// Les acomptes (paiements partiels) sont-ils acceptés ?
	AllowPartialPayments bool `json:"allow_partial_payments"`
	// Une annulation doit-elle être motivée ?
	RequireCancelReason bool `json:"require_cancel_reason"`
	// Nombre de fiches en cours qu'un laveur peut porter (0 = pas de limite).
	MaxOpenOrdersPerWasher int `json:"max_open_orders_per_washer"`
}

// reglesParDefaut : **le comportement d'avant ce lot**, à la lettre. Une
// station existante ne change donc pas de fonctionnement le jour de la mise à
// jour : les règles ne font que rendre explicite ce que le serveur appliquait
// déjà, et laissent l'exploitant en changer.
func reglesParDefaut() reglesCommandes {
	return reglesCommandes{
		AllowWalkIn:            true,
		RequireWasher:          false,
		QualityCheckRequired:   false,
		ReleaseRequiresPayment: true,
		AllowPartialPayments:   true,
		RequireCancelReason:    true,
		MaxOpenOrdersPerWasher: 0,
	}
}

// libellesReglesCommandes : ce que l'écran affiche pour chaque réglage.
var libellesReglesCommandes = map[string]string{
	"allow_walk_in":              "Accepter les clients de passage (sans fiche client)",
	"require_washer":             "Exiger un laveur affecté dès la création de la fiche",
	"quality_check_required":     "Passer par le contrôle qualité avant de déclarer un véhicule prêt",
	"release_requires_payment":   "Ne remettre le véhicule qu'une fois la fiche soldée",
	"allow_partial_payments":     "Accepter les acomptes (paiements partiels)",
	"require_cancel_reason":      "Exiger un motif pour annuler une fiche",
	"max_open_orders_per_washer": "Nombre de fiches en cours par laveur (0 = pas de limite)",
}

// lireReglesCommandes lit les règles d'une station, valeurs par défaut
// comprises. Les anciens noms éventuels sont ignorés : la clé est unique.
func (a *App) lireReglesCommandes(ctx context.Context, stationID string) (reglesCommandes, error) {
	regles := reglesParDefaut()
	if stationID == "" {
		return regles, nil
	}
	var raw []byte
	if err := a.Pool.QueryRow(ctx,
		`SELECT settings FROM stations WHERE id = $1`, stationID).Scan(&raw); err != nil {
		return regles, err
	}
	reglages := map[string]any{}
	parseJSONB(raw, &reglages)
	bloc := map[string]any{}
	if brut, ok := reglages["order_rules"].(map[string]any); ok {
		bloc = brut
	}
	if v, ok := bloc["allow_walk_in"].(bool); ok {
		regles.AllowWalkIn = v
	}
	if v, ok := bloc["require_washer"].(bool); ok {
		regles.RequireWasher = v
	}
	if v, ok := bloc["quality_check_required"].(bool); ok {
		regles.QualityCheckRequired = v
	}
	if v, ok := bloc["release_requires_payment"].(bool); ok {
		regles.ReleaseRequiresPayment = v
	}
	if v, ok := bloc["allow_partial_payments"].(bool); ok {
		regles.AllowPartialPayments = v
	}
	if v, ok := bloc["require_cancel_reason"].(bool); ok {
		regles.RequireCancelReason = v
	}
	if v, ok := bloc["max_open_orders_per_washer"].(float64); ok {
		regles.MaxOpenOrdersPerWasher = int(v)
	}
	return regles, nil
}

// reglagesReglesCommandes : sérialisation pour l'écriture.
func reglagesReglesCommandes(r reglesCommandes) map[string]any {
	return map[string]any{
		"allow_walk_in":              r.AllowWalkIn,
		"require_washer":             r.RequireWasher,
		"quality_check_required":     r.QualityCheckRequired,
		"release_requires_payment":   r.ReleaseRequiresPayment,
		"allow_partial_payments":     r.AllowPartialPayments,
		"require_cancel_reason":      r.RequireCancelReason,
		"max_open_orders_per_washer": r.MaxOpenOrdersPerWasher,
	}
}

// ---------------------------------------------------------------------------
// GET | PUT /api/v1/stations/{id}/order-rules
// ---------------------------------------------------------------------------

func (a *App) handleOrderRulesRead(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	regles, err := a.lireReglesCommandes(r.Context(), stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"station_id": stationID,
		"rules":      regles,
		"labels":     libellesReglesCommandes,
		"statuses":   statutsFiche,
	})
}

func (a *App) handleOrderRulesUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if err := a.stationAutorisee(r.Context(), deref(user.OrganizationID), stationID); err != nil {
		writeAPIError(w, err)
		return
	}

	var body struct {
		AllowWalkIn            *bool `json:"allow_walk_in"`
		RequireWasher          *bool `json:"require_washer"`
		QualityCheckRequired   *bool `json:"quality_check_required"`
		ReleaseRequiresPayment *bool `json:"release_requires_payment"`
		AllowPartialPayments   *bool `json:"allow_partial_payments"`
		RequireCancelReason    *bool `json:"require_cancel_reason"`
		MaxOpenOrdersPerWasher *int  `json:"max_open_orders_per_washer"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	regles, err := a.lireReglesCommandes(r.Context(), stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if body.AllowWalkIn != nil {
		regles.AllowWalkIn = *body.AllowWalkIn
	}
	if body.RequireWasher != nil {
		regles.RequireWasher = *body.RequireWasher
	}
	if body.QualityCheckRequired != nil {
		regles.QualityCheckRequired = *body.QualityCheckRequired
	}
	if body.ReleaseRequiresPayment != nil {
		regles.ReleaseRequiresPayment = *body.ReleaseRequiresPayment
	}
	if body.AllowPartialPayments != nil {
		regles.AllowPartialPayments = *body.AllowPartialPayments
	}
	if body.RequireCancelReason != nil {
		regles.RequireCancelReason = *body.RequireCancelReason
	}
	if body.MaxOpenOrdersPerWasher != nil {
		if *body.MaxOpenOrdersPerWasher < 0 || *body.MaxOpenOrdersPerWasher > 100 {
			writeError(w, http.StatusBadRequest, "validation_error",
				"La limite de fiches par laveur doit être comprise entre 0 (aucune) et 100.",
				map[string]string{"max_open_orders_per_washer": "Valeur invalide"})
			return
		}
		regles.MaxOpenOrdersPerWasher = *body.MaxOpenOrdersPerWasher
	}

	if _, err := a.Pool.Exec(r.Context(), `
		UPDATE stations SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb),
		       '{order_rules}', $2::jsonb, true)
		WHERE id = $1 AND organization_id = $3`,
		stationID, jsonDe(reglagesReglesCommandes(regles)), deref(user.OrganizationID)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'enregistrement.", nil)
		return
	}

	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"order_rules.updated", "station", stationID, reglagesReglesCommandes(regles))
	writeJSON(w, http.StatusOK, map[string]any{
		"station_id": stationID, "rules": regles, "labels": libellesReglesCommandes,
	})
}

// ---------------------------------------------------------------------------
// Cycle de vie d'une fiche.
// ---------------------------------------------------------------------------

// statutFiche : un état du cycle de vie, tel que l'écran le présente.
type statutFiche struct {
	Code        string `json:"code"`
	Label       string `json:"label"`
	Terminal    bool   `json:"terminal"`
	Description string `json:"description"`
}

// statutsFiche : la file réelle d'un véhicule dans une station.
//
// « Reçue » et « En attente » ne font qu'un (`pending`) : la réception est
// l'acte de création de la fiche, et l'attente commence à cet instant précis.
// En faire deux états obligerait à cliquer deux fois pour dire la même chose.
var statutsFiche = []statutFiche{
	{Code: "pending", Label: "Reçue, en attente",
		Description: "Le véhicule est enregistré, le lavage n'a pas commencé."},
	{Code: "in_progress", Label: "En cours de lavage",
		Description: "Un laveur travaille sur le véhicule."},
	{Code: "quality_check", Label: "Contrôle qualité",
		Description: "Le lavage est fini, le véhicule est vérifié avant d'être rendu."},
	{Code: "ready", Label: "Prête",
		Description: "Le véhicule est prêt et attend son client."},
	{Code: "completed", Label: "Livrée, soldée", Terminal: true,
		Description: "Le véhicule a été remis et la fiche est réglée."},
	{Code: "disputed", Label: "En litige",
		Description: "Le client conteste le travail : la fiche reste ouverte."},
	{Code: "canceled", Label: "Annulée", Terminal: true,
		Description: "Le lavage n'a pas été fait (ou a été dédommagé)."},
}

// transitionsFiche : d'où l'on peut aller où. Ce graphe est le seul endroit où
// un statut change — l'interface ne fait que proposer les flèches autorisées.
var transitionsFiche = map[string][]string{
	"pending":       {"in_progress", "quality_check", "ready", "completed", "canceled", "disputed"},
	"in_progress":   {"pending", "quality_check", "ready", "completed", "canceled", "disputed"},
	"quality_check": {"in_progress", "ready", "completed", "canceled", "disputed"},
	"ready":         {"in_progress", "quality_check", "completed", "canceled", "disputed"},
	"disputed":      {"in_progress", "quality_check", "ready", "completed", "canceled"},
	"completed":     {"disputed"},
	"canceled":      {},
}

// libelleStatut : libellé français d'un code de statut.
func libelleStatut(code string) string {
	for _, s := range statutsFiche {
		if s.Code == code {
			return s.Label
		}
	}
	return code
}

// libelleStatutLower : le même, en minuscules, pour une phrase.
func libelleStatutLower(code string) string {
	return strings.ToLower(libelleStatut(code))
}

// statutConnu : le code fait-il partie du cycle de vie ?
func statutConnu(code string) bool {
	for _, s := range statutsFiche {
		if s.Code == code {
			return true
		}
	}
	return false
}

// transitionAutorisee : la flèche existe-t-elle ?
func transitionAutorisee(depuis, vers string) bool {
	for _, cible := range transitionsFiche[depuis] {
		if cible == vers {
			return true
		}
	}
	return false
}

// verifierTransitionFiche applique, dans l'ordre :
//  1. le statut cible existe ;
//  2. la flèche existe dans le cycle de vie ;
//  3. le rôle a le droit d'aller jusque-là (annuler n'est pas modifier) ;
//  4. les règles de la station (contrôle qualité, remise subordonnée au
//     paiement) et les règles de gestion (motif obligatoire).
//
// Le retour est une *apiError : le message dit **pourquoi**, pas seulement
// « refusé ».
func (a *App) verifierTransitionFiche(ctx context.Context, order *WashOrder, cible string,
	regles reglesCommandes, motif string, user *User) *apiError {
	if !statutConnu(cible) {
		return &apiError{Status: http.StatusBadRequest, Code: "validation_error",
			Message: "Statut inconnu.", Fields: map[string]string{"status": "Statut invalide"}}
	}
	if !transitionAutorisee(order.Status, cible) {
		return &apiError{Status: http.StatusConflict, Code: "transition_forbidden",
			Message: "Une fiche « " + libelleStatutLower(order.Status) + " » ne peut pas passer à « " +
				libelleStatutLower(cible) + " ».",
			Fields: map[string]string{"status": "Transition interdite"}}
	}
	// Annuler un lavage engage la station : la permission dédiée est exigée,
	// sans quoi « modifier une fiche » suffirait à annuler un lavage.
	if cible == "canceled" && !hasPermission(user, "wash:cancel") {
		return &apiError{Status: http.StatusForbidden, Code: "forbidden",
			Message: "L'annulation d'un lavage nécessite l'accord du gérant."}
	}
	if cible == "canceled" && regles.RequireCancelReason {
		existant := ""
		if order.CancelReason != nil {
			existant = strings.TrimSpace(*order.CancelReason)
		}
		if strings.TrimSpace(motif) == "" && existant == "" {
			return &apiError{Status: http.StatusBadRequest, Code: "validation_error",
				Message: "Une annulation doit être motivée.",
				Fields:  map[string]string{"reason": "Motif requis"}}
		}
	}
	if cible == "disputed" && strings.TrimSpace(motif) == "" {
		return &apiError{Status: http.StatusBadRequest, Code: "validation_error",
			Message: "Un litige doit être motivé : écrivez ce que le client conteste.",
			Fields:  map[string]string{"dispute_reason": "Motif requis"}}
	}
	// Contrôle qualité obligatoire : on ne saute pas l'étape, mais on peut
	// toujours reculer (retour en atelier) ou annuler.
	if cible == "ready" && regles.QualityCheckRequired && order.Status != "quality_check" {
		return &apiError{Status: http.StatusConflict, Code: "quality_check_required",
			Message: "Cette station exige un contrôle qualité : faites passer la fiche par « Contrôle qualité » avant de la déclarer prête.",
			Fields:  map[string]string{"status": "Contrôle qualité requis"}}
	}
	// Remise subordonnée au paiement : c'est la règle qui empêche de livrer un
	// véhicule impayé — et l'inverse de 7.6, une fiche ne devient pas « livrée »
	// parce qu'on a coché une étape technique.
	if cible == "completed" && regles.ReleaseRequiresPayment {
		_, _, due, err := a.soldeFicheRequete(ctx, order.ID, order.OrganizationID)
		if err == nil && due > 0 {
			return &apiError{Status: http.StatusConflict, Code: "payment_required",
				Message: itoa(due) + " FCFA restent à encaisser avant de remettre ce véhicule.",
				Fields:  map[string]string{"status": "Encaissement requis"}}
		}
	}
	return nil
}

// verifierAcompte : la station accepte-t-elle un règlement partiel ?
//
// Les deux chemins d'encaissement (la fiche et la caisse) l'appellent : une
// règle appliquée à un seul des deux ne serait pas une règle. Un engagement de
// paiement (`differe`) n'est pas un encaissement et n'est jamais concerné.
func (a *App) verifierAcompte(ctx context.Context, stationID, method string, amount, due int) *apiError {
	if method == "differe" || due <= 0 || amount >= due {
		return nil
	}
	regles, err := a.lireReglesCommandes(ctx, stationID)
	if err != nil {
		return &apiError{Status: http.StatusInternalServerError, Code: "internal",
			Message: "Erreur interne."}
	}
	if regles.AllowPartialPayments {
		return nil
	}
	return &apiError{Status: http.StatusConflict, Code: "partial_payment_disabled",
		Message: "Cette station n'accepte pas les paiements partiels : encaissez le solde en une fois.",
		Fields:  map[string]string{"amount_xof": "Règlement en une fois requis"}}
}

// soldeFicheRequete : la même lecture que `soldeFiche`, sans requête HTTP —
// pour être utilisable depuis un validateur.
func (a *App) soldeFicheRequete(ctx context.Context, orderID, orgID string) (total, paid, due int, err error) {
	err = a.Pool.QueryRow(ctx, `
		SELECT o.total_xof,
		       COALESCE((SELECT sum(p.amount_xof) FROM payments p
		                  WHERE p.wash_order_id = o.id AND p.status = 'valid'
		                    AND p.method <> 'differe'), 0)
		FROM wash_orders o
		WHERE o.id = $1 AND o.organization_id = $2 AND o.status <> 'canceled'`,
		orderID, orgID).Scan(&total, &paid)
	if err != nil {
		return 0, 0, 0, err
	}
	due = total - paid
	if due < 0 {
		due = 0
	}
	return total, paid, due, nil
}

// fichesOuvertesDuLaveur compte les fiches qu'un laveur porte encore.
func (a *App) fichesOuvertesDuLaveur(ctx context.Context, orgID, employeeID string) (int, error) {
	var n int
	err := a.Pool.QueryRow(ctx, `
		SELECT count(*) FROM wash_orders
		WHERE organization_id = $1 AND employee_id = $2
		  AND status IN ('pending', 'in_progress', 'quality_check', 'ready', 'disputed')`,
		orgID, employeeID).Scan(&n)
	return n, err
}

// jsonDe : sérialisation JSON d'une valeur. Une erreur de sérialisation d'une
// carte de booléens n'arrive pas ; si elle arrivait, l'écriture partirait d'un
// objet vide plutôt que d'écrire n'importe quoi.
func jsonDe(valeur any) string {
	brut, err := json.Marshal(valeur)
	if err != nil {
		return "{}"
	}
	return string(brut)
}
