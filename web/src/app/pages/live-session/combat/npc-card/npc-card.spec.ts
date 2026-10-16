import { TestBed } from '@angular/core/testing';

import { AreaPlacement } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { AdvantageSourceKind, RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { CombatUndone } from '../../../../core/combat/combat-undone';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { NpcCard } from './npc-card';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

describe('NpcCard: the idempotency key follows the target', () => {
  const opts = (targets: string[]) =>
    ({
      options: {
        attacks: [
          {
            attack: {
              key: 'a1',
              saveDc: 0,
              attackBonus: 4,
              namePt: 'Espada',
              damage: '1d6',
              damageTypePt: 'cortante',
              rangeFt: 5,
              longRangeFt: 0,
            },
          },
        ],
      },
      attackTargets: [
        { attackKey: 'a1', targets: targets.map((id) => ({ combatantId: id, label: id })) },
      ],
      pendingDamages: [],
    }) as never;

  it('sends a new key when an options refresh moves the target after a failed roll', async () => {
    TestBed.resetTestingModule();
    const rollAttack = vi
      .fn()
      .mockRejectedValueOnce(new Error('lost'))
      .mockRejectedValue(new Error('lost again'));
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: fakeCombatClient({ rollAttack }) }],
    });
    const fixture = TestBed.createComponent(NpcCard);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [combatant({ id: 'npc', label: 'Goblin' })] }),
    );
    fixture.componentRef.setInput('subject', combatant({ id: 'npc', label: 'Goblin' }));
    fixture.componentRef.setInput('state', { apply: vi.fn() } as unknown as CombatState);
    fixture.componentRef.setInput('options', opts(['t1', 't2']));
    fixture.detectChanges();
    await fixture.whenStable();
    const card = fixture.componentInstance as unknown as {
      targetId(): string;
      rollApp(): Promise<void>;
    };
    expect(card.targetId()).toBe('t1');

    await card.rollApp();
    expect(rollAttack).toHaveBeenCalledTimes(1);

    // The options refresh: t1 is gone, the effect picks t2 by itself.
    fixture.componentRef.setInput('options', opts(['t2', 't3']));
    fixture.detectChanges();
    await fixture.whenStable();
    expect(card.targetId()).toBe('t2');

    await card.rollApp();
    expect(rollAttack).toHaveBeenCalledTimes(2);
    const [first, second] = rollAttack.mock.calls;
    expect(first[4]).toBe('t1');
    expect(second[4]).toBe('t2');
    expect(second[6]).not.toBe(first[6]);
  });

  it('never drops a roll silently: with no target left it says the options changed, and the buttons are off', async () => {
    TestBed.resetTestingModule();
    const rollAttack = vi.fn();
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: fakeCombatClient({ rollAttack }) }],
    });
    const fixture = TestBed.createComponent(NpcCard);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [combatant({ id: 'npc', label: 'Goblin' })] }),
    );
    fixture.componentRef.setInput('subject', combatant({ id: 'npc', label: 'Goblin' }));
    fixture.componentRef.setInput('state', { apply: vi.fn() } as unknown as CombatState);
    fixture.componentRef.setInput('options', opts([]));
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const card = fixture.componentInstance as unknown as {
      rollApp(): Promise<void>;
      canRoll(): boolean;
    };
    expect(card.canRoll()).toBe(false);
    await card.rollApp();
    fixture.detectChanges();
    expect(rollAttack).not.toHaveBeenCalled();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain(
      'O ataque ou o alvo mudou enquanto você confirmava.',
    );
  });

  it('does not roll a second attack while a hit of this attacker still has its damage open, and says why', async () => {
    TestBed.resetTestingModule();
    const rollAttack = vi.fn();
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: fakeCombatClient({ rollAttack }) }],
    });
    const fixture = TestBed.createComponent(NpcCard);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [combatant({ id: 'npc', label: 'Goblin' })] }),
    );
    fixture.componentRef.setInput('subject', combatant({ id: 'npc', label: 'Goblin' }));
    fixture.componentRef.setInput('state', { apply: vi.fn() } as unknown as CombatState);
    const open = { ...(opts(['t1']) as object), pendingDamages: [{ id: 'p1' }] } as never;
    fixture.componentRef.setInput('options', open);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const card = fixture.componentInstance as unknown as { rollApp(): Promise<void> };
    await card.rollApp();
    expect(rollAttack).not.toHaveBeenCalled();
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('#roll-why')?.textContent,
    ).toContain('dano do ataque anterior');
    // Once the damage is applied the options carry none, and the attack rolls again.
    rollAttack.mockRejectedValue(new Error('stop'));
    fixture.componentRef.setInput('options', opts(['t1']));
    fixture.detectChanges();
    await card.rollApp();
    expect(rollAttack).toHaveBeenCalledTimes(1);
    expect((fixture.nativeElement as HTMLElement).querySelector('#roll-why')).toBeNull();
  });

  it('clears the result card of the last roll when the master undoes an action (R4)', async () => {
    TestBed.resetTestingModule();
    const rollAttack = vi.fn(async () => ({
      roll: {
        attackerId: 'npc',
        targetId: 't1',
        d20: { total: 12, rolls: [8, 19], modifier: 4, count: 2, sides: 20, keep: 'highest' },
        outcome: 2,
        heldForReaction: false,
      },
      pending: undefined,
      encounter: encounter({ combatants: [combatant({ id: 'npc', label: 'Goblin' })] }),
    }));
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: fakeCombatClient({ rollAttack }) }],
    });
    const fixture = TestBed.createComponent(NpcCard);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [combatant({ id: 'npc', label: 'Goblin' })] }),
    );
    fixture.componentRef.setInput('subject', combatant({ id: 'npc', label: 'Goblin' }));
    fixture.componentRef.setInput('state', { apply: vi.fn() } as unknown as CombatState);
    fixture.componentRef.setInput('options', opts(['t1']));
    fixture.detectChanges();
    await fixture.whenStable();
    const card = fixture.componentInstance as unknown as {
      rollApp(): Promise<void>;
      last(): unknown;
    };
    await card.rollApp();
    expect(card.last()).not.toBeNull();

    TestBed.inject(CombatUndone).mark();
    fixture.detectChanges();
    await fixture.whenStable();
    expect(card.last()).toBeNull();
  });
});

