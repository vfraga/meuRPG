// Review 12, finding U12-11: "Adicionar combatente" makes a new idempotency key per call (CombatClient.add), so a retry after a lost
// answer adds the NPCs twice. The server replays by key (play/combat_write.go), so one key per dialog would have been safe.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterKind } from '../../../../../gen/meurpg/characters/v1/characters_pb';
import { TableRulesClient } from '../../../../core/campaigns/table-rules';
import { CampaignsService } from '../../../../core/campaigns/campaigns.service';
import { CombatClient } from '../../../../core/combat/combat-client';
import { encounter } from '../../../../core/combat/combat-testing';
import { CONNECT_TRANSPORT } from '../../../../core/connect/transport';
import { flat } from '../../../../core/creatures/creatures-testing';
import { RosterClient } from '../../../../core/maps/roster-client';
import { StartCombatDialog } from './start-combat-dialog';

describe('Review12 U12-11: reinforcements retry keeps one idempotency key', () => {
  it('sends the same key when "Adicionar" is pressed again after a lost answer', async () => {
    const sent: { idempotencyKey: string }[] = [];
    const close = vi.fn();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        CombatClient,
        { provide: CONNECT_TRANSPORT, useValue: {} },
        {
          provide: MAT_DIALOG_DATA,
          useValue: { campaignId: 'camp-1', mode: 'add', map: null, encounterId: 'enc-1', existing: 2 },
        },
        { provide: MatDialogRef, useValue: { close } },
        {
          provide: RosterClient,
          useValue: {
            list: async () => [
              {
                id: 'npc-1',
                name: 'Goblin',
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
            listMembers: async () => ({ members: [{ userId: 'u1', role: 0, dicePreference: DicePreference.APP }] }),
            getCampaign: async () => ({ campaign: { diceMode: DiceMode.PLAYERS_CHOOSE } }),
          },
        },
        { provide: TableRulesClient, useValue: { get: async () => ({ saved: { combatStartsWithMap: false } }) } },
      ],
    });
    const client = TestBed.inject(CombatClient);
    (client as unknown as { client: unknown }).client = {
      addCombatants: async (req: { idempotencyKey: string }) => {
        sent.push(req);
        if (sent.length === 1) {
          throw new ConnectError('timeout', Code.Unavailable); // committed, but the answer was lost
        }
        return { encounter: encounter({ id: 'enc-1' }) };
      },
    };
    const fixture = TestBed.createComponent(StartCombatDialog);
    const settle = async () => {
      for (let i = 0; i < 5; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
      }
    };
    await settle();
    (fixture.componentInstance as unknown as { setCount(id: string, n: number): void }).setCount('npc-1', 3);
    await settle();
    const press = async () => {
      Array.from((fixture.nativeElement as HTMLElement).querySelectorAll<HTMLButtonElement>('button'))
        .find((b) => /Adicionar/.test(flat(b) ?? ""))!
        .click();
      await settle();
    };
    await press();
    await press();
    expect(sent).toHaveLength(2);
    expect(sent[1].idempotencyKey).toBe(sent[0].idempotencyKey);
  });
});
