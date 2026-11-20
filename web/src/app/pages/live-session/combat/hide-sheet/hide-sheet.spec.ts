import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantSide,
  HeldRollKind,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  CheckOptionSchema,
  ContestBlockedReason,
  ContestBlockedSchema,
  HideAttemptStatus,
  RollModeKind,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { CombatState } from '../../../../core/combat/combat-state';
import { ContestClient } from '../../../../core/combat/contest-client';
import { ContestState } from '../../../../core/combat/contest-state';
import {
  FakeContestClient,
  checkRoll,
  hideAttempt,
  textOf,
} from '../../../../core/combat/contest-testing';
import { heldDie } from '../../../../core/held-rolls/held-roll-testing';
import { HideSheet, type HideSheetData } from './hide-sheet';

function table() {
  return encounter({
    combatants: [
      combatant({ id: 'b', label: 'Brisa', kind: CombatantKind.PLAYER, side: CombatantSide.PARTY }),
      combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER, side: CombatantSide.PARTY }),
    ],
  });
}

describe('HideSheet (board W7-Xc 8)', () => {
  let api: FakeContestClient;
  let close: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    Element.prototype.scrollTo = vi.fn() as never;
  });

  function setup(over: Partial<HideSheetData> = {}) {
    api = new FakeContestClient();
    api.encounterAnswer = table();
    close = vi.fn();
    const state = new CombatState();
    state.encounter.set(table());
    const contests = new ContestState();
    const data: HideSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      combatantId: 'b',
      actionKey: 'standard:hide',
      economy: 'Ação',
      option: create(CheckOptionSchema, { modifier: 7, known: true, mode: RollModeKind.NORMAL }),
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state,
      contests,
      ...over,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: ContestClient, useValue: api.as() },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close, disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(HideSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        await fixture.whenStable();
        fixture.detectChanges();
      }
    };
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.includes(name),
      );
    return { fixture, el, button, settle, text: () => textOf(el), contests };
  }

  it('says what the check is, who confirms it, and the player’s own bonus', () => {
    const { el, text, button } = setup();
    expect(textOf(el.querySelector('h2')!)).toBe('Esconder-se');
    expect(text()).toContain('Ação · Destreza (Furtividade)');
    expect(text()).toContain(
      'O mestre confirma se há onde se esconder. Você não se esconde de quem vê você claramente.',
    );
    expect(text()).toContain('Seu teste de Furtividade: +7.');
    expect(button('Rolar no app')).toBeTruthy();
    expect(button('Digitar o resultado')).toBeTruthy();
    expect(Array.from(el.querySelectorAll('.steps__name')).map((n) => n.textContent)).toEqual([
      'Teste',
      'Resultado',
    ]);
  });

  it('follows the campaign’s dice mode: forced app offers only "Rolar no app" and sends no typed value', async () => {
    const { el, button, settle } = setup({ diceMode: DiceMode.APP });
    expect(button('Digitar o resultado')).toBeUndefined();
    expect(el.querySelector('input[type="text"]')).toBeNull();
    button('Rolar no app')!.click();
    await settle();
    expect(api.hides.map((h) => h.die)).toEqual([{ inApp: true }]);
  });

  it('follows the campaign’s dice mode: forced physical offers only the typed entry and sends the face, never the total', async () => {
    const { el, button, settle } = setup({ diceMode: DiceMode.PHYSICAL });
    expect(button('Rolar no app')).toBeUndefined();
    const field = el.querySelector<HTMLInputElement>('input[type="text"]')!;
    field.value = '12';
    field.dispatchEvent(new Event('input'));
    await settle();
    button('Confirmar')!.click();
    await settle();
    // The +7 of Furtividade is not added: 19 is never sent.
    expect(api.hides.map((h) => h.die)).toEqual([{ faces: [12] }]);
  });

  it('says "Ação bônus" for the rogue’s Cunning Action', () => {
    const { text } = setup({ economy: 'Ação bônus', combatantId: 't' });
    expect(text()).toContain('Ação bônus · Destreza (Furtividade)');
  });

  it('rolls with the action key and waits for the master, saying nothing of who noticed', async () => {
    const { button, settle, text, el } = setup();
    api.attempt = hideAttempt({
      status: HideAttemptStatus.PENDING,
      roll: checkRoll({ skill: 0, skillKey: 'skill:stealth', faces: [12], modifier: 7, total: 19 }),
    });
    button('Rolar no app')!.click();
    await settle();
    expect(api.hides).toHaveLength(1);
    expect(api.hides[0]).toMatchObject({ actionKey: 'standard:hide', die: { inApp: true } });
    expect(api.hides[0].key).toMatch(/^[0-9a-f-]{36}$/);
    expect(el.querySelector('.roll__box')?.textContent).toBe('12');
    expect(el.querySelector('.roll__total')?.textContent).toBe('19');
    expect(textOf(el.querySelector('.roll__formula')!)).toBe('1d20 (12) + 7 · Furtividade');
    expect(textOf(el.querySelector('[data-testid="hide-wait"]')!)).toBe(
      'Esperando o mestre. Ele confirma se há onde se esconder.',
    );
    expect(text()).not.toMatch(/Percepção|passiva|notou|vê claramente: /);
    expect(button('Fechar a folha')).toBeTruthy();
  });

  it('asks the question when the d20 is kept for one and makes no attempt until it is answered', async () => {
    const { el, button, settle } = setup();
    api.held = heldDie(11, 7, 8, { kind: HeldRollKind.HIDE });
    button('Rolar no app')!.click();
    await settle();
    expect(api.hides).toHaveLength(1);
    expect(el.querySelector('app-held-roll-prompt')).not.toBeNull();
    expect(button('Rolar no app')).toBeUndefined();
  });

  it('says only "Você está escondido." once the master applied it, and when it ends', async () => {
    const { contests, fixture, button, settle, text } = setup();
    api.attempt = hideAttempt({
      status: HideAttemptStatus.PENDING,
      roll: checkRoll({ faces: [12], modifier: 7, total: 19 }),
    });
    button('Rolar no app')!.click();
    await settle();
    contests.applyAttempt(
      hideAttempt({
        status: HideAttemptStatus.APPLIED,
        roll: checkRoll({ faces: [12], modifier: 7, total: 19 }),
      }),
    );
    fixture.detectChanges();
    await settle();
    expect(text()).toContain('Você está escondido.');
    expect(text()).toContain(
      'Você sai do esconderijo quando atacar, conjurar ou quando alguém perceber você. O mestre decide o que cada criatura percebe.',
    );
    expect(document.activeElement?.textContent).toContain('Fechar');
  });

  it('reads the master’s refusal in his words, the action spent (board 8c)', async () => {
    const { contests, fixture, button, settle, text, el } = setup();
    button('Rolar no app')!.click();
    await settle();
    contests.applyAttempt(
      hideAttempt({
        status: HideAttemptStatus.REFUSED,
        refusal: 'Alguém vê você claramente: não dá para se esconder agora.',
      }),
    );
    fixture.detectChanges();
    await settle();
    expect(text()).toContain('Alguém vê você claramente: não dá para se esconder agora.');
    expect(el.querySelector('.mr-notice--danger b')?.textContent).toBe(
      'Alguém vê você claramente:',
    );
  });

  it('hands the page the attempt when the sheet is hidden while it waits', async () => {
    const { button, settle } = setup();
    api.attempt = hideAttempt({ id: 'hd7', status: HideAttemptStatus.PENDING });
    button('Rolar no app')!.click();
    await settle();
    button('Fechar a folha')!.click();
    expect(close).toHaveBeenCalledWith({ attemptId: 'hd7' });
  });

  it('says why a Hide is refused by the server, and keeps the roll form', async () => {
    const { button, settle, text } = setup();
    api.error = new ConnectError('x', Code.FailedPrecondition, undefined, [
      {
        desc: ContestBlockedSchema,
        value: create(ContestBlockedSchema, { reason: ContestBlockedReason.SURPRISED }),
      },
    ]);
    button('Rolar no app')!.click();
    await settle();
    expect(text()).toContain('Surpresa: não se move, não age e não reage até o fim do turno.');
    expect(button('Rolar no app')).toBeTruthy();
  });

  it('opens straight on an attempt that already exists', () => {
    const contests = new ContestState();
    contests.applyAttempt(hideAttempt({ id: 'hd1', status: HideAttemptStatus.APPLIED }));
    const { text } = setup({ attemptId: 'hd1', contests });
    expect(text()).toContain('Você está escondido.');
  });
});
