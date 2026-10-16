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
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, RouterLink } from '@angular/router';

import type { TableEntry } from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { TableContentClient, contentErrorText } from '../../../core/content/content-client';
import { type ContentContext, loadContext } from '../../../core/content/content-context';
import {
  CONTENT_FILTERS,
  CONTENT_NAV,
  type ContentFilter,
  type ContentNavKind,
  KIND_WORDS,
  countOfNav,
  entryState,
  entrySupport,
  filterEntries,
  limitLine,
  navBySlug,
  KIND_NOUNS,
} from '../../../core/content/content-kinds';
import { type CatalogVm, catalogVm } from '../../../core/content/catalog';
import { packFileName, packToText } from '../../../core/content/content-pack';
import { saveTextFile } from '../../../core/content/save-file';
import { ContentWatcher } from '../../../core/content/content-watcher';
import { LiveSessionSourceLive } from '../../../core/play/live-session-source.live';
import { TextField } from '../../../shared/form-fields/text-field';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { mediaQuery, PHONE_QUERY } from '../../../shared/map-view/media-query';
import { openSheet } from '../../../shared/sheet/sheet-host';
import { ArchiveSheet } from '../archive/archive-sheet';

type PageState =
  | { status: 'loading' }
  | { status: 'not-found' }
  | { status: 'error'; message: string }
  | { status: 'ready'; ctx: ContentContext };

/**
 * "Conteúdo da mesa" (MR-025, RN-23; E10-01 states 1, 2, 9 and 10): the entries of the table's own content, by kind.
 * The master on a laptop has the menu of kinds with counts, the search, "Mostrar" and the rows with their state in words;
 * on a phone the master reads and archives only, as the page says. A player reads every entry that is on, in full, kind
 * by kind: the server already left out the archived ones and the counts, so nothing here hides them. Entries open in the
 * entry page (`ContentEntry`), the editor for the master and the read view for everyone else.
 */
@Component({
  selector: 'app-content-list',
  imports: [
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    RouterLink,
    SelectField,
    TextField,
  ],
  providers: [ContentWatcher, LiveSessionSourceLive],
  templateUrl: './content-list.html',
  styleUrl: './content-list.scss',
})
export class ContentList {
  private readonly campaigns = inject(CampaignsService);
  private readonly client = inject(TableContentClient);
  private readonly route = inject(ActivatedRoute);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly injector = inject(Injector);
  private readonly watcher = inject(ContentWatcher);
  private readonly title = viewChild<ElementRef<HTMLElement>>('title');

  protected readonly nav = CONTENT_NAV;
  protected readonly filters = CONTENT_FILTERS;
  protected readonly kindWords = KIND_WORDS;
  protected readonly phone = mediaQuery(PHONE_QUERY);

  protected readonly campaignId = signal('');
  protected readonly state = signal<PageState>({ status: 'loading' });
  /** The list opens on the first kind of the menu (every kind has an editor now). */
  protected readonly slug = signal<ContentNavKind['slug']>(
    CONTENT_NAV.find((n) => n.editable)?.slug ?? 'classes',
  );
  protected readonly query = signal('');
  protected readonly filter = signal<ContentFilter>('all');
  protected readonly catalog = signal<CatalogVm | null>(null);
  /** What the last archive or unarchive did, for a screen reader and the eye. */
  protected readonly status = signal('');
  protected readonly actionError = signal('');
  protected readonly busyKey = signal('');
  protected readonly exporting = signal(false);

  protected readonly ctx = computed(() => {
    const s = this.state();
    return s.status === 'ready' ? s.ctx : null;
  });
  protected readonly entries = computed(() => this.ctx()?.entries ?? []);
  protected readonly isMaster = computed(() => this.ctx()?.isMaster ?? false);
  protected readonly current = computed(() => navBySlug(this.slug()) ?? CONTENT_NAV[0]);
  protected readonly counts = computed(
    () => new Map(CONTENT_NAV.map((n) => [n.slug, countOfNav(this.entries(), n)])),
  );
  protected readonly rows = computed(() =>
    filterEntries(this.entries(), this.current(), this.query(), this.filter()),
  );
  protected readonly limit = computed(() => limitLine(this.entries()));
  protected readonly empty = computed(() => this.entries().length === 0);
  /** The player's panels: only the kinds that have something. */
  protected readonly panels = computed(() =>
    CONTENT_NAV.map((n) => ({ nav: n, rows: filterEntries(this.entries(), n, '', 'all') })).filter(
      (p) => p.rows.length > 0,
    ),
  );
  protected readonly kindOptions = computed<SelectOption[]>(() =>
    CONTENT_NAV.map((n) => ({ value: n.slug, label: n.plural })),
  );
  protected readonly filterOptions: SelectOption[] = CONTENT_FILTERS.map((f) => ({
    value: f.value,
    label: f.label,
  }));

  constructor() {
    this.route.paramMap.pipe(takeUntilDestroyed(inject(DestroyRef))).subscribe((params) => {
      const id = params.get('id');
      if (id) {
        this.campaignId.set(id);
        void this.load();
      }
    });
    this.route.queryParamMap.pipe(takeUntilDestroyed(inject(DestroyRef))).subscribe((params) => {
      const nav = navBySlug(params.get('kind'));
      if (nav) {
        this.slug.set(nav.slug);
        this.query.set('');
        this.filter.set('all');
      }
    });
    // The table changed (an entry written, a switch turned): read the list again with this person's role, with no spinner.
    this.watcher.whileLive(this.campaignId, () => void this.refresh());
  }

