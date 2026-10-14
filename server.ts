import express, { Request, Response, NextFunction } from 'express'
import cookieParser from 'cookie-parser'
import cors from 'cors'
import path from 'path'
import { fileURLToPath } from 'url'
import fs from 'fs'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

const app = express()
const PORT = 3000
const HOST = '0.0.0.0'

app.use(express.json())
app.use(cookieParser())
app.use(cors({ origin: true, credentials: true }))

// ---------------------------------------------------------------------------
// DATA MODEL & IN-MEMORY STORE
// ---------------------------------------------------------------------------

interface Plan {
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
  organization_count?: number
}

interface User {
  id: string
  organization_id?: string
  station_id?: string
  full_name: string
  email: string
  phone: string
  password_hash: string
  role: 'superadmin' | 'owner' | 'manager' | 'cashier' | 'washer'
  is_active: boolean
  must_change_password: boolean
  last_login_at?: string | null
  created_at: string
  commission_mode?: 'none' | 'per_wash' | 'percent' | null
  commission_per_wash_xof?: number | null
  commission_percent?: number | null
}

interface Organization {
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
  legal_name?: string
  trade_name?: string
  activity_type?: string
  activity_description?: string
  activity_started_on?: string
  whatsapp?: string
  website?: string
  billing_name?: string
  billing_address?: string
  billing_email?: string
  billing_phone?: string
  legal_form?: string
  legal_representative?: string
  rccm?: string
  tax_id?: string
  cnps_employer_number?: string
  fiscal_regime?: string
  legal_mentions?: string
  notes?: string
  created_at: string
}

