/**
 * Formulaire d'inscription — même identité visuelle que LoginForm.
 * Crée l'organisation, sa première station et le compte propriétaire via
 * l'API existante (useAuth.register).
 */
import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'

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
      toast.success('Compte créé. Bienvenue sur WashGuard !')
      // Le compte neuf a une station à décrire : on entre dans la mise en route
      // plutôt que d'ouvrir un tableau de bord vide.
      navigate('/onboarding', { replace: true })
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

  const inputClass = 'h-10'

  return (
    <div className="mx-auto flex w-full max-w-sm flex-col justify-center md:mx-0">
      <div className="mb-6">
        <h1 className="text-xl font-bold tracking-tight text-ink">Créez votre compte</h1>
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

        <AnimatePresence>
          {error ? (
            <motion.p
              initial={{ opacity: 0, y: -4 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.18, ease: [0.22, 1, 0.36, 1] }}
              className="rounded-field border border-danger/30 bg-danger-soft px-3 py-2 text-sm text-danger"
            >
              {error}
            </motion.p>
          ) : null}
        </AnimatePresence>

        <Button
          type="submit"
          variant="primary"
          size="lg"
          block
          className="mt-1"
          loading={loading}
        >
          Créer mon compte
        </Button>
      </form>

      <p className="mt-4 text-center text-sm text-ink-muted md:text-left">
        Vous avez déjà un compte ?{' '}
        <Link to="/connexion" className="link-soft font-medium">
          Se connecter
        </Link>
      </p>

      {/* Rejoindre une entreprise n'est pas une inscription classique : le
          compte appartient à une entreprise existante, il n'en crée pas une. */}
      <p className="mt-2 text-center text-xs text-ink-subtle md:text-left">
        Vous avez reçu une invitation ?{' '}
        <Link to="/rejoindre" className="link-soft font-medium">
          Rejoindre une entreprise
        </Link>
      </p>
    </div>
  )
}
