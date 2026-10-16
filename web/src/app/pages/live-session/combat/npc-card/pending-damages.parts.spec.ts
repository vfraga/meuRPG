import { TestBed } from '@angular/core/testing';

import { CombatantKind, PendingDamageStatus } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { DamageStepKind } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { PendingDamages } from './pending-damages';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

describe('PendingDamages: the parts of a rolled damage and its resistance steps', () => {
  const pending = {
    id: 'p1',
    attackerId: 'cap',
    targetId: 'tie',
    status: PendingDamageStatus.ROLLED,
    diceCount: 1,
    diceSides: 8,
    bonus: 3,
    damageTypePt: 'fogo',
    amount: 10,
    amountAfterSteps: 5,
    roll: { diceCount: 1, diceSides: 8, faces: [7], modifier: 3, total: 10 },
    parts: [
      { key: 'weapon', labelPt: 'Espada', choosable: false },
      { key: 'sneak-attack', labelPt: 'Ataque Furtivo', choosable: true },
    ],
    partRolls: [
      {
        partKey: 'weapon',
        labelPt: 'Espada',
        diceCount: 1,
        diceSides: 8,
        faces: [7],
        flat: 3,
        sum: 7,
        counted: true,
        rerolled: [],
      },
      {
        partKey: 'sneak-attack',
        labelPt: 'Ataque Furtivo',
        diceCount: 1,
        diceSides: 6,
        faces: [4],
        flat: 0,
        sum: 4,
        counted: true,
        rerolled: [],
      },
    ],
    steps: [
      {
        kind: DamageStepKind.RESISTANCE,
        sourceKeys: ['race:tiefling'],
        labelPt: 'Resistência a fogo (tiefling)',
        before: 10,
        after: 5,
        ignored: false,
      },
    ],
  } as never;
  const enc = encounter({
    combatants: [
      combatant({ id: 'cap', label: 'Capitão Goblin' }),
      combatant({ id: 'tie', label: 'Kai', kind: CombatantKind.PLAYER }),
    ],
  } as never);
  const api = {
    removeDamagePart: vi.fn(),
    applyDamage: vi.fn(),
  };

  function setup() {
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: fakeCombatClient(api) }],
    });
    const fixture = TestBed.createComponent(PendingDamages);
    fixture.componentRef.setInput('pendings', [pending]);
    fixture.componentRef.setInput('encounter', enc);
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', new CombatState());
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const plain = () => el.textContent!.replace(/\s+/g, ' ');
    const button = (name: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes(name))!;
    return { fixture, el, plain, button };
  }

  it('shows a row for each part and the resistance step, and applies the damage after it', () => {
    const { el, plain, button } = setup();
    expect(el.querySelectorAll('tbody tr')).toHaveLength(2);
    expect(plain()).toContain('Resistência a fogo (tiefling): 10 → 5');
    expect(button('Aplicar 5 de dano')).toBeTruthy();
  });

  it('offers "Tirar" only on the extra, asks how far the damage falls and sends the reason', async () => {
    api.removeDamagePart.mockResolvedValue({ encounter: enc, pending, cast: [] });
    const { fixture, el, plain, button } = setup();
    expect(el.querySelectorAll('.take')).toHaveLength(1);
    button('Tirar').click();
    fixture.detectChanges();
    expect(plain()).toContain('Tirar o Ataque Furtivo? O dano cai de 10 para 6.');
    const field = el.querySelector<HTMLInputElement>('.ask__field')!;
    field.value = 'a mesa não usa';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    el.querySelector('.ask form, form.ask')!.dispatchEvent(new Event('submit'));
    await fixture.whenStable();
    expect(api.removeDamagePart).toHaveBeenCalledWith(
      'camp',
      expect.any(String),
      'p1',
      'sneak-attack',
      'a mesa não usa',
      expect.any(String),
    );
  });

  it('sends the sources of the resistances the master chose to ignore when it applies', async () => {
    api.applyDamage.mockResolvedValue({ encounter: enc, pending, cast: [] });
    const { fixture, el, button } = setup();
    el.querySelector<HTMLInputElement>('.step__toggle input')!.click();
    fixture.detectChanges();
    button('Aplicar 5 de dano').click();
    await fixture.whenStable();
    expect(api.applyDamage).toHaveBeenCalledWith(
      'camp',
      expect.any(String),
      'p1',
      undefined,
      expect.any(String),
      ['race:tiefling'],
    );
  });
});
