import { create } from '@bufbuild/protobuf';
import { Location } from '@angular/common';
import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { CampaignSchema, Role } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterKind } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  GetSpellDetailsResponseSchema,
  ListContentResponseSchema,
  ListSpellsResponseSchema,
  SpellDetailsSchema,
  SpellSchema,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { CONNECT_TRANSPORT } from '../../core/connect/transport';
import { fakeContentWatcher } from '../../core/content/content-testing';
import { RosterClient } from '../../core/maps/roster-client';
import { SpellsClient } from '../../core/spells/spells-client';
import { Spells } from './spells';

const spell = (key: string, namePt: string, extra: Record<string, unknown> = {}) =>
  create(SpellSchema, { key, namePt, name: namePt, level: 1, schoolNamePt: 'Evocação', ...extra });
const flat = (n: Element | null) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

describe('Spells, the players\' "Magias" page (MR-045, E10-11)', () => {
  let requests: Record<string, unknown>[];
  let list: (
    req: Record<string, unknown>,
  ) => Promise<ReturnType<typeof create<typeof ListSpellsResponseSchema>>>;
  let role: Role;
  let awaiting: boolean;
  let character: { id: string; name: string; kind: CharacterKind; classSummary: string }[];
  let wide = false;
  let campaignFails: Error | null = null;
  let detailsFail: Error | null = null;
  const originalMatchMedia = window.matchMedia;

  const rows = () => [
    spell('spell:burning-hands', 'Mãos Flamejantes', { name: 'Burning Hands' }),
    spell('spell:lamina-de-nanquim@mesa', 'Lâmina de Nanquim'),
    spell('spell:old@mesa', 'Velha', { archived: true }),
  ];

  let location: Location;
  let watcher = fakeContentWatcher();

  /** The history answers a step back on its own time: wait for what the page shows. */
  async function untilBack(settle: () => Promise<void>, done: () => boolean): Promise<void> {
    for (let i = 0; i < 20 && !done(); i++) {
      await new Promise((resolve) => setTimeout(resolve, 10));
      await settle();
    }
  }

  async function open(url = '/campaigns/camp-1/spells') {
    TestBed.overrideComponent(Spells, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([{ path: 'campaigns/:id/spells', component: Spells }]),
        {
          provide: CampaignsService,
          useValue: {
            getCampaign: async () => {
              if (campaignFails) {
                throw campaignFails;
              }
              return {
                campaign: create(CampaignSchema, {
                  id: 'camp-1',
                  name: 'Mirathel',
                  myRole: role,
                  awaitingApproval: awaiting,
                }),
              };
            },
          },
        },
        {
          provide: RosterClient,
          useValue: {
            list: async () =>
              character.map((c) => ({ ...c, playerUserId: 'u', raceName: '', playerName: null })),
          },
        },
        {
          provide: SpellsClient,
          useValue: {
            list: (req: Record<string, unknown>) => {
              requests.push(req);
              return list(req);
            },
            classes: async () => [{ key: 'class:wizard', namePt: 'Mago', archived: false }],
            details: async (_c: string, key: string) => {
              if (detailsFail) {
                throw detailsFail;
              }
              return create(SpellDetailsSchema, {
                spell: {
                  key,
                  namePt: key.endsWith('@mesa') ? 'Lâmina de Nanquim' : 'Mãos Flamejantes',
                  name: 'Burning Hands',
                  level: 1,
                  schoolNamePt: 'Evocação',
                  classKeys: ['class:wizard'],
                },
                target: { labelPt: 'Cone de 4,5 m' },
                description: ['As you hold your hands…'],
              });
            },
          },
        },
      ],
    });
    const harness = await RouterTestingHarness.create();
    location = TestBed.inject(Location);
    await harness.navigateByUrl(url, Spells);
    // The router listens to the browser's Back only after the initial navigation (the harness skips it).
    TestBed.inject(Router).initialNavigation();
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        harness.detectChanges();
        await harness.fixture.whenStable();
      }
    };
    await settle();
    return { el: harness.routeNativeElement as HTMLElement, settle, harness };
  }

  beforeEach(() => {
    watcher = fakeContentWatcher();
    requests = [];
    role = Role.PLAYER;
    awaiting = false;
    character = [
      { id: 'char-1', name: 'Pensantus', kind: CharacterKind.PLAYER, classSummary: 'Mago 4' },
    ];
    list = async () => create(ListSpellsResponseSchema, { spells: rows(), total: 3 });
    wide = false;
    campaignFails = null;
    detailsFail = null;
    window.matchMedia = ((q: string) => ({
      matches: wide && q.includes('1100'),
      media: q,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    })) as unknown as typeof window.matchMedia;
  });
  afterEach(() => {
    window.matchMedia = originalMatchMedia;
  });

  it('lists what the server answered, with "Da mesa" on a table spell and the count', async () => {
    const { el } = await open();
    expect(flat(el.querySelector('h1'))).toBe('Magias');
    const items = Array.from(el.querySelectorAll('button.row'));
    expect(items.map((r) => flat(r.querySelector('.row__name')))).toEqual([
      'Mãos Flamejantes',
      'Lâmina de Nanquim',
      'Velha',
    ]);
    expect(flat(items[0].querySelector('.row__sub'))).toBe('1º nível · Evocação');
    expect(flat(items[1])).toContain('Da mesa');
    expect(flat(items[0])).not.toContain('Da mesa');
    expect(flat(el.querySelector('.list__n'))).toContain('3 magias');
    expect(requests).toHaveLength(1);
    expect(requests[0]).toMatchObject({
      campaignId: 'camp-1',
      query: '',
      characterId: '',
      pageToken: '',
    });
  });

  it('marks a retired table spell "Arquivada" (the server only sends it to the master)', async () => {
    role = Role.MASTER;
    const { el } = await open();
    expect(flat(el.querySelectorAll('button.row')[2])).toContain('Arquivada');
    expect(flat(el.querySelectorAll('button.row')[0])).not.toContain('Arquivada');
  });

  it('asks only the server for the filters in the link, and "Só as que posso aprender" with the character', async () => {
    const { el } = await open(
      '/campaigns/camp-1/spells?q=maos&class=class:wizard&levels=0,1&school=school:evocation&mine=1',
    );
    expect(requests[0]).toMatchObject({
      query: 'maos',
      classKey: 'class:wizard',
      levels: [0, 1],
      schoolKeys: ['school:evocation'],
      characterId: 'char-1',
    });
    expect(flat(el.querySelector('.filters-btn'))).toContain('Filtros (4)');
    expect(flat(el.querySelector('.chips'))).toContain('Só as que posso aprender');
    expect(flat(el.querySelector('.mr-page-lead'))).toBe('Mirathel · As magias para Pensantus');
    // The count is said once, by the list.
    expect(flat(el.querySelector('.list__n'))).toContain('3 magias');
  });

  it('gives the master no "Só as que posso aprender"', async () => {
    role = Role.MASTER;
    const { el } = await open('/campaigns/camp-1/spells?mine=1');
    expect(requests[0]).toMatchObject({ characterId: '' });
    expect(flat(el.querySelector('.chips'))).not.toContain('posso aprender');
  });

  it('opens a spell in place of the list on the narrow page, with "Alvo" from the server, and goes back', async () => {
    const { el, settle } = await open();
    el.querySelectorAll<HTMLButtonElement>('button.row')[0].click();
    await settle();
    expect(el.querySelector('button.row')).toBeNull();
    expect(flat(el.querySelector('#spell-card-title'))).toBe('Mãos Flamejantes');
    expect(flat(el.querySelector('.spell__facts'))).toContain('Alvo Cone de 4,5 m');
    expect(flat(el.querySelector('.card__line'))).toContain(
      'Burning Hands · 1º nível · Evocação · Mago',
    );
    expect(flat(el)).toContain('Texto do SRD 5.1, em inglês.');
    expect(el.querySelector('a[href="/credits"]')).not.toBeNull();
    // "Voltar para Magias" takes back the step that opened the spell: the same list, no new ask, the focus on the row.
    document.body.appendChild(el);
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => flat(b) === 'arrow_back Voltar para Magias')!
      .click();
    await untilBack(settle, () => el.querySelectorAll('button.row').length === 3);
    expect(requests).toHaveLength(1);
    expect(location.path()).not.toContain('spell=');
    expect(document.activeElement).toBe(
      el.querySelector('button.row[data-key="spell:burning-hands"]'),
    );
    el.remove();
  });

  it("closes the spell with the browser's own Back too, and the list is the same one", async () => {
    const { el, settle } = await open();
    document.body.appendChild(el);
    el.querySelectorAll<HTMLButtonElement>('button.row')[1].click();
    await settle();
    expect(location.path()).toContain('spell=');
    location.back();
    await untilBack(settle, () => el.querySelectorAll('button.row').length === 3);
    expect(document.activeElement).toBe(el.querySelectorAll('button.row')[1]);
    el.remove();
  });

  it('does not push a step for a spell opened by a link: "Voltar para Magias" leaves the link behind', async () => {
    const { el, settle } = await open('/campaigns/camp-1/spells?spell=spell:burning-hands');
    expect(flat(el.querySelector('#spell-card-title'))).toBe('Mãos Flamejantes');
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => flat(b) === 'arrow_back Voltar para Magias')!
      .click();
    await settle();
    await settle();
    expect(location.path()).not.toContain('spell=');
    expect(el.querySelectorAll('button.row')).toHaveLength(3);
  });

  it("follows the link's filters when the history brings other ones, so the link and the state never drift", async () => {
    list = async (req) =>
      create(ListSpellsResponseSchema, {
        spells: req['query'] ? [] : rows(),
        total: req['query'] ? 0 : 3,
      });
    const { el, settle, harness } = await open('/campaigns/camp-1/spells?class=class:wizard');
    expect(requests.at(-1)).toMatchObject({ classKey: 'class:wizard', query: '' });
    // Back to an older entry: another search in the link.
    await harness.navigateByUrl('/campaigns/camp-1/spells?q=zzz');
    await settle();
    await settle();
    expect(requests.at(-1)).toMatchObject({ classKey: '', query: 'zzz' });
    expect(el.querySelector<HTMLInputElement>('input[type=search]')!.value).toBe('zzz');
    expect(flat(el.querySelector('.empty__t'))).toContain('Nenhuma magia com “zzz”.');
  });

  it('shows the list and the spell side by side on a wide screen', async () => {
    wide = true;
    const { el, settle } = await open();
    expect(flat(el.querySelector('.detail__empty'))).toContain('Escolha uma magia');
    el.querySelectorAll<HTMLButtonElement>('button.row')[0].click();
    await settle();
    expect(el.querySelectorAll('button.row')).toHaveLength(3);
    expect(el.querySelector('button.row--on')?.getAttribute('aria-current')).toBe('true');
    expect(flat(el.querySelector('.spell__facts'))).toContain('Alvo Cone de 4,5 m');
  });

  it('says there is no spell with the name, offers "Limpar a busca" and keeps the focus in the field', async () => {
    list = async (req) =>
      create(ListSpellsResponseSchema, {
        spells: req['query'] ? [] : rows(),
        total: req['query'] ? 0 : 3,
      });
    const { el, settle } = await open('/campaigns/camp-1/spells?q=zzz');
    expect(flat(el.querySelector('.empty__t'))).toContain('Nenhuma magia com “zzz”.');
    expect(flat(el.querySelector('.empty__s'))).toBe('Confira o nome ou tire um filtro.');
    document.body.appendChild(el);
    Array.from(el.querySelectorAll<HTMLButtonElement>('.empty button'))
      .find((b) => flat(b) === 'Limpar a busca')!
      .click();
    await settle();
    expect(el.querySelectorAll('button.row')).toHaveLength(3);
    expect(document.activeElement).toBe(el.querySelector('input[type=search]'));
    el.remove();
  });

  it('says what failed, with "Tentar de novo"', async () => {
    let fail = true;
    list = async () => {
      if (fail) {
        throw new ConnectError('x', Code.Unavailable);
      }
      return create(ListSpellsResponseSchema, { spells: rows(), total: 3 });
    };
    const { el, settle } = await open();
    expect(flat(el.querySelector('[role=alert]'))).toContain('o servidor não respondeu');
    fail = false;
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => flat(b) === 'Tentar de novo')!
      .click();
    await settle();
    expect(el.querySelectorAll('button.row')).toHaveLength(3);
  });

  it('reads a basic sheet with "Só as que posso aprender" as a sentence, with the way to every spell', async () => {
    list = async (req) => {
      if (req['characterId']) {
        throw new ConnectError('x', Code.FailedPrecondition);
      }
      return create(ListSpellsResponseSchema, { spells: rows(), total: 3 });
    };
    const { el, settle } = await open('/campaigns/camp-1/spells?mine=1');
    expect(flat(el.querySelector('.list__msg'))).toContain(
      'Essa ficha é básica e não tem classes que conjuram',
    );
    expect(flat(el.querySelector('.list__msg'))).not.toContain('failed');
    Array.from(el.querySelectorAll<HTMLButtonElement>('.list__msg button'))
      .find((b) => flat(b) === 'Ler todas as magias')!
      .click();
    await settle();
    expect(el.querySelectorAll('button.row')).toHaveLength(3);
  });

  it('pages with "Mostrar mais" and the server\'s token', async () => {
    list = async (req) =>
      req['pageToken']
        ? create(ListSpellsResponseSchema, { spells: [spell('spell:z', 'Zumbido')], total: 4 })
        : create(ListSpellsResponseSchema, { spells: rows(), total: 4, nextPageToken: 'tok' });
    const { el, settle } = await open();
    expect(flat(el.querySelector('.more__n'))).toBe('Mostrando 3 de 4 magias');
    Array.from(el.querySelectorAll<HTMLButtonElement>('.more button'))
      .find((b) => flat(b) === 'Mostrar mais')!
      .click();
    await settle();
    expect(requests[1]).toMatchObject({ pageToken: 'tok' });
    expect(el.querySelectorAll('button.row')).toHaveLength(4);
    expect(el.querySelector('.more')).toBeNull();
  });

  it('tells a member waiting for approval, and a stranger, without asking for spells', async () => {
    awaiting = true;
    const first = await open();
    expect(flat(first.el)).toContain('As magias abrem quando o mestre aprovar o seu personagem.');
    expect(requests).toHaveLength(0);
  });

  it("says so when the campaign is not the person's, without asking for spells (the stranger)", async () => {
    campaignFails = new ConnectError('x', Code.NotFound);
    const { el } = await open();
    expect(flat(el.querySelector('h1'))).toBe('Campanha não encontrada');
    expect(requests).toHaveLength(0);
  });

  it('gives a spell that is not there its own sentence and nothing to retry (an archived spell, for a player)', async () => {
    detailsFail = new ConnectError('x', Code.NotFound);
    const { el, settle } = await open();
    el.querySelectorAll<HTMLButtonElement>('button.row')[1].click();
    await settle();
    expect(flat(el.querySelector('.card [role=alert]'))).toContain(
      'Esta magia não está disponível.',
    );
    expect(flat(el)).not.toContain('Tentar de novo');
    expect(flat(el)).not.toContain('campanha não existe');
  });

  it('retries a spell that failed for another reason', async () => {
    detailsFail = new ConnectError('x', Code.Unavailable);
    const { el, settle } = await open();
    el.querySelectorAll<HTMLButtonElement>('button.row')[0].click();
    await settle();
    expect(flat(el.querySelector('.card [role=alert]'))).toContain('o servidor não respondeu');
    detailsFail = null;
    Array.from(el.querySelectorAll<HTMLButtonElement>('.card button'))
      .find((b) => flat(b) === 'Tentar de novo')!
      .click();
    await settle();
    expect(flat(el.querySelector('#spell-card-title'))).toBe('Mãos Flamejantes');
  });

  it('shows a table spell\'s card with "Da mesa" and the master\'s own words (no SRD label, no English)', async () => {
    const { el, settle } = await open();
    el.querySelectorAll<HTMLButtonElement>('button.row')[1].click();
    await settle();
    expect(flat(el.querySelector('.card__head'))).toContain('Da mesa');
    expect(el.querySelector('.spell__prose[lang=en]')).toBeNull();
    expect(flat(el)).not.toContain('Texto do SRD 5.1');
  });

  it('writes the filters into the link when a search starts, not on every key', async () => {
    const { el, settle } = await open();
    const q = el.querySelector<HTMLInputElement>('input[type=search]')!;
    q.value = 'm';
    q.dispatchEvent(new Event('input'));
    await settle();
    expect(location.path()).not.toContain('q=');
    await vi.waitFor(async () => {
      await settle();
      expect(location.path()).toContain('q=m');
    });
  });

  it('puts the focus on the next chip when one is taken off, and on the search when none is left', async () => {
    const { el, settle } = await open('/campaigns/camp-1/spells?class=class:wizard&mine=1');
    await settle();
    document.body.appendChild(el);
    const x = () => Array.from(el.querySelectorAll<HTMLButtonElement>('.chip__x'));
    expect(x()).toHaveLength(2);
    x()[0].click();
    await settle();
    expect(x()).toHaveLength(1);
    expect(document.activeElement).toBe(x()[0]);
    x()[0].click();
    await settle();
    expect(document.activeElement).toBe(el.querySelector('input[type=search]'));
    el.remove();
  });

  it('shows no chip for a class the link names until the class names are known', async () => {
    const { el } = await open('/campaigns/camp-1/spells?class=class:wizard');
    // The names load with the first answer; before them the chip would be empty, so it is not drawn. After them it says "Mago".
    expect(flat(el.querySelector('.chips'))).toBe('Magoclose');
  });

  it('asks "Confira o nome" only when a name was typed', async () => {
    list = async () => create(ListSpellsResponseSchema, { spells: [], total: 0 });
    const withName = await open('/campaigns/camp-1/spells?q=zzz');
    expect(flat(withName.el.querySelector('.empty__s'))).toBe('Confira o nome ou tire um filtro.');
  });
  it("reads the list again when the table's content changed (content_changed, RN-23): a spell the master switched off leaves it", async () => {
    const { el, settle } = await open('/campaigns/camp-1/spells?q=ma');
    expect(watcher.following()).toBe('camp-1');
    expect(el.querySelectorAll('button.row')).toHaveLength(3);
    list = async () => create(ListSpellsResponseSchema, { spells: rows().slice(0, 2), total: 2 });
    watcher.hint();
    await settle();
    await settle();
    expect(
      Array.from(el.querySelectorAll('button.row')).map((r) => flat(r.querySelector('.row__name'))),
    ).toEqual(['Mãos Flamejantes', 'Lâmina de Nanquim']);
    expect(flat(el.querySelector('.list__n'))).toContain('2 magias');
    // The same filter as the list on screen, from the first page.
    expect(requests).toHaveLength(2);
    expect(requests[1]).toMatchObject({ query: 'ma', pageToken: '' });
  });

  it('reads the open spell again too: one that is off now says it is not available, with nothing to retry', async () => {
    wide = true;
    const { el, settle } = await open('/campaigns/camp-1/spells?spell=spell:burning-hands');
    expect(flat(el.querySelector('#spell-card-title'))).toBe('Mãos Flamejantes');
    detailsFail = new ConnectError('gone', Code.NotFound);
    watcher.hint();
    await settle();
    await settle();
    expect(flat(el)).toContain('Esta magia não está disponível.');
    expect(
      Array.from(el.querySelectorAll('button')).some((b) => flat(b) === 'Tentar de novo'),
    ).toBe(false);
  });

  it('keeps the list on screen when the read after a change fails', async () => {
    const { el, settle } = await open();
    list = async () => {
      throw new ConnectError('offline', Code.Unavailable);
    };
    watcher.hint();
    await settle();
    await settle();
    expect(el.querySelectorAll('button.row')).toHaveLength(3);
    expect(flat(el)).not.toContain('Tentar de novo');
  });
});