  /** The list again after a `content_changed`: the rows, never the page's state (the search, the kind, an open sheet stay). */
  protected async refresh(): Promise<void> {
    if (this.state().status !== 'ready') {
      return;
    }
    try {
      const res = await loadContext(this.campaigns, this.client, this.campaignId());
      if (res.status === 'ok' && this.state().status === 'ready') {
        this.state.set({ status: 'ready', ctx: res.ctx });
        void this.client.catalog(this.campaignId()).then(
          (content) => this.catalog.set(catalogVm(content, res.ctx.entries)),
          () => undefined,
        );
      }
    } catch {
      // Keep what is on screen: the next change reads again.
    }
  }

  protected async load(): Promise<void> {
    this.state.set({ status: 'loading' });
    try {
      const res = await loadContext(this.campaigns, this.client, this.campaignId());
      if (res.status === 'not-found') {
        this.state.set({ status: 'not-found' });
        return;
      }
      this.state.set({ status: 'ready', ctx: res.ctx });
      // The names a subclass or a subrace row says ("Subclasse de Mago") come from the catalog.
      void this.client.catalog(this.campaignId()).then(
        (content) => this.catalog.set(catalogVm(content, res.ctx.entries)),
        () => undefined,
      );
      afterNextRender(() => this.title()?.nativeElement.focus({ preventScroll: true }), {
        injector: this.injector,
      });
    } catch (err) {
      this.state.set({
        status: 'error',
        message: contentErrorText(err, 'abrir o conteúdo da mesa'),
      });
    }
  }

  /** "Exportar": the campaign's own entries as a file the master keeps (`<campanha>-conteudo.json`). */
  protected async exportPack(): Promise<void> {
    const ctx = this.ctx();
    if (!ctx || this.exporting()) {
      return;
    }
    this.actionError.set('');
    this.status.set('');
    this.exporting.set(true);
    try {
      const pack = await this.client.exportPack(this.campaignId());
      const name = packFileName(ctx.campaignName);
      saveTextFile(name, packToText(pack));
      this.status.set(`O pacote foi salvo no arquivo ${name}.`);
    } catch (err) {
      this.actionError.set(contentErrorText(err, 'exportar o conteúdo'));
    } finally {
      this.exporting.set(false);
    }
  }

  protected link(e: TableEntry): string[] {
    return ['/campaigns', this.campaignId(), 'content', 'entries', e.key];
  }

  protected support(e: TableEntry): string {
    const cat = this.catalog();
    return entrySupport(
      e,
      (key) =>
        cat?.nameOf(key) ??
        this.entries().find((x) => x.key === key)?.namePt ??
        key.replace(/^[a-z]+:/, ''),
      (classKey) => cat?.subclassLevelOf(classKey) ?? 0,
    );
  }

  protected stateOf(e: TableEntry) {
    return entryState(e, this.isMaster());
  }

  protected newLink(n: ContentNavKind): string[] {
    return ['/campaigns', this.campaignId(), 'content', 'new', n.createSegment];
  }

  /** "Nova sub-raça" beside "Nova raça" in the Raças list: the race is picked on the page. */
  protected newSubraceLink(): string[] {
    return ['/campaigns', this.campaignId(), 'content', 'new', 'subrace'];
  }

  protected setQuery(text: string): void {
    this.query.set(text);
  }

  protected setFilter(value: string): void {
    this.filter.set(value as ContentFilter);
  }

  protected pickKind(slug: string): void {
    const nav = navBySlug(slug);
    if (nav) {
      this.slug.set(nav.slug);
      this.query.set('');
      this.filter.set('all');
    }
  }

  /** Phone: archive asks in a bottom sheet; "Desarquivar" comes back at once (nothing to lose). */
  protected async toggleArchive(e: TableEntry): Promise<void> {
    this.actionError.set('');
    if (!e.archived) {
      const confirmed = await new Promise<boolean>((resolve) =>
        openSheet<ArchiveSheet, { name: string; using: number }, boolean>(
          this.dialog,
          this.bottomSheet,
          ArchiveSheet,
          {
            data: { name: e.namePt, using: e.charactersUsing },
            ariaLabel: `Arquivar ${e.namePt}?`,
            labelledBy: 'archive-t',
          },
        ).subscribe((r) => resolve(r === true)),
      );
      if (!confirmed) {
        return;
      }
    }
    this.busyKey.set(e.key);
    try {
      const entry = e.archived
        ? await this.client.unarchive(this.campaignId(), e.key)
        : await this.client.archive(this.campaignId(), e.key);
      this.replace(entry);
      const noun = KIND_NOUNS[entry.kind];
      this.status.set(
        entry.archived
          ? `${noun.article} ${noun.noun} ${entry.namePt} foi ${noun.archived}.`
          : `${noun.article} ${noun.noun} ${entry.namePt} voltou.`,
      );
    } catch (err) {
      this.actionError.set(contentErrorText(err, e.archived ? 'desarquivar' : 'arquivar'));
    } finally {
      this.busyKey.set('');
    }
  }

  private replace(entry: TableEntry): void {
    const s = this.state();
    if (s.status !== 'ready') {
      return;
    }
    this.state.set({
      status: 'ready',
      ctx: { ...s.ctx, entries: s.ctx.entries.map((x) => (x.key === entry.key ? entry : x)) },
    });
  }
}
