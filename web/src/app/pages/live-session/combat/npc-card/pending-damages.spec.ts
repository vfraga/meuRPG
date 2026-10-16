import { TestBed } from '@angular/core/testing';

import {
  CombatantKind,
  CriticalDamageRule,
  PendingDamageStatus,
  ReactionKind,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CONFIRM_GUARD_MS, ConfirmGuard } from '../../../../core/confirm-guard/confirm-guard';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { PendingDamages } from './pending-damages';

// With --coverage test-setup.ts only hooks the first file of a worker, so the fake clock this file leaves on would
// reach the next file (a real setTimeout never fires on a fake clock).
afterEach(() => {
  vi.useRealTimers();
});

describe('PendingDamages, a hit that waits for a reaction (PM-04)', () => {
  const pending = (status: PendingDamageStatus) =>
    ({
      id: 'p1',
      attackerId: 'cap',
      targetId: 'pen',
      status,
      diceCount: 1,
      diceSides: 6,
      bonus: 2,
    }) as never;
  const combatants = [
    combatant({ id: 'cap', label: 'Capitão Goblin' }),
    combatant({ id: 'pen', label: 'Pensantus', kind: CombatantKind.PLAYER }),
    combatant({ id: 'm1', label: 'Mago 1' }),
  ];

  function setup(status: PendingDamageStatus, windows: ReturnType<typeof reactionWindow>[]) {
    TestBed.configureTestingModule({ providers: [{ provide: CombatClient, useValue: {} }] });
    const fixture = TestBed.createComponent(PendingDamages);
    fixture.componentRef.setInput('pendings', [pending(status)]);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ combatants, reactionWindows: windows } as never),
    );
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', new CombatState());
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }
  const buttons = (el: HTMLElement) =>
    Array.from(el.querySelectorAll('button')).map((b) => b.textContent?.trim());

  it('turns "Rolar dano" grey and dashed with the reason written, and has no answer of its own', () => {
    const el = setup(PendingDamageStatus.AWAITING_REACTION, [
      reactionWindow({ id: 'w1', reactorId: 'm1', reactorLabel: 'Mago 1' }),
    ]);
    const off = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Rolar dano'),
    )!;
    expect(off.getAttribute('aria-disabled')).toBe('true');
    expect(
      el
        .querySelector(`#${off.getAttribute('aria-describedby')}`)
        ?.textContent?.replace(/\u00a0/g, ' '),
    ).toContain('Espere a reação do Mago 1.');
    // The answer is the master's card (the queue), not this box.
    expect(buttons(el).some((b) => b?.includes('Usar'))).toBe(false);
  });

  it('holds a damage still to roll while any window is open, and says to answer the request above for the check', () => {
    const el = setup(PendingDamageStatus.AWAITING_ROLL, [
      reactionWindow({ id: 'w1', kind: ReactionKind.MASTER_CHECK }),
    ]);
    expect(el.textContent).toContain('Responda ao pedido acima.');
    expect(el.querySelector('app-roll-picker')).toBeNull();
  });

  it('offers the roll again once no window is open', () => {
    const el = setup(PendingDamageStatus.AWAITING_ROLL, []);
    expect(el.querySelector('app-roll-picker')).not.toBeNull();
  });
});

describe('PendingDamages, the critical hint (RN-24)', () => {
  const enc = encounter({
    combatants: [
      combatant({ id: 'cap', label: 'Capitão Goblin' }),
      combatant({ id: 'pen', label: 'Pensantus', kind: CombatantKind.PLAYER }),
    ],
  } as never);

  function setup(over: Record<string, unknown>) {
    TestBed.configureTestingModule({ providers: [{ provide: CombatClient, useValue: {} }] });
    const fixture = TestBed.createComponent(PendingDamages);
    const pending = {
      id: 'p1',
      attackerId: 'cap',
      targetId: 'pen',
      status: PendingDamageStatus.AWAITING_ROLL,
      diceSides: 6,
      bonus: 2,
      ...over,
    } as never;
    fixture.componentRef.setInput('pendings', [pending]);
    fixture.componentRef.setInput('encounter', enc);
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', new CombatState());
    fixture.detectChanges();
    return fixture;
  }
  const hint = (el: HTMLElement) =>
    (el.querySelector('[data-testid="critical-hint"]')?.textContent ?? '')
      .replace(/\s+/g, ' ')
      .trim();

  const typeIt = (fixture: ReturnType<typeof setup>) => {
    [...(fixture.nativeElement as HTMLElement).querySelectorAll('button')]
      .find((b) => (b.textContent ?? '').includes('Digitar o resultado'))!
      .click();
    fixture.detectChanges();
  };
  const clean = (el: HTMLElement) => (el.textContent ?? '').replace(/\s+/g, ' ');

  it('says "role os dados duas vezes" for a critical hit under the SRD rule, once the master types a physical roll', () => {
    const fixture = setup({
      diceCount: 4,
      critical: true,
      criticalRule: CriticalDamageRule.DOUBLED_DICE,
    });
    const el = fixture.nativeElement as HTMLElement;
    // With the app's dice the server rolls them: no hint.
    expect(hint(el)).toBe('');
    typeIt(fixture);
    expect(hint(el)).toBe('Acerto crítico: role os dados duas vezes (4d6 no total).');
  });

  it('says "o máximo mais uma rolagem", with the maximum as the fixed part and the live total', () => {
    const fixture = setup({
      diceCount: 2,
      critical: true,
      criticalRule: CriticalDamageRule.MAX_PLUS_ROLL,
      criticalMax: 12,
    });
    const el = fixture.nativeElement as HTMLElement;
    typeIt(fixture);
    expect(hint(el)).toBe(
      'Acerto crítico: o máximo mais uma rolagem. O máximo dos dados (12) já vale sem rolar; role 2d6 uma vez.',
    );
    // One instruction, the fixed parts named, and the live total the one the server records (typed 7 + 12 + 2 = 21).
    expect(clean(el)).toContain('Role 2d6 e digite só o que saiu, de 2 a 12. O app soma o resto.');
    expect(clean(el)).toContain('+ 12 do crítico + 2 de modificador');
    const field = el.querySelector(
      'input.type__field, input[type="text"], input',
    ) as HTMLInputElement;
    field.value = '7';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(clean(el)).toContain('7 + 14 = 21');
  });

  it('has no hint for a hit that is not critical', () => {
    expect(hint(setup({ diceCount: 1 }).nativeElement)).toBe('');
  });
});

