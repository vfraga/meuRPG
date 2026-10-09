import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { CampaignSessions } from '../game-session/campaign-sessions';
import { GameSessionSource, GameSessionVm } from '../game-session/game-session-card.types';
import { PastSessionsPanel } from './past-sessions-panel';

const plain = (text: string | null | undefined) =>
  (text ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

/** Session `n` ran on Sept 1 + n, from 19h05 to 22h47. */
function ended(n: number): GameSessionVm {
  return {
    id: `sess-${n}`,
    sessionNumber: n,
    startedAt: new Date(2026, 8, n, 19, 5),
    endedAt: new Date(2026, 8, n, 22, 47),
  };
}
const open = (n: number): GameSessionVm => ({
  id: `sess-${n}`,
  sessionNumber: n,
  startedAt: new Date(2026, 8, n, 19, 5),
  endedAt: null,
});

describe('PastSessionsPanel', () => {
  const listSessions = vi.fn();

  // The year shows only when it is not the current one: the date is pinned.
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date(2026, 9, 8, 12, 0));
    listSessions.mockReset();
    TestBed.configureTestingModule({
      imports: [PastSessionsPanel],
      providers: [
        provideRouter([]),
        CampaignSessions,
        { provide: GameSessionSource, useValue: { listSessions } },
      ],
    });
  });

  afterEach(() => vi.useRealTimers());

  async function render(isMaster: boolean): Promise<{
    el: HTMLElement;
    fixture: ComponentFixture<PastSessionsPanel>;
  }> {
    const fixture = TestBed.createComponent(PastSessionsPanel);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('isMaster', isMaster);
    fixture.detectChanges();
    await settle(fixture);
    return { el: fixture.nativeElement as HTMLElement, fixture };
  }
  async function settle(fixture: ComponentFixture<unknown>): Promise<void> {
    await fixture.whenStable();
    await new Promise((resolve) => setTimeout(resolve));
    fixture.detectChanges();
  }
  const rows = (el: HTMLElement) =>
    Array.from(el.querySelectorAll('li.row'), (r) => ({
      name: plain(r.querySelector('.row__name')?.textContent),
      when: plain(r.querySelector('.row__when')?.textContent),
      duration: plain(r.querySelector('.row__duration')?.textContent),
      link: r.querySelector('a')?.getAttribute('href'),
      label: r.querySelector('a')?.getAttribute('aria-label'),
    }));

  it('lists the ended sessions with their number, hours, duration and a link to the summary', async () => {
    listSessions.mockResolvedValue([open(4), ended(3), ended(2)]);
    const { el } = await render(true);
    expect(plain(el.querySelector('h2')?.textContent)).toBe('Sessões anteriores');
    expect(plain(el.querySelector('.past__count')?.textContent)).toBe('2 encerradas');
    expect(rows(el)).toEqual([
      {
        name: 'Sessão 3',
        when: 'qui., 3 de set., 19h05 às 22h47',
        duration: '3 h 42 min',
        link: '/campaigns/c1/sessions/3',
        label: 'Ver resumo da Sessão 3',
      },
      {
        name: 'Sessão 2',
        when: 'qua., 2 de set., 19h05 às 22h47',
        duration: '3 h 42 min',
        link: '/campaigns/c1/sessions/2',
        label: 'Ver resumo da Sessão 2',
      },
    ]);
  });

  it("keeps name, date, duration and link as the row's own children, so one grid lays them out without wrapping", async () => {
    listSessions.mockResolvedValue([ended(3)]);
    const { el } = await render(true);
    const row = el.querySelector('li.row')!;
    expect(Array.from(row.children, (c) => c.className.split(' ')[0])).toEqual([
      'row__name',
      'row__when',
      'row__duration',
      'row__link',
    ]);
  });

  it('leaves the open session out: it is the live notice above', async () => {
    listSessions.mockResolvedValue([open(4), ended(3)]);
    const { el } = await render(true);
    expect(rows(el).map((r) => r.name)).toEqual(['Sessão 3']);
  });

  it('says "1 encerrada" for one', async () => {
    listSessions.mockResolvedValue([ended(1)]);
    const { el } = await render(false);
    expect(plain(el.querySelector('.past__count')?.textContent)).toBe('1 encerrada');
  });

  it('shows the master the empty box, without a button', async () => {
    listSessions.mockResolvedValue([open(1)]);
    const { el } = await render(true);
    expect(plain(el.querySelector('.past__empty')?.textContent)).toBe(
      'Nenhuma sessão encerrada ainda. Quando você encerrar a primeira, o resumo dela fica guardado aqui.',
    );
    expect(el.querySelector('button, a')).toBeNull();
  });

  it('shows a player no panel at all when nothing has ended', async () => {
    listSessions.mockResolvedValue([open(1)]);
    const { el } = await render(false);
    expect(el.querySelector('section')).toBeNull();
    expect(el.classList).toContain('is-empty');
  });

  it('shows the five newest and "Mostrar as outras N", which shows the rest and moves the focus to the first of them', async () => {
    listSessions.mockResolvedValue(Array.from({ length: 12 }, (_, i) => ended(12 - i)));
    const { el, fixture } = await render(false);
    expect(plain(el.querySelector('.past__count')?.textContent)).toBe('12 encerradas');
    expect(rows(el).map((r) => r.name)).toEqual([
      'Sessão 12',
      'Sessão 11',
      'Sessão 10',
      'Sessão 9',
      'Sessão 8',
    ]);
    const more = el.querySelector<HTMLButtonElement>('.past__more')!;
    expect(plain(more.textContent)).toBe('Mostrar as outras 7');

    more.click();
    await settle(fixture);
    expect(rows(el)).toHaveLength(12);
    expect(el.querySelector('.past__more')).toBeNull();
    expect(document.activeElement).toBe(el.querySelectorAll('li.row a')[5]);
  });

  it('shows no "Mostrar as outras" up to five', async () => {
    listSessions.mockResolvedValue(Array.from({ length: 5 }, (_, i) => ended(5 - i)));
    const { el } = await render(true);
    expect(rows(el)).toHaveLength(5);
    expect(el.querySelector('.past__more')).toBeNull();
  });

  it('says it is loading, as a status', async () => {
    listSessions.mockReturnValue(new Promise(() => undefined));
    const { el } = await render(true);
    expect(el.querySelector('[role="status"]')?.textContent).toContain('Carregando as sessões...');
  });

  it('says it could not load, and "Tentar de novo" reads the list again', async () => {
    listSessions.mockRejectedValueOnce(new Error('down'));
    const { el, fixture } = await render(true);
    expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain(
      'Não foi possível carregar as sessões. Tente de novo.',
    );
    expect(el.textContent).not.toContain('down');

    listSessions.mockResolvedValueOnce([ended(1)]);
    el.querySelector<HTMLButtonElement>('button')!.click();
    await settle(fixture);
    expect(rows(el).map((r) => r.name)).toEqual(['Sessão 1']);
    expect(listSessions).toHaveBeenCalledTimes(2);
  });

  it('writes the year and the day of the end when the session asks for them', async () => {
    listSessions.mockResolvedValue([
      {
        id: 'x',
        sessionNumber: 7,
        startedAt: new Date(2025, 9, 3, 22, 30),
        endedAt: new Date(2025, 9, 4, 1, 12),
      },
    ]);
    const { el } = await render(true);
    expect(rows(el)[0].when).toBe('sex., 3 de out. de 2025, 22h30 às 1h12 (sáb.)');
    expect(rows(el)[0].duration).toBe('2 h 42 min');
  });
});
