import { powerAttackReading } from './power-attack-reading';

const NAMES: Readonly<Record<string, string>> = {
  'weapon-property:heavy': 'Pesada',
  'weapon-property:two-handed': 'Duas mãos',
  'weapon-property:light': 'Leve',
  'weapon-property:ammunition': 'Munição',
};
const names = {
  property: (key: string) => NAMES[key] ?? key,
  resource: (key: string) => (key === 'folego_do_atirador' ? 'Fôlego do Atirador' : key),
};
const read = (
  over: Partial<{
    weapon: string;
    weaponProperties: string[];
    onlyProficient: boolean;
    resource: string;
  }>,
) =>
  powerAttackReading(
    { weapon: '', weaponProperties: [], onlyProficient: false, resource: '', ...over },
    names,
  );

describe('powerAttackReading: the "Vale para:" line, the filter read back in words', () => {
  it('reads no filter as any weapon, proficient or not', () => {
    expect(read({})).toBe('qualquer arma, mesmo uma em que o personagem não tem proficiência.');
    expect(read({ weapon: 'any' })).toBe(read({}));
  });

  it('reads the weapon kind', () => {
    expect(read({ weapon: 'melee' })).toBe(
      'qualquer arma corpo a corpo, mesmo uma em que o personagem não tem proficiência.',
    );
    expect(read({ weapon: 'ranged' })).toBe(
      'qualquer arma à distância, mesmo uma em que o personagem não tem proficiência.',
    );
  });

  it('reads the proficiency switch', () => {
    expect(read({ weapon: 'melee', onlyProficient: true })).toBe(
      'qualquer arma corpo a corpo em que o personagem tem proficiência.',
    );
    expect(read({ onlyProficient: true })).toBe(
      'qualquer arma em que o personagem tem proficiência.',
    );
  });

  it('reads one property, the kind and the proficiency together', () => {
    expect(
      read({
        weapon: 'melee',
        weaponProperties: ['weapon-property:heavy'],
        onlyProficient: true,
      }),
    ).toBe('armas corpo a corpo, com a propriedade Pesada, em que o personagem tem proficiência.');
  });

  it('reads several properties as a list in words, with the names of the menu', () => {
    expect(
      read({
        weapon: 'melee',
        weaponProperties: ['weapon-property:heavy', 'weapon-property:two-handed'],
      }),
    ).toBe(
      'armas corpo a corpo, com as propriedades Pesada e Duas mãos, mesmo as em que o personagem não tem proficiência.',
    );
    expect(
      read({
        weaponProperties: [
          'weapon-property:heavy',
          'weapon-property:light',
          'weapon-property:two-handed',
        ],
        onlyProficient: true,
      }),
    ).toBe(
      'armas, com as propriedades Pesada, Leve e Duas mãos, em que o personagem tem proficiência.',
    );
  });

  it('keeps the key of a property the menu does not name', () => {
    expect(read({ weaponProperties: ['weapon-property:gone'] })).toContain(
      'com a propriedade weapon-property:gone',
    );
  });

  it('says what a use spends, once, when the trade has a resource', () => {
    expect(read({ weapon: 'ranged', resource: 'folego_do_atirador' })).toBe(
      'qualquer arma à distância, mesmo uma em que o personagem não tem proficiência. Cada troca gasta 1 uso de Fôlego do Atirador.',
    );
    expect(read({ resource: '   ' })).not.toContain('Cada troca');
  });
});
