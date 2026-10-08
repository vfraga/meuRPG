import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { of } from 'rxjs';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ContentSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { TableContentKind } from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { TableContentClient } from '../../../core/content/content-client';
import { entry, fakeContentWatcher, mirathel } from '../../../core/content/content-testing';
import { ContentList } from './content-list';

describe('ContentList', () => {
  const originalMatchMedia = window.matchMedia;
  afterEach(() => {
    window.matchMedia = originalMatchMedia;
  });

  const list = vi.fn();
  const catalog = vi.fn();
  const unarchive = vi.fn();
  const archive = vi.fn();
  const getCampaign = vi.fn();
  let watcher = fakeContentWatcher();

  async function setup(
    role: Role,
    opts: { entries?: ReturnType<typeof mirathel>; phone?: boolean; kind?: string } = {},
  ) {
    list.mockReset().mockResolvedValue({ entries: opts.entries ?? mirathel(), tableRevision: 7 });
    catalog.mockReset().mockResolvedValue(create(ContentSchema, {}));
    unarchive.mockReset().mockImplementation(async (_c: string, key: string) => ({
      ...(opts.entries ?? mirathel()).find((e) => e.key === key)!,
      archived: false,
    }));
    archive.mockReset();
    getCampaign.mockReset().mockResolvedValue({
      campaign: { id: 'camp-1', name: 'Mirathel', myRole: role, awaitingApproval: false },
    });
    window.matchMedia = ((q: string) => ({
      matches: !!opts.phone && q.includes('max-width'),
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    })) as never;
    TestBed.resetTestingModule();
    watcher = fakeContentWatcher();
    TestBed.overrideComponent(ContentList, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: of(convertToParamMap({ id: 'camp-1' })),
            queryParamMap: of(convertToParamMap(opts.kind ? { kind: opts.kind } : {})),
          },
        },
        { provide: CampaignsService, useValue: { getCampaign } },
        { provide: TableContentClient, useValue: { list, catalog, unarchive, archive } },
      ],
    });
    const fixture = TestBed.createComponent(ContentList);
    await settle(fixture);
    return { fixture, el: fixture.nativeElement as HTMLElement, settle: () => settle(fixture) };
  }

  async function settle(fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) {
    for (let i = 0; i < 3; i++) {
      fixture.detectChanges();
      await new Promise((r) => setTimeout(r));
      await fixture.whenStable();
    }
    fixture.detectChanges();
  }

  const text = (el: Element) => (el.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');

  it('opens on the first kind, "Classes", with "Nova classe"; "Raças" has "Nova sub-raça" beside "Nova raça"', async () => {
    const { el } = await setup(Role.MASTER);
    expect(el.querySelector('.menu__item--on')?.textContent).toContain('Classes');
    expect(text(el.querySelector('.list__head')!)).toContain('Nova classe');
    expect(text(el)).not.toContain('próxima fatia');
  });

  it('every kind has its "Nova …" link for the master, the subclass one included', async () => {
    const { el } = await setup(Role.MASTER, { kind: 'subclasses' });
    expect(text(el.querySelector('.list__head')!)).toContain('Nova subclasse');
    expect(el.querySelector('a[href$="/new/subclass"]')).not.toBeNull();
    const races = (await setup(Role.MASTER, { kind: 'races' })).el;
    const head = text(races.querySelector('.list__head')!);
    expect(head).toContain('Nova raça');
    expect(head).toContain('Nova sub-raça');
  });

  it('shows the limit line under the tabs too, below 1100 px (it is not hidden by size)', async () => {
    const { el } = await setup(Role.MASTER);
    expect(text(el.querySelector('.menu__limit')!)).toContain('7 de 300 entradas');
  });

  it('gives the master the kinds with their counts, the limit, and the rows with the state in words', async () => {
    const { el } = await setup(Role.MASTER, { kind: 'classes' });
    const items = Array.from(el.querySelectorAll('.menu__item')).map((a) => text(a).trim());
    expect(items).toEqual(['Classes2', 'Subclasses2', 'Raças1', 'Antecedentes1', 'Magias1']);
    expect(text(el)).toContain('7 de 300 entradas · o limite de uma campanha');
    expect(el.querySelector('.menu__item--on')?.getAttribute('aria-current')).toBe('page');
    const rows = Array.from(el.querySelectorAll('.row')).map((r) => text(r).trim());
    expect(rows[0]).toContain('Guardião do Vale');
    expect(rows[0]).toContain('Em uso por 2 fichas');
    expect(rows[1]).toContain('Bardo das Cinzas');
    expect(rows[1]).toContain('Arquivada · 1 ficha usa');
    expect(el.querySelector('.row--archived')).not.toBeNull();
    // There is no "Apagar" anywhere: what a sheet uses is archived, never deleted.
    expect(text(el)).not.toContain('Apagar');
  });

  it('links each row to its entry with the key once encoded (a double encoding found no entry)', async () => {
    const { el } = await setup(Role.MASTER, { kind: 'classes' });
    const hrefs = Array.from(el.querySelectorAll('a.row')).map((a) => a.getAttribute('href'));
    expect(hrefs[0]).toMatch(
      /^\/campaigns\/camp-1\/content\/entries\/class(:|%3A)guardi-o-do-vale(@|%40)mesa$/,
    );
    expect(hrefs.join('')).not.toContain('%25');
  });

  it('offers "Nova raça" and the search and filter for a kind that has an editor', async () => {
    const { el } = await setup(Role.MASTER, { kind: 'races' });
    expect(text(el.querySelector('.list__head')!)).toContain('Nova raça');
    expect(el.querySelector('select')).not.toBeNull();
    expect(text(el)).toContain('Corujeiro');
  });

  it('filters by "Mostrar" and by the name', async () => {
    const { fixture, el } = await setup(Role.MASTER, { kind: 'classes' });
    const select = el.querySelector('select') as HTMLSelectElement;
    select.selectedIndex = 3;
    select.dispatchEvent(new Event('change'));
    await settle(fixture);
    const rows = Array.from(el.querySelectorAll('.row')).map((r) => text(r));
    expect(rows).toHaveLength(1);
    expect(rows[0]).toContain('Bardo das Cinzas');
    const input = el.querySelector('input[data-field], input') as HTMLInputElement;
    input.value = 'zzz';
    input.dispatchEvent(new Event('input'));
    await settle(fixture);
    expect(text(el)).toContain('Nenhuma entrada com esta busca');
  });

  it('draws the empty campaign: "Nada cadastrado ainda." with the SRD still valid, five kinds with 0', async () => {
    const { el } = await setup(Role.MASTER, { entries: [], kind: 'spells' });
    expect(text(el)).toContain('Nada cadastrado ainda.');
    expect(text(el)).toContain('O SRD continua valendo');
    expect(Array.from(el.querySelectorAll('.menu__count')).map((c) => c.textContent)).toEqual([
      '0',
      '0',
      '0',
      '0',
      '0',
    ]);
    expect(text(el.querySelector('.empty__new')!)).toContain('Nova magia');
    expect(el.querySelector('input')).toBeNull();
  });

  it('gives a player every entry that is on, kind by kind, with "Da mesa" and no counts, states or controls', async () => {
    const onlyOn = mirathel()
      .filter((e) => !e.archived)
      .map((e) => ({ ...e, charactersUsing: 0 }));
    const { el } = await setup(Role.PLAYER, { entries: onlyOn });
    const titles = Array.from(el.querySelectorAll('h2')).map((h) => h.textContent?.trim());
    expect(titles).toEqual(['Classes', 'Subclasses', 'Raças', 'Antecedentes', 'Magias']);
    expect(text(el)).toContain('o que o mestre criou para esta campanha');
    expect(text(el)).not.toContain('fichas');
    expect(text(el)).not.toContain('Arquiv');
    expect(text(el)).not.toContain('300');
    expect(el.querySelector('button')).toBeNull();
    expect(el.querySelectorAll('.mr-tag')).toHaveLength(onlyOn.length);
    expect(text(el)).toContain('Da mesa');
    expect(el.querySelectorAll('a.row--read')).toHaveLength(onlyOn.length);
  });

  it('says so to a player when nothing is on', async () => {
    const { el } = await setup(Role.PLAYER, { entries: [] });
    expect(text(el)).toContain('O mestre ainda não criou nada para esta mesa.');
  });

  it('gives the master on a phone the reading and the archive only: a notice, a "Tipo" select and one button per row', async () => {
    const { fixture, el } = await setup(Role.MASTER, { phone: true, kind: 'classes' });
    expect(text(el)).toContain(
      'Para criar ou editar, abra o conteúdo da mesa no notebook. Aqui você lê e arquiva.',
    );
    expect(el.querySelector('.menu')).toBeNull();
    expect(text(el)).not.toContain('Nova classe');
    const buttons = Array.from(el.querySelectorAll('.prow__btn')).map((b) =>
      b.getAttribute('aria-label'),
    );
    expect(buttons).toEqual(['Arquivar Guardião do Vale', 'Desarquivar Bardo das Cinzas']);
    expect(text(el)).toContain('7 de 300 entradas na campanha.');
    // "Desarquivar" comes back at once, with no question.
    (el.querySelectorAll('.prow__btn')[1] as HTMLButtonElement).click();
    await settle(fixture);
    expect(unarchive).toHaveBeenCalledWith('camp-1', 'class:bardo-das-cinzas@mesa');
    expect(
      Array.from(el.querySelectorAll('.prow__btn')).map((b) => b.getAttribute('aria-label')),
    ).toEqual(['Arquivar Guardião do Vale', 'Arquivar Bardo das Cinzas']);
    expect(text(el.querySelector('[role="status"]')!)).toContain(
      'A classe Bardo das Cinzas voltou.',
    );
  });

  it('says why a refused unarchive was refused, and to check an outcome the server could not confirm', async () => {
    const { fixture, el } = await setup(Role.MASTER, { phone: true, kind: 'classes' });
    unarchive.mockRejectedValueOnce(new ConnectError('refused', Code.InvalidArgument));
    (el.querySelectorAll('.prow__btn')[1] as HTMLButtonElement).click();
    await settle(fixture);
    expect(text(el)).toContain('Não foi possível desarquivar: confira os dados e tente de novo.');
    unarchive.mockRejectedValueOnce(new ConnectError('maybe', Code.Unknown));
    (el.querySelectorAll('.prow__btn')[1] as HTMLButtonElement).click();
    await settle(fixture);
    expect(text(el)).toContain('Não deu para confirmar se a alteração foi salva');
  });

  it('tells a campaign that is not the caller\'s "não encontrada"', async () => {
    getCampaign.mockReset().mockResolvedValue({ campaign: undefined });
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: of(convertToParamMap({ id: 'x' })),
            queryParamMap: of(convertToParamMap({})),
          },
        },
        { provide: CampaignsService, useValue: { getCampaign } },
        { provide: TableContentClient, useValue: { list, catalog } },
      ],
    });
    const fixture = TestBed.createComponent(ContentList);
    await settle(fixture);
    expect(text(fixture.nativeElement)).toContain('Campanha não encontrada');
    expect(entry(TableContentKind.CLASS, 'x')).toBeDefined();
  });
  it('gives the master "Opções para os jogadores", on a laptop and on a phone', async () => {
    const laptop = await setup(Role.MASTER);
    const link = Array.from(laptop.el.querySelectorAll('a')).find((a) =>
      text(a).includes('Opções para os jogadores'),
    );
    expect(link?.getAttribute('href')).toBe('/campaigns/camp-1/content/options');
    const phone = await setup(Role.MASTER, { phone: true });
    expect(
      Array.from(phone.el.querySelectorAll('a')).some((a) =>
        text(a).includes('Opções para os jogadores'),
      ),
    ).toBe(true);
    // A player has no switches to turn.
    const player = await setup(Role.PLAYER);
    expect(text(player.el)).not.toContain('Opções para os jogadores');
  });

  it('flags an entry the master switched off, in words, and says how many sheets still use it (RN-23)', async () => {
    const entries = [
      { ...entry(TableContentKind.RACE, 'Corujeiro', { charactersUsing: 2 }), off: true },
    ] as ReturnType<typeof mirathel>;
    const { el } = await setup(Role.MASTER, { entries, kind: 'races' });
    expect(text(el.querySelector('.row')!)).toContain(
      'Desligada para os jogadores · 2 fichas usam',
    );
  });

  it('reads the list again when the table changed (content_changed), keeping the kind and the search', async () => {
    const { el, settle } = await setup(Role.MASTER, { kind: 'races' });
    expect(watcher.following()).toBe('camp-1');
    expect(el.querySelectorAll('a.row')).toHaveLength(1);
    list.mockResolvedValue({
      entries: [...mirathel(), entry(TableContentKind.RACE, 'Gnomo do Vale')],
      tableRevision: 8,
    });
    watcher.hint();
    await settle();
    expect(Array.from(el.querySelectorAll('a.row')).map((r) => text(r))).toEqual([
      expect.stringContaining('Corujeiro'),
      expect.stringContaining('Gnomo do Vale'),
    ]);
    expect(list).toHaveBeenCalledTimes(2);
    // No spinner replaced the page.
    expect(el.querySelector('mat-spinner')).toBeNull();
  });

  it('keeps what is on screen when the read after a change fails', async () => {
    const { el, settle } = await setup(Role.MASTER, { kind: 'races' });
    list.mockRejectedValue(new Error('offline'));
    watcher.hint();
    await settle();
    expect(el.querySelectorAll('a.row')).toHaveLength(1);
    expect(text(el)).not.toContain('Não foi possível');
  });
});
