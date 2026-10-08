import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  linkedSignal,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { CampaignDocument } from '../../../../gen/meurpg/campaigns/v1/campaign_document_pb';
import type { GalleryImage } from '../../../../gen/meurpg/maps/v1/gallery_pb';
import { FictionNotice } from '../../../shared/fiction-notice/fiction-notice';
import {
  MarkdownView,
  type MarkdownRefs,
  type RefOpen,
} from '../../../shared/markdown/markdown-view';
import { formatClock } from '../../../shared/session-time/session-time';
import { DocumentClient } from '../document-clients';
import { CONFLICT_MESSAGE, isConflict, saveErrorMessage } from '../document-copy';
import {
  type Edit,
  MAX_BODY_BYTES,
  applyEdit,
  bodyBytes,
  bytesNotice,
  insertImage,
  insertLink,
  normalizeBody,
  toggleHeading,
  toggleList,
  wrap,
} from '../edit-actions';
import { DocumentToolbar, type ToolbarAction } from '../document-toolbar/document-toolbar';
import { ImagePickerDialog } from '../image-picker-dialog/image-picker-dialog';
import { LinkPickerDialog, type PickedLink } from '../link-picker-dialog/link-picker-dialog';

type Pane = 'text' | 'preview';
type Picker = 'image' | 'map' | 'sheet';

/** How long after typing the preview catches up (README-B: about 150 ms). */
const PREVIEW_DELAY_MS = 150;

/**
 * The document's edit mode (E5-28): a plain `<textarea>` with a toolbar
 * that writes the syntax at the cursor, the live preview beside it (two
 * tabs, "Texto" and "Prévia", on a phone), the save status, "Salvar
 * documento" and "Descartar mudanças".
 *
 * The draft lives in memory only (no Web Storage); the page asks before
 * leaving with it unsaved (`dirty`). Saving sends the revision that was
 * read: `aborted` means somebody saved meanwhile, and the editor keeps the
 * draft on screen with "Recarregar" until the person chooses.
 */
@Component({
  selector: 'app-document-editor',
  imports: [
    DocumentToolbar,
    FictionNotice,
    ImagePickerDialog,
    LinkPickerDialog,
    MarkdownView,
    MatButtonModule,
    MatIconModule,
  ],
  templateUrl: './document-editor.html',
  styleUrl: './document-editor.scss',
})
export class DocumentEditor {
  private readonly client = inject(DocumentClient);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  /** The document as the server has it; a new one resets the draft. */
  readonly doc = input.required<CampaignDocument>();
  readonly refs = input<MarkdownRefs | null>(null);
  /** When the last version was saved, for "A última versão foi salva…". */
  readonly lastSaved = input('');

  readonly saved = output<CampaignDocument>();
  readonly discarded = output<void>();
  /** The person chose to drop their draft and read the server's version. */
  readonly reload = output<void>();
  readonly openRef = output<RefOpen>();
  /** The draft now points to something the page has not listed yet. */
  readonly refsStale = output<void>();

  protected readonly draft = linkedSignal(() => this.doc().body);
  protected readonly previewSource = linkedSignal(() => this.doc().body);
  protected readonly dirty = computed(() => normalizeBody(this.draft()) !== this.doc().body);
  protected readonly bytes = computed(() => bodyBytes(this.draft()));
  protected readonly overLimit = computed(() => this.bytes() > MAX_BODY_BYTES);
  protected readonly counter = computed(() => bytesNotice(this.bytes()));

  protected readonly pane = signal<Pane>('text');
  protected readonly picker = signal<Picker | null>(null);
  protected readonly saving = signal(false);
  protected readonly conflict = signal(false);
  protected readonly error = signal('');
  protected readonly confirmingDiscard = signal(false);
  protected readonly conflictMessage = CONFLICT_MESSAGE;
  protected readonly maxKb = Math.round(MAX_BODY_BYTES / 1024);

  private readonly textarea = viewChild.required<ElementRef<HTMLTextAreaElement>>('textarea');
  private readonly discardButton = viewChild('confirmDiscard', {
    read: ElementRef<HTMLButtonElement>,
  });
  private readonly conflictNotice = viewChild<ElementRef<HTMLElement>>('conflictNotice');
  private selection = { start: 0, end: 0 };
  private trigger: HTMLElement | null = null;
  private previewTimer: ReturnType<typeof setTimeout> | undefined;
  /** Set when the person left the page: a save still in flight must not emit or touch the view then. */
  private destroyed = false;

