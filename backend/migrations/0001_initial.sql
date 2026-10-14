-- Schéma initial WashGuard — appliqué au démarrage (idempotent).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS organizations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  phone text NOT NULL DEFAULT '',
  email text NOT NULL DEFAULT '',
  city text NOT NULL DEFAULT 'Abidjan',
  country text NOT NULL DEFAULT 'Côte d''Ivoire',
  legal_name text NOT NULL DEFAULT '',
  billing_address text NOT NULL DEFAULT '',
  settings jsonb NOT NULL DEFAULT '{}'::jsonb,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS stations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name text NOT NULL,
  code text NOT NULL UNIQUE,
  phone text NOT NULL DEFAULT '',
  address text NOT NULL DEFAULT '',
  city text NOT NULL DEFAULT '',
  country text NOT NULL DEFAULT '',
  district text NOT NULL DEFAULT '',
  commune text NOT NULL DEFAULT '',
  district_area text NOT NULL DEFAULT '',
  street text NOT NULL DEFAULT '',
  landmark text NOT NULL DEFAULT '',
  latitude numeric(9,6),
  longitude numeric(9,6),
  billing_address text NOT NULL DEFAULT '',
  timezone text NOT NULL DEFAULT 'Africa/Abidjan',
  currency text NOT NULL DEFAULT 'XOF',
  logo_url text NOT NULL DEFAULT '',
  site jsonb NOT NULL DEFAULT '{}'::jsonb,
  settings jsonb NOT NULL DEFAULT '{}'::jsonb,
  is_default boolean NOT NULL DEFAULT false,
  is_active boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS stations_org_idx ON stations(organization_id);

CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid REFERENCES stations(id) ON DELETE SET NULL,
  clerk_id text UNIQUE,
  full_name text NOT NULL,
  email text UNIQUE,
  phone text NOT NULL DEFAULT '',
  role text NOT NULL CHECK (role IN ('superadmin','owner','manager','cashier','washer')),
  password_hash text NOT NULL DEFAULT '',
  must_change_password boolean NOT NULL DEFAULT false,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS users_org_idx ON users(organization_id);

