import { TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';

import { DiceMode, DicePreference, Role } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterKind } from '../../../gen/meurpg/characters/v1/characters_pb';
import { EncounterMode } from '../../../gen/meurpg/play/v1/combat_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { TableRulesClient } from '../../core/campaigns/table-rules';
import { CombatClient } from '../../core/combat/combat-client';
import { encounter } from '../../core/combat/combat-testing';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { FakeCreaturesClient, flat } from '../../core/creatures/creatures-testing';
import { BANDIT } from '../../core/encounters/encounters-testing';
import { RosterClient } from '../../core/maps/roster-client';
import { CreaturePick } from '../creature-pick/creature-pick';
import { StartCombatDialog, type StartCombatData } from './start-combat-dialog';
import { fakeCombatClient } from '../../core/combat/combat-client-testing';

/** R4: a bestiary monster is picked inside "Iniciar combate", so the dialog is never left. */
describe('StartCombatDialog: monsters picked from the bestiary in place', () => {
  let starts: {
    participants: { characterId: string }[];
    key: string;
    extras: Record<string, unknown>;
  }[];

  async function setup() {
    starts = [];
    const data: StartCombatData = {
      campaignId: 'camp-1',
      mode: 'start',
      map: {
        id: 'map-1',
        name: 'Estrada',
        image: { url: '/i', width: 2400, height: 1600 },
        columns: 24,
        rows: 16,
      },
    };
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
        { provide: CreaturesClient, useValue: new FakeCreaturesClient() },
        {
          provide: RosterClient,
          useValue: {
            list: async () => [
              {
                id: 'pc-1',
                name: 'Toren',
                kind: CharacterKind.PLAYER,
                playerUserId: 'u1',
                classSummary: '',
                raceName: '',
                playerName: 'Rui',
              },
            ],
          },
        },
        {
          provide: CampaignsService,
          useValue: {
            listMembers: async () => ({
              members: [{ userId: 'u1', role: Role.PLAYER, dicePreference: DicePreference.APP }],
            }),
            getCampaign: async () => ({ campaign: { diceMode: DiceMode.PLAYERS_CHOOSE } }),
          },
        },
        {
          provide: TableRulesClient,
          useValue: { get: async () => ({ saved: { combatStartsWithMap: true } }) },
        },
        {
          provide: CombatClient,
          useValue: fakeCombatClient({
            start: vi.fn(
              async (
                _c: string,
                name: string,
                participants: { characterId: string }[],
                key: string,
                extras: Record<string, unknown>,
              ) => {
                starts.push({ participants, key, extras });
                return encounter({ id: 'enc-9', name });
              },
            ),
          }),
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
    return { el, settle, pick };
  }

  it('has no link to the bestiary: the picker is in the dialog', async () => {
    const { el } = await setup();
    expect(el.querySelector('a[href*="bestiary"]')).toBeNull();
    expect(el.querySelector('app-creature-pick')).not.toBeNull();
  });

  it('picking a creature lists it, shows how the monsters start, and starts with them and the chosen mode', async () => {
    const { el, settle, pick } = await setup();
    pick().picked.emit(BANDIT);
    await settle();
    expect(flat(el.querySelector('.pick__name'))).toBe('Bandido');
    expect(el.querySelector('.mon')).not.toBeNull();
    el.querySelector<HTMLInputElement>('input[type=radio]')!; // the mode choice is still there
    const theatre = Array.from(el.querySelectorAll<HTMLInputElement>('app-dice-choice input')).at(
      1,
    )!;
    theatre.click();
    await settle();
    el.querySelector<HTMLButtonElement>('.dlg__go')!.click();
    await settle();
    expect(starts).toHaveLength(1);
    expect(starts[0].extras).toMatchObject({
      mode: EncounterMode.THEATRE,
      monsters: [{ creatureKey: 'monster:bandit', count: 1, name: '' }],
      monsterHp: 'average',
      monstersHidden: true,
    });
  });

  it('a monster alone is enough to start, even with the whole party unchecked, and "Tirar" takes it out', async () => {
    const { el, settle, pick } = await setup();
    el.querySelector<HTMLInputElement>('app-player-row input')!.click();
    await settle();
    expect(el.querySelector('.dlg__go--off')).not.toBeNull();
    pick().picked.emit(BANDIT);
    await settle();
    expect(el.querySelector('.dlg__go--off')).toBeNull();
    el.querySelector<HTMLButtonElement>('.pick__row button[aria-label^="Tirar"]')!.click();
    await settle();
    expect(el.querySelector('.pick__row')).toBeNull();
    expect(el.querySelector('.dlg__why')).not.toBeNull();
  });
});
