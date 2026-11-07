import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantSide,
  EncounterMode,
  type ManeuverDieOption,
  ManeuverDieOptionSchema,
  TargetInReachSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ResourceBlockedReason,
  ResourceBlockedSchema,
} from '../../../../../gen/meurpg/play/v1/resources_pb';
import {
  AttackKind,
  AttackSchema,
  RollDieSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import {
  EffectModifierKind,
  EffectRollKind,
  type LastingEffect,
  LastingEffectSchema,
} from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import { RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { AttackSheet, type AttackSheetData } from './attack-sheet';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();
const longsword = create(AttackSchema, {
  key: 'attack:longsword',
  name: 'Longsword',
  namePt: 'Espada longa',
  attackBonus: 6,
  rangeFt: 5,
  damage: '1d8 + 3',
  damageTypePt: 'cortante',
  kind: AttackKind.WEAPON,
});
const blessing = create(LastingEffectSchema, {
  id: 'e1',
  sourceNamePt: 'Bênção',
  modifiers: [
    {
      kind: EffectModifierKind.ROLL_DIE,
      die: 4,
      sign: 1,
      appliesTo: [EffectRollKind.ATTACK],
    },
  ],
});

const option = (key: string, name: string, usesLeft: number): ManeuverDieOption =>
  create(ManeuverDieOptionSchema, {
    die: create(RollDieSchema, { key, namePt: name, applies: 'attack', sides: 8 }),
    resourceNamePt: 'Reserva de Golpes',
    usesLeft,
  });
const certeiro = option('golpe', 'Golpe Certeiro', 2);

describe('AttackSheet: the table die of a weapon attack (Golpe Certeiro)', () => {
  const api = { rollAttack: vi.fn(), turnOptions: vi.fn() };
  /** What the last `rollAttack` was sent: the reaction flag, the offer it answers and the extras (the die). */
  const lastArgs = () => {
    const call = api.rollAttack.mock.calls.at(-1) ?? [];
    return { reaction: call[7], offer: call[8], extras: call[11] };
  };

  function setup(
    opts: {
      dice?: readonly ManeuverDieOption[];
      diceMode?: DiceMode;
      advantage?: boolean;
      effects?: LastingEffect[];
    } = {},
  ) {
    const toren = combatant({
      id: 't',
      label: 'Toren',
      kind: CombatantKind.PLAYER,
      side: CombatantSide.PARTY,
      mine: true,
      effects: opts.effects ?? [],
    });
    const state = new CombatState();
    state.apply(
      encounter({
        mode: EncounterMode.THEATRE,
        currentCombatantId: 't',
        combatants: [toren, combatant({ id: 'cap', label: 'Capitão bandido' })],
      }),
    );
    const data: AttackSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 't',
      round: 3,
      attack: longsword,
      diceMode: opts.diceMode ?? DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state,
      ...(opts.advantage
        ? {
            target: 'cap',
            targets: [
              create(TargetInReachSchema, {
                combatantId: 'cap',
                label: 'Capitão bandido',
                rollMode: RollMode.ADVANTAGE,
                sources: [],
              }),
            ],
          }
        : {
            targets: [],
            asReaction: true,
            opportunity: {
              offerId: 'o1',
              targetId: 'cap',
              targetLabel: 'Capitão bandido',
              byMaster: false,
            },
          }),
      ...(opts.dice ? { maneuverDice: opts.dice } : {}),
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
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        await fixture.whenStable();
        fixture.detectChanges();
      }
    };
    const box = () => el.querySelector<HTMLInputElement>('app-roll-die-offer input')!;
    const button = (label: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        plain(b.textContent).includes(label),
      )!;
    const fill = (label: string, value: string) => {
      const l = Array.from(el.querySelectorAll('label')).find((x) =>
        plain(x.textContent).includes(label),
      )!;
      const input = el.querySelector<HTMLInputElement>(`#${l.getAttribute('for')}`)!;
      input.value = value;
      input.dispatchEvent(new Event('input'));
      fixture.detectChanges();
    };
    return { fixture, el, settle, box, button, fill };
  }

  beforeEach(() => {
    api.rollAttack.mockReset().mockRejectedValue(new ConnectError('stop', Code.Aborted));
    api.turnOptions.mockReset();
    Element.prototype.scrollTo = vi.fn();
  });

  it('offers the die above the roll buttons, with its uses', () => {
    const { el } = setup({ dice: [certeiro] });
    const text = plain(el.textContent);
    expect(text).toContain('Dados das suas características');
    expect(text).toContain('Usar Golpe Certeiro');
    expect(text).toContain('d8 somado ao ataque · gasta 1 de Reserva de Golpes · restam 2');
    const offer = el.querySelector('app-roll-die-offer')!;
    const roll = Array.from(el.querySelectorAll('button')).find((b) =>
      plain(b.textContent).includes('Rolar no app'),
    )!;
    expect(offer.compareDocumentPosition(roll) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('offers nothing without dice (a spell attack, an NPC, or an attacker with none)', () => {
    expect(setup({}).el.querySelector('app-roll-die-offer')).toBeNull();
    TestBed.resetTestingModule();
    expect(setup({ dice: [] }).el.querySelector('app-roll-die-offer')).toBeNull();
  });

  it('sends the maneuver key when ticked, and none when not', async () => {
    const ticked = setup({ dice: [certeiro] });
    ticked.box().click();
    ticked.fixture.detectChanges();
    ticked.button('Rolar no app').click();
    await ticked.settle();
    // An opportunity attack: it spends the reaction, names the offer, and carries the die all the same.
    expect(lastArgs().reaction).toBe(true);
    expect(lastArgs().offer).toBe('o1');
    expect(lastArgs().extras).toEqual({
      extraDieFaces: [],
      useExtraAction: false,
      legendaryOption: '',
      maneuverKey: 'golpe',
    });
    TestBed.resetTestingModule();
    api.rollAttack.mockClear();
    const plainRoll = setup({ dice: [certeiro] });
    plainRoll.button('Rolar no app').click();
    await plainRoll.settle();
    expect(lastArgs().extras).toEqual({
      extraDieFaces: [],
      useExtraAction: false,
      legendaryOption: '',
    });
  });

  it('asks the d8 beside the d20 with physical dice, only while ticked, and sends its face last', async () => {
    const { fixture, el, box, button, fill, settle } = setup({
      dice: [certeiro],
      diceMode: DiceMode.PHYSICAL,
    });
    expect(plain(el.textContent)).not.toContain('Resultado do d8');
    box().click();
    fixture.detectChanges();
    expect(plain(el.textContent)).toContain('Resultado do d8 (Golpe Certeiro)');
    fill('Resultado do d8', '5');
    fill('Role 1d20', '12');
    expect(plain(el.textContent)).toContain('12 + 6 + 5 = 23');
    expect(plain(el.textContent)).toContain('Confirmar 23');
    button('Confirmar').click();
    await settle();
    const args = api.rollAttack.mock.calls[0];
    expect(args[5]).toEqual({ face: 12 });
    expect(args[11]).toEqual({
      extraDieFaces: [5],
      useExtraAction: false,
      legendaryOption: '',
      maneuverKey: 'golpe',
    });
    box().click();
    fixture.detectChanges();
    expect(plain(el.textContent)).not.toContain('Resultado do d8');
  });

  it('refuses to send the d20 alone while the die is ticked and its face missing', async () => {
    const { fixture, el, box, button, fill, settle } = setup({
      dice: [certeiro],
      diceMode: DiceMode.PHYSICAL,
    });
    box().click();
    fixture.detectChanges();
    fill('Role 1d20', '12');
    button('Confirmar').click();
    await settle();
    expect(api.rollAttack).not.toHaveBeenCalled();
    expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain(
      'Digite o resultado do d8 antes de confirmar.',
    );
    fixture.detectChanges();
  });

  it('says nothing was spent when the server finds no use left, and reads the uses again', async () => {
    api.rollAttack.mockRejectedValue(
      new ConnectError('no use', Code.FailedPrecondition, undefined, [
        {
          desc: ResourceBlockedSchema,
          value: create(ResourceBlockedSchema, { reason: ResourceBlockedReason.NO_USES_LEFT }),
        },
      ]),
    );
    api.turnOptions.mockResolvedValue({ maneuverDice: [option('golpe', 'Golpe Certeiro', 0)] });
    const { fixture, el, box, button, settle } = setup({ dice: [certeiro] });
    box().click();
    fixture.detectChanges();
    button('Rolar no app').click();
    await settle();
    expect(plain(el.querySelector('[role="alert"]')?.textContent)).toContain(
      'Golpe Certeiro: não há usos de Reserva de Golpes. Nada foi gasto.',
    );
    expect(api.turnOptions).toHaveBeenCalledWith('c', 'enc', 't');
    expect(plain(el.textContent)).toContain('sem usos');
    expect(plain(el.textContent)).toContain('Sem usos de Reserva de Golpes.');
    expect(box().disabled).toBe(true);
    expect(box().checked).toBe(false);
  });

  it('adds the typed d8 to the total of the two d20 too (advantage)', async () => {
    const { fixture, el, box, fill } = setup({
      dice: [certeiro],
      diceMode: DiceMode.PHYSICAL,
      advantage: true,
    });
    box().click();
    fixture.detectChanges();
    fill('Resultado do d8', '5');
    fill('Primeiro d20', '12');
    fill('Segundo d20', '4');
    expect(plain(el.textContent)).toContain('23');
    expect(plain(el.querySelector('.type__sum')?.textContent)).toContain('23');
  });

  it('adds the d4 of an effect to the total with the die', () => {
    const { fixture, el, box, fill } = setup({
      dice: [certeiro],
      diceMode: DiceMode.PHYSICAL,
      effects: [blessing],
    });
    box().click();
    fixture.detectChanges();
    fill('Resultado do d4 (Bênção)', '3');
    fill('Resultado do d8', '5');
    fill('Role 1d20', '12');
    expect(plain(el.textContent)).toContain('12 + 6 + 3 + 5 = 26');
    expect(plain(el.textContent)).toContain('Confirmar 26');
  });
});
