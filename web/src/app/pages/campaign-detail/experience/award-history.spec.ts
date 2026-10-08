import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError } from '@connectrpc/connect';

import { XpMode } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  UndoLastXPAwardResponseSchema,
  XPAwardMode,
  XPAwardSchema,
  XPBlockedReason,
  XPBlockedSchema,
} from '../../../../gen/meurpg/progression/v1/progression_pb';
import { ExperienceStore } from '../../../core/progression/experience-store';
import { ProgressionClient } from '../../../core/progression/progression-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { AwardHistory } from './award-history';
import { undoConsequence, undoTitle } from './undo-text';

const nbsp = ' ';

function award(over: Parameters<typeof create<typeof XPAwardSchema>>[1]) {
  return create(XPAwardSchema, {
    mode: XPAwardMode.XP_AWARD_MODE_ENEMIES,
    givenByDisplayName: 'Samuel',
    createdAt: timestampFromDate(new Date(2026, 9, 2, 20, 41)),
    totalXp: 350,
    shares: [
      { characterId: 'p', characterName: 'Pensantus', xp: 116 },
      { characterId: 't', characterName: 'Toren', xp: 116 },
    ],
    ...over,
  });
}

const LAST = award({ id: 'a3', reason: 'Combate: Emboscada na estrada', canUndo: true });
const MIDDLE = award({
  id: 'a2',
  mode: XPAwardMode.XP_AWARD_MODE_MANUAL,
  reason: 'Pela ajuda ao ferreiro',
  totalXp: 80,
  shares: [
    { characterId: 'p', characterName: 'Pensantus', xp: 40 },
    { characterId: 't', characterName: 'Toren', xp: 40 },
  ],
});
const UNDONE = award({
  id: 'a1',
  reason: 'Combate: Lobos',
  undone: true,
  undoneByDisplayName: 'Samuel',
  undoneAt: timestampFromDate(new Date(2026, 9, 1, 22, 0)),
});

