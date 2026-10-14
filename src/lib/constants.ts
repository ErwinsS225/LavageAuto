import { formatXOF } from '@/lib/format'
import type { CommissionMode, PaymentMethod, Role, WashStatus } from '@/types'

export interface Labeled {
  id: string
  label: string
}

/** Moyens de paiement enregistrés par le personnel (saisie manuelle). */
/**
 * Libellés des **familles** de paiement (colonne `payment_methods.family`).
 *
 * Ce n'est pas le référentiel des moyens : chaque station déclare les siens
 * (« Orange Money », « Wave », « Chèque »…) dans Paramètres → Caisse et reçus,
 * et les écrans lisent `payment_method_name`. Cette table ne sert que de repli
 * pour afficher un encaissement ancien, enregistré avant l'introduction du
 * référentiel, ou pour nommer la famille sous un moyen.
 */
const methodLabels: Record<string, string> = {
  especes: 'Espèces',
  mobile_money: 'Mobile Money',
  carte: 'Carte bancaire',
  virement: 'Virement',
  differe: 'Paiement différé',
  autre: 'Autre',
}

export function methodLabel(method: PaymentMethod | string): string {
  return methodLabels[method] ?? method
}

/** Statuts d'une fiche de lavage. */
export const WASH_STATUSES: Array<{ id: WashStatus; label: string; tone: string }> = [
  { id: 'pending', label: 'En attente', tone: 'badge-warning' },
  { id: 'in_progress', label: 'En cours', tone: 'badge-brand' },
  { id: 'completed', label: 'Terminé', tone: 'badge-success' },
  { id: 'canceled', label: 'Annulé', tone: 'badge-danger' },
]

export function washStatusLabel(status: WashStatus | string): string {
  return WASH_STATUSES.find((item) => item.id === status)?.label ?? status
}

export function washStatusTone(status: WashStatus | string): string {
  return WASH_STATUSES.find((item) => item.id === status)?.tone ?? 'badge-neutral'
}

/** Catégories de prestations. */
export const SERVICE_CATEGORIES: Labeled[] = [
  { id: 'lavage', label: 'Lavage' },
  { id: 'interieur', label: 'Intérieur' },
  { id: 'moteur', label: 'Moteur' },
  { id: 'finition', label: 'Finition' },
  { id: 'autre', label: 'Autre' },
]

export function serviceCategoryLabel(category: string): string {
  return SERVICE_CATEGORIES.find((item) => item.id === category)?.label ?? category
}

/** Catégories de dépenses (identiques à celles du backend). */
export const EXPENSE_CATEGORIES: Labeled[] = [
  { id: 'produits', label: 'Produits de lavage' },
  { id: 'eau', label: 'Eau' },
  { id: 'electricite', label: 'Électricité' },
  { id: 'salaires', label: 'Salaires' },
  { id: 'loyer', label: 'Loyer' },
  { id: 'entretien', label: 'Entretien et réparations' },
  { id: 'equipement', label: 'Équipement' },
  { id: 'transport', label: 'Transport' },
  { id: 'taxes', label: 'Taxes et impôts' },
  { id: 'autre', label: 'Autre' },
]

export function expenseCategoryLabel(category: string): string {
  return EXPENSE_CATEGORIES.find((item) => item.id === category)?.label ?? category
}

/**
 * Barèmes de commission des laveurs (mêmes identifiants que le backend).
 *
 * Un seul barème s'applique à la fois : le réglage de la station, ou
 * l'exception posée sur un compte employé. Rien n'est recopié de l'un vers
 * l'autre — une exception suit le compte, un changement de tarif suit la station.
 */
export const COMMISSION_MODES: Array<{ id: CommissionMode; label: string; description: string }> = [
  {
    id: 'none',
    label: 'Aucune commission',
    description: 'La station ne calcule rien à verser aux laveurs.',
  },
  {
    id: 'per_wash',
    label: 'Montant par lavage',
    description: 'Un montant fixe pour chaque lavage terminé.',
  },
  {
    id: 'percent',
    label: 'Pourcentage du chiffre d’affaires',
    description: 'Un pourcentage du chiffre d’affaires des lavages terminés.',
  },
]

