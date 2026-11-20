import { askKeyNamer } from './ask-names';
import { catalog } from './content-testing';
import { emptyFeature, type FeatureDraft } from './feature-draft';

const option = (key: string, name: string): FeatureDraft => ({ ...emptyFeature(), key, name });

describe('the names of what an option asks for (#580, board 580-03)', () => {
  const nameOf = askKeyNamer(
    [option('feature:golpe@mesa', 'Golpe do Corvo'), option('', 'Sem chave')],
    catalog(),
    [
      { value: 'feature:srd', label: 'Pacto da Lâmina' },
      { value: 'option:mesa', label: 'Olhar de Brasa (da mesa)' },
    ],
  );

  it('names a sibling by the name it has now, a class by the catalog, and the options of the lists without "(da mesa)"', () => {
    expect(nameOf('feature:golpe@mesa')).toBe('Golpe do Corvo');
    expect(nameOf('class:paladin')).toBe('Paladino');
    expect(nameOf('feature:srd')).toBe('Pacto da Lâmina');
    expect(nameOf('option:mesa')).toBe('Olhar de Brasa');
  });

  it('gives the key itself for one nobody knows, as the other readers do (they make a phrase of it)', () => {
    expect(nameOf('feature:desconhecida')).toBe('feature:desconhecida');
  });

  it('names a key with no catalog by the lists alone', () => {
    const bare = askKeyNamer([], null, [{ value: 'feature:srd', label: 'Pacto da Lâmina' }]);
    expect(bare('feature:srd')).toBe('Pacto da Lâmina');
    expect(bare('class:paladin')).toBe('class:paladin');
  });
});