describe('AwardHistory (E7-09)', () => {
  const undoLast = vi.fn();
  const experience = vi.fn();
  const listAwards = vi.fn();

  beforeEach(() => {
    undoLast.mockReset().mockResolvedValue(create(UndoLastXPAwardResponseSchema, {}));
    experience.mockReset().mockResolvedValue({ xpMode: XpMode.ENEMIES, characters: [] });
    listAwards.mockReset().mockResolvedValue({ awards: [MIDDLE, UNDONE], nextPageToken: '' });
    // jsdom has no scrolling: the question scrolls itself into view.
    Element.prototype.scrollIntoView = vi.fn();
  });

  async function setup(awards = [LAST, MIDDLE, UNDONE], milestones = false, isMaster = false) {
    TestBed.configureTestingModule({
      providers: [
        ExperienceStore,
        { provide: ProgressionClient, useValue: { undoLast, experience, listAwards } },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
      ],
    });
    const store = TestBed.inject(ExperienceStore);
    store.awards.set(awards);
    store.rows.set([
      {
        id: 'p',
        name: 'Pensantus',
        playerUserId: '',
        sub: '',
        level: 3,
        xp: 2716,
        nextLevelXp: 2700,
        canLevelUp: true,
        levelUpReason: 1,
      },
      {
        id: 't',
        name: 'Toren',
        playerUserId: '',
        sub: '',
        level: 3,
        xp: 2366,
        nextLevelXp: 2700,
        canLevelUp: false,
        levelUpReason: 0,
      },
    ]);
    const fixture = TestBed.createComponent(AwardHistory);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('milestones', milestones);
    fixture.componentRef.setInput('isMaster', isMaster);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, store };
  }

  const items = (el: HTMLElement) => Array.from(el.querySelectorAll('li.item'));
  const button = (el: HTMLElement, name: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      b.textContent?.includes(name),
    )!;
  const settle = async (fixture: { whenStable(): Promise<unknown>; detectChanges(): void }) => {
    await fixture.whenStable();
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
  };

  it('lists each award with when, why, who gave it, the mode tag and how much each got', async () => {
    const { el } = await setup();
    const first = items(el)[0].textContent!;
    expect(first).toContain('Combate: Emboscada na estrada');
    expect(first).toContain('Samuel deu a Pensantus e Toren');
    expect(first).toContain('Por inimigos');
    expect(first).toContain(`116${nbsp}XP para cada`);
    expect(first).toContain(`Total de${nbsp}350${nbsp}XP`);
    expect(items(el)[1].textContent).toContain('Avulso');
  });

  it('keeps an undone award with the tag "Desfeito", and no button', async () => {
    const { el } = await setup();
    const undone = items(el)[2];
    expect(undone.textContent).toContain('Desfeito');
    expect(undone.textContent).toContain('Desfeito por Samuel');
    expect(undone.querySelector('button')).toBeNull();
  });

  it('offers "Desfazer" only on the one award the server says can be undone', async () => {
    const { el } = await setup();
    expect(
      Array.from(el.querySelectorAll('button')).filter((b) => b.textContent?.includes('Desfazer')),
    ).toHaveLength(1);
    expect(items(el)[0].querySelector('button')).not.toBeNull();
    expect(items(el)[1].querySelector('button')).toBeNull();
  });

  it('shows a player the same history without actions', async () => {
    const { el } = await setup([{ ...LAST, canUndo: false } as typeof LAST, MIDDLE, UNDONE]);
    expect(el.querySelectorAll('button')).toHaveLength(0);
    expect(items(el)).toHaveLength(3);
  });

  it('shows no XP number in a milestones campaign', async () => {
    const mark = award({
      id: 'm1',
      mode: XPAwardMode.XP_AWARD_MODE_MILESTONE,
      reason: 'Marco: a ponte',
      totalXp: 0,
      canUndo: true,
      shares: [{ characterId: 'p', characterName: 'Pensantus', xp: 0 }],
    });
    const { el } = await setup([mark], true);
    expect(el.textContent).toContain('Marco');
    expect(el.textContent).toContain('Samuel marcou Pensantus');
    expect(el.textContent).not.toMatch(/XP/);
  });

  describe('the question in place', () => {
    it('replaces the line with a question that says who loses what, and focuses "Voltar"', async () => {
      const { fixture, el } = await setup();
      button(el, 'Desfazer').click();
      await settle(fixture);

      const question = el.querySelector('[role="alertdialog"]')!;
      expect(question.textContent).toContain('Desfazer o XP de “Combate: Emboscada na estrada”?');
      expect(question.textContent).toContain(`Pensantus e Toren perdem 116${nbsp}XP cada.`);
      // Pensantus can level up now and would not after it: the question says so.
      expect(question.textContent).toContain(
        `Pensantus volta para 2.600${nbsp}XP e deixa de poder subir de nível.`,
      );
      expect(question.textContent).toContain('O histórico guarda o desfazer.');
      expect(document.activeElement).toBe(button(el, 'Voltar'));
      expect(undoLast).not.toHaveBeenCalled();
      // The line it replaced is gone from the list, not stacked under the question.
      expect(items(el)[0].querySelector('.what')).toBeNull();
    });

    it('has two buttons of the same kind, "Voltar" and "Desfazer XP"', async () => {
      const { fixture, el } = await setup();
      button(el, 'Desfazer').click();
      await settle(fixture);
      const pair = Array.from(el.querySelectorAll<HTMLButtonElement>('.ask__btn'));
      expect(pair.map((b) => b.textContent?.trim())).toEqual(['Voltar', 'Desfazer XP']);
      expect(pair.every((b) => b.classList.contains('mat-mdc-outlined-button'))).toBe(true);
    });

    it('puts the focus back on "Desfazer" after "Voltar", and sends nothing', async () => {
      const { fixture, el } = await setup();
      button(el, 'Desfazer').click();
      await settle(fixture);
      button(el, 'Voltar').click();
      await settle(fixture);

      expect(el.querySelector('[role="alertdialog"]')).toBeNull();
      expect(document.activeElement).toBe(button(el, 'Desfazer'));
      expect(undoLast).not.toHaveBeenCalled();
    });

    it('puts the question away with Esc too', async () => {
      const { fixture, el } = await setup();
      button(el, 'Desfazer').click();
      await settle(fixture);
      el.querySelector('[role="alertdialog"]')!.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
      );
      await settle(fixture);
      expect(el.querySelector('[role="alertdialog"]')).toBeNull();
      expect(document.activeElement).toBe(button(el, 'Desfazer'));
    });

    it('undoes the award the screen showed, sending its id as expected_award_id', async () => {
      const { fixture, el, store } = await setup();
      const refresh = vi.spyOn(store, 'refresh').mockResolvedValue();
      button(el, 'Desfazer').click();
      await settle(fixture);
      button(el, 'Desfazer XP').click();
      await settle(fixture);

      expect(undoLast).toHaveBeenCalledWith(
        'camp-1',
        'a3',
        expect.stringMatching(/^[0-9a-f-]{36}$/),
      );
      expect(refresh).toHaveBeenCalled();
      expect(el.querySelector('[role="alertdialog"]')).toBeNull();
      expect(el.querySelector('[role="status"]')?.textContent).toContain('XP desfeito');
    });

    it('re-reads and says so when someone changed the history meanwhile (aborted)', async () => {
      undoLast.mockRejectedValue(new ConnectError('x', Code.Aborted));
      const { fixture, el, store } = await setup();
      const refresh = vi.spyOn(store, 'refresh').mockResolvedValue();
      button(el, 'Desfazer').click();
      await settle(fixture);
      button(el, 'Desfazer XP').click();
      await settle(fixture);

      expect(refresh).toHaveBeenCalled();
      expect(el.querySelector('[role="alertdialog"]')).toBeNull();
      expect(el.querySelector('[role="status"]')?.textContent).toContain('O histórico mudou');
    });

    it('says why in words when it cannot, by the typed reason, and keeps the question open for a retry with the same key', async () => {
      undoLast.mockRejectedValue(
        new ConnectError('x', Code.FailedPrecondition, undefined, [
          {
            desc: XPBlockedSchema,
            value: { reason: XPBlockedReason.XP_BLOCKED_REASON_NOTHING_TO_UNDO },
          },
        ]),
      );
      const { fixture, el } = await setup();
      button(el, 'Desfazer').click();
      await settle(fixture);
      button(el, 'Desfazer XP').click();
      await settle(fixture);
      expect(el.querySelector('[role="alert"]')?.textContent).toContain(
        'Não há nenhum prêmio para desfazer',
      );
      expect(el.querySelector('[role="alertdialog"]')).not.toBeNull();

      button(el, 'Desfazer XP').click();
      await settle(fixture);
      expect(undoLast.mock.calls[1][2]).toBe(undoLast.mock.calls[0][2]);
    });

    it('scrolls the whole question into view below the app bar', async () => {
      const { fixture, el } = await setup();
      const scroll = vi.mocked(Element.prototype.scrollIntoView);
      button(el, 'Desfazer').click();
      await settle(fixture);
      expect(scroll).toHaveBeenCalledWith({ block: 'start' });
    });
  });

  it('shows "Mostrar mais" only while there is another page', async () => {
    const { fixture, el, store } = await setup();
    expect(button(el, 'Mostrar mais')).toBeUndefined();
    store.nextPageToken.set('next');
    fixture.detectChanges();
    const more = vi.spyOn(store, 'more').mockResolvedValue();
    button(el, 'Mostrar mais').click();
    expect(more).toHaveBeenCalled();
  });
});

