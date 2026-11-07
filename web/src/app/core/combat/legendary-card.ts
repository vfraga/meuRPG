import {
  type Combatant,
  type Encounter,
  EncounterStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';

/**
 * The master's card of a legendary creature's actions: what is left of the round, the options of its stat block
 * with their cost, and why one is off. The server decides what a spend costs and when it is allowed
 * (`LegendaryState.usable_now`, `LegendaryOption.affordable`); this only phrases it. An option's name is always the
 * Portuguese one (`name_pt`): the English `name` and `text` are never shown in the cards. Only a log line falls back to
 * the English name, when the creature is unknown after a reload. Folding the card is the
 * screen's own business and never calls the server. Pure functions, tested without a DOM.
 */

/** What the row's main button does: spends the cost on a text option, opens the attack sheet, or opens the move. */
export type LegendaryAction = 'spend' | 'attack' | 'move';

/** One option of the stat block. */
export interface LegendaryRow {
  readonly key: string;
  /** The Portuguese name ("Ataque de cauda"). */
  readonly name: string;
  /** "custa 1". */
  readonly costText: string;
  readonly cost: number;
  /** "Usar" (text and move) or "Atacar". */
  readonly buttonLabel: string;
  readonly action: LegendaryAction;
  /** The stat block action an attack option makes (`RollAttack.attack_key`); `''` for the others. */
  readonly attackKey: string;
  /** What the row says under the name when it has something to say: "Você resolve: o app só conta o gasto.". */
  readonly detail: string;
  readonly disabled: boolean;
  /** Why it is off ("Custa 2; restam 1."), or `''`. */
  readonly reason: string;
}

/** The card of one legendary creature. */
export interface LegendaryCard {
  readonly combatantId: string;
  /** The creature's label. */
  readonly title: string;
  /** One mark per action of the round: `true` for one left, `false` for one spent. */
  readonly pips: readonly boolean[];
  readonly spent: number;
  readonly left: number;
  readonly total: number;
  /** "3 de 3". */
  readonly counts: string;
  /** "voltam no começo do turno dele". */
  readonly recovery: string;
  /** "Depois" while a window holds the table or the creature cannot act, else "Opcional". */
  readonly badge: 'Depois' | 'Opcional';
  /** "Use no fim do turno de Tor, uma opção de cada vez.". */
  readonly hint: string;
  /** Whether a reaction window is open: the table waits and the rows are not offered yet. */
  readonly blocked: boolean;
  /** "Depois da janela acima: enquanto uma reação espera, nada anda." while blocked, else `''`. */
  readonly waiting: string;
  /** "Incapacitado: a criatura não pode usar ações lendárias agora." when the server says it cannot spend, else `''`. */
  readonly unusable: string;
  readonly usableNow: boolean;
  readonly rows: readonly LegendaryRow[];
}

/** What the card of the dragon's own turn says instead: "Ações lendárias: 3 de 3, renovadas agora". */
export interface LegendaryRecovered {
  readonly combatantId: string;
  readonly title: string;
  readonly actions: string;
  /** "Resistência Lendária: 2 de 3 hoje", or `''` for a stat block without the trait. */
  readonly resistance: string;
  /** "Ações lendárias se usam no turno de outra criatura, nunca no dele.". */
  readonly note: string;
}

export const LEGENDARY_UNUSABLE = 'Incapacitado: a criatura não pode usar ações lendárias agora.';

const WORDS: Readonly<Record<number, string>> = { 2: 'duas', 3: 'três', 4: 'quatro', 5: 'cinco' };

/** What blocks the table, counted once for the whole queue: the windows the master answers (`ahead`), and whether any
 * window is open at all (`playerWaits`: a player's own question, which holds the table but is not his to answer). */
export interface TableHold {
  readonly ahead: number;
  readonly playerWaits: boolean;
}

export const NO_HOLD: TableHold = { ahead: 0, playerWaits: false };

/** "da janela acima" / "das duas janelas acima": where the windows ahead are. */
export function windowsAbove(n: number): string {
  return n === 1 ? 'da janela acima' : `das ${WORDS[n] ?? n} janelas acima`;
}

/** Why the card waits: behind the master's windows, or on a player's reaction when the master has nothing to answer. */
export function waitingBehind(hold: TableHold): string {
  if (hold.ahead > 0) {
    return `Depois ${windowsAbove(hold.ahead)}: enquanto uma reação espera, nada anda.`;
  }
  return hold.playerWaits ? 'Esperando a reação de um jogador.' : '';
}

/** Why the server says the creature cannot spend now, said for the case: the combat, a fallen creature, or the rest. */
function unusableReason(e: Encounter, c: Combatant): string {
  if (e.status !== EncounterStatus.ACTIVE) {
    return 'O combate não está em andamento.';
  }
  return c.defeated ? 'Caído: a criatura não pode usar ações lendárias agora.' : LEGENDARY_UNUSABLE;
}

function legendaryOf(c: Combatant) {
  const l = c.legendary;
  return l && l.options.length > 0 ? l : null;
}

/** Under the name: the attack's numbers as the server wrote them, or that the master settles a text option himself. */
function detailOf(action: LegendaryAction, attackDetail: string): string {
  if (action === 'attack') {
    return attackDetail;
  }
  return action === 'spend' ? 'Você resolve: o app só conta o gasto.' : '';
}

function rowOf(
  o: NonNullable<Combatant['legendary']>['options'][number],
  left: number,
  unusable: string,
): LegendaryRow {
  const action: LegendaryAction =
    o.kind === 'attack' ? 'attack' : o.kind === 'move' ? 'move' : 'spend';
  return {
    key: o.key,
    // Never `o.name` (English): the server sends the Portuguese one, even for homebrew that has no other.
    name: o.namePt || 'Ação lendária',
    costText: `custa ${o.cost}`,
    cost: o.cost,
    buttonLabel: action === 'attack' ? 'Atacar' : 'Usar',
    action,
    attackKey: action === 'attack' ? o.attackKey : '',
    detail: detailOf(action, o.attackDetailPt),
    // The server says whether the cost fits (`affordable`); the cost is only spelled out when it is the reason.
    disabled: unusable !== '' || !o.affordable,
    reason:
      unusable ||
      (o.affordable
        ? ''
        : o.cost > left
          ? `Custa ${o.cost}; restam ${left}.`
          : 'Indisponível agora.'),
  };
}

/** The card of one legendary creature that does not have the turn, or `null` for any other combatant. */
export function legendaryCard(
  e: Encounter,
  c: Combatant,
  hold: TableHold = NO_HOLD,
): LegendaryCard | null {
  const l = legendaryOf(c);
  if (!l || c.id === e.currentCombatantId) {
    return null;
  }
  const unusable = l.usableNow ? '' : unusableReason(e, c);
  const blocked = hold.ahead > 0 || hold.playerWaits;
  const turn = e.combatants.find((x) => x.id === e.currentCombatantId);
  const left = Math.max(0, Math.min(l.actionsLeft, l.actionsPerRound));
  return {
    combatantId: c.id,
    title: c.label,
    pips: Array.from({ length: l.actionsPerRound }, (_, i) => i < left),
    spent: l.actionsPerRound - left,
    left,
    total: l.actionsPerRound,
    counts: `${left} de ${l.actionsPerRound}`,
    recovery: 'voltam no começo do turno dele',
    badge: blocked || !l.usableNow ? 'Depois' : 'Opcional',
    hint: `Use no fim do turno de ${turn?.label || 'outra criatura'}, uma opção de cada vez.`,
    blocked,
    waiting: waitingBehind(hold),
    unusable,
    usableNow: l.usableNow,
    rows: l.options.map((o) => rowOf(o, left, unusable)),
  };
}

/** The cards of every legendary creature of the combat that may be offered now (none while the combat is not running). */
export function legendaryCards(e: Encounter, hold: TableHold = NO_HOLD): readonly LegendaryCard[] {
  return e.combatants.flatMap((c) => {
    const card = legendaryCard(e, c, hold);
    return card ? [card] : [];
  });
}

/** The folded card: "Ações lendárias · 3 de 3". Folding never calls the server. */
export function legendaryFolded(card: LegendaryCard): string {
  return `Ações lendárias · ${card.counts}`;
}

/** The note of the dragon's own turn (the actions came back, the card is gone), or `null` when the creature is not on turn. */
export function legendaryRecovered(e: Encounter, c: Combatant): LegendaryRecovered | null {
  const l = legendaryOf(c);
  if (!l || c.defeated || c.id !== e.currentCombatantId) {
    return null;
  }
  // What the server says is left now: renewed at the start of its turn, so it is the whole round unless it could not renew.
  const left = Math.max(0, Math.min(l.actionsLeft, l.actionsPerRound));
  return {
    combatantId: c.id,
    title: c.label,
    actions: `Ações lendárias: ${left} de ${l.actionsPerRound}${left === l.actionsPerRound ? ', renovadas agora' : ''}`,
    resistance:
      l.resistancePerDay > 0
        ? `Resistência Lendária: ${l.resistanceLeft} de ${l.resistancePerDay} hoje`
        : '',
    note: 'Ações lendárias se usam no turno de outra criatura, nunca no dele.',
  };
}

/** The options' names of each legendary creature: its id, then the English name of the stat block to the Portuguese one. */
export type LegendaryNames = ReadonlyMap<string, ReadonlyMap<string, string>>;

/** The names of the creatures in the combat now, added to the ones already known: a creature that left the combat keeps
 * its names, so its old log lines still read in Portuguese. */
export function legendaryNames(e: Encounter, known: LegendaryNames = new Map()): LegendaryNames {
  const all = new Map(known);
  for (const c of e.combatants) {
    const names = new Map<string, string>();
    for (const o of c.legendary?.options ?? []) {
      if (o.name && o.namePt) {
        names.set(o.name, o.namePt);
      }
    }
    if (names.size > 0) {
      all.set(c.id, names);
    }
  }
  return all;
}

/** The Portuguese name of the option a log line spent (`CombatLogLegendarySpent.option_name` is the SRD's English): the
 * actor's own option, else the one name every known creature gives it. `null` when it cannot be known (the line would
 * need `CombatLogLegendarySpent.name_pt` from the server). */
export function legendaryOptionName(
  names: LegendaryNames | undefined,
  actorId: string,
  english: string,
): string | null {
  const own = names?.get(actorId)?.get(english);
  if (own) {
    return own;
  }
  const others = new Set(
    [...(names?.values() ?? [])].flatMap((m) => (m.has(english) ? [m.get(english) as string] : [])),
  );
  return others.size === 1 ? [...others][0] : null;
}
