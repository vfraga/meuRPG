// A concentration save of a halfling whose d20 shows a 1: the d20 is kept and the window stays open, the sheet asks
// "Tirou 1: rolar de novo?" before the result, and the answer settles the save, or asks about the Bardic Inspiration die
// on the new roll.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  ConcentrationSaveResultSchema,
  LuckyOfferSchema,
  OutsideInspirationOfferSchema,
  ReactionKind,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ResourceBlockedReason,
  ResourceBlockedSchema,
} from '../../../../../gen/meurpg/play/v1/resources_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { LUCKY_TRAIT } from '../../../../core/combat/lucky';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { ReactionSheet, type ReactionSheetData } from './reaction-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (text: string | null | undefined) => (text ?? '').replace(/\s+/g, ' ').trim();

const save = () =>
  reactionWindow({
    id: 'w1',
    kind: ReactionKind.CONCENTRATION_SAVE,
    reactorId: 'me',
    reactorLabel: 'Pipo',
    reactorIsPlayer: true,
    prompt: {
      case: 'concentrationSave',
      value: {
        spellNamePt: 'Bênção',
        damageTaken: 8,
        dc: 10,
        sourceLabel: 'Cimitarra do Goblin',
        saveBonus: 2,
        bonusKnown: true,
      },
    } as never,
  } as never);

const lucky = create(LuckyOfferSchema, {
  holdId: 'h1',
  luckyIndex: 0,
  d20: { diceCount: 1, diceSides: 20, faces: [1], modifier: 2, total: 3 },
});

/** What the answer settles: the save with the new d20 (or the 1 kept). */
const settled = (face: number, again?: number) => ({
  encounter: encounter(),
  result: create(ConcentrationSaveResultSchema, {
    save: {
      diceCount: 1,
      diceSides: 20,
      faces: [face],
      modifier: 2,
      total: face + 2,
      rerolled: again ? [{ index: 0, from: 1, to: again, sourceKey: LUCKY_TRAIT }] : [],
    },
    dc: 10,
    kept: face + 2 >= 10,
    spellKey: 'spell:bless',
    spellNamePt: 'Bênção',
  }),
});

