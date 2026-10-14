#!/usr/bin/env node
/**
 * Rendu des pages — attrape l'« écran noir ».
 *
 * Une erreur pendant le rendu React fait disparaître toute l'application :
 * l'onglet reste vide, sur fond sombre, sans le moindre message. C'est
 * exactement ce qui se produisait sur /connexion, où un composant appelait
 * useClerk() alors que Clerk n'était pas configuré.
 *
 * Ce contrôle rend les pages hors navigateur (react-dom/server), dans la pile
 * de fournisseurs réelle de l'application (thème, notifications, session,
 * station), et exige que chacune produise le texte attendu. Il ne remplace pas
 * un test dans un vrai navigateur (voir frontend/tools/rendu/) mais il
 * s'exécute partout en quelques secondes, sans navigateur à installer.
 *
 * Usage : node scripts/verifications/rendu-pages.mjs
 * Code de sortie non nul si une page ne se rend pas.
 */
import { build } from 'esbuild'
import { mkdirSync, writeFileSync, rmSync } from 'node:fs'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { execFileSync } from 'node:child_process'

/**
 * Les renvois de navigation (composant <Navigate> de React Router) ne sont pas
 * exécutés ici : ils ont lieu dans un effet React, et le rendu hors navigateur
 * n'exécute aucun effet. Un écran atteint par redirection doit donc apparaître
 * dans la liste sous son adresse réelle (/onboarding, par exemple).
 *
 * Page → texte que l'utilisateur doit voir.
 *
 * `session` alimente la réponse de /auth/me avant le rendu : sans elle, la
 * page d'accueil afficherait son écran de chargement (le serveur est hors
 * ligne pendant le contrôle) et le texte attendu serait absent — ce qui ne
 * dirait rien de l'état réel de l'application.
 */
