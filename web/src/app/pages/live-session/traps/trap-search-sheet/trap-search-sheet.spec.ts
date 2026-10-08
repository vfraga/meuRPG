import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { MapPointKind, MapPointSchema } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { SearchForTrapsResponseSchema } from '../../../../../gen/meurpg/play/v1/traps_pb';
import { TrapsClient } from '../../../../core/traps/traps-client';
import { TrapSearchSheet, type TrapSearchData } from './trap-search-sheet';

const roll = (face: number, total: number) => ({
  diceCount: 1,
  diceSides: 20,
  faces: [face],
  modifier: total - face,
  total,
});

function blockedBy(reason: EncounterBlockedReason): ConnectError {
  const err = new ConnectError('x', Code.FailedPrecondition);
  return Object.assign(err, { findDetails: () => [create(EncounterBlockedSchema, { reason })] });
}
const twoDiceError = () => blockedBy(EncounterBlockedReason.SEARCH_NEEDS_TWO_DICE);

describe('TrapSearchSheet', () => {
  function setup(
    responses: (unknown | Error)[],
    listed?: ReturnType<typeof create<typeof MapPointSchema>>[],
  ) {
    const sent: unknown[] = [];
    const keys: string[] = [];
    const api = {
      search: async (_c: string, skill: string, die: unknown, key: string) => {
        sent.push([skill, die]);
        keys.push(key);
        const next = responses.shift();
        if (next instanceof Error) {
          throw next;
        }
        return next;
      },
    };
    const found = create(MapPointSchema, {
      id: 'x',
      kind: MapPointKind.TRAP,
      name: 'Fosso escondido',
    });
    const data: TrapSearchData = {
      campaignId: 'c',
      skills: { perception: 4, investigation: 4 },
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state: {
        refresh: async () => undefined,
        points: () => listed ?? [found],
      } as unknown as TrapSearchData['state'],
      inCombat: false,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: TrapsClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(TrapSearchSheet);
    fixture.detectChanges();
    const roller = fixture.componentInstance as unknown as {
      rollWith(d: unknown): Promise<void>;
      pick(s: string): void;
    };
    return { fixture, el: fixture.nativeElement as HTMLElement, sent, keys, roller };
  }

  it('offers Percepção and Investigação with the bonus, the helper line and the three steps', () => {
    const { el } = setup([]);
    const text = el.textContent!.replace(/\s+/g, ' ');
    expect(text).toContain('Percepção +4');
    expect(text).toContain('Investigação +4');
    expect(text).toContain('Algumas armadilhas só se acham com Investigação.');
    expect(Array.from(el.querySelectorAll('.steps__name'), (e) => e.textContent)).toEqual([
      'Como',
      'Rolar',
      'Resultado',
    ]);
    expect(el.querySelector('[role=dialog], .frame')).toBeTruthy();
  });

  it('answers the same words for a miss as for no trap', async () => {
    const { fixture, el, roller } = setup([
      create(SearchForTrapsResponseSchema, { roll: roll(6, 13), foundPointIds: [] }),
    ]);
    roller.pick('investigation');
    await roller.rollWith({ face: 6 });
    fixture.detectChanges();
    expect(el.textContent).toContain('Você não encontrou nada.');
    expect(el.textContent).toContain('Nada');
  });

  it('reads the trap found by name', async () => {
    const { fixture, el, roller } = setup([
      create(SearchForTrapsResponseSchema, { roll: roll(13, 17), foundPointIds: ['x'] }),
    ]);
    await roller.rollWith({ inApp: true });
    fixture.detectChanges();
    expect(el.textContent).toContain('Você achou uma armadilha: Fosso escondido.');
    expect(el.textContent).toContain('Ela já aparece no seu mapa.');
  });

  it('asks for the second die when the server says the search has disadvantage, and sends both', async () => {
    const { fixture, el, sent, roller } = setup([
      twoDiceError(),
      create(SearchForTrapsResponseSchema, {
        roll: roll(9, 13),
        secondRoll: roll(4, 8),
        foundPointIds: [],
      }),
    ]);
    await roller.rollWith({ face: 9 });
    fixture.detectChanges();
    expect(el.textContent).toContain('Digite o segundo dado');
    expect(el.textContent).toContain('Há penumbra por perto');
    await roller.rollWith({ face: 4 });
    expect(sent[1]).toEqual(['perception', { face: 9, face2: 4 }]);
  });

  it('says why the server refused, by reason', async () => {
    for (const [reason, words] of [
      [EncounterBlockedReason.ACTION_USED, 'já usou a sua ação'],
      [EncounterBlockedReason.NOT_YOUR_TURN, 'só na sua vez'],
      [EncounterBlockedReason.TRAP_NOT_ON_MAP, 'não está no mapa'],
      [EncounterBlockedReason.TRAP_SEARCH_NOT_NOW, 'não está na vez de ninguém'],
    ] as const) {
      TestBed.resetTestingModule();
      const { fixture, el, roller } = setup([blockedBy(reason)]);
      await roller.rollWith({ inApp: true });
      fixture.detectChanges();
      expect(el.querySelector('[role=alert]')?.textContent).toContain(words);
    }
  });

  it('says a trap was found, and sends the player to the map, when its name could not be read', async () => {
    const { fixture, el, roller } = setup(
      [create(SearchForTrapsResponseSchema, { roll: roll(13, 17), foundPointIds: ['x'] })],
      [],
    );
    await roller.rollWith({ inApp: true });
    fixture.detectChanges();
    expect(el.textContent).toContain('Você achou uma armadilha.');
    expect(el.textContent).toContain('Veja no seu mapa.');
    expect(el.textContent).not.toContain('Você não encontrou nada.');
  });

  it('keeps the key of a search retried as it was, and takes a new one for another skill', async () => {
    const lost = new ConnectError('lost', Code.Unavailable);
    const { roller, keys } = setup([
      lost,
      lost,
      create(SearchForTrapsResponseSchema, { roll: roll(13, 17), foundPointIds: [] }),
    ]);
    await roller.rollWith({ inApp: true });
    await roller.rollWith({ inApp: true });
    roller.pick('investigation');
    await roller.rollWith({ inApp: true });
    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[0]);
  });
});
