import {
  type Encounter,
  ReactionKind,
  type ReactionWindow,
} from '../../../gen/meurpg/play/v1/combat_pb';
import type { SlotChoice } from '../../../gen/meurpg/rules/v1/rules_pb';
import { joinDots } from '../format/text';
import { metersText } from '../units';
import { circleLabel, freeLabel, slotTitle } from './combat-grid';
import { article } from '../format/article';
import { ofThe } from './move-plan';
import { combatantIsPlayer, ofReactor } from './reaction-bar';
import { type ParryCard, parryCard } from './parry-card';
import { type LegendaryResistanceCard, legendaryResistanceCard } from './legendary-resistance-card';
import { reactionKindName } from './reaction-record';
import { REACTION_NAMES, openWindows } from './reactions';

export { heldReason, windowsBarText } from './reaction-bar';

/**
 * The master's side of the reaction windows (PM-04): every open window, grouped by the action that provoked it, in
 * the order the server answers them, with the numbers that are the master's (`ReactionWindow.trigger`). The server
 * decides the order, who may answer and what each answer does; this only phrases it. A pronoun is never guessed from a
 * name: a player's window reads "pelo jogador". Pure functions, tested without a DOM.
 */

/** The name of the reaction a window offers, as `names_pt.json` writes it. */
export function reactionName(w: ReactionWindow): string {
  return reactionKindName(w.kind);
}

/** The Shield spell's bonus to armor class (SRD 5.1). */
const SHIELD_BONUS = 5;

/** How long a player has to answer before the question comes to the master (the server's number, said in the card). */
const LATE_SECONDS = 10;

/** One answer the master gives for an NPC's counterspell: the slot and what it does ("anula sem teste"). */
export interface SlotOption {
  readonly slot: SlotChoice;
  readonly title: string;
  readonly effect: string;
}

/** One row of a queue: a reactor and what it may do. */
export interface QueueRow {
  readonly window: ReactionWindow;
  readonly label: string;
  readonly isPlayer: boolean;
  /** "Jogador" or "NPC". */
  readonly tag: string;
  readonly description: string;
  /** "Respondendo no celular · vez de responder", "Depois do Mago 1", or `''`. */
  readonly status: string;
  readonly answerNow: boolean;
  readonly useLabel: string;
  readonly passLabel: string;
  readonly slots: readonly SlotOption[];
}

/** A card of the master's side. */
export type ReactionCard =
  | {
      readonly type: 'queue';
      readonly id: string;
      readonly title: string;
      readonly subtitle: string;
      readonly rows: readonly QueueRow[];
    }
  | { readonly type: 'single'; readonly id: string; readonly row: QueueRow; readonly text: string }
  | {
      /** A player's reaction nobody answered in 10 s: the question came to the master (C8-04). */
      readonly type: 'unanswered';
      readonly id: string;
      readonly row: QueueRow;
      readonly title: string;
      /** What the answer changes, with the numbers (the master's only). */
      readonly text: string;
      /** What it costs, and why it came to the master. */
      readonly cost: string;
    }
  | {
      readonly type: 'check';
      readonly id: string;
      readonly window: ReactionWindow;
      readonly summary: string;
      readonly canReact: boolean;
    }
  | { readonly type: 'rebukeSave'; readonly id: string; readonly window: ReactionWindow }
  | {
      readonly type: 'concentration';
      readonly id: string;
      readonly window: ReactionWindow;
      readonly player: boolean;
    }
  | { readonly type: 'effectSave'; readonly id: string; readonly window: ReactionWindow }
  /** A melee hit on a creature with Parry: the master answers for it (#557). */
  | {
      readonly type: 'parry';
      readonly id: string;
      readonly window: ReactionWindow;
      readonly card: ParryCard;
    }
  /** The failed saves of a held cast or rider that Legendary Resistance may turn into successes (#557). */
  | {
      readonly type: 'legendaryResistance';
      readonly id: string;
      readonly window: ReactionWindow;
      readonly card: LegendaryResistanceCard;
    };

function theName(label: string): string {
  return `${article(label)} ${label}`;
}

type Trigger = NonNullable<ReactionWindow['trigger']>;

