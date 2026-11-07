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
import { MatMenuModule } from '@angular/material/menu';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';

import {
  type AffectedCharacter,
  type GetClassTableDefaultsResponse,
  TableContentKind,
  type TableEntry,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { type CatalogVm, catalogVm } from '../../../core/content/catalog';
import { TableContentClient, contentErrorText } from '../../../core/content/content-client';
import { type ContentContext, loadContext } from '../../../core/content/content-context';
import {
  type ContentNavKind,
  entryState,
  navOfKind,
  savedSentence,
} from '../../../core/content/content-kinds';
import { ContentWatcher } from '../../../core/content/content-watcher';
import { EffectMenuVm } from '../../../core/content/effect-draft';
import { LiveSessionSourceLive } from '../../../core/play/live-session-source.live';
import { mediaQuery, PHONE_QUERY } from '../../../shared/map-view/media-query';
import { ArchiveQuestion } from '../archive/archive-question';
import { BackgroundEditor } from '../background-editor/background-editor';
import { EntryRead } from '../entry-read/entry-read';
import { FeatEditor } from '../feat-editor/feat-editor';
import { ClassFeatureEditor } from '../class-feature-editor/class-feature-editor';
import { CreatureEditor } from '../creature-editor/creature-editor';
import { CreatureRead } from '../creature-read/creature-read';
import { OptionEditor } from '../option-editor/option-editor';
import { RaceEditor } from '../race-editor/race-editor';
import { type EditorSaved, SpellEditor } from '../spell-editor/spell-editor';
import { ClassEditor } from '../class-editor/class-editor';
import {
  EDITABLE,
  NEW_KINDS,
  entriesAfterSave,
  extrasErrorOf,
  kindTagOf,
  usageOf,
  extrasErrors,
  backLabelOf,
  subraceParentOf,
  wireMenu,
  archivedDay,
  archivedWordOf,
  confirmArchiveInSheet,
  headingOf,
  mergeRefreshed,
  nameResolver,
  nounPhraseOf,
  readOfEntry,
  replaceEntry,
  updatedLine,
  usesNoMenu,
} from './content-entry-model';
import { SubclassEditor } from '../subclass-editor/subclass-editor';

const NO_READ = { rows: [], sections: [], text: [] };

type PageState =
  | { status: 'loading' }
  | { status: 'not-found' }
  | { status: 'error'; message: string }
  | { status: 'ready'; ctx: ContentContext };

/**
 * One entry of the table's content (MR-025, RN-23; E10-01 states 4 to 9): the master's editor for a spell, a race, a subrace
 * and a background on a laptop, and the read view for everything else (a player; the master on a phone, where it reads and
 * archives only; the master's classes and subclasses until 10.12). The header carries the state in words, "Arquivar" (behind
 * the "⋮" on a phone) and its question in place (a bottom sheet on a phone), and, after a write, what changed for the sheets
 * that use the entry. Nothing is ever deleted: an archived entry says so, offers "Desarquivar", and can still be edited.
 */
@Component({
  selector: 'app-content-entry',
  imports: [
    ArchiveQuestion,
    BackgroundEditor,
    ClassEditor,
    EntryRead,
    FeatEditor,
    OptionEditor,
    ClassFeatureEditor,
    CreatureEditor,
    CreatureRead,
    MatButtonModule,
    MatIconModule,
    MatMenuModule,
    MatProgressSpinnerModule,
    RaceEditor,
    RouterLink,
    SpellEditor,
    SubclassEditor,
  ],
  providers: [ContentWatcher, LiveSessionSourceLive],
  templateUrl: './content-entry.html',
  styleUrl: './content-entry.scss',
})
export class ContentEntry {
  private readonly campaigns = inject(CampaignsService);
  private readonly client = inject(TableContentClient);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly injector = inject(Injector);
  private readonly watcher = inject(ContentWatcher);
  private readonly title = viewChild<ElementRef<HTMLElement>>('title');

  protected readonly Kind = TableContentKind;
  protected readonly phone = mediaQuery(PHONE_QUERY);

  protected readonly campaignId = signal('');
  protected readonly state = signal<PageState>({ status: 'loading' });
  protected readonly catalog = signal<CatalogVm | null>(null);
  protected readonly menu = signal<EffectMenuVm | null>(null);
  /** The numbers the class and subclass editors start from (`GetClassTableDefaults`), for the master. */
  protected readonly defaults = signal<GetClassTableDefaultsResponse | null>(null);
  /** The key of the entry in the URL; empty for a new one. */
  protected readonly key = signal('');
  /** The kind of a new entry, from the URL. */
  protected readonly newKind = signal<TableContentKind | null>(null);
  protected readonly parentKey = signal('');
  /** A new subclass: the class it is for, from the link on the class's page. */
  protected readonly parentClass = signal('');
  protected readonly asking = signal(false);
  protected readonly archiving = signal(false);
  protected readonly status = signal('');
  /** "Magia Lâmina de Nanquim salva.": the line a write leaves under the title, until the next thing happens. */
  protected readonly savedLine = signal('');
  protected readonly affected = signal<readonly AffectedCharacter[]>([]);
  protected readonly actionError = signal('');
  /** The catalog or the effect menu did not come: the page says so, with "Tentar de novo". */
  /** Moves with each read of the entries: of two that overlap, only the newest lands. */
  private reads = 0;
  private readonly catalogError = signal('');
  private readonly menuError = signal('');
  /** What blocks this entry: the catalog always, the menu for every editor but the spell's (which does not use it), the defaults
   * for the class and the subclass. */
  protected readonly extrasError = computed(() =>
    extrasErrorOf({
      catalog: this.catalogError(),
      menu: this.menuError(),
      defaults: this.defaultsError(),
      noMenu: usesNoMenu(this.kind()),
      needsDefaults: this.needsDefaults(),
    }),
  );
  private readonly defaultsError = signal('');
  protected readonly needsDefaults = computed(
    () =>
      this.editing() &&
      (this.kind() === TableContentKind.CLASS || this.kind() === TableContentKind.SUBCLASS),
  );

  protected readonly ctx = computed(() => {
    const s = this.state();
    return s.status === 'ready' ? s.ctx : null;
  });
  protected readonly isMaster = computed(() => this.ctx()?.isMaster ?? false);
  protected readonly entry = computed<TableEntry | null>(
    () => this.ctx()?.entries.find((e) => e.key === this.key()) ?? null,
  );
  protected readonly kind = computed<TableContentKind | null>(
    () => this.entry()?.kind ?? this.newKind(),
  );
  protected readonly nav = computed<ContentNavKind | undefined>(() => {
    const k = this.kind();
    return k === null ? undefined : navOfKind(k);
  });
  /** The master on a laptop, for a kind that has an editor. */
  protected readonly editing = computed(() => {
    const k = this.kind();
    return this.isMaster() && !this.phone() && k !== null && EDITABLE.has(k);
  });
  protected readonly isNew = computed(() => this.entry() === null && this.newKind() !== null);
  protected readonly missing = computed(
    () => this.ctx() !== null && this.entry() === null && this.newKind() === null,
  );
  protected readonly noEditorYet = computed(
    () => this.isNew() && !EDITABLE.has(this.newKind() as TableContentKind),
  );

  protected readonly heading = computed(() => headingOf(this.entry(), this.newKind()));
  protected readonly kindTag = computed(() => kindTagOf(this.kind()));
  protected readonly state_ = computed(() => {
    const e = this.entry();
    return e ? entryState(e, this.isMaster()) : null;
  });
  protected readonly updated = computed(() => updatedLine(this.entry()));
  protected readonly archivedOn = computed(() => archivedDay(this.entry()));
  protected readonly using = computed(() => usageOf(this.entry()));
  protected readonly backLabel = computed(() => backLabelOf(this.isMaster(), this.nav()));
  protected readonly backQuery = computed(() =>
    this.isMaster() && this.nav() ? { kind: this.nav()!.slug } : {},
  );
  protected readonly nameOf = computed(() => nameResolver(this.catalog(), this.entries()));
  /** The entry a player reads (or the master where the app has no editor yet), written by `readEntry`. */
  protected readonly read = computed(() => {
    // The master's creature on a phone is read by `app-creature-read`, which writes its own text.
    const own = this.isMaster() && this.kind() === TableContentKind.CREATURE;
    return own
      ? NO_READ
      : readOfEntry(this.entry(), this.nameOf(), this.catalog(), this.entries(), this.menu());
  });
  /** "A raça Corujeiro" / "O antecedente Cartógrafo": the noun of the kind with its article, so the copy agrees. */
  protected readonly nounPhrase = computed(() => nounPhraseOf(this.entry()));
  protected readonly archivedWord = computed(() => archivedWordOf(this.entry()));
  protected readonly saveBlocked = computed(() =>
    this.asking() ? 'Responda à pergunta de arquivar para voltar a salvar.' : '',
  );
  protected readonly subraceParent = computed(() =>
    subraceParentOf(this.entry(), this.parentKey()),
  );
  protected readonly entries = computed(() => this.ctx()?.entries ?? []);

  constructor() {
    this.route.paramMap.pipe(takeUntilDestroyed(inject(DestroyRef))).subscribe((params) => {
      const id = params.get('id');
      if (!id) {
        return;
      }
      this.campaignId.set(id);
      // Going from one entry to another leaves nothing of the last one on the page.
      this.savedLine.set('');
      this.affected.set([]);
      this.actionError.set('');
      this.status.set('');
      this.key.set(params.get('key') ?? '');
      const newKind = params.get('kind');
      this.newKind.set(newKind ? (NEW_KINDS[newKind] ?? null) : null);
      this.parentKey.set(this.route.snapshot.queryParamMap.get('race') ?? '');
      this.parentClass.set(this.route.snapshot.queryParamMap.get('class') ?? '');
      void this.load();
    });
    // The table changed while this entry is open: the entries are read again with this person's role. An editor keeps what is
    // being typed (it restarts only when the entry's own revision changes), and a player's page learns the entry went away.
    this.watcher.whileLive(this.campaignId, () => void this.refresh());
  }

  /** The entries again after a `content_changed`, with no spinner and no change to the page's state. */
  protected async refresh(): Promise<void> {
    const s = this.state();
    if (s.status !== 'ready') {
      return;
    }
    const mine = ++this.reads;
    try {
      const res = await loadContext(this.campaigns, this.client, this.campaignId());
      const now = this.state();
      // Of two reads that overlap, the one that began last is the one that lands.
      if (mine !== this.reads) {
        return;
      }
      if (res.status === 'ok' && now.status === 'ready') {
        const entries = mergeRefreshed(
          now.ctx.entries,
          res.ctx.entries,
          this.key(),
          this.editing(),
        );
        this.state.set({ status: 'ready', ctx: { ...res.ctx, entries } });
        await this.refreshCatalog(entries, mine);
      }
    } catch {
      // Keep what is on screen: the next change reads again.
    }
  }

  /** The names are built from the catalog and the entries together, so they follow a change of either. */
  private async refreshCatalog(entries: readonly TableEntry[], read: number): Promise<void> {
    const c = await this.client.catalog(this.campaignId());
    if (read === this.reads) {
      this.catalog.set(catalogVm(c, entries));
    }
  }

  protected async load(): Promise<void> {
    this.reads++;
    this.state.set({ status: 'loading' });
    this.asking.set(false);
    try {
      const res = await loadContext(this.campaigns, this.client, this.campaignId());
      if (res.status === 'not-found') {
        this.state.set({ status: 'not-found' });
        return;
      }
      this.state.set({ status: 'ready', ctx: res.ctx });
      const nav = history.state as { saved?: string; affected?: number } | null;
      if (nav?.saved) {
        this.savedLine.set(nav.saved);
      }
      this.loadExtras(res.ctx);
      afterNextRender(() => this.title()?.nativeElement.focus({ preventScroll: true }), {
        injector: this.injector,
      });
    } catch (err) {
      this.state.set({ status: 'error', message: contentErrorText(err, 'abrir esta entrada') });
    }
  }

  /** The catalog (the names, the schools, the classes) and, for the master, the effect menu: after the page is up. A failure is
   * said on the page, with a way to try again; the spell editor needs only the catalog. */
  protected loadExtras(ctx: ContentContext | null = this.ctx()): void {
    if (!ctx) {
      return;
    }
    this.catalogError.set('');
    this.menuError.set('');
    this.defaultsError.set('');
    const catalog = this.client
      .catalog(ctx.campaignId)
      .then((c) => this.catalog.set(catalogVm(c, ctx.entries)));
    const defaults =
      ctx.isMaster && this.needsDefaults()
        ? this.client.classDefaults(ctx.campaignId).then((d) => this.defaults.set(d))
        : Promise.resolve();
    const menu = ctx.isMaster
      ? this.client.effectMenu(ctx.campaignId).then((m) => this.menu.set(new EffectMenuVm(m)))
      : Promise.resolve();
    void Promise.allSettled([catalog, menu, defaults]).then((results) => {
      wireMenu(this.menu(), this.catalog(), ctx.entries);
      const errors = extrasErrors(results);
      this.defaultsError.set(errors.defaults);
      this.catalogError.set(errors.catalog);
      this.menuError.set(errors.menu);
    });
  }

  protected backLink(): string[] {
    return ['/campaigns', this.campaignId(), 'content'];
  }

  protected goBack(): void {
    void this.router.navigate(this.backLink(), { queryParams: this.backQuery() });
  }

  protected async onSaved(res: EditorSaved): Promise<void> {
    const s = this.state();
    if (s.status !== 'ready') {
      return;
    }
    const entries = entriesAfterSave(s.ctx.entries, res.entry);
    this.state.set({
      status: 'ready',
      ctx: { ...s.ctx, entries, tableRevision: res.entry.revision },
    });
    this.affected.set(res.affected);
    const saved = savedSentence(res.entry.kind, res.entry.namePt);
    this.savedLine.set(saved);
    if (!this.key()) {
      // A new entry has its key now: its page is the entry's own.
      await this.router.navigate([...this.backLink(), 'entries', res.entry.key], {
        replaceUrl: true,
        state: { saved },
      });
    }
    window.scrollTo({ top: 0 });
  }

  /** "Arquivar": in place on a laptop, a bottom sheet on a phone. */
  protected async askArchive(): Promise<void> {
    this.actionError.set('');
    const e = this.entry();
    if (!e) {
      return;
    }
    if (!this.phone()) {
      this.asking.set(true);
      return;
    }
    const confirmed = await confirmArchiveInSheet(this.dialog, this.bottomSheet, e);
    if (confirmed) {
      await this.archive();
    }
  }

  protected cancelAsk(): void {
    this.asking.set(false);
    afterNextRender(() => this.title()?.nativeElement.focus({ preventScroll: true }), {
      injector: this.injector,
    });
  }

  protected async archive(): Promise<void> {
    await this.changeArchived(
      (id, key) => this.client.archive(id, key),
      'arquivar',
      () => {
        this.asking.set(false);
        this.status.set(`${this.nounPhrase()} foi ${this.archivedWord()}.`);
        this.savedLine.set('');
      },
    );
  }

  protected async unarchive(): Promise<void> {
    await this.changeArchived(
      (id, key) => this.client.unarchive(id, key),
      'desarquivar',
      () => this.status.set(`${this.nounPhrase()} voltou.`),
    );
  }

  /** "Arquivar" and "Desarquivar" share the shape: the entry as the server answers replaces the one on the page. */
  private async changeArchived(
    call: (campaignId: string, key: string) => Promise<TableEntry>,
    verb: string,
    done: () => void,
  ): Promise<void> {
    const e = this.entry();
    // The "⋮" stays enabled (the focus stays on it), so a second tap while the first call runs is ignored here.
    if (!e || this.archiving()) {
      return;
    }
    this.archiving.set(true);
    this.actionError.set('');
    try {
      this.replace(await call(this.campaignId(), e.key));
      done();
    } catch (err) {
      this.actionError.set(contentErrorText(err, verb));
    } finally {
      this.archiving.set(false);
    }
  }

  /** The entry as the server has it now: after a switch, an archive or a read again. */
  protected replace(entry: TableEntry): void {
    this.state.update((s) =>
      s.status === 'ready'
        ? { status: 'ready', ctx: { ...s.ctx, entries: replaceEntry(s.ctx.entries, entry) } }
        : s,
    );
  }
}
