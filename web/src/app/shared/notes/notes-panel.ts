import {
  Component,
  ElementRef,
  Injector,
  OnInit,
  afterNextRender,
  computed,
  inject,
  input,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Note } from '../../../gen/meurpg/notes/v1/notes_pb';
import { NoteEditing } from '../../core/notes/note-editing';
import { NotesClient } from '../../core/notes/notes-client';
import { NotesState } from '../../core/notes/notes-state';
import { FILTER_ALL, applyFilter, entryCount, filterOptions } from '../../core/notes/notes-view';
import { NoteFields } from './note-fields';
import { NoteList } from './note-list';
import { NotesSelect } from './notes-select';

/**
 * "Anotações" on the character sheet page (E8-07, MR-030): the first block of
 * the fourth column, for the player the notes belong to (the master never gets
 * this panel: the notes are the player's alone). The same notes as the
 * session's sheet, with the same filter by scene, list, tags and forms: what
 * is written here or there is one list on the server. They stay editable on a
 * locked sheet: they are not part of the sheet.
 *
 * "Nova anotação" is outlined (the page's filled button is "Editar ficha"); the
 * form opens in place at the top of the panel and the list goes while it is
 * open; "Salvar anotação" is the filled one, with "Cancelar" outlined and the
 * same size under the fields. Cancelling with text asks in place first.
 */
@Component({
  selector: 'app-notes-panel',
  imports: [MatButtonModule, MatIconModule, NoteFields, NoteList, NotesSelect],
  templateUrl: './notes-panel.html',
  styleUrl: './notes-panel.scss',
})
export class NotesPanel implements OnInit {
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly campaignId = input.required<string>();

  protected readonly state = new NotesState(inject(NotesClient), () => this.campaignId());
  protected readonly editing = new NoteEditing(this.state);
  protected readonly filter = signal(FILTER_ALL);
  protected readonly options = computed(() =>
    filterOptions(this.state.notes(), this.state.scenes()),
  );
  protected readonly visible = computed(() => applyFilter(this.state.notes(), this.filter()));
  protected readonly chosen = computed(() => this.options().find((o) => o.value === this.filter()));
  protected readonly count = computed(() => entryCount(this.state.notes().length));
  protected readonly announce = signal('');

  private readonly newButton = viewChild('newButton', { read: ElementRef<HTMLButtonElement> });
  private readonly keepButton = viewChild('keep', { read: ElementRef<HTMLButtonElement> });
  private opener: string | null = null;

  ngOnInit(): void {
    void this.state.refresh();
  }

  protected setFilter(value: string): void {
    this.filter.set(value);
    this.announce.set(entryCount(applyFilter(this.state.notes(), value).length));
  }

  protected newNote(): void {
    if (this.state.atLimit()) {
      return;
    }
    this.opener = null;
    this.editing.openNew();
  }

  protected editNote(note: Note): void {
    this.opener = note.id;
    this.editing.openEdit(note);
  }

  protected backToList(): void {
    afterNextRender(
      () => {
        const target = this.opener
          ? this.host.nativeElement.querySelector<HTMLElement>(`[data-note="${this.opener}"]`)
          : null;
        (target ?? this.newButton()?.nativeElement)?.focus();
      },
      { injector: this.injector },
    );
  }

  protected async save(): Promise<void> {
    if (await this.editing.save()) {
      this.backToList();
    }
  }

  protected cancel(): void {
    this.editing.cancel();
    if (this.editing.stage() === 'list') {
      this.backToList();
    }
    if (this.editing.confirmingDiscard()) {
      afterNextRender(() => this.keepButton()?.nativeElement.focus(), { injector: this.injector });
    }
  }

  protected discard(): void {
    this.editing.close();
    this.backToList();
  }
}
