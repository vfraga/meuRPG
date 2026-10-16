// The class resource actions open their own dialogs (TakeAction refuses them), the die card shows, and a held roll
// reopens its question once (the halfling's Lucky first, else the Bardic Inspiration die).
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantSide,
  CombatantState,
  DiceRollSchema,
  EncounterMode,
  InspirationDieSchema,
  InspirationOfferSchema,
  LuckyOfferSchema,
  ResourceTargetSchema,
  ResourceTargetsSchema,
  TargetInReachSchema,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  ActionOptionSchema,
  ActionSchema,
  AttackOptionSchema,
  AttackSchema,
  TurnOptionsSchema,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import { CombatClient } from '../../../core/combat/combat-client';
import { CombatState } from '../../../core/combat/combat-state';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { AttackSheet } from './attack-sheet/attack-sheet';
import { BardicInspirationSheet } from './bardic-inspiration-sheet/bardic-inspiration-sheet';
import { CombatView } from './combat-view';
import { FlexibleCastingSheet } from './flexible-casting-sheet/flexible-casting-sheet';
import { LayOnHandsSheet } from './lay-on-hands-sheet/lay-on-hands-sheet';
import { anyCallCombatClient } from '../../../core/combat/combat-client-testing';

const feature = (key: string, namePt: string, economy: ActionEconomy) =>
  create(ActionOptionSchema, {
    enabled: true,
    action: create(ActionSchema, { key, namePt, economy }),
  });

const longsword = create(AttackSchema, {
  key: 'attack:longsword',
  namePt: 'Espada longa',
  attackBonus: 5,
  rangeFt: 5,
});

const reach = (id: string, label: string) =>
  create(ResourceTargetSchema, {
    target: create(TargetInReachSchema, {
      combatantId: id,
      label,
      state: CombatantState.HURT,
      distanceFt: 5,
    }),
  });

const turnOptions = {
  options: create(TurnOptionsSchema, {
    featureActions: [
      feature('feature:lay-on-hands', 'Cura pelas Mãos', ActionEconomy.ACTION),
      feature(
        'feature:flexible-casting-creating-spell-slots',
        'Conjuração Flexível: criar espaço',
        ActionEconomy.BONUS_ACTION,
      ),
      feature(
        'feature:flexible-casting-converting-spell-slot',
        'Conjuração Flexível: converter espaço',
        ActionEconomy.BONUS_ACTION,
      ),
      feature('feature:bardic-inspiration-d6', 'Inspiração de Bardo', ActionEconomy.BONUS_ACTION),
    ],
    attacks: [create(AttackOptionSchema, { attack: longsword })],
  }),
  attackTargets: [],
  spellTargets: [],
  pendingDamages: [],
  resourceTargets: [
    create(ResourceTargetsSchema, {
      actionKey: 'feature:lay-on-hands',
      targets: [reach('b', 'Brisa'), reach('t', 'Toren')],
    }),
    create(ResourceTargetsSchema, {
      actionKey: 'feature:bardic-inspiration-d6',
      targets: [reach('b', 'Brisa')],
    }),
  ],
};

