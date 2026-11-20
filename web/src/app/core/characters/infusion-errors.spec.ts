import { Code, ConnectError } from '@connectrpc/connect';

import {
  InfusionBlockedReason,
  InfusionBlockedSchema,
} from '../../../gen/meurpg/characters/v1/infusions_pb';
import { describeInfusionError } from './infusion-errors';

/** What `describeInfusionError` says when it has no sentence for a reason. */
const GENERIC = 'Não deu para infundir agora.';

function blocked(reason: InfusionBlockedReason): ConnectError {
  return new ConnectError('blocked', Code.FailedPrecondition, undefined, [
    { desc: InfusionBlockedSchema, value: { reason } },
  ]);
}

/** Every value of the generated enum, so a reason added to infusions.proto is checked without touching this spec. */
const REASONS = Object.entries(InfusionBlockedReason)
  .filter((entry): entry is [string, InfusionBlockedReason] => typeof entry[1] === 'number')
  .filter(([, value]) => value !== InfusionBlockedReason.UNSPECIFIED)
  .map(([name, value]) => ({ name, value }));

describe('InfusionBlockedReason sentences', () => {
  it('reads the reasons from the generated enum', () => {
    expect(REASONS.length).toBeGreaterThan(0);
  });

  it('has a Portuguese sentence of its own for every reason the proto declares', () => {
    const missing = REASONS.filter(
      ({ value }) => describeInfusionError(blocked(value), 'Faísca') === GENERIC,
    ).map(({ name }) => name);
    expect(missing).toEqual([]);
  });

  it('names the character that does not know the infusion', () => {
    expect(describeInfusionError(blocked(InfusionBlockedReason.INFUSION_NOT_KNOWN), 'Faísca')).toBe(
      'Faísca não conhece esta infusão.',
    );
  });

  it('keeps apologies out of every sentence', () => {
    for (const { name, value } of REASONS) {
      expect(describeInfusionError(blocked(value), 'Faísca'), name).not.toMatch(
        /desculp|infelizmente|sentimos/i,
      );
    }
  });

  it('leaves any other error to the sheet wording', () => {
    const dead = new ConnectError('x', Code.Aborted);
    expect(describeInfusionError(dead, 'Faísca')).toBe(
      'A ficha mudou enquanto você editava. Recarregue a página e tente de novo.',
    );
  });
});
