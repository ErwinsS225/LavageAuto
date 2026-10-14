package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// routes_org.go — fiche de l'entreprise : GET/PATCH /organizations/current.
//
// La table organizations ne porte que l'essentiel (name, phone, email, city,
// country, legal_name, billing_address, settings). L'identité commerciale, la
// facturation détaillée et les mentions libres vivent dans le JSONB `settings`
// sous des clés dédiées ; les mentions administratives ont leur table
// registry_fields (créée à l'inscription, une ligne par mention, status='empty').

var champsIdentite = []string{"trade_name", "activity_type", "activity_description", "activity_started_on", "whatsapp", "website"}
var champsFacturation = []string{"billing_name", "billing_email", "billing_phone"}

// GET /organizations/current — entreprise + registre des mentions.
func (a *App) lireOrganisationDetaillee(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	orgID, ok := organisationSession(sess)
	if !ok {
		repondreErreur(w, http.StatusNotFound, "no_organization", "Aucune entreprise rattachée à ce compte.")
		return
	}
	if !sess.aPermission("settings:view") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	ctx := r.Context()
	org, err := a.lireOrganisationDetailleeParID(ctx, orgID)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, org)
}

func (a *App) lireOrganisationDetailleeParID(ctx context.Context, orgID string) (map[string]interface{}, error) {
	var name, phone, email, city, country, legalName, billingAddress string
	var settingsBody []byte
	var createdAt time.Time
	err := a.db.QueryRow(ctx, `
		SELECT name, phone, email, city, country, legal_name, billing_address, settings, created_at
		FROM organizations WHERE id = $1`, orgID).
		Scan(&name, &phone, &email, &city, &country, &legalName, &billingAddress, &settingsBody, &createdAt)
	if err != nil {
		return nil, err
	}
	reglages := decoderObjet(settingsBody)
	identite := map[string]interface{}{}
	if brut, ok := reglages["organization_identity"]; ok {
		if m, ok := brut.(map[string]interface{}); ok {
			identite = m
		}
	}
	out := map[string]interface{}{
		"organization": map[string]interface{}{
			"id":                   orgID,
			"name":                 name,
			"slug":                 slugDe(name),
			"email":                email,
			"phone":                phone,
			"address":              chaineOuVide(reglages["organization_address"]),
			"city":                 city,
			"country":              country,
			"currency":             chaineOuDefaut(reglages["organization_currency"], "XOF"),
			"timezone":             chaineOuDefaut(reglages["organization_timezone"], "Africa/Abidjan"),
			"logo_url":             chaineOuVide(reglages["organization_logo"]),
			"status":               statutOrganisation(reglages),
			"created_at":           createdAt.UTC().Format(time.RFC3339),
			"legal_name":           legalName,
			"trade_name":           chaineOuVide(identite["trade_name"]),
			"activity_type":        chaineOuVide(identite["activity_type"]),
			"activity_description": chaineOuVide(identite["activity_description"]),
			"activity_started_on":  chaineOuVide(identite["activity_started_on"]),
			"whatsapp":             chaineOuVide(identite["whatsapp"]),
			"website":              chaineOuVide(identite["website"]),
			"billing_name":         chaineOuVide(identite["billing_name"]),
			"billing_address":      billingAddress,
			"billing_email":        chaineOuVide(identite["billing_email"]),
			"billing_phone":        chaineOuVide(identite["billing_phone"]),
			"legal_form":           chaineOuVide(identite["legal_form"]),
			"legal_representative": chaineOuVide(identite["legal_representative"]),
			"rccm":                 chaineOuVide(identite["rccm"]),
			"tax_id":               chaineOuVide(identite["tax_id"]),
			"cnps_employer_number": chaineOuVide(identite["cnps_employer_number"]),
			"fiscal_regime":        chaineOuVide(identite["fiscal_regime"]),
			"legal_mentions":       chaineOuVide(identite["legal_mentions"]),
			"settings":             reglages,
		},
		"registry": a.registreMentions(ctx, orgID),
	}
	return out, nil
}

