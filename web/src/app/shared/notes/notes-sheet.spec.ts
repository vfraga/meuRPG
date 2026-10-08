import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import { NotesState } from '../../core/notes/notes-state';
import { FakeNotesClient, note, scene } from '../../core/notes/notes-testing';
import { NotesSheet, type NotesSheetData } from './notes-sheet';

// Relative to today, so the "Hoje" stamps hold on any day; day 3 is today, day 1 two days earlier.
const at = (day: number, h: number, m: number) => {
  const now = new Date();
  return new Date(now.getFullYear(), now.getMonth(), now.getDate() - (3 - day), h, m);
};

describe('NotesSheet', () => {
  let api: FakeNotesClient;
  let state: NotesState;
  let close: ReturnType<typeof vi.fn>;

  async function setup(
    notes = [
      note('n1', 'Perguntar ao ferreiro sobre o brasão de lobo', at(3, 21, 24), {
        sceneId: 's1',
        sceneName: 'A carroça tombada',
      }),
      note('c1', 'Um brasão de lobo queimado na lona da carroça.', at(3, 21, 20), {
        sceneId: 's1',
        sceneName: 'A carroça tombada',
        clue: true,
      }),
      note('n2', 'Brisa me deve 5 PO', at(1, 22, 3)),
      note('n3', 'Comprar tinta e pergaminho para o grimório', at(1, 18, 40)),
    ],
    openScene = '',
  ) {
    api = new FakeNotesClient();
    api.notes = notes;
    api.scenesList = [scene('s1', 'A carroça tombada'), scene('s2', 'A ponte do rio')];
    state = new NotesState(api as never, () => 'c1');
    await state.refresh();
    close = vi.fn();
    const data: NotesSheetData = { state, openScene: () => openScene };
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
      ],
    });
    Element.prototype.scrollIntoView = vi.fn();
    Element.prototype.scrollTo = vi.fn() as never;
    const fixture = TestBed.createComponent(NotesSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        await fixture.whenStable();
        await new Promise((r) => setTimeout(r));
        fixture.detectChanges();
      }
    };
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.trim().includes(name),
      )!;
    const flat = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();
    const type = (value: string) => {
      const field = el.querySelector<HTMLTextAreaElement>('textarea')!;
      field.value = value;
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
    };
    return { fixture, el, settle, button, flat, type };
  }

  it('starts with the lock line and lists notes and clues newest first, with where and when', async () => {
    const { el, flat } = await setup();
    expect(flat(el.querySelector('.ns__lock'))).toContain(
      'Só você lê as suas anotações. O mestre não vê.',
    );
    const rows = Array.from(el.querySelectorAll('.nl__row'));
    expect(rows).toHaveLength(4);
    expect(flat(rows[0])).toContain('Perguntar ao ferreiro sobre o brasão de lobo');
    expect(flat(rows[0])).toContain('A carroça tombada');
    expect(flat(rows[0])).toContain('Hoje, 21:24');
    expect(flat(rows[2])).toContain('Sem cena');
  });

  it('marks a received clue "Pista do mestre", read-only, with a lock, and no pencil or delete', async () => {
    const { el, flat } = await setup();
    const clue = el.querySelectorAll('.nl__row')[1];
    expect(flat(clue.querySelector('.nl__tag'))).toContain('Pista do mestre');
    expect(flat(clue)).toContain('Só leitura');
    expect(clue.querySelector('.nl__lock')).not.toBeNull();
    expect(clue.querySelector('.nl__edit')).toBeNull();
    // The pencils of the notes are named with the notes' own words.
    expect(el.querySelectorAll('.nl__edit')).toHaveLength(3);
    expect(el.querySelector('.nl__edit')?.getAttribute('aria-label')).toBe(
      'Editar a anotação: Perguntar ao ferreiro sobre o brasão de lobo',
    );
  });

  it('opening it marks the news as seen', async () => {
    await setup();
    // The sheet that just opened already saw the news; a clue that arrives after it is new again,
    // and opening the sheet once more clears it.
    state.fresh.set([note('c9', 'x', at(3, 21, 0), { clue: true })]);
    state.notice.set(true);
    TestBed.createComponent(NotesSheet).detectChanges();
    expect(state.fresh()).toEqual([]);
    expect(state.notice()).toBe(false);
  });

  it('filters by scene: the discovered scenes and "Sem cena", each with its count, and says how many it shows', async () => {
    const { fixture, el, flat, settle } = await setup();
    await settle();
    const select = el.querySelector('mat-select') as HTMLElement;
    expect(flat(select)).toContain('Todas as anotações · 4');
    select.click();
    fixture.detectChanges();
    await settle();
    const options = Array.from(document.querySelectorAll('.mr-notes-panel mat-option'), (o) =>
      flat(o),
    );
    expect(options).toEqual([
      'Todas as anotações4',
      'A carroça tombada2',
      'A ponte do rio0',
      'Sem cena2',
    ]);
    (document.querySelectorAll('.mr-notes-panel mat-option')[1] as HTMLElement).click();
    await settle();
    expect(el.querySelectorAll('.nl__row')).toHaveLength(2);
    expect(flat(el.querySelector('.mr-visually-hidden[aria-live]'))).toBe('2 anotações');
  });

  it('says a scene with nothing by its name, and keeps the filter to change scene', async () => {
    const { fixture, el, flat, settle } = await setup();
    el.querySelector<HTMLElement>('mat-select')!.click();
    fixture.detectChanges();
    await settle();
    (document.querySelectorAll('.mr-notes-panel mat-option')[2] as HTMLElement).click();
    await settle();
    expect(flat(el.querySelector('.ns__empty'))).toContain('Nada nesta cena ainda');
    expect(flat(el.querySelector('.ns__empty'))).toContain(
      'Você ainda não anotou nada sobre “A ponte do rio”.',
    );
    expect(el.querySelector('mat-select')).not.toBeNull();
  });

  it('with no notes at all invites the first one and shows no filter', async () => {
    const { el, flat, button } = await setup([]);
    expect(flat(el.querySelector('.ns__empty'))).toContain('Nenhuma anotação ainda');
    expect(flat(el.querySelector('.ns__empty'))).toContain(
      'As pistas que ele revelar para você aparecem aqui também.',
    );
    expect(el.querySelector('mat-select')).toBeNull();
    expect(button('Nova anotação')).toBeTruthy();
  });

  it('opens a new note in the sheet, tagged with the scene open in the session, with the fiction notice', async () => {
    const { el, button, settle, flat } = await setup(undefined, 's1');
    button('Nova anotação').click();
    await settle();
    expect(flat(el.querySelector('.frame__title, h2'))).toBe('Nova anotação');
    expect(document.activeElement).toBe(el.querySelector('textarea'));
    expect(flat(el.querySelector('app-notes-select mat-select'))).toContain('A carroça tombada');
    expect(el.querySelector('[role="note"]')?.textContent).toContain('É ficção');
    expect(flat(el.querySelector('.ns__pair'))).toBe('Cancelar Salvar anotação');
  });

  it('creates a note, goes back to the list and puts focus where it was', async () => {
    const { el, button, settle, type } = await setup();
    button('Nova anotação').click();
    await settle();
    type('Perguntar ao ferreiro');
    button('Salvar anotação').click();
    await settle();
    expect(api.calls).toContain('create Perguntar ao ferreiro ');
    expect(el.querySelector('.nl__text')?.textContent).toContain('Perguntar ao ferreiro');
    expect(document.activeElement).toBe(button('Nova anotação'));
  });

  it('says an empty note in words and keeps the form', async () => {
    const { el, button, settle } = await setup();
    button('Nova anotação').click();
    await settle();
    button('Salvar anotação').click();
    await settle();
    expect(el.querySelector('[role="alert"]')?.textContent).toContain(
      'Escreva a anotação antes de salvar.',
    );
  });

  it('cancels at once when nothing was written, and asks in place when there is text', async () => {
    const { el, button, settle, type } = await setup();
    button('Nova anotação').click();
    await settle();
    button('Cancelar').click();
    await settle();
    expect(el.querySelector('textarea')).toBeNull();
    button('Nova anotação').click();
    await settle();
    type('Algo');
    button('Cancelar').click();
    await settle();
    expect(el.querySelector('[role="alertdialog"]')?.textContent).toContain(
      'Descartar o que você escreveu?',
    );
    expect(document.activeElement).toBe(button('Continuar'));
    button('Continuar').click();
    await settle();
    expect(el.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Algo');
    button('Cancelar').click();
    await settle();
    button('Descartar').click();
    await settle();
    expect(el.querySelector('textarea')).toBeNull();
  });

  it('edits a note, and deletes it after a question with "Voltar" focused', async () => {
    const { el, settle, button, type, flat } = await setup();
    el.querySelectorAll<HTMLButtonElement>('.nl__edit')[1].click();
    await settle();
    expect(flat(el.querySelector('h2'))).toBe('Editar anotação');
    expect(el.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Brisa me deve 5 PO');
    type('Brisa me deve 10 PO');
    button('Salvar anotação').click();
    await settle();
    expect(api.calls).toContain('update n2 {"text":"Brisa me deve 10 PO"}');
    el.querySelectorAll<HTMLButtonElement>('.nl__edit')[0].click();
    await settle();
    button('Apagar anotação').click();
    await settle();
    expect(document.activeElement).toBe(button('Voltar'));
    Array.from(el.querySelectorAll<HTMLButtonElement>('.nf__ask button'))
      .find((b) => b.textContent?.includes('Apagar anotação'))!
      .click();
    await settle();
    expect(api.calls.some((c) => c.startsWith('delete'))).toBe(true);
    expect(el.querySelectorAll('.nl__row')).toHaveLength(3);
  });

  it('at 300 notes says the limit above a disabled "Nova anotação" tied to it', async () => {
    const many = Array.from({ length: 300 }, (_, i) => note(`x${i}`, `n${i}`, at(1, 10, 0)));
    const { el, button, flat } = await setup(many);
    expect(flat(el.querySelector('.ns__limit'))).toBe(
      'Limite de 300 anotações. Apague uma para escrever outra.',
    );
    expect(button('Nova anotação').getAttribute('aria-describedby')).toBe('ns-limit');
    expect(
      button('Nova anotação').disabled ||
        button('Nova anotação').getAttribute('aria-disabled') === 'true',
    ).toBe(true);
    // 300 rows render in about half a second alone, but past the 5 s default while the whole suite runs in parallel on a busy machine.
  }, 20_000);

  it('says the 300-note limit from the server by its code', async () => {
    const { el, button, settle, type } = await setup();
    button('Nova anotação').click();
    await settle();
    type('Mais uma');
    api.failWith = new ConnectError('x', Code.ResourceExhausted);
    button('Salvar anotação').click();
    await settle();
    expect(el.querySelector('[role="alert"]')?.textContent).toContain(
      'Limite de 300 anotações. Apague uma para escrever outra.',
    );
  });

  describe('the question "Descartar o que você escreveu?"', () => {
    async function askToDiscard() {
      const s = await setup([], '');
      s.button('Nova anotação').click();
      await s.settle();
      s.type('Algo importante');
      s.button('Cancelar').click();
      await s.settle();
      expect(s.el.querySelector('[role="alertdialog"]')?.textContent).toContain(
        'Descartar o que você escreveu?',
      );
      return s;
    }

    it('"Continuar" keeps the text', async () => {
      const { el, button, settle } = await askToDiscard();
      button('Continuar').click();
      await settle();
      expect(el.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Algo importante');
    });

    it('keeps the text when the sheet is closed with the ✕ while the question shows', async () => {
      const { el, settle } = await askToDiscard();
      el.querySelector<HTMLButtonElement>('button[aria-label="Fechar"]')!.click();
      await settle();
      expect(el.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Algo importante');
      expect(close).not.toHaveBeenCalled();
    });
  });
});
