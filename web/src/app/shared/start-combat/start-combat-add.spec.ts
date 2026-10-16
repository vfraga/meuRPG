import { TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';

import { DiceMode, DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterKind } from '../../../gen/meurpg/characters/v1/characters_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { TableRulesClient } from '../../core/campaigns/table-rules';
import { CombatClient } from '../../core/combat/combat-client';
import { encounter } from '../../core/combat/combat-testing';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { FakeCreaturesClient } from '../../core/creatures/creatures-testing';
import { GOBLIN } from '../../core/encounters/encounters-testing';
import { RosterClient } from '../../core/maps/roster-client';
import { CreaturePick } from '../creature-pick/creature-pick';
import { StartCombatDialog, type StartCombatData } from './start-combat-dialog';

interface AddMonstersCall {
  creatureKey: string;
  count: number;
  hp: string;
  hidden: boolean;
  key: string;
}

/** "Adicionar combatente" during a combat also adds the monsters picked from the bestiary (AddMonsters). */
describe('StartCombatDialog: monsters picked in add mode', () => {
  let adds: { specs: { characterId: string }[]; key: string }[];
  let monsterCalls: AddMonstersCall[];
  let failMonsters: number;
  let close: ReturnType<typeof vi.fn>;

  async function setup() {
    adds = [];
    monsterCalls = [];
    failMonsters = 0;
    close = vi.fn();
    const data: StartCombatData = {
      campaignId: 'camp-1',
      mode: 'add',
      map: null,
      encounterId: 'enc-1',
      existing: 2,
    };
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
        { provide: CreaturesClient, useValue: new FakeCreaturesClient() },
        {
          provide: RosterClient,
          useValue: {
            list: async () => [
              {
                id: 'npc-1',
                name: 'Guarda',
                kind: CharacterKind.MINION,
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
        {
          provide: CombatClient,
          useValue: {
            add: vi.fn(
              async (_c: string, _e: string, specs: { characterId: string }[], key: string) => {
                adds.push({ specs, key });
                return encounter({ id: 'enc-1', name: 'depois do NPC' });
              },
            ),
            addMonsters: vi.fn(
              async (
                _c: string,
                _e: string,
                add: { creatureKey: string; count: number; hp: string; hidden: boolean },
                key: string,
              ) => {
                monsterCalls.push({ ...add, key });
                if (failMonsters > 0) {
                  failMonsters--;
                  throw new Error('lost');
                }
                return {
                  encounter: encounter({ id: 'enc-1', name: 'depois dos monstros' }),
                  combatantIds: [],
                };
              },
            ),
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(StartCombatDialog);
    const settle = async () => {
      for (let i = 0; i < 6; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
      }
    };
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    const pick = () =>
      fixture.debugElement.query(By.directive(CreaturePick)).componentInstance as CreaturePick;
    const cmp = fixture.componentInstance as unknown as { setCount(id: string, n: number): void };
    const go = async () => {
      el.querySelector<HTMLButtonElement>('.dlg__go')!.click();
      await settle();
    };
    return { el, settle, pick, cmp, go };
  }

  it('two Goblins enable "Adicionar" and call addMonsters once with count 2, never add', async () => {
    const { el, settle, pick, go } = await setup();
    expect(el.querySelector('.dlg__go--off')).not.toBeNull();
    pick().picked.emit(GOBLIN);
    pick().picked.emit(GOBLIN);
    await settle();
    expect(el.querySelector('.dlg__go--off')).toBeNull();
    await go();
    expect(adds).toHaveLength(0);
    expect(monsterCalls).toHaveLength(1);
    expect(monsterCalls[0]).toMatchObject({
      creatureKey: 'monster:goblin',
      count: 2,
      hp: 'average',
      hidden: true,
    });
    expect(close).toHaveBeenCalledWith(expect.objectContaining({ name: 'depois dos monstros' }));
  });

  it('an NPC plus a monster calls both', async () => {
    const { settle, pick, cmp, go } = await setup();
    cmp.setCount('npc-1', 1);
    pick().picked.emit(GOBLIN);
    await settle();
    await go();
    expect(adds).toHaveLength(1);
    expect(adds[0].specs).toMatchObject([{ characterId: 'npc-1', count: 1 }]);
    expect(monsterCalls).toHaveLength(1);
  });

  it('a retry after a failure reuses the same keys', async () => {
    const { settle, pick, cmp, go } = await setup();
    cmp.setCount('npc-1', 1);
    pick().picked.emit(GOBLIN);
    await settle();
    failMonsters = 1;
    await go();
    expect(close).not.toHaveBeenCalled();
    await go();
    expect(adds).toHaveLength(2);
    expect(adds[1].key).toBe(adds[0].key);
    expect(monsterCalls).toHaveLength(2);
    expect(monsterCalls[1].key).toBe(monsterCalls[0].key);
    expect(close).toHaveBeenCalled();
  });
});
