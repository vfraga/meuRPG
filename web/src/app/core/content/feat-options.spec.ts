import { emptyEffect } from './effect-draft';
import { optionChoiceIndex, optionSource, optionSummary, withOptionSource } from './feat-options';
import { emptyFeature } from './feature-draft';
import { menu } from './content-testing-menu';

const choice = (patch: Record<string, unknown> = {}) => ({
  ...emptyEffect('choice'),
  choice: 'feature',
  ...patch,
});

describe('the feat options helpers', () => {
  it('finds the choice of options among the effects, and none when the choice is of another kind', () => {
    expect(optionChoiceIndex([emptyEffect('note'), choice()])).toBe(1);
    expect(optionChoiceIndex([{ ...emptyEffect('choice'), choice: 'skill' }])).toBe(-1);
    expect(optionChoiceIndex([])).toBe(-1);
  });

  it('reads where the options come from: a set, a marked list, or the feat itself', () => {
    expect(optionSource(choice({ set: 'fighting_style' }))).toBe('set');
    expect(optionSource(choice({ from: ['feature:fighting-style'] }))).toBe('srd');
    expect(optionSource(choice())).toBe('own');
  });

  it('keeps one source: the others fields are cleared', () => {
    const marked = choice({ from: ['feature:fighting-style'] });
    expect(withOptionSource(marked, 'set', 'invocation')).toMatchObject({
      set: 'invocation',
      from: [],
    });
    expect(withOptionSource(choice({ set: 'metamagic' }), 'set', 'invocation').set).toBe(
      'metamagic',
    );
    expect(withOptionSource(choice({ set: 'metamagic' }), 'srd', 'x')).toMatchObject({ set: '' });
    expect(withOptionSource(marked, 'own', 'x')).toMatchObject({ set: '', from: [] });
  });

  it('says in a line what the first effect of an option does', () => {
    const modifier = {
      ...emptyFeature(),
      effects: [{ ...emptyEffect('modifier'), target: 'ac', mode: 'add', value: '1' }],
    };
    expect(optionSummary(modifier, menu())).toBe('Modificador: Classe de Armadura +1');
    const proficiency = {
      ...emptyFeature(),
      effects: [{ ...emptyEffect('proficiency'), proficiency: 'skill:perception' }],
    };
    expect(optionSummary(proficiency, menu())).toBe('Proficiência em Percepção');
    expect(optionSummary(modifier, menu(), true)).toBe('Classe de Armadura +1');
    expect(optionSummary(proficiency, menu(), true)).toBe('Proficiência em Percepção');
    expect(optionSummary({ ...emptyFeature(), effects: [emptyEffect('modifier')] }, menu())).toBe(
      'Modificador',
    );
  });

  it('falls back to the start of the text when the option has no effect', () => {
    expect(optionSummary({ ...emptyFeature(), text: 'Seu golpe cai.\nOutra linha.' }, menu())).toBe(
      'Seu golpe cai.',
    );
    const long = 'a'.repeat(80);
    expect(optionSummary({ ...emptyFeature(), text: long }, menu())).toBe(`${'a'.repeat(60)}…`);
  });
});
