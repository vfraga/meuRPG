import { DiceMode, DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type {
  SummonOption,
  SummonSlot,
  SummonSpellOptions,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import type { Combatant } from '../../../gen/meurpg/play/v1/combat_pb';
import type { Creature, CreatureSummary } from '../../../gen/meurpg/rules/v1/rules_pb';
import { effectivePreference } from '../../core/campaigns/dice-labels';
import { type SlotRow, freeText } from '../../core/combat/cast-flow';
import { circleLabel } from '../../core/combat/combat-grid';
import { groupFeminine, pluralName } from '../../core/combat/creature-names';
import {
  beastAttacks,
  beastLine,
  formSubtitle,
  isTableCreature,
} from '../../core/creatures/creature-format';
import { creaturesText, replacesText } from '../../core/creatures/summon-labels';
import { tight } from '../../core/format/text';
import { metersText } from '../../core/units';
import type { ChoiceRow } from '../creatures/creature-choice-list';

/** What the summon sheet shows, as pure functions of what it reads: the sheet keeps the signals and the casting. */

/** "Conjurar Animais · 3º nível · concentração": the parts that are there, with a dot between them. */
export function joinDotsOf(parts: readonly string[]): string {
  return parts.filter(Boolean).join(' · ');
}

export function slotId(r: SlotRow): string {
  return `${r.level}${r.pact ? 'p' : ''}`;
}

export function slotRow(sl: SummonSlot): SlotRow {
  return {
    level: sl.level,
    pact: sl.pact,
    free: sl.free,
    total: sl.total,
    used: sl.total - sl.free,
    enabled: sl.free > 0,
    title: sl.pact ? `${circleLabel(sl.level)} (pacto)` : circleLabel(sl.level),
    count: freeText(sl.free, sl.total),
  };
}

/** The slots a spell can use: from its circle up, only the circles the server lists for it. */
export function slotRowsOf(
  spell: SummonSpellOptions | null,
  slots: readonly SummonSlot[],
): readonly SlotRow[] {
  if (!spell) {
    return [];
  }
  return slots
    .filter(
      (sl) =>
        sl.level >= spell.level && sl.total > 0 && spell.circles.some((c) => c.circle === sl.level),
    )
    .map((sl) => slotRow(sl));
}

/** The chosen option's label: "4 criaturas de ND 1/2 ou menos". */
export function optionLabel(o: SummonOption): string {
  // The spell asks for beasts ("2 feras de ND 1 ou menos"); any other kind says "criaturas".
  const what =
    o.type === 'beast' ? `${o.count} ${o.count === 1 ? 'fera' : 'feras'}` : creaturesText(o.count);
  return tight(`${what} de ND ${o.maxCr} ou menos`);
}

export interface SubtitleInput {
  readonly spell: SummonSpellOptions | null;
  /** The sheet is on its result stage (a casting in a combat). */
  readonly done: boolean;
  readonly inCombat: boolean;
  readonly ritual: boolean;
  readonly slot: SlotRow | null;
}

export function subtitleOf(i: SubtitleInput): string {
  const s = i.spell;
  if (!s) {
    return '';
  }
  if (i.done) {
    return tight(
      joinDotsOf([
        s.namePt,
        i.slot ? circleLabel(i.slot.level) : '',
        s.concentration ? 'concentração' : '',
      ]),
    );
  }
  if (s.grantedBy) {
    // "Druida 3 · 1 ação · gasta 1 uso (resta 1)": where the summon comes from, how long it takes and what it spends.
    return tight(
      joinDotsOf([
        s.grantedByLabelPt || 'Habilidade',
        s.castingTimePt,
        i.inCombat && s.concentration ? 'concentração' : '',
        `gasta 1 uso (${s.usesLeft === 1 ? 'resta' : 'restam'} ${s.usesLeft})`,
      ]),
    );
  }
  return tight(
    `Magia de ${circleLabel(s.level)}${i.ritual ? ' · ritual' : ''} · ${s.castingTimePt}${i.inCombat && s.concentration ? ' · concentração' : ''}`,
  );
}

/** "os 2 Lobos atrozes" once the player chose one kind; "as 2 criaturas" until then. */
function theCreatures(n: number, kindTitles: readonly string[]): string {
  if (n === 1) {
    return 'a criatura';
  }
  const name = kindTitles.length === 1 ? pluralName(kindTitles[0]) : '';
  const fem = kindTitles.length === 1 && /a$/i.test(kindTitles[0].split(/\s+/)[0]);
  return name ? `${fem ? 'as' : 'os'} ${n} ${name}` : `as ${n} criaturas`;
}

export interface ConcentrationInput {
  readonly spell: SummonSpellOptions;
  readonly count: number;
  /** The titles of the kinds chosen so far. */
  readonly kindTitles: readonly string[];
  /** The spell the caster concentrates on now; empty when none. */
  readonly concentrating: string;
}

/** In a combat: what losing the concentration does, and what casting it ends now. */
export function concentrationLineOf(i: ConcentrationInput) {
  const them = theCreatures(i.count, i.kindTitles);
  const ends = i.concentrating
    ? `${i.spell.namePt} encerra a concentração em ${i.concentrating}.`
    : '';
  return {
    lead: 'Concentração.',
    text: tight(`Se você perder a concentração, ${them} ${i.count > 1 ? 'somem' : 'some'}.`),
    ends: tight(ends),
  };
}

/** "3º nível · 2 feras de ND 1 ou menos". */
export function setupLineOf(slot: SlotRow | null, o: SummonOption | null): string {
  return tight(
    joinDotsOf([
      slot ? `${circleLabel(slot.level)}${slot.pact ? ' (pacto)' : ''}` : '',
      o ? (o.maxCr ? optionLabel(o) : creaturesText(o.count)) : '',
    ]),
  );
}

export function replacesNoteOf(s: SummonSpellOptions | null): string {
  return s ? replacesText(s.namePt, s.concentration, s.replaces) : '';
}

export interface RowsInput {
  readonly option: SummonOption | null;
  readonly beasts: readonly CreatureSummary[] | null;
  readonly beastBlocks: ReadonlyMap<string, Creature>;
  readonly forms: readonly Creature[] | null;
  /** The list is the creatures a table's feature or spell brings: a row says what a beast's row does (ND, CA, PV, speed, attacks), the table's first. */
  readonly bring?: boolean;
}

/** The list of the sheet: the beasts the search found, or the forms the option offers, by name. */
export function rowsOf(i: RowsInput): readonly ChoiceRow[] {
  const o = i.option;
  if (!o) {
    return [];
  }
  if (o.forms.length === 0) {
    return (i.beasts ?? []).map((b) => {
      const block = i.beastBlocks.get(b.key);
      const table = isTableCreature(b.key);
      return {
        key: b.key,
        title: b.namePt,
        subtitle: beastLine(b, block),
        // A table creature's numbers are the owner's (the row says so): the base's attacks would contradict that note.
        attacks: block && !table ? beastAttacks(block) : undefined,
        table,
      };
    });
  }
  const blocks = new Map((i.forms ?? []).map((c) => [c.summary?.key ?? '', c]));
  const rows = o.forms.map((f): ChoiceRow => {
    const block = blocks.get(f.monsterKey);
    const table = isTableCreature(f.monsterKey);
    if (!i.bring || !block?.summary) {
      return {
        key: f.monsterKey,
        title: f.namePt,
        subtitle: block ? formSubtitle(block) : '',
        table,
      };
    }
    return {
      key: f.monsterKey,
      title: f.namePt,
      subtitle: beastLine(block.summary, block),
      // A table creature's numbers are the owner's (the row says so): the base's attacks would contradict that note.
      attacks: table ? undefined : beastAttacks(block),
      table,
    };
  });
  // The table's creatures first (in a list to bring), then the book's, each by name.
  return rows.sort(
    (a, b) =>
      (i.bring ? Number(b.table) - Number(a.table) : 0) || a.title.localeCompare(b.title, 'pt-BR'),
  );
}

export function listLabelOf(
  rows: number,
  familiar: boolean,
  several: boolean,
  total: number,
  count: number,
): string {
  if (familiar) {
    return `Forma · ${rows} do livro`;
  }
  return several ? `Criaturas · ${total} de ${count}` : 'Criatura';
}

export interface MissingInput {
  /** The cast spends no slot (a ritual, or a feature's summon): there is no slot to choose. */
  readonly ritual: boolean;
  readonly hasSlot: boolean;
  readonly familiar: boolean;
  readonly name: string;
  readonly hasOption: boolean;
  readonly several: boolean;
  readonly count: number;
  readonly total: number;
}

/** What is still missing, in one sentence naming everything; empty when the cast is ready. */
export function missingOf(i: MissingInput): string {
  const parts: string[] = [];
  if (!i.ritual && !i.hasSlot) {
    parts.push('escolha o espaço de magia');
  }
  if (i.familiar && i.name.trim() === '') {
    parts.push('dê um nome ao familiar');
  }
  const need = i.count - i.total;
  if (i.hasOption && need > 0) {
    parts.push(
      i.several
        ? `escolha mais ${creaturesText(need)}`
        : i.familiar
          ? 'escolha a forma'
          : 'escolha a criatura',
    );
  } else if (need < 0) {
    parts.push(`tire ${creaturesText(-need)}`);
  }
  if (parts.length === 0) {
    return '';
  }
  // "Escolha a forma e dê um nome ao familiar.": the choice first, then the name.
  parts.sort((a, b) => (a.startsWith('dê') ? 1 : 0) - (b.startsWith('dê') ? 1 : 0));
  const text =
    parts.length === 1 ? parts[0] : `${parts.slice(0, -1).join(', ')} e ${parts[parts.length - 1]}`;
  return `${text[0].toUpperCase()}${text.slice(1)}.`;
}

/** "Conjurar como ritual · 1 hora · sem gastar espaço", what the slot costs, or the use a feature's summon spends. */
export function costLineOf(
  s: SummonSpellOptions | null,
  ritual: boolean,
  slot: SlotRow | null,
  count: number,
): string {
  if (!s) {
    return '';
  }
  if (ritual) {
    return tight(`Conjurar como ritual · ${s.castingTimePt} · sem gastar espaço`);
  }
  if (s.grantedBy) {
    // The subtitle says the time and the use; the line is left for what it adds, how many come.
    return count > 1 ? tight(creaturesText(count)) : '';
  }
  const spend = slot
    ? `gasta um espaço de ${circleLabel(slot.level)}${slot.pact ? ' (pacto)' : ''}`
    : 'gasta um espaço de magia';
  const made = count > 1 ? `${creaturesText(count)} · ` : '';
  return tight(`${made}${s.castingTimePt} · ${spend}`);
}

/** "Lobos atrozes conjurados": the title of the result stage, in the gender of the group. */
export function resultTitleOf(creatures: readonly Combatant[]): string {
  const first = creatures[0];
  const sameKind = creatures.every((c) => c.monsterKey === first?.monsterKey);
  const fem = creatures.length > 0 && groupFeminine(creatures);
  return creatures.length === 1
    ? `${first.label} ${fem ? 'conjurada' : 'conjurado'}`
    : `${sameKind && first?.monsterNamePt ? pluralName(first.monsterNamePt) : 'Criaturas'} ${fem ? 'conjuradas' : 'conjurados'}`;
}

/** "CA 14 · PV 37 de 37 · 15 m" for a creature of the result (its numbers are the owner's). */
export function resultLineOf(c: Combatant, ac: number | undefined): string {
  const hp =
    c.hitPointsCurrent !== undefined && c.hitPointsMax !== undefined
      ? `PV ${c.hitPointsCurrent} de ${c.hitPointsMax}`
      : '';
  return tight(joinDotsOf([ac === undefined ? '' : `CA ${ac}`, hp, metersText(c.speedFt)]));
}

/** The kinds in the list's order, each as many times as it was chosen. */
export function creatureKeysOf(
  rows: readonly ChoiceRow[],
  counts: Readonly<Record<string, number>>,
): string[] {
  return rows.flatMap((r) => Array.from({ length: counts[r.key] ?? 0 }, () => r.key));
}

/** How the group's initiative d20 is made in a combat: by the app, typed from a physical die, or either. */
export function diceFlagsOf(
  combat: { diceMode: DiceMode; preference: DicePreference } | undefined,
) {
  if (!combat) {
    return { canApp: true, canType: false, typing: false };
  }
  const canApp = combat.diceMode !== DiceMode.PHYSICAL;
  const canType = combat.diceMode !== DiceMode.APP;
  const preferApp = effectivePreference(combat.diceMode, combat.preference) === DicePreference.APP;
  return { canApp, canType, typing: !canApp || (canType && !preferApp) };
}

/** What the sheet answers when it cast: for the panel's live notice. */
export interface SummonSheetResult {
  readonly spellName: string;
  readonly ritual: boolean;
  readonly castingTime: string;
  readonly names: readonly string[];
  readonly count: number;
  readonly dismissed: number;
}
