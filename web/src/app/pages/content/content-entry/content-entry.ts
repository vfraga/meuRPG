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
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';

import {
  type AffectedCharacter,
  type GetClassTableDefaultsResponse,
  TableContentKind,
  type TableEntry,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { formatDateTime } from '../../../core/characters/character-labels';
import { type CatalogVm, catalogVm } from '../../../core/content/catalog';
import { TableContentClient, contentErrorText } from '../../../core/content/content-client';
import { type ContentContext, loadContext } from '../../../core/content/content-context';
import { readEntry } from '../../../core/content/content-read';
import {
  KIND_NOUNS,
  KIND_WORDS,
  type ContentNavKind,
  entryState,
  navOfKind,
  savedSentence,
  usageSentence,
} from '../../../core/content/content-kinds';
import { ContentWatcher } from '../../../core/content/content-watcher';
import { EffectMenuVm } from '../../../core/content/effect-draft';
import { LiveSessionSourceLive } from '../../live-session/live-session-source.live';
import { mediaQuery, PHONE_QUERY } from '../../../shared/map-view/media-query';
import { openSheet } from '../../../shared/sheet/sheet-host';
import { ArchiveQuestion } from '../archive/archive-question';
import { ArchiveSheet } from '../archive/archive-sheet';
import { BackgroundEditor } from '../background-editor/background-editor';
import { EntryRead } from '../entry-read/entry-read';
import { RaceEditor } from '../race-editor/race-editor';
import { type EditorSaved, SpellEditor } from '../spell-editor/spell-editor';
import { ClassEditor } from '../class-editor/class-editor';
import { SubclassEditor } from '../subclass-editor/subclass-editor';

type PageState =
  | { status: 'loading' }
  | { status: 'not-found' }
  | { status: 'error'; message: string }
  | { status: 'ready'; ctx: ContentContext };

/** The URL's word for the kind of a new entry ("new/spell"). */
const NEW_KINDS: Readonly<Record<string, TableContentKind>> = {
  spell: TableContentKind.SPELL,
  race: TableContentKind.RACE,
  subrace: TableContentKind.SUBRACE,
  background: TableContentKind.BACKGROUND,
  class: TableContentKind.CLASS,
  subclass: TableContentKind.SUBCLASS,
};

const NEW_TITLES: Readonly<Record<number, string>> = {
  [TableContentKind.SPELL]: 'Nova magia',
  [TableContentKind.RACE]: 'Nova raça',
  [TableContentKind.SUBRACE]: 'Nova sub-raça',
  [TableContentKind.BACKGROUND]: 'Novo antecedente',
  [TableContentKind.CLASS]: 'Nova classe',
  [TableContentKind.SUBCLASS]: 'Nova subclasse',
};

const EDITABLE = new Set<TableContentKind>([
  TableContentKind.SPELL,
  TableContentKind.RACE,
  TableContentKind.SUBRACE,
  TableContentKind.BACKGROUND,
  TableContentKind.CLASS,
  TableContentKind.SUBCLASS,
]);

/**
 * One entry of the table's content (MR-025, RN-23; E10-01 states 4 to 9): the master's editor for a spell, a race, a subrace
 * and a background on a laptop, and the read view for everything else (a player; the master on a phone, where it reads and
 * archives only; the master's classes and subclasses until 10.12). The header carries the state in words, "Arquivar" and
 * its question in place (a bottom sheet on a phone), and, after a write, what changed for the sheets that use the entry.
 * Nothing is ever deleted: an archived entry says so, offers "Desarquivar", and can still be edited.
 */
@Component({
  selector: 'app-content-entry',
  imports: [
    ArchiveQuestion,
    BackgroundEditor,
    ClassEditor,
    EntryRead,
    MatButtonModule,
    MatIconModule,
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
  protected readonly kindWords = KIND_WORDS;
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
  protected readonly loadingExtras = signal(false);
  /** The catalog or the effect menu did not come: the page says so, with "Tentar de novo". */
  /** Moves with each read of the entries: of two that overlap, only the newest lands. */
  private reads = 0;
  private readonly catalogError = signal('');
  private readonly menuError = signal('');
  /** What blocks this entry: the catalog always, the menu for every editor but the spell's (which does not use it), the defaults
   * for the class and the subclass. */
  protected readonly extrasError = computed(
    () =>
      this.catalogError() ||
      (this.kind() === TableContentKind.SPELL ? '' : this.menuError()) ||
      (this.needsDefaults() ? this.defaultsError() : ''),
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
  protected readonly readOnly = computed(() => !this.editing());
  protected readonly isNew = computed(() => this.entry() === null && this.newKind() !== null);
  protected readonly missing = computed(
    () => this.ctx() !== null && this.entry() === null && this.newKind() === null,
  );
  protected readonly noEditorYet = computed(
    () => this.isNew() && !EDITABLE.has(this.newKind() as TableContentKind),
  );

  protected readonly heading = computed(() => {
    const e = this.entry();
    if (e) {
      return e.namePt;
    }
    const k = this.newKind();
    return k === null ? 'Conteúdo da mesa' : (NEW_TITLES[k] ?? 'Conteúdo da mesa');
  });
  protected readonly kindTag = computed(() => {
    const k = this.kind();
    return k === null ? '' : `${KIND_WORDS[k]} da mesa`;
  });
  protected readonly state_ = computed(() => {
    const e = this.entry();
    return e ? entryState(e, this.isMaster()) : null;
  });
  protected readonly updated = computed(() => {
    const e = this.entry();
    return e?.updatedAt
      ? `Atualizada em ${formatDateTime(timestampDate(e.updatedAt)).slice(0, 10)}`
      : '';
  });
  protected readonly archivedOn = computed(() => {
    const e = this.entry();
    return e?.archivedAt ? formatDateTime(timestampDate(e.archivedAt)).slice(0, 10) : '';
  });
  protected readonly using = computed(() => {
    const e = this.entry();
    return e ? usageSentence(e.namePt, e.charactersUsing) : '';
  });
  protected readonly backLabel = computed(() =>
    this.isMaster()
      ? `Voltar para ${this.nav()?.plural ?? 'Conteúdo da mesa'}`
      : 'Voltar para Conteúdo da mesa',
  );
  protected readonly backQuery = computed(() =>
    this.isMaster() && this.nav() ? { kind: this.nav()!.slug } : {},
  );
  protected readonly nameOf = computed<(key: string) => string>(() => {
    const cat = this.catalog();
    const entries = this.ctx()?.entries ?? [];
    return (key) =>
      cat?.nameOf(key) ??
      entries.find((e) => e.key === key)?.namePt ??
      key.replace(/^[a-z-]+:/, '');
  });
  /** The entry a player reads (or the master where the app has no editor yet), written by `readEntry`. */
  protected readonly read = computed(() => {
    const e = this.entry();
    return e
      ? readEntry(e, this.nameOf(), (key) => this.catalog()?.subclassLevelOf(key) ?? 0)
      : null;
  });
  /** "A raça Corujeiro" / "O antecedente Cartógrafo": the noun of the kind with its article, so the copy agrees. */
  protected readonly nounPhrase = computed(() => {
    const e = this.entry();
    const n = e ? KIND_NOUNS[e.kind] : undefined;
    return e && n ? `${n.article} ${n.noun} ${e.namePt}` : '';
  });
  protected readonly archivedWord = computed(() => {
    const e = this.entry();
    return (e ? KIND_NOUNS[e.kind]?.archived : undefined) ?? 'arquivada';
  });
  protected readonly saveBlocked = computed(() =>
    this.asking() ? 'Responda à pergunta de arquivar para voltar a salvar.' : '',
  );
  protected readonly subraceParent = computed(() => {
    const body = this.entry()?.body;
    return body?.case === 'tableSubrace' ? body.value.raceKey : this.parentKey();
  });
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
        const open = now.ctx.entries.find((e) => e.key === this.key());
        const editing = this.editing();
        const entries = res.ctx.entries.map((e) =>
          // The entry being edited keeps its body and revision when another write changed them: what is typed is not thrown
          // away, and "Salvar" says the entry changed (stale) as it always did. Only the switches and the counts follow.
          // A reader (a player, or a master with no editor open) adopts the new entry whole.
          editing && open && e.key === open.key && e.revision !== open.revision
            ? ({
                ...open,
                off: e.off,
                archived: e.archived,
                archivedAt: e.archivedAt,
                charactersUsing: e.charactersUsing,
              } as TableEntry)
            : e,
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
    this.loadingExtras.set(true);
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
      ? this.client.effectMenu(ctx.campaignId).then((m) => {
          const vm = new EffectMenuVm(m);
          this.menu.set(vm);
        })
      : Promise.resolve();
    void Promise.allSettled([catalog, menu, defaults]).then((results) => {
      const cat = this.catalog();
      const vm = this.menu();
      if (cat && vm) {
        vm.classNamePt = (key) => cat.nameOf(key);
      }
      const [c, m, df] = results;
      this.defaultsError.set(
        df.status === 'rejected' ? contentErrorText(df.reason, 'abrir o editor') : '',
      );
      this.catalogError.set(
        c.status === 'rejected' ? contentErrorText(c.reason, 'abrir esta entrada') : '',
      );
      this.menuError.set(
        m.status === 'rejected' ? contentErrorText(m.reason, 'abrir o editor') : '',
      );
      this.loadingExtras.set(false);
    });
  }

  protected backLink(): string[] {
    return ['/campaigns', this.campaignId(), 'content'];
  }

  protected entryLink(e: TableEntry): string[] {
    return ['/campaigns', this.campaignId(), 'content', 'entries', e.key];
  }

  protected goBack(): void {
    void this.router.navigate(this.backLink(), { queryParams: this.backQuery() });
  }

  protected async onSaved(res: EditorSaved): Promise<void> {
    const s = this.state();
    if (s.status !== 'ready') {
      return;
    }
    const exists = s.ctx.entries.some((e) => e.key === res.entry.key);
    const entries = exists
      ? s.ctx.entries.map((e) => (e.key === res.entry.key ? res.entry : e))
      : [...s.ctx.entries, res.entry].sort((a, b) => a.namePt.localeCompare(b.namePt, 'pt-BR'));
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
    const e = this.entry();
    if (!e) {
      return;
    }
    this.archiving.set(true);
    this.actionError.set('');
    try {
      this.replace(await this.client.archive(this.campaignId(), e.key));
      this.asking.set(false);
      this.status.set(`${this.nounPhrase()} foi ${this.archivedWord()}.`);
      this.savedLine.set('');
    } catch (err) {
      this.actionError.set(contentErrorText(err, 'arquivar'));
    } finally {
      this.archiving.set(false);
    }
  }

  protected async unarchive(): Promise<void> {
    const e = this.entry();
    if (!e) {
      return;
    }
    this.archiving.set(true);
    this.actionError.set('');
    try {
      this.replace(await this.client.unarchive(this.campaignId(), e.key));
      this.status.set(`${this.nounPhrase()} voltou.`);
    } catch (err) {
      this.actionError.set(contentErrorText(err, 'desarquivar'));
    } finally {
      this.archiving.set(false);
    }
  }

  /** The entry as the server has it now: after a switch, an archive or a read again. */
  protected replace(entry: TableEntry): void {
    const s = this.state();
    if (s.status === 'ready') {
      this.state.set({
        status: 'ready',
        ctx: { ...s.ctx, entries: s.ctx.entries.map((x) => (x.key === entry.key ? entry : x)) },
      });
    }
  }

  protected characterLink(a: AffectedCharacter): string[] {
    return ['/campaigns', this.campaignId(), 'characters', a.characterId];
  }
}
