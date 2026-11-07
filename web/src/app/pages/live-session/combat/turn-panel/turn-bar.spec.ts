import { TestBed } from '@angular/core/testing';

import { combatant } from '../../../../core/combat/combat-testing';
import { TurnBar } from './turn-bar';

describe('TurnBar', () => {
  function setup(over: Record<string, unknown> = {}, inputs: Record<string, unknown> = {}) {
    const fixture = TestBed.createComponent(TurnBar);
    fixture.componentRef.setInput('own', combatant({ id: 'p', label: 'Pensantus', ...over }));
    for (const [name, value] of Object.entries(inputs)) {
      fixture.componentRef.setInput(name, value);
    }
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }
  const text = (el: Element | null) => (el?.textContent ?? '').replace(/\s+/g, ' ').trim();

  it('says what the turn still has with its shapes, and "Encerrar turno"', () => {
    const el = setup({ movementLeftFt: 30, movementLeftDft: 300, bonusActionUsed: true });
    const row = el.querySelector('.row');
    expect(text(row)).toContain('Ainda tem');
    expect(
      [...el.querySelectorAll('li')].map((li) =>
        li.getAttribute('aria-label')?.replace(/\u00a0/g, ' '),
      ),
    ).toEqual(['Ação disponível', 'Reação disponível', 'Movimento: 9,0 m']);
    expect(text(row)).toContain('Encerrar turno');
    expect(text(el)).not.toContain('Mover');
  });

  it('leaves the walker out when there is no movement left, and keeps it with no map (the count of the metres)', () => {
    const still = setup({ movementLeftFt: 0, movementLeftDft: 0 });
    expect(still.querySelector('.shape__icon .walker')).toBeNull();
    expect([...still.querySelectorAll('li')]).toHaveLength(3);
    const theatre = setup({ movementLeftFt: 25, movementLeftDft: 250 }, { theatre: true });
    expect([...theatre.querySelectorAll('li')]).toHaveLength(4);
  });

  it('says there is nothing more when everything is spent', () => {
    const el = setup({
      actionUsed: true,
      bonusActionUsed: true,
      reactionUsed: true,
      movementLeftFt: 0,
      movementLeftDft: 0,
    });
    expect(el.querySelector('ul')).toBeNull();
    expect(text(el.querySelector('.left'))).toContain('Nada mais neste turno');
  });

  it('says why the turn cannot end while an opportunity attack waits', () => {
    const el = setup({}, { waiting: 'Esperando a reação do mestre' });
    expect(text(el.querySelector('.row'))).toContain(
      'Esperando a reação do mestre: a vez continua quando responderem.',
    );
    expect(el.querySelector<HTMLButtonElement>('.end')?.getAttribute('aria-disabled')).toBe('true');
  });

  it("leaves the master's wait to the turn card: the bar shows what is left, and the button stays off", () => {
    const el = setup({}, { waiting: 'Esperando o mestre' });
    expect(text(el)).not.toContain('Esperando o mestre');
    expect(text(el.querySelector('.row'))).toContain('Ainda tem');
    expect(el.querySelector<HTMLButtonElement>('.end')?.getAttribute('aria-disabled')).toBe('true');
  });

  it('says "Passar o turno" for a surprised character (W7-X)', () => {
    const el = setup({}, { surprised: true });
    expect(text(el.querySelector('.row'))).toContain('Passar o turno');
    expect(text(el.querySelector('.row'))).not.toContain('Encerrar turno');
  });
});
