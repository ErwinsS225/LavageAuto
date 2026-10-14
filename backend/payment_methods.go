package main

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Moyens de paiement de la station.
//
// Une station ivoirienne n'accepte pas « mobile money » en général, mais Orange
// Money, MTN MoMo, Moov Money, Wave… Le référentiel est amorcé par le serveur,
// la station **active** ce qu'elle accepte, renomme à sa façon et ajoute ce qui
// lui manque. La **famille** (espèces, mobile money, carte, virement, différé,
// autre) décide du compteur de caisse alimenté — pas le libellé : renommer
// « Orange Money » en « OM » ne change aucun total.
//
// Désactiver ne supprime jamais : un encaissement passé cite son moyen par
// identifiant **et** par libellé recopié, donc il reste lisible même après un
// renommage ou une désactivation.
// ---------------------------------------------------------------------------

// modeleMoyenPaiement : une ligne du référentiel du produit.
type modeleMoyenPaiement struct {
	Code              string
	Nom               string
	Famille           string
	RequiertReference bool
	Ordre             int
}

// referentielMoyensPaiement : ce que WashGuard propose à chaque station.
// `especes` est actif d'emblée — une station qui lave des voitures encaisse des
// espèces le premier jour, et aucune autre valeur n'est activée sans que
// l'exploitant l'ait demandé.
var referentielMoyensPaiement = []modeleMoyenPaiement{
	{Code: "especes", Nom: "Espèces", Famille: "cash", Ordre: 10},
	{Code: "orange_money", Nom: "Orange Money", Famille: "mobile_money", RequiertReference: true, Ordre: 20},
	{Code: "mtn_momo", Nom: "MTN MoMo", Famille: "mobile_money", RequiertReference: true, Ordre: 30},
	{Code: "moov_money", Nom: "Moov Money", Famille: "mobile_money", RequiertReference: true, Ordre: 40},
	{Code: "wave", Nom: "Wave", Famille: "mobile_money", RequiertReference: true, Ordre: 50},
	{Code: "carte_bancaire", Nom: "Carte bancaire", Famille: "card", RequiertReference: true, Ordre: 60},
	{Code: "virement", Nom: "Virement bancaire", Famille: "transfer", RequiertReference: true, Ordre: 70},
	{Code: "differe", Nom: "Paiement différé", Famille: "deferred", RequiertReference: true, Ordre: 80},
	{Code: "autre", Nom: "Autre", Famille: "other", RequiertReference: true, Ordre: 90},
}

// moyenPaiement : un moyen déclaré par la station, tel qu'il est renvoyé.
type moyenPaiement struct {
	ID                string `json:"id"`
	Code              string `json:"code"`
	Name              string `json:"name"`
	Family            string `json:"family"`
	FamilyLabel       string `json:"family_label"`
	RequiresReference bool   `json:"requires_reference"`
	IsActive          bool   `json:"is_active"`
	SortOrder         int    `json:"sort_order"`
	Kind              string `json:"kind"`
}

// moyenPaiementColumns : colonnes de la table, dans l'ordre imposé au scan.
const moyenPaiementColumns = `id, code, name, family, requires_reference, is_active, sort_order, kind`

func scanMoyenPaiement(row scannable) (moyenPaiement, error) {
	var m moyenPaiement
	err := row.Scan(&m.ID, &m.Code, &m.Name, &m.Family, &m.RequiresReference,
		&m.IsActive, &m.SortOrder, &m.Kind)
	m.FamilyLabel = libelleFamilleMoyen(m.Family)
	return m, err
}

// methodeCanonique : la valeur écrite dans `payments.method`. Elle porte la
// famille, jamais le libellé commercial.
func methodeCanonique(famille string) string {
	switch famille {
	case "cash":
		return "especes"
	case "mobile_money":
		return "mobile_money"
	case "card":
		return "carte"
	case "transfer":
		return "virement"
	case "deferred":
		return "differe"
	default:
		return "autre"
	}
}

