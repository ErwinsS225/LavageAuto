/**
 * Layout public (connexion, inscription, mot de passe oublié).
 *
 * La fiche de gauche porte le formulaire, le panneau de droite la photographie
 * de station. L'ensemble flotte sur un fond animé très discret (deux halos de
 * marque qui dérivent lentement) : c'est la première impression du produit, et
 * la seule page où l'on peut se permettre un décor.
 */
import { Outlet, useLocation } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'

import AuthImagePanel from '@/components/auth/AuthImagePanel'
import { pageVariants } from '@/lib/motion'

export default function AuthLayout() {
  const location = useLocation()
  return (
    <div className="relative flex min-h-dvh items-center justify-center overflow-hidden bg-canvas px-4 py-8">
      {/* Halos de fond : deux taches de couleur très diffusées, animées. */}
      <div aria-hidden="true" className="pointer-events-none absolute inset-0">
        <div className="animate-aurora absolute -left-24 -top-24 h-[28rem] w-[28rem] rounded-full bg-brand/20 blur-[110px]" />
        <div
          className="animate-aurora absolute -bottom-32 right-0 h-[26rem] w-[26rem] rounded-full bg-success/15 blur-[120px]"
          style={{ animationDelay: '-6s' }}
        />
      </div>

      <motion.div
        initial={{ opacity: 0, y: 14, scale: 0.99 }}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{ duration: 0.5, ease: [0.22, 1, 0.36, 1] }}
        className="relative mx-auto w-full max-w-5xl overflow-hidden rounded-panel border border-line bg-surface shadow-float"
      >
        <div className="grid min-h-[640px] grid-cols-1 md:grid-cols-5">
          <div className="col-span-1 px-6 py-8 sm:px-8 md:col-span-2 md:px-10 md:py-10">
            <AnimatePresence mode="wait" initial={false}>
              <motion.div
                key={location.pathname}
                variants={pageVariants}
                initial="hidden"
                animate="visible"
                exit="exit"
              >
                <Outlet />
              </motion.div>
            </AnimatePresence>
          </div>
          <div className="col-span-1 md:col-span-3">
            <AuthImagePanel />
          </div>
        </div>
      </motion.div>
    </div>
  )
}
