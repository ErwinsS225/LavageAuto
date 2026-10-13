/**
 * Panneau droit de la carte d'authentification : photographie de lavage
 * fournie (public/images/lav-auto.jpeg) + texte de marque superposé.
 */
export default function AuthImagePanel() {
  return (
    <div className="relative hidden overflow-hidden lg:block lg:w-[58%]">
      <img
        src="/images/lav-auto.jpeg"
        alt="Voiture en cours de lavage avec mousse, gros plan sur le phare"
        className="absolute inset-0 h-full w-full object-cover"
      />
      {/* Voile léger pour la lisibilité du texte, sans cacher le sujet. */}
      <div className="absolute inset-0 bg-gradient-to-b from-black/45 via-black/10 to-transparent" />
      <div className="absolute right-6 top-6 max-w-xs text-right text-white">
        <p className="text-lg font-semibold leading-snug">Votre station. Votre réussite.</p>
        <p className="mt-1 text-sm leading-relaxed text-white/85">
          Gérez vos prestations, votre équipe et vos recettes depuis un seul espace.
        </p>
      </div>
    </div>
  )
}
