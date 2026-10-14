/**
 * Graphiques — tout le vocabulaire visuel de Recharts au même endroit.
 *
 * Objectifs :
 *  - les couleurs viennent des tokens du thème (`var(--brand)`…), donc les
 *    graphiques changent de mode clair/sombre comme le reste de l'application ;
 *  - les barres se remplissent d'un dégradé vertical, les aires aussi : c'est
 *    ce qui distingue un graphique « posé » d'un aplat de couleur ;
 *  - l'infobulle et la légende sont des composants maison, en verre dépoli, avec
 *    les montants formatés en FCFA ;
 *  - l'entrée est animée en 700 ms, courbe douce, une seule fois.
 */
import type { ReactNode } from 'react'
import { Area, Bar, CartesianGrid, Cell, Legend, Pie, Tooltip, XAxis, YAxis } from 'recharts'

import { DUR, EASE_SOFT } from '@/lib/motion'
import { formatNumber, formatXOF } from '@/lib/format'

/** Palette d'accent, lue dans les tokens : elle suit le thème. */
export const CHART_COLORS = [
  'var(--brand)',
  'var(--info)',
  'var(--success)',
  'var(--warning)',
  'var(--danger)',
  '#8b5cf6',
  '#ec4899',
] as const

/** Variantes claires des mêmes teintes, pour les dégradés et les fonds. */
export const CHART_TINTS = [
  'color-mix(in oklab, var(--brand) 45%, transparent)',
  'color-mix(in oklab, var(--info) 45%, transparent)',
  'color-mix(in oklab, var(--success) 45%, transparent)',
  'color-mix(in oklab, var(--warning) 45%, transparent)',
  'color-mix(in oklab, var(--danger) 45%, transparent)',
  'color-mix(in oklab, #8b5cf6 45%, transparent)',
  'color-mix(in oklab, #ec4899 45%, transparent)',
] as const

/** Durée d'entrée commune à tous les graphiques. */
export const CHART_ANIM = { animationDuration: DUR.chart * 1000, animationEasing: 'ease-out' as const }

export const axisXProps = {
  stroke: 'var(--ink-subtle)',
  fontSize: 11,
  tickLine: false,
  axisLine: false,
  dy: 6,
  ...CHART_ANIM,
}

export const axisYProps = {
  stroke: 'var(--ink-subtle)',
  fontSize: 11,
  tickLine: false,
  axisLine: false,
  width: 52,
  tickFormatter: (value: number | string) => formatNumber(Number(value)),
  ...CHART_ANIM,
}

export const gridProps = {
  stroke: 'var(--line)',
  strokeDasharray: '2 6',
  vertical: false,
  strokeOpacity: 0.9,
}

/**
 * Dégradés réutilisables, déclarés une fois dans le `<defs>` du graphique.
 * `id` sert de référence dans `fill="url(#id)"`.
 */
export function ChartGradients({ ids }: { ids: string[] }) {
  return (
    <>
      {ids.map((id, index) => {
        const base = CHART_COLORS[index % CHART_COLORS.length]
        return (
          <linearGradient key={id} id={id} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={base} stopOpacity={0.95} />
            <stop offset="100%" stopColor={base} stopOpacity={0.55} />
          </linearGradient>
        )
      })}
    </>
  )
}

interface TooltipPayloadItem {
  name?: string | number
  value?: number | string
  color?: string
  dataKey?: string | number
}

