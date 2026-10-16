import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';

import { DiceMode, DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterKind } from '../../../gen/meurpg/characters/v1/characters_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { TableRulesClient } from '../../core/campaigns/table-rules';
import { CombatClient, type JoinSpec } from '../../core/combat/combat-client';
import { encounter } from '../../core/combat/combat-testing';
import { RosterClient } from '../../core/maps/roster-client';
import { StartCombatDialog, type StartCombatData } from './start-combat-dialog';
import { nextTurn } from '../../../test-settle';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

const player = (id: string, name: string, reserved: boolean) => ({
  id,
  name,
  kind: CharacterKind.PLAYER,
  playerUserId: reserved ? '' : 'u-' + id,
  classSummary: 'Guerreiro 3',
  raceName: 'Humano',
  playerName: reserved ? null : 'Jogador',
  reserved,
});

describe('"Iniciar combate": a reserved character (no player yet) cannot fight', () => {
  it('leaves it out of the default selection, shows why and starts without it', async () => {
    const data: StartCombatData = { campaignId: 'c', mode: 'start', map: null };
    const sent: JoinSpec[][] = [];
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: () => undefined } },
        {
          provide: RosterClient,
          useValue: { list: async () => [player('a', 'Bruna', false), player('b', 'Caio', true)] },
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
        { provide: TableRulesClient, useValue: { get: async () => ({ saved: {} }) } },
        {
          provide: CombatClient,
          useValue: {
            start: async (_c: string, _n: string, specs: JoinSpec[]) => {
              sent.push(specs);
              return encounter();
            },
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(StartCombatDialog);
    fixture.detectChanges();
    await fixture.whenStable();
    await nextTurn();
    await fixture.whenStable();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;

    expect(plain(el.textContent)).toContain('1 de 1 no combate');
    expect(plain(el.querySelector('[data-testid="player-unavailable"]')?.textContent)).toBe(
      'Reservado: ainda sem jogador',
    );
    const boxes = Array.from(el.querySelectorAll<HTMLInputElement>('.list input[type="checkbox"]'));
    expect(boxes.map((b) => [b.checked, b.disabled])).toEqual([
      [true, false],
      [false, true],
    ]);

    el.querySelector<HTMLButtonElement>('.dlg__go')?.click();
    await fixture.whenStable();
    expect(sent).toEqual([[{ characterId: 'a' }]]);
  });
});
