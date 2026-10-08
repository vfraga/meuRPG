import { Location } from '@angular/common';
import {
  Component,
  DOCUMENT,
  DestroyRef,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  signal,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, ParamMap, Router, RouterLink } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import type { Spell } from '../../../gen/meurpg/rules/v1/rules_pb';
import { Role } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterKind } from '../../../gen/meurpg/characters/v1/characters_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { spellLevelLabel } from '../../core/characters/character-labels';
import { formatInt, joinDots } from '../../core/format/text';
import { RosterClient } from '../../core/maps/roster-client';
import { type SpellClass, SpellsClient } from '../../core/spells/spells-client';
import { BASIC_SHEET_SENTENCE, spellsErrorMessage } from '../../core/spells/spells-errors';
import {
  NO_FILTER,
  type SpellFilter,
  SCHOOLS,
  activeFilters,
  filterChips,
  isFiltered,
} from '../../core/spells/spells-filter';
import { ContentWatcher } from '../../core/content/content-watcher';
import { SpellsState } from '../../core/spells/spells-state';
import { LiveSessionSourceLive } from '../live-session/live-session-source.live';
import { mediaQuery } from '../../shared/map-view/media-query';
import { isTableSpellKey, spellDetailsFromGen } from '../../shared/spell-details/spell-details-map';
import { type SpellCardState, SpellCard } from './spell-card';
import { openSpellFilterSheet } from './spell-filter-sheet';
import { type MineCharacter, SpellFilters } from './spell-filters';

type Access =
  | { readonly status: 'loading' }
  | { readonly status: 'ok'; readonly campaignName: string; readonly master: boolean }
  | { readonly status: 'pending' }
  | { readonly status: 'not-found' }
  | { readonly status: 'error'; readonly message: string };

/** The query parameters: the filters survive a reload, and the open spell is a step of the browser's history. */
const PARAMS = {
  query: 'q',
  classKey: 'class',
  levels: 'levels',
  school: 'school',
  mine: 'mine',
  spell: 'spell',
} as const;

/** From this width the page has three columns (filters, list, description); under it, it is the phone's page. */
const WIDE_QUERY = '(min-width: 1100px)';

/**
 * "/campaigns/:id/spells" (MR-045, RN-23, E10-11): the players' spell reference, open to every active
 * member, the master too. The server finds, filters, sorts and counts (`ListSpells`): the page asks and
 * draws. From 1100 px: the filters at the left, the list in the middle, the chosen spell in full at the
 * right. On a phone and a tablet: the search, "Filtros (n)" and the chips of what is on; a spell opens
 * in place of the list, and "Voltar para Magias" brings the same list back at the same point.
 *
 * What a player never receives is the server's to leave out (an archived table spell, a retired class):
 * the master's list carries "Arquivada". "Só as que posso aprender" is for the player's own character;
 * the master has none, so it is not offered to them.
 */
