import { ComponentFixture, TestBed } from '@angular/core/testing';

import { NoteEditing } from '../../core/notes/note-editing';
import { FakeNotesClient, note, scene } from '../../core/notes/notes-testing';
import { NotesState } from '../../core/notes/notes-state';
import { NoteFields } from './note-fields';
import { nextTurn } from '../../../test-settle';

describe('NoteFields', () => {
  let api: FakeNotesClient;
  let state: NotesState;
  let editing: NoteEditing;
  let fixture: ComponentFixture<NoteFields>;
  let el: HTMLElement;

  async function setup(edit = false) {
    api = new FakeNotesClient();
    api.scenesList = [scene('s1', 'A carroça tombada')];
    api.notes = [note('n1', 'Brisa me deve 5 PO', new Date())];
    state = new NotesState(api as never, () => 'c1');
    await state.refresh();
    editing = new NoteEditing(state);
    Element.prototype.scrollIntoView = vi.fn();
    if (edit) {
      editing.openEdit(state.notes()[0]);
    } else {
      editing.openNew();
    }
    fixture = TestBed.createComponent(NoteFields);
    fixture.componentRef.setInput('editing', editing);
    fixture.componentRef.setInput('scenes', state.scenes());
    fixture.detectChanges();
    el = fixture.nativeElement;
    await settle();
  }

  async function settle() {
    for (let i = 0; i < 4; i++) {
      await fixture.whenStable();
      await nextTurn();
      fixture.detectChanges();
    }
  }

  const flat = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();

  it('starts with the cursor in the text, "Só você lê." and the counter, and carries the shared notice', async () => {
    await setup();
    expect(document.activeElement).toBe(el.querySelector('textarea'));
    expect(flat(el)).toContain('Só você lê.');
    expect(flat(el)).toContain('0 de 2.000');
    expect(el.querySelector('[role="note"]')?.textContent).toContain(
      'É ficção: não escreva dados reais de pessoas.',
    );
    expect(el.textContent).toContain('Só aparecem as cenas que o grupo já descobriu.');
  });

  it('stops at 2.000 characters (maxlength) and, at the limit, says so with an icon and words', async () => {
    await setup();
    expect(el.querySelector('textarea')?.getAttribute('maxlength')).toBe('2000');
    editing.text.setValue('x'.repeat(2000));
    await settle();
    expect(flat(el.querySelector('[role="alert"]'))).toContain(
      'Chegou ao limite de 2.000 caracteres. Corte um trecho para escrever mais.',
    );
    expect(el.querySelector('.nf__over')?.textContent).toContain('2.000 de 2.000');
  });

  it('has no delete for a new note', async () => {
    await setup();
    expect(flat(el)).not.toContain('Apagar anotação');
  });

  it('asks before deleting, in place, with "Voltar" focused first', async () => {
    await setup(true);
    const del = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Apagar anotação'),
    )!;
    del.click();
    await settle();
    const ask = el.querySelector('[role="alertdialog"]')!;
    expect(flat(ask)).toContain('Apagar esta anotação?');
    expect(flat(ask)).toContain('Não dá para desfazer.');
    const back = Array.from(ask.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Voltar'),
    )!;
    expect(document.activeElement).toBe(back);
  });
});