describe('Spells, the open card after a content_changed (real SpellsClient)', () => {
  const key = 'spell:table-spell:foo';
  const originalMatchMedia = window.matchMedia;
  let version: string;
  let namePt: string;
  let off: boolean;

  beforeEach(() => {
    version = 'v1';
    namePt = 'Mãos Flamejantes';
    off = false;
    window.matchMedia = ((q: string) => ({
      matches: q.includes('1100'),
      media: q,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    })) as unknown as typeof window.matchMedia;
  });
  afterEach(() => {
    window.matchMedia = originalMatchMedia;
  });

  /** The page over the real client, so the details cache is the one the app has. */
  async function openCard() {
    const transport = {
      unary: async (method: { name: string }) => {
        let message: unknown;
        if (method.name === 'ListSpells') {
          message = create(ListSpellsResponseSchema, {
            spells: off ? [] : [spell(key, namePt)],
            total: off ? 0 : 1,
            contentVersion: version,
          });
        } else if (method.name === 'GetSpellDetails') {
          if (off) {
            throw new ConnectError('gone', Code.NotFound);
          }
          message = create(GetSpellDetailsResponseSchema, {
            spell: create(SpellDetailsSchema, {
              spell: spell(key, namePt),
              description: ['Texto completo da magia.'],
            }),
          });
        } else {
          message = create(ListContentResponseSchema, {});
        }
        return {
          stream: false,
          service: {},
          method,
          header: new Headers(),
          trailer: new Headers(),
          message,
        };
      },
    };
    const watcher = fakeContentWatcher();
    TestBed.overrideComponent(Spells, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([{ path: 'campaigns/:id/spells', component: Spells }]),
        { provide: CONNECT_TRANSPORT, useValue: transport },
        {
          provide: CampaignsService,
          useValue: {
            getCampaign: async () => ({
              campaign: create(CampaignSchema, {
                id: 'camp-1',
                name: 'Mirathel',
                myRole: Role.PLAYER,
              }),
            }),
          },
        },
        { provide: RosterClient, useValue: { list: async () => [] } },
      ],
    });
    const harness = await RouterTestingHarness.create();
    await harness.navigateByUrl(`/campaigns/camp-1/spells?spell=${key}`, Spells);
    const settle = async () => {
      for (let i = 0; i < 6; i++) {
        harness.detectChanges();
        await harness.fixture.whenStable();
        await new Promise((r) => setTimeout(r, 5));
      }
    };
    await settle();
    return { el: harness.routeNativeElement as HTMLElement, watcher, settle };
  }

  it('shows the edited text of the spell the master changed', async () => {
    const { el, watcher, settle } = await openCard();
    expect(flat(el.querySelector('#spell-card-title'))).toBe('Mãos Flamejantes');

    version = 'v2';
    namePt = 'Mãos Flamejantes (editada)';
    watcher.hint();
    await settle();

    expect(flat(el.querySelector('#spell-card-title'))).toBe('Mãos Flamejantes (editada)');
  });

  it('says the spell is not available once the master switched it off', async () => {
    const { el, watcher, settle } = await openCard();
    expect(flat(el)).toContain('Texto completo da magia.');

    off = true;
    version = 'v2';
    watcher.hint();
    await settle();

    expect(el.querySelectorAll('button.row')).toHaveLength(0);
    expect(flat(el)).toContain('Esta magia não está disponível.');
    expect(flat(el)).not.toContain('Texto completo da magia.');
  });
});
