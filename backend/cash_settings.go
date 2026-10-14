package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Paramètres de caisse et identité des reçus : une seule source de vérité.
//
// Aucun de ces réglages n'est décoratif : chacun est lu à l'endroit où il
// décide. Un réglage que le serveur n'applique pas serait un mensonge affiché
// dans un formulaire.
//
// Les clés JSON sont celles-ci, et rien d'autre (les anciens noms préfixés
// `cash_…` n'existent plus) :
//
//   - `require_open_cash_session`     → refus d'encaisser des espèces hors session
//   - `max_cashier_discount_xof`      → plafond de remise du caissier
//   - `default_float_xof`             → fond de caisse proposé à l'ouverture
//   - `discrepancy_tolerance_xof`     → écart toléré avant justification écrite
//   - `allowed_roles`                 → rôles autorisés à ouvrir/clôturer
//   - `allow_void`                    → autorise l'annulation d'un encaissement
//   - `receipt_prefix`, `receipt_footer`, `receipt_legal_name`, `receipt_tax_id`,
//     `receipt_logo_url`              → ce que le justificatif porte, et rien de plus
// ---------------------------------------------------------------------------

// ParametresCaisse : ce que l'écran des paramètres affiche et modifie.
type parametresCaisse struct {
	RequireOpenCashSession  bool     `json:"require_open_cash_session"`
	MaxCashierDiscountXOF   int      `json:"max_cashier_discount_xof"`
	DefaultFloatXOF         int      `json:"default_float_xof"`
	DiscrepancyToleranceXOF int      `json:"discrepancy_tolerance_xof"`
	AllowedRoles            []string `json:"allowed_roles"`
	AllowVoid               bool     `json:"allow_void"`
	ReceiptPrefix           string   `json:"receipt_prefix"`
	ReceiptFooter           string   `json:"receipt_footer"`
	ReceiptLegalName        string   `json:"receipt_legal_name"`
	ReceiptTaxID            string   `json:"receipt_tax_id"`
	ReceiptLogoURL          string   `json:"receipt_logo_url"`
}

// roleOuvrantCaisse : les rôles qui peuvent se voir confier une caisse. Le
// liteur est le même que celui des permissions — un rôle sans `cash:open` ne
// peut pas être autorisé ici.
func roleOuvrantCaisse(role string) bool {
	switch role {
	case "owner", "manager", "cashier":
		return true
	}
	return false
}

// lireParametresCaisse lit les réglages d'une station et applique les valeurs
// par défaut **de l'application** (et non des valeurs inventées ici : ce sont
// celles que le serveur applique déjà quand la clé est absente).
func (a *App) lireParametresCaisse(ctx context.Context, stationID string) (parametresCaisse, error) {
	var raw []byte
	if err := a.Pool.QueryRow(ctx, `SELECT settings FROM stations WHERE id = $1`, stationID).Scan(&raw); err != nil {
		return parametresCaisse{}, err
	}
	reglages := map[string]any{}
	parseJSONB(raw, &reglages)

	parametres := parametresCaisse{
		RequireOpenCashSession: true,
		MaxCashierDiscountXOF:  1000,
		AllowedRoles:           []string{},
		AllowVoid:              true,
		ReceiptPrefix:          "REC",
	}
	if v, ok := reglages["require_open_cash_session"].(bool); ok {
		parametres.RequireOpenCashSession = v
	}
	if v, ok := reglages["max_cashier_discount_xof"].(float64); ok {
		parametres.MaxCashierDiscountXOF = int(v)
	}
	// Les clés sont les mêmes que celles du corps HTTP. Les noms préfixés
	// `cash_…` d'une première version restent lus en repli : une base déjà
	// réglée ne perd pas ses valeurs.
	if v, ok := nombreReglage(reglages, "default_float_xof", "cash_default_float_xof"); ok {
		parametres.DefaultFloatXOF = v
	}
	if v, ok := nombreReglage(reglages, "discrepancy_tolerance_xof", "cash_discrepancy_tolerance_xof"); ok {
		parametres.DiscrepancyToleranceXOF = v
	}
	if v, ok := booleenReglage(reglages, "allow_void", "cash_allow_void"); ok {
		parametres.AllowVoid = v
	}
	if roles, ok := rolesReglage(reglages, "allowed_roles", "cash_allowed_roles"); ok {
		for _, role := range roles {
			if roleOuvrantCaisse(role) {
				parametres.AllowedRoles = append(parametres.AllowedRoles, role)
			}
		}
	}
	parametres.ReceiptPrefix = orDefault(chaineReglage(reglages, "receipt_prefix"), "REC")
	parametres.ReceiptFooter = chaineReglage(reglages, "receipt_footer")
	parametres.ReceiptLegalName = chaineReglage(reglages, "receipt_legal_name")
	parametres.ReceiptTaxID = chaineReglage(reglages, "receipt_tax_id")
	parametres.ReceiptLogoURL = chaineReglage(reglages, "receipt_logo_url")
	return parametres, nil
}

