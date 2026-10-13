/**
 * Formulaire de connexion (style fidèle à la maquette).
 *
 * Authentification :
 * - e-mail + mot de passe → POST /api/v1/auth/login (cookie de session) ;
 * - Google / Facebook → OAuth Clerk (authenticateWithRedirect), si Clerk est
 *   configuré ; le pont ClerkSessionBridge ouvre ensuite la session applicative.
 */
import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Eye, EyeOff } from 'lucide-react'
import { useClerk } from '@clerk/clerk-react'

/** Logos sociaux en SVG inline (lucide ne fournit pas Google/Facebook). */
function GoogleIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true">
      <path fill="#4285F4" d="M23.5 12.3c0-.8-.1-1.6-.2-2.3H12v4.5h6.5a5.6 5.6 0 0 1-2.4 3.7v3h3.9c2.3-2.1 3.5-5.2 3.5-8.9Z" />
      <path fill="#34A853" d="M12 24c3.2 0 6-1.1 8-2.9l-3.9-3a7.2 7.2 0 0 1-10.7-3.8H1.3v3.1A12 12 0 0 0 12 24Z" />
      <path fill="#FBBC05" d="M5.4 14.3a7.2 7.2 0 0 1 0-4.6V6.6H1.3a12 12 0 0 0 0 10.8l4.1-3.1Z" />
      <path fill="#EA4335" d="M12 4.7c1.8 0 3.4.6 4.6 1.8l3.4-3.4A12 12 0 0 0 1.3 6.6l4.1 3.1A7.2 7.2 0 0 1 12 4.7Z" />
    </svg>
  )
}
function FacebookIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true">
      <path fill="#1877F2" d="M24 12a12 12 0 1 0-13.9 11.9v-8.4H7.1V12h3V9.4c0-3 1.8-4.7 4.5-4.7 1.3 0 2.7.2 2.7.2v3h-1.5c-1.5 0-2 .9-2 1.9V12h3.3l-.5 3.5h-2.8v8.4A12 12 0 0 0 24 12Z" />
    </svg>
  )
}

import { Button, Field, Input } from '@/components/ui'
import { useAuth } from '@/hooks/useAuth'
import { ApiError } from '@/lib/api'
import { useToast } from '@/components/Toaster'
import { usePageTitle } from '@/hooks/usePageTitle'
import { clerkEnabled } from '@/lib/clerk'