describe('"Voltar à cidade" in the history (E9-09)', () => {
  const TOWN = award({
    id: 'a9',
    mode: XPAwardMode.XP_AWARD_MODE_GOLD,
    reason: 'Voltar à cidade',
    gold: 420,
    totalXp: 420,
    treasureCount: 3,
    canUndo: true,
    shares: [
      { characterId: 'p', characterName: 'Pensantus', xp: 105 },
      { characterId: 't', characterName: 'Toren', xp: 105 },
    ],
  });
  const OLDER = award({
    id: 'a8',
    mode: XPAwardMode.XP_AWARD_MODE_GOLD,
    reason: 'Venda do cálice de prata',
    gold: 60,
    totalXp: 120,
    shares: [{ characterId: 'p', characterName: 'Pensantus', xp: 60 }],
  });

  const undoLast = vi.fn();
  const refresh = vi.fn();

  async function setup(awards: (typeof TOWN)[], isMaster: boolean) {
    undoLast.mockReset().mockResolvedValue(create(UndoLastXPAwardResponseSchema, {}));
    refresh.mockReset().mockResolvedValue(undefined);
    Element.prototype.scrollIntoView = vi.fn();
    TestBed.configureTestingModule({
      providers: [
        ExperienceStore,
        { provide: ProgressionClient, useValue: { undoLast } },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
      ],
    });
    const store = TestBed.inject(ExperienceStore);
    store.awards.set(awards);
    store.rows.set([]);
    vi.spyOn(store, 'refresh').mockImplementation(refresh);
    const fixture = TestBed.createComponent(AwardHistory);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('isMaster', isMaster);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }
  const items = (el: HTMLElement) => Array.from(el.querySelectorAll('li.item'));
  const settle = async (fixture: { whenStable(): Promise<unknown>; detectChanges(): void }) => {
    await fixture.whenStable();
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
  };

  it('writes the line from the treasures (the count and the PO), with the XP each and the total', async () => {
    const { el } = await setup([TOWN, OLDER], true);
    const first = items(el)[0].textContent!;
    expect(first).toContain(`Voltar à cidade · 420${nbsp}PO em 3\u00a0tesouros`);
    expect(first).toContain('Por ouro');
    expect(first).toContain(`105${nbsp}XP para cada`);
    expect(first).toContain(`Total de${nbsp}420${nbsp}XP`);
    // An award typed by hand keeps what the master wrote.
    expect(items(el)[1].textContent).toContain('Venda do cálice de prata');
  });

  it('reads the same for a player, who gets only the count and the total (no list, no buttons)', async () => {
    const { el } = await setup([{ ...TOWN, canUndo: false } as typeof TOWN, OLDER], false);
    expect(items(el)[0].textContent).toContain(`Voltar à cidade · 420${nbsp}PO em 3\u00a0tesouros`);
    expect(el.querySelectorAll('button')).toHaveLength(0);
    expect(items(el)[0].textContent).not.toContain('Só o último prêmio');
  });

  it('asks in place and says that the treasures go back to "encontrado, não convertido"', async () => {
    const { fixture, el } = await setup([TOWN, OLDER], true);
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.includes('Desfazer'))!
      .click();
    await settle(fixture);
    const question = el.querySelector('[role="alertdialog"]')!;
    expect(question.textContent).toContain('Desfazer o XP de “Voltar à cidade”?');
    expect(question.textContent).toContain(`Pensantus e Toren perdem 105${nbsp}XP cada.`);
    expect(question.textContent).toContain(
      `Os 3\u00a0tesouros (420${nbsp}PO) voltam a “encontrado, não convertido”.`,
    );
    expect(question.textContent).toContain('O histórico guarda o desfazer.');
  });

  it('says after undoing that the treasures are free again, and reads the XP and the treasures again', async () => {
    const { fixture, el } = await setup([TOWN, OLDER], true);
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.includes('Desfazer'))!
      .click();
    await settle(fixture);
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.trim() === 'Desfazer XP')!
      .click();
    await settle(fixture);
    expect(undoLast).toHaveBeenCalledWith('camp-1', 'a9', expect.any(String));
    expect(el.querySelector('[role="status"]')?.textContent).toContain(
      'os 3\u00a0tesouros voltaram a “encontrado, não convertido”',
    );
    expect(refresh).toHaveBeenCalled();
  });

  it('tells the master why a "Voltar à cidade" that is not the last cannot be undone', async () => {
    const newer = award({
      id: 'a10',
      mode: XPAwardMode.XP_AWARD_MODE_MANUAL,
      reason: 'Avulso',
      canUndo: true,
    });
    const { el } = await setup([newer, { ...TOWN, canUndo: false } as typeof TOWN], true);
    expect(items(el)[1].textContent).toContain(
      'Os tesouros ficam livres quando os prêmios mais novos forem desfeitos.',
    );
    expect(items(el)[1].querySelector('button')).toBeNull();
    expect(items(el)[0].textContent).not.toContain('Só o último prêmio');
  });

  it('puts "Desfazer" under the line\'s words (not in a column of its own), and the amounts in one column', async () => {
    const { el } = await setup([TOWN, OLDER], true);
    const row = items(el)[0];
    expect(row.querySelector('.what .undo')).not.toBeNull();
    expect(row.querySelectorAll(':scope > *')).toHaveLength(3);
    expect(row.querySelector('.amount')).not.toBeNull();
  });

  it('says nothing of its own while the host has news on screen, and tells the host about an undo', async () => {
    const { fixture, el } = await setup([TOWN, OLDER], true);
    const told = vi.fn();
    fixture.componentInstance.undone.subscribe(told);
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.includes('Desfazer'))!
      .click();
    await settle(fixture);
    fixture.componentRef.setInput('hideNotice', true);
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.trim() === 'Desfazer XP')!
      .click();
    await settle(fixture);
    expect(told).toHaveBeenCalled();
    expect(el.querySelector('.notice')).toBeNull();
    fixture.componentRef.setInput('hideNotice', false);
    fixture.detectChanges();
    expect(el.querySelector('.notice')?.textContent).toContain('XP desfeito');
  });

  it('keeps an undone one as it was, with its tag', async () => {
    const { el } = await setup(
      [{ ...TOWN, undone: true, canUndo: false, undoneByDisplayName: 'Samuel' } as typeof TOWN],
      true,
    );
    expect(items(el)[0].textContent).toContain('Desfeito por Samuel');
    expect(items(el)[0].textContent).not.toContain('Só o último prêmio');
  });
});

