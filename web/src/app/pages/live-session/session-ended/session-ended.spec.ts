import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CharacterHighlightsSchema,
  HighlightCategorySchema,
  HighlightKind,
  HighlightWinnerSchema,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  SessionCharacterSummarySchema,
  type SessionSummary,
  SessionSummarySchema,
} from '../../../../gen/meurpg/play/v1/summary_pb';
import { SessionSummaryClient } from '../../../core/play/session-summary';
import { SessionEnded } from './session-ended';

/** The text of an element as read, without the icons' ligature names. */
const words = (e: Element | null | undefined) => {
  if (!e) {
    return undefined;
  }
  const copy = e.cloneNode(true) as Element;
  copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
  return copy.textContent
    ?.replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
};

function category(kind: HighlightKind, value: number, ...winners: [string, string][]) {
  return create(HighlightCategorySchema, {
    kind,
    value,
    winners: winners.map(([characterId, name]) =>
      create(HighlightWinnerSchema, { characterId, name }),
    ),
  });
}
const row = (
  id: string,
  name: string,
  passed: number,
  tried: number,
  combat = {},
  treasureFoundPo = 0,
) =>
  create(SessionCharacterSummarySchema, {
    highlights: create(CharacterHighlightsSchema, { characterId: id, name, ...combat }),
    checksPassed: passed,
    checksTried: tried,
    treasureFoundPo,
  });

const categories = [
  category(HighlightKind.MOST_DAMAGE, 23, ['toren', 'Toren']),
  category(HighlightKind.TANK, 24, ['brisa', 'Brisa']),
  category(HighlightKind.FINAL_BLOW, 2, ['pens', 'Pensantus'], ['toren', 'Toren']),
  category(HighlightKind.CHECKS_PASSED, 4, ['brisa', 'Brisa']),
];
const common = {
  startedAt: timestampFromDate(new Date(2026, 9, 3, 20, 5)),
  endedAt: timestampFromDate(new Date(2026, 9, 3, 23, 10)),
  duration: { seconds: BigInt(3 * 3600 + 5 * 60), nanos: 0 },
  categories,
};
/** What the master gets (E8-11 state 4): the counts and the whole table. */
const masterSummary = create(SessionSummarySchema, {
  ...common,
  combats: 1,
  scenesOpened: 3,
  checksPassed: 9,
  checksTried: 12,
  players: [
    row('pens', 'Pensantus', 3, 4),
    row('toren', 'Toren', 2, 3),
    row('brisa', 'Brisa', 4, 5),
  ],
});
/** What a player gets (state 5): no counts, no table, and their own result. */
const playerSummary = create(SessionSummarySchema, {
  ...common,
  mine: row('pens', 'Pensantus', 3, 4, { damageDealt: 17, finalBlows: 2 }),
});

