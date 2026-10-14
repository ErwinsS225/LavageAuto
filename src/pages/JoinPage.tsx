/**
 * Page publique « Rejoindre une entreprise » (/rejoindre).
 *
 * Elle est atteignable par un lien d'invitation (`?jeton=…`) ou par saisie du
 * code court dicté au téléphone. L'aperçu vient du serveur et ne contient aucun
 * identifiant : l'invité reconnaît son employeur, rien de plus n'est exposé.
 *
 * Le compte créé ici appartient à l'entreprise invitante : aucune entreprise,
 * aucune station, aucun abonnement n'est créé (c'est le serveur qui décide, la
 * page n'a pas à le supposer).
 */
import { ArrowRight, BadgeCheck, KeyRound, LogIn } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'

import { AuthHeader } from '@/components/auth/AuthHeader'
import { Badge, Button, Card, Field, Input } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useInvitationPreview } from '@/features/invitations/api'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { ApiError } from '@/lib/api'

export default function JoinPage() {
  usePageTitle('Rejoindre une entreprise')
  const [parametres, setParametres] = useSearchParams()
  const navigate = useNavigate()
  const toast = useToast()
  const { register } = useAuth()

  const [saisie, setSaisie] = useState(parametres.get('code') ?? '')
  // Le jeton long du lien remplace le code : le serveur accepte les deux.
  const jeton = parametres.get('jeton') ?? ''
  const [code, setCode] = useState(parametres.get('code') ?? '')
  const apercu = useInvitationPreview(jeton || code)

  const [formulaire, setFormulaire] = useState({ full_name: '', email: '', phone: '', password: '' })
  const [erreur, setErreur] = useState('')
  const [champs, setChamps] = useState<Record<string, string>>({})
  const [envoi, setEnvoi] = useState(false)

  function chercher(event: FormEvent) {
    event.preventDefault()
    const propre = saisie.trim().toUpperCase()
    if (!propre) return
    setCode(propre)
    // Le code est aussi dans l'adresse : l'invité peut recharger ou partager.
    setParametres({ code: propre }, { replace: true })
  }

  async function creerCompte(event: FormEvent) {
    event.preventDefault()
    setErreur('')
    setChamps({})
    if (!formulaire.full_name.trim() || !formulaire.email.trim()) {
      setChamps({ full_name: 'Nom obligatoire.', email: 'E-mail obligatoire.' })
      return
    }
    if (formulaire.password.length < 8) {
      setChamps({ password: '8 caractères minimum.' })
      return
    }
    setEnvoi(true)
    try {
      await register({ ...formulaire, invitation_token: jeton })
      toast.success(`Bienvenue chez ${apercu.data?.organization_name ?? "l'entreprise"} !`)
      navigate('/tableau-de-bord', { replace: true })
    } catch (caught) {
      if (caught instanceof ApiError) {
        setErreur(caught.message)
        setChamps(caught.fields)
      } else {
        setErreur("La création du compte a échoué.")
      }
    } finally {
      setEnvoi(false)
    }
  }

  // Sans jeton dans le lien, l'invitation ne peut pas être rejointe par
  // formulaire : il faut d'abord retrouver l'invitation (code saisi, ou lien).
  const invitationVerifiee = apercu.data?.usable === true

  return (
    <>
      <AuthHeader />
      <div className="mx-auto w-full max-w-sm">
        <div className="mb-6">
          <h1 className="text-xl font-bold tracking-tight text-ink">Rejoindre une entreprise</h1>
          <p className="mt-1.5 text-sm leading-relaxed text-ink-muted">
            Vous avez reçu un code ou un lien d’invitation ? Saisissez-le ci-dessous : vous verrez
            quelle entreprise vous invite, et avec quel rôle, avant de créer votre compte.
          </p>
        </div>

        {!jeton && (
          <form className="space-y-3.5" onSubmit={chercher}>
            <Field label="Code d’invitation" hint="Huit caractères, dictés ou recopiés.">
              {(id) => (
                <Input
                  id={id}
                  value={saisie}
                  onChange={(event) => setSaisie(event.target.value)}
                  placeholder="EX : K7M2P9QT"
                  className="h-10 font-mono tracking-[0.2em] uppercase"
                  autoComplete="one-time-code"
                />
              )}
            </Field>
            <Button type="submit" variant="primary" icon={<KeyRound size={15} />} className="w-full">
              Vérifier l’invitation
            </Button>
          </form>
        )}

        {apercu.isLoading && <p className="mt-4 text-sm text-ink-muted">Vérification…</p>}

        {apercu.isError && (
          <Card className="mt-4">
            <div className="space-y-2 p-4">
              <p className="text-sm text-danger">
                {apercu.error instanceof ApiError
                  ? apercu.error.message
                  : "L’invitation n’a pas pu être vérifiée. Vérifiez le code auprès de la personne qui vous a invité."}
              </p>
              <p className="text-xs text-ink-subtle">
                Un code se lit à voix haute sans ambiguïté : les lettres I, L, O et les chiffres 0 et 1
                n’en font jamais partie.
              </p>
            </div>
          </Card>
        )}

        {apercu.data && (
          <Card className="mt-4">
            <div className="space-y-3 p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="flex items-center gap-1.5 text-sm font-semibold text-ink">
                    <BadgeCheck size={16} className="text-brand" aria-hidden />
                    {apercu.data.organization_name}
                  </p>
                  <p className="mt-0.5 text-xs text-ink-muted">
                    {apercu.data.city || 'Ville non renseignée'}
                  </p>
                </div>
                <Badge tone={apercu.data.usable ? 'brand' : 'warning'}>{apercu.data.role_label}</Badge>
              </div>

              <p className="text-sm text-ink-muted">{apercu.data.message}</p>

              {!apercu.data.usable && (
                <p className="text-xs text-ink-subtle">
                  Demandez une nouvelle invitation à la personne qui vous a invité : le gérant peut en
                  créer une en quelques secondes depuis Paramètres → Invitations.
                </p>
              )}

              {invitationVerifiee && jeton && (
                <form className="space-y-3.5 border-t border-line pt-3" onSubmit={creerCompte} noValidate>
                  <p className="text-xs text-ink-subtle">
                    Vous rejoignez cette entreprise : votre compte aura le rôle de{' '}
                    {apercu.data.role_label.toLowerCase()}, sans devenir propriétaire.
                  </p>
                  <Field label="Nom complet" required error={champs.full_name}>
                    {(id) => (
                      <Input
                        id={id}
                        value={formulaire.full_name}
                        onChange={(event) =>
                          setFormulaire((actuel) => ({ ...actuel, full_name: event.target.value }))
                        }
                        invalid={Boolean(champs.full_name)}
                        className="h-10"
                      />
                    )}
                  </Field>
                  <Field label="Adresse e-mail" required error={champs.email}>
                    {(id) => (
                      <Input
                        id={id}
                        type="email"
                        value={formulaire.email}
                        onChange={(event) =>
                          setFormulaire((actuel) => ({ ...actuel, email: event.target.value }))
                        }
                        invalid={Boolean(champs.email)}
                        className="h-10"
                      />
                    )}
                  </Field>
                  <Field label="Téléphone" error={champs.phone}>
                    {(id) => (
                      <Input
                        id={id}
                        type="tel"
                        value={formulaire.phone}
                        onChange={(event) =>
                          setFormulaire((actuel) => ({ ...actuel, phone: event.target.value }))
                        }
                        className="h-10"
                      />
                    )}
                  </Field>
                  <Field label="Mot de passe" required error={champs.password} hint="8 caractères minimum.">
                    {(id) => (
                      <Input
                        id={id}
                        type="password"
                        autoComplete="new-password"
                        value={formulaire.password}
                        onChange={(event) =>
                          setFormulaire((actuel) => ({ ...actuel, password: event.target.value }))
                        }
                        invalid={Boolean(champs.password)}
                        className="h-10"
                      />
                    )}
                  </Field>
                  {erreur && <p className="text-sm text-danger">{erreur}</p>}
                  <Button type="submit" variant="primary" loading={envoi} className="w-full" icon={<ArrowRight size={15} />}>
                    Rejoindre {apercu.data.organization_name}
                  </Button>
                </form>
              )}

              {invitationVerifiee && !jeton && (
                <p className="border-t border-line pt-3 text-xs text-ink-subtle">
                  Ouvrez le lien complet reçu par message pour créer votre compte. Si vous n’avez que le
                  code, demandez le lien à la personne qui vous a invité.
                </p>
              )}
            </div>
          </Card>
        )}

        <p className="mt-5 text-sm text-ink-muted">
          Vous avez déjà un compte ?{' '}
          <Link to="/connexion" className="inline-flex items-center gap-1 font-medium text-brand hover:underline">
            <LogIn size={14} aria-hidden />
            Se connecter
          </Link>
        </p>
      </div>
    </>
  )
}