export function commissionModeLabel(mode: CommissionMode | string): string {
  return COMMISSION_MODES.find((item) => item.id === mode)?.label ?? mode
}

export function commissionModeDescription(mode: CommissionMode | string): string {
  return COMMISSION_MODES.find((item) => item.id === mode)?.description ?? ''
}

/** « 500 FCFA par lavage », « 20 % du chiffre d’affaires », « Aucune commission ». */
export function commissionRateLabel(
  mode: CommissionMode | string,
  perWashXof?: number | null,
  percent?: number | null,
): string {
  if (mode === 'per_wash') return `${formatXOF(perWashXof ?? 0)} par lavage`
  if (mode === 'percent') return `${percent ?? 0} % du chiffre d’affaires`
  return 'Aucune commission'
}

/**
 * Choix proposés sur la fiche d'un employé : « station » vient en premier, car
 * c'est le cas normal — l'exception doit rester une exception.
 */
export const EMPLOYEE_COMMISSION_MODES: Array<{ value: string; label: string }> = [
  { value: 'station', label: 'Barème de la station' },
  { value: 'none', label: 'Aucune commission' },
  { value: 'per_wash', label: 'Montant par lavage propre' },
  { value: 'percent', label: 'Pourcentage propre' },
]

/** Rôles opérationnels et d'administration. */
export const ROLES: Array<{ id: Role; label: string; description: string }> = [
  { id: 'owner', label: 'Propriétaire', description: 'Accès complet à la station, aux tarifs, aux finances et à l’abonnement.' },
  { id: 'manager', label: 'Gérant', description: 'Supervise l’activité, affecte les laveurs, valide les dépenses et suit les performances.' },
  { id: 'cashier', label: 'Caissier', description: 'Accueille les clients, enregistre les fiches de lavage et encaisse les paiements.' },
  { id: 'washer', label: 'Laveur', description: 'Prend en charge les véhicules attribués, met à jour l’avancement et signale les incidents.' },
]

export function roleLabel(role: Role | string): string {
  return ROLES.find((item) => item.id === role)?.label ?? role
}

export const CURRENCY = 'FCFA'
export const COUNTRY = "Côte d'Ivoire"
export const DEFAULT_CITY = 'Abidjan'

/** Types d'export CSV proposés dans les rapports. */
export const EXPORT_TYPES: Array<Labeled & { description: string }> = [
  { id: 'daily', label: 'Activité journalière', description: 'Chiffre d’affaires, encaissements, dépenses et résultat par jour.' },
  { id: 'wash_orders', label: 'Fiches de lavage', description: 'Détail de chaque lavage avec les prestations et les montants.' },
  { id: 'payments', label: 'Encaissements', description: 'Tous les paiements reçus, avec le moyen de paiement.' },
  { id: 'expenses', label: 'Dépenses', description: 'Dépenses par catégorie, motif et moyen de paiement.' },
  { id: 'cash_sessions', label: 'Historique de caisse', description: 'Ouvertures, clôtures, soldes théoriques et écarts.' },
  { id: 'services', label: 'Prestations les plus vendues', description: 'Quantités et chiffre d’affaires par prestation.' },
  { id: 'employees', label: 'Activité par employé', description: 'Lavages enregistrés et encaissements par employé.' },
]

/** Pays proposés à l'inscription (liste courte, extensible). */
export const COUNTRIES: string[] = [
  'Côte d’Ivoire',
  'Sénégal',
  'Burkina Faso',
  'Mali',
  'Bénin',
  'Togo',
  'Guinée',
  'Cameroun',
]

/** Fuseaux usuels de la zone d'activité. */
export const TIMEZONES: string[] = [
  'Africa/Abidjan',
  'Africa/Dakar',
  'Africa/Ouagadougou',
  'Africa/Bamako',
  'Africa/Porto-Novo',
  'Africa/Lome',
  'Africa/Conakry',
  'Africa/Douala',
]
