// Types partagés avec l'API Go (les noms de champs suivent le backend).

export type Role = 'superadmin' | 'owner' | 'manager' | 'cashier' | 'washer'
/**
 * Cycle de vie d'une fiche de lavage.
 *
 * « Reçue » et « En attente » sont un seul état (`pending`) : la réception est
 * l'acte de création de la fiche. Le graphe des transitions autorisées est
 * décrit dans `src/lib/constants.ts` et appliqué par le serveur — l'interface
 * ne fait que proposer les flèches ouvertes.
 */
export type WashStatus =
  | 'pending'
  | 'in_progress'
  | 'quality_check'
  | 'ready'
  | 'completed'
  | 'canceled'
  | 'disputed'
/**
 * Famille **canonique** d'un encaissement : c'est elle, et non le libellé
 * commercial, qui décide du compteur de caisse touché. `differe` désigne un
 * engagement de paiement — jamais une somme reçue.
 */
export type PaymentMethod = 'especes' | 'mobile_money' | 'carte' | 'virement' | 'differe' | 'autre'

/** Famille déclarée d'un moyen de paiement de la station. */
export type PaymentFamily = 'cash' | 'mobile_money' | 'card' | 'transfer' | 'deferred' | 'other'

/** Un moyen de paiement accepté par la station (espèces, Orange Money, Wave…). */
export interface PaymentMethodDeclared {
  id: string
  code: string
  name: string
  family: PaymentFamily
  family_label: string
  requires_reference: boolean
  is_active: boolean
  sort_order: number
  kind: 'modele' | 'personnalisee'
}
export type PaymentKind = 'lavage' | 'acompte' | 'autre'
export type RecordStatus = 'valid' | 'voided'
/** Barème de commission : aucune, un montant par lavage, ou un pourcentage du CA. */
export type CommissionMode = 'none' | 'per_wash' | 'percent'

export interface User {
  id: string
  full_name: string
  email: string
  phone: string
  role: Role
  role_label?: string
  organization_id?: string
  station_id?: string
}

export interface Organization {
  id: string
  name: string
  slug: string
  email: string
  phone: string
  address: string
  city: string
  country: string
  currency: string
  timezone: string
  logo_url: string
  status: 'active' | 'suspended' | 'archived'
  created_at: string

  // Identité commerciale : ce que le client voit, distinct du nom légal.
  legal_name: string
  trade_name: string
  activity_type: string
  activity_description: string
  /** Date d'ouverture au format « AAAA-MM-JJ », vide si non renseignée. */
  activity_started_on: string
  whatsapp: string
  website: string

  // Facturation : à qui adresser la facture si c'est différent du siège.
  billing_name: string
  billing_address: string
  billing_email: string
  billing_phone: string

  // Mentions administratives : la valeur est ici, son état est dans `registry`.
  legal_form: string
  legal_representative: string
  rccm: string
  tax_id: string
  cnps_employer_number: string
  fiscal_regime: string
  legal_mentions: string
}

/**
 * Une mention administrative et son état.
 *
 * « Renseigné » n'est pas « vérifié » : l'entreprise saisit son RCCM, seule la
 * plateforme WashGuard peut attester l'avoir contrôlé.
 */
export interface RegistryField {
  field_code: string
  label: string
  help: string
  value: string
  status: 'empty' | 'provided' | 'pending_review' | 'verified'
  status_label: string
  verified_at: string | null
}

export interface Station {
  id: string
  organization_id: string
  name: string
  code: string
  phone: string
  address: string
  city: string
  timezone: string
  currency: string
  logo_url: string
  opening_hours: Record<string, { ouverture: string; fermeture: string; ferme?: boolean }>
  // Localisation : le quartier et les coordonnées servent à retrouver la
  // station et à l'afficher sur une carte (facultatives).
  country?: string
  district?: string
  commune?: string
  district_area?: string
  street?: string
  landmark?: string
  latitude?: string
  longitude?: string
  billing_address?: string
  settings: {
    max_cashier_discount_xof?: number
    require_open_cash_session?: boolean
    receipt_footer?: string
    /** Barème appliqué à toute l'équipe, sauf exception posée sur un compte. */
    commission_mode?: CommissionMode
    commission_per_wash_xof?: number
    commission_percent?: number
    /** Objectif de chiffre d'affaires du mois (0 ou absent = aucun objectif). */
    monthly_goal_xof?: number
  }
  is_default: boolean
  is_active: boolean
}

/**
 * Une invitation à rejoindre une entreprise existante.
 *
 * Le jeton long n'apparaît qu'une fois, à la création : il n'est pas stocké
 * côté serveur (seule son empreinte l'est), donc il ne peut pas être relu.
 */