interface StationSite {
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

interface OpeningHourDay {
  weekday: number
  is_open: boolean
  opens_at: string
  closes_at: string
  break_start: string
  break_end: string
  last_checkin_at: string
  booking_allowed: boolean
}

interface OpeningExceptionItem {
  id?: string
  day: string
  is_open: boolean
  opens_at: string
  closes_at: string
  label: string
}

interface DeclaredPaymentMethod {
  id: string
  station_id?: string
  code: string
  name: string
  family?: string
  family_label?: string
  requires_reference: boolean
  is_active: boolean
  sort_order: number
  kind?: string
}

interface StationOrderRules {
  allow_walk_in: boolean
  require_washer: boolean
  quality_check_required: boolean
  release_requires_payment: boolean
  allow_partial_payments: boolean
  require_cancel_reason: boolean
  max_open_orders_per_washer: number
}

interface PriceRule {
  id: string
  station_id: string
  service_id: string
  category_code?: string
  usage_code?: string
  customer_id?: string
  customer_name?: string
  price_xof?: number
  valid_from?: string
  valid_to?: string
  reason: string
  is_active: boolean
  created_at: string
}

interface ServiceOption {
  id: string
  station_id: string
  service_id?: string
  category_code?: string
  name: string
  description: string
  price_xof: number
  duration_minutes: number
  sort_order: number
  is_active: boolean
  scope?: 'station' | 'prestation' | 'vehicule'
}

interface Station {
  id: string
  organization_id: string
  name: string
  code: string
  phone: string
  address: string
  city: string
  country?: string
  district?: string
  commune?: string
  district_area?: string
  street?: string
  landmark?: string
  latitude?: string
  longitude?: string
  billing_address?: string
  timezone: string
  currency: string
  logo_url: string
  opening_hours: Record<string, { ouverture: string; fermeture: string; ferme?: boolean }>
  site?: StationSite
  opening_hours_days?: OpeningHourDay[]
  opening_exceptions?: OpeningExceptionItem[]
  payment_methods?: DeclaredPaymentMethod[]
  order_rules?: StationOrderRules
  vehicle_categories?: Array<{ code: string; is_active: boolean; name?: string; local_name?: string; sort_order?: number }>
  vehicle_usages?: Array<{ code: string; is_active: boolean; name?: string; sort_order?: number }>
  usage_categories?: Array<{ usage_code: string; category_code: string; is_active?: boolean }>
  settings: {
    max_cashier_discount_xof?: number
    require_open_cash_session?: boolean
    receipt_footer?: string
    commission_mode?: 'none' | 'per_wash' | 'percent'
    commission_per_wash_xof?: number
    commission_percent?: number
    monthly_goal_xof?: number
  }
  is_default: boolean
  is_active: boolean
  created_at: string
}

interface Subscription {
  id: string
  organization_id: string
  plan_id: string
  status: 'trialing' | 'active' | 'past_due' | 'expired' | 'suspended' | 'canceled'
  started_at: string
  trial_ends_at?: string | null
  current_period_start?: string | null
  current_period_end?: string | null
  notes?: string
}

interface VehicleCategory {
  id: string
  code?: string
  name: string
  description: string
  sort_order: number
  is_active: boolean
  family?: string
  aliases?: string[]
}

interface Service {
  id: string
  organization_id: string
  station_id: string
  name: string
  category: string
  description: string
  price_xof: number
  duration_minutes: number
  sort_order: number
  is_active: boolean
}

interface Customer {
  id: string
  organization_id: string
  station_id: string
  full_name: string
  phone: string
  plate: string
  vehicle_model: string
  notes: string
  is_active: boolean
  created_at: string
}

interface CashSession {
  id: string
  organization_id: string
  station_id: string
  station_name?: string
  status: 'open' | 'closed'
  opened_by: string
  opened_by_name: string
  opened_at: string
  opening_float_xof: number
  closed_by?: string
  closed_by_name?: string
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
  business_day: string
}

interface WashOrderItem {
  id: string
  wash_order_id: string
  service_id: string
  service_name: string
  unit_price_xof: number
  quantity: number
  line_total_xof: number
}

interface WashOrder {
  id: string
  organization_id: string
  station_id: string
  reference: string
  customer_id?: string
  customer_name: string
  vehicle_category_id?: string
  vehicle_category_name: string
  plate: string
  employee_id?: string
  employee_name: string
  status: 'pending' | 'in_progress' | 'quality_check' | 'ready' | 'completed' | 'canceled'
  subtotal_xof: number
  discount_xof: number
  discount_reason: string
  total_xof: number
  notes: string
  cancel_reason?: string
  created_by: string
  created_by_name?: string
  created_at: string
  started_at?: string
  completed_at?: string | null
  canceled_at?: string | null
  business_day: string
  incident_note?: string
  incident_reported_at?: string
  incident_reporter_name?: string
  items?: WashOrderItem[]
}

interface Payment {
  id: string
  organization_id: string
  station_id: string
  wash_order_id?: string
  customer_id?: string
  kind: 'lavage' | 'acompte' | 'autre'
  method: 'especes' | 'mobile_money' | 'carte' | 'virement' | 'autre'
  amount_xof: number
  reference: string
  note: string
  paid_at: string
  business_day: string
  cash_session_id?: string
  received_by: string
  received_by_name: string
  status: 'valid' | 'voided'
  void_reason?: string
}

interface Expense {
  id: string
  organization_id: string
  station_id: string
  category: string
  label: string
  amount_xof: number
  method: 'especes' | 'mobile_money' | 'carte' | 'virement' | 'autre'
  supplier: string
  reference: string
  note: string
  spent_at: string
  business_day: string
  cash_session_id?: string
  recorded_by: string
  recorded_by_name: string
  status: 'valid' | 'voided'
  void_reason?: string
  created_at: string
}

interface AuditEntry {
  id: number
  organization_id?: string
  station_id?: string
  actor_user_id?: string
  actor_name: string
  action: string
  entity: string
  entity_id: string
  details: Record<string, unknown>
  created_at: string
}

// Global in-memory storage tables
const plans: Plan[] = [
  {
    id: 'plan-essai',
    code: 'essai',
    name: 'Essai',
    description: 'Découverte complète pendant 14 jours, une station, deux utilisateurs.',
    price_xof: 0,
    billing_period: 'mensuel',
    max_stations: 1,
    max_employees: 2,
    max_wash_orders_month: 100,
    trial_days: 14,
    features: {
      reports: true,
      export_csv: false,
      multi_station: false,
      cash_management: true,
      customers: true,
      audit_log: false,
    },
    sort_order: 1,
    is_active: true,
  },
  {
    id: 'plan-standard',
    code: 'standard',
    name: 'Standard',
    description: 'Pour une station : activité quotidienne, caisse et rapports.',
    price_xof: 15000,
    billing_period: 'mensuel',
    max_stations: 1,
    max_employees: 5,
    max_wash_orders_month: 2000,
    trial_days: 14,
    features: {
      reports: true,
      export_csv: true,
      multi_station: false,
      cash_management: true,
      customers: true,
      audit_log: true,
    },
    sort_order: 2,
    is_active: true,
  },
  {
    id: 'plan-premium',
    code: 'premium',
    name: 'Premium',
    description: "Pour plusieurs stations : équipe élargie, export CSV et journal d'audit.",
    price_xof: 35000,
    billing_period: 'mensuel',
    max_stations: 5,
    max_employees: 25,
    max_wash_orders_month: 20000,
    trial_days: 14,
    features: {
      reports: true,
      export_csv: true,
      multi_station: true,
      cash_management: true,
      customers: true,
      audit_log: true,
    },
    sort_order: 3,
    is_active: true,
  },
]

const organizations: Organization[] = [
  {
    id: 'org-demo',
    name: 'Station Étoile',
    slug: 'station-etoile',
    email: 'demo@washguard.ci',
    phone: '+225 07 07 12 34 56',
    address: 'Boulevard Principal, Yopougon',
    city: 'Abidjan',
    country: "Côte d'Ivoire",
    currency: 'XOF',
    timezone: 'Africa/Abidjan',
    logo_url: '',
    status: 'active',
    legal_name: 'Station Étoile SARL',
    trade_name: 'Station Étoile Lavage Auto',
    activity_type: 'Station de lavage et entretien automobile',
    activity_description: 'Lavage express, nettoyage intérieur et entretien complet.',
    activity_started_on: '2024-01-15',
    whatsapp: '+225 07 07 12 34 56',
    website: 'https://washguard.ci',
    rccm: 'CI-ABJ-2024-B-1234',
    tax_id: '987654321A',
    legal_form: 'SARL',
    fiscal_regime: 'Régime réel simplifié',
    created_at: new Date(Date.now() - 45 * 86400000).toISOString(),
  },
]

function getDefaultStationSite(stationName?: string): StationSite {
  return {
    wash_types: ['manuel', 'haute_pression', 'mousse_active'],
    bays: 4,
    simultaneous_vehicles: 4,
    has_vacuum: true,
    has_drying: true,
    has_detailing: false,
    has_waiting_room: true,
    has_storage: true,
    water_source: 'Raccordement SODECI + Cuve tampon',
    water_reserve_liters: 5000,
    power_supply: 'Réseau CIE triphasé 380V',
    has_generator: true,
    notes: stationName ? `Station ${stationName} prête pour l’exploitation.` : 'Station moderne de grande capacité.',
    constraints: 'Alimentation continue avec surpresseur automatique.',
  }
}

function getDefaultOpeningDays(): OpeningHourDay[] {
  return [
    { weekday: 1, is_open: true, opens_at: '07:30', closes_at: '19:30', break_start: '', break_end: '', last_checkin_at: '19:00', booking_allowed: false },
    { weekday: 2, is_open: true, opens_at: '07:30', closes_at: '19:30', break_start: '', break_end: '', last_checkin_at: '19:00', booking_allowed: false },
    { weekday: 3, is_open: true, opens_at: '07:30', closes_at: '19:30', break_start: '', break_end: '', last_checkin_at: '19:00', booking_allowed: false },
    { weekday: 4, is_open: true, opens_at: '07:30', closes_at: '19:30', break_start: '', break_end: '', last_checkin_at: '19:00', booking_allowed: false },
    { weekday: 5, is_open: true, opens_at: '07:30', closes_at: '20:00', break_start: '', break_end: '', last_checkin_at: '19:30', booking_allowed: false },
    { weekday: 6, is_open: true, opens_at: '07:30', closes_at: '20:00', break_start: '', break_end: '', last_checkin_at: '19:30', booking_allowed: false },
    { weekday: 7, is_open: true, opens_at: '08:00', closes_at: '18:00', break_start: '', break_end: '', last_checkin_at: '17:30', booking_allowed: false },
  ]
}

function getDefaultPaymentMethods(stId?: string): DeclaredPaymentMethod[] {
  return [
    { id: `pm-${stId || 'def'}-1`, station_id: stId, code: 'especes', name: 'Espèces (Caisse)', family: 'cash', family_label: 'Espèces', requires_reference: false, is_active: true, sort_order: 10, kind: 'modele' },
    { id: `pm-${stId || 'def'}-2`, station_id: stId, code: 'wave', name: 'Wave Mobile Money', family: 'mobile_money', family_label: 'Mobile Money', requires_reference: true, is_active: true, sort_order: 20, kind: 'modele' },
    { id: `pm-${stId || 'def'}-3`, station_id: stId, code: 'orange_money', name: 'Orange Money', family: 'mobile_money', family_label: 'Mobile Money', requires_reference: true, is_active: true, sort_order: 30, kind: 'modele' },
    { id: `pm-${stId || 'def'}-4`, station_id: stId, code: 'mtn_momo', name: 'MTN Mobile Money', family: 'mobile_money', family_label: 'Mobile Money', requires_reference: true, is_active: true, sort_order: 40, kind: 'modele' },
    { id: `pm-${stId || 'def'}-5`, station_id: stId, code: 'carte', name: 'Carte Bancaire / TPE', family: 'card', family_label: 'Carte bancaire', requires_reference: true, is_active: true, sort_order: 50, kind: 'modele' },
  ]
}

function getDefaultOrderRules(): StationOrderRules {
  return {
    allow_walk_in: true,
    require_washer: true,
    quality_check_required: false,
    release_requires_payment: true,
    allow_partial_payments: false,
    require_cancel_reason: true,
    max_open_orders_per_washer: 3,
  }
}

function getDefaultVehicleCategories(): Array<{ code: string; is_active: boolean; name: string; local_name?: string; sort_order: number }> {
  return [
    { code: 'moto', is_active: true, name: 'Moto & Tricycle', sort_order: 10 },
    { code: 'citadine', is_active: true, name: 'Citadine', sort_order: 20 },
    { code: 'berline', is_active: true, name: 'Berline', sort_order: 30 },
    { code: 'suv_4x4', is_active: true, name: '4x4 / SUV', sort_order: 40 },
    { code: 'utilitaire', is_active: true, name: 'Camionnette / Utilitaire', sort_order: 50 },
    { code: 'minibus', is_active: true, name: 'Minibus / Gbaka', sort_order: 60 },
  ]
}

function getDefaultVehicleUsages(): Array<{ code: string; is_active: boolean; name: string; sort_order: number }> {
  return [
    { code: 'particulier', is_active: true, name: 'Particulier', sort_order: 10 },
    { code: 'taxi_compteur', is_active: true, name: 'Taxi compteur', sort_order: 20 },
    { code: 'vtc', is_active: true, name: 'VTC (Yango / Uber)', sort_order: 30 },
    { code: 'flotte', is_active: true, name: 'Flotte d’entreprise', sort_order: 40 },
    { code: 'livraison', is_active: true, name: 'Livraison express', sort_order: 50 },
  ]
}

function getDefaultUsageCategories(): Array<{ usage_code: string; category_code: string; is_active: boolean }> {
  return [
    { usage_code: 'particulier', category_code: 'citadine', is_active: true },
    { usage_code: 'particulier', category_code: 'berline', is_active: true },
    { usage_code: 'particulier', category_code: 'suv_4x4', is_active: true },
    { usage_code: 'particulier', category_code: 'moto', is_active: true },
    { usage_code: 'taxi_compteur', category_code: 'berline', is_active: true },
    { usage_code: 'taxi_compteur', category_code: 'citadine', is_active: true },
    { usage_code: 'vtc', category_code: 'berline', is_active: true },
    { usage_code: 'vtc', category_code: 'suv_4x4', is_active: true },
    { usage_code: 'flotte', category_code: 'berline', is_active: true },
    { usage_code: 'flotte', category_code: 'suv_4x4', is_active: true },
    { usage_code: 'flotte', category_code: 'utilitaire', is_active: true },
    { usage_code: 'livraison', category_code: 'utilitaire', is_active: true },
    { usage_code: 'livraison', category_code: 'moto', is_active: true },
  ]
}

const stations: Station[] = [
  {
    id: 'station-yop',
    organization_id: 'org-demo',
    name: 'Station Étoile — Yopougon',
    code: 'YOP',
    phone: '+225 07 07 12 34 56',
    address: 'Boulevard Principal, Yopougon',
    city: 'Abidjan',
    country: "Côte d'Ivoire",
    district: 'Abidjan',
    commune: 'Yopougon',
    district_area: 'Siporex / Bel Air',
    street: 'Boulevard Principal, face Pharmacie Siporex',
    landmark: 'En face de la pharmacie Siporex',
    latitude: '5.3489',
    longitude: '-4.0823',
    billing_address: '01 BP 4589 Abidjan 01',
    timezone: 'Africa/Abidjan',
    currency: 'XOF',
    logo_url: '',
    opening_hours: {},
    site: getDefaultStationSite('Station Étoile — Yopougon'),
    opening_hours_days: getDefaultOpeningDays(),
    opening_exceptions: [],
    payment_methods: getDefaultPaymentMethods('station-yop'),
    order_rules: getDefaultOrderRules(),
    vehicle_categories: getDefaultVehicleCategories(),
    vehicle_usages: getDefaultVehicleUsages(),
    usage_categories: getDefaultUsageCategories(),
    settings: {
      commission_mode: 'per_wash',
      commission_per_wash_xof: 500,
      monthly_goal_xof: 1500000,
      require_open_cash_session: true,
      max_cashier_discount_xof: 1000,
      receipt_footer: 'Merci de votre confiance. À très bientôt chez Station Étoile !',
    },
    is_default: true,
    is_active: true,
    created_at: new Date(Date.now() - 45 * 86400000).toISOString(),
  },
]

const subscriptions: Subscription[] = [
  {
    id: 'sub-demo',
    organization_id: 'org-demo',
    plan_id: 'plan-premium',
    status: 'active',
    started_at: new Date(Date.now() - 45 * 86400000).toISOString(),
    current_period_start: new Date(Date.now() - 15 * 86400000).toISOString(),
    current_period_end: new Date(Date.now() + 15 * 86400000).toISOString(),
  },
]

const users: User[] = [
  {
    id: 'user-owner',
    organization_id: 'org-demo',
    station_id: 'station-yop',
    full_name: 'Erwin Kouassi',
    email: 'demo@washguard.ci',
    phone: '+225 07 07 12 34 56',
    password_hash: 'demo1234',
    role: 'owner',
    is_active: true,
    must_change_password: false,
    created_at: new Date(Date.now() - 45 * 86400000).toISOString(),
  },
  {
    id: 'user-manager',
    organization_id: 'org-demo',
    station_id: 'station-yop',
    full_name: 'Awa Traoré',
    email: 'gerant@washguard.ci',
    phone: '+225 05 55 44 33 22',
    password_hash: 'demo1234',
    role: 'manager',
    is_active: true,
    must_change_password: false,
    created_at: new Date(Date.now() - 45 * 86400000).toISOString(),
  },
  {
    id: 'user-cashier',
    organization_id: 'org-demo',
    station_id: 'station-yop',
    full_name: 'Moussa Diomandé',
    email: 'caisse@washguard.ci',
    phone: '+225 01 23 45 67 89',
    password_hash: 'demo1234',
    role: 'cashier',
    is_active: true,
    must_change_password: false,
    created_at: new Date(Date.now() - 45 * 86400000).toISOString(),
  },
  {
    id: 'user-washer',
    organization_id: 'org-demo',
    station_id: 'station-yop',
    full_name: 'Bakary Koné',
    email: 'laveur@washguard.ci',
    phone: '+225 07 44 33 22 11',
    password_hash: 'demo1234',
    role: 'washer',
    is_active: true,
    must_change_password: false,
    commission_mode: 'per_wash',
    commission_per_wash_xof: 500,
    created_at: new Date(Date.now() - 45 * 86400000).toISOString(),
  },
  {
    id: 'user-admin',
    full_name: 'Support WashGuard',
    email: 'admin@washguard.app',
    phone: '+225 00 00 00 00',
    password_hash: 'admin1234',
    role: 'superadmin',
    is_active: true,
    must_change_password: false,
    created_at: new Date(Date.now() - 45 * 86400000).toISOString(),
  },
]

const vehicleCategories: VehicleCategory[] = [
  { id: 'cat-moto', code: 'moto', name: 'Moto', description: 'Deux-roues, tricycle', sort_order: 1, is_active: true },
  { id: 'cat-voiture', code: 'voiture', name: 'Voiture', description: 'Berline, citadine, SUV', sort_order: 2, is_active: true },
  { id: 'cat-4x4', code: '4x4', name: '4x4 / Pick-up', description: 'Véhicule haut ou tout-terrain', sort_order: 3, is_active: true },
  { id: 'cat-camionnette', code: 'camionnette', name: 'Camionnette', description: 'Fourgon, utilitaire léger', sort_order: 4, is_active: true },
  { id: 'cat-minibus', code: 'minibus', name: 'Minibus', description: 'Transport de personnes', sort_order: 5, is_active: true },
  { id: 'cat-poids-lourd', code: 'poids_lourd', name: 'Poids lourd', description: 'Camion, benne', sort_order: 6, is_active: true },
]

const vehicleUsages = [
  { id: 'use-particulier', code: 'particulier', name: 'Particulier', description: 'Usage privé courant', sort_order: 1, is_active: true },
  { id: 'use-taxi', code: 'taxi_compteur', name: 'Taxi compteur', description: 'Transport urbain', sort_order: 2, is_active: true },
  { id: 'use-vtc', code: 'vtc', name: 'VTC / Flotte', description: 'Chauffeur VTC ou véhicule professionnel', sort_order: 3, is_active: true },
  { id: 'use-livraison', code: 'livraison', name: 'Livraison', description: 'Véhicule de distribution', sort_order: 4, is_active: true },
]

const services: Service[] = [
  { id: 'srv-1', organization_id: 'org-demo', station_id: 'station-yop', name: 'Lavage moto', category: 'lavage', description: "Lavage extérieur d'une moto ou d'un tricycle.", price_xof: 1000, duration_minutes: 15, sort_order: 1, is_active: true },
  { id: 'srv-2', organization_id: 'org-demo', station_id: 'station-yop', name: 'Lavage extérieur voiture', category: 'lavage', description: 'Carrosserie, jantes et vitres.', price_xof: 2000, duration_minutes: 30, sort_order: 2, is_active: true },
  { id: 'srv-3', organization_id: 'org-demo', station_id: 'station-yop', name: 'Lavage complet voiture', category: 'lavage', description: 'Extérieur, intérieur, aspiration et tableau de bord.', price_xof: 4000, duration_minutes: 45, sort_order: 3, is_active: true },
  { id: 'srv-4', organization_id: 'org-demo', station_id: 'station-yop', name: 'Lavage extérieur 4x4 / Camionnette', category: 'lavage', description: 'Carrosserie haute, passages de roues.', price_xof: 2500, duration_minutes: 40, sort_order: 4, is_active: true },
  { id: 'srv-5', organization_id: 'org-demo', station_id: 'station-yop', name: 'Lavage complet 4x4 / Camionnette', category: 'lavage', description: 'Extérieur, intérieur, aspiration, coffre.', price_xof: 5000, duration_minutes: 60, sort_order: 5, is_active: true },
  { id: 'srv-6', organization_id: 'org-demo', station_id: 'station-yop', name: 'Nettoyage des sièges', category: 'interieur', description: 'Shampouinage des sièges avant et arrière.', price_xof: 5000, duration_minutes: 60, sort_order: 6, is_active: true },
  { id: 'srv-7', organization_id: 'org-demo', station_id: 'station-yop', name: 'Aspiration intérieure', category: 'interieur', description: 'Aspiration tapis, coffre et sièges.', price_xof: 1500, duration_minutes: 20, sort_order: 7, is_active: true },
  { id: 'srv-8', organization_id: 'org-demo', station_id: 'station-yop', name: 'Nettoyage moteur', category: 'moteur', description: 'Dégraissage et rinçage du compartiment moteur.', price_xof: 3000, duration_minutes: 30, sort_order: 8, is_active: true },
  { id: 'srv-9', organization_id: 'org-demo', station_id: 'station-yop', name: 'Cirage / lustrage', category: 'finition', description: 'Cire protectrice et brillance de la carrosserie.', price_xof: 6000, duration_minutes: 60, sort_order: 9, is_active: true },
  { id: 'srv-10', organization_id: 'org-demo', station_id: 'station-yop', name: 'Désinfection et parfum', category: 'finition', description: "Désinfection de l'habitacle et parfum.", price_xof: 2000, duration_minutes: 15, sort_order: 10, is_active: true },
]

const customers: Customer[] = [
  { id: 'cust-1', organization_id: 'org-demo', station_id: 'station-yop', full_name: 'Kouadio Koné', phone: '+225 07 12 34 56 78', plate: 'AB 1234 CD', vehicle_model: 'Toyota Corolla', notes: 'Client fidèle taxi', is_active: true, created_at: new Date(Date.now() - 40 * 86400000).toISOString() },
  { id: 'cust-2', organization_id: 'org-demo', station_id: 'station-yop', full_name: 'Aminata Traoré', phone: '+225 05 23 45 67 89', plate: 'EF 5678 GH', vehicle_model: 'Renault Clio', notes: '', is_active: true, created_at: new Date(Date.now() - 38 * 86400000).toISOString() },
  { id: 'cust-3', organization_id: 'org-demo', station_id: 'station-yop', full_name: 'Yao Bamba', phone: '+225 01 34 56 78 90', plate: 'JK 9012 LM', vehicle_model: 'Hyundai Tucson', notes: 'Entreprise VTC', is_active: true, created_at: new Date(Date.now() - 35 * 86400000).toISOString() },
  { id: 'cust-4', organization_id: 'org-demo', station_id: 'station-yop', full_name: 'Fatou Diallo', phone: '+225 07 45 67 89 01', plate: 'NP 3456 RS', vehicle_model: 'Nissan Almera', notes: '', is_active: true, created_at: new Date(Date.now() - 30 * 86400000).toISOString() },
  { id: 'cust-5', organization_id: 'org-demo', station_id: 'station-yop', full_name: 'Serge Kouassi', phone: '+225 05 56 78 90 12', plate: 'TU 7890 VW', vehicle_model: 'Toyota Hilux', notes: 'Paiement Wave habituel', is_active: true, created_at: new Date(Date.now() - 28 * 86400000).toISOString() },
]

const paymentMethodsDeclared = [
  { id: 'pm-cash', code: 'especes', name: 'Espèces (Caisse)', family: 'cash', family_label: 'Espèces', requires_reference: false, is_active: true, sort_order: 1, kind: 'modele' },
  { id: 'pm-wave', code: 'wave', name: 'Wave Mobile Money', family: 'mobile_money', family_label: 'Mobile Money', requires_reference: true, is_active: true, sort_order: 2, kind: 'modele' },
  { id: 'pm-om', code: 'orange_money', name: 'Orange Money', family: 'mobile_money', family_label: 'Mobile Money', requires_reference: true, is_active: true, sort_order: 3, kind: 'modele' },
  { id: 'pm-mtn', code: 'mtn_momo', name: 'MTN MoMo', family: 'mobile_money', family_label: 'Mobile Money', requires_reference: true, is_active: true, sort_order: 4, kind: 'modele' },
  { id: 'pm-card', code: 'carte', name: 'Carte Bancaire / TPE', family: 'card', family_label: 'Carte bancaire', requires_reference: true, is_active: true, sort_order: 5, kind: 'modele' },
]

const invitations: any[] = []
const equipments: any[] = [
  { id: 'eq-1', station_id: 'station-yop', template_code: 'karcher_hd', name: 'Nettoyeur haute pression Kärcher HD 7/18', quantity: 2, state: 'good', acquired_on: '2024-02-01', last_service_on: '2026-09-01', cost_xof: 450000, notes: 'Poste 1 et 2', is_active: true },
  { id: 'eq-2', station_id: 'station-yop', template_code: 'aspirateur_eau_poussiere', name: 'Aspirateur eau et poussière tri-moteurs', quantity: 2, state: 'good', acquired_on: '2024-03-10', cost_xof: 180000, notes: 'Zone séchage', is_active: true },
]

const serviceOptions: ServiceOption[] = [
  { id: 'opt-1', station_id: 'station-yop', name: 'Parfum d’ambiance vanille/fraîcheur', description: 'Pulvérisation habitacle longue durée', price_xof: 1000, duration_minutes: 5, sort_order: 1, is_active: true, scope: 'station' },
  { id: 'opt-2', station_id: 'station-yop', name: 'Traitement brillance pneus & plastiques', description: 'Rénovateur pneus et pare-chocs', price_xof: 1500, duration_minutes: 10, sort_order: 2, is_active: true, scope: 'station' },
  { id: 'opt-3', station_id: 'station-yop', name: 'Cire de protection déperlante express', description: 'Protection carrosserie anti-poussière', price_xof: 2500, duration_minutes: 15, sort_order: 3, is_active: true, scope: 'station' },
]

const priceRules: PriceRule[] = [
  { id: 'pr-1', station_id: 'station-yop', service_id: 'srv-1', category_code: 'moto', price_xof: 1000, reason: 'Tarif initial moto', is_active: true, created_at: new Date().toISOString() },
  { id: 'pr-2', station_id: 'station-yop', service_id: 'srv-2', category_code: 'berline', price_xof: 2000, reason: 'Tarif extérieur berline', is_active: true, created_at: new Date().toISOString() },
  { id: 'pr-3', station_id: 'station-yop', service_id: 'srv-2', category_code: 'citadine', price_xof: 1500, reason: 'Remise citadine compacte', is_active: true, created_at: new Date().toISOString() },
  { id: 'pr-4', station_id: 'station-yop', service_id: 'srv-3', category_code: 'berline', price_xof: 4000, reason: 'Complet berline', is_active: true, created_at: new Date().toISOString() },
  { id: 'pr-5', station_id: 'station-yop', service_id: 'srv-3', category_code: 'suv_4x4', price_xof: 5000, reason: 'Complet SUV/4x4', is_active: true, created_at: new Date().toISOString() },
  { id: 'pr-6', station_id: 'station-yop', service_id: 'srv-2', usage_code: 'taxi_compteur', price_xof: 1500, reason: 'Tarif préférentiel taxis', is_active: true, created_at: new Date().toISOString() },
]
const stockItems: any[] = [
  { id: 'stk-1', station_id: 'station-yop', name: 'Shampoing carrosserie moussant (Bidon 20L)', category_code: 'produits', unit: 'bidon', quantity: 6, min_quantity: 2, unit_cost_xof: 18000, supplier: 'ProWash Abidjan', location: 'Local produits', is_active: true },
  { id: 'stk-2', station_id: 'station-yop', name: 'Chiffons microfibres haute densité', category_code: 'consommables', unit: 'lot', quantity: 24, min_quantity: 10, unit_cost_xof: 1500, supplier: 'Grossiste Adjamé', location: 'Étagère 1', is_active: true },
  { id: 'stk-3', station_id: 'station-yop', name: 'Nettoyant jantes dégraissant', category_code: 'produits', unit: 'flacon', quantity: 8, min_quantity: 3, unit_cost_xof: 3500, supplier: 'ProWash Abidjan', location: 'Local produits', is_active: true },
]
const stockMovements: any[] = []

const cashSessions: CashSession[] = []
const washOrders: WashOrder[] = []
const payments: Payment[] = []
const expenses: Expense[] = []
let nextAuditId = 1
const auditLogs: AuditEntry[] = []

function addAudit(action: string, entity: string, entityId: string, actor: string, details: Record<string, unknown> = {}) {
  auditLogs.unshift({
    id: nextAuditId++,
    organization_id: 'org-demo',
    station_id: 'station-yop',
    actor_name: actor,
    action,
    entity,
    entity_id: entityId,
    details,
    created_at: new Date().toISOString(),
  })
}

// Seed data
function seedData() {
  const now = new Date()
  let orderSeq = 1000

  for (let dayOffset = 30; dayOffset >= 0; dayOffset--) {
    const d = new Date(now.getTime() - dayOffset * 86400000)
    const yyyy = d.getFullYear()
    const mm = String(d.getMonth() + 1).padStart(2, '0')
    const dd = String(d.getDate()).padStart(2, '0')
    const businessDay = `${yyyy}-${mm}-${dd}`
    const isToday = dayOffset === 0

    // Cash session
    const sessionId = `cs-${businessDay}`
    const openFloat = 15000 + (dayOffset % 4) * 5000
    const openedAt = new Date(d.setHours(7, 30, 0, 0)).toISOString()

    const session: CashSession = {
      id: sessionId,
      organization_id: 'org-demo',
      station_id: 'station-yop',
      station_name: 'Station Étoile — Yopougon',
      status: isToday ? 'open' : 'closed',
      opened_by: 'user-cashier',
      opened_by_name: 'Moussa Diomandé',
      opened_at: openedAt,
      opening_float_xof: openFloat,
      expected_cash_xof: openFloat,
      cash_in_xof: 0,
      mobile_money_xof: 0,
      other_in_xof: 0,
      expenses_xof: 0,
      operations: 0,
      payment_count: 0,
      expense_count: 0,
      notes: '',
      close_notes: isToday ? '' : 'Comptage de fin de journée',
      reopen_count: 0,
      business_day: businessDay,
    }

    // Orders for this day
    const orderCount = d.getDay() === 0 ? 5 : 7 + (dayOffset % 5)
    for (let i = 0; i < orderCount; i++) {
      orderSeq++
      const ref = `L-${yyyy}${mm}${dd}-${String(i + 1).padStart(4, '0')}`
      const srvIndex = (i + dayOffset) % services.length
      const srv = services[srvIndex]
      const cat = vehicleCategories[(i + dayOffset) % vehicleCategories.length]
      const isPassage = i % 3 === 0
      const cust = isPassage ? null : customers[(i + dayOffset) % customers.length]

      const item: WashOrderItem = {
        id: `item-${orderSeq}`,
        wash_order_id: `ord-${orderSeq}`,
        service_id: srv.id,
        service_name: srv.name,
        unit_price_xof: srv.price_xof,
        quantity: 1,
        line_total_xof: srv.price_xof,
      }

      const isCanceled = !isToday && i === 1 && dayOffset % 5 === 0
      const isPending = isToday && i === orderCount - 1
      const status: WashOrder['status'] = isCanceled ? 'canceled' : isPending ? 'pending' : 'completed'

      const subtotal = srv.price_xof
      const discount = i % 5 === 0 ? 500 : 0
      const total = Math.max(0, subtotal - discount)

      const orderTime = new Date(d.setHours(8 + (i % 10), (i * 12) % 60, 0, 0)).toISOString()
      const completedTime = new Date(d.setHours(8 + (i % 10) + 1, (i * 12) % 60, 0, 0)).toISOString()

      const order: WashOrder = {
        id: `ord-${orderSeq}`,
        organization_id: 'org-demo',
        station_id: 'station-yop',
        reference: ref,
        customer_id: cust ? cust.id : undefined,
        customer_name: cust ? cust.full_name : 'Client de passage',
        vehicle_category_id: cat.id,
        vehicle_category_name: cat.name,
        plate: cust ? cust.plate : `CI ${(1000 + orderSeq) % 9999} AB`,
        employee_id: i % 2 === 0 ? 'user-washer' : i % 4 === 1 ? 'user-cashier' : 'user-manager',
        employee_name: i % 2 === 0 ? 'Bakary Koné' : i % 4 === 1 ? 'Moussa Diomandé' : 'Awa Traoré',
        status,
        subtotal_xof: subtotal,
        discount_xof: discount,
        discount_reason: discount > 0 ? 'Geste commercial' : '',
        total_xof: total,
        notes: isToday && i === 0 ? 'Bien insister sur les jantes' : '',
        incident_note: i === 2 && dayOffset % 4 === 0 ? "Rayure superficielle sur l'aile avant droite constatée à la réception." : undefined,
        incident_reported_at: i === 2 && dayOffset % 4 === 0 ? orderTime : undefined,
        incident_reporter_name: i === 2 && dayOffset % 4 === 0 ? 'Bakary Koné' : undefined,
        cancel_reason: isCanceled ? 'Client pressé' : undefined,
        created_by: 'user-cashier',
        created_by_name: 'Moussa Diomandé',
        created_at: orderTime,
        started_at: orderTime,
        completed_at: status === 'completed' ? completedTime : null,
        canceled_at: status === 'canceled' ? completedTime : null,
        business_day: businessDay,
        items: [item],
      }
      washOrders.push(order)

      // Payment
      if (status === 'completed') {
        const isCash = i % 3 !== 0
        const method = isCash ? 'especes' : 'mobile_money'
        const payment: Payment = {
          id: `pay-${orderSeq}`,
          organization_id: 'org-demo',
          station_id: 'station-yop',
          wash_order_id: order.id,
          customer_id: cust?.id,
          kind: 'lavage',
          method,
          amount_xof: total,
          reference: isCash ? '' : `MM${Date.now() % 100000}`,
          note: '',
          paid_at: completedTime,
          business_day: businessDay,
          cash_session_id: sessionId,
          received_by: 'user-cashier',
          received_by_name: 'Moussa Diomandé',
          status: 'valid',
        }
        payments.push(payment)

        if (isCash) {
          session.cash_in_xof += total
        } else {
          session.mobile_money_xof += total
        }
        session.payment_count += 1
        session.operations += 1
      }
    }

    // Daily expenses
    const expAmount = 2500 + (dayOffset % 5) * 1000
    const expenseTime = new Date(d.setHours(14, 0, 0, 0)).toISOString()
    const exp: Expense = {
      id: `exp-${dayOffset}`,
      organization_id: 'org-demo',
      station_id: 'station-yop',
      category: dayOffset % 2 === 0 ? 'produits' : 'eau',
      label: dayOffset % 2 === 0 ? 'Savon liquide et chiffons' : 'Facture SODECI',
      amount_xof: expAmount,
      method: 'especes',
      supplier: 'Fournisseur local',
      reference: `FAC-${dayOffset}`,
      note: '',
      spent_at: expenseTime,
      business_day: businessDay,
      cash_session_id: sessionId,
      recorded_by: 'user-manager',
      recorded_by_name: 'Awa Traoré',
      status: 'valid',
      created_at: expenseTime,
    }
    expenses.push(exp)
    session.expenses_xof += expAmount
    session.expense_count += 1
    session.operations += 1

    session.expected_cash_xof = session.opening_float_xof + session.cash_in_xof - session.expenses_xof
    if (!isToday) {
      session.counted_cash_xof = session.expected_cash_xof
      session.difference_xof = 0
      session.closed_by = 'user-cashier'
      session.closed_by_name = 'Moussa Diomandé'
      session.closed_at = new Date(d.setHours(20, 0, 0, 0)).toISOString()
    }
    cashSessions.push(session)
  }
}

seedData()

// ---------------------------------------------------------------------------
// AUTH & PERMISSIONS HELPERS
// ---------------------------------------------------------------------------

const rolePermissions: Record<string, string[]> = {
  superadmin: [
    'dashboard:view', 'station:view', 'wash:view', 'wash:create', 'wash:update', 'wash:cancel', 'wash:assign',
    'payment:view', 'payment:create', 'payment:void', 'service:view', 'service:manage',
    'customer:view', 'customer:create', 'customer:manage', 'employee:view', 'employee:manage',
    'cash:view', 'cash:open', 'cash:close', 'cash:reopen', 'expense:view', 'expense:create',
    'expense:void', 'report:view', 'report:export', 'commission:view', 'commission:manage', 'settings:view',
    'settings:manage', 'organization:manage', 'subscription:view', 'subscription:manage',
    'audit:view', 'platform:manage', 'washer:view',
  ],
  owner: [
    'dashboard:view', 'station:view', 'wash:view', 'wash:create', 'wash:update', 'wash:cancel', 'wash:assign',
    'payment:view', 'payment:create', 'payment:void', 'service:view', 'service:manage',
    'customer:view', 'customer:create', 'customer:manage', 'employee:view', 'employee:manage',
    'cash:view', 'cash:open', 'cash:close', 'cash:reopen', 'expense:view', 'expense:create',
    'expense:void', 'report:view', 'report:export', 'commission:view', 'commission:manage', 'settings:view',
    'settings:manage', 'organization:manage', 'subscription:view', 'subscription:manage',
    'audit:view', 'washer:view',
  ],
  manager: [
    'dashboard:view', 'station:view', 'wash:view', 'wash:create', 'wash:update', 'wash:cancel', 'wash:assign',
    'payment:view', 'payment:create', 'payment:void', 'service:view', 'service:manage',
    'customer:view', 'customer:create', 'customer:manage', 'employee:view', 'employee:manage',
    'cash:view', 'cash:open', 'cash:close', 'cash:reopen', 'expense:view', 'expense:create',
    'expense:void', 'expense:approve', 'report:view', 'report:export', 'commission:view', 'commission:manage',
    'settings:view', 'settings:manage', 'subscription:view', 'audit:view', 'washer:view', 'washer:update_status', 'washer:report_incident',
  ],
  cashier: [
    'dashboard:view', 'station:view', 'wash:view', 'wash:create', 'wash:update',
    'payment:view', 'payment:create', 'service:view', 'customer:view', 'customer:create',
    'cash:view', 'cash:open', 'cash:close', 'expense:create', 'washer:view',
  ],
  washer: [
    'washer:view', 'washer:update_status', 'washer:report_incident', 'washer:personal_stats',
  ],
}

// Active sessions: token -> userId
const activeSessions = new Map<string, string>()

// Pre-authorize demo users
activeSessions.set('demo-session-token', 'user-owner')

function getPrincipal(req: Request): User | null {
  const authHeader = req.headers['authorization']
  const bearerToken = authHeader?.startsWith('Bearer ') ? authHeader.slice(7).trim() : null
  const token = req.cookies?.washguard_session || bearerToken
  if (!token) return null
  const userId = activeSessions.get(token)
  if (!userId) return null
  return users.find((u) => u.id === userId && u.is_active) ?? null
}

function calculateOnboarding(orgId: string, stationId?: string) {
  const org = organizations.find((o) => o.id === orgId) || organizations[0]
  const st = stations.find((s) => s.id === stationId) || stations.find((s) => s.organization_id === orgId) || stations[0]
  const stId = st?.id || ''

  const hasOwner = users.some((u) => u.organization_id === orgId && (u.role === 'owner' || u.role === 'manager'))
  const entrepriseDone = Boolean(org && org.name && org.city && (org.phone || org.email))
  const stationDone = Boolean(st && st.name && st.city && (st.site?.wash_types?.length || st.site?.bays || st.address))
  const horairesDone = Boolean(st && st.opening_hours_days && st.opening_hours_days.some((d) => d.is_open && d.opens_at && d.closes_at))
  
  const stCategories = st?.vehicle_categories ?? []
  const vehiculesDone = stCategories.length > 0 ? stCategories.some((c) => c.is_active) : vehicleCategories.some((c) => c.is_active)

  const stUsages = st?.vehicle_usages ?? []
  const stCouples = st?.usage_categories ?? []
  const usagesDone = (stUsages.length > 0 ? stUsages.some((u) => u.is_active) : vehicleUsages.some((u) => u.is_active)) &&
    stCouples.length > 0

  const stServices = services.filter((s) => s.station_id === stId && s.is_active)
  const prestationsDone = stServices.length > 0

  const stOptions = serviceOptions.filter((o) => o.station_id === stId)
  const optionsDone = true

  const hasGenericPrices = stServices.some((s) => s.price_xof > 0)
  const hasMatrixRules = priceRules.some((r) => r.station_id === stId && r.is_active && (r.price_xof ?? 0) > 0)
  const tarifsDone = hasGenericPrices || hasMatrixRules

  const stPaymentMethods = st?.payment_methods ?? paymentMethodsDeclared
  const moyensDone = stPaymentMethods.some((p) => p.is_active)

  const stEmployees = users.filter((u) => u.organization_id === orgId)
  const equipeDone = stEmployees.length >= 1

  const stEquipments = equipments.filter((e) => e.station_id === stId)
  const materielDone = stEquipments.length > 0

  const commandesDone = Boolean(st?.order_rules)

  const steps = [
    {
      code: 'espace',
      title: 'Espace propriétaire',
      description: 'Compte initialisé avec droits d’administration complets',
      required: true,
      done: hasOwner,
      state: hasOwner ? 'completed' : 'pending',
      detail: hasOwner ? 'Propriétaire actif' : 'En attente',
      screen: '/parametres',
      hint: '',
      form: 'espace',
      data: {},
    },
    {
      code: 'entreprise',
      title: 'Identité commerciale',
      description: 'Raison sociale, ville, contacts et mentions légales',
      required: true,
      done: entrepriseDone,
      state: entrepriseDone ? 'completed' : 'pending',
      detail: org?.name ? `${org.name} (${org.city})` : 'À renseigner',
      screen: '/parametres',
      hint: 'Renseignez le nom officiel et la ville de votre entreprise de lavage.',
      form: 'entreprise',
      data: org ?? {},
    },
    {
      code: 'station',
      title: 'Fiche & Capacités de la station',
      description: 'Localisation, pistes, eau, électricité et équipements de site',
      required: true,
      done: stationDone,
      state: stationDone ? 'completed' : 'pending',
      detail: st ? `${st.name} · ${st.site?.bays ?? 4} pistes` : 'À configurer',
      screen: '/parametres',
      hint: 'Déclarez les pistes de lavage, la réserve d’eau et les types de lavage pratiqués.',
      form: 'station',
      data: st ?? {},
    },
    {
      code: 'horaires',
      title: 'Horaires d’ouverture',
      description: 'Plages horaires par jour de la semaine et exceptions',
      required: true,
      done: horairesDone,
      state: horairesDone ? 'completed' : 'pending',
      detail: horairesDone ? `${st?.opening_hours_days?.filter((d) => d.is_open).length ?? 6} jours ouverts` : 'Horaires requis',
      screen: '/parametres',
      hint: 'Indiquez au moins un jour d’ouverture avec heure de début et fin de service.',
      form: 'horaires',
      data: {},
    },
    {
      code: 'vehicules',
      title: 'Catégories de véhicules',
      description: 'Types de véhicules acceptés (moto, berline, SUV, utilitaire...)',
      required: true,
      done: vehiculesDone,
      state: vehiculesDone ? 'completed' : 'pending',
      detail: `${(st?.vehicle_categories ?? vehicleCategories).filter((c) => c.is_active).length} catégories actives`,
      screen: '/tarifs',
      hint: 'Cochez les types de véhicules que votre station est équipée pour laver.',
      form: 'vehicules',
      data: {},
    },
    {
      code: 'usages',
      title: 'Usages commerciaux',
      description: 'Particulier, taxi compteur, VTC, flotte entreprise, livraison',
      required: true,
      done: usagesDone,
      state: usagesDone ? 'completed' : 'pending',
      detail: `${(st?.vehicle_usages ?? vehicleUsages).filter((u) => u.is_active).length} usages · ${stCouples.length} combinaisons`,
      screen: '/tarifs',
      hint: 'Activez les profils d’usage et associez les catégories de véhicules concernées.',
      form: 'usages',
      data: {},
    },
    {
      code: 'prestations',
      title: 'Catalogue des prestations',
      description: 'Services proposés (lavage simple, complet, moteur, detailing...)',
      required: true,
      done: prestationsDone,
      state: prestationsDone ? 'completed' : 'pending',
      detail: `${stServices.length} prestations actives`,
      screen: '/prestations',
      hint: 'Créez ou adoptez au moins une prestation de lavage.',
      form: 'prestations',
      data: {},
    },
    {
      code: 'options',
      title: 'Options & Suppléments',
      description: 'Suppléments optionnels facturables (parfum, cire, lustrage...)',
      required: false,
      done: optionsDone,
      state: optionsDone ? 'completed' : 'pending',
      detail: `${stOptions.length} option(s)`,
      screen: '/prestations',
      hint: 'Ajoutez des options de finition pour augmenter votre panier moyen.',
      form: 'options',
      data: {},
    },
    {
      code: 'tarifs',
      title: 'Grille tarifaire',
      description: 'Tarifs par catégorie de véhicule et profil d’usage commercial',
      required: true,
      done: tarifsDone,
      state: tarifsDone ? 'completed' : 'pending',
      detail: tarifsDone ? 'Grille configurée' : 'Prix à renseigner',
      screen: '/tarifs',
      hint: 'Fixez les tarifs de vos prestations principales selon le type de véhicule.',
      form: 'tarifs',
      data: {},
    },
    {
      code: 'moyens',
      title: 'Moyens de paiement',
      description: 'Espèces, Wave, Orange Money, MTN MoMo, Carte bancaire',
      required: true,
      done: moyensDone,
      state: moyensDone ? 'completed' : 'pending',
      detail: `${stPaymentMethods.filter((p) => p.is_active).length} moyens actifs`,
      screen: '/caisse',
      hint: 'Activez au moins un moyen d’encaissement pour la caisse.',
      form: 'moyens',
      data: {},
    },
    {
      code: 'equipe',
      title: 'Équipe & Fonctions',
      description: 'Comptes des gérants, caissiers et laveurs de la station',
      required: false,
      done: equipeDone,
      state: equipeDone ? 'completed' : 'pending',
      detail: `${stEmployees.length} membre(s)`,
      screen: '/employes',
      hint: 'Invitez vos collaborateurs ou gérez les comptes de caisse.',
      form: 'equipe',
      data: {},
    },
    {
      code: 'materiel',
      title: 'Équipements & Matériel',
      description: 'Matériel haute pression, aspirateurs et stock initial',
      required: false,
      done: materielDone,
      state: materielDone ? 'completed' : 'pending',
      detail: `${stEquipments.length} équipement(s)`,
      screen: '/parametres',
      hint: 'Déclarez votre matériel pour le suivi de maintenance.',
      form: 'materiel',
      data: {},
    },
    {
      code: 'commandes',
      title: 'Règles de caisse & Commandes',
      description: 'Ouverture de caisse, remises, cycle des fiches et commissions',
      required: false,
      done: commandesDone,
      state: commandesDone ? 'completed' : 'pending',
      detail: 'Règles définies',
      screen: '/parametres',
      hint: 'Ajustez le plafond de remise caissier et les commissions des laveurs.',
      form: 'commandes',
      data: {},
    },
  ]

  const requiredSteps = steps.filter((s) => s.required)
  const can_activate = requiredSteps.every((s) => s.done)
  const stepsDone = steps.filter((s) => s.done).length
  const percent = Math.round((stepsDone / steps.length) * 100)
  const isActuallyActive = (org?.status === 'active' && st?.is_active) || can_activate

  const missing = requiredSteps
    .filter((s) => !s.done)
    .map((s) => ({
      code: s.code,
      title: s.title,
      reason: s.description,
    }))

  return {
    organization_id: orgId,
    state: isActuallyActive ? ('active' as const) : ('in_progress' as const),
    progress: {
      done: stepsDone,
      total: steps.length,
      percent,
      steps_done: stepsDone,
      steps_total: steps.length,
    },
    steps,
    missing,
    can_activate,
    started_at: org?.created_at || new Date().toISOString(),
    activated_at: isActuallyActive ? (st?.created_at || new Date().toISOString()) : '',
  }
}

function buildMePayload(user: User) {
  const org = organizations.find((o) => o.id === user.organization_id) ?? null
  const st = stations.find((s) => s.id === user.station_id) ?? stations[0] ?? null
  const sub = subscriptions.find((s) => s.organization_id === user.organization_id) ?? null
  const plan = plans.find((p) => p.id === sub?.plan_id) ?? plans[1]

  const permissions = rolePermissions[user.role] ?? []

  const subscriptionPayload = sub
    ? {
        plan_id: plan.id,
        plan_code: plan.code,
        plan_name: plan.name,
        plan_description: plan.description,
        status: sub.status,
        status_label: sub.status === 'active' ? 'Actif' : sub.status === 'trialing' ? "Période d'essai" : sub.status,
        price_xof: plan.price_xof,
        billing_period: plan.billing_period,
        trial_ends_at: sub.trial_ends_at,
        current_period_start: sub.current_period_start,
        current_period_end: sub.current_period_end,
        max_stations: plan.max_stations,
        max_employees: plan.max_employees,
        max_wash_orders_month: plan.max_wash_orders_month,
        features: plan.features,
        usage: {
          period: new Date().toISOString().slice(0, 7),
          wash_orders: washOrders.filter((w) => w.organization_id === user.organization_id).length,
          stations: stations.filter((s) => s.organization_id === user.organization_id && s.is_active).length,
          employees: users.filter((u) => u.organization_id === user.organization_id && u.is_active).length,
        },
      }
    : null

  const onboardingPayload = calculateOnboarding(user.organization_id || 'org-demo', user.station_id || st?.id)

  return {
    user: {
      id: user.id,
      full_name: user.full_name,
      email: user.email,
      phone: user.phone,
      role: user.role,
      role_label: user.role === 'owner' ? 'Propriétaire' : user.role === 'manager' ? 'Gérant' : user.role === 'cashier' ? 'Caissier' : user.role === 'washer' ? 'Laveur' : 'Administrateur',
      organization_id: user.organization_id || '',
      station_id: user.station_id || '',
    },
    organization: org,
    station: st,
    subscription: subscriptionPayload,
    permissions,
    must_change_password: user.must_change_password,
    onboarding: onboardingPayload,
  }
}

function requireAuth(req: Request, res: Response, next: NextFunction) {
  const user = getPrincipal(req)
  if (!user) {
    return res.status(401).json({ error: { code: 'unauthorized', message: 'Vous devez vous connecter.' } })
  }
  ;(req as any).user = user
  next()
}

function setSessionCookie(res: Response, token: string, req: Request) {
  const isHttps = req.secure || req.headers['x-forwarded-proto'] === 'https'
  res.cookie('washguard_session', token, {
    path: '/',
    httpOnly: true,
    secure: isHttps,
    sameSite: isHttps ? 'none' : 'lax',
    maxAge: 30 * 86400 * 1000,
  })
}

// ---------------------------------------------------------------------------
// API V1 ROUTER
// ---------------------------------------------------------------------------

const api = express.Router()

// Health check
app.get('/health', (_req, res) => {
  res.json({ status: 'ok', environment: 'development', time: new Date().toISOString() })
})

// Public Plans
api.get('/plans', (_req, res) => {
  res.json({ items: plans.filter((p) => p.is_active) })
})

// Auth routes
api.post('/auth/login', (req, res) => {
  const { email, password } = req.body || {}
  const user = users.find((u) => u.email.toLowerCase() === String(email).trim().toLowerCase())
  if (!user || user.password_hash !== password) {
    return res.status(401).json({ error: { code: 'unauthorized', message: 'E-mail ou mot de passe incorrect.' } })
  }
  if (!user.is_active) {
    return res.status(403).json({ error: { code: 'forbidden', message: 'Ce compte a été désactivé.' } })
  }

  const token = `token-${Date.now()}-${Math.random().toString(36).substring(2)}`
  activeSessions.set(token, user.id)
  setSessionCookie(res, token, req)

  res.json({ ...buildMePayload(user), session_token: token })
})

api.post('/auth/register', (req, res) => {
  const { organization_name, full_name, email, phone, password, station_name, city } = req.body || {}
  if (!organization_name || !full_name || !email || !password || !station_name) {
    return res.status(400).json({ error: { code: 'validation_error', message: 'Tous les champs obligatoires doivent être renseignés.' } })
  }

  const existing = users.find((u) => u.email.toLowerCase() === String(email).trim().toLowerCase())
  if (existing) {
    return res.status(400).json({ error: { code: 'validation_error', message: 'Un compte existe déjà avec cette adresse e-mail.', fields: { email: 'E-mail déjà utilisé' } } })
  }

  const orgId = `org-${Date.now()}`
  const stationId = `st-${Date.now()}`
  const userId = `usr-${Date.now()}`

  const newOrg: Organization = {
    id: orgId,
    name: organization_name,
    slug: organization_name.toLowerCase().replace(/[^a-z0-9]+/g, '-'),
    email,
    phone: phone || '',
    address: '',
    city: city || 'Abidjan',
    country: "Côte d'Ivoire",
    currency: 'XOF',
    timezone: 'Africa/Abidjan',
    logo_url: '',
    status: 'active',
    created_at: new Date().toISOString(),
  }
  organizations.push(newOrg)

  const newStation: Station = {
    id: stationId,
    organization_id: orgId,
    name: station_name,
    code: station_name.slice(0, 3).toUpperCase(),
    phone: phone || '',
    address: '',
    city: city || 'Abidjan',
    country: "Côte d'Ivoire",
    district: city || 'Abidjan',
    commune: '',
    district_area: '',
    street: '',
    landmark: '',
    latitude: '5.3489',
    longitude: '-4.0823',
    billing_address: '',
    timezone: 'Africa/Abidjan',
    currency: 'XOF',
    logo_url: '',
    opening_hours: {},
    site: getDefaultStationSite(station_name),
    opening_hours_days: getDefaultOpeningDays(),
    opening_exceptions: [],
    payment_methods: getDefaultPaymentMethods(stationId),
    order_rules: getDefaultOrderRules(),
    vehicle_categories: getDefaultVehicleCategories(),
    vehicle_usages: getDefaultVehicleUsages(),
    usage_categories: getDefaultUsageCategories(),
    settings: {
      commission_mode: 'per_wash',
      commission_per_wash_xof: 500,
      monthly_goal_xof: 1000000,
      require_open_cash_session: true,
      max_cashier_discount_xof: 1000,
      receipt_footer: 'Merci de votre visite !',
    },
    is_default: true,
    is_active: true,
    created_at: new Date().toISOString(),
  }
  stations.push(newStation)

  equipments.push(
    { id: `eq-${Date.now()}-1`, station_id: stationId, template_code: 'karcher_hd', name: 'Nettoyeur haute pression Kärcher HD 7/18', quantity: 2, state: 'good', acquired_on: new Date().toISOString().slice(0, 10), is_active: true },
    { id: `eq-${Date.now()}-2`, station_id: stationId, template_code: 'aspirateur_eau_poussiere', name: 'Aspirateur eau et poussière tri-moteurs', quantity: 1, state: 'good', acquired_on: new Date().toISOString().slice(0, 10), is_active: true }
  )

  const newSub: Subscription = {
    id: `sub-${Date.now()}`,
    organization_id: orgId,
    plan_id: 'plan-essai',
    status: 'trialing',
    started_at: new Date().toISOString(),
    trial_ends_at: new Date(Date.now() + 14 * 86400000).toISOString(),
    current_period_start: new Date().toISOString(),
    current_period_end: new Date(Date.now() + 14 * 86400000).toISOString(),
  }
  subscriptions.push(newSub)

  const newUser: User = {
    id: userId,
    organization_id: orgId,
    station_id: stationId,
    full_name,
    email: email.trim().toLowerCase(),
    phone: phone || '',
    password_hash: password,
    role: 'owner',
    is_active: true,
    must_change_password: false,
    created_at: new Date().toISOString(),
  }
  users.push(newUser)

  // Copy default services
  services.forEach((srv, idx) => {
    services.push({
      ...srv,
      id: `srv-${Date.now()}-${idx}`,
      organization_id: orgId,
      station_id: stationId,
    })
  })

  const token = `token-${Date.now()}-${Math.random().toString(36).substring(2)}`
  activeSessions.set(token, newUser.id)
  setSessionCookie(res, token, req)

  res.status(201).json({ ...buildMePayload(newUser), session_token: token })
})

api.post('/auth/logout', (req, res) => {
  const authHeader = req.headers['authorization']
  const bearerToken = authHeader?.startsWith('Bearer ') ? authHeader.slice(7).trim() : null
  const token = req.cookies?.washguard_session || bearerToken
  if (token) activeSessions.delete(token)
  res.clearCookie('washguard_session')
  res.json({ ok: true })
})

api.get('/auth/me', requireAuth, (req, res) => {
  const user = (req as any).user as User
  res.json(buildMePayload(user))
})

api.patch('/auth/password', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const { current_password, new_password } = req.body || {}
  if (user.password_hash !== current_password) {
    return res.status(400).json({ error: { code: 'validation_error', message: 'Mot de passe actuel incorrect.', fields: { current_password: 'Mot de passe incorrect' } } })
  }
  user.password_hash = new_password
  user.must_change_password = false
  res.json({ message: 'Mot de passe mis à jour avec succès.' })
})

