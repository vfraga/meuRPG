import {
  AreaPlacement,
  type GetTurnOptionsResponse,
  ReadiedKind,
  type SpellTargets,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  AttackKind,
  DisabledReasonCode,
  type ActionOption,
  type AttackOption,
  type SpellOption,
  type TurnOptions,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { joinDots, tight } from '../format/text';
import type { ReadyArgs } from './combat-client-ready';
import { circleLabel } from './combat-grid';
import {
  attackDetail,
  attackName,
  damageText,
  rangeText,
  reasonText,
  spellLine,
} from './combat-options';

/**
 * The words and the choices of "Preparar" (#582, boards 582-01, 582-02 and 582-09): the Ready sheet's kinds and lists, the
 * trigger's count, and the request it sends. Everything here is for DISPLAY and for building the call: the server checks
 * the call again (`ReadyAction`) and its refusal is the one that counts. Plain functions, no Angular.
 */

/** The most characters a trigger may have, once trimmed (`ReadyActionRequest.trigger_text`). */
export const READY_TRIGGER_MAX = 200;

/** The four things a combatant may hold. */
export type ReadyKind = 'attack' | 'spell' | 'move' | 'other';

/** What a kind button says and, when it cannot be chosen, why. */
export interface ReadyKindChoice {
  readonly key: ReadyKind;
  readonly label: string;
  readonly icon: string;
  /** Why the kind is off, in the sheet; `''` when it can be chosen. */
  readonly blocked: string;
}

/** One slot level a held spell may be cast with. */
export interface ReadySlot {
  readonly level: number;
  readonly text: string;
}

/** One card of the list under the kind: an attack, a spell or a standard action. */
export interface ReadyRow {
  /** The key sent as `action_key` (attack, standard action) or `spell_key`. */
  readonly id: string;
  readonly title: string;
  readonly sub: string;
  /** Why it cannot be held, written in the card; `''` when it can. */
  readonly blocked: string;
  /** A spell: its circle, 0 for a cantrip; and the slot levels it may be cast with (empty for a cantrip). */
  readonly level: number;
  readonly slots: readonly ReadySlot[];
  /** A spell whose energy holds the concentration: every one does (the held spell is the concentration). */
  readonly spell: boolean;
}

/** The window has no way yet to move the token of a readied move: the master does it (#856). */
export const MOVE_PENDING_TEXT =
  'Mover pela janela ainda não dá: por enquanto o mestre move a ficha.';
/** A spell whose cast needs darts, several targets or a point: `AnswerReaction` carries a creature only (#857). */
export const SPELL_PENDING_TEXT =
  'Soltar pela janela ainda não dá para esta magia: ela pede dardos, alvos ou um ponto no mapa.';
export const NO_ATTACK_TEXT = 'Você não tem um ataque para preparar.';
export const NO_OTHER_TEXT = 'Nenhuma outra ação pode ser preparada agora.';
export const EMPTY_TRIGGER_TEXT = 'Escreva quando a ação dispara.';
export const LONG_TRIGGER_TEXT = `Até ${READY_TRIGGER_MAX} caracteres.`;
export const TRIGGER_HINT = 'O mestre decide quando acontece.';
/** The refusal of a call that did not reach the server or failed for no reason the app knows: nothing was spent. */
export const READY_FAILED_TEXT = 'Não deu para preparar agora. Nada foi gasto. Tente de novo.';

/** The standard actions that cannot be held: Attack, Cast a Spell and Ready have their own routes, Hide and Help have their own
 * calls, Dash and Disengage last only for the turn they are taken on. The server refuses them too (`READY_ACTION`). */
const NOT_HELD = /^standard:(attack|cast-a-spell|ready|hide|help|dash|disengage)$/;

/** Whether the standard action is the Ready action itself ("Preparar"). */
export function isReadyOption(o: ActionOption): boolean {
  return o.viaReadyAction;
}

/** The standard actions that stay buttons: everything but "Preparar", which has its own row. */
export function withoutReady(list: readonly ActionOption[]): ActionOption[] {
  return list.filter((a) => !isReadyOption(a));
}

export { holdsReadied } from './ready-card';

const KINDS: readonly Omit<ReadyKindChoice, 'blocked'>[] = [
  { key: 'attack', label: 'Ataque', icon: 'swords' },
  { key: 'spell', label: 'Magia', icon: 'auto_fix_high' },
  { key: 'move', label: 'Mover', icon: 'directions_walk' },
  { key: 'other', label: 'Outra', icon: 'more_horiz' },
];

/** The weapon and natural attacks that can be held; a cantrip that attacks is held as a spell. */
function heldAttacks(options: TurnOptions): readonly AttackOption[] {
  return options.attacks.filter((a) => a.attack && a.attack.kind !== AttackKind.SPELL);
}

/** The cantrips that attack or ask for a save: held as a spell, with no slot. */
function cantripAttacks(options: TurnOptions): readonly AttackOption[] {
  return options.attacks.filter((a) => a.attack && a.attack.kind === AttackKind.SPELL);
}

/** The standard actions the sheet offers under "Outra". */
function heldOthers(options: TurnOptions): readonly ActionOption[] {
  return options.standardActions.filter((a) => a.action && !NOT_HELD.test(a.action.key));
}

/**
 * Whether Mover is off for lack of speed, and how far the sheet says it goes. The walking speed is not the readier's speed: the
 * server holds the most of the speeds on any surface (`readyMoveFt`), so a swimmer or a flier with 0 to walk can ready a move. Only
 * what the turn options say blocks it (a grapple, a movement of 0); with no movement in them nothing is derived and the server
 * refuses. The distance is written only when it is the walking speed (a Dash or another surface changes it: "até o seu deslocamento").
 */
export function readySpeed(
  walkFt: number,
  res: GetTurnOptionsResponse,
): { readonly noSpeed: boolean; readonly speedFt: number } {
  const best = res.options?.economy?.movement?.speedFt;
  const noSpeed = res.contestState?.grappled === true || best === 0;
  return { noSpeed, speedFt: best === undefined || best === walkFt ? walkFt : 0 };
}

/** The kind buttons, in the board's order: "Magia" only for a combatant with spells; "Mover" is off, with its reason, until the window moves the token (#856). */
export function readyKinds(options: TurnOptions): readonly ReadyKindChoice[] {
  const hasSpells = readySpellRows(options).length > 0;
  return KINDS.filter((k) => k.key !== 'spell' || hasSpells).map((k) => ({
    ...k,
    blocked: kindBlocked(k.key, options),
  }));
}

function kindBlocked(kind: ReadyKind, options: TurnOptions): string {
  switch (kind) {
    case 'attack':
      return heldAttacks(options).length === 0 ? NO_ATTACK_TEXT : '';
    case 'move':
      return MOVE_PENDING_TEXT;
    case 'other':
      return heldOthers(options).length === 0 ? NO_OTHER_TEXT : '';
    default:
      return '';
  }
}

/** The attacks: "Espada longa · +6 para acertar · 1d8 + 3 cortante · 1,5 m". Held whether or not the Attack action is free now. */
export function readyAttackRows(options: TurnOptions): readonly ReadyRow[] {
  return heldAttacks(options).map((o) => ({
    id: o.attack?.key ?? '',
    title: attackName(o.attack!),
    sub: attackDetail(o.attack!, true),
    blocked: '',
    level: 0,
    slots: [],
    spell: false,
  }));
}

/** "1º círculo · restam 2": a slot level and how many are free. */
function slotText(level: number, free: number): string {
  const word = free === 1 ? 'resta 1' : `restam ${free}`;
  return `${level}º círculo · ${word}`;
}

/** The levels a spell can be held with, one entry each: pact and plain slots of one level count together. */
function slotsOf(o: SpellOption): readonly ReadySlot[] {
  const free = new Map<number, number>();
  for (const s of o.slots) {
    if (!s.resource) {
      free.set(s.level, (free.get(s.level) ?? 0) + s.free);
    }
  }
  return [...free.entries()]
    .filter(([, n]) => n > 0)
    .sort(([a], [b]) => a - b)
    .map(([level, n]) => ({ level, text: slotText(level, n) }));
}

/** Why a spell cannot be held, in the card; `''` when it can. Only a spell of 1 action can (SRD 5.1, "Ready"). */
function spellBlocked(o: SpellOption): string {
  if (o.economy === ActionEconomy.REACTION) {
    return 'É uma reação: só uma magia de 1 ação pode ser preparada.';
  }
  if (o.economy === ActionEconomy.BONUS_ACTION) {
    return 'É uma ação bônus: só uma magia de 1 ação pode ser preparada.';
  }
  if (o.economy !== ActionEconomy.ACTION) {
    return 'Demora mais de 1 ação: só uma magia de 1 ação pode ser preparada.';
  }
  if (!o.enabled) {
    return o.reason?.code === DisabledReasonCode.NO_SLOT
      ? `Sem espaços de ${o.spell?.level ?? 1}º círculo.`
      : reasonText(o.reason);
  }
  return '';
}

/**
 * Whether the window can release a spell today: it casts on the one creature the readier named (or the one it picks), so a spell that
 * wants darts, a placed area (a point or a direction), or any number or several targets cannot. Read from what `GetTurnOptions` says of
 * the spell (`spell_targets`); with no entry (an attack cantrip, which is released as an attack) nothing is known against it. A spell
 * that targets only the caster has `max_targets` 0 like an area in a combat with no map, so it is off too.
 */
function releaseBlocked(all: readonly SpellTargets[], key: string): string {
  const t = all.find((x) => x.spellKey === key);
  if (!t) {
    return '';
  }
  const single =
    t.maxTargets === 1 && t.darts.length === 0 && t.placement === AreaPlacement.UNSPECIFIED;
  return single ? '' : SPELL_PENDING_TEXT;
}

/** "1º círculo · 1 ação · Evocação ...": the line under a spell's name. */
function spellSub(o: SpellOption, level: number, summaries: ReadonlyMap<string, string>): string {
  const line = spellLine(o, summaries.get(o.spell?.key ?? '') ?? '');
  const action = o.economy === ActionEconomy.ACTION ? '1 ação' : '';
  return tight(joinDots([circleLabel(level), action, line].filter(Boolean)));
}

/**
 * The spells: the cantrips that attack, then the spells of the list the server sends. A spell a feature grants is not offered (the
 * server holds the class spells only). A spell that is not of 1 action, or has no slot, is listed with its reason.
 */
export function readySpellRows(
  options: TurnOptions,
  spellTargets: readonly SpellTargets[] = [],
  summaries: ReadonlyMap<string, string> = new Map(),
): readonly ReadyRow[] {
  const cantrips: ReadyRow[] = cantripAttacks(options).map((o) => ({
    id: o.attack?.key ?? '',
    title: attackName(o.attack!),
    sub: tight(
      joinDots([
        circleLabel(0),
        o.attack!.saveDc > 0 ? `CD ${o.attack!.saveDc}` : 'ataque de magia',
        damageText(o.attack!),
        rangeText(o.attack!),
      ]),
    ),
    blocked: releaseBlocked(spellTargets, o.attack?.key ?? ''),
    level: 0,
    slots: [],
    spell: true,
  }));
  const spells: ReadyRow[] = options.spells
    .filter((o) => o.spell && !o.grant)
    .map((o) => {
      const level = o.spell?.level ?? 0;
      return {
        id: o.spell?.key ?? '',
        title: o.spell?.namePt || o.spell?.name || '',
        sub: spellSub(o, level, summaries),
        blocked: spellBlocked(o) || releaseBlocked(spellTargets, o.spell?.key ?? ''),
        level,
        slots: level > 0 ? slotsOf(o) : [],
        spell: true,
      };
    });
  return [...cantrips, ...spells];
}

/** "Esquivar", "Procurar", "Usar um objeto": the standard actions that are worth holding. */
export function readyOtherRows(options: TurnOptions): readonly ReadyRow[] {
  return heldOthers(options).map((o) => ({
    id: o.action?.key ?? '',
    title: o.action?.namePt ?? '',
    sub: '',
    blocked: o.enabled ? '' : reasonText(o.reason),
    level: 0,
    slots: [],
    spell: false,
  }));
}

/** The trigger as the field stands: how many characters it has (trimmed, as the server counts) and what the field says about it. */
export function triggerState(
  text: string,
  touched: boolean,
): { readonly count: number; readonly error: string } {
  const count = [...text.trim()].length; // runes, as the server counts (utf8.RuneCount)
  if (count > READY_TRIGGER_MAX) {
    return { count, error: LONG_TRIGGER_TEXT };
  }
  return { count, error: touched && count === 0 ? EMPTY_TRIGGER_TEXT : '' };
}

/** The wire kind of a sheet kind. */
export function wireKind(kind: ReadyKind): ReadiedKind {
  switch (kind) {
    case 'attack':
      return ReadiedKind.ATTACK;
    case 'spell':
      return ReadiedKind.SPELL;
    case 'move':
      return ReadiedKind.MOVE;
    default:
      return ReadiedKind.OTHER;
  }
}

/** What the player picked. */
export interface ReadyPick {
  readonly kind: ReadyKind;
  /** The key of the attack, the standard action or the spell; `''` for a move. */
  readonly key: string;
  /** A leveled spell: the slot level; 0 otherwise. */
  readonly slotLevel: number;
  /** The creature the trigger names, or `''`. */
  readonly targetId: string;
  readonly trigger: string;
}

/** The arguments of `ReadyAction`, as the call takes them: the key goes to `action_key` or to `spell_key` by the kind. */
export function readyArgs(
  pick: ReadyPick,
): Pick<
  ReadyArgs,
  'kind' | 'actionKey' | 'spellKey' | 'slotLevel' | 'targetCombatantId' | 'triggerText'
> {
  const spell = pick.kind === 'spell';
  return {
    kind: wireKind(pick.kind),
    actionKey: pick.kind === 'attack' || pick.kind === 'other' ? pick.key : '',
    spellKey: spell ? pick.key : '',
    slotLevel: spell ? pick.slotLevel : 0,
    targetCombatantId: pick.targetId,
    triggerText: pick.trigger.trim(),
  };
}

/** Why "Preparar" is not ready yet, in words; `''` when it is. */
export function readyMissing(
  pick: ReadyPick,
  row: ReadyRow | undefined,
  kindBlockedText: string,
): string {
  if (kindBlockedText) {
    return kindBlockedText;
  }
  if (pick.kind !== 'move' && (!row || row.blocked)) {
    return row?.blocked || 'Escolha o que preparar.';
  }
  if (pick.kind === 'spell' && row && row.level > 0 && pick.slotLevel === 0) {
    return 'Escolha o espaço de magia.';
  }
  const t = triggerState(pick.trigger, true);
  return t.count === 0 ? EMPTY_TRIGGER_TEXT : t.error;
}

export { heldName } from './ready-held';
