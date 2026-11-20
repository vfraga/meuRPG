import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { ReadiedActionSchema, ReadiedKind } from '../../../gen/meurpg/play/v1/combat_pb';
import type { ReleasePlan } from './readied-release';
import { readiedRefusalText } from './readied-refusal';

const held = (kind: ReadiedKind, slotLevel = 1) =>
  create(ReadiedActionSchema, { id: 'r', kind, slotLevel });
const planOf = (kind: ReleasePlan['rule']['kind'], rays = false): ReleasePlan => ({
  rule: { kind, max: 3, min: 1, ...(rays ? { rays } : {}) },
  rows: [],
  total: kind === 'darts' ? 3 : 0,
});

describe('the refusal of the answer of a readied action (#582)', () => {
  it('tells a refused spell that splits darts to check its targets and darts, which the answer does send', () => {
    const refused = new ConnectError(
      'every target of Magic Missile needs at least one dart',
      Code.InvalidArgument,
    );
    const text = readiedRefusalText(
      refused,
      held(ReadiedKind.SPELL),
      'responder à reação',
      planOf('darts'),
    );
    expect(text).toBe(
      'Não deu para soltar a magia com essa escolha. Confira os alvos e os dardos. Deixar passar perde a magia e o espaço.',
    );
    // The answer carries the targets and the darts now: it must not say they are not sent.
    expect(text).not.toContain('ainda não envia');
    expect(text).not.toContain('ponto no mapa');
  });

  it('does not say a cantrip loses a slot, since it spent none', () => {
    const text = readiedRefusalText(
      new ConnectError('x', Code.InvalidArgument),
      held(ReadiedKind.SPELL, 0),
      'responder à reação',
    );
    expect(text).toBe(
      'Não deu para soltar a magia com essa escolha. Confira os alvos. Deixar passar perde a magia.',
    );
  });

  it('speaks of rays for a spell that splits rays', () => {
    const text = readiedRefusalText(
      new ConnectError('x', Code.InvalidArgument),
      held(ReadiedKind.SPELL),
      'responder à reação',
      planOf('darts', true),
    );
    expect(text).toContain('Confira os alvos e os raios.');
  });

  it('asks only for the targets of a spell with no darts, or one whose plan is not known', () => {
    const refused = new ConnectError('too many targets', Code.InvalidArgument);
    const say = (plan?: ReleasePlan | null) =>
      readiedRefusalText(refused, held(ReadiedKind.SPELL), 'responder à reação', plan);
    const asked =
      'Não deu para soltar a magia com essa escolha. Confira os alvos. Deixar passar perde a magia e o espaço.';
    expect(say(planOf('single'))).toBe(asked);
    expect(say(planOf('multi'))).toBe(asked);
    expect(say(null)).toBe(asked);
    expect(say()).toBe(asked);
  });

  it('leaves any other refusal, and any other action, to the usual words', () => {
    const usual = (err: unknown, kind: ReadiedKind) =>
      readiedRefusalText(err, held(kind), 'responder à reação');
    expect(usual(new ConnectError('x', Code.InvalidArgument), ReadiedKind.ATTACK)).not.toContain(
      'dardos',
    );
    expect(usual(new ConnectError('x', Code.FailedPrecondition), ReadiedKind.SPELL)).not.toContain(
      'dardos',
    );
  });
});