// Organizations
api.get('/organizations/current', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const org = organizations.find((o) => o.id === user.organization_id)
  const st = stations.find((s) => s.id === user.station_id) || stations[0] || null
  const orgStations = stations.filter((s) => s.organization_id === user.organization_id)
  const orgEmployees = users.filter((u) => u.organization_id === user.organization_id)
  const orgCustomers = customers.filter((c) => c.organization_id === user.organization_id)

  res.json({
    organization: org,
    station: st,
    registry: [
      { code: 'rccm', label: 'Numéro RCCM', value: org?.rccm || '', status: org?.rccm ? 'valid' : 'missing' },
      { code: 'tax_id', label: 'Compte Contribuable', value: org?.tax_id || '', status: org?.tax_id ? 'valid' : 'missing' },
    ],
    counts: {
      stations: orgStations.length,
      employees: orgEmployees.length,
      customers: orgCustomers.length,
    },
  })
})

api.patch('/organizations/current', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const org = organizations.find((o) => o.id === user.organization_id)
  if (!org) return res.status(404).json({ error: { message: 'Organisation introuvable.' } })
  Object.assign(org, req.body)
  addAudit('organization.updated', 'organization', org.id, user.full_name)
  res.json({ message: 'Organisation mise à jour.', fields: Object.keys(req.body) })
})

