import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';

import { CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  HideAttemptStatus,
  HideObserverSchema,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { ContestClient } from '../../../../core/combat/contest-client';
import { ContestState } from '../../../../core/combat/contest-state';
import {
  FakeContestClient,
  checkRoll,
  hideAttempt,
  textOf,
} from '../../../../core/combat/contest-testing';
import { HideMasterCard } from './hide-master-card';

const observer = (id: string, passive: number, noticed = false) =>
  create(HideObserverSchema, { combatantId: id, passivePerception: passive, known: true, noticed });

const attempt = () =>
  hideAttempt({
    id: 'hd1',
    hiderId: 'b',
    status: HideAttemptStatus.PENDING,
    roll: checkRoll({ faces: [12], modifier: 7, total: 19 }),
    observers: [
      observer('g1', 9),
      observer('g2', 9),
      observer('hob', 10),
      observer('cap', 10, true),
    ],
  });

function setup() {
  const api = new FakeContestClient();
  const e = encounter({
    combatants: [
      combatant({ id: 'b', label: 'Brisa', kind: CombatantKind.PLAYER }),
      combatant({ id: 'g1', label: 'Goblin 1' }),
      combatant({ id: 'g2', label: 'Goblin 2' }),
      combatant({ id: 'hob', label: 'Hobgoblin' }),
      combatant({ id: 'cap', label: 'Capitão bandido' }),
    ],
  });
  api.encounterAnswer = encounter({ ...e, revision: 9 } as never);
  api.attempt = hideAttempt({ id: 'hd1', status: HideAttemptStatus.APPLIED });
  const state = new CombatState();
  state.encounter.set(e);
  const contests = new ContestState();
  TestBed.configureTestingModule({ providers: [{ provide: ContestClient, useValue: api.as() }] });
  const fixture = TestBed.createComponent(HideMasterCard);
  const ref = fixture.componentRef;
  ref.setInput('attempt', attempt());
  ref.setInput('encounter', e);
  ref.setInput('campaignId', 'camp');
  ref.setInput('state', state);
  ref.setInput('contests', contests);
  const said: string[] = [];
  fixture.componentInstance.said.subscribe((s) => said.push(s));
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const button = (name: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      textOf(b).includes(name),
    );
  const settle = async () => {
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
      fixture.detectChanges();
    }
  };
  return { api, fixture, el, button, settle, said, state, contests };
}

describe("HideMasterCard, the master's decision on a Hide", () => {
  it('reads the total against each passive Perception and who notices, as the board draws', () => {
    const { el } = setup();
    const text = textOf(el);
    expect(textOf(el.querySelector('h2')!)).toBe('Brisa tenta se esconder: Furtividade 19');
    expect(text).toContain(
      'Compare com a Percepção passiva de quem poderia ver quem se esconde. Quem vê claramente quem se esconde não é enganado.',
    );
    const rows = Array.from(el.querySelectorAll('.row')).map((r) => textOf(r));
    expect(rows).toEqual([
      'Goblin 1 Percepção passiva 9 Não nota Vê claramente',
      'Goblin 2 Percepção passiva 9 Não nota Vê claramente',
      'Hobgoblin Percepção passiva 10 Não nota Vê claramente',
      'Capitão bandido Percepção passiva 10 Nota Vê claramente',
    ]);
    expect(text).toContain('Aplicar: escondido (3 não notam)');
    expect(text).toContain('Recusar: não há onde se esconder');
    expect(text).toContain(
      'Empate: continua notado (leitura do app: o total precisa superar a Percepção passiva).',
    );
  });

  it('"Vê claramente" takes one out of the count and sends it', async () => {
    const { api, el, button, settle, said, state, contests } = setup();
    el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')[2].click();
    await settle();
    expect(textOf(el)).toContain('Aplicar: escondido (2 não notam)');
    button('Aplicar: escondido')!.click();
    await settle();
    expect(api.hideDecisions).toHaveLength(1);
    expect(api.hideDecisions[0].decision).toEqual({
      attemptId: 'hd1',
      refuse: false,
      refusal: '',
      seesClearlyIds: ['hob'],
    });
    expect(api.hideDecisions[0].key).toEqual(expect.any(String));
    expect(state.encounter()?.revision).toBe(9);
    expect(contests.attempt('hd1')?.status).toBe(HideAttemptStatus.APPLIED);
    expect(said).toEqual(['Brisa se escondeu.']);
  });

  it('refuses with a reason of one sentence', async () => {
    const { api, el, button, settle, fixture, said } = setup();
    button('Recusar: não há onde se esconder')!.click();
    await settle();
    expect(textOf(el)).toContain(
      'Vazio: “Alguém vê você claramente: não dá para se esconder agora.”',
    );
    const field = el.querySelector<HTMLInputElement>('input[type="text"]')!;
    expect(field.getAttribute('maxlength')).toBe('120');
    field.value = '  Campo aberto, sem sombra.  ';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    button('Recusar o esconderijo')!.click();
    await settle();
    expect(api.hideDecisions[0].decision).toEqual({
      attemptId: 'hd1',
      refuse: true,
      refusal: 'Campo aberto, sem sombra.',
      seesClearlyIds: [],
    });
    expect(said).toEqual(['Esconderijo de Brisa recusado.']);
  });

  it('"Voltar" leaves the refusal and an empty reason sends the usual one', async () => {
    const { api, button, settle } = setup();
    button('Recusar: não há onde se esconder')!.click();
    await settle();
    button('Voltar')!.click();
    await settle();
    expect(button('Aplicar: escondido')).toBeTruthy();
    button('Recusar: não há onde se esconder')!.click();
    await settle();
    button('Recusar o esconderijo')!.click();
    await settle();
    expect(api.hideDecisions[0].decision.refusal).toBe('');
  });

  it('shows the server refusal (nothing pending any more)', async () => {
    const { api, el, button, settle } = setup();
    api.error = new ConnectError('gone', Code.FailedPrecondition);
    button('Aplicar: escondido')!.click();
    await settle();
    expect(el.querySelector('[role="alert"]')).toBeTruthy();
  });
});