// libelleFamilleMoyen : ce que l'écran affiche à côté du nom déclaré.
func libelleFamilleMoyen(famille string) string {
	switch famille {
	case "cash":
		return "Espèces"
	case "mobile_money":
		return "Mobile money"
	case "card":
		return "Carte bancaire"
	case "transfer":
		return "Virement"
	case "deferred":
		return "Paiement différé"
	default:
		return "Autre"
	}
}

// familleMoyenConnue : le référentiel borne les familles — une famille inventée
// ne saurait pas quel compteur de caisse alimenter.
func familleMoyenConnue(famille string) bool {
	switch famille {
	case "cash", "mobile_money", "card", "transfer", "deferred", "other":
		return true
	}
	return false
}

// nomAfficheMoyen : le libellé à montrer pour un encaissement. Le moyen déclaré
// prime ; à défaut on retombe sur la famille canonique.
func nomAfficheMoyen(p *Payment) string {
	if strings.TrimSpace(p.PaymentMethodName) != "" {
		return p.PaymentMethodName
	}
	return methodLabelFR(p.Method)
}

// equiperMoyensPaiement amorce les moyens d'une station : chaque ligne manquante
// est créée, aucune n'est écrasée. Un renommage local ou une désactivation
// volontaire survit donc à un redémarrage — et `ON CONFLICT DO NOTHING` rend
// l'opération rejouable sans doublon.
func (a *App) equiperMoyensPaiement(ctx context.Context, orgID, stationID string) error {
	for _, modele := range referentielMoyensPaiement {
		if _, err := a.Pool.Exec(ctx, `
			INSERT INTO payment_methods (id, organization_id, station_id, code, name, family,
			  requires_reference, is_active, sort_order, kind)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'modele')
			ON CONFLICT (station_id, code) DO NOTHING`,
			newID("pme"), orgID, stationID, modele.Code, modele.Nom, modele.Famille,
			modele.RequiertReference, modele.Code == "especes", modele.Ordre); err != nil {
			return err
		}
	}
	return nil
}

