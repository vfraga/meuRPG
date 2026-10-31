import { create } from '@bufbuild/protobuf';

import {
  EffectPhase,
  EffectSavePromptSchema,
} from '../../../gen/meurpg/play/v1/lasting_effects_pb';
import { encounter, spellZone } from '../combat/combat-testing';
import { effectSaveView } from './effects';
import { zoneCasterLabel, zoneReasonText, zoneSaveTitle } from './zone-save';

const prompt = (over: object = {}) =>
  create(EffectSavePromptSchema, {
    effectId: 'z1',
    zoneId: 'z1',
    zoneReason: 'zone_start',
    sourceNamePt: 'Raio Lunar',
    ability: 'con',
    abilityNamePt: 'Constituição',
    phase: EffectPhase.UNSPECIFIED,
    modifier: 3,
    bonusKnown: true,
    mode: 'normal',
    textPt:
      'Teste de resistência de Constituição. Se falhar, você sofre dano de radiante. Se passar, metade do dano.',
    damageTypeNamePt: 'radiante',
    halfOnSave: true,
    ...over,
  });

describe('the save prompt of a spell zone', () => {
  it('says why the creature is asked, for who answers and for the master reading about someone', () => {
    expect(zoneReasonText('zone_enter', '')).toBe('Você entrou na zona.');
    expect(zoneReasonText('zone_start', '')).toBe('A zona atingiu você no começo do seu turno.');
    expect(zoneReasonText('zone_enter', 'Tor')).toBe('Tor entrou na zona.');
    expect(zoneReasonText('zone_start', 'Tor')).toBe('A zona atingiu Tor no começo do turno.');
    expect(zoneReasonText('zone_cast', '')).toBe(
      'Você estava na área quando a magia foi conjurada.',
    );
    expect(zoneReasonText('zone_end', 'Tor')).toBe('Tor terminou o turno na zona.');
    expect(zoneReasonText('', '')).toBe('');
  });

  it('titles it with the zone and its caster, and with the zone alone when the caster is not seen', () => {
    expect(zoneSaveTitle(prompt(), 'Bartolo')).toBe('Raio Lunar de Bartolo');
    expect(zoneSaveTitle(prompt(), '')).toBe('Raio Lunar');
  });

  it('finds the caster among the zones the server sent, by the zone id', () => {
    const e = encounter({ zones: [spellZone({ id: 'z1', ownerLabel: 'Bartolo' })] });
    expect(zoneCasterLabel(e, 'z1')).toBe('Bartolo');
    expect(zoneCasterLabel(e, 'other')).toBe('');
    expect(zoneCasterLabel(null, 'z1')).toBe('');
  });

  it('writes the DC only when the server sent it: no number and no placeholder otherwise', () => {
    expect(effectSaveView(prompt({ dc: 14 }), 3, 'Bartolo').dc).toBe('CD\u00a014');
    expect(effectSaveView(prompt({ dc: undefined }), 3, 'Bartolo').dc).toBe('');
  });

  it('builds the sheet: title, reason, the damage and half on a pass, the bonus', () => {
    const v = effectSaveView(prompt({ zoneReason: 'zone_start' }), 3, 'Bartolo');
    expect(v.title).toBe('Raio Lunar de Bartolo');
    expect(v.subtitle).toBe('Magia que ficou no mapa. A zona atingiu você no começo do seu turno.');
    expect(v.ability).toBe('Constituição');
    expect(v.dc).toBe('');
    expect(v.consequence).toBe('Se falhar: sofre dano radiante. Se passar: metade do dano.');
    expect(v.modifier).toBe('Seu modificador: +3.');
    expect(v.source).toBe('Raio Lunar, de Bartolo');
  });

  it('reads the ability from its key when the name is empty, as the server sends it', () => {
    const real = prompt({
      ability: 'con',
      abilityNamePt: '',
      textPt:
        'Teste de resistência de . Se falhar, você sofre dano de radiante. Se passar, metade do dano.',
    });
    const v = effectSaveView(real, 3, 'Bartolo');
    expect(v.ability).toBe('Constituição');
    expect(v.consequence).toBe('Se falhar: sofre dano radiante. Se passar: metade do dano.');
    const web = effectSaveView(
      prompt({
        zoneReason: 'zone_enter',
        sourceNamePt: 'Teia',
        ability: 'dex',
        abilityNamePt: '',
        damageTypeNamePt: '',
        halfOnSave: false,
        textPt: 'Teste de resistência de . Se falhar, você fica contido até se soltar.',
      }),
      3,
      'Pensantus',
    );
    expect(web.ability).toBe('Destreza');
    expect(web.consequence).toBe(
      'Se falhar, você fica contido até se soltar. Soltar-se gasta uma ação.',
    );
  });

  it('shows the DC for the master or the caster own player', () => {
    expect(effectSaveView(prompt({ dc: 14 }), 3, 'Bartolo').dc).toBe('CD\u00a014');
  });

  it('says a pass with no half takes the damage away, and a restraint with its static line', () => {
    const noHalf = effectSaveView(prompt({ halfOnSave: false }), 3, '');
    expect(noHalf.consequence).toBe('Se falhar: sofre dano radiante. Se passar: não sofre o dano.');
    const web = effectSaveView(
      prompt({
        zoneReason: 'zone_enter',
        sourceNamePt: 'Teia',
        abilityNamePt: 'Destreza',
        damageTypeNamePt: '',
        halfOnSave: false,
        textPt: 'Teste de resistência de Destreza. Se falhar, você fica contido até se soltar.',
      }),
      3,
      'Pensantus',
    );
    expect(web.subtitle).toBe('Magia que ficou no mapa. Você entrou na zona.');
    expect(web.consequence).toBe(
      'Se falhar, você fica contido até se soltar. Soltar-se gasta uma ação.',
    );
  });

  it('leaves a prompt that is not a zone as it was', () => {
    const v = effectSaveView(prompt({ zoneId: '', zoneReason: '', phase: EffectPhase.END }), 3, '');
    expect(v.title).toBe('Fim do seu turno');
    expect(v.dc).toBe('');
    expect(v.source).toBe('');
  });
});
