import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { Subject, of } from 'rxjs';

import { CharacterVitalsSchema } from '../../../../gen/meurpg/play/v1/play_pb';
import { CreatureSource } from '../../../../gen/meurpg/characters/v1/characters_pb';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import {
  FakeCreaturesClient,
  creature,
  familiarSpell,
  flat,
  raven,
  summonAnswer,
} from '../../../core/creatures/creatures-testing';
import { OpenSessions } from '../../../shell/live-notice/open-sessions';
import { CreaturesPanel } from './creatures-panel';

const FAMILIAR = (): SpellAccess => ({ spells: [familiarSpell()] });
type SpellAccess = { spells: ReturnType<typeof familiarSpell>[] };

describe('CreaturesPanel (E9-10, MR-037, RN-20)', () => {
  let api: FakeCreaturesClient;
  const sessions = signal<readonly { campaignId: string }[]>([{ campaignId: 'camp-1' }]);
  const dialogOpen = vi.fn();

  async function setup(
    opts: {
      access?: SpellAccess;
      wildShape?: boolean;
      master?: boolean;
      creatures?: ReturnType<typeof creature>[];
      denied?: boolean;
      failing?: boolean;
    } = {},
  ) {
    api = new FakeCreaturesClient();
    api.creatures = opts.creatures ?? [];
    api.options = summonAnswer(opts.access?.spells ?? []);
    api.blocks.set('monster:raven', raven());
    if (opts.failing) {
      api.failWith = new ConnectError('down', Code.Unavailable);
    }
    if (opts.denied) {
      api.failWith = new ConnectError('nope', Code.NotFound);
    }
    dialogOpen.mockReset();
    dialogOpen.mockReturnValue({ afterClosed: () => of(undefined) });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: CreaturesClient, useValue: api },
        { provide: OpenSessions, useValue: { sessions } },
        { provide: MatDialog, useValue: { open: dialogOpen } },
        { provide: MatBottomSheet, useValue: { open: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(CreaturesPanel);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('characterId', 'char-1');
    fixture.componentRef.setInput('characterName', 'Pensantus');
    fixture.componentRef.setInput('isMaster', opts.master ?? false);
    fixture.componentRef.setInput('wildShape', opts.wildShape ?? false);
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        await fixture.whenStable();
        await new Promise((r) => setTimeout(r));
        fixture.detectChanges();
      }
    };
    fixture.detectChanges();
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    return { fixture, el, flat, settle };
  }

  beforeEach(() => sessions.set([{ campaignId: 'camp-1' }]));

  it('does not exist for a character that cannot have a creature: no empty panel to no use', async () => {
    const { el } = await setup();
    expect(el.querySelector('section')).toBeNull();
  });

  it('does not exist when the server says not_found: another player never reads the list (RN-20)', async () => {
    const { el } = await setup({ access: FAMILIAR(), denied: true });
    expect(el.querySelector('section')).toBeNull();
  });

  it('empty: invites the next action and offers the spell, with what it costs under the button', async () => {
    const { el, flat } = await setup({ access: FAMILIAR() });
    expect(flat(el.querySelector('h2'))).toBe('Criaturas');
    expect(flat(el.querySelector('.empty'))).toBe(
      'Nenhuma criatura ainda. Use Convocar Familiar ou peça ao mestre para dar uma.',
    );
    const button = el.querySelector<HTMLButtonElement>('.cast__btn')!;
    expect(flat(button)).toContain('Convocar Familiar');
    expect(button.classList).toContain('mat-mdc-outlined-button');
    expect(flat(el.querySelector('.cast__cost'))).toContain(
      'Ritual de 1 hora: não gasta espaço de magia. Só durante uma sessão, fora de combate.',
    );
    expect(el.querySelector('.count')).toBeNull();
  });

  it('a druid without a spell or a creature is told to ask the master, with no cast button', async () => {
    const { el, flat } = await setup({ wildShape: true });
    expect(flat(el.querySelector('.empty'))).toBe(
      'Nenhuma criatura ainda. Peça ao mestre para dar uma.',
    );
    // No summoning spell to cast; the druid's Wild Shape is its own line (E9-11), off outside a session.
    expect(el.querySelector('.js-cast')).toBeNull();
    expect(flat(el.querySelector('.js-wild'))).toContain('Transformar: Forma Selvagem');
  });

  it('outside a session the button is dashed and says why, and does not open the sheet', async () => {
    sessions.set([]);
    const { el, flat } = await setup({ access: FAMILIAR() });
    const button = el.querySelector<HTMLButtonElement>('.cast__btn')!;
    expect(button.classList).toContain('mr-button--off');
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(flat(el.querySelector('.cast__why'))).toBe('Agora não há sessão aberta.');
    button.click();
    expect(dialogOpen).not.toHaveBeenCalled();
  });

  it('lists a creature as a card: the name, the kind, the origin, CA, PV "1 de 1" and the speeds in metres', async () => {
    const { el, flat } = await setup({
      access: FAMILIAR(),
      creatures: [creature('cr-1', 'Nanquim')],
    });
    expect(flat(el.querySelector('.panel__count'))).toBe('1 criatura');
    const card = el.querySelector('app-creature-card')!;
    expect(flat(card.querySelector('h3'))).toBe('Nanquim');
    expect(flat(card.querySelector('.sub'))).toBe('Corvo · Miúdo · Familiar de Pensantus');
    const tiles = Array.from(card.querySelectorAll('.tile')).map((t) => flat(t));
    expect(tiles).toEqual(['CA 12', 'PV 1 de 1', 'Deslocamento 3 m voo 15 m']);
    const see = card.querySelector('a')!;
    expect(see.getAttribute('aria-label')).toBe('Ver a ficha de Nanquim');
    expect(see.getAttribute('href')).toBe('/campaigns/camp-1/characters/char-1/creatures/cr-1');
  });

  it('the player renames and dismisses any of their creatures; only the master corrects the hit points', async () => {
    const given = creature('cr-2', 'Mastim', {
      source: CreatureSource.MASTER,
      monsterKey: 'monster:mastiff',
    });
    const player = await setup({
      access: FAMILIAR(),
      creatures: [creature('cr-1', 'Nanquim'), given],
    });
    const cards = Array.from(player.el.querySelectorAll('app-creature-card'));
    // The character's player may dismiss any of their creatures, a gift included: the server says so.
    expect(cards[0].querySelector('.js-dismiss')).not.toBeNull();
    expect(cards[1].querySelector('.js-dismiss')).not.toBeNull();
    expect(cards[1].querySelector('.js-rename')).not.toBeNull();
    expect(player.el.querySelector('.js-hp')).toBeNull();

    TestBed.resetTestingModule();
    const master = await setup({ master: true, creatures: [given] });
    expect(master.el.querySelector('.js-dismiss')).not.toBeNull();
    expect(master.el.querySelector('.js-hp')).not.toBeNull();
    expect(master.el.querySelector('.cast__btn')).toBeNull();
  });

  it('a master looking at a character with no creature sees no panel', async () => {
    const { el } = await setup({ master: true, access: FAMILIAR() });
    expect(el.querySelector('section')).toBeNull();
  });

  it('opens the cast sheet for the spell (the sheet reads the rest from the server)', async () => {
    const { el } = await setup({ access: FAMILIAR(), creatures: [creature('cr-1', 'Nanquim')] });
    el.querySelector<HTMLButtonElement>('.cast__btn')!.click();
    expect(dialogOpen).toHaveBeenCalledTimes(1);
    const data = dialogOpen.mock.calls[0][1].data;
    expect(data).toMatchObject({
      campaignId: 'camp-1',
      characterId: 'char-1',
      spellKey: 'spell:find-familiar',
    });
  });

  it('after a cast, reads the list and says what arrived in a live region, with no slot spent', async () => {
    const { fixture, el, flat, settle } = await setup({ access: FAMILIAR() });
    dialogOpen.mockReturnValue({
      afterClosed: () =>
        of({
          spellName: 'Convocar Familiar',
          ritual: true,
          castingTime: '1 hora',
          names: ['Nanquim'],
          count: 1,
          dismissed: 0,
        }),
    });
    api.creatures = [creature('cr-1', 'Nanquim')];
    el.querySelector<HTMLButtonElement>('.cast__btn')!.click();
    await settle();
    fixture.detectChanges();
    expect(el.querySelector('.live')?.getAttribute('role')).toBe('status');
    expect(flat(el.querySelector('.live'))).toBe(
      'Nanquim chegou. Convocar Familiar, ritual de 1 hora. Nenhum espaço de magia foi gasto.',
    );
    expect(el.querySelectorAll('app-creature-card')).toHaveLength(1);
  });

  it('when the stream says the creatures changed and the master gave one, tells the player', async () => {
    const { fixture, el, flat, settle } = await setup({ access: FAMILIAR() });
    api.creatures = [
      creature('cr-2', 'Mastim', { source: CreatureSource.MASTER, monsterKey: 'monster:mastiff' }),
    ];
    fixture.componentRef.setInput('reload', 1);
    fixture.detectChanges();
    await settle();
    expect(flat(el.querySelector('.live'))).toBe('O mestre deu uma criatura a você: Mastim.');
    expect(flat(el.querySelector('.panel__count'))).toBe('1 criatura');
  });

  it('a failed read says so and offers to try again: never "Nenhuma criatura" for a list it could not read', async () => {
    const { el, flat, settle, fixture } = await setup({ access: FAMILIAR(), failing: true });
    expect(flat(el.querySelector('[role=alert]'))).toBe('Não deu para ler as criaturas agora.');
    expect(el.querySelector('.empty')).toBeNull();
    api.failWith = null;
    api.creatures = [creature('cr-1', 'Nanquim')];
    el.querySelector<HTMLButtonElement>('.retry')!.click();
    await settle();
    fixture.detectChanges();
    expect(el.querySelectorAll('app-creature-card')).toHaveLength(1);
  });

  it('a read that fails after the list was shown keeps the list and says it could not update, with a way to try again', async () => {
    const { fixture, el, flat, settle } = await setup({
      access: FAMILIAR(),
      creatures: [creature('cr-1', 'Nanquim')],
    });
    api.failWith = new ConnectError('down', Code.Unavailable);
    fixture.componentRef.setInput('reload', 1);
    fixture.detectChanges();
    await settle();
    expect(el.querySelectorAll('app-creature-card')).toHaveLength(1);
    expect(flat(el.querySelector('[role=alert]'))).toBe(
      'Não deu para atualizar as criaturas agora.',
    );
    api.failWith = null;
    api.creatures = [creature('cr-1', 'Nanquim'), creature('cr-2', 'Pena')];
    el.querySelector<HTMLButtonElement>('.retry')!.click();
    await settle();
    expect(el.querySelectorAll('app-creature-card')).toHaveLength(2);
    expect(el.querySelector('[role=alert]')).toBeNull();
  });

  it('a read that works after the list was shown says nothing about a failure (positive control)', async () => {
    const { fixture, el, settle } = await setup({
      access: FAMILIAR(),
      creatures: [creature('cr-1', 'Nanquim')],
    });
    fixture.componentRef.setInput('reload', 1);
    fixture.detectChanges();
    await settle();
    expect(el.querySelector('[role=alert]')).toBeNull();
  });

  it('a creature the master gives while a cast sheet is open is told too, after the cast is', async () => {
    const { fixture, el, flat, settle } = await setup({ access: FAMILIAR() });
    const closed = new Subject<unknown>();
    dialogOpen.mockReturnValue({ afterClosed: () => closed });
    el.querySelector<HTMLButtonElement>('.cast__btn')!.click();
    api.creatures = [
      creature('cr-2', 'Mastim', { source: CreatureSource.MASTER, monsterKey: 'monster:mastiff' }),
    ];
    fixture.componentRef.setInput('reload', 1);
    fixture.detectChanges();
    await settle();
    api.creatures = [...api.creatures, creature('cr-1', 'Nanquim')];
    closed.next({
      spellName: 'Convocar Familiar',
      ritual: true,
      castingTime: '1 hora',
      names: ['Nanquim'],
      count: 1,
      dismissed: 0,
    });
    closed.complete();
    await settle();
    expect(flat(el.querySelector('.live'))).toContain('Nanquim chegou.');
    expect(flat(el.querySelector('.live'))).toContain('O mestre deu uma criatura a você: Mastim.');
  });

  it('a creature the master gives while a cast sheet is open is told when the sheet is closed without casting', async () => {
    const { fixture, el, flat, settle } = await setup({ access: FAMILIAR() });
    const closed = new Subject<unknown>();
    dialogOpen.mockReturnValue({ afterClosed: () => closed });
    el.querySelector<HTMLButtonElement>('.cast__btn')!.click();
    api.creatures = [
      creature('cr-2', 'Mastim', { source: CreatureSource.MASTER, monsterKey: 'monster:mastiff' }),
    ];
    fixture.componentRef.setInput('reload', 1);
    fixture.detectChanges();
    await settle();
    closed.next(undefined);
    closed.complete();
    await settle();
    expect(flat(el.querySelector('.live'))).toBe('O mestre deu uma criatura a você: Mastim.');
  });

  it('the answer of an older read never replaces a newer one', async () => {
    const { fixture, el, settle } = await setup({ access: FAMILIAR() });
    let release!: () => void;
    const slow = new Promise<void>((r) => (release = r));
    api.list.mockImplementationOnce(async () => {
      await slow;
      return [creature('old', 'Antiga')];
    });
    fixture.componentRef.setInput('reload', 1);
    fixture.detectChanges();
    api.creatures = [creature('new', 'Nova')];
    fixture.componentRef.setInput('reload', 2);
    fixture.detectChanges();
    await settle();
    release();
    await settle();
    expect(
      Array.from(el.querySelectorAll('app-creature-card h3')).map((h) => h.textContent?.trim()),
    ).toEqual(['Nova']);
  });

  it('the notice goes away when the next action starts', async () => {
    const { fixture, el, flat, settle } = await setup({ access: FAMILIAR() });
    api.creatures = [
      creature('cr-2', 'Mastim', { source: CreatureSource.MASTER, monsterKey: 'monster:mastiff' }),
    ];
    fixture.componentRef.setInput('reload', 1);
    fixture.detectChanges();
    await settle();
    expect(flat(el.querySelector('.live'))).toContain('Mastim');
    el.querySelector<HTMLElement>('.js-rename')!.click();
    await settle();
    expect(flat(el.querySelector('.live'))).toBe('');
  });

  it('after a dismissal the focus goes to the next card, or to the title when none is left', async () => {
    const { fixture, el, settle } = await setup({
      access: FAMILIAR(),
      creatures: [
        creature('cr-1', 'Nanquim'),
        creature('cr-2', 'Pena', { monsterKey: 'monster:owl' }),
      ],
    });
    document.body.appendChild(fixture.nativeElement);
    el.querySelector<HTMLElement>('.js-dismiss')!.click();
    await settle();
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => flat(b)?.includes('Dispensar Nanquim'))!
      .click();
    await settle();
    await settle();
    expect(document.activeElement?.textContent?.trim()).toBe('Pena');
    el.querySelector<HTMLElement>('.js-dismiss')!.click();
    await settle();
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => flat(b)?.includes('Dispensar Pena'))!
      .click();
    await settle();
    await settle();
    expect(document.activeElement?.textContent?.trim()).toBe('Criaturas');
  });
});