const PAGES = [
  {
    "chemin": "/connexion",
    "attendu": "Bon retour parmi nous",
    "session": null
  },
  {
    "chemin": "/inscription",
    "attendu": "Créez votre compte",
    "session": null
  },
  {
    "chemin": "/mot-de-passe-oublie",
    "attendu": "Mot de passe oublié",
    "session": null
  },
  {
    "chemin": "/rejoindre?code=K7M2P9QT",
    "attendu": "Rejoindre une entreprise",
    "session": null
  },
  {
    "chemin": "/",
    "attendu": "Pilotez votre station de lavage",
    "session": null
  },
  {
    "chemin": "/tableau-de-bord",
    "attendu": "Tableau de bord",
    "session": {
      "user": {
        "id": "usr-1",
        "full_name": "Propriétaire Test",
        "email": "owner@test.local",
        "phone": "",
        "role": "owner",
        "role_label": "Propriétaire",
        "organization_id": "org-1",
        "station_id": "stn-1"
      },
      "organization": {
        "id": "org-1",
        "name": "Station Test",
        "slug": "station-test",
        "email": "",
        "phone": "",
        "address": "",
        "city": "Abidjan",
        "country": "Côte d'Ivoire",
        "currency": "XOF",
        "timezone": "Africa/Abidjan",
        "logo_url": "",
        "status": "active",
        "created_at": "2026-01-01T00:00:00Z"
      },
      "station": {
        "id": "stn-1",
        "organization_id": "org-1",
        "name": "Station Centrale",
        "code": "CEN",
        "city": "Abidjan",
        "is_default": true,
        "is_active": true,
        "settings": {
          "max_cashier_discount_xof": 1000,
          "require_open_cash_session": true,
          "monthly_goal_xof": 0,
          "receipt_footer": "",
          "commission_mode": "per_wash",
          "commission_per_wash_xof": 500
        }
      },
      "subscription": {
        "plan_id": "plan-essai",
        "plan_name": "Essai",
        "status": "trialing",
        "trial_ends_at": "2026-12-31T00:00:00Z",
        "current_period_end": "2026-12-31T00:00:00Z",
        "limits": {
          "stations": 1,
          "users": 5,
          "washes_per_month": 100,
          "features": []
        }
      },
      "permissions": [
        "dashboard:view",
        "station:view",
        "wash:view",
        "wash:create",
        "wash:update",
        "wash:cancel",
        "wash:assign",
        "payment:view",
        "payment:create",
        "payment:void",
        "service:view",
        "service:manage",
        "customer:view",
        "customer:create",
        "customer:manage",
        "employee:view",
        "employee:manage",
        "cash:view",
        "cash:open",
        "cash:close",
        "cash:reopen",
        "expense:view",
        "expense:create",
        "expense:void",
        "report:view",
        "report:export",
        "commission:view",
        "commission:manage",
        "settings:view",
        "settings:manage",
        "organization:manage",
        "subscription:view",
        "subscription:manage",
        "audit:view",
        "washer:view"
      ],
      "must_change_password": false
    }
  },
  {
    "chemin": "/caisse",
    "attendu": "Caisse",
    "session": {
      "user": {
        "id": "usr-1",
        "full_name": "Propriétaire Test",
        "email": "owner@test.local",
        "phone": "",
        "role": "owner",
        "role_label": "Propriétaire",
        "organization_id": "org-1",
        "station_id": "stn-1"
      },
      "organization": {
        "id": "org-1",
        "name": "Station Test",
        "slug": "station-test",
        "email": "",
        "phone": "",
        "address": "",
        "city": "Abidjan",
        "country": "Côte d'Ivoire",
        "currency": "XOF",
        "timezone": "Africa/Abidjan",
        "logo_url": "",
        "status": "active",
        "created_at": "2026-01-01T00:00:00Z"
      },
      "station": {
        "id": "stn-1",
        "organization_id": "org-1",
        "name": "Station Centrale",
        "code": "CEN",
        "city": "Abidjan",
        "is_default": true,
        "is_active": true,
        "settings": {
          "max_cashier_discount_xof": 1000,
          "require_open_cash_session": true,
          "monthly_goal_xof": 0,
          "receipt_footer": "",
          "commission_mode": "per_wash",
          "commission_per_wash_xof": 500
        }
      },
      "subscription": {
        "plan_id": "plan-essai",
        "plan_name": "Essai",
        "status": "trialing",
        "trial_ends_at": "2026-12-31T00:00:00Z",
        "current_period_end": "2026-12-31T00:00:00Z",
        "limits": {
          "stations": 1,
          "users": 5,
          "washes_per_month": 100,
          "features": []
        }
      },
      "permissions": [
        "dashboard:view",
        "station:view",
        "wash:view",
        "wash:create",
        "wash:update",
        "wash:cancel",
        "wash:assign",
        "payment:view",
        "payment:create",
        "payment:void",
        "service:view",
        "service:manage",
        "customer:view",
        "customer:create",
        "customer:manage",
        "employee:view",
        "employee:manage",
        "cash:view",
        "cash:open",
        "cash:close",
        "cash:reopen",
        "expense:view",
        "expense:create",
        "expense:void",
        "report:view",
        "report:export",
        "commission:view",
        "commission:manage",
        "settings:view",
        "settings:manage",
        "organization:manage",
        "subscription:view",
        "subscription:manage",
        "audit:view",
        "washer:view"
      ],
      "must_change_password": false
    }
  },
  {
    "chemin": "/employes",
    "attendu": "Employés",
    "session": {
      "user": {
        "id": "usr-1",
        "full_name": "Propriétaire Test",
        "email": "owner@test.local",
        "phone": "",
        "role": "owner",
        "role_label": "Propriétaire",
        "organization_id": "org-1",
        "station_id": "stn-1"
      },
      "organization": {
        "id": "org-1",
        "name": "Station Test",
        "slug": "station-test",
        "email": "",
        "phone": "",
        "address": "",
        "city": "Abidjan",
        "country": "Côte d'Ivoire",
        "currency": "XOF",
        "timezone": "Africa/Abidjan",
        "logo_url": "",
        "status": "active",
        "created_at": "2026-01-01T00:00:00Z"
      },
      "station": {
        "id": "stn-1",
        "organization_id": "org-1",
        "name": "Station Centrale",
        "code": "CEN",
        "city": "Abidjan",
        "is_default": true,
        "is_active": true,
        "settings": {
          "max_cashier_discount_xof": 1000,
          "require_open_cash_session": true,
          "monthly_goal_xof": 0,
          "receipt_footer": "",
          "commission_mode": "per_wash",
          "commission_per_wash_xof": 500
        }
      },
      "subscription": {
        "plan_id": "plan-essai",
        "plan_name": "Essai",
        "status": "trialing",
        "trial_ends_at": "2026-12-31T00:00:00Z",
        "current_period_end": "2026-12-31T00:00:00Z",
        "limits": {
          "stations": 1,
          "users": 5,
          "washes_per_month": 100,
          "features": []
        }
      },
      "permissions": [
        "dashboard:view",
        "station:view",
        "wash:view",
        "wash:create",
        "wash:update",
        "wash:cancel",
        "wash:assign",
        "payment:view",
        "payment:create",
        "payment:void",
        "service:view",
        "service:manage",
        "customer:view",
        "customer:create",
        "customer:manage",
        "employee:view",
        "employee:manage",
        "cash:view",
        "cash:open",
        "cash:close",
        "cash:reopen",
        "expense:view",
        "expense:create",
        "expense:void",
        "report:view",
        "report:export",
        "commission:view",
        "commission:manage",
        "settings:view",
        "settings:manage",
        "organization:manage",
        "subscription:view",
        "subscription:manage",
        "audit:view",
        "washer:view"
      ],
      "must_change_password": false
    }
  },
  {
    "chemin": "/parametres",
    "attendu": "Paramètres",
    "session": {
      "user": {
        "id": "usr-1",
        "full_name": "Propriétaire Test",
        "email": "owner@test.local",
        "phone": "",
        "role": "owner",
        "role_label": "Propriétaire",
        "organization_id": "org-1",
        "station_id": "stn-1"
      },
      "organization": {
        "id": "org-1",
        "name": "Station Test",
        "slug": "station-test",
        "email": "",
        "phone": "",
        "address": "",
        "city": "Abidjan",
        "country": "Côte d'Ivoire",
        "currency": "XOF",
        "timezone": "Africa/Abidjan",
        "logo_url": "",
        "status": "active",
        "created_at": "2026-01-01T00:00:00Z"
      },
      "station": {
        "id": "stn-1",
        "organization_id": "org-1",
        "name": "Station Centrale",
        "code": "CEN",
        "city": "Abidjan",
        "is_default": true,
        "is_active": true,
        "settings": {
          "max_cashier_discount_xof": 1000,
          "require_open_cash_session": true,
          "monthly_goal_xof": 0,
          "receipt_footer": "",
          "commission_mode": "per_wash",
          "commission_per_wash_xof": 500
        }
      },
      "subscription": {
        "plan_id": "plan-essai",
        "plan_name": "Essai",
        "status": "trialing",
        "trial_ends_at": "2026-12-31T00:00:00Z",
        "current_period_end": "2026-12-31T00:00:00Z",
        "limits": {
          "stations": 1,
          "users": 5,
          "washes_per_month": 100,
          "features": []
        }
      },
      "permissions": [
        "dashboard:view",
        "station:view",
        "wash:view",
        "wash:create",
        "wash:update",
        "wash:cancel",
        "wash:assign",
        "payment:view",
        "payment:create",
        "payment:void",
        "service:view",
        "service:manage",
        "customer:view",
        "customer:create",
        "customer:manage",
        "employee:view",
        "employee:manage",
        "cash:view",
        "cash:open",
        "cash:close",
        "cash:reopen",
        "expense:view",
        "expense:create",
        "expense:void",
        "report:view",
        "report:export",
        "commission:view",
        "commission:manage",
        "settings:view",
        "settings:manage",
        "organization:manage",
        "subscription:view",
        "subscription:manage",
        "audit:view",
        "washer:view"
      ],
      "must_change_password": false
    }
  },
  {
    "chemin": "/espace-laveur",
    "attendu": "Laveur Test",
    "session": {
      "user": {
        "id": "usr-2",
        "full_name": "Laveur Test",
        "email": "owner@test.local",
        "phone": "",
        "role": "washer",
        "role_label": "Laveur",
        "organization_id": "org-1",
        "station_id": "stn-1"
      },
      "organization": {
        "id": "org-1",
        "name": "Station Test",
        "slug": "station-test",
        "email": "",
        "phone": "",
        "address": "",
        "city": "Abidjan",
        "country": "Côte d'Ivoire",
        "currency": "XOF",
        "timezone": "Africa/Abidjan",
        "logo_url": "",
        "status": "active",
        "created_at": "2026-01-01T00:00:00Z"
      },
      "station": {
        "id": "stn-1",
        "organization_id": "org-1",
        "name": "Station Centrale",
        "code": "CEN",
        "city": "Abidjan",
        "is_default": true,
        "is_active": true,
        "settings": {
          "max_cashier_discount_xof": 1000,
          "require_open_cash_session": true,
          "monthly_goal_xof": 0,
          "receipt_footer": "",
          "commission_mode": "per_wash",
          "commission_per_wash_xof": 500
        }
      },
      "subscription": {
        "plan_id": "plan-essai",
        "plan_name": "Essai",
        "status": "trialing",
        "trial_ends_at": "2026-12-31T00:00:00Z",
        "current_period_end": "2026-12-31T00:00:00Z",
        "limits": {
          "stations": 1,
          "users": 5,
          "washes_per_month": 100,
          "features": []
        }
      },
      "permissions": [
        "washer:view",
        "washer:update_status",
        "washer:report_incident",
        "washer:personal_stats"
      ],
      "must_change_password": false
    }
  },
  {
    "chemin": "/administration",
    "attendu": "Plateforme",
    "session": {
      "user": {
        "id": "usr-3",
        "full_name": "Support Test",
        "email": "owner@test.local",
        "phone": "",
        "role": "superadmin",
        "role_label": "Support",
        "organization_id": null,
        "station_id": null
      },
      "organization": null,
      "station": null,
      "subscription": null,
      "permissions": [
        "dashboard:view",
        "station:view",
        "wash:view",
        "wash:create",
        "wash:update",
        "wash:cancel",
        "wash:assign",
        "payment:view",
        "payment:create",
        "payment:void",
        "service:view",
        "service:manage",
        "customer:view",
        "customer:create",
        "customer:manage",
        "employee:view",
        "employee:manage",
        "cash:view",
        "cash:open",
        "cash:close",
        "cash:reopen",
        "expense:view",
        "expense:create",
        "expense:void",
        "report:view",
        "report:export",
        "commission:view",
        "commission:manage",
        "settings:view",
        "settings:manage",
        "organization:manage",
        "subscription:view",
        "subscription:manage",
        "audit:view",
        "washer:view"
      ],
      "must_change_password": false
    }
  },
  {
    "chemin": "/onboarding",
    "attendu": "Mise en route de votre station",
    "session": {
      "user": {
        "id": "usr-1",
        "full_name": "Propriétaire Test",
        "email": "owner@test.local",
        "phone": "",
        "role": "owner",
        "role_label": "Propriétaire",
        "organization_id": "org-1",
        "station_id": "stn-1"
      },
      "organization": {
        "id": "org-1",
        "name": "Station Test",
        "slug": "station-test",
        "email": "",
        "phone": "",
        "address": "",
        "city": "Abidjan",
        "country": "Côte d'Ivoire",
        "currency": "XOF",
        "timezone": "Africa/Abidjan",
        "logo_url": "",
        "status": "active",
        "created_at": "2026-01-01T00:00:00Z"
      },
      "station": {
        "id": "stn-1",
        "organization_id": "org-1",
        "name": "Station Centrale",
        "code": "CEN",
        "city": "Abidjan",
        "is_default": true,
        "is_active": true,
        "settings": {
          "max_cashier_discount_xof": 1000,
          "require_open_cash_session": true,
          "monthly_goal_xof": 0,
          "receipt_footer": "",
          "commission_mode": "per_wash",
          "commission_per_wash_xof": 500
        }
      },
      "subscription": {
        "plan_id": "plan-essai",
        "plan_name": "Essai",
        "status": "trialing",
        "trial_ends_at": "2026-12-31T00:00:00Z",
        "current_period_end": "2026-12-31T00:00:00Z",
        "limits": {
          "stations": 1,
          "users": 5,
          "washes_per_month": 100,
          "features": []
        }
      },
      "permissions": [
        "dashboard:view",
        "station:view",
        "wash:view",
        "wash:create",
        "wash:update",
        "wash:cancel",
        "wash:assign",
        "payment:view",
        "payment:create",
        "payment:void",
        "service:view",
        "service:manage",
        "customer:view",
        "customer:create",
        "customer:manage",
        "employee:view",
        "employee:manage",
        "cash:view",
        "cash:open",
        "cash:close",
        "cash:reopen",
        "expense:view",
        "expense:create",
        "expense:void",
        "report:view",
        "report:export",
        "commission:view",
        "commission:manage",
        "settings:view",
        "settings:manage",
        "organization:manage",
        "subscription:view",
        "subscription:manage",
        "audit:view",
        "washer:view"
      ],
      "must_change_password": false,
      "onboarding": {
        "organization_id": "org-1",
        "state": "pending",
        "progress": {
          "done": 1,
          "total": 4,
          "percent": 25,
          "steps_done": 1,
          "steps_total": 5
        },
        "steps": [
          {
            "code": "espace",
            "title": "Votre espace existe",
            "description": "L'entreprise, la station et le compte propriétaire sont créés.",
            "required": true,
            "done": true,
            "state": "completed",
            "detail": "1 station enregistrée",
            "screen": "",
            "hint": "",
            "data": {}
          },
          {
            "code": "entreprise",
            "title": "Identité de l'entreprise",
            "description": "Raison sociale, téléphone et ville.",
            "required": true,
            "done": false,
            "state": "not_started",
            "detail": "Téléphone manquant, ville manquante",
            "screen": "/parametres",
            "hint": "Renseignez le téléphone et la ville dans les paramètres.",
            "data": {}
          },
          {
            "code": "station",
            "title": "Identité de la station",
            "description": "Nom et ville de la station exploitée.",
            "required": true,
            "done": true,
            "state": "completed",
            "detail": "Station Centrale, Abidjan",
            "screen": "",
            "hint": "",
            "data": {}
          },
          {
            "code": "prestations",
            "title": "Prestations proposées",
            "description": "Les lavages que vous vendez.",
            "required": true,
            "done": false,
            "state": "not_started",
            "detail": "3 prestations actives",
            "screen": "/prestations",
            "hint": "Vérifiez vos prestations.",
            "data": {}
          },
          {
            "code": "tarifs",
            "title": "Tarifs",
            "description": "Chaque prestation doit avoir son prix.",
            "required": true,
            "done": false,
            "state": "not_started",
            "detail": "0 prestation avec un prix",
            "screen": "/prestations",
            "hint": "Renseignez le prix de vos prestations.",
            "data": {}
          }
        ],
        "missing": [
          {
            "code": "entreprise",
            "title": "Identité de l'entreprise",
            "hint": "Renseignez le téléphone et la ville.",
            "screen": "/parametres",
            "description": "Raison sociale, téléphone et ville."
          },
          {
            "code": "tarifs",
            "title": "Tarifs",
            "hint": "Renseignez le prix de vos prestations.",
            "screen": "/prestations",
            "description": "Chaque prestation doit avoir son prix."
          }
        ],
        "can_activate": false,
        "started_at": "",
        "activated_at": ""
      }
    },
    "interdit": "Commissions",
    "note": "session en configuration : l'écran de mise en route s'affiche et le menu ne propose pas encore les écrans d'exploitation"
  },
  {
    "chemin": "/prestations",
    "attendu": "Prestations",
    "session": {
      "user": {
        "id": "usr-1",
        "full_name": "Propriétaire Test",
        "email": "owner@test.local",
        "phone": "",
        "role": "owner",
        "role_label": "Propriétaire",
        "organization_id": "org-1",
        "station_id": "stn-1"
      },
      "organization": {
        "id": "org-1",
        "name": "Station Test",
        "slug": "station-test",
        "email": "",
        "phone": "",
        "address": "",
        "city": "Abidjan",
        "country": "Côte d'Ivoire",
        "currency": "XOF",
        "timezone": "Africa/Abidjan",
        "logo_url": "",
        "status": "active",
        "created_at": "2026-01-01T00:00:00Z"
      },
      "station": {
        "id": "stn-1",
        "organization_id": "org-1",
        "name": "Station Centrale",
        "code": "CEN",
        "city": "Abidjan",
        "is_default": true,
        "is_active": true,
        "settings": {
          "max_cashier_discount_xof": 1000,
          "require_open_cash_session": true,
          "monthly_goal_xof": 0,
          "receipt_footer": "",
          "commission_mode": "per_wash",
          "commission_per_wash_xof": 500
        }
      },
      "subscription": {
        "plan_id": "plan-essai",
        "plan_name": "Essai",
        "status": "trialing",
        "trial_ends_at": "2026-12-31T00:00:00Z",
        "current_period_end": "2026-12-31T00:00:00Z",
        "limits": {
          "stations": 1,
          "users": 5,
          "washes_per_month": 100,
          "features": []
        }
      },
      "permissions": [
        "dashboard:view",
        "station:view",
        "wash:view",
        "wash:create",
        "wash:update",
        "wash:cancel",
        "wash:assign",
        "payment:view",
        "payment:create",
        "payment:void",
        "service:view",
        "service:manage",
        "customer:view",
        "customer:create",
        "customer:manage",
        "employee:view",
        "employee:manage",
        "cash:view",
        "cash:open",
        "cash:close",
        "cash:reopen",
        "expense:view",
        "expense:create",
        "expense:void",
        "report:view",
        "report:export",
        "commission:view",
        "commission:manage",
        "settings:view",
        "settings:manage",
        "organization:manage",
        "subscription:view",
        "subscription:manage",
        "audit:view",
        "washer:view"
      ],
      "must_change_password": false
    }
  },
  {
    "chemin": "/tarifs",
    "attendu": "Tarifs",
    "session": {
      "user": {
        "id": "usr-1",
        "full_name": "Propriétaire Test",
        "email": "owner@test.local",
        "phone": "",
        "role": "owner",
        "role_label": "Propriétaire",
        "organization_id": "org-1",
        "station_id": "stn-1"
      },
      "organization": {
        "id": "org-1",
        "name": "Station Test",
        "slug": "station-test",
        "email": "",
        "phone": "",
        "address": "",
        "city": "Abidjan",
        "country": "Côte d'Ivoire",
        "currency": "XOF",
        "timezone": "Africa/Abidjan",
        "logo_url": "",
        "status": "active",
        "created_at": "2026-01-01T00:00:00Z"
      },
      "station": {
        "id": "stn-1",
        "organization_id": "org-1",
        "name": "Station Centrale",
        "code": "CEN",
        "city": "Abidjan",
        "is_default": true,
        "is_active": true,
        "settings": {
          "max_cashier_discount_xof": 1000,
          "require_open_cash_session": true,
          "monthly_goal_xof": 0,
          "receipt_footer": "",
          "commission_mode": "per_wash",
          "commission_per_wash_xof": 500
        }
      },
      "subscription": {
        "plan_id": "plan-essai",
        "plan_name": "Essai",
        "status": "trialing",
        "trial_ends_at": "2026-12-31T00:00:00Z",
        "current_period_end": "2026-12-31T00:00:00Z",
        "limits": {
          "stations": 1,
          "users": 5,
          "washes_per_month": 100,
          "features": []
        }
      },
      "permissions": [
        "dashboard:view",
        "station:view",
        "wash:view",
        "wash:create",
        "wash:update",
        "wash:cancel",
        "wash:assign",
        "payment:view",
        "payment:create",
        "payment:void",
        "service:view",
        "service:manage",
        "customer:view",
        "customer:create",
        "customer:manage",
        "employee:view",
        "employee:manage",
        "cash:view",
        "cash:open",
        "cash:close",
        "cash:reopen",
        "expense:view",
        "expense:create",
        "expense:void",
        "report:view",
        "report:export",
        "commission:view",
        "commission:manage",
        "settings:view",
        "settings:manage",
        "organization:manage",
        "subscription:view",
        "subscription:manage",
        "audit:view",
        "washer:view"
      ],
      "must_change_password": false
    }
  },
{
  "chemin": "/parametres?onglet=equipe",
  "attendu": "Équipe de la station",
  "session": {
    "user": {
      "id": "usr-1",
      "full_name": "Propriétaire Test",
      "email": "owner@test.local",
      "phone": "",
      "role": "owner",
      "role_label": "Propriétaire",
      "organization_id": "org-1",
      "station_id": "stn-1"
    },
    "organization": {
      "id": "org-1",
      "name": "Station Test",
      "slug": "station-test",
      "email": "",
      "phone": "",
      "address": "",
      "city": "Abidjan",
      "country": "Côte d'Ivoire",
      "currency": "XOF",
      "timezone": "Africa/Abidjan",
      "logo_url": "",
      "status": "active",
      "created_at": "2026-01-01T00:00:00Z"
    },
    "station": {
      "id": "stn-1",
      "organization_id": "org-1",
      "name": "Station Centrale",
      "code": "CEN",
      "city": "Abidjan",
      "is_default": true,
      "is_active": true,
      "settings": {
        "max_cashier_discount_xof": 1000,
        "require_open_cash_session": true,
        "monthly_goal_xof": 0,
        "receipt_footer": "",
        "commission_mode": "per_wash",
        "commission_per_wash_xof": 500
      }
    },
    "subscription": {
      "plan_id": "plan-essai",
      "plan_name": "Essai",
      "status": "trialing",
      "trial_ends_at": "2026-12-31T00:00:00Z",
      "current_period_end": "2026-12-31T00:00:00Z",
      "limits": {
        "stations": 1,
        "users": 5,
        "washes_per_month": 100,
        "features": []
      }
    },
    "permissions": [
      "dashboard:view",
      "station:view",
      "wash:view",
      "wash:create",
      "wash:update",
      "wash:cancel",
      "wash:assign",
      "payment:view",
      "payment:create",
      "payment:void",
      "service:view",
      "service:manage",
      "customer:view",
      "customer:create",
      "customer:manage",
      "employee:view",
      "employee:manage",
      "cash:view",
      "cash:open",
      "cash:close",
      "cash:reopen",
      "expense:view",
      "expense:create",
      "expense:void",
      "report:view",
      "report:export",
      "commission:view",
      "commission:manage",
      "settings:view",
      "settings:manage",
      "organization:manage",
      "subscription:view",
      "subscription:manage",
      "audit:view",
      "washer:view"
    ],
    "must_change_password": false
  }
},
{
  "chemin": "/parametres?onglet=materiel",
  "attendu": "Matériel de la station",
  "session": {
    "user": {
      "id": "usr-1",
      "full_name": "Propriétaire Test",
      "email": "owner@test.local",
      "phone": "",
      "role": "owner",
      "role_label": "Propriétaire",
      "organization_id": "org-1",
      "station_id": "stn-1"
    },
    "organization": {
      "id": "org-1",
      "name": "Station Test",
      "slug": "station-test",
      "email": "",
      "phone": "",
      "address": "",
      "city": "Abidjan",
      "country": "Côte d'Ivoire",
      "currency": "XOF",
      "timezone": "Africa/Abidjan",
      "logo_url": "",
      "status": "active",
      "created_at": "2026-01-01T00:00:00Z"
    },
    "station": {
      "id": "stn-1",
      "organization_id": "org-1",
      "name": "Station Centrale",
      "code": "CEN",
      "city": "Abidjan",
      "is_default": true,
      "is_active": true,
      "settings": {
        "max_cashier_discount_xof": 1000,
        "require_open_cash_session": true,
        "monthly_goal_xof": 0,
        "receipt_footer": "",
        "commission_mode": "per_wash",
        "commission_per_wash_xof": 500
      }
    },
    "subscription": {
      "plan_id": "plan-essai",
      "plan_name": "Essai",
      "status": "trialing",
      "trial_ends_at": "2026-12-31T00:00:00Z",
      "current_period_end": "2026-12-31T00:00:00Z",
      "limits": {
        "stations": 1,
        "users": 5,
        "washes_per_month": 100,
        "features": []
      }
    },
    "permissions": [
      "dashboard:view",
      "station:view",
      "wash:view",
      "wash:create",
      "wash:update",
      "wash:cancel",
      "wash:assign",
      "payment:view",
      "payment:create",
      "payment:void",
      "service:view",
      "service:manage",
      "customer:view",
      "customer:create",
      "customer:manage",
      "employee:view",
      "employee:manage",
      "cash:view",
      "cash:open",
      "cash:close",
      "cash:reopen",
      "expense:view",
      "expense:create",
      "expense:void",
      "report:view",
      "report:export",
      "commission:view",
      "commission:manage",
      "settings:view",
      "settings:manage",
      "organization:manage",
      "subscription:view",
      "subscription:manage",
      "audit:view",
      "washer:view"
    ],
    "must_change_password": false
  }
},
{
  "chemin": "/parametres?onglet=commandes",
  "attendu": "Comment un véhicule circule dans la station",
  "session": {
    "user": {
      "id": "usr-1",
      "full_name": "Propriétaire Test",
      "email": "owner@test.local",
      "phone": "",
      "role": "owner",
      "role_label": "Propriétaire",
      "organization_id": "org-1",
      "station_id": "stn-1"
    },
    "organization": {
      "id": "org-1",
      "name": "Station Test",
      "slug": "station-test",
      "email": "",
      "phone": "",
      "address": "",
      "city": "Abidjan",
      "country": "Côte d'Ivoire",
      "currency": "XOF",
      "timezone": "Africa/Abidjan",
      "logo_url": "",
      "status": "active",
      "created_at": "2026-01-01T00:00:00Z"
    },
    "station": {
      "id": "stn-1",
      "organization_id": "org-1",
      "name": "Station Centrale",
      "code": "CEN",
      "city": "Abidjan",
      "is_default": true,
      "is_active": true,
      "settings": {
        "max_cashier_discount_xof": 1000,
        "require_open_cash_session": true,
        "monthly_goal_xof": 0,
        "receipt_footer": "",
        "commission_mode": "per_wash",
        "commission_per_wash_xof": 500
      }
    },
    "subscription": {
      "plan_id": "plan-essai",
      "plan_name": "Essai",
      "status": "trialing",
      "trial_ends_at": "2026-12-31T00:00:00Z",
      "current_period_end": "2026-12-31T00:00:00Z",
      "limits": {
        "stations": 1,
        "users": 5,
        "washes_per_month": 100,
        "features": []
      }
    },
    "permissions": [
      "dashboard:view",
      "station:view",
      "wash:view",
      "wash:create",
      "wash:update",
      "wash:cancel",
      "wash:assign",
      "payment:view",
      "payment:create",
      "payment:void",
      "service:view",
      "service:manage",
      "customer:view",
      "customer:create",
      "customer:manage",
      "employee:view",
      "employee:manage",
      "cash:view",
      "cash:open",
      "cash:close",
      "cash:reopen",
      "expense:view",
      "expense:create",
      "expense:void",
      "report:view",
      "report:export",
      "commission:view",
      "commission:manage",
      "settings:view",
      "settings:manage",
      "organization:manage",
      "subscription:view",
      "subscription:manage",
      "audit:view",
      "washer:view"
    ],
    "must_change_password": false
  }
}
]