func (a *App) registreMentions(ctx context.Context, orgID string) []map[string]interface{} {
	rows, err := a.db.Query(ctx, `
		SELECT field_code, value, status, verified_at FROM registry_fields
		WHERE organization_id = $1 ORDER BY field_code`, orgID)
	mentions := []map[string]interface{}{}
	if err != nil {
		return mentions
	}
	defer rows.Close()
	// Libellés du référentiel (support.go) ; les codes inconnus restent listés.
	labels := map[string]string{}
	aides := map[string]string{}
	for _, m := range mentionsRegistre {
		labels[m.code] = m.label
		aides[m.code] = m.aide
	}
	for rows.Next() {
		var code, valeur, statut string
		var verifie *time.Time
		if err := rows.Scan(&code, &valeur, &statut, &verifie); err != nil {
			continue
		}
		label := labels[code]
		if label == "" {
			label = code
		}
		mention := map[string]interface{}{
			"field_code":   code,
			"label":        label,
			"help":         aides[code],
			"value":        valeur,
			"status":       statut,
			"status_label": etiquetteStatut(statut),
			"verified_at": func() interface{} {
				if verifie == nil {
					return nil
				}
				return verifie.UTC().Format(time.RFC3339)
			}(),
		}
		mentions = append(mentions, mention)
	}
	return mentions
}

func etiquetteStatut(statut string) string {
	switch statut {
	case "verified":
		return "Vérifié"
	case "pending_review":
		return "En attente de vérification"
	case "provided":
		return "Renseigné"
	default:
		return "Non renseigné"
	}
}