describe('SessionEnded (MR-032, E8-11 states 4 and 5)', () => {
  const get = vi.fn();

  async function setup(summary: SessionSummary | Error, isMaster: boolean) {
    get.mockReset();
    if (summary instanceof Error) {
      get.mockRejectedValue(summary);
    } else {
      get.mockResolvedValue(summary);
    }
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: SessionSummaryClient, useValue: { get } }],
    });
    const fixture = TestBed.createComponent(SessionEnded);
    const set = (k: string, v: unknown) => fixture.componentRef.setInput(k, v);
    set('campaignId', 'c1');
    set('campaignName', 'Mirathel');
    set('sessionId', 's5');
    set('sessionNumber', 5);
    set('isMaster', isMaster);
    set(
      'players',
      new Map([
        ['toren', 'Caio'],
        ['brisa', 'Lia'],
      ]),
    );
    set('characterId', 'pens');
    set('characterName', 'Pensantus');
    fixture.detectChanges();
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
      fixture.detectChanges();
    }
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  describe('the master', () => {
    it('reads the summary of the session that ended', async () => {
      await setup(masterSummary, true);
      expect(get).toHaveBeenCalledWith('c1', 's5');
    });

    it('lands on "Sessão encerrada" with the duration, the combats, the scenes and the checks passed', async () => {
      const { el } = await setup(masterSummary, true);
      expect(words(el.querySelector('h2#se-title'))).toBe('Sessão encerrada');
      expect(el.querySelector('#se-title')?.closest('[role="status"]')).not.toBeNull();
      expect(words(el.querySelector('.end__sub'))).toBe('Sessão 5 · Mirathel · das 20:05 às 23:10');
      const stats = Array.from(el.querySelectorAll('.stat'), (s) => [
        words(s.querySelector('dt')),
        words(s.querySelector('dd')),
      ]);
      expect(stats).toEqual([
        ['Duração', '3 h 5 min'],
        ['Combates', '1'],
        ['Cenas abertas', '3'],
        ['Testes passados fora do combate', '9 de 12'],
      ]);
    });

    it('has "Voltar à campanha" as the only button, outlined', async () => {
      const { el } = await setup(masterSummary, true);
      const actions = Array.from(
        el.querySelectorAll<HTMLElement>('button, a[matButton], .mat-mdc-button-base'),
      );
      expect(actions.map((a) => words(a))).toEqual(['Voltar à campanha']);
      expect(actions[0].classList.contains('mat-mdc-outlined-button')).toBe(true);
      expect(el.querySelector('.end__leave')?.getAttribute('href')).toBe('/campaigns/c1');
    });

    it('shows "Resumo da sessão" with the session\'s tiles, "Mais testes passados fora do combate" among them, and the players behind them', async () => {
      const { el } = await setup(masterSummary, true);
      expect(words(el.querySelector('#se-sum'))).toBe('Resumo da sessão');
      const tiles = Array.from(el.querySelectorAll('.tile'));
      expect(tiles.map((t) => words(t.querySelector('.tile__label')))).toEqual([
        'Mais dano causado',
        'Tanque',
        'Golpe final',
        'Mais testes passados fora do combate',
      ]);
      const checks = tiles[3];
      expect(words(checks.querySelector('.tile__value'))).toBe('4 testes');
      expect(words(checks.querySelector('.tile__names'))).toBe('Brisa');
      expect(words(checks.querySelector('.tile__sub'))).toBe('de 5 tentados');
      expect(words(checks.querySelector('.tile__who'))).toBe('de Lia');
      // The same fixed grid as the combat's: one column, two from 720, four from 1100.
      expect(el.querySelector('app-highlight-tiles .hl__tiles')).not.toBeNull();
    });

    it('has the per-player table with the checks passed of the tried, and the caption about the DC', async () => {
      const { el } = await setup(masterSummary, true);
      expect(words(el.querySelector('.tbl__title'))).toBe('Testes passados fora do combate');
      expect(words(el.querySelector('.tbl__caption'))).toBe(
        'Só contam testes de cenas que mostraram a CD aos jogadores; cenas com a CD escondida não entram na conta.',
      );
      const rows = Array.from(el.querySelectorAll('.tbl__row:not(.tbl__row--head)'), (r) => [
        words(r.querySelector('b')),
        words(r.querySelector('.tbl__num')),
      ]);
      expect(rows).toEqual([
        ['Pensantus', '3 de 4'],
        ['Toren', '2 de 3'],
        ['Brisa', '4 de 5'],
      ]);
      expect(
        Array.from(
          el.querySelectorAll('.tbl__row--head [role="columnheader"]'),
          (h) => h.textContent,
        ),
      ).toEqual(['Personagem', 'Testes passados']);
    });

    it('says no treasure was found when none was (E9-09)', async () => {
      const { el } = await setup(masterSummary, true);
      expect(words(el.querySelector('.sum__none'))).toBe('Nenhum tesouro registrado nesta sessão.');
      expect(Array.from(el.querySelectorAll('.sum__h'), (h) => words(h))).toContain(
        'Mais tesouro encontrado',
      );
    });

    it('has the block "Mais tesouro encontrado": every finder with the PO, not only the top one (E9-09)', async () => {
      const withTreasure = create(SessionSummarySchema, {
        ...common,
        categories: [
          ...categories,
          category(HighlightKind.TREASURE_FOUND, 250, ['brisa', 'Brisa']),
        ],
        combats: 1,
        scenesOpened: 3,
        checksPassed: 9,
        checksTried: 12,
        players: [
          row('pens', 'Pensantus', 3, 4, {}, 25),
          row('toren', 'Toren', 2, 3),
          row('brisa', 'Brisa', 4, 5, {}, 1250),
        ],
      });
      const { el } = await setup(withTreasure, true);
      const titles = Array.from(el.querySelectorAll('.tbl__title'), (h) => words(h));
      expect(titles).toEqual(['Testes passados fora do combate', 'Mais tesouro encontrado']);
      const block = Array.from(el.querySelectorAll('.tbl'))[1];
      expect(words(block.querySelector('.tbl__caption'))).toBe(
        'Só conta o que foi marcado durante a sessão.',
      );
      expect(
        Array.from(
          block.querySelectorAll('.tbl__row--head [role="columnheader"]'),
          (h) => h.textContent,
        ),
      ).toEqual(['Personagem', 'Tesouro encontrado']);
      const rows = Array.from(block.querySelectorAll('.tbl__row:not(.tbl__row--head)'), (r) => [
        words(r.querySelector('b')),
        words(r.querySelector('.tbl__num')),
      ]);
      // The most first, the number with its unit tied and a thousands separator.
      expect(rows).toEqual([
        ['Brisa', '1.250 PO'],
        ['Pensantus', '25 PO'],
      ]);
      expect(words(el.querySelectorAll('.sum__note')[0])).toContain(
        'divide o valor, arredondado para baixo',
      );
      // The treasure has its block: the tiles of "Destaques" do not repeat it.
      const tiles = Array.from(el.querySelectorAll('.tile'), (t) => words(t));
      expect(tiles.some((t) => t?.includes('Mais tesouro encontrado'))).toBe(false);
      expect(el.querySelector('.sum__none')).toBeNull();
    });
  });

  describe('a player', () => {
    it('gets the card "A sessão acabou" with the winners and their numbers, with no "de N"', async () => {
      const { el } = await setup(playerSummary, false);
      expect(words(el.querySelector('.card__tag'))).toBe('Sessão encerrada');
      expect(words(el.querySelector('h2.card__title'))).toBe('A sessão acabou');
      expect(el.querySelector('h2.card__title')?.closest('[role="status"]')).not.toBeNull();
      expect(words(el.querySelector('.card__sub'))).toBe('Durou 3 h 5 min');
      expect(words(el.querySelector('.card__h'))).toBe('Resumo da sessão');
      const rows = Array.from(el.querySelectorAll('.row'), (r) =>
        words(r.querySelector('.row__label')),
      );
      expect(rows).toEqual([
        'Mais dano causado',
        'Tanque',
        'Golpe final Você',
        'Mais testes passados fora do combate',
      ]);
      const checks = el.querySelectorAll('.row')[3];
      expect(words(checks.querySelector('.row__value'))).toBe('4 testes');
      expect(checks.textContent).not.toMatch(/\bde 5\b|tentados/);
    });

    it('shows "Seu resultado, Pensantus" with the own numbers and the own checks', async () => {
      const { el } = await setup(playerSummary, false);
      expect(words(el.querySelectorAll('.card__h')[1])).toBe('Seu resultado, Pensantus');
      const own = Array.from(el.querySelectorAll('.own__tile'), (t) => [
        words(t.querySelector('.own__label')),
        words(t.querySelector('.own__value')),
      ]);
      expect(own).toEqual([
        ['Dano causado', '17'],
        ['Dano recebido', '0'],
        ['Golpes finais', '2'],
        ['Cura', '0'],
        ['Testes passados fora do combate', '3 de 4'],
      ]);
    });

    it("draws no table, no counts and no master's panel", async () => {
      const { el } = await setup(playerSummary, false);
      expect(el.querySelector('.tbl')).toBeNull();
      expect(el.querySelector('.stat')).toBeNull();
      expect(el.textContent).not.toContain('Cenas abertas');
      expect(el.textContent).not.toContain('Testes passados fora do combate N');
    });

    it('has a ✕ and an outlined "Fechar" and no filled button; both leave the plain notice', async () => {
      const { fixture, el } = await setup(playerSummary, false);
      expect(el.querySelector('.card__x')?.getAttribute('aria-label')).toBe('Fechar');
      const close = el.querySelector<HTMLElement>('.card__close')!;
      expect(words(close)).toBe('Fechar');
      expect(close.classList.contains('mat-mdc-outlined-button')).toBe(true);
      expect(el.querySelectorAll('.mat-mdc-unelevated-button')).toHaveLength(0);
      close.click();
      fixture.detectChanges();
      expect(el.querySelector('.card')).toBeNull();
      expect(words(el.querySelector('h1'))).toBe('Sessão 5 encerrada');
      expect(el.textContent).toContain('A sessão acabou.');
    });

    it('has "Mais tesouro encontrado" as a tile, the PO of the finder, and their own "Tesouro encontrado" (E9-09 state 11)', async () => {
      const found = create(SessionSummarySchema, {
        ...common,
        categories: [category(HighlightKind.TREASURE_FOUND, 250, ['brisa', 'Brisa'])],
        mine: row('pens', 'Pensantus', 0, 0, {}, 125),
      });
      const { el } = await setup(found, false);
      const rows = Array.from(el.querySelectorAll('.row'));
      expect(rows).toHaveLength(1);
      expect(words(rows[0].querySelector('.row__label'))).toBe('Mais tesouro encontrado');
      expect(words(rows[0].querySelector('.row__value'))).toBe('250 PO');
      expect(words(rows[0].querySelector('.row__names'))).toBe('Brisa');
      expect(words(el.querySelector('.own__tile'))).toBe('Tesouro encontrado125 PO');
    });

    it("leaves the player's own block out when the character took no part", async () => {
      const { el } = await setup(create(SessionSummarySchema, { ...common }), false);
      expect(el.querySelectorAll('.card__h')).toHaveLength(1);
      expect(el.querySelector('.own')).toBeNull();
    });
  });

  it('falls back to the plain "A sessão acabou" notice, says it could not read the summary and offers to try again', async () => {
    const { fixture, el } = await setup(new ConnectError('down', Code.Unavailable), true);
    expect(words(el.querySelector('h1'))).toBe('Sessão 5 encerrada');
    expect(el.textContent).toContain('A sessão acabou.');
    expect(el.querySelector('.end')).toBeNull();
    expect(el.querySelector('[role="alert"]')?.textContent).toContain(
      'Não deu para carregar o resumo',
    );
    get.mockResolvedValue(masterSummary);
    const retry = Array.from(el.querySelectorAll('button')).find(
      (b) => words(b) === 'Tentar de novo',
    )!;
    expect(retry.classList.contains('mat-mdc-outlined-button')).toBe(true);
    retry.click();
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
      fixture.detectChanges();
    }
    expect(el.querySelector('#se-title')?.textContent).toBe('Sessão encerrada');
  });

  it("opens the card of another session the player had not closed, though they closed the last one's", async () => {
    const { fixture, el } = await setup(playerSummary, false);
    el.querySelector<HTMLElement>('.card__close')!.click();
    fixture.detectChanges();
    expect(el.querySelector('.card')).toBeNull();
    fixture.componentRef.setInput('sessionId', 's6');
    fixture.detectChanges();
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
      fixture.detectChanges();
    }
    expect(el.querySelector('.card')).not.toBeNull();
  });

  it('drops a late answer of an older read when the session changes', async () => {
    const { fixture, el } = await setup(masterSummary, true);
    let late!: (s: SessionSummary) => void;
    get.mockReset().mockReturnValueOnce(new Promise<SessionSummary>((r) => (late = r)));
    get.mockResolvedValueOnce(create(SessionSummarySchema, { ...common, combats: 7 }));
    fixture.componentRef.setInput('sessionId', 's6');
    fixture.detectChanges();
    fixture.componentRef.setInput('sessionId', 's7');
    fixture.detectChanges();
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
      fixture.detectChanges();
    }
    late(create(SessionSummarySchema, { ...common, combats: 1 }));
    await fixture.whenStable();
    fixture.detectChanges();
    expect(words(el.querySelectorAll('.stat dd')[1])).toBe('7');
  });

  it('says there was nothing to highlight, for a session with no combat and no checks', async () => {
    const { el } = await setup(create(SessionSummarySchema, { ...common, categories: [] }), true);
    expect(el.textContent).toContain(
      'Ninguém causou, curou, sofreu dano nem passou em testes nesta sessão.',
    );
    expect(el.querySelector('.tile')).toBeNull();
  });
});