export interface Invitation {
  id: string
  station_id: string
  email: string
  phone: string
  role: Role
  role_label: string
  /** Code court à dicter au téléphone : il désigne la même invitation. */
  code: string
  state: 'pending' | 'accepted' | 'expired' | 'revoked'
  state_label: string
  expires_at: string
  accepted_at: string
  revoked_at: string
  created_at: string
  invited_by_name: string
}

/**
 * Aperçu public d'une invitation (`GET /invitations/{code}`, sans session).
 *
 * Il ne contient **aucun identifiant** : de quoi reconnaître son employeur,
 * rien de plus.
 */
export interface InvitationPreview {
  organization_name: string
  city: string
  role: Role
  role_label: string
  state: Invitation['state']
  state_label: string
  expires_at: string
  usable: boolean
  message: string
}

/**
 * Capacités physiques déclarées d'une station.
 *
 * Les valeurs sont `null` quand la question n'a pas été posée à l'exploitant :
 * « pas d'aspiration » et « on ne sait pas encore » ne sont pas la même chose,
 * et l'écran les affiche différemment.
 */
export interface StationSiteProfile {
  wash_types: string[]
  bays: number | null
  simultaneous_vehicles: number | null
  has_vacuum: boolean | null
  has_drying: boolean | null
  has_detailing: boolean | null
  has_waiting_room: boolean | null
  has_storage: boolean | null
  water_source: string
  water_reserve_liters: number | null
  power_supply: string
  has_generator: boolean | null
  notes: string
  constraints: string
}

/** Une journée type (1 = lundi … 7 = dimanche, ISO-8601). */
export interface OpeningHour {
  weekday: number
  is_open: boolean
  opens_at: string
  closes_at: string
  break_start: string
  break_end: string
  last_checkin_at: string
  booking_allowed: boolean
}

/** Une journée particulière : fermeture exceptionnelle ou horaires réduits. */
export interface OpeningException {
  id?: string
  day: string
  is_open: boolean
  opens_at: string
  closes_at: string
  label: string
}

/** Réponse de GET /stations/{id}/profile. */
export interface StationProfile {
  station: Station
  site: StationSiteProfile
  days: OpeningHour[]
  exceptions: OpeningException[]
  types_lavage_proposes: string[]
}

export interface Subscription {
  plan_id: string
  plan_code: string
  plan_name: string
  plan_description: string
  status: 'trialing' | 'active' | 'past_due' | 'expired' | 'suspended' | 'canceled' | ''
  status_label: string
  price_xof: number
  billing_period: string
  trial_ends_at?: string | null
  current_period_start?: string | null
  current_period_end?: string | null
  max_stations: number
  max_employees: number
  max_wash_orders_month: number
  features: Record<string, boolean>
  usage: {
    period: string
    wash_orders: number
    stations: number
    employees: number
  }
  blocked_reason?: string
}

export interface Me {
  user: User
  organization: Organization | null
  station: Station | null
  /** Nul pour un compte de la plateforme (support WashGuard). */
  subscription: Subscription | null
  permissions: string[]
  /** Un compte créé par un gérant doit changer son mot de passe provisoire. */
  must_change_password?: boolean
  /**
   * État de la mise en route de la station. Absent pour les profils qui ne
   * configurent rien (caissier, laveur) : ils ne sont jamais détournés vers le
   * parcours.
   */
  onboarding?: Onboarding | null
}

/** Une étape de la mise en route. « done » est calculé par le serveur. */
export interface OnboardingStep {
  code: string
  title: string
  description: string
  required: boolean
  done: boolean
  state: 'not_started' | 'in_progress' | 'completed' | 'needs_fix' | 'blocked'
  /** Ce qui est constaté en base (« 3 prestations actives »…). */
  detail: string
  /** Écran qui permet de compléter l'étape (vide si elle est déjà satisfaite). */
  screen: string
  /** Ce qu'il faut faire quand l'étape n'est pas satisfaite. */
  hint: string
  /** Formulaire à afficher (vide quand l'étape est un simple constat). */
  form: string
  /** Choix descriptifs enregistrés (jamais un indicateur « fait »). */
  data: Record<string, unknown>
}

export interface OnboardingMissing {
  code: string
  title: string
  hint: string
  screen: string
  description: string
}

