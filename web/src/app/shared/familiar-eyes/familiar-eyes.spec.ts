import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';
import { textOf as text } from '../../core/format/text-testing';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
  FamiliarSightBlockedReason,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  FamiliarEyesClient,
  familiarName,
  familiarSightMessage,
} from '../../core/play/familiar-eyes';
import { FamiliarBand } from './familiar-band';
import { type FamiliarEyesData, FamiliarEyesSheet } from './familiar-eyes-sheet';

function blocked(reason: FamiliarSightBlockedReason): ConnectError {
  return new ConnectError('blocked', Code.FailedPrecondition, undefined, [
    {
      desc: EncounterBlockedSchema,
      value: create(EncounterBlockedSchema, {
        reason: EncounterBlockedReason.FAMILIAR_SIGHT_BLOCKED,
        familiarSightReason: reason,
      }),
    },
  ]);
}

describe('familiarSightMessage', () => {
  it('says each refusal by its reason, never by the message', () => {
    expect(familiarSightMessage(blocked(FamiliarSightBlockedReason.TOO_FAR), 'Nanquim')).toBe(
      'Nanquim está a mais de 30 m de você. Chegue mais perto para ver pelos olhos dele.',
    );
    expect(familiarSightMessage(blocked(FamiliarSightBlockedReason.NO_FAMILIAR))).toBe(
      'Você não tem um familiar agora.',
    );
    expect(
      familiarSightMessage(blocked(FamiliarSightBlockedReason.NOT_ON_MAP), 'Nanquim'),
    ).toContain('precisam estar no mapa');
    expect(familiarSightMessage(blocked(FamiliarSightBlockedReason.ALREADY_SEEING))).toBe(
      'Você já está vendo pelos olhos do familiar.',
    );
    expect(familiarSightMessage(blocked(FamiliarSightBlockedReason.NOT_SEEING))).toBe(
      'Você já voltou aos seus olhos.',
    );
    expect(familiarSightMessage(blocked(FamiliarSightBlockedReason.COMBAT_NOT_BEGUN))).toContain(
      'Espere a sua vez',
    );
  });

  it("falls back to the combat's own messages for the rest", () => {
    expect(familiarSightMessage(new ConnectError('x', Code.Unavailable))).toContain('servidor');
  });

  it('names the familiar, or says "o familiar"', () => {
    expect(familiarName('Nanquim')).toBe('Nanquim');
    expect(familiarName(null)).toBe('O familiar');
    expect(familiarName('  ')).toBe('O familiar');
  });
});

describe('"Ver pelos olhos do Nanquim?" (E9-04 state 3)', () => {
  const api = { start: vi.fn(), stop: vi.fn(), name: vi.fn(async () => 'Nanquim') };
  const close = vi.fn();
  const ref = { close, disableClose: false };

  function setup(data: Partial<FamiliarEyesData> = {}) {
    TestBed.configureTestingModule({
      providers: [
        { provide: FamiliarEyesClient, useValue: api },
        {
          provide: MAT_DIALOG_DATA,
          useValue: {
            campaignId: 'c1',
            characterId: 'p',
            characterName: 'Pensantus',
            familiarName: 'Nanquim',
            inCombat: false,
            ...data,
          },
        },
        { provide: MatDialogRef, useValue: ref },
      ],
    });
    const fixture = TestBed.createComponent(FamiliarEyesSheet);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }
  const buttons = (el: HTMLElement) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('.pair button'));

  beforeEach(() => {
    api.start.mockReset();
    close.mockReset();
    ref.disableClose = false;
  });

  it('asks the question with its cost: the character is blind and deaf, and what it takes in and out of a combat', () => {
    const out = setup();
    expect(text(out.el.querySelector('h2'))).toBe('Ver pelos olhos do Nanquim?');
    expect(text(out.el.querySelector('.what'))).toContain('Pensantus fica cego e surdo');
    expect(text(out.el.querySelector('.rule'))).toContain('até 30 m');
    expect(text(out.el.querySelector('.rule'))).toContain('Dura até você voltar aos seus olhos.');
    TestBed.resetTestingModule();
    const inCombat = setup({ inCombat: true });
    expect(text(inCombat.el.querySelector('.rule'))).toContain('gasta a sua ação');
  });

  it('has "Cancelar" first and "Ver pelos olhos" beside it, the same size', () => {
    const { el } = setup();
    expect(buttons(el).map((b) => text(b))).toEqual(['Cancelar', 'visibility Ver pelos olhos']);
    expect(el.querySelector('[data-initial-focus]')).toBe(buttons(el)[0]);
  });

  it('starts the sight with one key for the sheet, and closes saying it started', async () => {
    const { fixture, el } = setup();
    api.start.mockResolvedValue({});
    buttons(el)[1].click();
    await fixture.whenStable();
    expect(api.start).toHaveBeenCalledWith('c1', 'p', expect.stringMatching(/^[0-9a-f-]{36}$/));
    expect(close).toHaveBeenCalledWith(true);
  });

  it('does not let Esc or the backdrop close the question while the sight is being started', async () => {
    const { fixture, el } = setup();
    let answer: (value: unknown) => void = () => undefined;
    api.start.mockReturnValue(new Promise((resolve) => (answer = resolve)));
    buttons(el)[1].click();
    fixture.detectChanges();
    expect(ref.disableClose).toBe(true);
    answer({});
    await fixture.whenStable();
    fixture.detectChanges();
    expect(ref.disableClose).toBe(false);
  });

  it('says why it was refused and keeps the question open, trying again with the same key', async () => {
    const { fixture, el } = setup();
    api.start
      .mockRejectedValueOnce(blocked(FamiliarSightBlockedReason.TOO_FAR))
      .mockResolvedValue({});
    buttons(el)[1].click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(text(el.querySelector('[role="alert"]'))).toContain(
      'Nanquim está a mais de 30 m de você',
    );
    expect(close).not.toHaveBeenCalled();
    buttons(el)[1].click();
    await fixture.whenStable();
    expect(api.start.mock.calls[1][2]).toBe(api.start.mock.calls[0][2]);
    expect(close).toHaveBeenCalledWith(true);
  });

  it('cannot be closed by the ✕ while the sight is being started, and shows the answer', async () => {
    const { fixture, el } = setup();
    let answer!: () => void;
    api.start.mockReturnValue(new Promise<object>((resolve) => (answer = () => resolve({}))));
    buttons(el)[1].click();
    fixture.detectChanges();
    (fixture.componentInstance as unknown as { cancel(): void }).cancel();
    expect(close).not.toHaveBeenCalled();
    answer();
    await fixture.whenStable();
    expect(close).toHaveBeenCalledWith(true);
  });

  it('closes with false on "Cancelar"', () => {
    const { el } = setup();
    buttons(el)[0].click();
    expect(close).toHaveBeenCalledWith(false);
    expect(api.start).not.toHaveBeenCalled();
  });
});

