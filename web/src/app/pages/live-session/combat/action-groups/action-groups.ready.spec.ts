import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import {
  CombatantSchema,
  ReadiedKind,
  ReadiedStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  ActionOptionSchema,
  DisabledReasonCode,
  TurnOptionsSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { ActionGroups } from './action-groups';

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const standard = (key: string, namePt: string, over: object = {}) =>
  create(ActionOptionSchema, {
    action: { key: `standard:${key}`, namePt, economy: ActionEconomy.ACTION },
    enabled: true,
    ...over,
  } as never);

function setup(ready: ReturnType<typeof standard>, locked = '') {
  const fixture = TestBed.createComponent(ActionGroups);
  fixture.componentRef.setInput(
    'options',
    create(TurnOptionsSchema, {
      standardActions: [standard('dodge', 'Esquivar'), standard('search', 'Procurar'), ready],
    }),
  );
  fixture.componentRef.setInput(
    'own',
    create(CombatantSchema, { movementLeftFt: 30, speedFt: 30 }),
  );
  fixture.componentRef.setInput('locked', locked);
  const readied: number[] = [];
  const taken: string[] = [];
  fixture.componentInstance.ready.subscribe(() => readied.push(1));
  fixture.componentInstance.action.subscribe((k) => taken.push(k));
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const row = () =>
    Array.from(el.querySelectorAll('app-action-row')).find(
      (r) => plain(r.querySelector('.row__name')?.textContent) === 'Preparar',
    );
  return { el, row, readied, taken, fixture };
}

const readyOption = (over: object = {}) =>
  standard('ready', 'Preparar', { viaReadyAction: true, ...over });

describe('ActionGroups: Preparar (#582)', () => {
  it('is a row of Outras, with the Action mark, what it does and a "Preparar" button', () => {
    const { row } = setup(readyOption());
    const r = row()!;
    expect(r).toBeTruthy();
    expect(plain(r.querySelector('.row__detail')?.textContent)).toBe(
      'Guarda uma ação para quando algo acontecer.',
    );
    expect(plain(r.querySelector('button')?.textContent)).toBe('alarm Preparar');
  });

  it('opens the sheet, and never sends the key of Ready as a plain standard action', () => {
    const { row, readied, taken, fixture } = setup(readyOption());
    (row()!.querySelector('button') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(readied).toEqual([1]);
    expect(taken).toEqual([]);
  });

  it('keeps the other standard actions as buttons and does not repeat Preparar among them', () => {
    const { el } = setup(readyOption());
    const buttons = Array.from(el.querySelectorAll('.std__btn')).map((b) => plain(b.textContent));
    expect(buttons).toEqual(['Esquivar', 'Procurar']);
  });

  it("writes the server's reason when the Action is spent, and does not open the sheet", () => {
    const { row, readied, fixture } = setup(
      readyOption({ enabled: false, reason: { code: DisabledReasonCode.ACTION_USED } }),
    );
    const r = row()!;
    expect(plain(r.querySelector('.row__why')?.textContent)).toContain('Ação já usada');
    (r.querySelector('button') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(readied).toEqual([]);
  });

  it('stays off while a window of the table waits, with the lock as the reason', () => {
    const { row, readied, fixture } = setup(readyOption(), 'Esperando a reação do mestre');
    (row()!.querySelector('button') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(readied).toEqual([]);
  });
});

describe('ActionGroups: the readied action under the economy row (#582)', () => {
  function withReadied(status = ReadiedStatus.WAITING) {
    const fixture = TestBed.createComponent(ActionGroups);
    fixture.componentRef.setInput(
      'options',
      create(TurnOptionsSchema, { standardActions: [standard('dodge', 'Esquivar')] }),
    );
    fixture.componentRef.setInput(
      'own',
      create(CombatantSchema, {
        movementLeftFt: 30,
        speedFt: 30,
        readied: {
          id: 'r1',
          triggerText: 'Quando o zumbi passar pela porta',
          kind: ReadiedKind.ATTACK,
          actionNamePt: 'Espada longa',
          targetCombatantId: 'z1',
          status,
        },
      }),
    );
    fixture.componentRef.setInput('combatants', [{ id: 'z1', label: 'Zumbi 1' }]);
    const given: string[] = [];
    fixture.componentInstance.giveUp.subscribe((id) => given.push(id));
    fixture.detectChanges();
    return { el: fixture.nativeElement as HTMLElement, given };
  }

  it('shows the card right after the economy row, outside every tab', () => {
    const { el } = withReadied();
    const card = el.querySelector('app-readied-card')!;
    expect(card).toBeTruthy();
    expect(card.closest('[role="tabpanel"]')).toBeNull();
    expect(el.querySelector('.econ')!.nextElementSibling).toBe(card);
    expect(plain(card.querySelector('h3')?.textContent)).toBe('Espada longa no Zumbi 1');
  });

  it('shows nothing for a combatant that holds none, or whose action is over', () => {
    expect(withReadied(ReadiedStatus.LOST).el.querySelector('app-readied-card')).toBeNull();
    const { el } = setup(readyOption());
    expect(el.querySelector('app-readied-card')).toBeNull();
  });
});
