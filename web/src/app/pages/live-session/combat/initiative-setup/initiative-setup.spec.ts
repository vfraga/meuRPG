import { TestBed } from '@angular/core/testing';

import { CombatantKind, EncounterStatus } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { InitiativeSetup } from './initiative-setup';

describe('InitiativeSetup', () => {
  const rolled = (id: string, label: string, total: number, extra = {}) =>
    combatant({
      id,
      label,
      initiative: total,
      initiativeFace: total - 2,
      initiativeBonus: 2,
      ...extra,
    });
  const list = [
    rolled('brisa', 'Brisa', 19, { kind: CombatantKind.PLAYER }),
    rolled('g1', 'Goblin 1', 12, { tieUnresolved: true }),
    rolled('g2', 'Goblin 2', 12, { tieUnresolved: true }),
    combatant({ id: 'toren', label: 'Toren', kind: CombatantKind.PLAYER, initiativeBonus: 2 }),
  ];

  function setup() {
    const fixture = TestBed.createComponent(InitiativeSetup);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ status: EncounterStatus.SETUP, round: 0, combatants: list }),
    );
    const orders: string[][] = [];
    const submits: { id: string; face: number }[] = [];
    fixture.componentInstance.order.subscribe((o) => orders.push(o));
    fixture.componentInstance.submit.subscribe((s) => submits.push(s));
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, orders, submits };
  }

  it('shows the formulas and the tie once, over its two rows', () => {
    const { el } = setup();
    expect(el.textContent).toContain('1d20 (17) + 2 = 19');
    expect(el.textContent?.match(/Empate em 12/g)).toHaveLength(1);
    expect(el.textContent).toContain(
      'Empate em 12: Goblin 1 e Goblin 2. Escolha a ordem com as setas.',
    );
  });

  it('gives each name its full text as the tooltip, since a narrow list ends it in an ellipsis', () => {
    const { el } = setup();
    const names = [...el.querySelectorAll<HTMLElement>('.row__name')];
    expect(names.map((n) => n.getAttribute('title'))).toEqual([
      'Brisa',
      'Goblin 1',
      'Goblin 2',
      'Toren',
    ]);
  });

  it('moves a tied combatant down with the arrow, and the outer arrows do nothing', () => {
    const { el, orders } = setup();
    const button = (name: string) => el.querySelector<HTMLButtonElement>(`[aria-label="${name}"]`)!;
    button('Subir Goblin 1 na ordem').click();
    expect(orders).toEqual([]);
    expect(button('Subir Goblin 1 na ordem').getAttribute('aria-disabled')).toBe('true');
    button('Descer Goblin 1 na ordem').click();
    expect(orders).toEqual([['g2', 'g1']]);
  });

  it("shows each monster's armor class and hit points before the first turn, and a player's none", () => {
    const fixture = TestBed.createComponent(InitiativeSetup);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        status: EncounterStatus.SETUP,
        round: 0,
        combatants: [
          rolled('g1', 'Goblin 1', 12, { armorClass: 15, hitPointsCurrent: 7, hitPointsMax: 7 }),
          rolled('brisa', 'Brisa', 19, { kind: CombatantKind.PLAYER }),
        ],
      }),
    );
    fixture.detectChanges();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('CA 15 · PV 7 de 7');
    expect(text.match(/CA \d+/g)).toHaveLength(1);
  });

  it('says who the master waits for and offers to type the roll', () => {
    const { el } = setup();
    expect(el.textContent).toContain('Esperando Toren');
    expect(el.textContent).toContain('1d20 + 2 = —');
    expect(el.textContent).toContain('Digitar pelo jogador');
  });

  it('types a face for the player: the total updates and only 1 to 20 saves', async () => {
    const { fixture, el, submits } = setup();
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.includes('Digitar pelo jogador'))!
      .click();
    fixture.detectChanges();
    const input = el.querySelector<HTMLInputElement>('input')!;
    input.value = '99';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(el.textContent).toContain('Digite um número de 1 a 20.');
    input.value = '7';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(el.textContent).toContain('= 9');
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.trim() === 'Salvar')!
      .click();
    expect(submits).toEqual([{ id: 'toren', face: 7 }]);
  });

  describe('the master typing a roll for a player', () => {
    const waiting = [
      combatant({ id: 'brisa', label: 'Brisa', kind: CombatantKind.PLAYER, initiativeBonus: 2 }),
    ];
    const answered = [
      combatant({
        id: 'brisa',
        label: 'Brisa',
        kind: CombatantKind.PLAYER,
        initiativeBonus: 2,
        initiative: 17,
        initiativeFace: 15,
      }),
    ];

    function open() {
      const fixture = TestBed.createComponent(InitiativeSetup);
      fixture.componentRef.setInput(
        'encounter',
        encounter({ status: EncounterStatus.SETUP, round: 0, combatants: waiting }),
      );
      const submits: { id: string; face: number }[] = [];
      fixture.componentInstance.submit.subscribe((s) => submits.push(s));
      fixture.detectChanges();
      const el = fixture.nativeElement as HTMLElement;
      Array.from(el.querySelectorAll('button'))
        .find((b) => b.textContent?.includes('Digitar pelo jogador'))!
        .click();
      fixture.detectChanges();
      return { fixture, el, submits };
    }

    const type = (el: HTMLElement, value: string) => {
      const field = el.querySelector<HTMLInputElement>('input')!;
      field.value = value;
      field.dispatchEvent(new Event('input'));
    };

    it('closes the field when the player rolls meanwhile, showing their roll', () => {
      const { fixture, el } = open();
      type(el, '3');
      fixture.componentRef.setInput(
        'encounter',
        encounter({ status: EncounterStatus.SETUP, round: 0, combatants: answered }),
      );
      fixture.detectChanges();
      expect(el.textContent).toContain('1d20 (15)');
      expect(el.querySelector('input')).toBeNull();
    });

    it('ignores Enter while a call is in flight and keeps the field open', () => {
      const { fixture, el, submits } = open();
      fixture.componentRef.setInput('busy', true);
      fixture.detectChanges();
      type(el, '7');
      fixture.detectChanges();
      el.querySelector<HTMLInputElement>('input')!.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Enter' }),
      );
      fixture.detectChanges();
      expect(submits).toEqual([]);
      expect(el.querySelector('input')).not.toBeNull();
    });
  });

  it('keeps the total in the formula line and tells the raised d20 in a note under it', () => {
    const fixture = TestBed.createComponent(InitiativeSetup);
    const name = 'Pé de Vento Muito Mais Que Longo';
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        status: EncounterStatus.SETUP,
        round: 0,
        combatants: [
          rolled('a', 'Ana', 15, {
            initiativeFace: 3,
            initiativeBonus: 3,
            initiativeTreatedAs: 12,
            initiativeTreatedAsSourceName: name,
          }),
          rolled('b', 'Beto', 19, { initiativeFace: 15, initiativeBonus: 4 }),
        ],
      }),
    );
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const sums = Array.from(el.querySelectorAll('.row__sum'), (m) => m.textContent?.trim());
    expect(sums).toEqual(['1d20 (12) + 3 = 15', '1d20 (15) + 4 = 19']);
    const notes = el.querySelectorAll('[data-testid="raised-note"]');
    expect(Array.from(notes, (m) => m.textContent?.trim())).toEqual([`d20: 3 → 12 (${name})`]);
  });

  it('tags a natural 20 and a natural 1 beside the formula, and nothing for another face', () => {
    const fixture = TestBed.createComponent(InitiativeSetup);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        status: EncounterStatus.SETUP,
        round: 0,
        combatants: [
          rolled('a', 'Ana', 22, { initiativeFace: 20 }),
          rolled('b', 'Beto', 3, { initiativeFace: 1 }),
          rolled('c', 'Caio', 12, { initiativeFace: 10 }),
          combatant({ id: 'd', label: 'Dani', initiativeBonus: 2 }),
        ],
      }),
    );
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(
      Array.from(el.querySelectorAll('[data-testid="natural-mark"]'), (m) => m.textContent?.trim()),
    ).toEqual(['20 natural', '1 natural']);
  });

  it('offers "Rolar por Toren" beside the player who has not rolled, when the app may roll, and asks the page to roll', () => {
    const { fixture, el } = setup();
    const rolls: string[] = [];
    fixture.componentInstance.rollFor.subscribe((id) => rolls.push(id));
    const find = () =>
      [...el.querySelectorAll('button')].find((b) => b.textContent?.includes('Rolar por Toren'));
    expect(find()).toBeUndefined();
    fixture.componentRef.setInput('canRoll', true);
    fixture.detectChanges();
    expect(
      [...el.querySelectorAll('button')].some((b) => b.textContent?.includes('Rolar por Brisa')),
    ).toBe(false);
    find()!.click();
    expect(rolls).toEqual(['toren']);
    expect(el.textContent).toContain('Digitar pelo jogador');
  });
});