export interface Onboarding {
  organization_id: string
  state: 'pending' | 'in_progress' | 'completed' | 'active'
  progress: {
    done: number
    total: number
    percent: number
    steps_done: number
    steps_total: number
  }
  steps: OnboardingStep[]
  missing: OnboardingMissing[]
  /** Vrai quand toutes les étapes obligatoires sont satisfaites. */
  can_activate: boolean
  started_at: string
  activated_at: string
  /** Bloc « mise en route » : présents sur /onboarding/summary et l'activation. */
  setup?: SetupStation
  organization?: {
    id: string
    name: string
    legal_name: string
    email: string
    phone: string
    city: string
    country: string
    currency: string
    timezone: string
    status: string
    rccm: string
    tax_id: string
  }
  /** Qui a activé la station, quand, et sur quels constats. */
  activated_by?: {
    activated_at: string
    activated_by: string
    activated_by_name: string
    snapshot: Record<string, unknown>
  }
  /** Résultat d'un appel à l'activation (absents d'une simple lecture). */
  activated?: boolean
  already_active?: boolean
  message?: string
}

/** Récapitulatif complet : l'état du parcours **avec** son bloc de mise en route. */
export interface OnboardingSummary extends Onboarding {
  setup: SetupStation
}

/**
 * État d'ouverture d'une journée, calculé par le serveur.
 *
 * `state` dit pourquoi : ouvert, en pause, avant l'ouverture, après la
 * fermeture, journée fermée, ou journée déclarée ouverte sans heures (un
 * réglage à finir, qui ne compte pas comme une journée d'ouverture).
 */
export interface SetupOpeningDay {
  weekday: number
  day_label: string
  date: string
  declared: boolean
  open_today: boolean
  opens_at: string
  closes_at: string
  break_start: string
  break_end: string
  last_checkin_at: string
  booking_allowed: boolean
  is_open_now: boolean
  state: 'open' | 'break' | 'before_open' | 'after_close' | 'closed_today' | 'hours_missing'
  label: string
  next_change_at: string
}

/** Un élément obligatoire du parcours, avec ce qu'il faut faire et où. */
export interface SetupRequirement {
  code: string
  title: string
  description?: string
  detail: string
  hint: string
  screen: string
  required: boolean
  done: boolean
}

/** Un réglage facultatif : fait ou vide, jamais « en cours ». */
export interface SetupOptionalSetting {
  code: string
  title: string
  state: 'done' | 'empty'
  detail: string
  screen: string
  required: boolean
  suggestion: string
}

/** Un bloc d'administration, proposé seulement si le profil peut l'ouvrir. */
export interface SetupBlock {
  code: string
  label: string
  permission: string
  href: string
  block: string
}

/** Un raccourci d'exploitation, proposé seulement si le profil en a l'usage. */
export interface SetupShortcut {
  code: string
  label: string
  href: string
  permission: string
  hint: string
}

/**
 * Bloc « mise en route » d'une station.
 *
 * Le même objet alimente le récapitulatif de mise en route et le tableau de
 * bord : un seul calcul côté serveur, donc deux écrans qui ne peuvent pas
 * raconter deux histoires différentes sur la même station.
 *
 * Aucun montant n'y figure : les chiffres du jour vivent dans le rapport
 * (`today`), et seulement s'il y a une fiche à compter.
 */
export interface SetupStation {
  role: string
  can: {
    manage_settings: boolean
    manage_organization: boolean
    see_amounts: boolean
    create_wash: boolean
  }
  blocks: SetupBlock[]
  shortcuts: SetupShortcut[]
  station: {
    id: string
    name: string
    code: string
    phone: string
    city: string
    locality: string
    location: {
      city: string
      commune: string
      district: string
      street: string
      landmark: string
      latitude: number | null
      longitude: number | null
      geolocated: boolean
      label: string
      missing: string[]
    }
    currency: string
    timezone: string
    is_active: boolean
    created_at: string
  }
  hours: {
    today: SetupOpeningDay
    days_open: number
    declared_days: number
    label: string
  }
  activation: {
    state: 'pending' | 'in_progress' | 'completed' | 'active'
    active: boolean
    state_label: string
    can_activate: boolean
    blockers: SetupRequirement[]
    blockers_count: number
    required_count: number
    required_done: number
  }
  prerequisites: SetupRequirement[]
  missing: SetupRequirement[]
  optional_settings: SetupOptionalSetting[]
  catalogue: {
    categories_active: number
    usages_active: number
    usage_categories_active: number
    services_active: number
    services_sellable: number
    services_priced: number
    services_sellable_priced: number
    services_without_usable_price: number
    options_active: number
    price_rules_active: number
    payment_methods_active: number
    team_active: number
    equipment_active: number
    equipment_out_of_service: number
    stock_items_active: number
    stock_items_uncounted: number
    stock_items_out: number
    stock_items_low: number
    stock_items_expired: number
  }
  activity: {
    has_activity: boolean
    orders_total: number
    orders_open: number
    orders_canceled: number
    first_order_at: string
    last_order_ref: string
    last_order_state: string
    last_order_label: string
    last_order_day: string
    last_order_at: string
  }
  readiness: {
    statements: string[]
    catalog_ready: boolean
    no_orders: boolean
  }
}

