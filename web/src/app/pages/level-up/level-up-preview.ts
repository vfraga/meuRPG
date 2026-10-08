import { signal } from '@angular/core';

import type { LevelUpRefusal } from '../../../gen/meurpg/characters/v1/characters_pb';
import type { DerivedSheet } from '../../../gen/meurpg/rules/v1/rules_pb';
import { LevelUpClient, type LevelUpChoicesInit } from '../../core/levelup/levelup-client';
import { describeLevelUpFailure } from '../../core/levelup/levelup-errors';

/** What the server says the sheet would be with the choices so far. */
export interface PreviewState {
  /** The derived sheet after the level, as far as the choices go; null before the first answer. */
  readonly after: DerivedSheet | null;
  /** The same with the average hit points, for "Média: 4" while the die card is open. */
  readonly afterAverage: DerivedSheet | null;
  /** The first rule the choices break: unset when `LevelUpCharacter` would accept them. */
  readonly refusal: LevelUpRefusal | null;
  /** A read is on its way. */
  readonly loading: boolean;
  /** Why the last read failed ('' when it did not). */
  readonly failed: string;
}

export const QUIET_MS = 150;

/**
 * Asks `PreviewLevelUp` after every change of the choices (MR-040): the browser has
 * no rules engine, so every number of "O que muda" comes from here. Reads are
 * debounced, and an answer that was overtaken by a newer read is dropped, so a slow
 * answer never overwrites a fresh one.
 */
export class LevelUpPreview {
  readonly state = signal<PreviewState>({
    after: null,
    afterAverage: null,
    refusal: null,
    loading: true,
    failed: '',
  });

  private seq = 0;
  private timer: ReturnType<typeof setTimeout> | undefined;

  constructor(
    private readonly client: LevelUpClient,
    private readonly campaignId: string,
    private readonly characterId: string,
  ) {}

  /** Reads now (the first time) or after a short quiet. `average` is the same choices with the average
   * hit points, or null when `choices` already use it. */
  request(
    choices: LevelUpChoicesInit,
    average: LevelUpChoicesInit | null,
    immediately = false,
  ): void {
    clearTimeout(this.timer);
    const seq = ++this.seq;
    this.state.update((s) => ({ ...s, loading: true }));
    const run = async () => {
      try {
        const [now, avg] = await Promise.all([
          this.client.preview(this.campaignId, this.characterId, choices),
          average
            ? this.client.preview(this.campaignId, this.characterId, average)
            : Promise.resolve(null),
        ]);
        if (seq === this.seq) {
          this.state.set({
            after: now.after ?? null,
            afterAverage: (avg ?? now).after ?? null,
            refusal: now.refusal ?? null,
            loading: false,
            failed: '',
          });
        }
      } catch (err) {
        if (seq === this.seq) {
          this.state.update((s) => ({
            ...s,
            loading: false,
            failed: describeLevelUpFailure(err).message,
          }));
        }
      }
    };
    if (immediately) {
      void run();
    } else {
      this.timer = setTimeout(() => void run(), QUIET_MS);
    }
  }

  stop(): void {
    clearTimeout(this.timer);
    this.seq++;
  }
}
