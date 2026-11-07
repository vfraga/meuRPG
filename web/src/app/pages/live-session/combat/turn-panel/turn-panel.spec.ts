import { TestBed } from '@angular/core/testing';

import { textOf } from '../../../../core/format/text-testing';

import { CombatantKind, CombatantState } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { TurnPanel } from './turn-panel';

describe('TurnPanel', () => {
  const order = [
    combatant({ id: 'brisa', label: 'Brisa', kind: CombatantKind.PLAYER }),
    combatant({ id: 'cap', label: 'Capitão Goblin', state: CombatantState.HURT }),
    combatant({
      id: 'pen',
      label: 'Pensantus',
      kind: CombatantKind.PLAYER,
      mine: true,
      speedFt: 25,
      movementLeftFt: 25,
      speedDft: 250,
      movementLeftDft: 250,
    }),
    combatant({ id: 'g1', label: 'Goblin 1', defeated: true, state: CombatantState.DEFEATED }),
  ];

  function text(over: Parameters<typeof encounter>[0]): HTMLElement {
    const fixture = TestBed.createComponent(TurnPanel);
    fixture.componentRef.setInput('encounter', encounter({ combatants: order, ...over }));
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('says a spent reaction stays spent until the start of the next turn', () => {
    const spent = order.map((c) => (c.id === 'pen' ? { ...c, reactionUsed: true } : c));
    const el = text({ currentCombatantId: 'cap', combatants: spent });
    expect(el.textContent).toContain('Sua reação: Usada até o começo do seu próximo turno.');
  });

  it('shows the reaction of a living player, and none for a dead one', () => {
    const living = text({ currentCombatantId: 'cap' });
    expect(living.textContent).toContain('Sua reação: Disponível.');
    const dead = [
      ...order.filter((c) => c.id !== 'pen'),
      combatant({
        id: 'pen',
        label: 'Pensantus',
        kind: CombatantKind.PLAYER,
        mine: true,
        state: CombatantState.DEAD,
        defeated: true,
      }),
    ];
    const el = text({ currentCombatantId: 'cap', combatants: dead });
    expect(el.textContent).not.toContain('Sua reação');
    expect(el.textContent).not.toContain('Ataque de oportunidade');
  });

  it('names another one on turn and says the player is next', () => {
    const el = text({ currentCombatantId: 'cap' });
    expect(el.querySelector('h2')?.textContent).toBe('Vez do Capitão Goblin');
    expect(el.textContent).toContain('Você é o próximo: depois dele, Pensantus.');
    expect(el.textContent).not.toContain('Mover');
  });

  it('shows state words for NPCs and never numbers', () => {
    const el = text({ currentCombatantId: 'cap' });
    const chips = Array.from(el.querySelectorAll('.chip'), (c) =>
      c.textContent?.replace(/\s+/g, ' ').trim(),
    );
    expect(chips.join('|')).toContain('Capitão GoblinFerido');
    expect(chips.join('|')).toContain('Goblin 1Derrotado');
    expect(el.textContent).not.toMatch(/PV|\d+ de \d+/);
  });

  it('says "Vez do mestre" when the turn is hidden, with no one highlighted', () => {
    const el = text({ currentCombatantId: '', masterTurn: true });
    expect(el.querySelector('h2')?.textContent).toBe('Vez do mestre');
    expect(el.querySelector('.chip--turn')).toBeNull();
    expect(el.textContent).not.toContain('Você é o próximo');
  });

  it("is the card of the player's own turn: whose it is, who is next, the shapes of what is left, no movement tile", () => {
    const el = text({ currentCombatantId: 'pen' });
    expect(el.querySelector('h2')?.textContent).toBe('Sua vez, Pensantus');
    expect(el.textContent).toContain('depois de você: Brisa');
    // The movement is the row under the map, and "Mover" is not in the card (a phone has it in that row).
    const buttons = Array.from(el.querySelectorAll('button'), (b) => b.textContent?.trim());
    expect(buttons.some((b) => b?.endsWith('Mover'))).toBe(false);
    expect(buttons.some((b) => b?.endsWith('Encerrar turno'))).toBe(true);
    expect(
      Array.from(el.querySelectorAll('.eco > li'), (li) => li.getAttribute('aria-label')),
    ).toEqual(['Ação disponível', 'Ação bônus disponível', 'Reação disponível']);
  });

  it('keeps the order strip in the card, unless the page draws it under the actions on the own turn', () => {
    const fixture = TestBed.createComponent(TurnPanel);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: order, currentCombatantId: 'pen' }),
    );
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('app-order-strip')).not.toBeNull();

    fixture.componentRef.setInput('orderBelow', true);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('app-order-strip')).toBeNull();
    // The three shapes are still there, one row of them.
    expect(fixture.nativeElement.querySelectorAll('.eco > li')).toHaveLength(3);

    // Off turn there are no actions under the card, so the strip stays in it.
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: order, currentCombatantId: 'cap' }),
    );
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('app-order-strip')).not.toBeNull();
  });

  it('says the turn waits for an opportunity attack, and what stopped a move short', () => {
    const fixture = TestBed.createComponent(TurnPanel);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: order, currentCombatantId: 'pen' }),
    );
    fixture.componentRef.setInput('waiting', {
      title: 'Esperando a reação do mestre',
      detail: 'Seu movimento já valeu; a sua vez continua quando ele responder.',
    });
    fixture.componentRef.setInput('moveNote', 'Você parou antes: algo bloqueou o caminho.');
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const notices = Array.from(el.querySelectorAll('.turn__notice'), (n) =>
      n.textContent?.replace(/\s+/g, ' ').trim(),
    );
    expect(notices[0]).toContain(
      'Esperando a reação do mestre. Seu movimento já valeu; a sua vez continua quando ele responder.',
    );
    expect(notices[1]).toContain('Você parou antes: algo bloqueou o caminho.');
    expect(el.querySelectorAll('.turn__notice[role="status"]').length).toBe(2);
  });

  it("says the player is blind while looking through the familiar's eyes, and offers no attack (MR-036, E9-04)", () => {
    const blindOrder = order.map((c) =>
      c.id === 'pen' ? { ...c, familiarSightCreatureId: 'nanquim' } : c,
    );
    const el = text({ combatants: blindOrder, currentCombatantId: 'pen' });
    expect(textOf(el.querySelector('[data-testid="familiar-blind"]'))).toBe(
      'visibility_off Cego: para atacar, fale com o mestre.',
    );
    // Without the sight there is no such line.
    expect(
      text({ currentCombatantId: 'pen' }).querySelector('[data-testid="familiar-blind"]'),
    ).toBeNull();
  });
});

