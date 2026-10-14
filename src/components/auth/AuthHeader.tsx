/**
 * En-tête des écrans publics : la marque, puis le titre de l'écran.
 *
 * Le nom et la couleur viennent du composant de marque commun : les écrans
 * d'authentification affichaient auparavant « LavPro » dans un violet qui
 * n'existait nulle part ailleurs dans le produit.
 */
import { Brand } from '@/components/Brand'

export function AuthHeader({ title, subtitle }: { title?: string; subtitle?: string }) {
  return (
    <div className="mb-6">
      <div className="mb-6 flex items-center justify-start">
        <Brand size="md" />
      </div>
      {title ? <h1 className="text-xl font-bold tracking-tight text-ink">{title}</h1> : null}
      {subtitle ? <p className="mt-1 text-sm text-ink-muted">{subtitle}</p> : null}
    </div>
  )
}

export default AuthHeader
