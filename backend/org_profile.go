package main

import (
	"context"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Profil de l'entreprise : identité commerciale, facturation, mentions légales.
//
// Trois principes :
//
//  1. **Rien n'est obligatoire ici.** Un lavage peut ouvrir son compte et
//     travailler sans connaître son numéro de contribuable : ces informations
//     servent à la facturation et aux démarches, pas au lavage d'une voiture.
//     Le parcours les demande, il ne les exige pas.
//  2. **Renseigné n'est pas vérifié.** L'entreprise saisit son RCCM ; seule la
//     plateforme (support WashGuard) peut attester l'avoir contrôlé. L'état est
//     stocké séparément de la valeur (table `organization_registry_fields`) et
//     retombe à « renseigné » dès que la valeur change : une vérification ne
//     survit pas à une modification.
//  3. **Les écrans n'écrivent que ce qu'ils affichent.** Le PATCH accepte une
//     liste fermée de champs ; une colonne inconnue est refusée plutôt
//     qu'ignorée en silence.
// ---------------------------------------------------------------------------

// champsProfilOrganisation : libellé d'API → colonne, pour l'identité
// commerciale et la facturation. Ces champs n'ont pas d'état de vérification :
// ce sont des coordonnées, pas des pièces administratives.
var champsProfilOrganisation = map[string]string{
	"legal_name":           "legal_name",
	"trade_name":           "trade_name",
	"activity_type":        "activity_type",
	"activity_description": "activity_description",
	"activity_started_on":  "activity_started_on",
	"whatsapp":             "whatsapp",
	"website":              "website",
	"billing_name":         "billing_name",
	"billing_address":      "billing_address",
	"billing_email":        "billing_email",
	"billing_phone":        "billing_phone",
	"legal_mentions":       "legal_mentions",
}

// typesActivite : les activités proposées à la saisie. La liste n'est pas
// fermée : « autre » existe, et le champ reste du texte libre côté base.
var typesActivite = []string{
	"lavage manuel", "lavage automatique", "haute pression", "nettoyage intérieur",
	"detailing", "mixte", "autre",
}

// Erreurs dédiées : elles sont traduites en réponses HTTP par les handlers,
// sans jamais laisser deviner au client ce qui a échoué côté serveur.
var (
	errMentionInconnue = &apiError{
		Status:  http.StatusBadRequest,
		Code:    "validation_error",
		Message: "Cette mention administrative n'existe pas.",
	}
	errMentionSansValeur = &apiError{
		Status:  http.StatusConflict,
		Code:    "conflict",
		Message: "Il n'y a rien à vérifier : l'entreprise n'a pas renseigné cette mention.",
	}
)

// mentionRegistre décrit une information administrative suivie séparément.
type mentionRegistre struct {
	Code  string // code stable : sert de colonne ET de clé d'état
	Label string
	Help  string
}

// catalogueRegistre : ordre d'affichage et libellés. Ajouter une mention ici
// suffit : la migration a posé les colonnes, le suivi d'état est générique.
var catalogueRegistre = []mentionRegistre{
	{Code: "legal_form", Label: "Forme juridique", Help: "Entreprise individuelle, SARL, SAS, coopérative…"},
	{Code: "legal_representative", Label: "Représentant légal", Help: "Nom de la personne qui engage l'entreprise."},
	{Code: "rccm", Label: "RCCM", Help: "Numéro du registre du commerce et du crédit mobilier."},
	{Code: "tax_id", Label: "Compte contribuable", Help: "Identifiant fiscal délivré par la Direction générale des impôts."},
	{Code: "cnps_employer_number", Label: "Numéro employeur CNPS", Help: "Numéro d'immatriculation employeur à la CNPS."},
	{Code: "fiscal_regime", Label: "Régime fiscal", Help: "Régime d'imposition déclaré (réel, simplifié…)."},
}

// codesRegistre : l'ensemble des mentions connues, pour valider une saisie.
func codesRegistre() map[string]mentionRegistre {
	index := make(map[string]mentionRegistre, len(catalogueRegistre))
	for _, mention := range catalogueRegistre {
		index[mention.Code] = mention
	}
	return index
}

// libelleEtatRegistre : l'état d'une mention, dit en français.
func libelleEtatRegistre(statut string) string {
	switch statut {
	case "provided":
		return "Renseigné"
	case "pending_review":
		return "En attente de vérification"
	case "verified":
		return "Vérifié par WashGuard"
	default:
		return "Non renseigné"
	}
}

// lireRegistre rend l'état des mentions administratives de l'entreprise.
// Aucune ligne n'existe tant qu'une valeur n'a pas été saisie : on renvoie donc
// toujours une entrée par mention connue, à l'état « non renseigné ».
func (a *App) lireRegistre(ctx context.Context, orgID string) ([]map[string]any, error) {
	etats := map[string]map[string]any{}
	rows, err := a.Pool.Query(ctx, `
		SELECT field_code, value, status, verified_at
		FROM organization_registry_fields WHERE organization_id = $1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code, valeur, statut string
		var verifieLe *time.Time
		if err := rows.Scan(&code, &valeur, &statut, &verifieLe); err != nil {
			return nil, err
		}
		etats[code] = map[string]any{"value": valeur, "status": statut, "verified_at": verifieLe}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	lignes := make([]map[string]any, 0, len(catalogueRegistre))
	for _, mention := range catalogueRegistre {
		etat := etats[mention.Code]
		valeur, statut := "", "empty"
		var verifieLe any
		if etat != nil {
			valeur, _ = etat["value"].(string)
			statut, _ = etat["status"].(string)
			verifieLe = etat["verified_at"]
		}
		lignes = append(lignes, map[string]any{
			"field_code": mention.Code, "label": mention.Label, "help": mention.Help,
			"value": valeur, "status": statut, "status_label": libelleEtatRegistre(statut),
			"verified_at": verifieLe,
		})
	}
	return lignes, nil
}

// enregistrerRegistre écrit les mentions administratives transmises par
// l'entreprise. La valeur va dans la colonne correspondante, l'état dans
// `organization_registry_fields` — et toute vérification antérieure est
// annulée : le support a vérifié un numéro, pas le suivant.
func (a *App) enregistrerRegistre(ctx context.Context, orgID, auteur string, mentions map[string]any) error {
	connues := codesRegistre()
	for code, brut := range mentions {
		mention, ok := connues[code]
		if !ok {
			return errMentionInconnue
		}
		valeur := strings.TrimSpace(toText(brut))
		if _, err := a.Pool.Exec(ctx,
			`UPDATE organizations SET `+mention.Code+` = $1 WHERE id = $2`, valeur, orgID); err != nil {
			return err
		}
		statut := "empty"
		if valeur != "" {
			statut = "provided"
		}
		if _, err := a.Pool.Exec(ctx, `
			INSERT INTO organization_registry_fields
				(organization_id, field_code, value, status, verified_by, verified_at, updated_at)
			VALUES ($1,$2,$3,$4,NULL,NULL,now())
			ON CONFLICT (organization_id, field_code) DO UPDATE
			SET value = EXCLUDED.value, status = EXCLUDED.status,
			    verified_by = NULL, verified_at = NULL, updated_at = now()`,
			orgID, mention.Code, valeur, statut); err != nil {
			return err
		}
	}
	return nil
}

// verifierMention : réservé au support (platform:manage). C'est la seule voie
// qui mène à l'état « vérifié », et elle est journalisée.
func (a *App) verifierMention(ctx context.Context, orgID, fieldCode, verificateurID, verificateur string) error {
	if _, ok := codesRegistre()[fieldCode]; !ok {
		return errMentionInconnue
	}
	tag, err := a.Pool.Exec(ctx, `
		UPDATE organization_registry_fields
		SET status = 'verified', verified_by = $1, verified_at = now(), updated_at = now()
		WHERE organization_id = $2 AND field_code = $3 AND btrim(value) <> ''`,
		verificateurID, orgID, fieldCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errMentionSansValeur
	}
	a.addAudit(ctx, orgID, "", verificateur, "registry.verified", "organization", orgID,
		map[string]any{"field_code": fieldCode})
	return nil
}

// handlePlatformRegistryVerify : POST /platform/organizations/{id}/registry/{code}/verify
func (a *App) handlePlatformRegistryVerify(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	orgID := r.PathValue("id")
	code := r.PathValue("code")

	org, err := a.getOrganization(r.Context(), orgID)
	if err != nil || org == nil {
		writeError(w, http.StatusNotFound, "not_found", "Organisation introuvable.", nil)
		return
	}
	if err := a.verifierMention(r.Context(), orgID, code, user.ID, user.FullName); err != nil {
		switch err {
		case errMentionInconnue:
			writeError(w, http.StatusBadRequest, "validation_error",
				"Cette mention administrative n'existe pas.", map[string]string{"field_code": "Mention inconnue"})
		case errMentionSansValeur:
			writeError(w, http.StatusConflict, "conflict",
				"Il n'y a rien à vérifier : l'entreprise n'a pas renseigné cette mention.", nil)
		default:
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		}
		return
	}
	lignes, err := a.lireRegistre(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"registry": lignes, "message": "Mention vérifiée."})
}

// toText : une valeur JSON quelconque, ramenée à du texte. Les champs de
// profil sont tous des chaînes (ou des dates écrites « 2026-03-01 ») : une
// valeur d'un autre type est refusée plus haut, ici on la rend lisible.
func toText(valeur any) string {
	switch v := valeur.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}

// validerEmailFacultatif : un e-mail de facturation vide est permis, un e-mail
// mal formé ne l'est pas.
func validerEmailFacultatif(valeur string) bool {
	if strings.TrimSpace(valeur) == "" {
		return true
	}
	_, err := mail.ParseAddress(valeur)
	return err == nil
}

// validerDateFacultative : « 2026-03-01 » ou vide.
func validerDateFacultative(valeur string) bool {
	if strings.TrimSpace(valeur) == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", valeur)
	return err == nil
}
