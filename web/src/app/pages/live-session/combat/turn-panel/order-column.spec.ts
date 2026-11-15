import { TestBed } from '@angular/core/testing';

import { CombatantKind, CombatantState } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { OrderColumn } from './order-column';

describe('OrderColumn', () => {
  it("numbers the order, marks the turn and says a word for each, never an NPC's numbers", () => {
    const fixture = TestBed.createComponent(OrderColumn);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        currentCombatantId: 'pen',
        combatants: [
          combatant({ id: 'brisa', label: 'Brisa', kind: CombatantKind.PLAYER }),
          combatant({ id: 'cap', label: 'Capitão Goblin', state: CombatantState.HURT }),
          combatant({ id: 'pen', label: 'Pensantus', kind: CombatantKind.PLAYER, mine: true }),
          combatant({
            id: 'g1',
            label: 'Goblin 1',
            defeated: true,
            state: CombatantState.DEFEATED,
          }),
        ],
      }),
    );
    fixture.detectChanges();
    const rows = Array.from((fixture.nativeElement as HTMLElement).querySelectorAll('.row'), (r) =>
      r.textContent?.replace(/\s+/g, ' ').trim(),
    );
    expect(rows).toHaveLength(4);
    expect(rows[0]).toMatch(/^1 .*Brisa Jogador$/);
    expect(rows[1]).toMatch(/Capitão Goblin Ferido$/);
    expect(rows[2]).toMatch(/Pensantus VezVocê$/);
    expect(rows[3]).toMatch(/^4.*Goblin 1 .*Derrotado$/);
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('.row--turn')?.textContent,
    ).toContain('Pensantus');
  });

  it("says the person's own hit points in numbers and everyone else's in words (RN-20)", () => {
    const fixture = TestBed.createComponent(OrderColumn);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        currentCombatantId: 'pen',
        combatants: [
          combatant({
            id: 'pen',
            label: 'Pensantus',
            kind: CombatantKind.PLAYER,
            mine: true,
            hitPointsCurrent: 23,
            hitPointsMax: 23,
          }),
          combatant({ id: 'cap', label: 'Capitão Goblin', state: CombatantState.HURT }),
        ],
      }),
    );
    fixture.detectChanges();
    const rows = Array.from((fixture.nativeElement as HTMLElement).querySelectorAll('.row'), (r) =>
      r.textContent?.replace(/\s+/g, ' ').trim(),
    );
    expect(rows[0]).toMatch(/Pensantus VezVocê · 23 de 23 PV$/);
    expect(rows[1]).toMatch(/Capitão Goblin Ferido$/);
    expect(rows[1]).not.toMatch(/PV|\d+ de \d+/);
  });

  it("takes the person's numbers from its vitals when the combatant carries none", () => {
    const fixture = TestBed.createComponent(OrderColumn);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        currentCombatantId: 'pen',
        combatants: [
          combatant({ id: 'pen', label: 'Pensantus', kind: CombatantKind.PLAYER, mine: true }),
        ],
      }),
    );
    fixture.componentRef.setInput('hitPoints', { current: 19, max: 23 });
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('.row')?.textContent).toContain(
      'Você · 19 de 23 PV',
    );
  });

  it('says a creature of another player with only its owner and a state word, and the concentration to everyone', () => {
    const fixture = TestBed.createComponent(OrderColumn);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        currentCombatantId: 'tor',
        combatants: [
          combatant({
            id: 'sal',
            label: 'Sálvia',
            kind: CombatantKind.PLAYER,
            characterId: 'sc',
            concentrationSpell: 'spell:conjure-animals',
            concentrationSpellNamePt: 'Conjurar Animais',
          }),
          combatant({
            id: 'w1',
            label: 'Lobo atroz 1',
            kind: CombatantKind.CREATURE,
            characterId: '',
            ownerCharacterId: 'sc',
            state: CombatantState.HURT,
          }),
          combatant({ id: 'tor', label: 'Toren', kind: CombatantKind.PLAYER, mine: true }),
        ],
      }),
    );
    fixture.detectChanges();
    const rows = Array.from((fixture.nativeElement as HTMLElement).querySelectorAll('.row'), (r) =>
      r.textContent?.replace(/\s+/g, ' ').trim(),
    );
    expect(rows[0]).toContain('Concentração');
    // No numbers of its hit points, armor class or actions: the owner and a word.
    expect(rows[1]).toMatch(/Lobo atroz 1 .*da Sálvia · Ferido/);
    expect(rows[1]).not.toMatch(/PV|CA|\d+ de \d+/);
  });

  it("says another player's druid is a wolf with the state word, never the beast's pool (RN-20)", () => {
    const fixture = TestBed.createComponent(OrderColumn);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        currentCombatantId: 'tor',
        combatants: [
          combatant({
            id: 'sal',
            label: 'Sálvia',
            kind: CombatantKind.PLAYER,
            wildShapeBeastKey: 'monster:wolf',
            wildShapeBeastNamePt: 'Lobo',
          }),
          combatant({ id: 'tor', label: 'Toren', kind: CombatantKind.PLAYER, mine: true }),
        ],
      }),
    );
    fixture.detectChanges();
    const rows = Array.from((fixture.nativeElement as HTMLElement).querySelectorAll('.row'), (r) =>
      r.textContent?.replace(/\s+/g, ' ').trim(),
    );
    expect(rows[0]).toContain('Na forma de Lobo');
    expect(rows[0]).toContain('Jogador');
    expect(rows[0]).not.toMatch(/\d+ de \d+/);
  });

  it('says "Você · 7 de 11 PV (Lobo)" for the druid in a form, once: no tag and no chip beside it (584-02)', () => {
    const fixture = TestBed.createComponent(OrderColumn);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        currentCombatantId: 'sal',
        combatants: [
          combatant({
            id: 'sal',
            label: 'Sálvia',
            kind: CombatantKind.PLAYER,
            mine: true,
            wildShapeBeastKey: 'monster:wolf',
            wildShapeBeastNamePt: 'Lobo',
            wildShapeHitPointsCurrent: 7,
            wildShapeHitPointsMax: 11,
          }),
          combatant({
            id: 'ana',
            label: 'Ana',
            kind: CombatantKind.PLAYER,
            wildShapeBeastKey: 'monster:wolf',
            wildShapeBeastNamePt: 'Lobo',
          }),
        ],
      }),
    );
    fixture.componentRef.setInput('hitPoints', { current: 7, max: 11 });
    fixture.detectChanges();
    const rows = Array.from((fixture.nativeElement as HTMLElement).querySelectorAll('.row'), (r) =>
      r.textContent?.replace(/\s+/g, ' ').trim(),
    );
    expect(rows[0]).toMatch(/Sálvia VezVocê · 7 de 11 PV \(Lobo\)$/);
    expect(rows[0]).not.toContain('Na forma de');
    // Another player's druid says the form by name, and no number (RN-20).
    expect(rows[1]).toContain('Na forma de Lobo');
  });

  it('boxes a rider with its mount as "Montado" and says whose the mount is', () => {
    const fixture = TestBed.createComponent(OrderColumn);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        currentCombatantId: 'tor',
        combatants: [
          combatant({
            id: 'tor',
            label: 'Tor',
            kind: CombatantKind.PLAYER,
            mine: true,
            initiative: 17,
            mountedOnId: 'h',
          }),
          combatant({
            id: 'h',
            label: 'Cavalo de guerra',
            riderId: 'tor',
            // The rider's player reads the mark: only a controlled mount shares the box.
            mountKind: 'controlled',
            state: CombatantState.UNHURT,
          }),
        ],
      }),
    );
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const box = el.querySelector('app-order-group');
    expect(box?.textContent?.replace(/\s+/g, ' ')).toContain('Montado· iniciativa 17');
    expect(box?.textContent).not.toContain('Turno conjunto');
    expect(box?.querySelector('.box')?.getAttribute('aria-label')).toBe(
      'Montado: Tor e Cavalo de guerra, iniciativa 17',
    );
    const mount = el.querySelector('.row--mount');
    expect(mount?.textContent).toContain('Montaria de Tor');
    expect(mount?.querySelector('.row__link')?.getAttribute('aria-hidden')).toBe('true');
    expect(getComputedStyle(mount!).paddingLeft).toBe('22px');
  });

  it('boxes an NPC pair without a number: a player has no total of theirs (RN-20)', () => {
    const fixture = TestBed.createComponent(OrderColumn);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        currentCombatantId: 'w',
        combatants: [
          combatant({ id: 'w', label: 'Wight', mountedOnId: 'e' }),
          combatant({ id: 'e', label: 'Esqueleto', riderId: 'w', mountKind: '' }),
        ],
      }),
    );
    fixture.detectChanges();
    const box = (fixture.nativeElement as HTMLElement).querySelector('app-order-group');
    expect(box?.querySelector('.head')?.textContent?.replace(/\s+/g, '')).toBe('linkMontado');
    expect(box?.querySelector('.box')?.getAttribute('aria-label')).toBe(
      'Montado: Wight e Esqueleto',
    );
  });
});
