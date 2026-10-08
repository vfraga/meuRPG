import { Injectable, inject, signal } from '@angular/core';

import { XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { LevelUpReason } from '../../../gen/meurpg/characters/v1/characters_pb';
import type { TreasureToConvert, XPAward } from '../../../gen/meurpg/progression/v1/progression_pb';
import { joinDots } from '../format/text';
import { RosterClient } from '../maps/roster-client';
import { ProgressionClient } from './progression-client';

/** One living player character as the XP screens list it: the XP and the
 * level from `GetCampaignExperience`, and the class line and the player's
 * name from the campaign's roster. */
export interface ExperienceRow {
  readonly id: string;
  readonly name: string;
  /** The account that plays it, empty when the player deleted theirs. */
  readonly playerUserId: string;
  /** "Mago 3 · de Vinicius", or what there is of it. */
  readonly sub: string;
  readonly level: number;
  readonly xp: number;
  /** The XP that reaches the next level; 0 at level 20. */
  readonly nextLevelXp: number;
  readonly canLevelUp: boolean;
  readonly levelUpReason: LevelUpReason;
}

export type LoadState = 'loading' | 'ready' | 'error';

/**
 * The campaign's XP as one screen reads it: how the campaign levels, each
 * living player character with its XP, and the history of awards. One
 * instance per host (the campaign page, the session page's "Dar XP", the
 * end-of-combat block): the host provides it and calls `load`; the screens
 * under it read the signals, and `refresh` reads the server again after an
 * award, an undo or an `xp_changed`. A read that was overtaken by a newer one
 * is dropped, so a slow answer never overwrites a fresh one.
 */
@Injectable()
export class ExperienceStore {
  private readonly api = inject(ProgressionClient);
  private readonly roster = inject(RosterClient);

  private campaignId = '';
  private withAwards = false;
  private withTreasures = false;
  private treasuresSeq = 0;
  private rowsSeq = 0;
  private awardsSeq = 0;

  readonly xpMode = signal<XpMode>(XpMode.UNSPECIFIED);
  readonly rows = signal<readonly ExperienceRow[]>([]);
  readonly rowsState = signal<LoadState>('loading');

  /** The history, newest first, the pages read so far. */
  readonly awards = signal<readonly XPAward[]>([]);
  readonly awardsState = signal<LoadState>('loading');
  /** Empty on the last page. */
  readonly nextPageToken = signal('');
  readonly loadingMore = signal(false);

  /** The found treasures nobody converted yet (MR-041), the oldest finds first,
   * at most 100. Only the master reads them (`withTreasures`), and only in a
   * campaign that counts XP: a milestones campaign has no use for them. */
  readonly treasures = signal<readonly TreasureToConvert[]>([]);
  /** How many there are in all, which may be more than `treasures` holds. */
  readonly treasuresTotal = signal(0);
  readonly treasuresState = signal<LoadState>('loading');

  /** Reads the XP of the campaign, its history when `withAwards` and, for the
   * master, the treasures waiting for "Voltar à cidade" when `withTreasures`. */
  async load(campaignId: string, withAwards: boolean, withTreasures = false): Promise<void> {
    if (campaignId !== this.campaignId) {
      // Another campaign: nothing of the last one may show, and an answer still on its way is dropped.
      this.rowsSeq++;
      this.awardsSeq++;
      this.treasuresSeq++;
      this.xpMode.set(XpMode.UNSPECIFIED);
      this.rows.set([]);
      this.awards.set([]);
      this.nextPageToken.set('');
      this.loadingMore.set(false);
      this.treasures.set([]);
      this.treasuresTotal.set(0);
    }
    this.campaignId = campaignId;
    this.withAwards = withAwards;
    this.withTreasures = withTreasures;
    this.rowsState.set('loading');
    this.awardsState.set('loading');
    this.treasuresState.set('loading');
    await Promise.all([this.loadRows(), withAwards ? this.loadAwards() : Promise.resolve()]);
    await this.loadTreasures();
  }

  /** Reads again, keeping what is on screen until the answer comes. `treasures: false` leaves the
   * treasures out (a screen whose sheets read them again as they open has no use for them here). */
  async refresh(opts: { readonly treasures?: boolean } = {}): Promise<void> {
    if (this.campaignId) {
      await Promise.all([this.loadRows(), this.withAwards ? this.loadAwards() : Promise.resolve()]);
      if (opts.treasures !== false) {
        await this.loadTreasures();
      }
    }
  }

  /** The treasures again (after an award, an undo, or a conversion that went wrong). */
  async loadTreasures(): Promise<void> {
    if (
      !this.withTreasures ||
      this.xpMode() === XpMode.MILESTONES ||
      this.xpMode() === XpMode.UNSPECIFIED
    ) {
      return;
    }
    const seq = ++this.treasuresSeq;
    try {
      const res = await this.api.listTreasures(this.campaignId);
      if (seq === this.treasuresSeq) {
        this.treasures.set(res.treasures);
        this.treasuresTotal.set(res.total);
        this.treasuresState.set('ready');
      }
    } catch {
      // A list that was read before stays (stale, and the sheets read again); with none, say it could not read.
      if (seq === this.treasuresSeq) {
        this.treasuresState.set(this.treasures().length > 0 ? 'ready' : 'error');
      }
    }
  }

  /** "Mostrar mais": the next page of the history. */
  async more(): Promise<void> {
    const token = this.nextPageToken();
    if (!token || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    const seq = this.awardsSeq;
    try {
      const res = await this.api.listAwards(this.campaignId, token);
      if (seq === this.awardsSeq) {
        this.awards.update((a) => [...a, ...res.awards]);
        this.nextPageToken.set(res.nextPageToken);
      }
    } catch {
      // The page stays as it is: "Mostrar mais" is still there to try again.
    } finally {
      this.loadingMore.set(false);
    }
  }

  private async loadRows(): Promise<void> {
    const seq = ++this.rowsSeq;
    try {
      const [exp, entries] = await Promise.all([
        this.api.experience(this.campaignId),
        // The class line is a detail: without it the rows still read.
        this.roster.list(this.campaignId).catch(() => []),
      ]);
      if (seq !== this.rowsSeq) {
        return;
      }
      const byId = new Map(entries.map((e) => [e.id, e]));
      this.xpMode.set(exp.xpMode);
      this.rows.set(
        exp.characters.map((c) => {
          const entry = byId.get(c.characterId);
          const player = (c.playerDisplayName || entry?.playerName || '').trim();
          return {
            id: c.characterId,
            name: c.name,
            playerUserId: c.playerUserId,
            sub: joinDots(
              [entry?.classSummary ?? '', player ? `de ${player}` : ''].filter(Boolean),
            ),
            level: c.level,
            xp: c.experiencePoints,
            nextLevelXp: c.nextLevelXp,
            canLevelUp: c.canLevelUp,
            levelUpReason: c.levelUpReason,
          };
        }),
      );
      this.rowsState.set('ready');
    } catch {
      // A list read before stays (stale); with none, say it could not read, never a spinner for good.
      if (seq === this.rowsSeq) {
        this.rowsState.set(this.rows().length > 0 ? 'ready' : 'error');
      }
    }
  }

  private async loadAwards(): Promise<void> {
    const seq = ++this.awardsSeq;
    try {
      const res = await this.api.listAwards(this.campaignId);
      if (seq === this.awardsSeq) {
        this.awards.set(res.awards);
        this.nextPageToken.set(res.nextPageToken);
        this.awardsState.set('ready');
      }
    } catch {
      if (seq === this.awardsSeq) {
        this.awardsState.set(this.awards().length > 0 ? 'ready' : 'error');
      }
    }
  }
}
