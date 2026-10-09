import { TestBed } from '@angular/core/testing';
import { Title } from '@angular/platform-browser';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError } from '@connectrpc/connect';
import { BehaviorSubject } from 'rxjs';

import { Role } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CharacterHighlightsSchema,
  HighlightCategorySchema,
  HighlightKind,
  HighlightWinnerSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  SessionCharacterSummarySchema,
  type SessionSummary,
  SessionSummarySchema,
} from '../../../gen/meurpg/play/v1/summary_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { SessionSummaryClient } from '../../core/play/session-summary';
import { PageTitle } from '../../core/title/page-title';
import {
  GameSessionSource,
  GameSessionVm,
} from '../campaign-detail/game-session/game-session-card.types';
import { SessionSummaryPage } from './session-summary-page';

/** The text of an element as read, without the icons' ligature names. */
const words = (e: Element | null | undefined) => {
  if (!e) {
    return undefined;
  }
  const copy = e.cloneNode(true) as Element;
  copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
  return copy.textContent?.replace(/ /g, ' ').replace(/\s+/g, ' ').trim();
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

const times = {
  startedAt: timestampFromDate(new Date(2026, 8, 24, 19, 12)),
  endedAt: timestampFromDate(new Date(2026, 8, 24, 22, 30)),
  duration: { seconds: BigInt(3 * 3600 + 18 * 60), nanos: 0 },
};
const categories = [
  category(HighlightKind.MOST_DAMAGE, 41, ['toren', 'Toren']),
  category(HighlightKind.TANK, 33, ['brisa', 'Brisa']),
  category(HighlightKind.CHECKS_PASSED, 4, ['brisa', 'Brisa']),
  category(HighlightKind.TREASURE_FOUND, 250, ['brisa', 'Brisa']),
];
/** What the master gets: the counts and the table of every player. */
const masterSummary = create(SessionSummarySchema, {
  ...times,
  categories,
  combats: 2,
  scenesOpened: 4,
  checksPassed: 9,
  checksTried: 12,
  players: [
    row('pens', 'Pensantus', 3, 4, { damageDealt: 17, damageTaken: 6, finalBlows: 1 }),
    row('toren', 'Toren', 1, 2, { damageDealt: 41, damageTaken: 19, finalBlows: 3 }, 125),
    row('brisa', 'Brisa', 4, 5, { damageDealt: 22, damageTaken: 33, criticalHits: 1 }, 250),
    row('salvia', 'Sálvia', 1, 1, { healingDone: 12, damageTaken: 9 }),
    row('mudo', 'Mudo', 0, 0),
  ],
});
/** What a player gets: no counts, no table, their own result. */
const playerSummary = create(SessionSummarySchema, {
  ...times,
  categories,
  mine: row('toren', 'Toren', 1, 2, { damageDealt: 41, damageTaken: 19, finalBlows: 3 }, 125),
});

const ended = (n: number): GameSessionVm => ({
  id: `s${n}`,
  sessionNumber: n,
  startedAt: new Date(2026, 8, n, 19, 0),
  endedAt: new Date(2026, 8, n, 22, 0),
});
const open = (n: number): GameSessionVm => ({
  id: `s${n}`,
  sessionNumber: n,
  startedAt: new Date(2026, 8, n, 19, 0),
  endedAt: null,
});

describe('SessionSummaryPage', () => {
  const getCampaign = vi.fn();
  const listSessions = vi.fn();
  const get = vi.fn();
  const playerNames = vi.fn();
  const params = new BehaviorSubject(convertToParamMap({ id: 'c1', number: '2' }));

  beforeEach(() => {
    getCampaign
      .mockReset()
      .mockResolvedValue({ campaign: { name: 'Mirathel', myRole: Role.MASTER } });
    listSessions.mockReset().mockResolvedValue([open(4), ended(3), ended(2), ended(1)]);
    get.mockReset().mockResolvedValue(masterSummary);
    playerNames.mockReset().mockResolvedValue(
      new Map([
        ['toren', 'Caio'],
        ['brisa', 'Lia'],
      ]),
    );
    params.next(convertToParamMap({ id: 'c1', number: '2' }));
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: ActivatedRoute, useValue: { paramMap: params } },
        { provide: CampaignsService, useValue: { getCampaign } },
        { provide: GameSessionSource, useValue: { listSessions } },
        { provide: SessionSummaryClient, useValue: { get, playerNames } },
      ],
    });
  });

  async function settle(fixture: { whenStable(): Promise<unknown>; detectChanges(): void }) {
    for (let i = 0; i < 4; i++) {
      await fixture.whenStable();
      await new Promise((resolve) => setTimeout(resolve));
      fixture.detectChanges();
    }
  }
  async function render(role = Role.MASTER) {
    getCampaign.mockResolvedValue({ campaign: { name: 'Mirathel', myRole: role } });
    const fixture = TestBed.createComponent(SessionSummaryPage);
    fixture.detectChanges();
    await settle(fixture);
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }
  const labels = (el: HTMLElement, selector: string) =>
    Array.from(el.querySelectorAll(selector), (e) => words(e));

  describe('the master', () => {
    it('opens the session whose number is in the address, by the id the list gave', async () => {
      await render();
      expect(get).toHaveBeenCalledWith('c1', 's2');
      expect(listSessions).toHaveBeenCalledTimes(1);
    });

    it('has the title, the campaign with the hours, the way back and the sessions on either side', async () => {
      const { el } = await render();
      expect(words(el.querySelector('h1'))).toBe('Sessão 2');
      expect(words(el.querySelector('.mr-page-lead'))).toBe(
        'Mirathel · qui., 24 de set., 19h12 às 22h30',
      );
      expect(words(el.querySelector('.back'))).toBe('Voltar para a campanha');
      expect(el.querySelector('.back')?.getAttribute('href')).toBe('/campaigns/c1');
      const pager = el.querySelector('nav[aria-label="Outras sessões"]')!;
      const links = Array.from(pager.querySelectorAll('a'));
      expect(links.map((a) => words(a))).toEqual(['Sessão 1', 'Sessão 3']);
      expect(links.map((a) => a.getAttribute('href'))).toEqual([
        '/campaigns/c1/sessions/1',
        '/campaigns/c1/sessions/3',
      ]);
    });

    it('has no green "Sessão encerrada" card: the four numbers are "Em números"', async () => {
      const { el } = await render();
      expect(el.textContent).not.toContain('Sessão encerrada');
      expect(el.querySelector('[role="status"]')).toBeNull();
      expect(words(el.querySelector('#numbers-title'))).toBe('Em números');
      expect(
        Array.from(el.querySelectorAll('.stat'), (s) => [
          words(s.querySelector('dt')),
          words(s.querySelector('dd')),
        ]),
      ).toEqual([
        ['Duração', '3 h 18 min'],
        ['Combates', '2'],
        ['Cenas abertas', '4'],
        ['Testes passados fora do combate', '9 de 12'],
      ]);
    });

    it('draws every highlight with the player behind it', async () => {
      const { el } = await render();
      expect(words(el.querySelector('#se-sum'))).toBe('Resumo da sessão');
      expect(labels(el, '.tile__label')).toEqual([
        'Mais dano causado',
        'Tanque',
        'Mais testes passados fora do combate',
      ]);
      expect(labels(el, '.tile__who')).toEqual(['de Caio', 'de Lia', 'de Lia']);
    });

    it('has "Números de cada jogador" with every number of whoever fought, zeros included', async () => {
      const { el } = await render();
      const table = Array.from(el.querySelectorAll('.tbl')).find(
        (t) => words(t.querySelector('.tbl__title')) === 'Números de cada jogador',
      )!;
      expect(words(table.querySelector('.tbl__caption'))).toBe('Do combate de toda a sessão.');
      expect(labels(table as HTMLElement, '.tbl__row--head [role="columnheader"]')).toEqual([
        'Personagem',
        'Dano causado',
        'Cura',
        'Dano recebido',
        'Golpes finais',
        'Acertos críticos',
      ]);
      expect(
        Array.from(table.querySelectorAll('.tbl__row:not(.tbl__row--head)'), (r) => [
          words(r.querySelector('b')),
          ...Array.from(r.querySelectorAll('.tbl__num'), (n) => words(n)),
        ]),
      ).toEqual([
        ['Pensantus', '17', '0', '6', '1', '0'],
        ['Toren', '41', '0', '19', '3', '0'],
        ['Brisa', '22', '0', '33', '0', '1'],
        ['Sálvia', '0', '12', '9', '0', '0'],
      ]);
    });

    it('has the checks per player and the treasure, as the ending screen does', async () => {
      const { el } = await render();
      const titles = labels(el, '.tbl__title');
      expect(titles).toEqual([
        'Números de cada jogador',
        'Testes passados fora do combate',
        'Mais tesouro encontrado',
      ]);
      const treasure = Array.from(el.querySelectorAll('.tbl')).at(-1)!;
      expect(
        Array.from(treasure.querySelectorAll('.tbl__row:not(.tbl__row--head)'), (r) => [
          words(r.querySelector('b')),
          words(r.querySelector('.tbl__num')),
        ]),
      ).toEqual([
        ['Brisa', '250 PO'],
        ['Toren', '125 PO'],
      ]);
    });

    it('says there was no combat, and draws no table of combat numbers', async () => {
      get.mockResolvedValue(
        create(SessionSummarySchema, {
          ...times,
          combats: 0,
          scenesOpened: 2,
          checksPassed: 4,
          checksTried: 5,
          categories: [
            category(HighlightKind.CHECKS_PASSED, 2, ['pens', 'Pensantus'], ['brisa', 'Brisa']),
          ],
          players: [row('pens', 'Pensantus', 2, 3), row('brisa', 'Brisa', 2, 2)],
        }),
      );
      const { el } = await render();
      expect(words(el.querySelector('.sum__note'))).toBe('Não houve combate nesta sessão.');
      expect(labels(el, '.tbl__title')).not.toContain('Números de cada jogador');
      expect(labels(el, '.tbl__title')).toContain('Testes passados fora do combate');
      expect(labels(el, '.stat dd')[1]).toBe('0');
      expect(el.querySelector('.sum__none')).not.toBeNull();
    });

    it('says nobody did anything, when nobody did', async () => {
      get.mockResolvedValue(create(SessionSummarySchema, { ...times }));
      const { el } = await render();
      expect(words(el.querySelector('.sum__note'))).toBe(
        'Ninguém causou, curou, sofreu dano nem passou em testes nesta sessão.',
      );
      expect(labels(el, '.stat dd')).toEqual(['3 h 18 min', '0', '0', '0 de 0']);
    });

    it("carries on without the players' names when they cannot be read", async () => {
      playerNames.mockRejectedValue(new Error('down'));
      const { el } = await render();
      expect(el.querySelectorAll('.tile').length).toBeGreaterThan(0);
      expect(el.querySelector('.tile__who')).toBeNull();
    });
  });

  describe('a player', () => {
    beforeEach(() => get.mockResolvedValue(playerSummary));

    it('gets the card of the ending as a page: no tag, no way to close it, no live region', async () => {
      const { el } = await render(Role.PLAYER);
      expect(words(el.querySelector('h1'))).toBe('Sessão 2');
      expect(words(el.querySelector('h2.card__title'))).toBe('A sessão acabou');
      expect(words(el.querySelector('.card__sub'))).toBe('Durou 3 h 18 min');
      expect(words(el.querySelector('.card__h'))).toBe('Destaques');
      expect(el.querySelector('.card__tag, .card__x, .card__close')).toBeNull();
      expect(el.textContent).not.toContain('Fechar');
      expect(el.querySelector('[role="status"]')).toBeNull();
    });

    it('marks what their character won and gives "Seu resultado" by the character the server sent', async () => {
      const { el } = await render(Role.PLAYER);
      const first = el.querySelector('.row')!;
      expect(words(first.querySelector('.row__label'))).toBe('Mais dano causado Você');
      expect(words(el.querySelectorAll('.card__h')[1])).toBe('Seu resultado, Toren');
      expect(
        Array.from(el.querySelectorAll('.own__tile'), (t) => [
          words(t.querySelector('.own__label')),
          words(t.querySelector('.own__value')),
        ]),
      ).toEqual([
        ['Dano causado', '41'],
        ['Dano recebido', '19'],
        ['Golpes finais', '3'],
        ['Cura', '0'],
        ['Acertos críticos', '0'],
        ['Testes passados fora do combate', '1 de 2'],
        ['Tesouro encontrado', '125 PO'],
      ]);
    });

    it('has no "Em números", no table and no player names: the server sent none', async () => {
      const { el } = await render(Role.PLAYER);
      expect(el.querySelector('#numbers-title, .tbl, .stat')).toBeNull();
      expect(playerNames).not.toHaveBeenCalled();
    });

    it('says in one neutral line that their character is not in the numbers', async () => {
      get.mockResolvedValue(create(SessionSummarySchema, { ...times, categories }));
      const { el } = await render(Role.PLAYER);
      expect(words(el.querySelector('.card__none'))).toBe(
        'Seu personagem não aparece nos números desta sessão.',
      );
      expect(el.querySelector('.card__none')?.getAttribute('role')).toBeNull();
      expect(el.textContent).not.toContain('Seu resultado');
      expect(el.querySelector('.row__you')).toBeNull();
    });
  });

  describe('the sessions on either side', () => {
    it('has no link to a session that does not exist', async () => {
      params.next(convertToParamMap({ id: 'c1', number: '1' }));
      const { el } = await render();
      expect(labels(el, 'nav[aria-label="Outras sessões"] a')).toEqual(['Sessão 2']);
    });

    it('says "em andamento" for the next one when it is open, and takes it to the live session', async () => {
      params.next(convertToParamMap({ id: 'c1', number: '3' }));
      const { el } = await render();
      const next = Array.from(el.querySelectorAll('nav a')).at(-1)!;
      expect(words(next)).toBe('Sessão 4 em andamento');
      expect(next.getAttribute('href')).toBe('/campaigns/c1/session');
    });

    it('opens the other session in place, moves the focus to its title and drops the older answer', async () => {
      const { fixture, el } = await render();
      let resolveOld!: (s: SessionSummary) => void;
      get.mockReturnValueOnce(new Promise((r) => (resolveOld = r)));
      params.next(convertToParamMap({ id: 'c1', number: '3' }));
      await settle(fixture);
      expect(words(el.querySelector('h1'))).toBe('Sessão 3');
      expect(words(el.querySelector('[role="status"]'))).toBe('Abrindo o resumo da sessão...');

      params.next(convertToParamMap({ id: 'c1', number: '1' }));
      await settle(fixture);
      resolveOld(create(SessionSummarySchema, { ...times, combats: 7 }));
      await settle(fixture);
      expect(words(el.querySelector('h1'))).toBe('Sessão 1');
      expect(words(el.querySelectorAll('.stat dd')[1])).toBe('2');
      expect(document.activeElement).toBe(el.querySelector('h1'));
    });
  });

  describe('the tab', () => {
    it('names the session and the campaign before the page and the app', async () => {
      TestBed.inject(PageTitle).begin('Resumo da sessão');
      await render();
      expect(TestBed.inject(Title).getTitle()).toBe(
        'Sessão 2 · Mirathel · Resumo da sessão · MeuRPG',
      );
    });
  });

  describe('while it loads', () => {
    it('shows the title and the campaign, and says it is opening the summary', async () => {
      get.mockReturnValue(new Promise(() => undefined));
      const { el } = await render();
      expect(words(el.querySelector('h1'))).toBe('Sessão 2');
      expect(words(el.querySelector('.mr-page-lead'))).toBe('Mirathel');
      expect(words(el.querySelector('[role="status"]'))).toBe('Abrindo o resumo da sessão...');
    });
  });

  describe('when the server would not read it', () => {
    it('says it could not load, and "Tentar de novo" reads both again, with the focus on the button', async () => {
      get.mockRejectedValueOnce(new ConnectError('down', Code.Unavailable));
      const { fixture, el } = await render();
      expect(words(el.querySelector('[role="alert"]'))).toBe(
        'Não foi possível carregar o resumo da sessão. Tente de novo.',
      );
      expect(el.textContent).not.toContain('down');
      const retry = el.querySelector<HTMLButtonElement>('button.js-retry')!;
      expect(document.activeElement).toBe(retry);

      retry.click();
      await settle(fixture);
      expect(el.querySelector('[role="alert"]')).toBeNull();
      expect(words(el.querySelector('#numbers-title'))).toBe('Em números');
      expect(listSessions).toHaveBeenCalledTimes(2);
    });

    it('says the session is still open when the server says so', async () => {
      get.mockRejectedValueOnce(new ConnectError('x', Code.FailedPrecondition));
      const { el } = await render();
      expect(words(el.querySelector('[role="status"]'))).toContain('ainda não acabou');
    });
  });

  describe('a session that is not there', () => {
    it('is not found by number: "Não há Sessão 9", with the last ended one', async () => {
      params.next(convertToParamMap({ id: 'c1', number: '9' }));
      const { el } = await render();
      expect(words(el.querySelector('h1'))).toBe('Não há Sessão 9');
      expect(words(el.querySelector('.mr-page-lead'))).toBe('Mirathel');
      expect(words(el.querySelector('.page-notice'))).toBe(
        'Esta campanha não tem uma Sessão 9. A última sessão encerrada é a 3.',
      );
      const actions = Array.from(el.querySelectorAll('.actions a'));
      expect(actions.map((a) => words(a))).toEqual(['Ver a Sessão 3', 'Voltar para a campanha']);
      expect(actions[0].getAttribute('href')).toBe('/campaigns/c1/sessions/3');
      expect(get).not.toHaveBeenCalled();
    });

    it('says the session has not ended when the number is the open one', async () => {
      params.next(convertToParamMap({ id: 'c1', number: '4' }));
      const { el } = await render();
      expect(words(el.querySelector('h1'))).toBe('Sessão 4');
      expect(words(el.querySelector('[role="status"]'))).toBe(
        'A Sessão 4 ainda não acabou. O resumo aparece quando o mestre a encerrar.',
      );
      expect(words(el.querySelector('.actions a'))).toBe('Entrar na sessão');
      expect(el.querySelector('.actions a')?.getAttribute('href')).toBe('/campaigns/c1/session');
      expect(get).not.toHaveBeenCalled();
    });

    it('is one generic page, with no campaign name, for someone who is not a member', async () => {
      const notFound = new ConnectError('nope', Code.NotFound);
      getCampaign.mockRejectedValue(notFound);
      listSessions.mockRejectedValue(notFound);
      const fixture = TestBed.createComponent(SessionSummaryPage);
      fixture.detectChanges();
      await settle(fixture);
      const el = fixture.nativeElement as HTMLElement;
      expect(words(el.querySelector('h1'))).toBe('Página não encontrada');
      expect(words(el.querySelector('.page-notice'))).toBe(
        'Esta página não existe ou é só para quem está na campanha. Um convite do mestre coloca você na mesa.',
      );
      expect(words(el.querySelector('.actions a'))).toBe('Ir para o início');
      expect(el.querySelector('.actions a')?.getAttribute('href')).toBe('/');
      expect(el.querySelector('.back')).toBeNull();
      expect(el.textContent).not.toContain('Mirathel');
      expect(get).not.toHaveBeenCalled();
    });

    it.each(['abc', '0', '-1', '1.5', ''])(
      'is that same generic page for the number "%s"',
      async (number) => {
        params.next(convertToParamMap({ id: 'c1', number }));
        const { el } = await render();
        expect(words(el.querySelector('h1'))).toBe('Página não encontrada');
        expect(listSessions).not.toHaveBeenCalled();
        expect(el.textContent).not.toContain('Mirathel');
      },
    );
  });
});
