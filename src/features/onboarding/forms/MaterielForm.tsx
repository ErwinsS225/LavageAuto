/**
 * Étape 12 du parcours : matériel et stock.
 *
 * Deux choses que l'application ne savait pas dire : **ce que la station
 * possède** (jets, aspirateurs, groupes, fosses…) et **ce qu'elle consomme**
 * (produits, consommables). Les deux sont facultatives à l'activation, mais un
 * stock décrit évite de découvrir une rupture en pleine journée.
 *
 * Le point important est ailleurs : **« je ne sais pas encore » n'est pas
 * « zéro »**. Un article dont la quantité n'a jamais été comptée reste marqué
 * « non renseigné » — aucune alerte de rupture n'est affichée, et un mouvement
 * de sortie est refusé tant que personne n'a compté. Une quantité se corrige
 * par un mouvement (inventaire, entrée, perte motivée), jamais à la main :
 * chaque variation de stock laisse une trace lisible.
 */
import { useState } from 'react'
import { CircleAlert, PackagePlus, Plus, Wrench } from 'lucide-react'

import { useToast } from '@/components/Toaster'
import {
  Badge,
  Button,
  Card,
  CardHeader,
  Field,
  Input,
  Modal,
  Select,
  SkeletonRows,
  Textarea,
} from '@/components/ui'
import {
  useCreateEquipment,
  useCreateStockItem,
  useCreateStockMovement,
  useEquipment,
  useStock,
  useUpdateEquipment,
  useUpdateStockItem,
} from '@/features/maintenance/api'
import { ApiError } from '@/lib/api'
import { formatXOF } from '@/lib/format'
import type { EquipmentState, StockItem, StockMovementKind } from '@/types'

const ETATS: Array<{ value: EquipmentState; label: string }> = [
  { value: 'neuf', label: 'Neuf' },
  { value: 'bon', label: 'Bon état' },
  { value: 'a_reparer', label: 'À réparer' },
  { value: 'hors_service', label: 'Hors service' },
]

const MOUVEMENTS: Array<{ value: StockMovementKind; label: string; hint: string }> = [
  { value: 'inventaire', label: 'Comptage (inventaire)', hint: 'Vous avez compté : indiquez la quantité trouvée.' },
  { value: 'entree', label: 'Entrée (réception)', hint: 'Ce qui arrive dans la station.' },
  { value: 'sortie', label: 'Sortie (consommation)', hint: 'Ce qui a été utilisé.' },
  { value: 'perte', label: 'Perte (casse, vol, péremption)', hint: 'Un motif est obligatoire.' },
  { value: 'ajustement', label: 'Ajustement justifié', hint: 'Un motif est obligatoire.' },
]