// Le dossier de travail vit dans node_modules (ignoré par Git) : les modules
// React doivent pouvoir être résolus depuis le projet.
const racine = new URL('../..', import.meta.url).pathname
const dossier = join(racine, 'node_modules', '.washguard-rendu')
rmSync(dossier, { recursive: true, force: true })
mkdirSync(dossier, { recursive: true })

const entree = `
// Décor minimal attendu par du code prévu pour le navigateur.
const noop = () => {}
const storage = { getItem: () => null, setItem: noop, removeItem: noop, clear: noop }
globalThis.localStorage = storage
globalThis.sessionStorage = storage
globalThis.window = {
  localStorage: storage,
  matchMedia: () => ({ matches: false, addEventListener: noop, removeEventListener: noop }),
  addEventListener: noop,
  removeEventListener: noop,
  dispatchEvent: () => true,
  setTimeout,
  clearTimeout,
  history: { pushState: noop, replaceState: noop },
  location: { pathname: '/', search: '', href: 'http://localhost/' },
}
globalThis.document = {
  documentElement: { classList: { add: noop, remove: noop, toggle: noop } },
  querySelector: () => null,
  addEventListener: noop,
  removeEventListener: noop,
  title: '',
  cookie: '',
}
globalThis.navigator = { userAgent: 'node' }
// Le serveur n'est pas joignable depuis ce contrôle : toute requête échoue
// proprement, comme si l'utilisateur était hors ligne.
globalThis.fetch = async () => { throw new Error('réseau indisponible (contrôle de rendu)') }

import { renderToString } from 'react-dom/server'

import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import App from '@/App'
import { ToastProvider } from '@/components/Toaster'
import { StationProvider, ThemeProvider } from '@/hooks/useStation'
import { AuthProvider } from '@/lib/clerk'

const pages = ${JSON.stringify(PAGES)}

const resultats = []
// Fonction asynchrone : attendre le chargement des écrans à la demande entre
// les deux passes est impossible au niveau racine d'un bundle CommonJS.
async function rendreToutes() {
for (const { chemin, session } of pages) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  // Réponse de /auth/me connue d'avance : le rendu est synchrone, il ne peut
  // pas attendre le réseau.
  queryClient.setQueryData(['me'], session)
  try {
    const element = (
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[chemin]}>
          <ThemeProvider>
            <ToastProvider>
              <AuthProvider>
                <StationProvider>
                  <App />
                </StationProvider>
              </AuthProvider>
            </ToastProvider>
          </ThemeProvider>
        </MemoryRouter>
      </QueryClientProvider>
    )
    // Deux passes : les écrans sont chargés à la demande (React.lazy). La
    // première passe ne montre donc que l'écran d'attente ; on laisse la boucle
    // d'événements travailler (les modules se chargent), puis on rend de
    // nouveau. Sans cela, le contrôle se contenterait de vérifier le menu
    // latéral — où figurent les noms des écrans — et laisserait passer un écran
    // vide (le « tableau de bord blanc » qu'on cherche justement à éviter).
    renderToString(element)
    await new Promise((suite) => setTimeout(suite, 0))
    const html = renderToString(element)
    resultats.push({ chemin, html })
  } catch (erreur) {
    resultats.push({ chemin, erreur: (erreur && erreur.message) || String(erreur) })
  }
}
console.log(JSON.stringify(resultats))
}
void rendreToutes()
`

