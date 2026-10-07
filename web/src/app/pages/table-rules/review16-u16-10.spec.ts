// Finding U16-10 in review/unit-16-web-content-campaigns.md
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { of } from 'rxjs';

import {
  CriticalRule,
  DeathSaveVisibility,
  DiceMode,
  HitPointsRule,
  Role,
  TableStyle,
  TableStylePresetSchema,
  XpMode,
} from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import {
  type RulesDraft,
  TableRulesClient,
  type TableRulesVm,
} from '../../core/campaigns/table-rules';
import { TableRulesPage } from './table-rules';

const presets = [
  create(TableStylePresetSchema, {
    style: TableStyle.TUDO_NO_APP,
    diceMode: DiceMode.APP,
    combatStartsWithMap: true,
    fogOnNewMaps: true,
  }),
];

const saved: RulesDraft = {
  diceMode: DiceMode.PLAYERS_CHOOSE,
  combatStartsWithMap: true,
  fogOnNewMaps: true,
  hitPoints: HitPointsRule.PLAYER_CHOOSES,
  standardArray: true,
  pointBuy: true,
  rolled4d6: true,
  typed: true,
  critical: CriticalRule.DOUBLED_DICE,
  deathSaves: DeathSaveVisibility.VISIBLE_TO_ALL,
  houseRules: ['Beber uma poção é uma ação bônus'],
};

const vm: TableRulesVm = {
  saved,
  style: TableStyle.PERSONALIZADO,
  presets,
  standardArray: [15, 14, 13, 12, 10, 8],
  pointBuyCosts: [0, 1, 2, 3, 4, 5, 7, 9],
  pointBuyMinScore: 8,
  pointBuyBudget: 27,
  typedMin: 3,
  typedMax: 18,
};

describe('Review16 U16-10: an edit made while the save is in flight is not lost', () => {
  async function settle(fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) {
    for (let i = 0; i < 3; i++) {
      fixture.detectChanges();
      await new Promise((r) => setTimeout(r));
      await fixture.whenStable();
    }
    fixture.detectChanges();
  }

  it('keeps the later edit in the draft and still counts it as unsaved', async () => {
    const get = vi.fn().mockResolvedValue(vm);
    let resolveSet!: (v: unknown) => void;
    const set = vi.fn().mockImplementation(
      (_id: string, d: RulesDraft) =>
        new Promise((r) => {
          resolveSet = () => r({ saved: d, style: TableStyle.PERSONALIZADO });
        }),
    );
    const getCampaign = vi.fn().mockResolvedValue({
      campaign: {
        id: 'camp-1',
        name: 'Mirathel',
        myRole: Role.MASTER,
        awaitingApproval: false,
        xpMode: XpMode.ENEMIES,
      },
    });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: ActivatedRoute,
          useValue: { paramMap: of(convertToParamMap({ id: 'camp-1' })) },
        },
        { provide: CampaignsService, useValue: { getCampaign } },
        { provide: TableRulesClient, useValue: { get, set, setXpMode: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(TableRulesPage);
    fixture.detectChanges();
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    const text = () => (el.textContent ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ');
    const button = (label: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.replace(/\s+/g, ' ').trim().includes(label),
      )!;

    // A first change, so there is something to save.
    const avg = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]')).find((i) =>
      i.closest('label')?.textContent?.includes('A média'),
    )!;
    avg.click();
    await settle(fixture);
    button('Salvar regras').click();
    await settle(fixture);
    expect(set).toHaveBeenCalledTimes(1);

    // While the call is in flight, the master edits a reminder.
    const field = el.querySelector<HTMLInputElement>('.reminder input')!;
    field.value = 'Lembrete novo escrito durante o salvamento';
    field.dispatchEvent(new Event('input'));
    await settle(fixture);

    // The server answers with the draft that was sent (without the later edit).
    resolveSet(null);
    await settle(fixture);

    const input = el.querySelector<HTMLInputElement>('.reminder input')!;
    expect(input.value).toBe('Lembrete novo escrito durante o salvamento');
    expect(text()).toContain('1 mudança não salva');
    expect(text()).not.toContain('Regras salvas.');
  });
});
