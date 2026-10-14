package main

// Stub pour routes_stock.go - à implémenter dans le Lot 6
func (a *App) enregistrerRoutesStock(rt *routeur) {
	rt.proteger("GET", "/api/v1/stock", "stock:view", nonImplemente)
}