writeFileSync(join(dossier, 'entree.tsx'), entree)

// Sortie en CommonJS : react-dom/server est du code CommonJS qui utilise
// require() pour les modules internes de Node, ce qu'un bundle ES ne permet pas.
const sortie = join(dossier, 'entree.cjs')
await build({
  entryPoints: [join(dossier, 'entree.tsx')],
  outfile: sortie,
  bundle: true,
  platform: 'node',
  format: 'cjs',
  jsx: 'automatic',
  logLevel: 'error',
  alias: { '@': join(racine, 'src') },
  // Tout est regroupé dans un seul fichier : le rendu vit dans un dossier
  // temporaire, hors du projet, et n'aurait sinon aucune dépendance à portée.
  define: {
    'process.env.NODE_ENV': '"production"',
    // Vite remplace import.meta.env à la compilation : on simule une
    // installation sans Clerk configuré (le cas par défaut).
    'import.meta.env.VITE_CLERK_PUBLISHABLE_KEY': 'undefined',
    'import.meta.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY': 'undefined',
  },
})

let resultats
try {
  const brut = execFileSync(process.execPath, [pathToFileURL(sortie).pathname], {
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  resultats = JSON.parse(brut.trim().split('\n').pop())
} catch (erreur) {
  console.error("Le rendu n'a pas pu s'exécuter :")
  console.error(erreur.stderr?.toString() || erreur.message)
  rmSync(dossier, { recursive: true, force: true })
  process.exit(2)
}

let echecs = 0
for (const { chemin, html, erreur } of resultats) {
  const page = PAGES.find((p) => p.chemin === chemin)
  const attendu = page?.attendu ?? ''
  const interdit = page?.interdit ?? ''
  if (erreur) {
    console.error(`KO    ${chemin} — le rendu a échoué : ${erreur}`)
    echecs += 1
    continue
  }
  if (!html || html.length < 200) {
    console.error(`KO    ${chemin} — page vide (${html?.length ?? 0} caractères) : écran noir`)
    echecs += 1
    continue
  }
  if (!html.includes(attendu)) {
    console.error(`KO    ${chemin} — le texte « ${attendu} » est absent`)
    echecs += 1
    continue
  }
  if (interdit && html.includes(interdit)) {
    console.error(`KO    ${chemin} — le texte « ${interdit} » ne devrait pas apparaître`)
    echecs += 1
    continue
  }
  console.log(`OK    ${chemin} rendu (${html.length} caractères) : « ${attendu} » présent`)
}

rmSync(dossier, { recursive: true, force: true })

if (echecs > 0) {
  console.error(`\n${echecs}/${PAGES.length} page(s) ne s'affichent pas`)
  process.exit(1)
}
console.log(`\n${PAGES.length}/${PAGES.length} pages s'affichent.`)
