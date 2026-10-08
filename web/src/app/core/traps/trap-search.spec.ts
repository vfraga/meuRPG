import { resultMessage, searchRoute, searchStep, skillOptions } from './trap-search';

describe('trap search', () => {
  it('lists the two skills with the character bonus', () => {
    const [p, i] = skillOptions({ perception: 4, investigation: -1 });
    expect(p.title).toBe('Percepção +4');
    expect(i.title).toBe('Investigação −1');
    expect(skillOptions(null)[0].title).toBe('Percepção');
  });

  it('follows the steps', () => {
    expect(searchStep(false, false)).toBe(1);
    expect(searchStep(false, true)).toBe(2);
    expect(searchStep(true, true)).toBe(3);
  });

  it('answers the same words for a miss and for no trap', () => {
    expect(resultMessage([])).toEqual({ title: 'Você não encontrou nada.', detail: '' });
    expect(resultMessage(['Fosso escondido']).title).toBe(
      'Você achou uma armadilha: Fosso escondido.',
    );
    expect(resultMessage(['A', 'B']).title).toBe('Você achou 2 armadilhas: A, B.');
    expect(resultMessage([], 1)).toEqual({
      title: 'Você achou uma armadilha.',
      detail: 'Veja no seu mapa.',
    });
    expect(resultMessage(['A'], 2).title).toBe('Você achou 2 armadilhas.');
  });

  it('sends the combat Search to the trap search only on a map with a grid, with the combatant placed on it', () => {
    expect(searchRoute(20, true)).toBe('traps');
    expect(searchRoute(0, true)).toBe('action');
    expect(searchRoute(20, false)).toBe('action');
  });
});
