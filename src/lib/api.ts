/**
 * Client API WashGuard.
 *
 * Toutes les requêtes partent vers /api/v1 avec le cookie de session
 * applicative (credentials: 'include'). Quand Clerk est configuré, le jeton
 * de session Clerk est aussi envoyé en « Authorization: Bearer » : le backend
 * l'accepte comme alternative (voir backend/auth.go).
 */
import { clerkSessionToken } from '@/lib/clerk'

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

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  const token = clerkSessionToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
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

  if (!response.ok) {
    const err = (data as { error?: { code?: string; message?: string; details?: Record<string, string> } })?.error
    throw new ApiError(
      response.status,
      err?.code ?? 'error',
      err?.message ?? `Erreur ${response.status}`,
      err?.details,
    )
  }
  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body ?? {}),
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
