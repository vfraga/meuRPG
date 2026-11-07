import { ComponentFixture, TestBed } from '@angular/core/testing';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { PlayerInitiative } from './player-initiative';

const salvia = combatant({
  id: 's',
  label: 'Sálvia',
  kind: CombatantKind.PLAYER,
  mine: true,
  controlledByMe: true,
  initiative: 14,
});
const creature = (id: string, over = {}) =>
  combatant({
    id,
    label: `Lobo atroz ${id}`,
    kind: CombatantKind.CREATURE,
    controlledByMe: true,
    ownerCharacterId: 'char-s',
    monsterNamePt: 'Lobo atroz',
    initiativeBonus: 2,
    ...over,
  });

describe('PlayerInitiative: the player creatures', () => {
  let fixture: ComponentFixture<PlayerInitiative>;
  const rolled: string[] = [];
  const typed: { id: string; face: number }[] = [];

  function setup(combatants: ReturnType<typeof combatant>[]): HTMLElement {
    rolled.length = 0;
    typed.length = 0;
    fixture = TestBed.createComponent(PlayerInitiative);
    fixture.componentRef.setInput('encounter', encounter({ combatants }));
    fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
    fixture.componentRef.setInput('dicePreference', DicePreference.APP);
    fixture.componentInstance.rollCreatureInApp.subscribe((id) => rolled.push(id));
    fixture.componentInstance.typeCreatureFace.subscribe((t) => typed.push(t));
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  function button(el: HTMLElement, text: string): HTMLButtonElement {
    return Array.from(el.querySelectorAll('.creatures button')).find((b) =>
      b.textContent?.includes(text),
    ) as HTMLButtonElement;
  }

  it('offers a roll for a creature without initiative, with the creature id', () => {
    const el = setup([salvia, creature('w1')]);
    expect(el.querySelector('.creatures')?.textContent).toContain('Iniciativa de Lobo atroz w1');
    button(el, 'Rolar no app').click();
    expect(rolled).toEqual(['w1']);
  });

  it('submits a typed face for the creature', () => {
    const el = setup([salvia, creature('w1')]);
    button(el, 'Digitar o resultado').click();
    fixture.detectChanges();
    const input = el.querySelector('.creatures input') as HTMLInputElement;
    input.value = '12';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    button(el, 'Confirmar').click();
    expect(typed).toEqual([{ id: 'w1', face: 12 }]);
  });

  it('shows the done state for a creature that has rolled', () => {
    const el = setup([salvia, creature('w1', { initiative: 15 })]);
    expect(el.querySelector('.creatures')?.textContent).toContain('Iniciativa de Lobo atroz w1');
    expect(el.querySelector('.creatures__total')?.textContent).toContain('15');
    expect(button(el, 'Rolar no app')).toBeUndefined();
  });

  it('shows one row for the creatures of one casting', () => {
    const el = setup([
      salvia,
      creature('w1', { summonGroupId: 'cast-1' }),
      creature('w2', { summonGroupId: 'cast-1' }),
    ]);
    expect(el.querySelectorAll('.creatures__row').length).toBe(1);
    expect(el.querySelector('.creatures')?.textContent).toContain('(2)');
    button(el, 'Rolar no app').click();
    expect(rolled).toEqual(['w1']);
  });
});

describe('PlayerInitiative: the natural mark (information only)', () => {
  function own(face: number | undefined): HTMLElement {
    const fixture = TestBed.createComponent(PlayerInitiative);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [{ ...salvia, initiativeFace: face, initiativeBonus: 2 } as never] }),
    );
    fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
    fixture.componentRef.setInput('dicePreference', DicePreference.APP);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }
  const mark = (el: HTMLElement) =>
    el.querySelector('[data-testid="natural-mark"]')?.textContent?.trim();

  it('tags the player own natural 20 and natural 1, and nothing for another face', () => {
    expect(mark(own(20))).toBe('20 natural');
    expect(mark(own(1))).toBe('1 natural');
    expect(mark(own(11))).toBeUndefined();
  });
});

describe('PlayerInitiative: a d20 a table minimum raised', () => {
  function raised(over: Record<string, unknown>): HTMLElement {
    const fixture = TestBed.createComponent(PlayerInitiative);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        combatants: [
          {
            ...salvia,
            initiative: 15,
            initiativeFace: 3,
            initiativeBonus: 3,
            initiativeTreatedAs: 12,
            initiativeTreatedAsSource: 'content:feat-keen',
            initiativeTreatedAsSourceName: 'Pé de Vento Muito Mais Que Longo',
            ...over,
          } as never,
        ],
      }),
    );
    fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
    fixture.componentRef.setInput('dicePreference', DicePreference.APP);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('keeps the formula with the total and puts what the feature did under it', () => {
    const el = raised({});
    expect(el.querySelector('.done__formula')?.textContent?.trim()).toBe('1d20 (12) + 3 = 15');
    expect(el.querySelector('[data-testid="raised-note"]')?.textContent?.trim()).toBe(
      'd20: 3 → 12 (Pé de Vento Muito Mais Que Longo)',
    );
  });

  it('shows no note when nothing raised the die', () => {
    const el = raised({ initiativeTreatedAs: undefined, initiative: 6 });
    expect(el.querySelector('.done__formula')?.textContent?.trim()).toBe('1d20 (3) + 3 = 6');
    expect(el.querySelector('[data-testid="raised-note"]')).toBeNull();
  });
});
