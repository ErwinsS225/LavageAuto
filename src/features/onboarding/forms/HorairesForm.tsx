/**
 * Étape « horaires » : les sept jours, plus les journées particulières.
 *
 * Les horaires ne sont pas décoratifs : c'est ce qui permet à l'application de
 * dire si la station est ouverte. Ils sont donc validés côté serveur, et cet
 * écran explique les refus plutôt que de les subir (le message d'erreur arrive
 * par jour).
 */
import { CalendarOff, Copy, Plus, Trash2 } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'

import { Button, Card, CardHeader, Input, Switch } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useSaveOpeningHours, useStationProfile } from '@/features/profile/api'
import { ApiError } from '@/lib/api'
import type { OpeningException, OpeningHour } from '@/types'

const JOURS = ['lundi', 'mardi', 'mercredi', 'jeudi', 'vendredi', 'samedi', 'dimanche']

export default function HorairesForm({
  stationId,
  onEnregistre,
}: {
  stationId: string
  onEnregistre: () => void
}) {
  const toast = useToast()
  const profil = useStationProfile(stationId)
  const enregistrement = useSaveOpeningHours(stationId)

  const [jours, setJours] = useState<OpeningHour[]>([])
  const [exceptions, setExceptions] = useState<OpeningException[]>([])
  const [erreurs, setErreurs] = useState<Record<string, string>>({})
  const [message, setMessage] = useState('')

  useEffect(() => {
    if (profil.data?.days) {
      // Les sept jours existent toujours côté serveur (un jour non décrit est
      // renvoyé fermé) : l'écran ne montre donc jamais une semaine trouée.
      setJours(profil.data.days.map((jour) => ({ ...jour })))
    }
    if (profil.data?.exceptions) setExceptions(profil.data.exceptions.map((item) => ({ ...item })))
  }, [profil.data])

  function majJour(weekday: number, changements: Partial<OpeningHour>) {
    setJours((actuel) =>
      actuel.map((jour) => (jour.weekday === weekday ? { ...jour, ...changements } : jour)),
    )
  }

  /** Recopie le lundi sur les autres jours : ce que fait tout gérant pressé. */
  function appliquerATous() {
    const modele = jours.find((jour) => jour.weekday === 1)
    if (!modele) return
    setJours((actuel) => actuel.map((jour) => ({ ...modele, weekday: jour.weekday })))
    toast.info('Horaires du lundi appliqués à toute la semaine.')
  }

  async function enregistrer(event: FormEvent) {
    event.preventDefault()
    setErreurs({})
    setMessage('')
    try {
      const reponse = await enregistrement.mutateAsync({ days: jours, exceptions })
      setJours(reponse.days.map((jour) => ({ ...jour })))
      setExceptions(reponse.exceptions.map((item) => ({ ...item })))
      toast.success('Horaires enregistrés.')
      onEnregistre()
    } catch (caught) {
      if (caught instanceof ApiError) {
        setErreurs(caught.fields)
        setMessage(caught.message)
      } else {
        setMessage("L'enregistrement a échoué. Vérifiez votre connexion puis réessayez.")
      }
    }
  }

  if (profil.isLoading) {
    return <p className="text-sm text-ink-muted">Chargement des horaires…</p>
  }
  if (profil.isError) {
    return (
      <Card>
        <CardHeader title="Horaires indisponibles" subtitle="La station n'a pas pu être lue." />
        <div className="p-4">
          <Button variant="secondary" onClick={() => void profil.refetch()}>
            Réessayer
          </Button>
        </div>
      </Card>
    )
  }

  const nombreOuverts = jours.filter((jour) => jour.is_open).length

  return (
    <form className="space-y-5" onSubmit={enregistrer} noValidate>
      <Card>
        <CardHeader
          title="Semaine type"
          subtitle="Un jour non coché est un jour fermé. Laissez les heures vides pour un jour fermé."
          actions={
            <Button type="button" variant="ghost" size="sm" icon={<Copy size={14} />} onClick={appliquerATous}>
              Appliquer le lundi à tous
            </Button>
          }
        />
        <div className="space-y-3 p-4">
          {jours.map((jour) => {
            const nom = JOURS[jour.weekday - 1]
            return (
              <div key={jour.weekday} className="rounded-field border border-line px-3 py-3">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="flex items-center gap-3">
                    <span className="w-20 text-sm font-medium capitalize text-ink">{nom}</span>
                    <Switch
                      checked={jour.is_open}
                      onChange={(ouvert) =>
                        majJour(jour.weekday, {
                          is_open: ouvert,
                          // Un jour fermé n'emporte pas d'horaires : on les vide
                          // pour que l'enregistrement soit cohérent.
                          ...(ouvert
                            ? {}
                            : { opens_at: '', closes_at: '', break_start: '', break_end: '', last_checkin_at: '' }),
                        })
                      }
                      label={jour.is_open ? 'Ouvert' : 'Fermé'}
                    />
                  </div>
                  <div className="flex flex-wrap items-center gap-2 text-xs text-ink-muted">
                    {jour.is_open ? (
                      <>
                        <label className="flex items-center gap-1.5">
                          De
                          <Input
                            type="time"
                            value={jour.opens_at}
                            onChange={(event) => majJour(jour.weekday, { opens_at: event.target.value })}
                            className="h-8 w-28"
                            invalid={Boolean(erreurs[nom])}
                          />
                        </label>
                        <label className="flex items-center gap-1.5">
                          à
                          <Input
                            type="time"
                            value={jour.closes_at}
                            onChange={(event) => majJour(jour.weekday, { closes_at: event.target.value })}
                            className="h-8 w-28"
                            invalid={Boolean(erreurs[nom])}
                          />
                        </label>
                      </>
                    ) : (
                      <span>Aucun véhicule reçu ce jour-là.</span>
                    )}
                  </div>
                </div>

                {jour.is_open && (
                  <div className="mt-3 flex flex-wrap items-end gap-3 border-t border-line/60 pt-3 text-xs text-ink-muted">
                    <label className="flex items-center gap-1.5">
                      Pause de
                      <Input
                        type="time"
                        value={jour.break_start}
                        onChange={(event) => majJour(jour.weekday, { break_start: event.target.value })}
                        className="h-8 w-28"
                      />
                      à
                      <Input
                        type="time"
                        value={jour.break_end}
                        onChange={(event) => majJour(jour.weekday, { break_end: event.target.value })}
                        className="h-8 w-28"
                      />
                    </label>
                    <label className="flex items-center gap-1.5">
                      Dernière réception
                      <Input
                        type="time"
                        value={jour.last_checkin_at}
                        onChange={(event) => majJour(jour.weekday, { last_checkin_at: event.target.value })}
                        className="h-8 w-28"
                      />
                    </label>
                    <label className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        checked={jour.booking_allowed}
                        onChange={(event) => majJour(jour.weekday, { booking_allowed: event.target.checked })}
                        className="h-4 w-4 accent-[var(--brand)]"
                      />
                      Réservations acceptées
                    </label>
                  </div>
                )}

                {erreurs[nom] ? <p className="mt-2 text-xs text-danger">{erreurs[nom]}</p> : null}
              </div>
            )
          })}
        </div>
        <p className="border-t border-line px-4 py-3 text-xs text-ink-subtle">
          {nombreOuverts === 0
            ? "Aucun jour ouvert : la station sera considérée comme fermée en permanence."
            : `${nombreOuverts} jour(s) d'ouverture déclaré(s). La pause, si elle est renseignée, sort du temps de travail compté.`}
        </p>
      </Card>

      <Card>
        <CardHeader
          title="Journées particulières"
          subtitle="Fermeture exceptionnelle, horaires réduits : une seule règle par journée."
          actions={
            <Button
              type="button"
              variant="ghost"
              size="sm"
              icon={<Plus size={14} />}
              onClick={() =>
                setExceptions((actuel) => [
                  ...actuel,
                  { day: '', is_open: false, opens_at: '', closes_at: '', label: '' },
                ])
              }
            >
              Ajouter une journée
            </Button>
          }
        />
        <div className="space-y-3 p-4">
          {exceptions.length === 0 ? (
            <p className="flex items-center gap-2 text-sm text-ink-muted">
              <CalendarOff size={16} className="text-ink-subtle" aria-hidden />
              Aucune journée particulière déclarée. Vous pourrez en ajouter à tout moment (fériés,
              inventaire, travaux).
            </p>
          ) : (
            exceptions.map((exception, index) => (
              <div key={index} className="flex flex-wrap items-end gap-3 rounded-field border border-line px-3 py-3">
                <label className="text-xs text-ink-muted">
                  Date
                  <Input
                    type="date"
                    value={exception.day}
                    onChange={(event) =>
                      setExceptions((actuel) =>
                        actuel.map((item, position) =>
                          position === index ? { ...item, day: event.target.value } : item,
                        ),
                      )
                    }
                    className="mt-1 h-9"
                    invalid={Boolean(erreurs[exception.day])}
                  />
                </label>
                <label className="text-xs text-ink-muted">
                  Motif
                  <Input
                    value={exception.label}
                    onChange={(event) =>
                      setExceptions((actuel) =>
                        actuel.map((item, position) =>
                          position === index ? { ...item, label: event.target.value } : item,
                        ),
                      )
                    }
                    placeholder="Ex : fête nationale"
                    className="mt-1 h-9"
                  />
                </label>
                <div className="pb-1">
                  <Switch
                    checked={exception.is_open}
                    onChange={(ouvert) =>
                      setExceptions((actuel) =>
                        actuel.map((item, position) =>
                          position === index
                            ? { ...item, is_open: ouvert, ...(ouvert ? {} : { opens_at: '', closes_at: '' }) }
                            : item,
                        ),
                      )
                    }
                    label={exception.is_open ? 'Ouvert' : 'Fermé'}
                  />
                </div>
                {exception.is_open && (
                  <div className="flex items-center gap-2 text-xs text-ink-muted">
                    <Input
                      type="time"
                      value={exception.opens_at}
                      onChange={(event) =>
                        setExceptions((actuel) =>
                          actuel.map((item, position) =>
                            position === index ? { ...item, opens_at: event.target.value } : item,
                          ),
                        )
                      }
                      className="h-9 w-28"
                    />
                    <Input
                      type="time"
                      value={exception.closes_at}
                      onChange={(event) =>
                        setExceptions((actuel) =>
                          actuel.map((item, position) =>
                            position === index ? { ...item, closes_at: event.target.value } : item,
                          ),
                        )
                      }
                      className="h-9 w-28"
                    />
                  </div>
                )}
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  icon={<Trash2 size={14} />}
                  onClick={() => setExceptions((actuel) => actuel.filter((_, position) => position !== index))}
                >
                  Retirer
                </Button>
                {erreurs[exception.day] ? (
                  <p className="w-full text-xs text-danger">{erreurs[exception.day]}</p>
                ) : null}
              </div>
            ))
          )}
        </div>
      </Card>

      {message ? (
        <p className="rounded-field border border-danger/30 bg-danger-soft px-3 py-2 text-sm text-danger">{message}</p>
      ) : null}

      <div>
        <Button type="submit" variant="primary" loading={enregistrement.isPending}>
          Enregistrer les horaires
        </Button>
      </div>
    </form>
  )
}
