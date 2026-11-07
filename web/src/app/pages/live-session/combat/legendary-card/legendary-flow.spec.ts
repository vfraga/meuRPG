import { Injector, runInInjectionContext, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import type { Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { legendaryCard } from '../../../../core/combat/legendary-card';
import type { AttackOpen } from '../combat-sheets-context';
import { LegendaryFlow } from './legendary-flow';
import type { ArmedMove } from './legendary-offers';

const options = [
  {
    key: 'tail',
    name: 'Tail Attack',
    namePt: 'Ataque de cauda',
    cost: 1,
    kind: 'attack',
    text: '',
    affordable: true,
    attackKey: 'm#tail',
  },
  {
    key: 'wing',
    name: 'Wing Attack',
    namePt: 'Ataque de asas',
    cost: 2,
    kind: 'move',
    text: '',
    affordable: true,
    attackKey: '',
  },
];

const dragon = () =>
  combatant({
    id: 'd',
    label: 'Dragão vermelho adulto',
    legendary: {
      actionsPerRound: 3,
      actionsLeft: 2,
      resistancePerDay: 3,
      resistanceLeft: 3,
      usableNow: true,
      options,
    },
  } as never);

const combat = (over: object = {}) =>
  encounter({
    id: 'enc',
    combatants: [dragon(), combatant({ id: 't', label: 'Tor' })],
    currentCombatantId: 't',
    ...over,
  } as never);

describe('the page side of the legendary cards', () => {
  const turnOptions = vi.fn();
  const attacks: AttackOpen[] = [];

  function setup(start: Encounter | null = combat()) {
    attacks.length = 0;
    turnOptions
      .mockReset()
      .mockResolvedValue({ options: { attacks: [{ attack: { key: 'm#tail' } }] } });
    TestBed.resetTestingModule();
    const encounterSignal = signal<Encounter | null>(start);
    const error = signal('');
    const armed = signal<ArmedMove | null>(null);
    const flow = runInInjectionContext(
      TestBed.inject(Injector),
      () =>
        new LegendaryFlow({
          api: { moves: { turnOptions } } as never,
          campaignId: () => 'camp',
          encounter: encounterSignal,
          attack: (req) => attacks.push(req),
          error,
          armed,
        }),
    );
    TestBed.tick();
    const pick = (key: string) => {
      const e = encounterSignal() as Encounter;
      const card = legendaryCard(e, e.combatants[0]);
      return { combatantId: 'd', row: card!.rows.find((r) => r.key === key)! };
    };
    return { flow, encounterSignal, error, armed, pick };
  }

  it('opens the attack sheet of the creature with the option, its title and what is left', async () => {
    const { flow, pick } = setup();
    await flow.attack(pick('tail'));
    expect(turnOptions).toHaveBeenCalledWith('camp', 'enc', 'd');
    expect(attacks).toHaveLength(1);
    expect(attacks[0].key).toBe('m#tail');
    expect(attacks[0].who?.id).toBe('d');
    expect(attacks[0].legendary).toEqual({
      option: 'tail',
      title: 'Ataque de cauda · ação lendária',
      subtitle: 'Dragão vermelho adulto · custa 1 de 2 restantes',
      cost: 1,
      label: 'Dragão vermelho adulto',
    });
    expect(attacks[0].asReaction).toBeUndefined();
  });

  it("says nothing was spent when the attack is not among the creature's options", async () => {
    const { flow, pick, error } = setup();
    turnOptions.mockResolvedValue({ options: { attacks: [] } });
    await flow.attack(pick('tail'));
    expect(attacks).toHaveLength(0);
    expect(error()).toBe('Esse ataque não está na ficha da criatura. Nada foi gasto.');
  });

  it('says why when the options cannot be read', async () => {
    const { flow, pick, error } = setup();
    turnOptions.mockRejectedValue(new Error('down'));
    await flow.attack(pick('tail'));
    expect(attacks).toHaveLength(0);
    expect(error()).not.toBe('');
  });

  it('keeps the same names object while nothing about them changes', () => {
    const { flow, encounterSignal } = setup();
    const before = flow.names();
    encounterSignal.set(combat({ revision: 9 }));
    TestBed.tick();
    expect(flow.names()).toBe(before);
  });

  it('takes one tap at a time on an attack while its options are read', async () => {
    const { flow, pick } = setup();
    const first = flow.attack(pick('tail'));
    expect(flow.opening()).toEqual({ combatantId: 'd', key: 'tail' });
    await flow.attack(pick('tail'));
    await first;
    expect(turnOptions).toHaveBeenCalledTimes(1);
    expect(flow.opening()).toBeNull();
  });

  it('arms the move of an option, and the next turn takes it back', () => {
    const { flow, pick, armed, encounterSignal } = setup();
    flow.move(pick('wing'));
    expect(armed()).toEqual({ combatantId: 'd', optionKey: 'wing' });
    encounterSignal.set(combat({ revision: 4 }));
    TestBed.tick();
    expect(armed()).not.toBeNull();
    encounterSignal.set(combat({ currentCombatantId: 'd' }));
    TestBed.tick();
    expect(armed()).toBeNull();
  });

  it("keeps the options' names of every creature seen, also after one leaves the combat", () => {
    const { flow, encounterSignal } = setup();
    expect(flow.names().get('d')?.get('Tail Attack')).toBe('Ataque de cauda');
    encounterSignal.set(combat({ combatants: [combatant({ id: 't', label: 'Tor' })] }));
    TestBed.tick();
    expect(flow.names().get('d')?.get('Wing Attack')).toBe('Ataque de asas');
  });
});
