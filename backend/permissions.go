package main

import (
	"context"
	"errors"
	"net/http"
)

// ---------------------------------------------------------------------------
// Autorisations : une seule porte d'entrée pour les permissions et le
// périmètre de station.
//
// La matrice de référence est rolePermissions (app.go). Le front s'en sert
// pour masquer les écrans ; le serveur s'en sert pour refuser réellement
// l'action. Les deux lisent la même liste, donc elles ne peuvent pas diverger.
// ---------------------------------------------------------------------------

// errStationForbidden : la station existe dans l'entreprise mais l'utilisateur
// est rattaché à une autre station.
var errStationForbidden = &apiError{
	Status:  http.StatusForbidden,
	Code:    "forbidden",
	Message: "Votre compte est rattaché à une autre station.",
}

// errStationNotFound : la station n'appartient pas à l'entreprise de
// l'utilisateur. On répond 404 pour ne pas révéler son existence.
var errStationNotFound = &apiError{
	Status:  http.StatusNotFound,
	Code:    "not_found",
	Message: "Station introuvable.",
}

// errNoStation : la station de travail n'a pas pu être déterminée.
var errNoStation = &apiError{
	Status:  http.StatusBadRequest,
	Code:    "validation_error",
	Message: "Station requise.",
}

// hasPermission indique si le rôle possède la permission demandée.
// Le superadmin (support WashGuard) possède tout.
func hasPermission(user *User, permission string) bool {
	if user == nil {
		return false
	}
	for _, p := range rolePermissions[user.Role] {
		if p == permission {
			return true
		}
	}
	return false
}

// requirePermission protège une route : la session doit porter la permission.
func (a *App) requirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		if !hasPermission(user, permission) {
			writeError(w, http.StatusForbidden, "forbidden",
				"Votre profil n'est pas autorisé à effectuer cette action.", nil)
			return
		}
		next(w, r)
	})
}

// isStationScoped indique si le rôle ne voit que sa propre station.
// Le propriétaire, lui, garde l'entreprise entière (nécessaire pour ouvrir une
// seconde station) ; un gérant rattaché à une station y est limité.
func isStationScoped(user *User) bool {
	switch user.Role {
	case "cashier", "washer":
		return true
	case "manager":
		return deref(user.StationID) != ""
	default:
		return false
	}
}

// stationAutorisee vérifie qu'une station appartient bien à l'entreprise.
func (a *App) stationAutorisee(ctx context.Context, orgID, stationID string) error {
	if stationID == "" {
		return errNoStation
	}
	var owner string
	err := a.Pool.QueryRow(ctx, `SELECT organization_id FROM stations WHERE id = $1`, stationID).Scan(&owner)
	if err != nil || owner != orgID {
		return errStationNotFound
	}
	return nil
}

// resolveStation détermine la station de travail d'une requête :
//   - la station demandée (corps ou ?station_id) si elle est autorisée ;
//   - celle de l'utilisateur sinon ;
//   - la station par défaut de l'entreprise pour un propriétaire qui n'en
//     précise aucune.
//
// Elle refuse une station hors entreprise (404) ou hors périmètre (403).
func (a *App) resolveStation(ctx context.Context, user *User, requested string) (string, error) {
	orgID := deref(user.OrganizationID)
	forced := deref(user.StationID)

	if isStationScoped(user) {
		if requested != "" && requested != forced {
			return "", errStationForbidden
		}
		if forced == "" {
			return "", errNoStation
		}
		if err := a.stationAutorisee(ctx, orgID, forced); err != nil {
			return "", err
		}
		return forced, nil
	}

	if requested == "" {
		return "", errNoStation
	}
	if err := a.stationAutorisee(ctx, orgID, requested); err != nil {
		return "", err
	}
	return requested, nil
}

// stationDeTravail est la variante tolérante utilisée en lecture : si aucune
// station n'est précisée, on retombe sur celle du compte puis sur la station
// par défaut de l'entreprise. Elle renvoie une chaîne vide quand aucune
// station n'existe encore.
func (a *App) stationDeTravail(ctx context.Context, user *User, requested string) (string, error) {
	orgID := deref(user.OrganizationID)
	forced := deref(user.StationID)

	if isStationScoped(user) {
		if forced == "" {
			return "", nil
		}
		return forced, nil
	}

	if requested != "" {
		if err := a.stationAutorisee(ctx, orgID, requested); err != nil {
			return "", err
		}
		return requested, nil
	}
	if forced != "" {
		return forced, nil
	}
	id, err := a.defaultStationID(ctx, orgID)
	if err != nil {
		return "", nil // entreprise sans station : la lecture reste possible
	}
	return id, nil
}

// stationFilter exprime le périmètre de station sous forme de prédicat SQL
// réutilisable : ("", aucune station) filtre toute l'entreprise, une valeur
// filtre cette station. Renvoie une erreur si la station est refusée.
func (a *App) stationFilter(ctx context.Context, user *User, requested string) (string, error) {
	if requested == "" && !isStationScoped(user) {
		return "", nil // propriétaire/gérant sans station : toute l'entreprise
	}
	station, err := a.stationDeTravail(ctx, user, requested)
	if err != nil {
		return "", err
	}
	if station == "" {
		return "", errNoStation
	}
	return station, nil
}

// requireOrgUser place l'identifiant d'entreprise dans le contexte des
// handlers : filtre obligatoire sur toutes les requêtes SQL.
func orgIDOf(r *http.Request) string {
	return deref(currentUser(r).OrganizationID)
}

// writeAPIError sérialise une apiError posée plus haut (ou un 500 neutre).
func writeAPIError(w http.ResponseWriter, err error) {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		writeError(w, apiErr.Status, apiErr.Code, apiErr.Message, apiErr.Fields)
		return
	}
	writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
}