CREATE TABLE IF NOT EXISTS sessions (
  token_hash text PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS auth_failures (
  key text PRIMARY KEY,
  count integer NOT NULL DEFAULT 0,
  window_started_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS plans (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code text NOT NULL UNIQUE,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  price_xof integer NOT NULL DEFAULT 0,
  billing_period text NOT NULL DEFAULT 'monthly',
  max_stations integer NOT NULL DEFAULT 1,
  max_employees integer NOT NULL DEFAULT 3,
  max_wash_orders_month integer NOT NULL DEFAULT 0,
  features jsonb NOT NULL DEFAULT '{}'::jsonb,
  is_active boolean NOT NULL DEFAULT true
);

CREATE TABLE IF NOT EXISTS subscriptions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  plan_id uuid REFERENCES plans(id),
  status text NOT NULL DEFAULT 'trialing',
  trial_ends_at timestamptz,
  current_period_start timestamptz,
  current_period_end timestamptz,
  requested_plan_code text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS subs_org_idx ON subscriptions(organization_id);

CREATE TABLE IF NOT EXISTS registry_fields (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  field_code text NOT NULL,
  value text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'empty',
  verified_at timestamptz,
  UNIQUE (organization_id, field_code)
);

CREATE TABLE IF NOT EXISTS onboarding_states (
  organization_id uuid PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
  state text NOT NULL DEFAULT 'pending',
  step_data jsonb NOT NULL DEFAULT '{}'::jsonb,
  started_at timestamptz,
  activated_at timestamptz,
  activated_by jsonb,
  snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- Choix descriptifs enregistrés par les formulaires d'onboarding. L'état
-- « fait » n'y est JAMAIS stocké : il est recalculé sur les données réelles.
CREATE TABLE IF NOT EXISTS onboarding_steps (
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  step_code text NOT NULL,
  state text NOT NULL DEFAULT 'in_progress' CHECK (state IN ('not_started','in_progress')),
  data jsonb NOT NULL DEFAULT '{}'::jsonb,
  updated_by uuid,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, step_code)
);

CREATE TABLE IF NOT EXISTS opening_hours (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  weekday smallint NOT NULL CHECK (weekday BETWEEN 1 AND 7),
  is_open boolean NOT NULL DEFAULT true,
  opens_at text NOT NULL DEFAULT '',
  closes_at text NOT NULL DEFAULT '',
  break_start text NOT NULL DEFAULT '',
  break_end text NOT NULL DEFAULT '',
  last_checkin_at text NOT NULL DEFAULT '',
  booking_allowed boolean NOT NULL DEFAULT true,
  UNIQUE (station_id, weekday)
);

CREATE TABLE IF NOT EXISTS opening_exceptions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  day date NOT NULL,
  is_open boolean NOT NULL DEFAULT false,
  opens_at text NOT NULL DEFAULT '',
  closes_at text NOT NULL DEFAULT '',
  label text NOT NULL DEFAULT '',
  UNIQUE (station_id, day)
);

CREATE TABLE IF NOT EXISTS vehicle_categories (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid REFERENCES organizations(id) ON DELETE CASCADE, -- NULL = référentiel produit
  code text NOT NULL,
  name text NOT NULL,
  family text NOT NULL DEFAULT '',
  description text NOT NULL DEFAULT '',
  aliases text[] NOT NULL DEFAULT ARRAY[]::text[],
  sort_order integer NOT NULL DEFAULT 0,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS vcat_unique ON vehicle_categories ((COALESCE(organization_id, '00000000-0000-0000-0000-000000000000'::uuid)), code);

CREATE TABLE IF NOT EXISTS vehicle_usages (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid REFERENCES organizations(id) ON DELETE CASCADE,
  code text NOT NULL,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  sort_order integer NOT NULL DEFAULT 0,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS vusage_unique ON vehicle_usages ((COALESCE(organization_id, '00000000-0000-0000-0000-000000000000'::uuid)), code);

CREATE TABLE IF NOT EXISTS station_vehicle_categories (
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  category_code text NOT NULL,
  is_active boolean NOT NULL DEFAULT true,
  PRIMARY KEY (station_id, category_code)
);

CREATE TABLE IF NOT EXISTS station_vehicle_usages (
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  usage_code text NOT NULL,
  is_active boolean NOT NULL DEFAULT true,
  PRIMARY KEY (station_id, usage_code)
);

CREATE TABLE IF NOT EXISTS station_usage_categories (
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  usage_code text NOT NULL,
  category_code text NOT NULL,
  PRIMARY KEY (station_id, usage_code, category_code)
);

CREATE TABLE IF NOT EXISTS service_families (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid REFERENCES organizations(id) ON DELETE CASCADE,
  code text NOT NULL,
  name text NOT NULL,
  sort_order integer NOT NULL DEFAULT 0,
  is_active boolean NOT NULL DEFAULT true
);
CREATE UNIQUE INDEX IF NOT EXISTS sfam_unique ON service_families ((COALESCE(organization_id, '00000000-0000-0000-0000-000000000000'::uuid)), code);

CREATE TABLE IF NOT EXISTS services (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid REFERENCES stations(id) ON DELETE CASCADE,
  family_code text NOT NULL DEFAULT '',
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  duration_minutes integer NOT NULL DEFAULT 0,
  price_xof integer NOT NULL DEFAULT 0,
  is_free boolean NOT NULL DEFAULT false,
  requires_quote boolean NOT NULL DEFAULT false,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS services_station_idx ON services(station_id);

CREATE TABLE IF NOT EXISTS service_templates (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code text NOT NULL UNIQUE,
  name text NOT NULL,
  family_code text NOT NULL DEFAULT '',
  description text NOT NULL DEFAULT '',
  default_duration_minutes integer NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS service_option_categories (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid REFERENCES organizations(id) ON DELETE CASCADE,
  code text NOT NULL,
  name text NOT NULL,
  scope text NOT NULL DEFAULT 'station', -- station | prestation | vehicule
  sort_order integer NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS soc_unique ON service_option_categories ((COALESCE(organization_id, '00000000-0000-0000-0000-000000000000'::uuid)), code);

CREATE TABLE IF NOT EXISTS service_options (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid REFERENCES stations(id) ON DELETE CASCADE,
  category_code text NOT NULL DEFAULT '',
  service_id uuid REFERENCES services(id) ON DELETE CASCADE,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  price_xof integer NOT NULL DEFAULT 0,
  is_active boolean NOT NULL DEFAULT true
);
CREATE INDEX IF NOT EXISTS sopts_station_idx ON service_options(station_id);

CREATE TABLE IF NOT EXISTS service_compatibilities (
  service_id uuid NOT NULL REFERENCES services(id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('category','usage')),
  code text NOT NULL,
  PRIMARY KEY (service_id, kind, code)
);

CREATE TABLE IF NOT EXISTS price_rules (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  service_id uuid NOT NULL REFERENCES services(id) ON DELETE CASCADE,
  customer_id uuid,
  category_code text NOT NULL DEFAULT '',
  usage_code text NOT NULL DEFAULT '',
  price_xof integer NOT NULL,
  valid_from date,
  valid_to date,
  reason text NOT NULL DEFAULT '',
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS prules_station_idx ON price_rules(station_id);

CREATE TABLE IF NOT EXISTS customers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid REFERENCES stations(id) ON DELETE CASCADE,
  name text NOT NULL,
  phone text NOT NULL DEFAULT '',
  email text NOT NULL DEFAULT '',
  note text NOT NULL DEFAULT '',
  is_company boolean NOT NULL DEFAULT false,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE price_rules DROP CONSTRAINT IF EXISTS price_rules_customer_id_fkey;
ALTER TABLE price_rules ADD CONSTRAINT price_rules_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS customers_org_idx ON customers(organization_id);

CREATE TABLE IF NOT EXISTS wash_orders (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  code text NOT NULL,
  status text NOT NULL DEFAULT 'pending',
  customer_id uuid REFERENCES customers(id),
  washer_id uuid,
  manager_note text NOT NULL DEFAULT '',
  vehicle_plate text NOT NULL DEFAULT '',
  vehicle_ref text NOT NULL DEFAULT '',
  vehicle_category_id uuid,
  vehicle_category_name text NOT NULL DEFAULT '',
  vehicle_usage_id uuid,
  vehicle_usage_name text NOT NULL DEFAULT '',
  subtotal_xof integer NOT NULL DEFAULT 0,
  options_total_xof integer NOT NULL DEFAULT 0,
  discount_xof integer NOT NULL DEFAULT 0,
  total_xof integer NOT NULL DEFAULT 0,
  paid_xof integer NOT NULL DEFAULT 0,
  due_xof integer NOT NULL DEFAULT 0,
  started_at timestamptz,
  quality_checked_at timestamptz,
  ready_at timestamptz,
  completed_at timestamptz,
  canceled_at timestamptz,
  cancel_reason text NOT NULL DEFAULT '',
  created_by uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS worders_station_idx ON wash_orders(station_id, status);

CREATE TABLE IF NOT EXISTS wash_order_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id uuid NOT NULL REFERENCES wash_orders(id) ON DELETE CASCADE,
  service_id uuid REFERENCES services(id),
  service_name text NOT NULL,
  quantity integer NOT NULL DEFAULT 1,
  unit_price_xof integer NOT NULL DEFAULT 0,
  line_total_xof integer NOT NULL DEFAULT 0,
  price_source text NOT NULL DEFAULT '',
  price_source_label text NOT NULL DEFAULT '',
  options jsonb NOT NULL DEFAULT '[]'::jsonb,
  options_total_xof integer NOT NULL DEFAULT 0
);

ALTER TABLE wash_order_items DROP CONSTRAINT IF EXISTS witems_service_fkey;
ALTER TABLE wash_order_items ADD CONSTRAINT witems_service_fkey FOREIGN KEY (service_id) REFERENCES services(id) ON DELETE SET NULL;
ALTER TABLE wash_orders DROP CONSTRAINT IF EXISTS worders_washer_fkey;
ALTER TABLE wash_orders ADD CONSTRAINT worders_washer_fkey FOREIGN KEY (washer_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE wash_orders DROP CONSTRAINT IF EXISTS worders_vcat_fkey;
ALTER TABLE wash_orders ADD CONSTRAINT worders_vcat_fkey FOREIGN KEY (vehicle_category_id) REFERENCES vehicle_categories(id) ON DELETE SET NULL;
ALTER TABLE wash_orders DROP CONSTRAINT IF EXISTS worders_vusage_fkey;
ALTER TABLE wash_orders ADD CONSTRAINT worders_vusage_fkey FOREIGN KEY (vehicle_usage_id) REFERENCES vehicle_usages(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS payments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  order_id uuid REFERENCES wash_orders(id) ON DELETE SET NULL,
  cash_session_id uuid,
  kind text NOT NULL DEFAULT 'lavage',
  method text NOT NULL,
  payment_method_id uuid,
  amount_xof integer NOT NULL,
  reference text NOT NULL DEFAULT '',
  note text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'valid',
  void_reason text NOT NULL DEFAULT '',
  received_by uuid,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS payments_station_idx ON payments(station_id, occurred_at);

-- Lignes à organization_id/station_id NULL = modèle produit (seedé au démarrage).
-- Lors de la création d'une station, ces modèles sont copiés vers la station.
CREATE TABLE IF NOT EXISTS payment_methods (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid REFERENCES stations(id) ON DELETE CASCADE,
  code text NOT NULL,
  name text NOT NULL,
  family text NOT NULL DEFAULT 'other',
  requires_reference boolean NOT NULL DEFAULT false,
  sort_order integer NOT NULL DEFAULT 0,
  is_active boolean NOT NULL DEFAULT true
);
-- Modèle produit (organization_id/station_id NULL) ou instance de station.
CREATE UNIQUE INDEX IF NOT EXISTS payment_methods_tpl_idx ON payment_methods(code)
  WHERE organization_id IS NULL AND station_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS payment_methods_org_idx ON payment_methods(organization_id, code)
  WHERE organization_id IS NOT NULL AND station_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS payment_methods_station_idx ON payment_methods(station_id, code)
  WHERE station_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS cash_sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  opened_by uuid,
  opening_float_xof integer NOT NULL DEFAULT 0,
  status text NOT NULL DEFAULT 'open',
  expected_cash_xof integer NOT NULL DEFAULT 0,
  counted_cash_xof integer,
  discrepancy_xof integer,
  note text NOT NULL DEFAULT '',
  opened_at timestamptz NOT NULL DEFAULT now(),
  closed_at timestamptz
);
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_session_fkey;
ALTER TABLE payments ADD CONSTRAINT payments_session_fkey FOREIGN KEY (cash_session_id) REFERENCES cash_sessions(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS receipt_counters (
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  day date NOT NULL,
  seq integer NOT NULL DEFAULT 0,
  PRIMARY KEY (station_id, day)
);

CREATE TABLE IF NOT EXISTS receipts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  order_id uuid NOT NULL UNIQUE REFERENCES wash_orders(id) ON DELETE CASCADE,
  number text NOT NULL,
  issued_at timestamptz NOT NULL DEFAULT now(),
  payload jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS employees (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  user_id uuid REFERENCES users(id) ON DELETE SET NULL,
  full_name text NOT NULL,
  phone text NOT NULL DEFAULT '',
  email text NOT NULL DEFAULT '',
  role text NOT NULL DEFAULT 'washer',
  job_title text NOT NULL DEFAULT '',
  hired_on date,
  work_schedule text NOT NULL DEFAULT '',
  employment_status text NOT NULL DEFAULT 'active',
  commission_mode text,
  commission_per_wash_xof integer,
  commission_percent numeric(5,2),
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE employees DROP CONSTRAINT IF EXISTS employees_user_fkey;
ALTER TABLE employees ADD CONSTRAINT employees_user_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS employees_station_idx ON employees(station_id);

CREATE TABLE IF NOT EXISTS invitations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid REFERENCES stations(id) ON DELETE SET NULL,
  invited_by uuid,
  email text NOT NULL DEFAULT '',
  phone text NOT NULL DEFAULT '',
  role text NOT NULL,
  code text NOT NULL UNIQUE,
  token_hash text NOT NULL UNIQUE,
  state text NOT NULL DEFAULT 'pending',
  expires_at timestamptz NOT NULL,
  accepted_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS equipment (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  template_code text NOT NULL DEFAULT '',
  name text NOT NULL,
  category text NOT NULL DEFAULT '',
  quantity integer NOT NULL DEFAULT 1,
  status text NOT NULL DEFAULT 'en_service',
  purchase_date date,
  warranty_until date,
  notes text NOT NULL DEFAULT '',
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS equip_station_idx ON equipment(station_id);

CREATE TABLE IF NOT EXISTS equipment_templates (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code text NOT NULL UNIQUE,
  name text NOT NULL,
  category text NOT NULL DEFAULT '',
  description text NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS stock_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  name text NOT NULL,
  category text NOT NULL DEFAULT '',
  unit text NOT NULL DEFAULT 'unité',
  quantity numeric(12,2) NOT NULL DEFAULT 0,
  quantity_known boolean NOT NULL DEFAULT false,
  min_quantity numeric(12,2),
  expiry_date date,
  purchase_price_xof integer,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS stock_station_idx ON stock_items(station_id);

CREATE TABLE IF NOT EXISTS stock_movements (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  item_id uuid NOT NULL REFERENCES stock_items(id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('entree','sortie','inventaire','perte')),
  quantity numeric(12,2) NOT NULL,
  reason text NOT NULL DEFAULT '',
  created_by uuid,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS expenses (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id uuid NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  category text NOT NULL DEFAULT 'autre',
  label text NOT NULL,
  amount_xof integer NOT NULL,
  spent_on date NOT NULL DEFAULT CURRENT_DATE,
  status text NOT NULL DEFAULT 'pending',
  approved_by uuid,
  void_reason text NOT NULL DEFAULT '',
  note text NOT NULL DEFAULT '',
  created_by uuid,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS expenses_station_idx ON expenses(station_id, spent_on);

CREATE TABLE IF NOT EXISTS audit_logs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid,
  station_id uuid,
  actor_id uuid,
  action text NOT NULL,
  detail jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_org_idx ON audit_logs(organization_id, created_at DESC);
