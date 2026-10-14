/**
 * Bloc « mise en route » — ce que la station est, ce qui lui manque.
 *
 * Deux écrans le lisent, et c'est voulu : le récapitulatif qui précède
 * l'activation et le tableau de bord. Le serveur calcule le bloc une seule fois
 * (`blocMiseEnRoute`) ; ici, on ne fait que le mettre en forme. Un écran qui
 * recalculerait de son côté finirait par annoncer « catalogue prêt » là où le
 * serveur refuse d'activer.
 *
 * Trois principes d'affichage :
 *
 *  - **rien d'inventé** : un chiffre affiché est un comptage serveur, un état
 *    vide est dit « vide » (pas « 0 FCFA ») ;
 *  - **le rôle décide** : les blocs d'administration, les raccourcis et les
 *    montants viennent du serveur, déjà filtrés — l'interface ne cache pas ce
 *    qu'elle a reçu, elle n'aurait pas dû le recevoir ;
 *  - **une étape facultative est dite facultative** : elle ne bloque pas
 *    l'ouverture, et elle reste atteignable après.
 */
import {
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  Circle,
  CircleSlash,
  LayoutGrid,
  Package,
  Percent,
  Settings2,
  Wallet,
} from 'lucide-react'
import { Link } from 'react-router-dom'

import { Badge, Button, Card, CardHeader, StatCard } from '@/components/ui'
import type { SetupStation } from '@/types'

/** Étiquette de l'état d'activation, avec son ton. */
function badgeActivation(setup: SetupStation) {
  if (setup.activation.active) return <Badge tone="success" dot>En exploitation</Badge>
  if (setup.activation.blockers_count === 0) return <Badge tone="brand" dot>Prête à activer</Badge>
  return <Badge tone="warning" dot>{setup.activation.blockers_count} élément(s) à compléter</Badge>
}

/** Étiquette du statut d'ouverture du jour. */
function badgeOuverture(setup: SetupStation) {
  const jour = setup.hours.today
  const ton = jour.is_open_now ? 'success' : jour.state === 'hours_missing' ? 'warning' : 'neutral'
  return <Badge tone={ton} dot>{jour.label}</Badge>
}

/**
 * SetupPanel — l'état complet de la station.
 *
 * `compact` sert au tableau de bord d'une station déjà en activité : on garde
 * l'identité, les horaires et les raccourcis, sans redéployer tout le
 * récapitulatif de mise en route.
 */