export interface Service {
  id: string
  organization_id: string
  station_id: string
  name: string
  category: string
  description: string
  /**
   * Tarif **du catalogue** : le repli de la grille tarifaire, pas la vérité
   * d'une vente. Le prix réellement appliqué vient du serveur (`price_source`).
   */
  price_xof: number
  duration_minutes: number
  sort_order: number
  is_active: boolean
  /** Code stable de la prestation (issu du référentiel ou dérivé du nom). */
  code: string
  family_code: string
  family_name: string
  /** Code du modèle de référence quand la prestation en vient. */
  template_code: string
  /** Ce que la prestation comprend, affiché tel quel au comptoir. */
  includes: string[]
  equipment_notes: string
  safety_notes: string
  preconditions: string
  /** Gratuité décidée par l'exploitant : la vente est à zéro, volontairement. */
  is_free: boolean
  /** Prestation chiffrée sur devis : elle ne se vend pas au comptoir. */
  is_quote_only: boolean
  /** Aucun tarif utilisable : la caisse refusera la vente (`price_required`). */
  needs_price: boolean
}

/** Une famille de prestations (extérieur, intérieur, formules…). */
export interface ServiceFamily {
  id: string
  code: string
  kind: 'modele' | 'personnalisee'
  name: string
  description: string
  sort_order: number
  is_active: boolean
}

/** Une prestation du référentiel du produit — **sans prix**. */
export interface ServiceTemplate {
  code: string
  name: string
  family_code: string
  family_name: string
  description: string
  includes: string[]
  duration_minutes: number
  equipment_notes: string
  safety_notes: string
  preconditions: string
  is_quote_only: boolean
  category_codes: string[]
  /** La station a-t-elle déjà repris ce modèle dans son catalogue ? */
  adopted: boolean
  service_id: string
  price_xof: number
}

export interface ServiceTemplateCatalogue {
  station_id: string
  families: Array<{
    code: string
    name: string
    description: string
    local_name: string
    is_active: boolean
    configured: boolean
  }>
  templates: ServiceTemplate[]
  prix: string
}

/** Option ou supplément facturé en plus d'une prestation. */
export interface ServiceOption {
  id: string
  service_id: string | null
  category_code: string
  name: string
  description: string
  price_xof: number
  duration_minutes: number
  sort_order: number
  is_active: boolean
}

/** Une règle de prix : ce qui décide du montant d'une ligne. */
export interface PriceRule {
  id: string
  service_id: string
  service_name: string
  category_code: string
  category_name: string
  usage_code: string
  usage_name: string
  customer_id: string | null
  customer_name: string
  price_xof: number
  valid_from: string | null
  valid_to: string | null
  reason: string
  created_by_name: string
  is_active: boolean
  /** Comment la règle se classe dans la hiérarchie des prix. */
  specificity_label: string
}

export interface PriceCell {
  category_code: string
  usage_code?: string
  price_xof: number | null
  rule_id: string
  source: string
  is_free?: boolean
}

export interface PriceMatrixRow {
  service_id: string
  name: string
  family_code: string
  family_name: string
  duration_minutes: number
  generic_price_xof: number
  is_free: boolean
  is_quote_only: boolean
  needs_price: boolean
  cells: PriceCell[]
  usage_cells: PriceCell[]
}

export interface PriceMatrix {
  station_id: string
  services: PriceMatrixRow[]
  categories: Array<{ code: string; name: string; family: string }>
  usages: Array<{ code: string; name: string; family: string }>
  generique_autorise: boolean
  a_tarifer: number
  message: string
}

/** Ce que le serveur répond pour une ligne, avant enregistrement de la fiche. */
export interface PricePreview {
  service_id: string
  service_name: string
  is_free: boolean
  is_quote_only: boolean
  date: string
  price_xof: number | null
  price_source: string
  price_source_label: string
  price_rule_id?: string
  needs_price: boolean
  message?: string
  options: Array<{
    id: string
    name: string
    description: string
    price_xof: number
    service_id: string | null
    category_code: string
    duration_minutes: number
    scope: 'station' | 'prestation' | 'vehicule'
  }>
}

export interface VehicleCategory {
  id: string
  /** Code stable du référentiel (ou code local pour une catégorie créée). */
  code?: string
  /** `modele` : entrée du référentiel. `personnalisee` : créée par la station. */
  kind?: 'modele' | 'personnalisee'
  name: string
  description: string
  family?: string
  aliases?: string[]
  pricing_class?: string
  sort_order: number
  is_active: boolean
  /** Vrai quand le nom vient encore du référentiel (aucun renommage local). */
  name_from_template?: boolean
}