// assurerMoyensPaiement couvre toutes les stations existantes (démarrage) : une
// base en service reçoit le référentiel sans intervention manuelle.
func (a *App) assurerMoyensPaiement(ctx context.Context) error {
	rows, err := a.Pool.Query(ctx, `SELECT id, organization_id FROM stations`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type station struct{ id, orgID string }
	stations := []station{}
	for rows.Next() {
		var s station
		if err := rows.Scan(&s.id, &s.orgID); err != nil {
			return err
		}
		stations = append(stations, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, s := range stations {
		if err := a.equiperMoyensPaiement(ctx, s.orgID, s.id); err != nil {
			return err
		}
	}
	return nil
}

// moyenPaiementDeLaStation charge un moyen en le bornant à l'entreprise **et** à
// la station : l'identifiant d'une autre entreprise est introuvable.
func (a *App) moyenPaiementDeLaStation(ctx context.Context, orgID, stationID, id string) (moyenPaiement, error) {
	return scanMoyenPaiement(a.Pool.QueryRow(ctx, `
		SELECT `+moyenPaiementColumns+` FROM payment_methods
		WHERE id = $1 AND organization_id = $2 AND station_id = $3`, id, orgID, stationID))
}

// moyenPaiementParCode : même chose, par code (l'écran envoie des codes stables
// pour les moyens du référentiel).
func (a *App) moyenPaiementParCode(ctx context.Context, orgID, stationID, code string) (moyenPaiement, error) {
	return scanMoyenPaiement(a.Pool.QueryRow(ctx, `
		SELECT `+moyenPaiementColumns+` FROM payment_methods
		WHERE code = $1 AND organization_id = $2 AND station_id = $3`, code, orgID, stationID))
}

// ---------------------------------------------------------------------------
// Résolution du moyen au moment d'encaisser.
// ---------------------------------------------------------------------------

// encaissementDemande : ce que la caisse envoie pour un règlement.
type encaissementDemande struct {
	MethodID  string
	Method    string // famille canonique, pour les clients qui n'ont pas le moyen
	Reference string
}

// resoudreMoyenPaiement traduit la demande d'encaissement en écriture.
//
// Trois refus explicites, parce qu'ils protègent la caisse :
//   - un moyen désactivé (`409 payment_method_disabled`) : la station a décidé
//     de ne plus l'accepter, l'encaissement ne doit pas passer en silence ;
//   - une référence manquante sur un moyen qui l'exige (mobile money, virement,
//     différé) : sans elle, l'encaissement n'est pas vérifiable ;
//   - un paiement différé sans autorisation : reporter un paiement est une
//     décision de crédit, réservée à qui a la permission.
func (a *App) resoudreMoyenPaiement(ctx context.Context, user *User, stationID string, demande encaissementDemande,
) (methode string, methodID *string, methodName string, err error) {

	if demande.MethodID == "" {
		methode = orDefault(demande.Method, "especes")
		switch methode {
		case "especes", "mobile_money", "carte", "virement", "differe", "autre":
		default:
			return "", nil, "", &apiError{Status: http.StatusBadRequest, Code: "validation_error",
				Message: "Moyen de paiement inconnu.", Fields: map[string]string{"method": "Famille inconnue"}}
		}
		if methode == "differe" && !hasPermission(user, "payment:create_deferred") {
			return "", nil, "", errDiffereNonAutorise
		}
		return methode, nil, methodLabelFR(methode), nil
	}

	moyen, err := a.moyenPaiementDeLaStation(ctx, deref(user.OrganizationID), stationID, demande.MethodID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, "", &apiError{Status: http.StatusBadRequest, Code: "validation_error",
				Message: "Moyen de paiement introuvable pour cette station.",
				Fields:  map[string]string{"payment_method_id": "Moyen inconnu"}}
		}
		return "", nil, "", err
	}
	if !moyen.IsActive {
		return "", nil, "", &apiError{Status: http.StatusConflict, Code: "payment_method_disabled",
			Message: "« " + moyen.Name + " » est désactivé : activez-le ou encaissez par un autre moyen.",
			Fields:  map[string]string{"payment_method_id": "Moyen désactivé"}}
	}
	if moyen.RequiresReference && strings.TrimSpace(demande.Reference) == "" {
		return "", nil, "", &apiError{Status: http.StatusBadRequest, Code: "validation_error",
			Message: "« " + moyen.Name + " » exige une référence (numéro de transaction, bordereau…).",
			Fields:  map[string]string{"reference": "Référence obligatoire"}}
	}
	if moyen.Family == "deferred" && !hasPermission(user, "payment:create_deferred") {
		return "", nil, "", errDiffereNonAutorise
	}
	return methodeCanonique(moyen.Family), &moyen.ID, moyen.Name, nil
}

// errDiffereNonAutorise : reporter un paiement n'est pas encaisser.
var errDiffereNonAutorise = &apiError{
	Status:  http.StatusForbidden,
	Code:    "forbidden",
	Message: "Un paiement différé engage la station : il nécessite l'accord du gérant.",
	Fields:  map[string]string{"payment_method_id": "Autorisation requise"},
}

// ancienMethodPourMoyen : la famille canonique d'un moyen, ou la valeur fournie
// quand aucun moyen n'est cité (encaissements libres).
func ancienMethodPourMoyen(moyen moyenPaiement, methode string) string {
	if moyen.ID == "" {
		return orDefault(methode, "especes")
	}
	return methodeCanonique(moyen.Family)
}

// ---------------------------------------------------------------------------
// Routes.
// ---------------------------------------------------------------------------

// GET /payment-methods?station_id=…[&actives=1]
func (a *App) handlePaymentMethodsList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if stationID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"items": []moyenPaiement{}, "total": 0, "actifs": 0})
		return
	}
	activesSeulement := r.URL.Query().Get("actives") == "1"
	rows, err := a.Pool.Query(r.Context(), `
		SELECT `+moyenPaiementColumns+` FROM payment_methods
		WHERE organization_id = $1 AND station_id = $2 AND (NOT $3 OR is_active)
		ORDER BY is_active DESC, sort_order, name`,
		deref(user.OrganizationID), stationID, activesSeulement)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	items := []moyenPaiement{}
	actifs := 0
	for rows.Next() {
		moyen, err := scanMoyenPaiement(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		if moyen.IsActive {
			actifs++
		}
		items = append(items, moyen)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": len(items), "actifs": actifs,
		"familles": []map[string]string{
			{"code": "cash", "label": libelleFamilleMoyen("cash")},
			{"code": "mobile_money", "label": libelleFamilleMoyen("mobile_money")},
			{"code": "card", "label": libelleFamilleMoyen("card")},
			{"code": "transfer", "label": libelleFamilleMoyen("transfer")},
			{"code": "deferred", "label": libelleFamilleMoyen("deferred")},
			{"code": "other", "label": libelleFamilleMoyen("other")},
		},
	})
}