export function SetupPanel({ setup, compact = false }: { setup: SetupStation; compact?: boolean }) {
  const catalogue = setup.catalogue
  const activite = setup.activity
  const enRoute = !setup.activation.active || !activite.has_activity

  return (
    <div className="space-y-5">
      <Card>
        <CardHeader
          title={setup.station.name || 'Station sans nom'}
          subtitle={
            (setup.station.locality || setup.station.city || 'Localisation non renseignée') +
            ' · ' +
            setup.hours.label
          }
          actions={
            <div className="flex flex-wrap items-center gap-2">
              {badgeActivation(setup)}
              {badgeOuverture(setup)}
            </div>
          }
        />

        {/* Les états honnêtes : ce que la station peut dire d'elle-même sans
            qu'aucune fiche n'existe. Ils remplacent une colonne de zéros. */}
        {enRoute && setup.readiness.statements.length > 0 ? (
          <ul className="space-y-2 border-t border-line p-4">
            {setup.readiness.statements.map((phrase) => (
              <li key={phrase} className="flex items-start gap-2 text-sm text-ink-muted">
                <CircleSlash size={14} className="mt-0.5 shrink-0 text-ink-subtle" aria-hidden />
                <span>{phrase}</span>
              </li>
            ))}
          </ul>
        ) : null}

        <div className="grid gap-4 border-t border-line p-4 sm:grid-cols-2 xl:grid-cols-4">
          <StatCard
            label="Catégories acceptées"
            value={String(catalogue.categories_active)}
            hint={catalogue.categories_active === 0 ? 'Aucune catégorie activée' : 'Véhicules que la station lave'}
            icon={<LayoutGrid size={16} />}
            tone={catalogue.categories_active === 0 ? 'warning' : 'brand'}
          />
          <StatCard
            label="Prestations vendables"
            value={String(catalogue.services_sellable_priced)}
            hint={
              catalogue.services_without_usable_price > 0
                ? `${catalogue.services_without_usable_price} prestation(s) sans prix utilisable`
                : 'Toutes les prestations vendables sont tarifées'
            }
            icon={<Percent size={16} />}
            tone={catalogue.services_sellable_priced === 0 ? 'warning' : 'brand'}
          />
          <StatCard
            label="Moyens de paiement actifs"
            value={String(catalogue.payment_methods_active)}
            hint={catalogue.payment_methods_active === 0 ? 'La caisse ne peut rien encaisser' : 'Ce que la caisse accepte'}
            icon={<Wallet size={16} />}
            tone={catalogue.payment_methods_active === 0 ? 'warning' : 'brand'}
          />
          <StatCard
            label="Équipe et matériel"
            value={`${catalogue.team_active} · ${catalogue.equipment_active}`}
            hint={`${catalogue.team_active} compte(s) · ${catalogue.equipment_active} équipement(s) · ${catalogue.stock_items_active} article(s) de stock`}
            icon={<Package size={16} />}
          />
        </div>

        {/* Les raccourcis du quotidien : ceux que ce profil peut réellement
            ouvrir. Le serveur les a déjà filtrés par permission. */}
        {setup.shortcuts.length > 0 ? (
          <div className="flex flex-wrap gap-2 border-t border-line p-4">
            {setup.shortcuts.map((raccourci) => (
              <Link key={raccourci.code} to={raccourci.href} title={raccourci.hint}>
                <Button variant="secondary" size="sm">
                  {raccourci.label}
                </Button>
              </Link>
            ))}
          </div>
        ) : null}
      </Card>

      {!compact ? (
        <>
          {!setup.activation.active ? (
            <Card>
              <CardHeader
                title="Ce qui manque pour ouvrir"
                subtitle="Le serveur revérifie chaque point au moment de l'activation : cette liste est celle qu'il refuse."
                actions={
                  <Link to="/mise-en-route/recapitulatif">
                    <Button variant="secondary" size="sm" icon={<ArrowRight size={14} />}>
                      Récapitulatif
                    </Button>
                  </Link>
                }
              />
              <ul className="divide-y divide-line">
                {setup.prerequisites.map((point) => (
                  <li key={point.code} className="flex items-start gap-3 p-4">
                    {point.done ? (
                      <CheckCircle2 size={16} className="mt-0.5 shrink-0 text-success" aria-hidden />
                    ) : (
                      <AlertTriangle size={16} className="mt-0.5 shrink-0 text-warning" aria-hidden />
                    )}
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-ink">
                        {point.title}{' '}
                        {point.done ? (
                          <span className="font-normal text-ink-muted">— satisfait</span>
                        ) : (
                          <span className="font-normal text-warning">— à compléter</span>
                        )}
                      </p>
                      <p className="text-xs text-ink-muted">{point.detail || point.hint}</p>
                      {!point.done && point.hint ? <p className="text-xs text-warning">{point.hint}</p> : null}
                      {!point.done && point.screen ? (
                        <Link to={point.screen} className="text-xs font-medium text-brand hover:underline">
                          Ouvrir l'écran de l'étape
                        </Link>
                      ) : null}
                    </div>
                  </li>
                ))}
              </ul>
            </Card>
          ) : null}

          {setup.optional_settings.length > 0 ? (
            <Card>
              <CardHeader
                title="Réglages facultatifs"
                subtitle="Facultatifs ne veut pas dire inutiles : la station ouvre sans eux, et reste complétable après."
              />
              <ul className="divide-y divide-line">
                {setup.optional_settings.map((reglage) => (
                  <li key={reglage.code} className="flex flex-wrap items-start justify-between gap-3 p-4">
                    <div className="flex min-w-0 items-start gap-3">
                      {reglage.state === 'done' ? (
                        <CheckCircle2 size={16} className="mt-0.5 shrink-0 text-success" aria-hidden />
                      ) : (
                        <Circle size={16} className="mt-0.5 shrink-0 text-ink-subtle" aria-hidden />
                      )}
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-ink">
                          {reglage.title}{' '}
                          <Badge tone={reglage.state === 'done' ? 'success' : 'neutral'}>
                            {reglage.state === 'done' ? 'Renseigné' : 'À compléter'}
                          </Badge>
                        </p>
                        <p className="text-xs text-ink-muted">{reglage.detail}</p>
                        {reglage.state === 'empty' ? (
                          <p className="text-xs text-ink-muted">{reglage.suggestion}</p>
                        ) : null}
                      </div>
                    </div>
                    {reglage.state === 'empty' && reglage.screen ? (
                      <Link to={reglage.screen}>
                        <Button variant="ghost" size="sm" icon={<Settings2 size={14} />}>
                          Compléter
                        </Button>
                      </Link>
                    ) : null}
                  </li>
                ))}
              </ul>
            </Card>
          ) : null}
        </>
      ) : null}
    </div>
  )
}

export default SetupPanel