describe('CombatView: the class resource dialogs', () => {
  const takeAction = vi.fn();
  const open = vi.fn();

  const die = create(InspirationDieSchema, {
    sides: 8,
    fromLabel: 'Orla',
    fromCombatantId: 'o',
    expiresAtRound: 82,
  });
  const offer = create(InspirationOfferSchema, {
    holdId: 'hold-1',
    die,
    attackKey: 'attack:longsword',
    targetId: 'g',
    d20: create(DiceRollSchema, {
      diceCount: 1,
      diceSides: 20,
      faces: [9],
      modifier: 5,
      total: 14,
    }),
  });

  const fresh = (revision: number, own: Record<string, unknown> = {}) =>
    encounter({
      mode: EncounterMode.THEATRE,
      mapId: '',
      gridColumns: 0,
      gridRows: 0,
      revision,
      round: 2,
      currentCombatantId: 't',
      combatants: [
        combatant({
          id: 't',
          label: 'Toren',
          kind: CombatantKind.PLAYER,
          side: CombatantSide.PARTY,
          characterId: 'toren-c',
          mine: true,
          placed: false,
          ...own,
        }),
        combatant({ id: 'g', label: 'Goblin' }),
      ],
    });

  async function render(own: Record<string, unknown> = {}) {
    const api = anyCallCombatClient((name) =>
      name === 'turnOptions'
        ? async () => turnOptions
        : name === 'takeAction'
          ? takeAction
          : name === 'log'
            ? async () => ({ rounds: [], undoableEventId: '' })
            : async () => ({}),
    );
    const state = new CombatState();
    state.apply(fresh(1, own));
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: CombatClient, useValue: api },
        { provide: RosterClient, useValue: { list: async () => [] } },
        { provide: MapsClient, useValue: { layers: async () => ({}) } },
        { provide: SpellCatalog, useValue: { details: async () => null } },
        {
          provide: CreaturesClient,
          useValue: {
            list: async () => [],
            statBlock: async () => null,
            summonOptions: async () => [],
          },
        },
        {
          provide: TableRulesClient,
          useValue: { get: async () => ({ saved: { deathSaves: 0 } }) },
        },
        {
          provide: MatDialog,
          useValue: {
            open: (...args: unknown[]) => {
              open(...args);
              return { afterClosed: () => ({ subscribe: () => undefined }) };
            },
          },
        },
        {
          provide: MatBottomSheet,
          useValue: { open: () => ({ afterDismissed: () => ({ subscribe: () => undefined }) }) },
        },
      ],
    });
    const fixture = TestBed.createComponent(CombatView);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput('isMaster', false);
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('mapState', new MapState(async () => ({}) as never));
    fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
    fixture.componentRef.setInput('dicePreference', DicePreference.APP);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const view = fixture.componentInstance as unknown as {
      useFeature(key: string): Promise<void>;
    };
    return { fixture, state, view, el: fixture.nativeElement as HTMLElement };
  }

  beforeEach(() => {
    takeAction.mockReset();
    open.mockReset();
    TestBed.resetTestingModule();
  });

  it('opens Cura pelas Mãos with the targets the server listed, and sends no TakeAction', async () => {
    const { view } = await render();
    await view.useFeature('feature:lay-on-hands');
    expect(takeAction).not.toHaveBeenCalled();
    const [component, config] = open.mock.calls[0];
    expect(component).toBe(LayOnHandsSheet);
    expect(config.data.actorId).toBe('t');
    expect(config.data.targets.map((t: { target: { label: string } }) => t.target.label)).toEqual([
      'Brisa',
      'Toren',
    ]);
  });

  it('opens Conjuração Flexível in the direction of each row', async () => {
    const { view } = await render();
    await view.useFeature('feature:flexible-casting-creating-spell-slots');
    await view.useFeature('feature:flexible-casting-converting-spell-slot');
    expect(open.mock.calls.map((c) => [c[0], c[1].data.direction])).toEqual([
      [FlexibleCastingSheet, 'create'],
      [FlexibleCastingSheet, 'convert'],
    ]);
    expect(takeAction).not.toHaveBeenCalled();
  });

  it('opens Inspiração de Bardo with its own targets (not the paladin touch list)', async () => {
    const { view } = await render();
    await view.useFeature('feature:bardic-inspiration-d6');
    const [component, config] = open.mock.calls[0];
    expect(component).toBe(BardicInspirationSheet);
    expect(config.data.targets).toHaveLength(1);
    expect(takeAction).not.toHaveBeenCalled();
  });

  it('shows the die card of the character that holds a die: the die, who gave it and the time left', async () => {
    const { el } = await render({ inspirationDie: die });
    const card = el.querySelector('app-inspiration-card');
    expect(card?.querySelector('.card__title')?.textContent).toBe('Inspiração de Bardo: d8');
    expect(card?.querySelector('.card__sub')?.textContent?.replace(/\s+/g, ' ').trim()).toBe(
      'de Orla · até 8 min',
    );
  });

  it('shows no card for a character without a die', async () => {
    const { el } = await render();
    expect(el.querySelector('app-inspiration-card')).toBeNull();
  });

  it('opens the question of a held roll by itself, once, and says in the page that it waits', async () => {
    const { fixture, state, el } = await render({ inspirationOffer: offer });
    const sheets = () => open.mock.calls.filter((c) => c[0] === AttackSheet);
    expect(sheets()).toHaveLength(1);
    expect(sheets()[0][1].data.inspiration.offer.holdId).toBe('hold-1');
    expect(sheets()[0][1].data.inspiration.targetLabel).toBe('Goblin');
    expect(el.querySelector('[data-testid=inspiration-pending]')).not.toBeNull();
    // Reading the combat again with the same hold does not open a second question.
    state.apply(fresh(2, { inspirationOffer: offer }));
    fixture.detectChanges();
    await fixture.whenStable();
    expect(sheets()).toHaveLength(1);
  });

  it('opens the Lucky question of a held roll by itself (a halfling’s d20 of 1), and says in the page that it waits', async () => {
    const lucky = create(LuckyOfferSchema, {
      holdId: 'hold-2',
      attackKey: 'attack:longsword',
      targetId: 'g',
      luckyIndex: 0,
      d20: create(DiceRollSchema, {
        diceCount: 1,
        diceSides: 20,
        faces: [1],
        modifier: 5,
        total: 6,
      }),
    });
    const { el } = await render({ luckyOffer: lucky });
    const sheets = open.mock.calls.filter((c) => c[0] === AttackSheet);
    expect(sheets).toHaveLength(1);
    expect(sheets[0][1].data.lucky.offer.holdId).toBe('hold-2');
    expect(sheets[0][1].data.lucky.targetLabel).toBe('Goblin');
    expect(sheets[0][1].data.inspiration).toBeUndefined();
    expect(el.querySelector('[data-testid=lucky-pending]')).not.toBeNull();
    expect(el.querySelector('[data-testid=inspiration-pending]')).toBeNull();
  });

  it('does not open a question for a character with no held roll', async () => {
    await render();
    expect(open.mock.calls.filter((c) => c[0] === AttackSheet)).toHaveLength(0);
  });
});