api.get('/organizations/subscription', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const me = buildMePayload(user)
  res.json({
    subscription: me.subscription,
    plans: plans.filter((p) => p.is_active),
    payment_note: 'Paiement sécurisé par Orange Money, Wave ou Virement bancaire.',
  })
})

api.post('/organizations/subscription/request', requireAuth, (req, res) => {
  const user = (req as any).user as User
  addAudit('subscription.requested', 'subscription', user.organization_id || '', user.full_name, req.body)
  res.json({ message: 'Votre demande a bien été transmise à notre équipe.' })
})

api.get('/organizations/audit', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const items = auditLogs.filter((a) => !a.organization_id || a.organization_id === user.organization_id)
  res.json({ items, total: items.length })
})

// Stations
api.get('/stations', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const items = stations.filter((s) => s.organization_id === user.organization_id)
  res.json({ items, total: items.length })
})

api.post('/stations', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const {
    name,
    code,
    phone,
    address,
    city,
    commune,
    district_area,
    timezone,
    bays,
    water_source,
    water_reserve_liters,
    wash_types,
  } = req.body || {}

  if (!name || !name.trim()) {
    return res.status(400).json({ error: { code: 'validation_error', message: 'Le nom de la station est obligatoire.' } })
  }

  const stationId = `st-${Date.now()}`
  const site = getDefaultStationSite(name)
  if (bays) site.bays = Number(bays)
  if (water_source) site.water_source = water_source
  if (water_reserve_liters) site.water_reserve_liters = Number(water_reserve_liters)
  if (wash_types && Array.isArray(wash_types) && wash_types.length > 0) site.wash_types = wash_types

  const newStation: Station = {
    id: stationId,
    organization_id: user.organization_id || 'org-demo',
    name: name.trim(),
    code: (code && code.trim()) || name.trim().slice(0, 3).toUpperCase(),
    phone: phone || '',
    address: address || '',
    city: city || 'Abidjan',
    country: "Côte d'Ivoire",
    district: city || 'Abidjan',
    commune: commune || '',
    district_area: district_area || '',
    street: address || '',
    landmark: '',
    latitude: '5.3489',
    longitude: '-4.0823',
    billing_address: address || '',
    timezone: timezone || 'Africa/Abidjan',
    currency: 'XOF',
    logo_url: '',
    opening_hours: {},
    site,
    opening_hours_days: getDefaultOpeningDays(),
    opening_exceptions: [],
    payment_methods: getDefaultPaymentMethods(stationId),
    order_rules: getDefaultOrderRules(),
    vehicle_categories: getDefaultVehicleCategories(),
    vehicle_usages: getDefaultVehicleUsages(),
    usage_categories: getDefaultUsageCategories(),
    settings: {
      commission_mode: 'per_wash',
      commission_per_wash_xof: 500,
      monthly_goal_xof: 1200000,
      require_open_cash_session: true,
      max_cashier_discount_xof: 1000,
      receipt_footer: `Merci de votre confiance. À très bientôt chez ${name.trim()} !`,
    },
    is_default: stations.filter((s) => s.organization_id === user.organization_id).length === 0,
    is_active: true,
    created_at: new Date().toISOString(),
  }

  stations.push(newStation)

  // Seed default services for this newly created station
  const defaultServices = [
    { name: 'Lavage simple extérieur', category: 'lavage', description: 'Nettoyage carrosserie, jantes et vitres', price_xof: 2000, duration_minutes: 25 },
    { name: 'Lavage complet (Intérieur + Extérieur)', category: 'lavage', description: 'Nettoyage complet avec aspiration et tableau de bord', price_xof: 4000, duration_minutes: 45 },
    { name: 'Lavage Express', category: 'lavage', description: 'Rinçage haute pression et shampoing rapide', price_xof: 1500, duration_minutes: 15 },
    { name: 'Aspiration & Habitacle', category: 'interieur', description: 'Aspiration moquettes, sièges et coffre', price_xof: 1500, duration_minutes: 20 },
    { name: 'Lavage compartiment moteur', category: 'moteur', description: 'Dégraissage et séchage du moteur', price_xof: 3000, duration_minutes: 30 },
  ]
  defaultServices.forEach((s, idx) => {
    services.push({
      id: `srv-${Date.now()}-${idx}`,
      organization_id: user.organization_id || 'org-demo',
      station_id: stationId,
      name: s.name,
      category: s.category,
      description: s.description,
      price_xof: s.price_xof,
      duration_minutes: s.duration_minutes,
      sort_order: idx + 1,
      is_active: true,
    })
  })

  // Seed default equipments
  equipments.push(
    { id: `eq-${Date.now()}-1`, station_id: stationId, template_code: 'karcher_hd', name: 'Nettoyeur haute pression Kärcher', quantity: 2, state: 'good', acquired_on: new Date().toISOString().slice(0, 10), is_active: true },
    { id: `eq-${Date.now()}-2`, station_id: stationId, template_code: 'aspirateur_pro', name: 'Aspirateur professionnel bi-moteur', quantity: 1, state: 'good', acquired_on: new Date().toISOString().slice(0, 10), is_active: true }
  )

  if (!user.station_id) {
    user.station_id = stationId
  }

  addAudit('station.created', 'station', newStation.id, user.full_name)
  res.status(201).json(newStation)
})

api.patch('/stations/:id', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const st = stations.find((s) => s.id === req.params.id)
  if (!st) return res.status(404).json({ error: { message: 'Station introuvable.' } })
  const { site, settings, ...directFields } = req.body || {}
  Object.assign(st, directFields)
  if (site) {
    st.site = { ...(st.site || getDefaultStationSite(st.name)), ...site }
  }
  if (settings) {
    st.settings = { ...(st.settings || {}), ...settings }
  }
  addAudit('station.updated', 'station', st.id, user.full_name)
  res.json(st)
})

api.put('/stations/:id/profile', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const st = stations.find((s) => s.id === req.params.id)
  if (!st) return res.status(404).json({ error: { message: 'Station introuvable.' } })
  const { site, settings, ...directFields } = req.body || {}
  Object.assign(st, directFields)
  if (site) {
    st.site = { ...(st.site || getDefaultStationSite(st.name)), ...site }
  }
  if (settings) {
    st.settings = { ...(st.settings || {}), ...settings }
  }
  addAudit('station.profile_updated', 'station', st.id, user.full_name)
  res.json(st)
})

api.get('/stations/:id/profile', requireAuth, (req, res) => {
  const st = stations.find((s) => s.id === req.params.id) || stations[0]
  if (!st) return res.status(404).json({ error: { message: 'Station introuvable.' } })
  if (!st.site) st.site = getDefaultStationSite(st.name)
  if (!st.opening_hours_days) st.opening_hours_days = getDefaultOpeningDays()
  if (!st.opening_exceptions) st.opening_exceptions = []
  const types_lavage_proposes = [
    'manuel',
    'haute_pression',
    'mousse_active',
    'portique_automatique',
    'tunnel',
    'vapeur',
    'sans_eau_ecologique',
  ]
  res.json({
    station: st,
    site: st.site,
    days: st.opening_hours_days,
    exceptions: st.opening_exceptions,
    types_lavage_proposes,
  })
})

api.put('/stations/:id/opening-hours', requireAuth, (req, res) => {
  const st = stations.find((s) => s.id === req.params.id) || stations[0]
  if (st) {
    if (req.body.days) st.opening_hours_days = req.body.days
    if (req.body.exceptions) st.opening_exceptions = req.body.exceptions
  }
  res.json({
    days: st?.opening_hours_days || [],
    exceptions: st?.opening_exceptions || [],
    message: 'Horaires enregistrés avec succès.',
  })
})

api.get('/stations/:id/payment-methods', requireAuth, (req, res) => {
  const st = stations.find((s) => s.id === req.params.id) || stations[0]
  if (st && !st.payment_methods) st.payment_methods = getDefaultPaymentMethods(st.id)
  const items = st?.payment_methods || paymentMethodsDeclared
  res.json({ items, total: items.length, actifs: items.filter((p) => p.is_active).length })
})

api.put('/stations/:id/payment-methods', requireAuth, (req, res) => {
  const st = stations.find((s) => s.id === req.params.id) || stations[0]
  if (st && req.body.methods) {
    st.payment_methods = req.body.methods
  }
  const items = st?.payment_methods || paymentMethodsDeclared
  res.json({ items, message: 'Moyens de paiement enregistrés avec succès.' })
})

api.get('/stations/:id/equipment', requireAuth, (req, res) => {
  const items = equipments.filter((e) => e.station_id === req.params.id || !e.station_id)
  res.json({ items, total: items.length })
})

api.post('/stations/:id/equipment', requireAuth, (req, res) => {
  const eq = { id: `eq-${Date.now()}`, station_id: req.params.id, ...req.body, is_active: true }
  equipments.push(eq)
  res.status(201).json({ id: eq.id })
})

api.patch('/equipment/:id', requireAuth, (req, res) => {
  const eq = equipments.find((e) => e.id === req.params.id)
  if (eq) Object.assign(eq, req.body)
  res.json({ ok: true })
})

api.get('/stations/:id/stock', requireAuth, (req, res) => {
  const items = stockItems.filter((s) => s.station_id === req.params.id || !s.station_id)
  res.json({ items, total: items.length })
})

api.post('/stations/:id/stock/items', requireAuth, (req, res) => {
  const item = { id: `stk-${Date.now()}`, station_id: req.params.id, ...req.body, is_active: true }
  stockItems.push(item)
  res.status(201).json({ id: item.id })
})

api.patch('/stock/items/:id', requireAuth, (req, res) => {
  const it = stockItems.find((s) => s.id === req.params.id)
  if (it) Object.assign(it, req.body)
  res.json({ ok: true })
})

api.get('/stock/items/:id/movements', requireAuth, (req, res) => {
  const it = stockItems.find((s) => s.id === req.params.id)
  res.json({ item_name: it?.name || 'Article', unit: it?.unit || 'unité', items: stockMovements.filter((m) => m.item_id === req.params.id) })
})

api.post('/stock/items/:id/movements', requireAuth, (req, res) => {
  const mov = { id: `mov-${Date.now()}`, item_id: req.params.id, ...req.body, at: new Date().toISOString() }
  stockMovements.unshift(mov)
  res.status(201).json(mov)
})

api.get('/stations/:id/order-rules', requireAuth, (req, res) => {
  const st = stations.find((s) => s.id === req.params.id) || stations[0]
  if (st && !st.order_rules) st.order_rules = getDefaultOrderRules()
  res.json({
    rules: st?.order_rules || getDefaultOrderRules(),
    statuses: [
      { code: 'pending', label: 'Reçue / En attente', description: 'Le véhicule est stationné dans la file d’attente.', terminal: false },
      { code: 'in_progress', label: 'Lavage en cours', description: 'Le laveur affecté traite le véhicule sur piste.', terminal: false },
      { code: 'quality_check', label: 'Contrôle qualité', description: 'Vérification des finitions avant remise.', terminal: false },
      { code: 'ready', label: 'Véhicule prêt', description: 'Lavage terminé, prêt pour remise au client.', terminal: false },
      { code: 'completed', label: 'Restitué & Réglé', description: 'Fiche clôturée et véhicule remis.', terminal: true },
      { code: 'canceled', label: 'Annulé', description: 'Prestation annulée avec motif consigné.', terminal: true },
    ],
  })
})

api.put('/stations/:id/order-rules', requireAuth, (req, res) => {
  const st = stations.find((s) => s.id === req.params.id) || stations[0]
  if (st) {
    st.order_rules = { ...(st.order_rules || getDefaultOrderRules()), ...req.body }
  }
  res.json({ rules: st?.order_rules || req.body, message: 'Règles de commande enregistrées.' })
})

// Vehicle Categories & Usages
api.get('/vehicle-categories', requireAuth, (_req, res) => {
  res.json({ items: vehicleCategories })
})

api.post('/vehicle-categories', requireAuth, (req, res) => {
  const cat: VehicleCategory = {
    id: `cat-${Date.now()}`,
    code: req.body.code || `cat_${Date.now()}`,
    name: req.body.name,
    description: req.body.description || '',
    sort_order: vehicleCategories.length + 1,
    is_active: true,
    family: req.body.family,
  }
  vehicleCategories.push(cat)
  res.status(201).json({ items: vehicleCategories })
})

api.patch('/vehicle-categories/:code', requireAuth, (req, res) => {
  const cat = vehicleCategories.find((c) => c.code === req.params.code || c.id === req.params.code)
  if (cat) Object.assign(cat, req.body)
  res.json({ items: vehicleCategories })
})

api.get('/vehicle-usages', requireAuth, (_req, res) => {
  res.json({ items: vehicleUsages })
})

api.get('/vehicle-category-options', requireAuth, (req, res) => {
  const stationId = String(req.query.station_id || stations[0]?.id)
  const st = stations.find((s) => s.id === stationId) || stations[0]
  const cats = (st?.vehicle_categories || getDefaultVehicleCategories()).filter((c) => c.is_active)
  const uses = (st?.vehicle_usages || getDefaultVehicleUsages()).filter((u) => u.is_active)
  res.json({
    categories: cats,
    usages: uses,
  })
})

api.get('/vehicle-catalogue', requireAuth, (req, res) => {
  const stationId = String(req.query.station_id || stations[0]?.id)
  const st = stations.find((s) => s.id === stationId) || stations[0]
  if (st && !st.vehicle_categories) st.vehicle_categories = getDefaultVehicleCategories()
  if (st && !st.vehicle_usages) st.vehicle_usages = getDefaultVehicleUsages()

  const categories = (st?.vehicle_categories || getDefaultVehicleCategories()).map((cat, i) => ({
    code: cat.code,
    name: cat.name,
    family: cat.code === 'moto' ? 'Deux-roues' : cat.code === 'utilitaire' || cat.code === 'minibus' ? 'Utilitaires' : 'Véhicules légers',
    description: cat.code === 'citadine' ? 'Voitures compactes de gabarit standard' : cat.code === 'suv_4x4' ? 'Grands véhicules ou tout-terrain' : 'Véhicule de catégorie ' + cat.name,
    aliases: [cat.code, cat.name],
    sort_order: cat.sort_order ?? (i + 1) * 10,
    is_active: cat.is_active,
    configured: true,
    local_name: cat.local_name || cat.name,
  }))

  const usages = (st?.vehicle_usages || getDefaultVehicleUsages()).map((u, i) => ({
    code: u.code,
    name: u.name,
    family: u.code === 'taxi_compteur' || u.code === 'vtc' ? 'Transport de personnes' : u.code === 'livraison' || u.code === 'flotte' ? 'Usage professionnel' : 'Usage privé',
    description: u.code === 'taxi_compteur' ? 'Taxis urbains enregistrés' : u.code === 'vtc' ? 'Véhicules Yango / Uber / chauffeurs privés' : 'Clients réguliers pour leur véhicule personnel',
    aliases: [u.code, u.name],
    sort_order: u.sort_order ?? (i + 1) * 10,
    is_active: u.is_active,
    configured: true,
    local_name: u.name,
  }))

  res.json({
    station_id: stationId,
    categories,
    usages,
    actives: categories.filter((c) => c.is_active).length,
    usages_actifs: usages.filter((u) => u.is_active).length,
    familles_categories: Array.from(new Set(categories.map((c) => c.family))),
    familles_usages: Array.from(new Set(usages.map((u) => u.family))),
    message_referentiel: 'Référentiel synchronisé avec la station.',
  })
})

api.get('/vehicle-usage-categories', requireAuth, (req, res) => {
  const stationId = String(req.query.station_id || stations[0]?.id)
  const st = stations.find((s) => s.id === stationId) || stations[0]
  if (st && !st.usage_categories) st.usage_categories = getDefaultUsageCategories()
  const items = (st?.usage_categories || getDefaultUsageCategories()).map((c, i) => ({
    usage_code: c.usage_code,
    category_code: c.category_code,
    is_active: c.is_active ?? true,
    sort_order: (i + 1) * 10,
  }))
  res.json({ items, total: items.length, station_id: stationId })
})

api.put('/stations/:id/vehicle-categories', requireAuth, (req, res) => {
  const st = stations.find((s) => s.id === req.params.id) || stations[0]
  if (st && req.body.categories) {
    st.vehicle_categories = req.body.categories.map((c: any, index: number) => ({
      code: c.code,
      name: c.name || c.code,
      local_name: c.local_name || c.name,
      is_active: Boolean(c.is_active),
      sort_order: c.sort_order || (index + 1) * 10,
    }))
  }
  const actives = (st?.vehicle_categories || []).filter((c) => c.is_active).length
  res.json({ items: st?.vehicle_categories || [], actives, message: 'Catégories enregistrées avec succès.' })
})