/** Matériel : le parc de la station, sans rien inventer. */
function Materiel({ stationId }: { stationId: string }) {
  const toast = useToast()
  const requete = useEquipment(stationId)
  const creation = useCreateEquipment(stationId)
  const miseAJour = useUpdateEquipment(stationId)
  const [ouvert, setOuvert] = useState(false)
  const [modele, setModele] = useState('')
  const [nomLibre, setNomLibre] = useState('')
  const [quantite, setQuantite] = useState('1')
  const [etat, setEtat] = useState<EquipmentState>('bon')
  const [achete, setAchete] = useState('')
  const [cout, setCout] = useState('')
  const [note, setNote] = useState('')

  async function ajouter() {
    try {
      await creation.mutateAsync({
        template_code: modele || undefined,
        name: modele ? undefined : nomLibre,
        quantity: quantite === '' ? 1 : Number(quantite),
        state: etat,
        acquired_on: achete,
        cost_xof: cout === '' ? 0 : Number(cout),
        notes: note,
      })
      toast.success('Matériel ajouté.')
      setOuvert(false)
      setModele('')
      setNomLibre('')
      setQuantite('1')
      setAchete('')
      setCout('')
      setNote('')
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'ajout n'a pas abouti.")
    }
  }

  async function retirer(id: string, actif: boolean) {
    try {
      await miseAJour.mutateAsync({ id, is_active: actif })
      toast.success(actif ? 'Matériel remis en service.' : 'Matériel retiré du parc (historique conservé).')
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'enregistrement n'a pas abouti.")
    }
  }

  const donnees = requete.data
  if (!donnees) {
    return (
      <Card>
        <CardHeader
          title="Matériel de la station"
          subtitle="Un matériel retiré n'est jamais supprimé : ses dates et son coût restent consultables."
        />
        <div className="space-y-3 p-4">
          {requete.isLoading ? (
            <SkeletonRows rows={3} />
          ) : (
            <>
              {/* Une lecture qui échoue se dit : un écran vide laisserait croire
                  que la station n'a aucun matériel. */}
              <p className="text-sm text-ink-muted">
                Le parc de la station n'a pas pu être lu depuis le serveur.
              </p>
              <Button variant="secondary" onClick={() => void requete.refetch()}>
                Réessayer
              </Button>
            </>
          )}
        </div>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader
        title="Matériel de la station"
        subtitle="Un matériel retiré n'est jamais supprimé : ses dates et son coût restent consultables."
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone="neutral">{donnees.summary.en_service} en service</Badge>
            {donnees.summary.a_reparer > 0 ? (
              <Badge tone="warning">{donnees.summary.a_reparer} à réparer</Badge>
            ) : null}
            {donnees.summary.maintenance_a_prevoir > 0 ? (
              <Badge tone="warning">
                {donnees.summary.maintenance_a_prevoir} maintenance(s) à prévoir
              </Badge>
            ) : null}
            <Button icon={<Plus size={15} />} onClick={() => setOuvert(true)}>
              Ajouter
            </Button>
          </div>
        }
      />
      <div className="space-y-2 p-4">
        {donnees.items.length === 0 ? (
          <p className="text-sm text-ink-muted">
            Aucun matériel déclaré. Le référentiel propose les équipements courants — un jet, un
            aspirateur, une fosse — et accepte aussi un matériel qui n'y figure pas.
          </p>
        ) : null}
        {donnees.items.map((materiel) => (
          <div
            key={materiel.id}
            className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3"
          >
            <div className="flex flex-wrap items-center gap-2">
              <Wrench size={15} className="text-ink-subtle" aria-hidden />
              <span className={materiel.is_active ? 'font-medium' : 'font-medium text-ink-muted line-through'}>
                {materiel.name}
              </span>
              <Badge tone="neutral">{materiel.quantity}</Badge>
              <Badge tone={materiel.state === 'bon' || materiel.state === 'neuf' ? 'success' : 'warning'}>
                {materiel.state_label}
              </Badge>
              {materiel.service_due ? <Badge tone="warning">Maintenance à faire</Badge> : null}
            </div>
            <div className="flex flex-wrap items-center gap-2 text-xs text-ink-muted">
              <span>{materiel.family_label}</span>
              <span>{materiel.acquired_on ? `acheté le ${materiel.acquired_on}` : 'date d’achat non renseignée'}</span>
              <span>{materiel.cost_xof > 0 ? formatXOF(materiel.cost_xof) : 'coût non renseigné'}</span>
              <Button variant="ghost" onClick={() => void retirer(materiel.id, !materiel.is_active)}>
                {materiel.is_active ? 'Retirer' : 'Remettre en service'}
              </Button>
            </div>
          </div>
        ))}
      </div>

      <Modal
        open={ouvert}
        title="Ajouter un matériel"
        onClose={() => setOuvert(false)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setOuvert(false)}>
              Annuler
            </Button>
            <Button
              onClick={() => void ajouter()}
              disabled={creation.isPending || (modele === '' && nomLibre.trim() === '')}
            >
              Ajouter
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          <Field label="Modèle" hint="Laissez vide pour un matériel qui n'est pas dans la liste.">
            {(id) => (
              <Select
                id={id}
                value={modele}
                placeholder="— choisir un modèle —"
                onChange={(event) => setModele(event.target.value)}
                options={donnees.templates.map((modeleListe) => ({
                  value: modeleListe.code,
                  label: `${modeleListe.name} · ${modeleListe.family_label}`,
                }))}
              />
            )}
          </Field>
          {modele === '' ? (
            <Field label="Nom du matériel" required>
              {(id) => (
                <Input id={id} value={nomLibre} onChange={(event) => setNomLibre(event.target.value)} />
              )}
            </Field>
          ) : null}
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Quantité">
              {(id) => (
                <Input
                  id={id}
                  type="number"
                  min={0}
                  value={quantite}
                  onChange={(event) => setQuantite(event.target.value)}
                />
              )}
            </Field>
            <Field label="État">
              {(id) => (
                <Select
                  id={id}
                  value={etat}
                  onChange={(event) => setEtat(event.target.value as EquipmentState)}
                  options={ETATS}
                />
              )}
            </Field>
            <Field label="Date d'achat (facultatif)">
              {(id) => (
                <Input id={id} type="date" value={achete} onChange={(event) => setAchete(event.target.value)} />
              )}
            </Field>
            <Field label="Coût d'achat (facultatif)">
              {(id) => (
                <Input
                  id={id}
                  type="number"
                  min={0}
                  value={cout}
                  onChange={(event) => setCout(event.target.value)}
                  placeholder="FCFA"
                />
              )}
            </Field>
          </div>
          <Field label="Observation">
            {(id) => (
              <Textarea id={id} value={note} onChange={(event) => setNote(event.target.value)} />
            )}
          </Field>
        </div>
      </Modal>
    </Card>
  )
}

