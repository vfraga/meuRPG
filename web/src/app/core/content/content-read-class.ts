import type {
  TableClass,
  TableClassLevel,
  TableSubclass,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { optionsAsked } from './content-read-feat';
import { joinDots } from '../format/text';
import { slotsText } from './class-draft';
import { DEFAULT_SUBCLASS_LEVEL } from './content-kinds';
import {
  type EntryRead,
  type NameOf,
  type ReadItem,
  type ReadRow,
  type ReadSection,
  featureItem,
  names,
} from './content-read-parts';
import { ABILITY_FIELDS } from './feature-draft';

/** A subclass that casts begins at the level of a third caster unless it says otherwise. */
const THIRD_CASTER_START = 3;

/** The way of casting as a row says it: "Metade · Sabedoria · preparadas". */
const READ_CASTING: Readonly<Record<string, string>> = {
  full: 'Completa',
  half: 'Metade',
  pact: 'Pacto',
  third: 'Um terço',
};

/** "Metade · Sabedoria · preparadas" (and "· rituais"): how a class or a third caster's subclass casts. */
function castingLine(
  c: NonNullable<TableClass['casting']>,
  abilityName: (a: number) => string,
): string {
  return joinDots(
    [
      READ_CASTING[c.kind] ?? c.kind,
      abilityName(c.ability),
      c.preparation === 'prepared' ? 'preparadas' : 'conhecidas',
      c.ritual ? 'rituais' : '',
    ].filter((x) => x !== ''),
  );
}

/** One row of a class's table in a line: "+3 · Ataque extra, Vigília · 2 truques · 4 de 1º, 2 de 2º". What the server stored, in words. */
function levelLine(lv: TableClassLevel, pact: boolean, marks: readonly string[] = []): string {
  const slots = slotsText(
    {
      profBonus: lv.profBonus,
      cantrips: lv.cantripsKnown,
      spells: lv.spellsKnown,
      slots: [...lv.slots],
    },
    pact,
  );
  return [
    lv.profBonus > 0 ? `+${lv.profBonus}` : '',
    [...lv.features.map((f) => f.namePt), ...marks].join(', '),
    lv.cantripsKnown > 0
      ? `${lv.cantripsKnown} ${lv.cantripsKnown === 1 ? 'truque' : 'truques'}`
      : '',
    lv.spellsKnown > 0
      ? `${lv.spellsKnown} ${lv.spellsKnown === 1 ? 'magia conhecida' : 'magias conhecidas'}`
      : '',
    slots,
  ]
    .filter((x) => x !== '')
    .join(' · ');
}

/** The skills a class chooses from. */
function skillRows(c: TableClass, nameOf: NameOf): ReadRow[] {
  return c.skillChoose > 0
    ? [
        {
          label: 'Perícias',
          value: `${c.skillChoose} de ${c.skillFrom.length}: ${names(c.skillFrom, nameOf)}`,
        },
      ]
    : [];
}

/** Armor, weapons and the rest, each only when the class has one. */
function proficiencyRows(c: TableClass, nameOf: NameOf): ReadRow[] {
  const armor = c.proficiencies
    .filter((k) => k.includes('armor') || k.includes('shield'))
    .map(nameOf);
  const weapons = c.proficiencies.filter((k) => k.includes('weapon')).map(nameOf);
  const other = c.proficiencies
    .filter((k) => !k.includes('armor') && !k.includes('shield') && !k.includes('weapon'))
    .map(nameOf);
  const rows: ReadRow[] = [];
  if (armor.length > 0) rows.push({ label: 'Armaduras', value: armor.join(', ') });
  if (weapons.length > 0) rows.push({ label: 'Armas', value: weapons.join(', ') });
  if (other.length > 0) rows.push({ label: 'Outras proficiências', value: other.join(', ') });
  return rows;
}

function classCastingRows(
  casting: NonNullable<TableClass['casting']>,
  nameOf: NameOf,
  abilityName: (a: number) => string,
): ReadRow[] {
  return [
    { label: 'Conjuração', value: castingLine(casting, abilityName) },
    {
      label: 'Lista de magias',
      value: casting.listFrom
        ? `A lista do ${nameOf(casting.listFrom)}`
        : 'A própria lista da classe',
    },
  ];
}

/** The ability scores a multiclass asks for: "Força 13, Carisma 13". */
function minimumsText(m: TableClass['minimums'], nameOf: NameOf): string {
  return ABILITY_FIELDS.filter((a) => (m?.[a] ?? 0) > 0)
    .map((a) => `${nameOf(`ability:${a}`)} ${m?.[a]}`)
    .join(', ');
}

function multiclassRows(c: TableClass, nameOf: NameOf): ReadRow[] {
  const all = minimumsText(c.minimums, nameOf);
  const any = minimumsText(c.anyOf, nameOf);
  const rows: ReadRow[] = [];
  if (all) rows.push({ label: 'Para multiclasse', value: all });
  if (any) rows.push({ label: 'Para multiclasse (uma delas)', value: any });
  return rows;
}

/** The "Características" section of every level's features; `levelOf` says which level a row of the list is. */
function featureSections<T extends { readonly features: TableClassLevel['features'] }>(
  levels: readonly T[],
  levelOf: (lv: T, i: number) => number,
  nameOf: NameOf,
): ReadSection[] {
  const items: ReadItem[] = [];
  levels.forEach((lv, i) =>
    lv.features.forEach((f) => {
      const item = featureItem(f, nameOf, `Nível ${levelOf(lv, i)} · `);
      // How many the player chooses, as the preview's "Escolha" row says it: "Escolha 3 de 5 opções."
      const asked = f.effects
        .filter((e) => e.type === 'choice' && e.choice === 'feature')
        .map((e) => optionsAsked(e, f.options.length))
        .filter((a) => a !== '')
        .map((a) => `Escolha ${a}.`);
      items.push(asked.length > 0 ? { ...item, text: [item.text, ...asked].join(' ') } : item);
    }),
  );
  return items.length > 0 ? [{ title: 'Características', items }] : [];
}

/** The table in words, level by level (the numbers the server stored; a 0 proficiency bonus is the SRD's and is not written). */
function levelTableSections(c: TableClass): ReadSection[] {
  if (c.levels.length === 0) return [];
  const choose = c.subclassLevel || DEFAULT_SUBCLASS_LEVEL;
  return [
    {
      title: 'Tabela dos níveis',
      items: c.levels.map((lv, i) => {
        const marks = [
          c.asiLevels.includes(i + 1) ? 'Incremento no Valor de Habilidade' : '',
          choose === i + 1 ? 'Escolha de subclasse' : '',
        ].filter((x) => x !== '');
        return {
          title: `Nível ${i + 1}`,
          text: levelLine(lv, c.casting?.kind === 'pact', marks),
        };
      }),
    },
  ];
}

export function readClass(c: TableClass, nameOf: NameOf): EntryRead {
  const abilityName = (a: number): string => nameOf(`ability:${a}`);
  const rows: ReadRow[] = [
    { label: 'Dado de vida', value: `d${c.hitDie}` },
    { label: 'Testes de resistência', value: c.savingThrows.map(abilityName).join(', ') },
    ...skillRows(c, nameOf),
    ...proficiencyRows(c, nameOf),
    ...(c.casting ? classCastingRows(c.casting, nameOf, abilityName) : []),
    {
      label: 'Escolhe a subclasse',
      value: `no nível ${c.subclassLevel || DEFAULT_SUBCLASS_LEVEL}`,
    },
    ...multiclassRows(c, nameOf),
  ];
  const sections = [
    ...featureSections(c.levels, (_, i) => i + 1, nameOf),
    ...levelTableSections(c),
  ];
  return { rows, sections, text: [] };
}

function subclassCastingRows(
  casting: NonNullable<TableSubclass['casting']>,
  nameOf: NameOf,
  abilityName: (a: number) => string,
): ReadRow[] {
  const rows: ReadRow[] = [{ label: 'Conjuração', value: castingLine(casting, abilityName) }];
  if (casting.listFrom) {
    rows.push({ label: 'Lista de magias', value: `A lista do ${nameOf(casting.listFrom)}` });
  }
  return rows;
}

/** A third caster's table from where it starts casting. */
function subclassCastingSections(s: TableSubclass): ReadSection[] {
  if (!s.casting) return [];
  const from = s.casting.startLevel > 0 ? s.casting.startLevel : THIRD_CASTER_START;
  const table = s.levels
    .filter((lv) => lv.level >= from)
    .map((lv) => ({
      title: `Nível ${lv.level}`,
      text: levelLine(
        {
          profBonus: 0,
          cantripsKnown: lv.cantripsKnown,
          spellsKnown: lv.spellsKnown,
          slots: lv.slots,
          features: [],
        } as unknown as TableClassLevel,
        false,
      ),
    }));
  return table.length > 0 ? [{ title: 'Conjuração por nível', items: table }] : [];
}

function alwaysPreparedSections(s: TableSubclass, nameOf: NameOf): ReadSection[] {
  return s.alwaysPrepared.length > 0
    ? [
        {
          title: 'Sempre preparadas',
          items: s.alwaysPrepared.map((a) => ({
            title: `Nível ${a.classLevel}`,
            text: nameOf(a.spellKey),
          })),
        },
      ]
    : [];
}

export function readSubclass(
  s: TableSubclass,
  nameOf: NameOf,
  subclassLevelOf: (classKey: string) => number,
): EntryRead {
  const abilityName = (a: number): string => nameOf(`ability:${a}`);
  const chosen = s.level || subclassLevelOf(s.classKey);
  const rows: ReadRow[] = [
    { label: 'Subclasse de', value: nameOf(s.classKey) },
    ...(chosen > 0 ? [{ label: 'Escolhida no nível', value: String(chosen) }] : []),
    ...(s.casting ? subclassCastingRows(s.casting, nameOf, abilityName) : []),
  ];
  const sections = [
    ...featureSections(s.levels, (lv) => lv.level, nameOf),
    ...subclassCastingSections(s),
    ...alwaysPreparedSections(s, nameOf),
  ];
  return { rows, sections, text: s.descPt };
}