  constructor() {
    inject(DestroyRef).onDestroy(() => {
      this.destroyed = true;
      clearTimeout(this.previewTimer);
    });
  }

  /** Whether there is text the server does not have. */
  hasUnsavedChanges(): boolean {
    return this.dirty();
  }

  protected onInput(value: string): void {
    this.draft.set(value);
    this.error.set('');
    this.confirmingDiscard.set(false);
    clearTimeout(this.previewTimer);
    this.previewTimer = setTimeout(() => this.previewSource.set(this.draft()), PREVIEW_DELAY_MS);
  }

  // The toolbar --------------------------------------------------------

  protected onToolbar(event: { action: ToolbarAction; trigger: HTMLElement }): void {
    const ta = this.textarea().nativeElement;
    const start = ta.selectionStart;
    const end = ta.selectionEnd;
    const text = this.draft();
    switch (event.action) {
      case 'heading':
        this.apply(toggleHeading(text, start));
        break;
      case 'bold':
        this.apply(wrap(text, start, end, '**'));
        break;
      case 'italic':
        this.apply(wrap(text, start, end, '*'));
        break;
      case 'list':
        this.apply(toggleList(text, start, end));
        break;
      default:
        this.selection = { start, end };
        this.trigger = event.trigger;
        this.picker.set(event.action);
    }
  }

  protected closePicker(): void {
    this.picker.set(null);
    this.trigger?.focus();
    this.trigger = null;
  }

  protected onImage(image: GalleryImage): void {
    const { start, end } = this.selection;
    this.finishPicker(insertImage(this.draft(), start, end, image.name, image.id));
  }

  protected onLink(kind: 'map' | 'character', link: PickedLink): void {
    const { start, end } = this.selection;
    this.finishPicker(insertLink(this.draft(), start, end, link.name, `${kind}:${link.id}`));
  }

  private finishPicker(edit: Edit): void {
    this.picker.set(null);
    this.trigger = null;
    // After the dialog left the DOM: while it is open the page behind it is
    // inert and the text box cannot take focus.
    afterNextRender(() => this.apply(edit), { injector: this.injector });
    this.refsStale.emit();
  }

  /** Replaces the range in the text box. `execCommand('insertText')` keeps
   * the browser's undo history (Ctrl+Z undoes the toolbar too); where it is
   * not there, the value is set directly. */
  private apply(edit: Edit): void {
    // A save is on its way with the text as it was: what is typed now would be lost when its answer comes.
    if (this.saving()) {
      return;
    }
    const ta = this.textarea().nativeElement;
    const before = ta.value;
    const expected = applyEdit(before, edit);
    ta.focus();
    ta.setSelectionRange(edit.from, edit.to);
    let done = false;
    if (typeof document.execCommand === 'function') {
      try {
        done = document.execCommand('insertText', false, edit.insert) && ta.value === expected;
      } catch {
        done = false;
      }
    }
    if (!done) {
      ta.value = expected;
    }
    ta.setSelectionRange(edit.selStart, edit.selEnd);
    this.onInput(ta.value);
  }

  // Saving and leaving --------------------------------------------------

  protected save(): void {
    if (this.saving() || this.conflict() || this.overLimit() || !this.dirty()) {
      return;
    }
    this.saving.set(true);
    this.error.set('');
    const body = normalizeBody(this.draft());
    this.client.save(this.campaignId(), body, this.doc().revision).then(
      (doc) => {
        // An output emitted after its component died logs NG0953; the parent that wanted the news is gone too.
        if (this.destroyed) {
          return;
        }
        this.saving.set(false);
        this.saved.emit(doc);
      },
      (err: unknown) => {
        if (this.destroyed) {
          return;
        }
        this.saving.set(false);
        if (isConflict(err)) {
          this.conflict.set(true);
          afterNextRender(() => this.conflictNotice()?.nativeElement.focus(), {
            injector: this.injector,
          });
        } else {
          this.error.set(saveErrorMessage(err));
        }
      },
    );
  }

  protected discard(): void {
    if (this.dirty()) {
      this.confirmingDiscard.set(true);
      afterNextRender(() => this.discardButton()?.nativeElement.focus(), {
        injector: this.injector,
      });
    } else {
      this.discarded.emit();
    }
  }

  protected reloadFromServer(): void {
    this.conflict.set(false);
    this.reload.emit();
  }

  protected clock(date: Date): string {
    return formatClock(date);
  }
}
