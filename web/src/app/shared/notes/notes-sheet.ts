import {
  Component,
  DOCUMENT,
  DestroyRef,
  ElementRef,
  Injector,
  OnInit,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import type { Note } from '../../../gen/meurpg/notes/v1/notes_pb';
import { NoteEditing } from '../../core/notes/note-editing';
import type { NotesState } from '../../core/notes/notes-state';
import { FILTER_ALL, applyFilter, entryCount, filterOptions } from '../../core/notes/notes-view';
import { SheetFrame } from '../../pages/live-session/combat/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../pages/live-session/combat/sheet-host';
import { NoteFields } from './note-fields';
import { NoteList } from './note-list';
import { NotesSelect } from './notes-select';

/** What the session page hands the notes sheet. */
export interface NotesSheetData {
  readonly state: NotesState;
  /** The scene open in the session ('' for none): a new note starts tagged with it. */
  readonly openScene: () => string;
}

/** "Anotações": a bottom sheet on a phone and a dialog from a tablet up. */
export function openNotesSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: NotesSheetData,
): Observable<void | undefined> {
  return openSheet<NotesSheet, NotesSheetData, void>(dialog, bottomSheet, NotesSheet, {
    data,
    ariaLabel: 'Anotações',
    labelledBy: 'notes-t',
    width: '520px',
  });
}

/**
 * The player's notes in the session (E8-06, MR-030): the list, newest first, with
 * the clues the master revealed marked "Pista do mestre" (read-only), the filter
 * by scene, a new note, and editing and deleting one's own. The frame is the
 * combat's (`sheet-frame`): the title and the footer's one filled button never
 * scroll away, so with the keyboard open on a 320×568 phone the buttons are
 * still reachable: on a phone the sheet follows what is left of the screen above
 * the keyboard (`visualViewport`).
 *
 * - The first line is always "Só você lê as suas anotações. O mestre não vê."
 *   with a lock.
 * - The filter lists only the scenes the group discovered, plus "Sem cena", each
 *   with its count; it resets when the sheet closes. A scene with nothing says
 *   so by name ("Nada nesta cena ainda").
 * - Two empty states: no notes at all (an invitation, no filter), and a filter
 *   with no result (the filter stays, to change scene).
 * - Opening it marks the news as seen (the app bar's "nova" and the notice go).
 *
 * Focus: the title first; "Nova anotação" and the pencils open the form with the
 * cursor in the text; closing a form hands focus back to the button or pencil
 * that opened it.
 */
@Component({
  selector: 'app-notes-sheet',
  imports: [MatButtonModule, MatIconModule, NoteFields, NoteList, NotesSelect, SheetFrame],
  templateUrl: './notes-sheet.html',
  styleUrl: './notes-sheet.scss',
})
export class NotesSheet implements OnInit {
  private readonly sheet = injectSheet<NotesSheetData, void>();
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly document = inject(DOCUMENT);
  private readonly destroyRef = inject(DestroyRef);
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly state = this.data.state;
  protected readonly editing = new NoteEditing(this.state, this.data.openScene);

  protected readonly filter = signal(FILTER_ALL);
  protected readonly options = computed(() =>
    filterOptions(this.state.notes(), this.state.scenes()),
  );
  protected readonly visible = computed(() => applyFilter(this.state.notes(), this.filter()));
  protected readonly chosen = computed(() => this.options().find((o) => o.value === this.filter()));
  protected readonly empty = computed(() => this.state.notes().length === 0);
  protected readonly title = computed(() =>
    this.editing.stage() === 'list'
      ? 'Anotações'
      : this.editing.editing()
        ? 'Editar anotação'
        : 'Nova anotação',
  );
  /** What a screen reader hears after the filter changes. */
  protected readonly announce = signal('');
  protected readonly limitReason = 'Limite de 300 anotações. Apague uma para escrever outra.';

  private readonly frame = viewChild(SheetFrame);
  private readonly newButton = viewChild('newButton', { read: ElementRef<HTMLButtonElement> });
  private readonly keepButton = viewChild('keep', { read: ElementRef<HTMLButtonElement> });
  private opener: string | null = null;

  constructor() {
    this.watchKeyboard();
    // Each change of stage starts at the top of the sheet.
    effect(() => {
      this.editing.stage();
      untracked(() => this.frame()?.scrollToTop());
    });
    effect(() => {
      if (this.editing.confirmingDiscard()) {
        afterNextRender(() => this.keepButton()?.nativeElement.focus(), {
          injector: this.injector,
        });
      }
    });
  }

  ngOnInit(): void {
    this.state.seen();
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

  /** Back to the list: focus goes where the form was opened. */
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
  }

  protected closeSheet(): void {
    // The ✕ of a form is a "Cancelar": it asks before losing text.
    if (this.editing.stage() === 'form') {
      this.cancel();
      return;
    }
    this.sheet.close();
  }

  /**
   * On a phone the sheet is as tall as what is left above the keyboard: the
   * visual viewport shrinks when it opens (Android and iOS), and the sheet,
   * which sits at the bottom of the layout viewport, is lifted by what the
   * keyboard covers. The title and the footer stay on screen.
   */
  private watchKeyboard(): void {
    afterNextRender(
      () => {
        const view = this.document.defaultView;
        const viewport = view?.visualViewport;
        const container = this.host.nativeElement.closest<HTMLElement>(
          '.mat-bottom-sheet-container',
        );
        if (!view || !viewport || !container) {
          return;
        }
        const fit = () => {
          const covered = Math.max(0, view.innerHeight - viewport.height - viewport.offsetTop);
          container.style.maxHeight = covered > 0 ? `${viewport.height - 24}px` : '';
          container.style.marginBottom = covered > 0 ? `${covered}px` : '';
        };
        viewport.addEventListener('resize', fit);
        viewport.addEventListener('scroll', fit);
        fit();
        this.destroyRef.onDestroy(() => {
          viewport.removeEventListener('resize', fit);
          viewport.removeEventListener('scroll', fit);
          container.style.maxHeight = '';
          container.style.marginBottom = '';
        });
      },
      { injector: this.injector },
    );
  }
}