/** Ce que le client fait de son véhicule — référentiel distinct des catégories. */
export interface VehicleUsage {
  id: string
  code: string
  kind?: 'modele' | 'personnalisee'
  name: string
  family: string
  description: string
  aliases: string[]
  clientele?: string
  sort_order: number
  is_active: boolean
}

/** Les deux listes que la caisse propose, servies en un seul appel. */
export interface VehicleOptions {
  categories: VehicleCategory[]
  usages: VehicleUsage[]
}

/** Une entrée du référentiel, avec ce que la station en a fait. */
export interface VehicleCatalogueEntry {
  code: string
  name: string
  family: string
  description: string
  aliases: string[]
  sort_order: number
  is_active: boolean
  configured: boolean
  local_name?: string
}

export interface VehicleCatalogue {
  station_id: string
  categories: VehicleCatalogueEntry[]
  usages: VehicleCatalogueEntry[]
  actives: number
  usages_actifs: number
  familles_categories: string[]
  familles_usages: string[]
  message_referentiel: string
}

/**
 * Une combinaison (catégorie, usage) proposée par la station. Sans prix : la
 * grille tarifaire par couple arrive avec les tarifs, l'application n'invente
 * aucun montant.
 */
export interface UsageCategorie {
  usage_code: string
  category_code: string
  is_active: boolean
  sort_order: number
}

export interface Customer {
  id: string
  full_name: string
  phone: string
  plate: string
  vehicle_model: string
  notes: string
  is_active: boolean
  created_at: string
  wash_count: number
  total_spent_xof: number
  last_visit_at?: string | null
}

export interface CustomerDetail {
  customer: Customer
  history: Array<{
    id: string
    reference: string
    created_at: string
    status: WashStatus
    total_xof: number
    paid_xof: number
    due_xof: number
    vehicle_category_name: string
    plate: string
    services: string
  }>
}

/**
 * Un matériel de la station. Aucun champ n'est obligatoire au-delà du nom :
 * une date inconnue reste vide, et l'écran écrit « non renseigné » — jamais
 * une date inventée.
 */
export interface Equipment {
  id: string
  station_id: string
  template_code?: string | null
  name: string
  family: string
  family_label: string
  quantity: number
  state: EquipmentState
  state_label: string
  acquired_on?: string | null
  last_service_on?: string | null
  next_service_on?: string | null
  cost_xof: number
  notes: string
  is_active: boolean
  /** Maintenance à échéance (date de passage atteinte). */
  service_due: boolean
}

export type EquipmentState = 'neuf' | 'bon' | 'a_reparer' | 'hors_service'

export interface EquipmentTemplate {
  code: string
  name: string
  family: string
  family_label: string
  sort_order: number
}

export interface EquipmentResponse {
  station_id: string
  templates: EquipmentTemplate[]
  items: Equipment[]
  families: Record<string, string>
  states: Record<string, string>
  summary: {
    total: number
    en_service: number
    a_reparer: number
    hors_service: number
    retires: number
    valeur_xof: number
    maintenance_a_prevoir: number
  }
}

/**
 * Un article de stock. `quantity_known` est la colonne qui compte :
 * `false` veut dire « personne n'a encore compté » — ce n'est pas zéro, et
 * aucune alerte de rupture n'est affichée dans ce cas.
 */
export interface StockItem {
  id: string
  station_id: string
  category_code: string
  category_name: string
  name: string
  internal_code: string
  unit: string
  quantity_known: boolean
  quantity: number
  unit_cost_xof: number
  min_quantity?: number | null
  supplier: string
  location: string
  expires_on?: string | null
  notes: string
  is_active: boolean
  /** Calculée sur les seules quantités connues (0 sinon). */
  value_xof: number
  state: 'inconnu' | 'rupture' | 'seuil' | 'peremption' | 'ok'
  state_label: string
  expires_soon: boolean
  movements_count: number
}

export interface StockCategory {
  code: string
  name: string
  sort_order: number
  is_active: boolean
}

export interface StockResponse {
  station_id: string
  categories: StockCategory[]
  items: StockItem[]
  summary: {
    articles: number
    rupture: number
    sous_seuil: number
    quantite_inconnue: number
    valeur_xof: number
  }
}

export type StockMovementKind = 'entree' | 'sortie' | 'inventaire' | 'ajustement' | 'perte'

export interface StockMovement {
  id: string
  kind: StockMovementKind
  kind_label: string
  delta: number
  counted?: number | null
  /** `null` tant que la quantité n'est pas connue : on n'invente pas un chiffre. */
  quantity_after?: number | null
  quantity_known: boolean
  unit_cost_xof: number
  reason: string
  created_by_name: string
  created_at: string
}

