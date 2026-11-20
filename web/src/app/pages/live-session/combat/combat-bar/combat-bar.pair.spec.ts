import { TestBed } from '@angular/core/testing';

import { CombatantKind, EncounterStatus } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { CombatBar } from './combat-bar';

describe('CombatBar, a rider and the mount it directs', () => {
  const isolda = combatant({
    id: 'r',
    label: 'Isolda',
    kind: CombatantKind.PLAYER,
    initiative: 20,
    mountedOnId: 'h',
  });
  const horse = combatant({ id: 'h', label: 'Cavalo de guerra', mountKind: 'controlled' });
  const goblin = combatant({ id: 'g', label: 'Goblin 1', initiative: 12 });
  const flat = (n: Element | null) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

  function setup(over: object = {}) {
    const fixture = TestBed.createComponent(CombatBar);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        status: EncounterStatus.ACTIVE,
        combatants: [isolda, horse, goblin],
        turnGroupIds: ['r', 'h'],
        currentCombatantId: 'r',
        ...over,
      } as never),
    );
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('draws the pair token and says the mount in the subtitle, as the rider’s player reads it', () => {
    const el = setup();
    expect(flat(el.querySelector('.bar__title'))).toBe('Vez de Isolda');
    expect(el.querySelector('.bar__turn app-pair-token')).not.toBeNull();
    expect(flat(el.querySelector('.bar__sub'))).toBe(
      'Cavalgando o Cavalo de guerra · em seguida: Goblin 1',
    );
  });

  it('keeps "Próximo turno" as it is for a turn of one: not off for a joint turn', () => {
    const el = setup();
    const next = el.querySelector<HTMLButtonElement>('app-next-turn button.next')!;
    expect(next.textContent).toContain('Próximo turno');
    expect(next.classList).not.toContain('next--off');
    expect(el.querySelector('app-next-turn .why')).toBeNull();
  });

  it('keeps the joint turn when the pair is tied with others', () => {
    const el = setup({ turnGroupIds: ['r', 'h', 'g'] });
    expect(el.querySelector('.bar__turn app-pair-token')).toBeNull();
    expect(flat(el.querySelector('.bar__title'))).toContain('Vez de');
  });
});
