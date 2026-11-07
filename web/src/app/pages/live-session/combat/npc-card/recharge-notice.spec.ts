import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';

import { LimitedActionSchema } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { settle } from '../../../../../test-settle';
import { RechargeNotice } from './recharge-notice';

const limited = [
  create(LimitedActionSchema, {
    actionKey: 'a',
    kind: 'recharge',
    usage: 'Recharge 5-6',
    name: 'Fire Breath',
    namePt: 'Sopro de fogo',
  }),
  create(LimitedActionSchema, {
    actionKey: 'b',
    kind: 'recharge',
    usage: 'Recharge 6',
    name: 'Spores',
    namePt: 'Esporos',
  }),
];

async function show(rolls: { action: string; face: number; recharged: boolean }[]) {
  TestBed.resetTestingModule();
  const fixture = TestBed.createComponent(RechargeNotice);
  fixture.componentRef.setInput('rolls', rolls);
  fixture.componentRef.setInput('limited', limited);
  await settle(fixture);
  return (fixture.nativeElement as HTMLElement).querySelector<HTMLElement>(
    '[data-testid="recharge-rolled"]',
  );
}

describe('RechargeNotice', () => {
  it('says nothing when no d6 was rolled', async () => {
    expect(await show([])).toBeNull();
  });

  it('is green when the action recharged', async () => {
    const note = await show([{ action: 'Sopro de fogo', face: 5, recharged: true }]);
    expect(note?.textContent).toContain('Recarga no começo do turno:');
    expect(note?.textContent).toContain('rolou 5 no d6. Sopro de fogo recarregou.');
    expect(note?.classList.contains('mr-notice--success')).toBe(true);
  });

  it('is neutral and says what it needed when it did not recharge', async () => {
    const note = await show([{ action: 'Sopro de fogo', face: 2, recharged: false }]);
    expect(note?.textContent).toContain('Sopro de fogo não recarregou (precisa de 5 ou 6).');
    expect(note?.classList.contains('mr-notice--neutral')).toBe(true);
    expect(note?.classList.contains('mr-notice--success')).toBe(false);
  });

  it('is green only when every action rolled came back, and names each result otherwise', async () => {
    const mixed = await show([
      { action: 'Sopro de fogo', face: 6, recharged: true },
      { action: 'Esporos', face: 3, recharged: false },
    ]);
    expect(mixed?.classList.contains('mr-notice--neutral')).toBe(true);
    expect(mixed?.classList.contains('mr-notice--success')).toBe(false);
    expect(mixed?.textContent).toContain('Sopro de fogo recarregou.');
    expect(mixed?.textContent).toContain('Esporos não recarregou (precisa de 6).');
    const all = await show([
      { action: 'Sopro de fogo', face: 6, recharged: true },
      { action: 'Esporos', face: 6, recharged: true },
    ]);
    expect(all?.classList.contains('mr-notice--success')).toBe(true);
  });
});
