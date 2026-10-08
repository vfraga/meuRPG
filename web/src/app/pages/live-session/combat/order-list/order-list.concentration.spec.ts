// The "perdeu a concentração?" question closes when the combatant stops concentrating.
import { TestBed } from '@angular/core/testing';

import { CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { OrderList } from './order-list';

describe('lose-concentration question ends with the concentration', () => {
  const salvia = combatant({
    id: 's',
    label: 'Sálvia',
    kind: CombatantKind.PLAYER,
    characterId: 'sc',
    hitPointsCurrent: 38,
    hitPointsMax: 38,
    initiative: 13,
    concentrationSpell: 'spell:conjure-animals',
    concentrationSpellNamePt: 'Conjurar Animais',
  });
  const wolf = combatant({
    id: 'w1',
    label: 'Lobo atroz 1',
    kind: CombatantKind.CREATURE,
    characterId: '',
    ownerCharacterId: 'sc',
    summonGroupId: 'cast',
    monsterKey: 'monster:dire-wolf',
    monsterNamePt: 'Lobo atroz',
    hitPointsCurrent: 37,
    hitPointsMax: 37,
    initiative: 10,
  });

  it('closes the open question when the player ends the concentration meanwhile', () => {
    const fixture = TestBed.createComponent(OrderList);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [salvia, wolf], currentCombatantId: 's' }),
    );
    const ended: string[] = [];
    fixture.componentInstance.endConcentration.subscribe((id) => ended.push(id));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => b.textContent?.includes('Perdeu a concentração'))!
      .click();
    fixture.detectChanges();
    expect(el.textContent).toContain('A Sálvia perdeu a concentração?');

    // The player ends it; the creatures are gone and the stream sends the new combat.
    const stopped = {
      ...salvia,
      concentrationSpell: '',
      concentrationSpellNamePt: '',
    } as typeof salvia;
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [stopped], currentCombatantId: 's' }),
    );
    fixture.detectChanges();

    expect(el.textContent).not.toContain('perdeu a concentração?');
    expect(ended).toEqual([]);
  });
});