// nombreReglage : premier réglage numérique renseigné, dans l'ordre des clés
// données (la clé courante d'abord, les anciens noms ensuite).
func nombreReglage(reglages map[string]any, cles ...string) (int, bool) {
	for _, cle := range cles {
		if v, ok := reglages[cle].(float64); ok {
			return int(v), true
		}
	}
	return 0, false
}

func booleenReglage(reglages map[string]any, cles ...string) (bool, bool) {
	for _, cle := range cles {
		if v, ok := reglages[cle].(bool); ok {
			return v, true
		}
	}
	return false, false
}

func rolesReglage(reglages map[string]any, cles ...string) ([]string, bool) {
	for _, cle := range cles {
		if bruts, ok := reglages[cle].([]any); ok {
			roles := []string{}
			for _, brut := range bruts {
				if role, ok := brut.(string); ok {
					roles = append(roles, role)
				}
			}
			return roles, true
		}
	}
	return nil, false
}

func chaineReglage(reglages map[string]any, cle string) string {
	if v, ok := reglages[cle].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// reglagesCaisse : sérialise les paramètres pour l'écriture en base.
func reglagesCaisse(p parametresCaisse) map[string]any {
	roles := p.AllowedRoles
	if roles == nil {
		roles = []string{}
	}
	return map[string]any{
		"require_open_cash_session": p.RequireOpenCashSession,
		"max_cashier_discount_xof":  p.MaxCashierDiscountXOF,
		"default_float_xof":         p.DefaultFloatXOF,
		"discrepancy_tolerance_xof": p.DiscrepancyToleranceXOF,
		"allowed_roles":             roles,
		"allow_void":                p.AllowVoid,
		"receipt_prefix":            p.ReceiptPrefix,
		"receipt_footer":            p.ReceiptFooter,
		"receipt_legal_name":        p.ReceiptLegalName,
		"receipt_tax_id":            p.ReceiptTaxID,
		"receipt_logo_url":          p.ReceiptLogoURL,
	}
}

// GET /stations/{id}/cash-settings
func (a *App) handleCashSettingsRead(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	st, err := a.getStation(r.Context(), id)
	if err != nil || st == nil || st.OrganizationID != deref(user.OrganizationID) {
		writeError(w, http.StatusNotFound, "not_found", "Station introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != id {
		writeAPIError(w, errStationForbidden)
		return
	}
	parametres, err := a.lireParametresCaisse(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"station_id": id,
		"settings":   parametres,
		"roles_connus": []map[string]string{
			{"code": "owner", "label": roleLabelFR("owner")},
			{"code": "manager", "label": roleLabelFR("manager")},
			{"code": "cashier", "label": roleLabelFR("cashier")},
		},
	})
}

// PUT /stations/{id}/cash-settings
func (a *App) handleCashSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	st, err := a.getStation(r.Context(), id)
	if err != nil || st == nil || st.OrganizationID != deref(user.OrganizationID) {
		writeError(w, http.StatusNotFound, "not_found", "Station introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != id {
		writeAPIError(w, errStationForbidden)
		return
	}

	present := false
	var body struct {
		RequireOpenCashSession  *bool    `json:"require_open_cash_session"`
		MaxCashierDiscountXOF   *int     `json:"max_cashier_discount_xof"`
		DefaultFloatXOF         *int     `json:"default_float_xof"`
		DiscrepancyToleranceXOF *int     `json:"discrepancy_tolerance_xof"`
		AllowedRoles            []string `json:"allowed_roles"`
		AllowVoid               *bool    `json:"allow_void"`
		ReceiptPrefix           *string  `json:"receipt_prefix"`
		ReceiptFooter           *string  `json:"receipt_footer"`
		ReceiptLegalName        *string  `json:"receipt_legal_name"`
		ReceiptTaxID            *string  `json:"receipt_tax_id"`
		ReceiptLogoURL          *string  `json:"receipt_logo_url"`
	}
	// Le corps est lu une fois, puis interprété deux fois : comme champs typés
	// (validation) et comme dictionnaire (présence). C'est ce qui distingue
	// « champ absent » de « champ vidé » — effacer les mentions fiscales est une
	// intention légitime, ne rien envoyer aussi.
	brut, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil || json.Unmarshal(brut, &body) != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	corps := map[string]any{}
	_ = json.Unmarshal(brut, &corps)
	if _, fourni := corps["allowed_roles"]; fourni {
		present = true
	}

	parametres, err := a.lireParametresCaisse(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	if body.RequireOpenCashSession != nil {
		parametres.RequireOpenCashSession = *body.RequireOpenCashSession
	}
	if body.AllowVoid != nil {
		parametres.AllowVoid = *body.AllowVoid
	}
	for _, valeur := range []struct {
		champ string
		ptr   *int
		cible *int
	}{
		{"max_cashier_discount_xof", body.MaxCashierDiscountXOF, &parametres.MaxCashierDiscountXOF},
		{"default_float_xof", body.DefaultFloatXOF, &parametres.DefaultFloatXOF},
		{"discrepancy_tolerance_xof", body.DiscrepancyToleranceXOF, &parametres.DiscrepancyToleranceXOF},
	} {
		if valeur.ptr == nil {
			continue
		}
		if *valeur.ptr < 0 {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Un montant de caisse ne peut pas être négatif.",
				map[string]string{valeur.champ: "Montant négatif"})
			return
		}
		*valeur.cible = *valeur.ptr
	}
	if present {
		roles := []string{}
		for _, role := range body.AllowedRoles {
			if !roleOuvrantCaisse(role) {
				writeError(w, http.StatusBadRequest, "validation_error",
					"Rôle inconnu pour la tenue de caisse : "+role,
					map[string]string{"allowed_roles": "Rôle invalide"})
				return
			}
			roles = append(roles, role)
		}
		parametres.AllowedRoles = roles
	}
	for _, valeur := range []struct {
		champ  string
		ptr    *string
		cible  *string
		limite int
	}{
		{"receipt_prefix", body.ReceiptPrefix, &parametres.ReceiptPrefix, 10},
		{"receipt_footer", body.ReceiptFooter, &parametres.ReceiptFooter, 400},
		{"receipt_legal_name", body.ReceiptLegalName, &parametres.ReceiptLegalName, 160},
		{"receipt_tax_id", body.ReceiptTaxID, &parametres.ReceiptTaxID, 60},
		{"receipt_logo_url", body.ReceiptLogoURL, &parametres.ReceiptLogoURL, 300},
	} {
		if valeur.ptr == nil {
			continue
		}
		texte := strings.TrimSpace(*valeur.ptr)
		if len([]rune(texte)) > valeur.limite {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Ce texte est trop long (maximum "+itoa(valeur.limite)+" caractères).",
				map[string]string{valeur.champ: "Texte trop long"})
			return
		}
		*valeur.cible = texte
	}
	if parametres.ReceiptPrefix == "" {
		// Un préfixe vide produirait des numéros sans repère : on garde celui
		// par défaut plutôt que d'écrire une chaîne vide.
		parametres.ReceiptPrefix = "REC"
	}

	enregistres, err := a.mergeStationSettings(r, id, reglagesCaisse(parametres))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if _, err := a.Pool.Exec(r.Context(),
		`UPDATE stations SET settings = $1::jsonb WHERE id = $2`, jsonOrNull(enregistres), id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	a.addAudit(r.Context(), deref(user.OrganizationID), id, user.FullName,
		"cash_settings.updated", "station", id, map[string]any{
			"require_open_cash_session": parametres.RequireOpenCashSession,
			"allow_void":                parametres.AllowVoid,
			"allowed_roles":             parametres.AllowedRoles,
			"actor_role":                user.Role,
		})

	relus, err := a.lireParametresCaisse(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"station_id": id,
		"settings":   relus,
		"message":    "Paramètres de caisse enregistrés.",
	})
}

// roleAutoriseCaisseApplique : la règle des rôles autorisés, appliquée à
// l'ouverture comme à la clôture. Une liste vide signifie « tous les rôles que
// leurs permissions autorisent déjà » — on ne restreint rien par défaut, mais
// on ne l'invente pas non plus quand la station s'est prononcée.
func (a *App) roleAutoriseCaisse(ctx context.Context, stationID string, user *User,
) (bool, error) {
	parametres, err := a.lireParametresCaisse(ctx, stationID)
	if err != nil {
		return true, err
	}
	if len(parametres.AllowedRoles) == 0 {
		return true, nil
	}
	for _, role := range parametres.AllowedRoles {
		if role == user.Role || user.Role == "owner" || user.Role == "superadmin" {
			return true, nil
		}
	}
	return false, nil
}

// errCaisseRole interdit l'ouverture/la clôture à un rôle non déclaré.
var errCaisseRole = &apiError{
	Status:  http.StatusForbidden,
	Code:    "cash_role_forbidden",
	Message: "Votre rôle n'est pas autorisé à tenir la caisse de cette station : voir les paramètres de caisse.",
}