api.put('/stations/:id/vehicle-usages', requireAuth, (req, res) => {
  const st = stations.find((s) => s.id === req.params.id) || stations[0]
  if (st && req.body.usages) {
    st.vehicle_usages = req.body.usages.map((u: any, index: number) => ({
      code: u.code,
      name: u.name || u.code,
      is_active: Boolean(u.is_active),
      sort_order: u.sort_order || (index + 1) * 10,
    }))
  }
  const actifs = (st?.vehicle_usages || []).filter((u) => u.is_active).length
  res.json({ items: st?.vehicle_usages || [], actifs, message: 'Usages commerciaux enregistrés avec succès.' })
})

api.put('/stations/:id/vehicle-usage-categories', requireAuth, (req, res) => {
  const st = stations.find((s) => s.id === req.params.id) || stations[0]
  if (st && req.body.couples) {
    st.usage_categories = req.body.couples.map((c: any) => ({
      usage_code: c.usage_code,
      category_code: c.category_code,
      is_active: true,
    }))
  }
  res.json({
    items: st?.usage_categories || [],
    total: (st?.usage_categories || []).length,
    message: 'Combinaisons (usages × catégories) enregistrées avec succès.',
  })
})

api.put('/stations/:id/service-families', requireAuth, (_req, res) => {
  res.json({ message: 'Familles de prestations mises à jour.' })
})

// Services
api.get('/services', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const stationId = req.query.station_id || user.station_id || stations[0]?.id
  const includeInactive = req.query.include_inactive === 'true'
  const items = services.filter((s) => s.station_id === stationId && (includeInactive || s.is_active))
  const categories = Array.from(new Set(items.map((s) => s.category)))
  res.json({ items, categories })
})

api.post('/services', requireAuth, (req, res) => {
  const user = (req as any).user as User
  if (user.role === 'cashier' || user.role === 'washer') {
    return res.status(403).json({
      error: { code: 'forbidden', message: 'Seul le gérant ou le propriétaire est autorisé à ajouter des prestations.' },
    })
  }
  const stId = req.body.station_id || user.station_id || stations[0]?.id
  const newSrv: Service = {
    id: `srv-${Date.now()}`,
    organization_id: user.organization_id || 'org-demo',
    station_id: stId,
    name: req.body.name || req.body.template_code || 'Prestation',
    category: req.body.category || 'lavage',
    description: req.body.description || '',
    price_xof: Number(req.body.price_xof) || 2000,
    duration_minutes: Number(req.body.duration_minutes) || 30,
    sort_order: Number(req.body.sort_order) || services.length + 1,
    is_active: true,
  }
  services.push(newSrv)
  addAudit('service.created', 'service', newSrv.id, user.full_name)
  res.status(201).json(newSrv)
})

api.patch('/services/:id', requireAuth, (req, res) => {
  const srv = services.find((s) => s.id === req.params.id)
  if (!srv) return res.status(404).json({ error: { message: 'Prestation introuvable.' } })
  Object.assign(srv, req.body)
  res.json(srv)
})

api.delete('/services/:id', requireAuth, (req, res) => {
  const srv = services.find((s) => s.id === req.params.id)
  if (srv) srv.is_active = false
  res.json({ message: 'Prestation désactivée.' })
})

api.get('/services/:id/compatibilities', requireAuth, (req, res) => {
  res.json({
    service_id: req.params.id,
    categories: ['moto', 'citadine', 'berline', 'suv_4x4', 'utilitaire'],
    categories_disponibles: vehicleCategories.map((c) => ({ code: c.code || c.id, name: c.name })),
    message: 'Compatibilités chargées.',
  })
})

api.put('/services/:id/compatibilities', requireAuth, (_req, res) => {
  res.json({ message: 'Compatibilités enregistrées.' })
})

api.get('/service-templates', requireAuth, (req, res) => {
  res.json({
    families: [
      { code: 'lavage', name: 'Lavage & Extérieur', configured: true, is_active: true },
      { code: 'interieur', name: 'Habitacle & Tissus', configured: true, is_active: true },
      { code: 'moteur', name: 'Compartiment Moteur', configured: true, is_active: true },
      { code: 'finition', name: 'Soins & Finition', configured: true, is_active: true },
    ],
    templates: [
      { code: 'lav_ext_simple', name: 'Lavage Extérieur Simple', family_code: 'lavage', family_name: 'Lavage & Extérieur', description: 'Carrosserie, vitres et jantes au jet haute pression', duration_minutes: 20, suggested_price_xof: 2000 },
      { code: 'lav_complet', name: 'Lavage Complet', family_code: 'lavage', family_name: 'Lavage & Extérieur', description: 'Extérieur, habitacle, dépoussiérage et aspiration complète', duration_minutes: 45, suggested_price_xof: 4000 },
      { code: 'lav_express', name: 'Lavage Express Mousse', family_code: 'lavage', family_name: 'Lavage & Extérieur', description: 'Nettoyage carrosserie rapide sans essuyage approfondi', duration_minutes: 15, suggested_price_xof: 1500 },
      { code: 'net_sieges', name: 'Shampouinage des Sièges', family_code: 'interieur', family_name: 'Habitacle & Tissus', description: 'Nettoyage en profondeur des tissus ou cuirs des sièges', duration_minutes: 60, suggested_price_xof: 5000 },
      { code: 'asp_interieure', name: 'Aspiration Intérieure Approfondie', family_code: 'interieur', family_name: 'Habitacle & Tissus', description: 'Aspiration des tapis, moquettes, sièges et coffre', duration_minutes: 20, suggested_price_xof: 1500 },
      { code: 'net_moteur', name: 'Nettoyage Compartiment Moteur', family_code: 'moteur', family_name: 'Compartiment Moteur', description: 'Dégraissage haute sécurité et lustrage des plastiques', duration_minutes: 30, suggested_price_xof: 3000 },
      { code: 'lustrage_cire', name: 'Lustrage Carrosserie Haute Protection', family_code: 'finition', family_name: 'Soins & Finition', description: 'Cire de carnauba pour brillance éclatante et déperlance', duration_minutes: 60, suggested_price_xof: 6000 },
    ],
  })
})

api.get('/service-options', requireAuth, (req, res) => {
  const stationId = String(req.query.station_id || stations[0]?.id)
  const items = serviceOptions.filter((o) => !stationId || o.station_id === stationId)
  res.json({ items, total: items.length })
})

api.post('/service-options', requireAuth, (req, res) => {
  const opt: ServiceOption = {
    id: `opt-${Date.now()}`,
    station_id: req.body.station_id || stations[0]?.id,
    name: req.body.name,
    description: req.body.description || '',
    price_xof: Number(req.body.price_xof) || 1000,
    duration_minutes: Number(req.body.duration_minutes) || 10,
    sort_order: serviceOptions.length + 1,
    is_active: true,
    scope: req.body.scope || 'station',
  }
  serviceOptions.push(opt)
  res.status(201).json(opt)
})

api.patch('/service-options/:id', requireAuth, (req, res) => {
  const opt = serviceOptions.find((o) => o.id === req.params.id)
  if (opt) Object.assign(opt, req.body)
  res.json(opt || { ok: true })
})

api.delete('/service-options/:id', requireAuth, (req, res) => {
  const opt = serviceOptions.find((o) => o.id === req.params.id)
  if (opt) opt.is_active = false
  res.json({ ok: true })
})

api.get('/price-rules', requireAuth, (req, res) => {
  const stationId = String(req.query.station_id || stations[0]?.id)
  const items = priceRules.filter((r) => !stationId || r.station_id === stationId)
  res.json({ items, total: items.length })
})

api.post('/price-rules', requireAuth, (req, res) => {
  const pr: PriceRule = {
    id: `pr-${Date.now()}`,
    station_id: req.body.station_id || stations[0]?.id,
    service_id: req.body.service_id,
    category_code: req.body.category_code,
    usage_code: req.body.usage_code,
    customer_id: req.body.customer_id,
    customer_name: req.body.customer_name,
    price_xof: req.body.price_xof !== undefined ? Number(req.body.price_xof) : undefined,
    valid_from: req.body.valid_from,
    valid_to: req.body.valid_to,
    reason: req.body.reason || 'Règle personnalisée',
    is_active: req.body.is_active ?? true,
    created_at: new Date().toISOString(),
  }
  priceRules.push(pr)
  res.status(201).json(pr)
})

api.patch('/price-rules/:id', requireAuth, (req, res) => {
  const pr = priceRules.find((r) => r.id === req.params.id)
  if (pr) Object.assign(pr, req.body)
  res.json(pr || { ok: true })
})

api.delete('/price-rules/:id', requireAuth, (req, res) => {
  const pr = priceRules.find((r) => r.id === req.params.id)
  if (pr) pr.is_active = false
  res.json({ ok: true })
})

api.get('/price-rules/matrix', requireAuth, (req, res) => {
  const stationId = String(req.query.station_id || stations[0]?.id)
  const st = stations.find((s) => s.id === stationId) || stations[0]
  const stServices = services.filter((s) => s.station_id === stationId && s.is_active)
  const stCategories = (st?.vehicle_categories || getDefaultVehicleCategories()).filter((c) => c.is_active)
  const stUsages = (st?.vehicle_usages || getDefaultVehicleUsages()).filter((u) => u.is_active)

  const rows = stServices.map((srv) => {
    const cells = stCategories.map((cat) => {
      const match = priceRules.find((r) => r.station_id === stationId && r.service_id === srv.id && r.category_code === cat.code && r.is_active)
      return {
        category_code: cat.code,
        price_xof: match ? match.price_xof ?? null : srv.price_xof || null,
        rule_id: match?.id || '',
        source: match ? 'règle' : 'catalogue',
        is_free: false,
      }
    })

    const usage_cells = stUsages.map((usage) => {
      const match = priceRules.find((r) => r.station_id === stationId && r.service_id === srv.id && r.usage_code === usage.code && r.is_active)
      return {
        category_code: '',
        usage_code: usage.code,
        price_xof: match ? match.price_xof ?? null : null,
        rule_id: match?.id || '',
        source: match ? 'règle' : 'hérité',
        is_free: false,
      }
    })

    return {
      service_id: srv.id,
      name: srv.name,
      family_code: srv.category,
      family_name: srv.category === 'lavage' ? 'Lavage & Extérieur' : srv.category === 'interieur' ? 'Habitacle & Tissus' : srv.category === 'moteur' ? 'Moteur' : 'Finition',
      duration_minutes: srv.duration_minutes,
      generic_price_xof: srv.price_xof,
      is_free: false,
      is_quote_only: false,
      needs_price: !srv.price_xof,
      cells,
      usage_cells,
    }
  })

  res.json({
    station_id: stationId,
    services: rows,
    categories: stCategories.map((c) => ({ code: c.code, name: c.name, family: 'Véhicules' })),
    usages: stUsages.map((u) => ({ code: u.code, name: u.name, family: 'Usages' })),
    generique_autorise: true,
    a_tarifer: rows.filter((r) => r.needs_price).length,
    message: 'Matrice tarifaire synchronisée avec la station.',
  })
})

api.put('/price-rules/matrix', requireAuth, (req, res) => {
  const { station_id, cells, usage_cells, generic_prices, reason } = req.body || {}
  const stId = station_id || stations[0]?.id

  if (Array.isArray(cells)) {
    for (const cell of cells) {
      if (!cell.service_id || !cell.category_code) continue
      const existing = priceRules.find((r) => r.station_id === stId && r.service_id === cell.service_id && r.category_code === cell.category_code)
      if (existing) {
        existing.price_xof = cell.price_xof !== undefined ? Number(cell.price_xof) : existing.price_xof
        existing.reason = reason || 'Mise à jour matrice'
        existing.is_active = true
      } else if (cell.price_xof !== undefined && cell.price_xof !== null) {
        priceRules.push({
          id: `pr-${Date.now()}-${Math.random().toString(36).substring(2, 6)}`,
          station_id: stId,
          service_id: cell.service_id,
          category_code: cell.category_code,
          price_xof: Number(cell.price_xof),
          reason: reason || 'Saisie grille tarifaire',
          is_active: true,
          created_at: new Date().toISOString(),
        })
      }
    }
  }

  if (Array.isArray(usage_cells)) {
    for (const cell of usage_cells) {
      if (!cell.service_id || !cell.usage_code) continue
      const existing = priceRules.find((r) => r.station_id === stId && r.service_id === cell.service_id && r.usage_code === cell.usage_code)
      if (existing) {
        existing.price_xof = cell.price_xof !== undefined ? Number(cell.price_xof) : existing.price_xof
        existing.reason = reason || 'Mise à jour usage'
        existing.is_active = true
      } else if (cell.price_xof !== undefined && cell.price_xof !== null) {
        priceRules.push({
          id: `pr-${Date.now()}-${Math.random().toString(36).substring(2, 6)}`,
          station_id: stId,
          service_id: cell.service_id,
          usage_code: cell.usage_code,
          price_xof: Number(cell.price_xof),
          reason: reason || 'Saisie tarif usage',
          is_active: true,
          created_at: new Date().toISOString(),
        })
      }
    }
  }

  if (Array.isArray(generic_prices)) {
    for (const gp of generic_prices) {
      const srv = services.find((s) => s.id === gp.service_id)
      if (srv && gp.price_xof !== undefined) {
        srv.price_xof = Number(gp.price_xof)
      }
    }
  }

  res.json({ message: 'Grille tarifaire enregistrée avec succès.' })
})

api.get('/price-preview', requireAuth, (req, res) => {
  const stationId = String(req.query.station_id || stations[0]?.id)
  const serviceId = String(req.query.service_id || '')
  const categoryCode = String(req.query.category_code || '')
  const usageCode = String(req.query.usage_code || '')

  const srv = services.find((s) => s.id === serviceId)
  let foundPrice = srv ? srv.price_xof : 2000
  let source = 'catalogue'
  let sourceLabel = 'Prix du catalogue'

  if (usageCode) {
    const matchUsage = priceRules.find((r) => r.station_id === stationId && r.service_id === serviceId && r.usage_code === usageCode && r.is_active)
    if (matchUsage && matchUsage.price_xof !== undefined) {
      foundPrice = matchUsage.price_xof
      source = 'règle'
      sourceLabel = 'Règle usage commercial'
    }
  }

  if (categoryCode) {
    const matchCat = priceRules.find((r) => r.station_id === stationId && r.service_id === serviceId && r.category_code === categoryCode && r.is_active)
    if (matchCat && matchCat.price_xof !== undefined) {
      foundPrice = matchCat.price_xof
      source = 'règle'
      sourceLabel = 'Règle catégorie véhicule'
    }
  }

  const stOptions = serviceOptions.filter((o) => o.station_id === stationId && o.is_active)

  res.json({
    service_id: serviceId,
    service_name: srv?.name || 'Prestation',
    is_free: false,
    is_quote_only: false,
    date: new Date().toISOString().slice(0, 10),
    price_xof: foundPrice,
    price_source: source,
    price_source_label: sourceLabel,
    needs_price: !foundPrice,
    options: stOptions.map((o) => ({
      id: o.id,
      name: o.name,
      description: o.description,
      price_xof: o.price_xof,
      service_id: o.service_id || null,
      category_code: o.category_code || '',
      duration_minutes: o.duration_minutes,
      scope: o.scope || 'station',
    })),
  })
})

api.get('/payment-methods', requireAuth, (req, res) => {
  const stationId = String(req.query.station_id || stations[0]?.id)
  const st = stations.find((s) => s.id === stationId) || stations[0]
  if (st && !st.payment_methods) st.payment_methods = getDefaultPaymentMethods(st.id)
  const items = st?.payment_methods || paymentMethodsDeclared
  const actives = req.query.actives === '1'
  const list = actives ? items.filter((p) => p.is_active) : items
  res.json({ items: list, total: list.length, actifs: items.filter((p) => p.is_active).length })
})

api.post('/payment-methods', requireAuth, (req, res) => {
  const stationId = req.body.station_id || stations[0]?.id
  const st = stations.find((s) => s.id === stationId) || stations[0]
  if (st && !st.payment_methods) st.payment_methods = getDefaultPaymentMethods(st.id)
  const pm: DeclaredPaymentMethod = {
    id: `pm-${Date.now()}`,
    station_id: stationId,
    code: req.body.code || req.body.name.toLowerCase().replace(/[^a-z0-9]+/g, '_'),
    name: req.body.name,
    family: req.body.family || 'mobile_money',
    family_label: req.body.family === 'cash' ? 'Espèces' : req.body.family === 'card' ? 'Carte bancaire' : req.body.family === 'transfer' ? 'Virement' : 'Mobile Money',
    requires_reference: req.body.requires_reference ?? true,
    is_active: req.body.is_active ?? true,
    sort_order: ((st?.payment_methods || paymentMethodsDeclared).length + 1) * 10,
    kind: 'personnalise',
  }
  if (st?.payment_methods) st.payment_methods.push(pm)
  paymentMethodsDeclared.push(pm as any)
  res.status(201).json(pm)
})

api.patch('/payment-methods/:id', requireAuth, (req, res) => {
  const stationId = req.body.station_id || stations[0]?.id
  const st = stations.find((s) => s.id === stationId) || stations[0]
  const target = (st?.payment_methods || []).find((p) => p.id === req.params.id) || paymentMethodsDeclared.find((p) => p.id === req.params.id)
  if (target) Object.assign(target, req.body)
  res.json(target || { ok: true })
})

api.get('/stations/:id/cash-settings', requireAuth, (req, res) => {
  const st = stations.find((s) => s.id === req.params.id) || stations[0]
  res.json({
    station_id: st?.id || req.params.id,
    settings: {
      require_open_cash_session: st?.settings.require_open_cash_session ?? true,
      max_cashier_discount_xof: st?.settings.max_cashier_discount_xof ?? 1000,
      receipt_footer: st?.settings.receipt_footer ?? 'Merci de votre visite !',
      commission_mode: st?.settings.commission_mode ?? 'per_wash',
      commission_per_wash_xof: st?.settings.commission_per_wash_xof ?? 500,
      commission_percent: st?.settings.commission_percent ?? 15,
      monthly_goal_xof: st?.settings.monthly_goal_xof ?? 1500000,
    },
    roles_connus: [
      { code: 'owner', label: 'Propriétaire' },
      { code: 'manager', label: 'Gérant' },
      { code: 'cashier', label: 'Caissier' },
      { code: 'washer', label: 'Laveur' },
    ],
  })
})

api.put('/stations/:id/cash-settings', requireAuth, (req, res) => {
  const st = stations.find((s) => s.id === req.params.id) || stations[0]
  if (st) {
    st.settings = { ...(st.settings || {}), ...req.body }
  }
  res.json({
    station_id: st?.id || req.params.id,
    settings: st?.settings,
    message: 'Paramètres de caisse mis à jour avec succès.',
  })
})

api.get('/cash-settings', requireAuth, (_req, res) => {
  const st = stations[0]
  res.json({
    station_id: st?.id,
    settings: st?.settings,
    require_open_cash_session: st?.settings.require_open_cash_session ?? true,
    max_cashier_discount_xof: st?.settings.max_cashier_discount_xof ?? 1000,
    receipt_footer: st?.settings.receipt_footer ?? 'Merci de votre visite !',
  })
})

api.put('/cash-settings', requireAuth, (req, res) => {
  const st = stations[0]
  if (st) {
    st.settings = { ...(st.settings || {}), ...req.body }
  }
  res.json({ settings: st?.settings || req.body, message: 'Paramètres de caisse mis à jour.' })
})


