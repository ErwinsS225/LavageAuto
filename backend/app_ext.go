package main

import "sync"

// AppExt complète App sans toucher à main.go : état partagé du serveur.
type AppExt struct {
	Mu           sync.Mutex
	JetonsApercu map[string]bool
	Verrous      *verrousOrg
}

func (a *App) ext() *AppExt { return appGlobale }

var appGlobale = &AppExt{JetonsApercu: map[string]bool{}, Verrous: nouvellesVerrous()}
