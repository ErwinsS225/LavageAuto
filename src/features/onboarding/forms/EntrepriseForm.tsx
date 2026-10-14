/**
 * Étape « entreprise » : identité commerciale, facturation, mentions légales.
 *
 * Trois sections, un seul enregistrement. Les mentions administratives portent
 * chacune leur état — « renseigné » n'est pas « vérifié », et seule la
 * plateforme WashGuard peut vérifier (l'écran ne propose donc aucun bouton pour
 * le faire).
 */
import { useState, type FormEvent } from 'react'

import { Badge, Button, Card, CardHeader, Field, Input, Select, Textarea } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useSaveOrganisation, type OrganisationDetaillee } from '@/features/profile/api'
import { ApiError } from '@/lib/api'
import type { RegistryField } from '@/types'

/** Activités proposées. « autre » laisse le champ libre : on ne range personne. */
const ACTIVITES = [
  'lavage manuel',
  'lavage automatique',
  'haute pression',
  'nettoyage intérieur',
  'detailing',
  'mixte',
  'autre',
]

const TONS_ETAT: Record<RegistryField['status'], 'success' | 'brand' | 'warning' | 'neutral'> = {
  verified: 'success',
  provided: 'brand',
  pending_review: 'warning',
  empty: 'neutral',
}