/** What each reaction adds to its row after the name: the facts of the trigger, in words. */
const ROW_FACTS: {
  readonly [K in ReactionKind]?: (
    w: ReactionWindow,
    t: Trigger | undefined,
    far: string,
  ) => string[];
} = {
  [ReactionKind.COUNTERSPELL]: (_w, _t, far) => [
    'espaço de 3º nível ou maior',
    ...(far ? [far] : []),
  ],
  [ReactionKind.UNCANNY_DODGE]: (w, t) => [
    `o ataque atingiu ${w.reactorLabel}`,
    ...(t ? [`vê ${theName(t.actorLabel)}`] : []),
  ],
  [ReactionKind.SHIELD]: (w) => [`o ataque atingiu ${w.reactorLabel}`],
  [ReactionKind.DEFLECT_MISSILES]: () => ['ataque à distância que acertou'],
  [ReactionKind.MANEUVER_REDUCE]: () => ['ataque corpo a corpo que acertou'],
  [ReactionKind.PROTECTION]: (_w, t) => [
    ...(t?.targetLabel ? [`${t.targetLabel} é o alvo`] : []),
    ...(t ? [`vê ${theName(t.actorLabel)}`] : []),
  ],
  [ReactionKind.HELLISH_REBUKE]: (_w, t) => (t ? [`o dano veio ${ofThe([t.actorLabel])}`] : []),
  [ReactionKind.FEATHER_FALL]: (_w, t) => (t?.actorLabel ? [`${t.actorLabel} cai`] : []),
  [ReactionKind.SLOW_FALL]: (w) => [`${w.reactorLabel} cai`],
  [ReactionKind.CUTTING_WORDS]: (_w, t, far) => [
    ...(t ? [`rolagem ${ofThe([t.actorLabel])}`] : []),
    ...(far ? [far] : []),
  ],
};

/** What the window offers, said for the row: "Contramágica · espaço de 3º nível ou maior · a 9 m do Mago 1". */
export function describeRow(w: ReactionWindow): string {
  const t = w.trigger;
  const far =
    t?.distanceFt !== undefined ? `a ${metersText(t.distanceFt)} ${ofThe([t.actorLabel])}` : '';
  const facts = ROW_FACTS[w.kind]?.(w, t, far) ?? (far ? [far] : []);
  return joinDots([reactionName(w), ...facts]);
}

/** What the group's title says: the kind of action that provoked its windows. */
function groupTitle(first: ReactionWindow): string {
  switch (first.kind) {
    case ReactionKind.COUNTERSPELL:
      return 'Reações a uma magia';
    case ReactionKind.FEATHER_FALL:
    case ReactionKind.SLOW_FALL:
      return 'Reações a uma queda';
    case ReactionKind.HELLISH_REBUKE:
      return 'Reações a um dano';
    default:
      return 'Reações a um ataque';
  }
}

function groupSubtitle(first: ReactionWindow): string {
  const t = first.trigger;
  if (!t) {
    return 'na ordem da iniciativa';
  }
  switch (first.kind) {
    case ReactionKind.COUNTERSPELL:
      return joinDots([`${t.actionNamePt} ${ofThe([t.actorLabel])}`, 'na ordem da iniciativa']);
    case ReactionKind.SHIELD:
    case ReactionKind.UNCANNY_DODGE:
    case ReactionKind.DEFLECT_MISSILES:
    case ReactionKind.MANEUVER_REDUCE:
      return joinDots([
        `Ataque ${ofThe([t.actorLabel])}${t.targetLabel ? ` contra ${t.targetLabel}` : ''}`,
        'acertou',
      ]);
    case ReactionKind.FEATHER_FALL:
    case ReactionKind.SLOW_FALL:
      return joinDots(['Queda num fosso', 'na ordem da iniciativa']);
    case ReactionKind.PROTECTION:
      return joinDots([
        `Ataque ${ofThe([t.actorLabel])}${t.targetLabel ? ` contra ${t.targetLabel}` : ''}`,
        'ainda não rolado',
      ]);
    default:
      return joinDots([
        `${t.actionNamePt || reactionName(first)} ${ofThe([t.actorLabel])}`,
        'na ordem da iniciativa',
      ]);
  }
}

/** The slots the master may use for an NPC's Counterspell, with what each does. */
export function slotOptions(w: ReactionWindow): readonly SlotOption[] {
  return (w.trigger?.slotEffects ?? []).flatMap((s) =>
    s.slot
      ? [
          {
            slot: s.slot,
            title: slotTitle(s.slot),
            effect: s.noCheck ? 'anula sem teste' : `teste de conjuração CD ${s.checkDc}`,
          },
        ]
      : [],
  );
}

