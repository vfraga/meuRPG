import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantSide,
  EncounterMode,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  EffectModifierKind,
  EffectRollKind,
  LastingEffectSchema,
} from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import type { Attack } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { AttackSheet, type AttackSheetData } from './attack-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();
const longsword = {
  key: 'attack:longsword',
  name: 'Longsword',
  namePt: 'Espada longa',
  attackBonus: 5,
  saveDc: 0,
  rangeFt: 5,
  longRangeFt: 0,
  damage: '1d8 + 3',
  damageTypePt: 'cortante',
  kind: 0,
} as unknown as Attack;

const blessing = create(LastingEffectSchema, {
  id: 'e1',
  sourceNamePt: 'Bênção',
  modifiers: [
    {
      kind: EffectModifierKind.ROLL_DIE,
      die: 4,
      sign: 1,
      appliesTo: [EffectRollKind.ATTACK, EffectRollKind.SAVE],
    },
  ],
});

describe('AttackSheet: the dice an effect adds (RN-22)', () => {
  const api = { rollAttack: vi.fn() };

  function setup(opts: { effects?: unknown[]; diceMode?: DiceMode; useExtraAction?: boolean }) {
    const toren = combatant({
      id: 't',
      label: 'Toren',
      kind: CombatantKind.PLAYER,
      side: CombatantSide.PARTY,
      mine: true,
      effects: opts.effects ?? [],
    } as never);
    const state = new CombatState();
    state.apply(
      encounter({
        mode: EncounterMode.THEATRE,
        currentCombatantId: 't',
        combatants: [toren, combatant({ id: 'cap', label: 'Capitão Goblin' })],
      }),
    );
    const data: AttackSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 't',
      round: 3,
      attack: longsword,
      targets: [],
      diceMode: opts.diceMode ?? DiceMode.PHYSICAL,
      preference: DicePreference.APP,
      state,
      opportunity: {
        offerId: 'o1',
        targetId: 'cap',
        targetLabel: 'Capitão Goblin',
        byMaster: false,
      },
      ...(opts.useExtraAction ? { useExtraAction: true } : {}),
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: () => undefined } },
        { provide: CombatClient, useValue: fakeCombatClient(api) },
      ],
    });
    const fixture = TestBed.createComponent(AttackSheet);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const fill = (fixture: { detectChanges(): void }, input: HTMLInputElement, value: string) => {
    input.value = value;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
  };
  const fieldOf = (el: HTMLElement, label: string) => {
    const l = Array.from(el.querySelectorAll('label')).find((x) =>
      plain(x.textContent).includes(label),
    )!;
    return el.querySelector<HTMLInputElement>(`#${l.getAttribute('for')}`)!;
  };
  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      plain(b.textContent).includes(label),
    )!;

  beforeEach(() => {
    api.rollAttack.mockReset();
    // jsdom has no scrollTo: the sheet scrolls an error into view.
    Element.prototype.scrollTo = vi.fn();
  });

  it('asks the d4 of Bênção beside the d20 with physical dice, and refuses to send without it', async () => {
    const { fixture, el } = setup({ effects: [blessing] });
    expect(plain(el.textContent)).toContain('Resultado do d4 (Bênção)');
    expect(plain(el.textContent)).toContain(
      'Bênção soma 1d4 a esta jogada. Role um d4 além do d20.',
    );
    fill(fixture, fieldOf(el, 'Role 1d20'), '14');
    button(el, 'Confirmar').click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(api.rollAttack).not.toHaveBeenCalled();
    expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain(
      'Digite o resultado do d4 antes de confirmar.',
    );
  });

  it('sends the d20 and the faces of the d4 together, and spends the extra action when asked', async () => {
    api.rollAttack.mockRejectedValue(new ConnectError('stop', Code.Aborted));
    const { fixture, el } = setup({ effects: [blessing], useExtraAction: true });
    expect(plain(el.textContent)).toContain('Ação extra');
    fill(fixture, fieldOf(el, 'Resultado do d4 (Bênção)'), '3');
    fill(fixture, fieldOf(el, 'Role 1d20'), '14');
    button(el, 'Confirmar').click();
    await fixture.whenStable();
    const args = api.rollAttack.mock.calls[0];
    expect(args[5]).toEqual({ face: 14 });
    expect(args[11]).toEqual({ extraDieFaces: [3], useExtraAction: true });
  });

  it('draws no d4 field without an effect that adds one, nor with the app rolling the dice', () => {
    expect(plain(setup({}).el.textContent)).not.toContain('d4');
    TestBed.resetTestingModule();
    const app = setup({ effects: [blessing], diceMode: DiceMode.APP });
    expect(plain(app.el.textContent)).not.toContain('Resultado do d4');
  });

  it('asks for the d4 a refusal says the roll takes, for an effect the sheet did not list', async () => {
    api.rollAttack.mockRejectedValue(
      new ConnectError(
        'the roll takes 1 more die(s): type their faces in extra_die_faces',
        Code.InvalidArgument,
      ),
    );
    const { fixture, el } = setup({});
    fill(fixture, fieldOf(el, 'Role 1d20'), '14');
    button(el, 'Confirmar').click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain(
      'Esta rolagem leva mais um d4',
    );
    expect(plain(el.textContent)).toContain('Resultado do d4 (Outra fonte)');
  });
});
