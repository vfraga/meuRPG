import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';

import {
  CombatantKind,
  JumpKind,
  OpportunityAttackSchema,
  OpportunityOfferSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { AttackKind, AttackSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from './combat-testing';
import {
  attackLabel,
  barText,
  masterAsk,
  masterNews,
  offersHolding,
  offersOn,
  offersToAnswer,
  playerQuestion,
  reachingAttacks,
  spendText,
  reactorAttacks,
  reactorIsMasters,
  waitingText,
} from './opportunity';

const plain = (t: string) => t.replace(/\u00a0/g, ' ');

// E9-13: Toren leaves the reach of Goblin 2.
const toGoblin = create(OpportunityOfferSchema, {
  id: 'o1',
  moverId: 'toren',
  moverLabel: 'Toren',
  reactorId: 'g2',
  reactorLabel: 'Goblin 2',
  forYou: true,
  leftCol: 17,
  leftRow: 6,
  attacks: [create(OpportunityAttackSchema, { key: 'attack:scimitar', namePt: 'Cimitarra' })],
});
const toToren = create(OpportunityOfferSchema, {
  id: 'o2',
  moverId: 'g2',
  moverLabel: 'Goblin 2',
  reactorId: 'toren',
  reactorLabel: 'Toren',
  forYou: true,
  attacks: [create(OpportunityAttackSchema, { key: 'attack:longsword', namePt: 'Espada longa' })],
});
const unseen = create(OpportunityOfferSchema, {
  id: 'o3',
  moverId: 'toren',
  moverLabel: 'Toren',
  reactorId: '',
  reactorLabel: '',
  forYou: false,
});

const toren = combatant({
  id: 'toren',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  mine: true,
  characterId: 'toren-c',
});
const g2 = combatant({ id: 'g2', label: 'Goblin 2' });

describe('opportunity attacks in words', () => {
  it('lists what the caller answers, and what holds a mover', () => {
    const e = encounter({ combatants: [toren, g2], opportunityOffers: [toGoblin, unseen] });
    expect(offersToAnswer(e).map((o) => o.id)).toEqual(['o1']);
    expect(offersOn(e, 'toren').map((o) => o.id)).toEqual(['o1', 'o3']);
    expect(reactorIsMasters(e, toGoblin)).toBe(true);
    expect(reactorIsMasters(encounter({ combatants: [toren, g2] }), toToren)).toBe(false);
  });

  it('names the creature that would react when it is not the character: "do alcance do Lobo atroz 1", "Gasta a reação do Lobo atroz 1"', () => {
    const toWolf = { ...toToren, reactorLabel: 'Lobo atroz 1' } as typeof toToren;
    expect(playerQuestion(toWolf, false)).toBe(
      'O Goblin 2 está saindo do alcance do Lobo atroz 1. Ataque de oportunidade?',
    );
    expect(spendText(toWolf, false)).toBe(
      'Gasta a reação do Lobo atroz 1: ela só volta no começo do próximo turno do Lobo atroz 1.',
    );
    expect(spendText(toToren)).toBe(
      'Gasta a sua reação: ela só volta no começo do seu próximo turno.',
    );
  });

  it('holds a mover for the offers on the whole turn of the caller, creatures they control included', () => {
    const wolf = combatant({
      id: 'wolf',
      label: 'Lobo atroz 1',
      kind: CombatantKind.CREATURE,
      controlledByMe: true,
    });
    const onWolf = create(OpportunityOfferSchema, { ...toGoblin, id: 'o4', moverId: 'wolf' });
    const onGoblin = create(OpportunityOfferSchema, { ...toGoblin, id: 'o5', moverId: 'g2' });
    const e = encounter({
      combatants: [toren, wolf, g2],
      turnGroupIds: ['toren', 'wolf', 'g2'],
      opportunityOffers: [toGoblin, onWolf, onGoblin],
    });
    expect(offersHolding(e, 'toren').map((o) => o.id)).toEqual(['o1', 'o4']);
  });

  it("counts a reactor the caller cannot see among the master's", () => {
    const e = encounter({ combatants: [toren, g2] });
    expect(reactorIsMasters(e, create(OpportunityOfferSchema, { reactorId: 'ghost' }))).toBe(true);
    expect(reactorIsMasters(e, toGoblin)).toBe(true);
    expect(reactorIsMasters(e, toToren)).toBe(false);
  });

  it('says "podem fazer ataques" when more than one of the master\'s can react', () => {
    const g3 = combatant({ id: 'g3', label: 'Goblin 3' });
    const e = encounter({ combatants: [toren, g2, g3] });
    const two = [
      create(OpportunityOfferSchema, { ...toGoblin, forYou: false }),
      create(OpportunityOfferSchema, {
        ...toGoblin,
        id: 'o6',
        reactorId: 'g3',
        reactorLabel: 'Goblin 3',
        forYou: false,
      }),
    ];
    expect(plain(waitingText(e, two)!.detail)).toContain(
      'O Goblin 2 e o Goblin 3 podem fazer ataques de oportunidade.',
    );
  });

  it('adds what the master said only when the mover was moved by hand, in the gender of the mover', () => {
    const byHand = (moverLabel: string) =>
      create(OpportunityOfferSchema, { ...toToren, moverLabel, byHand: true });
    expect(spendText(toToren)).toBe(
      'Gasta a sua reação: ela só volta no começo do seu próximo turno.',
    );
    expect(spendText(byHand('Goblin 2'))).toBe(
      'Gasta a sua reação: ela só volta no começo do seu próximo turno. O mestre disse que ele saiu do seu alcance.',
    );
    expect(spendText(byHand('Brisa'))).toBe(
      'Gasta a sua reação: ela só volta no começo do seu próximo turno. O mestre disse que ela saiu do seu alcance.',
    );
  });

  it('asks the player, and tells the master', () => {
    expect(playerQuestion(toToren)).toBe(
      'O Goblin 2 está saindo do seu alcance. Ataque de oportunidade?',
    );
    expect(plain(masterNews(toGoblin))).toBe('O Toren saiu do alcance do Goblin 2.');
    // Out of the reach by a long jump, the word is "saltou".
    expect(
      plain(masterNews(create(OpportunityOfferSchema, { ...toGoblin, jump: JumpKind.LONG }))),
    ).toBe('O Toren saltou para fora do alcance do Goblin 2.');
    expect(plain(masterAsk(toGoblin))).toBe('Goblin 2 ataca o Toren?');
  });

  it('says who the mover waits for, and names no reactor it does not see', () => {
    const e = encounter({ combatants: [toren, g2] });
    const mine = waitingText(e, [create(OpportunityOfferSchema, { ...toGoblin, forYou: false })]);
    expect(mine?.title).toBe('Esperando a reação do mestre');
    expect(plain(mine!.detail)).toContain('O Goblin 2 pode fazer um ataque de oportunidade.');
    const hidden = waitingText(e, [unseen]);
    expect(hidden).toEqual({
      title: 'Esperando o mestre',
      detail: 'Seu movimento já valeu; a sua vez continua quando ele responder.',
    });
    expect(hidden?.detail).not.toContain('Goblin');
    expect(waitingText(e, [])).toBeNull();
    const aPlayer = waitingText(e, [
      create(OpportunityOfferSchema, { ...toToren, moverId: 'x', forYou: false }),
    ]);
    expect(aPlayer?.title).toBe('Esperando a reação do jogador de Toren');
  });

  it("says the master's wait on the bar", () => {
    const e = encounter({ combatants: [toren, g2], opportunityOffers: [toGoblin] });
    expect(barText(e, () => '')).toBe('Esperando a sua reação: Goblin 2');
    const player = encounter({ combatants: [toren, g2], opportunityOffers: [toToren] });
    expect(barText(player, () => 'Caio')).toBe('Esperando a reação do Caio (Toren)');
    expect(barText(encounter({ combatants: [toren] }), () => '')).toBe('');
  });

  it("puts numbers on the offer's attacks from the reactor's options", () => {
    const scimitar = create(AttackSchema, {
      key: 'attack:scimitar',
      name: 'Scimitar',
      namePt: 'Cimitarra',
      kind: AttackKind.WEAPON,
      attackBonus: 4,
      damage: '1d6+2',
      damageTypePt: 'cortante',
      rangeFt: 5,
    });
    const [a] = reactorAttacks(toGoblin, [scimitar]);
    expect(plain(a.detail)).toBe('Cimitarra +4 · 1d6 + 2 cortante');
    expect(attackLabel(a)).toBe('Atacar com Cimitarra');
    // An attack the options do not list is still offered by its name.
    const [b] = reactorAttacks(toToren, []);
    expect(b.attack).toBeNull();
    expect(attackLabel(b)).toBe('Atacar com Espada longa');
  });
});

describe('reachingAttacks', () => {
  const weapon = (key: string, extra: Record<string, unknown>) =>
    create(AttackSchema, { key, kind: AttackKind.WEAPON, melee: true, saveDc: 0, ...extra });
  const options = (attacks: ReturnType<typeof weapon>[], targets: Record<string, unknown[]>) =>
    ({
      options: { attacks: attacks.map((attack) => ({ attack })) },
      attackTargets: Object.entries(targets).map(([attackKey, list]) => ({
        attackKey,
        targets: list,
      })),
    }) as never;

  it('offers a reach weapon at 10 ft, which the server says is within reach', () => {
    const glaive = weapon('glaive', { namePt: 'Glaive', rangeFt: 10 });
    const sword = weapon('sword', { namePt: 'Espada', rangeFt: 5 });
    const found = reachingAttacks(
      options([glaive, sword], {
        glaive: [{ distanceFt: 10, tooFar: false }],
        sword: [{ distanceFt: 10, tooFar: true }],
      }),
    );
    expect(found).toEqual([{ key: 'glaive', name: 'Glaive' }]);
  });

  it('offers a thrown weapon for a target within the reach a weapon rule gives it, not beyond', () => {
    const dagger = weapon('dagger', { namePt: 'Adaga', rangeFt: 20, longRangeFt: 60, reachFt: 30 });
    expect(
      reachingAttacks(options([dagger], { dagger: [{ distanceFt: 30, tooFar: false }] })),
    ).toHaveLength(1);
    expect(
      reachingAttacks(options([dagger], { dagger: [{ distanceFt: 35, tooFar: false }] })),
    ).toEqual([]);
  });

  it('offers a thrown weapon only for a target next to the player, whatever its range', () => {
    const dagger = weapon('dagger', { namePt: 'Adaga', rangeFt: 20, longRangeFt: 60 });
    expect(
      reachingAttacks(options([dagger], { dagger: [{ distanceFt: 15, tooFar: false }] })),
    ).toEqual([]);
    expect(
      reachingAttacks(options([dagger], { dagger: [{ distanceFt: 5, tooFar: false }] })),
    ).toHaveLength(1);
  });

  it('offers nothing for a target without a distance, a spell or a ranged weapon', () => {
    const bow = weapon('bow', { melee: false, rangeFt: 150 });
    expect(reachingAttacks(options([bow], { bow: [{ distanceFt: 5, tooFar: false }] }))).toEqual(
      [],
    );
    const sword = weapon('sword', { rangeFt: 5 });
    expect(reachingAttacks(options([sword], { sword: [{ tooFar: false }] }))).toEqual([]);
  });
});
