import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
  EncounterStatus,
  type Encounter,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  GameSessionBlockedReason,
  GameSessionBlockedSchema,
} from '../../../../gen/meurpg/play/v1/play_pb';
import { EncounterMode } from '../../../../gen/meurpg/play/v1/combat_pb';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import { MapsClient } from '../../../core/maps/maps-client';
import { OpenSessionLookup } from '../../../core/play/open-session';
import { CombatClient } from '../../../core/combat/combat-client';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { flat, isOff } from '../../../core/creatures/creatures-testing';
import { BANDIT } from '../../../core/encounters/encounters-testing';
import { PutMonstersSheet, type PutMonstersData } from './put-sheet';
import { fakeCombatClient } from '../../../core/combat/combat-client-testing';

/** The combat client of the sheet: the open combat, and what was asked of it. */
class FakeCombat {
  open: Encounter | null = encounter({
    id: 'enc-1',
    name: 'Emboscada na ponte',
    status: EncounterStatus.SETUP,
    combatants: [],
  });
  getFail: unknown = null;
  failures: unknown[] = [];
  addIds: string[] = ['m1', 'm2', 'm3'];
  adds: {
    encounterId: string;
    add: { creatureKey: string; count: number; name: string; hp: string; hidden: boolean };
    key: string;
  }[] = [];
  starts: {
    name: string;
    participants: unknown[];
    key: string;
    extras: Record<string, unknown>;
  }[] = [];
  get = vi.fn(async () => {
    if (this.getFail) {
      throw this.getFail;
    }
    return this.open;
  });
  addMonsters = vi.fn(
    async (
      _c: string,
      encounterId: string,
      add: (typeof this.adds)[number]['add'],
      key: string,
    ) => {
      this.adds.push({ encounterId, add, key });
      const failure = this.failures.shift();
      if (failure) {
        throw failure;
      }
      return { encounter: this.open!, combatantIds: this.addIds };
    },
  );
  start = vi.fn(
    async (
      _c: string,
      name: string,
      participants: unknown[],
      key: string,
      extras: Record<string, unknown>,
    ) => {
      this.starts.push({ name, participants, key, extras });
      const failure = this.failures.shift();
      if (failure) {
        throw failure;
      }
      return encounter({ id: 'enc-new', name });
    },
  );
}

