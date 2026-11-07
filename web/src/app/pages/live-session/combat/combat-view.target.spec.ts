import { create } from '@bufbuild/protobuf';

import {
  AttackTargetsSchema,
  CombatantKind,
  CombatantSide,
  DartsAtSlotSchema,
  EncounterMode,
  SpellTargetsSchema,
  TargetInReachSchema,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  AttackKind,
  AttackOptionSchema,
  AttackSchema,
  SlotChoiceSchema,
  SpellOptionSchema,
  TurnOptionsSchema,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { PHONE_QUERY } from '../../../shared/map-view/media-query';
import { settle } from '../../../../test-settle';
import { AttackSheet } from './attack-sheet/attack-sheet';
import { CastSheet } from './cast-sheet/cast-sheet';
import { mountCombatView } from './combat-view-testing';

// Choosing the target on the map (C7-04, C7-05): "Atacar" and "Conjurar" bring the map into view; a tap names the target, "Confirmar" opens the sheet.

const LAPTOP = ['(min-width: 1024px)', '(min-width: 1280px)'];
const pensantus = combatant({
  id: 'p',
  label: 'Pensantus',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  characterId: 'p-c',
  mine: true,
  col: 2,
  row: 5,
});
const goblin1 = combatant({ id: 'g1', label: 'Goblin 1', col: 8, row: 5 });
const goblin2 = combatant({ id: 'g2', label: 'Goblin 2', col: 12, row: 3 });
const goblin3 = combatant({ id: 'g3', label: 'Goblin 3', col: 14, row: 12 });
const enc = encounter({
  mode: EncounterMode.GRID,
  currentCombatantId: 'p',
  gridColumns: 24,
  gridRows: 16,
  combatants: [pensantus, goblin1, goblin2, goblin3],
});

const who = (id: string, label: string, over: Record<string, unknown> = {}) =>
  create(TargetInReachSchema, { combatantId: id, label, ...over });
const sword = create(AttackSchema, {
  key: 'equipment:dagger',
  name: 'Dagger',
  namePt: 'Adaga',
  attackBonus: 4,
  damage: '1d4+2',
  damageTypePt: 'perfurante',
  kind: AttackKind.WEAPON,
  rangeFt: 20,
  longRangeFt: 60,
});
const turnOptions = () => ({
  options: create(TurnOptionsSchema, {
    attacks: [create(AttackOptionSchema, { attack: sword, enabled: true })],
    spells: [
      create(SpellOptionSchema, {
        spell: {
          key: 'spell:magic-missile',
          name: 'Magic Missile',
          namePt: 'Mísseis Mágicos',
          level: 1,
        },
        economy: ActionEconomy.ACTION,
        enabled: true,
        slots: [create(SlotChoiceSchema, { level: 1, free: 2 })],
      }),
    ],
  }),
  attackTargets: [
    create(AttackTargetsSchema, {
      attackKey: 'equipment:dagger',
      targets: [
        who('g1', 'Goblin 1'),
        who('g2', 'Goblin 2', { distanceFt: 40 }),
        who('g3', 'Goblin 3', { tooFar: true }),
      ],
    }),
  ],
  spellTargets: [
    create(SpellTargetsSchema, {
      spellKey: 'spell:magic-missile',
      targets: [who('g1', 'Goblin 1'), who('g2', 'Goblin 2')],
      darts: [create(DartsAtSlotSchema, { slotLevel: 1, darts: 3 })],
    }),
  ],
  maneuverDice: [],
});

async function mount(matches: readonly string[] = [PHONE_QUERY], map = true, master = false) {
  const m = mountCombatView({ enc, matches, turnOptions, master });
  if (map) {
    m.mapState.setMap({
      id: 'map',
      name: 'Cripta do Corvo',
      image: { url: '/m.png', width: 100, height: 100 },
    } as never);
  }
  m.fixture.detectChanges();
  await settle(m.fixture);
  const button = (label: string) =>
    [...m.el.querySelectorAll('button')].find((b) => {
      const copy = b.cloneNode(true) as HTMLElement;
      copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
      return (b.getAttribute('aria-label') ?? copy.textContent?.trim()) === label;
    })!;
  const press = async (label: string) => {
    button(label).click();
    m.fixture.detectChanges();
    await settle(m.fixture);
  };
  const tap = async (id: string) => {
    m.el.querySelector<HTMLButtonElement>(`.cm__pick[data-pick="${id}"]`)!.click();
    m.fixture.detectChanges();
    await settle(m.fixture);
  };
  return { ...m, button, press, tap };
}

describe('CombatView: choosing the target on the map, on a phone', () => {
  it('asks for the target on the map instead of opening the sheet, with the bar in the place of the turn’s bar', async () => {
    const m = await mount();
    await m.press('Atacar com Adaga');
    expect(m.opened).toHaveLength(0);
    const bar = m.el.querySelector('app-target-bar')!;
    expect(bar.textContent).toContain('Escolha o alvo · Adaga');
    expect(m.el.querySelector('app-turn-bar')).toBeNull();
    expect(m.el.querySelector('app-combat-map-card section')?.classList).toContain('card--pick');
    expect(
      [...m.el.querySelectorAll('.cm__pick')].map((b) => b.getAttribute('aria-label')),
    ).toEqual([
      'Escolher Goblin 1 como alvo',
      'Escolher Goblin 2 como alvo. Longe: desvantagem',
      'Goblin 3: fora de alcance',
    ]);
    expect(m.el.querySelector('.cm__far')?.textContent).toBe('Longe: desvantagem');
    expect(m.el.querySelector('app-action-row [class="row__choosing"]')?.textContent).toBe(
      'Escolhendo o alvo',
    );
  });

  it('names the tapped target, and Confirmar opens the attack sheet at the roll with it', async () => {
    const m = await mount();
    await m.press('Atacar com Adaga');
    await m.tap('g1');
    expect(m.el.querySelector('app-target-bar')?.textContent).toContain('Adaga em Goblin 1');
    await m.press('Confirmar');
    expect(m.opened).toHaveLength(1);
    expect(m.opened[0].component).toBe(AttackSheet);
    expect(m.opened[0].config.data['target']).toBe('g1');
    expect(m.el.querySelector('app-target-bar')).toBeNull();
    expect(m.el.querySelector('app-turn-bar')).not.toBeNull();
  });

  it('says why a dimmed creature cannot be the target and keeps Confirmar off', async () => {
    const m = await mount();
    await m.press('Atacar com Adaga');
    await m.tap('g3');
    expect(m.el.querySelector('.cm__why')?.textContent).toContain('Goblin 3: fora de alcance');
    expect(m.button('Confirmar').getAttribute('aria-disabled')).toBe('true');
  });

  it('cancels back to the list of actions with nothing opened', async () => {
    const m = await mount();
    await m.press('Atacar com Adaga');
    await m.press('Cancelar');
    expect(m.opened).toHaveLength(0);
    expect(m.el.querySelector('app-target-bar')).toBeNull();
    expect(m.el.querySelector('.cm__pick')).toBeNull();
  });

  it('opens the sheet with its own list through "Escolher na lista"', async () => {
    const m = await mount();
    await m.press('Atacar com Adaga');
    await m.press('Escolher na lista');
    expect(m.opened).toHaveLength(1);
    expect(m.opened[0].component).toBe(AttackSheet);
    expect(m.opened[0].config.data['target']).toBeUndefined();
  });

  it('hands out the darts of Mísseis Mágicos one tap each and opens the cast sheet with them', async () => {
    const m = await mount();
    await m.press('Conjurar Mísseis Mágicos');
    expect(m.el.querySelector('app-target-bar')?.textContent).toContain(
      'Mísseis Mágicos · 0 de 3 dardos',
    );
    await m.tap('g1');
    await m.tap('g1');
    expect(m.el.querySelector('app-target-bar')?.textContent).toContain('2 de 3 dardos');
    expect(m.el.querySelector('.cm__mark-n')?.textContent?.replace(/\s+/g, '')).toBe('×2');
    await m.press('Desfazer');
    expect(m.view.targeting.mode.picks()).toEqual(['g1']);
    await m.tap('g2');
    await m.tap('g2');
    await m.press('Confirmar');
    expect(m.opened[0].component).toBe(CastSheet);
    const preset = m.opened[0].config.data['preset'] as { dealt: Map<string, number> };
    expect([...preset.dealt]).toEqual([
      ['g1', 1],
      ['g2', 2],
    ]);
  });

  it('leaves the master’s sheet, a combat without a map and the reaction as they are', async () => {
    const noMap = await mount([PHONE_QUERY], false);
    noMap.view.openAttack('equipment:dagger');
    expect(noMap.opened).toHaveLength(1);
    expect(noMap.view.targeting.mode.active()).toBe(false);
    const reaction = await mount();
    reaction.view.openAttack('equipment:dagger', true);
    expect(reaction.opened).toHaveLength(1);
    expect(reaction.view.targeting.mode.active()).toBe(false);
  });

  it('ends the choice when the turn passes', async () => {
    const m = await mount();
    await m.press('Atacar com Adaga');
    m.state.apply({ ...enc, currentCombatantId: 'g1' } as never);
    m.fixture.detectChanges();
    await settle(m.fixture);
    expect(m.view.targeting.mode.active()).toBe(false);
  });
});

describe('CombatView: choosing the target on the map, at 1280', () => {
  it('shows the pending card under the action tabs, tags the row and highlights the map', async () => {
    const m = await mount(LAPTOP);
    await m.press('Atacar com Adaga');
    const groups = m.el.querySelector('app-action-groups')!;
    expect(groups.querySelector('app-target-bar')?.textContent).toContain('Escolha o alvo no mapa');
    expect(groups.querySelector('.row__choosing')?.textContent).toBe('Escolhendo o alvo');
    expect(m.el.querySelector('app-combat-map-card .bar__pick')?.textContent).toContain(
      'Toque no alvo',
    );
    expect(m.el.querySelector('app-turn-bar')).not.toBeNull();
    await m.tap('g2');
    expect(groups.querySelector('app-target-bar')?.textContent).toContain('Adaga em Goblin 2');
    await m.press('Confirmar');
    expect(m.opened[0].config.data['target']).toBe('g2');
  });

  it('has exactly one "Mover" and one "Encerrar turno" while choosing', async () => {
    const m = await mount(LAPTOP);
    await m.press('Atacar com Adaga');
    const named = (name: string) =>
      [...m.el.querySelectorAll('button')].filter((b) => b.textContent?.trim().endsWith(name));
    expect(named('Mover')).toHaveLength(1);
    // The turn bar of the phone is `display: none` from 1024px: the banner's is the one on the screen.
    expect(m.el.querySelectorAll('app-turn-panel .banner button.end')).toHaveLength(1);
  });
});