describe("NpcCard: an NPC's area spell", () => {
  function setup(theatre: boolean) {
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({ providers: [{ provide: CombatClient, useValue: {} }] });
    const fixture = TestBed.createComponent(NpcCard);
    const zuk = combatant({ id: 'z', label: 'Zuk' });
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput('encounter', encounter({ combatants: [zuk] }));
    fixture.componentRef.setInput('subject', zuk);
    fixture.componentRef.setInput('state', {
      apply: vi.fn(),
      encounter: () => encounter({ combatants: [zuk] }),
    } as unknown as CombatState);
    fixture.componentRef.setInput('theatre', theatre);
    fixture.componentRef.setInput('options', {
      options: {
        attacks: [],
        spells: [
          {
            spell: { key: 'spell:fireball', namePt: 'Bola de Fogo', level: 3 },
            enabled: true,
            slots: [],
          },
          { spell: { key: 'spell:bless', namePt: 'Bênção', level: 1 }, enabled: true, slots: [] },
        ],
      },
      attackTargets: [],
      spellTargets: [
        { spellKey: 'spell:fireball', placement: AreaPlacement.POINT, targets: [] },
        { spellKey: 'spell:bless', placement: AreaPlacement.UNSPECIFIED, targets: [] },
      ],
      pendingDamages: [],
    } as never);
    const cast: string[] = [];
    fixture.componentInstance.castArea.subscribe((k) => cast.push(k));
    fixture.detectChanges();
    return { el: fixture.nativeElement as HTMLElement, cast };
  }

  it('offers "Conjurar" for a spell placed on the map only, and hands its key to the page', () => {
    const { el, cast } = setup(false);
    const buttons = Array.from(el.querySelectorAll<HTMLButtonElement>('.area-spell'));
    expect(buttons.map((b) => b.textContent?.replace(/\s+/g, ' ').trim())).toEqual([
      'auto_awesomeConjurar Bola de Fogo',
    ]);
    buttons[0].click();
    expect(cast).toEqual(['spell:fireball']);
  });

  it('offers none without a map', () => {
    expect(setup(true).el.querySelector('.area-spell')).toBeNull();
  });
});

