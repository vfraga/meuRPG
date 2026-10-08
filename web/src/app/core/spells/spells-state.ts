import { signal } from '@angular/core';

import type { ListSpellsResponse, Spell } from '../../../gen/meurpg/rules/v1/rules_pb';
import { NO_FILTER, type SpellFilter, toListRequest } from './spells-filter';
import { isBasicSheet, spellsErrorMessage } from './spells-errors';

/** What the list shows: the first answer is awaited, then it is a list, a refused ask, or a sheet with no classes. */
export type SpellsStatus = 'loading' | 'ready' | 'error' | 'basic-sheet';

/** What the state needs of the client (the page gives the real one, a test gives a stub). */
export interface SpellsSource {
  list(request: ReturnType<typeof toListRequest>): Promise<ListSpellsResponse>;
}

/**
 * The "Magias" list (MR-045): the filters, the rows the server answered, the count, and the next page.
 * It holds no rule and filters nothing: a change of filter asks `ListSpells` again from the first page,
 * "Mostrar mais" asks for the next with the server's `page_token`. A slower answer to an older ask is
 * dropped. The rows on screen stay while the next answer is on its way, so the list does not jump.
 */
export class SpellsState {
  readonly filter = signal<SpellFilter>(NO_FILTER);
  readonly spells = signal<readonly Spell[]>([]);
  /** How many spells pass the filters, over all the pages (the server's `total`). */
  readonly total = signal(0);
  readonly status = signal<SpellsStatus>('loading');
  readonly error = signal('');
  /** An ask is on its way: the list is dimmed and announced as busy, never emptied. */
  readonly searching = signal(false);
  readonly nextToken = signal('');
  readonly loadingMore = signal(false);
  readonly moreError = signal('');

  private seq = 0;
  /** Moves with each `refresh()`: of two overlapping ones only the newest lands, and a page asked before it is dropped. */
  private refreshes = 0;

  /** The filter and character the rows on screen answer: "Mostrar mais" asks the next page of THAT list, never of a newer filter. */
  private answered: { filter: SpellFilter; characterId: string | null } | null = null;

  constructor(
    private readonly source: SpellsSource,
    private readonly campaignId: string,
    /** The player's own character, for "Só as que posso aprender"; null when there is none. */
    private readonly characterId: () => string | null,
    private readonly hooks: {
      /** An ask starts (the page writes the filters into the link). */
      readonly onSearch?: () => void;
      /** An answer arrived (the page checks the content version, so the class names follow an edit). */
      readonly onAnswered?: () => void;
    } = {},
  ) {}

  private timer: ReturnType<typeof setTimeout> | null = null;

  /** The name typed so far: the box shows it at once and the server is asked after a short pause, so a fast typist asks once. */
  typeQuery(query: string): void {
    this.filter.update((f) => ({ ...f, query }));
    if (this.timer) {
      clearTimeout(this.timer);
    }
    this.timer = setTimeout(() => void this.search(), 250);
  }

  /** The page is going away: no pending ask. */
  dispose(): void {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    this.seq++;
  }

  /** Changes some filters and asks again from the first page. */
  change(patch: Partial<SpellFilter>): Promise<void> {
    this.filter.update((f) => ({ ...f, ...patch }));
    return this.search();
  }

  clear(onlyName = false): Promise<void> {
    return this.change(onlyName ? { query: '' } : { ...NO_FILTER });
  }

  /** Every filter but the name (the sheet's "Limpar": the name is the page's own field). */
  clearFilters(): Promise<void> {
    return this.change({ classKey: '', levels: [], schoolKey: '', onlyMine: false });
  }

  async search(): Promise<void> {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    const seq = ++this.seq;
    this.searching.set(true);
    this.moreError.set('');
    this.hooks.onSearch?.();
    const filter = this.filter();
    const characterId = this.characterId();
    try {
      const res = await this.source.list(toListRequest(this.campaignId, filter, characterId));
      if (seq !== this.seq) {
        return;
      }
      this.answered = { filter, characterId };
      this.spells.set(res.spells);
      this.total.set(res.total);
      this.nextToken.set(res.nextPageToken);
      this.error.set('');
      this.status.set('ready');
      this.hooks.onAnswered?.();
    } catch (err) {
      if (seq !== this.seq) {
        return;
      }
      if (isBasicSheet(err) && this.filter().onlyMine) {
        this.status.set('basic-sheet');
      } else {
        this.error.set(spellsErrorMessage(err));
        this.status.set('error');
      }
    } finally {
      if (seq === this.seq) {
        this.searching.set(false);
      }
    }
  }

  /**
   * The table's content changed (`content_changed`, RN-23): the list that is on screen is asked again from its first page,
   * with the filter and character it answers, and swapped in when the answer comes. No spinner, no URL change, and a failure
   * leaves the list as it was. An ask of the person (a new filter) that starts or finishes meanwhile wins.
   */
  async refresh(): Promise<void> {
    const answered = this.answered;
    if (!answered || this.status() !== 'ready') {
      return;
    }
    const seq = this.seq;
    const mine = ++this.refreshes;
    // The pages the person had opened with "Mostrar mais" stay: as many are read again as were on screen.
    const wanted = this.spells().length;
    try {
      let res = await this.source.list(
        toListRequest(this.campaignId, answered.filter, answered.characterId),
      );
      let rows = [...res.spells];
      while (rows.length < wanted && res.nextPageToken) {
        if (seq !== this.seq || mine !== this.refreshes || this.answered !== answered) {
          return;
        }
        res = await this.source.list(
          toListRequest(this.campaignId, answered.filter, answered.characterId, res.nextPageToken),
        );
        rows = [...rows, ...res.spells];
      }
      if (seq !== this.seq || mine !== this.refreshes || this.answered !== answered) {
        return;
      }
      this.spells.set(rows);
      this.total.set(res.total);
      this.nextToken.set(res.nextPageToken);
      this.hooks.onAnswered?.();
    } catch {
      // Keep the list on screen: the next change reads again.
    }
  }

  /** The next page, added under the rows already there. */
  async more(): Promise<void> {
    const token = this.nextToken();
    // Never a new filter with an old token: while a search is on its way, the page waits for it.
    if (!token || this.loadingMore() || this.searching() || !this.answered) {
      return;
    }
    const { filter, characterId } = this.answered;
    const seq = this.seq;
    const refreshes = this.refreshes;
    this.loadingMore.set(true);
    this.moreError.set('');
    try {
      const res = await this.source.list(
        toListRequest(this.campaignId, filter, characterId, token),
      );
      if (seq !== this.seq || refreshes !== this.refreshes) {
        return;
      }
      this.spells.update((rows) => [...rows, ...res.spells]);
      this.total.set(res.total);
      this.nextToken.set(res.nextPageToken);
    } catch (err) {
      if (seq === this.seq && refreshes === this.refreshes) {
        this.moreError.set(spellsErrorMessage(err));
      }
    } finally {
      this.loadingMore.set(false);
    }
  }
}
