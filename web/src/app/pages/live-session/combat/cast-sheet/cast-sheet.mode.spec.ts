import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CoverDegree, CoverSource } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { ActionEconomy, SpellAttackType } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { CastSheet, type CastSheetData } from './cast-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

function tgt(id: string, label: string, rollMode: RollMode) {
  return {
    combatantId: id,
    label,
    state: 1,
    cover: CoverDegree.NONE,
    coverSource: CoverSource.UNSPECIFIED,
    tooFar: false,
    untargetable: false,
    distanceFt: 30,
    rollMode,
    sources:
      rollMode === RollMode.ADVANTAGE
        ? [{ effect: RollMode.ADVANTAGE, textPt: 'Alvo Derrubado a 1,5 m: vantagem' }]
        : [],
    criticalOnHit: false,
  };
}

async function open(opts: { master?: boolean; diceMode?: DiceMode; targets: unknown[] }) {
  TestBed.resetTestingModule();
  const castSpell = vi.fn().mockRejectedValue(new Error('stop'));
  const bolt = {
    spell: { level: 0 },
    attackType: SpellAttackType.RANGED,
    healBySlotLevel: {},
    damage: [],
    higherLevel: [],
  };
  const state = {
    encounter: signal(encounter({ combatants: [combatant({ id: 'c', label: 'Mago' })] } as never)),
    apply: vi.fn(),
  };
  const data = {
    campaignId: 'c',
    encounterId: 'enc',
    casterId: 'c',
    round: 1,
    spellKey: 'spell:fire-bolt',
    name: 'Raio de Fogo',
    level: 0,
    concentration: false,
    cantripDice: '1d10',
    economy: ActionEconomy.ACTION,
    slots: [],
    usage: [],
    pact: null,
    targets: {
      spellKey: 'spell:fire-bolt',
      targets: opts.targets,
      maxTargets: 1,
      extraTargetPerLevel: false,
      darts: [],
    },
    shieldFree: null,
    shieldName: 'Escudo Arcano',
    attackBonus: 6,
    diceMode: opts.diceMode ?? DiceMode.PLAYERS_CHOOSE,
    preference: DicePreference.APP,
    state,
    master: opts.master,
  } as unknown as CastSheetData;
  TestBed.configureTestingModule({
    providers: [
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: { close: () => undefined } },
      { provide: CombatClient, useValue: fakeCombatClient({ castSpell }) },
      { provide: SpellCatalog, useValue: { details: () => Promise.resolve(bolt) } },
    ],
  });
  const fixture = TestBed.createComponent(CastSheet);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  return { fixture, el, castSpell };
}

describe('CastSheet: the roll mode of a spell attack', () => {
  it('shows the suggestion of the chosen target and its sources in a radio group', async () => {
    const { fixture, el } = await open({
      targets: [tgt('g', 'Goblin', RollMode.ADVANTAGE)],
    });
    el.querySelector<HTMLInputElement>('app-cast-targets input')?.click();
    fixture.detectChanges();
    expect(el.querySelector('[role="radiogroup"][aria-labelledby$="-t"]')).not.toBeNull();
    expect(plain(el.textContent)).toContain('Alvo Derrubado a 1,5 m: vantagem');
    const on = el.querySelector<HTMLInputElement>('.radio input:checked')!;
    expect(on.closest('.radio')!.textContent).toContain('Vantagem');
  });

  it('keeps a player from rolling a better mode than the suggestion: only the master gives it', async () => {
    const { fixture, el } = await open({ targets: [tgt('g', 'Goblin', RollMode.NORMAL)] });
    el.querySelector<HTMLInputElement>('app-cast-targets input')?.click();
    fixture.detectChanges();
    const adv = [...el.querySelectorAll<HTMLElement>('.radio')].find((r) =>
      r.textContent!.includes('Vantagem'),
    )!;
    adv.querySelector('input')!.click();
    fixture.detectChanges();
    expect(plain(el.textContent)).toContain('Só o mestre dá Vantagem aqui.');
    expect(plain(el.textContent)).not.toContain('Pedir ao mestre');
    expect(
      [...el.querySelectorAll('button')].some((b) =>
        plain(b.textContent).includes('Conjurar e rolar'),
      ),
    ).toBe(false);
  });

  it('casts with disadvantage and its reason, asking for two physical d20', async () => {
    const { fixture, el, castSpell } = await open({
      diceMode: DiceMode.PHYSICAL,
      targets: [tgt('g', 'Goblin', RollMode.NORMAL)],
    });
    el.querySelector<HTMLInputElement>('app-cast-targets input')?.click();
    fixture.detectChanges();
    [...el.querySelectorAll<HTMLElement>('.radio')]
      .find((r) => r.textContent!.includes('Desvantagem'))!
      .querySelector('input')!
      .click();
    fixture.detectChanges();
    const reason = el.querySelector<HTMLInputElement>('.reason__field')!;
    reason.value = 'atrapalhado pela fumaça';
    reason.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    const fields = el.querySelectorAll<HTMLInputElement>('app-multi-roll input');
    expect(fields).toHaveLength(2);
    fields[0].value = '12';
    fields[0].dispatchEvent(new Event('input'));
    fields[1].value = '4';
    fields[1].dispatchEvent(new Event('input'));
    fixture.detectChanges();
    el.querySelector('app-multi-roll form')!.dispatchEvent(new Event('submit'));
    await fixture.whenStable();
    const args = castSpell.mock.calls[0];
    expect(args[6]).toEqual({ faces: [12, 4] });
    expect(args[10]).toEqual({ mode: RollMode.DISADVANTAGE, reason: 'atrapalhado pela fumaça' });
  });

  it('lets the master pick advantage', async () => {
    const { fixture, el } = await open({
      master: true,
      targets: [tgt('g', 'Goblin', RollMode.NORMAL)],
    });
    el.querySelector<HTMLInputElement>('app-cast-targets input')?.click();
    fixture.detectChanges();
    [...el.querySelectorAll<HTMLElement>('.radio')]
      .find((r) => r.textContent!.includes('Vantagem'))!
      .querySelector('input')!
      .click();
    fixture.detectChanges();
    expect(plain(el.textContent)).not.toContain('Só o mestre dá');
    expect(el.querySelector('.reason__field')).not.toBeNull();
  });
});
