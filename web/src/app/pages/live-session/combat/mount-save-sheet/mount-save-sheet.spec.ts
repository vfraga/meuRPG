import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  MountSaveCause,
  MountSavePromptSchema,
  MountSaveResultSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { heldAnswer, heldLucky } from '../../../../core/held-rolls/held-roll-testing';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { MountSaveSheet, type MountSaveSheetData } from './mount-save-sheet';

const plain = (text: string | null | undefined) =>
  (text ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const prompt = (over: object = {}) =>
  create(MountSavePromptSchema, {
    riderId: 'tor',
    mountId: 'h',
    dc: 0,
    cause: MountSaveCause.PUSHED,
    mode: RollMode.NORMAL,
    saveBonus: 2,
    bonusKnown: true,
    ...over,
  });

const fighters = [
  combatant({ id: 'tor', label: 'Tor', kind: CombatantKind.PLAYER, mine: true, mountedOnId: 'h' }),
  combatant({ id: 'h', label: 'Cavalo de guerra', riderId: 'tor' }),
  combatant({ id: 'w', label: 'Wight' }),
];
const fight = (p: ReturnType<typeof prompt> | undefined, revision = 1, held?: object) =>
  encounter({
    revision,
    round: 4,
    currentCombatantId: 'w',
    // The server keeps a kept d20's question on the roller's row too.
    combatants: held ? [{ ...fighters[0], heldRollOffer: held }, ...fighters.slice(1)] : fighters,
    mountSave: p,
  } as never);

const held17 = create(MountSaveResultSchema, {
  save: { diceCount: 1, diceSides: 20, faces: [15], modifier: 2, total: 17 },
  held: true,
} as never);
const fell8 = create(MountSaveResultSchema, {
  save: { diceCount: 1, diceSides: 20, faces: [6], modifier: 2, total: 8 },
  held: false,
} as never);

describe('MountSaveSheet (#588, boards 588-06 and 588-10 card 6)', () => {
  const rollMountSave = vi.fn();
  const get = vi.fn();
  const resources = { answerHeldRoll: vi.fn() };
  const closed: unknown[] = [];

  function setup(p = prompt(), over: Partial<MountSaveSheetData> = {}, kept?: object) {
    const state = new CombatState();
    state.apply(fight(p, 1, kept));
    const data: MountSaveSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      prompt: p,
      byMaster: false,
      state,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      ...over,
    };
    closed.length = 0;
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: fakeCombatClient({ rollMountSave, get }) },
        { provide: ResourceClient, useValue: resources },
        { provide: MAT_DIALOG_DATA, useValue: data },
        {
          provide: MatDialogRef,
          useValue: { close: (r?: unknown) => closed.push(r ?? 'closed'), disableClose: false },
        },
      ],
    });
    const fixture = TestBed.createComponent(MountSaveSheet);
    document.body.appendChild(fixture.nativeElement);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, state };
  }

  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      plain(b.textContent).includes(label),
    )!;
  const flush = async (fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) => {
    await fixture.whenStable();
    fixture.detectChanges();
  };
  const field = (el: HTMLElement, label: string) => {
    const l = Array.from(el.querySelectorAll('label')).find((x) =>
      plain(x.textContent).includes(label),
    )!;
    return el.querySelector<HTMLInputElement>(`#${l.getAttribute('for')}`)!;
  };
  const fill = (input: HTMLInputElement, value: string) => {
    input.value = value;
    input.dispatchEvent(new Event('input'));
  };

  beforeEach(() => {
    rollMountSave.mockReset();
    get.mockReset();
    // By default the read after a refusal fails too: the screen keeps what it has.
    get.mockRejectedValue(new Error('offline'));
    resources.answerHeldRoll.mockReset();
  });

  it('asks the rider as the board does, with no DC on a table that hides it, and has to be answered', () => {
    const { el } = setup();
    expect(plain(el.querySelector('h2')?.textContent)).toBe('Você cai da montaria?');
    expect(plain(el.querySelector('.frame__sub')?.textContent)).toBe(
      'Teste de resistência de Destreza',
    );
    expect(plain(el.querySelector('.what')?.textContent)).toBe(
      'O Cavalo de guerra foi empurrado. Passe para continuar montado; se falhar, você cai Derrubado num quadrado ao lado dele.',
    );
    expect(Array.from(el.querySelectorAll('.tag')).map((t) => plain(t.textContent))).toEqual([
      '1d20 + 2',
      'Destreza +2',
    ]);
    expect(plain(el.querySelector('.wait')?.textContent)).toBe(
      'hourglass_topA vez do Wight espera este teste.',
    );
    expect(plain(el.textContent)).not.toMatch(/\bCD\b/);
    const app = button(el, 'Rolar no app');
    expect(app.classList.contains('mat-mdc-unelevated-button')).toBe(true);
    expect(app.hasAttribute('data-initial-focus')).toBe(true);
    expect(button(el, 'Digitar o resultado').classList.contains('mat-mdc-outlined-button')).toBe(
      true,
    );
    // The table waits for it: the rider's sheet has no "Fechar" while the save is owed.
    expect(el.querySelector('.frame__close')).toBeNull();
  });

  it('shows the DC where the server sent it: in the line under the title, a tag and the result', async () => {
    rollMountSave.mockResolvedValue({
      encounter: fight(undefined, 2),
      result: create(MountSaveResultSchema, { ...held17, dc: 10 }),
      held: undefined,
    });
    const { fixture, el } = setup(prompt({ dc: 10 }));
    expect(plain(el.querySelector('.frame__sub')?.textContent)).toBe(
      'Teste de resistência de Destreza · CD 10',
    );
    expect(Array.from(el.querySelectorAll('.tag')).map((t) => plain(t.textContent))).toContain(
      'CD 10',
    );
    button(el, 'Rolar no app').click();
    await flush(fixture);
    expect(plain(el.querySelector('.res')?.textContent)).toContain(
      '15 + 2 = 17 contra CD 10: você continua montado.',
    );
  });

  it('"Rolar no app" rolls for the rider, applies the combat and tells the result', async () => {
    rollMountSave.mockResolvedValue({
      encounter: fight(undefined, 2),
      result: held17,
      held: undefined,
    });
    const { fixture, el, state } = setup();
    button(el, 'Rolar no app').click();
    await flush(fixture);
    expect(rollMountSave).toHaveBeenCalledWith(
      'c',
      'enc',
      'tor',
      { kind: 'app' },
      expect.any(String),
    );
    expect(state.encounter()?.revision).toBe(2);
    const res = el.querySelector('.res')!;
    expect(res.getAttribute('role')).toBe('status');
    expect(plain(res.querySelector('.pill')?.textContent)).toBe('checkPassou');
    expect(plain(res.querySelector('.what')?.textContent)).toBe(
      '15 + 2 = 17: você continua montado.',
    );
    expect(plain(res.textContent)).not.toMatch(/\bCD\b/);
    // The buttons that had the focus are gone: it goes to "Fechar", not to the page behind.
    expect(document.activeElement).toBe(button(el, 'Fechar'));
    button(el, 'Fechar').click();
    expect(closed).toHaveLength(1);
  });

  it('reads the combat again after a refusal that may mean the screen is stale: the master rolled it meanwhile', async () => {
    rollMountSave.mockRejectedValueOnce(new ConnectError('changed', Code.Aborted));
    get.mockResolvedValue(fight(undefined, 9));
    const { fixture, el, state } = setup();
    button(el, 'Rolar no app').click();
    await flush(fixture);
    expect(get).toHaveBeenCalledWith('c');
    expect(state.encounter()?.revision).toBe(9);
    expect(button(el, 'Rolar no app')).toBeUndefined();
    expect(plain(el.textContent)).toContain('Este teste não espera mais a sua rolagem');
  });

  it('tells a fall with the mount it fell from', async () => {
    rollMountSave.mockResolvedValue({
      encounter: fight(undefined, 2),
      result: fell8,
      held: undefined,
    });
    const { fixture, el } = setup();
    button(el, 'Rolar no app').click();
    await flush(fixture);
    const res = el.querySelector('.res')!;
    expect(plain(res.querySelector('.pill')?.textContent)).toBe('closeFalhou');
    expect(res.querySelector('.pill')?.classList.contains('pill--bad')).toBe(true);
    expect(plain(res.querySelector('.what')?.textContent)).toBe(
      '6 + 2 = 8: você caiu Derrubado ao lado do Cavalo de guerra.',
    );
  });

  it('"Digitar o resultado" takes the face of the d20, with its live sum, and sends it', async () => {
    rollMountSave.mockResolvedValue({
      encounter: fight(undefined, 2),
      result: held17,
      held: undefined,
    });
    const { fixture, el } = setup();
    button(el, 'Digitar o resultado').click();
    fixture.detectChanges();
    fill(field(el, 'Face do d20 (1 a 20)'), '15');
    fixture.detectChanges();
    expect(plain(el.textContent)).toContain('15 + 2 = 17');
    button(el, 'Confirmar').click();
    await flush(fixture);
    expect(rollMountSave.mock.calls[0][3]).toEqual({ kind: 'typed', face: 15, extra: [] });
  });

  it('asks the two d20 of a save with disadvantage, in words, and sends both', async () => {
    rollMountSave.mockResolvedValue({
      encounter: fight(undefined, 2),
      result: fell8,
      held: undefined,
    });
    const { fixture, el } = setup(prompt({ mode: RollMode.DISADVANTAGE }));
    expect(plain(el.textContent)).toContain(
      'Você tem desvantagem neste teste. Role dois d20; vale o menor.',
    );
    button(el, 'Digitar o resultado').click();
    fixture.detectChanges();
    fill(field(el, 'Primeiro d20'), '15');
    fill(field(el, 'Segundo d20'), '6');
    fixture.detectChanges();
    button(el, 'Confirmar').click();
    await flush(fixture);
    expect(rollMountSave.mock.calls[0][3]).toEqual({
      kind: 'typed',
      face: 15,
      second: 6,
      extra: [],
    });
  });

  it('opens the typed fields at once on a table of physical dice, with no app roll', () => {
    const { el } = setup(prompt(), { diceMode: DiceMode.PHYSICAL });
    expect(button(el, 'Rolar no app')).toBeUndefined();
    expect(field(el, 'Face do d20 (1 a 20)')).toBeTruthy();
  });

  it('has no d20 for a rider that fails by itself: one button, "Continuar"', async () => {
    rollMountSave.mockResolvedValue({
      encounter: fight(undefined, 2),
      result: create(MountSaveResultSchema, { held: false, autoFail: true }),
      held: undefined,
    });
    const { fixture, el } = setup(prompt({ autoFail: true }));
    expect(Array.from(el.querySelectorAll('.tag')).map((t) => plain(t.textContent))).toEqual([
      'Falha automática',
    ]);
    expect(plain(el.textContent)).toContain('Você não consegue se segurar: cai sem rolar.');
    expect(button(el, 'Rolar no app')).toBeUndefined();
    expect(button(el, 'Digitar o resultado')).toBeUndefined();
    button(el, 'Continuar').click();
    await flush(fixture);
    expect(rollMountSave.mock.calls[0][3]).toEqual({ kind: 'app' });
    expect(plain(el.querySelector('.res .pill')?.textContent)).toBe('closeFalha automática');
    expect(plain(el.querySelector('.res .what')?.textContent)).toBe(
      'Você caiu Derrubado ao lado do Cavalo de guerra, sem rolar.',
    );
  });

  it("asks the held roll's question in its place (Lucky, a Bardic Inspiration die) before any result", async () => {
    const kept = heldLucky([1], 1);
    rollMountSave.mockResolvedValue({
      encounter: fight(prompt(), 2, kept),
      result: undefined,
      held: kept,
    });
    const { fixture, el } = setup();
    button(el, 'Rolar no app').click();
    await flush(fixture);
    expect(el.querySelector('app-held-roll-prompt')).not.toBeNull();
    expect(el.querySelector('.res')).toBeNull();
    expect(button(el, 'Rolar no app')).toBeUndefined();
  });

  it('keeps the question and says why when the server refuses, and retries under the same key', async () => {
    rollMountSave.mockRejectedValueOnce(new ConnectError('down', Code.Unavailable));
    rollMountSave.mockResolvedValue({
      encounter: fight(undefined, 2),
      result: held17,
      held: undefined,
    });
    const { fixture, el } = setup();
    button(el, 'Rolar no app').click();
    await flush(fixture);
    expect(el.querySelector('[role=alert]')).not.toBeNull();
    expect(closed).toHaveLength(0);
    button(el, 'Rolar no app').click();
    await flush(fixture);
    expect(rollMountSave.mock.calls[1][4]).toBe(rollMountSave.mock.calls[0][4]);
    expect(el.querySelector('.res')).not.toBeNull();
  });

  it('opens on the question of a d20 already rolled and kept (the screen was read again), never on a second roll', () => {
    const { el } = setup(prompt(), {}, heldLucky([1], 1));
    expect(el.querySelector('app-held-roll-prompt')).not.toBeNull();
    expect(button(el, 'Rolar no app')).toBeUndefined();
    expect(button(el, 'Rolar de novo')).toBeTruthy();
  });

  it('settles the save with the answer to the question, and tells the result then', async () => {
    const kept = heldLucky([1], 1);
    rollMountSave.mockResolvedValue({
      encounter: fight(prompt(), 2, kept),
      result: undefined,
      held: kept,
    });
    resources.answerHeldRoll.mockResolvedValue(
      heldAnswer({
        result: { case: 'mountSave', value: { encounter: fight(undefined, 3), result: held17 } },
      }),
    );
    const { fixture, el, state } = setup();
    button(el, 'Rolar no app').click();
    await flush(fixture);
    button(el, 'Rolar de novo').click();
    await flush(fixture);
    expect(resources.answerHeldRoll.mock.calls[0].slice(0, 2)).toEqual(['c', kept.holdId]);
    expect(state.encounter()?.revision).toBe(3);
    expect(plain(el.querySelector('.res .what')?.textContent)).toBe(
      '15 + 2 = 17: você continua montado.',
    );
  });

  it('asks for the d4 a refusal says the roll takes (an effect on the rider), and sends it with the d20', async () => {
    rollMountSave.mockRejectedValueOnce(
      new ConnectError(
        'the roll takes 1 more die(s): type their faces in extra_die_faces',
        Code.InvalidArgument,
      ),
    );
    rollMountSave.mockResolvedValue({
      encounter: fight(undefined, 2),
      result: held17,
      held: undefined,
    });
    const { fixture, el } = setup();
    button(el, 'Rolar no app').click();
    await flush(fixture);
    expect(plain(el.querySelector('[role=alert]')?.textContent)).toContain(
      'Este teste leva mais um d4: role-o e digite o resultado.',
    );
    fill(field(el, 'Face do d20 (1 a 20)'), '15');
    fixture.detectChanges();
    button(el, 'Confirmar').click();
    await flush(fixture);
    expect(plain(el.querySelector('[role=alert]')?.textContent)).toContain(
      'Digite o resultado de cada d4 antes de confirmar.',
    );
    expect(rollMountSave).toHaveBeenCalledTimes(1);
    fill(field(el, 'Resultado do d4 (Outra fonte)'), '3');
    fixture.detectChanges();
    button(el, 'Confirmar').click();
    await flush(fixture);
    expect(rollMountSave.mock.calls[1][3]).toEqual({ kind: 'typed', face: 15, extra: [3] });
  });

  it('says the save is no longer owed when the master rolled it, or it closed by itself, and only closes', async () => {
    const { fixture, el, state } = setup();
    state.apply(fight(undefined, 3));
    await flush(fixture);
    expect(plain(el.querySelector('[role=status]')?.textContent)).toBe(
      'Este teste não espera mais a sua rolagem: o mestre rolou, ou ele deixou de ser preciso.',
    );
    expect(button(el, 'Rolar no app')).toBeUndefined();
    button(el, 'Fechar').click();
    expect(closed).toHaveLength(1);
    expect(rollMountSave).not.toHaveBeenCalled();
  });

  it('speaks of the rider by name to the master, who may close it and roll later', () => {
    const { el } = setup(prompt({ dc: 10 }), { byMaster: true });
    expect(plain(el.querySelector('h2')?.textContent)).toBe('Tor cai da montaria?');
    expect(plain(el.querySelector('.what')?.textContent)).toContain(
      'Se passar, continua montado; se falhar, cai Derrubado num quadrado ao lado dele.',
    );
    expect(el.querySelector('.frame__close')).not.toBeNull();
  });
});