describe('the undo of an award that the history no longer has', () => {
  const undoLast = vi.fn();

  async function run(rejection: ConnectError) {
    undoLast.mockReset().mockRejectedValue(rejection);
    Element.prototype.scrollIntoView = vi.fn();
    TestBed.configureTestingModule({
      providers: [
        ExperienceStore,
        { provide: ProgressionClient, useValue: { undoLast } },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
      ],
    });
    const store = TestBed.inject(ExperienceStore);
    store.awards.set([LAST]);
    store.rows.set([]);
    const refresh = vi.spyOn(store, 'refresh').mockResolvedValue();
    const fixture = TestBed.createComponent(AwardHistory);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('isMaster', true);
    const undone = vi.fn();
    fixture.componentInstance.undone.subscribe(undone);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const click = async (name: string) => {
      Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
        .find(
          (b) => b.textContent?.trim() === name || b.getAttribute('aria-label')?.startsWith(name),
        )!
        .click();
      await fixture.whenStable();
      fixture.detectChanges();
      await fixture.whenStable();
      fixture.detectChanges();
    };
    await click('Desfazer');
    await click('Desfazer XP');
    return { el, refresh, undone };
  }

  it('re-reads the history after an undo the server aborts', async () => {
    const { refresh } = await run(new ConnectError('x', Code.Aborted));
    expect(refresh).toHaveBeenCalled();
  });

  it('re-reads the history when the server says there is nothing to undo, as its message says', async () => {
    const { el, refresh } = await run(
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: XPBlockedSchema,
          value: { reason: XPBlockedReason.XP_BLOCKED_REASON_NOTHING_TO_UNDO },
        },
      ]),
    );
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('A tela foi atualizada');
    expect(refresh).toHaveBeenCalled();
  });

  it('tells the page the XP moved when there is nothing to undo, as another tab undid it', async () => {
    const { undone } = await run(
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: XPBlockedSchema,
          value: { reason: XPBlockedReason.XP_BLOCKED_REASON_NOTHING_TO_UNDO },
        },
      ]),
    );
    expect(undone).toHaveBeenCalledTimes(1);
  });
});

