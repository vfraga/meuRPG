import { signal } from '@angular/core';

import type { Creature } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import type { CreaturesClient } from '../../../../core/creatures/creatures-client';

/** Where the card's read of the creature's stat block is: its "Outras ações" show a skeleton while it loads and "Tentar de novo" when it cannot be read. */
export type StatBlockState = 'idle' | 'loading' | 'failed' | 'ready';

/** The creature's stat block as the card reads it: `null` until read, and when it cannot be. The client keeps what it read once per kind. */
export class StatBlockLoad {
  readonly block = signal<Creature | null>(null);
  readonly state = signal<StatBlockState>('idle');
  private again = (): void => undefined;

  /** Reads the stat block of `key`; an answer that comes after the card moved to another creature (`current()` differs) is dropped. */
  read(
    creatures: Pick<CreaturesClient, 'statBlock'>,
    campaignId: string,
    key: string,
    current: () => string,
  ): void {
    this.again = () => this.read(creatures, campaignId, key, current);
    this.state.set('loading');
    creatures.statBlock(campaignId, key).then(
      (block) => {
        if (current() === key) {
          this.block.set(block);
          this.state.set('ready');
        }
      },
      () => {
        if (current() === key) {
          this.state.set('failed');
        }
      },
    );
  }

  /** "Tentar de novo": the last read, made again. */
  retry(): void {
    this.again();
  }

  /** A new creature: nothing read yet, and loading when it has a stat block to read. */
  reset(hasBlock: boolean): void {
    this.block.set(null);
    this.state.set(hasBlock ? 'loading' : 'idle');
  }
}