/** Infobulle maison : verre dépoli, valeurs alignées, chiffres tabulaires. */
export function ChartTooltip({
  active,
  payload,
  label,
  formatValue = (value: number) => formatXOF(value),
  labelFormatter,
}: {
  active?: boolean
  payload?: TooltipPayloadItem[]
  label?: string | number
  formatValue?: (value: number) => string
  labelFormatter?: (label: string | number | undefined) => string
}) {
  if (!active || !payload || payload.length === 0) return null
  return (
    <div className="chart-tooltip">
      {label !== undefined ? (
        <p className="mb-1.5 font-semibold text-ink">{labelFormatter ? labelFormatter(label) : label}</p>
      ) : null}
      <div className="space-y-1">
        {payload.map((entry, index) => (
          <div key={index} className="flex items-center gap-2.5">
            <span className="dot" style={{ color: entry.color ?? 'var(--brand)', background: entry.color ?? 'var(--brand)' }} />
            <span className="text-ink-muted">{entry.name}</span>
            <span className="tabular ml-auto font-medium text-ink">
              {formatValue(Number(entry.value ?? 0))}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

/** Légende maison : pastilles alignées, sans cadre Recharts par défaut. */
export function ChartLegend({
  payload,
}: {
  payload?: Array<{ value?: string | number; color?: string }>
}) {
  if (!payload || payload.length === 0) return null
  return (
    <ul className="mt-2 flex flex-wrap items-center justify-center gap-x-4 gap-y-1.5">
      {payload.map((entry, index) => (
        <li key={index} className="flex items-center gap-1.5 text-xs text-ink-muted">
          <span className="dot" style={{ background: entry.color ?? 'var(--brand)', color: entry.color ?? 'var(--brand)' }} />
          {entry.value}
        </li>
      ))}
    </ul>
  )
}

/**
 * Barre d'histogramme : dégradé vertical, sommet arrondi, survol qui éclaircit.
 * L'animation d'entrée est décalée par série pour un effet de vague.
 */
export function ChartBar({
  dataKey,
  name,
  gradientId,
  radius = [6, 6, 0, 0],
  delay = 0,
}: {
  dataKey: string
  name: string
  gradientId: string
  radius?: [number, number, number, number]
  delay?: number
}) {
  return (
    <Bar
      dataKey={dataKey}
      name={name}
      fill={`url(#${gradientId})`}
      radius={radius}
      maxBarSize={34}
      animationDuration={DUR.chart * 1000 + delay * 1000}
      animationEasing="ease-out"
      activeBar={{ fill: `url(#${gradientId})`, filter: 'brightness(1.12)' }}
    />
  )
}

/** Aire sous courbe : dégradé qui s'efface vers le bas, ligne fine. */
export function ChartArea({
  dataKey,
  name,
  gradientId,
  stroke = 'var(--brand)',
}: {
  dataKey: string
  name: string
  gradientId: string
  stroke?: string
}) {
  return (
    <Area
      type="monotone"
      dataKey={dataKey}
      name={name}
      stroke={stroke}
      strokeWidth={2}
      fill={`url(#${gradientId})`}
      animationDuration={DUR.chart * 1000}
      animationEasing="ease-out"
    />
  )
}

/** Anneau (répartition par moyen de paiement) : trou large, coins arrondis. */
export function ChartDonut({
  data,
  dataKey,
  nameKey,
  innerRadius = '62%',
  outerRadius = '88%',
  padding = 3,
  children,
}: {
  data: Array<Record<string, unknown>>
  dataKey: string
  nameKey: string
  innerRadius?: string | number
  outerRadius?: string | number
  padding?: number
  children?: ReactNode
}) {
  return (
    <Pie
      data={data}
      dataKey={dataKey}
      nameKey={nameKey}
      innerRadius={innerRadius}
      outerRadius={outerRadius}
      paddingAngle={padding}
      cornerRadius={6}
      stroke="var(--surface)"
      strokeWidth={2}
      animationDuration={DUR.chart * 1000}
      animationEasing="ease-out"
    >
      {children}
    </Pie>
  )
}

/** Cellule colorée d'un anneau, dans l'ordre de la palette (teinte + clair). */
export function DonutCell({ index, color }: { index: number; color?: string }) {
  const base = color ?? CHART_COLORS[index % CHART_COLORS.length]
  const clair = CHART_TINTS[index % CHART_TINTS.length]
  return (
    <>
      <Cell fill={base} stroke="var(--surface)" strokeWidth={2} />
      <Cell fill={clair} opacity={0} />
    </>
  )
}

/** Grille, axes et infobulle : le trio que chaque graphique doit recevoir. */
export function ChartBase({
  yFormatter,
  tooltipFormat,
  data,
}: {
  yFormatter?: (value: number) => string
  tooltipFormat?: (value: number) => string
  /** Données affichées : sert de clé pour rejouer l'animation à chaque filtre. */
  data?: unknown
}) {
  return (
    <>
      <CartesianGrid {...gridProps} />
      <XAxis dataKey="label" {...axisXProps} />
      <YAxis
        {...axisYProps}
        tickFormatter={(value) => (yFormatter ? yFormatter(Number(value)) : formatNumber(Number(value)))}
      />
      <Tooltip
        content={<ChartTooltip formatValue={tooltipFormat} />}
        cursor={{ fill: 'color-mix(in oklab, var(--brand) 8%, transparent)' }}
        animationDuration={140}
      />
      <Legend content={<ChartLegend />} />
      {data !== undefined ? null : null}
    </>
  )
}

/** Atténuation d'une couleur (survol, fond de barre). */
export function tint(color: string, percent = 20): string {
  return `color-mix(in oklab, ${color} ${percent}%, transparent)`
}

/** Fin d'animation commune, exportée pour les cas particuliers. */
export const SOFT_EASE = EASE_SOFT
