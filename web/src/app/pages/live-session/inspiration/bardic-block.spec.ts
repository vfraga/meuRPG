import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  type HeldRollOffer,
  HeldRollKind,
  OutsideInspirationOfferSchema,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { GetOutsideInspirationResponseSchema } from '../../../../gen/meurpg/play/v1/resources_pb';
import { RollSceneCheckResponseSchema } from '../../../../gen/meurpg/play/v1/scene_pb';
import { LUCKY_TRAIT } from '../../../core/combat/lucky';
import {
  heldAnswer,
  heldDie,
  heldIndomitable,
  heldReroll,
} from '../../../core/held-rolls/held-roll-testing';
import { luckyOffer, sceneRoll } from '../../../core/play/scene-testing';
import { ResourceClient } from '../../../core/resources/resources-client';
import { BardicBlock, luckyAnswerText, minutesText } from './bardic-block';

describe('BardicBlock', () => {
  function setup(
    state: Parameters<typeof create<typeof GetOutsideInspirationResponseSchema>>[1],
    listed: HeldRollOffer[] | null = null,
  ) {
    const api = {
      outsideInspiration: vi
        .fn()
        .mockResolvedValue(create(GetOutsideInspirationResponseSchema, state)),
      giveBardicInspirationOutside: vi.fn().mockResolvedValue({}),
      answerHeldRoll: vi.fn(),
      listHeldRolls: vi.fn().mockResolvedValue({ offers: listed ?? [] }),
    };
    TestBed.configureTestingModule({ providers: [{ provide: ResourceClient, useValue: api }] });
    const fixture = TestBed.createComponent(BardicBlock);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('strokeQuestions', listed !== null);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        await fixture.whenStable();
        fixture.detectChanges();
      }
    };
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.includes(name),
      );
    return { api, el, button, settle, fixture };
  }

  it('shows nothing to a character with no die that is not a bard', async () => {
    const { el, settle } = setup({});
    await settle();
    expect(el.textContent?.trim()).toBe('');
  });

  it('tells the holder the die, who gave it and the game time left', async () => {
    const { el, settle } = setup({
      mine: {
        characterId: 'tavo',
        characterName: 'Tavo',
        sides: 6,
        fromName: 'Orla',
        secondsLeft: 540,
      },
    });
    await settle();
    expect(el.textContent).toContain('Você tem uma Inspiração de Bardo (d6) de Orla');
    expect(el.textContent).toContain('9 min de jogo');
  });

  it('promises the die only for the rolls that ask for it', async () => {
    const { el, settle } = setup({
      mine: {
        characterId: 'tavo',
        characterName: 'Tavo',
        sides: 6,
        fromName: 'Orla',
        secondsLeft: 540,
      },
    });
    await settle();
    const text = el.textContent ?? '';
    expect(text).toContain('jogada de ataque');
    expect(text).toContain('teste de resistência que você mesmo rola');
    expect(text).toContain('Os testes que o app rola por você');
    expect(text).not.toContain('salvaguarda');
  });

  it("shows a question that waits for the die in a combat, where the die is the combatant's", async () => {
    const { el, settle } = setup({
      offers: [
        {
          holdId: 'h1',
          sides: 8,
          fromName: 'Orla',
          d20: { faces: [4], countedIndex: 0, modifier: 1, total: 5 },
        },
      ],
    });
    await settle();
    expect(el.textContent).toContain('Usar a Inspiração de Bardo (d8)?');
  });

  it('shows the bard the uses left and the party, greying who already holds a die', async () => {
    const { el, button, settle } = setup({
      isBard: true,
      sides: 8,
      usesLeft: 2,
      usesMax: 3,
      targets: [
        { characterId: 'tavo', name: 'Tavo', disabledReasonPt: '' },
        { characterId: 'nael', name: 'Nael', disabledReasonPt: 'Já tem um dado' },
      ],
    });
    await settle();
    expect(el.textContent).toContain('Usos: 2 de 3');
    expect(button('Dar a Tavo')!.disabled).toBe(false);
    expect(button('Dar a Nael')!.getAttribute('aria-disabled')).toBe('true');
    expect(el.textContent).toContain('Já tem um dado');
  });

  it('gives the die with one request and reads the state again', async () => {
    const { api, button, settle } = setup({
      isBard: true,
      sides: 8,
      usesLeft: 1,
      usesMax: 3,
      targets: [{ characterId: 'tavo', name: 'Tavo', disabledReasonPt: '' }],
    });
    await settle();
    button('Dar a Tavo')!.click();
    await settle();
    expect(api.giveBardicInspirationOutside).toHaveBeenCalledWith('c1', 'tavo', expect.any(String));
    expect(api.outsideInspiration).toHaveBeenCalledTimes(2);
  });

  it('does not offer to give in a combat, where the combat sheet does it', async () => {
    const { el, settle } = setup({
      isBard: true,
      sides: 8,
      usesLeft: 3,
      usesMax: 3,
      combatOpen: true,
    });
    await settle();
    expect(el.textContent).not.toContain('Usos:');
  });

  it('asks a held Lucky question again after a reload, in its own section, with no "Inspiração de Bardo" heading', async () => {
    const { el, button, settle } = setup({ luckyOffers: [luckyOffer([1], 4)] });
    await settle();
    const headings = Array.from(el.querySelectorAll('h2'), (h) => h.textContent?.trim());
    expect(headings).toHaveLength(1);
    expect(headings[0]).toContain('Sortudo');
    expect(el.textContent).toContain('Tirou 1: rolar de novo?');
    expect(el.textContent).not.toContain('Inspiração de Bardo');
    expect(button('Ficar com o 1')).toBeTruthy();
  });

  it('asks a reroll rule question again after a reload, for a roll outside the attack sheet, in its own section', async () => {
    const scene = heldReroll(2, 5, { kind: HeldRollKind.SCENE_CHECK, attackKey: '' });
    const { el, button, settle } = setup({}, [scene]);
    await settle();
    const headings = Array.from(el.querySelectorAll('h2'), (h) => h.textContent?.trim());
    expect(headings).toEqual(['casinoRolar de novo']);
    expect(el.textContent).toContain('Tirou 2: rolar de novo?');
    expect(el.textContent).toContain('Sorte Teimosa: você rola este d20 de novo');
    expect(el.textContent).not.toContain('Inspiração de Bardo');
    expect(button('Ficar com o 2')).toBeTruthy();
  });

  it('leaves a weapon attack’s reroll question to the attack sheet', async () => {
    const { el, settle } = setup({}, [
      heldReroll(2, 5, { kind: HeldRollKind.WEAPON_ATTACK, attackKey: 'a' }),
    ]);
    await settle();
    expect(el.textContent?.trim()).toBe('');
  });

  it('keeps an Indomitable question out of the die section, since it has no die', async () => {
    const { el, settle } = setup({}, [
      heldIndomitable(5, 0, true, { kind: HeldRollKind.SCENE_CHECK }),
    ]);
    await settle();
    expect(el.textContent).not.toContain('Inspiração de Bardo');
    expect(el.querySelector('h2')).toBeNull();
  });

  it('answers the reroll question with the server rolling, and says what it came to', async () => {
    const scene = heldReroll(2, 5, { kind: HeldRollKind.SCENE_CHECK, attackKey: '' });
    const { api, el, button, settle } = setup({}, [scene]);
    api.answerHeldRoll.mockResolvedValue(
      heldAnswer({
        result: {
          case: 'sceneCheck',
          value: create(RollSceneCheckResponseSchema, {
            roll: sceneRoll('r1', 'a1', 'Pipo', 19, {
              roll: {
                diceCount: 1,
                diceSides: 20,
                faces: [14],
                modifier: 5,
                total: 19,
                rerolled: [
                  {
                    index: 0,
                    from: 2,
                    to: 14,
                    sourceKey: 'feature:stubborn-luck',
                    sourceName: 'Sorte Teimosa',
                  },
                ],
              },
            }),
          }),
        },
      }),
    );
    await settle();
    button('Rolar de novo')!.click();
    await settle();
    expect(api.answerHeldRoll).toHaveBeenCalledWith(
      'c1',
      'h1',
      { kind: 'reroll', reroll: true, roll: null },
      expect.any(String),
    );
    expect(el.textContent).toContain('Sorte Teimosa: 2 → 14. Seu total: 19.');
  });

  it('answers the Lucky question once per answer and reads the state again', async () => {
    const { api, el, button, settle } = setup({ luckyOffers: [luckyOffer([1], 4)] });
    api.answerHeldRoll.mockResolvedValue(
      heldAnswer({
        result: {
          case: 'sceneCheck',
          value: create(RollSceneCheckResponseSchema, {
            roll: sceneRoll('r1', 'a1', 'Pipo', 18, {
              roll: {
                diceCount: 1,
                diceSides: 20,
                faces: [14],
                modifier: 4,
                total: 18,
                rerolled: [{ index: 0, from: 1, to: 14, sourceKey: LUCKY_TRAIT }],
              },
            }),
          }),
        },
      }),
    );
    await settle();
    button('Rolar de novo')!.click();
    await settle();
    expect(api.answerHeldRoll).toHaveBeenCalledWith(
      'c1',
      'h1',
      { kind: 'lucky', reroll: true, roll: { inApp: true } },
      expect.any(String),
    );
    expect(api.outsideInspiration).toHaveBeenCalledTimes(2);
    expect(el.textContent).toContain('Sortudo: 1 → 14. Seu total: 18.');
  });

  it('answers the question of the die: used in the app, typed or kept, once per answer, then reads the state again', async () => {
    const { api, el, button, settle } = setup({
      offers: [create(OutsideInspirationOfferSchema, { holdId: 'h2', sides: 6, fromName: 'Orla' })],
    });
    api.answerHeldRoll.mockResolvedValue(
      heldAnswer({
        result: {
          case: 'sceneCheck',
          value: create(RollSceneCheckResponseSchema, {
            roll: sceneRoll('r1', 'a1', 'Pipo', 21, {
              roll: { diceCount: 1, diceSides: 20, faces: [14], modifier: 3, total: 21 },
            }),
          }),
        },
      }),
    );
    await settle();
    expect(el.textContent).toContain('Usar a Inspiração de Bardo (d6)?');
    button('Usar e rolar o d6')!.click();
    await settle();
    expect(api.answerHeldRoll).toHaveBeenCalledWith(
      'c1',
      'h2',
      { kind: 'die', use: true, roll: { inApp: true } },
      expect.any(String),
    );
    expect(api.outsideInspiration).toHaveBeenCalledTimes(2);
    expect(el.textContent).toContain('Dado somado. Seu total: 21.');
    button('Guardar o dado')!.click();
    await settle();
    expect(api.answerHeldRoll.mock.calls[1].slice(1, 3)).toEqual([
      'h2',
      { kind: 'die', use: false, roll: null },
    ]);
    const keys = api.answerHeldRoll.mock.calls.map((c) => c[3] as string);
    expect(keys[0]).not.toBe(keys[1]);
  });

  it('says why an answer about the die failed, and keeps the question', async () => {
    const { api, el, button, settle } = setup({
      offers: [create(OutsideInspirationOfferSchema, { holdId: 'h2', sides: 6 })],
    });
    api.answerHeldRoll.mockRejectedValue(new ConnectError('down', Code.Unavailable));
    await settle();
    button('Guardar o dado')!.click();
    await settle();
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('Não deu para responder');
    expect(el.textContent).toContain('Usar a Inspiração de Bardo (d6)?');
  });

  it('words what a Lucky answer came to: the 1 kept, the die’s question next, or the master sees it', () => {
    const next = heldAnswer({
      result: { case: 'trapSearch', value: { heldRollOffer: heldDie(9, 4, 8, {}, 1) } },
    });
    expect(luckyAnswerText(next, true)).toBe(
      'Sortudo: 1 → 9. Agora, a pergunta da Inspiração de Bardo.',
    );
    const kept = heldAnswer({ result: { case: 'hintTry', value: { roll: { faces: [1] } } } });
    expect(luckyAnswerText(kept, false)).toBe('Você ficou com o 1. O mestre vê o resultado.');
  });

  it('writes the minutes of game time left, rounded up', () => {
    expect(minutesText(1)).toBe('1 min');
    expect(minutesText(61)).toBe('2 min');
    expect(minutesText(600)).toBe('10 min');
  });

  it('tells the player the master answered when the question is gone on the next read and they did not answer it here', async () => {
    const offer = {
      holdId: 'h-gone',
      sides: 8,
      fromName: 'Orla',
      d20: { faces: [4], countedIndex: 0, modifier: 1, total: 5 },
    };
    const { api, el, settle, fixture } = setup({ offers: [offer] });
    await settle();
    expect(el.textContent).toContain('Usar a Inspiração de Bardo (d8)?');
    expect(el.textContent).not.toContain('O mestre respondeu por você.');
    api.outsideInspiration.mockResolvedValue(create(GetOutsideInspirationResponseSchema, {}));
    fixture.componentRef.setInput('tick', 1);
    await settle();
    expect(el.textContent).toContain('O mestre respondeu por você.');
  });
});
