import { Injectable, computed, inject, signal } from '@angular/core';

import type { Milestone } from '../../../gen/meurpg/progression/v1/progression_pb';
import { ProgressionClient } from './progression-client';
import { type LoadState } from './experience-store';
import { plannedOf, reachedOf } from './milestones';

/**
 * The campaign's milestones as the "Experiência" panel of a milestones
 * campaign reads them (MR-016): the master's planned list and the reached
 * ones, or, for a player, the reached ones only (the server leaves the others
 * out). The panel provides one instance and calls `load`; every change the
 * master makes answers with the whole list, which replaces this one, so the
 * screen never drifts. A read that was overtaken by a newer one is dropped.
 */
@Injectable()
export class MilestonesStore {
  private readonly api = inject(ProgressionClient);

  private campaignId = '';
  private seq = 0;

  readonly list = signal<readonly Milestone[]>([]);
  readonly state = signal<LoadState>('loading');
  readonly planned = computed(() => plannedOf(this.list()));
  readonly reached = computed(() => reachedOf(this.list()));

  async load(campaignId: string): Promise<void> {
    if (campaignId !== this.campaignId) {
      // Another campaign: nothing of the last one may show, and an answer still on its way is dropped.
      this.seq++;
      this.list.set([]);
    }
    this.campaignId = campaignId;
    this.state.set('loading');
    await this.refresh();
  }

  /** Reads again, keeping what is on screen until the answer comes. */
  async refresh(): Promise<void> {
    if (!this.campaignId) {
      return;
    }
    const seq = ++this.seq;
    try {
      const res = await this.api.listMilestones(this.campaignId);
      if (seq === this.seq) {
        this.list.set(res.milestones);
        this.state.set('ready');
      }
    } catch {
      if (seq === this.seq && this.list().length === 0) {
        this.state.set('error');
      }
    }
  }

  /** Takes the list a change answered with. */
  adopt(list: readonly Milestone[]): void {
    this.seq++; // a read still in flight is older than this
    this.list.set(list);
    this.state.set('ready');
  }
}
