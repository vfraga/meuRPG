import { create } from '@bufbuild/protobuf';

import { CombatantSchema } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { movementOf, turnLineOf } from './turn-line';

const rider = create(CombatantSchema, {
  label: 'Isolda',
  speedDft: 300,
  movementLeftDft: 150,
  movementLeftFt: 15,
});
const horse = create(CombatantSchema, {
  label: 'Cavalo de guerra',
  speedDft: 335,
  movementLeftDft: 335,
  movementLeftFt: 33,
});
const opts = { grappled: false, blocked: false, attacksLeft: 1, attacksPerAction: 1 };

describe('turnLineOf', () => {
  it('shows the rider’s own metres when it walks', () => {
    const line = turnLineOf(rider, opts);
    expect(line.canMove).toBe(true);
    expect(line.shapes.map((s) => s.text).join('|')).toContain('4,5');
  });

  it('shows the mount’s metres while the rider directs it', () => {
    const line = turnLineOf(rider, { ...opts, mount: horse });
    const text = line.shapes.map((s) => s.text).join('|');
    expect(text).toContain('10,1');
    expect(text).not.toContain('4,5');
  });
});

const plain = (t: string) => t.replace(/\u00a0/g, ' ');

describe('movementOf', () => {
  it('reads the rider’s own feet when it walks', () => {
    expect(plain(movementOf(rider, false).pill)).toBe('Restam 4,5 m');
  });

  it('reads the mount’s feet while the rider directs it', () => {
    const m = movementOf(rider, false, horse);
    expect(m.left).toBe(33);
    expect(plain(m.pill)).toBe('Restam 10,1 m');
    expect(m.text).not.toContain('4,5');
  });

  it('a grapple still stops the rider on the mount', () => {
    expect(movementOf(rider, true, horse).pill).toBe('Sem movimento');
  });
});
