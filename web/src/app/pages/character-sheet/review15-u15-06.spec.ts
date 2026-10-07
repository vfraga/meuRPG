import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { BehaviorSubject } from 'rxjs';

import { CharacterVitalsSchema } from '../../../gen/meurpg/play/v1/play_pb';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import {
  FakeCreaturesClient,
  creature,
  flat,
  raven,
  summonAnswer,
} from '../../core/creatures/creatures-testing';
import { OpenSessions } from '../../shell/live-notice/open-sessions';
import { CharacterSheetSource } from './character-sheet.types';
import { CreaturePage } from './creature-page/creature-page';
import { CreaturesPanel } from './creatures-panel/creatures-panel';

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const tick = () => new Promise((r) => setTimeout(r));

describe('Review15 U15-6: creature page and Wild Shape panel race with their own requests', () => {
  it('A: back/forward between two creatures with the first answer landing last still shows the creature of the route', async () => {
    const api = new FakeCreaturesClient();
    api.blocks.set('monster:raven', raven());
    const a = creature('cr-A', 'Alfa');
    const b = creature('cr-B', 'Beta');
    const pending = [deferred<unknown>(), deferred<unknown>()];
    let n = 0;
    api.list = vi.fn(() => pending[n++].promise) as never;
    const params = new BehaviorSubject(
      convertToParamMap({ id: 'camp-1', characterId: 'char-1', creatureId: 'cr-A' }),
    );
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: CreaturesClient, useValue: api },
        { provide: ActivatedRoute, useValue: { paramMap: params } },
        {
          provide: CharacterSheetSource,
          useValue: {
            getCharacterSheet: async () => ({ name: 'Pensantus', isMaster: false }),
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(CreaturePage);
    const el = fixture.nativeElement as HTMLElement;
    fixture.detectChanges();
    await tick();
    // The person goes to B before A's (slow) answer arrives: the same component is reused.
    params.next(
      convertToParamMap({ id: 'camp-1', characterId: 'char-1', creatureId: 'cr-B' }),
    );
    fixture.detectChanges();
    await tick();
    expect(api.list).toHaveBeenCalledTimes(2);
    pending[1].resolve([a, b]); // B's answer first
    await tick();
    await tick();
    fixture.detectChanges();
    expect(flat(el.querySelector('h1'))).toBe('Beta');
    pending[0].resolve([a, b]); // then the stale answer for A
    await tick();
    await tick();
    fixture.detectChanges();
    expect(flat(el.querySelector('h1'))).toBe('Beta');
  });

  describe('creatures panel, a druid in Wild Shape', () => {
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

    it('B: a second tap on "Voltar à forma normal" while the first is pending neither sends another key nor shows a refusal', async () => {
      const api = new FakeCreaturesClient();
      api.vitals = vitals('Lobo');
      const fixture = build(api);
      fixture.detectChanges();
      await settle(fixture);
      const el = fixture.nativeElement as HTMLElement;
      const button = el.querySelector<HTMLButtonElement>('.js-wild')!;
      expect(flat(button)).toContain('Voltar à forma normal');

      // The server: the first call ends the form; any call with another key finds the druid in her own shape
      // (backend shapeChange checks the state, not the key: NOT_IN_WILD_SHAPE).
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
      button.click(); // the impatient second tap
      await tick();
      fixture.detectChanges();
      first.resolve({ vitals: undefined, encounter: undefined });
      await settle(fixture);

      expect(api.leaveWildShape).toHaveBeenCalledTimes(1);
      expect(flat(el.querySelector('[role="alert"]'))).toBeUndefined();
    });

    it('C: an older vitals read that lands after a newer one does not overwrite the form', async () => {
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
  });
});
