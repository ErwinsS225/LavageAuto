-- ---------------------------------------------------------------------------
-- WashGuard — schéma PostgreSQL (Neon)
-- Conforme au modèle du serveur Express (server.ts), avec persistance réelle.
-- ---------------------------------------------------------------------------
-- Formules d'abonnement
CREATE TABLE IF NOT EXISTS plans (
  id                      TEXT PRIMARY KEY,
  code                    TEXT NOT NULL UNIQUE,
  name                    TEXT NOT NULL,
  description             TEXT NOT NULL DEFAULT '',
  price_xof               INTEGER NOT NULL DEFAULT 0,
  billing_period          TEXT NOT NULL DEFAULT 'mensuel',
  max_stations            INTEGER NOT NULL DEFAULT 1,
  max_employees           INTEGER NOT NULL DEFAULT 5,
  max_wash_orders_month   INTEGER NOT NULL DEFAULT 2000,
  trial_days              INTEGER NOT NULL DEFAULT 14,
  features                JSONB NOT NULL DEFAULT '{}'::jsonb,
  sort_order              INTEGER NOT NULL DEFAULT 0,
  is_active               BOOLEAN NOT NULL DEFAULT TRUE
);
-- Entreprises clientes (multi-tenant)
CREATE TABLE IF NOT EXISTS organizations (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  slug        TEXT NOT NULL UNIQUE,
  email       TEXT NOT NULL DEFAULT '',
  phone       TEXT NOT NULL DEFAULT '',
  address     TEXT NOT NULL DEFAULT '',
  city        TEXT NOT NULL DEFAULT 'Abidjan',
  country     TEXT NOT NULL DEFAULT 'Côte d''Ivoire',
  currency    TEXT NOT NULL DEFAULT 'XOF',
  timezone    TEXT NOT NULL DEFAULT 'Africa/Abidjan',
  logo_url    TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','archived')),
  notes       TEXT NOT NULL DEFAULT '',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Stations de lavage
CREATE TABLE IF NOT EXISTS stations (
  id              TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name            TEXT NOT NULL,
  code            TEXT NOT NULL DEFAULT '',
  phone           TEXT NOT NULL DEFAULT '',
  address         TEXT NOT NULL DEFAULT '',
  city            TEXT NOT NULL DEFAULT 'Abidjan',
  timezone        TEXT NOT NULL DEFAULT 'Africa/Abidjan',
  currency        TEXT NOT NULL DEFAULT 'XOF',
  logo_url        TEXT NOT NULL DEFAULT '',
  opening_hours   JSONB NOT NULL DEFAULT '{}'::jsonb,
  settings        JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_default      BOOLEAN NOT NULL DEFAULT FALSE,
  is_active       BOOLEAN NOT NULL DEFAULT TRUE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_stations_org ON stations(organization_id);
-- Abonnements
CREATE TABLE IF NOT EXISTS subscriptions (
  id                   TEXT PRIMARY KEY,
  organization_id      TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  plan_id              TEXT NOT NULL REFERENCES plans(id),
  status               TEXT NOT NULL DEFAULT 'trialing'
                       CHECK (status IN ('trialing','active','past_due','expired','suspended','canceled')),
  started_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  trial_ends_at        TIMESTAMPTZ,
  current_period_start TIMESTAMPTZ,
  current_period_end   TIMESTAMPTZ,
  notes                TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_subscriptions_org ON subscriptions(organization_id);
-- Utilisateurs (employés et propriétaires)
CREATE TABLE IF NOT EXISTS users (
  id                     TEXT PRIMARY KEY,
  organization_id        TEXT REFERENCES organizations(id) ON DELETE CASCADE,
  station_id             TEXT REFERENCES stations(id) ON DELETE SET NULL,
  full_name              TEXT NOT NULL,
  email                  TEXT NOT NULL,
  phone                  TEXT NOT NULL DEFAULT '',
  password_hash          TEXT NOT NULL,
  role                   TEXT NOT NULL CHECK (role IN ('superadmin','owner','manager','cashier','washer')),
  is_active              BOOLEAN NOT NULL DEFAULT TRUE,
  must_change_password   BOOLEAN NOT NULL DEFAULT FALSE,
  last_login_at          TIMESTAMPTZ,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  commission_mode        TEXT CHECK (commission_mode IN ('none','per_wash','percent')),
  commission_per_wash_xof INTEGER,
  commission_percent     NUMERIC(5,2)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users(lower(email));
CREATE INDEX IF NOT EXISTS idx_users_org ON users(organization_id);
-- Sessions d'authentification (jeton aléatoire, durée 30 jours)
CREATE TABLE IF NOT EXISTS sessions (
  token      TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
-- Catégories de véhicules (référentiel global)
CREATE TABLE IF NOT EXISTS vehicle_categories (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  sort_order  INTEGER NOT NULL DEFAULT 0,
  is_active   BOOLEAN NOT NULL DEFAULT TRUE
);
-- Catalogue de prestations par station
CREATE TABLE IF NOT EXISTS services (
  id               TEXT PRIMARY KEY,
  organization_id  TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id       TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  name             TEXT NOT NULL,
  category         TEXT NOT NULL DEFAULT 'lavage',
  description      TEXT NOT NULL DEFAULT '',
  price_xof        INTEGER NOT NULL DEFAULT 0,
  duration_minutes INTEGER NOT NULL DEFAULT 30,
  sort_order       INTEGER NOT NULL DEFAULT 0,
  is_active        BOOLEAN NOT NULL DEFAULT TRUE
);
CREATE INDEX IF NOT EXISTS idx_services_station ON services(station_id);
-- Clients
CREATE TABLE IF NOT EXISTS customers (
  id              TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id      TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  full_name       TEXT NOT NULL,
  phone           TEXT NOT NULL DEFAULT '',
  plate           TEXT NOT NULL DEFAULT '',
  vehicle_model   TEXT NOT NULL DEFAULT '',
  notes           TEXT NOT NULL DEFAULT '',
  is_active       BOOLEAN NOT NULL DEFAULT TRUE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Sessions de caisse
CREATE TABLE IF NOT EXISTS cash_sessions (
  id                 TEXT PRIMARY KEY,
  organization_id    TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id         TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  station_name       TEXT NOT NULL DEFAULT '',
  status             TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed')),
  opened_by          TEXT NOT NULL,
  opened_by_name     TEXT NOT NULL DEFAULT '',
  opened_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  opening_float_xof  INTEGER NOT NULL DEFAULT 0,
  expected_cash_xof  INTEGER NOT NULL DEFAULT 0,
  counted_cash_xof   INTEGER,
  difference_xof     INTEGER,
  cash_in_xof        INTEGER NOT NULL DEFAULT 0,
  mobile_money_xof   INTEGER NOT NULL DEFAULT 0,
  other_in_xof       INTEGER NOT NULL DEFAULT 0,
  expenses_xof       INTEGER NOT NULL DEFAULT 0,
  operations         INTEGER NOT NULL DEFAULT 0,
  payment_count      INTEGER NOT NULL DEFAULT 0,
  expense_count      INTEGER NOT NULL DEFAULT 0,
  notes              TEXT NOT NULL DEFAULT '',
  close_notes        TEXT NOT NULL DEFAULT '',
  reopen_count       INTEGER NOT NULL DEFAULT 0,
  business_day       TEXT NOT NULL,
  closed_by          TEXT,
  closed_by_name     TEXT,
  closed_at          TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_cash_sessions_station ON cash_sessions(station_id, business_day);
-- Fiches de lavage
CREATE TABLE IF NOT EXISTS wash_orders (
  id                    TEXT PRIMARY KEY,
  organization_id       TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id            TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  reference             TEXT NOT NULL,
  customer_id           TEXT REFERENCES customers(id) ON DELETE SET NULL,
  customer_name         TEXT NOT NULL DEFAULT 'Client de passage',
  vehicle_category_id   TEXT,
  vehicle_category_name TEXT NOT NULL DEFAULT '',
  plate                 TEXT NOT NULL DEFAULT '',
  employee_id           TEXT,
  employee_name         TEXT NOT NULL DEFAULT '',
  status                TEXT NOT NULL DEFAULT 'in_progress'
                        CHECK (status IN ('pending','in_progress','completed','canceled')),
  subtotal_xof          INTEGER NOT NULL DEFAULT 0,
  discount_xof          INTEGER NOT NULL DEFAULT 0,
  discount_reason       TEXT NOT NULL DEFAULT '',
  total_xof             INTEGER NOT NULL DEFAULT 0,
  notes                 TEXT NOT NULL DEFAULT '',
  incident_note         TEXT,
  incident_reported_at  TIMESTAMPTZ,
  incident_reporter_name TEXT,
  cancel_reason         TEXT,
  created_by            TEXT NOT NULL,
  created_by_name       TEXT NOT NULL DEFAULT '',
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at            TIMESTAMPTZ,
  completed_at          TIMESTAMPTZ,
  canceled_at           TIMESTAMPTZ,
  business_day          TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_wash_orders_station_day ON wash_orders(station_id, business_day);
CREATE INDEX IF NOT EXISTS idx_wash_orders_org ON wash_orders(organization_id);
CREATE INDEX IF NOT EXISTS idx_wash_orders_customer ON wash_orders(customer_id);
CREATE INDEX IF NOT EXISTS idx_wash_orders_employee ON wash_orders(employee_id);
-- Encaissements
CREATE TABLE IF NOT EXISTS payments (
  id               TEXT PRIMARY KEY,
  organization_id  TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id       TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  wash_order_id    TEXT REFERENCES wash_orders(id) ON DELETE SET NULL,
  customer_id      TEXT REFERENCES customers(id) ON DELETE SET NULL,
  kind             TEXT NOT NULL DEFAULT 'lavage' CHECK (kind IN ('lavage','acompte','autre')),
  method           TEXT NOT NULL DEFAULT 'especes'
                   CHECK (method IN ('especes','mobile_money','carte','virement','autre')),
  amount_xof       INTEGER NOT NULL DEFAULT 0,
  reference        TEXT NOT NULL DEFAULT '',
  note             TEXT NOT NULL DEFAULT '',
  paid_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  business_day     TEXT NOT NULL,
  cash_session_id  TEXT REFERENCES cash_sessions(id) ON DELETE SET NULL,
  received_by      TEXT NOT NULL,
  received_by_name TEXT NOT NULL DEFAULT '',
  status           TEXT NOT NULL DEFAULT 'valid' CHECK (status IN ('valid','voided')),
  void_reason      TEXT
);
CREATE INDEX IF NOT EXISTS idx_payments_station_day ON payments(station_id, business_day);
CREATE INDEX IF NOT EXISTS idx_payments_order ON payments(wash_order_id);
-- Dépenses
CREATE TABLE IF NOT EXISTS expenses (
  id               TEXT PRIMARY KEY,
  organization_id  TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  station_id       TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
  category         TEXT NOT NULL DEFAULT 'autre',
  label            TEXT NOT NULL,
  amount_xof       INTEGER NOT NULL DEFAULT 0,
  method           TEXT NOT NULL DEFAULT 'especes'
                   CHECK (method IN ('especes','mobile_money','carte','virement','autre')),
  supplier         TEXT NOT NULL DEFAULT '',
  reference        TEXT NOT NULL DEFAULT '',
  note             TEXT NOT NULL DEFAULT '',
  spent_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  business_day     TEXT NOT NULL,
  cash_session_id  TEXT REFERENCES cash_sessions(id) ON DELETE SET NULL,
  recorded_by      TEXT NOT NULL,
  recorded_by_name TEXT NOT NULL DEFAULT '',
  status           TEXT NOT NULL DEFAULT 'valid' CHECK (status IN ('valid','voided')),
  void_reason      TEXT,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_expenses_station_day ON expenses(station_id, business_day);
-- Journal d'audit
CREATE TABLE IF NOT EXISTS audit_logs (
  id              BIGSERIAL PRIMARY KEY,
  organization_id TEXT,
  station_id      TEXT,
  actor_name      TEXT NOT NULL DEFAULT '',
  action          TEXT NOT NULL,
  entity          TEXT NOT NULL DEFAULT '',
  entity_id       TEXT NOT NULL DEFAULT '',
  details         JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_org ON audit_logs(organization_id, created_at DESC);
-- Lignes de prestation d'une fiche
CREATE TABLE IF NOT EXISTS wash_order_items (
  id             TEXT PRIMARY KEY,
  wash_order_id  TEXT NOT NULL REFERENCES wash_orders(id) ON DELETE CASCADE,
  service_id     TEXT,
  service_name   TEXT NOT NULL DEFAULT '',
  unit_price_xof INTEGER NOT NULL DEFAULT 0,
  quantity       INTEGER NOT NULL DEFAULT 1,
  line_total_xof INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_wash_order_items_order ON wash_order_items(wash_order_id);
CREATE INDEX IF NOT EXISTS idx_customers_org ON customers(organization_id);

-- ---------------------------------------------------------------------------
-- Compatibilité : colonnes ajoutées après coup (idempotent)
-- ---------------------------------------------------------------------------
ALTER TABLE stations ADD COLUMN IF NOT EXISTS slug TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS slug TEXT NOT NULL DEFAULT '';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE services ADD COLUMN IF NOT EXISTS organization_id TEXT;
ALTER TABLE services ADD COLUMN IF NOT EXISTS price_xof INTEGER;
ALTER TABLE services ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS price_xof INTEGER;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS plan_id TEXT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS organization_id TEXT;
ALTER TABLE cash_sessions ADD COLUMN IF NOT EXISTS reference TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS clerk_user_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_clerk ON users(clerk_user_id);

-- ---------------------------------------------------------------------------
-- Référentiel des plans (idempotent : ON CONFLICT sur le code métier).
-- ---------------------------------------------------------------------------
INSERT INTO plans (id, code, name, description, price_xof, billing_period,
                   max_stations, max_employees, max_wash_orders_month,
                   trial_days, features, sort_order)
VALUES
  ('plan-essai', 'essai', 'Essai gratuit',
   'Fonctionnalités complètes pendant 14 jours, sans engagement.',
   0, 'mensuel', 1, 3, 300, 14,
   '{"mobile_money":false,"commissions":false,"multi_sites":false,"export_csv":false,"support_prioritaire":false}'::jsonb,
   1),
  ('plan-starter', 'starter', 'Starter',
   'Pour une station indépendante qui démarre.',
   15000, 'mensuel', 1, 8, 1000, 14,
   '{"mobile_money":true,"commissions":false,"multi_sites":false,"export_csv":true,"support_prioritaire":false}'::jsonb,
   2),
  ('plan-standard', 'standard', 'Standard',
   'Pour une station avec une équipe et un suivi des commissions.',
   35000, 'mensuel', 2, 20, 5000, 14,
   '{"mobile_money":true,"commissions":true,"multi_sites":false,"export_csv":true,"support_prioritaire":true}'::jsonb,
   3),
  ('plan-premium', 'premium', 'Premium',
   'Pour un réseau de plusieurs stations de lavage.',
   75000, 'mensuel', 10, 60, 25000, 14,
   '{"mobile_money":true,"commissions":true,"multi_sites":true,"export_csv":true,"support_prioritaire":true}'::jsonb,
   4)
ON CONFLICT (code) DO UPDATE SET
  name                  = EXCLUDED.name,
  description           = EXCLUDED.description,
  price_xof             = EXCLUDED.price_xof,
  billing_period        = EXCLUDED.billing_period,
  max_stations          = EXCLUDED.max_stations,
  max_employees         = EXCLUDED.max_employees,
  max_wash_orders_month = EXCLUDED.max_wash_orders_month,
  trial_days            = EXCLUDED.trial_days,
  features              = EXCLUDED.features,
  sort_order            = EXCLUDED.sort_order,
  is_active             = TRUE;

