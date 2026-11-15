import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';

import {
  CombatantKind,
  EncounterBlockedReason,
  EncounterBlockedSchema,
  PendingDamageStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { PendingDamages } from './pending-damages';

const part = (over: Record<string, unknown>) => ({
  key: 'weapon',
  labelPt: 'Espada curta',
  diceCount: 1,
  diceSides: 6,
  flat: 3,
  damageTypeKey: 'damage-type:piercing',
  damageTypePt: 'perfurante',
  choosable: false,
  selected: false,
  available: false,
  auto: false,
  ...over,
});
const roll = (over: Record<string, unknown>) => ({
  partKey: 'weapon',
  labelPt: 'Espada curta',
  diceCount: 1,
  diceSides: 6,
  faces: [],
  flat: 3,
  sum: 10,
  counted: true,
  damageTypeKey: 'damage-type:piercing',
  rerolled: [],
  hitEffect: false,
  ...over,
});
const frost = part({
  key: 'frost',
  labelPt: 'Geada no Punho',
  diceCount: 2,
  diceSides: 8,
  flat: 0,
  auto: true,
  damageTypeKey: 'damage-type:cold',
  damageTypePt: 'frio',
});
const sneak = part({
  key: 'sneak-attack',
  labelPt: 'Ataque Furtivo',
  diceCount: 3,
  flat: 0,
  choosable: true,
  selected: true,
  available: true,
});
const frostRoll = roll({
  partKey: 'frost',
  labelPt: 'Geada no Punho',
  diceCount: 2,
  diceSides: 8,
  faces: [3, 3],
  flat: 0,
  sum: 6,
  damageTypeKey: 'damage-type:cold',
  hitEffect: true,
});
const base = { id: 'p1', attackerId: 'jab', targetId: 'zum', amount: 19, steps: [] };
const applied = {
  ...base,
  status: PendingDamageStatus.APPLIED,
  parts: [part({}), sneak, frost],
  partRolls: [
    roll({}),
    roll({ partKey: 'sneak-attack', labelPt: 'Ataque Furtivo', diceCount: 3, flat: 0, sum: 0 }),
    frostRoll,
  ],
} as never;
const waiting = {
  ...base,
  status: PendingDamageStatus.AWAITING_GROUP_SUM,
  awaitedDamageTypeKey: 'damage-type:piercing',
  parts: [part({}), { ...sneak, selected: false, available: false }, frost],
  partRolls: [frostRoll],
} as never;

const enc = encounter({
  combatants: [
    combatant({ id: 'jab', label: 'Jabuti', kind: CombatantKind.PLAYER }),
    combatant({
      id: 'zum',
      label: 'Zumbi 1',
      hitPointsCurrent: 3,
      hitPointsMax: 22,
      hitPointsTemporary: 0,
    } as never),
  ],
} as never);

describe('PendingDamages: taking a part out of a typed group', () => {
  const api = { removeDamagePart: vi.fn(), rollDamage: vi.fn() };

  function setup(pendings: unknown[], appliedRows: unknown[] = []) {
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: fakeCombatClient(api) }],
    });
    const state = new CombatState();
    const fixture = TestBed.createComponent(PendingDamages);
    fixture.componentRef.setInput('pendings', pendings);
    fixture.componentRef.setInput('applied', appliedRows);
    fixture.componentRef.setInput('encounter', enc);
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', state);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    return {
      fixture,
      el,
      state,
      text: () => el.textContent!.replace(/\s+/g, ' '),
      button: (name: string) =>
        Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes(name))!,
    };
  }

  it('draws the applied damage only as its breakdown with "Tirar" on the extra of the group', () => {
    const { el, text } = setup([], [applied]);
    expect(text()).toContain('Espada curta no Zumbi 1');
    expect(text()).toContain('Aplicado · 19');
    expect(text()).toContain('4d6 (10) + 3 · uma soma digitada');
    expect(el.querySelectorAll('.take')).toHaveLength(1);
    expect(text()).not.toContain('Aplicar 19 de dano');
  });

  it('lists what the take-out undoes and sends the reason, keeping it for the card that waits', async () => {
    api.removeDamagePart.mockResolvedValue({ encounter: enc, pending: waiting, cast: [] });
    const { fixture, el, state, text, button } = setup([], [applied]);
    button('Tirar').click();
    fixture.detectChanges();
    expect(text()).toContain('Tirar o Ataque Furtivo do dano?');
    expect(text()).toContain('Desfaz os 19 de dano: o Zumbi 1 volta a 22 PV.');
    expect(text()).toContain('Pede a Jabuti só a soma de perfurante (1d6).');
    const field = el.querySelector<HTMLInputElement>('.ask__field')!;
    field.value = 'Sem aliado perto do alvo';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    await fixture.whenStable();
    expect(api.removeDamagePart).toHaveBeenCalledWith(
      'camp',
      expect.any(String),
      'p1',
      'sneak-attack',
      'Sem aliado perto do alvo',
      expect.any(String),
    );
    expect(state.removals()[0]).toMatchObject({
      key: 'sneak-attack',
      reason: 'Sem aliado perto do alvo',
    });
  });

  it('puts the focus on the head of the card that waits for the sum once the take-out went through', async () => {
    const h = setup([], [applied]);
    api.removeDamagePart.mockImplementation(async () => {
      // The master's options come back with the damage waiting for the sum: the card changes under the panel.
      h.fixture.componentRef.setInput('applied', []);
      h.fixture.componentRef.setInput('pendings', [waiting]);
      h.fixture.detectChanges();
      return { encounter: enc, pending: waiting, cast: [] };
    });
    h.button('Tirar').click();
    h.fixture.detectChanges();
    const field = h.el.querySelector<HTMLInputElement>('.ask__field')!;
    field.value = 'motivo';
    field.dispatchEvent(new Event('input'));
    h.fixture.detectChanges();
    h.el.querySelector('form')!.dispatchEvent(new Event('submit'));
    await h.fixture.whenStable();
    h.fixture.detectChanges();
    await h.fixture.whenStable();
    expect(document.activeElement).toBe(h.el.querySelector('.dmg__head'));
  });

  it('does not show "Tirando…" for a take-out that never started because the card was busy', async () => {
    const h = setup([], [applied]);
    api.removeDamagePart.mockClear();
    const c = h.fixture.componentInstance as unknown as {
      busy: { set(v: boolean): void };
      taking(): string | null;
      takeOut(p: unknown, part: { partKey: string; reason: string }): Promise<void>;
    };
    c.busy.set(true);
    await c.takeOut(applied, { partKey: 'sneak-attack', reason: 'x' });
    expect(api.removeDamagePart).not.toHaveBeenCalled();
    expect(c.taking()).toBeNull();
  });

  it('keeps the panel and says so when the server refuses, nothing changed', async () => {
    api.removeDamagePart.mockRejectedValue(
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: EncounterBlockedSchema,
          value: create(EncounterBlockedSchema, { reason: EncounterBlockedReason.NOTHING_TO_UNDO }),
        },
      ]),
    );
    const { fixture, el, text, button } = setup([], [applied]);
    button('Tirar').click();
    fixture.detectChanges();
    const field = el.querySelector<HTMLInputElement>('.ask__field')!;
    field.value = 'motivo';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    await fixture.whenStable();
    fixture.detectChanges();
    expect(text()).toContain('Já aconteceu outra coisa depois deste dano');
    expect(el.querySelector('.ask')).not.toBeNull();
    // Going back and opening the panel again: the old refusal is not shown as if it were new.
    el.querySelector<HTMLButtonElement>('.ask__back')!.click();
    fixture.detectChanges();
    button('Tirar').click();
    fixture.detectChanges();
    expect(text()).not.toContain('Já aconteceu outra coisa depois deste dano');
  });

  it('shows the damage that awaits the sum: who is awaited, the part taken out with its reason, the sum kept', () => {
    const { el, state, text, button, fixture } = setup([waiting]);
    state.noteRemoval({
      attackerId: 'jab',
      targetId: 'zum',
      key: 'sneak-attack',
      name: 'Ataque Furtivo',
      reason: 'Sem aliado perto do alvo',
    });
    fixture.detectChanges();
    // The same rows as the take-out card: the group head, the parts under it, the taken one struck, the kept sum tagged.
    expect(el.querySelectorAll('ul.rows > li.row--child')).toHaveLength(2);
    expect(el.querySelector('.sumrows')).toBeNull();
    expect(el.querySelector('s')!.textContent).toContain('Ataque Furtivo');
    expect(text()).toMatch(/Geada no Punho\s*automático/);
    expect(text()).toContain('Esperando Jabuti');
    expect(text()).toContain('tirado: “Sem aliado perto do alvo”');
    expect(text()).toContain('Perfurante');
    expect(text()).toContain('esperando a soma de Jabuti');
    expect(text()).toContain('Geada no Punho');
    expect(text()).toContain('Totalfalta');
    expect(text()).toContain(
      'Jabuti digita a soma de 1d6 (de 1 a 6). Até lá, o Zumbi 1 fica com 3 PV.',
    );
    expect(button('Rolar 1d6 no app')).toBeTruthy();
    expect(button('Digitar por Jabuti')).toBeTruthy();
  });

  it('rolls the awaited sum in the app or types it, only of that damage type', async () => {
    api.rollDamage.mockResolvedValue({ encounter: enc, pending: waiting, cast: [] });
    const { fixture, el, button } = setup([waiting]);
    button('Digitar por Jabuti').click();
    fixture.detectChanges();
    const field = el.querySelector<HTMLInputElement>('input')!;
    field.value = '4';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    el.querySelector<HTMLButtonElement>('.type button, .type__row button')!.click();
    await fixture.whenStable();
    expect(api.rollDamage).toHaveBeenCalledWith(
      'camp',
      expect.any(String),
      'p1',
      { sum: 4 },
      expect.any(String),
    );
  });
});
