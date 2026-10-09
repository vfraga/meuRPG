import { ApplicationRef } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  type Milestone,
  MilestoneSchema,
  XPBlockedReason,
  XPBlockedSchema,
} from '../../../../gen/meurpg/progression/v1/progression_pb';
import { MilestonesStore } from '../../../core/progression/milestones-store';
import { ProgressionClient } from '../../../core/progression/progression-client';
import { PlannedMilestones } from './planned-milestones';

const ms = (id: string, text: string): Milestone => create(MilestoneSchema, { id, text });
const three = [
  ms('a', 'Salvar o mercador'),
  ms('b', 'Chegar ao Vale Seco'),
  ms('c', 'Derrotar o Barão Ivo'),
];

describe('PlannedMilestones (E8-14)', () => {
  const api = {
    addMilestone: vi.fn(),
    updateMilestone: vi.fn(),
    moveMilestone: vi.fn(),
    removeMilestone: vi.fn(),
    listMilestones: vi.fn(),
  };

  async function setup(list: Milestone[] = three, total = list.length) {
    TestBed.configureTestingModule({
      providers: [MilestonesStore, { provide: ProgressionClient, useValue: api }],
    });
    const fixture = TestBed.createComponent(PlannedMilestones);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('milestones', list);
    fixture.componentRef.setInput('total', total);
    document.body.appendChild(fixture.nativeElement); // focus needs a document
    fixture.detectChanges();
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      fixture.detectChanges();
      await fixture.whenStable();
      TestBed.inject(ApplicationRef).tick(); // runs afterNextRender, where the focus moves
      fixture.detectChanges();
      await fixture.whenStable();
    };
    return { fixture, el, settle, store: TestBed.inject(MilestonesStore) };
  }
  const btn = (el: HTMLElement, sel: string) => el.querySelector<HTMLButtonElement>(sel)!;

  beforeEach(() => {
    Object.values(api).forEach((f) => f.mockReset());
  });

  it('lists the milestones with the count only the master reads, and a labelled button on each', async () => {
    const { el } = await setup();
    expect(el.textContent).toContain('3 marcos · só você vê');
    const reach = Array.from(el.querySelectorAll<HTMLButtonElement>('[data-act="reach"]'));
    expect(reach.map((b) => b.getAttribute('aria-label'))).toEqual([
      'Marcar “Salvar o mercador” como alcançado',
      'Marcar “Chegar ao Vale Seco” como alcançado',
      'Marcar “Derrotar o Barão Ivo” como alcançado',
    ]);
    expect(el.querySelector('[data-id="a"][data-act="up"]')?.getAttribute('aria-disabled')).toBe(
      'true',
    );
    expect(el.querySelector('[data-id="c"][data-act="down"]')?.getAttribute('aria-disabled')).toBe(
      'true',
    );
    expect(el.querySelector('[data-id="b"][data-act="up"]')?.getAttribute('aria-label')).toBe(
      'Subir Chegar ao Vale Seco',
    );
  });

  it('invites the first milestone when the list is empty', async () => {
    const { el } = await setup([]);
    expect(el.textContent).toContain('Nenhum marco planejado');
    expect(el.textContent).toContain('Só você vê esta lista.');
    expect(el.querySelector('[data-add]')).not.toBeNull();
  });

  it('opens the field in place with the focus on it, and an empty name says so under the field', async () => {
    const { el, settle } = await setup();
    btn(el, '[data-add]').click();
    await settle();
    expect(el.querySelector('[data-add]')).toBeNull(); // the button goes while the field is open
    const input = el.querySelector<HTMLInputElement>('app-milestone-name-form input')!;
    expect(document.activeElement).toBe(input);

    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(new Event('submit'));
    await settle();
    expect(el.querySelector('#name-error')?.textContent).toContain(
      'Escreva o nome do marco. Ele pode ter até 120 caracteres.',
    );
    expect(document.activeElement).toBe(input);
    expect(api.addMilestone).not.toHaveBeenCalled();
  });

  it('adds the milestone, adopts the list and puts the focus back on "Adicionar marco"', async () => {
    const { el, settle, store } = await setup();
    api.addMilestone.mockResolvedValue({ milestones: [...three, ms('d', 'Fechar o portal')] });
    btn(el, '[data-add]').click();
    await settle();
    const input = el.querySelector<HTMLInputElement>('app-milestone-name-form input')!;
    input.value = '  Fechar o portal ';
    input.dispatchEvent(new Event('input'));
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(new Event('submit'));
    await settle();
    expect(api.addMilestone).toHaveBeenCalledWith('c1', 'Fechar o portal', expect.any(String));
    expect(store.list()).toHaveLength(4);
    expect(document.activeElement).toBe(el.querySelector('[data-add]'));
  });

  it('keeps the field open and says why when the server refuses', async () => {
    const { el, settle } = await setup();
    api.addMilestone.mockRejectedValue(new Error('down'));
    api.listMilestones.mockResolvedValue({ milestones: three });
    btn(el, '[data-add]').click();
    await settle();
    const input = el.querySelector<HTMLInputElement>('app-milestone-name-form input')!;
    input.value = 'Fechar o portal';
    input.dispatchEvent(new Event('input'));
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(new Event('submit'));
    await settle();
    expect(el.querySelector('app-milestone-name-form')).not.toBeNull();
    expect(el.querySelector('#name-error')?.textContent).toContain('servidor');
  });

  it('sends the same idempotency key when the same milestone is added again after a failure', async () => {
    const { el, settle } = await setup();
    api.addMilestone.mockRejectedValueOnce(new Error('down'));
    api.addMilestone.mockResolvedValue({ milestones: [...three, ms('d', 'Fechar o portal')] });
    api.listMilestones.mockResolvedValue({ milestones: three });
    btn(el, '[data-add]').click();
    await settle();
    const input = el.querySelector<HTMLInputElement>('app-milestone-name-form input')!;
    input.value = 'Fechar o portal';
    input.dispatchEvent(new Event('input'));
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(new Event('submit'));
    await settle();
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(new Event('submit'));
    await settle();
    const [first, retry] = api.addMilestone.mock.calls.map((c) => c[2] as string);
    expect(retry).toBe(first);
  });

  it('moves a milestone and keeps the focus on the button that was pressed', async () => {
    const { el, settle } = await setup();
    api.moveMilestone.mockResolvedValue({ milestones: [three[1], three[0], three[2]] });
    btn(el, '[data-id="b"][data-act="up"]').click();
    await settle();
    expect(api.moveMilestone).toHaveBeenCalledWith('c1', 'b', 'up');
    expect(document.activeElement).toBe(el.querySelector('[data-id="b"][data-act="up"]'));
  });

  it('does nothing at the edges, and keeps them in the tab order', async () => {
    const { el, settle } = await setup();
    btn(el, '[data-id="a"][data-act="up"]').click();
    await settle();
    expect(api.moveMilestone).not.toHaveBeenCalled();
    expect(btn(el, '[data-id="a"][data-act="up"]').disabled).toBe(false);
  });

  it('asks in place before removing, with "Voltar" in focus, and "Voltar" returns to the bin', async () => {
    const { el, settle } = await setup();
    btn(el, '[data-id="b"][data-act="remove"]').click();
    await settle();
    const ask = el.querySelector('app-milestone-ask')!;
    expect(ask.textContent).toContain('Remover o marco Chegar ao Vale Seco?');
    expect(ask.textContent).toContain('Ele some da lista. Marcos já alcançados não mudam.');
    expect(document.activeElement?.textContent?.trim()).toBe('Voltar');
    expect(api.removeMilestone).not.toHaveBeenCalled();

    (document.activeElement as HTMLElement).click();
    await settle();
    expect(el.querySelector('app-milestone-ask')).toBeNull();
    expect(document.activeElement).toBe(el.querySelector('[data-id="b"][data-act="remove"]'));
  });

  it('removes, and the focus goes to the row that took its place', async () => {
    const { el, settle } = await setup();
    api.removeMilestone.mockResolvedValue({ milestones: [three[0], three[2]] });
    btn(el, '[data-id="b"][data-act="remove"]').click();
    await settle();
    const go = Array.from(el.querySelectorAll<HTMLButtonElement>('app-milestone-ask button')).find(
      (b) => b.textContent?.includes('Remover'),
    )!;
    go.click();
    await settle();
    expect(api.removeMilestone).toHaveBeenCalledWith('c1', 'b');
    expect(document.activeElement).toBe(el.querySelector('[data-id="c"][data-act="reach"]'));
  });

  it('does not offer to remove a milestone the list says has history, other rows keep it', async () => {
    const withHistory = create(MilestoneSchema, {
      id: 'b',
      text: 'Chegar ao Vale Seco',
      hasHistory: true,
    });
    const { el } = await setup([three[0], withHistory, three[2]]);
    expect(el.querySelector('[data-id="b"][data-act="remove"]')).toBeNull();
    expect(el.querySelector('[data-id="a"][data-act="remove"]')).not.toBeNull();
    expect(el.querySelector('[data-id="c"][data-act="remove"]')).not.toBeNull();
  });

  it('says why a milestone reached once and undone stays, and stops offering to remove it', async () => {
    const { el, settle } = await setup();
    api.removeMilestone.mockRejectedValue(
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: XPBlockedSchema,
          value: { reason: XPBlockedReason.XP_BLOCKED_REASON_MILESTONE_HAS_HISTORY },
        },
      ]),
    );
    api.listMilestones.mockResolvedValue({ milestones: three });
    btn(el, '[data-id="b"][data-act="remove"]').click();
    await settle();
    Array.from(el.querySelectorAll<HTMLButtonElement>('app-milestone-ask button'))
      .find((b) => b.textContent?.includes('Remover'))!
      .click();
    await settle();
    expect(el.querySelector('app-milestone-ask')).toBeNull();
    expect(el.querySelector('.problem')?.textContent).toContain(
      'O marco “Chegar ao Vale Seco” já foi alcançado e depois desfeito: o histórico de XP guarda isso, então ele não pode ser removido. Ele continua na lista de planejados.',
    );
    expect(el.querySelector('[data-id="b"][data-act="remove"]')).toBeNull();
    // The row keeps its four places, so the other tools stay under the ones of the rows around it.
    const tools = el.querySelector('[data-id="b"][data-act="edit"]')!.parentElement!;
    expect(tools.querySelectorAll('.tool')).toHaveLength(4);
    expect(tools.querySelector('.tool--gap')?.getAttribute('aria-hidden')).toBe('true');
    // Positive control: the others still offer it.
    expect(el.querySelector('[data-id="a"][data-act="remove"]')).not.toBeNull();
  });

  it('edits the name in place of the row, filled, and saves it', async () => {
    const { el, settle } = await setup();
    api.updateMilestone.mockResolvedValue({
      milestones: [ms('a', 'Salvar a mercadora'), three[1], three[2]],
    });
    btn(el, '[data-id="a"][data-act="edit"]').click();
    await settle();
    const input = el.querySelector<HTMLInputElement>('app-milestone-name-form input')!;
    expect(input.value).toBe('Salvar o mercador');
    expect(el.querySelector('app-milestone-name-form')?.textContent).toContain('Editar marco');
    input.value = 'Salvar a mercadora';
    input.dispatchEvent(new Event('input'));
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(new Event('submit'));
    await settle();
    expect(api.updateMilestone).toHaveBeenCalledWith('c1', 'a', 'Salvar a mercadora');
    expect(document.activeElement).toBe(el.querySelector('[data-id="a"][data-act="edit"]'));
  });

  it('stops offering to add at 100 milestones', async () => {
    const { el } = await setup(three, 100);
    expect(el.querySelector('[data-add]')).toBeNull();
    expect(el.textContent).toContain('já tem 100 marcos');
  });
});