@Component({
  selector: 'app-spells',
  imports: [
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    RouterLink,
    SpellCard,
    SpellFilters,
  ],
  providers: [ContentWatcher, LiveSessionSourceLive],
  templateUrl: './spells.html',
  styleUrls: ['./spells.scss', './spells-list.scss'],
})
export class Spells {
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly campaigns = inject(CampaignsService);
  private readonly roster = inject(RosterClient);
  private readonly client = inject(SpellsClient);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly document = inject(DOCUMENT);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);
  private readonly location = inject(Location);
  private readonly watcher = inject(ContentWatcher);

  protected readonly campaignId = this.route.snapshot.paramMap.get('id') ?? '';
  protected readonly access = signal<Access>({ status: 'loading' });
  protected readonly mine = signal<MineCharacter | null>(null);
  protected readonly classes = signal<readonly SpellClass[]>([]);
  protected readonly wide = mediaQuery(WIDE_QUERY);
  protected readonly state = new SpellsState(
    this.client,
    this.campaignId,
    () => this.mine()?.id ?? null,
    {
      // The filters go into the link when a search starts (not on every key), and the class names follow an edit of the table.
      onSearch: () => this.syncUrl(),
      onAnswered: () => void this.loadClasses(),
    },
  );

  /** The key of the spell open (`?spell=`), or ''. */
  protected readonly selected = signal(this.route.snapshot.queryParamMap.get(PARAMS.spell) ?? '');
  protected readonly card = signal<SpellCardState | null>(null);
  private cardSeq = 0;
  /** The spell was opened from the list with a step of the history: "Voltar para Magias" takes that step back. */
  private pushed = false;
  /** The spell that was open when it closed, for the focus. */
  private lastKey = '';
  /** Where the list was when a spell opened on the narrow page, so "Voltar para Magias" returns there. */
  private scrollBefore = 0;

  /** The narrow page is showing a spell instead of the list. */
  protected readonly detailOpen = computed(() => !this.wide() && this.selected() !== '');
  protected readonly filter = this.state.filter;
  protected readonly filtersOn = computed(() => activeFilters(this.filter()));
  protected readonly filtered = computed(() => isFiltered(this.filter()));
  protected readonly chips = computed(() =>
    filterChips(this.filter(), (key) => this.className(key)),
  );
  protected readonly count = computed(() => {
    const n = this.state.total();
    return `${formatInt(n)} ${n === 1 ? 'magia' : 'magias'}`;
  });
  protected readonly lead = computed(() => {
    const a = this.access();
    if (a.status !== 'ok') {
      return '';
    }
    if (this.wide()) {
      return `${a.campaignName} · As magias do SRD e as que o mestre criou para a mesa.`;
    }
    // The count is the list's own line, said once.
    const m = this.mine();
    return `${a.campaignName} · ${this.filter().onlyMine && m ? `As magias para ${m.name}` : 'As magias da mesa'}`;
  });
  /** Nothing matched: with a name typed, the name is what to check; without one, only the filters are left. */
  protected readonly emptyHint = computed(() =>
    this.filter().query.trim()
      ? 'Confira o nome ou tire um filtro.'
      : 'Tire um filtro para ver mais magias.',
  );
  protected readonly emptyTitle = computed(() => {
    const q = this.filter().query.trim();
    return q ? `Nenhuma magia com “${q}”.` : 'Nenhuma magia passa nesses filtros.';
  });
  protected readonly basicSheetSentence = BASIC_SHEET_SENTENCE;
  protected readonly skeleton = Array.from({ length: 8 });
  protected readonly rows = computed(() =>
    this.state.spells().map((s) => ({
      spell: s,
      sub: joinDots([spellLevelLabel(s.level), s.schoolNamePt]),
      table: isTableSpellKey(s.key),
    })),
  );

  constructor() {
    const destroyRef = inject(DestroyRef);
    destroyRef.onDestroy(() => this.state.dispose());
    const sub = this.route.queryParamMap.subscribe((params) => {
      this.applyFilters(params);
      this.select(params.get(PARAMS.spell) ?? '');
    });
    destroyRef.onDestroy(() => sub.unsubscribe());
    void this.start();
    // The master turned something on or off, or wrote a spell (`content_changed`): the list, the open spell and the class names
    // are read again with this person's role, so a spell that went off leaves the list and the card says so.
    this.watcher.whileLive(
      () => this.campaignId,
      () => void this.contentChanged(),
    );
  }

  private async contentChanged(): Promise<void> {
    if (this.access().status !== 'ok') {
      return;
    }
    // The list first: its answer carries the new content version, and the card is then read from the same moment.
    await this.state.refresh();
    if (this.selected() !== '') {
      void this.loadCard(this.selected(), true);
    }
  }

  protected async start(): Promise<void> {
    this.access.set({ status: 'loading' });
    try {
      const { campaign } = await this.campaigns.getCampaign(this.campaignId);
      if (!campaign) {
        this.access.set({ status: 'not-found' });
        return;
      }
      if (campaign.awaitingApproval) {
        this.access.set({ status: 'pending' });
        return;
      }
      const master = campaign.myRole === Role.MASTER;
      // The master has no character of their own: "Só as que posso aprender" is the player's.
      this.mine.set(master ? null : await this.ownCharacter());
      this.state.filter.set(this.filterFromParams(this.route.snapshot.queryParamMap));
      this.access.set({ status: 'ok', campaignName: campaign.name, master });
      void this.state.search();
    } catch (err) {
      this.access.set(
        ConnectError.from(err, Code.Unavailable).code === Code.NotFound
          ? { status: 'not-found' }
          : { status: 'error', message: spellsErrorMessage(err) },
      );
    }
  }

  private async ownCharacter(): Promise<MineCharacter | null> {
    try {
      const mine = (await this.roster.list(this.campaignId)).find(
        (c) => c.kind === CharacterKind.PLAYER && c.classSummary !== '',
      );
      return mine
        ? { id: mine.id, name: mine.name, label: `${mine.name}, ${mine.classSummary}` }
        : null;
    } catch {
      // Without the character, the switch is not offered; every spell can still be read.
      return null;
    }
  }

  /** The class names for the filter: asked again after an answer, so an edit of the table's classes shows (the client keeps them per content version). */
  private async loadClasses(): Promise<void> {
    try {
      const classes = await this.client.classes(this.campaignId);
      if (classes !== this.classes()) {
        this.classes.set(classes);
      }
    } catch {
      // The filter then offers only "Todas": the list itself still works.
    }
  }

  protected className(key: string): string {
    return this.classes().find((c) => c.key === key)?.namePt ?? '';
  }

  /** The filters in the link, so a reload or a shared link brings the same search. */
  private filterFromParams(q: ParamMap): SpellFilter {
    const levels = (q.get(PARAMS.levels) ?? '')
      .split(',')
      .filter((x) => /^[0-9]$/.test(x))
      .map(Number);
    const school = q.get(PARAMS.school) ?? '';
    return {
      ...NO_FILTER,
      query: (q.get(PARAMS.query) ?? '').slice(0, 100),
      classKey: q.get(PARAMS.classKey) ?? '',
      levels: [...new Set(levels)].sort((a, b) => a - b),
      schoolKey: SCHOOLS.some((s) => s.key === school) ? school : '',
      onlyMine: q.get(PARAMS.mine) === '1' && this.mine() !== null,
    };
  }

  /** The link's words for the filters. */
  private filterParams(): Record<string, string | null> {
    const f = this.filter();
    return {
      [PARAMS.query]: f.query.trim() || null,
      [PARAMS.classKey]: f.classKey || null,
      [PARAMS.levels]: f.levels.length > 0 ? f.levels.join(',') : null,
      [PARAMS.school]: f.schoolKey || null,
      [PARAMS.mine]: f.onlyMine ? '1' : null,
    };
  }

  /** The filters go into the URL when a search starts, replacing the entry: Back leaves the page, not the last search. */
  private syncUrl(): void {
    if (this.access().status !== 'ok') {
      return;
    }
    void this.router.navigate([], {
      relativeTo: this.route,
      replaceUrl: true,
      queryParamsHandling: 'merge',
      queryParams: this.filterParams(),
    });
  }

  /** The browser's Back or Forward brought other filters than the ones on screen: the state follows the link, so the two never drift. */
  private applyFilters(params: ParamMap): void {
    if (this.access().status !== 'ok') {
      return;
    }
    const fromUrl = this.filterFromParams(params);
    const now = this.filter();
    const same =
      fromUrl.query === now.query.trim() &&
      fromUrl.classKey === now.classKey &&
      fromUrl.schoolKey === now.schoolKey &&
      fromUrl.onlyMine === now.onlyMine &&
      fromUrl.levels.join(',') === now.levels.join(',');
    if (!same) {
      this.state.filter.set(fromUrl);
      void this.state.search();
    }
  }

  protected clear(onlyName: boolean): void {
    void this.state.clear(onlyName);
    // The button that was pressed is gone: the focus stays in the field, where the person starts over.
    this.host.nativeElement.querySelector<HTMLInputElement>('input[type=search]')?.focus();
  }

  protected removeChip(id: 'class' | 'level' | 'school' | 'mine'): void {
    const before = this.chips().findIndex((c) => c.id === id);
    const patch: Partial<SpellFilter> =
      id === 'class'
        ? { classKey: '' }
        : id === 'level'
          ? { levels: [] }
          : id === 'school'
            ? { schoolKey: '' }
            : { onlyMine: false };
    void this.state.change(patch);
    // The chip that was pressed is gone: the focus goes to the next one, or to the search when none is left.
    afterNextRender(
      () => {
        const chips = this.host.nativeElement.querySelectorAll<HTMLElement>('.chip__x');
        (
          chips[Math.min(before, chips.length - 1)] ??
          this.host.nativeElement.querySelector<HTMLInputElement>('input[type=search]')
        )?.focus();
      },
      { injector: this.injector },
    );
  }

  protected openFilters(): void {
    openSpellFilterSheet(this.dialog, this.bottomSheet, {
      state: this.state,
      classes: this.classes(),
      mine: this.mine(),
    }).subscribe();
  }

  protected open(spell: Spell): void {
    this.scrollBefore = this.document.defaultView?.scrollY ?? 0;
    // On the narrow page the spell is a step of the history (Back closes it); on the wide page the list stays, so it is not.
    this.pushed = !this.wide();
    // The filters ride along, so the link names the whole page and Back never brings an older search.
    void this.router.navigate([], {
      relativeTo: this.route,
      queryParams: { ...this.filterParams(), [PARAMS.spell]: spell.key },
      queryParamsHandling: 'merge',
      replaceUrl: this.wide(),
    });
  }

  /** "Voltar para Magias": the step of the history that opened the spell, taken back (so Forward still works); a spell that came from a link has none to take. */
  protected backToList(): void {
    if (this.pushed) {
      this.location.back();
      return;
    }
    void this.router.navigate([], {
      relativeTo: this.route,
      queryParams: { [PARAMS.spell]: null },
      queryParamsHandling: 'merge',
      replaceUrl: true,
    });
  }

  /** The URL names a spell (a click, Back, Android's Back, a shared link): read it. */
  private select(key: string): void {
    if (key === this.selected() && this.card() !== null) {
      return;
    }
    const closed = this.selected() !== '' && key === '';
    if (closed) {
      this.lastKey = this.selected();
    }
    this.selected.set(key);
    if (key === '') {
      this.card.set(null);
      this.pushed = false;
      if (closed && !this.wide()) {
        this.restoreList();
      }
      return;
    }
    void this.loadCard(key);
    if (!this.wide()) {
      // The description replaces the list: it starts at the top, with its title as the first stop.
      afterNextRender(
        () => {
          this.scrollTo(0);
          this.host.nativeElement
            .querySelector<HTMLElement>('#spell-card-title')
            ?.focus({ preventScroll: true });
        },
        { injector: this.injector },
      );
    }
  }

  /** The spell closed on the narrow page, by "Voltar para Magias" or by the browser's Back: the list is at the same point, with the focus on the row that was open. */
  private restoreList(): void {
    const key = this.lastKey;
    afterNextRender(
      () => {
        this.scrollTo(this.scrollBefore);
        this.host.nativeElement
          .querySelector<HTMLElement>(`button.row[data-key="${CSS.escape(key)}"]`)
          ?.focus({ preventScroll: true });
      },
      { injector: this.injector },
    );
  }

  private scrollTo(top: number): void {
    try {
      this.document.defaultView?.scrollTo?.({ top });
    } catch {
      // No scrolling here (a test window): the list is still the same list.
    }
  }

  protected async loadCard(key = this.selected(), quiet = false): Promise<void> {
    const seq = ++this.cardSeq;
    const namePt = this.state.spells().find((s) => s.key === key)?.namePt ?? '';
    // Read again after a `content_changed`: the card on screen stays until the answer comes.
    if (!(quiet && this.card()?.status === 'ready')) {
      this.card.set({ status: 'loading', namePt });
    }
    try {
      const details = spellDetailsFromGen(await this.client.details(this.campaignId, key, quiet));
      if (seq === this.cardSeq) {
        this.card.set({ status: 'ready', details });
      }
    } catch (err) {
      if (seq === this.cardSeq) {
        // A spell that is not there (or not for this person: an archived one, for a player) is not the campaign's error: it has its own sentence and nothing to retry.
        const gone = ConnectError.from(err, Code.Unavailable).code === Code.NotFound;
        this.card.set({
          status: 'error',
          namePt,
          message: gone ? 'Esta magia não está disponível.' : spellsErrorMessage(err, 'read'),
          canRetry: !gone,
        });
      }
    }
  }

  protected async more(): Promise<void> {
    const before = this.state.spells().length;
    await this.state.more();
    // The first new row takes the focus: the button that was pressed moves down with the list.
    afterNextRender(
      () => this.host.nativeElement.querySelectorAll<HTMLElement>('button.row')[before]?.focus(),
      { injector: this.injector },
    );
  }

  /** The class names for the card's line; a function so the card needs no catalog of its own. */
  protected readonly nameOfClass = (key: string) => this.className(key);
}
