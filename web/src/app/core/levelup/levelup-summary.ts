import type {
  Attack,
  DerivedSheet,
  DerivedSkill,
  SavingThrow,
  Spellcasting,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { Ability as GenAbility } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  abilityLabel,
  formatModifier,
  pactSlotsText,
  spellLevelLabel,
} from '../characters/character-labels';
import type { AbilityKey } from '../characters/characters.types';
import { joinDots } from '../format/text';

/** One line of "O que muda": a label, the value before and after, and a small line under it. */
export interface ChangeRow {
  readonly key: string;
  readonly label: string;
  readonly before: string;
  readonly after: string;
  readonly sub: string;
  /** The number comes from the table's own class ("Da mesa"), not the SRD's. */
  readonly table?: boolean;
}

/** What the summary needs besides the two sheets: how the hit points were decided and what was picked. */
export interface SummaryContext {
  /** The class gaining the level: a multiclass sheet lists one `spellcasting` entry per casting class, and these rows follow this one. */
  readonly classKey: string;
  /** Whose spell list the known spells are counted on, when it is not the class's own (a table class reusing a list, a third caster). */
  readonly spellListClassKey?: string;
  /** "Média 4 + Constituição +3" / "Rolado 5 + Constituição +3", or '' when it can't be told yet. */
  readonly hpSub: string;
  readonly cantrips: readonly string[];
  readonly spells: readonly string[];
  readonly prepared: readonly string[];
  /** The spells of a spellbook are "Livro de magias"; the others "Magias conhecidas". */
  readonly spellbook: boolean;
  /** How many more spells the level asks for and how many were picked: "(falta 1)". */
  readonly spellsMissing: number;
  /** The class learns spells (a book or a known list) rather than preparing from its list: it shows "Magias conhecidas"
   * or "Livro de magias"; a class that prepares shows only "Magias preparadas". Unset reads as it learns (the old way). */
  readonly learnsSpells?: boolean;
  /** The class gaining the level is the table's own (its key ends in "@mesa"): the slots, which come from its table, say so. */
  readonly table?: boolean;
  /** The features the level gives by themselves, by name ("Estilo de luta", "Conjuração"). */
  readonly newFeatures?: readonly string[];
}

const LIST = new Intl.ListFormat('pt-BR', { type: 'conjunction' });

const ABILITY_KEY: Record<number, AbilityKey> = {
  [GenAbility.STRENGTH]: 'str',
  [GenAbility.DEXTERITY]: 'dex',
  [GenAbility.CONSTITUTION]: 'con',
  [GenAbility.INTELLIGENCE]: 'int',
  [GenAbility.WISDOM]: 'wis',
  [GenAbility.CHARISMA]: 'cha',
};

/** "3d6", or "3d6 + 1d8" for a multiclass. */
function hitDice(sheet: DerivedSheet): string {
  return sheet.hitDice.map((d) => `${d.count}d${d.faces}`).join(' + ');
}

/** "Novo: A" / "Novas: A e B". */
function newNames(names: readonly string[], one: string, many: string): string {
  return names.length === 0 ? '' : `${names.length === 1 ? one : many}: ${LIST.format(names)}`;
}

function row(
  key: string,
  label: string,
  before: string,
  after: string,
  sub = '',
): ChangeRow | null {
  return before === after ? null : { key, label, before, after, sub };
}

function cast(sheet: DerivedSheet, classKey: string) {
  return sheet.spellcasting.find((c) => c.classKey === classKey);
}

function savesChanged(before: DerivedSheet, after: DerivedSheet): ChangeRow[] {
  const was = new Map<number, SavingThrow>(before.savingThrows.map((s) => [s.ability, s]));
  return after.savingThrows.flatMap((s) => {
    const b = was.get(s.ability);
    const key = ABILITY_KEY[s.ability];
    const r =
      b && key
        ? row(
            `save-${key}`,
            `Teste de resistência de ${abilityLabel(key)}`,
            formatModifier(b.bonus),
            formatModifier(s.bonus),
          )
        : null;
    return r ? [r] : [];
  });
}

