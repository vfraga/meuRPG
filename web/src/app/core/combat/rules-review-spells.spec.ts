import { create } from '@bufbuild/protobuf';
import {
  ActionEconomy,
  SpellDamageSchema,
  SpellDetailsSchema,
  SpellRangeKind,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { castSubtitle, damageDice } from './cast-flow';
import { spellSummary } from './spell-summary';

// Sacred Flame (save cantrip): SRD 5.1 damage is 1d8 at 1st, 2d8 at 5th,
// 3d8 at 11th and 4d8 at 17th character level. The server's DamageAt(slot,
// characterLevel) rolls by character level; the details carry the whole table.
function sacredFlame() {
  return create(SpellDetailsSchema, {
    spell: { key: 'spell:sacred-flame', level: 0 },
    range: { kind: SpellRangeKind.RANGED, distanceFt: 60 },
    damage: [
      create(SpellDamageSchema, {
        damageTypePt: 'radiante',
        bySlotLevel: {},
        byCharacterLevel: { 1: '1d8', 5: '2d8', 11: '3d8', 17: '4d8' },
      }),
    ],
  });
}

describe('rules review spells', () => {
  it('cantrip summary in the action list does not pin the level 1 dice (a level 11 caster rolls 3d8)', () => {
    const summary = spellSummary(sacredFlame());
    expect(
      summary,
      `Sacred Flame at character level 11: SRD/server roll 3d8, but spellSummary shows "${summary}" ` +
        '(always the level 1 dice, cast-flow.ts damageDice takes Object.values(byCharacterLevel)[0])',
    ).not.toMatch(/(^|[^\d])1d8(?!.*(2d8|3d8|4d8))/);
  });

  it('cast sheet subtitle of a save cantrip does not pin the level 1 dice', () => {
    const sub = castSubtitle(ActionEconomy.ACTION, 'save', sacredFlame(), 0, 0);
    expect(
      sub,
      `Sacred Flame at character level 11: SRD/server roll 3d8, but castSubtitle shows "${sub}"`,
    ).not.toMatch(/(^|[^\d])1d8(?!.*(2d8|3d8|4d8))/);
  });

  it('damageDice of a cantrip is not simply the level 1 entry of the table', () => {
    expect(
      damageDice(sacredFlame(), 0),
      'damageDice has no character level; for level 11 the SRD says 3d8, app says 1d8',
    ).not.toBe('1d8');
  });
});