/** Un article de stock : quantité, seuil, et l'état qui en découle. */
function LigneArticle({ stationId, article }: { stationId: string; article: StockItem }) {
  const toast = useToast()
  const mouvement = useCreateStockMovement(stationId)
  const miseAJour = useUpdateStockItem(stationId)
  const [ouvert, setOuvert] = useState(false)
  const [type, setType] = useState<StockMovementKind>('inventaire')
  const [quantite, setQuantite] = useState('')
  const [motif, setMotif] = useState('')

  const ton =
    article.state === 'ok'
      ? 'success'
      : article.state === 'inconnu'
        ? 'neutral'
        : 'warning'

  async function enregistrer() {
    try {
      const reponse = await mouvement.mutateAsync({
        itemId: article.id,
        kind: type,
        quantity: quantite === '' ? 0 : Number(quantite),
        reason: motif,
      })
      toast.success(
        reponse.quantity_known
          ? `Mouvement enregistré : ${reponse.quantity_after} ${article.unit} en stock.`
          : 'Mouvement enregistré : quantité toujours non renseignée.',
      )
      setOuvert(false)
      setQuantite('')
      setMotif('')
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "Le mouvement n'a pas abouti.")
    }
  }

  async function declarerInconnu() {
    try {
      await miseAJour.mutateAsync({ id: article.id, quantity_known: false })
      toast.success('Article marqué « quantité non renseignée ».')
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'enregistrement n'a pas abouti.")
    }
  }

  const choix = MOUVEMENTS.find((item) => item.value === type)

  return (
    <div className="space-y-2 rounded-xl border border-line p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{article.name}</span>
          <Badge tone="neutral">{article.category_name}</Badge>
          <Badge tone={ton}>{article.state_label}</Badge>
          {article.expires_on ? (
            <span className="text-xs text-ink-muted">péremption {article.expires_on}</span>
          ) : null}
        </div>
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <span className="tabular">
            {article.quantity_known
              ? `${article.quantity} ${article.unit}`
              : `quantité non renseignée (${article.unit})`}
          </span>
          {article.min_quantity ? (
            <span className="text-xs text-ink-muted">seuil {article.min_quantity}</span>
          ) : null}
          <Button variant="secondary" onClick={() => setOuvert(true)}>
            Mouvement
          </Button>
          {article.quantity_known ? (
            <Button variant="ghost" onClick={() => void declarerInconnu()}>
              Marquer « non compté »
            </Button>
          ) : null}
        </div>
      </div>
      {!article.quantity_known ? (
        <p className="flex items-center gap-2 text-xs text-ink-muted">
          <CircleAlert size={13} aria-hidden />
          Personne n'a compté cet article : ce n'est pas zéro, et aucune alerte de rupture n'est
          affichée tant qu'un inventaire n'a pas été fait.
        </p>
      ) : null}

      <Modal
        open={ouvert}
        title={`Mouvement de stock — ${article.name}`}
        onClose={() => setOuvert(false)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setOuvert(false)}>
              Annuler
            </Button>
            <Button
              onClick={() => void enregistrer()}
              disabled={mouvement.isPending || quantite === ''}
            >
              Enregistrer le mouvement
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          <Field label="Type de mouvement" hint={choix?.hint}>
            {(id) => (
              <Select
                id={id}
                value={type}
                onChange={(event) => setType(event.target.value as StockMovementKind)}
                options={MOUVEMENTS.map((item) => ({ value: item.value, label: item.label }))}
              />
            )}
          </Field>
          <Field
            label={type === 'inventaire' ? `Quantité comptée (${article.unit})` : `Quantité (${article.unit})`}
            required
          >
            {(id) => (
              <Input
                id={id}
                type="number"
                min={0}
                value={quantite}
                onChange={(event) => setQuantite(event.target.value)}
              />
            )}
          </Field>
          <Field label="Motif" hint="Obligatoire pour une perte ou un ajustement.">
            {(id) => (
              <Input id={id} value={motif} onChange={(event) => setMotif(event.target.value)} />
            )}
          </Field>
        </div>
      </Modal>
    </div>
  )
}

