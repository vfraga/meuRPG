import { create } from '@bufbuild/protobuf';

import { catalog } from './content-testing';
import { Ability, SpellRangeKind, SpellSaveSuccess } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableAreaShape,
  TableEffectApplies,
  TableEffectModifierKind,
  TableSpellEffectSchema,
  TableSpellSchema,
  TableSpellTargetKind,
  TableSpellZoneSchema,
  TableZoneTrigger,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { draftToSpell, spellToDraft, withTarget } from './spell-draft';
import { RANGE_OPTIONS, TARGET_OPTIONS, moreLabel, sizeLabel } from './spell-options';
import { emptySpell } from './spell-types';
import { previewRows, targetText } from './spell-preview';

const lamina = () => ({
  ...emptySpell(),
  name: ' Lâmina de Nanquim ',
  rangeM: '18',
  verbal: true,
  somatic: true,
  material: true,
  materialPt: 'uma pena molhada em tinta',
  classKeys: ['class:wizard'],
  text: 'Um risco de tinta.\n\nSegundo parágrafo.',
  mechanic: 'attack' as const,
  dice: '2d8',
  more: '1d8',
});

describe('the spell editor form (E10-01 states 4, 4b)', () => {
  it('offers the four targets of the artboard', () => {
    expect(TARGET_OPTIONS.map((o) => o.label)).toEqual([
      'Uma criatura',
      'Várias criaturas',
      'Área',
      'Só quem conjura',
    ]);
  });

  it('offers "Pessoal" and "Toque" beside a distance in the range', () => {
    expect(RANGE_OPTIONS.map((o) => o.label)).toEqual([
      'Distância',
      'Pessoal',
      'Toque',
      'À vista',
      'Ilimitado',
    ]);
  });

  it('names the size by the shape: length, side, radius', () => {
    expect(sizeLabel('cone')).toBe('Comprimento');
    expect(sizeLabel('line')).toBe('Comprimento');
    expect(sizeLabel('cube')).toBe('Lado');
    expect(sizeLabel('sphere')).toBe('Raio');
    expect(sizeLabel('cylinder')).toBe('Raio');
  });

  it('says the "more damage" by the circle, or by the truque\'s steps', () => {
    expect(moreLabel(1)).toBe('Mais dano por nível acima do 1º');
    expect(moreLabel(3)).toBe('Mais dano por nível acima do 3º');
    expect(moreLabel(0)).toBe('Mais dano a cada degrau do truque');
  });

  it('makes the range "Pessoal" for "Só quem conjura" and gives the distance back when the target changes', () => {
    const self = withTarget(lamina(), 'self');
    expect(self.range).toBe('self');
    expect(withTarget(self, 'creature').range).toBe('ranged');
    // A range the master chose on purpose is not touched when the target changes between two others.
    expect(withTarget({ ...lamina(), range: 'touch' }, 'creatures').range).toBe('touch');
  });

  it('keeps "Pessoal" when the target moves to an area (a cone comes out of the caster)', () => {
    const self = withTarget(lamina(), 'self');
    expect(withTarget(self, 'area').range).toBe('self');
    expect(withTarget(self, 'creatures').range).toBe('ranged');
  });

  it('builds the request of a spell with an attack: metres to feet, only the attack and its damage', () => {
    const body = draftToSpell(lamina()) as Record<string, any>;
    expect(body['namePt']).toBe('Lâmina de Nanquim');
    expect(body['range']).toEqual({ kind: SpellRangeKind.RANGED, distanceFt: 60 });
    expect(body['castingTime']).toEqual({ unit: 1, amount: 1 });
    expect(body['components']).toEqual({
      verbal: true,
      somatic: true,
      material: true,
      materialPt: 'uma pena molhada em tinta',
    });
    expect(body['descPt']).toEqual(['Um risco de tinta.', 'Segundo parágrafo.']);
    expect(body['target']).toEqual({ kind: TableSpellTargetKind.CREATURE });
    expect(body['attack']).toBe('ranged');
    expect(body['damage']).toEqual([
      { damageTypeKey: 'damage-type:necrotic', dice: '2d8', perSlotLevel: '1d8' },
    ]);
    expect('save' in body).toBe(false);
    expect('heal' in body).toBe(false);
  });

  it('builds an area spell with a saving throw: the shape and the size, no attack', () => {
    const d = {
      ...lamina(),
      name: 'Sopro de Nanquim',
      target: 'area' as const,
      shape: 'cone' as const,
      sizeM: '4,5',
      range: 'self' as const,
      mechanic: 'save' as const,
      saveOnSuccess: 'half' as const,
      dice: '3d6',
      more: '1d6',
      count: 3,
    };
    const body = draftToSpell(d) as Record<string, any>;
    expect(body['target']).toEqual({
      kind: TableSpellTargetKind.AREA,
      shape: TableAreaShape.CONE,
      sizeFt: 15,
    });
    expect(body['range']).toEqual({ kind: SpellRangeKind.SELF });
    expect(body['save']).toEqual({ ability: Ability.DEXTERITY, onSuccess: SpellSaveSuccess.HALF });
    expect('attack' in body).toBe(false);
    expect(body['damage'][0].dice).toBe('3d6');
  });

  it('sends nothing of a mechanic that is not chosen: "Só texto" has no attack, save, damage or heal', () => {
    const body = draftToSpell({ ...lamina(), mechanic: 'text' }) as Record<string, any>;
    expect(body['damage']).toEqual([]);
    expect(['attack', 'save', 'heal'].filter((k) => k in body)).toEqual([]);
  });

  it('sends the count of several creatures, the per-circle extra of one, and the damage steps of a truque', () => {
    expect(
      (draftToSpell({ ...lamina(), target: 'creatures', count: 3, perSlot: 1 }) as any)['target'],
    ).toEqual({ kind: TableSpellTargetKind.CREATURES, count: 3, perSlotLevel: 1 });
    expect(
      (draftToSpell({ ...lamina(), target: 'creature', perSlot: 1 }) as any)['target'],
    ).toEqual({ kind: TableSpellTargetKind.CREATURE, perSlotLevel: 1 });
    const cantrip = draftToSpell({ ...lamina(), level: 0, more: '1d10' }) as any;
    expect(cantrip['damage'][0]).toEqual({
      damageTypeKey: 'damage-type:necrotic',
      dice: '2d8',
      perTier: '1d10',
    });
  });

  it('sends a heal and an area-less shape never', () => {
    const body = draftToSpell({
      ...lamina(),
      mechanic: 'heal',
      healDice: '1d8',
      healMore: '1d8',
      healModifier: true,
      shape: 'cube',
      sizeM: '9',
    }) as any;
    expect(body['heal']).toEqual({ dice: '1d8', perSlotLevel: '1d8', addsModifier: true });
    expect(body['damage']).toEqual([]);
    expect(body['target']).toEqual({ kind: TableSpellTargetKind.CREATURE });
  });

  it('keeps a concentration spell "up to" and a reaction with its trigger', () => {
    const timed = draftToSpell({
      ...lamina(),
      duration: 'timed',
      durationAmount: 1,
      durationUnit: 'minute',
      concentration: true,
    }) as any;
    expect(timed['duration']).toMatchObject({ amount: 1, upTo: true });
    const reaction = draftToSpell({
      ...lamina(),
      time: 'reaction',
      trigger: ' quando você sofre dano ',
    }) as any;
    expect(reaction['castingTime']).toEqual({
      unit: 3,
      amount: 1,
      triggerPt: 'quando você sofre dano',
    });
  });

  it('reads a stored spell back into the form (and keeps a second damage entry as it was)', () => {
    const stored = create(TableSpellSchema, {
      namePt: 'Gelo',
      level: 2,
      schoolKey: 'school:evocation',
      castingTime: { unit: 1, amount: 1 },
      range: { kind: SpellRangeKind.RANGED, distanceFt: 90 },
      target: { kind: TableSpellTargetKind.AREA, shape: TableAreaShape.SPHERE, sizeFt: 20 },
      save: { ability: Ability.CONSTITUTION, onSuccess: SpellSaveSuccess.NONE },
      damage: [
        { damageTypeKey: 'damage-type:cold', dice: '2d6', perSlotLevel: '1d6' },
        { damageTypeKey: 'damage-type:bludgeoning', dice: '1d4' },
      ],
    });
    const d = spellToDraft(stored);
    expect(d).toMatchObject({
      name: 'Gelo',
      rangeM: '27',
      target: 'area',
      shape: 'sphere',
      sizeM: '6',
      mechanic: 'save',
      saveAbility: Ability.CONSTITUTION,
      saveOnSuccess: 'none',
      dice: '2d6',
      more: '1d6',
    });
    expect(d.otherDamage).toHaveLength(1);
    const again = draftToSpell(d) as any;
    expect(again['damage']).toHaveLength(2);
    expect(again['target']).toEqual({
      kind: TableSpellTargetKind.AREA,
      shape: TableAreaShape.SPHERE,
      sizeFt: 20,
    });
  });

  it('writes "Como os jogadores veem" from the form, with the Alvo row', () => {
    const rows = previewRows(lamina(), (k) => catalog().nameOf(k)).map(
      (r) => `${r.label}: ${r.value.replace(/\u00a0/g, ' ')}`,
    );
    expect(rows).toEqual([
      'Tempo: 1 ação',
      'Alcance: 18 m',
      'Alvo: Uma criatura',
      'Componentes: V, S, M (uma pena molhada em tinta)',
      'Duração: Instantânea',
      'Ataque: Ataque de magia à distância',
      'Dano: 2d8 necrótico, +1d8 por nível acima do 1º',
    ]);
    const area = previewRows(
      {
        ...lamina(),
        target: 'area',
        sizeM: '4,5',
        shape: 'cone',
        range: 'self',
        mechanic: 'save',
        dice: '3d6',
      },
      (k) => catalog().nameOf(k),
    ).map((r) => `${r.label}: ${r.value}`);
    expect(area).toContain('Alcance: Pessoal');
    expect(area.some((r) => r.startsWith('Alvo: Cone de 4,5'))).toBe(true);
    expect(area).toContain('Teste de resistência: Destreza, metade do dano ao passar');
    expect(targetText({ ...lamina(), target: 'self' })).toBe('Só quem conjura');
  });
});

