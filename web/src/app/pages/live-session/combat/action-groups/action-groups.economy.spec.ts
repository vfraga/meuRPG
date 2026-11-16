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
  TurnOptionsSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { ActionGroups } from './action-groups';

// The first row of "Ações" (584-01, 584-02): what is left of the turn, "Mover", and "Permanecer na forma" after "Voltar".
function setup(own: Record<string, unknown> = {}, inputs: Record<string, unknown> = {}) {
  const fixture = TestBed.createComponent(ActionGroups);
  fixture.componentRef.setInput('options', create(TurnOptionsSchema, {}));
  fixture.componentRef.setInput(
    'own',
    create(CombatantSchema, {
      movementLeftFt: 40,
      movementLeftDft: 400,
      speedFt: 40,
      speedDft: 400,
      ...own,
    }),
  );
  for (const [k, v] of Object.entries(inputs)) {
    fixture.componentRef.setInput(k, v);
  }
  const events: string[] = [];
  fixture.componentInstance.move.subscribe(() => events.push('move'));
  fixture.componentInstance.stayForm.subscribe(() => events.push('stay'));
  fixture.componentInstance.leaveForm.subscribe(() => events.push('leave'));
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  return { el, events, fixture };
}
const text = (n: Element | null) => (n?.textContent ?? '').replace(/\s+/g, ' ').trim();
const label = (li: Element) => li.getAttribute('aria-label')?.replace(/ /g, ' ');