/** Stock : ce que la station consomme, et ce qu'on en sait vraiment. */
function Stock({ stationId }: { stationId: string }) {
  const toast = useToast()
  const requete = useStock(stationId)
  const creation = useCreateStockItem(stationId)
  const [ouvert, setOuvert] = useState(false)
  const [nom, setNom] = useState('')
  const [categorie, setCategorie] = useState('')
  const [unite, setUnite] = useState('')
  const [quantite, setQuantite] = useState('')
  const [cout, setCout] = useState('')
  const [seuil, setSeuil] = useState('')

  async function ajouter() {
    try {
      await creation.mutateAsync({
        name: nom,
        category_code: categorie || undefined,
        unit: unite || undefined,
        // Champ laissé vide : aucune quantité n'est inventée, l'article est
        // simplement déclaré « non compté ».
        quantity: quantite === '' ? undefined : Number(quantite),
        unit_cost_xof: cout === '' ? undefined : Number(cout),
        min_quantity: seuil === '' ? undefined : Number(seuil),
      })
      toast.success(
        quantite === ''
          ? 'Article ajouté, quantité non renseignée : vous la comptabiliserez quand vous voudrez.'
          : 'Article ajouté.',
      )
      setOuvert(false)
      setNom('')
      setQuantite('')
      setCout('')
      setSeuil('')
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'ajout n'a pas abouti.")
    }
  }

  const donnees = requete.data
  if (!donnees) {
    return (
      <Card>
        <CardHeader
          title="Stock et consommables"
          subtitle="Vous pouvez répondre « je ne sais pas encore » : l'article est enregistré sans quantité, et aucune rupture n'est annoncée à tort."
        />
        <div className="space-y-3 p-4">
          {requete.isLoading ? (
            <SkeletonRows rows={3} />
          ) : (
            <>
              <p className="text-sm text-ink-muted">
                Le stock n'a pas pu être lu depuis le serveur.
              </p>
              <Button variant="secondary" onClick={() => void requete.refetch()}>
                Réessayer
              </Button>
            </>
          )}
        </div>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader
        title="Stock et consommables"
        subtitle="Vous pouvez répondre « je ne sais pas encore » : l'article est enregistré sans quantité, et aucune rupture n'est annoncée à tort."
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone="neutral">{donnees.summary.articles} article(s)</Badge>
            {donnees.summary.quantite_inconnue > 0 ? (
              <Badge tone="neutral">{donnees.summary.quantite_inconnue} non compté(s)</Badge>
            ) : null}
            {donnees.summary.rupture > 0 ? (
              <Badge tone="danger">{donnees.summary.rupture} en rupture</Badge>
            ) : null}
            {donnees.summary.sous_seuil > 0 ? (
              <Badge tone="warning">{donnees.summary.sous_seuil} sous le seuil</Badge>
            ) : null}
            {donnees.summary.valeur_xof > 0 ? (
              <Badge tone="neutral">valeur connue {formatXOF(donnees.summary.valeur_xof)}</Badge>
            ) : null}
            <Button icon={<PackagePlus size={15} />} onClick={() => setOuvert(true)}>
              Ajouter un article
            </Button>
          </div>
        }
      />
      <div className="space-y-2 p-4">
        {donnees.items.length === 0 ? (
          <p className="text-sm text-ink-muted">
            Aucun article déclaré. Les catégories sont déjà en place (produits de lavage, finition,
            consommables, carburant…).
          </p>
        ) : null}
        {donnees.items.map((article) => (
          <LigneArticle key={article.id} stationId={stationId} article={article} />
        ))}
      </div>

      <Modal
        open={ouvert}
        title="Ajouter un article de stock"
        onClose={() => setOuvert(false)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setOuvert(false)}>
              Annuler
            </Button>
            <Button onClick={() => void ajouter()} disabled={creation.isPending || nom.trim() === ''}>
              Ajouter
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          <Field label="Nom de l'article" required>
            {(id) => <Input id={id} value={nom} onChange={(event) => setNom(event.target.value)} />}
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Catégorie">
              {(id) => (
                <Select
                  id={id}
                  value={categorie}
                  placeholder="— choisir —"
                  onChange={(event) => setCategorie(event.target.value)}
                  options={donnees.categories.map((item) => ({ value: item.code, label: item.name }))}
                />
              )}
            </Field>
            <Field label="Unité">
              {(id) => (
                <Input
                  id={id}
                  value={unite}
                  placeholder="bidon, litre, pièce…"
                  onChange={(event) => setUnite(event.target.value)}
                />
              )}
            </Field>
            <Field label="Quantité" hint="Laissez vide si vous ne savez pas encore.">
              {(id) => (
                <Input
                  id={id}
                  type="number"
                  min={0}
                  value={quantite}
                  onChange={(event) => setQuantite(event.target.value)}
                />
              )}
            </Field>
            <Field label="Seuil d'alerte" hint="Sans quantité connue, le seuil ne déclenche rien.">
              {(id) => (
                <Input
                  id={id}
                  type="number"
                  min={0}
                  value={seuil}
                  onChange={(event) => setSeuil(event.target.value)}
                />
              )}
            </Field>
            <Field label="Coût unitaire">
              {(id) => (
                <Input
                  id={id}
                  type="number"
                  min={0}
                  value={cout}
                  placeholder="FCFA"
                  onChange={(event) => setCout(event.target.value)}
                />
              )}
            </Field>
          </div>
        </div>
      </Modal>
    </Card>
  )
}

export default function MaterielForm({ stationId }: { stationId: string }) {
  return (
    <div className="space-y-4">
      <Materiel stationId={stationId} />
      <Stock stationId={stationId} />
    </div>
  )
}
