import { create } from '@bufbuild/protobuf';

import type { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type {
  Combatant,
  Encounter,
  GetTurnOptionsResponse,
  InspirationOffer,
  LuckyOffer,
  PendingDamage,
  ReactionWindow,
  TargetInReach,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  type Attack,
  type AttackOption,
  AttackKind,
  AttackSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import type { CombatState } from '../../../../core/combat/combat-state';
import type { AttackSheetData } from './attack-sheet';

/** The key the throw-back sheet gives its made-up attack: the server builds the real one from the window and ignores it. */
export const THROW_BACK_KEY = 'deflect-missiles:throw';

/** The questions of a held attack roll the page reopens the attack sheet at: the halfling's Lucky about a 1 (asked
 * first) and the Bardic Inspiration die. */
export interface HeldRoll {
  readonly offer?: InspirationOffer;
  readonly lucky?: LuckyOffer;
}

/** An attack that is not in the options: the monk's throw-back of a caught missile, with its own targets. */
export interface ThrownAttack {
  readonly attack: Attack;
  readonly targets: TargetInReach[];
}

/** Everything the page knows when it opens the attack sheet. */
export interface AttackSheetRequest {
  readonly key: string;
  readonly campaignId: string;
  readonly encounter: Encounter;
  /** Whoever attacks: the player's character, one of their creatures, or the master's NPC. */
  readonly attacker: Combatant;
  /** `GetTurnOptions` of the attacker: the attacks with their numbers, the targets, the economy. */
  readonly options: GetTurnOptionsResponse | null | undefined;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  readonly state: CombatState;
  readonly master: boolean;
  readonly asReaction: boolean;
  readonly useExtraAction: boolean;
  readonly catchWindowId: string;
  readonly resume?: PendingDamage;
  readonly held?: HeldRoll;
  readonly thrown?: ThrownAttack;
  /** A roll of the sheet was held for a question: the page learns the hold. */
  readonly onHeld: (holdId: string) => void;
}

/** The label of a combatant of the combat, `''` when it is not in it. */
function labelOf(e: Encounter, id: string): string {
  return e.combatants.find((c) => c.id === id)?.label ?? '';
}

/** The attack the sheet is for and the option it came from (the bonus rule, the beams left); the thrown one has no option. */
function findAttack(
  options: GetTurnOptionsResponse | null | undefined,
  key: string,
  thrown: ThrownAttack | undefined,
): { readonly attack: Attack; readonly option: AttackOption | undefined } | null {
  const option = options?.options?.attacks.find((a) => a.attack?.key === key);
  const attack = thrown?.attack ?? option?.attack;
  return attack ? { attack, option } : null;
}

/** The Extra Attack and bonus-attack numbers: the options' own, with the defaults of a combatant that has no economy. */
function economyOf(
  options: GetTurnOptionsResponse | null | undefined,
  option: AttackOption | undefined,
  attack: Attack,
) {
  const economy = options?.options?.economy;
  return {
    bonusRule: option?.bonusRule,
    bonusAttacksLeft: option?.bonusAttacksLeft,
    beamsLeft: option?.beamsLeft || attack.beams,
    attacksLeft: economy?.attacksLeft ?? 0,
    attacksPerAction: economy?.attacksPerAction ?? 1,
  };
}

/** The held roll the sheet reopens at: Lucky's question and the Bardic Inspiration die, each with the target's name. */
function heldOf(e: Encounter, held: HeldRoll | undefined) {
  return {
    inspiration: held?.offer
      ? { offer: held.offer, targetLabel: labelOf(e, held.offer.targetId) }
      : undefined,
    lucky: held?.lucky
      ? { offer: held.lucky, targetLabel: labelOf(e, held.lucky.targetId) }
      : undefined,
  };
}

/** What the attack sheet is handed, or `null` when the attack is not among the options (nothing to open). */
export function attackSheetData(req: AttackSheetRequest): AttackSheetData | null {
  const found = findAttack(req.options, req.key, req.thrown);
  if (!found) {
    return null;
  }
  const e = req.encounter;
  return {
    campaignId: req.campaignId,
    encounterId: e.id,
    attackerId: req.attacker.id,
    round: e.round,
    attack: found.attack,
    targets:
      req.thrown?.targets ??
      req.options?.attackTargets.find((t) => t.attackKey === req.key)?.targets ??
      [],
    diceMode: req.diceMode,
    preference: req.preference,
    state: req.state,
    asReaction: req.asReaction,
    master: req.master,
    ...(req.catchWindowId ? { catchWindowId: req.catchWindowId } : {}),
    ...(req.useExtraAction ? { useExtraAction: req.useExtraAction } : {}),
    ...economyOf(req.options, found.option, found.attack),
    resume: req.resume
      ? { pending: req.resume, targetLabel: labelOf(e, req.resume.targetId) }
      : undefined,
    ...heldOf(e, req.held),
    onHeld: req.onHeld,
  };
}

/** The sheet's accessible name: an opportunity attack says so; a creature's attack names who attacks. */
export function attackAriaLabel(attack: Attack, asReaction: boolean, creature?: Combatant): string {
  const name = attack.namePt || attack.name;
  return asReaction
    ? `Ataque de oportunidade com ${name}`
    : `Atacar com ${name}${creature ? `: ${creature.label}` : ''}`;
}

/** The monk's targets for the throw-back: the ones any attack of the options lists, once each. */
function throwBackTargets(options: GetTurnOptionsResponse | null | undefined): TargetInReach[] {
  const seen = new Map<string, TargetInReach>();
  for (const list of options?.attackTargets ?? []) {
    for (const t of list.targets) {
      if (!seen.has(t.combatantId)) {
        seen.set(t.combatantId, t);
      }
    }
  }
  return [...seen.values()];
}

/**
 * "Devolver (1 de chi)": the ranged attack with the missile the monk caught, part of the same reaction. The server
 * builds the attack from the missile (its dice, Dex, proficiency, 20/60 ft) and ignores the key; the sheet shows what
 * the prompt says. The targets are the ones any of the monk's attacks lists, with the reach re-measured at the long
 * range. `null` when the window is not a throw-back.
 */
export function throwBackAttack(
  window: ReactionWindow | undefined,
  options: GetTurnOptionsResponse | null | undefined,
): ThrownAttack | null {
  const prompt = window?.prompt.case === 'deflectThrow' ? window.prompt.value : null;
  if (!prompt) {
    return null;
  }
  const name = prompt.missileNamePt || 'Projétil';
  const attack = create(AttackSchema, {
    key: THROW_BACK_KEY,
    name,
    namePt: name,
    attackBonus: prompt.attackBonus,
    kind: AttackKind.WEAPON,
    rangeFt: prompt.normalRangeFt,
    longRangeFt: prompt.longRangeFt,
  });
  return { attack, targets: throwBackTargets(options) };
}