/** Règles de fonctionnement des commandes d'une station. */
export interface OrderRules {
  allow_walk_in: boolean
  require_washer: boolean
  quality_check_required: boolean
  release_requires_payment: boolean
  allow_partial_payments: boolean
  require_cancel_reason: boolean
  max_open_orders_per_washer: number
}

export interface OrderRulesResponse {
  station_id: string
  rules: OrderRules
  labels: Record<string, string>
  statuses?: WashStatusInfo[]
}

export interface WashStatusInfo {
  code: WashStatus
  label: string
  terminal: boolean
  description: string
}

/** Situation RH d'un employé — distincte de son accès applicatif. */
export type EmploymentStatus = 'active' | 'on_leave' | 'departed'

export interface EmployeeCompensation {
  mode: '' | 'none' | 'per_wash' | 'percent'
  mode_label: string
  per_wash_xof?: number | null
  percent?: number | null
}

export interface Employee {
  id: string
  full_name: string
  email: string
  phone: string
  role: Role
  role_label?: string
  station_id?: string
  station_name?: string
  is_active: boolean
  last_login_at?: string | null
  created_at: string
  washes_today: number
  washes_month: number
  revenue_today_xof: number
  revenue_month_xof: number
  collected_month_xof: number
  /**
   * Barème propre à cet employé. `null` = il suit le barème de la station
   * (voir Paramètres) : aucune valeur n'est recopiée d'un côté à l'autre.
   */
  commission_mode?: CommissionMode | null
  commission_per_wash_xof?: number | null
  commission_percent?: number | null
  /** Fonction exercée (« Caissière principale »), facultative. */
  job_title?: string
  /** Date d'arrivée (AAAA-MM-JJ), facultative. */
  hired_on?: string | null
  /** Horaires de la personne, texte libre : descriptif, pas un planning. */
  work_schedule?: string
  employment_status?: EmploymentStatus
  employment_status_label?: string
  /**
   * Données de rémunération. `null` (ou absent) quand l'appelant n'a pas la
   * permission dédiée : l'écran le dit au lieu d'afficher des zéros.
   */
  compensation?: EmployeeCompensation | null
}

export interface WashOrderItem {
  id?: string
  service_id?: string
  service_name: string
  unit_price_xof: number
  quantity: number
  line_total_xof: number
  /** D'où vient le prix appliqué : `service_category`, `client_contract`… */
  price_source?: string
  price_source_label?: string
  /** Options retenues au moment de la vente, avec leur prix d'alors. */
  options?: Array<{ option_id: string; name: string; price_xof: number; quantity: number; total_xof: number }>
  options_total_xof?: number
}

export interface WashOrderPayment {
  id: string
  /** Famille comptable de l'encaissement (elle seule compte dans les totaux). */
  method: PaymentMethod
  amount_xof: number
  reference: string
  paid_at: string
  received_by_name: string
  status: RecordStatus
  note: string
  method_label?: string
  /** Moyen déclaré par la station (« Orange Money », « Wave »…), s'il existe. */
  payment_method_id?: string | null
  payment_method_name?: string
  /** Motif d'annulation, renseigné sur un encaissement annulé. */
  void_reason?: string
}

export interface WashOrder {
  id: string
  reference: string
  station_id: string
  station_name?: string
  customer_id?: string
  customer_name: string
  vehicle_category_id?: string
  vehicle_category_name: string
  /** Usage commercial : ce que le client fait de son véhicule. */
  vehicle_usage_id?: string | null
  vehicle_usage_name: string
  plate: string
  employee_id?: string
  employee_name: string
  status: WashStatus
  subtotal_xof: number
  discount_xof: number
  discount_reason: string
  total_xof: number
  paid_xof: number
  due_xof: number
  notes: string
  cancel_reason?: string
  created_by?: string
  created_by_name?: string
  status_label?: string
  business_day?: string
  payment_count?: number
  created_at: string
  completed_at?: string | null
  canceled_at?: string | null
  items?: WashOrderItem[]
  payments?: WashOrderPayment[]
  services_label?: string
  payment_methods?: PaymentMethod[]
  incident_note?: string
  incident_reported_at?: string
  incident_reporter_name?: string
}

export interface Paginated<T> {
  items: T[]
  total: number
  page?: number
  per_page?: number
}

