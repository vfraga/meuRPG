import { type ReactionWindow } from '../../../gen/meurpg/play/v1/combat_pb';
import { ofThe } from './move-plan';

/**
 * Legendary Resistance in words: the master's window that offers to turn failed saving throws into successes, and
 * the "Se falhar, usar Resistência Lendária" box of a save the master rolls. The server decides which failures are offered
 * and spends one use per creature named; this phrases the offer. Nothing starts ticked. Pure functions, no DOM.
 */

/** One failed save the master may turn into a success. */
export interface ResistanceOfferRow {
  readonly combatantId: string;
  readonly label: string;
  /** "Dragão vermelho adulto falhou: d20 4 + 6 = 10 contra CD 15". */
  readonly failed: string;
  readonly checkboxLabel: string;
  /** Many creatures: "vira sucesso · gasta 1 · restam 3 de 3 hoje". One creature: "Resistência Lendária: restam 3 de 3 hoje · usar gasta 1". */
  readonly line: string;
  readonly checked: false;
}

export interface LegendaryResistanceCard {
  readonly id: string;
  readonly overline: string;
  /** "Transformar a falha em sucesso?" (one creature) or "Transformar falhas em sucesso?". */
  readonly title: string;
  /** "Relâmpago de Pensantus · teste de resistência, CD 15. A magia espera a sua resposta; os dados já foram rolados.". */
  readonly source: string;
  /** `one`: a single failing creature, no checkbox, the primary button uses it. `many`: a checkbox per creature. */
  readonly mode: 'one' | 'many';
  readonly rows: readonly ResistanceOfferRow[];
  /** First, and the default: every save stays as it was rolled. */
  readonly declineLabel: string;
  /** "Usar Resistência Lendária" for one creature, "Usar nas marcadas" for many. */
  readonly confirmLabel: string;
  readonly footer: string;
}

const MINUS = '−';

/** "d20 4 + 6 = 10", or "d20 4, contou como 8 + 2 = 10" when a table's minimum raised the d20. */
function rollText(d20: number, total: number, treatedAs: number | undefined): string {
  const counted = treatedAs ?? d20;
  const mod = total - counted;
  const sum = `${mod < 0 ? MINUS : '+'} ${Math.abs(mod)} = ${total}`;
  return treatedAs === undefined || treatedAs === d20
    ? `d20 ${d20} ${sum}`
    : `d20 ${d20}, contou como ${treatedAs} ${sum}`;
}

function sourceLine(w: ReactionWindow, actorIsPlayer: boolean, dcs: readonly number[]): string {
  const t = w.trigger;
  const same = dcs.length > 0 && dcs.every((d) => d === dcs[0]);
  const save = same ? `teste de resistência, CD ${dcs[0]}` : 'teste de resistência';
  const wait = 'espera a sua resposta; os dados já foram rolados.';
  if (!t?.actionNamePt) {
    return `${save[0].toUpperCase()}${save.slice(1)}. A ação ${wait}`;
  }
  // Only a spell cast carries the spell's key; a rider's save (Golpe Atordoante) is an action, not a magic.
  const what = t.spellKey ? 'A magia' : 'A ação';
  const by = t.actorLabel ? ` ${actorIsPlayer ? `de ${t.actorLabel}` : ofThe([t.actorLabel])}` : '';
  return `${t.actionNamePt}${by} · ${save}. ${what} ${wait}`;
}

/** The card of a window of kind LEGENDARY_RESISTANCE, or `null` for a window without its prompt. `actorIsPlayer`: the
 * trigger's actor is a player's character (it takes no article). */
export function legendaryResistanceCard(
  w: ReactionWindow,
  actorIsPlayer = false,
): LegendaryResistanceCard | null {
  if (w.prompt.case !== 'legendaryResistance') {
    return null;
  }
  const offers = w.prompt.value.offers;
  const one = offers.length === 1;
  return {
    id: w.id,
    overline: 'Resistência Lendária',
    title: one ? 'Transformar a falha em sucesso?' : 'Transformar falhas em sucesso?',
    mode: one ? 'one' : 'many',
    source: sourceLine(
      w,
      actorIsPlayer,
      offers.map((o) => o.dc),
    ),
    rows: offers.map((o) => ({
      combatantId: o.combatantId,
      label: o.label,
      failed: `${o.label} falhou${o.d20 === 0 ? ' sem rolar' : `: ${rollText(o.d20, o.total, o.treatedAs)}`} contra CD ${o.dc}`,
      checkboxLabel: 'Usar Resistência Lendária',
      line: one
        ? `Resistência Lendária: restam ${o.usesLeft} de ${o.usesPerDay} hoje · usar gasta 1`
        : `vira sucesso · gasta 1 · restam ${o.usesLeft} de ${o.usesPerDay} hoje`,
      checked: false as const,
    })),
    declineLabel: 'Deixar como rolou',
    confirmLabel: one ? 'Usar Resistência Lendária' : 'Usar nas marcadas',
    footer: 'Os jogadores veem “Esperando o mestre” e, depois, o resultado do teste.',
  };
}

/** The creatures to send with the primary button: the only creature of a single window, or the ticked ones that the
 * window offers, in the window's order. */
export function resistanceIds(
  card: LegendaryResistanceCard,
  ticked: ReadonlySet<string>,
): readonly string[] {
  if (card.mode === 'one') {
    return card.rows.map((r) => r.combatantId);
  }
  return card.rows.filter((r) => ticked.has(r.combatantId)).map((r) => r.combatantId);
}

/** The primary button works for a single creature; with many it is offered only with something ticked. "Deixar como rolou" always. */
export function canConfirmResistance(card: LegendaryResistanceCard, ticked: ReadonlySet<string>) {
  return resistanceIds(card, ticked).length > 0;
}

/** The box on a save the master rolls for an NPC: "Se falhar, usar Resistência Lendária". */
export interface ResistanceBox {
  readonly label: string;
  /** "gasta 1 · restam 3 de 3 hoje". */
  readonly line: string;
  readonly checked: false;
}

/** The box for a combatant's save, or `null` unless its stat block has the trait and a use is left. */
export function resistanceBox(
  legendary: { readonly resistanceLeft: number; readonly resistancePerDay: number } | undefined,
): ResistanceBox | null {
  if (!legendary || legendary.resistanceLeft <= 0) {
    return null;
  }
  return {
    label: 'Se falhar, usar Resistência Lendária',
    line: `gasta 1 · restam ${legendary.resistanceLeft} de ${legendary.resistancePerDay} hoje`,
    checked: false,
  };
}
