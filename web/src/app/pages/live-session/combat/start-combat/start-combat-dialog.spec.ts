import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterKind } from '../../../../../gen/meurpg/characters/v1/characters_pb';
import { EncounterMode } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CampaignsService } from '../../../../core/campaigns/campaigns.service';
import { TableRulesClient } from '../../../../core/campaigns/table-rules';
import { CombatClient } from '../../../../core/combat/combat-client';
import { encounter } from '../../../../core/combat/combat-testing';
import { CONNECT_TRANSPORT } from '../../../../core/connect/transport';
import { flat } from '../../../../core/creatures/creatures-testing';
import { RosterClient } from '../../../../core/maps/roster-client';
import { type CombatMapInfo, StartCombatDialog, type StartCombatData } from './start-combat-dialog';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

const map: CombatMapInfo = {
  id: 'map',
  name: 'Emboscada na estrada',
  image: { url: '/i', width: 2000, height: 1400 },
  columns: 20,
  rows: 14,
};
const roster = [
  {
    id: 'p',
    name: 'Pensantus',
    kind: CharacterKind.PLAYER,
    playerUserId: 'u1',
    classSummary: 'Mago 4',
    raceName: 'Gnomo',
    playerName: 'Vinicius',
  },
  {
    id: 'g',
    name: 'Goblin',
    kind: CharacterKind.MINION,
    playerUserId: '',
    classSummary: '',
    raceName: '',
    playerName: null,
  },
];

/** Lets the dialog's reads (the roster, the members, the table's rule) finish, then draws. */
async function settle(fixture: {
  detectChanges: () => void;
  whenStable: () => Promise<unknown>;
}): Promise<void> {
  await fixture.whenStable();
  await new Promise((r) => setTimeout(r, 0));
  await fixture.whenStable();
  fixture.detectChanges();
}

/** The dialog with the table's "combate com mapa" rule `withMap`, over a session with or without a map. */
async function setup(opts: { withMap: boolean | 'fails'; map?: CombatMapInfo | null }) {
  const data: StartCombatData = {
    campaignId: 'c',
    mode: 'start',
    map: opts.map === undefined ? map : opts.map,
  };
  const started: { mode: EncounterMode | undefined }[] = [];
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: { close: () => undefined } },
      { provide: RosterClient, useValue: { list: async () => roster } },
      {
        provide: CampaignsService,
        useValue: {
          listMembers: async () => ({ members: [] }),
          getCampaign: async () => ({
            campaign: { diceMode: DiceMode.PLAYERS_CHOOSE, dicePreference: DicePreference.APP },
          }),
        },
      },
      {
        provide: TableRulesClient,
        useValue: {
          get: async () => {
            if (opts.withMap === 'fails') {
              throw new Error('down');
            }
            return { saved: { combatStartsWithMap: opts.withMap } };
          },
        },
      },
      {
        provide: CombatClient,
        useValue: {
          start: async (
            _c: string,
            _n: string,
            _s: unknown,
            _k: string,
            extras?: { mode?: EncounterMode },
          ) => {
            started.push({ mode: extras?.mode });
            return encounter();
          },
        },
      },
    ],
  });
  const fixture = TestBed.createComponent(StartCombatDialog);
  fixture.detectChanges();
  await settle(fixture);
  return { fixture, el: fixture.nativeElement as HTMLElement, started };
}

const radios = (el: HTMLElement) =>
  Array.from(el.querySelectorAll<HTMLInputElement>('input[name="combat-mode"]'));