const tick = () => new Promise((r) => setTimeout(r));

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe('CreaturesPanel, a druid in Wild Shape: the reads of her form', () => {
  const sessions = signal<readonly { campaignId: string }[]>([{ campaignId: 'camp-1' }]);

  function vitals(beast: string) {
    return create(CharacterVitalsSchema, {
      characterId: 'char-1',
      ...(beast ? { wildShape: { beastNamePt: beast } } : {}),
    });
  }

  function build(api: FakeCreaturesClient) {
    api.options = summonAnswer([]);
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: CreaturesClient, useValue: api },
        { provide: OpenSessions, useValue: { sessions } },
        { provide: MatDialog, useValue: { open: vi.fn() } },
        { provide: MatBottomSheet, useValue: { open: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(CreaturesPanel);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('characterId', 'char-1');
    fixture.componentRef.setInput('characterName', 'Pensantus');
    fixture.componentRef.setInput('wildShape', true);
    return fixture;
  }

  const settle = async (f: { detectChanges(): void; whenStable(): Promise<unknown> }) => {
    for (let i = 0; i < 4; i++) {
      await f.whenStable();
      await tick();
      f.detectChanges();
    }
  };

  it('keeps the form of the newest vitals read when an older one lands after it', async () => {
    const api = new FakeCreaturesClient();
    const reads = [deferred<unknown>(), deferred<unknown>()];
    let n = 0;
    api.vitalsOf = vi.fn(() => reads[n++].promise) as never;
    const fixture = build(api);
    fixture.detectChanges();
    await settle(fixture); // read #1 (older) is pending
    fixture.componentRef.setInput('reload', 1); // creatures_changed -> read #2 (newer)
    fixture.detectChanges();
    await settle(fixture);
    expect(api.vitalsOf).toHaveBeenCalledTimes(2);

    reads[1].resolve(vitals('Lobo')); // newer: she is a wolf
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(flat(el.querySelector('.js-wild'))).toContain('Voltar à forma normal');
    reads[0].resolve(vitals('')); // older, stale: own shape
    await settle(fixture);
    expect(flat(el.querySelector('.js-wild'))).toContain('Voltar à forma normal');
  });

  it('reads the form again when the page says the vitals changed, so a form that ended elsewhere leaves the panel', async () => {
    const api = new FakeCreaturesClient();
    api.vitals = vitals('Lobo');
    const fixture = build(api);
    fixture.detectChanges();
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(flat(el.querySelector('.js-wild'))).toContain('Voltar à forma normal');

    // The form ended in the master's hand: no creature changed, only the vitals did.
    api.vitals = vitals('');
    fixture.componentRef.setInput('formReload', 1);
    fixture.detectChanges();
    await settle(fixture);
    expect(flat(el.querySelector('.js-wild'))).toContain('Transformar');
    expect(flat(el.querySelector('.js-wild'))).not.toContain('Voltar à forma normal');
  });

  it('sends one request when "Voltar à forma normal" is tapped twice, and shows no refusal', async () => {
    const api = new FakeCreaturesClient();
    api.vitals = vitals('Lobo');
    const fixture = build(api);
    fixture.detectChanges();
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    const button = el.querySelector<HTMLButtonElement>('.js-wild')!;
    expect(flat(button)).toContain('Voltar à forma normal');

    // The server ends the form on the first call; any other call finds the druid in her own shape.
    const first = deferred<{ vitals: undefined; encounter: undefined }>();
    const keys: string[] = [];
    api.leaveWildShape = vi.fn((async (_c: string, _ch: string, key: string) => {
      keys.push(key);
      if (keys.length === 1) {
        return first.promise;
      }
      throw new Error('NOT_IN_WILD_SHAPE');
    }) as never);
    api.vitals = vitals('');

    button.click();
    await tick();
    fixture.detectChanges();
    button.click();
    await tick();
    fixture.detectChanges();
    first.resolve({ vitals: undefined, encounter: undefined });
    await settle(fixture);

    expect(api.leaveWildShape).toHaveBeenCalledTimes(1);
    expect(el.querySelector('[role="alert"]')).toBeNull();
  });
});