function rowOf(e: Encounter, w: ReactionWindow, before: ReactionWindow | undefined): QueueRow {
  const name = reactionName(w);
  const player = w.reactorIsPlayer || combatantIsPlayer(e, w.reactorId);
  const status = player
    ? joinDots(['Respondendo no celular', ...(w.answerNow ? ['vez de responder'] : [])])
    : '';
  return {
    window: w,
    label: w.reactorLabel,
    isPlayer: player,
    tag: player ? 'Jogador' : 'NPC',
    description: describeRow(w),
    status:
      !w.answerNow && before
        ? `Depois ${ofReactor(before, before.reactorIsPlayer || combatantIsPlayer(e, before.reactorId))}`
        : status,
    answerNow: w.answerNow,
    // A player's window is answered "pelo jogador": the name never tells a pronoun.
    useLabel: player
      ? 'Usar pelo jogador'
      : `Usar ${name} ${article(w.reactorLabel) === 'a' ? 'pela' : 'pelo'} ${w.reactorLabel}`,
    passLabel: player ? 'Deixar passar pelo jogador' : 'Deixar passar',
    slots: slotOptions(w),
  };
}

/** The text above an NPC's single card: "Esperando a sua reação: o Mago 1 pode conjurar Escudo Arcano (+5 na CA...)". */
function singleText(e: Encounter, w: ReactionWindow): string {
  const t = w.trigger;
  const who = theName(w.reactorLabel);
  let what: string;
  switch (w.kind) {
    case ReactionKind.SHIELD:
      what =
        t?.armorClassWithShield !== undefined && t.attackTotal !== undefined
          ? `pode conjurar ${REACTION_NAMES.shield} (+5 na CA: ${t.armorClassWithShield} contra ${t.attackTotal} viraria erro)`
          : `pode conjurar ${REACTION_NAMES.shield} (+5 na CA)`;
      break;
    case ReactionKind.COUNTERSPELL:
      what = t?.spellKey
        ? `pode usar ${REACTION_NAMES.counterspell} contra ${t.actionNamePt} (${circleLabel(t.spellLevel)}). Um espaço de ${circleLabel(t.spellLevel)} ou maior a anula sem teste; um espaço menor vale só com o teste`
        : `pode usar ${REACTION_NAMES.counterspell}`;
      break;
    default:
      what = `pode usar ${reactionName(w)}`;
  }
  const reader =
    t && combatantIsPlayer(e, t.actorId)
      ? ` O jogador de ${t.actorLabel} lê só “Esperando o mestre”.`
      : ' Os jogadores leem só “Esperando o mestre”.';
  return `Esperando a sua reação: ${who} ${what}.${reader}`;
}

/** "Pensantus pode usar Escudo Arcano": the question that came to the master. */
function lateTitle(w: ReactionWindow): string {
  return `${w.reactorLabel} pode usar ${reactionName(w)}`;
}

/** The numbers of the reaction that came to the master: the Shield's hit against its armor class, with and without the spell. */
function lateText(w: ReactionWindow): string {
  const t = w.trigger;
  if (
    w.kind === ReactionKind.SHIELD &&
    t?.attackTotal !== undefined &&
    t.armorClassWithShield !== undefined
  ) {
    const base = t.armorClassWithShield - SHIELD_BONUS;
    return `O ataque de ${t.attackTotal} ${ofThe([t.actorLabel])} acerta a CA ${base}. Com ${reactionName(w)} a CA vai a ${t.armorClassWithShield} e o ataque ${t.attackTotal >= t.armorClassWithShield ? 'acerta' : 'erra'}.`;
  }
  return describeRow(w);
}

/** What the reaction costs the character: its reaction, and the slot a spell takes. */
function lateCost(w: ReactionWindow): string {
  const slots = w.prompt.case === 'shield' ? w.prompt.value.slots.filter((x) => x.free > 0) : [];
  const slot = [...slots].sort((x, y) => x.level - y.level)[0];
  const what = slot?.resource
    ? 'um uso inato'
    : slot
      ? `um espaço de ${circleLabel(slot.level)}`
      : '';
  const spent = slot ? ` e ${what} (${freeLabel(slot)})` : '';
  return `Gasta a reação${spent}. O jogador não respondeu em ${LATE_SECONDS} s; você responde por ele.`;
}