describe('what undoing says', () => {
  const rows = [
    {
      id: 'p',
      name: 'Pensantus',
      playerUserId: '',
      sub: '',
      level: 3,
      xp: 2716,
      nextLevelXp: 2700,
      canLevelUp: true,
      levelUpReason: 1,
    },
    {
      id: 't',
      name: 'Toren',
      playerUserId: '',
      sub: '',
      level: 3,
      xp: 3000,
      nextLevelXp: 2700,
      canLevelUp: true,
      levelUpReason: 1,
    },
    {
      id: 'b',
      name: 'Brisa',
      playerUserId: '',
      sub: '',
      level: 3,
      xp: 2066,
      nextLevelXp: 2700,
      canLevelUp: false,
      levelUpReason: 0,
    },
  ];

  it('names the one who stops being able to level up, and not the one who still can', () => {
    const text = undoConsequence(
      award({
        id: 'x',
        reason: 'r',
        shares: [
          { characterId: 'p', characterName: 'Pensantus', xp: 116 },
          { characterId: 't', characterName: 'Toren', xp: 116 },
        ],
      }),
      rows,
    );
    expect(text).toContain(`Pensantus volta para 2.600${nbsp}XP e deixa de poder subir de nível.`);
    expect(text).not.toContain('Toren volta');
  });

  it('says several at once', () => {
    const close = rows.map((r) => (r.id === 't' ? { ...r, xp: 2750 } : r));
    const text = undoConsequence(
      award({
        id: 'x',
        reason: 'r',
        shares: [
          { characterId: 'p', characterName: 'Pensantus', xp: 116 },
          { characterId: 't', characterName: 'Toren', xp: 116 },
        ],
      }),
      close,
    );
    expect(text).toContain('Pensantus e Toren deixam de poder subir de nível.');
  });

  it('is about the mark for a milestone', () => {
    const mark = award({
      id: 'm',
      reason: 'Marco: a ponte',
      mode: XPAwardMode.XP_AWARD_MODE_MILESTONE,
      shares: [{ characterId: 'p', characterName: 'Pensantus', xp: 0 }],
    });
    expect(undoTitle(mark)).toBe('Desfazer o marco “Marco: a ponte”?');
    expect(undoConsequence(mark, rows)).toBe(
      'Pensantus perde a marca “Pode subir de nível”. O histórico guarda o desfazer.',
    );
  });
});