describe("PendingDamages: the damage of a player's critical with Crítico Brutal, waiting for the master (PM-03b)", () => {
  it("reads the groups of dice with the feature's name", () => {
    const rolled = {
      id: 'p9',
      attackerId: 'rag',
      targetId: 'hob',
      status: PendingDamageStatus.ROLLED,
      diceCount: 2,
      diceSides: 12,
      bonus: 3,
      amount: 25,
      damageTypePt: 'cortante',
      critical: true,
      criticalRule: CriticalDamageRule.DOUBLED_DICE,
      criticalMax: 0,
      extraDiceCount: 1,
      extraDiceNamePt: 'Crítico Brutal',
      roll: {
        diceCount: 3,
        diceSides: 12,
        faces: [7, 11, 4],
        modifier: 3,
        total: 25,
        physical: false,
      },
    } as never;
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: {} }],
    });
    const fixture = TestBed.createComponent(PendingDamages);
    fixture.componentRef.setInput('pendings', [rolled]);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        combatants: [
          combatant({ id: 'rag', label: 'Ragna', kind: CombatantKind.PLAYER }),
          combatant({ id: 'hob', label: 'Hobgoblin' }),
        ],
      }),
    );
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', new CombatState());
    fixture.detectChanges();
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('.dmg__formula')?.textContent?.trim(),
    ).toBe('2d12 (7, 11) + 1d12 Crítico Brutal (4) + 3 = 25 de dano cortante');
  });
});

describe('PendingDamages: rolling again after "Desfazer" (R2-11)', () => {
  it('sends a new idempotency key for the same damage, so the server rolls it instead of replaying the old answer', async () => {
    const keys: string[] = [];
    const enc = encounter({
      combatants: [
        combatant({ id: 'cap', label: 'Capitão Goblin' }),
        combatant({ id: 'pen', label: 'Pensantus', kind: CombatantKind.PLAYER }),
      ],
    } as never);
    const api = {
      rollDamage: async (_c: string, _e: string, _p: string, _d: unknown, key: string) => {
        keys.push(key);
        return { encounter: enc, pending: {}, cast: [] };
      },
    };
    TestBed.configureTestingModule({ providers: [{ provide: CombatClient, useValue: api }] });
    const fixture = TestBed.createComponent(PendingDamages);
    const p = {
      id: 'p1',
      attackerId: 'cap',
      targetId: 'pen',
      status: PendingDamageStatus.AWAITING_ROLL,
      diceCount: 1,
      diceSides: 6,
      bonus: 2,
    };
    fixture.componentRef.setInput('pendings', [p]);
    fixture.componentRef.setInput('encounter', enc);
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', new CombatState());
    fixture.detectChanges();
    const sheet = fixture.componentInstance as unknown as {
      rollInApp(p: unknown): Promise<void>;
    };
    await sheet.rollInApp(p);
    await sheet.rollInApp(p);
    expect(keys).toHaveLength(2);
    expect(keys[0]).not.toBe(keys[1]);
  });

  it('keeps the key when the answer was lost, so a retry never rolls twice', async () => {
    const keys: string[] = [];
    const enc = encounter({ combatants: [] } as never);
    let fail = true;
    const api = {
      rollDamage: async (_c: string, _e: string, _p: string, _d: unknown, key: string) => {
        keys.push(key);
        if (fail) {
          fail = false;
          throw new Error('lost');
        }
        return { encounter: enc, pending: {}, cast: [] };
      },
    };
    TestBed.configureTestingModule({ providers: [{ provide: CombatClient, useValue: api }] });
    const fixture = TestBed.createComponent(PendingDamages);
    const p = {
      id: 'p1',
      attackerId: 'cap',
      targetId: 'pen',
      status: PendingDamageStatus.AWAITING_ROLL,
      diceCount: 1,
      diceSides: 6,
    };
    fixture.componentRef.setInput('pendings', [p]);
    fixture.componentRef.setInput('encounter', enc);
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', new CombatState());
    fixture.detectChanges();
    const sheet = fixture.componentInstance as unknown as {
      rollInApp(p: unknown): Promise<void>;
    };
    await sheet.rollInApp(p);
    await sheet.rollInApp(p);
    expect(keys[0]).toBe(keys[1]);
  });
});

