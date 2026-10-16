import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import type { CampaignDocument } from '../../../gen/meurpg/campaigns/v1/campaign_document_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { describeConnectError } from '../../core/connect/connect-errors';
import { GalleryClient } from '../../core/images/gallery-client';
import type { MarkdownRefs, RefOpen } from '../../shared/markdown/markdown-view';
import { formatClock } from '../../shared/session-time/session-time';
import { DocDialog } from './doc-dialog/doc-dialog';
import { DocumentClient, DocumentLinks } from '../../shared/campaign-document/document-clients';
import { DocumentEditor } from './document-editor/document-editor';
import { editedLine, whenText } from '../../shared/campaign-document/document-format';
import { DocumentRead } from './document-read/document-read';
import { DocumentMapDialog } from './map-dialog/map-dialog';
import { DocumentSheetDialog } from './sheet-dialog/sheet-dialog';

type PageState =
  | { status: 'loading' }
  | { status: 'not-found' }
  | { status: 'forbidden' }
  | { status: 'error'; message: string }
  | { status: 'ready' };

/**
 * "/campaigns/:id/document" (MR-018): the campaign's document, for its
 * master only (the default answer to question 27: it holds prep notes and
 * spoilers). E5-27 (read), E5-28 (edit), E5-29 (a map link in a dialog) and
 * E5-30 (the phone).
 *
 * A player gets `permission_denied` and a calm "Só o mestre vê o documento
 * da campanha."; a non-member and a campaign that does not exist both get
 * `not_found` and the same "Campanha não encontrada" (ADR-0011).
 *
 * The page owns the document and the mode. Leaving edit mode with unsaved
 * text asks first (the route's `canDeactivate` calls `confirmLeave`): never a
 * `beforeunload` dialog. The links in the text resolve through the
 * ordinary maps, characters and gallery calls (`refs`), so a link to
 * something deleted shows "(mapa apagado)" and an image "Imagem apagada".
 */
@Component({
  selector: 'app-campaign-document',
  imports: [
    DocDialog,
    DocumentEditor,
    DocumentMapDialog,
    DocumentRead,
    DocumentSheetDialog,
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    RouterLink,
  ],
  templateUrl: './campaign-document.html',
  styleUrl: './campaign-document.scss',
})
export class CampaignDocumentPage {
  private readonly documents = inject(DocumentClient);
  private readonly links = inject(DocumentLinks);
  private readonly gallery = inject(GalleryClient);
  private readonly campaigns = inject(CampaignsService);
  private readonly route = inject(ActivatedRoute);
  private readonly injector = inject(Injector);

  protected readonly campaignId = signal('');
  protected readonly campaignName = signal('');
  protected readonly state = signal<PageState>({ status: 'loading' });
  protected readonly doc = signal<CampaignDocument | null>(null);
  protected readonly mode = signal<'read' | 'edit'>('read');
  protected readonly refs = signal<MarkdownRefs | null>(null);
  protected readonly savedAt = signal('');
  protected readonly opened = signal<RefOpen | null>(null);
  protected readonly leaving = signal(false);
  protected readonly announcement = signal('');

  protected readonly title = computed(() =>
    this.campaignName() ? `${this.campaignName()} — preparação` : 'Documento da campanha',
  );
  protected readonly edited = computed(() => {
    const doc = this.doc();
    return doc
      ? editedLine(doc.updatedAt ? timestampDate(doc.updatedAt) : null, doc.updatedByDisplayName)
      : '';
  });
  /** "A última versão foi salva ontem às 22:10." under the draft status. */
  protected readonly lastSaved = computed(() => {
    const at = this.doc()?.updatedAt;
    return at ? `A última versão foi salva ${whenText(timestampDate(at))}.` : '';
  });

  private readonly editor = viewChild(DocumentEditor);
  private readonly editButton = viewChild('editButton', { read: ElementRef<HTMLButtonElement> });
  private trigger: HTMLElement | null = null;
  private leaveResolve: ((leave: boolean) => void) | null = null;

