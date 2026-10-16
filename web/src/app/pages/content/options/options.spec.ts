import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { type MessageInitShape, create } from '@bufbuild/protobuf';
import { BehaviorSubject } from 'rxjs';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type OptionSwitchEntry,
  OptionSwitchEntrySchema,
  TableContentKind,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { TableContentClient } from '../../../core/content/content-client';
import { SpellsClient } from '../../../core/spells/spells-client';
import { fakeContentWatcher } from '../../../core/content/content-testing';
import { ContentOptions } from './options';
import { settle } from '../../../../test-settle';

const opt = (
  kind: TableContentKind,
  namePt: string,
  over: MessageInitShape<typeof OptionSwitchEntrySchema> = {},
) =>
  create(OptionSwitchEntrySchema, {
    key: `${TableContentKind[kind].toLowerCase()}:${namePt.toLowerCase().replace(/ /g, '-')}`,
    kind,
    namePt,
    ...over,
  });

const list = (): OptionSwitchEntry[] => [
  opt(TableContentKind.CLASS, 'Mago', { key: 'class:wizard', charactersUsing: 1 }),
  opt(TableContentKind.CLASS, 'Guardião do Vale', { key: 'class:guardiao@mesa', table: true }),
  opt(TableContentKind.SUBCLASS, 'Evocação', {
    key: 'subclass:evocation',
    parentKey: 'class:wizard',
  }),
  opt(TableContentKind.RACE, 'Anão', { key: 'race:dwarf' }),
  opt(TableContentKind.RACE, 'Corujeiro', {
    key: 'race:corujeiro@mesa',
    table: true,
    charactersUsing: 2,
  }),
  opt(TableContentKind.RACE, 'Gnomo', { key: 'race:gnome', charactersUsing: 1 }),
  opt(TableContentKind.RACE, 'Tiefling', { key: 'race:tiefling', off: true, hidden: true }),
  opt(TableContentKind.SUBRACE, 'Anão da Colina', {
    key: 'subrace:hill-dwarf',
    parentKey: 'race:dwarf',
  }),
  opt(TableContentKind.BACKGROUND, 'Acólito', { key: 'background:acolyte' }),
  opt(TableContentKind.SPELL, 'Luz', { key: 'spell:light' }),
];