// Customers
api.get('/customers', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const search = String(req.query.search || '').toLowerCase().trim()
  const status = req.query.status || 'all'

  let list = customers.filter((c) => c.organization_id === user.organization_id)
  if (status === 'active') list = list.filter((c) => c.is_active)
  if (status === 'inactive') list = list.filter((c) => !c.is_active)
  if (search) {
    list = list.filter(
      (c) =>
        c.full_name.toLowerCase().includes(search) ||
        c.phone.toLowerCase().includes(search) ||
        c.plate.toLowerCase().includes(search),
    )
  }

  const items = list.map((c) => {
    const custOrders = washOrders.filter((w) => w.customer_id === c.id && w.status !== 'canceled')
    const totalSpent = custOrders.reduce((sum, o) => sum + o.total_xof, 0)
    const lastVisit = custOrders.length > 0 ? custOrders[custOrders.length - 1].created_at : null
    return {
      ...c,
      wash_count: custOrders.length,
      total_spent_xof: totalSpent,
      last_visit_at: lastVisit,
    }
  })

  res.json({ items, total: items.length, page_size: items.length })
})

api.get('/customers/:id', requireAuth, (req, res) => {
  const cust = customers.find((c) => c.id === req.params.id)
  if (!cust) return res.status(404).json({ error: { message: 'Client introuvable.' } })

  const custOrders = washOrders.filter((w) => w.customer_id === cust.id)
  const totalSpent = custOrders.filter((w) => w.status !== 'canceled').reduce((sum, o) => sum + o.total_xof, 0)
  const lastVisit = custOrders.length > 0 ? custOrders[custOrders.length - 1].created_at : null

  const customerObj = {
    ...cust,
    wash_count: custOrders.length,
    total_spent_xof: totalSpent,
    last_visit_at: lastVisit,
  }

  const history = custOrders.map((o) => ({
    id: o.id,
    reference: o.reference,
    created_at: o.created_at,
    status: o.status,
    total_xof: o.total_xof,
    paid_xof: o.status === 'completed' ? o.total_xof : 0,
    due_xof: o.status === 'completed' ? 0 : o.total_xof,
    vehicle_category_name: o.vehicle_category_name,
    plate: o.plate,
    services: o.items?.map((it) => it.service_name).join(', ') || 'Lavage',
  }))

  res.json({ customer: customerObj, history })
})

api.post('/customers', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const newCust: Customer = {
    id: `cust-${Date.now()}`,
    organization_id: user.organization_id || 'org-demo',
    station_id: req.body.station_id || user.station_id || stations[0]?.id,
    full_name: req.body.full_name,
    phone: req.body.phone || '',
    plate: req.body.plate || '',
    vehicle_model: req.body.vehicle_model || '',
    notes: req.body.notes || '',
    is_active: true,
    created_at: new Date().toISOString(),
  }
  customers.push(newCust)
  addAudit('customer.created', 'customer', newCust.id, user.full_name)
  res.status(201).json(newCust)
})

api.patch('/customers/:id', requireAuth, (req, res) => {
  const cust = customers.find((c) => c.id === req.params.id)
  if (!cust) return res.status(404).json({ error: { message: 'Client introuvable.' } })
  Object.assign(cust, req.body)
  res.json(cust)
})

api.delete('/customers/:id', requireAuth, (req, res) => {
  const cust = customers.find((c) => c.id === req.params.id)
  if (cust) cust.is_active = false
  res.json({ message: 'Client archivé.' })
})

// Employees
api.get('/employees/options', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const stationId = req.query.station_id || user.station_id
  const orgEmployees = users.filter((u) => u.organization_id === user.organization_id && u.is_active && (!stationId || u.station_id === stationId || !u.station_id))
  const items = orgEmployees.map((u) => ({
    id: u.id,
    full_name: u.full_name,
    role: u.role,
    role_label: u.role === 'owner' ? 'Propriétaire' : u.role === 'manager' ? 'Gérant' : u.role === 'cashier' ? 'Caissier' : 'Laveur',
  }))
  res.json({ items })
})

api.get('/employees', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const orgEmployees = users.filter((u) => u.organization_id === user.organization_id)
  const items = orgEmployees.map((u) => {
    const userOrders = washOrders.filter((w) => w.employee_id === u.id && w.status === 'completed')
    const revenue = userOrders.reduce((sum, o) => sum + o.total_xof, 0)
    const station = stations.find((s) => s.id === u.station_id)
    return {
      id: u.id,
      full_name: u.full_name,
      email: u.email,
      phone: u.phone,
      role: u.role,
      role_label: u.role === 'owner' ? 'Propriétaire' : u.role === 'manager' ? 'Gérant' : u.role === 'cashier' ? 'Caissier' : 'Laveur',
      station_id: u.station_id,
      station_name: station?.name,
      is_active: u.is_active,
      last_login_at: u.last_login_at,
      created_at: u.created_at,
      washes_today: userOrders.filter((o) => o.business_day === new Date().toISOString().slice(0, 10)).length,
      washes_month: userOrders.length,
      revenue_today_xof: userOrders.filter((o) => o.business_day === new Date().toISOString().slice(0, 10)).reduce((s, o) => s + o.total_xof, 0),
      revenue_month_xof: revenue,
      collected_month_xof: revenue,
      commission_mode: u.commission_mode,
      commission_per_wash_xof: u.commission_per_wash_xof,
      commission_percent: u.commission_percent,
    }
  })
  res.json({
    items,
    total: items.length,
    roles: [
      { id: 'owner', label: 'Propriétaire', description: 'Accès complet à la station et la facturation.' },
      { id: 'manager', label: 'Gérant', description: 'Supervision quotidienne, caisse et rapports.' },
      { id: 'cashier', label: 'Caissier', description: 'Enregistrement des lavages et encaissement.' },
      { id: 'washer', label: 'Laveur', description: 'Suivi des lavages attribués et contrôle qualité.' },
    ],
  })
})

api.get('/employees/:id/activity', requireAuth, (req, res) => {
  const emp = users.find((u) => u.id === req.params.id)
  if (!emp) return res.status(404).json({ error: { message: 'Employé introuvable.' } })

  const empOrders = washOrders.filter((w) => w.employee_id === emp.id)
  const completedOrders = empOrders.filter((w) => w.status === 'completed')
  const revenue = completedOrders.reduce((sum, o) => sum + o.total_xof, 0)

  res.json({
    employee: emp,
    range: { from: new Date(Date.now() - 30 * 86400000).toISOString().slice(0, 10), to: new Date().toISOString().slice(0, 10) },
    totals: {
      wash_count: empOrders.length,
      completed_count: completedOrders.length,
      canceled_count: empOrders.filter((w) => w.status === 'canceled').length,
      revenue_xof: revenue,
      collected_xof: revenue,
      discount_xof: empOrders.reduce((sum, o) => sum + o.discount_xof, 0),
    },
    orders: empOrders.slice(0, 50).map((o) => ({
      id: o.id,
      reference: o.reference,
      created_at: o.created_at,
      status: o.status,
      customer_name: o.customer_name,
      plate: o.plate,
      vehicle_category_name: o.vehicle_category_name,
      total_xof: o.total_xof,
      discount_xof: o.discount_xof,
      paid_xof: o.status === 'completed' ? o.total_xof : 0,
      due_xof: o.status === 'completed' ? 0 : o.total_xof,
      payment_methods: ['especes'],
    })),
  })
})

api.post('/employees', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const tempPass = Math.random().toString(36).slice(-8)
  const newEmp: User = {
    id: `usr-${Date.now()}`,
    organization_id: user.organization_id || 'org-demo',
    station_id: req.body.station_id || user.station_id || stations[0]?.id,
    full_name: req.body.full_name,
    email: req.body.email.trim().toLowerCase(),
    phone: req.body.phone || '',
    password_hash: req.body.password || tempPass,
    role: req.body.role || 'cashier',
    is_active: true,
    must_change_password: !req.body.password,
    commission_mode: req.body.commission_mode,
    commission_per_wash_xof: req.body.commission_per_wash_xof,
    commission_percent: req.body.commission_percent,
    created_at: new Date().toISOString(),
  }
  users.push(newEmp)
  addAudit('employee.created', 'employee', newEmp.id, user.full_name)
  res.status(201).json({
    id: newEmp.id,
    message: 'Employé créé.',
    email: newEmp.email,
    role: newEmp.role,
    password_generated: !req.body.password,
    temporary_password: req.body.password ? undefined : tempPass,
  })
})

api.patch('/employees/:id', requireAuth, (req, res) => {
  const emp = users.find((u) => u.id === req.params.id)
  if (!emp) return res.status(404).json({ error: { message: 'Employé introuvable.' } })
  Object.assign(emp, req.body)
  res.json(emp)
})

api.post('/employees/:id/reset-password', requireAuth, (req, res) => {
  const emp = users.find((u) => u.id === req.params.id)
  if (!emp) return res.status(404).json({ error: { message: 'Employé introuvable.' } })
  const newPass = Math.random().toString(36).slice(-8)
  emp.password_hash = newPass
  emp.must_change_password = true
  res.json({ temporary_password: newPass, message: 'Mot de passe réinitialisé.' })
})

// Invitations
api.get('/invitations', requireAuth, (_req, res) => {
  res.json({ items: invitations, total: invitations.length })
})

api.post('/invitations', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const code = Math.random().toString(36).substring(2, 8).toUpperCase()
  const token = `inv-${Date.now()}-${Math.random().toString(36).substring(2)}`
  const inv = {
    id: `inv-${Date.now()}`,
    organization_id: user.organization_id,
    station_id: req.body.station_id || user.station_id,
    email: req.body.email || '',
    phone: req.body.phone || '',
    role: req.body.role || 'washer',
    role_label: req.body.role === 'manager' ? 'Gérant' : req.body.role === 'cashier' ? 'Caissier' : 'Laveur',
    code,
    token,
    status: 'pending',
    expires_at: new Date(Date.now() + 7 * 86400000).toISOString(),
    created_at: new Date().toISOString(),
  }
  invitations.unshift(inv)
  res.status(201).json({
    id: inv.id,
    code: inv.code,
    token: inv.token,
    role: inv.role,
    expires_at: inv.expires_at,
    message: 'Invitation créée avec succès.',
    notice: 'Transmettez le lien ou le code directement à votre collaborateur.',
  })
})

api.post('/invitations/:id/revoke', requireAuth, (req, res) => {
  const inv = invitations.find((i) => i.id === req.params.id)
  if (inv) inv.status = 'revoked'
  res.json({ message: 'Invitation révoquée.' })
})

api.get('/invitations/:code', (_req, res) => {
  const inv = invitations.find((i) => i.code === _req.params.code || i.token === _req.params.code) || {
    id: 'inv-preview',
    organization_name: 'Station Étoile',
    role: 'cashier',
    role_label: 'Caissier',
    expires_at: new Date(Date.now() + 3 * 86400000).toISOString(),
    is_valid: true,
  }
  res.json(inv)
})

// Washer Portal
api.get('/washer/tasks', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const list = washOrders.filter((w) => w.station_id === (user.station_id || stations[0]?.id) && (w.status === 'in_progress' || w.status === 'pending' || w.status === 'quality_check'))
  res.json({ items: list })
})

api.get('/washer/me/performance', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const today = new Date().toISOString().slice(0, 10)
  const myToday = washOrders.filter((w) => w.employee_id === user.id && w.business_day === today && w.status === 'completed')
  const myMonth = washOrders.filter((w) => w.employee_id === user.id && w.status === 'completed')
  const rate = user.commission_per_wash_xof || 500
  res.json({
    washes_today: myToday.length,
    washes_month: myMonth.length,
    commission_today_xof: myToday.length * rate,
    commission_month_xof: myMonth.length * rate,
    commission_rate_label: `${rate.toLocaleString('fr-FR')} FCFA par lavage terminé`,
  })
})

api.post('/washer/tasks/:id/status', requireAuth, (req, res) => {
  const order = washOrders.find((w) => w.id === req.params.id)
  if (!order) return res.status(404).json({ error: { message: 'Tâche introuvable.' } })
  order.status = req.body.status
  if (req.body.status === 'completed') order.completed_at = new Date().toISOString()
  res.json(order)
})

api.post('/washer/tasks/:id/incident', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const order = washOrders.find((w) => w.id === req.params.id)
  if (!order) return res.status(404).json({ error: { message: 'Tâche introuvable.' } })
  order.incident_note = req.body.incident_note
  order.incident_reported_at = new Date().toISOString()
  order.incident_reporter_name = user.full_name
  res.json(order)
})

// Wash Orders
api.get('/wash-orders', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const from = req.query.from ? String(req.query.from) : ''
  const to = req.query.to ? String(req.query.to) : ''
  const status = req.query.status ? String(req.query.status) : ''
  const search = String(req.query.search || '').toLowerCase().trim()
  const openOnly = req.query.open_only === 'true'

  let list = washOrders.filter((w) => w.organization_id === user.organization_id)
  if (from) list = list.filter((w) => w.business_day >= from)
  if (to) list = list.filter((w) => w.business_day <= to)
  if (status) list = list.filter((w) => w.status === status)
  if (openOnly) list = list.filter((w) => w.status === 'pending' || w.status === 'in_progress')
  if (search) {
    list = list.filter(
      (w) =>
        w.reference.toLowerCase().includes(search) ||
        w.customer_name.toLowerCase().includes(search) ||
        w.plate.toLowerCase().includes(search) ||
        w.employee_name.toLowerCase().includes(search),
    )
  }

  list.sort((a, b) => b.created_at.localeCompare(a.created_at))

  const completed = list.filter((w) => w.status === 'completed')
  const totals = {
    revenue_xof: completed.reduce((s, o) => s + o.total_xof, 0),
    paid_xof: completed.reduce((s, o) => s + o.total_xof, 0),
    due_xof: list.filter((w) => w.status === 'pending' || w.status === 'in_progress').reduce((s, o) => s + o.total_xof, 0),
    discount_xof: list.reduce((s, o) => s + o.discount_xof, 0),
  }

  res.json({
    items: list.slice(0, 100).map((o) => ({
      ...o,
      paid_xof: o.status === 'completed' ? o.total_xof : 0,
      due_xof: o.status === 'completed' ? 0 : o.total_xof,
      payment_count: o.status === 'completed' ? 1 : 0,
      services_label: o.items?.map((it) => it.service_name).join(', ') || 'Lavage',
      payment_methods: o.status === 'completed' ? ['especes'] : [],
    })),
    total: list.length,
    page_size: Math.min(list.length, 100),
    range: { from, to },
    totals,
  })
})

api.get('/wash-orders/:id', requireAuth, (req, res) => {
  const order = washOrders.find((w) => w.id === req.params.id)
  if (!order) return res.status(404).json({ error: { message: 'Fiche de lavage introuvable.' } })

  const orderPayments = payments
    .filter((p) => p.wash_order_id === order.id)
    .map((p) => ({
      id: p.id,
      method: p.method,
      amount_xof: p.amount_xof,
      reference: p.reference,
      note: p.note,
      paid_at: p.paid_at,
      received_by_name: p.received_by_name,
      status: p.status,
      void_reason: p.void_reason,
    }))

  const fullOrder = {
    ...order,
    paid_xof: order.status === 'completed' ? order.total_xof : 0,
    due_xof: order.status === 'completed' ? 0 : order.total_xof,
    services_label: order.items?.map((it) => it.service_name).join(', ') || 'Lavage',
    payment_methods: orderPayments.map((p) => p.method),
  }

  res.json({ order: fullOrder, payments: orderPayments })
})

api.post('/wash-orders', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const today = new Date().toISOString().slice(0, 10)
  const d = new Date()
  const ref = `L-${today.replace(/-/g, '')}-${String(washOrders.length + 1).padStart(4, '0')}`

  const orderItems: WashOrderItem[] = (req.body.items || []).map((it: any, idx: number) => {
    const srv = services.find((s) => s.id === it.service_id)
    const unitPrice = it.unit_price_xof || srv?.price_xof || 2000
    const qty = it.quantity || 1
    return {
      id: `item-${Date.now()}-${idx}`,
      wash_order_id: '',
      service_id: it.service_id,
      service_name: srv?.name || 'Prestation',
      unit_price_xof: unitPrice,
      quantity: qty,
      line_total_xof: unitPrice * qty,
    }
  })

  const subtotal = orderItems.reduce((s, it) => s + it.line_total_xof, 0)
  const discount = Number(req.body.discount_xof) || 0
  const stationId = req.body.station_id || user.station_id || stations[0]?.id
  const stationObj = stations.find((s) => s.id === stationId)

  if (user.role === 'cashier') {
    const maxDiscount = Number(stationObj?.settings?.max_cashier_discount_xof ?? 1000)
    if (discount > maxDiscount) {
      return res.status(400).json({
        error: {
          code: 'validation_error',
          message: `La remise demandée (${discount.toLocaleString('fr-FR')} FCFA) dépasse le plafond autorisé pour le caissier (${maxDiscount.toLocaleString('fr-FR')} FCFA).`,
          fields: { discount_xof: 'Plafond de remise caissier dépassé' },
        },
      })
    }
  }

  const total = Math.max(0, subtotal - discount)
  const cat = vehicleCategories.find((c) => c.id === req.body.vehicle_category_id || c.code === req.body.vehicle_category_id)
  const emp = users.find((u) => u.id === req.body.employee_id)

  const newOrder: WashOrder = {
    id: `ord-${Date.now()}`,
    organization_id: user.organization_id || 'org-demo',
    station_id: stationId,
    reference: ref,
    customer_id: req.body.customer_id,
    customer_name: req.body.customer_name || 'Client de passage',
    vehicle_category_id: req.body.vehicle_category_id,
    vehicle_category_name: cat?.name || 'Voiture',
    plate: req.body.plate || '',
    employee_id: req.body.employee_id,
    employee_name: emp?.full_name || (user.role === 'washer' ? user.full_name : 'Non attribué'),
    status: req.body.status || 'in_progress',
    subtotal_xof: subtotal,
    discount_xof: discount,
    discount_reason: req.body.discount_reason || '',
    total_xof: total,
    notes: req.body.notes || '',
    created_by: user.id,
    created_by_name: user.full_name,
    created_at: d.toISOString(),
    started_at: d.toISOString(),
    business_day: today,
    items: orderItems,
  }

  orderItems.forEach((it) => (it.wash_order_id = newOrder.id))
  washOrders.unshift(newOrder)
  addAudit('wash_order.created', 'wash_order', newOrder.id, user.full_name, { total_xof: total, discount_xof: discount })

  res.status(201).json(newOrder)
})

api.patch('/wash-orders/:id', requireAuth, (req, res) => {
  const order = washOrders.find((w) => w.id === req.params.id)
  if (!order) return res.status(404).json({ error: { message: 'Fiche introuvable.' } })
  Object.assign(order, req.body)
  res.json(order)
})

api.post('/wash-orders/:id/assign', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const order = washOrders.find((w) => w.id === req.params.id)
  if (!order) return res.status(404).json({ error: { message: 'Fiche introuvable.' } })
  const emp = users.find((u) => u.id === req.body.employee_id)
  order.employee_id = req.body.employee_id || ''
  order.employee_name = emp?.full_name || 'Non attribué'
  addAudit('wash_order.assigned', 'wash_order', order.id, user.full_name)
  res.json(order)
})

api.post('/wash-orders/:id/cancel', requireAuth, (req, res) => {
  const user = (req as any).user as User
  if (user.role === 'cashier' || user.role === 'washer') {
    return res.status(403).json({
      error: { code: 'forbidden', message: "L'annulation d'un lavage nécessite l'autorisation du gérant ou du propriétaire." },
    })
  }
  const order = washOrders.find((w) => w.id === req.params.id)
  if (!order) return res.status(404).json({ error: { message: 'Fiche introuvable.' } })
  order.status = 'canceled'
  order.cancel_reason = req.body.reason || 'Annulation validée par le gérant'
  order.canceled_at = new Date().toISOString()
  addAudit('wash_order.canceled', 'wash_order', order.id, user.full_name)
  res.json(order)
})

