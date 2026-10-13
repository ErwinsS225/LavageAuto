/**
 * Formulaire d'inscription — même identité visuelle que LoginForm (LavPro).
 * Crée l'organisation, sa première station et le compte propriétaire via
 * l'API existante (useAuth.register).
 */
import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'

import { Button, Field, Input } from '@/components/ui'
import { useAuth } from '@/hooks/useAuth'
import { ApiError } from '@/lib/api'
import { useToast } from '@/components/Toaster'
import { usePageTitle } from '@/hooks/usePageTitle'

export default function RegisterForm() {
  usePageTitle('Créer un compte')
  const navigate = useNavigate()
  const toast = useToast()
  const { register } = useAuth()

  const [organization, setOrganization] = useState('')
  const [station, setStation] = useState('')
  const [fullName, setFullName] = useState('')
  const [email, setEmail] = useState('')
  const [phone, setPhone] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(false)

  function validate(): boolean {
    if (!organization.trim()) {
      setFields({ organization_name: 'Le nom de votre entreprise est obligatoire.' })
      return false
    }
    if (!station.trim()) {
      setFields({ station_name: 'Le nom de la station est obligatoire.' })
      return false
    }
    if (!fullName.trim()) {
      setFields({ full_name: 'Le nom complet est obligatoire.' })
      return false
    }
    if (!email.trim()) {
      setFields({ email: "L'adresse e-mail est obligatoire." })
      return false
    }
    if (password.length < 8) {
      setFields({ password: '8 caractères minimum.' })
      return false
    }
    if (confirm !== password) {
      setFields({ password_confirmation: 'Les mots de passe ne correspondent pas.' })
      return false
    }
    return true
  }

  async function onSubmit(event: FormEvent) {
    event.preventDefault()
    setError('')
    setFields({})
    if (!validate()) return
    setLoading(true)
    try {
      // L'API /auth/register crée organisation + première station + compte
      // propriétaire en une transaction ; organization_name et station_name
      // sont obligatoires côté serveur (la ville a une valeur par défaut).
      await register({
        organization_name: organization,
        station_name: station,
        full_name: fullName,
        email,
        phone,
        password,
      })
      toast.success('Compte créé. Bienvenue sur LavPro !')
      navigate('/tableau-de-bord', { replace: true })
    } catch (caught) {
      if (caught instanceof ApiError) {
        setError(caught.message)
        setFields(caught.fields)
      } else {
        setError("L'inscription a échoué.")
      }
    } finally {
      setLoading(false)
    }
  }

  const inputClass = 'h-10 rounded-lg'

  return (
    <div className="mx-auto flex w-full max-w-sm flex-col justify-center md:mx-0">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight text-ink md:text-3xl">Créez votre compte</h1>
        <p className="mt-1.5 text-sm leading-relaxed text-ink-muted">
          Commencez à simplifier la gestion de votre station.
        </p>
      </div>

      <form className="space-y-3.5" onSubmit={onSubmit} noValidate>
        <Field label="Nom de l'entreprise" required error={fields.organization_name}>
          {(id) => (
            <Input
              id={id}
              autoComplete="organization"
              value={organization}
              onChange={(event) => setOrganization(event.target.value)}
              placeholder="Ex : Lavage Moderne Cocody"
              invalid={Boolean(fields.organization_name)}
              className={inputClass}
            />
          )}
        </Field>

        <Field label="Nom de la station" required error={fields.station_name}>
          {(id) => (
            <Input
              id={id}
              value={station}
              onChange={(event) => setStation(event.target.value)}
              placeholder="Ex : Station Plateau"
              invalid={Boolean(fields.station_name)}
              className={inputClass}
            />
          )}
        </Field>

        <Field label="Nom complet" required error={fields.full_name}>
          {(id) => (
            <Input
              id={id}
              autoComplete="name"
              value={fullName}
              onChange={(event) => setFullName(event.target.value)}
              placeholder="Awa Traoré"
              invalid={Boolean(fields.full_name)}
              className={inputClass}
            />
          )}
        </Field>

        <Field label="Adresse e-mail" required error={fields.email}>
          {(id) => (
            <Input
              id={id}
              type="email"
              autoComplete="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              placeholder="nom@exemple.com"
              invalid={Boolean(fields.email)}
              className={inputClass}
            />
          )}
        </Field>

        <Field label="Téléphone" error={fields.phone}>
          {(id) => (
            <Input
              id={id}
              type="tel"
              autoComplete="tel"
              value={phone}
              onChange={(event) => setPhone(event.target.value)}
              placeholder="07 00 00 00 00"
              className={inputClass}
            />
          )}
        </Field>

        <Field label="Mot de passe" required error={fields.password} hint="8 caractères minimum.">
          {(id) => (
            <Input
              id={id}
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="••••••••"
              invalid={Boolean(fields.password)}
              className={inputClass}
            />
          )}
        </Field>

        <Field label="Confirmation du mot de passe" required error={fields.password_confirmation}>
          {(id) => (
            <Input
              id={id}
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(event) => setConfirm(event.target.value)}
              placeholder="••••"
              invalid={Boolean(fields.password_confirmation)}
              className={inputClass}
            />
          )}
        </Field>

        {error ? (
          <p className="rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p>
        ) : null}

        <Button
          type="submit"
          variant="primary"
          className="mt-1 h-10 w-full rounded-lg bg-[#47253F]! text-white! hover:bg-[#3a1f35]! focus-visible:outline-[#47253F]/40"
          loading={loading}
        >
          Créer mon compte
        </Button>
      </form>

      <p className="mt-4 text-center text-sm text-ink-muted md:text-left">
        Vous avez déjà un compte ?{' '}
        <Link to="/connexion" className="font-medium text-[#47253F] hover:underline">
          Se connecter
        </Link>
      </p>
    </div>
  )
}
