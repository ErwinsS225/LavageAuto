/**
 * Client API WashGuard.
 *
 * Toutes les requêtes partent vers /api/v1 avec le cookie de session
 * applicative (credentials: 'include'). Quand Clerk est configuré, le jeton
 * de session Clerk est aussi envoyé en « Authorization: Bearer » : le backend
 * l'accepte comme alternative (voir backend/auth.go).
 *
 * En développement, le jeton de session renvoyé par l'API est aussi présenté en
 * « Authorization: Bearer » : un cookie tiers n'est pas conservé par le
 * navigateur quand l'application est affichée dans un cadre d'un autre site.
 * Le paquet de production ne contient pas ce mécanisme (voir
 * src/lib/session-preview.ts et backend/apercu_session.go).
 */
import { clerkSessionToken } from '@/lib/clerk'
import { jetonApercu, oublierJetonApercu, retenirJetonApercu } from '@/lib/session-preview'
import { signalerSessionExpiree } from '@/lib/session-signal'

export class ApiError extends Error {
  status: number
  code: string
  fields: Record<string, string>

  constructor(status: number, code: string, message: string, fields?: Record<string, string>) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.fields = fields ?? {}
  }
}

const BASE = '/api/v1'

/** Routes où un 401 veut dire « identifiants refusés », pas « session finie ». */
const EST_POINT_ENTREE_AUTH = /^\/auth\/(login|register|clerk)/

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  const token = clerkSessionToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
  // Hors production seulement, et seulement si Clerk n'a pas déjà fourni un
  // jeton : celui de Clerk reste prioritaire quand il est configuré.
  if (import.meta.env.DEV && !token) {
    const apercu = jetonApercu()
    if (apercu) headers['Authorization'] = `Bearer ${apercu}`
  }
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  const response = await fetch(`${BASE}${path}`, {
    method,
    headers,
    credentials: 'include',
    body: body === undefined ? undefined : JSON.stringify(body),
  })

  const text = await response.text()
  let data: unknown
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = null
  }

  // Hors production, la réponse d'authentification porte le jeton de session à
  // côté du cookie : on le garde pour l'onglet (voir src/lib/session-preview.ts).
  if (response.ok) retenirJetonApercu(data)
  if (path === '/auth/logout' && response.ok) oublierJetonApercu()

  // Session refusée par le serveur : l'application ne peut plus rien faire de
  // cette session. On prévient qui de droit (src/hooks/useAuth.ts) au lieu de
  // laisser chaque écran afficher « Vous devez vous connecter » sans issue.
  // Les points d'entrée d'authentification sont exclus : un mot de passe erroné
  // n'est pas une session expirée.
  if (response.status === 401 && !EST_POINT_ENTREE_AUTH.test(path)) {
    oublierJetonApercu()
    signalerSessionExpiree()
  }

  if (!response.ok) {
    // Le serveur nomme les champs fautifs : `fields` (writeError), `details` sur
    // d'anciennes réponses. Les deux sont acceptés pour ne rien perdre.
    const err = (
      data as { error?: { code?: string; message?: string; fields?: Record<string, string>; details?: Record<string, string> } }
    )?.error
    throw new ApiError(
      response.status,
      err?.code ?? 'error',
      err?.message ?? `Erreur ${response.status}`,
      err?.fields ?? err?.details,
    )
  }
  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body ?? {}),
  // put remplace la ressource : les horaires d'une station sont enregistrés en
  // bloc (les sept jours), un PATCH champ par champ n'aurait pas de sens ici.
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body ?? {}),
  delete: <T>(path: string) => request<T>('DELETE', path),
}

/** Construit une chaîne de requête en ignorant les valeurs vides. */
export function query(params: Record<string, string | number | boolean | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') search.set(key, String(value))
  }
  const raw = search.toString()
  return raw ? `?${raw}` : ''
}
