import { create } from '@bufbuild/protobuf';
import { describe, expect, it, vi } from 'vitest';

import { CreatureSchema } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { nextTurn } from '../../../../../test-settle';
import { StatBlockLoad } from './stat-block-load';

const flush = nextTurn;

describe('StatBlockLoad', () => {
  it('loads, then holds the stat block', async () => {
    const load = new StatBlockLoad();
    const block = create(CreatureSchema, {});
    load.read({ statBlock: async () => block }, 'c', 'k', () => 'k');
    expect(load.state()).toBe('loading');
    await flush();
    expect(load.state()).toBe('ready');
    expect(load.block()).toBe(block);
  });

  it('fails, and "Tentar de novo" reads it again', async () => {
    const load = new StatBlockLoad();
    const statBlock = vi
      .fn()
      .mockRejectedValueOnce(new Error('no'))
      .mockResolvedValue(create(CreatureSchema, {}));
    load.read({ statBlock }, 'c', 'k', () => 'k');
    await flush();
    expect(load.state()).toBe('failed');
    load.retry();
    expect(load.state()).toBe('loading');
    await flush();
    expect(load.state()).toBe('ready');
    expect(statBlock).toHaveBeenCalledTimes(2);
  });

  it('drops an answer that comes after the card moved to another creature', async () => {
    const load = new StatBlockLoad();
    load.read({ statBlock: async () => create(CreatureSchema, {}) }, 'c', 'k', () => 'other');
    await flush();
    expect(load.block()).toBeNull();
    expect(load.state()).toBe('loading');
  });

  it('is idle for a creature with no stat block', () => {
    const load = new StatBlockLoad();
    load.reset(false);
    expect(load.state()).toBe('idle');
  });
});
