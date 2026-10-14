/**
 * Panneau droit de la carte d'authentification : photographie de lavage
 * (public/images/lav-auto.jpeg) + texte de marque superposé.
 *
 * Le voile part du bas : il garantit la lisibilité du texte sans ternir le
 * sujet, et l'image est très légèrement agrandie au survol — un détail qui
 * donne un peu de vie à une page par ailleurs immobile.
 */
import { motion } from 'framer-motion'
import { Droplets, ShieldCheck, Wallet } from 'lucide-react'

const ARGUMENTS = [
  { icon: Wallet, texte: 'Caisse et recettes suivies au franc près' },
  { icon: Droplets, texte: 'Lavages, équipe et commissions au même endroit' },
  { icon: ShieldCheck, texte: 'Accès séparés par rôle et par station' },
]

export default function AuthImagePanel() {
  return (
    <div className="relative hidden h-full min-h-[640px] overflow-hidden lg:block">
      <motion.img
        src="/images/lav-auto.jpeg"
        alt="Voiture en cours de lavage avec mousse, gros plan sur le phare"
        initial={{ scale: 1.06, opacity: 0 }}
        animate={{ scale: 1, opacity: 1 }}
        transition={{ duration: 1.1, ease: [0.22, 1, 0.36, 1] }}
        className="absolute inset-0 h-full w-full object-cover"
      />
      {/* Voile pour la lisibilité du texte, sans cacher le sujet. */}
      <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-black/35 to-black/10" />

      <div className="absolute inset-x-6 bottom-6 text-white">
        <p className="text-lg font-semibold leading-snug">Votre station. Votre réussite.</p>
        <p className="mt-1.5 max-w-sm text-sm leading-relaxed text-white/80">
          Gérez vos prestations, votre équipe et vos recettes depuis un seul espace.
        </p>
        <ul className="mt-5 space-y-2.5">
          {ARGUMENTS.map((argument, index) => (
            <motion.li
              key={argument.texte}
              initial={{ opacity: 0, x: -10 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ duration: 0.4, ease: [0.22, 1, 0.36, 1], delay: 0.25 + index * 0.12 }}
              className="flex items-center gap-2.5 text-xs text-white/85"
            >
              <span className="flex h-6 w-6 items-center justify-center rounded-full border border-white/25 bg-white/10 backdrop-blur">
                <argument.icon size={13} />
              </span>
              {argument.texte}
            </motion.li>
          ))}
        </ul>
      </div>
    </div>
  )
}