api.post('/wash-orders/:id/payments', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const order = washOrders.find((w) => w.id === req.params.id)
  if (!order) return res.status(404).json({ error: { message: 'Fiche introuvable.' } })

  const amount = Number(req.body.amount_xof) || order.total_xof
  const currentSession = cashSessions.find((s) => s.station_id === order.station_id && s.status === 'open')

  const payment: Payment = {
    id: `pay-${Date.now()}`,
    organization_id: user.organization_id || 'org-demo',
    station_id: order.station_id,
    wash_order_id: order.id,
    customer_id: order.customer_id,
    kind: 'lavage',
    method: req.body.method || 'especes',
    amount_xof: amount,
    reference: req.body.reference || '',
    note: req.body.note || '',
    paid_at: new Date().toISOString(),
    business_day: order.business_day,
    cash_session_id: currentSession?.id,
    received_by: user.id,
    received_by_name: user.full_name,
    status: 'valid',
  }
  payments.push(payment)

  if (currentSession) {
    if (payment.method === 'especes') {
      currentSession.cash_in_xof += amount
    } else {
      currentSession.mobile_money_xof += amount
    }
    currentSession.expected_cash_xof = currentSession.opening_float_xof + currentSession.cash_in_xof - currentSession.expenses_xof
    currentSession.payment_count += 1
    currentSession.operations += 1
  }

  order.status = 'completed'
  order.completed_at = new Date().toISOString()

  addAudit('payment.created', 'payment', payment.id, user.full_name)
  res.json({ order })
})

api.get('/wash-orders/:id/receipt', requireAuth, (req, res) => {
  const order = washOrders.find((w) => w.id === req.params.id)
  const st = stations.find((s) => s.id === order?.station_id) || stations[0]
  const org = organizations.find((o) => o.id === st.organization_id) || organizations[0]
  res.json({
    station_name: st.name,
    organization_name: org.name,
    address: st.address,
    phone: st.phone,
    receipt_number: order?.reference || 'REC-001',
    created_at: order?.created_at || new Date().toISOString(),
    customer_name: order?.customer_name,
    plate: order?.plate,
    vehicle_category_name: order?.vehicle_category_name,
    items: order?.items || [],
    subtotal_xof: order?.subtotal_xof || 2000,
    discount_xof: order?.discount_xof || 0,
    total_xof: order?.total_xof || 2000,
    footer: st.settings.receipt_footer || 'Merci de votre confiance !',
  })
})

// Cash Management
api.get('/cash/current', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const stationId = req.query.station_id || user.station_id || stations[0]?.id
  const openSession = cashSessions.find((s) => s.station_id === stationId && s.status === 'open') || null
  const lastSession = cashSessions.filter((s) => s.station_id === stationId && s.status === 'closed')[cashSessions.length - 1] || null

  const movements: any[] = []
  if (openSession) {
    const sessionPayments = payments.filter((p) => p.cash_session_id === openSession.id)
    const sessionExpenses = expenses.filter((e) => e.cash_session_id === openSession.id)

    sessionPayments.forEach((p) => {
      movements.push({
        kind: 'payment',
        id: p.id,
        label: p.wash_order_id ? 'Paiement lavage' : 'Encaissement libre',
        amount_xof: p.amount_xof,
        method: p.method,
        method_label: p.method === 'especes' ? 'Espèces' : 'Mobile Money',
        at: p.paid_at,
        actor: p.received_by_name,
        status: p.status,
        reference: p.reference,
      })
    })

    sessionExpenses.forEach((e) => {
      movements.push({
        kind: 'expense',
        id: e.id,
        label: e.label,
        amount_xof: e.amount_xof,
        method: e.method,
        method_label: e.method === 'especes' ? 'Espèces' : 'Mobile Money',
        at: e.spent_at,
        actor: e.recorded_by_name,
        status: e.status,
        reference: e.reference,
      })
    })
    movements.sort((a, b) => b.at.localeCompare(a.at))
  }

  res.json({
    is_open: Boolean(openSession),
    session: openSession,
    last_session: lastSession,
    movements,
  })
})

api.get('/cash/sessions', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const stationId = req.query.station_id || user.station_id || stations[0]?.id
  const list = cashSessions.filter((s) => s.station_id === stationId).reverse()
  res.json({ items: list, total: list.length })
})

api.get('/cash/sessions/:id', requireAuth, (req, res) => {
  const session = cashSessions.find((s) => s.id === req.params.id)
  if (!session) return res.status(404).json({ error: { message: 'Session introuvable.' } })
  res.json(session)
})

api.post('/cash/sessions', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const stationId = req.body.station_id || user.station_id || stations[0]?.id
  const today = new Date().toISOString().slice(0, 10)
  const openFloat = Number(req.body.opening_float_xof) || 0

  const newSession: CashSession = {
    id: `cs-${Date.now()}`,
    organization_id: user.organization_id || 'org-demo',
    station_id: stationId,
    station_name: stations.find((s) => s.id === stationId)?.name || 'Station',
    status: 'open',
    opened_by: user.id,
    opened_by_name: user.full_name,
    opened_at: new Date().toISOString(),
    opening_float_xof: openFloat,
    expected_cash_xof: openFloat,
    cash_in_xof: 0,
    mobile_money_xof: 0,
    other_in_xof: 0,
    expenses_xof: 0,
    operations: 0,
    payment_count: 0,
    expense_count: 0,
    notes: req.body.notes || '',
    close_notes: '',
    reopen_count: 0,
    business_day: today,
  }
  cashSessions.push(newSession)
  addAudit('cash_session.opened', 'cash_session', newSession.id, user.full_name)
  res.status(201).json(newSession)
})

api.post('/cash/sessions/:id/close', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const session = cashSessions.find((s) => s.id === req.params.id)
  if (!session) return res.status(404).json({ error: { message: 'Session introuvable.' } })

  const counted = Number(req.body.counted_cash_xof) || 0
  session.status = 'closed'
  session.closed_by = user.id
  session.closed_by_name = user.full_name
  session.closed_at = new Date().toISOString()
  session.counted_cash_xof = counted
  session.difference_xof = counted - session.expected_cash_xof
  session.close_notes = req.body.close_notes || ''

  addAudit('cash_session.closed', 'cash_session', session.id, user.full_name, {
    expected_xof: session.expected_cash_xof,
    counted_xof: counted,
    difference_xof: session.difference_xof,
  })
  res.json(session)
})

api.post('/cash/sessions/:id/reopen', requireAuth, (req, res) => {
  const user = (req as any).user as User
  if (user.role === 'cashier' || user.role === 'washer') {
    return res.status(403).json({
      error: { code: 'forbidden', message: "La réouverture d'une caisse déjà clôturée nécessite l'autorisation du gérant ou du propriétaire." },
    })
  }
  const session = cashSessions.find((s) => s.id === req.params.id)
  if (!session) return res.status(404).json({ error: { message: 'Session introuvable.' } })

  session.status = 'open'
  session.reopen_count = (session.reopen_count || 0) + 1
  session.closed_at = null
  session.counted_cash_xof = null
  session.difference_xof = null

  addAudit('cash_session.reopened', 'cash_session', session.id, user.full_name)
  res.json(session)
})

// Payments
api.get('/cash/payments', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const stationId = req.query.station_id || user.station_id || stations[0]?.id
  const method = req.query.method ? String(req.query.method) : ''
  const status = req.query.status ? String(req.query.status) : ''
  const search = String(req.query.search || '').toLowerCase().trim()

  let list = payments.filter((p) => p.station_id === stationId)
  if (method) list = list.filter((p) => p.method === method)
  if (status && status !== 'all') list = list.filter((p) => p.status === status)
  if (search) {
    list = list.filter((p) => p.reference.toLowerCase().includes(search) || p.received_by_name.toLowerCase().includes(search))
  }

  list.sort((a, b) => b.paid_at.localeCompare(a.paid_at))

  const totalAmount = list.filter((p) => p.status === 'valid').reduce((s, p) => s + p.amount_xof, 0)
  const currentSession = cashSessions.find((s) => s.station_id === stationId && s.status === 'open')

  const items = list.map((p) => {
    const w = washOrders.find((ord) => ord.id === p.wash_order_id)
    return {
      ...p,
      station_name: 'Station Étoile — Yopougon',
      wash_order_reference: w?.reference,
      customer_name: w?.customer_name,
      method_label: p.method === 'especes' ? 'Espèces' : 'Mobile Money',
      session_open: Boolean(currentSession && currentSession.id === p.cash_session_id),
    }
  })

  res.json({
    items: items.slice(0, 100),
    total: items.length,
    page_size: Math.min(items.length, 100),
    range: { from: '', to: '' },
    total_xof: totalAmount,
    methods: [
      { id: 'especes', label: 'Espèces' },
      { id: 'mobile_money', label: 'Mobile Money' },
      { id: 'carte', label: 'Carte bancaire' },
      { id: 'virement', label: 'Virement' },
    ],
    definitions: {},
  })
})

api.post('/cash/payments', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const stationId = req.body.station_id || user.station_id || stations[0]?.id
  const currentSession = cashSessions.find((s) => s.station_id === stationId && s.status === 'open')
  const amount = Number(req.body.amount_xof) || 0

  const payment: Payment = {
    id: `pay-${Date.now()}`,
    organization_id: user.organization_id || 'org-demo',
    station_id: stationId,
    customer_id: req.body.customer_id,
    kind: req.body.kind || 'autre',
    method: req.body.method || 'especes',
    amount_xof: amount,
    reference: req.body.reference || '',
    note: req.body.note || '',
    paid_at: new Date().toISOString(),
    business_day: new Date().toISOString().slice(0, 10),
    cash_session_id: currentSession?.id,
    received_by: user.id,
    received_by_name: user.full_name,
    status: 'valid',
  }
  payments.unshift(payment)

  if (currentSession) {
    if (payment.method === 'especes') {
      currentSession.cash_in_xof += amount
    } else {
      currentSession.mobile_money_xof += amount
    }
    currentSession.expected_cash_xof = currentSession.opening_float_xof + currentSession.cash_in_xof - currentSession.expenses_xof
    currentSession.payment_count += 1
    currentSession.operations += 1
  }

  addAudit('payment.free_created', 'payment', payment.id, user.full_name)
  res.status(201).json(payment)
})

api.post('/cash/payments/:id/void', requireAuth, (req, res) => {
  const user = (req as any).user as User
  if (user.role === 'cashier' || user.role === 'washer') {
    return res.status(403).json({
      error: { code: 'forbidden', message: "L'annulation d'un encaissement nécessite l'autorisation du gérant ou du propriétaire." },
    })
  }
  const p = payments.find((pay) => pay.id === req.params.id)
  if (!p) return res.status(404).json({ error: { message: 'Paiement introuvable.' } })

  p.status = 'voided'
  p.void_reason = req.body.reason || 'Annulation autorisée par le gérant'

  const session = cashSessions.find((s) => s.id === p.cash_session_id)
  if (session) {
    if (p.method === 'especes') session.cash_in_xof -= p.amount_xof
    else session.mobile_money_xof -= p.amount_xof
    session.expected_cash_xof = session.opening_float_xof + session.cash_in_xof - session.expenses_xof
  }

  addAudit('payment.voided', 'payment', p.id, user.full_name)
  res.json(p)
})

// Expenses
api.get('/expenses', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const stationId = req.query.station_id || user.station_id || stations[0]?.id
  const category = req.query.category ? String(req.query.category) : ''
  const status = req.query.status ? String(req.query.status) : ''
  const search = String(req.query.search || '').toLowerCase().trim()

  let list = expenses.filter((e) => e.station_id === stationId)
  if (category) list = list.filter((e) => e.category === category)
  if (status && status !== 'all') list = list.filter((e) => e.status === status)
  if (search) {
    list = list.filter((e) => e.label.toLowerCase().includes(search) || e.supplier.toLowerCase().includes(search))
  }

  list.sort((a, b) => b.spent_at.localeCompare(a.spent_at))
  const totalAmount = list.filter((e) => e.status === 'valid').reduce((s, e) => s + e.amount_xof, 0)
  const currentSession = cashSessions.find((s) => s.station_id === stationId && s.status === 'open')

  res.json({
    items: list.map((e) => ({
      ...e,
      station_name: 'Station Étoile — Yopougon',
      method_label: e.method === 'especes' ? 'Espèces' : 'Mobile Money',
      session_open: Boolean(currentSession && currentSession.id === e.cash_session_id),
    })),
    total: list.length,
    page_size: list.length,
    total_xof: totalAmount,
    range: { from: '', to: '' },
    categories: [
      { id: 'produits', label: 'Produits de lavage' },
      { id: 'eau', label: 'Eau' },
      { id: 'electricite', label: 'Électricité' },
      { id: 'entretien', label: 'Entretien matériel' },
      { id: 'autre', label: 'Autre' },
    ],
  })
})

api.post('/expenses', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const stationId = req.body.station_id || user.station_id || stations[0]?.id
  const currentSession = cashSessions.find((s) => s.station_id === stationId && s.status === 'open')
  const amount = Number(req.body.amount_xof) || 0

  const exp: Expense = {
    id: `exp-${Date.now()}`,
    organization_id: user.organization_id || 'org-demo',
    station_id: stationId,
    category: req.body.category || 'autre',
    label: req.body.label,
    amount_xof: amount,
    method: req.body.method || 'especes',
    supplier: req.body.supplier || '',
    reference: req.body.reference || '',
    note: req.body.note || '',
    spent_at: req.body.spent_at || new Date().toISOString(),
    business_day: new Date().toISOString().slice(0, 10),
    cash_session_id: currentSession?.id,
    recorded_by: user.id,
    recorded_by_name: user.full_name,
    status: 'valid',
    created_at: new Date().toISOString(),
  }
  expenses.unshift(exp)

  if (currentSession && exp.method === 'especes') {
    currentSession.expenses_xof += amount
    currentSession.expected_cash_xof = currentSession.opening_float_xof + currentSession.cash_in_xof - currentSession.expenses_xof
    currentSession.expense_count += 1
    currentSession.operations += 1
  }

  addAudit('expense.created', 'expense', exp.id, user.full_name)
  res.status(201).json(exp)
})

api.patch('/expenses/:id', requireAuth, (req, res) => {
  const exp = expenses.find((e) => e.id === req.params.id)
  if (!exp) return res.status(404).json({ error: { message: 'Dépense introuvable.' } })
  Object.assign(exp, req.body)
  res.json(exp)
})

api.post('/expenses/:id/void', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const exp = expenses.find((e) => e.id === req.params.id)
  if (!exp) return res.status(404).json({ error: { message: 'Dépense introuvable.' } })

  exp.status = 'voided'
  exp.void_reason = req.body.reason || 'Annulation'

  const session = cashSessions.find((s) => s.id === exp.cash_session_id)
  if (session && exp.method === 'especes') {
    session.expenses_xof -= exp.amount_xof
    session.expected_cash_xof = session.opening_float_xof + session.cash_in_xof - session.expenses_xof
  }

  addAudit('expense.voided', 'expense', exp.id, user.full_name)
  res.json(exp)
})

// Reports & Dashboard
api.get('/reports/dashboard', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const stationId = (req.query.station_id as string) || user.station_id || stations[0]?.id
  const dateStr = (req.query.date as string) || new Date().toISOString().slice(0, 10)

  const dayOrders = washOrders.filter((w) => w.station_id === stationId && w.business_day === dateStr)
  const completedOrders = dayOrders.filter((w) => w.status === 'completed')
  const revenue = completedOrders.reduce((s, o) => s + o.total_xof, 0)
  const dayPayments = payments.filter((p) => p.station_id === stationId && p.business_day === dateStr && p.status === 'valid')
  const paymentsTotal = dayPayments.reduce((s, p) => s + p.amount_xof, 0)
  const dayExpenses = expenses.filter((e) => e.station_id === stationId && e.business_day === dateStr && e.status === 'valid')
  const expensesTotal = dayExpenses.reduce((s, e) => s + e.amount_xof, 0)

  // 7-day series
  const series: any[] = []
  const baseD = new Date(dateStr)
  for (let i = 6; i >= 0; i--) {
    const cur = new Date(baseD.getTime() - i * 86400000).toISOString().slice(0, 10)
    const oList = washOrders.filter((w) => w.station_id === stationId && w.business_day === cur && w.status === 'completed')
    const rev = oList.reduce((s, o) => s + o.total_xof, 0)
    const pList = payments.filter((p) => p.station_id === stationId && p.business_day === cur && p.status === 'valid')
    const pTot = pList.reduce((s, p) => s + p.amount_xof, 0)
    const eList = expenses.filter((e) => e.station_id === stationId && e.business_day === cur && e.status === 'valid')
    const eTot = eList.reduce((s, e) => s + e.amount_xof, 0)
    series.push({
      day: cur,
      wash_count: oList.length,
      revenue_xof: rev,
      payments_xof: pTot,
      expenses_xof: eTot,
      result_xof: rev - eTot,
    })
  }

  const currentSession = cashSessions.find((s) => s.station_id === stationId && s.business_day === dateStr)

  // Employees activity
  const stationEmployees = users.filter((u) => u.organization_id === user.organization_id && (u.station_id === stationId || !u.station_id))
  const employeeData = stationEmployees.map((emp) => {
    const oEmp = dayOrders.filter((w) => w.employee_id === emp.id && w.status === 'completed')
    const rev = oEmp.reduce((s, o) => s + o.total_xof, 0)
    return {
      employee_id: emp.id,
      employee_name: emp.full_name,
      wash_count: oEmp.length,
      revenue_xof: rev,
      collected_xof: rev,
    }
  })

  // Recent activity
  const recent: any[] = []
  dayOrders.slice(0, 5).forEach((w) => {
    recent.push({
      type: 'wash',
      label: `Lavage ${w.reference} — ${w.vehicle_category_name}`,
      amount_xof: w.total_xof,
      at: w.created_at,
      actor: w.employee_name,
      status: w.status,
      reference: w.reference,
    })
  })
  dayPayments.slice(0, 5).forEach((p) => {
    recent.push({
      type: 'payment',
      label: `Encaissement ${p.method === 'especes' ? 'espèces' : 'Mobile Money'}`,
      amount_xof: p.amount_xof,
      at: p.paid_at,
      actor: p.received_by_name,
      status: p.status,
      reference: p.reference,
    })
  })
  recent.sort((a, b) => b.at.localeCompare(a.at))

  // Top services
  const srvCounts: Record<string, { quantity: number; revenue: number }> = {}
  completedOrders.forEach((o) => {
    o.items?.forEach((it) => {
      if (!srvCounts[it.service_name]) srvCounts[it.service_name] = { quantity: 0, revenue: 0 }
      srvCounts[it.service_name].quantity += it.quantity
      srvCounts[it.service_name].revenue += it.line_total_xof
    })
  })
  const topServices = Object.entries(srvCounts).map(([name, data]) => ({
    service_name: name,
    quantity: data.quantity,
    revenue_xof: data.revenue,
  })).slice(0, 5)

  // Goal
  const stationObj = stations.find((s) => s.id === stationId)
  const targetGoal = stationObj?.settings.monthly_goal_xof || 1500000
  const monthOrders = washOrders.filter((w) => w.station_id === stationId && w.business_day.slice(0, 7) === dateStr.slice(0, 7) && w.status === 'completed')
  const monthAchieved = monthOrders.reduce((s, o) => s + o.total_xof, 0)
  const goalObj = {
    month: dateStr.slice(0, 7),
    target_xof: targetGoal,
    achieved_xof: monthAchieved,
    remaining_xof: Math.max(0, targetGoal - monthAchieved),
    percent: Math.min(100, Math.round((monthAchieved / targetGoal) * 100)),
    days_left: 10,
    reached: monthAchieved >= targetGoal,
    needed_per_day_xof: Math.max(0, Math.round((targetGoal - monthAchieved) / 10)),
  }

  res.json({
    date: dateStr,
    station_id: stationId,
    today: {
      wash_count: completedOrders.length,
      canceled_count: dayOrders.filter((w) => w.status === 'canceled').length,
      service_count: completedOrders.reduce((s, o) => s + (o.items?.length || 1), 0),
      revenue_xof: revenue,
      discount_xof: dayOrders.reduce((s, o) => s + o.discount_xof, 0),
      payments_xof: paymentsTotal,
      cash_xof: dayPayments.filter((p) => p.method === 'especes').reduce((s, p) => s + p.amount_xof, 0),
      mobile_money_xof: dayPayments.filter((p) => p.method === 'mobile_money').reduce((s, p) => s + p.amount_xof, 0),
      other_xof: 0,
      expenses_xof: expensesTotal,
      expenses_cash_xof: dayExpenses.filter((e) => e.method === 'especes').reduce((s, e) => s + e.amount_xof, 0),
      result_xof: revenue - expensesTotal,
      cash_result_xof: (dayPayments.filter((p) => p.method === 'especes').reduce((s, p) => s + p.amount_xof, 0)) - (dayExpenses.filter((e) => e.method === 'especes').reduce((s, e) => s + e.amount_xof, 0)),
      due_xof: dayOrders.filter((w) => w.status === 'pending' || w.status === 'in_progress').reduce((s, o) => s + o.total_xof, 0),
      average_ticket_xof: completedOrders.length > 0 ? Math.round(revenue / completedOrders.length) : 0,
      voided_payments_xof: 0,
    },
    series,
    cash: currentSession ? {
      is_open: currentSession.status === 'open',
      session_id: currentSession.id,
      opened_at: currentSession.opened_at,
      opened_by_name: currentSession.opened_by_name,
      opening_float_xof: currentSession.opening_float_xof,
      cash_in_xof: currentSession.cash_in_xof,
      mobile_money_xof: currentSession.mobile_money_xof,
      expenses_xof: currentSession.expenses_xof,
      expected_cash_xof: currentSession.expected_cash_xof,
    } : null,
    employees: employeeData,
    recent,
    top_services: topServices,
    payment_methods: [
      { method: 'especes', amount_xof: dayPayments.filter((p) => p.method === 'especes').reduce((s, p) => s + p.amount_xof, 0), count: dayPayments.filter((p) => p.method === 'especes').length },
      { method: 'mobile_money', amount_xof: dayPayments.filter((p) => p.method === 'mobile_money').reduce((s, p) => s + p.amount_xof, 0), count: dayPayments.filter((p) => p.method === 'mobile_money').length },
    ],
    alerts: [],
    goal: goalObj,
    definitions: {
      chiffre_affaires: 'Total des lavages enregistrés aujourd’hui (hors fiches annulées).',
      encaissements: 'Argent réellement reçu aujourd’hui.',
      dépenses: 'Dépenses enregistrées aujourd’hui.',
      résultat: 'Chiffre d’affaires moins les dépenses de la journée.',
    },
  })
})

