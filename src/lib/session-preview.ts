/**
 * Session d'aperçu — **développement et aperçu uniquement**.
 *
 * En aperçu, l'application est affichée dans un cadre d'un autre site, et
 * parfois servie en `http` : le cookie de session y est un cookie *tiers* (ou
 * un cookie `Secure` sur une page non chiffrée), que le navigateur refuse de
 * conserver. L'API répond alors `401` à toutes les requêtes, alors que
 * l'interface, elle, sait qu'une session vient de s'ouvrir — d'où l'écran
 * « Vous devez vous connecter » deux secondes après l'inscription.
 *
 * Le serveur règle cela **hors production** (`backend/apercu_session.go`) en
 * joignant le jeton de session au corps de la réponse d'authentification. Ce
 * module le garde le temps de l'onglet et le client API le présente en
 * `Authorization: Bearer` — même mécanisme que les jetons Clerk, déjà acceptés
 * par le serveur.
 *
 * Hors développement, rien de tout cela n'existe : `import.meta.env.DEV` vaut
 * `false` à la compilation, les branches deviennent mortes, et la session reste
 * portée par le seul cookie HttpOnly.
 */
const CLE = 'washguard.apercu.session'

/** Jeton conservé pour l'onglet courant (chaîne vide s'il n'y en a pas). */
export function jetonApercu(): string {
  if (!import.meta.env.DEV) return ''
  try {
    return window.sessionStorage.getItem(CLE) ?? ''
  } catch {
    // Navigation privée ou stockage refusé : on retombe sur le cookie.
    return ''
  }
}

/** Retient le jeton que la réponse d'authentification vient de donner. */
export function retenirJetonApercu(reponse: unknown): void {
  if (!import.meta.env.DEV) return
  const jeton = (reponse as { session_token?: unknown } | null)?.session_token
  if (typeof jeton !== 'string' || jeton === '') return
  try {
    window.sessionStorage.setItem(CLE, jeton)
  } catch {
    // Sans stockage, la session reste portée par le cookie seul.
  }
}

/** Oublie le jeton (déconnexion, ou session refusée par le serveur). */
export function oublierJetonApercu(): void {
  if (!import.meta.env.DEV) return
  try {
    window.sessionStorage.removeItem(CLE)
  } catch {
    // Rien à oublier.
  }
}
