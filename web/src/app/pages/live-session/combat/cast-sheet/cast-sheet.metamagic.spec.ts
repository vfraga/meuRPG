import { create } from '@bufbuild/protobuf';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantState,
  SpellCastSchema,
  SpellTargetsSchema,
  TargetInReachSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  MetamagicOptionSchema,
  SpellDetailsSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { CastSheet, type CastSheetData } from './cast-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const flat = (n: Element | null | undefined) =>
  n?.textContent?.replace(/\s+/g, ' ').trim().replace(/ /g, ' ') ?? '';

/** The blocks of a row, each on its own line as the browser draws them. */
/** What a block says, without the name of its icon. */
const said = (n: Element | null | undefined) => {
  const copy = n?.cloneNode(true) as Element | undefined;
  copy?.querySelectorAll('mat-icon').forEach((icon) => icon.remove());
  return flat(copy);
};

const lines = (n: Element | null | undefined) =>
  Array.from(n?.children ?? [], (c) => {
    const copy = c.cloneNode(true) as Element;
    copy.querySelectorAll('mat-icon').forEach((icon) => icon.remove());
    return flat(copy);
  }).join(' ');

const option = (key: string, namePt: string, cost: number, why = '') =>
  create(MetamagicOptionSchema, {
    key,
    namePt,
    cost,
    summaryPt: `${namePt}: efeito`,
    allowed: why === '',
    disabledReasonPt: why,
  });

const reach = (id: string, label: string, feet: number) =>
  create(TargetInReachSchema, {
    combatantId: id,
    label,
    state: CombatantState.UNHURT,
    distanceFt: feet,
  });

