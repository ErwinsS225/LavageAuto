/**
 * Garde-fou d'affichage.
 *
 * Une erreur pendant le rendu React fait disparaître toute l'application et
 * laisse une page vide : sur fond sombre, l'utilisateur voit un « écran noir »
 * sans aucune explication — c'est ce qui se produisait sur la page de
 * connexion. Cette barrière affiche un message lisible, le détail technique
 * (utile au support) et un bouton pour recharger, au lieu du vide.
 */
import { Component, type ErrorInfo, type ReactNode } from 'react'
import { RotateCw, TriangleAlert } from 'lucide-react'

interface Props {
  children: ReactNode
}

interface State {
  erreur: Error | null
}

export default class ErrorBoundary extends Component<Props, State> {
  state: State = { erreur: null }

  static getDerivedStateFromError(erreur: Error): State {
    return { erreur }
  }

  componentDidCatch(erreur: Error, info: ErrorInfo) {
    // Trace conservée dans la console : c'est le seul endroit où un incident
    // d'affichage laisse une piste exploitable.
    console.error("Erreur d'affichage WashGuard :", erreur, info.componentStack)
  }

  render() {
    const { erreur } = this.state
    if (!erreur) return this.props.children

    return (
      <div className="flex min-h-dvh flex-col items-center justify-center gap-3 bg-surface-muted px-4 text-center">
        <TriangleAlert size={30} className="text-warning" />
        <p className="text-lg font-semibold text-ink">L'écran n'a pas pu s'afficher</p>
        <p className="max-w-md text-sm leading-relaxed text-ink-muted">
          Une erreur est survenue au chargement de cette page. Rechargez-la : si le problème
          persiste, transmettez le message ci-dessous au support WashGuard.
        </p>
        <p className="max-w-md rounded-lg border border-line bg-surface px-3 py-2 text-left text-xs text-ink-muted">
          {erreur.message}
        </p>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="mt-1 flex items-center gap-2 rounded-lg bg-brand px-4 py-2 text-sm font-medium text-white hover:bg-brand-strong"
        >
          <RotateCw size={16} />
          Recharger la page
        </button>
      </div>
    )
  }
}