describe('the band "Pensantus está cego e surdo" (E9-04)', () => {
  const api = { start: vi.fn(), stop: vi.fn(), name: vi.fn(async () => 'Nanquim') };

  function create(inputs: Record<string, unknown> = {}) {
    TestBed.configureTestingModule({ providers: [{ provide: FamiliarEyesClient, useValue: api }] });
    const fixture = TestBed.createComponent(FamiliarBand);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('characterId', 'p');
    fixture.componentRef.setInput('characterName', 'Pensantus');
    fixture.componentRef.setInput('creatureId', 'cr1');
    for (const [k, v] of Object.entries(inputs)) {
      fixture.componentRef.setInput(k, v);
    }
    fixture.detectChanges();
    return fixture;
  }

  beforeEach(() => {
    api.stop.mockReset();
    api.name.mockClear();
  });

  it('is a status with an icon and words, naming the familiar and the blind and deaf character, and the one filled button', async () => {
    const fixture = create();
    await fixture.whenStable();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const band = el.querySelector('[data-testid="familiar-band"]')!;
    expect(band.getAttribute('role')).toBe('status');
    expect(text(band)).toBe(
      'visibility Você está vendo pelos olhos do Nanquim. Pensantus está cego e surdo.',
    );
    const back = el.querySelector('button')!;
    expect(text(back)).toBe('arrow_back Voltar aos seus olhos');
    expect(back.classList.contains('mat-mdc-unelevated-button')).toBe(true);
  });

  it('in a combat, says until when', async () => {
    const fixture = create({ inCombat: true });
    await fixture.whenStable();
    fixture.detectChanges();
    expect(
      text((fixture.nativeElement as HTMLElement).querySelector('[data-testid="familiar-band"]')),
    ).toContain('Até o começo da sua próxima vez. Pensantus está cego e surdo.');
  });

  it("goes back to the character's own eyes, and says it", async () => {
    const fixture = create();
    api.stop.mockResolvedValue({});
    const stopped = vi.fn();
    fixture.componentInstance.stopped.subscribe(stopped);
    (fixture.nativeElement as HTMLElement).querySelector('button')!.click();
    await fixture.whenStable();
    expect(api.stop).toHaveBeenCalledWith('c1', 'p', expect.stringMatching(/^[0-9a-f-]{36}$/));
    expect(stopped).toHaveBeenCalled();
  });

  it('says why when it could not go back, and stays', async () => {
    const fixture = create();
    api.stop.mockRejectedValue(blocked(FamiliarSightBlockedReason.NOT_SEEING));
    const stopped = vi.fn();
    fixture.componentInstance.stopped.subscribe(stopped);
    (fixture.nativeElement as HTMLElement).querySelector('button')!.click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(text((fixture.nativeElement as HTMLElement).querySelector('[role="alert"]'))).toBe(
      'Você já voltou aos seus olhos.',
    );
    expect(stopped).not.toHaveBeenCalled();
  });

  it('says "o familiar" while the name is not known', async () => {
    api.name.mockResolvedValueOnce(null as never);
    const fixture = create();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(text((fixture.nativeElement as HTMLElement).querySelector('strong'))).toBe(
      'Você está vendo pelos olhos do familiar.',
    );
  });
});
