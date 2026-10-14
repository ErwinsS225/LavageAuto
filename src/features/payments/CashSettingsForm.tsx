/**
 * Paramètres de caisse et identité des reçus.
 *
 * Chaque champ est **appliqué par le serveur** : le fond proposé à l'ouverture,
 * la tolérance d'écart avant justification écrite, les rôles autorisés à tenir
 * la caisse, l'ouverture des annulations, et ce que le reçu porte. Un réglage
 * affiché mais non appliqué serait un mensonge : ici, tous le sont.
 */
import { useEffect, useState } from 'react'

import { Button, Card, CardHeader, Field, Input, SkeletonRows, Switch, Textarea } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { ApercuRecu } from '@/features/payments/ReceiptView'
import { useCashSettings, useSaveCashSettings } from '@/features/payments/api'
import { ApiError } from '@/lib/api'
import { parseAmountInput } from '@/lib/format'
import type { Role } from '@/types'

export default function CashSettingsForm({ stationId, ville }: { stationId: string; ville?: string }) {
  const toast = useToast()
  const requete = useCashSettings(stationId)
  const enregistrement = useSaveCashSettings()
  const [erreur, setErreur] = useState('')

  const [formulaire, setFormulaire] = useState({
    default_float_xof: 0,
    discrepancy_tolerance_xof: 0,
    max_cashier_discount_xof: 1000,
    allowed_roles: [] as Role[],
    allow_void: true,
    require_open_cash_session: true,
    receipt_prefix: 'REC',
    receipt_legal_name: '',
    receipt_tax_id: '',
    receipt_footer: '',
    receipt_logo_url: '',
  })

  useEffect(() => {
    const reglages = requete.data?.settings
    if (!reglages) return
    setFormulaire({
      default_float_xof: reglages.default_float_xof,
      discrepancy_tolerance_xof: reglages.discrepancy_tolerance_xof,
      max_cashier_discount_xof: reglages.max_cashier_discount_xof,
      allowed_roles: reglages.allowed_roles,
      allow_void: reglages.allow_void,
      require_open_cash_session: reglages.require_open_cash_session,
      receipt_prefix: reglages.receipt_prefix,
      receipt_legal_name: reglages.receipt_legal_name,
      receipt_tax_id: reglages.receipt_tax_id,
      receipt_footer: reglages.receipt_footer,
      receipt_logo_url: reglages.receipt_logo_url,
    })
  }, [requete.data])

  function basculerRole(role: Role, coche: boolean) {
    setFormulaire((precedent) => ({
      ...precedent,
      allowed_roles: coche
        ? [...precedent.allowed_roles, role]
        : precedent.allowed_roles.filter((item) => item !== role),
    }))
  }

  async function enregistrer() {
    setErreur('')
    try {
      await enregistrement.mutateAsync({
        stationId,
        settings: {
          default_float_xof: formulaire.default_float_xof,
          discrepancy_tolerance_xof: formulaire.discrepancy_tolerance_xof,
          max_cashier_discount_xof: formulaire.max_cashier_discount_xof,
          allowed_roles: formulaire.allowed_roles,
          allow_void: formulaire.allow_void,
          require_open_cash_session: formulaire.require_open_cash_session,
          receipt_prefix: formulaire.receipt_prefix.trim() || 'REC',
          receipt_legal_name: formulaire.receipt_legal_name,
          receipt_tax_id: formulaire.receipt_tax_id,
          receipt_footer: formulaire.receipt_footer,
          receipt_logo_url: formulaire.receipt_logo_url,
        },
      })
      toast.success('Paramètres de caisse enregistrés.')
    } catch (caught) {
      setErreur(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué.")
    }
  }

  if (requete.isLoading) return <SkeletonRows rows={5} />
  if (requete.isError || !requete.data) {
    return (
      <Card>
        <CardHeader title="Paramètres de caisse" subtitle="Ces réglages n'ont pas pu être chargés." />
      </Card>
    )
  }

  const roles = requete.data.roles_connus

  return (
    <div className="space-y-5">
      <Card>
        <CardHeader
          title="Tenue de caisse"
          subtitle="Ce que le comptoir peut faire, et ce qui exige une justification."
        />
        <div className="grid gap-4 p-4 sm:grid-cols-2">
          <Field
            label="Fond de caisse proposé (FCFA)"
            hint="Montant pré-rempli à l'ouverture d'une session. Chaque session garde son propre fond."
          >
            {(id) => (
              <Input
                id={id}
                inputMode="numeric"
                value={formulaire.default_float_xof === 0 ? '' : formulaire.default_float_xof}
                onChange={(event) =>
                  setFormulaire((precedent) => ({
                    ...precedent,
                    default_float_xof: parseAmountInput(event.target.value),
                  }))
                }
              />
            )}
          </Field>
          <Field
            label="Écart toléré à la clôture (FCFA)"
            hint="Au-delà, la clôture exige un motif écrit. Mettez 0 pour tout exiger."
          >
            {(id) => (
              <Input
                id={id}
                inputMode="numeric"
                value={formulaire.discrepancy_tolerance_xof === 0 ? '' : formulaire.discrepancy_tolerance_xof}
                onChange={(event) =>
                  setFormulaire((precedent) => ({
                    ...precedent,
                    discrepancy_tolerance_xof: parseAmountInput(event.target.value),
                  }))
                }
              />
            )}
          </Field>
          <Field
            label="Remise maximale du caissier (FCFA)"
            hint="Une remise plus élevée demande l'accord du gérant."
          >
            {(id) => (
              <Input
                id={id}
                inputMode="numeric"
                value={formulaire.max_cashier_discount_xof === 0 ? '' : formulaire.max_cashier_discount_xof}
                onChange={(event) =>
                  setFormulaire((precedent) => ({
                    ...precedent,
                    max_cashier_discount_xof: parseAmountInput(event.target.value),
                  }))
                }
              />
            )}
          </Field>
          <div className="space-y-3">
            <Switch
              checked={formulaire.require_open_cash_session}
              label="Caisse obligatoire pour les espèces"
              hint="Sans session ouverte, les espèces sont refusées."
              onChange={(valeur) =>
                setFormulaire((precedent) => ({ ...precedent, require_open_cash_session: valeur }))
              }
            />
            <Switch
              checked={formulaire.allow_void}
              label="Annulation d'encaissement autorisée"
              hint="La contrepassation reste tracée et limitée au gérant / propriétaire."
              onChange={(valeur) => setFormulaire((precedent) => ({ ...precedent, allow_void: valeur }))}
            />
          </div>

          <div className="sm:col-span-2">
            <p className="text-sm font-medium text-ink">Rôles autorisés à tenir la caisse</p>
            <p className="mt-1 text-xs text-ink-muted">
              Aucun coché signifie « tous ceux dont les droits d&apos;ouverture le permettent déjà ». Le
              propriétaire garde toujours l&apos;accès.
            </p>
            <div className="mt-2 flex flex-wrap gap-2">
              {roles.map((role) => {
                const coche = formulaire.allowed_roles.includes(role.code as Role)
                return (
                  <button
                    key={role.code}
                    type="button"
                    aria-pressed={coche}
                    onClick={() => basculerRole(role.code as Role, !coche)}
                    className={'chip ' + (coche ? 'chip-active' : '')}
                  >
                    {role.label}
                  </button>
                )
              })}
            </div>
          </div>
        </div>
      </Card>

      <Card>
        <CardHeader
          title="Identité du reçu"
          subtitle="Ce que le justificatif remis au client porte. Ces mentions sont saisies par la station : WashGuard n'en invente aucune."
        />
        <div className="grid gap-4 p-4 sm:grid-cols-2">
          <Field label="Préfixe de numérotation" hint="Les numéros sont PRÉFIXE-DATE-0001, par station et par jour.">
            {(id) => (
              <Input
                id={id}
                value={formulaire.receipt_prefix}
                onChange={(event) =>
                  setFormulaire((precedent) => ({ ...precedent, receipt_prefix: event.target.value }))
                }
                placeholder="REC"
              />
            )}
          </Field>
          <Field label="Raison sociale" hint="Ex : SARL LAVAGE EXPRESS. Laissé vide, il n'est pas imprimé.">
            {(id) => (
              <Input
                id={id}
                value={formulaire.receipt_legal_name}
                onChange={(event) =>
                  setFormulaire((precedent) => ({ ...precedent, receipt_legal_name: event.target.value }))
                }
              />
            )}
          </Field>
          <Field label="Identifiant fiscal (NCC / RCCM)" hint="Vide : la ligne n'apparaît pas sur le reçu.">
            {(id) => (
              <Input
                id={id}
                value={formulaire.receipt_tax_id}
                onChange={(event) =>
                  setFormulaire((precedent) => ({ ...precedent, receipt_tax_id: event.target.value }))
                }
              />
            )}
          </Field>
          <Field label="Adresse du logo (URL)" hint="Affiché si le lien est valide. Facultatif.">
            {(id) => (
              <Input
                id={id}
                value={formulaire.receipt_logo_url}
                onChange={(event) =>
                  setFormulaire((precedent) => ({ ...precedent, receipt_logo_url: event.target.value }))
                }
                placeholder="https://…"
              />
            )}
          </Field>
          <div className="sm:col-span-2">
            <Field label="Bas de reçu" hint="Remerciement, conditions de garantie, contacts.">
              {(id) => (
                <Textarea
                  id={id}
                  rows={2}
                  value={formulaire.receipt_footer}
                  onChange={(event) =>
                    setFormulaire((precedent) => ({ ...precedent, receipt_footer: event.target.value }))
                  }
                />
              )}
            </Field>
          </div>
        </div>
        <div className="border-t border-line p-4">
          <ApercuRecu settings={{ ...requete.data.settings, ...formulaire }} ville={ville} />
        </div>
      </Card>

      {erreur ? (
        <p className="rounded-field border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{erreur}</p>
      ) : null}

      <div className="flex justify-end">
        <Button variant="primary" loading={enregistrement.isPending} onClick={() => void enregistrer()}>
          Enregistrer les paramètres de caisse
        </Button>
      </div>
    </div>
  )
}
