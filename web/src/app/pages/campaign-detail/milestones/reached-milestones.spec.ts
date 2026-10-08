import { ApplicationRef } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  type Milestone,
  MilestoneSchema,
  XPAwardSchema,
  XPBlockedReason,
  XPBlockedSchema,
} from '../../../../gen/meurpg/progression/v1/progression_pb';
import { ProgressionClient } from '../../../core/progression/progression-client';
import { ReachedMilestones } from './reached-milestones';

const award = (id: string, names: string[], canUndo: boolean) =>
  create(XPAwardSchema, {
    id,
    givenByDisplayName: 'Samuel',
    createdAt: timestampFromDate(new Date(2026, 9, 3, 22, 5)),
    canUndo,
    shares: names.map((n) => ({ characterId: n, characterName: n })),
  });
const vale: Milestone = create(MilestoneSchema, {
  id: 'm1',
  text: 'Chegar ao Vale Seco',
  reached: true,
  reachedAt: timestampFromDate(new Date(2026, 9, 3, 22, 5)),
  marks: [award('a1', ['Pensantus', 'Toren'], true)],
});
const old: Milestone = create(MilestoneSchema, {
  id: 'm0',
  text: 'Salvar o mercador',
  reached: true,
  reachedAt: timestampFromDate(new Date(2026, 9, 2, 20, 0)),
  marks: [award('a0', ['Pensantus'], false)],
});

describe('ReachedMilestones (E8-14)', () => {
  const api = { undoLast: vi.fn() };

  async function setup(isMaster: boolean, list = [vale, old], giveable = new Set(['m1'])) {
    TestBed.configureTestingModule({ providers: [{ provide: ProgressionClient, useValue: api }] });
    const fixture = TestBed.createComponent(ReachedMilestones);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('milestones', list);
    fixture.componentRef.setInput('isMaster', isMaster);
    fixture.componentRef.setInput('giveable', giveable);
    document.body.appendChild(fixture.nativeElement);
    fixture.detectChanges();
    await fixture.whenStable();
    const settle = async () => {
      fixture.detectChanges();
      await fixture.whenStable();
      TestBed.inject(ApplicationRef).tick(); // runs afterNextRender, where the focus moves
      fixture.detectChanges();
    };
    return { fixture, el: fixture.nativeElement as HTMLElement, settle, undone: [] as string[] };
  }
  const text = (el: HTMLElement) => el.textContent!.replace(/\s+/g, ' ');

  beforeEach(() => {
    api.undoLast = vi.fn();
  });

  it('shows the master when, who marked, "Dar a mais alguém" and "Desfazer" on the last mark only', async () => {
    const { el } = await setup(true);
    expect(text(el)).toContain('03/10 às 22:05');
    expect(text(el)).toContain('Samuel marcou Pensantus e Toren');
    expect(el.querySelectorAll('.item__give')).toHaveLength(1);
    expect(el.querySelector('.item__give')?.getAttribute('aria-label')).toBe(
      'Dar Chegar ao Vale Seco a mais alguém',
    );
    expect(el.querySelectorAll('[data-undo]')).toHaveLength(1);
    expect(el.querySelector('[data-undo]')?.getAttribute('aria-label')).toBe(
      'Desfazer Chegar ao Vale Seco',
    );
  });

  it('shows a player who levelled and nothing to press', async () => {
    const { el } = await setup(false);
    expect(text(el)).toContain('Subiram de nível: Pensantus e Toren');
    expect(text(el)).not.toContain('Samuel');
    expect(el.querySelectorAll('button')).toHaveLength(0);
  });

  it('asks in place, "Voltar" first and in focus, and undoes with the last award as expected', async () => {
    const { fixture, el, settle, undone } = await setup(true);
    fixture.componentInstance.undone.subscribe((m) => undone.push(m));
    el.querySelector<HTMLButtonElement>('[data-undo]')!.click();
    await settle();
    expect(text(el.querySelector('app-milestone-ask')!)).toContain(
      'Desfazer o marco “Chegar ao Vale Seco”?',
    );
    expect(text(el)).toContain('volta para “Marcos planejados”');
    expect(document.activeElement?.textContent?.trim()).toBe('Voltar');

    api.undoLast.mockResolvedValue({});
    const go = Array.from(el.querySelectorAll<HTMLButtonElement>('app-milestone-ask button')).find(
      (b) => b.textContent?.includes('Desfazer marco'),
    )!;
    go.click();
    await settle();
    expect(api.undoLast).toHaveBeenCalledWith('c1', 'a1', expect.any(String));
    expect(undone).toEqual(['Marco desfeito.']);
  });

  it('a stale screen changes nothing and says so', async () => {
    const { el, settle } = await setup(true);
    // A plain function: vitest reports a rejection that a mock records, even when the code handles it.
    api.undoLast = (() =>
      new Promise((_, reject) => reject(new ConnectError('x', Code.Aborted)))) as never;
    el.querySelector<HTMLButtonElement>('[data-undo]')!.click();
    await settle();
    Array.from(el.querySelectorAll<HTMLButtonElement>('app-milestone-ask button'))
      .find((b) => b.textContent?.includes('Desfazer marco'))!
      .click();
    await settle();
    expect(text(el)).toContain('A lista mudou enquanto você olhava');
    expect(el.querySelector('app-milestone-ask')).toBeNull();
  });

  it('tells the host to read the list again when there is no mark left to undo', async () => {
    const { fixture, el, settle, undone } = await setup(true);
    fixture.componentInstance.undone.subscribe((m) => undone.push(m));
    api.undoLast = (() =>
      new Promise((_, reject) =>
        reject(
          new ConnectError('x', Code.FailedPrecondition, undefined, [
            {
              desc: XPBlockedSchema,
              value: { reason: XPBlockedReason.XP_BLOCKED_REASON_NOTHING_TO_UNDO },
            },
          ]),
        ),
      )) as never;
    el.querySelector<HTMLButtonElement>('[data-undo]')!.click();
    await settle();
    Array.from(el.querySelectorAll<HTMLButtonElement>('app-milestone-ask button'))
      .find((b) => b.textContent?.includes('Desfazer marco'))!
      .click();
    await settle();
    expect(undone).toEqual(['']);
    expect(text(el)).toContain('A tela foi atualizada');
  });
});