api.get('/reports/overview', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const stationId = (req.query.station_id as string) || user.station_id || stations[0]?.id
  const from = (req.query.from as string) || new Date(Date.now() - 30 * 86400000).toISOString().slice(0, 10)
  const to = (req.query.to as string) || new Date().toISOString().slice(0, 10)

  const periodOrders = washOrders.filter((w) => w.station_id === stationId && w.business_day >= from && w.business_day <= to)
  const completedOrders = periodOrders.filter((w) => w.status === 'completed')
  const revenue = completedOrders.reduce((s, o) => s + o.total_xof, 0)
  const periodPayments = payments.filter((p) => p.station_id === stationId && p.business_day >= from && p.business_day <= to && p.status === 'valid')
  const periodExpenses = expenses.filter((e) => e.station_id === stationId && e.business_day >= from && e.business_day <= to && e.status === 'valid')
  const expensesTotal = periodExpenses.reduce((s, e) => s + e.amount_xof, 0)

  res.json({
    from,
    to,
    days: 30,
    station_id: stationId,
    station_name: 'Station Étoile — Yopougon',
    totals: {
      wash_count: completedOrders.length,
      canceled_count: periodOrders.filter((w) => w.status === 'canceled').length,
      service_count: completedOrders.reduce((s, o) => s + (o.items?.length || 1), 0),
      revenue_xof: revenue,
      discount_xof: periodOrders.reduce((s, o) => s + o.discount_xof, 0),
      payments_xof: periodPayments.reduce((s, p) => s + p.amount_xof, 0),
      cash_xof: periodPayments.filter((p) => p.method === 'especes').reduce((s, p) => s + p.amount_xof, 0),
      mobile_money_xof: periodPayments.filter((p) => p.method === 'mobile_money').reduce((s, p) => s + p.amount_xof, 0),
      other_xof: 0,
      expenses_xof: expensesTotal,
      expenses_cash_xof: periodExpenses.filter((e) => e.method === 'especes').reduce((s, e) => s + e.amount_xof, 0),
      result_xof: revenue - expensesTotal,
      cash_result_xof: (periodPayments.filter((p) => p.method === 'especes').reduce((s, p) => s + p.amount_xof, 0)) - (periodExpenses.filter((e) => e.method === 'especes').reduce((s, e) => s + e.amount_xof, 0)),
      due_xof: periodOrders.filter((w) => w.status === 'pending' || w.status === 'in_progress').reduce((s, o) => s + o.total_xof, 0),
      average_ticket_xof: completedOrders.length > 0 ? Math.round(revenue / completedOrders.length) : 0,
      voided_payments_xof: 0,
    },
    series: [],
    by_service: [],
    by_employee: [],
    by_method: [
      { method: 'especes', amount_xof: periodPayments.filter((p) => p.method === 'especes').reduce((s, p) => s + p.amount_xof, 0), count: periodPayments.filter((p) => p.method === 'especes').length },
      { method: 'mobile_money', amount_xof: periodPayments.filter((p) => p.method === 'mobile_money').reduce((s, p) => s + p.amount_xof, 0), count: periodPayments.filter((p) => p.method === 'mobile_money').length },
    ],
    by_expense_category: [
      { category: 'produits', label: 'Produits de lavage', amount_xof: periodExpenses.filter((e) => e.category === 'produits').reduce((s, e) => s + e.amount_xof, 0), count: 5 },
      { category: 'eau', label: 'Eau', amount_xof: periodExpenses.filter((e) => e.category === 'eau').reduce((s, e) => s + e.amount_xof, 0), count: 3 },
    ],
    by_vehicle_category: [
      { vehicle_category_name: 'Voiture', wash_count: completedOrders.filter((o) => o.vehicle_category_name === 'Voiture').length, revenue_xof: 60000 },
      { vehicle_category_name: '4x4 / Pick-up', wash_count: completedOrders.filter((o) => o.vehicle_category_name.includes('4x4')).length, revenue_xof: 45000 },
    ],
    cash_sessions: cashSessions.slice(0, 10),
    top_customers: customers.slice(0, 5).map((c) => ({
      customer_id: c.id,
      customer_name: c.full_name,
      wash_count: 5,
      revenue_xof: 15000,
    })),
    definitions: {},
  })
})

api.get('/reports/export', requireAuth, (_req, res) => {
  const csvContent = '\uFEFFDate;Référence;Véhicule;Total XOF;Statut\n2026-10-12;L-001;Voiture;4000;Terminé\n'
  res.setHeader('Content-Type', 'text/csv; charset=utf-8')
  res.setHeader('Content-Disposition', 'attachment; filename="washguard-export.csv"')
  res.send(csvContent)
})

// Commissions
api.get('/commissions', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const stationId = (req.query.station_id as string) || user.station_id || stations[0]?.id
  const stationObj = stations.find((s) => s.id === stationId)

  const stMode = stationObj?.settings.commission_mode || 'per_wash'
  const stPerWash = stationObj?.settings.commission_per_wash_xof || 500
  const stPercent = stationObj?.settings.commission_percent || 10
  const monthlyGoal = stationObj?.settings.monthly_goal_xof || 1500000

  const stationEmployees = users.filter((u) => u.organization_id === user.organization_id && (u.station_id === stationId || !u.station_id))

  const items = stationEmployees.map((emp) => {
    const empOrders = washOrders.filter((w) => w.employee_id === emp.id && w.status === 'completed')
    const rev = empOrders.reduce((s, o) => s + o.total_xof, 0)
    const mode = emp.commission_mode || stMode
    const perWash = emp.commission_per_wash_xof ?? stPerWash
    const percent = emp.commission_percent ?? stPercent
    let comm = 0
    if (mode === 'per_wash') comm = empOrders.length * perWash
    else if (mode === 'percent') comm = Math.round((rev * percent) / 100)

    return {
      employee_id: emp.id,
      employee_name: emp.full_name,
      role: emp.role,
      role_label: emp.role === 'owner' ? 'Propriétaire' : emp.role === 'manager' ? 'Gérant' : emp.role === 'cashier' ? 'Caissier' : 'Laveur',
      wash_count: empOrders.length,
      revenue_xof: rev,
      mode,
      per_wash_xof: perWash,
      percent,
      rate_from_employee: Boolean(emp.commission_mode),
      commission_xof: comm,
    }
  })

  const totalComm = items.reduce((s, it) => s + it.commission_xof, 0)
  const totalRev = items.reduce((s, it) => s + it.revenue_xof, 0)
  const totalWash = items.reduce((s, it) => s + it.wash_count, 0)

  res.json({
    period: { from: '01/10/2026', to: '14/10/2026' },
    settings: {
      mode: stMode,
      per_wash_xof: stPerWash,
      percent: stPercent,
      monthly_goal_xof: monthlyGoal,
    },
    goal: {
      month: '2026-10',
      target_xof: monthlyGoal,
      achieved_xof: totalRev,
      remaining_xof: Math.max(0, monthlyGoal - totalRev),
      percent: Math.min(100, Math.round((totalRev / monthlyGoal) * 100)),
      days_left: 17,
      reached: totalRev >= monthlyGoal,
    },
    items,
    totals: {
      employee_count: items.length,
      wash_count: totalWash,
      revenue_xof: totalRev,
      commission_xof: totalComm,
    },
  })
})

api.get('/commissions/export', requireAuth, (_req, res) => {
  const csvContent = "\uFEFFEmployé;Rôle;Nombre de lavages;Chiffre d'affaires;Commission XOF\nAwa Traoré;Gérant;24;78000;12000\nMoussa Diomandé;Caissier;32;96000;16000\nBakary Koné;Laveur;40;120000;20000\n"
  res.setHeader('Content-Type', 'text/csv; charset=utf-8')
  res.setHeader('Content-Disposition', 'attachment; filename="washguard-commissions.csv"')
  res.send(csvContent)
})

// Onboarding
api.get('/onboarding', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const payload = calculateOnboarding(user.organization_id || 'org-demo', user.station_id)
  res.json(payload)
})

api.get('/onboarding/summary', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const org = organizations.find((o) => o.id === user.organization_id)
  const st = stations.find((s) => s.id === user.station_id) || stations[0]
  const ob = calculateOnboarding(user.organization_id || 'org-demo', user.station_id)

  res.json({
    ...ob,
    organization: org,
    station: st,
    setup: {
      services_count: services.filter((s) => s.station_id === st?.id).length,
      employees_count: users.filter((u) => u.organization_id === user.organization_id).length,
      categories_count: (st?.vehicle_categories || vehicleCategories).filter((c) => c.is_active).length,
      usages_count: (st?.vehicle_usages || vehicleUsages).filter((u) => u.is_active).length,
      opening_days_count: (st?.opening_hours_days || []).filter((d) => d.is_open).length,
      equipments_count: equipments.filter((e) => e.station_id === st?.id).length,
    },
  })
})

api.patch('/onboarding/steps/:code', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const code = req.params.code
  const data = req.body.data || {}
  const st = stations.find((s) => s.id === user.station_id) || stations[0]

  if (code === 'station' && st && data) {
    if (data.site) st.site = { ...(st.site || getDefaultStationSite(st.name)), ...data.site }
    if (data.name) st.name = data.name
    if (data.city) st.city = data.city
  }

  const updated = calculateOnboarding(user.organization_id || 'org-demo', user.station_id)
  res.json(updated)
})

api.post('/onboarding/activate', requireAuth, (req, res) => {
  const user = (req as any).user as User
  const org = organizations.find((o) => o.id === user.organization_id)
  const st = stations.find((s) => s.id === user.station_id) || stations[0]

  const currentOb = calculateOnboarding(user.organization_id || 'org-demo', user.station_id)
  if (!currentOb.can_activate && currentOb.missing.length > 0) {
    return res.status(400).json({
      error: {
        code: 'preconditions_failed',
        message: `Certains prérequis obligatoires manquent encore : ${currentOb.missing.map((m) => m.title).join(', ')}.`,
      },
    })
  }

  if (org) org.status = 'active'
  if (st) st.is_active = true

  addAudit('onboarding.activated', 'station', st?.id || '', user.full_name)

  const activeOb = {
    ...currentOb,
    state: 'active' as const,
    activated: true,
    already_active: false,
    message: 'Votre station de lavage est désormais active et opérationnelle !',
    activated_at: new Date().toISOString(),
    activated_by: {
      activated_at: new Date().toISOString(),
      activated_by: user.id,
      activated_by_name: user.full_name,
      snapshot: {
        organization_name: org?.name,
        station_name: st?.name,
        services_count: services.filter((s) => s.station_id === st?.id).length,
      },
    },
  }

  res.json(activeOb)
})

// Platform Admin Routes
api.get('/platform/overview', requireAuth, (_req, res) => {
  res.json({
    organizations: organizations.length,
    active_organizations: organizations.filter((o) => o.status === 'active').length,
    suspended_organizations: organizations.filter((o) => o.status === 'suspended').length,
    stations: stations.length,
    users: users.length,
    wash_orders_month: washOrders.length,
    revenue_month_xof: washOrders.filter((w) => w.status === 'completed').reduce((s, w) => s + w.total_xof, 0),
    mrr_xof: 35000,
    plans: plans.map((p) => ({ plan_id: p.id, plan_name: p.name, price_xof: p.price_xof, count: subscriptions.filter((s) => s.plan_id === p.id).length })),
    recent_organizations: organizations.slice(0, 5).map((o) => ({
      id: o.id,
      name: o.name,
      city: o.city,
      status: o.status,
      created_at: o.created_at,
      plan_name: 'Premium',
      subscription_status: 'active',
      subscription_status_label: 'Actif',
    })),
  })
})

api.get('/platform/organizations', requireAuth, (_req, res) => {
  const items = organizations.map((o) => {
    const sub = subscriptions.find((s) => s.organization_id === o.id)
    const plan = plans.find((p) => p.id === sub?.plan_id)
    const owner = users.find((u) => u.organization_id === o.id && u.role === 'owner')
    return {
      id: o.id,
      name: o.name,
      slug: o.slug,
      city: o.city,
      phone: o.phone,
      email: o.email,
      status: o.status,
      created_at: o.created_at,
      plan_id: plan?.id || 'plan-premium',
      plan_name: plan?.name || 'Premium',
      subscription_status: sub?.status || 'active',
      subscription_status_label: 'Actif',
      trial_ends_at: sub?.trial_ends_at,
      station_count: stations.filter((s) => s.organization_id === o.id).length,
      employee_count: users.filter((u) => u.organization_id === o.id).length,
      wash_orders_month: washOrders.filter((w) => w.organization_id === o.id).length,
      owner_name: owner?.full_name || 'Propriétaire',
      owner_email: owner?.email || o.email,
    }
  })
  res.json({ items, total: items.length })
})

api.get('/platform/organizations/:id', requireAuth, (req, res) => {
  const org = organizations.find((o) => o.id === req.params.id)
  if (!org) return res.status(404).json({ error: { message: 'Organisation introuvable.' } })
  const sub = subscriptions.find((s) => s.organization_id === org.id)
  const orgStations = stations.filter((s) => s.organization_id === org.id)
  const orgUsers = users.filter((u) => u.organization_id === org.id)

  res.json({
    organization: org,
    subscription: sub,
    stations: orgStations,
    users: orgUsers.map((u) => ({
      id: u.id,
      full_name: u.full_name,
      email: u.email,
      role: u.role,
      role_label: u.role === 'owner' ? 'Propriétaire' : u.role === 'manager' ? 'Gérant' : u.role === 'cashier' ? 'Caissier' : 'Laveur',
      is_active: u.is_active,
      last_login_at: u.last_login_at || null,
    })),
    audit: auditLogs.filter((a) => a.organization_id === org.id),
  })
})

api.patch('/platform/organizations/:id', requireAuth, (req, res) => {
  const org = organizations.find((o) => o.id === req.params.id)
  if (!org) return res.status(404).json({ error: { message: 'Organisation introuvable.' } })
  Object.assign(org, req.body)
  res.json({ message: 'Organisation mise à jour.' })
})

api.post('/platform/organizations/:id/subscription', requireAuth, (req, res) => {
  const plan = plans.find((p) => p.code === req.body.plan_code)
  if (!plan) return res.status(400).json({ error: { message: 'Formule inconnue.' } })
  const orgId = String(req.params.id)
  const existingSub = subscriptions.find((s) => s.organization_id === orgId)
  if (!existingSub) {
    const newSub: Subscription = {
      id: `sub-${Date.now()}`,
      organization_id: orgId,
      plan_id: plan.id,
      status: req.body.status || 'active',
      started_at: new Date().toISOString(),
    }
    subscriptions.push(newSub)
  } else {
    existingSub.plan_id = plan.id
    existingSub.status = req.body.status || 'active'
  }
  res.json({ message: `Abonnement mis à jour : formule ${plan.name} appliquée.` })
})

api.get('/platform/plans', requireAuth, (_req, res) => {
  res.json({ items: plans, total: plans.length })
})

api.patch('/platform/plans/:id', requireAuth, (req, res) => {
  const plan = plans.find((p) => p.id === req.params.id)
  if (!plan) return res.status(404).json({ error: { message: 'Formule introuvable.' } })
  Object.assign(plan, req.body)
  res.json({ items: plans, message: 'Formule mise à jour.' })
})

api.get('/platform/audit', requireAuth, (req, res) => {
  const limit = Number(req.query.limit) || 100
  res.json({ items: auditLogs.slice(0, limit), total: auditLogs.length })
})

// Mount API v1 router
app.use('/api/v1', api)

// ---------------------------------------------------------------------------
// VITE DEV SERVER / STATIC PRODUCTION FALLBACK
// ---------------------------------------------------------------------------

async function startServer() {
  const isProd = process.env.NODE_ENV === 'production'

  if (!isProd) {
    const { createServer: createViteServer } = await import('vite')
    const vite = await createViteServer({
      server: { middlewareMode: true },
      appType: 'spa',
    })
    app.use(vite.middlewares)
  } else {
    const distPath = path.resolve(__dirname, 'dist')
    if (fs.existsSync(distPath)) {
      app.use(express.static(distPath))
      app.get('*', (_req, res) => {
        res.sendFile(path.resolve(distPath, 'index.html'))
      })
    }
  }

  app.listen(PORT, HOST, () => {
    console.log(`WashGuard server running on http://${HOST}:${PORT}`)
  })
}

startServer().catch((err) => {
  console.error('Failed to start WashGuard server:', err)
  process.exit(1)
})
