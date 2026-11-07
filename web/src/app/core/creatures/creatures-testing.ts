import { Code, ConnectError } from '@connectrpc/connect';
import { vi } from 'vitest';
import { create } from '@bufbuild/protobuf';

import {
  type CharacterCreature,
  CharacterSchema,
  InvalidFieldSchema,
  CharacterCreatureSchema,
  CreatureSource,
  type GetSummonOptionsResponse,
  GetSummonOptionsResponseSchema,
  type SummonSpellOptions,
  SummonSpellOptionsSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { SpellSlotUsageSchema, CharacterVitalsSchema } from '../../../gen/meurpg/play/v1/play_pb';
import {
  Ability,
  type Creature,
  CreatureSize,
  CreatureAbilityScoreSchema,
  CreatureActionSchema,
  type CreatureSummary,
  CreatureSchema,
  CreatureSummarySchema,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import type { CreatureFilter, SummonCast } from './creatures-client';

type Init<T> = Partial<Omit<T, '$typeName' | '$unknown'>>;

/** Convocar Familiar for a wizard: a ritual, no slot, one creature out of the given forms (key, name, none attack). */
export function familiarSpell(
  forms: [string, string][] = [
    ['monster:raven', 'Corvo'],
    ['monster:cat', 'Gato'],
    ['monster:bat', 'Morcego'],
  ],
  over: Init<SummonSpellOptions> = {},
): SummonSpellOptions {
  return create(SummonSpellOptionsSchema, {
    spellKey: 'spell:find-familiar',
    namePt: 'Convocar Familiar',
    level: 1,
    castingTimePt: '1 hora',
    ritual: true,
    canRitual: true,
    circles: [
      {
        circle: 1,
        options: [
          {
            count: 1,
            attack: 1,
            forms: forms.map(([monsterKey, namePt]) => ({ monsterKey, namePt, attack: 1 })),
          },
        ],
      },
    ],
    ...over,
  });
}

/** Animar Mortos for a cleric with slots of the 3rd and the 5th circle: 1 and 5 undead. */
export function undeadSpell(over: Init<SummonSpellOptions> = {}): SummonSpellOptions {
  const forms = [
    { monsterKey: 'monster:skeleton', namePt: 'Esqueleto', attack: 3 },
    { monsterKey: 'monster:zombie', namePt: 'Zumbi', attack: 3 },
  ];
  return create(SummonSpellOptionsSchema, {
    spellKey: 'spell:animate-dead',
    namePt: 'Animar Mortos',
    level: 3,
    castingTimePt: '1 minuto',
    canCastWithSlot: true,
    circles: [
      { circle: 3, options: [{ count: 1, forms }] },
      { circle: 5, options: [{ count: 5, forms }] },
    ],
    ...over,
  });
}

/** Conjurar Animais for a druid: four options by challenge rating, at the 3rd circle. */
export function beastSpell(over: Init<SummonSpellOptions> = {}): SummonSpellOptions {
  return create(SummonSpellOptionsSchema, {
    spellKey: 'spell:conjure-animals',
    namePt: 'Conjurar Animais',
    level: 3,
    castingTimePt: '1 ação',
    concentration: true,
    canCastWithSlot: true,
    circles: [
      {
        circle: 3,
        options: [
          { count: 1, type: 'beast', maxCr: '2', attack: 3 },
          { count: 2, type: 'beast', maxCr: '1', attack: 3 },
          { count: 4, type: 'beast', maxCr: '1/2', attack: 3 },
          { count: 8, type: 'beast', maxCr: '1/4', attack: 3 },
        ],
      },
    ],
    ...over,
  });
}

/** What `GetSummonOptions` answers: the spells and the slots (level, total, free, pact). */
export function summonAnswer(
  spells: SummonSpellOptions[],
  slots: [number, number, number, boolean?][] = [],
): GetSummonOptionsResponse {
  return create(GetSummonOptionsResponseSchema, {
    spells,
    slots: slots.map(([level, total, free, pact]) => ({ level, total, free, pact: pact ?? false })),
  });
}

/** A creature of a character, as `ListCharacterCreatures` sends it. */
export function creature(
  id: string,
  name: string,
  over: Init<CharacterCreature> = {},
): CharacterCreature {
  return create(CharacterCreatureSchema, {
    id,
    characterId: 'char-1',
    monsterKey: 'monster:raven',
    monsterNamePt: 'Corvo',
    name,
    source: CreatureSource.FAMILIAR,
    attack: 1,
    hitPointsCurrent: 1,
    hitPointsMax: 1,
    ...over,
  });
}

/** A challenge rating as a number, to compare: "1/8" is 0.125. */
function crValue(cr: string): number {
  const [a, b] = cr.split('/');
  return b ? Number(a) / Number(b) : Number(a);
}

/** A catalog row. */
export function summary(
  key: string,
  namePt: string,
  over: Init<CreatureSummary> = {},
): CreatureSummary {
  return create(CreatureSummarySchema, {
    key,
    name: key.replace('monster:', ''),
    namePt,
    size: 'Tiny',
    sizePt: 'Miúdo',
    type: 'beast',
    typePt: 'fera',
    challengeRating: '0',
    ...over,
  });
}

/** The raven's stat block, with the numbers of the artboard (E9-10). */
export function raven(over: Init<Creature> = {}): Creature {
  const abilities = [2, 14, 8, 2, 12, 6].map((score, i) =>
    create(CreatureAbilityScoreSchema, {
      ability: (i + 1) as Ability,
      namePt: ['Força', 'Destreza', 'Constituição', 'Inteligência', 'Sabedoria', 'Carisma'][i],
      score,
      modifier: Math.floor((score - 10) / 2),
    }),
  );
  return create(CreatureSchema, {
    summary: summary('monster:raven', 'Corvo'),
    armorClass: 12,
    hitPoints: 1,
    hitPointsRoll: '1d4-1',
    speedWalkFt: 10,
    speedFlyFt: 50,
    abilities,
    skills: [{ key: 'skill:perception', namePt: 'Percepção', bonus: 3 }],
    passivePerception: 13,
    traits: [
      { name: 'Mimicry', text: 'The raven can mimic simple sounds it has heard.', usage: '' },
    ],
    actions: [
      create(CreatureActionSchema, {
        name: 'Beak',
        text: 'Melee Weapon Attack: +4 to hit, reach 5 ft., one target. Hit: 1 piercing damage.',
        hasAttack: true,
        attackBonus: 4,
      }),
    ],
    ...over,
  });
}

/** Vitals with the slots the cast sheet reads. */
export function vitalsWith(
  characterId: string,
  slots: { level: number; total: number; used: number }[],
) {
  return create(CharacterVitalsSchema, {
    characterId,
    spellSlots: slots.map((s) => create(SpellSlotUsageSchema, s)),
  });
}

/** The client of the creatures, with what each test needs and a record of what was called. */
export class FakeCreaturesClient {
  creatures: CharacterCreature[] = [];
  blocks = new Map<string, Creature>();
  catalog: CreatureSummary[] = [];
  options: GetSummonOptionsResponse = summonAnswer([]);
  vitals = vitalsWith('char-1', []);
  calls: string[] = [];
  casts: SummonCast[] = [];
  searches: CreatureFilter[] = [];
  failWith: unknown = null;

  list = vi.fn(async (_campaignId: string, _characterId: string) => {
    this.calls.push('list');
    if (this.failWith) {
      throw this.failWith;
    }
    return this.creatures;
  });
  give = vi.fn(async (_c: string, characterId: string, monsterKey: string, name: string) => {
    this.calls.push(`give ${monsterKey} ${name}`);
    if (this.failWith) {
      throw this.failWith;
    }
    const made = creature('new', name, { monsterKey, source: CreatureSource.MASTER, characterId });
    this.creatures = [...this.creatures, made];
    return made;
  });
  rename = vi.fn(async (_c: string, id: string, name: string) => {
    this.calls.push(`rename ${id} ${name}`);
    if (this.failWith) {
      throw this.failWith;
    }
    this.creatures = this.creatures.map((c) => (c.id === id ? { ...c, name } : c));
    return this.creatures.find((c) => c.id === id);
  });
  dismiss = vi.fn(async (_c: string, id: string) => {
    this.calls.push(`dismiss ${id}`);
    if (this.failWith) {
      throw this.failWith;
    }
    this.creatures = this.creatures.filter((c) => c.id !== id);
  });
  setHitPoints = vi.fn(async (_c: string, id: string, hp: number) => {
    this.calls.push(`hp ${id} ${hp}`);
    return this.creatures.find((c) => c.id === id);
  });
  search = vi.fn(async (_c: string, filter: CreatureFilter) => {
    this.searches.push(filter);
    const q = (filter.query ?? '').toLowerCase();
    const found = this.catalog.filter(
      (s) =>
        (q === '' || s.namePt.toLowerCase().includes(q) || s.name.toLowerCase().includes(q)) &&
        (!filter.type || s.type === filter.type) &&
        (!filter.size ||
          s.size.toLowerCase() === (CreatureSize[filter.size] ?? '').toLowerCase()) &&
        (!filter.minCr || crValue(s.challengeRating) >= crValue(filter.minCr)) &&
        (!filter.maxCr || crValue(s.challengeRating) <= crValue(filter.maxCr)),
    );
    if (this.searchFail) {
      throw this.searchFail;
    }
    return { creatures: found, total: found.length };
  });
  /** An error every `search` throws, while set. */
  searchFail: unknown = null;
  /** What each `createNpc` was asked: the creature, the name, the role and the key. */
  npcCalls: { creatureKey: string; name: string; role: string; key: string }[] = [];
  /** Errors for the next `createNpc` calls, one per call, then it works. */
  npcFailures: unknown[] = [];
  createNpc = vi.fn(
    async (_c: string, creatureKey: string, name: string, role: string, key: string) => {
      this.npcCalls.push({ creatureKey, name, role, key });
      const failure = this.npcFailures.shift();
      if (failure) {
        throw failure;
      }
      return create(CharacterSchema, {
        id: 'npc-1',
        name,
        sheet: {
          content: {
            case: 'basic',
            value: { attacks: [{ name: 'Clava grande' }, { name: 'Azagaia' }] },
          },
        },
      });
    },
  );
  statBlock = vi.fn(async (_c: string, key: string) => {
    const b = this.blocks.get(key);
    if (!b) {
      throw new Error('no stat block');
    }
    return b;
  });
  /** The stat blocks of a character's creature (by its id): what the server worked out for its owner. */
  ownBlocks = new Map<string, Creature>();
  creatureBlock = vi.fn(async (_c: string, _key: string, id: string) => {
    const b = this.ownBlocks.get(id);
    if (!b) {
      throw new Error('no stat block');
    }
    return b;
  });
  vitalsOf = vi.fn(async () => this.vitals);
  summonOptions = vi.fn(async (_c: string, _ch: string) => {
    this.calls.push('options');
    if (this.optionsFail) {
      throw this.optionsFail;
    }
    return this.options;
  });
  optionsFail: unknown = null;
  forms: CreatureSummary[] = [];
  wild = { maxCr: '1/2', noFly: true, noSwim: false };
  assumed: string[] = [];
  wildShapeForms = vi.fn(async (_c: string, _ch: string) => ({ forms: this.forms, ...this.wild }));
  assumeWildShape = vi.fn(async (_c: string, _ch: string, beastKey: string, _k: string) => {
    this.assumed.push(beastKey);
    if (this.failWith) {
      throw this.failWith;
    }
    return { vitals: undefined, encounter: undefined };
  });
  leaveWildShape = vi.fn(async () => ({ vitals: undefined, encounter: undefined }));
  castSummon = vi.fn(async (cast: SummonCast) => {
    this.casts.push(cast);
    if (this.failWith) {
      throw this.failWith;
    }
    return { creatureIds: ['new'], replacedIds: [] as string[] };
  });
}

/**
 * What a person reads in an element: the icons' ligature names are left out, and the
 * boxes the stylesheet puts side by side or one under the other are told apart with a
 * space ("CA 12", "Força −4 valor 2"), then the spaces are collapsed.
 */
export function flat(e: Element | null | undefined): string | undefined {
  if (!e) {
    return undefined;
  }
  const copy = e.cloneNode(true) as Element;
  copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
  copy.querySelectorAll('*').forEach((n) => {
    n.insertAdjacentText('beforebegin', ' ');
    n.insertAdjacentText('afterend', ' ');
  });
  return (copy.textContent ?? '')
    .replace(/\s+/g, ' ')
    .replace(/ ([.,:;)])/g, '$1')
    .replace(/\( /g, '(')
    .trim();
}

/** Whether a button is off: a button that stays focusable says it with `aria-disabled`. */
export function isOff(b: HTMLButtonElement): boolean {
  return b.disabled || b.getAttribute('aria-disabled') === 'true';
}

/** The Ogre's stat block, with the numbers of the artboard (E10-08, state 3). */
export function ogre(over: Init<Creature> = {}): Creature {
  const scores = [19, 8, 16, 5, 7, 7];
  const names = ['Força', 'Destreza', 'Constituição', 'Inteligência', 'Sabedoria', 'Carisma'];
  return create(CreatureSchema, {
    summary: summary('monster:ogre', 'Ogro', {
      name: 'Ogre',
      size: 'Large',
      sizePt: 'Grande',
      type: 'giant',
      typePt: 'gigante',
      challengeRating: '2',
      xp: 450,
      armorClass: 11,
      hitPoints: 59,
    }),
    alignment: 'chaotic evil',
    armorClass: 11,
    armorClassLabelPt: 'Armadura',
    armorClassNote: 'gibão de peles',
    hitPoints: 59,
    hitDice: '7d10',
    hitPointsRoll: '7d10+21',
    speedWalkFt: 40,
    abilities: scores.map((score, i) =>
      create(CreatureAbilityScoreSchema, {
        ability: (i + 1) as Ability,
        namePt: names[i],
        score,
        modifier: Math.floor((score - 10) / 2),
      }),
    ),
    senses: [{ namePt: 'Visão no escuro', rangeFt: 60 }],
    passivePerception: 8,
    languages: 'Common, Giant',
    proficiencyBonus: 2,
    npcAttackNames: ['Clava grande', 'Azagaia'],
    actions: [
      create(CreatureActionSchema, {
        name: 'Greatclub',
        namePt: 'Clava grande',
        text: 'Melee Weapon Attack: +6 to hit, reach 5 ft., one target. Hit: 13 (2d8 + 4) bludgeoning damage.',
        hasAttack: true,
        attackBonus: 6,
        damage: [
          { dice: '2d8+4', damageTypeKey: 'damage-type:bludgeoning', damageTypePt: 'contundente' },
        ],
      }),
      create(CreatureActionSchema, {
        name: 'Javelin',
        namePt: 'Azagaia',
        text: 'Melee or Ranged Weapon Attack: +6 to hit, reach 5 ft. or range 30/120 ft., one target. Hit: 11 (2d6 + 4) piercing damage.',
        hasAttack: true,
        attackBonus: 6,
        damage: [
          { dice: '2d6+4', damageTypeKey: 'damage-type:piercing', damageTypePt: 'perfurante' },
        ],
      }),
    ],
    ...over,
  });
}

/** A refused request as the server sends it: `invalid_argument` with the `InvalidField` detail naming the field. */
export function invalidField(field: string): ConnectError {
  return new ConnectError('refused', Code.InvalidArgument, undefined, [
    { desc: InvalidFieldSchema, value: create(InvalidFieldSchema, { field }) },
  ]);
}
