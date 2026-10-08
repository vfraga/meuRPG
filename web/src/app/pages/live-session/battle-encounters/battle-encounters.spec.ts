import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { of } from 'rxjs';

import { MapPointKind } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { flat, isOff } from '../../../core/creatures/creatures-testing';
import { EncountersClient } from '../../../core/encounters/encounters-client';
import { EncounterWarning } from '../../../../gen/meurpg/play/v1/encounters_pb';
import {
  FakeEncountersClient,
  GOBLIN,
  artboardEncounter,
  evaluation,
  line,
} from '../../../core/encounters/encounters-testing';
import { MapState } from '../../../core/maps/map-state';
import { mapMessage, mapPoint } from '../../../core/maps/maps-testing';
import { BattleEncounters } from './battle-encounters';

@Component({
  imports: [BattleEncounters],
  template:
    '<app-battle-encounters campaignId="camp-1" [state]="state" (started)="started.set($event.name)" />',
})
class Host {
  state = new MapState(async () => {
    throw new Error('not read');
  });
  started = signal('');
}

describe('BattleEncounters: "Começar este combate" on the master\'s session (MR-043, E10-09 state 7)', () => {
  let api: FakeEncountersClient;
  let opened: { data: { saved?: Record<string, unknown>; map: unknown; campaignId: string } }[];
  let result: unknown;

  async function setup(prep: (api: FakeEncountersClient) => void = () => undefined) {
    api = new FakeEncountersClient();
    const art = artboardEncounter();
    api.kept = [{ mapPointId: 'pt-1', creatureCount: 13 }];
    api.battle.set('pt-1', {
      encounter: art.encounter,
      evaluation: art.evaluation,
      unknownKeys: [],
    });
    prep(api);
    opened = [];
    result = undefined;
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: EncountersClient, useValue: api },
        {
          provide: MatDialog,
          useValue: {
            open: (_c: unknown, config: (typeof opened)[number]) => {
              opened.push(config);
              return { afterClosed: () => of(result) };
            },
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(Host);
    const state = fixture.componentInstance.state;
    state.map.set(mapMessage('map-1', 'Estrada do Vale', { gridColumns: 24, gridRows: 16 }));
    state.points.set([
      mapPoint('pt-1', 'Emboscada na ponte', { kind: MapPointKind.BATTLE }),
      mapPoint('pt-2', 'Ruínas do forte', { kind: MapPointKind.BATTLE }),
      mapPoint('pt-3', 'Taverna', { kind: MapPointKind.SCENE }),
    ]);
    const settle = async () => {
      for (let i = 0; i < 5; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
      }
    };
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    return { el, settle, host: fixture.componentInstance };
  }

  it("shows the point that keeps an encounter, with the band against today's party, the label, and the creatures", async () => {
    const { el } = await setup();
    expect(api.list).toHaveBeenCalledWith('camp-1', 'map-1');
    // Only the point that keeps one is read (the other battle point and the scene are not).
    expect(api.getCalls).toEqual(['pt-1']);
    expect(el.querySelectorAll('.enc')).toHaveLength(1);
    expect(flat(el.querySelector('.enc__t'))).toBe('Emboscada na ponte');
    expect(flat(el.querySelector('.enc__pill'))).toBe('Encontro guardado');
    expect(flat(el.querySelector('.enc__band'))).toBe('Moderada · 1.550 de 1.875 XP');
    expect(flat(el.querySelector('.enc__guide'))).toBe(
      'Guia de dificuldade do SRD 5.2.1 (regras de 2024) · Créditos',
    );
    expect(Array.from(el.querySelectorAll('.enc__row')).map((r) => flat(r))).toEqual([
      '1 × Ogro ND 2 450 XP',
      '2 × Bugbear ND 1 400 XP',
      '4 × Hobgoblin ND 1/2 400 XP',
      '6 × Goblin ND 1/4 300 XP',
    ]);
    expect(flat(el.querySelector('.enc__go'))).toBe('Começar este combate');
  });

  it('shows nothing when no battle point keeps an encounter', async () => {
    const { el } = await setup((a) => (a.kept = []));
    expect(el.querySelector('.enc')).toBeNull();
    expect(el.querySelector('.wrap')).toBeNull();
  });

  it('"Começar este combate" opens "Iniciar combate" filled from the point: the name, the monsters, the average, hidden, and the point', async () => {
    const { el, settle, host } = await setup();
    result = { name: 'Emboscada na ponte' };
    el.querySelector<HTMLButtonElement>('.enc__go')!.click();
    await settle();
    expect(opened).toHaveLength(1);
    expect(opened[0].data).toMatchObject({
      campaignId: 'camp-1',
      mode: 'start',
      saved: {
        pointId: 'pt-1',
        pointName: 'Emboscada na ponte',
        hp: 'average',
        hidden: true,
        groups: [
          { key: 'monster:ogre', namePt: 'Ogro', count: 1 },
          { key: 'monster:bugbear', namePt: 'Bugbear', count: 2 },
          { key: 'monster:hobgoblin', namePt: 'Hobgoblin', count: 4 },
          { key: 'monster:goblin', namePt: 'Goblin', count: 6 },
        ],
      },
    });
    // The combat the dialog started is the page's.
    expect(host.started()).toBe('Emboscada na ponte');
  });

  it('starts the encounter as the point keeps it now, when it was edited elsewhere without changing the points', async () => {
    const { el, settle } = await setup();
    const edited = artboardEncounter();
    api.battle.set('pt-1', {
      ...edited,
      encounter: { ...edited.encounter, monsters: edited.encounter.monsters.slice(0, 1) },
      unknownKeys: [],
    });
    result = undefined;
    el.querySelector<HTMLButtonElement>('.enc__go')!.click();
    await settle();
    expect(api.getCalls).toEqual(['pt-1', 'pt-1']);
    expect(opened).toHaveLength(1);
    expect((opened[0].data.saved!['groups'] as { key: string }[]).map((g) => g.key)).toEqual([
      'monster:ogre',
    ]);
  });

  it('a saved encounter with a creature the SRD lost says so and does not start', async () => {
    const { el } = await setup((a) => {
      const read = a.battle.get('pt-1')!;
      a.battle.set('pt-1', { ...read, unknownKeys: ['monster:gone'] });
    });
    expect(flat(el.querySelector('.mr-notice--warning'))).toContain(
      'Uma criatura deste encontro não está mais no SRD.',
    );
    const go = el.querySelector<HTMLButtonElement>('.enc__go')!;
    expect(isOff(go)).toBe(true);
    // The button stays focusable (disabledInteractive), so a click still reaches the handler: it does nothing.
    go.click();
    expect(opened).toHaveLength(0);
  });

  it('a saved encounter that does not fit one combat of 40 shows the warning the server sends, and the lost-creature notice links to the builder on that point', async () => {
    const { el } = await setup((a) => {
      const read = a.battle.get('pt-1')!;
      a.battle.set('pt-1', {
        ...read,
        unknownKeys: ['monster:gone'],
        evaluation: evaluation([line(GOBLIN, 40)], { warnings: [EncounterWarning.TOO_MANY] }),
      });
    });
    const notices = Array.from(el.querySelectorAll('.mr-notice--warning')).map((n) => flat(n));
    expect(notices.some((n) => n?.startsWith('Criaturas demais para um combate.'))).toBe(true);
    expect(el.querySelector('.mr-notice--warning a')?.getAttribute('href')).toBe(
      '/campaigns/camp-1/encounters?map=map-1&point=pt-1',
    );
  });

  it('says in words when the saved encounters cannot be read', async () => {
    const { el } = await setup((a) => a.list.mockRejectedValueOnce(new Error('down')));
    expect(el.querySelector('[role=alert]')?.textContent).toContain(
      'Não deu para ler o encontro guardado',
    );
  });
});
