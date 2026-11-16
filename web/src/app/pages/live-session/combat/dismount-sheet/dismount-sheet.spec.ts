import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import { ReactionKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { CombatState as State } from '../../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { DismountSheet, type DismountSheetData } from './dismount-sheet';

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

const window = reactionWindow({
  id: 'w1',
  kind: ReactionKind.DISMOUNT,
  reactorId: 'tor',
  reactorLabel: 'Tor',
  reactorIsPlayer: true,
  forYou: true,
} as never);

const riding = [
  combatant({ id: 'tor', label: 'Tor', mine: true, mountedOnId: 'horse' }),
  combatant({ id: 'horse', label: 'Cavalo de guerra', riderId: 'tor' }),
  combatant({ id: 'z1', label: 'Zumbi 1' }),
];

describe('DismountSheet (#588, board 588-07)', () => {
  const answerReaction = vi.fn();
  const get = vi.fn();
  const close = vi.fn();

  function setup() {
    const state = new State();
    state.apply(
      encounter({
        round: 4,
        currentCombatantId: 'z1',
        combatants: riding,
        reactionWindows: [window],
      } as never),
    );
    const data: DismountSheetData = { campaignId: 'c', encounterId: 'enc', window, state };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close, disableClose: false } },
        { provide: CombatClient, useValue: fakeCombatClient({ answerReaction, get }) },
      ],
    });
    const fixture = TestBed.createComponent(DismountSheet);
    document.body.appendChild(fixture.nativeElement);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
    };
    const button = (text: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => flat(b).endsWith(text))!;
    return { el, settle, button, state };
  }

  beforeEach(() => {
    answerReaction.mockReset();
    get.mockReset();
    // By default the read after a refusal fails too: the screen keeps what it has.
    get.mockRejectedValue(new Error('offline'));
    close.mockReset();
  });
  afterEach(() => vi.useRealTimers());

  it('asks whether the rider uses the reaction, with the fall first and the dismount filled', async () => {
    const { el, settle } = setup();
    await settle();
    expect(flat(el.querySelector('h2'))).toBe('A montaria caiu');
    expect(flat(el.querySelector('.frame__sub'))).toContain('Reação · desmontar · Rodada 4');
    expect(flat(el.querySelector('.what'))).toBe(
      'O Cavalo de guerra caiu. Usar a reação para desmontar de pé?',
    );
    expect(flat(el.querySelector('.what b'))).toBe('O Cavalo de guerra caiu.');
    expect(flat(el.querySelector('.small'))).toBe(
      'Com a reação, você desce de pé num quadrado ao lado da montaria. Sem ela, cai Derrubado a até 1,5 m.',
    );
    expect(Array.from(el.querySelectorAll('.tag')).map((t) => flat(t))).toEqual([
      'Gasta a sua reação',
      'Volta no começo da sua vez',
    ]);
    expect(flat(el.querySelector('.wait'))).toBe(
      'hourglass_topA vez do Zumbi 1 espera a resposta. O mestre também pode responder.',
    );
    const answers = Array.from(el.querySelectorAll('.pair button'));
    expect(answers.map((b) => flat(b.querySelector('.mdc-button__label')))).toEqual([
      'Cair derrubado',
      'Desmontar de pé',
    ]);
    expect(answers[0].classList.contains('mat-mdc-outlined-button')).toBe(true);
    expect(answers[1].classList.contains('mat-mdc-unelevated-button')).toBe(true);
    // Neither answer is safe to take by a stray Enter on a sheet that opens by itself: the focus starts on the title.
    expect(el.querySelector('.pair [data-initial-focus]')).toBeNull();
    expect(document.activeElement).toBe(el.querySelector('h2'));
    // An answer is owed: the sheet has no "Fechar".
    expect(el.querySelector('.frame__close')).toBeNull();
  });

  it('never counts down to an automatic pass, however long the rider takes', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'Date'] });
    const { el, settle } = setup();
    await settle();
    vi.advanceTimersByTime(120_000);
    await settle();
    expect(flat(el)).not.toMatch(/passa sozinho|em \d+ s/);
    expect(el.querySelectorAll('.pair button').length).toBe(2);
    expect(answerReaction).not.toHaveBeenCalled();
  });

  it('"Desmontar de pé" answers USE, applies the combat and closes', async () => {
    answerReaction.mockResolvedValue({ encounter: encounter({ revision: 5 } as never) });
    const { settle, button, state } = setup();
    await settle();
    button('Desmontar de pé').click();
    await settle();
    expect(answerReaction).toHaveBeenCalledWith(
      'c',
      'enc',
      'w1',
      { use: true },
      expect.any(String),
    );
    expect(state.encounter()?.revision).toBe(5);
    expect(close).toHaveBeenCalled();
  });

  it('"Cair derrubado" answers PASS', async () => {
    answerReaction.mockResolvedValue({ encounter: encounter({ revision: 6 } as never) });
    const { settle, button } = setup();
    await settle();
    button('Cair derrubado').click();
    await settle();
    expect(answerReaction).toHaveBeenCalledWith(
      'c',
      'enc',
      'w1',
      { use: false },
      expect.any(String),
    );
    expect(close).toHaveBeenCalled();
  });

  it('keeps the question and says why when the server refuses, with the same key on the retry', async () => {
    answerReaction.mockRejectedValueOnce(new ConnectError('down', Code.Unavailable));
    answerReaction.mockResolvedValue({ encounter: encounter({ revision: 7 } as never) });
    const { el, settle, button } = setup();
    await settle();
    button('Desmontar de pé').click();
    await settle();
    expect(close).not.toHaveBeenCalled();
    expect(el.querySelector('[role=alert]')).not.toBeNull();
    expect(el.querySelectorAll('.pair button').length).toBe(2);
    button('Desmontar de pé').click();
    await settle();
    expect(answerReaction.mock.calls[1][4]).toBe(answerReaction.mock.calls[0][4]);
    expect(close).toHaveBeenCalled();
  });

  it('reads the combat again after a refusal that may mean the screen is stale: the window was answered elsewhere', async () => {
    answerReaction.mockRejectedValueOnce(new ConnectError('changed', Code.Aborted));
    get.mockResolvedValue(
      encounter({ revision: 9, round: 4, currentCombatantId: 'z1', combatants: riding } as never),
    );
    const { el, settle, button, state } = setup();
    await settle();
    button('Desmontar de pé').click();
    await settle();
    expect(get).toHaveBeenCalledWith('c');
    expect(state.encounter()?.revision).toBe(9);
    expect(el.querySelectorAll('.pair button')).toHaveLength(0);
    expect(button('Fechar')).toBeTruthy();
  });

  it('says how it ended when the window left the combat without the answer, and only closes', async () => {
    const { el, settle, button, state } = setup();
    await settle();
    state.apply(
      encounter({
        revision: 9,
        combatants: [
          combatant({ id: 'tor', label: 'Tor', mine: true, conditions: ['condition:prone'] }),
        ],
        reactionWindows: [],
      } as never),
    );
    await settle();
    expect(flat(el.querySelector('[role=status]'))).toBe(
      'A pergunta se fechou sem a sua resposta: você caiu Derrubado.',
    );
    expect(el.querySelector('.pair')).toBeNull();
    button('Fechar').click();
    expect(close).toHaveBeenCalled();
    expect(answerReaction).not.toHaveBeenCalled();
  });
});
