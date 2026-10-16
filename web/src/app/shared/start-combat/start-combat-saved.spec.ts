import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';

import { DiceMode, DicePreference, Role } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterKind } from '../../../gen/meurpg/characters/v1/characters_pb';
import { TableRulesClient } from '../../core/campaigns/table-rules';
import { EncounterMode } from '../../../gen/meurpg/play/v1/combat_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { CombatClient } from '../../core/combat/combat-client';
import { encounter } from '../../core/combat/combat-testing';
import { flat, isOff } from '../../core/creatures/creatures-testing';
import { RosterClient } from '../../core/maps/roster-client';
import { StartCombatDialog, type StartCombatData } from './start-combat-dialog';

/** "Começar este combate" (MR-043, E10-09 state 7): "Iniciar combate" filled from the saved encounter of a battle point. */
describe('StartCombatDialog with a saved encounter', () => {
  let starts: {
    name: string;
    participants: { characterId: string }[];
    key: string;
    extras: Record<string, unknown>;
  }[];
  let close: ReturnType<typeof vi.fn>;
  let failures: unknown[];

  async function setup(over: Partial<StartCombatData['saved']> = {}, withMap = true) {
    starts = [];
    failures = [];
    close = vi.fn();
    const data: StartCombatData = {
      campaignId: 'camp-1',
      mode: 'start',
      map: withMap
        ? {
            id: 'map-1',
            name: 'Estrada do Vale',
            image: { url: '/i', width: 2400, height: 1600 },
            columns: 24,
            rows: 16,
          }
        : null,
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
        ...over,
      },
    };
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
        {
          provide: RosterClient,
          useValue: {
            list: async () => [
              {
                id: 'pc-1',
                name: 'Pensantus',
                kind: CharacterKind.PLAYER,
                playerUserId: 'u1',
                classSummary: 'Mago 4',
                raceName: 'Humano',
                playerName: 'Ana',
              },
              {
                id: 'pc-2',
                name: 'Toren',
                kind: CharacterKind.PLAYER,
                playerUserId: 'u2',
                classSummary: 'Guerreiro 4',
                raceName: 'Anão',
                playerName: 'Rui',
              },
              {
                id: 'npc-1',
                name: 'Orin',
                kind: CharacterKind.STORY,
                playerUserId: '',
                classSummary: '',
                raceName: '',
                playerName: null,
              },
            ],
          },
        },
        {
          provide: CampaignsService,
          useValue: {
            listMembers: async () => ({
              members: [
                { userId: 'u1', role: Role.PLAYER, dicePreference: DicePreference.APP },
                { userId: 'u2', role: Role.PLAYER, dicePreference: DicePreference.APP },
              ],
            }),
            getCampaign: async () => ({ campaign: { diceMode: DiceMode.PLAYERS_CHOOSE } }),
          },
        },
        // The table's rule says "Sem mapa": from a battle point it must not flip the dialog (item 4 of the fix round).
        {
          provide: TableRulesClient,
          useValue: { get: async () => ({ saved: { combatStartsWithMap: false } }) },
        },
        {
          provide: CombatClient,
          useValue: {
            start: vi.fn(
              async (
                _c: string,
                name: string,
                participants: { characterId: string }[],
                key: string,
                extras: Record<string, unknown>,
              ) => {
                starts.push({ name, participants, key, extras });
                const failure = failures.shift();
                if (failure) {
                  throw failure;
                }
                return encounter({ id: 'enc-9', name });
              },
            ),
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(StartCombatDialog);
    const settle = async () => {
      for (let i = 0; i < 5; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
      }
    };
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    return { el, settle };
  }

  it("opens filled: the point's name, the party checked, the monsters as read-only lines with what they become, hidden on and the average", async () => {
    const { el } = await setup();
    expect(flat(el.querySelector('#start-title'))).toBe('Iniciar combate');
    expect(el.querySelector<HTMLInputElement>('input[matInput], .dlg__name input')!.value).toBe(
      'Emboscada na ponte',
    );
    expect(flat(el.querySelector('#mon-title'))).toBe('Monstros do encontro');
    expect(Array.from(el.querySelectorAll('.mon__row')).map((r) => flat(r))).toEqual([
      'O Ogro Vira Ogro × 1 monstros',
      'B Bugbear Vira Bugbear 1 e 2 × 2 monstros',
      'H Hobgoblin Vira Hobgoblin 1 a 4 × 4 monstros',
      'G Goblin Vira Goblin 1 a 6 × 6 monstros',
    ]);
    expect(flat(el.querySelector('.mon .seg__item--on'))).toBe('Média');
    expect(el.querySelector('[role=switch]')?.getAttribute('aria-checked')).toBe('true');
    expect(flat(el.querySelector('#mon-title')?.parentElement?.querySelector('.sec__count')!)).toBe(
      '13 NPCs',
    );
    // The campaign's NPCs can join too (a boss next to the bestiary's monsters), but no second bestiary picker.
    expect(flat(el.querySelector('#npcs-title'))).toBe('NPCs da campanha');
    expect(el.querySelector('app-creature-pick')).toBeNull();
  });

  it("a campaign NPC (a boss) joins the saved encounter: it goes with the party, the monsters stay the encounter's", async () => {
    const { el, settle } = await setup();
    const orin = el.querySelector('app-npc-row');
    expect(flat(orin)).toContain('Orin');
    Array.from(orin!.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => (b.getAttribute('aria-label') ?? '').startsWith('Mais'))!
      .click();
    await settle();
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => flat(b) === 'Iniciar combate')!
      .click();
    await settle();
    expect(starts[0].participants).toEqual([
      expect.objectContaining({ characterId: 'pc-1' }),
      expect.objectContaining({ characterId: 'pc-2' }),
      expect.objectContaining({ characterId: 'npc-1', count: 1, hidden: true }),
    ]);
    expect(starts[0].extras['monsters']).toHaveLength(4);
    expect(starts[0].extras['mapPointId']).toBe('pt-1');
  });

  it('starts with the monsters, the average, hidden, the point and one key, and the party that is checked', async () => {
    const { el, settle } = await setup();
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => flat(b) === 'Iniciar combate')!
      .click();
    await settle();
    expect(starts).toHaveLength(1);
    expect(starts[0].name).toBe('Emboscada na ponte');
    expect(starts[0].participants).toEqual([
      expect.objectContaining({ characterId: 'pc-1' }),
      expect.objectContaining({ characterId: 'pc-2' }),
    ]);
    expect(starts[0].extras).toEqual({
      monsters: [
        { creatureKey: 'monster:ogre', count: 1 },
        { creatureKey: 'monster:bugbear', count: 2 },
        { creatureKey: 'monster:hobgoblin', count: 4 },
        { creatureKey: 'monster:goblin', count: 6 },
      ],
      mode: EncounterMode.GRID,
      monsterHp: 'average',
      monstersHidden: true,
      mapPointId: 'pt-1',
    });
    expect(starts[0].key).toMatch(/^[0-9a-f-]{36}$/);
    expect(close).toHaveBeenCalledTimes(1);
  });

  it('the master can still change it: rolled hit points, revealed, another name', async () => {
    const { el, settle } = await setup();
    const rolar = Array.from(el.querySelectorAll<HTMLLabelElement>('.mon .seg__item'))
      .find((l) => flat(l) === 'Rolar')!
      .querySelector('input')!;
    rolar.click();
    el.querySelector<HTMLButtonElement>('[role=switch]')!.click();
    const name = el.querySelector<HTMLInputElement>('.dlg__name input')!;
    name.value = 'A ponte cai';
    name.dispatchEvent(new Event('input'));
    await settle();
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => flat(b) === 'Iniciar combate')!
      .click();
    await settle();
    expect(starts[0].name).toBe('A ponte cai');
    expect(starts[0].extras).toMatchObject({
      monsterHp: 'rolled',
      monstersHidden: false,
      mapPointId: 'pt-1',
    });
  });

  it('a retry after a failure sends the same key, so a lost answer starts one combat', async () => {
    const { el, settle } = await setup();
    failures = [new Error('down')];
    const go = () =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find(
        (b) => flat(b) === 'Iniciar combate',
      )!;
    go().click();
    await settle();
    expect(close).not.toHaveBeenCalled();
    go().click();
    await settle();
    expect(starts).toHaveLength(2);
    expect(starts[1].key).toBe(starts[0].key);
  });

  it('the monsters count in the 40: a party and monsters past it leave the button waiting, with the reason', async () => {
    const { el } = await setup({
      groups: [{ key: 'monster:goblin', namePt: 'Goblin', count: 40 }],
    });
    expect(flat(el.querySelector('.dlg__why'))).toBe('Um combate tem no máximo 40 combatentes.');
    expect(
      isOff(
        Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find(
          (b) => flat(b) === 'Iniciar combate',
        )!,
      ),
    ).toBe(true);
  });

  it('from a battle point on a map with a grid the dialog opens on "Com mapa", whatever the table\'s rule says', async () => {
    const { el } = await setup();
    const radios = Array.from(
      el.querySelectorAll<HTMLInputElement>(
        'input[name^="dice-"], .mode input[type=radio], app-dice-choice input[type=radio]',
      ),
    );
    expect(radios.length).toBeGreaterThan(1);
    expect(radios.find((r) => r.checked)?.parentElement?.textContent).toContain('Com mapa');
  });

  it('"Sem mapa" sends the monsters with no map point', async () => {
    const { el, settle } = await setup();
    const radios = Array.from(
      el.querySelectorAll<HTMLInputElement>('app-dice-choice input[type=radio]'),
    );
    radios.find((r) => r.parentElement?.textContent?.includes('Sem mapa'))!.click();
    await settle();
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => flat(b) === 'Iniciar combate')!
      .click();
    await settle();
    expect(starts[0].extras['mode']).toBe(EncounterMode.THEATRE);
    expect(starts[0].extras['monsters']).toHaveLength(4);
  });
});
