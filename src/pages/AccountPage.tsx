/**
 * Mon compte : profil, changement de mot de passe et informations de session.
 */
import { useState } from 'react'

import { Button, Card, CardHeader, Field, Input, PageHeader, Spinner } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { api, ApiError } from '@/lib/api'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'

export default function AccountPage() {
  const toast = useToast()
  const { me } = useAuth()
  usePageTitle('Mon compte')

  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)

  if (!me) return <Spinner label="Chargement…" />
  const user = me.user

  async function submitPassword(event: React.FormEvent) {
    event.preventDefault()
    setError('')
    setFields({})
    if (newPassword.length < 8) {
      setFields({ new_password: '8 caractères minimum' })
      return
    }
    if (newPassword !== confirmPassword) {
      setFields({ confirm: 'Les mots de passe ne correspondent pas.' })
      return
    }
    setSaving(true)
    try {
      await api.patch('/auth/password', { current_password: currentPassword, new_password: newPassword })
      toast.success('Mot de passe mis à jour.')
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
    } catch (caught) {
      if (caught instanceof ApiError) {
        setError(caught.message)
        setFields(caught.fields)
      } else {
        setError('Le changement a échoué.')
      }
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader title="Mon compte" subtitle="Vos informations personnelles et votre sécurité." />

      <Card>
        <CardHeader title="Profil" subtitle={user.role_label ?? user.role} />
        <div className="grid gap-4 p-4 sm:grid-cols-2">
          <div>
            <p className="stat-label">Nom</p>
            <p className="text-sm font-medium text-ink">{user.full_name}</p>
          </div>
          <div>
            <p className="stat-label">E-mail</p>
            <p className="text-sm font-medium text-ink">{user.email}</p>
          </div>
          <div>
            <p className="stat-label">Téléphone</p>
            <p className="text-sm font-medium text-ink">{user.phone || '—'}</p>
          </div>
          <div>
            <p className="stat-label">Station</p>
            <p className="text-sm font-medium text-ink">{me.station?.name ?? '—'}</p>
          </div>
        </div>
      </Card>

      <Card>
        <CardHeader title="Changer de mot de passe" subtitle="8 caractères minimum, avec chiffres et lettres recommandés" />
        <form onSubmit={submitPassword} className="grid gap-4 p-4 sm:grid-cols-3">
          <Field label="Mot de passe actuel" required error={fields.current_password}>
            {(id) => (
              <Input
                id={id}
                type="password"
                value={currentPassword}
                onChange={(event) => setCurrentPassword(event.target.value)}
                invalid={Boolean(fields.current_password)}
              />
            )}
          </Field>
          <Field label="Nouveau mot de passe" required error={fields.new_password}>
            {(id) => (
              <Input
                id={id}
                type="password"
                value={newPassword}
                onChange={(event) => setNewPassword(event.target.value)}
                invalid={Boolean(fields.new_password)}
              />
            )}
          </Field>
          <Field label="Confirmer" required error={fields.confirm}>
            {(id) => (
              <Input
                id={id}
                type="password"
                value={confirmPassword}
                onChange={(event) => setConfirmPassword(event.target.value)}
                invalid={Boolean(fields.confirm)}
              />
            )}
          </Field>
          <div className="sm:col-span-3">
            {error ? (
              <p className="mb-3 rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p>
            ) : null}
            <Button type="submit" variant="primary" loading={saving}>
              Mettre à jour
            </Button>
          </div>
        </form>
      </Card>
    </div>
  )
}
