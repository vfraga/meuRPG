import { create } from '@bufbuild/protobuf';

import {
  type GetClassTableDefaultsResponse,
  GetClassTableDefaultsResponseSchema,
  type TableEntry,
  TableContentKind,
  TableEntrySchema,
  TableClassSchema,
  TableRaceSchema,
  TableSpellSchema,
  TableSubclassSchema,
  TableBackgroundSchema,
  TableFeatSchema,
  TableOptionSchema,
  TableClassFeatureSchema,
  TableFeatureSchema,
  TableEffectSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { Ability, ContentSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import { type CatalogAbility, type CatalogVm, catalogVm } from './catalog';
import { ContentWatcher } from './content-watcher';

/** Test builders: an entry as the server sends it, and a small effect menu with the shape of the real one. */

type Body = TableEntry['body'];

/** The Strength a test feat asks of a character. */
const STRENGTH_MINIMUM = 13;

export function entry(
  kind: TableContentKind,
  namePt: string,
  over: { archived?: boolean; charactersUsing?: number; key?: string; body?: Body } = {},
): TableEntry {
  const slug = namePt.toLowerCase().replace(/[^a-z]+/g, '-');
  const prefix = {
    [TableContentKind.CLASS]: 'class',
    [TableContentKind.SUBCLASS]: 'subclass',
    [TableContentKind.RACE]: 'race',
    [TableContentKind.SUBRACE]: 'subrace',
    [TableContentKind.BACKGROUND]: 'background',
    [TableContentKind.SPELL]: 'spell',
    [TableContentKind.FEAT]: 'feat',
    [TableContentKind.OPTION]: 'option:invocation',
    [TableContentKind.CLASS_FEATURE]: 'classfeature',
    [TableContentKind.CREATURE]: 'creature',
  }[kind as 1];
  const body: Body =
    over.body ??
    (kind === TableContentKind.CLASS
      ? {
          case: 'tableClass',
          value: create(TableClassSchema, { namePt, hitDie: 10, subclassLevel: 3 }),
        }
      : kind === TableContentKind.SUBCLASS
        ? {
            case: 'tableSubclass',
            value: create(TableSubclassSchema, { namePt, classKey: 'class:wizard', level: 2 }),
          }
        : kind === TableContentKind.RACE
          ? {
              case: 'tableRace',
              value: create(TableRaceSchema, { namePt, size: 'Medium', speedFt: 30 }),
            }
          : kind === TableContentKind.BACKGROUND
            ? {
                case: 'tableBackground',
                value: create(TableBackgroundSchema, {
                  namePt,
                  skills: ['skill:arcana', 'skill:history'],
                }),
              }
            : kind === TableContentKind.CLASS_FEATURE
              ? {
                  case: 'tableClassFeature',
                  value: create(TableClassFeatureSchema, {
                    namePt,
                    classKey: 'class:wizard',
                    level: 2,
                    descPt: ['Texto da característica.'],
                  }),
                }
              : kind === TableContentKind.OPTION
                ? {
                    case: 'tableOption',
                    value: create(TableOptionSchema, {
                      namePt,
                      set: 'invocation',
                      descPt: ['Texto da opção.'],
                      prerequisite: { classLevel: { classKey: 'class:warlock', level: 5 } },
                    }),
                  }
                : kind === TableContentKind.FEAT
                  ? {
                      case: 'tableFeat',
                      value: create(TableFeatSchema, {
                        namePt,
                        descPt: ['Texto do talento.'],
                        prerequisite: { minimums: { strength: STRENGTH_MINIMUM } },
                      }),
                    }
                  : { case: 'tableSpell', value: create(TableSpellSchema, { namePt, level: 1 }) });
  return create(TableEntrySchema, {
    key: over.key ?? `${prefix}:${slug}@mesa`,
    kind,
    namePt,
    archived: over.archived ?? false,
    charactersUsing: over.charactersUsing ?? 0,
    revision: 3,
    body,
  });
}

/** The seven entries of the artboard (E10-01): two classes, one archived, two subclasses, a race, a background, a spell. */
export function mirathel(): TableEntry[] {
  return [
    entry(TableContentKind.CLASS, 'Guardião do Vale', { charactersUsing: 2 }),
    entry(TableContentKind.CLASS, 'Bardo das Cinzas', { archived: true, charactersUsing: 1 }),
    entry(TableContentKind.SUBCLASS, 'Tradição da Tinta'),
    entry(TableContentKind.SUBCLASS, 'Domínio do Caminho'),
    entry(TableContentKind.RACE, 'Corujeiro', { charactersUsing: 2 }),
    entry(TableContentKind.BACKGROUND, 'Cartógrafo do Vale'),
    entry(TableContentKind.SPELL, 'Lâmina de Nanquim', { charactersUsing: 1 }),
  ];
}

export function feature(
  name: string,
  effects: Parameters<typeof create<typeof TableEffectSchema>>[1][] = [],
) {
  return create(TableFeatureSchema, {
    key: `feature:${name}`,
    namePt: name,
    descPt: ['Texto.'],
    effects: effects.map((e) => create(TableEffectSchema, e)),
  });
}

/** The six abilities as the catalog names them, for the specs. */
export const abilities: readonly CatalogAbility[] = (
  ['strength', 'dexterity', 'constitution', 'intelligence', 'wisdom', 'charisma'] as const
).map((field, i) => ({
  field,
  ability: (i + 1) as Ability,
  name: ['Força', 'Destreza', 'Constituição', 'Inteligência', 'Sabedoria', 'Carisma'][i],
}));

/** A catalog like the server's, with what a table entry points at named (languages, proficiencies, damage types, skills). */
export function catalog(extra: Parameters<typeof catalogVm>[1] = []): CatalogVm {
  return catalogVm(
    create(ContentSchema, {
      abilities: abilities.map((a) => ({ ability: a.ability, namePt: a.name })),
      races: [{ key: 'race:human', namePt: 'Humano' }],
      classes: [
        { key: 'class:wizard', namePt: 'Mago', subclassLevel: 2, spellcasting: { ability: 4 } },
        { key: 'class:paladin', namePt: 'Paladino', subclassLevel: 3 },
      ],
      skills: [
        { key: 'skill:arcana', namePt: 'Arcanismo' },
        { key: 'skill:perception', namePt: 'Percepção' },
        { key: 'skill:investigation', namePt: 'Investigação' },
      ],
      spells: [
        {
          key: 'spell:light',
          namePt: 'Luz',
          schoolKey: 'school:evocation',
          schoolNamePt: 'Evocação',
        },
      ],
      languages: [
        { key: 'language:common', namePt: 'Comum' },
        { key: 'language:primordial', namePt: 'Primordial' },
      ],
      proficiencies: [
        { key: 'proficiency:cartographers-tools', namePt: 'Ferramentas de cartógrafo' },
        { key: 'proficiency:light-armor', namePt: 'Armadura leve' },
        { key: 'proficiency:medium-armor', namePt: 'Armadura média' },
        { key: 'proficiency:shields', namePt: 'Escudos' },
        { key: 'proficiency:simple-weapons', namePt: 'Armas simples' },
        { key: 'proficiency:martial-weapons', namePt: 'Armas marciais' },
      ],
      damageTypes: [
        { key: 'damage-type:necrotic', namePt: 'necrótico' },
        { key: 'damage-type:fire', namePt: 'fogo' },
      ],
    }),
    extra,
  );
}

/**
 * A stand-in for the page's `ContentWatcher` (RN-23, the live content hint): `provider` goes in `providers` (or
 * `TestBed.overrideComponent(Page, { set: { providers: [provider] } })`), and `hint()` plays a `content_changed` that arrived
 * (what the debounce lets through). `following()` is whether the page asked to follow the campaign's session.
 */
export function fakeContentWatcher() {
  let onChange: (() => void) | null = null;
  let id: (() => string) | null = null;
  return {
    provider: {
      provide: ContentWatcher,
      useValue: {
        whileLive: (campaignId: () => string, cb: () => void) => {
          id = campaignId;
          onChange = cb;
        },
        follow: () => undefined,
      },
    },
    hint: () => onChange?.(),
    following: () => id?.() ?? '',
  };
}

/** The levels of a class table, the spell levels a slot row has, and the level a prof bonus step takes. */
const CLASS_LEVELS = 20;
const SPELL_LEVELS = 9;
const PROF_STEP = 4;
/** Numbers as a space separated text: an array literal of them would be all magic numbers. */
const numbers = (text: string): number[] => text.split(' ').map(Number);
const ASI_LEVELS = numbers('4 8 12 16 19');
const THIRD_CASTER_START = 3;
/** Slots by level, as "from level:slots" steps: the last step that starts at or below the level wins. */
const slotLadder =
  (steps: string) =>
  (level: number): number[] =>
    steps
      .split('|')
      .map((step) => step.split(':'))
      .filter(([from]) => level >= Number(from))
      .map(([, slots]) => numbers(slots))
      .pop() ?? [];
/** A few cantrips and spells known, as the rows carry them. */
const CANTRIPS = { cleric: 3, sorcerer: 4, pact: 2, third: 2 };
const KNOWN = { sorcerer: 2, ranger: 2, pact: 2, third: 3 };

/** `GetClassTableDefaults` with the shape of the server's: the proficiency bonus, the ASI levels and one table per way of casting.
 * The slots are a few of the SRD's (the half caster of the paladin at 2, 5 and 9); the specs prove what the editor does with the
 * rows, not the numbers. */
export function classDefaults(): GetClassTableDefaultsResponse {
  const prof = (i: number) => 2 + Math.floor(i / PROF_STEP);
  const rows = (start: number, slotsAt: (level: number) => number[], cantrips = 0, known = 0) =>
    Array.from({ length: CLASS_LEVELS }, (_, i) => {
      const level = i + 1;
      const casts = start > 0 && level >= start;
      return {
        profBonus: prof(i),
        features: [],
        cantripsKnown: casts ? cantrips : 0,
        spellsKnown: casts ? known : 0,
        slots: casts
          ? [...slotsAt(level), ...Array<number>(SPELL_LEVELS - slotsAt(level).length).fill(0)]
          : [],
      };
    });
  const half = slotLadder('0:2|3:3|5:4 2|9:4 3 2');
  const full = slotLadder('0:2|2:3|3:4 2|5:4 3 3');
  const pact = slotLadder('0:0 0 2|5:0 0 4');
  const third = slotLadder('0:2|7:4 2');
  return create(GetClassTableDefaultsResponseSchema, {
    profBonus: Array.from({ length: CLASS_LEVELS }, (_, i) => prof(i)),
    asiLevels: ASI_LEVELS,
    subclassLevel: THIRD_CASTER_START,
    tables: [
      { kind: '', preparation: '', startLevel: 0, referenceClassKey: '', rows: rows(0, () => []) },
      {
        kind: 'full',
        preparation: 'prepared',
        startLevel: 1,
        referenceClassKey: 'class:cleric',
        rows: rows(1, full, CANTRIPS.cleric),
      },
      {
        kind: 'full',
        preparation: 'known',
        startLevel: 1,
        referenceClassKey: 'class:sorcerer',
        rows: rows(1, full, CANTRIPS.sorcerer, KNOWN.sorcerer),
      },
      {
        kind: 'half',
        preparation: 'prepared',
        startLevel: 2,
        referenceClassKey: 'class:paladin',
        rows: rows(2, half),
      },
      {
        kind: 'half',
        preparation: 'known',
        startLevel: 2,
        referenceClassKey: 'class:ranger',
        rows: rows(2, half, 0, KNOWN.ranger),
      },
      {
        kind: 'pact',
        preparation: 'known',
        startLevel: 1,
        referenceClassKey: 'class:warlock',
        rows: rows(1, pact, CANTRIPS.pact, KNOWN.pact),
      },
      {
        kind: 'pact',
        preparation: 'prepared',
        startLevel: 1,
        referenceClassKey: 'class:warlock',
        rows: rows(1, pact, CANTRIPS.pact),
      },
      {
        kind: 'third',
        preparation: 'known',
        startLevel: THIRD_CASTER_START,
        referenceClassKey: '',
        rows: rows(THIRD_CASTER_START, third, CANTRIPS.third, KNOWN.third),
      },
      {
        kind: 'third',
        preparation: 'prepared',
        startLevel: THIRD_CASTER_START,
        referenceClassKey: '',
        rows: rows(THIRD_CASTER_START, third, CANTRIPS.third),
      },
    ],
  });
}
