import { TestBed } from '@angular/core/testing';

import { AreaPlacement, AttackOutcome } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { DisabledReasonCode } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import type { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { settle } from '../../../../../test-settle';
import { NpcCard } from './npc-card';

/** What the master's card says: the result of the roll, the stats, the area spells and why a roll is off. */
describe('NpcCard: the words of the card', () => {
  const attack = (over: Record<string, unknown> = {}) => ({
    key: 'a1',
    saveDc: 0,
    attackBonus: 4,
    namePt: 'Espada',
    damage: '1d6',
    damageTypePt: 'cortante',
    rangeFt: 5,
    longRangeFt: 0,
    ...over,
  });
  const options = (extra: Record<string, unknown> = {}, attackOver: Record<string, unknown> = {}) =>
    ({
      options: { attacks: [{ attack: attack(attackOver) }], spells: [] },
      attackTargets: [
        {
          attackKey: 'a1',
          targets: [{ combatantId: 'toren', label: 'Toren', distanceFt: 10, tooFar: true }],
        },
      ],
      pendingDamages: [],
      spellTargets: [],
      ...extra,
    }) as never;
  const d20 = { total: 17, diceCount: 1, diceSides: 20, faces: [13], modifier: 4 };

  function open(
    opts: {
      subject?: Record<string, unknown>;
      options?: unknown;
      theatre?: boolean;
      stopped?: boolean;
      roll?: Record<string, unknown>;
      people?: ReturnType<typeof combatant>[];
    } = {},
  ) {
    TestBed.resetTestingModule();
    const rollAttack = vi.fn(async () => ({
      roll: {
        attackerId: 'npc',
        targetId: 'toren',
        d20,
        outcome: AttackOutcome.HIT,
        heldForReaction: false,
        ...opts.roll,
      },
      pending: undefined,
      encounter: encounter({ combatants: [] }),
    }));
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: { rollAttack } }],
    });
    const subject = combatant({ id: 'npc', label: 'Goblin', ...opts.subject } as never);
    const fixture = TestBed.createComponent(NpcCard);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        combatants: [subject, ...(opts.people ?? [combatant({ id: 'toren', label: 'Toren' })])],
      }),
    );
    fixture.componentRef.setInput('subject', subject);
    fixture.componentRef.setInput('state', {
      apply: vi.fn(),
      encounter: () => null,
    } as unknown as CombatState);
    fixture.componentRef.setInput('options', opts.options ?? options());
    fixture.componentRef.setInput('theatre', opts.theatre ?? false);
    fixture.componentRef.setInput('reactionStopped', opts.stopped ?? false);
    fixture.detectChanges();
    return fixture;
  }
  const text = (f: ReturnType<typeof open>) =>
    ((f.nativeElement as HTMLElement).textContent ?? '').replace(/\s+/g, ' ').replace(/ /g, ' ');
  async function rolled(f: ReturnType<typeof open>) {
    await settle(f);
    await (f.componentInstance as unknown as { rollApp(): Promise<void> }).rollApp();
    f.detectChanges();
    return f.nativeElement as HTMLElement;
  }

  it('says "Acertou" against the armor class of a masculine target', async () => {
    const el = await rolled(open({ roll: { targetArmorClass: 15 } }));
    expect(el.querySelector('.pill')?.textContent).toContain('Acertou');
    expect(el.querySelector('.res__against')?.textContent).toBe('contra CA 15 do Toren');
    expect(el.querySelector('.res__formula')?.textContent).toBe('1d20 (13) + 4 = 17');
  });

  it('says "da" for a feminine target', async () => {
    const el = await rolled(
      open({
        roll: { targetArmorClass: 12 },
        people: [combatant({ id: 'toren', label: 'Brisa' })],
      }),
    );
    expect(el.querySelector('.res__against')?.textContent).toBe('contra CA 12 da Brisa');
  });

  it('tells a miss, with no armor class when the server sends none', async () => {
    const el = await rolled(open({ roll: { outcome: AttackOutcome.MISS } }));
    expect(el.querySelector('.pill--miss')?.textContent).toContain('Errou');
    expect(el.querySelector('.res__against')).toBeNull();
  });

  it('tells a critical hit', async () => {
    const el = await rolled(open({ roll: { outcome: AttackOutcome.CRITICAL_HIT } }));
    expect(el.querySelector('.pill')?.textContent).toContain('Crítico');
  });

  it('tells no outcome while a reaction holds the roll', async () => {
    const el = await rolled(open({ roll: { heldForReaction: true } }));
    expect(el.querySelector('.pill--held')?.textContent).toContain('Esperando a reação');
    expect(el.querySelector('.pill--miss')).toBeNull();
  });

  it('turns a hit into a miss named after the shield that held it', async () => {
    const f = open({ stopped: true });
    const el = await rolled(f);
    expect(el.querySelector('.pill--miss')?.textContent).toContain(
      'Errou: o Escudo Arcano segurou',
    );
  });

  it('reads a physical d20 and a roll with no dice as they are', async () => {
    const physical = await rolled(open({ roll: { d20: { ...d20, physical: true } } }));
    expect(physical.querySelector('.res__formula')?.textContent).toContain('· dado físico');
    const none = await rolled(open({ roll: { d20: undefined } }));
    expect(none.querySelector('.res__formula')?.textContent?.trim()).toBe('');
  });

  it('shows the hit points, armor class and speed of an NPC, with the squares on a map', () => {
    const f = open({
      subject: {
        hitPointsCurrent: 7,
        hitPointsMax: 11,
        armorClass: 13,
        speedDft: 90,
        speedFt: 30,
        movementLeftDft: 45,
        movementLeftFt: 15,
      },
    });
    expect(text(f)).toContain('Pontos de vida7 de 11CA13Deslocamento2,7 m · 6 quadrados');
    expect(text(f)).toContain('Movimento 1,4 m · 3 quadrados');
  });

  it('says what is left of the movement in a combat without a map', () => {
    const f = open({ theatre: true, subject: { speedDft: 90, movementLeftDft: 0 } });
    expect(text(f)).toContain('Movimento Sem movimento');
    const g = open({ theatre: true, subject: { speedDft: 90, movementLeftDft: 30 } });
    expect(text(g)).toContain('Movimento Restam 0,9 m');
  });

  it('lists the spells whose area is placed on the map, named in Portuguese when it is', () => {
    const f = open({
      options: options({
        spellTargets: [
          { spellKey: 'fireball', placement: AreaPlacement.POINT },
          { spellKey: 'sphere', placement: AreaPlacement.UNSPECIFIED },
          { spellKey: 'off', placement: AreaPlacement.POINT },
        ],
      }),
    });
    const withSpells = open({
      options: {
        ...(options({
          spellTargets: [
            { spellKey: 'fireball', placement: AreaPlacement.POINT },
            { spellKey: 'sphere', placement: AreaPlacement.UNSPECIFIED },
            { spellKey: 'off', placement: AreaPlacement.POINT },
            { spellKey: 'plain', placement: AreaPlacement.POINT },
          ],
        }) as object),
        options: {
          attacks: [{ attack: attack() }],
          spells: [
            { enabled: true, spell: { key: 'fireball', namePt: 'Bola de Fogo', name: 'Fireball' } },
            { enabled: true, spell: { key: 'sphere', namePt: 'Esfera', name: 'Sphere' } },
            { enabled: false, spell: { key: 'off', namePt: 'Desligada', name: 'Off' } },
            { enabled: true, spell: { key: 'plain', namePt: '', name: 'Plain' } },
          ],
        },
      },
    });
    expect(text(f)).not.toContain('Magias de área');
    const names = [
      ...(withSpells.nativeElement as HTMLElement).querySelectorAll('.area-spell'),
    ].map((b) => b.textContent?.replace(/\s+/g, ' ').trim());
    expect(names).toEqual(['auto_awesomeConjurar Bola de Fogo', 'auto_awesomeConjurar Plain']);
  });

  it('turns the roll off for an NPC that cannot act, with the reason written', async () => {
    const f = open({
      options: {
        ...(options() as object),
        options: {
          attacks: [
            {
              enabled: false,
              reason: { code: DisabledReasonCode.INCAPACITATED },
              attack: attack(),
            },
          ],
          spells: [],
        },
      },
    });
    await settle(f);
    f.detectChanges();
    expect(
      (f.nativeElement as HTMLElement).querySelector('#roll-why')?.textContent?.trim(),
    ).not.toBe('');
  });

  it('writes a negative bonus with the minus sign in the label of the typed roll', async () => {
    const f = open({ options: options({}, { attackBonus: -1 }) });
    await settle(f);
    f.detectChanges();
    const picker = (f.nativeElement as HTMLElement).querySelector('app-roll-picker');
    expect(picker?.textContent).toContain('Digitar');
    expect((f.componentInstance as unknown as { rollLabel(): string }).rollLabel()).toBe(
      'Role 1d20 para Espada (−1)',
    );
  });

  it('says how far a target is and when it is out of reach', () => {
    const f = open();
    const card = f.componentInstance as unknown as {
      distance(ft: number | undefined, tooFar: boolean): string;
      coverNote(t: { cover: number; coverSource: number }): string;
    };
    expect(card.distance(undefined, false)).toBe('');
    expect(card.distance(10, false).replace(/\u00a0/g, ' ')).toBe('a 3,0 m');
    expect(card.distance(undefined, true)).toBe('fora do alcance');
    expect(card.distance(10, true).replace(/\u00a0/g, ' ')).toBe('a 3,0 m, fora do alcance');
  });
});
