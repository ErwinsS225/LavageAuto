/**
 * Signal de session expirée.
 *
 * Une seule question traitée ici : que faire quand le serveur refuse une
 * session que l'application croyait valable ? Sans signal, l'écran se contente
 * d'afficher « Vous devez vous connecter » sans issue — l'utilisateur reste
 * bloqué sur une page morte.
 *
 * Le client API (`src/lib/api.ts`) signale ; `useAuth` vide la session connue et
 * la garde de route ramène à la connexion, en disant pourquoi.
 */
const EVENEMENT = 'washguard:session-expiree'

let recente = false

/** Prévient l'application qu'un appel vient d'être refusé (401). */
export function signalerSessionExpiree(): void {
  recente = true
  window.dispatchEvent(new Event(EVENEMENT))
}

/** S'abonne au signal ; renvoie de quoi se désabonner. */
export function surSessionExpiree(rappel: () => void): () => void {
  window.addEventListener(EVENEMENT, rappel)
  return () => window.removeEventListener(EVENEMENT, rappel)
}

/**
 * Session expirée **en cours de navigation** ? La lecture consomme le signal :
 * c'est l'écran qui redirige qui pose la question, une seule fois.
 */
export function sessionVientDExpirer(): boolean {
  const valeur = recente
  recente = false
  return valeur
}
