import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { BehaviorSubject } from 'rxjs';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ContentSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableContentKind,
  TableContentRefusalSchema,
  TableSubraceSchema,
  TableContentViolationSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { TableContentClient } from '../../../core/content/content-client';
import {
  classDefaults,
  entry,
  fakeContentWatcher,
  menuResponse,
  mirathel,
} from '../../../core/content/content-testing';
import { ContentEntry } from './content-entry';

describe('ContentEntry', () => {
  const originalMatchMedia = window.matchMedia;
  afterEach(() => {
    window.matchMedia = originalMatchMedia;
  });

  const list = vi.fn();
  const catalog = vi.fn();
  const effectMenu = vi.fn();
  const classDefaultsCall = vi.fn();
  const archive = vi.fn();
  const unarchive = vi.fn();
  const getCampaign = vi.fn();
  const setSwitches = vi.fn();
  let watcher = fakeContentWatcher();
  let params$ = new BehaviorSubject(convertToParamMap({}));

  async function setup(
    role: Role,
    key: string,
    entries = mirathel(),
    opts: {
      failCatalog?: boolean;
      failMenu?: boolean;
      failDefaults?: boolean;
      phone?: boolean;
    } = {},
  ) {
    list.mockReset().mockResolvedValue({ entries, tableRevision: 7 });
    catalog.mockReset().mockResolvedValue(create(ContentSchema, {}));
    effectMenu.mockReset().mockResolvedValue(menuResponse());
    classDefaultsCall.mockReset().mockResolvedValue(classDefaults());
    if (opts.failCatalog) catalog.mockRejectedValue(new Error('offline'));
    if (opts.failMenu) effectMenu.mockRejectedValue(new Error('offline'));
    if (opts.failDefaults) classDefaultsCall.mockRejectedValue(new Error('offline'));
    archive.mockReset().mockImplementation(async (_c: string, k: string) => ({
      ...entries.find((e) => e.key === k)!,
      archived: true,
    }));
    unarchive.mockReset().mockImplementation(async (_c: string, k: string) => ({
      ...entries.find((e) => e.key === k)!,
      archived: false,
    }));
    setSwitches.mockReset().mockResolvedValue({ tableRevision: 8, changed: 1, options: [] });
    getCampaign.mockReset().mockResolvedValue({
      campaign: { id: 'camp-1', name: 'Mirathel', myRole: role, awaitingApproval: false },
    });
    params$ = new BehaviorSubject(convertToParamMap({ id: 'camp-1', key }));
    window.matchMedia = (() => ({
      matches: opts.phone === true,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    })) as never;
    TestBed.resetTestingModule();
    watcher = fakeContentWatcher();
    TestBed.overrideComponent(ContentEntry, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: ActivatedRoute,
          useValue: { paramMap: params$, snapshot: { queryParamMap: convertToParamMap({}) } },
        },
        { provide: CampaignsService, useValue: { getCampaign } },
        {
          provide: TableContentClient,
          useValue: {
            list,
            catalog,
            effectMenu,
            archive,
            unarchive,
            setSwitches,
            classDefaults: classDefaultsCall,
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(ContentEntry);
    await settle(fixture);
    return { fixture, el: fixture.nativeElement as HTMLElement, params$ };
  }

  async function settle(fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) {
    for (let i = 0; i < 4; i++) {
      fixture.detectChanges();
      await new Promise((r) => setTimeout(r));
      await fixture.whenStable();
    }
    fixture.detectChanges();
  }

  const text = (el: Element) => (el.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const click = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => text(b).includes(label))!
      .click();

  it('opens the race in the editor for the master, with the state in words and "Arquivar"', async () => {
    const { el } = await setup(Role.MASTER, 'race:corujeiro@mesa');
    expect(text(el.querySelector('h1')!)).toBe('Corujeiro');
    expect(el.querySelector('app-race-editor')).not.toBeNull();
    expect(text(el.querySelector('.tags')!)).toContain('Raça da mesa');
    expect(text(el.querySelector('.tags')!)).toContain('Em uso por 2 fichas');
    expect(text(el)).toContain('Voltar para Raças');
    expect(
      Array.from(el.querySelectorAll('button')).some((b) => text(b).includes('Arquivar')),
    ).toBe(true);
  });

  it('asks in place, turns "Salvar raça" off with the reason, and archives: the result says it and offers "Desarquivar"', async () => {
    const { fixture, el } = await setup(Role.MASTER, 'race:corujeiro@mesa');
    click(el, 'Arquivar');
    await settle(fixture);
    const ask = el.querySelector('app-archive-question')!;
    expect(text(ask)).toContain('Arquivar Corujeiro?');
    expect(text(ask)).toContain(
      'As fichas que usam Corujeiro continuam funcionando. A entrada só deixa de aparecer para fichas novas.',
    );
    expect(text(ask)).toContain('2 fichas usam Corujeiro agora.');
    expect(text(el.querySelector('app-editor-bar')!)).toContain(
      'Responda à pergunta de arquivar para voltar a salvar.',
    );
    expect(document.activeElement?.id).toBe('ask-t');
    // "Voltar" closes it and nothing is archived.
    click(ask as HTMLElement, 'Voltar');
    await settle(fixture);
    expect(el.querySelector('app-archive-question')).toBeNull();
    expect(archive).not.toHaveBeenCalled();
    click(el, 'Arquivar');
    await settle(fixture);
    click(el.querySelector('app-archive-question') as HTMLElement, 'Arquivar Corujeiro');
    await settle(fixture);
    expect(archive).toHaveBeenCalledWith('camp-1', 'race:corujeiro@mesa');
    expect(text(el.querySelector('.archived')!)).toContain('A raça Corujeiro está arquivada.');
    expect(text(el.querySelector('.archived')!)).toContain(
      'A entrada só deixa de aparecer para fichas novas.',
    );
    expect(text(el.querySelector('.tags')!)).toContain('Arquivada · 2 fichas usam');
    // An archived entry can still be edited, and there is no "Apagar".
    expect(el.querySelector('app-race-editor')).not.toBeNull();
    expect(text(el)).not.toContain('Apagar');
    click(el, 'Desarquivar');
    await settle(fixture);
    expect(unarchive).toHaveBeenCalledWith('camp-1', 'race:corujeiro@mesa');
    expect(el.querySelector('.archived')).toBeNull();
  });

  it('gives a player the read view of a race in full, with no editor, no state and no "Arquivar"', async () => {
    const onlyOn = mirathel().map((e) => ({ ...e, charactersUsing: 0 }));
    const { el } = await setup(Role.PLAYER, 'race:corujeiro@mesa', onlyOn);
    expect(el.querySelector('app-entry-read')).not.toBeNull();
    expect(el.querySelector('app-race-editor')).toBeNull();
    expect(text(el)).toContain('Voltar para Conteúdo da mesa');
    expect(text(el.querySelector('.tags')!)).toBe('menu_bookRaça da mesa');
    expect(text(el)).toContain('Deslocamento');
    expect(el.querySelector('form, input')).toBeNull();
    expect(
      Array.from(el.querySelectorAll('button')).some((b) => text(b).includes('Arquivar')),
    ).toBe(false);
    expect(effectMenu).not.toHaveBeenCalled();
  });

  it("opens a class for the master in the class editor, fed by the server's defaults", async () => {
    const { el } = await setup(Role.MASTER, 'class:guardi-o-do-vale@mesa');
    expect(el.querySelector('app-class-editor')).not.toBeNull();
    expect(el.querySelector('app-entry-read')).toBeNull();
    expect(classDefaultsCall).toHaveBeenCalledWith('camp-1');
    expect(text(el.querySelector('.tags')!)).toContain('Classe da mesa');
    expect(text(el)).toContain('Voltar para Classes');
    expect(
      Array.from(el.querySelectorAll('button')).some((b) => text(b).includes('Arquivar')),
    ).toBe(true);
  });

  it('has the switch "Disponível para os jogadores" in the class editor (under the section list) and in the subclass editor (before the save bar)', async () => {
    const klass = await setup(Role.MASTER, 'class:guardi-o-do-vale@mesa');
    expect(
      klass.el
        .querySelector('aside.side app-players-switch [role="switch"]')
        ?.getAttribute('aria-checked'),
    ).toBe('true');
    klass.el.querySelector<HTMLButtonElement>('app-players-switch [role="switch"]')!.click();
    await settle(klass.fixture);
    expect(setSwitches).toHaveBeenCalledWith('camp-1', [
      { key: 'class:guardi-o-do-vale@mesa', off: true },
    ]);
    // The header and the state follow, in the feminine for a class.
    expect(text(klass.el.querySelector('.tags')!)).toContain('Desligada para os jogadores');
    const sub = await setup(Role.MASTER, 'subclass:tradi-o-da-tinta@mesa');
    const panel = sub.el.querySelector('app-subclass-editor app-players-switch');
    expect(panel).not.toBeNull();
    expect(panel!.nextElementSibling?.tagName.toLowerCase()).toBe('app-editor-bar');
  });

  it("writes the background's switch in the masculine", async () => {
    const { fixture, el } = await setup(Role.MASTER, 'background:cart-grafo-do-vale@mesa');
    el.querySelector<HTMLButtonElement>('app-players-switch [role="switch"]')!.click();
    await settle(fixture);
    expect(text(el.querySelector('.tags')!)).toContain('Desligado para os jogadores');
    expect(text(el.querySelector('app-players-switch')!)).toContain(
      'ninguém o escolhe numa ficha nova e os jogadores não o leem',
    );
  });

  it('opens a subclass for the master in the subclass editor, and a new one for the class named in the link', async () => {
    const { el } = await setup(Role.MASTER, 'subclass:tradi-o-da-tinta@mesa');
    expect(el.querySelector('app-subclass-editor')).not.toBeNull();
    expect(text(el.querySelector('.tags')!)).toContain('Subclasse da mesa');
  });

  it("reads a class for a player in full, and never asks the server for the master's defaults or menu", async () => {
    const { el } = await setup(
      Role.PLAYER,
      'class:guardi-o-do-vale@mesa',
      mirathel().map((e) => ({ ...e, charactersUsing: 0 })),
    );
    expect(el.querySelector('app-entry-read')).not.toBeNull();
    expect(el.querySelector('app-class-editor')).toBeNull();
    expect(text(el)).toContain('Testes de resistência');
    expect(classDefaultsCall).not.toHaveBeenCalled();
    expect(effectMenu).not.toHaveBeenCalled();
  });

  it('on a phone the master reads a class and archives it, with no editor and no defaults asked', async () => {
    const { el } = await setup(Role.MASTER, 'class:guardi-o-do-vale@mesa', mirathel(), {
      phone: true,
    });
    expect(el.querySelector('app-class-editor')).toBeNull();
    expect(classDefaultsCall).not.toHaveBeenCalled();
    expect(text(el)).toContain('Testes de resistência');
    expect(
      Array.from(el.querySelectorAll('button')).some((b) => text(b).includes('Arquivar')),
    ).toBe(true);
  });

  it('says when the defaults of the class table did not come, with "Tentar de novo"', async () => {
    const { fixture, el } = await setup(Role.MASTER, 'class:guardi-o-do-vale@mesa', mirathel(), {
      failDefaults: true,
    });
    expect(el.querySelector('app-class-editor')).toBeNull();
    expect(text(el)).toContain('Tentar de novo');
    classDefaultsCall.mockResolvedValue(classDefaults());
    click(el, 'Tentar de novo');
    await settle(fixture);
    expect(el.querySelector('app-class-editor')).not.toBeNull();
  });

  it('says so when the entry is not in the list (a player never gets an archived one)', async () => {
    const { el } = await setup(
      Role.PLAYER,
      'class:bardo-das-cinzas@mesa',
      mirathel().filter((e) => !e.archived),
    );
    expect(text(el)).toContain('Entrada não encontrada');
    expect(entry(TableContentKind.CLASS, 'x')).toBeDefined();
  });

  it('keeps an unsaved draft when the entry is archived and brought back', async () => {
    const { fixture, el } = await setup(Role.MASTER, 'race:corujeiro@mesa');
    const field = () => el.querySelector<HTMLInputElement>('[data-field="table_race.name_pt"]')!;
    field().value = 'Corujeiro Pálido';
    field().dispatchEvent(new Event('input'));
    await settle(fixture);
    click(el, 'Arquivar');
    await settle(fixture);
    click(el.querySelector('app-archive-question') as HTMLElement, 'Arquivar Corujeiro');
    await settle(fixture);
    expect(archive).toHaveBeenCalled();
    expect(field().value).toBe('Corujeiro Pálido');
    click(el, 'Desarquivar');
    await settle(fixture);
    expect(field().value).toBe('Corujeiro Pálido');
  });

  it('shows why the server refused an unarchive, not a connection problem', async () => {
    const { fixture, el } = await setup(Role.MASTER, 'class:bardo-das-cinzas@mesa');
    const refusal = create(TableContentRefusalSchema, {
      violations: [
        create(TableContentViolationSchema, {
          field: 'table_class.name_pt',
          reason: 'duplicate_name',
        }),
      ],
    });
    unarchive.mockRejectedValue(
      new ConnectError('x', Code.InvalidArgument, undefined, [
        { desc: TableContentRefusalSchema, value: refusal },
      ]),
    );
    click(el, 'Desarquivar');
    await settle(fixture);
    expect(unarchive).toHaveBeenCalled();
    expect(text(el)).toContain('Não foi possível desarquivar');
    expect(text(el)).not.toContain('Confira a conexão');
  });

  it('leaves nothing of the last entry when the page goes to another one', async () => {
    const { fixture, el, params$: p } = await setup(Role.MASTER, 'race:corujeiro@mesa');
    const page = fixture.componentInstance as unknown as Record<
      string,
      { set(v: unknown): void; (): unknown }
    >;
    page['savedLine'].set('A raça Corujeiro foi salva.');
    page['affected'].set([{ characterId: 'c1', name: 'Pensantus' }]);
    page['actionError'].set('Não foi possível arquivar.');
    await settle(fixture);
    expect(text(el)).toContain('A raça Corujeiro foi salva.');
    p.next(convertToParamMap({ id: 'camp-1', key: 'spell:l-mina-de-nanquim@mesa' }));
    await settle(fixture);
    expect(text(el)).not.toContain('A raça Corujeiro foi salva.');
    expect(text(el)).not.toContain('Pensantus');
    expect(text(el)).not.toContain('Não foi possível arquivar.');
    expect(text(el.querySelector('h1')!)).toBe('Lâmina de Nanquim');
  });

  it('says so, with "Tentar de novo", when the catalog or the effect menu does not come (no endless loading)', async () => {
    const { fixture, el } = await setup(Role.MASTER, 'race:corujeiro@mesa', mirathel(), {
      failMenu: true,
    });
    expect(el.querySelector('[role="alert"]')).not.toBeNull();
    expect(text(el)).toContain('Tentar de novo');
    expect(el.querySelector('mat-spinner')).toBeNull();
    effectMenu.mockResolvedValue(menuResponse());
    click(el, 'Tentar de novo');
    await settle(fixture);
    expect(el.querySelector('app-race-editor')).not.toBeNull();
    const failed = await setup(Role.PLAYER, 'race:corujeiro@mesa', mirathel(), {
      failCatalog: true,
    });
    expect(text(failed.el)).toContain('Tentar de novo');
  });

  it('opens the spell editor with the catalog alone: it does not wait for the effect menu', async () => {
    const { el } = await setup(Role.MASTER, 'spell:l-mina-de-nanquim@mesa', mirathel(), {
      failMenu: true,
    });
    expect(el.querySelector('app-spell-editor')).not.toBeNull();
    expect(text(el)).not.toContain('Tentar de novo');
  });
  it('has the entry\'s own switch "Disponível para os jogadores" in the editor, and saves it at once (RN-23)', async () => {
    const { fixture, el } = await setup(Role.MASTER, 'race:corujeiro@mesa');
    const sw = el.querySelector<HTMLButtonElement>('app-players-switch [role="switch"]')!;
    expect(text(el.querySelector('app-players-switch')!)).toContain('Disponível para os jogadores');
    expect(sw.getAttribute('aria-checked')).toBe('true');
    expect(text(el.querySelector('app-players-switch')!)).toContain('Ligado');
    sw.click();
    await settle(fixture);
    expect(setSwitches).toHaveBeenCalledWith('camp-1', [{ key: 'race:corujeiro@mesa', off: true }]);
    expect(
      el.querySelector('app-players-switch [role="switch"]')!.getAttribute('aria-checked'),
    ).toBe('false');
    expect(text(el.querySelector('app-players-switch')!)).toContain('Desligado');
    expect(text(el.querySelector('app-players-switch')!)).toContain(
      'As 2 fichas que a usam continuam funcionando.',
    );
    // The header says it too, and the form is where it was (no reload of the editor).
    expect(text(el.querySelector('.tags')!)).toContain('Desligada para os jogadores');
    expect(el.querySelector('app-race-editor')).not.toBeNull();
  });

  it('puts the switch back and says why when the server refuses it', async () => {
    const { fixture, el } = await setup(Role.MASTER, 'race:corujeiro@mesa');
    setSwitches.mockRejectedValue(new Error('offline'));
    el.querySelector<HTMLButtonElement>('app-players-switch [role="switch"]')!.click();
    await settle(fixture);
    expect(
      el.querySelector('app-players-switch [role="switch"]')!.getAttribute('aria-checked'),
    ).toBe('true');
    expect(text(el.querySelector('app-players-switch [role="alert"]')!)).toContain(
      'O interruptor continua como estava.',
    );
  });

  it('has no switch for a player, who only reads what is on', async () => {
    const { el } = await setup(Role.PLAYER, 'race:corujeiro@mesa');
    expect(el.querySelector('app-players-switch')).toBeNull();
  });

  it('reads the entries again when the table changed (content_changed): the master sees the switch another tab turned', async () => {
    const { fixture, el } = await setup(Role.MASTER, 'race:corujeiro@mesa');
    expect(watcher.following()).toBe('camp-1');
    list.mockResolvedValue({
      entries: mirathel().map((e) =>
        e.key === 'race:corujeiro@mesa' ? ({ ...e, off: true } as typeof e) : e,
      ),
      tableRevision: 8,
    });
    watcher.hint();
    await settle(fixture);
    expect(text(el.querySelector('.tags')!)).toContain('Desligada para os jogadores');
    expect(
      el.querySelector('app-players-switch [role="switch"]')!.getAttribute('aria-checked'),
    ).toBe('false');
  });

  it('keeps the body the master is editing when another write changed the entry, and still shows its switch', async () => {
    const { fixture, el } = await setup(Role.MASTER, 'race:corujeiro@mesa');
    const input = el.querySelector<HTMLInputElement>('app-race-editor input')!;
    input.value = 'Corujeiro dos Vales';
    input.dispatchEvent(new Event('input'));
    await settle(fixture);
    list.mockResolvedValue({
      entries: mirathel().map((e) =>
        e.key === 'race:corujeiro@mesa' ? ({ ...e, revision: 9, off: true } as typeof e) : e,
      ),
      tableRevision: 9,
    });
    watcher.hint();
    await settle(fixture);
    expect(el.querySelector<HTMLInputElement>('app-race-editor input')!.value).toBe(
      'Corujeiro dos Vales',
    );
    expect(text(el.querySelector('.tags')!)).toContain('Desligada para os jogadores');
  });

  describe('when the table changes under an open sub-race', () => {
    const RACE = 'race:corujeiro@mesa';
    const sub = () =>
      entry(TableContentKind.SUBRACE, 'Corujeiro Pálido', {
        body: {
          case: 'tableSubrace',
          value: create(TableSubraceSchema, { namePt: 'Corujeiro Pálido', raceKey: RACE }),
        },
      });
    const race = (name: string) => entry(TableContentKind.RACE, name, { key: RACE });

    it('shows the name of a race that became visible, not its key', async () => {
      const { fixture, el } = await setup(Role.PLAYER, sub().key, [sub()]);
      expect(text(el)).toContain('Sub-raça de');
      expect(text(el)).toContain(RACE);
      list.mockResolvedValue({ entries: [sub(), race('Corujeiro')], tableRevision: 8 });
      watcher.hint();
      await settle(fixture);
      expect(text(el)).not.toContain(RACE);
      expect(text(el)).toContain('Corujeiro');
    });

    it('lets the newest read win when two reads finish out of order', async () => {
      const { fixture, el } = await setup(Role.PLAYER, sub().key, [sub()]);
      const deferred = () => {
        let resolve!: (v: unknown) => void;
        const promise = new Promise((r) => (resolve = r));
        return { promise, resolve };
      };
      const older = deferred();
      const newer = deferred();
      list.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
      watcher.hint();
      watcher.hint();
      await settle(fixture);
      newer.resolve({ entries: [sub(), race('Corujeiro Novo')], tableRevision: 9 });
      await settle(fixture);
      older.resolve({ entries: [sub(), race('Corujeiro Velho')], tableRevision: 8 });
      await settle(fixture);
      expect(text(el)).toContain('Corujeiro Novo');
      expect(text(el)).not.toContain('Corujeiro Velho');
    });
  });
});
