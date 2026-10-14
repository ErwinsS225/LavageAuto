/**
 * Données : tableaux.
 *
 * Les lignes entrent en fondu décalé, mais le décalage est plafonné : au-delà
 * d'une vingtaine de lignes, une liste ne doit pas « couler » pendant deux
 * secondes. On garde donc l'effet sur les premières et les suivantes arrivent
 * ensemble.
 */
import type { ReactNode } from 'react'
import { motion } from 'framer-motion'
import clsx from 'clsx'

import { DUR, EASE_SOFT } from '@/lib/motion'

/** Décalage d'apparition d'une ligne : 18 ms, plafonné à 220 ms. */
export function rowDelay(index: number): number {
  return Math.min(index * 0.018, 0.22)
}

/**
 * Cadre d'un tableau : contour, ombre portée et défilement horizontal.
 * À utiliser quand le tableau n'est pas déjà posé dans une carte.
 */
export function TableFrame({
  children,
  toolbar,
  className,
}: {
  children: ReactNode
  toolbar?: ReactNode
  className?: string
}) {
  return (
    <div className={clsx('card overflow-hidden', className)}>
      {toolbar ? (
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          {toolbar}
        </div>
      ) : null}
      <div className="overflow-x-auto">{children}</div>
    </div>
  )
}

/** Ligne animée : utilisée par les pages qui écrivent leur propre `<tbody>`. */
export function MotionRow({
  index,
  children,
  onClick,
  className,
}: {
  index: number
  children: ReactNode
  onClick?: () => void
  className?: string
}) {
  return (
    <motion.tr
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: DUR.base, ease: EASE_SOFT, delay: rowDelay(index) }}
      onClick={onClick}
      className={className}
    >
      {children}
    </motion.tr>
  )
}

/** Tableau simple : colonnes (libellés) + lignes de cellules. */
export function Table({ columns, rows }: { columns: string[]; rows: Array<Array<ReactNode>> }) {
  return (
    <div className="overflow-x-auto">
      <table className="table">
        <thead>
          <tr>
            {columns.map((column, index) => (
              <th key={index} className={index > 0 ? 'text-right' : ''}>
                {column}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, rowIndex) => (
            <MotionRow key={rowIndex} index={rowIndex}>
              {row.map((cell, cellIndex) => (
                <td key={cellIndex} className={cellIndex > 0 ? 'text-right' : ''}>
                  {cell}
                </td>
              ))}
            </MotionRow>
          ))}
        </tbody>
      </table>
    </div>
  )
}
