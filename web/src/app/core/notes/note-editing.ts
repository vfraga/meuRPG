import { computed, signal } from '@angular/core';
import { FormControl } from '@angular/forms';
import { Code, ConnectError } from '@connectrpc/connect';

import type { Note } from '../../../gen/meurpg/notes/v1/notes_pb';
import { NOTE_MAX, noteErrorMessage, noteTextError } from './notes-errors';
import { NEW, type NotesState } from './notes-state';

/**
 * The flow of writing a note, shared by the notes sheet (the session) and the
 * character sheet's panel (E8-06, E8-07), so both behave the same:
 *
 * - "Nova anotação" opens the form with the scene that is open in the session
 *   already tagged (or "Sem cena"); the pencil on a note opens it filled;
 * - "Salvar anotação" with nothing written says so, in words, and keeps the
 *   form; a text over 2.000 characters is not cut: the message says how many
 *   to take off;
 * - "Cancelar" discards at once when nothing was written, and asks in place
 *   ("Descartar o que você escreveu?") when there is text or a changed tag;
 * - "Apagar anotação" asks in place ("Apagar esta anotação? Não dá para
 *   desfazer.") and "Voltar" is the safe answer;
 * - what the server refuses is said by its code (the 300-note limit, a scene
 *   the group has not discovered), never by its message.
 *
 * One write at a time per note: while it is in flight, a second tap is ignored.
 */
export class NoteEditing {
  readonly stage = signal<'list' | 'form'>('list');
  /** The note being edited, or `null` for a new one. */
  readonly editing = signal<Note | null>(null);
  readonly text = new FormControl('', { nonNullable: true });
  /** The scene tag: a point ID, or `''` for none. */
  readonly sceneId = signal('');
  readonly length = signal(0);
  /** The text as typed, for what depends on it (a form control's value is not a signal). */
  private readonly typed = signal('');
  readonly error = signal('');
  readonly confirmingDelete = signal(false);
  readonly confirmingDiscard = signal(false);

  readonly max = NOTE_MAX;
  readonly busy = computed(() => this.notes.isWriting(this.editing()?.id ?? NEW));
  /** Something was written or changed that would be lost. */
  readonly dirty = computed(() => {
    const note = this.editing();
    const text = this.typed().trim();
    return note
      ? text !== note.text || this.sceneId() !== note.scenePointId
      : text !== '' || this.sceneId() !== this.startScene;
  });

  private startScene = '';

  constructor(
    private readonly notes: NotesState,
    /** The scene open in the session, to tag a new note with ('' for none). */
    private readonly openScene: () => string = () => '',
  ) {
    this.text.valueChanges.subscribe((value) => {
      this.typed.set(value);
      this.length.set([...value].length);
      this.error.set('');
    });
  }

  openNew(): void {
    // A scene counts only once the group discovered it (the server refuses any other).
    const scene = this.openScene();
    this.startScene = this.notes.scenes().some((s) => s.id === scene) ? scene : '';
    this.begin(null, '', this.startScene);
    void this.notes.refreshScenes();
  }

  openEdit(note: Note): void {
    this.startScene = note.scenePointId;
    this.begin(note, note.text, note.scenePointId);
    void this.notes.refreshScenes();
  }

  private begin(note: Note | null, text: string, scene: string): void {
    this.editing.set(note);
    this.text.setValue(text);
    this.typed.set(text);
    this.sceneId.set(scene);
    this.length.set([...text].length);
    this.error.set('');
    this.confirmingDelete.set(false);
    this.confirmingDiscard.set(false);
    this.stage.set('form');
  }

  /** Back to the list, forgetting the form. */
  close(): void {
    this.stage.set('list');
    this.editing.set(null);
    this.error.set('');
    this.confirmingDelete.set(false);
    this.confirmingDiscard.set(false);
  }

  /**
   * "Cancelar": discards at once, or asks first when there is something to lose.
   * While the question shows, it changes nothing: only "Descartar" discards.
   */
  cancel(): void {
    if (this.confirmingDiscard()) {
      return;
    }
    if (this.dirty()) {
      this.confirmingDelete.set(false);
      this.confirmingDiscard.set(true);
      return;
    }
    this.close();
  }

  keepWriting(): void {
    this.confirmingDiscard.set(false);
  }

  askDelete(): void {
    this.confirmingDiscard.set(false);
    this.confirmingDelete.set(true);
  }

  keepNote(): void {
    this.confirmingDelete.set(false);
  }

  /** "Salvar anotação". `true` once saved (the form is closed). */
  async save(): Promise<boolean> {
    if (this.busy()) {
      return false;
    }
    const text = this.text.value.trim();
    const problem = noteTextError(text);
    if (problem) {
      this.error.set(problem);
      return false;
    }
    this.error.set('');
    const note = this.editing();
    // Nothing changed (whitespace around the text does not count): the server
    // refuses an update with no field, so there is nothing to send.
    if (note && !this.dirty()) {
      this.close();
      return true;
    }
    try {
      const saved = note
        ? await this.notes.update(note.id, {
            ...(text !== note.text ? { text } : {}),
            ...(this.sceneId() !== note.scenePointId ? { scenePointId: this.sceneId() } : {}),
          })
        : await this.notes.create(text, this.sceneId());
      if (saved === null) {
        return false;
      }
      this.close();
      return true;
    } catch (err) {
      this.fail(err, note ? 'salvar a anotação' : 'escrever a anotação');
      return false;
    }
  }

  /** "Apagar" confirmed. `true` once it is gone (the form is closed). */
  async remove(): Promise<boolean> {
    const note = this.editing();
    if (!note || this.busy()) {
      return false;
    }
    try {
      if (!(await this.notes.remove(note.id))) {
        return false;
      }
      this.close();
      return true;
    } catch (err) {
      this.confirmingDelete.set(false);
      this.fail(err, 'apagar a anotação');
      return false;
    }
  }

  private fail(err: unknown, what: string): void {
    // The note is not there any more: the list is read again and the form closes.
    if (ConnectError.from(err).code === Code.NotFound) {
      void this.notes.refresh();
      this.close();
      return;
    }
    this.error.set(noteErrorMessage(err, what));
  }
}
