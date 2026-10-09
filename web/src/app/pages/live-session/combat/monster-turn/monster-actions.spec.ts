import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import {
  CreatureActionKind,
  CreatureActionSchema,
  CreatureTurnSchema,
  CreatureUsageKind,
  CreatureUsageSchema,
} from '../../../../../gen/meurpg/play/v1/creatures_pb';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { CreatureClient } from '../../../../core/combat/creature-client';
import { MonsterActions } from './monster-actions';

const dragon = combatant({ id: 'dragon', label: 'Dragão vermelho adulto' });
const toren = combatant({ id: 'toren', label: 'Toren' });

const bite = create(CreatureActionSchema, {
  key: 'monster:adult-red-dragon#bite',
  name: 'Bite',
  namePt: 'Mordida',
  kind: CreatureActionKind.ATTACK,
  available: true,
  attackBonus: 14,
  melee: true,
  reachFt: 10,
});
const breath = create(CreatureActionSchema, {
  key: 'monster:adult-red-dragon#fire-breath',
  name: 'Fire Breath',
  namePt: 'Sopro de Fogo',
  kind: CreatureActionKind.SAVE,
  available: false,
  usage: create(CreatureUsageSchema, {
    kind: CreatureUsageKind.RECHARGE,
    rechargeMin: 5,
    recharging: true,
    rechargeRoll: 3,
  }),
});

function setup(useAction = vi.fn()) {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [{ provide: CreatureClient, useValue: { useAction } }],
  });
  const fixture = TestBed.createComponent(MonsterActions);
  const apply = vi.fn();
  fixture.componentRef.setInput('campaignId', 'c');
  fixture.componentRef.setInput('encounter', encounter({ combatants: [dragon, toren] }));
  fixture.componentRef.setInput('subject', dragon);
  fixture.componentRef.setInput('turn', create(CreatureTurnSchema, { actions: [bite, breath] }));
  fixture.componentRef.setInput('state', { apply } as unknown as CombatState);
  fixture.detectChanges();
  return { fixture, useAction, apply, el: fixture.nativeElement as HTMLElement };
}

describe('MonsterActions', () => {
  it('greys an attack until one target is picked, and says why', () => {
    const { el } = setup();
    const button = el.querySelector('article button') as HTMLButtonElement;
    expect(button.textContent).toContain('Atacar');
    expect(button.getAttribute('aria-disabled')).toBe('true');
    const why = document.getElementById(button.getAttribute('aria-describedby') ?? '');
    expect(why).not.toBeNull();
    expect(why?.textContent).toContain('Escolha um alvo');
  });

  it('shows the recharge roll the server made and keeps the action grey', () => {
    const { el } = setup();
    const text = el.textContent ?? '';
    expect(text).toContain('Recarga 5–6 · Recarregando');
    expect(text).toContain('Não recarregou.');
    const buttons = Array.from(el.querySelectorAll('article button'));
    expect(buttons[1].getAttribute('aria-disabled')).toBe('true');
    expect(el.querySelector('.action__recharge')?.textContent).toContain('3');
  });

  it('uses the attack on the target with a key it keeps for a retry', async () => {
    const result = { attackRoll: {}, outcome: 3, damageParts: [], immuneTargetIds: [] };
    const useAction = vi
      .fn()
      .mockRejectedValueOnce(new Error('lost'))
      .mockResolvedValue({ encounter: encounter({ combatants: [dragon, toren] }), result });
    const { fixture, apply } = setup(useAction);
    const actions = fixture.componentInstance as unknown as {
      pickTargets(ids: string[]): void;
      use(a: typeof bite): Promise<void>;
    };
    actions.pickTargets(['toren']);
    await actions.use(bite);
    expect(useAction).toHaveBeenCalledTimes(1);
    await actions.use(bite);
    expect(useAction).toHaveBeenCalledTimes(2);
    const [first, second] = useAction.mock.calls;
    expect(first[3]).toBe(bite.key);
    expect(first[4]).toEqual(['toren']);
    expect(second[5]).toBe(first[5]); // the same change, tried again after a lost answer
    expect(apply).toHaveBeenCalledTimes(1);
  });
});
