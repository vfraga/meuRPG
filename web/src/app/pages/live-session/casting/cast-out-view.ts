import type { CastingGrant, CastingSpell } from '../../../../gen/meurpg/play/v1/casting_pb';
import {
  type CastWay,
  minutesWords,
  needsTarget,
  ritualTimeText,
  spellLine,
} from '../../../core/casting/cast-out-flow';
import {
  castableNow,
  grantCard,
  wayPicksOf,
  type WayPick,
} from '../../../core/casting/cast-grants';
import type { CastDice, OutsideCastRequest } from '../../../core/casting/casting-client';
import type { SlotRow } from '../../../core/combat/cast-flow';
import { abilityMissing } from '../../../core/effects/ability-choice';
import { castNotice, creaturesText } from '../../../core/creatures/summon-labels';
import type { SummonSheetResult } from '../../../shared/summon-sheet/summon-sheet';
import type { ChoiceRow } from './choice-cards';
import type { CastConfirmData } from './confirm-sheet';

/** The NPCs the master can cast as ("Quem conjura"). */
export function casterRowsOf(
  npcs: readonly { readonly id: string; readonly name: string; readonly detail: string }[],
): readonly ChoiceRow[] {
  return npcs.map((n) => ({
    id: n.id,
    title: n.name,
    detail: n.detail,
    enabled: true,
    reason: '',
  }));
}

/** The caster's spells, each off when there is neither a free slot nor a ritual to cast it with. */
export function spellRowsOf(spells: readonly CastingSpell[]): readonly ChoiceRow[] {
  return spells.map((s) => ({
    id: s.spell?.key ?? '',
    title: s.spell?.namePt ?? '',
    detail: spellLine(s),
    enabled: castableNow(s),
    reason: castableNow(s)
      ? ''
      : s.grants.length > 0
        ? 'Sem espaço de magia nem usos livres.'
        : 'Sem espaço de magia livre.',
  }));
}

/** The ways to cast the spell: with a slot (its casting time) or as a ritual (that time plus the ritual's, no slot). */
export function wayRowsOf(s: CastingSpell, ways: readonly CastWay[]): readonly ChoiceRow[] {
  return ways.map((w) =>
    w === 'slot'
      ? {
          id: 'slot',
          title: 'Com espaço de magia',
          detail:
            s.castingMinutes > 0 ? `Tempo: ${minutesWords(s.castingMinutes)}` : s.castingTimePt,
          enabled: true,
          reason: '',
        }
      : {
          id: 'ritual',
          title: 'Como ritual',
          detail: `${ritualTimeText(s)} · sem espaço de magia`,
          enabled: true,
          reason: '',
        },
  );
}

/** Every way to cast the spell as cards: the class's (a slot, a ritual) and each grant's. */
export function wayCardsOf(s: CastingSpell): readonly ChoiceRow[] {
  const picks = wayPicksOf(s);
  const own = wayRowsOf(
    s,
    picks.filter((p) => p.grant === null).map((p) => p.way),
  );
  return picks.map((p) =>
    p.grant ? grantCard(p, s.spell?.level ?? 0) : (own.find((r) => r.id === p.way) as ChoiceRow),
  );
}

/** The targets as cards: the caster's own row says "(você)". */
export function targetChoicesOf(
  rows: readonly {
    readonly id: string;
    readonly name: string;
    readonly self: boolean;
    readonly detail: string;
    readonly enabled: boolean;
    readonly reason: string;
  }[],
): readonly ChoiceRow[] {
  return rows.map((t) => ({
    id: t.id,
    title: t.self ? `${t.name} (você)` : t.name,
    detail: t.detail,
    enabled: t.enabled,
    reason: t.reason,
  }));
}

/** The dice a spell rolls ("2d6"); one d4 when the text is not a dice expression. */
export function diceOf(rollDice: string): { readonly count: number; readonly sides: number } {
  const m = /^(\d+)d(\d+)$/.exec(rollDice);
  return m ? { count: Number(m[1]), sides: Number(m[2]) } : { count: 1, sides: 4 };
}

