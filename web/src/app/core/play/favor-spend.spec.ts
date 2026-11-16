import { TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { create } from '@bufbuild/protobuf';

import {
  AdvantageSourceKind,
  AdvantageSourceSchema,
  RollMode,
} from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { RollModeKind, RollNoteSchema } from '../../../gen/meurpg/play/v1/contest_types_pb';
import { FavorBlockedReason } from '../../../gen/meurpg/play/v1/favor_pb';
import { FavorClient } from './favor-client';
import {
  FAVOR_LOCK_WORD,
  FAVOR_ONLY_PLAYER,
  FAVOR_READ_FAILED,
  type FavorCircumstances,
  FavorSpend,
  type FavorSpendHost,
  circumstancesOfNotes,
  circumstancesOfPrompt,
  circumstancesOfSources,
  favorByMasterText,
  judgeFavor,
} from './favor-spend';
import { FavorState } from './favor-state';
import { FakeFavorClient, favorPass, favorRefused, openFavor } from './favor-testing';

const source = (kind: AdvantageSourceKind, effect: RollMode, textPt: string) =>
  create(AdvantageSourceSchema, { kind, effect, textPt });
const none: FavorCircumstances = { mode: RollMode.NORMAL, advantages: [], disadvantages: [] };

describe('the judgement of the Inspiração on a roll', () => {
  it('turns a normal roll into advantage and says how to choose again', () => {
    const j = judgeFavor(none);
    expect(j).toMatchObject({ ok: true, mode: RollMode.ADVANTAGE });
    expect(j.ok && j.line).toContain('Desmarque a caixa');
  });

  it('turns a disadvantage into a normal roll, naming the source', () => {
    const j = judgeFavor({ ...none, mode: RollMode.DISADVANTAGE, disadvantages: ['Envenenada'] });
    expect(j).toEqual({
      ok: true,
      mode: RollMode.NORMAL,
      line: 'Desvantagem (Envenenada) e Inspiração se anulam: um d20. Desmarque a caixa para voltar a desvantagem.',
    });
  });

  it('adds nothing to an advantage the roller already reads', () => {
    expect(judgeFavor({ ...none, mode: RollMode.ADVANTAGE, advantages: ['Ajuda de Tor'] })).toEqual(
      {
        ok: false,
        reason: 'A Inspiração não mudaria esta rolagem: ela já tem vantagem (Ajuda de Tor).',
      },
    );
  });

  it('adds nothing to an advantage and a disadvantage that cancel', () => {
    const j = judgeFavor({ ...none, advantages: ['Ajuda de Tor'], disadvantages: ['Envenenada'] });
    expect(j.ok).toBe(false);
    expect(!j.ok && j.reason).toContain('se anulam');
  });
});

describe('the circumstances a sheet reads', () => {
  it('leave out the sources the roller does not read and the Inspiração itself', () => {
    const c = circumstancesOfSources(RollMode.ADVANTAGE, [
      source(105, RollMode.ADVANTAGE, 'Outra fonte'),
      source(AdvantageSourceKind.FAVOR, RollMode.ADVANTAGE, 'Inspiração'),
      source(1, RollMode.ADVANTAGE, 'Ajuda de Tor: vantagem'),
    ]);
    expect(c.advantages).toEqual(['Ajuda de Tor']);
    expect(c.mode).toBe(RollMode.ADVANTAGE);
  });

  it('of a check come from its option, without the hidden source and the bonus of an effect', () => {
    const note = (kind: string, labelPt: string, advantage: boolean) =>
      create(RollNoteSchema, { kind, labelPt, advantage });
    const c = circumstancesOfNotes(RollModeKind.DISADVANTAGE, [
      note('condition', 'Envenenada', false),
      note('other', 'Outra fonte', true),
      note('effect_bonus', 'Bênção', true),
    ]);
    expect(c).toMatchObject({
      mode: RollMode.DISADVANTAGE,
      advantages: [],
      disadvantages: ['Envenenada'],
    });
  });

  it('of a saving throw come from the prompt, and a hidden source is not counted', () => {
    expect(
      circumstancesOfPrompt('disadvantage', [
        'Envenenada: desvantagem',
        'Outra fonte: desvantagem',
      ]),
    ).toMatchObject({
      mode: RollMode.DISADVANTAGE,
      advantages: [],
      disadvantages: ['Envenenada'],
      hiddenDisadvantage: true,
    });
    expect(circumstancesOfPrompt('normal', [])).toMatchObject(none);
  });
});

describe("the judgement on the server's own table (favorMode, visibleSources)", () => {
  it('refuses a visible advantage and a visible disadvantage that cancel to a normal prompt', () => {
    const c = circumstancesOfPrompt('normal', ['Esquivando: vantagem', 'Envenenada: desvantagem']);
    expect(c).toMatchObject({ advantages: ['Esquivando'], disadvantages: ['Envenenada'] });
    const j = judgeFavor(c);
    expect(j.ok).toBe(false);
    expect(!j.ok && j.reason).toContain('se anulam');
  });

  it('rolls Normal, not advantage, when an advantage and a disadvantage are both hidden', () => {
    const c = circumstancesOfPrompt('normal', [
      'Outra fonte: vantagem',
      'Outra fonte: desvantagem',
    ]);
    expect(c.advantages).toEqual([]);
    expect(judgeFavor(c)).toMatchObject({ ok: true, mode: RollMode.NORMAL });
    const sources = circumstancesOfSources(RollMode.NORMAL, [
      source(105, RollMode.ADVANTAGE, 'Outra fonte: vantagem'),
      source(105, RollMode.DISADVANTAGE, 'Outra fonte: desvantagem'),
    ]);
    expect(judgeFavor(sources)).toMatchObject({ ok: true, mode: RollMode.NORMAL });
    const notes = circumstancesOfNotes(RollModeKind.NORMAL, [
      create(RollNoteSchema, { kind: 'other', labelPt: 'Outra fonte', advantage: true }),
      create(RollNoteSchema, { kind: 'other', labelPt: 'Outra fonte', advantage: false }),
    ]);
    expect(judgeFavor(notes)).toMatchObject({ ok: true, mode: RollMode.NORMAL });
  });

  it('gives advantage when the only sources are hidden advantages, and Normal under a hidden disadvantage alone', () => {
    expect(judgeFavor(circumstancesOfPrompt('advantage', ['Outra fonte: vantagem']))).toMatchObject(
      {
        ok: true,
        mode: RollMode.ADVANTAGE,
      },
    );
    expect(
      judgeFavor(circumstancesOfPrompt('disadvantage', ['Outra fonte: desvantagem'])),
    ).toMatchObject({ ok: true, mode: RollMode.NORMAL });
  });

  it('leaves the mode to the server when a sentence names no side under a normal mode', () => {
    const c = circumstancesOfPrompt('normal', [
      'Luz do sol (Sensibilidade)',
      'Outra fonte: vantagem',
    ]);
    expect(judgeFavor(c)).toEqual({ ok: true, mode: null, line: '' });
  });
});

describe('FavorSpend', () => {
  function setup(
    host: Partial<FavorSpendHost> = {},
    holders: Record<string, string> = { c1: 'Brisa' },
  ) {
    TestBed.resetTestingModule();
    const client = new FakeFavorClient();
    TestBed.configureTestingModule({
      providers: [FavorState, { provide: FavorClient, useValue: client.asClient() }],
    });
    const favor = TestBed.inject(FavorState);
    openFavor(favor, holders);
    const fav = TestBed.runInInjectionContext(
      () =>
        new FavorSpend({
          favor,
          who: () => ({ characterId: 'c1', name: 'Brisa' }),
          circumstances: () => none,
          ...host,
        }),
    );
    return { fav, favor, client };
  }

  beforeEach(() => TestBed.resetTestingModule());

  it('draws nothing for a character that has none, a creature, or before the read', () => {
    expect(setup({}, {}).fav.view()).toBeNull();
    expect(setup({ who: () => null }).fav.view()).toBeNull();
    const { fav, favor } = setup();
    favor.campaignId.set('');
    expect(fav.view()).toBeNull();
  });

  it('is open while the character has it, and only spends once ticked', () => {
    const { fav } = setup();
    expect(fav.view()).toMatchObject({ kind: 'open', mode: RollMode.ADVANTAGE });
    expect(fav.spend()).toBe(false);
    expect(fav.mode()).toBeNull();
    expect(fav.lock()).toBeNull();
    fav.set(true);
    expect(fav.spend()).toBe(true);
    expect(fav.mode()).toBe(RollMode.ADVANTAGE);
    expect(fav.lock()).toMatchObject({ mode: RollMode.ADVANTAGE, word: FAVOR_LOCK_WORD });
  });

  it('reads Normal under a disadvantage while ticked', () => {
    const { fav } = setup({
      circumstances: () => ({
        ...none,
        mode: RollMode.DISADVANTAGE,
        disadvantages: ['Envenenada'],
      }),
    });
    fav.set(true);
    expect(fav.mode()).toBe(RollMode.NORMAL);
    expect(fav.lock()?.line).toContain('Desvantagem (Envenenada) e Inspiração se anulam');
  });

  it('is dashed, with the reason, when the visible sources already give advantage', () => {
    const { fav } = setup({
      circumstances: () => ({ ...none, mode: RollMode.ADVANTAGE, advantages: ['Ajuda de Tor'] }),
    });
    expect(fav.view()).toEqual({
      kind: 'off',
      sub: '',
      reason: 'A Inspiração não mudaria esta rolagem: ela já tem vantagem (Ajuda de Tor).',
    });
    fav.set(true);
    expect(fav.spend()).toBe(false);
  });

  it('is dashed when the master acts for the player', () => {
    const { fav } = setup({ byMaster: () => true });
    expect(fav.view()).toEqual({
      kind: 'off',
      sub: favorByMasterText('Brisa'),
      reason: FAVOR_ONLY_PLAYER,
    });
    expect(favorByMasterText('Brisa')).toBe('O mestre agindo por Brisa');
  });

  it('is dashed when the read failed', () => {
    const { fav, favor } = setup();
    favor.failed.set(true);
    expect(fav.view()).toEqual({ kind: 'off', sub: '', reason: FAVOR_READ_FAILED });
  });

  it('is dashed with the reason of the sheet, and of a pass a sheet names', () => {
    expect(setup({ sheetReason: () => 'Há um pedido.' }).fav.view()).toMatchObject({
      kind: 'off',
      reason: 'Há um pedido.',
    });
    expect(setup({ passPending: () => 'Passe pendente.' }).fav.view()).toMatchObject({
      kind: 'off',
      reason: 'Passe pendente.',
    });
  });

  it("is dashed with the player's own ask that waits for the master, and opens again when it is gone (581-07, item 4)", () => {
    const { fav, favor } = setup();
    favor.passes.set([favorPass({ fromId: 'c1' })]);
    expect(fav.view()).toEqual({
      kind: 'off',
      sub: '',
      reason: 'Você pediu para passar a Inspiração a Tor. Cancele o pedido para usar.',
    });
    fav.set(true);
    expect(fav.spend()).toBe(false);
    favor.passes.set([]);
    expect(fav.view()?.kind).toBe('open');
  });

  it("reads the ask of any of the player's characters on a sheet that names none", () => {
    const { fav, favor } = setup({ who: () => ({ characterId: '', name: '' }) });
    favor.passes.set([favorPass({ toName: 'Pensantus' })]);
    expect(fav.view()).toMatchObject({ kind: 'off', reason: expect.stringContaining('Pensantus') });
  });

  it('is open with no known mode when the sheet does not know the circumstances', () => {
    const { fav } = setup({ circumstances: undefined });
    expect(fav.view()).toEqual({ kind: 'open', mode: null, line: '' });
    fav.set(true);
    expect(fav.spend()).toBe(true);
    expect(fav.lock()).toBeNull();
  });

  it('unticks after the roll and reads who has it again', () => {
    const { fav, favor } = setup();
    const refresh = vi.spyOn(favor, 'refresh').mockResolvedValue();
    fav.spent();
    expect(refresh).not.toHaveBeenCalled();
    fav.set(true);
    fav.spent();
    expect(fav.ticked()).toBe(false);
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it('tells the state whose Inspiração the roll spent, so that its coming back reads as the master giving it back', () => {
    const { fav, favor } = setup();
    vi.spyOn(favor, 'refresh').mockResolvedValue();
    const spentBy = vi.spyOn(favor, 'spentBy');
    fav.set(true);
    fav.spent();
    expect(spentBy).toHaveBeenCalledTimes(1);
  });

  it('unticks when the box closes, so it is not ticked when it opens again', () => {
    const circumstances = signal<FavorCircumstances>(none);
    const { fav } = setup({ circumstances: () => circumstances() });
    fav.set(true);
    TestBed.tick();
    expect(fav.spend()).toBe(true);
    circumstances.set({ ...none, mode: RollMode.ADVANTAGE, advantages: ['Ajuda de Tor'] });
    TestBed.tick();
    expect(fav.view()?.kind).toBe('off');
    expect(fav.ticked()).toBe(false);
    circumstances.set(none);
    TestBed.tick();
    expect(fav.view()?.kind).toBe('open');
    expect(fav.ticked()).toBe(false);
  });

  it('words a refusal, unticks, and reads again when it has none', () => {
    const { fav, favor } = setup();
    const refresh = vi.spyOn(favor, 'refresh').mockResolvedValue();
    fav.set(true);
    expect(fav.refused(favorRefused(FavorBlockedReason.ADDS_NOTHING))).toBe(
      'A Inspiração não mudaria esta rolagem.',
    );
    expect(fav.ticked()).toBe(false);
    expect(refresh).not.toHaveBeenCalled();
    fav.set(true);
    expect(fav.refused(favorRefused(FavorBlockedReason.HAS_NONE))).toBe('Você não tem Inspiração.');
    expect(refresh).toHaveBeenCalledTimes(1);
    expect(fav.refused(new Error('x'))).toBeNull();
  });
});