/** Skills that move by the same amount are one line: "Arcanismo, História, Investigação +6 → +7". */
function skillsChanged(before: DerivedSheet, after: DerivedSheet): ChangeRow[] {
  const was = new Map<string, DerivedSkill>(before.skills.map((s) => [s.key, s]));
  const groups = new Map<string, { names: string[]; before: string; after: string }>();
  for (const s of after.skills) {
    const b = was.get(s.key);
    if (!b || b.bonus === s.bonus) continue;
    const id = `${b.bonus}>${s.bonus}`;
    const g = groups.get(id) ?? {
      names: [],
      before: formatModifier(b.bonus),
      after: formatModifier(s.bonus),
    };
    g.names.push(s.namePt);
    groups.set(id, g);
  }
  return [...groups].map(([id, g]) => ({
    key: `skills-${id}`,
    label: LIST.format(g.names),
    before: g.before,
    after: g.after,
    sub: '',
  }));
}

/**
 * The attacks whose bonus or damage moved (a Strength increase raises the weapon's to-hit and damage; a cantrip
 * rolls more dice), one line each: "+5 · 1d8+3" → "+7 · 1d8+5", with the damage type under it. A spell that asks
 * for a saving throw has no to-hit, so it shows the damage alone.
 */
function attacksChanged(before: DerivedSheet, after: DerivedSheet): ChangeRow[] {
  const was = new Map<string, Attack>(before.attacks.map((a) => [a.key, a]));
  const text = (a: Attack) =>
    [a.saveDc > 0 ? '' : formatModifier(a.attackBonus), a.damage]
      .filter((p) => p !== '')
      .join(' · ');
  return after.attacks.flatMap((a) => {
    const b = was.get(a.key);
    const r = b ? row(`attack-${a.key}`, a.namePt, text(b), text(a), a.damageTypePt) : null;
    return r ? [r] : [];
  });
}

/**
 * Everything the level changes, before → after, as the server derived both sheets
 * (`Character.derived` and `PreviewLevelUp.after`): the browser only lines the
 * numbers up and words them. A line whose two sides are the same is left out;
 * "Nível" always stays.
 */
