/**
 * Intégration Clerk côté frontend.
 *
 * - ClerkProvider est posé une seule fois dans main.tsx ;
 * - useClerkAuth fait le pont vers /api/v1/auth/clerk : dès qu'une session
 *   Clerk existe, le jeton est envoyé au backend qui ouvre une session
 *   applicative (cookie) et synchronise l'utilisateur dans Neon ;
 * - clerkSessionToken() fournit le jeton courant au client API (header
 *   « Authorization: Bearer » en complément du cookie).
 */
import { ClerkProvider, useSession, useClerk } from '@clerk/clerk-react'
import { useEffect, useRef, type ReactNode } from 'react'
import { api } from '@/lib/api'

const PUBLISHABLE_KEY = import.meta.env.VITE_CLERK_PUBLISHABLE_KEY
  ?? import.meta.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY
  ?? (typeof window !== 'undefined'
    ? (document.querySelector('meta[name="clerk-publishable-key"]')?.getAttribute('content') ?? '')
    : '')

export const clerkEnabled = Boolean(PUBLISHABLE_KEY)

/** Jeton de session Clerk courant (chaîne vide si absent). */
let currentSessionToken = ''
export function clerkSessionToken(): string {
  return currentSessionToken
}

/** Enveloppe l'application : ClerkProvider si configuré, sinon passthrough. */
export function AuthProvider({ children }: { children: ReactNode }) {
  if (!clerkEnabled) return <>{children}</>
  return (
    <ClerkProvider
      publishableKey={PUBLISHABLE_KEY}
      signInUrl="/connexion"
      signInFallbackRedirectUrl="/tableau-de-bord"
      signUpFallbackRedirectUrl="/tableau-de-bord"
    >
      <ClerkSessionBridge>{children}</ClerkSessionBridge>
    </ClerkProvider>
  )
}

/**
 * ClerkSessionBridge : surveille la session Clerk et ouvre la session
 * applicative dès qu'elle est prête (POST /auth/clerk avec le jeton).
 */
export function ClerkSessionBridge({ children }: { children: ReactNode }) {
  const { session } = useSession()
  const clerk = useClerk()
  const exchanged = useRef('')

  useEffect(() => {
    void session?.getToken().then((token) => {
      if (!token) return
      currentSessionToken = token
      if (exchanged.current === session?.id) return
      exchanged.current = session?.id ?? ''
      void api
        .post('/auth/clerk')
        .then(() => window.dispatchEvent(new Event('washguard:auth-clerk')))
        .catch(() => {
          // Compte Clerk sans employé associé dans Neon : la page affichera 403.
          exchanged.current = ''
        })
    })
  }, [session, clerk])

  return <>{children}</>
}
