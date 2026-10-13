/**
 * Formatage : montants FCFA, nombres, dates (jj/mm/aaaa) et dates-heures.
 * Aucune dépendance : le fuseau de la station est géré par le backend.
 */

/** Date du jour au format AAAA-MM-JJ (entrée des filtres). */
export function today(): string {
  const now = new Date();
  const month = `${now.getMonth() + 1}`.padStart(2, "0");
  const day = `${now.getDate()}`.padStart(2, "0");
  return `${now.getFullYear()}-${month}-${day}`;
}

/** Premier jour du mois courant, AAAA-MM-JJ. */
export function startOfMonth(date = today()): string {
  return `${date.slice(0, 7)}-01`;
}

/** Lundi de la semaine contenant la date passée. */
export function startOfWeek(date = today()): string {
  const d = new Date(`${date}T12:00:00`);
  const shift = (d.getDay() + 6) % 7;
  d.setDate(d.getDate() - shift);
  const month = `${d.getMonth() + 1}`.padStart(2, "0");
  const day = `${d.getDate()}`.padStart(2, "0");
  return `${d.getFullYear()}-${month}-${day}`;
}

/** Date décalée de n jours (n négatif = en arrière). */
export function addDays(date: string, days: number): string {
  const d = new Date(`${date}T12:00:00`);
  d.setDate(d.getDate() + days);
  const month = `${d.getMonth() + 1}`.padStart(2, "0");
  const day = `${d.getDate()}`.padStart(2, "0");
  return `${d.getFullYear()}-${month}-${day}`;
}

/** Montant en FCFA : 12 500 FCFA. */
export function formatXOF(amount: number | null | undefined): string {
  const value = Math.round(Number(amount ?? 0));
  return `${value.toLocaleString("fr-FR").replace(/\u202f|\u00a0/g, " ")} FCFA`;
}

/** Nombre entier avec séparateur de milliers. */
export function formatNumber(value: number | null | undefined): string {
  return Number(value ?? 0)
    .toLocaleString("fr-FR")
    .replace(/\u202f|\u00a0/g, " ");
}

/** Date AAAA-MM-JJ ou ISO → jj/mm/aaaa. */
export function formatDate(value: string | null | undefined): string {
  if (!value) return "—";
  const date = value.slice(0, 10);
  const [y, m, d] = date.split("-");
  if (!y || !m || !d) return value;
  return `${d}/${m}/${y}`;
}

/** ISO → jj/mm/aaaa à hh:mm. */
export function formatDateTime(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return formatDate(value);
  const time =
    `${date.getHours()}`.padStart(2, "0") +
    ":" +
    `${date.getMinutes()}`.padStart(2, "0");
  return `${formatDate(value)} à ${time}`;
}

/** ISO → hh:mm (heure locale du navigateur). */
export function formatTime(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return (
    `${date.getHours()}`.padStart(2, "0") +
    ":" +
    `${date.getMinutes()}`.padStart(2, "0")
  );
}

/** Montant compact pour graphiques : 1,2 k / 3,4 M. */
export function formatCompact(value: number | null | undefined): string {
  const n = Number(value ?? 0);
  if (Math.abs(n) >= 1_000_000)
    return `${(n / 1_000_000).toLocaleString("fr-FR", { maximumFractionDigits: 1 })} M`;
  if (Math.abs(n) >= 1_000)
    return `${(n / 1_000).toLocaleString("fr-FR", { maximumFractionDigits: 1 })} k`;
  return `${n}`;
}

/** Jour court pour les séries : « lun 12 ». */
export function formatDayShort(date: string | null | undefined): string {
  if (!date) return "";
  const d = new Date(`${date.slice(0, 10)}T12:00:00`);
  if (Number.isNaN(d.getTime())) return date;
  const label = d
    .toLocaleDateString("fr-FR", { weekday: "short" })
    .replace(".", "");
  return `${label} ${d.getDate()}`;
}

/** Clé de mois lisible : « janvier 2026 ». */
export function formatMonthKey(month: string | null | undefined): string {
  if (!month) return "—";
  const d = new Date(`${month.slice(0, 7)}-01T12:00:00`);
  if (Number.isNaN(d.getTime())) return month;
  return d.toLocaleDateString("fr-FR", { month: "long", year: "numeric" });
}

/**
 * Saisie d'un montant : garde les chiffres uniquement, renvoie un entier.
 * Utile avec inputMode="numeric" : l'utilisateur peut taper des espaces.
 */
export function parseAmountInput(raw: string): number {
  const digits = raw.replace(/[^\d]/g, "");
  if (!digits) return 0;
  const value = Number(digits);
  return Number.isFinite(value) ? value : 0;
}