export function changeRows(
  before: DerivedSheet,
  after: DerivedSheet,
  ctx: SummaryContext,
): ChangeRow[] {
  const rows: (ChangeRow | null)[] = [];
  const level = (s: DerivedSheet) => s.classes.map((c) => `${c.namePt} ${c.level}`).join(' / ');
  rows.push({ key: 'level', label: 'Nível', before: level(before), after: level(after), sub: '' });

  const was = new Map(before.abilities.map((a) => [a.ability, a]));
  for (const a of after.abilities) {
    const b = was.get(a.ability);
    const key = ABILITY_KEY[a.ability];
    if (b && key && b.score !== a.score) {
      rows.push(
        row(
          `ability-${key}`,
          abilityLabel(key),
          String(b.score),
          String(a.score),
          `Modificador ${formatModifier(b.modifier)} → ${formatModifier(a.modifier)}`,
        ),
      );
    }
  }
  rows.push(
    row('hp', 'Pontos de vida', String(before.hitPointsMax), String(after.hitPointsMax), ctx.hpSub),
  );
  rows.push(row('hit-dice', 'Dados de vida', hitDice(before), hitDice(after)));
  rows.push(
    row(
      'proficiency',
      'Bônus de proficiência',
      formatModifier(before.proficiencyBonus),
      formatModifier(after.proficiencyBonus),
    ),
  );
  rows.push(
    row('armor', 'Classe de Armadura', String(before.armorClass), String(after.armorClass)),
  );
  rows.push(
    row(
      'initiative',
      'Iniciativa',
      formatModifier(before.initiative),
      formatModifier(after.initiative),
    ),
  );

  const bc = cast(before, ctx.classKey);
  const ac = cast(after, ctx.classKey);
  if (ac) {
    // Before a class casts there is nothing to compare: a dash, never a 0 or a +0 that looks like a number.
    const none = '—';
    rows.push(row('dc', 'CD das magias', bc ? String(bc.saveDc) : none, String(ac.saveDc)));
    rows.push(
      row(
        'attack',
        'Ataque com magia',
        bc ? formatModifier(bc.attackBonus) : none,
        formatModifier(ac.attackBonus),
      ),
    );
    // A class with no cantrips has nothing to gain: no row for "— → 0".
    if (bc || ac.cantripsKnown > 0) {
      rows.push(
        row(
          'cantrips',
          'Truques',
          bc ? String(bc.cantripsKnown) : none,
          String(ac.cantripsKnown),
          newNames(ctx.cantrips, 'Novo', 'Novos'),
        ),
      );
    }
    const list = ctx.spellListClassKey || ctx.classKey;
    // A class that learns a fixed number of spells has it from the server, whichever list they came from (a Bard's Magical
    // Secrets); the others (a spellbook, a table class) are counted among the spells of the sheet that are on the class list.
    const known = (s: DerivedSheet, c: Spellcasting | undefined) =>
      c && c.spellsKnown > 0
        ? c.spellsKnown
        : s.spells.filter((x) => (x.spell?.level ?? 0) > 0 && x.spell?.classKeys.includes(list))
            .length;
    const knownBefore = known(before, bc);
    const knownAfter = known(after, ac);
    const missing = ctx.spellsMissing > 0 ? ` (falta ${ctx.spellsMissing})` : '';
    if (ctx.learnsSpells !== false && (knownAfter !== knownBefore || ctx.spells.length > 0)) {
      rows.push({
        key: 'spells',
        label: ctx.spellbook ? 'Livro de magias' : 'Magias conhecidas',
        before: bc ? String(knownBefore) : none,
        after: `${knownAfter}${missing}`,
        sub: newNames(ctx.spells, 'Nova', 'Novas'),
      });
    }
    const slots = (s: DerivedSheet, level: number) =>
      s.spellSlots.find((x) => x.level === level)?.count ?? 0;
    const circles = new Set([...before.spellSlots, ...after.spellSlots].map((x) => x.level));
    for (const level of [...circles].sort((a, b) => a - b)) {
      const slotRow = row(
        `slots-${level}`,
        `Espaços de ${spellLevelLabel(level)}`,
        String(slots(before, level)),
        String(slots(after, level)),
        ctx.table ? 'Da tabela da classe' : '',
      );
      rows.push(slotRow && ctx.table ? { ...slotRow, table: true } : slotRow);
    }
    if (after.pactMagic) {
      const p = before.pactMagic;
      const pactRow = row(
        'pact',
        'Espaços do pacto',
        pactSlotsText(p),
        pactSlotsText(after.pactMagic),
        ctx.table ? 'Da tabela da classe' : '',
      );
      rows.push(pactRow && ctx.table ? { ...pactRow, table: true } : pactRow);
    }
    if (ac.preparedMax > 0) {
      const names = newNames(ctx.prepared, 'Nova', 'Novas');
      rows.push(
        row(
          'prepared',
          'Magias preparadas',
          bc ? String(bc.preparedMax) : none,
          String(ac.preparedMax),
          names,
        ),
      );
    }
  }
  if (ctx.newFeatures && ctx.newFeatures.length > 0) {
    rows.push({
      key: 'features',
      label: 'Novas características',
      before: '',
      after: String(ctx.newFeatures.length),
      sub: joinDots([...ctx.newFeatures]),
    });
  }
  rows.push(...savesChanged(before, after));
  rows.push(...attacksChanged(before, after));
  rows.push(...skillsChanged(before, after));
  rows.push(
    row(
      'passive-perception',
      'Percepção passiva',
      String(before.passivePerception),
      String(after.passivePerception),
    ),
  );
  rows.push(
    row(
      'passive-investigation',
      'Investigação passiva',
      String(before.passiveInvestigation),
      String(after.passiveInvestigation),
    ),
  );
  rows.push(
    row(
      'passive-insight',
      'Intuição passiva',
      String(before.passiveInsight),
      String(after.passiveInsight),
    ),
  );
  return rows.filter((r): r is ChangeRow => r !== null);
}
