import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import { CombatantKind, CreatureAttack } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  AttackKind,
  AttackSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant } from '../../../../core/combat/combat-testing';
import { CreaturesClient } from '../../../../core/creatures/creatures-client';
import { FakeCreaturesClient, flat, raven } from '../../../../core/creatures/creatures-testing';
import { CreatureBlock } from './creature-block';

function options(attacks: string[], standard: string[] = []) {
  return {
    options: {
      attacks: attacks.map((key) => ({
        enabled: true,
        attack: create(AttackSchema, {
          key,
          name: 'Bite',
          namePt: 'Mordida',
          notes: 'Melee Weapon Attack: +5 to hit. Hit: 7 (2d6 + 3) piercing damage.',
          attackBonus: 5,
          damage: '2d6+3',
          damageTypePt: 'perfurante',
          kind: AttackKind.WEAPON,
          rangeFt: 5,
          melee: true,
        }),
      })),
      standardActions: standard.map((k) => ({
        enabled: true,
        action: { key: `standard:${k}`, namePt: k, economy: ActionEconomy.ACTION },
      })),
    },
  } as never;
}

describe('CreatureBlock (E9-12 states 3 and 6)', () => {
  function setup(
    over: Record<string, unknown>,
    opts: unknown,
    input: Record<string, unknown> = {},
  ) {
    const api = new FakeCreaturesClient();
    api.blocks.set('monster:dire-wolf', raven({ armorClass: 14 }));
    api.blocks.set('monster:raven', raven());
    TestBed.configureTestingModule({ providers: [{ provide: CreaturesClient, useValue: api }] });
    const fixture = TestBed.createComponent(CreatureBlock);
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput(
      'creature',
      combatant({
        id: 'w1',
        label: 'Lobo atroz 1',
        kind: CombatantKind.CREATURE,
        monsterKey: 'monster:dire-wolf',
        hitPointsCurrent: 37,
        hitPointsMax: 37,
        speedDft: 500,
        movementLeftDft: 500,
        speedFt: 15,
        movementLeftFt: 15,
        creatureAttack: CreatureAttack.FULL,
        ...over,
      }),
    );
    fixture.componentRef.setInput('options', opts);
    fixture.componentRef.setInput('acting', true);
    for (const [k, v] of Object.entries(input)) {
      fixture.componentRef.setInput(k, v);
    }
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }
  const settle = async (f: { whenStable(): Promise<unknown>; detectChanges(): void }) => {
    await f.whenStable();
    f.detectChanges();
  };

  it('shows the creature with its numbers, an attack with "Atacar" and "Mover o Lobo atroz 1"', async () => {
    const { fixture, el } = setup({}, options(['monster:dire-wolf#bite']), { joint: true });
    await settle(fixture);
    expect(flat(el.querySelector('.head'))).toContain('Lobo atroz 1 PV 37 de 37 · CA 14');
    expect(flat(el.querySelector('.head'))).toContain('Ainda age');
    expect(flat(el.querySelector('.eco'))).toContain('Ação Disponível');
    expect(flat(el.querySelector('.eco'))).toContain('Movimento 15,0 m de 15,0 m');
    expect(flat(el.querySelector('.atk'))).toContain(
      'Mordida +5 para acertar · 2d6 + 3 perfurante · corpo a corpo, 1,5 m',
    );
    expect(flat(el.querySelector('.atk button'))).toBe('Atacar');
    expect(el.querySelector('.go')?.getAttribute('aria-label')).toBe('Mover o Lobo atroz 1');
  });

  it('asks the attack and the move of its own creature', () => {
    const { fixture, el } = setup({}, options(['monster:dire-wolf#bite']));
    const attacked: string[] = [];
    let moved = 0;
    fixture.componentInstance.attack.subscribe((k) => attacked.push(k));
    fixture.componentInstance.move.subscribe(() => moved++);
    el.querySelector<HTMLButtonElement>('.atk button')!.click();
    el.querySelector<HTMLButtonElement>('.go')!.click();
    expect(attacked).toEqual(['monster:dire-wolf#bite']);
    expect(moved).toBe(1);
  });

  it('a familiar does not attack: the line says so and the standard actions are all it has', () => {
    const { el } = setup(
      {
        label: 'Nanquim',
        monsterKey: 'monster:raven',
        creatureAttack: CreatureAttack.NONE,
        speedFlyFt: 50,
        hitPointsCurrent: 1,
        hitPointsMax: 1,
      },
      options(['x'], ['dash', 'help']),
    );
    expect(flat(el.querySelector('.note'))).toBe('O familiar não ataca. Ele usa as outras ações:');
    expect(el.querySelector('.atk')).toBeNull();
    expect(Array.from(el.querySelectorAll('.std__btn'), (b) => b.textContent?.trim())).toEqual([
      'dash',
      'help',
    ]);
    expect(flat(el.querySelector('.eco'))).toContain('voo');
  });

  it('the familiar of the Pacto da Corrente attacks with its reaction', () => {
    const { el } = setup(
      { creatureAttack: CreatureAttack.REACTION },
      options(['monster:imp#sting']),
    );
    expect(flat(el.querySelector('.atk'))).toContain('Reação');
    expect(el.textContent).not.toContain('O familiar não ataca');
  });

  it('off its turn every button is off, with the reason in a line', () => {
    const { el } = setup({}, options(['monster:dire-wolf#bite']), { acting: false });
    expect(flat(el.querySelector('.why'))).toBe('Ainda não é a vez do Lobo atroz 1.');
    expect(el.querySelector('.atk button')?.getAttribute('aria-disabled')).toBe('true');
    expect(el.querySelector('.go')?.getAttribute('aria-disabled')).toBe('true');
  });

  it('says the SRD text of an attack under it, in English, and the Portuguese name the server sent', () => {
    const { el } = setup({}, options(['monster:dire-wolf#bite']));
    const rider = el.querySelector('.atk .row__rider');
    expect(rider?.getAttribute('lang')).toBe('en');
    expect(flat(rider)).toBe('Melee Weapon Attack: +5 to hit. Hit: 7 (2d6 + 3) piercing damage.');
    expect(flat(el.querySelector('.atk .row__name'))).toBe('Mordida');
  });

  it('a hit whose damage was not rolled is said, with "Rolar o dano", and the page rolls it', () => {
    const { fixture, el } = setup({}, options(['monster:dire-wolf#bite']), {
      pending: { id: 'p1', attackKey: 'monster:dire-wolf#bite', attackerId: 'w1' },
    });
    expect(flat(el.querySelector('.owed'))).toContain(
      'Falta rolar o dano do ataque do Lobo atroz 1.',
    );
    let rolled = 0;
    fixture.componentInstance.rollDamage.subscribe(() => rolled++);
    Array.from(el.querySelectorAll<HTMLButtonElement>('.owed button'))
      .find((b) => flat(b) === 'Rolar o dano')!
      .click();
    expect(rolled).toBe(1);
  });

  it('has its live region on the page when no damage is owed', () => {
    const { el } = setup({}, options(['monster:dire-wolf#bite']));
    const region = el.querySelector('.mr-live[role="status"]');
    expect(region).not.toBeNull();
    expect(region?.querySelector('.owed')).toBeNull();
  });

  it('says the owed damage inside that live region, and the notice has no role of its own', () => {
    const { el } = setup({}, options(['monster:dire-wolf#bite']), {
      pending: { id: 'p1', attackKey: 'monster:dire-wolf#bite', attackerId: 'w1' },
    });
    const notice = el.querySelector('.owed');
    expect(notice?.closest('.mr-live[role="status"]')).not.toBeNull();
    expect(notice?.hasAttribute('role')).toBe(false);
  });

  it('the reaction row of the chain familiar follows what the server says: off when it says so', () => {
    const base = options(['monster:imp#sting']) as { options: { attacks: { enabled: boolean }[] } };
    base.options.attacks[0].enabled = false;
    const { el } = setup({ creatureAttack: CreatureAttack.REACTION }, base);
    expect(el.querySelector('.atk button')?.getAttribute('aria-disabled')).toBe('true');
  });

  it('the reaction row is off once the reaction is used, with the reason in words', () => {
    const { el } = setup(
      { creatureAttack: CreatureAttack.REACTION, reactionUsed: true },
      options(['monster:imp#sting']),
    );
    expect(el.querySelector('.atk button')?.getAttribute('aria-disabled')).toBe('true');
    expect(flat(el.querySelector('.atk .row__why'))).toContain('Reação já usada');
  });

  it('says what a trap did to this creature on its own part, and that the master applies the damage', () => {
    const { el } = setup({}, options(['monster:dire-wolf#bite']), {
      trapNote: {
        title: 'O Lobo atroz 1 caiu na armadilha Fosso escondido.',
        detail: '7 de concussão.',
        waiting: true,
      },
    });
    expect(flat(el.querySelector('.trapped'))).toContain(
      'O Lobo atroz 1 caiu na armadilha Fosso escondido. 7 de concussão.',
    );
    expect(flat(el.querySelector('.trapped__wait'))).toContain('Esperando o mestre aplicar o dano');
  });

  it('has no trap line without a trap', () => {
    const { el } = setup({}, options(['monster:dire-wolf#bite']));
    expect(el.querySelector('.trapped')).toBeNull();
  });

  it('another player never gets numbers: only the state word', () => {
    const { el } = setup({ hitPointsCurrent: undefined, hitPointsMax: undefined }, null);
    expect(flat(el.querySelector('.head__sub'))).not.toMatch(/PV|\d+ de \d+/);
  });
});
