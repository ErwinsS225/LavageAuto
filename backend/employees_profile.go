package main

// ---------------------------------------------------------------------------
// L'équipe, décrite au-delà du compte.
//
// Un compte d'employé disait : qui peut se connecter, à quelle station, avec
// quel rôle. Il ne disait pas ce que la personne fait réellement, depuis quand,
// ni comment elle est rémunérée. Ces informations sont **facultatives** : une
// station qui n'en renseigne aucune reste parfaitement utilisable, et un champ
// vide s'affiche « non renseigné » — jamais « 0 », jamais une date inventée.
//
// La rémunération est à part : elle demande la permission `employee:compensation`
// en plus des droits de gestion d'équipe. Un gérant qui constitue l'équipe n'a
// pas forcément à connaître les salaires.
// ---------------------------------------------------------------------------

// libellesSituationEmploye : situation RH, distincte de l'accès applicatif
// (`is_active`). Quelqu'un en congé garde son accès ; quelqu'un qui est parti
// ne l'a plus, mais son historique reste.
var libellesSituationEmploye = map[string]string{
	"active":   "En poste",
	"on_leave": "En congé",
	"departed": "Parti",
}

// libellesModeRemuneration : comment la personne est rémunérée. Aucun calcul de
// paie n'est fait à partir de ces valeurs : elles décrivent un accord, elles ne
// produisent pas un bulletin.
var libellesModeRemuneration = map[string]string{
	"none":     "Aucune commission",
	"per_wash": "Montant par lavage",
	"percent":  "Pourcentage du chiffre d'affaires",
}

// nomSituationEmploye : libellé d'une situation, avec repli lisible.
func nomSituationEmploye(code string) string {
	if libelle, ok := libellesSituationEmploye[code]; ok {
		return libelle
	}
	return code
}
