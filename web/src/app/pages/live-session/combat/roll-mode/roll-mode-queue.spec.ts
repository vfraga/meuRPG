import { TestBed } from '@angular/core/testing';

import { RollMode, RollModeRequestStatus } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { RollModeQueue } from './roll-mode-queue';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const request = (id: string, status = RollModeRequestStatus.PENDING) =>
  ({
    id,
    combatantId: 't',
    targetId: 'g',
    attackKey: 'attack:shortsword',
    attackNamePt: 'Espada curta',
    suggestedMode: RollMode.NORMAL,
    requestedMode: RollMode.ADVANTAGE,
    reason: 'estou nas costas dele',
    status,
  }) as never;

function setup(requests: unknown[]) {
  const api = { answerRollModeRequest: vi.fn() };
  TestBed.configureTestingModule({
    providers: [{ provide: CombatClient, useValue: fakeCombatClient(api) }],
  });
  const enc = encounter({
    combatants: [combatant({ id: 't', label: 'Toren' }), combatant({ id: 'g', label: 'Goblin' })],
    rollModeRequests: requests,
  } as never);
  api.answerRollModeRequest.mockResolvedValue(enc);
  const state = new CombatState();
  const fixture = TestBed.createComponent(RollModeQueue);
  fixture.componentRef.setInput('encounter', enc);
  fixture.componentRef.setInput('campaignId', 'camp');
  fixture.componentRef.setInput('state', state);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const button = (name: string) =>
    Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes(name))!;
  return { api, fixture, el, button };
}

describe('RollModeQueue', () => {
  it('draws nothing while no request waits', () => {
    const { el } = setup([request('r0', RollModeRequestStatus.ANSWERED)]);
    expect(el.textContent!.trim()).toBe('');
  });

  it('writes the request and offers Aprovar, Recusar and Desvantagem', () => {
    const { el } = setup([request('r1')]);
    expect(el.textContent!.replace(/\s+/g, ' ')).toContain(
      'Pedido de Vantagem: Toren → Goblin (Espada curta) — “estou nas costas dele”',
    );
    const names = Array.from(el.querySelectorAll('button')).map((b) => b.textContent!.trim());
    expect(names).toEqual(['Aprovar', 'Recusar', 'Desvantagem']);
  });

  it.each([
    ['Aprovar', RollMode.ADVANTAGE],
    ['Recusar', RollMode.NORMAL],
    ['Desvantagem', RollMode.DISADVANTAGE],
  ])('%s answers with the right mode', async (name, mode) => {
    const { api, fixture, button } = setup([request('r1')]);
    button(name).click();
    await fixture.whenStable();
    expect(api.answerRollModeRequest).toHaveBeenCalledWith(
      'camp',
      expect.any(String),
      'r1',
      mode,
      expect.any(String),
    );
  });

  it('says in words when the answer fails', async () => {
    const { api, fixture, el, button } = setup([request('r1')]);
    api.answerRollModeRequest.mockRejectedValue(new Error('x'));
    button('Aprovar').click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(el.querySelector('[role="alert"]')).not.toBeNull();
  });
});
