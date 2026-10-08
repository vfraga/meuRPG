import { ComponentFixture, TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  MarkMilestoneResponseSchema,
  XPAwardSchema,
  XPBlockedReason,
  XPBlockedSchema,
} from '../../../gen/meurpg/progression/v1/progression_pb';
import type { ExperienceRow } from '../../core/progression/experience-store';
import { ProgressionClient } from '../../core/progression/progression-client';
import { type MilestoneData, MilestoneSheet } from './milestone-sheet';

const row = (id: string, name: string, sub: string): ExperienceRow => ({
  id,
  name,
  playerUserId: '',
  sub,
  level: 3,
  xp: 0,
  nextLevelXp: 2700,
  canLevelUp: false,
  levelUpReason: 0,
});
const PARTY = [
  row('p1', 'Pensantus', 'Mago 3 · de Vinicius'),
  row('t1', 'Toren', 'Guerreiro 3'),
  row('b1', 'Brisa', 'Ladina 3'),
];

describe('MilestoneSheet (E7-08)', () => {
  const markMilestone = vi.fn();
  const close = vi.fn();

  beforeEach(() => {
    markMilestone
      .mockReset()
      .mockResolvedValue(
        create(MarkMilestoneResponseSchema, { award: create(XPAwardSchema, { id: 'a1' }) }),
      );
    close.mockReset();
  });

  function setup(rows = PARTY) {
    const data: MilestoneData = { campaignId: 'camp-1', campaignName: 'Sombras de Valdor', rows };
    TestBed.configureTestingModule({
      imports: [MilestoneSheet],
      providers: [
        { provide: ProgressionClient, useValue: { markMilestone } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
      ],
    });
    const fixture = TestBed.createComponent(MilestoneSheet);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const reason = (el: HTMLElement) => el.querySelector<HTMLInputElement>('app-xp-reason input')!;
  const boxes = (el: HTMLElement) =>
    Array.from(el.querySelectorAll<HTMLInputElement>('app-xp-recipients input[type="checkbox"]'));
  const primary = (el: HTMLElement) =>
    el.querySelector<HTMLButtonElement>('app-xp-actions .primary')!;
  const line = (el: HTMLElement) =>
    el
      .querySelector('.effect')
      ?.textContent?.replace(/[ \t\r\n]+/g, ' ')
      .trim();
  function type(fixture: ComponentFixture<MilestoneSheet>, value: string) {
    const input = fixture.nativeElement.querySelector('app-xp-reason input') as HTMLInputElement;
    input.value = value;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
  }

  it('says there is no XP, shows no XP number, and asks "O que aconteceu"', () => {
    const { el } = setup();
    expect(el.querySelector('h2')?.textContent).toBe('Registrar marco');
    expect(el.textContent).toContain(
      'Sombras de Valdor · campanha por marcos: não há XP para contar.',
    );
    expect(el.textContent).toContain('O que aconteceu');
    expect(el.textContent).not.toMatch(/\d+\s*XP/);
    expect(reason(el).hasAttribute('data-initial-focus')).toBe(true);
    expect(el.querySelector('app-fiction-notice')).not.toBeNull();
  });

  it('starts with everyone checked and says what the mark does, live', () => {
    const { fixture, el } = setup();
    expect(boxes(el).every((b) => b.checked)).toBe(true);
    expect(line(el)).toContain('3 personagens podem subir de nível');
    expect(line(el)).toContain('O aviso some quando o nível sobe na ficha.');
    expect(el.querySelector('.effect')?.getAttribute('role')).toBe('status');

    boxes(el)[1].click();
    boxes(el)[2].click();
    fixture.detectChanges();
    expect(line(el)).toContain('1 personagem pode subir de nível');
  });

  it('waits with nobody checked, and says why', () => {
    const { fixture, el } = setup();
    type(fixture, 'Marco: a ponte do rio foi salva');
    boxes(el).forEach((b) => b.click());
    fixture.detectChanges();

    expect(el.querySelector('app-xp-actions .reason')?.textContent?.trim()).toBe(
      'Marque pelo menos um personagem',
    );
    expect(primary(el).getAttribute('aria-disabled')).toBe('true');
    expect(line(el)).toBe('');
  });

  it('shows the error when the reason is left empty, and goes to it when pressed', async () => {
    const { fixture, el } = setup();
    expect(el.querySelector('mat-error')).toBeNull();
    primary(el).click();
    await fixture.whenStable();
    fixture.detectChanges();

    expect(markMilestone).not.toHaveBeenCalled();
    expect(el.querySelector('mat-error')?.textContent).toContain('Escreva o que aconteceu.');
    expect(document.activeElement).toBe(reason(el));
  });

  it('marks the ones checked, and closes with the award', async () => {
    const { fixture, el } = setup();
    type(fixture, '  Marco: a ponte do rio foi salva ');
    boxes(el)[2].click();
    fixture.detectChanges();
    primary(el).click();
    await fixture.whenStable();

    expect(markMilestone).toHaveBeenCalledWith(
      'camp-1',
      'Marco: a ponte do rio foi salva',
      ['p1', 't1'],
      expect.stringMatching(/^[0-9a-f-]{36}$/),
    );
    expect(close).toHaveBeenCalledWith(expect.objectContaining({ id: 'a1' }));
  });

  it('says why in words when the server refuses, and keeps the key for a retry', async () => {
    markMilestone.mockRejectedValueOnce(
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: XPBlockedSchema,
          value: { reason: XPBlockedReason.XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE },
        },
      ]),
    );
    const { fixture, el } = setup();
    type(fixture, 'Marco');
    primary(el).click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('morreu ou saiu da campanha');
    expect(close).not.toHaveBeenCalled();

    primary(el).click();
    await fixture.whenStable();
    expect(markMilestone.mock.calls[1][3]).toBe(markMilestone.mock.calls[0][3]);
  });

  it('drops a character that can no longer receive, so the retry can go', async () => {
    markMilestone.mockRejectedValueOnce(
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: XPBlockedSchema,
          value: {
            reason: XPBlockedReason.XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE,
            characterId: 't1',
          },
        },
      ]),
    );
    const { fixture, el } = setup();
    type(fixture, 'Marco');
    primary(el).click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(markMilestone.mock.calls[0][2]).toEqual(['p1', 't1', 'b1']);
    expect(el.querySelector('[role="alert"]')).not.toBeNull();
    expect(boxes(el)).toHaveLength(2);

    primary(el).click();
    await fixture.whenStable();
    expect(markMilestone.mock.calls[1][2]).toEqual(['p1', 'b1']);
  });
});