describe('ActionGroups: the economy row', () => {
  it('is the first thing in the panel, a list named "O que resta da vez", with the state in words', () => {
    const { el } = setup({ bonusActionUsed: true });
    const groups = el.querySelector('section.groups')!;
    const row = groups.querySelector('.econ')!;
    expect(row).not.toBeNull();
    // It comes before the tabs and stays while any of them is open.
    expect(row.compareDocumentPosition(groups.querySelector('app-tab-list')!)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    const list = row.querySelector('ul')!;
    expect(list.getAttribute('aria-label')).toBe('O que resta da vez');
    expect([...list.querySelectorAll('li')].map(label)).toEqual([
      'Ação disponível',
      'Ação bônus usada',
      'Reação disponível',
      'Movimento: 12,0 m',
    ]);
  });

  it('has no divider and no room of its own under it when the pair card comes next (588-05): the column gap, 12 px, is all there is', () => {
    const plain = getComputedStyle(setup().el.querySelector('.econ')!);
    expect(plain.borderBottomStyle).toBe('solid');
    expect(plain.paddingBottom).toBe('12px');
    const { el } = setup({}, { paired: true });
    const row = getComputedStyle(el.querySelector('.econ')!);
    expect(row.borderBottomStyle).toBe('none');
    expect(parseFloat(row.paddingBottom)).toBe(0);
    expect(parseFloat(row.marginBottom)).toBe(0);
    expect(getComputedStyle(el.querySelector('section.groups')!).gap).toBe('var(--mr-space-3)');
  });

  it('has "Mover" as its only button, and it opens the move page', () => {
    const { el, events } = setup();
    const buttons = [...el.querySelectorAll('.econ button')];
    expect(buttons.map(text)).toEqual(['open_withMover']);
    (buttons[0] as HTMLButtonElement).click();
    expect(events).toEqual(['move']);
  });

  it('turns "Mover" off while an answer is awaited, and leaves it out with nothing left to move', () => {
    const waiting = setup({}, { locked: 'Esperando a reação do mestre' });
    const move = waiting.el.querySelector<HTMLButtonElement>('.econ button')!;
    expect(move.getAttribute('aria-disabled') === 'true' || move.disabled).toBe(true);
    move.click();
    expect(waiting.events).toEqual([]);
    TestBed.resetTestingModule();
    const still = setup({ movementLeftFt: 0, movementLeftDft: 0 });
    expect(still.el.querySelector('.econ button')).toBeNull();
    expect([...still.el.querySelectorAll('.econ li')]).toHaveLength(3);
  });

  it('leaves "Mover" to the map header while the master acts for the character', () => {
    const acting = setup({}, { moveAside: true });
    expect(acting.el.querySelector('.econ button')).toBeNull();
    expect([...acting.el.querySelectorAll('.econ li')]).toHaveLength(4);
  });

  it('leaves "Mover" out for a grappled or a surprised character, and for a combat without a map', () => {
    const grappled = setup({}, { contest: create(ContestTurnStateSchema, { grappled: true }) });
    expect(grappled.el.querySelector('.econ button')).toBeNull();
    TestBed.resetTestingModule();
    const surprised = setup({}, { contest: create(ContestTurnStateSchema, { surprised: true }) });
    expect(surprised.el.querySelector('.econ button')).toBeNull();
    TestBed.resetTestingModule();
    const theatre = setup({}, { theatre: true });
    expect(theatre.el.querySelector('.econ button')).toBeNull();
    // With no map the walker keeps the count of the metres.
    expect([...theatre.el.querySelectorAll('.econ li')]).toHaveLength(4);
  });
});

describe('ActionGroups: "Permanecer na forma" after "Voltar"', () => {
  const stay = {
    detail: 'Sem magias · 12,0 m · gasta 1 uso (restam 1 de 2) e o tempo volta a 2 h',
    reason: '',
  };
  const rows = (el: HTMLElement) => [...el.querySelectorAll('app-action-row')];
  const named = (el: HTMLElement, name: string) => rows(el).find((r) => text(r).startsWith(name))!;

  it('comes second, with a bolt and "Sem ação", and presses as a stay', () => {
    const { el, events } = setup({}, { beast: 'Lobo', stay });
    const order = rows(el)
      .map((r) => text(r.querySelector('.row__name, h4, h3, b') ?? r))
      .filter((t) => /Voltar à forma normal|Permanecer na forma/.test(t));
    expect(order.length).toBeGreaterThanOrEqual(2);
    expect(rows(el).indexOf(named(el, 'Voltar à forma normal'))).toBeLessThan(
      rows(el).indexOf(named(el, 'Permanecer na forma')),
    );
    const row = named(el, 'Permanecer na forma');
    expect(text(row)).toContain('Sem ação');
    expect(text(row)).toContain(
      'Sem magias · 12,0 m · gasta 1 uso (restam 1 de 2) e o tempo volta a 2 h',
    );
    row.querySelector('button')!.click();
    expect(events).toEqual(['stay']);
  });

  it('is off, with the reason, with no use left or after the form ended, and spends nothing then', () => {
    for (const reason of [
      'Sem usos de Forma Selvagem · voltam no descanso curto ou longo',
      'A forma já acabou. Para virar fera de novo, use Forma Selvagem.',
    ]) {
      TestBed.resetTestingModule();
      const { el, events } = setup({}, { beast: 'Lobo', stay: { ...stay, reason } });
      const row = named(el, 'Permanecer na forma');
      expect(row.querySelector('button')?.getAttribute('aria-disabled')).toBe('true');
      expect(text(row)).toContain(reason);
      row.querySelector('button')!.click();
      expect(events).toEqual([]);
    }
  });

  it("is not there in the character's own shape", () => {
    const { el } = setup({}, { beast: '', stay: null });
    expect(text(el)).not.toContain('Permanecer na forma');
  });

  it('does not take the action: its row costs nothing even with the Ação and the Bônus spent', () => {
    const { el } = setup({ actionUsed: true, bonusActionUsed: true }, { beast: 'Lobo', stay });
    const row = named(el, 'Permanecer na forma');
    expect(row.querySelector('button')?.getAttribute('aria-disabled')).not.toBe('true');
  });
});

describe('ActionGroups: the tabs in a form (584-01, 584-02)', () => {
  const stay = { detail: 'Sem magias · 12,0 m · gasta 1 uso e o tempo volta a 2 h', reason: '' };
  const bite = create(AttackOptionSchema, {
    enabled: true,
    attack: create(AttackSchema, {
      key: 'attack:bite',
      name: 'Bite',
      namePt: 'Mordida',
      kind: AttackKind.WEAPON,
      attackBonus: 4,
      damage: '2d4 + 2',
      damageTypePt: 'perfurante',
    }),
  });
  const options = create(TurnOptionsSchema, { attacks: [bite] });
  const tabs = (el: HTMLElement) =>
    [...el.querySelectorAll('[role="tab"]')].map((t) => text(t).replace(/^\S+?(?=[A-ZÀ-Ú])/, ''));

  it('has Ataques, the class and Outras, no Magias tab, and the notice at the foot', () => {
    const { el } = setup({}, { beast: 'Lobo', stay, options });
    expect(tabs(el).map((t) => t.replace(/\d+$/, '').trim())).toEqual([
      'Ataques',
      'Druida',
      'Outras',
    ]);
    expect(el.querySelector('[id$="spells"][role="tabpanel"]')).toBeNull();
    const notice = el.querySelector('#no-spells')!;
    expect(text(notice)).toContain(
      'Sem magias na forma de fera. Volte à forma normal para conjurar.',
    );
    expect(el.querySelector('section.groups')!.lastElementChild).toBe(notice);
  });

  it('counts in the class tab what it holds: Voltar and Permanecer', () => {
    const { el } = setup({}, { beast: 'Lobo', stay, options });
    expect(tabs(el)[1]).toMatch(/Druida\s*2$/);
    TestBed.resetTestingModule();
    const ended = setup({}, { beast: 'Lobo', stay: null, options });
    expect(tabs(ended.el)[1]).toMatch(/Druida\s*1$/);
  });

  it('keeps the shove in a form and drops the grapple: a beast has no free hand (SRD 5.1)', () => {
    const special = (kind: ContestAttackOptionKind) =>
      create(ContestAttackOptionSchema, { kind, replacesAttack: true, enabled: true });
    const contestAttacks = [
      special(ContestAttackOptionKind.GRAPPLE),
      special(ContestAttackOptionKind.SHOVE),
    ];
    const { el } = setup(
      {},
      { beast: 'Lobo', stay, options, contestAttacks, gearFlow: { gear: () => [] } },
    );
    // No drawing or putting away of weapons in a form.
    expect(el.querySelector('.panel app-gear-fold')).toBeNull();
    expect(text(el)).not.toContain('Agarrar');
    expect(text(el)).toContain('Empurrar');
    // The count of the tab is what it holds: the bite and the shove.
    expect(tabs(el)[0]).toMatch(/Ataques\s*2$/);
    TestBed.resetTestingModule();
    // Out of a form the character keeps both.
    const own = setup({}, { beast: '', options, contestAttacks });
    expect(text(own.el)).toContain('Agarrar');
    expect(text(own.el)).toContain('Empurrar');
  });
});
