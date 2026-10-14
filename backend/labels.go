package main

// ---------------------------------------------------------------------------
// Libellés français : le serveur renvoie déjà les intitulés que l'interface
// affiche, pour que les exports CSV et les écrans ne divergent pas.
// ---------------------------------------------------------------------------

// roleLabelFR libellé français d'un rôle (payload /auth/me, rapports, exports).
func roleLabelFR(role string) string {
	switch role {
	case "superadmin":
		return "Support WashGuard"
	case "owner":
		return "Propriétaire"
	case "manager":
		return "Gérant"
	case "cashier":
		return "Caissier"
	case "washer":
		return "Laveur"
	default:
		return "Administrateur"
	}
}

func methodLabelFR(method string) string {
	switch method {
	case "especes":
		return "Espèces"
	case "mobile_money":
		return "Mobile Money"
	case "carte":
		return "Carte bancaire"
	case "virement":
		return "Virement"
	case "differe":
		return "Paiement différé"
	case "autre":
		return "Autre"
	default:
		return method
	}
}

func statusLabelFR(status string) string {
	switch status {
	case "pending":
		return "En attente"
	case "in_progress":
		return "En cours"
	case "completed":
		return "Terminé"
	case "canceled":
		return "Annulé"
	case "open":
		return "Ouverte"
	case "closed":
		return "Clôturée"
	case "valid":
		return "Valide"
	case "voided":
		return "Annulé"
	case "active":
		return "Active"
	case "suspended":
		return "Suspendue"
	default:
		return status
	}
}

// definitionsRapport : libellés des indicateurs affichés par les rapports.
func definitionsRapport() map[string]any {
	return map[string]any{
		"wash_count":          "Lavages terminés",
		"canceled_count":      "Fiches annulées",
		"service_count":       "Prestations vendues",
		"revenue_xof":         "Chiffre d'affaires des fiches terminées",
		"discount_xof":        "Remises accordées",
		"payments_xof":        "Encaissements validés",
		"cash_xof":            "Encaissements en espèces",
		"mobile_money_xof":    "Encaissements Mobile Money",
		"other_xof":           "Carte, virement et autres",
		"expenses_xof":        "Dépenses validées",
		"expenses_cash_xof":   "Dépenses payées en espèces",
		"result_xof":          "Chiffre d'affaires moins les dépenses",
		"cash_result_xof":     "Encaissements espèces moins dépenses espèces",
		"due_xof":             "Reste à encaisser sur les fiches ouvertes",
		"average_ticket_xof":  "Panier moyen par lavage terminé",
		"voided_payments_xof": "Encaissements annulés",
	}
}