/** The dice the spell rolls cast at `level` ("3d8"): the entry of `diceByLevel`, else the spell's own `rollDice`. */
export function diceTextAt(s: CastingSpell, level: number): string {
  return s.diceByLevel.find((d) => d.level === level)?.dice ?? s.rollDice;
}

/** What the cast still needs from the picks so far. */
export interface CastPicks {
  readonly way: CastWay;
  readonly slot: SlotRow | null;
  readonly showTargets: boolean;
  readonly chosen: readonly string[];
  readonly spellKey: string;
  readonly ability: string;
  /** The grant the cast goes through, and whether it casts with a slot; the class's own when absent. */
  readonly pick?: WayPick | null;
}

/** What the slot, the way and the targets still lack, as the button says it; empty when nothing does. */
export function missingOf(s: CastingSpell | null, picks: CastPicks): string {
  if (!s) {
    return '';
  }
  const slotWay = picks.pick ? picks.pick.slot : picks.way === 'slot';
  if (slotWay && (s.spell?.level ?? 0) > 0 && !picks.slot) {
    return 'Escolha o espaço de magia.';
  }
  if (picks.showTargets && needsTarget(s) && picks.chosen.length === 0) {
    return 'Escolha em quem a magia age.';
  }
  return abilityMissing(picks.spellKey, picks.ability);
}

/** Everything the cast request is made of. */
export interface CastRequestParts {
  readonly campaignId: string;
  readonly casterId: string;
  readonly spellKey: string;
  readonly way: CastWay;
  readonly spell: CastingSpell | null;
  readonly slot: SlotRow | null;
  readonly chosen: readonly string[];
  /** `null`: the spell rolls nothing here. */
  readonly dice: CastDice | null;
  /** `null`: the spell asks for no ability. */
  readonly ability: string | null;
  /** The way picked, for a grant's: `grantedBy` and whether a slot goes with it. */
  readonly pick?: WayPick | null;
}

/** What the cast sends: the way, the slot, the targets, the dice and the ability when the spell asks for one. */
export function castRequestOf(p: CastRequestParts): OutsideCastRequest {
  const grant: CastingGrant | null = p.pick?.grant ?? null;
  const slotted = p.pick ? p.pick.slot : p.way === 'slot';
  const slot = !slotted || (p.spell?.spell?.level ?? 0) === 0 ? null : p.slot;
  return {
    campaignId: p.campaignId,
    casterId: p.casterId,
    spellKey: p.spellKey,
    asRitual: p.way === 'ritual',
    slot: slot ? { level: slot.level, pact: slot.pact } : null,
    targetIds: p.chosen,
    dice: p.dice,
    ...(p.ability === null ? {} : { abilityKey: p.ability }),
    ...(grant ? { grantedBy: grant.sourceKey } : {}),
  };
}

/** The line that says a summoned creature arrived, with what the cast cost. */
export function summonedNotice(r: SummonSheetResult): string {
  const arrived =
    r.names.length === 1
      ? `${r.names[0]} chegou.`
      : r.count === 1
        ? 'A criatura chegou.'
        : `${creaturesText(r.count)} chegaram.`;
  return castNotice(arrived, r.spellName, r.ritual, r.castingTime, r.dismissed);
}

/** The question asked before a cast that ends a concentration. */
export function endsConfirmText(spellName: string, ends: string): CastConfirmData {
  return {
    title: `Isso encerra ${ends}`,
    body: `Conjurar ${spellName} encerra a concentração em ${ends}, e os efeitos dela terminam.`,
    confirm: `Encerrar ${ends} e conjurar`,
    cancel: 'Cancelar',
  };
}

/** A notice split at its first sentence: "Estrela chegou." and " Convocar Montaria: o espaço de magia foi gasto.". */
export function noticeLead(text: string): { lead: string; rest: string } {
  const end = text.indexOf('. ');
  return end < 0
    ? { lead: text, rest: '' }
    : { lead: text.slice(0, end + 1), rest: text.slice(end + 1) };
}
