import { ComponentFixture, TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError } from '@connectrpc/connect';

import { type SceneClue, SceneClueSchema } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient } from '../../../core/maps/maps-testing';
import type { CluePlayer } from '../../../core/maps/scene-clues';
import { ClueList } from './clue-list';

function clue(id: string, text: string, to: string[] = []): SceneClue {
  return create(SceneClueSchema, {
    id,
    text,
    revealedTo: to.map((characterId) => ({
      characterId,
      characterName: characterId,
      revealedAt: timestampFromDate(new Date()),
    })),
  });
}

const PLAYERS: CluePlayer[] = [
  { id: 'Pensantus', name: 'Pensantus', playerName: 'Vinicius' },
  { id: 'Brisa', name: 'Brisa', playerName: 'Lia' },
];

const THREE = [
  clue('k1', 'Um brasão de lobo queimado na lona da carroça.', ['Pensantus', 'Brisa']),
  clue('k2', 'Rastros de três goblins e de botas pesadas, rumo ao norte.'),
  clue('k3', 'Uma carta rasgada: “…entregar no Vale Seco antes da lua cheia.”', ['Brisa']),
];

describe('ClueList', () => {
  let api: FakeMapsClient;
  let fixture: ComponentFixture<ClueList>;
  let el: HTMLElement;
  let emitted: (readonly SceneClue[])[];

  function setup(clues: SceneClue[]) {
    api = new FakeMapsClient();
    api.sceneClues = clues;
    emitted = [];
    TestBed.configureTestingModule({ providers: [{ provide: MapsClient, useValue: api }] });
    Element.prototype.scrollIntoView = vi.fn();
    fixture = TestBed.createComponent(ClueList);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('mapId', 'm1');
    fixture.componentRef.setInput('pointId', 'p1');
    fixture.componentRef.setInput('clues', clues);
    fixture.componentRef.setInput('players', PLAYERS);
    fixture.componentInstance.cluesChange.subscribe((list) => {
      emitted.push(list);
      // The panel puts the answer on the point, and the point comes back as the input.
      fixture.componentRef.setInput('clues', list);
    });
    fixture.detectChanges();
    el = fixture.nativeElement;
  }

  async function settle(): Promise<void> {
    for (let i = 0; i < 4; i++) {
      await fixture.whenStable();
      await new Promise((r) => setTimeout(r));
      fixture.detectChanges();
    }
  }

  const rows = () => Array.from(el.querySelectorAll('.cl__row'));
  const control = (id: string, which: 'text' | 'up' | 'down' | 'remove') =>
    el.querySelector<HTMLButtonElement>(`[data-clue="${id}"][data-control="${which}"]`)!;
  const button = (name: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      b.textContent?.includes(name),
    )!;
  const flat = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();
  function type(value: string): HTMLTextAreaElement {
    const field = el.querySelector<HTMLTextAreaElement>('textarea')!;
    field.value = value;
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    return field;
  }

  it('lists the clues in order with a count and who has each one', () => {
    setup([...THREE]);
    expect(flat(el.querySelector('.cl__count'))).toBe('3 de 30');
    expect(
      rows().map((r) =>
        r.querySelector('.cl__text')?.textContent?.replace('Editar a pista', '').trim(),
      ),
    ).toHaveLength(3);
    expect(rows().map((r) => flat(r.querySelector('.cl__who')))).toEqual([
      'groupsTodos',
      'visibility_offNinguém ainda',
      'personSó Brisa',
    ]);
  });

  it('says in the first line what the section is for', () => {
    setup([...THREE]);
    expect(flat(el.querySelector('.cl__privacy'))).toContain(
      'Os jogadores só recebem uma pista quando você a revela, na sessão.',
    );
  });

  it('names every control by its clue, and quiets the first ↑ and the last ↓', () => {
    setup([...THREE]);
    expect(control('k2', 'up').getAttribute('aria-label')).toBe('Subir a pista 2');
    expect(control('k2', 'down').getAttribute('aria-label')).toBe('Descer a pista 2');
    expect(control('k2', 'remove').getAttribute('aria-label')).toBe('Remover a pista 2');
    expect(control('k1', 'up').getAttribute('aria-disabled')).toBe('true');
    expect(control('k3', 'down').getAttribute('aria-disabled')).toBe('true');
    expect(control('k2', 'up').getAttribute('aria-disabled')).toBe('false');
    expect(control('k2', 'text').textContent).toContain('Editar a pista 2: Rastros');
  });

  it('invites the first clue when there is none', () => {
    setup([]);
    expect(el.textContent).toContain(
      'Nenhuma pista ainda. Escreva o que os jogadores podem descobrir aqui.',
    );
    expect(flat(el.querySelector('.cl__count'))).toBe('0 de 30');
    expect(el.querySelector('.cl__list')).toBeNull();
  });

  it('moves a clue up, saves at once, says so, and keeps focus on the same button of the moved row', async () => {
    setup([...THREE]);
    control('k3', 'up').focus();
    control('k3', 'up').click();
    await settle();
    expect(api.calls).toEqual(['moveSceneClue p1 k3 up']);
    expect(rows().map((r) => r.getAttribute('data-row'))).toEqual(['k1', 'k3', 'k2']);
    expect(document.activeElement).toBe(control('k3', 'up'));
    expect(flat(el.querySelector('[role="status"]'))).toBe('Pista 3 subiu para a posição 2.');
    control('k3', 'down').click();
    await settle();
    expect(document.activeElement).toBe(control('k3', 'down'));
  });

  it('does nothing for the first ↑, and answers nothing while a write is in flight', async () => {
    setup([...THREE]);
    control('k1', 'up').click();
    await settle();
    expect(api.calls).toEqual([]);
    control('k2', 'up').click();
    control('k3', 'up').click();
    await settle();
    expect(api.calls).toEqual(['moveSceneClue p1 k2 up']);
  });

  it('asks before removing, in place, with "Voltar" focused, and "Voltar" gives focus back', async () => {
    setup([...THREE]);
    control('k2', 'remove').click();
    await settle();
    const ask = el.querySelector('[role="alertdialog"]')!;
    expect(flat(ask)).toContain('Remover a pista 2?');
    expect(flat(ask)).toContain('Quem já recebeu continua com ela.');
    expect(document.activeElement).toBe(button('Voltar'));
    expect(api.calls).toEqual([]);
    // The two answers are the same size, "Voltar" first.
    const answers = Array.from(ask.querySelectorAll<HTMLButtonElement>('button'));
    expect(answers.map((b) => flat(b))).toEqual(['Voltar', 'Remover pista']);
    button('Voltar').click();
    await settle();
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    expect(document.activeElement).toBe(control('k2', 'remove'));
    expect(rows()).toHaveLength(3);
  });

  it('removes after the answer and focuses the next row; the last one sends focus to "Adicionar pista"', async () => {
    setup([...THREE]);
    control('k2', 'remove').click();
    await settle();
    button('Remover pista').click();
    await settle();
    expect(api.calls).toEqual(['removeSceneClue p1 k2']);
    expect(rows().map((r) => r.getAttribute('data-row'))).toEqual(['k1', 'k3']);
    expect(document.activeElement).toBe(control('k3', 'remove'));
    control('k3', 'remove').click();
    await settle();
    button('Remover pista').click();
    await settle();
    control('k1', 'remove').click();
    await settle();
    button('Remover pista').click();
    await settle();
    expect(el.textContent).toContain('Nenhuma pista ainda');
    expect(document.activeElement).toBe(button('Adicionar pista'));
  });

  it('opens the form in place with focus on the field, and "Cancelar" gives focus back', async () => {
    setup([...THREE]);
    button('Adicionar pista').click();
    await settle();
    expect(el.querySelector('form')).not.toBeNull();
    expect(document.activeElement).toBe(el.querySelector('textarea'));
    // "Adicionar pista" is gone while the form is open; the form has its own, outlined, with "Cancelar" as text, under the field.
    expect(el.querySelectorAll('.cl__add')).toHaveLength(0);
    const buttons = Array.from(el.querySelectorAll<HTMLButtonElement>('.cf__actions button'));
    expect(buttons.map((b) => flat(b))).toEqual(['Adicionar pista', 'Cancelar']);
    expect(buttons[0].classList).toContain('mat-mdc-outlined-button');
    expect(buttons[1].classList).toContain('mat-mdc-button');
    button('Cancelar').click();
    await settle();
    expect(el.querySelector('form')).toBeNull();
    expect(document.activeElement).toBe(button('Adicionar pista'));
    expect(api.calls).toEqual([]);
  });

  it('adds a clue, saves at once, counts it and focuses "Adicionar pista"', async () => {
    setup([...THREE]);
    button('Adicionar pista').click();
    await settle();
    type('Uma moeda de prata suja de lama.');
    expect(
      flat(el.querySelector('.cf mat-hint[align="end"], .cf .mat-mdc-form-field-hint-wrapper')),
    ).toContain('32 de 500');
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(
      new Event('submit', { cancelable: true }),
    );
    await settle();
    expect(api.calls).toEqual(['addSceneClue p1 Uma moeda de prata suja de lama.']);
    expect(emitted.at(-1)).toHaveLength(4);
    expect(el.querySelector('form')).toBeNull();
    expect(flat(el.querySelector('.cl__count'))).toBe('4 de 30');
    expect(flat(el.querySelector('[role="status"]'))).toBe('Pista adicionada. 4 de 30.');
    expect(document.activeElement).toBe(button('Adicionar pista'));
  });

  it('refuses an empty clue in a box with an icon and words, and puts focus back on the field', async () => {
    setup([...THREE]);
    button('Adicionar pista').click();
    await settle();
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(
      new Event('submit', { cancelable: true }),
    );
    await settle();
    const alert = el.querySelector('[role="alert"]')!;
    expect(flat(alert)).toContain(
      'Escreva a pista antes de salvar. Ela pode ter até 500 caracteres.',
    );
    expect(alert.querySelector('mat-icon')?.textContent).toBe('error');
    expect(document.activeElement).toBe(el.querySelector('textarea'));
    expect(api.calls).toEqual([]);
    // Typing again clears it.
    type('Algo');
    await settle();
    expect(el.querySelector('[role="alert"]')).toBeNull();
  });

  it('does not cut a clue over 500 characters: the counter turns red and the message says how many to take off', async () => {
    setup([]);
    button('Adicionar pista').click();
    await settle();
    type('x'.repeat(512));
    expect(el.querySelector('.cf__over')?.textContent).toContain('512 de 500');
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(
      new Event('submit', { cancelable: true }),
    );
    await settle();
    expect(flat(el.querySelector('[role="alert"]'))).toContain(
      'A pista passa de 500 caracteres: tem 512, tire 12.',
    );
    expect(api.calls).toEqual([]);
  });

  it('turns a pasted line break into a space: a clue is one line', async () => {
    setup([]);
    button('Adicionar pista').click();
    await settle();
    const field = type('primeira\nsegunda');
    expect(field.value).toBe('primeira segunda');
  });

  it('says what the server refused inside the form, by its code', async () => {
    setup([...THREE]);
    button('Adicionar pista').click();
    await settle();
    type('Algo');
    api.failWith = new ConnectError('x', Code.NotFound);
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(
      new Event('submit', { cancelable: true }),
    );
    await settle();
    expect(flat(el.querySelector('.cf [role="alert"]'))).toContain(
      'Essa pista ou esse ponto não existe mais. Recarregue a página.',
    );
    expect(el.querySelector('form')).not.toBeNull();
  });

  it('edits a clue in place by tapping its words, saves at once and focuses its words again', async () => {
    setup([...THREE]);
    control('k2', 'text').click();
    await settle();
    expect(flat(el.querySelector('.cf__title'))).toBe('Editar a pista 2');
    const field = el.querySelector<HTMLTextAreaElement>('textarea')!;
    expect(field.value).toBe('Rastros de três goblins e de botas pesadas, rumo ao norte.');
    expect(document.activeElement).toBe(field);
    type('Rastros de dois goblins.');
    expect(flat(button('Salvar pista'))).toBe('Salvar pista');
    el.querySelector<HTMLFormElement>('form')!.dispatchEvent(
      new Event('submit', { cancelable: true }),
    );
    await settle();
    expect(api.calls).toEqual(['updateSceneClue p1 k2 Rastros de dois goblins.']);
    expect(el.querySelector('form')).toBeNull();
    expect(control('k2', 'text').textContent).toContain('Rastros de dois goblins.');
    expect(document.activeElement).toBe(control('k2', 'text'));
  });

  it('at 30 of 30 says the limit in words above a disabled button, tied to it, and folds the list', () => {
    setup(Array.from({ length: 30 }, (_, i) => clue(`x${i}`, `Pista ${i + 1}`)));
    expect(flat(el.querySelector('.cl__count'))).toBe('30 de 30');
    expect(flat(el.querySelector('.cl__limit'))).toBe(
      'Limite de 30 pistas. Remova uma para adicionar outra.',
    );
    const add = button('Adicionar pista');
    expect(add.getAttribute('aria-describedby')).toBe('cl-limit');
    expect(add.disabled || add.getAttribute('aria-disabled') === 'true').toBe(true);
    expect(
      el.querySelector('.cl__limit')!.compareDocumentPosition(add) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    // Past eight rows the list folds, and says how many are not on screen.
    expect(rows()).toHaveLength(8);
    expect(flat(el.querySelector('.cl__more'))).toContain('Mais 22 pistas na lista.');
    button('Mostrar todas').click();
    fixture.detectChanges();
    expect(rows()).toHaveLength(30);
    expect(button('Mostrar menos').getAttribute('aria-expanded')).toBe('true');
  });

  it('says a 30-clue refusal from the server by its code, above the list', async () => {
    setup([...THREE]);
    api.failWith = new ConnectError('x', Code.ResourceExhausted);
    control('k2', 'up').click();
    await settle();
    expect(flat(el.querySelector('[role="alert"]'))).toContain(
      'Limite de 30 pistas. Remova uma para adicionar outra.',
    );
  });
});