  constructor() {
    this.route.paramMap.pipe(takeUntilDestroyed()).subscribe((params) => {
      const id = params.get('id');
      if (id) {
        this.campaignId.set(id);
        this.load(id);
      }
    });
    inject(DestroyRef).onDestroy(() => this.leaveResolve?.(true));
  }

  protected load(campaignId = this.campaignId()): void {
    this.state.set({ status: 'loading' });
    this.campaigns.getCampaign(campaignId).then(
      (res) => this.campaignName.set(res.campaign?.name ?? ''),
      () => undefined,
    );
    this.documents.get(campaignId).then(
      (doc) => {
        this.doc.set(doc);
        this.state.set({ status: 'ready' });
        this.loadRefs();
      },
      (err: unknown) => {
        const code = ConnectError.from(err, Code.Unavailable).code;
        if (code === Code.NotFound) {
          this.state.set({ status: 'not-found' });
        } else if (code === Code.PermissionDenied) {
          this.state.set({ status: 'forbidden' });
        } else {
          this.state.set({ status: 'error', message: describeConnectError(err, {}) });
        }
      },
    );
  }

  /** What exists in the campaign, so links to deleted things say so. One
   * list that fails leaves its links shown as usual. */
  protected loadRefs(): void {
    const id = this.campaignId();
    void Promise.allSettled([
      this.links.listMaps(id),
      this.links.listCharacters(id),
      this.gallery.list(id),
    ]).then(([maps, characters, gallery]) => {
      this.refs.set({
        maps: maps.status === 'fulfilled' ? new Set(maps.value.map((m) => m.id)) : null,
        characters:
          characters.status === 'fulfilled' ? new Set(characters.value.map((c) => c.id)) : null,
        images:
          gallery.status === 'fulfilled'
            ? new Map(gallery.value.images.map((i) => [i.id, { width: i.width, height: i.height }]))
            : null,
      });
    });
  }

  // Modes ----------------------------------------------------------------

  protected edit(): void {
    this.mode.set('edit');
    this.savedAt.set('');
    this.loadRefs();
    afterNextRender(() => document.getElementById('doc-text')?.focus(), {
      injector: this.injector,
    });
  }

  protected onSaved(doc: CampaignDocument): void {
    this.doc.set(doc);
    this.savedAt.set(
      `Salvo às ${formatClock(doc.updatedAt ? timestampDate(doc.updatedAt) : new Date())}`,
    );
    this.announce(this.savedAt());
    this.backToReading();
  }

  protected onDiscarded(): void {
    this.backToReading();
  }

  /** "Recarregar" after a conflict: the server's version replaces the draft. */
  protected reload(): void {
    this.documents.get(this.campaignId()).then(
      (doc) => {
        this.doc.set(doc);
        this.loadRefs();
      },
      (err: unknown) => this.announce(describeConnectError(err, {})),
    );
  }

  private backToReading(): void {
    this.mode.set('read');
    this.loadRefs();
    afterNextRender(() => this.editButton()?.nativeElement.focus(), { injector: this.injector });
  }

  // Links ---------------------------------------------------------------

  protected openRef(ref: RefOpen): void {
    this.trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    this.opened.set(ref);
  }

  protected closeRef(): void {
    this.opened.set(null);
    this.trigger?.focus();
    this.trigger = null;
  }

  // Leaving with unsaved text -------------------------------------------

  /** For the route guard: true when it is fine to leave. */
  confirmLeave(): boolean | Promise<boolean> {
    if (this.mode() !== 'edit' || !this.editor()?.hasUnsavedChanges()) {
      return true;
    }
    this.trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    this.leaving.set(true);
    return new Promise<boolean>((resolve) => {
      this.leaveResolve = resolve;
    });
  }

  protected answerLeave(leave: boolean): void {
    this.leaving.set(false);
    const resolve = this.leaveResolve;
    this.leaveResolve = null;
    resolve?.(leave);
    if (!leave) {
      this.trigger?.focus();
    }
    this.trigger = null;
  }

  private announce(text: string): void {
    this.announcement.set('');
    setTimeout(() => this.announcement.set(text), 50);
  }
}