describe('a count the text does not read as a number', () => {
  it('never goes out as NaN, even when its field is hidden at level 0', () => {
    const d = {
      ...emptySpell(),
      name: 'Teste',
      level: 0,
      target: 'creatures' as const,
      perSlot: Number.NaN,
    };
    expect(draftToSpell(d).target?.perSlotLevel).toBe(0);
  });
});

describe('what the form has no control for', () => {
  it('goes back as it was loaded: the effect and the zone', () => {
    const stored = create(TableSpellSchema, {
      namePt: 'Marca de Nanquim',
      level: 2,
      schoolKey: 'school:abjuration',
      effect: create(TableSpellEffectSchema, {
        applies: TableEffectApplies.ALL,
        conditions: ['condition:invisible'],
        modifiers: [{ kind: TableEffectModifierKind.AC_BONUS, value: 2 }],
      }),
      zone: create(TableSpellZoneSchema, {
        trigger: TableZoneTrigger.START_OF_TURN,
        followsCaster: true,
      }),
    });
    const draft = { ...spellToDraft(stored), name: 'Marca de Cinza' };
    const sent = create(TableSpellSchema, draftToSpell(draft));
    expect(sent.namePt).toBe('Marca de Cinza');
    expect(sent.effect).toEqual(stored.effect);
    expect(sent.zone).toEqual(stored.zone);
  });

  it('sends none of them for a spell that had none', () => {
    const sent = draftToSpell(
      spellToDraft(create(TableSpellSchema, { namePt: 'Simples', level: 1 })),
    );
    expect(sent.effect).toBeUndefined();
    expect(sent.zone).toBeUndefined();
    expect(sent.summon).toBeUndefined();
  });
});
