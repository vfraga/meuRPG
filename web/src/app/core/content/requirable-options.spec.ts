import { menu } from './content-testing-menu';
import { emptyFeature, type FeatureDraft } from './feature-draft';
import { requirableOptions, siblingOptions } from './requirable-options';

const option = (key: string, name: string): FeatureDraft => ({ ...emptyFeature(), key, name });

describe('the sibling options a feature option may ask for (#580, board 580-03)', () => {
  const list = [
    option('feature:a@mesa', 'Golpe do Corvo'),
    option('feature:b@mesa', 'Asa Partida'),
    option('', 'Bico de Ferro'),
    option('feature:d@mesa', 'Pena Negra'),
  ];

  it('offers the saved siblings with their place in the list, never the option itself and never one with no key', () => {
    expect(siblingOptions(list, 'feature:b@mesa')).toEqual([
      { value: 'feature:a@mesa', label: 'Golpe do Corvo', badge: 'Opção 1' },
      { value: 'feature:d@mesa', label: 'Pena Negra', badge: 'Opção 4' },
    ]);
  });

  it('keeps the place of an option whose siblings come before and after an unsaved one', () => {
    expect(siblingOptions(list, '').map((o) => o.badge)).toEqual(['Opção 1', 'Opção 2', 'Opção 4']);
  });

  it('names a saved sibling with no name by its place', () => {
    expect(siblingOptions([option('feature:a@mesa', ' '), option('', 'x')], '')[0].label).toBe(
      'Opção 1',
    );
  });

  it('puts the siblings before the SRD and table options', () => {
    const all = requirableOptions(
      menu(),
      [],
      'feature:b@mesa',
      siblingOptions(list, 'feature:b@mesa'),
    );
    expect(all.slice(0, 2).map((o) => o.label)).toEqual(['Golpe do Corvo', 'Pena Negra']);
    expect(all.length).toBeGreaterThan(2);
    expect(all.some((o) => o.value === 'feature:b@mesa')).toBe(false);
  });
});
