import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { BehaviorSubject } from 'rxjs';

import { create } from '@bufbuild/protobuf';

import {
  CreatureActionSchema,
  CreatureDamagePartSchema,
  CreatureTraitSchema,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import { CreatureSource } from '../../../../gen/meurpg/characters/v1/characters_pb';
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

  describe('a table creature', () => {
    async function open(monsterKey: string, ownerName: string): Promise<HTMLElement> {
      const api = new FakeCreaturesClient();
      api.blocks.set(monsterKey, raven());
      api.ownBlocks.set('cr-A', raven());
      api.list = vi.fn(async () => [
        creature('cr-A', 'Lobo do Vale', { monsterKey, source: CreatureSource.TABLE }),
      ]) as never;
      TestBed.configureTestingModule({
        providers: [
          provideRouter([]),
          { provide: CreaturesClient, useValue: api },
          {
            provide: ActivatedRoute,
            useValue: {
              paramMap: new BehaviorSubject(
                convertToParamMap({ id: 'camp-1', characterId: 'char-1', creatureId: 'cr-A' }),
              ),
            },
          },
          {
            provide: CharacterSheetSource,
            useValue: { getCharacterSheet: async () => ({ name: ownerName, isMaster: false }) },
          },
        ],
      });
      const fixture = TestBed.createComponent(CreaturePage);
      fixture.detectChanges();
      await nextTurn();
      await nextTurn();
      fixture.detectChanges();
      return fixture.nativeElement as HTMLElement;
    }

    async function openTable(ownerName: string) {
      const api = new FakeCreaturesClient();
      const own = raven({
        armorClass: 16,
        hitPoints: 24,
        traits: [
          create(CreatureTraitSchema, {
            name: 'Faro do Vale',
            text: 'Segue o cheiro de quem cruzou o vale.',
          }),
        ],
        actions: [
          create(CreatureActionSchema, {
            name: 'Mordida Sombria',
            namePt: 'Mordida Sombria',
            text: 'Melee Weapon Attack.',
            hasAttack: true,
            attackBonus: 7,
            damage: [
              create(CreatureDamagePartSchema, {
                dice: '2d6+5',
                damageTypeKey: 'damage-type:necrotic',
                damageTypePt: 'necrótico',
              }),
            ],
          }),
        ],
      });
      api.ownBlocks.set('cr-A', own);
      api.list = vi.fn(async () => [
        creature('cr-A', 'Lobo do Vale', {
          monsterKey: 'creature:lobo-do-vale@mesa',
          source: CreatureSource.TABLE,
          hitPointsCurrent: 24,
          hitPointsMax: 24,
        }),
      ]) as never;
      TestBed.configureTestingModule({
        providers: [
          provideRouter([]),
          { provide: CreaturesClient, useValue: api },
          {
            provide: ActivatedRoute,
            useValue: {
              paramMap: new BehaviorSubject(
                convertToParamMap({ id: 'camp-1', characterId: 'char-1', creatureId: 'cr-A' }),
              ),
            },
          },
          {
            provide: CharacterSheetSource,
            useValue: { getCharacterSheet: async () => ({ name: ownerName, isMaster: false }) },
          },
        ],
      });
      const fixture = TestBed.createComponent(CreaturePage);
      fixture.detectChanges();
      await nextTurn();
      await nextTurn();
      fixture.detectChanges();
      return { el: fixture.nativeElement as HTMLElement, api };
    }

    it('says "Da mesa" and whose sheet the numbers come from', async () => {
      const el = await open('creature:lobo-do-vale@mesa', 'Tor');
      expect(flat(el.querySelector('.head__origin'))).toContain('Da mesa');
      expect(flat(el.querySelector('.table-note'))).toBe(
        'Os números vêm da ficha de Tor quando ele chegou',
      );
    });

    it("shows the numbers, the attacks and the traits worked out for the owner, not the entry's static ones", async () => {
      const { el, api } = await openTable('Tor');
      expect(api.creatureBlock).toHaveBeenCalledWith(
        'camp-1',
        'creature:lobo-do-vale@mesa',
        'cr-A',
      );
      expect(api.statBlock).not.toHaveBeenCalled();
      const tiles = flat(el.querySelector('.tiles'));
      expect(tiles).toContain('CA 16');
      expect(tiles).toContain('PV 24 de 24');
      expect(flat(el.querySelector('.attacks'))).toContain(
        'Mordida Sombria +7 · 2d6 + 5 necrótico',
      );
      expect(flat(el.querySelector('.block:last-of-type'))).toContain(
        'Faro do Vale. Segue o cheiro',
      );
      // No ability table: a compact page.
      expect(el.querySelector('.abilities')).toBeNull();
    });

    it('ends with the note, after the traits and before the actions', async () => {
      const { el } = await openTable('Tor');
      const note = el.querySelector('.table-note')!;
      expect(note.previousElementSibling?.classList.contains('block')).toBe(true);
      expect(note.nextElementSibling?.classList.contains('acts')).toBe(true);
    });

    it('falls back to "do dono" when the owner has no name', async () => {
      const el = await open('creature:lobo-do-vale@mesa', '  ');
      expect(flat(el.querySelector('.table-note'))).toBe(
        'Os números vêm da ficha do dono quando ele chegou',
      );
    });

    it('says nothing of the kind for a book creature', async () => {
      const el = await open('monster:raven', 'Tor');
      expect(el.querySelector('.table-note')).toBeNull();
      expect(flat(el.querySelector('.head__origin'))).not.toContain('Da mesa');
    });
  });
});
