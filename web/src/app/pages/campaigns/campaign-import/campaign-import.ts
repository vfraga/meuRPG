import {
  ChangeDetectionStrategy,
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
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { RouterLink } from '@angular/router';

import type {
  CampaignImport,
  PackageCounts,
  PreviewCampaignImportResponse,
} from '../../../../gen/meurpg/campaignpackage/v1/campaignpackage_pb';
import { CampaignPackageClient } from '../../../core/campaign-package/campaign-package-client';
import { ImportPartUploader } from '../../../core/campaign-package/import-part-uploader';
import { type PartsProgress, sendParts } from '../../../core/campaign-package/import-upload';
import {
  PROBLEMS_SHOWN,
  beginName,
  countRows,
  createdSentenceTail,
  fileRefusalText,
  fingerprintOf,
  importUploadFailureText,
  keptUploadText,
  moreProblemsText,
  precheckPackageFile,
  previewHeadline,
  uploadProgressText,
} from '../../../core/campaign-package/package-copy';
import { type ImportIssue, importIssue } from '../../../core/campaign-package/package-errors';
import { hasNewerVersion, problemSentence } from '../../../core/campaign-package/package-problems';
import { describeConnectError } from '../../../core/connect/connect-errors';
import { ActionKey } from '../../../core/connect/idempotency';
import { formatBytes } from '../../../core/images/image-format';
import { UploadFailed } from '../../../core/images/upload-errors';
import { creationRefusalText } from '../../../core/campaigns/campaign-copy';

/** Where a failure happened, which is where "Tentar de novo" picks up: before the package was read, or when creating. */
type Stage = 'upload' | 'create';

interface Failure {
  readonly title: string;
  readonly cause: string;
  /** Nothing was made on the server: said in words. */
  readonly createdNothing: boolean;
  /** The server could not say whether the campaign was made: "Tentar de novo" is safe, the key sees to it. */
  readonly unknown: boolean;
  readonly stage: Stage;
}

type ImportState =
  | { status: 'loading' }
  | { status: 'choose'; kept: string; notice: string }
  | { status: 'uploading'; fileName: string; size: number; canceling: boolean }
  | { status: 'reading' }
  | { status: 'preview'; preview: PreviewCampaignImportResponse }
  | { status: 'refused'; preview: PreviewCampaignImportResponse }
  | { status: 'newer' }
  | { status: 'creating' }
  | { status: 'cap'; max: number }
  | { status: 'not-allowed'; text: string }
  | { status: 'failed'; failure: Failure }
  | { status: 'done'; campaignId: string; name: string; counts: PackageCounts | undefined };

const NOT_ALLOWED_FALLBACK =
  'Este servidor só deixa algumas pessoas criarem campanhas. Peça ao mestre da sua mesa um convite para jogar.';

/**
 * "/campaigns/import" (MR-050, PM-09 state 2): make a NEW campaign, with the caller as its master, from a
 * `.meurpg.zip` file. The steps, each a card of the board:
 *
 * 1. choose the file (a drop area and "Escolher o arquivo"); a file over 200 MB, or one that is not a `.zip`,
 *    is refused here with a friendly message, before a byte is sent;
 * 2. upload in parts, one after another (`sendParts`), skipping the parts the server already holds, so a
 *    dropped connection, or choosing the same file again after closing the page, carries on where it stopped;
 * 3. the preview (`PreviewCampaignImport`): what the package holds, or every problem in plain words with only
 *    "Escolher outro arquivo" (all or nothing: with any problem there is no "Criar campanha"), or the card of a
 *    package from a newer MeuRPG;
 * 4. "Criar campanha" (`CreateCampaignFromImport`, with an `ActionKey`: a retry is the same creation), then
 *    "Campanha criada", or the campaign-cap card, or the failure card whose "Tentar de novo" sends what is
 *    missing and creates again with the same key.
 *
 * Nothing is kept in Web Storage: a page that opens with an upload still on the server (`GetCampaignImport`)
 * offers "Continuar o envio", which is choosing the same file again (the fingerprint is the file's name, size
 * and last-modified time). The progress text is a polite live region and changes once per part.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-campaign-import',
  imports: [MatButtonModule, MatIconModule, MatProgressSpinnerModule, RouterLink],
  templateUrl: './campaign-import.html',
  styleUrl: './campaign-import.scss',
})
export class CampaignImportPage {
  private readonly client = inject(CampaignPackageClient);
  private readonly uploader = inject(ImportPartUploader);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly createKey = new ActionKey();

  /** The file in memory, for "Tentar de novo"; gone with the page. */
  private file: File | null = null;
  /** The upload on the server that this page began or took up. */
  private upload: CampaignImport | null = null;
  private abort: AbortController | null = null;
  private gone = false;
  /** A creation is in flight: a second tap on "Criar campanha" must not start another. */
  private creating = false;

  protected readonly state = signal<ImportState>({ status: 'loading' });
  protected readonly progress = signal<PartsProgress | null>(null);
  protected readonly dragging = signal(false);
  /** The polite live region for what the cards do not say themselves (the preview is ready, the campaign made). */
  protected readonly announcement = signal('');

  protected readonly fileInput = viewChild.required<ElementRef<HTMLInputElement>>('fileInput');

  protected readonly progressText = computed(() => {
    const p = this.progress();
    return p ? uploadProgressText(p.part, p.partCount) : 'Preparando o envio…';
  });
  protected readonly barPercent = computed(() =>
    Math.round(100 * (this.progress()?.fraction ?? 0)),
  );

  private readonly shownPreview = computed(() => {
    const s = this.state();
    return s.status === 'preview' || s.status === 'refused' ? s.preview : null;
  });
  protected readonly rows = computed(() => countRows(this.shownPreview()?.counts));
  protected readonly headline = computed(() => {
    const preview = this.shownPreview();
    return preview ? previewHeadline(preview) : '';
  });
  protected readonly problems = computed(() =>
    (this.shownPreview()?.problems ?? []).slice(0, PROBLEMS_SHOWN).map(problemSentence),
  );
  protected readonly moreProblems = computed(() => {
    const hidden = (this.shownPreview()?.problems.length ?? 0) - PROBLEMS_SHOWN;
    return hidden > 0 ? moreProblemsText(hidden) : '';
  });

  protected readonly formatBytes = formatBytes;
  protected readonly createdTail = createdSentenceTail;

  constructor() {
    inject(DestroyRef).onDestroy(() => {
      this.gone = true;
      this.abort?.abort();
    });
    void this.loadKept();
  }

  /** Opens on "Escolher o arquivo", with the upload the server still holds offered first (`GetCampaignImport`). */
  private async loadKept(): Promise<void> {
    try {
      const { upload } = await this.client.getImport();
      this.openChoose({ status: 'choose', kept: upload ? keptUploadText(upload) : '', notice: '' });
    } catch (err) {
      this.openChoose({ status: 'choose', kept: '', notice: describeConnectError(err, {}) });
    }
  }

  /**
   * Shows the first card once the kept-upload read ends, unless the person has already moved on: the file input
   * is there while this loads, so a file chosen (and refused, or sent) before the answer arrives must not be
   * wiped by that late answer.
   */
  private openChoose(state: ImportState): void {
    if (this.state().status === 'loading') {
      this.go(state);
    }
  }

  /** Sets the state and moves the focus to the new card's `data-focus` element (the button, or the heading). */
  private go(state: ImportState, announce = ''): void {
    if (this.gone) {
      return;
    }
    this.state.set(state);
    this.announcement.set(announce);
    afterNextRender(
      () => this.host.nativeElement.querySelector<HTMLElement>('[data-focus]')?.focus(),
      { injector: this.injector },
    );
  }

  protected pick(): void {
    this.fileInput().nativeElement.click();
  }

  protected onPicked(input: HTMLInputElement): void {
    const file = input.files?.[0];
    // Clearing the value lets the same file be chosen again (after a refusal, or to resume).
    input.value = '';
    if (file) {
      void this.choose(file);
    }
  }

  protected onDragOver(event: DragEvent): void {
    event.preventDefault();
    this.dragging.set(true);
  }

  protected onDragLeave(): void {
    this.dragging.set(false);
  }

  protected onDrop(event: DragEvent): void {
    event.preventDefault();
    this.dragging.set(false);
    const file = event.dataTransfer?.files?.[0];
    if (file) {
      void this.choose(file);
    }
  }

  /** "Escolher outro arquivo": back to the first card. */
  protected again(): void {
    this.go({ status: 'choose', kept: '', notice: '' });
  }

  private async choose(file: File): Promise<void> {
    const refusal = precheckPackageFile(file);
    if (refusal) {
      this.go({ status: 'choose', kept: '', notice: fileRefusalText(refusal) });
      return;
    }
    this.file = file;
    await this.send(file, 'preview');
  }

  /**
   * Begins the upload (the same fingerprint takes up the one the server holds), sends the parts it lacks, then
   * reads the package (`after` is 'preview') or creates the campaign at once (a retry after a failed creation).
   */
  private async send(file: File, after: 'preview' | 'create'): Promise<void> {
    const abort = new AbortController();
    this.abort = abort;
    this.progress.set(null);
    this.go({ status: 'uploading', fileName: file.name, size: file.size, canceling: false });
    try {
      const { upload } = await this.client.beginImport(
        beginName(file),
        file.size,
        fingerprintOf(file),
      );
      if (!upload) {
        throw new Error('BeginCampaignImport answered without the upload');
      }
      this.upload = upload;
      await sendParts(
        {
          importId: upload.id,
          file,
          partSize: upload.partSize,
          partCount: upload.partCount,
          received: upload.receivedParts,
        },
        {
          uploader: this.uploader,
          signal: abort.signal,
          onProgress: (p) => this.progress.set(p),
        },
      );
    } catch (err) {
      await this.uploadFailed(err, abort);
      return;
    }
    if (abort.signal.aborted) {
      await this.discard();
      return;
    }
    if (after === 'create') {
      await this.create();
    } else {
      await this.read();
    }
  }

  private async uploadFailed(err: unknown, abort: AbortController): Promise<void> {
    if (this.gone) {
      return;
    }
    if (abort.signal.aborted) {
      await this.discard();
      return;
    }
    if (err instanceof UploadFailed) {
      this.go({
        status: 'failed',
        failure: {
          title: 'O envio não terminou.',
          cause: importUploadFailureText(err.kind),
          createdNothing: false,
          unknown: false,
          stage: 'upload',
        },
      });
      return;
    }
    this.handleIssue(err, 'upload');
  }

  /** "Cancelar o envio": stops the part in flight, then asks the server to drop the upload and its parts. */
  protected cancelUpload(): void {
    const s = this.state();
    if (s.status === 'uploading' && !s.canceling) {
      this.state.set({ ...s, canceling: true });
      this.abort?.abort();
    }
  }

  private async discard(): Promise<void> {
    const upload = this.upload;
    this.upload = null;
    this.file = null;
    let notice = '';
    if (upload) {
      try {
        await this.client.cancelImport(upload.id);
      } catch (err) {
        notice = describeConnectError(err, {});
      }
    }
    this.go({ status: 'choose', kept: '', notice }, notice === '' ? 'Envio cancelado.' : '');
  }

  private async read(): Promise<void> {
    const upload = this.upload;
    if (!upload) {
      return;
    }
    this.go({ status: 'reading' });
    try {
      const preview = await this.client.previewImport(upload.id);
      this.showPreview(preview);
    } catch (err) {
      this.handleIssue(err, 'upload');
    }
  }

  /** The preview, or the card of a package that cannot be created (or is from a newer MeuRPG). */
  private showPreview(preview: PreviewCampaignImportResponse): void {
    if (hasNewerVersion(preview.problems)) {
      this.go({ status: 'newer' }, 'Este pacote é de uma versão mais nova do MeuRPG.');
    } else if (preview.problems.length > 0) {
      this.go({ status: 'refused', preview }, 'Este pacote não pode ser criado.');
    } else {
      this.go({ status: 'preview', preview }, 'Prévia do pacote pronta.');
    }
  }

  protected async create(): Promise<void> {
    const upload = this.upload;
    if (!upload || this.creating) {
      return;
    }
    this.creating = true;
    this.go({ status: 'creating' });
    try {
      // One key per import: "Tentar de novo" after a lost answer is the same creation, and makes one campaign.
      const res = await this.client.createFromImport(upload.id, this.createKey.keyFor([upload.id]));
      this.createKey.renew();
      const campaign = res.campaign;
      if (!campaign) {
        throw new Error('CreateCampaignFromImport answered without the campaign');
      }
      this.upload = null;
      this.file = null;
      this.go(
        { status: 'done', campaignId: campaign.id, name: campaign.name, counts: res.counts },
        'Campanha criada.',
      );
    } catch (err) {
      this.handleIssue(err, 'create');
    } finally {
      this.creating = false;
    }
  }

  /** "Tentar de novo": sends what the server lacks (all of it if the upload expired) and goes on from the stage that failed. */
  protected async retry(): Promise<void> {
    const s = this.state();
    const file = this.file;
    if (s.status !== 'failed' || !file) {
      this.again();
      return;
    }
    await this.send(file, s.failure.stage === 'create' ? 'create' : 'preview');
  }

  private handleIssue(err: unknown, stage: Stage): void {
    const issue = importIssue(err);
    switch (issue.kind) {
      case 'cap':
        this.go({ status: 'cap', max: issue.max });
        return;
      case 'not-allowed':
        this.go({ status: 'not-allowed', text: creationRefusalText(err) ?? NOT_ALLOWED_FALLBACK });
        return;
      case 'problems':
        this.showPreview(issue.preview);
        return;
      default:
        if (issue.kind === 'text' && !issue.retryable && stage === 'upload') {
          // The server refused the file itself (empty name, too big, no blob store): another file, not another try.
          this.go({ status: 'choose', kept: '', notice: issue.text });
          return;
        }
        this.go({ status: 'failed', failure: failureFor(issue, stage) });
    }
  }
}

/** The failure card for an issue that is not a refusal with its own card. */
function failureFor(issue: Extract<ImportIssue, { kind: 'text' | 'lost' }>, stage: Stage): Failure {
  if (issue.kind === 'lost') {
    return {
      title: 'O envio não existe mais.',
      cause: 'As partes ficam guardadas só por 1 hora, e esse prazo acabou.',
      createdNothing: stage === 'create',
      unknown: false,
      stage,
    };
  }
  if (issue.unknown) {
    return {
      title: 'Não deu para confirmar se a campanha foi criada.',
      cause: issue.text,
      createdNothing: false,
      unknown: true,
      stage,
    };
  }
  return {
    title:
      stage === 'create'
        ? 'A importação falhou no meio e nada foi criado.'
        : 'Não deu para conferir o pacote.',
    cause: issue.text,
    createdNothing: stage === 'create',
    unknown: false,
    stage,
  };
}
