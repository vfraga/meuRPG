import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { BehaviorSubject } from 'rxjs';

import { CreaturesClient } from '../../../core/creatures/creatures-client';
import {
  FakeCreaturesClient,
  creature,
  flat,
  raven,
} from '../../../core/creatures/creatures-testing';
import { CharacterSheetSource } from '../character-sheet.types';
import { CreaturePage } from './creature-page';
import { nextTurn } from '../../../../test-settle';

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe('CreaturePage', () => {
  it('shows the creature of the route when the answer for the one the person left lands last', async () => {
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
    await nextTurn();
    // The person goes to B before A's (slow) answer arrives: the same component is reused.
    params.next(convertToParamMap({ id: 'camp-1', characterId: 'char-1', creatureId: 'cr-B' }));
    fixture.detectChanges();
    await nextTurn();
    expect(api.list).toHaveBeenCalledTimes(2);
    pending[1].resolve([a, b]); // B's answer first
    await nextTurn();
    await nextTurn();
    fixture.detectChanges();
    expect(flat(el.querySelector('h1'))).toBe('Beta');
    pending[0].resolve([a, b]); // then the stale answer for A
    await nextTurn();
    await nextTurn();
    fixture.detectChanges();
    expect(flat(el.querySelector('h1'))).toBe('Beta');
  });
});
