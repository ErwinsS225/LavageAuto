/**
 * Lecture de l'abonnement côté front : limites et fonctions utilitaires.
 * Les décisions de sécurité restent côté serveur — ici on ne fait qu'afficher.
 */
import type { Me, Subscription } from "@/types";

export interface SubscriptionLimit {
  label: string;
  used: number;
  max: number;
  reached: boolean;
}

export function subscriptionStatusLabel(status: string): string {
  switch (status) {
    case "trialing":
      return "Essai gratuit";
    case "active":
      return "Actif";
    case "past_due":
      return "Paiement en retard";
    case "expired":
      return "Expiré";
    case "suspended":
      return "Suspendu";
    case "canceled":
      return "Annulé";
    default:
      return "—";
  }
}

export function subscriptionTone(status: string): string {
  switch (status) {
    case "trialing":
      return "badge-warning";
    case "active":
      return "badge-success";
    case "past_due":
    case "expired":
    case "suspended":
    case "canceled":
      return "badge-danger";
    default:
      return "badge-neutral";
  }
}

/** Limite d'usage d'une ressource ; reached true quand la formule bloque. */
export function subscriptionLimit(
  subscription: Subscription | null | undefined,
  key: "stations" | "employees" | "wash_orders",
): SubscriptionLimit {
  const labels = {
    stations: "stations",
    employees: "employés",
    wash_orders: "lavages / mois",
  } as const;
  const used = Number(subscription?.usage?.[key] ?? 0);
  const max = Number(
    subscription?.[
      `max_${key === "wash_orders" ? "wash_orders_month" : key}` as keyof Subscription
    ] ?? 0,
  );
  return {
    label: labels[key],
    used,
    max,
    reached: max > 0 && used >= max,
  };
}

/** Vrai si l'organisation est bloquée (abonnement expiré, essai terminé…). */
export function subscriptionBlocked(me: Me | null | undefined): boolean {
  const status = me?.subscription?.status ?? "";
  return ["expired", "suspended", "canceled"].includes(status);
}
