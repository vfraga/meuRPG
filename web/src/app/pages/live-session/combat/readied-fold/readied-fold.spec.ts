import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import {
  CombatantSchema,
  EncounterSchema,
  EncounterStatus,
  ReadiedActionSchema,
  ReadiedKind,
  ReadiedStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ContestClient } from '../../../../core/combat/contest-client';
import { FakeContestClient, contestState } from '../../../../core/combat/contest-testing';
import { ReadiedFold } from './readied-fold';

const flat = (n: Element | null | undefined) =>
  n?.textContent
    ?.replace(/\s+/g, ' ')
    .replace(/\u00a0/g, ' ')
    .trim() ?? '';

const readied = (id: string, trigger: string, name: string) =>
  create(ReadiedActionSchema, {
    id,
    triggerText: trigger,
    kind: ReadiedKind.ATTACK,
    actionNamePt: name,
    targetCombatantId: 'z1',
    status: ReadiedStatus.WAITING,
  });

const combat = (members: Record<string, unknown>[]) =>
  create(EncounterSchema, {
    id: 'e1',
    status: EncounterStatus.ACTIVE,
    currentCombatantId: 'z1',
    combatants: [
      create(CombatantSchema, { id: 'z1', label: 'Zumbi 1', hitPointsCurrent: 22 }),
      ...members.map((m) => create(CombatantSchema, { hitPointsCurrent: 30, ...m } as never)),
    ],
  } as never);

describe('ReadiedFold (#582, board 582-04)', () => {
  const api = new FakeContestClient();

  function render(e: ReturnType<typeof combat>, busy = false) {
    api.stateResponse = contestState({ surprise: { combatantIds: ['jabuti'] } } as never);
    TestBed.configureTestingModule({ providers: [{ provide: ContestClient, useValue: api.as() }] });
    const fixture = TestBed.createComponent(ReadiedFold);
    fixture.componentRef.setInput('encounter', e);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('busy', busy);
    fixture.detectChanges();
    return fixture;
  }

  const rows = (f: ReturnType<typeof render>) =>
    Array.from((f.nativeElement as HTMLElement).querySelectorAll('li.row'));

  it('shows nothing while nobody holds a readied action', () => {
    const f = render(combat([{ id: 'tor', label: 'Tor' }]));
    expect(f.nativeElement.querySelector('app-fold')).toBeNull();
  });

  it('lists who holds what with the trigger quoted, and the count', async () => {
    const f = render(
      combat([
        {
          id: 'tor',
          label: 'Tor',
          readied: readied('r1', 'Quando o zumbi passar pela porta', 'Espada longa'),
        },
        { id: 'b', label: 'Brisa', readied: readied('r2', 'Quando Tor recuar', 'Arco longo') },
      ]),
    );
    await f.whenStable();
    const root = f.nativeElement as HTMLElement;
    expect(flat(root.querySelector('.mr-fold__title'))).toBe('Ações preparadas');
    expect(flat(root.querySelector('.mr-fold__note'))).toBe('2');
    expect(rows(f).map(flat)).toEqual([
      expect.stringContaining('Tor · Espada longa no Zumbi 1 “Quando o zumbi passar pela porta”'),
      expect.stringContaining('Brisa · Arco longo no Zumbi 1 “Quando Tor recuar”'),
    ]);
    expect(root.textContent).toContain(
      'Dispare quando o gatilho acontecer; quem preparou decide se usa.',
    );
  });

  it('fires the row that is free and says nothing else of it', async () => {
    const f = render(
      combat([
        {
          id: 'tor',
          label: 'Tor',
          readied: readied('r1', 'Quando o zumbi passar', 'Espada longa'),
        },
      ]),
    );
    await f.whenStable();
    const seen: string[] = [];
    f.componentInstance.fire.subscribe((id) => seen.push(id));
    const go = (f.nativeElement as HTMLElement).querySelector(
      'button.row__fire',
    ) as HTMLButtonElement;
    expect(go.disabled).toBe(false);
    expect(go.getAttribute('aria-label')).toBe('Disparar a ação de Tor');
    go.click();
    expect(seen).toEqual(['r1']);
    expect(flat(rows(f)[0])).not.toContain('não reage');
  });

  it('keeps "Disparar" off with the reason in the row, and does not fire', async () => {
    const f = render(
      combat([
        {
          id: 'tor',
          label: 'Tor',
          reactionUsed: true,
          readied: readied('r1', 'Quando o zumbi passar', 'Espada longa'),
        },
      ]),
    );
    await f.whenStable();
    const seen: string[] = [];
    f.componentInstance.fire.subscribe((id) => seen.push(id));
    const go = (f.nativeElement as HTMLElement).querySelector(
      'button.row__fire',
    ) as HTMLButtonElement;
    expect(go.getAttribute('aria-disabled')).toBe('true');
    expect(flat(rows(f)[0])).toContain('Tor já usou a reação nesta rodada.');
    expect(go.getAttribute('aria-describedby')).toBe(rows(f)[0].querySelector('.row__why')?.id);
    go.click();
    expect(seen).toEqual([]);
  });

  it('reads who is surprised from the contest state of the table', async () => {
    const f = render(
      combat([
        { id: 'jabuti', label: 'Jabuti', readied: readied('r3', 'Quando Tor recuar', 'Esquivar') },
      ]),
    );
    await f.whenStable();
    f.detectChanges();
    expect(flat(rows(f)[0])).toContain(
      'Jabuti foi surpreendido: só reage depois da primeira vez dele.',
    );
  });

  it('marks the hidden creature that readied: only the master sees it', async () => {
    const f = render(
      combat([
        {
          id: 'z2',
          label: 'Zumbi 2',
          hidden: true,
          readied: readied('r4', 'Quando o guerreiro passar', 'Mordida'),
        },
      ]),
    );
    await f.whenStable();
    expect(flat(rows(f)[0])).toContain('Só você vê');
  });

  it('keeps every "Disparar" off while the master is busy', async () => {
    const f = render(
      combat([
        {
          id: 'tor',
          label: 'Tor',
          readied: readied('r1', 'Quando o zumbi passar', 'Espada longa'),
        },
      ]),
      true,
    );
    await f.whenStable();
    const go = (f.nativeElement as HTMLElement).querySelector(
      'button.row__fire',
    ) as HTMLButtonElement;
    expect(go.getAttribute('aria-disabled')).toBe('true');
  });
});
