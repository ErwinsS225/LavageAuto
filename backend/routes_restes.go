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

func (a *App) enregistrerRoutesOrganisation(rt *routeur) {}

func (a *App) enregistrerRoutesReferentiels(rt *routeur) {}

func (a *App) enregistrerRoutesCatalogue(rt *routeur) {}

func (a *App) enregistrerRoutesEquipe(rt *routeur) {}

func (a *App) enregistrerRoutesLavages(rt *routeur) {}

func (a *App) enregistrerRoutesCaisse(rt *routeur) {}

func (a *App) enregistrerRoutesStock(rt *routeur) {}

func (a *App) enregistrerRoutesRegles(rt *routeur) {}

func (a *App) enregistrerRapports(rt *routeur) {}