describe('NpcCard: the typed d20 follows the roll mode of the target', () => {
  const opts = (rollMode: RollMode, sources: unknown[] = []) =>
    ({
      options: {
        attacks: [
          {
            attack: {
              key: 'a1',
              saveDc: 0,
              attackBonus: 4,
              namePt: 'Garras',
              damage: '2d4',
              damageTypePt: 'cortante',
              rangeFt: 5,
              longRangeFt: 0,
            },
          },
        ],
      },
      attackTargets: [
        { attackKey: 'a1', targets: [{ combatantId: 't1', label: 'Iolanda', rollMode, sources }] },
      ],
      pendingDamages: [],
    }) as never;

  async function render(rollMode: RollMode, sources: unknown[] = []) {
    TestBed.resetTestingModule();
    const rollAttack = vi.fn().mockRejectedValue(new Error('stop'));
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: fakeCombatClient({ rollAttack }) }],
    });
    const fixture = TestBed.createComponent(NpcCard);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [combatant({ id: 'npc', label: 'Carniçal' })] }),
    );
    fixture.componentRef.setInput('subject', combatant({ id: 'npc', label: 'Carniçal' }));
    fixture.componentRef.setInput('state', { apply: vi.fn() } as unknown as CombatState);
    fixture.componentRef.setInput('options', opts(rollMode, sources));
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    return { fixture, rollAttack, el: fixture.nativeElement as HTMLElement };
  }

  it('asks for two d20 against a target that gives advantage (a restrained one) and sends the pair', async () => {
    const { fixture, rollAttack, el } = await render(RollMode.ADVANTAGE);
    expect(el.querySelector('app-roll-picker')).toBeNull();
    expect(el.querySelector('app-multi-roll')).not.toBeNull();
    const card = fixture.componentInstance as unknown as {
      rollTypedPair(faces: number[]): Promise<void>;
    };
    await card.rollTypedPair([18, 4]);
    expect(rollAttack.mock.calls[0][5]).toEqual({ faces: [18, 4] });
  });

  it('asks for two d20 against a disadvantage too, counting the lower', async () => {
    const { fixture, el } = await render(RollMode.DISADVANTAGE);
    expect(el.querySelector('app-multi-roll')).not.toBeNull();
    const card = fixture.componentInstance as unknown as { pairHint(): string };
    expect(card.pairHint()).toContain('Conta o menor');
  });

  it('says why the roll has advantage, so the master knows which die counts', async () => {
    const { el } = await render(RollMode.ADVANTAGE, [
      {
        kind: AdvantageSourceKind.UNSEEN_ATTACKER,
        effect: RollMode.ADVANTAGE,
        textPt: 'atacante não visto',
      },
    ]);
    expect(el.textContent).toContain('Vantagem: atacante não visto');
  });

  it('says nothing about the mode when the roll is normal', async () => {
    const { el } = await render(RollMode.NORMAL);
    expect(el.textContent).not.toContain('Vantagem');
  });

  it('keeps one d20 when the roll is normal', async () => {
    const { el } = await render(RollMode.NORMAL);
    expect(el.querySelector('app-roll-picker')).not.toBeNull();
    expect(el.querySelector('app-multi-roll')).toBeNull();
  });
});

describe('NpcCard: Levantar-se of a prone NPC', () => {
  const card = (over: object) => {
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({ providers: [{ provide: CombatClient, useValue: {} }] });
    const fixture = TestBed.createComponent(NpcCard);
    const goblin = combatant({
      id: 'npc',
      label: 'Goblin 2',
      conditions: ['condition:prone'],
      standUpCostDft: 150,
      movementLeftDft: 300,
      ...over,
    });
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput('encounter', encounter({ combatants: [goblin] }));
    fixture.componentRef.setInput('subject', goblin);
    fixture.componentRef.setInput('state', { apply: vi.fn() } as unknown as CombatState);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  };

  it('offers it to a prone NPC with the movement', () => {
    expect(card({}).textContent).toContain('Levantar-se');
  });

  it('does not offer it to an incapacitated one (Riso Histérico keeps it prone)', () => {
    expect(
      card({ conditions: ['condition:prone', 'condition:incapacitated'] }).textContent,
    ).not.toContain('Levantar-se');
  });
});