describe('CastSheet: Metamagic (PM-07c 11)', () => {
  const castSpell = vi.fn();
  const close = vi.fn();

  function open(
    over: {
      points?: { left: number; total: number } | null;
      options?: CastSheetData['metamagic'];
    } = {},
  ) {
    TestBed.resetTestingModule();
    const nael = combatant({ id: 'n', label: 'Nael', mine: true, characterId: 'nc' });
    const data = {
      campaignId: 'c',
      encounterId: 'enc',
      casterId: 'n',
      round: 2,
      spellKey: 'spell:ray-of-frost',
      name: 'Raio de Gelo',
      level: 0,
      concentration: false,
      cantripDice: '1d8',
      economy: ActionEconomy.ACTION,
      slots: [],
      usage: [],
      pact: null,
      targets: create(SpellTargetsSchema, {
        spellKey: 'spell:ray-of-frost',
        maxTargets: 1,
        targets: [reach('g1', 'Goblin 1', 20), reach('g2', 'Goblin 2', 25)],
      }),
      shieldFree: null,
      shieldName: '',
      attackBonus: 0,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state: {
        encounter: signal(encounter({ combatants: [nael] })),
        apply: vi.fn(),
      },
      metamagic: over.options ?? [
        option('feature:metamagic-twinned-spell', 'Magia Duplicada', 1),
        option(
          'feature:metamagic-careful-spell',
          'Magia Cuidadosa',
          1,
          'O Raio de Gelo não pede teste de resistência.',
        ),
      ],
      sorceryPoints: over.points === undefined ? { left: 5, total: 5 } : over.points,
    } as unknown as CastSheetData;
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: fakeCombatClient({ castSpell }) },
        {
          provide: SpellCatalog,
          useValue: { details: () => Promise.resolve(create(SpellDetailsSchema, {})) },
        },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close, disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(CastSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
      await fixture.whenStable();
      fixture.detectChanges();
    };
    const button = (text: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => flat(b).includes(text));
    const meta = () => el.querySelector('app-metamagic-picker');
    const check = (label: string) => {
      const box = Array.from(
        meta()!.querySelectorAll<HTMLInputElement>('input[type=checkbox]'),
      ).find((c) => flat(c.closest('label')).includes(label))!;
      box.checked = !box.checked;
      box.dispatchEvent(new Event('change'));
      fixture.detectChanges();
    };
    const pickRadio = (scope: Element, label: string) => {
      const r = Array.from(scope.querySelectorAll<HTMLInputElement>('input[type=radio]')).find(
        (x) => flat(x.closest('label')).includes(label),
      )!;
      r.checked = true;
      r.dispatchEvent(new Event('change'));
      fixture.detectChanges();
    };
    return { fixture, el, settle, button, meta, check, pickRadio };
  }

  beforeEach(() => {
    castSpell.mockReset();
    close.mockReset();
  });

  it('lists only the known options with their cost, the one the spell does not take greyed with the reason', async () => {
    const { el, settle, meta } = open();
    await settle();
    expect(flat(meta()?.querySelector('h3'))).toBe(
      'Metamagia · 2 opções conhecidas · Pontos de Feitiçaria: 5 de 5',
    );
    const rows = Array.from(meta()!.querySelectorAll('.opt__text'), lines);
    expect(rows[0]).toContain('Magia Duplicada');
    expect(rows[0]).toContain('1 ponto');
    expect(rows[1]).toContain('Magia Cuidadosa');
    expect(rows[1]).toContain('O Raio de Gelo não pede teste de resistência.');
    const boxes = meta()!.querySelectorAll<HTMLInputElement>('input[type=checkbox]');
    expect(boxes[0].disabled).toBe(false);
    expect(boxes[1].disabled).toBe(true);
    expect(el.textContent).not.toContain('Magia Potencializada');
  });

  it('shows no Metamagic at all for a caster that knows none', async () => {
    const { meta, settle } = open({ options: [] });
    await settle();
    expect(meta()).toBeNull();
  });

  it('shows what was chosen, the cost on the button and what is missing (the second target of Twinned Spell)', async () => {
    const { el, settle, check, button } = open();
    await settle();
    check('Magia Duplicada');
    expect(flat(el.querySelector('.mm__line'))).toBe('Você escolheu: Magia Duplicada (1 ponto).');
    // The cast button names the cost; it waits for the spell's own target first.
    expect(button('Conjurar e gastar 1 ponto')).toBeTruthy();
    expect(button('Conjurar Raio de Gelo')).toBeUndefined();
    expect(said(el.querySelector('.missing'))).toBe('Escolha o alvo.');
  });

  it('casts with the option and the second target, and says what it spent and what is left', async () => {
    castSpell.mockResolvedValue({
      encounter: encounter(),
      cast: create(SpellCastSchema, {
        spellKey: 'spell:ray-of-frost',
        metamagicKeys: ['feature:metamagic-twinned-spell'],
        sorceryPointsSpent: 1,
        targets: [],
      }),
      summoned: [],
    });
    const { el, settle, check, button, pickRadio } = open();
    await settle();
    check('Magia Duplicada');
    pickRadio(el.querySelector('app-cast-targets')!, 'Goblin 1');
    // The second target is another creature than the first.
    const second = el.querySelector('app-metamagic-picker app-resource-pick')!;
    expect(flat(second)).toContain('Goblin 2');
    expect(flat(second)).not.toContain('Goblin 1');
    expect(said(el.querySelector('.missing'))).toBe('Escolha o segundo alvo da Magia Duplicada.');
    pickRadio(second, 'Goblin 2');
    expect(el.querySelector('.missing')).toBeNull();
    button('Conjurar e gastar 1 ponto')!.click();
    await settle();
    const args = castSpell.mock.calls[0];
    expect(args[3]).toBe('spell:ray-of-frost');
    expect(args[4]).toBeNull();
    expect(args[5]).toEqual([{ combatantId: 'g1', darts: 0 }]);
    expect(args[11]).toEqual([
      {
        key: 'feature:metamagic-twinned-spell',
        targetIds: ['g2'],
        carefulIds: [],
        heightenedId: '',
      },
    ]);
    expect(flat(el)).toContain('Magia Duplicada · gastou 1 ponto de feitiçaria (restam 4 de 5).');
  });

  it('"Sem Metamagia" unmarks the option and the cast goes without it', async () => {
    const { el, settle, check, button } = open();
    await settle();
    check('Magia Duplicada');
    button('Sem Metamagia')!.click();
    await settle();
    expect(flat(el.querySelector('.mm__line'))).toBe('');
    expect(button('Conjurar Raio de Gelo')).toBeTruthy();
    expect(button('Sem Metamagia')).toBeUndefined();
  });

  it('greys the other options with the reason once one is marked, except Empowered Spell', async () => {
    const { meta, settle, check } = open({
      options: [
        option('feature:metamagic-twinned-spell', 'Magia Duplicada', 1),
        option('feature:metamagic-heightened-spell', 'Magia Aumentada', 3),
        option('feature:metamagic-empowered-spell', 'Magia Potencializada', 1),
      ],
    });
    await settle();
    check('Magia Duplicada');
    const rows = Array.from(meta()!.querySelectorAll('.opt__text'), lines);
    expect(rows[1]).toContain('Só uma opção por magia');
    expect(rows[2]).not.toContain('Só uma opção por magia');
  });

  it('says the points are not enough instead of casting, and sends nothing', async () => {
    const { el, settle, check, button, pickRadio } = open({
      points: { left: 0, total: 5 },
      options: [option('feature:metamagic-twinned-spell', 'Magia Duplicada', 1)],
    });
    await settle();
    check('Magia Duplicada');
    pickRadio(el.querySelector('app-cast-targets')!, 'Goblin 1');
    expect(said(el.querySelector('.missing'))).toBe(
      'Faltam pontos de feitiçaria: custa 1 ponto e você tem 0.',
    );
    button('Conjurar e gastar 1 ponto')!.click();
    await settle();
    expect(castSpell).not.toHaveBeenCalled();
  });
});
