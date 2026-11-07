import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { of } from 'rxjs';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ContentSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableContentKind,
  TableContentViolationSchema,
  type TableContentViolation,
  TableClassLevelSchema,
  TableSubclassSchema,
  TableCreatureSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { TableContentClient } from '../../../core/content/content-client';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { TableContentPackSchema as PackSchema } from '../../../../gen/meurpg/rules/v1/table_content_pb';
import {
  entry,
  fakeContentWatcher,
  feature,
  mirathel,
} from '../../../core/content/content-testing';
import { ContentList } from './content-list';
import { settle } from '../../../../test-settle';

describe('ContentList', () => {
  const originalMatchMedia = window.matchMedia;
  afterEach(() => {
    window.matchMedia = originalMatchMedia;
  });

  const list = vi.fn();
  const catalog = vi.fn();
  const unarchive = vi.fn();
  const archive = vi.fn();
  const exportPack = vi.fn();
  const getCampaign = vi.fn();
  const statBlock = vi.fn();
  let watcher = fakeContentWatcher();

  async function setup(
    role: Role,
    opts: {
      entries?: ReturnType<typeof mirathel>;
      phone?: boolean;
      kind?: string;
      leftOut?: TableContentViolation[];
    } = {},
  ) {
    list.mockReset().mockResolvedValue({
      entries: opts.entries ?? mirathel(),
      tableRevision: 7,
      contentVersion: '',
      leftOut: opts.leftOut ?? [],
    });
    catalog.mockReset().mockResolvedValue(create(ContentSchema, {}));
    unarchive.mockReset().mockImplementation(async (_c: string, key: string) => ({
      ...(opts.entries ?? mirathel()).find((e) => e.key === key)!,
      archived: false,
    }));
    archive.mockReset();
    exportPack.mockReset();
    statBlock.mockReset().mockResolvedValue({ summary: { namePt: 'Lobo' } });
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
        { provide: CreaturesClient, useValue: { statBlock } },
        {
          provide: TableContentClient,
          useValue: { list, catalog, unarchive, archive, exportPack },
        },
      ],
    });
    const fixture = TestBed.createComponent(ContentList);
    await settle(fixture, 3);
    return { fixture, el: fixture.nativeElement as HTMLElement, settle: () => settle(fixture, 3) };
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
    expect(text(el.querySelector('.menu__limit')!)).toContain('7 de 1000 entradas');
  });

  it('gives the master the kinds with their counts, the limit, and the rows with the state in words', async () => {
    const { el } = await setup(Role.MASTER, { kind: 'classes' });
    const items = Array.from(el.querySelectorAll('.menu__item')).map((a) => text(a).trim());
    expect(items).toEqual([
      'Classes2',
      'Subclasses2',
      'Raças1',
      'Antecedentes1',
      'Magias1',
      'Talentos0',
      'Opções0',
      'Características de classe0',
      'Criaturas0',
    ]);
    expect(text(el)).toContain('7 de 1000 entradas · o limite de uma campanha');
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
    await settle(fixture, 3);
    const rows = Array.from(el.querySelectorAll('.row')).map((r) => text(r));
    expect(rows).toHaveLength(1);
    expect(rows[0]).toContain('Bardo das Cinzas');
    const input = el.querySelector('input[data-field], input') as HTMLInputElement;
    input.value = 'zzz';
    input.dispatchEvent(new Event('input'));
    await settle(fixture, 3);
    expect(text(el)).toContain('Nenhuma entrada com esta busca');
  });

  it('draws the empty campaign: "Nada cadastrado ainda." with the SRD still valid, nine kinds with 0', async () => {
    const { el } = await setup(Role.MASTER, { entries: [], kind: 'spells' });
    expect(text(el)).toContain('Nada cadastrado ainda.');
    expect(text(el)).toContain('O SRD continua valendo');
    expect(Array.from(el.querySelectorAll('.menu__count')).map((c) => c.textContent)).toEqual([
      '0',
      '0',
      '0',
      '0',
      '0',
      '0',
      '0',
      '0',
      '0',
    ]);
    expect(text(el.querySelector('.empty__new')!)).toContain('Nova magia');
    expect(el.querySelector('input')).toBeNull();
  });

  it('lists the creatures: the base by its Portuguese name, the numbers of the owner, an archived one tagged', async () => {
    const wolf = entry(TableContentKind.CREATURE, 'Lobo do Vale', {
      body: {
        case: 'tableCreature',
        value: create(TableCreatureSchema, {
          namePt: 'Lobo do Vale',
          baseKey: 'monster:wolf',
          hitPointsFormula: '4 + level()',
        }),
      },
    });
    const old = entry(TableContentKind.CREATURE, 'Corvo Antigo', {
      archived: true,
      body: {
        case: 'tableCreature',
        value: create(TableCreatureSchema, { namePt: 'Corvo Antigo', baseKey: 'monster:wolf' }),
      },
    });
    const { el } = await setup(Role.MASTER, { entries: [wolf, old], kind: 'creatures' });
    expect(statBlock).toHaveBeenCalledTimes(1);
    expect(statBlock).toHaveBeenCalledWith('camp-1', 'monster:wolf');
    expect(text(el.querySelector('.list__head')!)).toContain('Criaturas');
    expect(text(el.querySelector('.list__head')!)).toContain('Nova criatura');
    expect(el.querySelector('a[href$="/new/creature"]')).not.toBeNull();
    const rows = Array.from(el.querySelectorAll('.row')).map((r) => text(r).trim());
    expect(rows[0]).toContain('Lobo do Vale');
    expect(rows[0]).toContain('Base Lobo · números do dono');
    expect(rows[1]).toContain('Corvo Antigo');
    expect(rows[1]).toContain('Arquivada');
    expect(el.querySelector('.row--archived')).not.toBeNull();
  });

  it('says what a creature of the table is when there is none, with "Nova criatura"', async () => {
    const { el } = await setup(Role.MASTER, { entries: [], kind: 'creatures' });
    expect(text(el)).toContain('Nenhuma criatura da mesa ainda.');
    expect(text(el)).toContain('parte de uma do SRD');
    expect(text(el.querySelector('.empty__new')!)).toContain('Nova criatura');
    expect(statBlock).not.toHaveBeenCalled();
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
    expect(text(el)).not.toContain('1000');
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
    expect(text(el)).toContain('7 de 1000 entradas na campanha.');
    // "Desarquivar" comes back at once, with no question.
    (el.querySelectorAll('.prow__btn')[1] as HTMLButtonElement).click();
    await settle(fixture, 3);
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
    await settle(fixture, 3);
    expect(text(el)).toContain('Não foi possível desarquivar: confira os dados e tente de novo.');
    unarchive.mockRejectedValueOnce(new ConnectError('maybe', Code.Unknown));
    (el.querySelectorAll('.prow__btn')[1] as HTMLButtonElement).click();
    await settle(fixture, 3);
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
    await settle(fixture, 3);
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
      contentVersion: '',
      leftOut: [],
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

  it('lists the feats like the other kinds, with "Novo talento" and the prerequisite as the support line', async () => {
    const feat = entry(TableContentKind.FEAT, 'Lutador de Corda');
    const { el } = await setup(Role.MASTER, { entries: [...mirathel(), feat], kind: 'feats' });
    expect(text(el.querySelector('.list__head')!)).toContain('Novo talento');
    expect(el.querySelector('a[href$="/new/feat"]')).not.toBeNull();
    expect(text(el.querySelector('.row')!)).toContain('Lutador de Corda');
    expect(text(el.querySelector('.row')!)).toContain('Força 13');
  });

  it('gives the master "Importar pacote" and "Exportar" beside "Regras da mesa", and a player neither', async () => {
    const { el } = await setup(Role.MASTER);
    const actions = text(el.querySelector('.head__actions')!);
    expect(actions).toContain('Importar pacote');
    expect(actions).toContain('Exportar');
    expect(el.querySelector('a[href$="/content/import"]')).not.toBeNull();
    const player = (await setup(Role.PLAYER)).el;
    expect(text(player)).not.toContain('Importar pacote');
    expect(text(player)).not.toContain('Exportar');
  });

  it('exports the pack as "<campanha>-conteudo.json" in proto JSON and says where it went', async () => {
    const saved: { name: string; blob: Blob }[] = [];
    const realCreate = URL.createObjectURL;
    const realRevoke = URL.revokeObjectURL;
    URL.createObjectURL = ((blob: Blob) => {
      saved.push({ name: '', blob });
      return 'blob:pack';
    }) as never;
    URL.revokeObjectURL = (() => undefined) as never;
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      saved[saved.length - 1].name = this.download;
    });
    try {
      const { el, settle: again } = await setup(Role.MASTER);
      exportPack.mockResolvedValue(
        create(PackSchema, { format: 'meurpg.table-content', version: 1, name: 'Mirathel' }),
      );
      Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
        .find((b) => text(b).includes('Exportar'))!
        .click();
      await again();
      expect(exportPack).toHaveBeenCalledWith('camp-1');
      expect(saved).toHaveLength(1);
      expect(saved[0].name).toBe('mirathel-conteudo.json');
      expect(await saved[0].blob.text()).toContain('"format": "meurpg.table-content"');
      expect(text(el.querySelector('[role="status"]')!)).toContain('mirathel-conteudo.json');
    } finally {
      URL.createObjectURL = realCreate;
      URL.revokeObjectURL = realRevoke;
      click.mockRestore();
    }
  });

  it('says it when the export fails, and the button comes back', async () => {
    const { el, settle: again } = await setup(Role.MASTER);
    exportPack.mockRejectedValue(new ConnectError('x', Code.PermissionDenied));
    const button = () =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        text(b).includes('Exportar'),
      )!;
    button().click();
    await again();
    expect(text(el.querySelector('[role="alert"]')!)).toContain('Só o mestre');
    expect(button().disabled).toBe(false);
  });

  describe('the entries the rules left out (K1, K3)', () => {
    const TARGET = 'table_class.levels[0].features[0].effects[0].target';
    const violation = (key: string, field: string, reason: string) =>
      create(TableContentViolationSchema, { key, field, reason });
    function stored() {
      const classe = entry(TableContentKind.CLASS, 'Guardião do Vale', { charactersUsing: 1 });
      const body = classe.body.value as { levels: unknown[] };
      body.levels = [
        create(TableClassLevelSchema, {
          features: [feature('Vigília', [{ type: 'modifier', target: 'gone' }])],
        }),
      ];
      const sub = entry(TableContentKind.SUBCLASS, 'Juramento da Névoa', {
        body: {
          case: 'tableSubclass',
          value: create(TableSubclassSchema, {
            namePt: 'Juramento da Névoa',
            classKey: classe.key,
          }),
        },
      });
      const feat = entry(TableContentKind.FEAT, 'Chama Insistente');
      const entries = [classe, sub, feat, entry(TableContentKind.CLASS, 'Andarilho das Cinzas')];
      const leftOut = [
        violation(classe.key, TARGET, 'bad_value'),
        violation(sub.key, '', 'dangling_reference'),
        violation(feat.key, 'table_feat.effects[0].damage_type', 'bad_value'),
      ];
      return { entries, leftOut, classe, sub, feat };
    }
    const menuItem = (label: string) =>
      Array.from(document.querySelectorAll<HTMLButtonElement>('.mat-mdc-menu-item')).find((b) =>
        text(b).includes(label),
      );

    it('has no panel and no tag when nothing is left out, and none for a player', async () => {
      const { el } = await setup(Role.MASTER);
      expect(el.querySelector('.out')).toBeNull();
      expect(el.querySelector('.mr-tag--danger')).toBeNull();
      const { entries, leftOut } = stored();
      const player = await setup(Role.PLAYER, { entries, leftOut });
      expect(player.el.querySelector('.out')).toBeNull();
      expect(text(player.el)).not.toContain('fora da campanha');
    });

    it('says how many are out, where in the entry and why, with the dependents under their class (K1)', async () => {
      const { entries, leftOut } = stored();
      const { el } = await setup(Role.MASTER, { entries, leftOut });
      expect(text(el.querySelector('.out__title')!)).toBe(
        'error_outline3 entradas fora da campanha',
      );
      const rows = Array.from(el.querySelectorAll('.out__item'));
      expect(rows).toHaveLength(2);
      const first = text(rows[0]);
      expect(first).toContain('Guardião do Vale');
      expect(first).toContain('Classe');
      expect(first).toContain(
        'Nível 1 · Vigília · efeito 1 · O que muda: este alvo não está mais no menu de efeitos. Escolha outro.',
      );
      expect(first).toContain(
        'Junto com ela: Juramento da Névoa (subclasse), que depende dela. · 1 ficha usa esta classe.',
      );
      expect(first).toContain(
        'Saiu porque a classe dela, Guardião do Vale, está fora. Volta sozinha quando a classe voltar.',
      );
      expect(first).toContain('Corrigir');
      expect(text(rows[1])).toContain('Chama Insistente');
      expect(text(rows[1])).toContain('Talento');
      const fix = rows[0].querySelector('a.out__fix')!;
      expect(fix.getAttribute('href')).toMatch(
        /\/content\/entries\/class(:|%3A)guardi-o-do-vale(@|%40)mesa$/,
      );
    });

    it('marks the left-out entry in its kind list, instead of the state, and not the others', async () => {
      const { entries, leftOut } = stored();
      const { el } = await setup(Role.MASTER, { entries, leftOut, kind: 'classes' });
      const rows = Array.from(el.querySelectorAll('a.row'));
      expect(text(rows[0])).toContain('Fora da campanha');
      expect(text(rows[0])).not.toContain('Em uso por');
      expect(rows[0].querySelector('.mr-tag--danger')).not.toBeNull();
      expect(text(rows[1])).not.toContain('Fora da campanha');
    });

    it('is shorter on a phone: no "Corrigir", the kind and the place in one line, "Junto:", and "⋮" for the rest (K3)', async () => {
      const { entries, leftOut } = stored();
      const { el } = await setup(Role.MASTER, { entries, leftOut, phone: true });
      expect(text(el.querySelector('.out__lead')!)).toContain(
        'Para corrigir, abra a entrada no computador.',
      );
      expect(el.querySelector('.out__fix')).toBeNull();
      expect(el.querySelector('.out__dep')).toBeNull();
      const first = text(el.querySelector('.out__item')!);
      expect(first).toContain(
        'Classe · Vigília, efeito 1: este alvo não está mais no menu de efeitos.',
      );
      expect(first).toContain('Junto: Juramento da Névoa · 1 ficha usa');
      expect(el.querySelectorAll('.out__act .more')).toHaveLength(2);
      expect(text(el.querySelector('.prow .mr-tag--danger')!)).toContain('Fora da campanha');
    });

    it('offers "Remover" and, on a laptop, "Exportar para corrigir num pacote" in the "⋮"', async () => {
      const { entries, leftOut } = stored();
      const laptop = await setup(Role.MASTER, { entries, leftOut });
      laptop.el.querySelector<HTMLButtonElement>('.out__act .more')!.click();
      await laptop.settle();
      expect(menuItem('Remover')).toBeDefined();
      expect(menuItem('Exportar para corrigir num pacote')).toBeDefined();
    });

    it('offers no export in the "⋮" of a phone', async () => {
      const { entries, leftOut } = stored();
      const phone = await setup(Role.MASTER, { entries, leftOut, phone: true });
      phone.el.querySelector<HTMLButtonElement>('.out__act .more')!.click();
      await phone.settle();
      expect(menuItem('Remover')).toBeDefined();
      expect(menuItem('Exportar')).toBeUndefined();
    });

    it('archives on "Remover" after the question, reads the list again and leaves the panel without it', async () => {
      const { entries, leftOut, classe } = stored();
      const { fixture, el, settle: again } = await setup(Role.MASTER, { entries, leftOut });
      archive.mockResolvedValue({ ...classe, archived: true });
      el.querySelector<HTMLButtonElement>('.out__act .more')!.click();
      await again();
      menuItem('Remover')!.click();
      await again();
      expect(archive).not.toHaveBeenCalled();
      const ask = (label: string) =>
        Array.from(
          document.querySelectorAll<HTMLButtonElement>('app-archive-sheet app-pair-foot button'),
        ).find((b) => text(b).includes(label))!;
      const sheet = text(document.querySelector('app-archive-sheet')!);
      expect(sheet).toContain('Remover Guardião do Vale?');
      expect(sheet).toContain('A entrada é arquivada, não apagada.');
      expect(sheet).not.toContain('Arquivar');
      list.mockResolvedValue({
        entries: entries.map((e) => (e.key === classe.key ? { ...e, archived: true } : e)),
        tableRevision: 8,
        contentVersion: '',
        leftOut: leftOut.filter((v) => v.key === 'feat:chama-insistente@mesa'),
      });
      ask('Remover').click();
      await again();
      expect(archive).toHaveBeenCalledWith('camp-1', classe.key);
      expect(text(el.querySelector('.out__title')!)).toContain('1 entrada fora da campanha');
      expect(text(el.querySelector('.out')!)).not.toContain('Guardião do Vale');
      expect(text(el.querySelector('[role="status"]')!)).toContain(
        'A classe Guardião do Vale foi arquivada e saiu da lista de entradas fora da campanha.',
      );
      expect(fixture.nativeElement).toBe(el);
    });

    it('leaves the entry as it is on "Voltar", and says why a refused archive was refused', async () => {
      const { entries, leftOut } = stored();
      const { el, settle: again } = await setup(Role.MASTER, { entries, leftOut });
      const open = async () => {
        el.querySelector<HTMLButtonElement>('.out__act .more')!.click();
        await again();
        menuItem('Remover')!.click();
        await again();
      };
      const ask = (label: string) =>
        Array.from(
          document.querySelectorAll<HTMLButtonElement>('app-archive-sheet app-pair-foot button'),
        ).find((b) => text(b).includes(label))!;
      await open();
      ask('Voltar').click();
      await again();
      expect(archive).not.toHaveBeenCalled();
      archive.mockRejectedValueOnce(new ConnectError('refused', Code.PermissionDenied));
      await open();
      ask('Remover').click();
      await again();
      expect(text(el.querySelector('[role="alert"]')!)).toContain('Só o mestre');
      expect(el.querySelectorAll('.out__item')).toHaveLength(2);
    });

    it('exports the pack from the "⋮"', async () => {
      const { entries, leftOut } = stored();
      const { el, settle: again } = await setup(Role.MASTER, { entries, leftOut });
      exportPack.mockRejectedValue(new ConnectError('x', Code.PermissionDenied));
      el.querySelector<HTMLButtonElement>('.out__act .more')!.click();
      await again();
      menuItem('Exportar para corrigir num pacote')!.click();
      await again();
      expect(exportPack).toHaveBeenCalledWith('camp-1');
    });
  });
});
