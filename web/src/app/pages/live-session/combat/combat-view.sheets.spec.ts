import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { Observable } from 'rxjs';
import { vi } from 'vitest';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CreatureSource } from '../../../../gen/meurpg/characters/v1/characters_pb';
import { MapSchema } from '../../../../gen/meurpg/maps/v1/maps_pb';
import {
  AreaPlacement,
  CombatantKind,
  CombatantSide,
  type Encounter,
  EncounterMode,
  GetTurnOptionsResponseSchema,
  PendingDamageStatus,
  ReactionKind,
  ReactionWindowStatus,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  AttackKind,
  SpellDetailsSchema,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter, reactionWindow } from '../../../core/combat/combat-testing';
import { settle } from '../../../../test-settle';
import { PHONE_QUERY } from '../../../shared/map-view/media-query';
import { spellDetailsFromGen } from '../../../shared/spell-details/spell-details-map';
import { FamiliarEyesSheet } from '../../../shared/familiar-eyes/familiar-eyes-sheet';
import { SpellDetails } from '../../../shared/spell-details/spell-details';
import { SummonSheet } from '../../../shared/summon-sheet/summon-sheet';
import { WildShapeSheet } from '../../../shared/wild-shape/wild-shape-sheet';
import type { VitalsVm } from '../../../core/play/live-session.types';
import { MapState } from '../../../core/maps/map-state';
import { EffectSaveSheet } from '../effects/effect-save-sheet/effect-save-sheet';
import { DoorSheet } from '../door-sheet/door-sheet';
import { TrapSearchSheet } from '../traps/trap-search-sheet/trap-search-sheet';
import { MasterAreaCast } from './area-cast/master-area-cast';
import { AdjustNpc } from './adjust-npc/adjust-npc';
import { AttackSheet } from './attack-sheet/attack-sheet';
import { BardicInspirationSheet } from './bardic-inspiration-sheet/bardic-inspiration-sheet';
import { CastSheet } from './cast-sheet/cast-sheet';
import { type MountOptions, mountCombatView } from './combat-view-testing';
import { ConditionsDialog } from './conditions-dialog/conditions-dialog';
import { FeatureSheet } from './feature-sheet/feature-sheet';
import { FlexibleCastingSheet } from './flexible-casting-sheet/flexible-casting-sheet';
import { HelpSheet } from './help-sheet/help-sheet';
import { HideSheet } from './hide-sheet/hide-sheet';
import { LayOnHandsSheet } from './lay-on-hands-sheet/lay-on-hands-sheet';
import { ReactionSheet } from './reaction-sheet/reaction-sheet';
import { RevivifySheet } from './revivify-sheet/revivify-sheet';
import { SpendSheet } from './theatre/spend-sheet';

// The data each sheet and dialog of the combat page is handed, field by field, and the calls the page makes before or instead
// of opening one: the contract the page's sheet builders keep when they are moved around.

const PHONE = [PHONE_QUERY];

const toren = (over: object = {}) =>
  combatant({
    id: 't',
    label: 'Toren',
    kind: CombatantKind.PLAYER,
    side: CombatantSide.PARTY,
    characterId: 'toren-c',
    mine: true,
    placed: false,
    ...over,
  } as never);
const goblin = (over: object = {}) =>
  combatant({ id: 'g', label: 'Goblin', placed: false, ...over } as never);
const wolf = (over: object = {}) =>
  combatant({
    id: 'w',
    label: 'Lobo',
    kind: CombatantKind.CREATURE,
    side: CombatantSide.PARTY,
    controlledByMe: true,
    ownerCharacterId: 'toren-c',
    placed: false,
    ...over,
  } as never);

/** A combat without a map (theatre of the mind), round 3, with Toren on turn. */
const theatre = (over: object = {}, combatants = [toren(), goblin()]): Encounter =>
  encounter({
    mode: EncounterMode.THEATRE,
    mapId: '',
    gridColumns: 0,
    gridRows: 0,
    round: 3,
    currentCombatantId: 't',
    combatants,
    ...over,
  } as never);

/** The master's combat: none of the combatants is his own, so he has no character of his own to act for. */
const masterEnc = () => theatre({ currentCombatantId: 'g' }, [toren({ mine: false }), goblin()]);

type TurnInit = Record<string, unknown>;
type OptionsInit = Record<string, unknown>;
/** `GetTurnOptions` answers by combatant id. */
const turns = (byId: Record<string, TurnInit>) => (id: string) =>
  byId[id] ? create(GetTurnOptionsResponseSchema, byId[id] as never) : undefined;
/** The options of Toren (and what goes with them), keyed for the harness. */
const forToren = (options: OptionsInit, rest: TurnInit = {}) => turns({ t: { ...rest, options } });

const sword = {
  key: 'attack:longsword',
  name: 'Longsword',
  namePt: 'Espada longa',
  kind: AttackKind.WEAPON,
  melee: true,
  attackBonus: 5,
  rangeFt: 5,
};
const swordTargets = {
  attackKey: 'attack:longsword',
  targets: [{ combatantId: 'g', label: 'Goblin', distanceFt: 5 }],
};
const pending = (over: object = {}) => ({
  id: 'pd1',
  attackerId: 't',
  targetId: 'g',
  attackKey: 'attack:longsword',
  status: PendingDamageStatus.AWAITING_ROLL,
  ...over,
});
const feature = (key: string, namePt: string, over: object = {}) => ({
  enabled: true,
  action: { key, namePt, economy: ActionEconomy.ACTION, ...over },
});

const vitals = (over: Partial<VitalsVm> = {}): VitalsVm => ({
  characterId: 'toren-c',
  name: 'Toren',
  playerUserId: 'u',
  hitPointsCurrent: 20,
  hitPointsMax: 20,
  hitPointsTemporary: 0,
  spellSlots: [],
  pactSlots: null,
  hitDice: '3d8',
  hitDiceSizes: [],
  hitDiceTotal: 3,
  hitDiceUsed: 0,
  revision: 1,
  ...over,
});

async function setup(opts: Partial<MountOptions> = {}) {
  const m = mountCombatView({ enc: theatre(), ...opts });
  await settle(m.fixture, 2);
  return m;
}

type Mounted = Awaited<ReturnType<typeof setup>>;
const data = (m: Mounted, i = 0) => m.opened[i].config.data as Record<string, unknown>;

/** A combat on a map with a grid: Toren and the Goblin stand on it. */
const onMap = (over: object = {}) =>
  theatre(
    {
      mode: EncounterMode.UNSPECIFIED,
      mapId: 'map',
      gridColumns: 20,
      gridRows: 14,
      ...over,
    },
    [toren({ placed: true }), goblin({ placed: true, col: 4, row: 4 })],
  );
const mapWithImage = () => {
  const mapState = new MapState(async () => ({}) as never);
  mapState.map.set(
    create(MapSchema, {
      id: 'map',
      name: 'Estrada na floresta',
      gridColumns: 20,
      image: { url: '/m.png', width: 1000, height: 700 },
    }),
  );
  return mapState;
};

