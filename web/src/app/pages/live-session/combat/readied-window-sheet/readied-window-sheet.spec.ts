import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import { timestampFromMs } from '@bufbuild/protobuf/wkt';

import {
  AreaPlacement,
  AttackTargetsSchema,
  CombatantState,
  GetTurnOptionsResponseSchema,
  ReactionKind,
  ReadiedActionSchema,
  ReadiedKind,
  ReadiedStatus,
  SpellTargetsSchema,
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
  const markReactionRolling = vi.fn();
  const close = vi.fn();

  function setup(
    opts: {
      row?: Record<string, unknown>;
      fail?: boolean;
      gone?: boolean;
      used?: boolean;
      spellTargets?: Record<string, unknown>;
      /** The map opens for the release: what the sheet calls. */
      toMap?: ReturnType<typeof vi.fn>;
      moveLeft?: number;
      window?: Record<string, unknown>;
      conditions?: string[];
    } = {},
  ) {
    const state = new State();
    state.apply(
      encounter({
        combatants: [
          combatant({
            id: 'tor',
            label: 'Tor',
            mine: true,
            reactionUsed: !!opts.used,
            conditions: opts.conditions ?? [],
            readied: held(opts.row),
          }),
          combatant({ id: 'z1', label: 'Zumbi 1' }),
        ],
        reactionWindows: opts.gone ? [] : [{ ...window, ...opts.window }],
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
        spellTargets: opts.spellTargets
          ? [create(SpellTargetsSchema, opts.spellTargets as never)]
          : [],
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
      window: { ...window, ...opts.window } as typeof window,
      round: 2,
      readied: held(opts.row),
      reactorLabel: 'Tor',
      state,
      toMap: opts.toMap as ReadiedWindowSheetData['toMap'],
    };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close, disableClose: false } },
        {
          provide: CombatClient,
          useValue: fakeCombatClient({
            answerReaction,
            markReactionRolling,
            turnOptions: loadOptions,
            moveOptions: async () => ({ movementLeftDft: opts.moveLeft ?? 300 }),
          }),
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
    markReactionRolling.mockReset();
    markReactionRolling.mockResolvedValue(
      encounter({
        revision: 3,
        combatants: [combatant({ id: 'tor', label: 'Tor', mine: true })],
        // The server's mark: the window rolls and its one wait of 120 s begins now.
        reactionWindows: [{ ...window, rolling: true, waitBeganAt: timestampFromMs(Date.now()) }],
      } as never),
    );
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
    // "Deixar passar" has the focus at open. Whether it shows a ring is the browser's call (the e2e checks it after a click); the sheet forces none.
    expect(document.activeElement).toBe(button('Deixar passar'));
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

  it('splits the darts of a held Magic Missile among the creatures and sends them as targets', async () => {
    answerReaction.mockResolvedValue({ encounter: encounter({ revision: 9 } as never) });
    const { el, settle, button } = setup({
      row: {
        kind: ReadiedKind.SPELL,
        actionKey: 'spell:magic-missile',
        actionNamePt: 'Mísseis Mágicos',
        targetCombatantId: 'z1',
      },
      spellTargets: {
        spellKey: 'spell:magic-missile',
        maxTargets: 3,
        darts: [{ slotLevel: 1, darts: 3 }],
        targets: [reach('tor', 'Tor', 0), reach('z1', 'Zumbi 1', 5)],
      },
    });
    await settle();
    expect(flat(el.querySelector('.cap'))).toBe('Divida os 3 dardos entre os alvos');
    expect(flat(el.querySelector('[role=status]:not(.foot__clock)'))).toContain('0 de 3');
    const use = () => button('Soltar');
    expect(use().getAttribute('aria-disabled')).toBe('true');
    const add = (label: string) =>
      (
        el.querySelector(`button[aria-label="Pôr um dardo em ${label}"]`) as HTMLButtonElement
      ).click();
    add('Zumbi 1');
    add('Zumbi 1');
    add('Tor (você)');
    await settle();
    expect(use().getAttribute('aria-disabled')).toBeNull();
    use().click();
    await settle();
    expect(answerReaction).toHaveBeenCalledWith(
      'c',
      'enc',
      'w1',
      {
        use: true,
        targets: [
          { combatantId: 'z1', darts: 2 },
          { combatantId: 'tor', darts: 1 },
        ],
        die: { inApp: true },
      },
      expect.any(String),
    );
  });

  describe('the table waits longer once the player is choosing (#857)', () => {
    const missiles = {
      row: {
        kind: ReadiedKind.SPELL,
        actionKey: 'spell:magic-missile',
        actionNamePt: 'Mísseis Mágicos',
        targetCombatantId: '',
      },
      spellTargets: {
        spellKey: 'spell:magic-missile',
        maxTargets: 3,
        darts: [{ slotLevel: 1, darts: 3 }],
        targets: [reach('tor', 'Tor', 0), reach('z1', 'Zumbi 1', 5)],
      },
    };
    const add = (el: HTMLElement, label: string) =>
      (
        el.querySelector(`button[aria-label="Pôr um dardo em ${label}"]`) as HTMLButtonElement
      ).click();

    it('marks the window once, on the first dart placed, and says how long the table waits', async () => {
      const { el, settle } = setup(missiles);
      await settle();
      expect(markReactionRolling).not.toHaveBeenCalled();
      expect(flat(el.querySelector('.foot__clock'))).toMatch(/^Se você não responder/);
      add(el, 'Zumbi 1');
      add(el, 'Zumbi 1');
      add(el, 'Tor (você)');
      await settle();
      expect(markReactionRolling).toHaveBeenCalledTimes(1);
      expect(markReactionRolling).toHaveBeenCalledWith('c', 'enc', 'w1');
      expect(flat(el.querySelector('.foot__clock'))).toBe(
        'Você está escolhendo: a mesa espera 2 min. Depois disso, passa sozinho.',
      );
    });

    it('marks it on the first creature ticked of a spell that takes several, and on the first of an attack', async () => {
      const bless = setup({
        row: { kind: ReadiedKind.SPELL, actionKey: 'spell:bless', targetCombatantId: '' },
        spellTargets: {
          spellKey: 'spell:bless',
          maxTargets: 3,
          targets: [reach('tor', 'Tor', 0), reach('z1', 'Zumbi 1', 5)],
        },
      });
      await bless.settle();
      (bless.el.querySelectorAll('input[type=checkbox]')[1] as HTMLInputElement).click();
      (bless.el.querySelectorAll('input[type=checkbox]')[0] as HTMLInputElement).click();
      await bless.settle();
      expect(markReactionRolling).toHaveBeenCalledTimes(1);
      markReactionRolling.mockClear();
      const attack = setup({ row: { targetCombatantId: '' } });
      await attack.settle();
      (attack.el.querySelector('input[type=radio]') as HTMLInputElement).click();
      await attack.settle();
      expect(markReactionRolling).toHaveBeenCalledTimes(1);
    });

    it('does not mark it when the player only lets the window pass', async () => {
      answerReaction.mockResolvedValue({ encounter: encounter({ revision: 6 } as never) });
      const { settle, button } = setup(missiles);
      await settle();
      button('Deixar passar').click();
      await settle();
      expect(markReactionRolling).not.toHaveBeenCalled();
    });

    it('goes back to the 30 s when the master could not be told', async () => {
      markReactionRolling.mockRejectedValue(new Error('down'));
      const { el, settle } = setup(missiles);
      await settle();
      add(el, 'Zumbi 1');
      await settle();
      expect(markReactionRolling).toHaveBeenCalledTimes(1);
      expect(flat(el.querySelector('.foot__clock'))).toMatch(/^Se você não responder/);
    });
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

  it('opens the map for a move with "Mover agora", says what the map does and closes', async () => {
    const toMap = vi.fn();
    const move = setup({
      toMap,
      row: { kind: ReadiedKind.MOVE, moveFt: 25, actionNamePt: '', targetCombatantId: '' },
    });
    await move.settle();
    expect(flat(move.el.querySelector('.what'))).toContain('Mover agora, até 7,5 m?');
    expect(flat(move.el.querySelector('[data-testid=release-map-notice] p'))).toBe(
      'O mapa abre para você escolher o quadrado. Lá a mesa espera até 2 min, e dá para voltar antes de mover.',
    );
    expect(flat(move.el.querySelector('[data-testid=release-map-notice] mat-icon'))).toBe('map');
    expect(
      Array.from(move.el.querySelectorAll('[data-testid=readied-legend] li span')).map((l) =>
        flat(l),
      ),
    ).toEqual([
      'Reação: gasta quando você mover; volta no começo da sua vez.',
      'Provoca ataques de oportunidade, como qualquer movimento.',
    ]);
    expect(Array.from(move.el.querySelectorAll('.pair button')).map((b) => flat(b))).toEqual([
      'Deixar passar',
      'directions_walkMover agora',
    ]);
    move.button('directions_walk').click();
    expect(close).toHaveBeenCalled();
    expect(toMap).toHaveBeenCalledWith(null);
    expect(answerReaction).not.toHaveBeenCalled();
  });

  it('turns "Mover agora" off with a speed of 0, and says why in the grapple\'s words too', async () => {
    const toMap = vi.fn();
    const row = { kind: ReadiedKind.MOVE, moveFt: 25, actionNamePt: '', targetCombatantId: '' };
    const speed0 = setup({ toMap, row, moveLeft: 0 });
    await speed0.settle();
    expect(flat(speed0.el.querySelector('#readied-limit'))).toContain(
      'O seu deslocamento é 0. Dá para deixar passar.',
    );
    const go = speed0.button('directions_walk');
    expect(go.classList.contains('pair__btn--off')).toBe(true);
    expect(go.getAttribute('aria-describedby')).toBe('readied-limit');
    go.click();
    expect(toMap).not.toHaveBeenCalled();
    const grapple = setup({ toMap, row, moveLeft: 0, conditions: ['condition:grappled'] });
    await grapple.settle();
    expect(flat(grapple.el.querySelector('#readied-limit'))).toContain(
      'Condição Agarrado: o seu deslocamento é 0.',
    );
  });

  it('releases a held spell that needs the map with "Soltar no mapa": the sheet closes and the map gets the plan', async () => {
    const toMap = vi.fn();
    const { el, settle, button } = setup({
      toMap,
      row: {
        kind: ReadiedKind.SPELL,
        actionKey: 'spell:fireball',
        actionNamePt: 'Bola de Fogo',
        targetCombatantId: '',
        slotLevel: 3,
      },
      spellTargets: { spellKey: 'spell:fireball', placement: AreaPlacement.POINT },
    });
    await settle();
    expect(flat(el.querySelector('[data-testid=release-map-notice] p'))).toBe(
      'Ela pede um ponto no mapa. Lá a mesa espera até 2 min, nada sai antes de “Conjurar” e dá para voltar.',
    );
    expect(
      Array.from(el.querySelectorAll('[data-testid=readied-legend] li span')).map((l) => flat(l)),
    ).toEqual([
      'Reação: gasta quando a magia sair.',
      'Sem espaço: o de 3º nível foi gasto ao preparar.',
      'Guardada, a magia prende a sua concentração. Deixar passar perde a magia e o espaço.',
    ]);
    expect(Array.from(el.querySelectorAll('.pair button')).map((b) => flat(b))).toEqual([
      'Deixar passar',
      'mapSoltar no mapa',
    ]);
    button('map').click();
    expect(close).toHaveBeenCalled();
    expect(toMap).toHaveBeenCalledWith(expect.objectContaining({ onMap: true }));
    expect(answerReaction).not.toHaveBeenCalled();
  });

  it('counts the 120 s from the start the server dated, and says it is another device when the player is on the map', async () => {
    const began = timestampFromMs(Date.now() - 48_000);
    const mine = setup({ window: { waitBeganAt: began, waitedMs: 48_000n } });
    await mine.settle();
    expect(flat(mine.el.querySelector('[data-testid=readied-clock]'))).toBe(
      'Se você não responder, passa sozinho em 1 min 12 s.',
    );
    const other = setup({
      toMap: vi.fn(),
      row: { kind: ReadiedKind.MOVE, moveFt: 25, actionNamePt: '' },
      window: { waitBeganAt: began, waitedMs: 48_000n, rolling: true },
    });
    await other.settle();
    expect(flat(other.el.querySelector('[data-testid=readied-clock]'))).toContain(
      'Você abriu o mapa em outro aparelho. Passa sozinho em 1 min 12 s.',
    );
  });

  it('does not say the map is open elsewhere when the mark is a player choosing targets', async () => {
    const began = timestampFromMs(Date.now() - 48_000);
    const { el, settle } = setup({
      row: { kind: ReadiedKind.ATTACK },
      window: { waitBeganAt: began, waitedMs: 48_000n, rolling: true },
    });
    await settle();
    const clock = flat(el.querySelector('[data-testid=readied-clock]'));
    expect(clock).not.toContain('outro aparelho');
    expect(clock).toBe(
      'Você está escolhendo: a mesa espera 1 min 12 s. Depois disso, passa sozinho.',
    );
  });

  it('says the master undid the trigger when the window goes away and the held spell still waits', async () => {
    const { el, settle, state } = setup({
      row: { kind: ReadiedKind.SPELL, actionKey: 'spell:fireball', actionNamePt: 'Bola de Fogo' },
    });
    await settle();
    state.apply(
      encounter({
        revision: 9,
        combatants: [
          combatant({
            id: 'tor',
            label: 'Tor',
            mine: true,
            readied: held({ kind: ReadiedKind.SPELL, actionNamePt: 'Bola de Fogo' }),
          }),
        ],
        reactionWindows: [],
      } as never),
    );
    await settle();
    expect(flat(el.querySelector('[data-testid=readied-gone]'))).toBe(
      'O mestre desfez o gatilho: a Bola de Fogo continua preparada.',
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

  it('names the darts in the refusal of a held Magic Missile', async () => {
    answerReaction.mockRejectedValue(new ConnectError('não', Code.InvalidArgument));
    const dart = setup({
      row: {
        kind: ReadiedKind.SPELL,
        actionKey: 'spell:magic-missile',
        actionNamePt: 'Mísseis Mágicos',
        targetCombatantId: 'z1',
      },
      spellTargets: {
        spellKey: 'spell:magic-missile',
        maxTargets: 3,
        darts: [{ slotLevel: 1, darts: 3 }],
        targets: [reach('z1', 'Zumbi 1', 5)],
      },
    });
    await dart.settle();
    (
      dart.el.querySelector('button[aria-label="Pôr um dardo em Zumbi 1"]') as HTMLButtonElement
    ).click();
    (
      dart.el.querySelector('button[aria-label="Pôr um dardo em Zumbi 1"]') as HTMLButtonElement
    ).click();
    (
      dart.el.querySelector('button[aria-label="Pôr um dardo em Zumbi 1"]') as HTMLButtonElement
    ).click();
    await dart.settle();
    dart.button('Soltar').click();
    await dart.settle();
    expect(flat(dart.el.querySelector('[role=alert]'))).toContain('Confira os alvos e os dardos.');
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