describe('PendingDamages: "Não aplicar" is not confirmed by a double-click (R4)', () => {
  async function setup() {
    vi.useFakeTimers();
    const discardDamage = vi.fn(async () => ({
      encounter: encounter({ combatants: [] } as never),
    }));
    const rolled = {
      id: 'p1',
      attackerId: 'cap',
      targetId: 'pen',
      status: PendingDamageStatus.ROLLED,
      diceCount: 1,
      diceSides: 6,
      bonus: 2,
      amount: 11,
      critical: true,
      roll: { diceCount: 1, diceSides: 6, faces: [5], modifier: 2, total: 11, physical: false },
    } as never;
    TestBed.configureTestingModule({
      providers: [{ provide: CombatClient, useValue: { discardDamage } }],
    });
    TestBed.inject(ConfirmGuard).start();
    const fixture = TestBed.createComponent(PendingDamages);
    fixture.componentRef.setInput('pendings', [rolled]);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        combatants: [
          combatant({ id: 'cap', label: 'Capitão Goblin' }),
          combatant({ id: 'pen', label: 'Garrick', kind: CombatantKind.PLAYER }),
        ],
      }),
    );
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', new CombatState());
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const byText = (text: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.trim() === text)!;
    byText('Não aplicar').click();
    fixture.detectChanges();
    await vi.advanceTimersByTimeAsync(0);
    fixture.detectChanges();
    return { fixture, byText, discardDamage };
  }

  it('ignores the second click of a double-click on "Descartar", and works after the guard', async () => {
    const { byText, discardDamage } = await setup();
    byText('Descartar').click();
    await vi.advanceTimersByTimeAsync(300);
    byText('Descartar').click();
    expect(discardDamage).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(CONFIRM_GUARD_MS);
    byText('Descartar').click();
    expect(discardDamage).toHaveBeenCalledTimes(1);
  });

  it('puts the focus on the safe choice, which sits where "Não aplicar" was', async () => {
    const { byText } = await setup();
    expect(document.activeElement).toBe(byText('Voltar'));
    const row = byText('Voltar').parentElement!;
    expect(Array.from(row.querySelectorAll('button')).map((b) => b.textContent?.trim())).toEqual([
      'Descartar',
      'Voltar',
    ]);
  });
});

describe('PendingDamages, a target in Wild Shape', () => {
  function line(amount: number, druid: Record<string, unknown> = {}) {
    TestBed.configureTestingModule({ providers: [{ provide: CombatClient, useValue: {} }] });
    const fixture = TestBed.createComponent(PendingDamages);
    fixture.componentRef.setInput('pendings', [
      {
        id: 'p1',
        attackerId: 'gob',
        targetId: 'nina',
        status: PendingDamageStatus.ROLLED,
        diceCount: 1,
        diceSides: 6,
        bonus: 2,
        amount,
      } as never,
    ]);
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        combatants: [
          combatant({ id: 'gob', label: 'Goblin 1' }),
          combatant({
            id: 'nina',
            label: 'Nina Folhaverde',
            kind: CombatantKind.PLAYER,
            hitPointsCurrent: 31,
            hitPointsMax: 31,
            wildShapeBeastKey: 'monster:wolf',
            wildShapeBeastNamePt: 'Lobo',
            wildShapeHitPointsCurrent: 11,
            wildShapeHitPointsMax: 11,
            ...druid,
          } as never),
        ],
      } as never),
    );
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('state', new CombatState());
    fixture.detectChanges();
    return (fixture.nativeElement as HTMLElement).textContent?.replace(/ /g, ' ') ?? '';
  }

  it("previews the beast's pool, not the druid's", () => {
    const text = line(7);
    expect(text).toContain('Lobo: 11 de 11 PV, depois 4');
    expect(text).not.toContain('31 de 31');
  });

  it('says what passes to the druid when the beast drops', () => {
    expect(line(14)).toContain(
      'Lobo: 11 de 11 PV, depois 0; o resto (3) passa para Nina Folhaverde: 31 de 31 PV, depois 28',
    );
  });
});
