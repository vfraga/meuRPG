import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AttackOutcome,
  EncounterBlockedReason,
  EncounterBlockedSchema,
  ReactionKind,
  ReactionRollKind,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { inspirationOffer } from '../../../../core/play/scene-testing';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { ReactionSheet, type ReactionSheetData } from './reaction-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (text: string | null | undefined) =>
  (text ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const blocked = (reason: EncounterBlockedReason) =>
  new ConnectError('x', Code.FailedPrecondition, undefined, [
    { desc: EncounterBlockedSchema, value: create(EncounterBlockedSchema, { reason }) },
  ]);

function win(case_: string, value: object, over: object = {}) {
  return reactionWindow({
    id: 'w1',
    reactorId: 'me',
    reactorLabel: 'Brisa',
    reactorIsPlayer: true,
    prompt: { case: case_, value } as never,
    ...over,
  } as never);
}

const shield = () =>
  win(
    'shield',
    {
      attackerLabel: 'Capitão Goblin',
      attackNamePt: 'Cimitarra',
      spellNamePt: 'Escudo Arcano',
      slots: [
        { level: 1, pact: false, free: 2 },
        { level: 2, pact: false, free: 1 },
      ],
    },
    { kind: ReactionKind.SHIELD },
  );

describe('ReactionSheet', () => {
  const resources = { answerOutsideInspiration: vi.fn() };
  const api = {
    answerReaction: vi.fn(),
    resolveConcentrationSave: vi.fn(),
    markReactionRolling: vi.fn(),
    get: vi.fn(),
  };
  const closed: unknown[] = [];
  let opened = shield();

  function setup(
    window = shield(),
    over: Partial<ReactionSheetData> & {
      announced?: boolean;
      others?: ReturnType<typeof win>[];
    } = {},
  ) {
    opened = window;
    const state = new CombatState();
    state.apply(encounter({ reactionWindows: [window, ...(over.others ?? [])] } as never));
    if (over.announced !== false) {
      state.noteReactionOpened(window.id);
    }
    const data: ReactionSheetData = {
      campaignId: 'camp',
      encounterId: 'enc',
      window,
      round: 2,
      usage: [
        { level: 1, total: 4, used: 2 },
        { level: 2, total: 2, used: 1 },
        { level: 3, total: 2, used: 0 },
      ],
      pact: null,
      state,
      armorClass: 15,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      ...over,
    };
    closed.length = 0;
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        {
          provide: MatDialogRef,
          useValue: { close: (r?: unknown) => closed.push(r ?? 'closed'), disableClose: true },
        },
        { provide: CombatClient, useValue: fakeCombatClient(api) },
        { provide: ResourceClient, useValue: resources },
      ],
    });
    const fixture = TestBed.createComponent(ReactionSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    return { fixture, el, state, data };
  }

  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      plain(b.textContent).includes(label),
    )!;
  const flush = async (fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) => {
    await fixture.whenStable();
    fixture.detectChanges();
  };
  const sent = () => api.answerReaction.mock.calls.at(-1)!;

  beforeEach(() => {
    for (const fn of Object.values(api)) {
      fn.mockReset();
    }
    api.answerReaction.mockImplementation(async () => {
      throw new Error('not stubbed');
    });
    // The server answers with the combat, the window still open (and now marked).
    api.markReactionRolling.mockImplementation(async () =>
      encounter({ reactionWindows: [{ ...opened, rolling: true }] } as never),
    );
  });

  describe('the question', () => {
    it('keeps the Escudo copy, with two buttons of one size and the safe one first in focus', () => {
      const { el } = setup();
      expect(el.querySelector('h2')?.textContent).toBe('Você foi atingido');
      expect(plain(el.querySelector('.frame__sub')?.textContent)).toBe(
        'Capitão Goblin · Cimitarra · Rodada 2',
      );
      expect(plain(el.textContent)).toContain(
        'Usar Escudo Arcano? A sua CA sobe 5 até o começo do seu próximo turno, e o ataque pode virar erro. Gasta a sua reação e um espaço de magia.',
      );
      const use = button(el, 'Conjurar Escudo Arcano');
      const pass = button(el, 'Deixar passar');
      expect(use.classList).toContain('pair__btn');
      expect(pass.classList).toContain('pair__btn');
      expect(pass.hasAttribute('data-initial-focus')).toBe(true);
      expect(use.hasAttribute('data-initial-focus')).toBe(false);
      // An optional reaction passes by itself after 30 s (the master's screen sends the pass).
      expect(plain(el.textContent)).toMatch(/Se você não responder, passa sozinho em (29|30) s\./);
    });

    it('lists the slots as radios with the lowest free one chosen', () => {
      const { el } = setup();
      const radios = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
      // The circles the character has from the spell's own up; the one with no free slot is listed and off.
      expect(radios).toHaveLength(3);
      expect(radios[0].checked).toBe(true);
      expect(radios[2].disabled).toBe(true);
    });

    it('says "Combate atualizado agora." only when the prompt came with a read (a reload)', () => {
      expect(plain(setup().el.textContent)).not.toContain('Combate atualizado agora.');
      TestBed.resetTestingModule();
      const reloaded = setup(shield(), { announced: false });
      expect(plain(reloaded.el.textContent)).toContain('Combate atualizado agora.');
      expect(reloaded.el.querySelector('[role="status"]')?.textContent).toContain(
        'Combate atualizado agora.',
      );
    });
  });

  describe('Escudo', () => {
    it('sends the chosen slot with a key, and shows what it did with "Fechar"', async () => {
      api.answerReaction.mockResolvedValue({
        encounter: encounter({ reactionWindows: [] } as never),
        result: {
          used: true,
          kind: ReactionKind.SHIELD,
          nextWindowId: '',
          result: { case: 'shield', value: { stopped: true } },
        },
      });
      const { fixture, el } = setup();
      button(el, 'Conjurar Escudo Arcano').click();
      await flush(fixture);
      const [campaign, enc, id, answer, key] = sent();
      expect([campaign, enc, id]).toEqual(['camp', 'enc', 'w1']);
      expect(answer).toEqual({ use: true, slot: { level: 1, pact: false } });
      expect(key).toEqual(expect.any(String));
      expect(el.querySelector('h2')?.textContent).toBe('Escudo Arcano conjurado');
      expect(plain(el.textContent)).toContain(
        'O Escudo Arcano segurou o ataque do Capitão Goblin.',
      );
      expect(plain(el.textContent)).toContain('Sua CA é 20 até o começo do seu próximo turno.');
      expect(plain(el.textContent)).toContain('Espaços de 1º nível: 1 livre de 4');
      button(el, 'Fechar').click();
      expect(closed).toEqual(['closed']);
    });

    it('keeps the key for the same answer and makes a new one for another slot', async () => {
      api.answerReaction.mockRejectedValue(new ConnectError('timeout', Code.Unavailable));
      const { fixture, el } = setup();
      button(el, 'Conjurar Escudo Arcano').click();
      await flush(fixture);
      button(el, 'Conjurar Escudo Arcano').click();
      await flush(fixture);
      expect(api.answerReaction.mock.calls[1][4]).toBe(api.answerReaction.mock.calls[0][4]);
      el.querySelectorAll<HTMLInputElement>('input[type="radio"]')[1].click();
      fixture.detectChanges();
      button(el, 'Conjurar Escudo Arcano').click();
      await flush(fixture);
      expect(api.answerReaction.mock.calls[2][4]).not.toBe(api.answerReaction.mock.calls[0][4]);
      expect(el.querySelector('[role="alert"]')).not.toBeNull();
    });

    it('"Deixar passar" passes and closes the sheet', async () => {
      api.answerReaction.mockResolvedValue({ encounter: encounter(), result: undefined });
      const { fixture, el } = setup();
      button(el, 'Deixar passar').click();
      await flush(fixture);
      expect(sent()[3]).toEqual({ use: false });
      expect(closed).toEqual(['closed']);
    });
  });

  describe('keys', () => {
    it('Escape passes', async () => {
      api.answerReaction.mockResolvedValue({ encounter: encounter(), result: undefined });
      const { fixture, el } = setup();
      el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
      await flush(fixture);
      expect(sent()[3]).toEqual({ use: false });
    });

    it('the arrows change the slot from the buttons, and the radios keep their own arrows', async () => {
      api.answerReaction.mockRejectedValue(new Error('stop'));
      const { fixture, el } = setup();
      const pass = button(el, 'Deixar passar');
      pass.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
      fixture.detectChanges();
      expect(el.querySelectorAll<HTMLInputElement>('input[type="radio"]')[1].checked).toBe(true);
      pass.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true }));
      fixture.detectChanges();
      expect(el.querySelectorAll<HTMLInputElement>('input[type="radio"]')[0].checked).toBe(true);
      // An arrow on a radio is the browser's own: the sheet leaves it.
      const radio = el.querySelector<HTMLInputElement>('input[type="radio"]')!;
      radio.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
      fixture.detectChanges();
      expect(radios(el)[0].checked).toBe(true);
    });
  });

  describe('Esquiva Sobrenatural', () => {
    const dodge = () =>
      win(
        'uncannyDodge',
        { attackerLabel: 'Hobgoblin', attackNamePt: 'Espada longa', damage: 9, halved: 4 },
        { kind: ReactionKind.UNCANNY_DODGE },
      );

    it('asks with the damage, sends no slot, and tells the new damage', async () => {
      api.answerReaction.mockResolvedValue({
        encounter: encounter(),
        result: {
          used: true,
          nextWindowId: '',
          result: { case: 'uncannyDodge', value: { damageBefore: 9, damageAfter: 4 } },
        },
      });
      const { fixture, el } = setup(dodge());
      expect(plain(el.textContent)).toContain(
        'O ataque causaria 9 de dano; ele cai pela metade, para 4.',
      );
      expect(plain(el.textContent)).toContain('Sem espaço de magia');
      expect(el.querySelector('input[type="radio"]')).toBeNull();
      button(el, 'Usar Esquiva Sobrenatural').click();
      await flush(fixture);
      expect(sent()[3]).toEqual({ use: true });
      expect(plain(el.textContent)).toContain('O dano do ataque caiu de 9 para 4.');
    });
  });

  describe('Repreensão Infernal', () => {
    const rebuke = () =>
      win(
        'hellishRebuke',
        {
          aggressorLabel: 'Hobgoblin',
          attackNamePt: 'Espada longa',
          distanceFt: 5,
          saveDc: 13,
          options: [
            { racial: true, level: 2, diceCount: 3, usesLeft: 1 },
            { racial: false, level: 2, diceCount: 3, slot: { level: 2, pact: true, free: 2 } },
          ],
        },
        { kind: ReactionKind.HELLISH_REBUKE },
      );

    it('lists the Infernal Legacy and the pact slot, and the arrows switch between them', async () => {
      api.answerReaction.mockRejectedValue(new Error('stop'));
      const { fixture, el } = setup(rebuke());
      expect(el.querySelector('h2')?.textContent).toBe('Você sofreu dano');
      expect(plain(el.textContent)).toContain('Legado Infernal');
      expect(plain(el.textContent)).toContain(
        '2º nível, sem gastar espaço · 1 uso por descanso longo',
      );
      expect(plain(el.textContent)).toContain('Espaço de pacto');
      expect(plain(el.textContent)).toContain('(CD 13)');
      button(el, 'Usar Repreensão Infernal').click();
      await flush(fixture);
      expect(sent()[3]).toEqual({ use: true, useRacial: true });
      button(el, 'Deixar passar').dispatchEvent(
        new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }),
      );
      fixture.detectChanges();
      button(el, 'Usar Repreensão Infernal').click();
      await flush(fixture);
      expect(sent()[3]).toEqual({ use: true, slot: { level: 2, pact: true } });
    });
  });

  describe('Contramágica', () => {
    const counter = () =>
      win(
        'counterspell',
        {
          casterLabel: 'Mago 1',
          distanceFt: 30,
          slotOptions: [{ level: 3, pact: false, free: 2 }],
        },
        { kind: ReactionKind.COUNTERSPELL },
      );

    it('never shows a spell or a level, asks for the die when the server needs it, and sends the typed face', async () => {
      api.answerReaction.mockRejectedValueOnce(blocked(EncounterBlockedReason.REACTION_NEEDS_ROLL));
      const { fixture, el } = setup(counter());
      expect(el.querySelector('h2')?.textContent).toBe('Mago 1 está conjurando');
      expect(plain(el.textContent)).toContain('não sabe qual magia é nem o nível dela');
      button(el, 'Usar Contramágica').click();
      await flush(fixture);
      expect(sent()[3]).toEqual({ use: true, slot: { level: 3, pact: false } });
      // The die: the roll step, with "Digitar o resultado".
      expect(plain(el.textContent)).toContain('faça o teste de habilidade de conjuração');
      api.answerReaction.mockResolvedValueOnce({
        encounter: encounter(),
        result: {
          used: true,
          nextWindowId: '',
          result: {
            case: 'counterspell',
            value: {
              countered: false,
              spellNamePt: 'Bola de Fogo',
              spellLevel: 4,
              checkDc: 14,
              check: {
                diceCount: 1,
                diceSides: 20,
                faces: [9],
                modifier: 0,
                total: 9,
                physical: true,
              },
            },
          },
        },
      });
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      const field = el.querySelector<HTMLInputElement>('input[type="text"]')!;
      field.value = '9';
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      button(el, 'Confirmar').click();
      await flush(fixture);
      expect(sent()[3]).toEqual({ use: true, slot: { level: 3, pact: false }, die: { typed: 9 } });
      expect(el.querySelector('h2')?.textContent).toBe('Contramágica usada');
      expect(plain(el.textContent)).toContain('Não anulada');
      expect(plain(el.textContent)).toContain('Bola de Fogo, de 4º nível');
    });
  });

  describe('Palavras de Interrupção', () => {
    const words = () =>
      win(
        'cuttingWords',
        {
          rollKind: ReactionRollKind.ATTACK,
          rollerLabel: 'Hobgoblin',
          targetLabel: 'Toren',
          distanceFt: 40,
          dieSides: 8,
          usesLeft: 3,
        },
        { kind: ReactionKind.CUTTING_WORDS },
      );
    const done = {
      encounter: encounter(),
      result: {
        used: true,
        nextWindowId: '',
        result: {
          case: 'cuttingWords',
          value: {
            effective: true,
            die: { diceSides: 8, total: 4 },
            outcome: AttackOutcome.MISS,
            rollKind: ReactionRollKind.ATTACK,
          },
        },
      },
    };

    it('lets the app roll the die when the player rolls in the app', async () => {
      api.answerReaction.mockResolvedValue(done);
      const { fixture, el } = setup(words());
      expect(el.querySelector('h2')?.textContent).toBe('O Hobgoblin vai atacar');
      button(el, 'Usar Palavras de Interrupção').click();
      await flush(fixture);
      expect(sent()[3]).toEqual({ use: true, die: { inApp: true } });
      expect(plain(el.textContent)).toContain('O d8 saiu 4 e foi subtraído da rolagem.');
      expect(plain(el.textContent)).toContain('O total e a CA não aparecem');
    });

    it('asks for the physical die when the table rolls its own', async () => {
      api.answerReaction.mockResolvedValue(done);
      const { fixture, el } = setup(words(), { diceMode: DiceMode.PHYSICAL });
      button(el, 'Usar Palavras de Interrupção').click();
      await flush(fixture);
      expect(api.answerReaction).not.toHaveBeenCalled();
      const field = el.querySelector<HTMLInputElement>('input[type="text"]')!;
      field.value = '4';
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      button(el, 'Confirmar').click();
      await flush(fixture);
      expect(sent()[3]).toEqual({ use: true, die: { typed: 4 } });
    });
  });

  describe('the roll step tells the table it is rolling', () => {
    const words = () =>
      win(
        'cuttingWords',
        {
          rollKind: ReactionRollKind.ATTACK,
          rollerLabel: 'Hobgoblin',
          targetLabel: 'Toren',
          dieSides: 8,
          usesLeft: 3,
        },
        { kind: ReactionKind.CUTTING_WORDS },
      );

    it('marks the window once on the way to a physical die and shows the longer wait', async () => {
      const { fixture, el } = setup(words(), { diceMode: DiceMode.PHYSICAL });
      expect(plain(el.textContent)).toContain('passa sozinho em 30 s');
      button(el, 'Usar Palavras de Interrupção').click();
      await flush(fixture);
      expect(api.markReactionRolling).toHaveBeenCalledTimes(1);
      expect(api.markReactionRolling).toHaveBeenCalledWith('camp', 'enc', 'w1');
      // The roll step has a countdown too: the 2 minutes the table waits for the die.
      expect(plain(el.textContent)).toContain('Você está rolando: a mesa espera 2 min.');
      button(el, 'Voltar').click();
      fixture.detectChanges();
      // Back at the question, the same longer wait is the one the master's clock keeps.
      expect(plain(el.textContent)).toContain('a mesa espera');
      button(el, 'Usar Palavras de Interrupção').click();
      await flush(fixture);
      expect(api.markReactionRolling).toHaveBeenCalledTimes(1);
    });

    it('does not mark the window when the app rolls the die, or when the player passes', async () => {
      api.answerReaction.mockRejectedValue(new Error('stop'));
      const { fixture, el } = setup(words());
      button(el, 'Deixar passar').click();
      await flush(fixture);
      button(el, 'Usar Palavras de Interrupção')?.click();
      await flush(fixture);
      expect(api.markReactionRolling).not.toHaveBeenCalled();
    });

    it('goes on with the roll when telling the server fails', async () => {
      api.markReactionRolling.mockRejectedValue(new Error('offline'));
      const { fixture, el } = setup(words(), { diceMode: DiceMode.PHYSICAL });
      button(el, 'Usar Palavras de Interrupção').click();
      await flush(fixture);
      expect(el.querySelector('input[type="text"]')).not.toBeNull();
      expect(plain(el.textContent)).not.toContain('Você está rolando');
    });
  });

  describe('Defletir Projéteis', () => {
    const deflect = () =>
      win(
        'deflectMissiles',
        {
          attackerLabel: 'Goblin',
          attackNamePt: 'Arco curto',
          damage: 9,
          dieSides: 10,
          flatBonus: 8,
          dexMod: 3,
          monkLevel: 5,
        },
        { kind: ReactionKind.DEFLECT_MISSILES },
      );
    const throwWindow = () =>
      win(
        'deflectThrow',
        { kiLeft: 4, normalRangeFt: 20, longRangeFt: 60 },
        { id: 'w2', kind: ReactionKind.DEFLECT_MISSILES, secondStep: true },
      );
    const caught = () => ({
      encounter: encounter({ reactionWindows: [throwWindow()] } as never),
      result: {
        used: true,
        nextWindowId: 'w2',
        result: {
          case: 'deflectMissiles',
          value: {
            reduction: {
              diceCount: 1,
              diceSides: 10,
              faces: [7],
              modifier: 8,
              total: 15,
              physical: false,
            },
            damageBefore: 9,
            damageAfter: 0,
            caught: true,
            throwBackAvailable: true,
          },
        },
      },
    });

    it('catches the missile and offers "Devolver (1 de chi)" and "Guardar a flecha", the safe one in focus', async () => {
      api.answerReaction.mockResolvedValue(caught());
      const { fixture, el } = setup(deflect());
      button(el, 'Usar Defletir Projéteis').click();
      await flush(fixture);
      expect(sent()[3]).toEqual({ use: true, die: { inApp: true } });
      expect(el.querySelector('h2')?.textContent).toBe('Você apanhou a flecha');
      expect(plain(el.textContent)).toContain('Devolver o ataque? Gasta 1 de chi (4 restantes)');
      expect(button(el, 'Guardar a flecha').hasAttribute('data-initial-focus')).toBe(true);
      expect(button(el, 'Devolver (1 de chi)')).toBeDefined();
    });

    it('"Devolver" closes the sheet naming the window, for the attack with catch_window_id', async () => {
      api.answerReaction.mockResolvedValue(caught());
      const { fixture, el } = setup(deflect());
      button(el, 'Usar Defletir Projéteis').click();
      await flush(fixture);
      button(el, 'Devolver (1 de chi)').click();
      expect(closed).toEqual([{ throwWindowId: 'w2' }]);
    });

    it('"Guardar a flecha" passes the second window', async () => {
      api.answerReaction.mockResolvedValue(caught());
      const { fixture, el } = setup(deflect());
      button(el, 'Usar Defletir Projéteis').click();
      await flush(fixture);
      api.answerReaction.mockResolvedValue({ encounter: encounter(), result: undefined });
      button(el, 'Guardar a flecha').click();
      await flush(fixture);
      const [, , id, answer] = sent();
      expect(id).toBe('w2');
      expect(answer).toEqual({ use: false });
      expect(closed).toEqual(['closed']);
    });

    it('comes back at the second step after a reload', () => {
      const { el } = setup(throwWindow());
      expect(button(el, 'Devolver (1 de chi)')).toBeDefined();
      expect(button(el, 'Guardar a flecha')).toBeDefined();
      expect(plain(el.textContent)).toContain('Gasta 1 de chi (4 restantes)');
    });
  });

  describe('Queda Suave', () => {
    const fall = () =>
      win(
        'featherFall',
        {
          falling: [
            { combatantId: 't', label: 'Toren', ally: true, distanceFt: 15 },
            { combatantId: 'g', label: 'Goblin 2', ally: false, distanceFt: 25 },
          ],
          maxTargets: 1,
          fallFt: 20,
          slotOptions: [{ level: 1, pact: false, free: 2 }],
        },
        { kind: ReactionKind.FEATHER_FALL },
      );

    it('lists who falls with boxes of 44 px, allies chosen, and at most max_targets', async () => {
      api.answerReaction.mockResolvedValue({
        encounter: encounter(),
        result: {
          used: true,
          nextWindowId: '',
          result: { case: 'featherFall', value: { savedIds: ['t'] } },
        },
      });
      const { fixture, el } = setup(fall());
      expect(el.querySelector('h2')?.textContent).toBe('2 criaturas estão caindo');
      const boxes = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]'));
      expect(boxes.map((b) => b.checked)).toEqual([true, false]);
      // One is the most: the other box is off until one is taken out.
      expect(boxes[1].disabled).toBe(true);
      expect(plain(el.textContent)).toContain('aliado');
      button(el, 'Usar Queda Suave').click();
      await flush(fixture);
      expect(sent()[3]).toEqual({ use: true, slot: { level: 1, pact: false }, creatureIds: ['t'] });
      expect(plain(el.textContent)).toContain('Toren desce devagar e não sofreu dano de queda');
    });

    it('does not use the spell with nobody chosen', () => {
      const { fixture, el } = setup(fall());
      el.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click();
      fixture.detectChanges();
      expect(button(el, 'Usar Queda Suave').getAttribute('aria-disabled')).toBe('true');
    });
  });

  describe('the concentration save', () => {
    const save = (mode: Record<string, unknown> = {}) =>
      win(
        'concentrationSave',
        {
          spellNamePt: 'Constrição',
          damageTaken: 28,
          dc: 14,
          sourceLabel: 'Bola de Fogo do Mago 1',
          saveBonus: 2,
          bonusKnown: true,
          ...mode,
        },
        { kind: ReactionKind.CONCENTRATION_SAVE },
      );
    const lost = {
      encounter: encounter(),
      result: {
        save: { diceCount: 1, diceSides: 20, faces: [7], modifier: 2, total: 9, physical: false },
        dc: 14,
        kept: false,
        spellKey: 'spell:entangle',
        spellNamePt: 'Constrição',
      },
    };

    it('opens on "Rolar no app", with the three ways and no "Deixar passar"', () => {
      const { el } = setup(save());
      expect(el.querySelector('h2')?.textContent).toBe('Concentração em risco');
      expect(button(el, 'Rolar no app').hasAttribute('data-initial-focus')).toBe(true);
      expect(button(el, 'Digitar o resultado')).toBeDefined();
      expect(button(el, 'Deixar o mestre rolar por mim')).toBeDefined();
      expect(plain(el.textContent)).not.toContain('Deixar passar');
    });

    it('asks about the Bardic Inspiration die before the result, and the answer settles the save', async () => {
      // The server keeps the window open while the question waits, so the answer still carries it.
      const held = save();
      api.resolveConcentrationSave.mockResolvedValue({
        encounter: encounter({ reactionWindows: [held] } as never),
        result: undefined,
        offer: inspirationOffer(7, 2),
      });
      const { fixture, el } = setup(held);
      button(el, 'Rolar no app').click();
      await flush(fixture);
      expect(plain(el.textContent)).toContain('Usar a Inspiração de Bardo (d8)?');
      expect(plain(el.textContent)).not.toContain('Concentração perdida');
      expect(plain(el.textContent)).not.toContain('Deixar o mestre rolar por mim');
      resources.answerOutsideInspiration.mockResolvedValue({
        result: { case: 'concentrationSave', value: lost },
      });
      button(el, 'Guardar o dado').click();
      await flush(fixture);
      expect(resources.answerOutsideInspiration.mock.calls[0].slice(0, 3)).toEqual([
        'camp',
        'h1',
        false,
      ]);
      expect(plain(el.textContent)).toContain('Concentração perdida');
    });

    it('rolls in the app and shows what was lost', async () => {
      api.resolveConcentrationSave.mockResolvedValue(lost);
      const { fixture, el } = setup(save());
      button(el, 'Rolar no app').click();
      await flush(fixture);
      const [campaign, enc, id, how, key] = api.resolveConcentrationSave.mock.calls[0];
      expect([campaign, enc, id, how]).toEqual(['camp', 'enc', 'w1', { kind: 'app' }]);
      expect(key).toEqual(expect.any(String));
      expect(el.querySelector('h2')?.textContent).toBe('Você perdeu a concentração');
      expect(plain(el.textContent)).toContain('Concentração perdida');
      expect(plain(el.textContent)).toContain('contra CD 14');
    });

    it('types the d20, or leaves the roll to the master', async () => {
      api.resolveConcentrationSave.mockResolvedValue({ encounter: encounter(), result: undefined });
      const { fixture, el } = setup(save());
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      const field = el.querySelector<HTMLInputElement>('input[type="text"]')!;
      field.value = '12';
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      button(el, 'Confirmar').click();
      await flush(fixture);
      expect(api.resolveConcentrationSave.mock.calls[0][3]).toEqual({ kind: 'typed', face: 12 });
      TestBed.resetTestingModule();
      const again = setup(save());
      button(again.el, 'Deixar o mestre rolar por mim').click();
      await flush(again.fixture);
      expect(api.resolveConcentrationSave.mock.calls[1][3]).toEqual({ kind: 'hand' });
      expect(closed).toEqual(['closed']);
    });

    it('says nothing of the mode and asks for one d20 when the roll is normal', () => {
      const { fixture, el } = setup(save({ mode: 'normal' }));
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      expect(plain(el.textContent)).not.toContain('desvantagem');
      expect(plain(el.textContent)).not.toContain('vantagem');
      expect(el.querySelectorAll('input[type="text"]')).toHaveLength(1);
    });

    it('says the mode and asks for both d20 with disadvantage, and sends both', async () => {
      api.resolveConcentrationSave.mockResolvedValue({ encounter: encounter(), result: undefined });
      const { fixture, el } = setup(
        save({ mode: 'disadvantage', modeSourcesPt: ['Exaustão 3: desvantagem'] }),
      );
      expect(plain(el.textContent)).toContain('Você tem desvantagem neste teste');
      expect(plain(el.textContent)).toContain('Exaustão 3: desvantagem');
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      const fields = el.querySelectorAll<HTMLInputElement>('input[type="text"]');
      expect(fields).toHaveLength(2);
      [15, 3].forEach((n, i) => {
        fields[i].value = String(n);
        fields[i].dispatchEvent(new Event('input'));
      });
      fixture.detectChanges();
      button(el, 'Confirmar').click();
      await flush(fixture);
      expect(api.resolveConcentrationSave.mock.calls[0][3]).toEqual({
        kind: 'typed',
        face: 15,
        second: 3,
      });
    });

    it('asks for the d4 of an effect the server says the save takes, and sends it with the d20', async () => {
      api.resolveConcentrationSave
        .mockRejectedValueOnce(
          new ConnectError('the roll takes 1 more die(s): type their faces', Code.InvalidArgument),
        )
        .mockResolvedValue({ encounter: encounter(), result: undefined });
      const { fixture, el } = setup(save());
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      const type = (i: number, text: string) => {
        const field = el.querySelectorAll<HTMLInputElement>('input[type="text"]')[i];
        field.value = text;
        field.dispatchEvent(new Event('input'));
        fixture.detectChanges();
      };
      type(0, '12');
      button(el, 'Confirmar').click();
      await flush(fixture);
      expect(api.resolveConcentrationSave.mock.calls[0][3]).toEqual({ kind: 'typed', face: 12 });
      expect(plain(el.textContent)).toContain('Esta rolagem leva mais um d4');
      type(0, '3');
      type(1, '12');
      button(el, 'Confirmar').click();
      await flush(fixture);
      expect(api.resolveConcentrationSave.mock.calls[1][3]).toEqual({
        kind: 'typed',
        face: 12,
        extra: [3],
      });
    });

    it('Escape does not decide a concentration save', () => {
      const { el } = setup(save());
      el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
      expect(api.answerReaction).not.toHaveBeenCalled();
      expect(closed).toEqual([]);
    });
  });

  describe('when the window is gone', () => {
    it('says the master answered, and closes by itself when the window closed by itself', async () => {
      const { fixture, el, state } = setup();
      state.apply(encounter({ reactionWindows: [], revision: 5 } as never));
      fixture.detectChanges();
      expect(plain(el.textContent)).toContain('O mestre respondeu por você');
      expect(button(el, 'Fechar')).toBeDefined();
      state.noteReactionClosed('w1', 'Queda Suave fechou. Você já usou a sua reação.');
      await flush(fixture);
      expect(closed).toEqual(['closed']);
    });

    it('reads the combat again when an answer came out of order', async () => {
      api.answerReaction.mockRejectedValue(blocked(EncounterBlockedReason.NOT_YOUR_TURN_TO_ANSWER));
      api.get.mockResolvedValue(encounter({ revision: 9, reactionWindows: [shield()] } as never));
      const { fixture, el } = setup();
      button(el, 'Conjurar Escudo Arcano').click();
      await flush(fixture);
      expect(api.get).toHaveBeenCalledWith('camp');
      expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain(
        'não espera a sua resposta agora',
      );
    });
  });
});

function radios(el: HTMLElement): HTMLInputElement[] {
  return Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
}