export default function LoginForm() {
  usePageTitle('Connexion')
  const { login } = useAuth()
  const navigate = useNavigate()
  const toast = useToast()
  const clerk = useClerk()
  const [socialLoading, setSocialLoading] = useState<'google' | 'facebook' | null>(null)

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [remember, setRemember] = useState(false)
  const [error, setError] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(false)

  async function onSubmit(event: FormEvent) {
    event.preventDefault()
    setError('')
    setFields({})
    setLoading(true)
    try {
      const me = await login({ email, password })
      toast.success(`Bienvenue ${me.user.full_name.split(' ')[0]} !`)
      const target =
        me.user.role === 'superadmin'
          ? '/administration'
          : me.user.role === 'washer'
            ? '/espace-laveur'
            : '/tableau-de-bord'
      navigate(target, { replace: true })
    } catch (caught) {
      if (caught instanceof ApiError) {
        setError(caught.message)
        setFields(caught.fields)
      } else {
        setError("La connexion a échoué. Vérifiez votre réseau puis réessayez.")
      }
    } finally {
      setLoading(false)
    }
  }

  /**
   * Connexion sociale : redirection vers le fournisseur via Clerk, retour sur
   * /connexion où le pont Clerk → /auth/clerk ouvre la session applicative.
   * Sans Clerk configuré, on l'explique franchement plutôt que de simuler.
   */
  async function signInSocial(strategy: 'oauth_google' | 'oauth_facebook') {
    if (!clerkEnabled) {
      toast.info('La connexion sociale sera disponible prochainement.')
      return
    }
    setSocialLoading(strategy === 'oauth_google' ? 'google' : 'facebook')
    try {
      await clerk.client.signIn.authenticateWithRedirect({
        strategy,
        redirectUrl: '/connexion',
        redirectUrlComplete: '/tableau-de-bord',
      })
    } catch {
      setSocialLoading(null)
      toast.error('La connexion sociale a échoué. Réessayez.')
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-sm flex-col justify-center md:mx-0">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight text-ink md:text-3xl">Bon retour parmi nous</h1>
        <p className="mt-1.5 text-sm leading-relaxed text-ink-muted">
          Connectez-vous pour gérer votre station de lavage.
        </p>
      </div>

      <div className="mb-4 flex flex-col gap-2.5">
        <button
          type="button"
          disabled={socialLoading !== null}
          onClick={() => void signInSocial('oauth_google')}
          className="flex h-10 w-full items-center justify-center gap-2 rounded-lg border border-[#e5e7eb] bg-white text-sm text-[#111827] transition hover:bg-[#f9fafb] disabled:cursor-not-allowed disabled:opacity-60 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#47253F]/40"
        >
          <GoogleIcon />
          {socialLoading === 'google' ? 'Redirection…' : 'Continuer avec Google'}
        </button>
        <button
          type="button"
          disabled={socialLoading !== null}
          onClick={() => void signInSocial('oauth_facebook')}
          className="flex h-10 w-full items-center justify-center gap-2 rounded-lg border border-[#e5e7eb] bg-white text-sm text-[#111827] transition hover:bg-[#f9fafb] disabled:cursor-not-allowed disabled:opacity-60 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#47253F]/40"
        >
          <FacebookIcon />
          {socialLoading === 'facebook' ? 'Redirection…' : 'Continuer avec Facebook'}
        </button>
      </div>

      <div className="relative mb-4 flex items-center">
        <div className="flex-1 border-t border-line" />
        <span className="px-3 text-xs text-ink-muted">OU</span>
        <div className="flex-1 border-t border-line" />
      </div>

      <form className="space-y-3.5" onSubmit={onSubmit} noValidate>
        <Field label="Email" required error={fields.email}>
          {(id) => (
            <Input
              id={id}
              type="email"
              autoComplete="username"
              autoFocus
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              placeholder="nom@exemple.com"
              invalid={Boolean(fields.email)}
              className="h-10 rounded-lg focus:outline-[#47253F]/50"
            />
          )}
        </Field>

        <Field label="Mot de passe" required error={fields.password}>
          {(id) => (
            <div className="relative">
              <Input
                id={id}
                type={showPassword ? 'text' : 'password'}
                autoComplete="current-password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="••••••••"
                invalid={Boolean(fields.password)}
                className="h-10 rounded-lg pr-10 focus:outline-[#47253F]/50"
              />
              <button
                type="button"
                onClick={() => setShowPassword((v) => !v)}
                className="absolute inset-y-0 right-2 flex items-center text-ink-muted transition hover:text-ink"
                aria-label={showPassword ? 'Masquer le mot de passe' : 'Afficher le mot de passe'}
              >
                {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
              </button>
            </div>
          )}
        </Field>

        <div className="flex items-center justify-between text-xs">
          <label className="flex items-center gap-2 text-ink-muted">
            <input
              type="checkbox"
              checked={remember}
              onChange={(event) => setRemember(event.target.checked)}
              className="h-3.5 w-3.5 rounded border-line accent-[#47253F]"
            />
            Se souvenir de moi
          </label>
          <Link to="/mot-de-passe-oublie" className="font-medium text-[#47253F] hover:underline">
            Mot de passe oublié ?
          </Link>
        </div>

        {error ? (
          <p className="rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p>
        ) : null}

        <Button
          type="submit"
          variant="primary"
          className="mt-1 h-10 w-full rounded-lg bg-[#47253F]! text-white! hover:bg-[#3a1f35]! focus-visible:outline-[#47253F]/40"
          loading={loading}
        >
          Se connecter
        </Button>
      </form>

      <p className="mt-4 text-center text-sm text-ink-muted md:text-left">
        Vous n’avez pas encore de compte ?{' '}
        <Link to="/inscription" className="font-medium text-[#47253F] hover:underline">
          S’inscrire
        </Link>
      </p>
    </div>
  )
}
