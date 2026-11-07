import { create } from '@bufbuild/protobuf';

import { CreatureSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  baseModsText,
  duplicateText,
  emptyDamageMod,
  listWords,
  modSub,
  modTitle,
  previewMods,
  qualifierWhy,
} from './creature-damage-mods';
import { creaturePreview } from './creature-preview';
import { emptyCreature } from './creature-draft';

const NAMES: Record<string, string> = {
  'damage-type:bludgeoning': 'Concussão',
  'damage-type:piercing': 'perfurante',
  'damage-type:slashing': 'cortante',
  'damage-type:necrotic': 'necrótico',
  'damage-type:poison': 'veneno',
  'damage-type:radiant': 'radiante',
};
const nameOf = (k: string) => NAMES[k] ?? k;
const row = (kind: string, types: string[], qualifier = '') => ({
  ...emptyDamageMod(),
  kind,
  types,
  qualifier,
});
const PHYSICAL = ['damage-type:bludgeoning', 'damage-type:piercing', 'damage-type:slashing'];

const zombie = create(CreatureSchema, {
  immunities: [{ types: [{ key: 'damage-type:poison', namePt: 'Veneno' }] }],
  resistances: [{ note: 'from nonmagical attacks' }],
});

describe('the resistance rows of a table creature (#585)', () => {
  it('turns "Vale contra" off for a vulnerability and for a damage type that is not physical', () => {
    expect(qualifierWhy('resist', PHYSICAL)).toBe('');
    expect(qualifierWhy('immune', ['damage-type:slashing'])).toBe('');
    expect(qualifierWhy('vulnerable', PHYSICAL)).toContain('Só resistência ou imunidade');
    expect(qualifierWhy('resist', ['damage-type:slashing', 'damage-type:necrotic'])).toContain(
      'Só resistência ou imunidade',
    );
    // No type yet: nothing is against it.
    expect(qualifierWhy('resist', [])).toBe('');
  });

  it('says a row in one line and the line under it', () => {
    const m = row('resist', PHYSICAL, 'nonmagical_unless_silvered');
    expect(modTitle(m, nameOf)).toBe('Resistência a concussão, perfurante e cortante');
    expect(modSub(m)).toBe('de ataque não mágico nem prateado');
    expect(modSub(row('resist', PHYSICAL, 'nonmagical'))).toBe('de ataque não mágico');
    expect(modSub(row('resist', PHYSICAL, 'nonmagical_unless_adamantine'))).toBe(
      'de ataque não mágico nem adamantino',
    );
    expect(modSub(row('resist', PHYSICAL))).toBe('Todo dano desses tipos');
    expect(modTitle(row('vulnerable', ['damage-type:radiant']), nameOf)).toBe(
      'Vulnerabilidade a radiante',
    );
    expect(modSub(row('vulnerable', ['damage-type:radiant']))).toBe('Todo dano desse tipo');
    expect(modTitle(row('immune', []), nameOf)).toBe('Imunidade');
  });

  it('names the damage type a row holds twice', () => {
    expect(duplicateText(['damage-type:necrotic', 'damage-type:necrotic'], nameOf)).toBe(
      'Necrótico está duas vezes. Tire o repetido.',
    );
    expect(duplicateText(['damage-type:fire', 'damage-type:necrotic'], nameOf)).toBe('');
  });

  it('joins words with commas and an "e"', () => {
    expect(listWords([])).toBe('');
    expect(listWords(['a'])).toBe('a');
    expect(listWords(['a', 'b'])).toBe('a e b');
    expect(listWords(['a', 'b', 'c'])).toBe('a, b e c');
  });

  it('writes the preview with the table first and the base after, one line for each kind', () => {
    const lines = previewMods(
      [
        row('resist', PHYSICAL, 'nonmagical_unless_silvered'),
        row('vulnerable', ['damage-type:radiant']),
        row('immune', ['damage-type:necrotic']),
        row('immune', []),
      ],
      zombie,
      nameOf,
    );
    expect(lines.map((l) => l.kind)).toEqual(['Resistência', 'Vulnerabilidade', 'Imunidade']);
    expect(lines[0].parts).toEqual([
      {
        text: 'a concussão, perfurante e cortante de ataque não mágico nem prateado',
        origin: 'da mesa',
      },
      { text: 'from nonmagical attacks', origin: 'da base' },
    ]);
    expect(lines[2].parts).toEqual([
      { text: 'a necrótico', origin: 'da mesa' },
      { text: 'a veneno', origin: 'da base' },
    ]);
    expect(previewMods([], null, nameOf)).toEqual([]);
  });

  it('says what the base already has', () => {
    expect(baseModsText(zombie)).toBe('resistência from nonmagical attacks; imunidade a veneno');
    expect(baseModsText(null)).toBe('');
  });

  it('puts the lines in the preview of the creature', () => {
    const d = {
      ...emptyCreature(),
      baseKey: 'monster:zombie',
      damageMods: [row('immune', ['damage-type:necrotic'])],
    };
    expect(creaturePreview(d, zombie, 'Zumbi', nameOf).mods.map((l) => l.kind)).toEqual([
      'Resistência',
      'Imunidade',
    ]);
    expect(creaturePreview(emptyCreature(), null, '', nameOf).mods).toEqual([]);
  });
});