// POST /payment-methods — un moyen propre à la station (code `perso_…`).
func (a *App) handlePaymentMethodCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	var body struct {
		StationID         string `json:"station_id"`
		Name              string `json:"name"`
		Family            string `json:"family"`
		RequiresReference *bool  `json:"requires_reference"`
		SortOrder         *int   `json:"sort_order"`
		IsActive          *bool  `json:"is_active"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	nom := strings.TrimSpace(body.Name)
	if nom == "" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Le nom du moyen de paiement est obligatoire.",
			map[string]string{"name": "Nom obligatoire"})
		return
	}
	if !familleMoyenConnue(body.Family) {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Famille de moyen de paiement inconnue.",
			map[string]string{"family": "Famille inconnue"})
		return
	}
	stationID, err := a.resolveStation(ctx, user, body.StationID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	// Une famille qui exige une référence l'exige par défaut : c'est le cas de
	// tous les paiements à distance. L'exploitant peut le décocher s'il sait
	// pourquoi.
	requiert := body.Family != "cash" && body.Family != "other"
	if body.RequiresReference != nil {
		requiert = *body.RequiresReference
	}
	actif := false
	if body.IsActive != nil {
		actif = *body.IsActive
	}
	ordre := 100
	if body.SortOrder != nil {
		ordre = *body.SortOrder
	}
	id := newID("pm")
	code := "perso_" + slugify(nom)
	tag, err := a.Pool.Exec(ctx, `
		INSERT INTO payment_methods (id, organization_id, station_id, code, name, family,
		  requires_reference, is_active, sort_order, kind, created_by_name)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'personnalisee',$10)
		ON CONFLICT (station_id, code) DO NOTHING`,
		id, deref(user.OrganizationID), stationID, code, nom, body.Family,
		requiert, actif, ordre, user.FullName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	// Aucune ligne écrite : le code est déjà pris — deux moyens de même nom
	// dans une même caisse seraient indiscernables à l'encaissement.
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "conflict",
			"Un moyen de paiement porte déjà ce nom pour cette station.",
			map[string]string{"name": "Nom déjà utilisé"})
		return
	}
	moyen, err := a.moyenPaiementParCode(ctx, deref(user.OrganizationID), stationID, code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(ctx, deref(user.OrganizationID), stationID, user.FullName,
		"payment_method.created", "payment_method", moyen.ID,
		map[string]any{"name": nom, "family": body.Family, "actor_role": user.Role})
	writeJSON(w, http.StatusCreated, moyen)
}

// PATCH /payment-methods/{id} — renommer, activer, réordonner.
func (a *App) handlePaymentMethodUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")
	orgID := deref(user.OrganizationID)

	var body map[string]any
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	stationID, err := a.stationDeTravail(ctx, user, toText(body["station_id"]))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if stationID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Station requise.", nil)
		return
	}
	moyen, err := a.moyenPaiementDeLaStation(ctx, orgID, stationID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Moyen de paiement introuvable.", nil)
		return
	}

	if nom, ok := body["name"].(string); ok {
		if strings.TrimSpace(nom) == "" {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Le nom du moyen de paiement ne peut pas être vide.",
				map[string]string{"name": "Nom obligatoire"})
			return
		}
		if _, err := a.Pool.Exec(ctx, `UPDATE payment_methods SET name = $1, updated_at = now() WHERE id = $2`,
			strings.TrimSpace(nom), id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
	}
	if reference, ok := body["requires_reference"].(bool); ok {
		if _, err := a.Pool.Exec(ctx, `UPDATE payment_methods SET requires_reference = $1, updated_at = now() WHERE id = $2`,
			reference, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
	}
	if ordre, ok := body["sort_order"].(float64); ok {
		if _, err := a.Pool.Exec(ctx, `UPDATE payment_methods SET sort_order = $1, updated_at = now() WHERE id = $2`,
			int(ordre), id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
	}
	if actif, ok := body["is_active"].(bool); ok {
		if !actif {
			// Garde-fou : sans aucun moyen actif, la caisse ne peut plus rien
			// encaisser. On refuse le dernier — en le disant, plutôt qu'en
			// laissant l'exploitation se bloquer d'un clic.
			var actifs int
			if err := a.Pool.QueryRow(ctx, `
				SELECT count(*) FROM payment_methods
				WHERE station_id = $1 AND is_active AND id <> $2`, stationID, id).Scan(&actifs); err != nil {
				writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
				return
			}
			if actifs == 0 {
				writeError(w, http.StatusConflict, "conflict",
					"Impossible de désactiver le dernier moyen de paiement actif : activez-en un autre d'abord.",
					map[string]string{"is_active": "Dernier moyen actif"})
				return
			}
		}
		if _, err := a.Pool.Exec(ctx, `UPDATE payment_methods SET is_active = $1, updated_at = now() WHERE id = $2`,
			actif, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
	}

	a.addAudit(ctx, orgID, stationID, user.FullName,
		"payment_method.updated", "payment_method", id,
		map[string]any{"name": moyen.Name, "actor_role": user.Role})

	relu, err := a.moyenPaiementDeLaStation(ctx, orgID, stationID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusOK, relu)
}

// DELETE /payment-methods/{id} — désactivation, jamais de suppression.
func (a *App) handlePaymentMethodDisable(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")
	orgID := deref(user.OrganizationID)

	stationID, err := a.stationDeTravail(ctx, user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if stationID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Station requise.", nil)
		return
	}
	moyen, err := a.moyenPaiementDeLaStation(ctx, orgID, stationID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Moyen de paiement introuvable.", nil)
		return
	}
	var actifs int
	if err := a.Pool.QueryRow(ctx, `
		SELECT count(*) FROM payment_methods WHERE station_id = $1 AND is_active AND id <> $2`,
		stationID, id).Scan(&actifs); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if actifs == 0 {
		writeError(w, http.StatusConflict, "conflict",
			"Impossible de désactiver le dernier moyen de paiement actif : activez-en un autre d'abord.", nil)
		return
	}
	if _, err := a.Pool.Exec(ctx,
		`UPDATE payment_methods SET is_active = false, updated_at = now() WHERE id = $1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(ctx, orgID, stationID, user.FullName,
		"payment_method.disabled", "payment_method", id,
		map[string]any{"name": moyen.Name, "actor_role": user.Role})
	writeJSON(w, http.StatusOK, map[string]any{"message": "Moyen de paiement désactivé."})
}
