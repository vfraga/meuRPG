import { create } from '@bufbuild/protobuf';

import {
  Ability,
  FeatPrerequisiteSchema,
  FeatUnmetKind,
  FeatUnmetSchema,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { prerequisiteParts, prerequisiteSupport, unmetText } from './feat-text';

const HIDDEN_LINE = 'Pede uma condição que a mesa não mostra.';

const names: Record<string, string> = {
  'proficiency:medium-armor': 'Armadura média',
  'race:dwarf': 'Anão',
  'race:elf': 'Elfo',
  'trait:darkvision': 'Visão no Escuro',
};
const nameOf = (key: string) => names[key] ?? key;

describe('what a feat asks, in Portuguese', () => {
  it('writes every condition of a prerequisite, in the sheet order', () => {
    const p = create(FeatPrerequisiteSchema, {
      minimums: { dexterity: 13, strength: 13 },
      anyOf: { intelligence: 13, wisdom: 13 },
      proficiencyKey: 'proficiency:medium-armor',
      spellcasting: true,
      raceKey: 'race:dwarf',
      level: 4,
    });
    expect(prerequisiteParts(p, nameOf)).toEqual([
      'Força 13 e Destreza 13',
      'Inteligência 13 ou Sabedoria 13 (uma delas)',
      'proficiência em Armadura média',
      'poder conjurar ao menos uma magia',
      'ser Anão',
      'nível 4 ou mais',
    ]);
    expect(prerequisiteSupport(p, nameOf)).toContain(
      'Força 13 e Destreza 13, Inteligência 13 ou Sabedoria 13 (uma delas), ',
    );
  });

  it('writes the races to choose from, the size and the feature to lack', () => {
    const p = create(FeatPrerequisiteSchema, {
      races: ['race:elf', 'race:dwarf'],
      size: 'Small',
      lacksFeatureKey: 'trait:darkvision',
      level: 2,
    });
    expect(prerequisiteParts(p, nameOf)).toEqual([
      'ser Elfo ou Anão',
      'ter tamanho pequeno',
      'não ter Visão no Escuro',
      'nível 2 ou mais',
    ]);
    // The single race_key is read together with the list, each race once.
    expect(
      prerequisiteParts(
        create(FeatPrerequisiteSchema, { raceKey: 'race:elf', races: ['race:elf', 'race:dwarf'] }),
        nameOf,
      ),
    ).toEqual(['ser Elfo ou Anão']);
  });

  it('says nothing for a feat that asks nothing', () => {
    expect(prerequisiteParts(undefined, nameOf)).toEqual([]);
    expect(prerequisiteSupport(create(FeatPrerequisiteSchema), nameOf)).toBe('');
  });

  it("writes what a character lacks from the server's codes", () => {
    const unmet = (init: Parameters<typeof create<typeof FeatUnmetSchema>>[1]) =>
      unmetText(create(FeatUnmetSchema, init), nameOf);
    expect(
      unmet({
        kind: FeatUnmetKind.ABILITY_MINIMUM,
        abilities: [{ ability: Ability.STRENGTH, minimum: 13 }],
      }),
    ).toBe('Precisa de Força 13.');
    expect(
      unmet({
        kind: FeatUnmetKind.ABILITY_ANY_OF,
        abilities: [
          { ability: Ability.INTELLIGENCE, minimum: 13 },
          { ability: Ability.WISDOM, minimum: 13 },
        ],
      }),
    ).toBe('Precisa de Inteligência 13 ou Sabedoria 13.');
    expect(unmet({ kind: FeatUnmetKind.PROFICIENCY, key: 'proficiency:medium-armor' })).toBe(
      'Precisa de proficiência em Armadura média.',
    );
    expect(unmet({ kind: FeatUnmetKind.SPELLCASTING })).toBe('Precisa conseguir conjurar magias.');
    expect(unmet({ kind: FeatUnmetKind.RACE, key: 'race:dwarf' })).toBe('Só para Anão.');
    expect(unmet({ kind: FeatUnmetKind.RACE, keys: ['race:elf', 'race:dwarf'] })).toBe(
      'Só para Elfo ou Anão.',
    );
    expect(unmet({ kind: FeatUnmetKind.SIZE, key: 'Medium' })).toBe('Só para tamanho médio.');
    expect(unmet({ kind: FeatUnmetKind.LACKS_FEATURE, key: 'trait:darkvision' })).toBe(
      'Não pode ter Visão no Escuro.',
    );
    expect(unmet({ kind: FeatUnmetKind.LACKS_FEATURE, key: 'x', namePt: 'Dom do Mar' })).toBe(
      'Não pode ter Dom do Mar.',
    );
    // What the players cannot see is blanked by the server: no empty name is written.
    expect(unmet({ kind: FeatUnmetKind.LACKS_FEATURE })).toBe('Não atende ao pré-requisito.');
    expect(unmet({ kind: FeatUnmetKind.RACE })).toBe('Não atende ao pré-requisito.');
    expect(unmet({ kind: FeatUnmetKind.LACKS_FEATURE, hidden: true })).toBe(HIDDEN_LINE);
    expect(unmet({ kind: FeatUnmetKind.RACE, hidden: true })).toBe(HIDDEN_LINE);
    expect(prerequisiteParts(create(FeatPrerequisiteSchema, { size: 'Large' }), nameOf)).toEqual([
      'ter tamanho grande',
    ]);
    expect(unmet({ kind: FeatUnmetKind.RACE, keys: ['race:elf', 'race:dwarf', 'race:elf'] })).toBe(
      'Só para Elfo, Anão ou Elfo.',
    );
    expect(unmet({ kind: FeatUnmetKind.LEVEL, value: 8 })).toBe('Precisa de nível 8 ou mais.');
  });

  it('writes the feature to lack as the player sees it: by name, or as a hidden condition with no key', () => {
    const p = create(FeatPrerequisiteSchema, {
      minimums: { dexterity: 13 },
      lacksFeatureKey: 'feat:grappler',
    });
    expect(prerequisiteSupport(p, nameOf, { name: 'Agarrador', hidden: false })).toBe(
      'Destreza 13 e não ter Agarrador',
    );
    const hidden = create(FeatPrerequisiteSchema, { minimums: { dexterity: 13 } });
    const line = prerequisiteSupport(hidden, nameOf, { name: '', hidden: true });
    expect(line).toBe('Destreza 13 e uma condição que a mesa não mostra');
    expect(line).not.toMatch(/feat:|trait:/);
  });

  it("reads the server's lacks_hidden mark as the hidden condition, with no key", () => {
    const p = create(FeatPrerequisiteSchema, { lacksHidden: true });
    expect(prerequisiteSupport(p, nameOf)).toBe('uma condição que a mesa não mostra');
  });

  it("names the feature to lack with the server's name, never the key", () => {
    const p = create(FeatPrerequisiteSchema, {
      lacksFeatureKey: 'feat:grappler',
      lacksFeatureNamePt: 'Agarrador',
    });
    expect(prerequisiteSupport(p, (k) => k)).toBe('não ter Agarrador');
    const bare = create(FeatPrerequisiteSchema, { lacksFeatureKey: 'feat:grappler' });
    expect(prerequisiteSupport(bare, () => 'Agarrador')).toBe('não ter Agarrador');
  });

  it('reads races_hidden as the hidden condition, said once with a hidden lacked feature', () => {
    const races = create(FeatPrerequisiteSchema, { racesHidden: true, level: 4 });
    expect(prerequisiteSupport(races, nameOf)).toBe(
      'uma condição que a mesa não mostra e nível 4 ou mais',
    );
    const both = create(FeatPrerequisiteSchema, { racesHidden: true, lacksHidden: true });
    expect(prerequisiteSupport(both, nameOf)).toBe('uma condição que a mesa não mostra');
  });

  it("writes the whole prerequisite of the master's board in one line", () => {
    const p = create(FeatPrerequisiteSchema, {
      minimums: { dexterity: 13 },
      races: ['race:elf', 'race:dwarf'],
      size: 'Small',
      lacksFeatureKey: 'trait:darkvision',
    });
    expect(prerequisiteSupport(p, nameOf)).toBe(
      'Destreza 13, ser Elfo ou Anão, ter tamanho pequeno e não ter Visão no Escuro',
    );
  });
});