/** The windows a queue shows: not the checks, the saves, the second steps nor the opportunity attack. */
function inQueue(w: ReactionWindow): boolean {
  return (
    w.kind !== ReactionKind.OPPORTUNITY &&
    w.kind !== ReactionKind.HIDDEN_REVEAL &&
    w.kind !== ReactionKind.MASTER_CHECK &&
    w.kind !== ReactionKind.CONCENTRATION_SAVE &&
    w.kind !== ReactionKind.EFFECT_SAVE &&
    w.kind !== ReactionKind.PARRY &&
    w.kind !== ReactionKind.LEGENDARY_RESISTANCE &&
    !w.secondStep
  );
}

/** The master answers an NPC's effect save and a player's save the player left to the master; any other is the player's. */
function effectSaveCard(e: Encounter, w: ReactionWindow): ReactionCard | null {
  const handed = w.prompt.case === 'effectSave' && w.prompt.value.handedToMaster;
  const player = w.reactorIsPlayer || combatantIsPlayer(e, w.reactorId);
  return !player || handed ? { type: 'effectSave', id: w.id, window: w } : null;
}

/** The cards of the master's own offers: Parry and Legendary Resistance (#557). */
function offerCard(e: Encounter, w: ReactionWindow): ReactionCard | null {
  if (w.kind === ReactionKind.PARRY) {
    const card = parryCard(w, w.trigger ? combatantIsPlayer(e, w.trigger.actorId) : false);
    return card ? { type: 'parry', id: w.id, window: w, card } : null;
  }
  if (w.kind === ReactionKind.LEGENDARY_RESISTANCE) {
    const card = legendaryResistanceCard(
      w,
      w.trigger ? combatantIsPlayer(e, w.trigger.actorId) : false,
    );
    return card ? { type: 'legendaryResistance', id: w.id, window: w, card } : null;
  }
  return null;
}

/** The card of a window that is a card of its own, or `null` for one that belongs to a queue. */
function ownCard(e: Encounter, w: ReactionWindow): ReactionCard | null {
  if (w.kind === ReactionKind.MASTER_CHECK) {
    const p = w.prompt.case === 'masterCheck' ? w.prompt.value : null;
    return {
      type: 'check',
      id: w.id,
      window: w,
      summary: p?.summaryPt ?? '',
      canReact: p?.enemyCanReact ?? false,
    };
  }
  if (w.kind === ReactionKind.CONCENTRATION_SAVE) {
    return {
      type: 'concentration',
      id: w.id,
      window: w,
      player: w.reactorIsPlayer || combatantIsPlayer(e, w.reactorId),
    };
  }
  if (w.kind === ReactionKind.EFFECT_SAVE) {
    return effectSaveCard(e, w);
  }
  const offer = offerCard(e, w);
  if (offer) {
    return offer;
  }
  return w.secondStep && w.prompt.case === 'hellishRebukeSave'
    ? { type: 'rebukeSave', id: w.id, window: w }
    : null;
}

/** The card of one action's windows: an NPC alone is its own card, the others share a queue. */
function groupCard(e: Encounter, key: string, group: readonly ReactionWindow[]): ReactionCard {
  const rows = group.map((x, i) => rowOf(e, x, group[i - 1]));
  if (rows.length === 1 && rows[0].isPlayer && group[0].unanswered) {
    return {
      type: 'unanswered',
      id: key,
      row: rows[0],
      title: lateTitle(group[0]),
      text: lateText(group[0]),
      cost: lateCost(group[0]),
    };
  }
  if (rows.length === 1 && !rows[0].isPlayer) {
    return { type: 'single', id: key, row: rows[0], text: singleText(e, group[0]) };
  }
  return {
    type: 'queue',
    id: key,
    title: groupTitle(group[0]),
    subtitle: groupSubtitle(group[0]),
    rows,
  };
}

/** The cards of the master's side, in the order the server answers them. */
export function reactionCards(e: Encounter): readonly ReactionCard[] {
  const open = openWindows(e);
  const groups = new Map<string, ReactionWindow[]>();
  for (const w of open.filter(inQueue)) {
    const key = w.groupId || w.id;
    groups.set(key, [...(groups.get(key) ?? []), w]);
  }
  const cards: ReactionCard[] = [];
  const emitted = new Set<string>();
  for (const w of open) {
    const own = ownCard(e, w);
    const key = w.groupId || w.id;
    if (own) {
      cards.push(own);
    } else if (groups.has(key) && !emitted.has(key)) {
      emitted.add(key);
      cards.push(groupCard(e, key, groups.get(key) ?? []));
    }
  }
  return cards;
}
