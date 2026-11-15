import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import {
  AttackTargetsSchema,
  CombatantKind,
  GetTurnOptionsResponseSchema,
  ReactionKind,
  ReactionWaitSchema,
  ReadiedActionSchema,
  ReadiedKind,
  ReadiedStatus,
  TargetInReachSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  AttackKind,
  AttackOptionSchema,
  TurnOptionsSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { EffectsClient } from '../../../../core/effects/effects-client';
import { ReactionQueue } from './reaction-queue';

const plain = (text: string | null | undefined) =>
  (text ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const held = (over: Record<string, unknown> = {}) =>
  create(ReadiedActionSchema, {
    id: 'r1',
    triggerText: 'Quando o zumbi passar pela porta',
    kind: ReadiedKind.ATTACK,
    actionKey: 'equipment:longsword',
    actionNamePt: 'Espada longa',
    targetCombatantId: 'z1',
    status: ReadiedStatus.WAITING,
    ...over,
  } as never);

describe("ReactionQueue, a readied action's window (#582, board 582-05)", () => {
  const turnOptions = vi.fn();
  const api = { answerReaction: vi.fn(), turnOptions };

  function setup(
    row: Record<string, unknown> = {},
    window: Record<string, unknown> = {},
    wait = false,
  ) {
    turnOptions.mockReset().mockResolvedValue(
      create(GetTurnOptionsResponseSchema, {
        options: create(TurnOptionsSchema, {
          attacks: [
            create(AttackOptionSchema, {
              enabled: true,
              attack: {
                key: 'equipment:longsword',
                kind: AttackKind.WEAPON,
                melee: true,
                rangeFt: 5,
              },
            } as never),
          ],
        }),
        attackTargets: [
          create(AttackTargetsSchema, {
            attackKey: 'equipment:longsword',
            targets: [
              create(TargetInReachSchema, { combatantId: 'z1', label: 'Zumbi 1', distanceFt: 5 }),
            ],
          }),
        ],
      }),
    );
    api.answerReaction.mockReset().mockImplementation(async () => ({
      encounter: encounter({ revision: 9 } as never),
      result: undefined,
    }));
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: fakeCombatClient(api) },
        { provide: EffectsClient, useValue: { rollEffectSave: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(ReactionQueue);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        combatants: [
          combatant({ id: 'tor', label: 'Tor', kind: CombatantKind.PLAYER, readied: held(row) }),
          combatant({ id: 'z1', label: 'Zumbi 1' }),
        ],
        reactionWindows: [
          reactionWindow({
            id: 'w1',
            kind: ReactionKind.READIED,
            groupId: 'fire-1',
            reactorId: 'tor',
            reactorLabel: 'Tor',
            reactorIsPlayer: true,
            answerNow: true,
            ...window,
          } as never),
        ],
        reactionWait: wait
          ? create(ReactionWaitSchema, {
              titlePt: 'Esperando o mestre',
              detailPt: 'Um ataque preparado espera outras reações',
            })
          : undefined,
      } as never),
    );
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', new CombatState());
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }
  const buttons = (el: HTMLElement) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('.btns button'));

  it('reads what it holds and the trigger, who it waits for, and "Deixar passar" first', async () => {
    const { fixture, el } = setup();
    await fixture.whenStable();
    fixture.detectChanges();
    const row = el.querySelector('.row') as HTMLElement;
    expect(plain(row.querySelector('.row__desc')?.textContent)).toBe(
      'Ação preparada: Espada longa no Zumbi 1 · “Quando o zumbi passar pela porta”',
    );
    expect(plain(row.querySelector('.row__status')?.textContent)).toMatch(/^Esperando Tor/);
    expect(buttons(el).map((b) => plain(b.textContent))).toEqual([
      'Deixar passar',
      'Atacar por Tor',
    ]);
    expect(buttons(el).every((b) => !b.classList.contains('btn--off'))).toBe(true);
  });

  it('answers a readied attack with the dice of the app, and the pass with nothing', async () => {
    const { fixture, el } = setup();
    await fixture.whenStable();
    fixture.detectChanges();
    buttons(el)[1].click();
    await fixture.whenStable();
    expect(api.answerReaction).toHaveBeenCalledWith(
      'camp',
      'enc',
      'w1',
      { use: true, creatureIds: ['z1'], die: { inApp: true } },
      expect.any(String),
    );
    api.answerReaction.mockClear();
    buttons(el)[0].click();
    await fixture.whenStable();
    expect(api.answerReaction).toHaveBeenCalledWith(
      'camp',
      'enc',
      'w1',
      { use: false },
      expect.any(String),
    );
  });

  it('says the wait the server writes while another window holds the roll, with both buttons off', () => {
    const { el } = setup({}, { answerNow: false }, true);
    expect(plain(el.querySelector('.row')?.textContent)).toContain(
      'Esperando o mestre · Um ataque preparado espera outras reações',
    );
    expect(buttons(el).every((b) => b.classList.contains('btn--off'))).toBe(true);
  });

  it('asks the master who a readied attack is aimed at when the trigger named no one, and keeps it off until he picks', async () => {
    const { fixture, el } = setup({ targetCombatantId: '' });
    await fixture.whenStable();
    fixture.detectChanges();
    expect(plain(el.querySelector('legend')?.textContent)).toBe('Em quem · 1 criatura ao alcance');
    expect(buttons(el)[1].classList.contains('btn--off')).toBe(true);
    buttons(el)[1].click();
    await fixture.whenStable();
    expect(api.answerReaction).not.toHaveBeenCalled();
    (el.querySelector('input[type=radio]') as HTMLInputElement).click();
    fixture.detectChanges();
    expect(buttons(el)[1].classList.contains('btn--off')).toBe(false);
    buttons(el)[1].click();
    await fixture.whenStable();
    expect(api.answerReaction).toHaveBeenCalledWith(
      'camp',
      'enc',
      'w1',
      { use: true, creatureIds: ['z1'], die: { inApp: true } },
      expect.any(String),
    );
  });

  it('releases a readied spell for the player from the queue, with the creature the master picks', async () => {
    const { fixture, el } = setup({
      kind: ReadiedKind.SPELL,
      actionKey: 'spell:magic-missile',
      actionNamePt: 'Mísseis Mágicos',
      targetCombatantId: '',
    });
    await fixture.whenStable();
    fixture.detectChanges();
    expect(buttons(el).map((b) => plain(b.textContent))).toEqual([
      'Deixar passar',
      'Soltar por Tor',
    ]);
    expect(el.querySelector('.row__status[id$="-off"]')).toBeNull();
    expect(buttons(el)[1].classList.contains('btn--off')).toBe(false);
    (el.querySelector('input[type=radio]') as HTMLInputElement).click();
    fixture.detectChanges();
    buttons(el)[1].click();
    await fixture.whenStable();
    expect(api.answerReaction).toHaveBeenCalledWith(
      'camp',
      'enc',
      'w1',
      { use: true, creatureIds: ['tor'], die: { inApp: true } },
      expect.any(String),
    );
  });

  it('does not use a move for the player from the queue, and says why', async () => {
    const { fixture, el } = setup({ kind: ReadiedKind.MOVE, moveFt: 25, actionNamePt: '' });
    expect(plain(el.querySelector('.row')?.textContent)).toContain(
      'Mover é pelo mapa: mova Tor com a janela aberta.',
    );
    expect(buttons(el)[1].classList.contains('btn--off')).toBe(true);
    buttons(el)[1].click();
    await fixture.whenStable();
    expect(api.answerReaction).not.toHaveBeenCalled();
  });
});
