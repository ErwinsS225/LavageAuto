package main

import "encoding/json"

// ---------------------------------------------------------------------------
// Réglages par défaut d'une station.
//
// Une seule définition pour tous les chemins de création (inscription, écran
// Réglages, jeu de démonstration) : auparavant chaque appel écrivait son propre
// littéral JSON, et l'inscription avait « oublié » la règle de caisse — une
// entreprise créée en s'inscrivant acceptait les espèces sans session de caisse
// ouverte, alors que l'écran Réglages affiche l'interrupteur activé.
// ---------------------------------------------------------------------------

// defaultStationSettings rend les réglages d'une nouvelle station, en JSON.
// monthlyGoalXOF : objectif de chiffre d'affaires mensuel (0 = aucun).
func defaultStationSettings(monthlyGoalXOF int) string {
	reglages := map[string]any{
		"commission_mode":           "per_wash",
		"commission_per_wash_xof":   500,
		"monthly_goal_xof":          monthlyGoalXOF,
		"require_open_cash_session": true,
	}
	brut, err := json.Marshal(reglages)
	if err != nil {
		// json.Marshal n'échoue pas sur ces types : repli défensif.
		return `{"commission_mode":"per_wash","commission_per_wash_xof":500,"monthly_goal_xof":0,"require_open_cash_session":true}`
	}
	return string(brut)
}
