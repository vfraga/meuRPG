import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  AttackTargetsSchema,
  CombatantState,
  GetTurnOptionsResponseSchema,
  ReactionKind,
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
import { CombatState as State } from '../../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { ReadiedWindowSheet, type ReadiedWindowSheetData } from './readied-window-sheet';

const flat = (n: Element | null | undefined) =>
  n?.textContent?.replace(/\s+/g, ' ').replace(/ /g, ' ').trim() ?? '';

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

const reach = (id: string, label: string, feet: number, tooFar = false) =>
  create(TargetInReachSchema, {
    combatantId: id,
    label,
    state: CombatantState.UNHURT,
    distanceFt: feet,
    tooFar,
  });

const window = reactionWindow({
  id: 'w1',
  kind: ReactionKind.READIED,
  reactorId: 'tor',
  reactorLabel: 'Tor',
  reactorIsPlayer: true,
  forYou: true,
} as never);

describe('ReadiedWindowSheet (#582, boards 582-06 to 582-08)', () => {
  const answerReaction = vi.fn();
  const close = vi.fn();

  function setup(
    opts: { row?: Record<string, unknown>; fail?: boolean; gone?: boolean; used?: boolean } = {},
  ) {
    const state = new State();
    state.apply(
      encounter({
        combatants: [
          combatant({ id: 'tor', label: 'Tor', mine: true, reactionUsed: !!opts.used }),
          combatant({ id: 'z1', label: 'Zumbi 1' }),
        ],
        reactionWindows: opts.gone ? [] : [window],
      } as never),
    );
    const loadOptions = async () => {
      if (opts.fail) {
        throw new Error('down');
      }
      return create(GetTurnOptionsResponseSchema, {
        options: create(TurnOptionsSchema, {
          attacks: [
            create(AttackOptionSchema, {
              enabled: true,
              attack: {
                key: 'equipment:longsword',
                namePt: 'Espada longa',
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
            targets: [reach('z1', 'Zumbi 1', 5), reach('z2', 'Zumbi 2', 15, true)],
          }),
        ],
      });
    };
    const data: ReadiedWindowSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      window,
      round: 2,
      readied: held(opts.row),
      reactorLabel: 'Tor',
      state,
    };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close, disableClose: false } },
        {
          provide: CombatClient,
          useValue: fakeCombatClient({ answerReaction, turnOptions: loadOptions }),
        },
      ],
    });
    const fixture = TestBed.createComponent(ReadiedWindowSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
    };
    const button = (text: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => flat(b).startsWith(text))!;
    return { el, settle, button, state };
  }

  beforeEach(() => {
    answerReaction.mockReset();
    close.mockReset();
  });

  it('quotes the trigger, lists who the attack reaches with the named one chosen, and "Deixar passar" first', async () => {
    const { el, settle, button } = setup();
    await settle();
    expect(flat(el.querySelector('h2'))).toBe('Ação preparada');
    expect(flat(el.querySelector('.what'))).toBe(
      'O mestre disse que o gatilho aconteceu: “Quando o zumbi passar pela porta”. Atacar com a Espada longa agora?',
    );
    expect(flat(el.querySelector('legend'))).toBe('Em quem · 1 criatura ao alcance');
    expect(flat(el.querySelector('input[type=radio]:checked')?.closest('label'))).toContain(
      'O alvo que você disse',
    );
    expect(flat(el.querySelectorAll('label')[1])).toContain('Fora de alcance: até 1,5 m');
    expect(Array.from(el.querySelectorAll('.pair button')).map((b) => flat(b))).toEqual([
      'Deixar passar',
      'Atacar o Zumbi 1',
    ]);
    expect(button('Deixar passar').hasAttribute('data-initial-focus')).toBe(true);
    expect(flat(el.querySelector('.foot__clock'))).toMatch(
      /^Se você não responder, passa sozinho em \d+ s\.$/,
    );
    expect(el.querySelector('.frame__close')).toBeNull();
  });

  it('answers USE on the creature with the dice of the app, and closes', async () => {
    answerReaction.mockResolvedValue({ encounter: encounter({ revision: 5 } as never) });
    const { settle, button, state } = setup();
    await settle();
    button('Atacar o Zumbi 1').click();
    await settle();
    expect(answerReaction).toHaveBeenCalledWith(
      'c',
      'enc',
      'w1',
      { use: true, creatureIds: ['z1'], die: { inApp: true } },
      expect.any(String),
    );
    expect(state.encounter()?.revision).toBe(5);
    expect(close).toHaveBeenCalled();
  });

  it('keeps the prompt open while the roll is held for other reactions, and closes by itself once the window is answered', async () => {
    const mine = [
      combatant({ id: 'tor', label: 'Tor', mine: true }),
      combatant({ id: 'z1', label: 'Zumbi 1' }),
    ];
    answerReaction.mockResolvedValue({
      encounter: encounter({ revision: 5, combatants: mine, reactionWindows: [window] } as never),
      result: { used: false },
    });
    const { el, settle, button, state } = setup();
    await settle();
    button('Atacar o Zumbi 1').click();
    await settle();
    expect(close).not.toHaveBeenCalled();
    expect(flat(el.querySelector('.what'))).toContain('A sua jogada espera as reações de outros');
    // The question stays: the window's `answer_now` is the master's, so the server refuses an answer while the hold lasts.
    expect(flat(el.querySelectorAll('.what')[1])).toContain('Atacar com a Espada longa agora?');
    expect(el.querySelectorAll('.pair button').length).toBe(2);
    // The use happens: the window leaves the combat and the sheet closes, with no "passou sozinho".
    state.apply(
      encounter({
        revision: 7,
        combatants: [combatant({ id: 'tor', label: 'Tor', mine: true })],
        reactionWindows: [],
      } as never),
    );
    await settle();
    expect(close).toHaveBeenCalled();
  });

  it('lets it pass with nothing chosen', async () => {
    answerReaction.mockResolvedValue({ encounter: encounter({ revision: 6 } as never) });
    const { settle, button } = setup();
    await settle();
    button('Deixar passar').click();
    await settle();
    expect(answerReaction).toHaveBeenCalledWith(
      'c',
      'enc',
      'w1',
      { use: false },
      expect.any(String),
    );
    expect(close).toHaveBeenCalled();
  });

  it('does not choose a creature that was named but is out of reach, and keeps the attack off', async () => {
    const { el, settle, button } = setup({ row: { targetCombatantId: 'z2' } });
    await settle();
    expect(el.querySelector('input[type=radio]:checked')).toBeNull();
    button('Atacar').click();
    await settle();
    expect(answerReaction).not.toHaveBeenCalled();
  });

  it('has one button for another standard action, in its own words', async () => {
    const { el, settle } = setup({
      row: {
        kind: ReadiedKind.OTHER,
        actionNamePt: 'Esquivar',
        triggerText: 'Se a porta abrir',
        targetCombatantId: '',
      },
    });
    await settle();
    expect(flat(el.querySelector('.what'))).toBe(
      'O mestre disse que o gatilho aconteceu: “Se a porta abrir”. Esquivar agora?',
    );
    expect(el.querySelector('app-resource-pick')).toBeNull();
    expect(Array.from(el.querySelectorAll('.pair button')).map((b) => flat(b))).toEqual([
      'Deixar passar',
      'Esquivar agora',
    ]);
  });

  it('releases a readied spell with "Soltar", naming the creature it chose, and closes', async () => {
    answerReaction.mockResolvedValue({ encounter: encounter({ revision: 7 } as never) });
    const { el, settle, button } = setup({
      row: {
        kind: ReadiedKind.SPELL,
        actionKey: 'spell:magic-missile',
        actionNamePt: 'Mísseis Mágicos',
        targetCombatantId: 'z1',
      },
    });
    await settle();
    expect(flat(el.querySelector('h2'))).toBe('Soltar Mísseis Mágicos');
    expect(flat(el.querySelector('legend'))).toBe('Em quem (opcional)');
    expect(Array.from(el.querySelectorAll('.pair button')).map((b) => flat(b))).toEqual([
      'Deixar passar',
      'Soltar no Zumbi 1',
    ]);
    expect(el.querySelector('#readied-limit')).toBeNull();
    button('Soltar').click();
    await settle();
    expect(answerReaction).toHaveBeenCalledWith(
      'c',
      'enc',
      'w1',
      { use: true, creatureIds: ['z1'], die: { inApp: true } },
      expect.any(String),
    );
    expect(close).toHaveBeenCalled();
  });

  it('releases a readied spell with no creature chosen: the server uses the one named at Ready', async () => {
    answerReaction.mockResolvedValue({ encounter: encounter({ revision: 8 } as never) });
    const { settle, button } = setup({
      row: {
        kind: ReadiedKind.SPELL,
        actionKey: 'spell:sleep',
        actionNamePt: 'Sono',
        targetCombatantId: '',
      },
    });
    await settle();
    button('Soltar agora').click();
    await settle();
    expect(answerReaction).toHaveBeenCalledWith(
      'c',
      'enc',
      'w1',
      { use: true, die: { inApp: true } },
      expect.any(String),
    );
  });

  it('lets a move only pass, and says so', async () => {
    const move = setup({
      row: { kind: ReadiedKind.MOVE, moveFt: 25, actionNamePt: '', targetCombatantId: '' },
    });
    await move.settle();
    expect(flat(move.el.querySelector('.what'))).toContain('Mover agora, até 7,5 m?');
    expect(move.el.querySelectorAll('.pair button').length).toBe(1);
    expect(flat(move.el.querySelector('#readied-limit'))).toContain(
      'Por aqui só dá para deixar passar.',
    );
  });

  it("writes the server's refusal in a red band and stays open", async () => {
    answerReaction.mockRejectedValue(new ConnectError('não', Code.FailedPrecondition));
    const { el, settle, button } = setup();
    await settle();
    button('Atacar o Zumbi 1').click();
    await settle();
    expect(el.querySelector('[role=alert]')).not.toBeNull();
    expect(close).not.toHaveBeenCalled();
  });

  it('says only that the window closed when it did before the clock ran out, and only closes', async () => {
    const { el, settle, button } = setup({ gone: true });
    await settle();
    expect(flat(el.querySelector('.what'))).toBe('A janela fechou.');
    expect(el.querySelector('.pair')).toBeNull();
    button('Fechar').click();
    expect(close).toHaveBeenCalled();
  });

  it('says it passed by itself when the clock ran out before the window closed', async () => {
    vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] });
    try {
      const { el, settle, state } = setup();
      await settle();
      vi.advanceTimersByTime(31_000);
      state.apply(
        encounter({
          revision: 9,
          combatants: [combatant({ id: 'tor', label: 'Tor', mine: true })],
          reactionWindows: [],
        } as never),
      );
      await settle();
      expect(flat(el.querySelector('.what'))).toBe('Passou sozinho: você não respondeu em 30 s.');
    } finally {
      vi.useRealTimers();
    }
  });

  it('moves the focus to "Fechar" when the window goes away under the prompt', async () => {
    const { el, settle, button, state } = setup();
    await settle();
    expect(document.activeElement).toBe(button('Deixar passar'));
    state.apply(
      encounter({
        revision: 9,
        combatants: [combatant({ id: 'tor', label: 'Tor', mine: true })],
        reactionWindows: [],
      } as never),
    );
    await settle();
    expect(el.querySelector('.pair')).toBeNull();
    expect(document.activeElement).toBe(button('Fechar'));
  });

  it('can still pass when the list of creatures cannot be read', async () => {
    const { el, settle } = setup({ fail: true });
    await settle();
    expect(flat(el.querySelector('.body'))).toContain('Não deu para ler quem está ao alcance');
    expect(Array.from(el.querySelectorAll('.pair button')).map((b) => flat(b))[0]).toBe(
      'Deixar passar',
    );
  });
});
