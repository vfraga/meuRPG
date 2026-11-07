import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';

import { LimitedActionSchema } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { OrderList } from './order-list';
import { openAllRows } from './order-list-testing';

describe('OrderList: the limits of a creature (RN-29)', () => {
  function mount(limited: Parameters<typeof create<typeof LimitedActionSchema>>[1][]) {
    const fixture = TestBed.createComponent(OrderList);
    const vrock = {
      ...combatant({ id: 'v', label: 'Vrock', hitPointsCurrent: 104, hitPointsMax: 104 }),
      limitedActions: limited.map((l) => create(LimitedActionSchema, l)),
    };
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [vrock], currentCombatantId: 'v' }),
    );
    fixture.detectChanges();
    openAllRows(fixture);
    return [
      ...(fixture.nativeElement as HTMLElement).querySelectorAll('[data-testid="order-limit"]'),
    ].map((e) => (e.textContent ?? '').replace(/\s+/g, ' ').trim());
  }

  it('lists a spent Recharge action and the uses a day of an opened row, one line each', () => {
    const lines = mount([
      {
        actionKey: 'a',
        kind: 'recharge',
        usage: 'Recharge 6',
        name: 'Spores',
        namePt: 'Esporos',
        usesAllowed: 1,
        usesLeft: 0,
      },
      {
        actionKey: 'b',
        kind: 'per_day',
        usage: '1/day',
        name: 'Stunning Screech',
        namePt: 'Guincho atordoante',
        usesAllowed: 1,
        usesLeft: 1,
        available: true,
      },
    ]);
    expect(lines).toEqual([
      expect.stringContaining('Esporos · Recarga 6 · gasta'),
      expect.stringContaining('Guincho atordoante · 1/dia · 1 de 1'),
    ]);
  });

  it('says a ready action is ready', () => {
    const lines = mount([
      {
        actionKey: 'a',
        kind: 'recharge',
        usage: 'Recharge 6',
        name: 'Spores',
        namePt: 'Esporos',
        usesAllowed: 1,
        usesLeft: 1,
        available: true,
      },
    ]);
    expect(lines[0]).toContain('Esporos · Recarga 6 · pronta');
  });

  it('shows no line for a combatant the server sent no limits for (a player never gets them)', () => {
    expect(mount([])).toEqual([]);
  });
});
