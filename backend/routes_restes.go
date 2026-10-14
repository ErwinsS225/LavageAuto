package main

import (
	"net/http"
)

// routes_restes.go — enregistrement des modules livrés dans les lots suivants.
// Chaque lot remplace son enregistrement minimal par ses vrais handlers ; le
// serveur démarre et répond 404 proprement en attendant (jamais 500).

func nonImplemente(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	repondreErreur(w, http.StatusNotImplemented, "not_implemented",
		"Cette partie de l'API est en cours de livraison : le module sera disponible dans un prochain lot.")
}

func (a *App) enregistrerRoutesOrganisation(rt *routeur) {
	// La fiche d'entreprise est lue avec settings:view, modifiée par les
	// profils qui gèrent (settings:manage) — voir routes_org.go.
	rt.proteger("GET", "/api/v1/organizations/current", "settings:view", a.lireOrganisationDetaillee)
	rt.proteger("PATCH", "/api/v1/organizations/current", "settings:manage", a.modifierOrganisation)
}

func (a *App) enregistrerRoutesReferentiels(rt *routeur) {
	// Référentiels véhicules et usages (Lot 1). Les PUT en bloc sur la station
	// sont déjà routés par dispatchStation (routes_stations.go).
	rt.proteger("GET", "/api/v1/vehicle-catalogue", "station:view", a.catalogueVehicules)
	rt.proteger("GET", "/api/v1/vehicle-categories", "station:view", a.listerCategories)
	rt.proteger("GET", "/api/v1/vehicle-usages", "station:view", a.listerUsages)
	rt.proteger("GET", "/api/v1/vehicle-category-options", "wash:create", a.optionsVehicules)
	rt.proteger("GET", "/api/v1/vehicle-usage-categories", "station:view", a.listerCouples)
	rt.proteger("POST", "/api/v1/vehicle-categories", "settings:manage", a.creerCategorie)
	rt.proteger("PATCH", "/api/v1/vehicle-categories/", "settings:manage", a.modifierCategorie)
}

func (a *App) enregistrerRoutesCatalogue(rt *routeur) {
	// Catalogue prestations et options (Lot 2)
	// Référentiels de templates et familles
	rt.proteger("GET", "/api/v1/service-templates", "station:view", a.listerTemplatesServices)
	rt.proteger("GET", "/api/v1/service-families", "station:view", a.listerFamillesServices)
	rt.proteger("PUT", "/api/v1/stations/", "settings:manage", a.mettreAJourFamillesStation) // /stations/{id}/service-families

	// CRUD Services
	rt.proteger("GET", "/api/v1/services", "service:view", a.listerServices)
	rt.proteger("GET", "/api/v1/services/", "service:view", a.lireService)
	rt.proteger("POST", "/api/v1/services", "service:manage", a.creerService)
	rt.proteger("PATCH", "/api/v1/services/", "service:manage", a.modifierService)
	rt.proteger("DELETE", "/api/v1/services/", "service:manage", a.supprimerService)

	// Compatibilités des services
	rt.proteger("GET", "/api/v1/services/", "service:view", a.lireCompatibilitesService) // /services/{id}/compatibilities
	rt.proteger("PUT", "/api/v1/services/", "service:manage", a.mettreAJourCompatibilitesService)

	// CRUD Options
	rt.proteger("GET", "/api/v1/service-options", "service:view", a.listerOptions)
	rt.proteger("POST", "/api/v1/service-options", "service:manage", a.creerOption)
	rt.proteger("PATCH", "/api/v1/service-options/", "service:manage", a.modifierOption)
	rt.proteger("DELETE", "/api/v1/service-options/", "service:manage", a.supprimerOption)
}

func (a *App) enregistrerRoutesTarifs(rt *routeur) {
	// Grille tarifaire et moteur de prix (Lot 3).
	//
	// Le routeur prend la PREMIÈRE route dont le préfixe correspond : les
	// chemins exacts (`/price-rules/matrix`) doivent être déclarés AVANT le
	// sous-arbre (`/price-rules/`), sinon « matrix » tomberait dans la lecture
	// d'une règle et répondrait « règle introuvable ».
	rt.proteger("GET", "/api/v1/price-rules/matrix", "service:view", a.lireMatricePrix)
	rt.proteger("PUT", "/api/v1/price-rules/matrix", "service:manage", a.mettreAJourMatricePrix)

	rt.proteger("GET", "/api/v1/price-rules", "service:view", a.listerPriceRules)
	rt.proteger("GET", "/api/v1/price-rules/", "service:view", a.lirePriceRule)
	rt.proteger("POST", "/api/v1/price-rules", "service:manage", a.creerPriceRule)
	rt.proteger("PATCH", "/api/v1/price-rules/", "service:manage", a.modifierPriceRule)
	rt.proteger("DELETE", "/api/v1/price-rules/", "service:manage", a.supprimerPriceRule)

	// Aperçu de prix pour création de lavage
	rt.proteger("GET", "/api/v1/price-preview", "wash:create", a.previewPrix)
}

func (a *App) enregistrerRoutesEquipe(rt *routeur) {
	// Équipe et invitations (Lot 4) — contrat docs/api.md.
	//
	// Le routeur prend la première route qui matche : les chemins exacts
	// sont déclarés avant les sous-arbres (`/employees/options` avant
	// `/employees/`), sinon la liste courte tomberait dans l'activité.
	rt.proteger("GET", "/api/v1/employees/options", "employee:view", a.listerOptionsEmployes)
	rt.proteger("GET", "/api/v1/employees", "employee:view", a.listerEmployes)
	rt.proteger("POST", "/api/v1/employees", "employee:manage", a.creerEmploye)
	rt.proteger("GET", "/api/v1/employees/", "employee:view", a.lireActiviteEmploye)               // …/{id}/activity
	rt.proteger("PATCH", "/api/v1/employees/", "employee:manage", a.modifierEmploye)               // …/{id}
	rt.proteger("POST", "/api/v1/employees/", "employee:manage", a.reinitialiserMotDePasseEmploye) // …/{id}/reset-password

	rt.proteger("GET", "/api/v1/invitations", "employee:view", a.listerInvitations)
	rt.proteger("POST", "/api/v1/invitations", "employee:manage", a.creerInvitation)
	// L'aperçu est lu sans session par la personne invitée
	// (page « Rejoindre une entreprise ») : route publique explicite.
	rt.routes = append(rt.routes,
		route{methode: "GET", prefixe: "/api/v1/invitations/", handler: a.apercuInvitation, public: true})
	rt.proteger("POST", "/api/v1/invitations/", "employee:manage", a.revoquerInvitation) // …/{id}/revoke
}

// enregistrerRoutesRegles / enregistrerRapports : Lots 6-7 en cours —
// voir ProblemesBackend.md.

func (a *App) enregistrerRoutesRegles(rt *routeur) {}

func (a *App) enregistrerRapports(rt *routeur) {}
