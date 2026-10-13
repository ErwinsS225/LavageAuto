/**
 * Idempotence : clé de geste par requête mutation sensible.
 *
 * Un même geste utilisateur ne doit créer qu'une ressource, même si
 * l'appel part deux fois (réseau instable, double clic). La clé est stable
 * par geste et réinitialisée à chaque nouvelle intention.
 */

const VERSION = "v1";

/**
 * Clé stable pour un geste donné (ex : encaissement de la fiche X avec le
 * montant Y). Même entrée ⇒ même clé ⇒ le serveur dédoublonne.
 */
export function gestureKey(
  scope: string,
  ...parts: Array<string | number>
): string {
  return [VERSION, scope, ...parts.map((part) => String(part))].join(":");
}

/** Clé nouvelle à chaque appel : pour les créations non re-jouables. */
export function freshKey(scope: string): string {
  return `${VERSION}:${scope}:${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}
