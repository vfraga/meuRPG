import {
  AttackOutcome,
  type CombatantState,
  type DiceRoll,
  type PendingDamage,
  type SpellCast,
  type SpellTargetResult,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  type MetamagicOption,
  type SpellDetails,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import {
  type SlotRow,
  type SpellKind,
  type TargetRule,
  freeText,
  saveAbility,
  spentLine,
} from '../../../../core/combat/cast-flow';
import { article } from '../../../../core/combat/combat-log';
import { diceName, sumRange } from '../../../../core/combat/combat-dice';
import { circleLabel } from '../../../../core/combat/combat-grid';
import { isHit, outcomeWord } from '../../../../core/combat/attack-flow';
import { criticalHint, criticalTypedHint, fixedParts } from '../../../../core/combat/critical';
import { metamagicSpentLine } from '../../../../core/resources/metamagic';
import type { Pool } from '../../../../core/resources/pools';
import type { LuckyRoll } from '../../../../core/resources/resources-client';
import { luckyReroll } from '../../../../core/combat/lucky-question';
import {
  type RollShow,
  showOfDice,
  showOfDie,
} from '../../../../shared/roll-overlay/roll-animator';
import type { SlotAfter } from './cast-result';

/** What the cast sheet shows once a spell is cast: the result lines, the titles, the notes and the dice to show. */

function creaturesText(n: number): string {
  return `${n} ${n === 1 ? 'criatura' : 'criaturas'}`;
}

/** "3º nível · 4 criaturas · teste de resistência de Destreza (CD 16)": what an area hit, counted from the cast's own
 * targets (a player's never holds a hidden creature, RN-10). */
export function areaResultLine(
  circle: string,
  cast: SpellCast | null,
  details: SpellDetails | null,
): string {
  const parts = [circle, creaturesText(cast?.targets.length ?? 0)];
  const ability = saveAbility(details);
  if (ability) {
    const dc = cast?.targets.find((t) => (t.save?.dc ?? 0) > 0)?.save?.dc ?? 0;
    parts.push(`teste de resistência de ${ability}${dc > 0 ? ` (CD ${dc})` : ''}`);
  }
  return parts.filter((p) => p !== '').join(' · ');
}

/** The circle line of a cast sheet that is done: "3º nível", or "Truque"; a pool spell counts the creatures too. */
export function doneSubtitle(i: {
  readonly cast: SpellCast | null;
  readonly level: number;
  readonly kind: SpellKind;
  readonly placedArea: boolean;
  readonly details: SpellDetails | null;
}): string {
  const s = i.cast?.slot;
  const circle = s ? circleLabel(s.level) : i.level === 0 ? 'Truque' : '';
  const n = i.cast?.targets.length ?? 0;
  if (i.placedArea) {
    return areaResultLine(circle, i.cast, i.details);
  }
  return i.kind === 'pool' && n > 0 ? `${circle} · ${creaturesText(n)} na área` : circle;
}

/** The title of the sheet in each stage. */
export function castTitle(i: {
  readonly name: string;
  readonly resume: boolean;
  readonly done: boolean;
  readonly hpDone: boolean;
  readonly placedArea: boolean;
  readonly typing: boolean;
}): string {
  if (i.resume) {
    return `Role o dano de ${i.name}`;
  }
  if (i.hpDone || (i.placedArea && i.done)) {
    return `${i.name} conjurad${article(i.name) === 'a' ? 'a' : 'o'}`;
  }
  if (i.done) {
    return `Você conjurou ${i.name}`;
  }
  return i.typing ? 'Digite o resultado' : `Conjurar ${i.name}`;
}

/** The slot count after the cast, for "Espaços de 1º nível: 1 livre de 4". */
export function slotAfter(cast: SpellCast | null, rows: readonly SlotRow[]): SlotAfter | null {
  const s = cast?.slot;
  const row = s ? rows.find((r) => r.level === s.level && r.pact === s.pact) : undefined;
  if (!s || !row) {
    return null;
  }
  const free = Math.max(0, row.free - 1);
  return {
    level: s.level,
    total: row.total,
    used: row.total === null ? 0 : row.total - free,
    text: `Espaços de ${circleLabel(s.level)}: ${freeText(free, row.total)}`,
  };
}

/** "Escudo Arcano indisponível: sem espaço de 1º nível." when the cast took Escudo's last slot. */
export function shieldLostText(
  cast: SpellCast | null,
  shieldFree: number | null,
  shieldName: string,
): string {
  const s = cast?.slot;
  return s && shieldFree === 1
    ? `${shieldName} indisponível: sem espaço de ${circleLabel(s.level)}.`
    : '';
}

/** What the cast leaves to say: the concentration, the metamagic spent and the action. */
export function castNotes(
  c: SpellCast | null,
  spell: {
    readonly name: string;
    readonly economy: ActionEconomy;
    readonly metaOptions: readonly MetamagicOption[];
    readonly sorceryPoints: Pool | null;
  },
): string[] {
  if (!c) {
    return [];
  }
  const out: string[] = [];
  if (c.concentrating) {
    out.push(`Você está concentrado em ${spell.name}.`);
  }
  if (c.concentrationEndedSpellKey) {
    out.push('A sua concentração anterior acabou.');
  }
  if (c.metamagicKeys.length > 0) {
    const names = c.metamagicKeys.map(
      (k) => spell.metaOptions.find((o) => o.key === k)?.namePt ?? 'Metamagia',
    );
    out.push(`${metamagicSpentLine(names, c.sorceryPointsSpent, spell.sorceryPoints)}.`);
  }
  out.push(spentLine(spell.economy));
  return out;
}

/** The spell is plain: nothing to roll, hit or save. */
export function isPlainCast(c: SpellCast | null, kind: SpellKind): boolean {
  return (
    !!c &&
    kind === 'plain' &&
    c.targets.every((t) => !t.pendingDamageId && t.outcome === AttackOutcome.UNSPECIFIED && !t.save)
  );
}

/** The roll of the first damage group: its dice, the sum a typed roll may be. */
export function nextRollOf(
  groups: readonly (readonly PendingDamage[])[],
  labels: ReadonlyMap<string, { label: string; state: CombatantState }>,
) {
  const g = groups[0];
  if (!g) {
    return null;
  }
  const p = g[0];
  const who = g.map((x) => labels.get(x.targetId)?.label ?? 'alvo');
  const name = diceName(p.diceCount, p.diceSides);
  const range = sumRange(p.diceCount, p.diceSides);
  return {
    pending: p,
    name,
    range,
    label: p.diceCount > 1 ? `Role ${name} para o dano: some os dois` : `Role ${name} para o dano`,
    what: p.healing ? `Cura em ${who.join(', ')}` : `Dano em ${who.join(', ')}`,
    count: groups.length,
    // A critical spell attack follows the table's rule: what to roll, said the way it asks (RN-24).
    crit: p.critical
      ? (criticalHint(p.criticalRule, p.diceCount, p.diceSides, p.criticalMax)?.line ??
        'Acerto crítico: os dados dobram.')
      : '',
    typedHint: criticalTypedHint(p.criticalRule, name, range.min, range.max, p.criticalMax),
    // The modifier and the critical's fixed maximum, the server's numbers, added in the total before it is sent.
    modifier: p.bonus + p.criticalMax,
    fixedText: fixedParts(p.criticalMax, p.bonus),
  };
}

/** A cast sheet that opens on a damage the player never rolled has no cast to read: its rows are made from the damages
 * themselves. */
export function resumedCastOf(pendings: Iterable<PendingDamage>): SpellCast {
  const targets = [...pendings].map(
    (p) =>
      ({
        combatantId: p.targetId,
        darts: p.attackKey === 'spell:magic-missile' ? p.diceCount : 0,
        outcome: AttackOutcome.UNSPECIFIED,
        pendingDamageId: p.id,
      }) as unknown as SpellTargetResult,
  );
  return { targets } as unknown as SpellCast;
}

/**
 * The dice the cast just rolled in the app, as the result step shows them: the pool of a spell that reads hit points,
 * or the spell attack d20 (one die per target in the same overlay; with a single target, the total and the word too).
 * The targets' saving throws are not animated: they are not the caster's roll.
 */
export function castShow(name: string, cast: SpellCast): RollShow | null {
  const pool = showOfDice(name, cast.poolRoll, { withTotal: true });
  if (pool) {
    return pool;
  }
  const attacks = cast.targets.filter((t) => t.attackRoll);
  if (attacks.length === 1) {
    return singleAttackShow(name, attacks[0]);
  }
  if (attacks.length > 1) {
    const dice = attacks.flatMap((t) => showOfDice(name, t.attackRoll)?.dice ?? []);
    return dice.length > 0 ? { label: `Ataque com ${name}`, dice } : null;
  }
  return null;
}

function singleAttackShow(name: string, t: SpellTargetResult): RollShow | null {
  const told = t.outcome !== AttackOutcome.UNSPECIFIED;
  const hit = isHit(t.outcome);
  return showOfDice(`Ataque com ${name}`, t.attackRoll, {
    withTotal: true,
    outcome: told ? { word: outcomeWord(t.outcome), good: hit } : undefined,
    critical: t.outcome === AttackOutcome.CRITICAL_HIT,
    fumble: told && !hit,
  });
}

/** The d20 the app rolled again for Lucky, tumbling, when the die's question comes next (a cast that is told animates
 * its own d20): the new face alone, "1 → 14". */
export function luckyShow(roll: DiceRoll | undefined, die: LuckyRoll | null): RollShow | null {
  const again = die && 'inApp' in die ? luckyReroll(roll) : undefined;
  return again
    ? showOfDie('Sortudo', roll?.diceSides ?? 0, again.to, `${again.from} → ${again.to}`)
    : null;
}

/** Who the cast names: nobody for a placed area (the server works it out), the darts, or the targets ticked. */
export function castTargetsOf(
  placedArea: boolean,
  rule: TargetRule,
  dartTargets: () => { combatantId: string; darts: number }[],
  chosen: readonly string[],
): { combatantId: string; darts: number }[] {
  if (placedArea || rule.kind === 'none') {
    return [];
  }
  return rule.kind === 'darts'
    ? dartTargets()
    : chosen.map((combatantId) => ({ combatantId, darts: 0 }));
}