describe('PutMonstersSheet: "Pôr no combate" (MR-042, RN-29, E10-08 states 4 and 7)', () => {
  let api: FakeCombat;
  let close: ReturnType<typeof vi.fn>;

  async function setup(
    open: Encounter | null | 'closed' = encounter({
      id: 'enc-1',
      name: 'Emboscada na ponte',
      status: EncounterStatus.SETUP,
      combatants: [],
    }),
    world: { map: 'grid' | 'nogrid' | 'none'; rule?: boolean } = { map: 'grid' },
  ) {
    api = new FakeCombat();
    if (open === 'closed') {
      api.getFail = new ConnectError('no session', Code.FailedPrecondition, undefined, [
        {
          desc: GameSessionBlockedSchema,
          value: create(GameSessionBlockedSchema, {
            reason: GameSessionBlockedReason.NO_OPEN_SESSION,
          }),
        },
      ]);
    } else {
      api.open = open;
    }
    close = vi.fn();
    const data: PutMonstersData = { campaignId: 'camp-1', creature: BANDIT };
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: fakeCombatClient(api) },
        {
          provide: OpenSessionLookup,
          useValue: {
            currentMap: async () => ({
              sessionNumber: 1,
              mapId: world.map === 'none' ? '' : 'map-1',
            }),
          },
        },
        {
          provide: MapsClient,
          useValue: {
            get: async () => ({ map: { id: 'map-1', gridColumns: world.map === 'grid' ? 20 : 0 } }),
          },
        },
        {
          provide: TableRulesClient,
          useValue: { get: async () => ({ saved: { combatStartsWithMap: world.rule ?? true } }) },
        },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
      ],
    });
    const fixture = TestBed.createComponent(PutMonstersSheet);
    const settle = async () => {
      for (let i = 0; i < 8; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
      }
    };
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find(
        (b) => (flat(b) ?? '').includes(name) || b.getAttribute('aria-label')?.includes(name),
      )!;
    const more = () =>
      el.querySelector<HTMLButtonElement>('app-count-stepper button[aria-label^="Mais"]')!;
    const radio = (label: string) =>
      Array.from(el.querySelectorAll<HTMLLabelElement>('.seg__item'))
        .find((l) => flat(l)?.includes(label))!
        .querySelector('input')!;
    const mode = (title: string) =>
      Array.from(el.querySelectorAll<HTMLLabelElement>('app-dice-choice label.dice-choice__card'))
        .find((l) => flat(l)?.includes(title))!
        .querySelector('input')!;
    return { el, button, more, radio, mode, settle, fixture };
  }

  it('opens on the combat in preparation with one Bandido, the average hit points and hidden on', async () => {
    const { el } = await setup();
    expect(flat(el.querySelector('.frame__title'))).toBe('Pôr no combate');
    expect(flat(el.querySelector('.frame__sub'))).toBe('Bandido do bestiário');
    expect(flat(el.querySelector('.readonly'))).toBe('Emboscada na ponte · em preparação');
    expect(flat(el.querySelector('.who'))).toContain('ND 1/8 · CA 12 · PV 11 (média)');
    expect(flat(el.querySelector('.count__names'))).toBe('Entram como Bandido.');
    expect(el.querySelector<HTMLInputElement>('input[name=name]')!.value).toBe('Bandido');
    expect(el.querySelector('[role=switch]')?.getAttribute('aria-checked')).toBe('true');
    expect(flat(el.querySelector('.seg__item--on'))).toBe('Média (11)');
  });

  it('previews "Bandido 1, Bandido 2 e Bandido 3" for three, and the base name changes it', async () => {
    const { el, more, settle } = await setup();
    more().click();
    more().click();
    await settle();
    expect(flat(el.querySelector('.count__names'))).toBe(
      'Entram como Bandido 1, Bandido 2 e Bandido 3.',
    );
    const input = el.querySelector<HTMLInputElement>('input[name=name]')!;
    input.value = 'Salteador';
    input.dispatchEvent(new Event('input'));
    await settle();
    expect(flat(el.querySelector('.count__names'))).toBe(
      'Entram como Salteador 1, Salteador 2 e Salteador 3.',
    );
  });

  it('puts three hidden Bandidos with the average in the open combat, under one key', async () => {
    const { button, more, settle } = await setup();
    more().click();
    more().click();
    await settle();
    expect(flat(button('Pôr 3 no combate'))).toBe('Pôr 3 no combate');
    button('Pôr 3 no combate').click();
    await settle();
    expect(api.adds).toEqual([
      {
        encounterId: 'enc-1',
        add: {
          creatureKey: 'monster:bandit',
          count: 3,
          name: 'Bandido',
          hp: 'average',
          hidden: true,
        },
        key: expect.stringMatching(/^[0-9a-f-]{36}$/),
      },
    ]);
    expect(close).toHaveBeenCalledWith(
      expect.objectContaining({
        count: 3,
        names: 'Bandido 1, Bandido 2 e Bandido 3',
        combatName: 'Emboscada na ponte',
        started: false,
        hidden: true,
      }),
    );
  });

  it('names what the server made: with Bandidos already in, it numbers on', async () => {
    const { button, more, settle } = await setup(
      encounter({
        id: 'enc-1',
        name: 'Emboscada na ponte',
        status: EncounterStatus.SETUP,
        combatants: [1, 2, 3, 4, 5, 6].map((n) =>
          combatant({ id: `m${n}`, label: `Bandido ${n}` }),
        ),
      }),
    );
    api.addIds = ['m4', 'm5', 'm6'];
    more().click();
    more().click();
    await settle();
    button('Pôr 3 no combate').click();
    await settle();
    expect(close).toHaveBeenCalledWith(
      expect.objectContaining({ names: 'Bandido 4, Bandido 5 e Bandido 6' }),
    );
  });

  it("names them in their numbers' order, not the combat's initiative order", async () => {
    const { button, more, settle } = await setup(
      encounter({
        id: 'enc-1',
        name: 'Emboscada na ponte',
        status: EncounterStatus.SETUP,
        // The combat lists by initiative: Bandido 3 rolled highest.
        combatants: [3, 1, 2].map((n) => combatant({ id: `m${n}`, label: `Bandido ${n}` })),
      }),
    );
    api.addIds = ['m3', 'm1', 'm2'];
    more().click();
    more().click();
    await settle();
    button('Pôr 3 no combate').click();
    await settle();
    expect(close).toHaveBeenCalledWith(
      expect.objectContaining({ names: 'Bandido 1, Bandido 2 e Bandido 3' }),
    );
  });

  it('rolls the hit points and starts them revealed when the master says so', async () => {
    const { button, radio, el, settle } = await setup();
    radio('Rolar').click();
    el.querySelector<HTMLButtonElement>('[role=switch]')!.click();
    await settle();
    button('Pôr no combate').click();
    await settle();
    expect(api.adds[0].add).toMatchObject({ hp: 'rolled', hidden: false });
  });

  it('a double tap adds once; a retry with the same parameters repeats the key; another choice takes a new key', async () => {
    const { button, settle } = await setup();
    api.failures = [new ConnectError('down', Code.Unavailable)];
    const go = button('Pôr no combate');
    go.click();
    go.click();
    await settle();
    expect(api.adds).toHaveLength(1);
    // The answer was lost: the retry is the same add, the same key.
    button('Pôr no combate').click();
    await settle();
    expect(api.adds).toHaveLength(2);
    expect(api.adds[1].key).toBe(api.adds[0].key);
    expect(close).toHaveBeenCalledTimes(1);
  });

  it('takes a new key when the choice changed since the failed try', async () => {
    const { button, radio, settle } = await setup();
    api.failures = [new ConnectError('down', Code.Unavailable)];
    button('Pôr no combate').click();
    await settle();
    radio('Rolar').click();
    await settle();
    button('Pôr no combate').click();
    await settle();
    expect(api.adds[1].key).not.toBe(api.adds[0].key);
  });

  it('says what a refusal means in words, and keeps the sheet open', async () => {
    const { button, el, settle } = await setup();
    api.failures = [new ConnectError('a combat has at most 40 combatants', Code.InvalidArgument)];
    button('Pôr no combate').click();
    await settle();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('40 combatentes');
    expect(close).not.toHaveBeenCalled();
  });

  it('says a combat that ended', async () => {
    const { button, el, settle } = await setup();
    api.failures = [
      new ConnectError('ended', Code.FailedPrecondition, undefined, [
        {
          desc: EncounterBlockedSchema,
          value: create(EncounterBlockedSchema, { reason: EncounterBlockedReason.ENCOUNTER_ENDED }),
        },
      ]),
    ];
    button('Pôr no combate').click();
    await settle();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('já terminou');
  });

  it('stops the count at what fits in a combat of 40, and says why; a full combat has no add', async () => {
    const thirty = encounter({
      id: 'enc-1',
      name: 'Cheio',
      status: EncounterStatus.ACTIVE,
      combatants: Array.from({ length: 36 }, (_, i) => combatant({ id: `c${i}`, label: `N ${i}` })),
    });
    const { el, more, settle, button } = await setup(thirty);
    for (let i = 0; i < 6; i++) {
      more().click();
    }
    await settle();
    expect(flat(el.querySelector('.count__names'))).toBe(
      'Entram como Bandido 1, Bandido 2, Bandido 3 e Bandido 4.',
    );
    expect(flat(el.querySelector('.field__hint'))).toContain('Cabem mais 4 neste combate');
    button('Pôr 4 no combate').click();
    await settle();
    expect(api.adds[0].add.count).toBe(4);

    const full = encounter({
      id: 'enc-2',
      name: 'Lotado',
      status: EncounterStatus.ACTIVE,
      combatants: Array.from({ length: 40 }, (_, i) => combatant({ id: `d${i}`, label: `N ${i}` })),
    });
    TestBed.resetTestingModule();
    const again = await setup(full);
    expect(flat(again.el.querySelector('.mr-notice--warning'))).toContain('O combate está cheio');
    expect(isOff(again.button('Pôr no combate'))).toBe(true);
  });

  it('with no combat open, "Criar o combate e pôr" starts one with the monsters and the party', async () => {
    const { button, more, el, settle } = await setup(null);
    expect(flat(el.querySelector('.readonly'))).toBe('Nenhum combate aberto');
    more().click();
    await settle();
    button('Criar o combate e pôr').click();
    await settle();
    expect(api.adds).toHaveLength(0);
    expect(api.starts).toHaveLength(1);
    expect(api.starts[0]).toMatchObject({
      name: 'Combate: Bandido',
      participants: [],
      extras: {
        monsters: [{ creatureKey: 'monster:bandit', count: 2, name: 'Bandido' }],
        monsterHp: 'average',
        monstersHidden: true,
      },
    });
    expect(close).toHaveBeenCalledWith(
      expect.objectContaining({ started: true, combatName: 'Combate: Bandido', count: 2 }),
    );
  });

  it('a session that is not open waits: the button is off and says to open it', async () => {
    const { el, button } = await setup('closed');
    expect(flat(el.querySelector('.readonly'))).toBe('Sem sessão aberta');
    expect(flat(el.querySelector('.why'))).toBe('Abra a sessão primeiro.');
    expect(isOff(button('Pôr no combate'))).toBe(true);
  });

  it('refuses a name past 30 letters in place, with the focus on the field', async () => {
    const { el, button, settle } = await setup();
    const input = el.querySelector<HTMLInputElement>('input[name=name]')!;
    input.removeAttribute('maxlength');
    input.value = 'x'.repeat(31);
    input.dispatchEvent(new Event('input'));
    await settle();
    button('Pôr no combate').click();
    await settle();
    expect(flat(el.querySelector('.field__err'))).toContain('de 1 a 30 letras');
    expect(api.adds).toHaveLength(0);
  });

  it('starting a combat offers the same choice as "Iniciar combate": with a gridded map, "Com mapa" by default', async () => {
    const { button, mode, settle } = await setup(null);
    expect(mode('Com mapa').checked).toBe(true);
    button('Criar o combate e pôr').click();
    await settle();
    expect(api.starts[0].extras).toMatchObject({ mode: EncounterMode.GRID });
  });

  it('the master picks "Sem mapa (teatro da mente)" and the combat starts without a map', async () => {
    const { button, mode, el, settle } = await setup(null);
    mode('Sem mapa').click();
    await settle();
    expect(el.querySelector('[data-testid=theatre-why]')).not.toBeNull();
    button('Criar o combate e pôr').click();
    await settle();
    expect(api.starts[0].extras).toMatchObject({ mode: EncounterMode.THEATRE });
  });

  it('a session map without a grid, or no map, defaults to the theatre of the mind with "Com mapa" off', async () => {
    for (const map of ['nogrid', 'none'] as const) {
      TestBed.resetTestingModule();
      const { button, mode, settle } = await setup(null, { map });
      expect(mode('Sem mapa').checked).toBe(true);
      expect(mode('Com mapa').closest('label')?.className).toContain('dice-choice__card--off');
      button('Criar o combate e pôr').click();
      await settle();
      expect(api.starts[0].extras).toMatchObject({ mode: EncounterMode.THEATRE });
    }
  });

  it('the table\'s rule "combate com mapa" off makes the theatre the default on a gridded map', async () => {
    const { mode } = await setup(null, { map: 'grid', rule: false });
    expect(mode('Sem mapa').checked).toBe(true);
  });

  it('a combat already open shows no mode choice: it is fixed once it starts', async () => {
    const { el } = await setup();
    expect(el.querySelector('app-dice-choice')).toBeNull();
  });
});
