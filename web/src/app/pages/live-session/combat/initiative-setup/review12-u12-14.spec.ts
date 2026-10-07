// Finding U12-14: the master's initiative edit goes stale / Enter ignores busy().
import { TestBed } from '@angular/core/testing';

import { CombatantKind, EncounterStatus } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { InitiativeSetup } from './initiative-setup';

describe('Review12 U12-14: initiative edit vs stream update and busy', () => {
  const waiting = [
    combatant({ id: 'brisa', label: 'Brisa', kind: CombatantKind.PLAYER, initiativeBonus: 2 }),
  ];
  const rolled = [
    combatant({
      id: 'brisa',
      label: 'Brisa',
      kind: CombatantKind.PLAYER,
      initiativeBonus: 2,
      initiative: 17,
      initiativeFace: 15,
    }),
  ];

  function setup() {
    const fixture = TestBed.createComponent(InitiativeSetup);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ status: EncounterStatus.SETUP, round: 0, combatants: waiting }),
    );
    const submits: { id: string; face: number }[] = [];
    fixture.componentInstance.submit.subscribe((s) => submits.push(s));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.includes('Digitar pelo jogador'))!
      .click();
    fixture.detectChanges();
    return { fixture, el, submits };
  }

  const type = (el: HTMLElement, value: string) => {
    const input = el.querySelector<HTMLInputElement>('input')!;
    input.value = value;
    input.dispatchEvent(new Event('input'));
  };

  it('closes the typed edit when the player rolls meanwhile (stream update)', () => {
    const { fixture, el } = setup();
    type(el, '3');
    fixture.componentRef.setInput(
      'encounter',
      encounter({ status: EncounterStatus.SETUP, round: 0, combatants: rolled }),
    );
    fixture.detectChanges();
    // The player's roll (17) is on screen; the stale field must not stay open.
    expect(el.textContent).toContain('1d20 (15)');
    expect(el.querySelector('input')).toBeNull();
  });

  it('Enter while busy emits nothing and leaves the edit open', () => {
    const { fixture, el, submits } = setup();
    fixture.componentRef.setInput('busy', true);
    fixture.detectChanges();
    type(el, '7');
    fixture.detectChanges();
    el.querySelector<HTMLInputElement>('input')!.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter' }),
    );
    fixture.detectChanges();
    expect(submits).toEqual([]);
    expect(el.querySelector('input')).not.toBeNull();
  });
});