describe('"Iniciar combate": how the combat is played (RN-25, E10-04 state 1)', () => {
  it('offers "Com mapa" and "Sem mapa (teatro da mente)", with the table\'s rule as the default: with a map', async () => {
    const { el } = await setup({ withMap: true });
    const text = plain(el.textContent);
    expect(text).toContain('Como este combate é jogado');
    expect(text).toContain('Com mapa');
    expect(text).toContain('Sem mapa (teatro da mente)');
    expect(radios(el).map((r) => r.checked)).toEqual([true, false]);
    // With a map the dialog is today's: the map and "Mudar a grade", and the line that says why is not there.
    expect(text).toContain('Mapa do combate');
    expect(text).toContain('Mudar a grade');
    expect(el.querySelector('[data-testid="theatre-why"]')).toBeNull();
  });

  it('starts on "Sem mapa" when the table\'s rule is off, says why once and leaves the map out', async () => {
    const { el } = await setup({ withMap: false });
    expect(radios(el).map((r) => r.checked)).toEqual([false, true]);
    const why = plain(el.querySelector('[data-testid="theatre-why"]')?.textContent);
    expect(why).toContain('Sem mapa, o app não sabe onde ninguém está.');
    expect(why).toContain('o modo vale até o fim do combate');
    expect(plain(el.textContent)).not.toContain('Mapa do combate');
  });

  it('keeps "Com mapa" when the rule cannot be read', async () => {
    const { el } = await setup({ withMap: 'fails' });
    expect(radios(el).map((r) => r.checked)).toEqual([true, false]);
  });

  it('waits "Com mapa" (dashed, with the reason) and starts on "Sem mapa" when the session has no map', async () => {
    const { el } = await setup({ withMap: true, map: null });
    const [grid, theatre] = radios(el);
    expect(grid.disabled).toBe(true);
    expect(theatre.checked).toBe(true);
    expect(plain(el.textContent)).toContain('A sessão não tem um mapa atual.');
  });

  it('lets the master choose, and the choice is the one sent', async () => {
    const { fixture, el, started } = await setup({ withMap: false });
    radios(el)[0].click();
    fixture.detectChanges();
    expect(plain(el.textContent)).toContain('Mapa do combate');
    radios(el)[1].click();
    fixture.detectChanges();
    el.querySelector<HTMLButtonElement>('.dlg__go')!.click();
    await settle(fixture);
    expect(started).toEqual([{ mode: EncounterMode.THEATRE }]);
  });

  it('counts a map without a grid as no map: "Com mapa" waits with its reason, "Sem mapa" is chosen, and the way to give it a grid stays', async () => {
    const noGrid: CombatMapInfo = { ...map, columns: 0, rows: 0 };
    const { fixture, el, started } = await setup({ withMap: true, map: noGrid });
    const [grid, theatre] = radios(el);
    expect(grid.disabled).toBe(true);
    expect(theatre.checked).toBe(true);
    expect(plain(el.textContent)).toContain('O mapa atual não tem grade.');
    expect(plain(el.textContent)).toContain('Esse mapa ainda não tem grade.');
    expect(plain(el.textContent)).toContain('Definir a grade');
    expect(el.querySelector<HTMLButtonElement>('.dlg__go')!.getAttribute('aria-disabled')).not.toBe(
      'true',
    );
    el.querySelector<HTMLButtonElement>('.dlg__go')!.click();
    await settle(fixture);
    expect(started).toEqual([{ mode: EncounterMode.THEATRE }]);
  });

  it("is not ready until the table's rule is read, so the radio never flips under the master's hand", async () => {
    let release: (v: { saved: { combatStartsWithMap: boolean } }) => void = () => undefined;
    const data: StartCombatData = { campaignId: 'c', mode: 'start', map };
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: () => undefined } },
        { provide: RosterClient, useValue: { list: async () => roster } },
        {
          provide: CampaignsService,
          useValue: {
            listMembers: async () => ({ members: [] }),
            getCampaign: async () => ({ campaign: { diceMode: DiceMode.PLAYERS_CHOOSE } }),
          },
        },
        { provide: TableRulesClient, useValue: { get: () => new Promise((r) => (release = r)) } },
        { provide: CombatClient, useValue: {} },
      ],
    });
    const fixture = TestBed.createComponent(StartCombatDialog);
    fixture.detectChanges();
    await Promise.resolve();
    await new Promise((r) => setTimeout(r, 0));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(plain(el.textContent)).toContain('Carregando');
    expect(el.querySelector<HTMLButtonElement>('.dlg__go')!.getAttribute('aria-disabled')).toBe(
      'true',
    );
    release({ saved: { combatStartsWithMap: false } });
    await settle(fixture);
    expect(radios(el).map((r) => r.checked)).toEqual([false, true]);
    expect(plain(el.textContent)).not.toContain('Carregando');
  });
});

describe('"Adicionar combatente": the key of one addition', () => {
  async function adding() {
    const sent: { idempotencyKey: string }[] = [];
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        CombatClient,
        { provide: CONNECT_TRANSPORT, useValue: {} },
        {
          provide: MAT_DIALOG_DATA,
          useValue: {
            campaignId: 'camp-1',
            mode: 'add',
            map: null,
            encounterId: 'enc-1',
            existing: 2,
          },
        },
        { provide: MatDialogRef, useValue: { close: () => undefined } },
        { provide: RosterClient, useValue: { list: async () => roster } },
        {
          provide: CampaignsService,
          useValue: {
            listMembers: async () => ({ members: [] }),
            getCampaign: async () => ({
              campaign: { diceMode: DiceMode.PLAYERS_CHOOSE, dicePreference: DicePreference.APP },
            }),
          },
        },
        {
          provide: TableRulesClient,
          useValue: { get: async () => ({ saved: { combatStartsWithMap: false } }) },
        },
      ],
    });
    (TestBed.inject(CombatClient) as unknown as { client: unknown }).client = {
      addCombatants: async (req: { idempotencyKey: string }) => {
        sent.push(req);
        if (sent.length === 1) {
          throw new ConnectError('timeout', Code.Unavailable); // committed, but the answer was lost
        }
        return { encounter: encounter({ id: 'enc-1' }) };
      },
    };
    const fixture = TestBed.createComponent(StartCombatDialog);
    fixture.detectChanges();
    await settle(fixture);
    const cmp = fixture.componentInstance as unknown as { setCount(id: string, n: number): void };
    const press = async () => {
      Array.from(
        (fixture.nativeElement as HTMLElement).querySelectorAll<HTMLButtonElement>('button'),
      )
        .find((b) => /Adicionar/.test(flat(b) ?? ''))!
        .click();
      await settle(fixture);
    };
    return { fixture, cmp, press, sent, settle: () => settle(fixture) };
  }

  it('sends the same key when "Adicionar" is pressed again after a lost answer', async () => {
    const { cmp, press, sent, settle: again } = await adding();
    cmp.setCount('g', 3);
    await again();
    await press();
    await press();
    expect(sent).toHaveLength(2);
    expect(sent[1].idempotencyKey).toBe(sent[0].idempotencyKey);
  });

  it('sends another key when the NPCs to add changed', async () => {
    const { cmp, press, sent, settle: again } = await adding();
    cmp.setCount('g', 3);
    await again();
    await press();
    cmp.setCount('g', 4);
    await again();
    await press();
    expect(sent).toHaveLength(2);
    expect(sent[1].idempotencyKey).not.toBe(sent[0].idempotencyKey);
  });
});
