// Types partagés avec l'API Go (les noms de champs suivent le backend).

export type Role = 'superadmin' | 'owner' | 'manager' | 'cashier' | 'washer'
export type WashStatus = 'pending' | 'in_progress' | 'completed' | 'canceled'
export type PaymentMethod = 'especes' | 'mobile_money' | 'carte' | 'virement' | 'autre'
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
}

export interface Service {
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

export interface VehicleCategory {
  id: string
  name: string
  description: string
  sort_order: number
  is_active: boolean
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
}

export interface WashOrderItem {
  id?: string
  service_id?: string
  service_name: string
  unit_price_xof: number
  quantity: number
  line_total_xof: number
}

export interface WashOrderPayment {
  id: string
  method: PaymentMethod
  amount_xof: number
  reference: string
  paid_at: string
  received_by_name: string
  status: RecordStatus
  note: string
  method_label?: string
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
  today: Totals
  series: DaySeries[]
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
  employees: Array<{
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
  top_services: Array<{ service_name: string; quantity: number; revenue_xof: number }>
  payment_methods: Array<{ method: PaymentMethod; amount_xof: number; count: number }>
  alerts: Array<{ level: 'info' | 'warning' | 'danger'; message: string }>
  /** Objectif du mois : `null` tant que la station n'en a pas fixé. */
  goal?: MonthlyGoal | null
  definitions?: Record<string, string>
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
  paid_at: string
  business_day?: string
  status: RecordStatus
  void_reason?: string
  cash_session_id?: string
  session_open: boolean
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
