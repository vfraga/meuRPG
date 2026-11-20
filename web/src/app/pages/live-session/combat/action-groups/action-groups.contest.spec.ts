import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import { CombatantSchema } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ContestAttackOptionKind,
  ContestAttackOptionSchema,
  ContestTurnStateSchema,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import {
  AttackKind,
  AttackOptionSchema,
  AttackSchema,
  DisabledReasonCode,
  TurnOptionsSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { textOf } from '../../../../core/combat/contest-testing';
import { ActionGroups } from './action-groups';

const sword = create(AttackOptionSchema, {
  enabled: true,
  attack: create(AttackSchema, {
    key: 'attack:longsword',
    name: 'Longsword',
    namePt: 'Espada longa',
    kind: AttackKind.WEAPON,
    attackBonus: 5,
    damage: '1d8 + 3',
    damageTypePt: 'cortante',
  }),
});

const special = (kind: ContestAttackOptionKind, over: object = {}) =>
  create(ContestAttackOptionSchema, { kind, replacesAttack: true, enabled: true, ...over });

function setup(inputs: {
  contestAttacks?: ReturnType<typeof special>[];
  contest?: ReturnType<typeof create<typeof ContestTurnStateSchema>>;
  label?: string;
  movementLeftFt?: number;
  conditions?: string[];
  standUpCostDft?: number;
  theatre?: boolean;
}) {
  const fixture = TestBed.createComponent(ActionGroups);
  const ref = fixture.componentRef;
  ref.setInput('options', create(TurnOptionsSchema, { attacks: [sword] }));
  ref.setInput(
    'own',
    create(CombatantSchema, {
      label: inputs.label ?? 'Toren',
      movementLeftFt: inputs.movementLeftFt ?? 30,
      movementLeftDft: (inputs.movementLeftFt ?? 30) * 10,
      speedFt: 30,
      conditions: inputs.conditions ?? [],
      standUpCostDft: inputs.standUpCostDft ?? 0,
    }),
  );
  ref.setInput('theatre', inputs.theatre ?? false);
  ref.setInput('contestAttacks', inputs.contestAttacks ?? []);
  ref.setInput('contest', inputs.contest);
  const kinds: ContestAttackOptionKind[] = [];
  let escapes = 0;
  let stands = 0;
  fixture.componentInstance.standUp.subscribe(() => stands++);
  fixture.componentInstance.contestAttack.subscribe((k) => kinds.push(k));
  fixture.componentInstance.escape.subscribe(() => escapes++);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const row = (name: string) =>
    Array.from(el.querySelectorAll<HTMLElement>('app-action-row')).find(
      (r) => r.querySelector('.row__name')?.textContent?.trim() === name,
    );
  return { fixture, el, kinds, escapes: () => escapes, stands: () => stands, row };
}

describe('ActionGroups: the special attacks, escape and the states of the turn (W7-X)', () => {
  describe('Agarrar and Empurrar (board W7-Xa 2)', () => {
    const both = [special(ContestAttackOptionKind.GRAPPLE), special(ContestAttackOptionKind.SHOVE)];

    it('lists them under the attacks, each saying it replaces an attack, with the note of the board', () => {
      const { el, row } = setup({ contestAttacks: both });
      expect(row('Espada longa')).toBeTruthy();
      expect(textOf(row('Agarrar')!)).toBe('Agarrar Ação Substitui um ataque · Atletismo Agarrar');
      expect(textOf(row('Empurrar')!)).toBe(
        'Empurrar Ação Substitui um ataque · Atletismo Empurrar',
      );
      expect(textOf(el)).toContain(
        'Agarrar e Empurrar usam o seu ataque. Com mais de um ataque, você escolhe qual troca.',
      );
      // They come after the weapon's line.
      const names = Array.from(el.querySelectorAll('.row__name')).map((n) => n.textContent?.trim());
      expect(names.indexOf('Espada longa')).toBeLessThan(names.indexOf('Agarrar'));
    });

    it('tells the page which one was chosen, with a button named for what it does', () => {
      const { row, kinds } = setup({ contestAttacks: both });
      const grapple = row('Agarrar')!.querySelector<HTMLButtonElement>('button')!;
      expect(grapple.getAttribute('aria-label')).toBe('Agarrar: escolher o alvo');
      grapple.click();
      row('Empurrar')!.querySelector<HTMLButtonElement>('button')!.click();
      expect(kinds).toEqual([ContestAttackOptionKind.GRAPPLE, ContestAttackOptionKind.SHOVE]);
    });

    it('keeps one that is off in its place, with the reason, and does nothing when pressed', () => {
      const { row, kinds } = setup({
        contestAttacks: [
          special(ContestAttackOptionKind.GRAPPLE, {
            enabled: false,
            reason: { code: DisabledReasonCode.ATTACKS_USED },
          }),
        ],
      });
      const r = row('Agarrar')!;
      expect(textOf(r)).toContain('Ataques desta ação já usados');
      r.querySelector<HTMLButtonElement>('button')!.click();
      expect(kinds).toEqual([]);
    });

    it('has no such lines, and no note, when the combatant cannot grapple or shove', () => {
      const { el, row } = setup({});
      expect(row('Agarrar')).toBeUndefined();
      expect(textOf(el)).not.toContain('Agarrar e Empurrar usam o seu ataque');
    });
  });

  describe('a grappled combatant (board W7-Xb 5)', () => {
    const grappled = (over: object = {}) =>
      create(ContestTurnStateSchema, { grappled: true, grapplerId: 'h', canEscape: true, ...over });

    it('puts "Escapar" first in the Ação, saying it is against the one that holds', () => {
      const { el, row, escapes } = setup({ contest: grappled(), label: 'Brisa' });
      const names = Array.from(el.querySelectorAll('.row__name')).map((n) => n.textContent?.trim());
      expect(names[0]).toBe('Escapar');
      expect(textOf(row('Escapar')!)).toContain(
        'Ação · Atletismo ou Acrobacia, contra quem agarra',
      );
      row('Escapar')!.querySelector<HTMLButtonElement>('button')!.click();
      expect(escapes()).toBe(1);
    });

    it('turns "Escapar" off with the reason when the action is spent', () => {
      const { row, escapes } = setup({
        contest: grappled({
          canEscape: false,
          escapeReason: { code: DisabledReasonCode.ACTION_USED },
        }),
      });
      expect(textOf(row('Escapar')!)).toContain('Ação já usada');
      row('Escapar')!.querySelector<HTMLButtonElement>('button')!.click();
      expect(escapes()).toBe(0);
    });

    it('has no "Mover": a grappled character\u2019s movement says so in the row under the map', () => {
      const { el } = setup({ contest: grappled(), label: 'Brisa' });
      expect(el.querySelector('button[aria-label="Mover"]')).toBeNull();
    });

    it('has no "Escapar" for a combatant that is not grappled', () => {
      const { row } = setup({ contest: create(ContestTurnStateSchema, {}) });
      expect(row('Escapar')).toBeUndefined();
    });
  });

  describe('a hidden combatant (board W7-Xc 8b)', () => {
    it('says that attacking from hiding gives advantage and ends the hiding for everyone', () => {
      const { el } = setup({
        contest: create(ContestTurnStateSchema, { hidden: true }),
        label: 'Brisa',
      });
      expect(textOf(el.querySelector('[data-testid="hidden-note"]')!)).toBe(
        'Atacar de onde você está escondido dá vantagem ao ataque. Depois do ataque, acertando ou errando, você deixa de estar escondido para todos.',
      );
    });

    it('says nothing when not hidden', () => {
      const { el } = setup({ contest: create(ContestTurnStateSchema, {}) });
      expect(el.querySelector('[data-testid="hidden-note"]')).toBeNull();
    });
  });

  describe('a surprised combatant (board W7-Xc 11)', () => {
    it('shows the four groups unavailable with the reason, and no action to take', () => {
      const { el } = setup({ contest: create(ContestTurnStateSchema, { surprised: true }) });
      const groups = Array.from(el.querySelectorAll('section.group')).map((g) => textOf(g));
      expect(groups).toEqual([
        'Atacar Indisponível Surpresa.',
        'Movimento Indisponível Surpresa.',
        'Ação bônus Indisponível Surpresa.',
        'Reação Indisponível Surpresa até o fim do turno.',
      ]);
      expect(el.querySelectorAll('button')).toHaveLength(0);
    });

    it('is the normal turn when not surprised', () => {
      const { el } = setup({ contest: create(ContestTurnStateSchema, { surprised: false }) });
      expect(textOf(el)).not.toContain('Surpresa.');
      expect(textOf(el)).toContain('Espada longa');
    });
  });
});

describe('ActionGroups: Levantar-se in the Movimento group (SRD 5.1, Being Prone)', () => {
  it.each([false, true])('offers it to a prone combatant with its cost (theatre %s)', (theatre) => {
    const { row, stands } = setup({
      conditions: ['condition:prone'],
      standUpCostDft: 150,
      theatre,
    });
    const r = row('Levantar-se')!;
    expect(textOf(r)).toContain('Levantar-se gasta 4,5 m de movimento');
    r.querySelector<HTMLButtonElement>('button')!.click();
    expect(stands()).toBe(1);
  });

  it('is not there for a combatant that stands', () => {
    expect(setup({}).row('Levantar-se')).toBeUndefined();
  });

  it('is off, with the missing movement as the reason, when the movement left is short', () => {
    const { row, stands } = setup({
      conditions: ['condition:prone'],
      standUpCostDft: 150,
      movementLeftFt: 10,
    });
    const r = row('Levantar-se')!;
    expect(textOf(r)).toContain('Faltam 1,5 m de movimento.');
    r.querySelector<HTMLButtonElement>('button')!.click();
    expect(stands()).toBe(0);
  });

  it('is off with no speed (a cost of 0)', () => {
    const { row } = setup({ conditions: ['condition:prone'], standUpCostDft: 0 });
    expect(textOf(row('Levantar-se')!)).toContain('Sem velocidade, não dá para se levantar.');
  });
});
