package main

// Session d'aperçu — **hors production uniquement**.
//
// En aperçu, l'application est affichée dans un cadre d'un autre site, et
// parfois servie en `http`. Le cookie de session y est alors un cookie **tiers**
// (ou un cookie `Secure` sur une page non chiffrée) : le navigateur ne le
// conserve pas. Résultat observé : l'interface sait qu'une session vient de
// s'ouvrir, le serveur non, et l'écran de mise en route affiche « Vous devez
// vous connecter » deux secondes après l'inscription.
//
// Deux ajustements, tous deux fermés en production par `APP_ENV=production` :
//
//   - la réponse d'authentification (`/auth/login`, `/auth/register`,
//     `/auth/clerk`) porte aussi le jeton de session, en clair, à côté du
//     cookie. L'interface le garde pour l'onglet et le présente en
//     `Authorization: Bearer` — même mécanisme que les jetons Clerk, déjà
//     acceptés par `userFromRequest` ;
//   - ce jeton est accepté en en-tête à la place du cookie.
//
// La production ne change pas d'un iota : le jeton n'y est jamais renvoyé dans
// le corps, et un jeton de session présenté en en-tête y est refusé. Le cookie
// HttpOnly (`SameSite` réglable) reste l'unique voie d'authentification.

// apercuAutorise : le serveur accepte-t-il la session d'aperçu ?
// `false` en production, où l'aperçu n'a pas de sens.
func (a *App) apercuAutorise() bool {
	return a.Cfg.Env != "production"
}

// avecJetonApercu ajoute le jeton de session au corps d'authentification.
// Le cookie reste posé par ailleurs : les deux voies coexistent, et une
// installation servie sur une seule origine n'utilise que le cookie.
func (a *App) avecJetonApercu(payload map[string]any, token string) map[string]any {
	if !a.apercuAutorise() || token == "" {
		return payload
	}
	payload["session_token"] = token
	return payload
}