describe('TurnPanel as a beast (MR-037, E9-11 state 3)', () => {
  it('says the form and what changes (the two reserves and the armor class are on the vitals card)', () => {
    const druid = combatant({
      id: 'sal',
      label: 'Sálvia',
      kind: CombatantKind.PLAYER,
      mine: true,
      speedDft: 400,
      speedFt: 40,
      movementLeftDft: 400,
      movementLeftFt: 40,
      wildShapeBeastKey: 'monster:wolf',
      wildShapeBeastNamePt: 'Lobo',
      wildShapeHitPointsCurrent: 11,
      wildShapeHitPointsMax: 11,
    });
    const fixture = TestBed.createComponent(TurnPanel);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: [druid], currentCombatantId: 'sal' }),
    );
    fixture.componentRef.setInput('hitPointsMax', 38);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const flatText = (n: Element | null) => n?.textContent?.replace(/\s+/g, ' ').trim();
    expect(flatText(el.querySelector('app-wild-band'))).toContain('Na forma de Lobo');
    expect(flatText(el.querySelector('app-wild-band'))).toContain('Sem magias · 12,0');
    // One armor class on the page, and it is the vitals card's: not in the band.
    expect(flatText(el.querySelector('app-wild-band'))).not.toContain('CA');
    expect(el.querySelector('app-wild-pools')).toBeNull();
  });
});

describe('TurnPanel, the wait line (PM-04)', () => {
  const order = [
    combatant({ id: 'cap', label: 'Capitão Goblin' }),
    combatant({ id: 'pen', label: 'Pensantus', kind: CombatantKind.PLAYER, mine: true }),
    combatant({ id: 'brisa', label: 'Brisa', kind: CombatantKind.PLAYER }),
  ];
  const wait = { title: 'Esperando o mestre', detail: 'O turno continua quando ele responder.' };

  function panel(over: Parameters<typeof encounter>[0]) {
    const fixture = TestBed.createComponent(TurnPanel);
    fixture.componentRef.setInput('encounter', encounter({ combatants: order, ...over }));
    fixture.componentRef.setInput('waiting', wait);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('says it, in a status line, in the turn of someone else', () => {
    const el = panel({ currentCombatantId: 'cap' });
    const line = el.querySelector('[data-testid="reaction-wait"]');
    expect(line?.getAttribute('role')).toBe('status');
    expect(line?.textContent?.replace(/\s+/g, ' ')).toContain(
      'Esperando o mestre. O turno continua quando ele responder.',
    );
    expect(line?.tagName.toLowerCase()).toBe('app-master-wait');
    expect(el.querySelector('h2')?.textContent).toBe('Vez do Capitão Goblin');
  });

  it('says it once in the turn of someone else', () => {
    const waits = (el: HTMLElement) =>
      Array.from(el.querySelectorAll('.mr-notice, app-master-wait')).filter((n) =>
        n.textContent?.includes('Esperando o mestre.'),
      ).length;
    expect(waits(panel({ currentCombatantId: 'cap' }))).toBe(1);
    expect(waits(panel({ currentCombatantId: 'brisa' }))).toBe(1);
  });

  it('says it in the player\'s own turn, and "Encerrar turno" stays reachable with the reason', () => {
    const el = panel({ currentCombatantId: 'pen' });
    const status = Array.from(el.querySelectorAll('[role="status"]')).find((s) =>
      s.textContent?.includes('Esperando o mestre.'),
    );
    expect(status).toBeDefined();
    const end = Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      b.textContent?.includes('Encerrar turno'),
    )!;
    expect(end.getAttribute('aria-disabled')).toBe('true');
    expect(end.getAttribute('aria-describedby')).toBeTruthy();
  });

  it('draws nothing when nothing waits', () => {
    const fixture = TestBed.createComponent(TurnPanel);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants: order, currentCombatantId: 'cap' }),
    );
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[data-testid="reaction-wait"]')).toBeNull();
  });

  it('keeps the condition tags in the card\u2019s second line, and no row of their own when there are none', () => {
    const fixture = TestBed.createComponent(TurnPanel);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        combatants: [
          combatant({ id: 'pen', label: 'Pensantus', kind: CombatantKind.PLAYER, mine: true }),
        ],
        currentCombatantId: 'pen',
      }),
    );
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('.turn__who .turn__states')).not.toBeNull();
    expect(el.querySelector('.turn > .turn__states')).toBeNull();
  });
});
