// Review12 finding U12-21: the NpcCard effects reset `targetId` without renewing the idempotency
// key, so a retry after a lost answer is sent for a different target with the old key (the server
// replays by key and does not compare the body).
import { TestBed } from '@angular/core/testing';

import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { NpcCard } from './npc-card';

describe('Review12 U12-21: NpcCard keeps the idempotency key when the target changes', () => {
  const opts = (targets: string[]) =>
    ({
      options: { attacks: [{ attack: { key: 'a1', saveDc: 0, attackBonus: 4, namePt: 'Espada', damage: '1d6', damageTypePt: 'cortante', rangeFt: 5, longRangeFt: 0 } }] },
      attackTargets: [
        { attackKey: 'a1', targets: targets.map((id) => ({ combatantId: id, label: id })) },
      ],
      pendingDamages: [],
    }) as never;

  it('sends a different key for a different target after a failed roll', async () => {
    TestBed.resetTestingModule();
    const rollAttack = vi
      .fn()
      .mockRejectedValueOnce(new Error('lost'))
      .mockRejectedValue(new Error('lost again'));
    TestBed.configureTestingModule({ providers: [{ provide: CombatClient, useValue: { rollAttack } }] });
    const fixture = TestBed.createComponent(NpcCard);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [combatant({ id: 'npc', label: 'Goblin' })] }),
    );
    fixture.componentRef.setInput('subject', combatant({ id: 'npc', label: 'Goblin' }));
    fixture.componentRef.setInput('state', { apply: vi.fn() } as unknown as CombatState);
    fixture.componentRef.setInput('options', opts(['t1', 't2']));
    fixture.detectChanges();
    await fixture.whenStable();
    const card = fixture.componentInstance as any;
    expect(card.targetId()).toBe('t1');

    await card.rollApp();
    expect(rollAttack).toHaveBeenCalledTimes(1);

    // The options refresh: t1 is gone, the effect picks t2 by itself.
    fixture.componentRef.setInput('options', opts(['t2', 't3']));
    fixture.detectChanges();
    await fixture.whenStable();
    expect(card.targetId()).toBe('t2');

    await card.rollApp();
    expect(rollAttack).toHaveBeenCalledTimes(2);
    const [first, second] = rollAttack.mock.calls;
    expect(first[4]).toBe('t1');
    expect(second[4]).toBe('t2');
    expect(second[6]).not.toBe(first[6]);
  });
});