describe('ReactionSheet: the halfling’s Lucky question on a concentration save', () => {
  const resources = { answerOutsideLucky: vi.fn(), answerOutsideInspiration: vi.fn() };
  const api = {
    answerReaction: vi.fn(),
    resolveConcentrationSave: vi.fn(),
    markReactionRolling: vi.fn(),
    get: vi.fn(),
  };

  function setup() {
    const window = save();
    const state = new CombatState();
    state.apply(encounter({ reactionWindows: [window] } as never));
    state.noteReactionOpened(window.id);
    const data: ReactionSheetData = {
      campaignId: 'camp',
      encounterId: 'enc',
      window,
      round: 2,
      usage: [],
      pact: null,
      state,
      armorClass: 15,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn(), disableClose: true } },
        { provide: CombatClient, useValue: fakeCombatClient(api) },
        { provide: ResourceClient, useValue: resources },
      ],
    });
    const fixture = TestBed.createComponent(ReactionSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const button = (label: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        plain(b.textContent).includes(label),
      );
    const flush = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
    };
    return { fixture, el, button, flush };
  }

  /** Rolls in the app and gets the question back: the server keeps the window open while it waits. */
  async function asked(s: ReturnType<typeof setup>) {
    api.resolveConcentrationSave.mockResolvedValue({
      encounter: encounter({ reactionWindows: [save()] } as never),
      result: undefined,
      lucky,
    });
    s.button('Rolar no app')!.click();
    await s.flush();
  }

  beforeEach(() => {
    for (const fn of [...Object.values(api), ...Object.values(resources)]) {
      fn.mockReset();
    }
  });

  it('asks after a d20 of 1 and before the result: the 1 is there, the way to roll is not', async () => {
    const s = setup();
    await asked(s);
    const text = plain(s.el.textContent);
    expect(text).toContain('Tirou 1: rolar de novo?');
    expect(text).toContain(
      'Seu d20: 1d20 (1) + 2 = 3. O resultado só aparece depois da sua escolha.',
    );
    expect(text).not.toMatch(/Concentração (mantida|perdida)|Deixar o mestre rolar por mim/);
    expect(s.button('Rolar de novo')).toBeTruthy();
    expect(s.button('Ficar com o 1')).toBeTruthy();
  });

  it('"Rolar de novo" settles the save with the new d20 and says "Sortudo: 1 → 14"', async () => {
    const s = setup();
    await asked(s);
    resources.answerOutsideLucky.mockResolvedValue({
      result: { case: 'concentrationSave', value: settled(14, 14) },
    });
    s.button('Rolar de novo')!.click();
    await s.flush();
    expect(resources.answerOutsideLucky.mock.calls[0].slice(0, 4)).toEqual([
      'camp',
      'h1',
      true,
      { inApp: true },
    ]);
    const text = plain(s.el.textContent);
    expect(text).not.toContain('Tirou 1: rolar de novo?');
    expect(text).toContain('Concentração mantida');
    expect(text).toContain('Sortudo: 1 → 14');
  });

  it('"Ficar com o 1" keeps it: the concentration is lost, and nothing says Lucky rolled', async () => {
    const s = setup();
    await asked(s);
    resources.answerOutsideLucky.mockResolvedValue({
      result: { case: 'concentrationSave', value: settled(1) },
    });
    s.button('Ficar com o 1')!.click();
    await s.flush();
    expect(resources.answerOutsideLucky.mock.calls[0].slice(0, 4)).toEqual([
      'camp',
      'h1',
      false,
      null,
    ]);
    const text = plain(s.el.textContent);
    expect(text).toContain('Concentração perdida');
    expect(text).not.toContain('Sortudo:');
  });

  it('after the new d20, a Bardic Inspiration die the character holds is asked about, on the new roll', async () => {
    const s = setup();
    await asked(s);
    resources.answerOutsideLucky.mockResolvedValue({
      result: {
        case: 'concentrationSave',
        value: {
          encounter: encounter({ reactionWindows: [save()] } as never),
          inspirationOffer: create(OutsideInspirationOfferSchema, {
            holdId: 'h1',
            sides: 8,
            fromName: 'Orla',
            d20: { diceCount: 1, diceSides: 20, faces: [6], modifier: 2, total: 8 },
          }),
        },
      },
    });
    s.button('Rolar de novo')!.click();
    await s.flush();
    const text = plain(s.el.textContent);
    expect(text).not.toContain('Tirou 1: rolar de novo?');
    expect(text).toContain('Usar a Inspiração de Bardo (d8)?');
    expect(text).not.toMatch(/Concentração (mantida|perdida)/);
  });

  it('keeps the key of an answer that failed, so a retry never rolls the d20 twice', async () => {
    const s = setup();
    await asked(s);
    resources.answerOutsideLucky.mockRejectedValue(new ConnectError('down', Code.Unavailable));
    s.button('Rolar de novo')!.click();
    await s.flush();
    s.button('Rolar de novo')!.click();
    await s.flush();
    const keys = resources.answerOutsideLucky.mock.calls.map((c) => c[4] as string);
    expect(keys).toHaveLength(2);
    expect(keys[0]).toBe(keys[1]);
    expect(plain(s.el.textContent)).toContain('Tirou 1: rolar de novo?');
  });

  it('says another roll waits for the Lucky answer when the server refuses with LUCKY_PENDING', async () => {
    const s = setup();
    api.resolveConcentrationSave.mockRejectedValue(
      new ConnectError('pending', Code.FailedPrecondition, undefined, [
        {
          desc: ResourceBlockedSchema,
          value: create(ResourceBlockedSchema, { reason: ResourceBlockedReason.LUCKY_PENDING }),
        },
      ]),
    );
    s.button('Rolar no app')!.click();
    await s.flush();
    expect(plain(s.el.querySelector('[role=alert] p')?.textContent)).toContain(
      'Responda primeiro à pergunta do Sortudo',
    );
  });
});