export interface DashboardData {
  date: string
  station_id: string
  /**
   * Montants du jour. **Absents** quand la station n'a encore enregistré
   * aucune fiche (`no_activity`) ou quand le profil n'a pas accès aux
   * paiements : le serveur ne les envoie pas, plutôt que de les masquer.
   */
  today?: Totals
  series?: DaySeries[]
  cash: {
    is_open: boolean
    session_id?: string
    opened_at?: string
    opened_by_name?: string
    opening_float_xof: number
    cash_in_xof: number
    mobile_money_xof: number
    expenses_xof: number
    expected_cash_xof: number
  } | null
  employees?: Array<{
    employee_id: string
    employee_name: string
    wash_count: number
    revenue_xof: number
    collected_xof: number
  }>
  recent: Array<{
    type: 'wash' | 'payment' | 'expense'
    label: string
    amount_xof: number
    at: string
    actor: string
    status: string
    reference?: string
  }>
  top_services?: Array<{ service_name: string; quantity: number; revenue_xof: number }>
  payment_methods?: Array<{ method: PaymentMethod; amount_xof: number; count: number }>
  alerts: Array<{ level: 'info' | 'warning' | 'danger'; message: string }>
  /** Objectif du mois : `null` tant que la station n'en a pas fixé. */
  goal?: MonthlyGoal | null
  definitions?: Record<string, string>
  /** État de la station : identité, horaires, catalogue, prérequis, rôles. */
  setup?: SetupStation
  /** Vrai quand la station n'a encore enregistré aucune fiche. */
  no_activity?: boolean
  no_activity_note?: string
  /** Vrai quand le profil n'a pas accès aux montants. */
  amounts_hidden?: boolean
  amounts_note?: string
}

export interface Totals {
  wash_count: number
  canceled_count: number
  service_count: number
  revenue_xof: number
  discount_xof: number
  payments_xof: number
  cash_xof: number
  mobile_money_xof: number
  other_xof: number
  expenses_xof: number
  expenses_cash_xof: number
  result_xof: number
  cash_result_xof: number
  due_xof: number
  average_ticket_xof: number
  voided_payments_xof: number
}

export interface DaySeries {
  day: string
  wash_count: number
  revenue_xof: number
  payments_xof: number
  expenses_xof: number
  result_xof: number
}

export interface CashSession {
  id: string
  station_id: string
  station_name?: string
  status: 'open' | 'closed'
  opened_by_name: string
  opened_at: string
  opening_float_xof: number
  closed_by_name: string
  closed_at?: string | null
  counted_cash_xof?: number | null
  expected_cash_xof: number
  difference_xof?: number | null
  cash_in_xof: number
  mobile_money_xof: number
  other_in_xof: number
  expenses_xof: number
  operations: number
  payment_count: number
  expense_count: number
  notes: string
  close_notes: string
  reopen_count: number
}

export interface CashMovement {
  kind: 'payment' | 'expense'
  id: string
  label: string
  amount_xof: number
  method: PaymentMethod
  method_label?: string
  at: string
  actor: string
  status: RecordStatus
  reference?: string
}

export interface PaymentRow {
  id: string
  station_id: string
  station_name: string
  kind: PaymentKind
  method: PaymentMethod
  amount_xof: number
  reference: string
  note: string
  wash_order_reference?: string
  wash_order_id?: string
  customer_name?: string
  received_by_name: string
  method_label?: string
  /** Moyen déclaré par la station (« Orange Money ») ; vide pour l'historique. */
  payment_method_name?: string
  payment_method_id?: string | null
  paid_at: string
  business_day?: string
  status: RecordStatus
  void_reason?: string
  cash_session_id?: string
  session_open: boolean
}

/**
 * Paramètres de caisse d'une station. Chaque champ est appliqué par le serveur :
 * un réglage affiché ici et ignoré à l'encaissement serait un mensonge.
 */
export interface CashSettings {
  require_open_cash_session: boolean
  max_cashier_discount_xof: number
  default_float_xof: number
  discrepancy_tolerance_xof: number
  allowed_roles: Role[]
  allow_void: boolean
  receipt_prefix: string
  receipt_footer: string
  receipt_legal_name: string
  receipt_tax_id: string
  receipt_logo_url: string
}

/**
 * Justificatif numérique d'une fiche soldée. Rien n'y est inventé : les
 * mentions absentes sont listées dans `missing_mentions` plutôt que remplacées
 * par une valeur d'exemple.
 */
export interface Receipt {
  number: string
  issued_at: string
  station_name: string
  station_city: string
  station_phone: string
  legal_name: string
  tax_id: string
  logo_url: string
  footer: string
  order_reference: string
  business_day: string
  customer_name: string
  plate: string
  vehicle_category_name: string
  employee_name: string
  lines: Array<{
    service_name: string
    vehicle_category_name: string
    quantity: number
    unit_price_xof: number
    total_xof: number
  }>
  subtotal_xof: number
  discount_xof: number
  discount_reason: string
  total_xof: number
  paid_xof: number
  due_xof: number
  payments: Array<{
    amount_xof: number
    method: PaymentMethod
    method_label: string
    reference: string
    paid_at: string
    received_by_name: string
  }>
  missing_mentions: string[]
}