describe('CombatView sheets: Ver pelos olhos / Forma Selvagem', () => {
  const nanquim = {
    list: async () => [{ source: CreatureSource.FAMILIAR, name: 'Nanquim' }],
  };

  it('asks "Ver pelos olhos do Nanquim?" with the character, the familiar and the combat flag', async () => {
    const m = await setup({ matches: PHONE, creatures: nanquim });
    m.view.askFamiliarEyes();
    expect(m.opened).toHaveLength(1);
    expect(m.opened[0].component).toBe(FamiliarEyesSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Ver pelos olhos do Nanquim?');
    expect(data(m)).toEqual({
      campaignId: 'c',
      characterId: 'toren-c',
      characterName: 'Toren',
      familiarName: 'Nanquim',
      inCombat: true,
    });
  });

  it('opens nothing when the character has no familiar', async () => {
    const m = await setup({ matches: PHONE });
    m.view.askFamiliarEyes();
    expect(m.opened).toHaveLength(0);
  });

  it('opens nothing for the master, who plays no character', async () => {
    const m = await setup({ master: true, creatures: nanquim });
    m.view.askFamiliarEyes();
    expect(m.opened).toHaveLength(0);
  });

  const wildShape = (used: number) =>
    vitals({
      resources: [{ key: 'wild_shape', total: 2, used, recharge: 'long_rest' }],
    });

  it('opens Forma Selvagem with the uses that are left of the resource', async () => {
    const m = await setup({ matches: PHONE, inputs: { vitals: [wildShape(1)] } });
    m.view.openTransform();
    expect(m.opened[0].component).toBe(WildShapeSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Forma Selvagem');
    expect(data(m)).toEqual({
      campaignId: 'c',
      characterId: 'toren-c',
      inCombat: true,
      uses: { left: 1, total: 2 },
    });
  });

  it('gives the sheet no uses when the character has no Wild Shape resource', async () => {
    const m = await setup({ inputs: { vitals: [vitals()] } });
    m.view.openTransform();
    expect(data(m)['uses']).toBeNull();
  });

  it('gives the sheet no uses when the vitals on the page are another character\u2019s', async () => {
    const m = await setup({
      inputs: { vitals: [{ ...wildShape(0), characterId: 'orla-c' }] },
    });
    m.view.openTransform();
    expect(data(m)['uses']).toBeNull();
  });

  it('gives the sheet no uses when the vitals are not known yet', async () => {
    const m = await setup();
    m.view.openTransform();
    expect(data(m)['uses']).toBeNull();
  });

  it('applies the combat the transformation answers, and ignores a result without one', async () => {
    const m = await setup({ inputs: { vitals: [wildShape(0)] } });
    m.view.openTransform();
    m.opened[0].close({ beastNamePt: 'Lobo' });
    expect(m.view.encounter().round).toBe(3);
    m.view.openTransform();
    m.opened[1].close({ beastNamePt: 'Lobo', encounter: theatre({ revision: 9, round: 7 }) });
    expect(m.view.encounter().round).toBe(7);
  });

  it('has no vitals of its own for a master with no character', async () => {
    const m = await setup({ master: true, enc: masterEnc(), inputs: { vitals: [vitals()] } });
    expect(m.view.ownVitals()).toBeNull();
  });

  it('opens nothing for the master', async () => {
    const m = await setup({ master: true, enc: masterEnc() });
    m.view.openTransform();
    expect(m.opened).toHaveLength(0);
  });
});

describe('CombatView sheets: the standard actions', () => {
  const hideActions = [{ key: 'standard:hide', economy: ActionEconomy.ACTION, enabled: true }];

  it('opens Hide for the character when the turn lists Hide, and spends nothing by itself', async () => {
    const m = await setup({
      turnOptions: forToren({}, { contestState: { hideActions } }),
    });
    m.view.standardAction('standard:hide');
    expect(m.opened[0].component).toBe(HideSheet);
    expect(data(m)).toMatchObject({
      campaignId: 'c',
      encounterId: 'enc',
      combatantId: 't',
      actionKey: 'standard:hide',
      economy: 'Ação',
    });
    expect(m.callsOf('takeAction')).toHaveLength(0);
  });

  it('opens Help for the character, with no plain action', async () => {
    const m = await setup();
    m.view.standardAction('standard:help');
    expect(m.opened[0].component).toBe(HelpSheet);
    expect(data(m)['helperId']).toBe('t');
    expect(m.callsOf('takeAction')).toHaveLength(0);
  });

  it('opens the trap search when the combat map has a grid and the character stands on it', async () => {
    const mapState = new MapState(async () => ({}) as never);
    mapState.map.set(create(MapSchema, { id: 'map', gridColumns: 20 }));
    const skills = { perception: 4, investigation: 1 };
    const m = await setup({
      matches: PHONE,
      mapState,
      enc: theatre({}, [toren({ placed: true }), goblin()]),
      inputs: { trapSkills: skills },
    });
    m.view.standardAction('standard:search');
    expect(m.opened[0].component).toBe(TrapSearchSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Procurar armadilhas');
    expect(data(m)).toMatchObject({
      campaignId: 'c',
      skills,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      inCombat: true,
    });
    expect(data(m)['state']).toBe(mapState);
    expect(m.callsOf('takeAction')).toHaveLength(0);
  });

  it('gives the trap search no skills when the sheet has not told them', async () => {
    const mapState = new MapState(async () => ({}) as never);
    mapState.map.set(create(MapSchema, { id: 'map', gridColumns: 20 }));
    const m = await setup({ mapState, enc: theatre({}, [toren({ placed: true }), goblin()]) });
    m.view.standardAction('standard:search');
    expect(data(m)['skills']).toBeNull();
  });

  it('takes the plain Search action when the character is not on the map', async () => {
    const mapState = new MapState(async () => ({}) as never);
    mapState.map.set(create(MapSchema, { id: 'map', gridColumns: 20 }));
    const m = await setup({ mapState });
    m.view.standardAction('standard:search');
    await settle(m.fixture);
    expect(m.opened).toHaveLength(0);
    expect(m.callsOf('takeAction').map((c) => c.args)).toEqual([
      ['c', 'enc', 't', 'standard:search'],
    ]);
  });

  it('takes the plain Search action when the map has no grid or there is no map', async () => {
    const m = await setup({ enc: theatre({}, [toren({ placed: true }), goblin()]) });
    m.view.standardAction('standard:search');
    await settle(m.fixture);
    expect(m.opened).toHaveLength(0);
    expect(m.callsOf('takeAction')).toHaveLength(1);
  });

  it('takes any other standard action as a call that spends it', async () => {
    const m = await setup();
    m.view.standardAction('standard:dash');
    await settle(m.fixture);
    expect(m.opened).toHaveLength(0);
    expect(m.callsOf('takeAction').map((c) => c.args)).toEqual([
      ['c', 'enc', 't', 'standard:dash'],
    ]);
  });

  it('takes nothing when the player has no character in the combat', async () => {
    const m = await setup({ master: true, enc: masterEnc() });
    m.view.standardAction('standard:dash');
    m.view.standardAction('standard:search');
    await settle(m.fixture);
    expect(m.opened).toHaveLength(0);
    expect(m.callsOf('takeAction')).toHaveLength(0);
  });

  it('spends the extra action of an effect with the extra flag set', async () => {
    const m = await setup();
    m.view.takeExtra('standard:dash');
    await settle(m.fixture);
    expect(m.callsOf('takeAction').map((c) => c.args)).toEqual([
      ['c', 'enc', 't', 'standard:dash', undefined, undefined, true],
    ]);
  });

  it('spends no extra action for the master', async () => {
    const m = await setup({ master: true, enc: masterEnc() });
    m.view.takeExtra('standard:dash');
    await settle(m.fixture);
    expect(m.callsOf('takeAction')).toHaveLength(0);
  });
});

describe('CombatView sheets: the standard actions of a creature', () => {
  const lobo = (over: TurnInit = {}) =>
    turns({
      w: {
        options: {},
        contestState: {
          hideActions: [
            { key: 'standard:hide', economy: ActionEconomy.BONUS_ACTION, enabled: true },
          ],
          hideOption: {},
        },
        ...over,
      },
    });
  const mount = () => setup({ enc: theatre({}, [toren(), wolf(), goblin()]), turnOptions: lobo() });

  it('opens Help for the creature as the helper', async () => {
    const m = await mount();
    m.view.creatureAction('w', 'standard:help');
    expect(m.opened[0].component).toBe(HelpSheet);
    expect(data(m)['helperId']).toBe('w');
    expect(m.callsOf('takeAction')).toHaveLength(0);
  });

  it("opens Hide for the creature, with the creature's own option and economy", async () => {
    const m = await mount();
    m.view.creatureAction('w', 'standard:hide');
    expect(m.opened[0].component).toBe(HideSheet);
    expect(data(m)).toMatchObject({
      combatantId: 'w',
      actionKey: 'standard:hide',
      economy: 'Ação bônus',
    });
    expect(data(m)['option']).toBe(m.view.creatureOptions.data().get('w').contestState.hideOption);
  });

  it('opens Help and Hide for a creature whose options were not read, with none to hand over', async () => {
    const m = await mount();
    m.view.creatureAction('far', 'standard:help');
    m.view.creatureAction('far', 'standard:hide');
    expect(m.opened.map((o) => o.component)).toEqual([HelpSheet, HideSheet]);
    expect(data(m, 0)['helperId']).toBe('far');
    expect(data(m, 1)).toMatchObject({ combatantId: 'far', economy: 'Ação' });
    expect(data(m, 1)['option']).toBeUndefined();
  });

  it('takes any other standard action for the creature', async () => {
    const m = await mount();
    m.view.creatureAction('w', 'standard:dash');
    await settle(m.fixture);
    expect(m.opened).toHaveLength(0);
    expect(m.callsOf('takeAction').map((c) => c.args)).toEqual([
      ['c', 'enc', 'w', 'standard:dash'],
    ]);
  });
});

describe('CombatView sheets: useFeature', () => {
  const withFeatures = (...features: ReturnType<typeof feature>[]) =>
    forToren(
      { featureActions: features },
      {
        resourceTargets: [
          {
            actionKey: 'feature:lay-on-hands',
            targets: [{ target: { combatantId: 'g', label: 'Goblin' } }],
          },
          {
            actionKey: 'feature:bardic-inspiration-d6',
            targets: [{ target: { combatantId: 'g', label: 'Goblin' } }],
          },
        ],
        contestState: {
          hideActions: [
            {
              key: 'feature:cunning-action-hide',
              economy: ActionEconomy.BONUS_ACTION,
              enabled: true,
            },
          ],
        },
      },
    );

  it('does nothing for a key the turn does not list', async () => {
    const m = await setup({ turnOptions: withFeatures(feature('feature:rage', 'Fúria')) });
    await m.view.useFeature('feature:unknown');
    expect(m.opened).toHaveLength(0);
    expect(m.callsOf('takeAction')).toHaveLength(0);
  });

  it('does nothing for the master', async () => {
    const m = await setup({
      master: true,
      enc: masterEnc(),
      turnOptions: turns({
        g: { options: { featureActions: [feature('feature:rage', 'Fúria')] } },
      }),
    });
    await m.view.useFeature('feature:rage');
    expect(m.callsOf('takeAction')).toHaveLength(0);
  });

  it('opens Hide with the bonus action cost for the rogue’s Cunning Action', async () => {
    const m = await setup({
      turnOptions: withFeatures(
        feature('feature:cunning-action-hide', 'Ação Ardilosa: Esconder', {
          economy: ActionEconomy.BONUS_ACTION,
        }),
      ),
    });
    await m.view.useFeature('feature:cunning-action-hide');
    expect(m.opened[0].component).toBe(HideSheet);
    expect(data(m)).toMatchObject({
      combatantId: 't',
      actionKey: 'feature:cunning-action-hide',
      economy: 'Ação bônus',
    });
    expect(m.callsOf('takeAction')).toHaveLength(0);
  });

  it('opens Cura pelas Mãos with the combat, the actor, the vitals and the targets of the action', async () => {
    const m = await setup({
      matches: PHONE,
      turnOptions: withFeatures(feature('feature:lay-on-hands', 'Cura pelas Mãos')),
    });
    await m.view.useFeature('feature:lay-on-hands');
    expect(m.opened[0].component).toBe(LayOnHandsSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Cura pelas Mãos');
    expect(data(m)).toMatchObject({ campaignId: 'c', encounterId: 'enc', actorId: 't' });
    expect(data(m)['vitals']).toBe(m.view.ownVitals);
    expect(data(m)['state']).toBe(m.state);
    expect(
      (data(m)['targets'] as { target: { label: string } }[]).map((t) => t.target.label),
    ).toEqual(['Goblin']);
  });

  it('hands a flow with no listed targets an empty list', async () => {
    const m = await setup({
      turnOptions: forToren({
        featureActions: [feature('feature:bardic-inspiration-d6', 'Inspiração de Bardo')],
      }),
    });
    await m.view.useFeature('feature:bardic-inspiration-d6');
    expect(m.opened[0].component).toBe(BardicInspirationSheet);
    expect(data(m)['targets']).toEqual([]);
  });

  it('opens Inspiração de Bardo with its own targets', async () => {
    const m = await setup({
      turnOptions: withFeatures(feature('feature:bardic-inspiration-d6', 'Inspiração de Bardo')),
    });
    await m.view.useFeature('feature:bardic-inspiration-d6');
    expect(m.opened[0].component).toBe(BardicInspirationSheet);
    expect(data(m)).toMatchObject({ actorId: 't', encounterId: 'enc' });
    expect(data(m)['targets']).toHaveLength(1);
  });

  it('opens Conjuração Flexível with no targets, in the direction of the action', async () => {
    const m = await setup({
      turnOptions: withFeatures(
        feature('feature:flexible-casting-creating-spell-slots', 'Criar espaço'),
        feature('feature:flexible-casting-converting-spell-slot', 'Converter espaço'),
      ),
    });
    await m.view.useFeature('feature:flexible-casting-creating-spell-slots');
    await m.view.useFeature('feature:flexible-casting-converting-spell-slot');
    expect(m.opened.map((o) => o.component)).toEqual([FlexibleCastingSheet, FlexibleCastingSheet]);
    expect(m.opened.map((o) => o.config.data['direction'])).toEqual(['create', 'convert']);
    expect('targets' in data(m)).toBe(false);
    expect(m.callsOf('takeAction')).toHaveLength(0);
  });

  it('opens Retomar o Fôlego in its sheet with the bonus action cost', async () => {
    const m = await setup({
      matches: PHONE,
      turnOptions: withFeatures(
        feature('feature:second-wind', 'Retomar o Fôlego', { economy: ActionEconomy.BONUS_ACTION }),
      ),
    });
    await m.view.useFeature('feature:second-wind');
    expect(m.opened[0].component).toBe(FeatureSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Retomar o Fôlego');
    expect(data(m)).toMatchObject({
      campaignId: 'c',
      encounterId: 'enc',
      combatantId: 't',
      actionKey: 'feature:second-wind',
      name: 'Retomar o Fôlego',
      cost: 'Ação bônus',
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
    });
    expect(data(m)['state']).toBe(m.state);
    expect(m.callsOf('takeAction')).toHaveLength(0);
  });

  it('names the cost of Retomar o Fôlego as an action when it is not a bonus action', async () => {
    const m = await setup({
      turnOptions: withFeatures(feature('feature:second-wind', 'Retomar o Fôlego')),
    });
    await m.view.useFeature('feature:second-wind');
    expect(data(m)['cost']).toBe('Ação');
  });

  it('spends the action of a plain feature and notes it', async () => {
    const m = await setup({ turnOptions: withFeatures(feature('feature:rage', 'Fúria')) });
    await m.view.useFeature('feature:rage');
    expect(m.opened).toHaveLength(0);
    expect(m.callsOf('takeAction').map((c) => c.args)).toEqual([
      ['c', 'enc', 't', 'feature:rage', undefined, undefined, false, false],
    ]);
    expect(m.view.actionNote()).toBe('Usou Fúria.');
  });

  it('notes the extra action Surto de Ação gives back', async () => {
    const m = await setup({
      turnOptions: withFeatures(
        feature('feature:action-surge', 'Surto de Ação', { resourceKey: 'action_surge' }),
      ),
    });
    await m.view.useFeature('feature:action-surge');
    expect(m.view.actionNote()).toBe('Surto de Ação: você tem outra ação.');
  });

  it('sends the frenzy flag and notes the frenzy instead of the use', async () => {
    const m = await setup({ turnOptions: withFeatures(feature('feature:rage', 'Fúria')) });
    await m.view.useFeature('feature:rage', true);
    expect(m.callsOf('takeAction')[0].args).toEqual([
      'c',
      'enc',
      't',
      'feature:rage',
      undefined,
      undefined,
      false,
      true,
    ]);
    expect(m.view.actionNote()).toBe('Fúria: você entrou em frenesi.');
  });

  it('keeps the note away when the server refuses the feature, and says why', async () => {
    const m = await setup({
      turnOptions: withFeatures(feature('feature:rage', 'Fúria')),
      api: {
        takeAction: () => {
          throw new ConnectError('no', Code.InvalidArgument);
        },
      },
    });
    await m.view.useFeature('feature:rage');
    expect(m.view.actionNote()).toBe('');
    expect(m.view.error()).not.toBe('');
  });
});

describe('CombatView sheets: the attack sheet', () => {
  const longsword = (over: object = {}) => ({ enabled: true, attack: sword, ...over });

  it("hands the attack sheet the attack, the targets in reach and the turn's numbers", async () => {
    const m = await setup({
      matches: PHONE,
      turnOptions: forToren(
        {
          attacks: [longsword({ bonusRule: 1, bonusAttacksLeft: 2, beamsLeft: 3 })],
          economy: { attacksLeft: 2, attacksPerAction: 3 },
        },
        { attackTargets: [swordTargets] },
      ),
    });
    m.view.openAttack('attack:longsword');
    expect(m.opened).toHaveLength(1);
    expect(m.opened[0].component).toBe(AttackSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Atacar com Espada longa');
    const d = data(m);
    const options = m.view.options();
    expect(d).toMatchObject({
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 't',
      round: 3,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      asReaction: false,
      master: false,
      bonusRule: 1,
      bonusAttacksLeft: 2,
      beamsLeft: 3,
      attacksLeft: 2,
      attacksPerAction: 3,
    });
    expect(d['attack']).toBe(options.options.attacks[0].attack);
    expect(d['targets']).toBe(options.attackTargets[0].targets);
    expect(d['state']).toBe(m.state);
    expect(d['resume']).toBeUndefined();
    expect(d['inspiration']).toBeUndefined();
    expect(d['lucky']).toBeUndefined();
    expect('catchWindowId' in d).toBe(false);
    expect('useExtraAction' in d).toBe(false);
    expect(typeof d['onHeld']).toBe('function');
  });

  it('falls back to no attacks left, one attack per action, no beams and no targets', async () => {
    const m = await setup({ turnOptions: forToren({ attacks: [longsword()] }) });
    m.view.openAttack('attack:longsword');
    expect(data(m)).toMatchObject({ attacksLeft: 0, attacksPerAction: 1, beamsLeft: 0 });
    expect(data(m)['targets']).toEqual([]);
  });

  it('takes the beams of the attack when the option has none left to count', async () => {
    const m = await setup({
      turnOptions: forToren({ attacks: [longsword({ attack: { ...sword, beams: 2 } })] }),
    });
    m.view.openAttack('attack:longsword');
    expect(data(m)['beamsLeft']).toBe(2);
  });

  it('prefers the beams left of the option over the beams of the attack', async () => {
    const m = await setup({
      turnOptions: forToren({
        attacks: [longsword({ beamsLeft: 1, attack: { ...sword, beams: 3 } })],
      }),
    });
    m.view.openAttack('attack:longsword');
    expect(data(m)['beamsLeft']).toBe(1);
  });

  it('labels an opportunity attack and says it is a reaction', async () => {
    const m = await setup({
      matches: PHONE,
      turnOptions: forToren({ attacks: [longsword()] }),
    });
    m.view.openAttack('attack:longsword', true);
    expect(m.opened[0].config.ariaLabel).toBe('Ataque de oportunidade com Espada longa');
    expect(data(m)['asReaction']).toBe(true);
  });

  it('names the attack by its English name when it has no Portuguese one', async () => {
    const m = await setup({
      matches: PHONE,
      turnOptions: forToren({ attacks: [longsword({ attack: { ...sword, namePt: '' } })] }),
    });
    m.view.openAttack('attack:longsword');
    expect(m.opened[0].config.ariaLabel).toBe('Atacar com Longsword');
    m.view.openAttack('attack:longsword', true);
    expect(m.opened[1].config.ariaLabel).toBe('Ataque de oportunidade com Longsword');
  });

  it('marks the extra action only when it is used', async () => {
    const m = await setup({ turnOptions: forToren({ attacks: [longsword()] }) });
    m.view.openAttack('attack:longsword', false, true);
    expect(data(m)['useExtraAction']).toBe(true);
    expect(data(m)['asReaction']).toBe(false);
  });

  it('opens nothing for an attack the options do not list', async () => {
    const m = await setup({ turnOptions: forToren({ attacks: [longsword()] }) });
    m.view.openAttack('attack:dagger');
    expect(m.opened).toHaveLength(0);
    expect(m.view.attackSheetOpen()).toBe(false);
  });

  it('opens nothing before the options arrive', () => {
    const m = mountCombatView({
      enc: theatre(),
      turnOptions: forToren({ attacks: [longsword()] }),
    });
    m.view.openAttack('attack:longsword');
    expect(m.opened).toHaveLength(0);
  });

  it('tells the master sheet apart', async () => {
    const m = await setup({
      master: true,
      turnOptions: forToren({ attacks: [longsword()] }),
    });
    m.view.openAttack('attack:longsword');
    expect(data(m)['master']).toBe(true);
  });

  it('keeps the flag of an open attack sheet until the sheet closes', async () => {
    const m = await setup({ turnOptions: forToren({ attacks: [longsword()] }) });
    expect(m.view.attackSheetOpen()).toBe(false);
    m.view.openAttack('attack:longsword');
    expect(m.view.attackSheetOpen()).toBe(true);
    m.opened[0].close(true);
    expect(m.view.attackSheetOpen()).toBe(false);
  });

  it('lowers the flag when the sheet fails, and does not keep a missile', async () => {
    const m = await setup({ turnOptions: forToren({ attacks: [longsword()] }) });
    let fail: (err: unknown) => void = () => undefined;
    m.view.dialog.open = () => ({
      afterClosed: () =>
        new Observable((subscriber) => {
          fail = (err) => subscriber.error(err);
        }),
    });
    m.view.sheets.attack({ key: 'attack:longsword', asReaction: true, catchWindowId: 'w9' });
    expect(m.view.attackSheetOpen()).toBe(true);
    fail(new Error('boom'));
    expect(m.view.attackSheetOpen()).toBe(false);
    expect(m.callsOf('answerReaction')).toHaveLength(0);
  });
});

describe('CombatView sheets: the held roll', () => {
  const offer = (over: object = {}) => ({
    holdId: 'hold-1',
    attackKey: 'attack:longsword',
    targetId: 'g',
    die: { sides: 8, fromLabel: 'Orla', fromCombatantId: 'o', expiresAtRound: 9 },
    ...over,
  });
  const lucky = (over: object = {}) => ({
    holdId: 'hold-2',
    attackKey: 'attack:longsword',
    targetId: 'g',
    luckyIndex: 0,
    ...over,
  });
  const options = forToren({ attacks: [{ enabled: true, attack: sword }] });
  const mount = (own: object, extra: Partial<MountOptions> = {}) =>
    setup({
      enc: theatre({}, [toren(own), goblin()]),
      turnOptions: options,
      ...extra,
    });

  it('reopens the attack sheet by itself at the die question, with the label of the target', async () => {
    const m = await mount({ inspirationOffer: offer() });
    expect(m.opened).toHaveLength(1);
    expect(m.opened[0].component).toBe(AttackSheet);
    const d = data(m);
    expect(d['inspiration']).toMatchObject({ offer: { holdId: 'hold-1' }, targetLabel: 'Goblin' });
    expect(d['lucky']).toBeUndefined();
    expect(d['asReaction']).toBe(false);
    expect(d['resume']).toBeUndefined();
    expect(d['attackerId']).toBe('t');
    expect(m.view.inspirationHandled.has('hold-1')).toBe(true);
  });

  it('asks the Lucky question first and passes both when the roll holds both', async () => {
    const m = await mount({ inspirationOffer: offer(), luckyOffer: lucky() });
    expect(m.opened).toHaveLength(1);
    const d = data(m);
    expect(d['lucky']).toMatchObject({ offer: { holdId: 'hold-2' }, targetLabel: 'Goblin' });
    expect(d['inspiration']).toMatchObject({ offer: { holdId: 'hold-1' }, targetLabel: 'Goblin' });
    expect(m.view.inspirationHandled.has('hold-2')).toBe(true);
  });

  it('leaves the label empty when the target is not in the combat', async () => {
    const m = await mount({ inspirationOffer: offer({ targetId: 'gone' }) });
    expect(data(m)['inspiration']).toMatchObject({ targetLabel: '' });
  });

  it('registers the hold a roll made by its own sheet through onHeld', async () => {
    const m = await mount({ inspirationOffer: offer() });
    (data(m)['onHeld'] as (id: string) => void)('hold-x');
    expect(m.view.inspirationHandled.has('hold-x')).toBe(true);
  });

  it('answers false and opens nothing when the attack of the held roll is not in the options', async () => {
    const m = await mount({ inspirationOffer: offer({ attackKey: 'attack:gone' }) });
    expect(m.opened).toHaveLength(0);
    expect(m.view.openHeldRoll()).toBe(false);
    expect(m.view.inspirationHandled.has('hold-1')).toBe(false);
  });

  it('answers false when nothing is held', async () => {
    const m = await mount({});
    expect(m.view.openHeldRoll()).toBe(false);
    expect(m.opened).toHaveLength(0);
  });

  it('opens the question again when asked, with the roll that waits', async () => {
    const m = await mount({ luckyOffer: lucky() });
    expect(m.opened).toHaveLength(1);
    expect(m.view.openHeldRoll()).toBe(true);
    expect(m.opened).toHaveLength(2);
    expect(data(m, 1)['lucky']).toMatchObject({ offer: { holdId: 'hold-2' } });
  });
});

describe('CombatView sheets: the damage that waits', () => {
  const save = {
    key: 'spell:sacred-flame',
    name: 'Sacred Flame',
    namePt: 'Chama Sagrada',
    kind: AttackKind.SPELL,
    saveDc: 14,
    spellDice: '2d8',
  };
  const mount = (over: TurnInit, extra: Partial<MountOptions> = {}) =>
    setup({
      matches: PHONE,
      turnOptions: forToren(
        {
          attacks: [
            { enabled: true, attack: sword },
            { enabled: true, attack: save },
          ],
          ...((over['options'] as object | undefined) ?? {}),
        },
        { pendingDamages: over['pendingDamages'], attackTargets: [swordTargets] },
      ),
      ...extra,
    });

  it('does nothing when no hit waits for its damage', async () => {
    const m = await mount({});
    m.view.rollPendingDamage();
    expect(m.opened).toHaveLength(0);
  });

  it('ignores the damage of other attackers and of rolls already made', async () => {
    const m = await mount({
      pendingDamages: [
        pending({ attackerId: 'g' }),
        pending({ id: 'pd2', status: PendingDamageStatus.ROLLED }),
      ],
    });
    m.view.rollPendingDamage();
    expect(m.opened).toHaveLength(0);
  });

  it('reopens the attack sheet of a weapon hit at its damage, naming the target', async () => {
    const m = await mount({ pendingDamages: [pending()] });
    m.view.rollPendingDamage();
    expect(m.opened[0].component).toBe(AttackSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Atacar com Espada longa');
    expect(data(m)['resume']).toMatchObject({ pending: { id: 'pd1' }, targetLabel: 'Goblin' });
    expect(data(m)['attackerId']).toBe('t');
  });

  it('names no target when the one hit left the combat', async () => {
    const m = await mount({ pendingDamages: [pending({ targetId: 'gone' })] });
    m.view.rollPendingDamage();
    expect(data(m)['resume']).toMatchObject({ targetLabel: '' });
  });

  it('opens the cast sheet at the damage of a spell whose attack asks a saving throw', async () => {
    const m = await mount({
      pendingDamages: [
        pending({ attackKey: 'spell:sacred-flame' }),
        pending({ id: 'pd2', attackKey: 'spell:sacred-flame', targetId: 'h' }),
        // These are not owed by this cast: another spell, an attacker that is not the caster, a damage already rolled.
        pending({ id: 'pd3', attackKey: 'attack:longsword' }),
        pending({ id: 'pd4', attackKey: 'spell:sacred-flame', attackerId: 'g' }),
        pending({ id: 'pd5', attackKey: 'spell:sacred-flame', status: PendingDamageStatus.ROLLED }),
      ],
    });
    m.view.rollPendingDamage();
    expect(m.opened).toHaveLength(1);
    expect(m.opened[0].component).toBe(CastSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Rolar o dano de Chama Sagrada');
    expect(data(m)['spellKey']).toBe('spell:sacred-flame');
    expect((data(m)['resume'] as { id: string }[]).map((p) => p.id)).toEqual(['pd1', 'pd2']);
  });

  it('opens the cast sheet at the damage of a spell that is no attack of the turn', async () => {
    const m = await setup({
      matches: PHONE,
      turnOptions: forToren(
        {
          spells: [
            {
              spell: {
                key: 'spell:magic-missile',
                name: 'Magic Missile',
                namePt: 'Mísseis Mágicos',
                level: 1,
              },
              economy: ActionEconomy.ACTION,
              enabled: true,
            },
          ],
        },
        { pendingDamages: [pending({ attackKey: 'spell:magic-missile' })] },
      ),
    });
    m.view.rollPendingDamage();
    expect(m.opened[0].component).toBe(CastSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Rolar o dano de Mísseis Mágicos');
    expect((data(m)['resume'] as { id: string }[]).map((p) => p.id)).toEqual(['pd1']);
  });

  it('names the cast by its key when neither the spells nor the attacks know it', async () => {
    const m = await mount({ pendingDamages: [pending({ attackKey: 'spell:unknown' })] });
    m.view.rollPendingDamage();
    expect(m.opened[0].component).toBe(CastSheet);
    expect(data(m)).toMatchObject({ name: 'spell:unknown', level: 0 });
    expect(m.opened[0].config.ariaLabel).toBe('Rolar o dano de spell:unknown');
  });
});

describe('CombatView sheets: the attacks of a creature', () => {
  const bite = {
    key: 'attack:bite',
    name: 'Bite',
    namePt: 'Mordida',
    kind: AttackKind.WEAPON,
    melee: true,
    attackBonus: 4,
    rangeFt: 5,
  };
  const wolfTurn = (over: TurnInit = {}): TurnInit => ({
    options: {
      attacks: [{ enabled: true, attack: bite }],
      economy: { attacksLeft: 1, attacksPerAction: 2 },
    },
    attackTargets: [
      { attackKey: 'attack:bite', targets: [{ combatantId: 'g', label: 'Goblin', distanceFt: 5 }] },
    ],
    pendingDamages: [pending({ id: 'pw', attackerId: 'w', attackKey: 'attack:bite' })],
    ...over,
  });
  const mount = (w: TurnInit = wolfTurn(), extra: Partial<MountOptions> = {}) =>
    setup({
      matches: PHONE,
      enc: theatre({}, [toren(), wolf(), goblin()]),
      turnOptions: turns({
        t: { options: { economy: { attacksLeft: 5, attacksPerAction: 9 } } },
        w,
      }),
      ...extra,
    });

  it("opens the attack sheet with the creature as the attacker and its own numbers, not the character's", async () => {
    const m = await mount();
    m.view.openCreatureAttack('w', 'attack:bite');
    expect(m.opened[0].component).toBe(AttackSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Atacar com Mordida: Lobo');
    const d = data(m);
    expect(d).toMatchObject({
      attackerId: 'w',
      encounterId: 'enc',
      round: 3,
      asReaction: false,
      attacksLeft: 1,
      attacksPerAction: 2,
    });
    const own = m.view.creatureOptions.data().get('w');
    expect(d['attack']).toBe(own.options.attacks[0].attack);
    expect(d['targets']).toBe(own.attackTargets[0].targets);
  });

  it('falls back to no attacks left and one per action for a creature with no economy', async () => {
    const m = await mount(wolfTurn({ options: { attacks: [{ enabled: true, attack: bite }] } }));
    m.view.openCreatureAttack('w', 'attack:bite');
    expect(data(m)).toMatchObject({ attacksLeft: 0, attacksPerAction: 1 });
  });

  it('labels the creature that attacks as a reaction (the pact chain familiar)', async () => {
    const m = await mount();
    m.view.openCreatureAttack('w', 'attack:bite', true);
    expect(m.opened[0].config.ariaLabel).toBe('Ataque de oportunidade com Mordida');
    expect(data(m)['asReaction']).toBe(true);
  });

  it('opens nothing for a creature that is not in the combat or has no such attack', async () => {
    const m = await mount();
    m.view.openCreatureAttack('gone', 'attack:bite');
    m.view.openCreatureAttack('w', 'attack:claw');
    m.view.openCreatureAttack('g', 'attack:bite');
    expect(m.opened).toHaveLength(0);
  });

  it('finds the hit of the creature that waits for its damage, and no other', async () => {
    const m = await mount();
    expect(m.view.creaturePending('w')).toMatchObject({ id: 'pw' });
    expect(m.view.creaturePending('t')).toBeNull();
    expect(m.view.creaturePending('gone')).toBeNull();
  });

  it('does not take a hit already rolled or a hit of another attacker for the creature’s', async () => {
    const m = await mount(
      wolfTurn({
        pendingDamages: [
          pending({ id: 'p1', attackerId: 'w', status: PendingDamageStatus.ROLLED }),
          pending({ id: 'p2', attackerId: 't' }),
        ],
      }),
    );
    expect(m.view.creaturePending('w')).toBeNull();
    m.view.rollCreatureDamage('w');
    expect(m.opened).toHaveLength(0);
  });

  it('reopens the attack sheet of the creature at its damage, naming the target', async () => {
    const m = await mount();
    m.view.rollCreatureDamage('w');
    expect(m.opened[0].component).toBe(AttackSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Atacar com Mordida: Lobo');
    expect(data(m)).toMatchObject({ attackerId: 'w', asReaction: false, attacksLeft: 1 });
    expect(data(m)['resume']).toMatchObject({ pending: { id: 'pw' }, targetLabel: 'Goblin' });
  });

  it('opens nothing at the damage of a creature that left the combat', async () => {
    const m = await mount();
    m.view.rollCreatureDamage('gone');
    expect(m.opened).toHaveLength(0);
  });
});

describe('CombatView sheets: the missile caught by Defletir Projéteis', () => {
  const throwWindow = (over: object = {}, missileNamePt = 'Arco curto') =>
    reactionWindow({
      id: 'w2',
      kind: ReactionKind.DEFLECT_MISSILES,
      reactorId: 't',
      secondStep: true,
      // Not the player's to answer by itself, so the reaction sheet does not open on its own.
      forYou: false,
      prompt: {
        case: 'deflectThrow',
        value: { kiLeft: 3, normalRangeFt: 20, longRangeFt: 60, missileNamePt, attackBonus: 7 },
      } as never,
      ...over,
    } as never);
  const bow = {
    key: 'attack:shortbow',
    name: 'Shortbow',
    namePt: 'Arco curto',
    kind: AttackKind.WEAPON,
    rangeFt: 80,
  };
  const mount = (window = throwWindow(), extra: Partial<MountOptions> = {}) =>
    setup({
      matches: PHONE,
      enc: theatre({ reactionWindows: [window] }),
      turnOptions: forToren(
        {
          attacks: [{ enabled: true, attack: bow }],
          economy: { attacksLeft: 1, attacksPerAction: 2 },
        },
        {
          attackTargets: [
            {
              attackKey: 'attack:shortbow',
              targets: [
                { combatantId: 'g', label: 'Goblin' },
                { combatantId: 'h', label: 'Hobgoblin' },
              ],
            },
            {
              attackKey: 'attack:dagger',
              targets: [
                { combatantId: 'h', label: 'Hobgoblin (outra lista)' },
                { combatantId: 'k', label: 'Kobold' },
              ],
            },
          ],
        },
      ),
      ...extra,
    });

  it('builds the thrown attack from the prompt and lists each target once, the first list winning', async () => {
    const m = await mount();
    m.view.sheets.throwBack('w2');
    expect(m.opened[0].component).toBe(AttackSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Ataque de oportunidade com Arco curto');
    const d = data(m);
    expect(d['attack']).toMatchObject({
      key: 'deflect-missiles:throw',
      name: 'Arco curto',
      namePt: 'Arco curto',
      attackBonus: 7,
      kind: AttackKind.WEAPON,
      rangeFt: 20,
      longRangeFt: 60,
    });
    expect(
      (d['targets'] as { combatantId: string; label: string }[]).map((t) => [
        t.combatantId,
        t.label,
      ]),
    ).toEqual([
      ['g', 'Goblin'],
      ['h', 'Hobgoblin'],
      ['k', 'Kobold'],
    ]);
    expect(d).toMatchObject({
      asReaction: true,
      catchWindowId: 'w2',
      attackerId: 't',
      attacksLeft: 1,
      attacksPerAction: 2,
      beamsLeft: 0,
    });
    expect('useExtraAction' in d).toBe(false);
  });

  it('calls a missile with no name "Projétil"', async () => {
    const m = await mount(throwWindow({}, ''));
    m.view.sheets.throwBack('w2');
    expect(data(m)['attack']).toMatchObject({ name: 'Projétil', namePt: 'Projétil' });
    expect(m.opened[0].config.ariaLabel).toBe('Ataque de oportunidade com Projétil');
  });

  it('opens nothing for a window that is not there or has another prompt', async () => {
    const m = await mount(
      throwWindow({ id: 'w3', prompt: { case: 'masterCheck', value: {} } as never }),
    );
    m.view.sheets.throwBack('w2');
    m.view.sheets.throwBack('w3');
    expect(m.opened).toHaveLength(0);
  });

  it('lists no targets when the options have not arrived', () => {
    const m = mountCombatView({
      enc: theatre({ reactionWindows: [throwWindow()] }),
      matches: PHONE,
    });
    m.view.sheets.throwBack('w2');
    expect(data(m)['targets']).toEqual([]);
    expect(data(m)).toMatchObject({ attacksLeft: 0, attacksPerAction: 1, catchWindowId: 'w2' });
  });

  it('answers the window "Deixar passar" when the sheet closes without a roll, under a key of its own', async () => {
    const m = await mount();
    m.view.sheets.throwBack('w2');
    m.opened[0].close();
    await settle(m.fixture);
    expect(m.callsOf('answerReaction').map((c) => c.args)).toEqual([
      ['c', 'enc', 'w2', { use: false }, 'keep-w2'],
    ]);
  });

  it('keeps no missile for a window that was answered meanwhile', async () => {
    const m = await mount(throwWindow({ status: ReactionWindowStatus.ANSWERED }));
    m.view.sheets.throwBack('w2');
    m.opened[0].close();
    await settle(m.fixture);
    expect(m.callsOf('answerReaction')).toHaveLength(0);
  });

  it('swallows the refusal of the pass', async () => {
    const m = await mount(throwWindow(), {
      api: {
        answerReaction: () => {
          throw new ConnectError('already', Code.FailedPrecondition);
        },
      },
    });
    m.view.sheets.throwBack('w2');
    m.opened[0].close();
    await settle(m.fixture);
    expect(m.callsOf('answerReaction')).toHaveLength(1);
    expect(m.view.error()).toBe('');
  });
});

describe('CombatView sheets: the reaction and the effect saving throw', () => {
  const shield = (over: object = {}) =>
    reactionWindow({
      id: 'w1',
      kind: ReactionKind.SHIELD,
      reactorId: 't',
      reactorIsPlayer: true,
      prompt: {
        case: 'shield',
        value: {
          attackerLabel: 'Goblin',
          spellNamePt: 'Escudo Arcano',
          slots: [{ level: 1, pact: false, free: 2 }],
        },
      } as never,
      ...over,
    } as never);
  const effectSave = () =>
    reactionWindow({
      id: 'w5',
      kind: ReactionKind.EFFECT_SAVE,
      reactorId: 't',
      reactorIsPlayer: true,
      prompt: {
        case: 'effectSave',
        value: { effectId: 'e1', sourceNamePt: 'Imobilizar Pessoa' },
      } as never,
    } as never);
  const slots = vitals({
    spellSlots: [{ level: 1, total: 4, used: 1 }],
    pactSlots: { slotLevel: 2, total: 2, used: 0 },
  });

  it('hands the reaction sheet the window, the slots of the character and the player’s armor class', async () => {
    const m = await setup({
      matches: PHONE,
      inputs: { vitals: [slots], armorClass: 15 },
    });
    const window = shield();
    m.view.sheets.reaction(window);
    expect(m.opened).toHaveLength(1);
    expect(m.opened[0].component).toBe(ReactionSheet);
    const d = data(m);
    expect(d).toMatchObject({
      campaignId: 'c',
      encounterId: 'enc',
      round: 3,
      usage: [{ level: 1, total: 4, used: 1 }],
      pact: { slotLevel: 2, total: 2, used: 0 },
      armorClass: 15,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
    });
    expect(d['window']).toBe(window);
    expect(d['state']).toBe(m.state);
  });

  it('hands it no slots and no armor class when the sheet does not say', async () => {
    const m = await setup();
    m.view.sheets.reaction(shield());
    expect(data(m)).toMatchObject({ usage: [], pact: null, armorClass: null });
  });

  it('is an alert that must be answered, labelled by its prompt', async () => {
    const m = await setup({ matches: PHONE });
    m.view.sheets.reaction(shield());
    expect(m.opened[0].config.ariaLabel).toMatch(/\S/);
    expect(m.opened[0].config.ariaLabel).not.toBe('Reação');
    expect(m.opened[0].config.disableClose).toBe(true);
    expect(m.opened[0].config.autoFocus).toBe('[data-initial-focus]');
  });

  it('labels a window with no prompt of its own "Reação"', async () => {
    const m = await setup({ matches: PHONE });
    m.view.sheets.reaction(
      reactionWindow({ id: 'w7', prompt: { case: 'masterCheck', value: {} } as never } as never),
    );
    expect(m.opened[0].config.ariaLabel).toBe('Reação');
  });

  it('holds the reaction flag while the sheet is open', async () => {
    const m = await setup();
    m.view.sheets.reaction(shield());
    expect(m.view.reactionOpen()).toBe(true);
    m.opened[0].close();
    expect(m.view.reactionOpen()).toBe(false);
  });

  it('hands over the monk’s throw-back request and marks its window as handled', async () => {
    const m = await setup({
      enc: theatre({
        reactionWindows: [
          reactionWindow({
            id: 'w2',
            kind: ReactionKind.DEFLECT_MISSILES,
            secondStep: true,
            forYou: false,
            prompt: {
              case: 'deflectThrow',
              value: { normalRangeFt: 20, longRangeFt: 60, missileNamePt: 'Dardo', attackBonus: 6 },
            } as never,
          } as never),
        ],
      }),
    });
    m.view.sheets.reaction(shield());
    m.opened[0].close({ throwWindowId: 'w2' });
    expect(m.view.reactionHandled.has('w2')).toBe(true);
    expect(m.opened[1].component).toBe(AttackSheet);
    expect(data(m, 1)['catchWindowId']).toBe('w2');
  });

  it('opens nothing on a result that asks for no throw', async () => {
    const m = await setup();
    m.view.sheets.reaction(shield());
    m.opened[0].close({});
    expect(m.opened).toHaveLength(1);
  });

  it('sends a window with an effect saving throw to its own sheet', async () => {
    const m = await setup({ matches: PHONE });
    const window = effectSave();
    m.view.sheets.reaction(window);
    expect(m.opened).toHaveLength(1);
    expect(m.opened[0].component).toBe(EffectSaveSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Teste de resistência do fim do turno');
    expect(data(m)).toMatchObject({
      campaignId: 'c',
      encounterId: 'enc',
      round: 3,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
    });
    expect(data(m)['window']).toBe(window);
    expect(data(m)['state']).toBe(m.state);
    expect(m.view.reactionOpen()).toBe(true);
    m.opened[0].close();
    expect(m.view.reactionOpen()).toBe(false);
  });

  it('is labelled by its title on a dialog, and not an alert', async () => {
    const m = await setup();
    m.view.openEffectSave(effectSave());
    expect(m.opened[0].config.ariaLabelledBy).toBe('sheet-t');
    expect(m.opened[0].config.disableClose).toBeFalsy();
  });

  it('opens no second sheet over one that is open', async () => {
    const m = await setup();
    m.view.sheets.reaction(shield());
    m.view.openEffectSave(effectSave());
    expect(m.opened.map((o) => o.component)).toEqual([ReactionSheet]);
  });
});

describe('CombatView sheets: the door, the spell details and the small dialogs', () => {
  it('hands the door sheet the door and the walls painted under its block', async () => {
    const m = await setup({ master: true, matches: PHONE });
    m.view.layersState.layers.set({
      columns: 20,
      rows: 14,
      walls: [
        { col: 3, row: 4 },
        { col: 9, row: 9 },
      ],
      terrain: [],
      half: [],
      threeQuarters: [],
    });
    const door = { col: 3, row: 4, kind: 2 };
    m.view.openDoor(door);
    expect(m.opened[0].component).toBe(DoorSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Porta');
    expect(data(m)).toMatchObject({
      campaignId: 'c',
      mapId: '',
      wallSquares: [{ col: 3, row: 4 }],
    });
    expect(data(m)['door']).toBe(door);
  });

  it('measures the wall under the door on the whole drawing square of a calibrated map', async () => {
    const mapState = new MapState(async () => ({}) as never);
    mapState.map.set(create(MapSchema, { id: 'map', squareFactor: 2 }));
    const m = await setup({ master: true, mapState });
    m.view.layersState.layers.set({
      columns: 20,
      rows: 14,
      walls: [
        { col: 2, row: 5 },
        { col: 3, row: 4 },
        { col: 4, row: 4 },
      ],
      terrain: [],
      half: [],
      threeQuarters: [],
    });
    m.view.openDoor({ col: 3, row: 4, kind: 2 });
    expect(data(m)['wallSquares']).toEqual([
      { col: 3, row: 4 },
      { col: 2, row: 5 },
    ]);
  });

  it('opens the door sheet with no walls when none is painted', async () => {
    const m = await setup({ master: true });
    m.view.openDoor({ col: 3, row: 4, kind: 2 });
    expect(data(m)['wallSquares']).toEqual([]);
    expect(m.opened[0].config.ariaLabelledBy).toBe('door-t');
    expect(m.opened[0].config.width).toBe('440px');
  });

  it('opens the spell description with the name now and the text loaded through the catalog', async () => {
    const m = await setup({ matches: PHONE });
    const details = create(SpellDetailsSchema, {
      spell: { key: 'spell:fire-bolt', name: 'Fire Bolt', namePt: 'Raio de Fogo', level: 0 },
    });
    m.view.catalog.details = vi.fn(async () => details);
    m.view.describeSpell('spell:fire-bolt', 'Raio de Fogo');
    expect(m.opened[0].component).toBe(SpellDetails);
    expect(m.opened[0].config.ariaLabel).toBe('Descrição de Raio de Fogo');
    expect(data(m)['namePt']).toBe('Raio de Fogo');
    const loaded = await (data(m)['load'] as () => Promise<unknown>)();
    expect(loaded).toEqual(spellDetailsFromGen(details));
    expect(m.view.catalog.details).toHaveBeenCalledWith('c', 'spell:fire-bolt');
  });

  it('fails the description load when the catalog has no such spell', async () => {
    const m = await setup();
    m.view.describeSpell('spell:nope', 'Nada');
    expect(m.opened[0].config.ariaLabelledBy).toBe('spell-title');
    await expect((data(m)['load'] as () => Promise<unknown>)()).rejects.toThrow(
      'spell details unavailable',
    );
  });

  it('opens Condições for a combatant of the combat, as the master', async () => {
    const m = await setup({ master: true, matches: PHONE });
    m.view.openConditions('g');
    expect(m.opened[0].component).toBe(ConditionsDialog);
    expect(m.opened[0].config.ariaLabel).toBe('Condições de Goblin');
    expect(data(m)).toMatchObject({ campaignId: 'c', encounterId: 'enc', combatant: { id: 'g' } });
    expect(data(m)['state']).toBe(m.state);
  });

  it('is labelled by its title and 600 px wide on a dialog', async () => {
    const m = await setup({ master: true });
    m.view.openConditions('g');
    expect(m.opened[0].config.ariaLabelledBy).toBe('sheet-t');
    expect(m.opened[0].config.width).toBe('600px');
  });

  it('opens Condições for nobody who is not in the combat', async () => {
    const m = await setup({ master: true });
    m.view.openConditions('gone');
    expect(m.opened).toHaveLength(0);
  });

  it('opens Dano ou cura for an NPC with its own title and width', async () => {
    const m = await setup({ master: true, matches: PHONE });
    m.view.adjustNpc('g');
    expect(m.opened[0].component).toBe(AdjustNpc);
    expect(m.opened[0].config.ariaLabel).toBe('Dano ou cura em Goblin');
    expect(data(m)).toMatchObject({ campaignId: 'c', encounterId: 'enc', combatant: { id: 'g' } });
    expect(data(m)['state']).toBe(m.state);
  });

  it('labels it by its title and makes it 440 px wide on a dialog', async () => {
    const m = await setup({ master: true });
    m.view.adjustNpc('g');
    expect(m.opened[0].config.ariaLabelledBy).toBe('adjust-npc-title');
    expect(m.opened[0].config.width).toBe('440px');
  });

  it('opens Dano ou cura for nobody who is not in the combat', async () => {
    const m = await setup({ master: true });
    m.view.adjustNpc('gone');
    expect(m.opened).toHaveLength(0);
  });

  it('asks how far for the character’s own movement', async () => {
    const m = await setup({ matches: PHONE });
    m.view.openSpend();
    expect(m.opened[0].component).toBe(SpendSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Gastar movimento');
    expect(data(m)).toMatchObject({ campaignId: 'c', encounterId: 'enc', combatantId: 't' });
    expect(data(m)['state']).toBe(m.state);
  });

  it('asks how far for the creature it is given', async () => {
    const m = await setup();
    m.view.openSpend('w');
    expect(data(m)['combatantId']).toBe('w');
    expect(m.opened[0].config.ariaLabelledBy).toBe('sheet-t');
  });

  it('clears the movement note when the spend sheet answers', async () => {
    const m = await setup();
    m.view.moveNote.set('Você parou antes.');
    m.view.openSpend();
    m.opened[0].close(true);
    expect(m.view.moveNote()).toBe('');
  });

  it('asks about nobody when the player has no character and no creature is given', async () => {
    const m = await setup({ master: true, enc: masterEnc() });
    m.view.openSpend();
    expect(m.opened).toHaveLength(0);
  });
});

describe('CombatView sheets: casting', () => {
  const missile = (over: object = {}) => ({
    spell: {
      key: 'spell:magic-missile',
      name: 'Magic Missile',
      namePt: 'Mísseis Mágicos',
      level: 1,
      concentration: true,
    },
    economy: ActionEconomy.BONUS_ACTION,
    enabled: true,
    slots: [
      { level: 1, pact: false, free: 2 },
      { level: 2, pact: false, free: 1 },
    ],
    metamagicOptions: [{ key: 'quickened', namePt: 'Magia Acelerada', cost: 2 }],
    sculptSpells: true,
    ...over,
  });
  const shield = (over: object = {}) => ({
    spell: { key: 'spell:shield', name: 'Shield', namePt: 'Escudo Arcano', level: 1 },
    economy: ActionEconomy.REACTION,
    enabled: true,
    slots: [
      { level: 1, pact: false, free: 2 },
      { level: 2, pact: false, free: 3 },
    ],
    ...over,
  });
  const fireBolt = {
    key: 'spell:fire-bolt',
    name: 'Fire Bolt',
    namePt: 'Raio de Fogo',
    kind: AttackKind.SPELL,
    saveDc: 0,
    attackBonus: 7,
    spellDice: '2d10',
  };
  const sacredFlame = {
    key: 'spell:sacred-flame',
    name: 'Sacred Flame',
    namePt: '',
    kind: AttackKind.SPELL,
    saveDc: 15,
    spellDice: '1d8',
  };
  const slotVitals = vitals({
    spellSlots: [
      { level: 1, total: 4, used: 1 },
      { level: 2, total: 2, used: 0 },
    ],
    pactSlots: { slotLevel: 3, total: 2, used: 1 },
    resources: [{ key: 'sorcery_points', total: 5, used: 2, recharge: 'long_rest' }],
  });
  const mount = (options: OptionsInit, rest: TurnInit = {}, extra: Partial<MountOptions> = {}) =>
    setup({ matches: PHONE, turnOptions: forToren(options, rest), ...extra });

  it('hands the cast sheet a spell field by field', async () => {
    const targets = {
      spellKey: 'spell:magic-missile',
      targets: [{ combatantId: 'g', label: 'Goblin' }],
      maxTargets: 3,
    };
    const m = await mount(
      { spells: [missile(), shield()], attacks: [{ enabled: true, attack: sword }] },
      { spellTargets: [targets] },
      { inputs: { vitals: [slotVitals] } },
    );
    m.view.sheets.castSheet('spell:magic-missile');
    expect(m.opened).toHaveLength(1);
    expect(m.opened[0].component).toBe(CastSheet);
    expect(m.opened[0].config.ariaLabel).toBe('Conjurar Mísseis Mágicos');
    const d = data(m);
    const options = m.view.options();
    expect(d).toMatchObject({
      campaignId: 'c',
      encounterId: 'enc',
      casterId: 't',
      round: 3,
      spellKey: 'spell:magic-missile',
      name: 'Mísseis Mágicos',
      level: 1,
      concentration: true,
      cantripDice: '',
      economy: ActionEconomy.BONUS_ACTION,
      usage: [
        { level: 1, total: 4, used: 1 },
        { level: 2, total: 2, used: 0 },
      ],
      pact: { slotLevel: 3, total: 2, used: 1 },
      shieldFree: 5,
      shieldName: 'Escudo Arcano',
      sculptSpells: true,
      sorceryPoints: { left: 3, total: 5 },
      attackBonus: 0,
      master: false,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      map: null,
    });
    expect(d['slots']).toBe(options.options.spells[0].slots);
    expect(d['metamagic']).toBe(options.options.spells[0].metamagicOptions);
    expect(d['targets']).toBe(options.spellTargets[0]);
    expect(d['state']).toBe(m.state);
    expect(d['resume']).toBeUndefined();
  });

  it('hands the sheet empty slots, no pact, no points and no Escudo when the character has none', async () => {
    const m = await mount({ spells: [missile({ metamagicOptions: [], sculptSpells: false })] });
    m.view.sheets.castSheet('spell:magic-missile');
    expect(data(m)).toMatchObject({
      usage: [],
      pact: null,
      sorceryPoints: null,
      shieldFree: null,
      shieldName: 'Escudo Arcano',
      metamagic: [],
      sculptSpells: false,
    });
    expect(data(m)['targets']).toBeUndefined();
  });

  it('counts the sorcery points left of the resource, never below zero', async () => {
    const m = await mount(
      { spells: [missile()] },
      {},
      {
        inputs: {
          vitals: [
            vitals({
              resources: [{ key: 'sorcery_points', total: 2, used: 4, recharge: 'long_rest' }],
            }),
          ],
        },
      },
    );
    m.view.sheets.castSheet('spell:magic-missile');
    expect(data(m)['sorceryPoints']).toEqual({ left: 0, total: 2 });
  });

  it('takes Escudo name from the list and counts its free slots, none when it has none', async () => {
    const m = await mount({
      spells: [
        missile(),
        shield({ spell: { key: 'spell:shield', name: 'Shield', namePt: '', level: 1 }, slots: [] }),
      ],
    });
    m.view.sheets.castSheet('spell:magic-missile');
    expect(data(m)['shieldFree']).toBe(0);
    expect(data(m)['shieldName']).toBe('Escudo Arcano');
  });

  it('names Escudo as the list names it', async () => {
    const m = await mount({
      spells: [
        missile(),
        shield({ spell: { key: 'spell:shield', name: 'Shield', namePt: 'Escudo', level: 1 } }),
      ],
    });
    m.view.sheets.castSheet('spell:magic-missile');
    expect(data(m)['shieldName']).toBe('Escudo');
  });

  it('names the spell by its English name when it has no Portuguese one', async () => {
    const m = await mount({
      spells: [missile({ spell: { key: 'spell:magic-missile', name: 'Magic Missile', level: 1 } })],
    });
    m.view.sheets.castSheet('spell:magic-missile');
    expect(data(m)['name']).toBe('Magic Missile');
    expect(m.opened[0].config.ariaLabel).toBe('Conjurar Magic Missile');
  });

  it('casts a cantrip of the attacks as level 0 with its dice and the economy of the action', async () => {
    const m = await mount({
      attacks: [
        { enabled: true, attack: sword },
        { enabled: true, attack: fireBolt },
      ],
    });
    m.view.sheets.castSheet('spell:fire-bolt');
    expect(m.opened[0].config.ariaLabel).toBe('Conjurar Raio de Fogo');
    expect(data(m)).toMatchObject({
      spellKey: 'spell:fire-bolt',
      name: 'Raio de Fogo',
      level: 0,
      concentration: false,
      cantripDice: '2d10',
      economy: ActionEconomy.ACTION,
      metamagic: [],
      sculptSpells: false,
      attackBonus: 7,
    });
    expect(data(m)['slots']).toEqual([]);
  });

  it('names a cantrip with no Portuguese name by its English one, and takes a save cantrip’s DC less 8', async () => {
    const m = await mount({ attacks: [{ enabled: true, attack: sacredFlame }] });
    m.view.sheets.castSheet('spell:sacred-flame');
    expect(data(m)).toMatchObject({ name: 'Sacred Flame', cantripDice: '1d8', attackBonus: 7 });
  });

  it('skips an attack option that carries no attack when it looks for the spell attack bonus', async () => {
    const m = await mount({ attacks: [{ enabled: false }, { enabled: true, attack: fireBolt }] });
    m.view.sheets.castSheet('spell:fire-bolt');
    expect(data(m)['attackBonus']).toBe(7);
  });

  it('prefers the spell attack bonus over the DC of a save cantrip', async () => {
    const m = await mount({
      attacks: [
        { enabled: true, attack: sacredFlame },
        { enabled: true, attack: fireBolt },
      ],
    });
    m.view.sheets.castSheet('spell:sacred-flame');
    expect(data(m)['attackBonus']).toBe(7);
  });

  it('names a key that is neither a spell nor an attack by itself, at level 0', async () => {
    const m = await mount({ spells: [missile()] });
    m.view.sheets.castSheet('spell:unknown');
    expect(data(m)).toMatchObject({
      name: 'spell:unknown',
      level: 0,
      concentration: false,
      cantripDice: '',
      economy: ActionEconomy.ACTION,
      slots: [],
      metamagic: [],
      sculptSpells: false,
    });
    expect(m.opened[0].config.ariaLabel).toBe('Conjurar spell:unknown');
  });

  it('opens at the damage with the pending hits, and labels it so', async () => {
    const m = await mount({ spells: [missile()] });
    const owed = [pending({ attackKey: 'spell:magic-missile' })];
    m.view.sheets.castSheet('spell:magic-missile', owed);
    expect(m.opened[0].config.ariaLabel).toBe('Rolar o dano de Mísseis Mágicos');
    expect(data(m)['resume']).toBe(owed);
  });

  it('opens nothing without a character, or before the options arrive', async () => {
    const master = await mount({ spells: [missile()] }, {}, { master: true, enc: masterEnc() });
    master.view.sheets.castSheet('spell:magic-missile');
    expect(master.opened).toHaveLength(0);
    const early = mountCombatView({
      enc: theatre(),
      turnOptions: forToren({ spells: [missile()] }),
    });
    early.view.sheets.castSheet('spell:magic-missile');
    expect(early.opened).toHaveLength(0);
  });

  describe('on the battle map', () => {
    const area = (placement: AreaPlacement) => ({
      spellKey: 'spell:fireball',
      targets: [],
      placement,
    });
    const fireball = {
      spell: { key: 'spell:fireball', name: 'Fireball', namePt: 'Bola de Fogo', level: 3 },
      economy: ActionEconomy.ACTION,
      enabled: true,
      slots: [{ level: 3, pact: false, free: 2 }],
    };
    const onTheMap = (placement?: AreaPlacement, extra: Partial<MountOptions> = {}) =>
      setup({
        matches: PHONE,
        enc: onMap(),
        mapState: mapWithImage(),
        turnOptions: forToren(
          { spells: [fireball] },
          placement === undefined ? {} : { spellTargets: [area(placement)] },
        ),
        ...extra,
      });

    it('hands the sheet the map: the picture, the grid, the layers and no fog', async () => {
      const m = await onTheMap(AreaPlacement.POINT);
      m.view.sheets.castSheet('spell:fireball');
      expect(data(m)['map']).toEqual({
        image: { url: '/m.png', width: 1000, height: 700 },
        mapName: 'Estrada na floresta',
        columns: 20,
        rows: 14,
        layers: m.view.layers(),
        fog: null,
      });
    });

    it.each([
      ['a point', AreaPlacement.POINT],
      ['a direction', AreaPlacement.DIRECTION],
    ])('opens on the map when the area is placed by %s', async (_name, placement) => {
      const m = await onTheMap(placement);
      m.view.sheets.castSheet('spell:fireball');
      expect(m.opened[0].config.autoFocus).toBe('[role="application"]');
    });

    it.each([
      ['on the caster', AreaPlacement.CASTER],
      ['unspecified', AreaPlacement.UNSPECIFIED],
    ])('opens on the title when the area is %s', async (_name, placement) => {
      const m = await onTheMap(placement);
      m.view.sheets.castSheet('spell:fireball');
      expect(m.opened[0].config.autoFocus).toBe('first-heading');
    });

    it('opens on the title when the spell has no targets entry', async () => {
      const m = await onTheMap();
      m.view.sheets.castSheet('spell:fireball');
      expect(m.opened[0].config.autoFocus).toBe('first-heading');
    });

    it('opens on the title when it opens at the damage', async () => {
      const m = await onTheMap(AreaPlacement.POINT);
      m.view.sheets.castSheet('spell:fireball', [pending({ attackKey: 'spell:fireball' })]);
      expect(m.opened[0].config.autoFocus).toBe('first-heading');
    });

    it('gives no map, and keeps the title first, in a combat without a map', async () => {
      const m = await setup({
        matches: PHONE,
        mapState: mapWithImage(),
        turnOptions: forToren(
          { spells: [fireball] },
          { spellTargets: [area(AreaPlacement.POINT)] },
        ),
      });
      m.view.sheets.castSheet('spell:fireball');
      expect(data(m)['map']).toBeNull();
      expect(m.opened[0].config.autoFocus).toBe('first-heading');
    });

    it('gives no map while the map’s picture is not the combat’s', async () => {
      const mapState = mapWithImage();
      mapState.map.set(
        create(MapSchema, { id: 'other', image: { url: '/o.png', width: 1, height: 1 } }),
      );
      const m = await onTheMap(AreaPlacement.POINT, { mapState });
      m.view.sheets.castSheet('spell:fireball');
      expect(data(m)['map']).toBeNull();
    });
  });

  describe('through openCast', () => {
    const conjure = {
      spell: {
        key: 'spell:conjure-animals',
        name: 'Conjure Animals',
        namePt: 'Conjurar Animais',
        level: 3,
        concentration: true,
      },
      economy: ActionEconomy.ACTION,
      enabled: true,
    };
    const summons = (spellKeys: string[]) => ({
      summonOptions: vi.fn(async () => ({ spells: spellKeys.map((spellKey) => ({ spellKey })) })),
    });

    it('asks the creatures of a summoning spell, with the combat to answer to', async () => {
      const creatures = summons(['spell:conjure-animals']);
      const m = await mount(
        { spells: [conjure] },
        {},
        { enc: theatre({}, [toren({ concentrationSpellNamePt: 'Bênção' }), goblin()]), creatures },
      );
      await m.view.openCast('spell:conjure-animals');
      expect(m.opened[0].component).toBe(SummonSheet);
      expect(m.opened[0].config.ariaLabel).toBe('Conjurar Conjurar Animais');
      expect(data(m)).toMatchObject({
        campaignId: 'c',
        characterId: 'toren-c',
        spellKey: 'spell:conjure-animals',
        combat: {
          encounterId: 'enc',
          casterId: 't',
          diceMode: DiceMode.PLAYERS_CHOOSE,
          preference: DicePreference.APP,
          concentrating: 'Bênção',
        },
      });
      expect((data(m)['combat'] as { state: unknown }).state).toBe(m.state);
      expect(creatures.summonOptions).toHaveBeenCalledWith('c', 'toren-c');
    });

    it('is labelled by its title and 560 px wide on a dialog', async () => {
      const m = await mount(
        { spells: [conjure] },
        {},
        { matches: [], creatures: summons(['spell:conjure-animals']) },
      );
      await m.view.openCast('spell:conjure-animals');
      expect(m.opened[0].config.ariaLabelledBy).toBe('summon-t');
      expect(m.opened[0].config.width).toBe('560px');
    });

    it('names the summoning spell by its English name when it has no Portuguese one', async () => {
      const m = await mount(
        {
          spells: [
            {
              ...conjure,
              spell: { key: 'spell:conjure-animals', name: 'Conjure Animals', level: 3 },
            },
          ],
        },
        {},
        { creatures: summons(['spell:conjure-animals']) },
      );
      await m.view.openCast('spell:conjure-animals');
      expect(m.opened[0].config.ariaLabel).toBe('Conjurar Conjure Animals');
    });

    it('reads the summoning spells once for the character', async () => {
      const creatures = summons(['spell:conjure-animals']);
      const m = await mount({ spells: [conjure, missile()] }, {}, { creatures });
      await m.view.openCast('spell:conjure-animals');
      await m.view.openCast('spell:magic-missile');
      await m.view.openCast('spell:conjure-animals');
      expect(creatures.summonOptions).toHaveBeenCalledTimes(1);
      expect(m.opened.map((o) => o.component)).toEqual([SummonSheet, CastSheet, SummonSheet]);
    });

    it('casts a spell the character cannot summon with on the cast sheet', async () => {
      const m = await mount(
        { spells: [missile()] },
        {},
        { creatures: summons(['spell:conjure-animals']) },
      );
      await m.view.openCast('spell:magic-missile');
      expect(m.opened[0].component).toBe(CastSheet);
      expect(data(m)['spellKey']).toBe('spell:magic-missile');
    });

    it('asks again after a failed read, and casts on the cast sheet meanwhile', async () => {
      const summonOptions = vi
        .fn()
        .mockRejectedValueOnce(new Error('offline'))
        .mockResolvedValue({ spells: [{ spellKey: 'spell:conjure-animals' }] });
      const m = await mount({ spells: [conjure] }, {}, { creatures: { summonOptions } });
      await m.view.openCast('spell:conjure-animals');
      expect(m.opened.map((o) => o.component)).toEqual([CastSheet]);
      await m.view.openCast('spell:conjure-animals');
      expect(m.opened.map((o) => o.component)).toEqual([CastSheet, SummonSheet]);
      expect(summonOptions).toHaveBeenCalledTimes(2);
    });

    it('does not ask the summoning spells for a spell the options do not list', async () => {
      const creatures = summons(['spell:conjure-animals']);
      const m = await mount({ spells: [] }, {}, { creatures });
      await m.view.openCast('spell:conjure-animals');
      expect(creatures.summonOptions).not.toHaveBeenCalled();
      expect(m.opened[0].component).toBe(CastSheet);
    });

    it('opens Revivificar on its own sheet with the caster and the combat', async () => {
      const m = await mount({}, {}, { inputs: { casterClasses: 'Clérigo 5' } });
      await m.view.openCast('spell:revivify');
      expect(m.opened[0].component).toBe(RevivifySheet);
      expect(m.opened[0].config.ariaLabel).toBe('Revivificar');
      expect(data(m)).toMatchObject({
        campaignId: 'c',
        casterName: 'Toren',
        classes: 'Clérigo 5',
        casterCharacterId: 'toren-c',
        combat: { encounterId: 'enc', casterId: 't', round: 3 },
      });
      expect((data(m)['combat'] as { state: unknown }).state).toBe(m.state);
      expect((data(m)['reload'] as () => number)()).toBe(0);
    });

    it('opens no Revivificar sheet for the master, who has no caster of his own', async () => {
      const m = await mount({}, {}, { master: true, enc: masterEnc() });
      m.view.sheets.revivify();
      expect(m.opened).toHaveLength(0);
    });

    it('does not take a summoning read for a master', async () => {
      const creatures = summons(['spell:conjure-animals']);
      const m = await mount(
        { spells: [conjure] },
        {},
        { master: true, enc: masterEnc(), creatures },
      );
      expect(await m.view.sheets.isSummon('spell:conjure-animals')).toBe(false);
      expect(creatures.summonOptions).not.toHaveBeenCalled();
    });
  });
});

describe("CombatView sheets: the master's area spell", () => {
  const fireball = (over: object = {}) => ({
    spell: { key: 'spell:fireball', name: 'Fireball', namePt: 'Bola de Fogo', level: 3 },
    economy: ActionEconomy.ACTION,
    enabled: true,
    slots: [
      { level: 3, pact: false, free: 0 },
      { level: 4, pact: false, free: 2 },
    ],
    ...over,
  });
  const targets = (placement: AreaPlacement) => ({
    spellKey: 'spell:fireball',
    targets: [],
    placement,
  });
  const mount = (
    spell: object = fireball(),
    placement: AreaPlacement | null = AreaPlacement.POINT,
    extra: Partial<MountOptions> = {},
  ) =>
    setup({
      master: true,
      matches: PHONE,
      enc: onMap({
        currentCombatantId: 'g',
        combatants: [toren({ placed: true }), goblin({ placed: true, col: 4, row: 4 })],
      }),
      mapState: mapWithImage(),
      turnOptions: turns({
        g: {
          options: { spells: [spell as never] },
          spellTargets: placement === null ? [] : [targets(placement)],
        },
      }),
      ...extra,
    });

  it('hands the area sheet the caster on turn, the lowest free slot, the targets and the map', async () => {
    const m = await mount();
    m.view.openMasterArea('spell:fireball');
    expect(m.opened).toHaveLength(1);
    expect(m.opened[0].component).toBe(MasterAreaCast);
    expect(m.opened[0].config.ariaLabel).toBe('Goblin conjura Bola de Fogo');
    const d = data(m);
    expect(d).toMatchObject({
      campaignId: 'c',
      encounterId: 'enc',
      caster: { id: 'g' },
      spellKey: 'spell:fireball',
      name: 'Bola de Fogo',
      level: 3,
      slot: { level: 4, pact: false },
      economy: ActionEconomy.ACTION,
    });
    expect(d['targets']).toBe(m.view.options().spellTargets[0]);
    expect(d['map']).toMatchObject({
      mapName: 'Estrada na floresta',
      columns: 20,
      rows: 14,
      fog: null,
    });
    expect(d['state']).toBe(m.state);
  });

  it('is a tall bottom sheet on the map on a phone', async () => {
    const m = await mount();
    m.view.openMasterArea('spell:fireball');
    expect((m.opened[0].config as { panelClass?: string }).panelClass).toBe('mr-sheet-tall');
    expect(m.opened[0].config.autoFocus).toBe('[role="application"]');
  });

  it('is a 1100 px dialog labelled by its title on a computer', async () => {
    const m = await mount(fireball(), AreaPlacement.POINT, { matches: [] });
    m.view.openMasterArea('spell:fireball');
    expect(m.opened[0].config.width).toBe('1100px');
    expect(m.opened[0].config.ariaLabelledBy).toBe('sheet-t');
  });

  it('opens on the title for an area centered on the caster', async () => {
    const m = await mount(fireball(), AreaPlacement.CASTER);
    m.view.openMasterArea('spell:fireball');
    expect(m.opened[0].config.autoFocus).toBe('first-heading');
  });

  it('opens on the map for an area with no placement said', async () => {
    const m = await mount(fireball(), AreaPlacement.UNSPECIFIED);
    m.view.openMasterArea('spell:fireball');
    expect(m.opened[0].config.autoFocus).toBe('[role="application"]');
  });

  it('casts at the pact slot when only the pact offers one', async () => {
    const m = await mount(fireball({ slots: [{ level: 5, pact: true, free: 1 }] }));
    m.view.openMasterArea('spell:fireball');
    expect(data(m)['slot']).toEqual({ level: 5, pact: true });
  });

  it("casts at the spell's own level when no slot is free", async () => {
    const m = await mount(fireball({ slots: [{ level: 3, pact: false, free: 0 }] }));
    m.view.openMasterArea('spell:fireball');
    expect(data(m)['slot']).toEqual({ level: 3, pact: false });
  });

  it("casts at the spell's own level when the options list no slots", async () => {
    const m = await mount(fireball({ slots: [] }));
    m.view.openMasterArea('spell:fireball');
    expect(data(m)['slot']).toEqual({ level: 3, pact: false });
  });

  it('takes no slot for a cantrip', async () => {
    const m = await mount(
      fireball({
        spell: { key: 'spell:fireball', name: 'Fireball', namePt: 'Bola de Fogo', level: 0 },
      }),
    );
    m.view.openMasterArea('spell:fireball');
    expect(data(m)['slot']).toBeNull();
    expect(data(m)['level']).toBe(0);
  });

  it('names the spell by its English name when it has no Portuguese one', async () => {
    const m = await mount(
      fireball({ spell: { key: 'spell:fireball', name: 'Fireball', level: 3 } }),
    );
    m.view.openMasterArea('spell:fireball');
    expect(data(m)['name']).toBe('Fireball');
    expect(m.opened[0].config.ariaLabel).toBe('Goblin conjura Fireball');
  });

  it('opens nothing for a spell the options do not list or that has no targets', async () => {
    const unlisted = await mount();
    unlisted.view.openMasterArea('spell:other');
    expect(unlisted.opened).toHaveLength(0);
    const noTargets = await mount(fireball(), null);
    noTargets.view.openMasterArea('spell:fireball');
    expect(noTargets.opened).toHaveLength(0);
  });

  it('opens nothing in a combat without a map', async () => {
    const m = await setup({
      master: true,
      enc: theatre({ currentCombatantId: 'g' }),
      turnOptions: turns({
        g: {
          options: { spells: [fireball() as never] },
          spellTargets: [targets(AreaPlacement.POINT)],
        },
      }),
    });
    m.view.openMasterArea('spell:fireball');
    expect(m.opened).toHaveLength(0);
  });

  it('opens nothing when nobody is on turn', async () => {
    const m = await mount(fireball(), AreaPlacement.POINT, {
      enc: onMap({ currentCombatantId: '' }),
    });
    m.view.openMasterArea('spell:fireball');
    expect(m.opened).toHaveLength(0);
  });
});

describe('CombatView sheets: with no combat on screen', () => {
  const owned = {
    spell: {
      key: 'spell:magic-missile',
      name: 'Magic Missile',
      namePt: 'Mísseis Mágicos',
      level: 1,
    },
    economy: ActionEconomy.ACTION,
    enabled: true,
  };

  // The page can be asked for a sheet in the instant the combat goes away (a reading that came back empty): every opener
  // then does nothing, and calls nothing.
  it('opens no sheet and calls nothing', async () => {
    const m = await setup({
      turnOptions: forToren(
        { attacks: [{ enabled: true, attack: sword }], spells: [owned] },
        { pendingDamages: [pending()] },
      ),
    });
    m.state.clear();
    const window = reactionWindow({ id: 'w2', forYou: false } as never);
    m.view.openDoor({ col: 1, row: 1, kind: 2 });
    m.view.openConditions('g');
    m.view.adjustNpc('g');
    m.view.openSpend();
    m.view.openSpend('w');
    m.view.sheets.reaction(window);
    m.view.openEffectSave(window);
    m.view.sheets.throwBack('w2');
    m.view.sheets.keepMissile('w2');
    m.view.sheets.revivify();
    m.view.sheets.summon('spell:conjure-animals', 'Conjurar Animais');
    m.view.openMasterArea('spell:fireball');
    m.view.openAttack('attack:longsword');
    m.view.openCreatureAttack('w', 'attack:bite');
    m.view.rollCreatureDamage('w');
    m.view.rollPendingDamage();
    m.view.sheets.castSheet('spell:magic-missile');
    await m.view.openCast('spell:magic-missile');
    await m.view.useFeature('feature:rage');
    await settle(m.fixture);
    expect(m.view.openHeldRoll()).toBe(false);
    expect(m.view.sheets.castMap()).toBeNull();
    expect(m.opened).toHaveLength(0);
    expect(m.callsOf('answerReaction')).toHaveLength(0);
    expect(m.callsOf('takeAction')).toHaveLength(0);
  });

  it('opens no summoning sheet for the master, who has no caster of his own', async () => {
    const m = await setup({ master: true, enc: masterEnc() });
    m.view.sheets.summon('spell:conjure-animals', 'Conjurar Animais');
    expect(m.opened).toHaveLength(0);
  });

  it('gives no map to the cast sheet when there is no combat', async () => {
    const m = await setup({ enc: onMap(), mapState: mapWithImage() });
    expect(m.view.sheets.castMap()).not.toBeNull();
    m.state.clear();
    expect(m.view.sheets.castMap()).toBeNull();
  });
});
