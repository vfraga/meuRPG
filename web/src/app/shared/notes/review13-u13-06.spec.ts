// Finding U13-06 (review/unit-13-web-live-rest.md): the sheet's ✕ (closed) or a second cancel() while "Descartar o que você escreveu?" is showing discards the text.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { NoteEditing } from '../../core/notes/note-editing';
import { NotesState } from '../../core/notes/notes-state';
import { FakeNotesClient, scene } from '../../core/notes/notes-testing';
import { NotesSheet, type NotesSheetData } from './notes-sheet';

describe('Review13 U13-06: dismissing the discard question must not discard', () => {
  it('model: a second cancel() while asking keeps the text and the form', async () => {
    const api = new FakeNotesClient();
    const state = new NotesState(api as never, () => 'c1');
    await state.refresh();
    const editing = new NoteEditing(state, () => '');
    editing.openNew();
    editing.text.setValue('Algo importante');
    editing.cancel();
    expect(editing.confirmingDiscard()).toBe(true);
    editing.cancel();
    expect(editing.stage()).toBe('form');
    expect(editing.text.value).toBe('Algo importante');
  });

  async function setup() {
    const api = new FakeNotesClient();
    api.notes = [];
    api.scenesList = [scene('s1', 'A carroça tombada')];
    const state = new NotesState(api as never, () => 'c1');
    await state.refresh();
    const data: NotesSheetData = { state, openScene: () => '' };
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
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
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find(
        (b) => b.textContent?.trim().includes(name) || b.getAttribute('aria-label') === name,
      )!;
    return { fixture, el, settle, button };
  }

  async function askToDiscard() {
    const s = await setup();
    s.button('Nova anotação').click();
    await s.settle();
    const field = s.el.querySelector<HTMLTextAreaElement>('textarea')!;
    field.value = 'Algo importante';
    field.dispatchEvent(new Event('input'));
    s.fixture.detectChanges();
    s.button('Cancelar').click();
    await s.settle();
    expect(s.el.querySelector('[role="alertdialog"]')?.textContent).toContain(
      'Descartar o que você escreveu?',
    );
    return s;
  }

  it('control: "Continuar" on the question keeps the text', async () => {
    const { el, button, settle } = await askToDiscard();
    button('Continuar').click();
    await settle();
    expect(el.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Algo importante');
  });

  it('the ✕ (Fechar) while the question shows keeps the text instead of discarding it', async () => {
    const { el, button, settle } = await askToDiscard();
    button('Fechar').click();
    await settle();
    expect(el.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Algo importante');
  });
});