// Petit slug lisible : minuscules, tirets. Sert d'identifiant d'affichage.
func slugDe(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	tiret := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			tiret = true
		default:
			if tiret {
				b.WriteRune('-')
				tiret = false
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func chaineOuDefaut(v interface{}, defaut string) string {
	if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	return defaut
}

func statutOrganisation(reglages map[string]interface{}) string {
	if suspendu, ok := reglages["organization_suspended"].(bool); ok && suspendu {
		return "suspended"
	}
	return "active"
}

// PATCH /organizations/current — identité, facturation, mentions, réglages.
func (a *App) modifierOrganisation(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	orgID, ok := organisationSession(sess)
	if !ok {
		repondreErreur(w, http.StatusNotFound, "no_organization", "Aucune entreprise rattachée à ce compte.")
		return
	}
	if !sess.aPermission("settings:manage") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Seuls le propriétaire et le gérant modifient la fiche d'entreprise.")
		return
	}
	corps, ok := decoderCorpsLibre(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	// Nom affiché : name reste la source d'affichage ; le front envoie
	// trade_name/legal_name, on suit le nom commercial sinon la raison sociale.
	var nom string
	if brut, ok := corps["trade_name"]; ok {
		var v string
		_ = json.Unmarshal(brut, &v)
		if strings.TrimSpace(v) != "" {
			nom = strings.TrimSpace(v)
		}
	}
	if nom == "" {
		if brut, ok := corps["legal_name"]; ok {
			var v string
			_ = json.Unmarshal(brut, &v)
			if strings.TrimSpace(v) != "" {
				nom = strings.TrimSpace(v)
			}
		}
	}

	// legal_name et billing_address ont leur colonne ; le reste va dans settings.
	var legalName, billingAddress *string
	if brut, ok := corps["legal_name"]; ok {
		var v string
		_ = json.Unmarshal(brut, &v)
		legalName = &v
	}
	if brut, ok := corps["billing_address"]; ok {
		var v string
		_ = json.Unmarshal(brut, &v)
		billingAddress = &v
	}

	// Tout ce qui n'est ni colonne ni registre est rangé dans settings.organization_identity.
	champsSettings := map[string]bool{}
	for _, c := range champsIdentite {
		champsSettings[c] = true
	}
	for _, c := range champsFacturation {
		champsSettings[c] = true
	}
	champsSettings["legal_mentions"] = true
	champsSettings["registry"] = true
	champsSettings["trade_name"] = true // déjà traité (nom affiché) mais aussi mémorisé
	champsSettings["legal_name"] = true
	champsSettings["billing_address"] = true
	champsSettings["name"] = true

	affecteIdentite := map[string]interface{}{}
	for cle, brut := range corps {
		if champsSettings[cle] {
			var v interface{}
			_ = json.Unmarshal(brut, &v)
			if s, ok := v.(string); ok {
				affecteIdentite[cle] = s
			} else if v != nil {
				affecteIdentite[cle] = v
			}
		}
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	defer tx.Rollback(ctx)

	// Colonnes dédiées.
	sets := []string{}
	args := []interface{}{}
	if nom != "" {
		sets = append(sets, "name = $"+itoa(len(args)+1))
		args = append(args, nom)
	}
	if legalName != nil {
		sets = append(sets, "legal_name = $"+itoa(len(args)+1))
		args = append(args, *legalName)
	}
	if billingAddress != nil {
		sets = append(sets, "billing_address = $"+itoa(len(args)+1))
		args = append(args, *billingAddress)
	}
	if len(sets) > 0 {
		args = append(args, orgID)
		if _, err := tx.Exec(ctx, `UPDATE organizations SET `+strings.Join(sets, ", ")+`, updated_at = now() WHERE id = $`+itoa(len(args)), args...); err != nil {
			ecrireInterne(w, err)
			return
		}
	}

	// Bloc identité dans settings (lecture-modification-écriture).
	var settingsBody []byte
	if err := tx.QueryRow(ctx, `SELECT settings FROM organizations WHERE id = $1 FOR UPDATE`, orgID).Scan(&settingsBody); err != nil {
		ecrireInterne(w, err)
		return
	}
	reglages := decoderObjet(settingsBody)
	bloc, _ := reglages["organization_identity"].(map[string]interface{})
	if bloc == nil {
		bloc = map[string]interface{}{}
	}
	for cle, valeur := range affecteIdentite {
		bloc[cle] = valeur
	}
	if nom != "" {
		bloc["trade_name"] = nom
	}
	reglages["organization_identity"] = bloc
	reglagesJSON, _ := json.Marshal(reglages)
	if _, err := tx.Exec(ctx, `UPDATE organizations SET settings = $1, updated_at = now() WHERE id = $2`, reglagesJSON, orgID); err != nil {
		ecrireInterne(w, err)
		return
	}

	// Mentions administratives : registry = { rccm: '…', niu: '…' }.
	if brut, ok := corps["registry"]; ok {
		var mentions map[string]string
		if err := json.Unmarshal(brut, &mentions); err == nil {
			for code, valeur := range mentions {
				// Le code niu du front correspond à tax_id dans l'objet organization.
				_ = code
				_ = valeur
			}
			for code, valeur := range mentions {
				statut := "provided"
				if strings.TrimSpace(valeur) == "" {
					statut = "empty"
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO registry_fields (organization_id, field_code, value, status)
					VALUES ($1, $2, $3, $4)
					ON CONFLICT (organization_id, field_code)
					DO UPDATE SET value = EXCLUDED.value, status = EXCLUDED.status, verified_at = NULL`, orgID, code, valeur, statut); err != nil {
					ecrireInterne(w, err)
					return
				}
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		ecrireInterne(w, err)
		return
	}
	a.journaliser(ctx, &orgID, nil, &sess.UserID, "organization.update", nil)
	ecrireJSON(w, http.StatusOK, map[string]interface{}{"message": "Fiche d'entreprise enregistrée."})
}

func itoa(n int) string {
	return fmt_Sprint(n)
}

func fmt_Sprint(n int) string {
	return strconv.Itoa(n)
}
