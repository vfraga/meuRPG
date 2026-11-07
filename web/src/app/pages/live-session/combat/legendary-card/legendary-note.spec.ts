import { TestBed } from '@angular/core/testing';

import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { LegendaryNote } from './legendary-note';

const dragon = (over: object = {}) =>
  combatant({
    id: 'd',
    label: 'Dragão vermelho adulto',
    legendary: {
      actionsPerRound: 3,
      actionsLeft: 1,
      resistancePerDay: 3,
      resistanceLeft: 2,
      usableNow: true,
      options: [{ key: 'detect', name: 'Detect', namePt: 'Detectar', cost: 1, kind: 'text' }],
      ...over,
    },
  } as never);

function render(current: string, subject = dragon()) {
  const fixture = TestBed.createComponent(LegendaryNote);
  fixture.componentRef.setInput(
    'encounter',
    encounter({
      combatants: [subject, combatant({ id: 't', label: 'Tor' })],
      currentCombatantId: current,
    }),
  );
  fixture.componentRef.setInput('subject', subject);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}
const lines = (el: HTMLElement) =>
  Array.from(el.querySelectorAll('p'), (p) =>
    (p.querySelector('span') ?? p).textContent?.replace(/\s+/g, ' ').trim(),
  );

describe("the legendary creature's own turn on the master's card", () => {
  it('says the actions came back, what is left of the Resistance and that they are spent on another turn', () => {
    const el = render('d', dragon({ actionsLeft: 3 }));
    expect(lines(el)).toEqual([
      'Ações lendárias: 3 de 3 , renovadas agora',
      'Resistência Lendária: 2 de 3 hoje',
      'Ações lendárias se usam no turno de outra criatura, nunca no dele.',
    ]);
    expect(Array.from(el.querySelectorAll('b'), (b) => b.textContent)).toEqual([
      '3 de 3',
      '2 de 3',
    ]);
  });

  it('leaves out the Resistance for a stat block without the trait', () => {
    expect(lines(render('d', dragon({ resistancePerDay: 0 })))).toHaveLength(2);
  });

  it('says nothing off its turn, nor for a creature without legendary actions', () => {
    expect(render('t').querySelector('[data-testid="legendary-recovered"]')).toBeNull();
    const plain = combatant({ id: 'g', label: 'Goblin' });
    expect(render('g', plain).querySelector('[data-testid="legendary-recovered"]')).toBeNull();
  });
});