export default function EntrepriseForm({
  donnees,
  onEnregistre,
}: {
  donnees: OrganisationDetaillee
  onEnregistre: () => void
}) {
  const toast = useToast()
  const enregistrement = useSaveOrganisation()
  const organisation = donnees.organization
  const registre = donnees.registry ?? []

  const [identite, setIdentite] = useState({
    legal_name: organisation.legal_name ?? '',
    trade_name: organisation.trade_name ?? '',
    activity_type: organisation.activity_type ?? '',
    activity_description: organisation.activity_description ?? '',
    activity_started_on: organisation.activity_started_on ?? '',
    whatsapp: organisation.whatsapp ?? '',
    website: organisation.website ?? '',
  })
  const [facturation, setFacturation] = useState({
    billing_name: organisation.billing_name ?? '',
    billing_address: organisation.billing_address ?? '',
    billing_email: organisation.billing_email ?? '',
    billing_phone: organisation.billing_phone ?? '',
  })
  const [mentions, setMentions] = useState<Record<string, string>>(
    Object.fromEntries(registre.map((champ) => [champ.field_code, champ.value ?? ''])),
  )
  const [mentionsLibres, setMentionsLibres] = useState({
    legal_mentions: organisation.legal_mentions ?? '',
    website_ok: organisation.website ?? '',
  })
  const [erreur, setErreur] = useState('')

  function majIdentite(champ: string, valeur: string) {
    setIdentite((actuel) => ({ ...actuel, [champ]: valeur }))
  }
  function majFacturation(champ: string, valeur: string) {
    setFacturation((actuel) => ({ ...actuel, [champ]: valeur }))
  }

  async function enregistrer(event: FormEvent) {
    event.preventDefault()
    setErreur('')
    try {
      await enregistrement.mutateAsync({
        // `name` reste le nom affiché partout (reçus, listes) : il suit le nom
        // commercial s'il est renseigné, sinon la raison sociale.
        ...identite,
        ...facturation,
        legal_mentions: mentionsLibres.legal_mentions,
        registry: mentions,
      })
      toast.success('Fiche d’entreprise enregistrée.')
      onEnregistre()
    } catch (caught) {
      if (caught instanceof ApiError) {
        setErreur(caught.message)
      } else {
        setErreur("L'enregistrement a échoué. Vérifiez votre connexion puis réessayez.")
      }
    }
  }

  return (
    <form className="space-y-5" onSubmit={enregistrer} noValidate>
      <Card>
        <CardHeader
          title="Identité commerciale"
          subtitle="Ce que voient vos clients et ce qui figure sur les reçus."
        />
        <div className="grid gap-4 p-4 sm:grid-cols-2">
          <Field label="Nom commercial" hint="Le nom que vos clients connaissent.">
            {(id) => (
              <Input
                id={id}
                value={identite.trade_name}
                onChange={(event) => majIdentite('trade_name', event.target.value)}
                placeholder="Ex : Lavage du Plateau"
              />
            )}
          </Field>
          <Field label="Raison sociale" hint="Le nom légal, si différent du nom commercial.">
            {(id) => (
              <Input
                id={id}
                value={identite.legal_name}
                onChange={(event) => majIdentite('legal_name', event.target.value)}
                placeholder="Ex : LAVAGE DU PLATEAU SARL U"
              />
            )}
          </Field>
          <Field label="Activité principale">
            {(id) => (
              <Select
                id={id}
                value={identite.activity_type}
                onChange={(event) => majIdentite('activity_type', event.target.value)}
                placeholder="Choisir…"
                options={ACTIVITES.map((activite) => ({ value: activite, label: activite }))}
              />
            )}
          </Field>
          <Field label="Date d'ouverture" hint="Format AAAA-MM-JJ.">
            {(id) => (
              <Input
                id={id}
                type="date"
                value={identite.activity_started_on}
                onChange={(event) => majIdentite('activity_started_on', event.target.value)}
              />
            )}
          </Field>
          <Field label="WhatsApp">
            {(id) => (
              <Input
                id={id}
                value={identite.whatsapp}
                onChange={(event) => majIdentite('whatsapp', event.target.value)}
                placeholder="+225 07 00 00 00 00"
              />
            )}
          </Field>
          <Field label="Site web">
            {(id) => (
              <Input
                id={id}
                value={identite.website}
                onChange={(event) => majIdentite('website', event.target.value)}
                placeholder="https://…"
              />
            )}
          </Field>
          <div className="sm:col-span-2">
            <Field label="Description de l'activité">
              {(id) => (
                <Textarea
                  id={id}
                  value={identite.activity_description}
                  onChange={(event) => majIdentite('activity_description', event.target.value)}
                  placeholder="Ex : lavage à la main, nettoyage intérieur et polissage."
                />
              )}
            </Field>
          </div>
        </div>
      </Card>

      <Card>
        <CardHeader
          title="Facturation"
          subtitle="À renseigner seulement si la facture ne doit pas partir à l'adresse de la station."
        />
        <div className="grid gap-4 p-4 sm:grid-cols-2">
          <Field label="Nom sur la facture">
            {(id) => (
              <Input id={id} value={facturation.billing_name} onChange={(event) => majFacturation('billing_name', event.target.value)} />
            )}
          </Field>
          <Field label="E-mail de facturation">
            {(id) => (
              <Input id={id} type="email" value={facturation.billing_email} onChange={(event) => majFacturation('billing_email', event.target.value)} />
            )}
          </Field>
          <Field label="Téléphone de facturation">
            {(id) => (
              <Input id={id} value={facturation.billing_phone} onChange={(event) => majFacturation('billing_phone', event.target.value)} />
            )}
          </Field>
          <Field label="Adresse de facturation">
            {(id) => (
              <Input id={id} value={facturation.billing_address} onChange={(event) => majFacturation('billing_address', event.target.value)} />
            )}
          </Field>
        </div>
      </Card>

      <Card>
        <CardHeader
          title="Registre administratif"
          subtitle="Facultatif : ces mentions servent aux démarches et aux factures officielles. Chacune garde son propre état."
        />
        <div className="grid gap-4 p-4 sm:grid-cols-2">
          {registre.map((champ) => (
            <Field key={champ.field_code} label={champ.label} hint={champ.help}>
              {(id) => (
                <div className="space-y-1.5">
                  <Input
                    id={id}
                    value={mentions[champ.field_code] ?? ''}
                    onChange={(event) =>
                      setMentions((actuel) => ({ ...actuel, [champ.field_code]: event.target.value }))
                    }
                  />
                  <div className="flex items-center gap-2">
                    <Badge tone={TONS_ETAT[champ.status]} dot>
                      {champ.status_label}
                    </Badge>
                    {champ.verified_at ? (
                      <span className="text-xs text-ink-subtle">
                        le {new Date(champ.verified_at).toLocaleDateString('fr-FR')}
                      </span>
                    ) : null}
                  </div>
                </div>
              )}
            </Field>
          ))}
          <div className="sm:col-span-2">
            <Field
              label="Mentions légales à imprimer"
              hint="Texte libre repris tel quel sur les documents de la station."
            >
              {(id) => (
                <Textarea
                  id={id}
                  value={mentionsLibres.legal_mentions}
                  onChange={(event) =>
                    setMentionsLibres((actuel) => ({ ...actuel, legal_mentions: event.target.value }))
                  }
                />
              )}
            </Field>
          </div>
        </div>
        <p className="border-t border-line px-4 py-3 text-xs text-ink-subtle">
          La mention « vérifié » est posée par le support WashGuard après contrôle — l'application ne
          la décerne pas elle-même. Modifier une valeur annule la vérification précédente.
        </p>
      </Card>

      {erreur ? (
        <p className="rounded-field border border-danger/30 bg-danger-soft px-3 py-2 text-sm text-danger">{erreur}</p>
      ) : null}

      <div className="flex flex-wrap gap-2">
        <Button type="submit" variant="primary" loading={enregistrement.isPending}>
          Enregistrer l’entreprise
        </Button>
      </div>
    </form>
  )
}