export interface Expense {
  id: string
  station_id: string
  station_name?: string
  category: string
  label: string
  amount_xof: number
  method: PaymentMethod
  supplier: string
  reference: string
  note: string
  spent_at: string
  recorded_by_name: string
  method_label?: string
  business_day?: string
  status: RecordStatus
  void_reason?: string
  cash_session_id?: string
  session_open: boolean
  created_at: string
}

export interface ReportOverview {
  from: string
  to: string
  days: number
  station_id: string
  station_name: string
  totals: Totals
  series: DaySeries[]
  by_service: Array<{ service_name: string; quantity: number; revenue_xof: number }>
  by_employee: Array<{ employee_id: string; employee_name: string; wash_count: number; revenue_xof: number; collected_xof: number }>
  by_method: Array<{ method: PaymentMethod; amount_xof: number; count: number }>
  by_expense_category: Array<{ category: string; label: string; amount_xof: number; count: number }>
  by_vehicle_category: Array<{ vehicle_category_name: string; wash_count: number; revenue_xof: number }>
  cash_sessions: Array<{
    id: string
    opened_at: string
    closed_at?: string | null
    opened_by_name: string
    closed_by_name?: string
    status: 'open' | 'closed'
    opening_float_xof: number
    cash_in_xof: number
    expenses_xof: number
    expected_cash_xof: number
    counted_cash_xof?: number | null
    difference_xof?: number | null
  }>
  top_customers: Array<{ customer_id: string; customer_name: string; wash_count: number; revenue_xof: number }>
  definitions: Record<string, string>
}

/** Barème renvoyé par l'API pour une période (station + exception employé). */
export interface CommissionSettings {
  mode: CommissionMode
  per_wash_xof: number
  percent: number
  monthly_goal_xof: number
}

/**
 * Avancement de l'objectif du **mois en cours** — quel que soit l'historique
 * affiché à côté. Absent (null) si la station n'a fixé aucun objectif : rien
 * n'est inventé.
 */
export interface MonthlyGoal {
  /** Mois concerné, au format AAAA-MM. */
  month: string
  target_xof: number
  achieved_xof: number
  remaining_xof: number
  percent: number
  days_left: number
  reached: boolean
  /** Repère indicatif (« il vous reste X par jour »), absent si déjà atteint. */
  needed_per_day_xof?: number
}

/** Une ligne de l'état de commissions : un laveur et ce qu'on lui doit. */
export interface CommissionItem {
  /** Vide pour les lavages sans laveur attribué (« Non attribué »). */
  employee_id: string
  employee_name: string
  role: string
  role_label: string
  wash_count: number
  revenue_xof: number
  mode: CommissionMode
  per_wash_xof: number
  percent: number
  /** Vrai si le taux vient d'une exception posée sur le compte de l'employé. */
  rate_from_employee: boolean
  commission_xof: number
}

export interface CommissionReport {
  /** Dates déjà formatées en jj/mm/aaaa par le serveur. */
  period: { from: string; to: string }
  settings: CommissionSettings
  goal: MonthlyGoal | null
  items: CommissionItem[]
  totals: {
    employee_count: number
    wash_count: number
    revenue_xof: number
    commission_xof: number
  }
}

export interface AuditEntry {
  id: number
  action: string
  entity: string
  entity_id: string
  actor_name: string
  details: Record<string, unknown>
  station_id?: string
  created_at: string
}

export interface Plan {
  id: string
  code: string
  name: string
  description: string
  price_xof: number
  billing_period: string
  max_stations: number
  max_employees: number
  max_wash_orders_month: number
  trial_days: number
  features: Record<string, boolean>
  sort_order: number
  is_active: boolean
  organization_count: number
}

export interface AdminOverview {
  organizations: number
  active_organizations: number
  suspended_organizations: number
  stations: number
  users: number
  wash_orders_month: number
  revenue_month_xof: number
  mrr_xof: number
  plans: Array<{ plan_id: string; plan_name: string; price_xof: number; count: number }>
  recent_organizations: Array<{
    id: string
    name: string
    city: string
    status: string
    created_at: string
    plan_name: string
    subscription_status: string
    subscription_status_label?: string
  }>
}

export interface AdminOrganization {
  id: string
  name: string
  slug: string
  city: string
  phone: string
  email: string
  status: string
  created_at: string
  plan_id: string
  plan_name: string
  subscription_status: string
  subscription_status_label?: string
  trial_ends_at?: string | null
  station_count: number
  employee_count: number
  wash_orders_month: number
  owner_name: string
  owner_email: string
}