describe('ContentOptions, "Opções para os jogadores" (MR-025, RN-23, E10-01 state 3)', () => {
  const originalMatchMedia = window.matchMedia;
  afterEach(() => {
    window.matchMedia = originalMatchMedia;
  });

  const switches = vi.fn();
  const setSwitches = vi.fn();
  const getCampaign = vi.fn();
  const spellsList = vi.fn();
  let watcher = fakeContentWatcher();
  let query$ = new BehaviorSubject(convertToParamMap({}));

  async function setup(
    role: Role = Role.MASTER,
    opts: {
      phone?: boolean;
      kind?: string;
      options?: OptionSwitchEntry[];
      stranger?: boolean;
    } = {},
  ) {
    switches.mockReset().mockResolvedValue({ options: opts.options ?? list(), tableRevision: 4 });
    spellsList.mockReset().mockResolvedValue({ spells: [{ key: 'spell:light' }] });
    setSwitches.mockReset().mockResolvedValue({ tableRevision: 5, changed: 1, options: [] });
    getCampaign.mockReset().mockResolvedValue({
      campaign: opts.stranger
        ? undefined
        : { id: 'camp-1', name: 'Mirathel', myRole: role, awaitingApproval: false },
    });
    query$ = new BehaviorSubject(convertToParamMap(opts.kind ? { kind: opts.kind } : {}));
    window.matchMedia = ((q: string) => ({
      matches: !!opts.phone && q.includes('max-width'),
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    })) as never;
    TestBed.resetTestingModule();
    watcher = fakeContentWatcher();
    TestBed.overrideComponent(ContentOptions, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: new BehaviorSubject(convertToParamMap({ id: 'camp-1' })),
            queryParamMap: query$,
          },
        },
        { provide: CampaignsService, useValue: { getCampaign } },
        { provide: TableContentClient, useValue: { switches, setSwitches } },
        { provide: SpellsClient, useValue: { list: spellsList } },
      ],
    });
    const fixture = TestBed.createComponent(ContentOptions);
    await settle(fixture, 4);
    return { fixture, el: fixture.nativeElement as HTMLElement, settle: () => settle(fixture, 4) };
  }

  // The text a person reads: the icons' ligature names ("check") are not words.
  const text = (el: Element) => {
    const copy = el.cloneNode(true) as Element;
    copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
    return (copy.textContent ?? '')
      .replace(/\u00a0/g, ' ')
      .replace(/\s+/g, ' ')
      .trim();
  };
  const rows = (el: HTMLElement) => Array.from(el.querySelectorAll('.orow'));
  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      text(b).startsWith(label),
    )!;
  /** The row of the option with exactly this name (a sub-race's name starts with its race's). */
  const rowOf = (el: HTMLElement, name: string) =>
    rows(el).find((r) => text(r.querySelector('.label')!) === name)!;
  const sw = (el: HTMLElement, name: string) =>
    rowOf(el, name).querySelector<HTMLButtonElement>('[role="switch"]')!;

  it('opens on the classes with the menu of kinds and "9 de 10"-style counts, and the group\'s counter', async () => {
    const { el } = await setup();
    const items = Array.from(el.querySelectorAll('.menu__item')).map((a) => text(a));
    expect(items).toEqual([
      'Classes2 de 2',
      'Subclasses1 de 1',
      'Raças3 de 4',
      'Antecedentes1 de 1',
      'Magias1 de 1',
      'Talentos0 de 0',
    ]);
    expect(text(el.querySelector('h1')!)).toBe('Opções para os jogadores');
    expect(text(el.querySelector('.counter strong')!)).toBe('Classes: 2 de 2 ligadas');
    expect(rows(el).map((r) => text(r.querySelector('app-switch-field')!))).toEqual([
      'MagoLigada',
      'Guardião do ValeLigada',
    ]);
  });

  it('draws a race group like the artboard: the word beside the switch, "N ficha usa", "SRD" or "Da mesa", the off one flagged', async () => {
    const { el } = await setup(Role.MASTER, { kind: 'races' });
    expect(text(el.querySelector('.counter strong')!)).toBe('Raças: 3 de 4 ligadas');
    expect(text(el.querySelector('.menu__note')!)).toContain('Sub-raças: 1 de 1 ligada');
    const byName = (n: string) => text(rowOf(el, n));
    expect(byName('Anão')).toContain('Ligada');
    // Nothing is said of an option no sheet uses; an off one says that turning it off touches no sheet.
    expect(byName('Anão')).not.toContain('Nenhuma ficha usa');
    expect(byName('Tiefling')).toContain('Nenhuma ficha usa');
    expect(byName('Anão')).toContain('SRD');
    expect(byName('Gnomo')).toContain('1 ficha usa');
    expect(byName('Corujeiro')).toContain('2 fichas usam');
    expect(byName('Corujeiro')).toContain('Da mesa');
    expect(byName('Tiefling')).toContain('Desligada');
    expect(sw(el, 'Tiefling').getAttribute('aria-checked')).toBe('false');
    expect(sw(el, 'Anão').getAttribute('aria-checked')).toBe('true');
    // The sub-race sits under its race, nested, and says which race it belongs to.
    expect(text(rows(el)[1])).toContain('Anão da Colina');
    expect(text(rows(el)[1])).toContain('Sub-raça de Anão');
    expect(rows(el)[1].classList).toContain('orow--nested');
    expect(rows(el)[0].classList).not.toContain('orow--nested');
  });

  it('turns an option off at once with one request, and says that the sheet that uses it keeps working', async () => {
    const { el, settle } = await setup(Role.MASTER, { kind: 'races' });
    setSwitches.mockResolvedValue({
      tableRevision: 5,
      changed: 1,
      options: [{ ...list()[5], off: true, hidden: true }],
    });
    sw(el, 'Gnomo').click();
    await settle();
    expect(setSwitches).toHaveBeenCalledWith('camp-1', [{ key: 'race:gnome', off: true }]);
    expect(sw(el, 'Gnomo').getAttribute('aria-checked')).toBe('false');
    expect(text(el.querySelector('.counter strong')!)).toBe('Raças: 2 de 4 ligadas');
    expect(text(el.querySelector('.saved')!)).toContain('Tudo salvo');
    expect(text(rowOf(el, 'Gnomo'))).toContain('A ficha que a usa continua funcionando.');
    expect(text(el.querySelector('[role="status"].mr-visually-hidden')!)).toBe(
      'Gnomo: desligada para os jogadores. 1 ficha usa e continua funcionando.',
    );
  });

  it('turns "Ligar todas" off when it would change nothing, and "Desligar todas" too', async () => {
    const { el, settle } = await setup(Role.MASTER, { kind: 'classes' });
    expect(button(el, 'Ligar todas').disabled).toBe(true);
    expect(button(el, 'Desligar todas').disabled).toBe(false);
    button(el, 'Desligar todas').click();
    await settle();
    expect(button(el, 'Desligar todas').disabled).toBe(true);
    expect(button(el, 'Ligar todas').disabled).toBe(false);
  });

  it('says that "Desligar todas" in Raças also takes the sub-races', async () => {
    const { el } = await setup(Role.MASTER, { kind: 'races' });
    expect(text(el.querySelector('.bulk-note')!)).toContain('também desliga as sub-raças (1)');
  });

  it('agrees the bulk words with the noun: "Ligar todos" for the backgrounds', async () => {
    const { el } = await setup(Role.MASTER, { kind: 'backgrounds' });
    expect(text(el.querySelector('.counter strong')!)).toBe('Antecedentes: 1 de 1 ligado');
    expect(button(el, 'Desligar todos')).toBeTruthy();
  });

  it('gives the spells the circle in the support line and the filters by class and circle', async () => {
    const options = [
      opt(TableContentKind.CLASS, 'Mago', { key: 'class:wizard' }),
      opt(TableContentKind.SPELL, 'Luz', { key: 'spell:light', level: 0 }),
      opt(TableContentKind.SPELL, 'Mísseis Mágicos', { key: 'spell:magic-missile', level: 1 }),
      opt(TableContentKind.SPELL, 'Bola de Fogo', { key: 'spell:fireball', level: 3 }),
    ];
    const { el, settle } = await setup(Role.MASTER, { kind: 'spells', options });
    expect(rows(el).map((r) => text(r.querySelector('.orow__note')!))).toEqual([
      'Truque',
      '1º nível',
      '3º nível',
    ]);
    const selects = Array.from(el.querySelectorAll<HTMLSelectElement>('app-select-field select'));
    expect(selects).toHaveLength(2);
    // The circle: only the 3rd.
    selects[1].value = '3';
    selects[1].dispatchEvent(new Event('change'));
    await settle();
    expect(rows(el).map((r) => text(r.querySelector('.label')!))).toEqual(['Bola de Fogo']);
    expect(text(button(el, 'Desligar todas'))).toBe('Desligar todas (1)');
    selects[1].value = '';
    selects[1].dispatchEvent(new Event('change'));
    // The class: the spells of its list, as the server serves them.
    selects[0].value = 'class:wizard';
    selects[0].dispatchEvent(new Event('change'));
    await settle();
    expect(spellsList).toHaveBeenCalledWith({
      campaignId: 'camp-1',
      classKey: 'class:wizard',
      pageSize: 400,
    });
    expect(rows(el).map((r) => text(r.querySelector('.label')!))).toEqual(['Luz']);
  });

  it('keeps an error under the counter until it is closed, and a new change does not hide it', async () => {
    const { el, settle } = await setup(Role.MASTER, { kind: 'races' });
    setSwitches.mockRejectedValueOnce(new Error('offline'));
    sw(el, 'Anão').click();
    await settle();
    setSwitches.mockResolvedValue({ tableRevision: 6, changed: 1, options: [] });
    sw(el, 'Gnomo').click();
    await settle();
    expect(text(el.querySelector('.saved')!)).toContain('Não salvou');
    button(el, 'Fechar o aviso').click();
    await settle();
    expect(text(el.querySelector('.saved')!)).toContain('Tudo salvo');
  });

  it('shows "Tudo salvo" at rest', async () => {
    const { el } = await setup(Role.MASTER, { kind: 'races' });
    expect(text(el.querySelector('.saved')!)).toContain('Tudo salvo');
  });

  it('puts the switch back and says it when the save fails', async () => {
    const { el, settle } = await setup(Role.MASTER, { kind: 'races' });
    setSwitches.mockRejectedValue(new Error('offline'));
    sw(el, 'Anão').click();
    await settle();
    expect(sw(el, 'Anão').getAttribute('aria-checked')).toBe('true');
    expect(text(el.querySelector('.saved')!)).toContain('Não salvou');
    expect(text(el.querySelector('.list [role="alert"]')!)).toContain(
      'O que você mudou voltou ao que era.',
    );
  });

  it('"Desligar todas" and "Ligar todas" send one call each with what changes', async () => {
    const { el, settle } = await setup(Role.MASTER, { kind: 'races' });
    button(el, 'Desligar todas').click();
    await settle();
    expect(setSwitches).toHaveBeenLastCalledWith('camp-1', [
      { key: 'race:dwarf', off: true },
      { key: 'race:corujeiro@mesa', off: true },
      { key: 'race:gnome', off: true },
      { key: 'subrace:hill-dwarf', off: true },
    ]);
    expect(text(el.querySelector('.counter strong')!)).toBe('Raças: 0 de 4 ligadas');
    button(el, 'Ligar todas').click();
    await settle();
    expect(setSwitches).toHaveBeenCalledTimes(2);
    expect(setSwitches.mock.calls[1][1]).toHaveLength(5);
    expect(text(el.querySelector('.counter strong')!)).toBe('Raças: 4 de 4 ligadas');
  });

  it('searches by name, and the bulk buttons act on what the search shows, saying how many', async () => {
    const { el, settle } = await setup(Role.MASTER, { kind: 'races' });
    const input = el.querySelector<HTMLInputElement>('input')!;
    input.value = 'anao';
    input.dispatchEvent(new Event('input'));
    await settle();
    expect(rows(el).map((r) => text(r))).toEqual([
      expect.stringContaining('AnãoLigada'),
      expect.stringContaining('Anão da ColinaLigada'),
    ]);
    expect(text(button(el, 'Desligar todas'))).toBe('Desligar todas (2)');
    button(el, 'Desligar todas').click();
    await settle();
    expect(setSwitches).toHaveBeenLastCalledWith('camp-1', [
      { key: 'race:dwarf', off: true },
      { key: 'subrace:hill-dwarf', off: true },
    ]);
    input.value = 'zzz';
    input.dispatchEvent(new Event('input'));
    await settle();
    expect(
      text(el.querySelector('.list [role="status"]:not(.saved):not(.mr-visually-hidden)')!),
    ).toBe('Nenhuma opção com esta busca.');
    expect(button(el, 'Ligar todas').disabled).toBe(true);
  });

  it('says why a subclass of an off class is hidden, and keeps its own switch on (the parent hides the children)', async () => {
    const options = list().map((o) =>
      o.key === 'class:wizard'
        ? ({ ...o, off: true, hidden: true } as OptionSwitchEntry)
        : o.key === 'subclass:evocation'
          ? ({ ...o, hidden: true } as OptionSwitchEntry)
          : o,
    );
    const { el } = await setup(Role.MASTER, { kind: 'subclasses', options });
    const row = rows(el)[0];
    expect(text(row)).toContain('Subclasse de Mago');
    expect(text(row)).toContain('Some para os jogadores: a classe Mago está desligada.');
    expect(text(row)).toContain('Escondida');
    expect(row.classList).toContain('orow--hidden');
    // The group's count says it too: the subclass is on, and the players still do not get it.
    expect(text(el.querySelector('.counter strong')!)).toBe(
      'Subclasses: 1 de 1 ligada · 1 escondida pela classe',
    );
    expect(row.querySelector('[role="switch"]')!.getAttribute('aria-checked')).toBe('true');
  });

  it('on a phone, a select of kinds replaces the menu, and the switches are still one per row', async () => {
    const { el } = await setup(Role.MASTER, { phone: true, kind: 'races' });
    expect(el.querySelector('.menu')).toBeNull();
    expect(el.querySelector('app-select-field')).not.toBeNull();
    expect(rows(el)).toHaveLength(5);
  });

  it('tells a player that only the master chooses, without asking for the list', async () => {
    const { el } = await setup(Role.PLAYER);
    expect(text(el)).toContain('Só o mestre escolhe o que os jogadores veem.');
    expect(switches).not.toHaveBeenCalled();
  });

  it("says so when the campaign is not the person's, without asking for the list", async () => {
    const { el } = await setup(Role.MASTER, { stranger: true });
    expect(text(el)).toContain('Essa campanha não existe, ou você não é membro dela.');
    expect(switches).not.toHaveBeenCalled();
  });

  it('reads the list again when the table changed (content_changed), without a spinner and keeping the search', async () => {
    const { el, settle } = await setup(Role.MASTER, { kind: 'races' });
    expect(watcher.following()).toBe('camp-1');
    switches.mockResolvedValue({
      options: [...list(), opt(TableContentKind.RACE, 'Elfo', { key: 'race:elf' })],
      tableRevision: 6,
    });
    watcher.hint();
    await settle();
    expect(rows(el)).toHaveLength(6);
    expect(text(el.querySelector('.counter strong')!)).toBe('Raças: 4 de 5 ligadas');
    expect(el.querySelector('mat-spinner')).toBeNull();
  });

  it('keeps the list on screen when the read after a change fails', async () => {
    const { el, settle } = await setup(Role.MASTER, { kind: 'races' });
    switches.mockRejectedValue(new Error('offline'));
    watcher.hint();
    await settle();
    expect(rows(el)).toHaveLength(5);
    expect(text(el)).not.toContain('Tentar de novo');
  });
});
