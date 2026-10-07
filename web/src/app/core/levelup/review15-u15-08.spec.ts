import { create } from '@bufbuild/protobuf';

import {
  LevelUpHitPointsRule,
  LevelUpSubclassSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { LevelUpDraft } from './levelup-draft';
import { SKILLS, SPELLS, WIZARD_KEYS, fighterOptions, wizardOptions } from './levelup-testing';

const catalog = { spells: SPELLS, skills: SKILLS };
// Two prepared today, new maximum 3 from the server's options: one more to prepare.
const wizardWith = (over = {}) =>
  new LevelUpDraft(wizardOptions({ preparedMaxAfter: 3, ...over }), WIZARD_KEYS, catalog);

/** A wizard draft that holds the two new book spells and has the preview's maximum raised to 4. */
const raised = () => {
  const d = wizardWith();
  d.toggleSpell('spell:misty-step');
  d.toggleSpell('spell:mirror-image');
  d.preparedMaxAfter.set(4);
  d.togglePrepared('spell:misty-step');
  d.togglePrepared('spell:mirror-image');
  return d;
};

describe('Review15 U15-8: LevelUpDraft keeps picks the counts no longer allow', () => {
  it('(a) trims the prepared picks when the preview maximum falls back', () => {
    const d = raised();
    expect(d.choices().preparedSpellKeys).toHaveLength(2);
    // The player changes the ability back to STR: the preview's maximum is 3 again.
    d.preparedMaxAfter.set(3);
    expect(d.preparedAsked()).toBe(1);
    expect((d.choices().preparedSpellKeys ?? []).length).toBeLessThanOrEqual(1);
  });

  const fighter = (skillChoices: number, expertiseChoices = 1) =>
    fighterOptions({
      subclassDue: true,
      expertiseChoices,
      subclasses: [
        create(LevelUpSubclassSchema, { key: 'sub:lore', namePt: 'Conhecimento', skillChoices }),
        create(LevelUpSubclassSchema, { key: 'sub:champion', namePt: 'Campeão' }),
      ],
    });

  it('(b) drops the expertise of a skill that the subclass change trimmed away', () => {
    const d = new LevelUpDraft(fighter(2), WIZARD_KEYS, catalog);
    d.setSubclass('sub:lore');
    d.toggleSkill('skill:stealth');
    d.toggleSkill('skill:perception');
    d.toggleExpertise('skill:stealth');
    d.setSubclass('sub:champion');
    expect([...d.skills()]).toEqual([]);
    const trained = new Set([...WIZARD_KEYS.skills, ...(d.choices().skillProficiencyKeys ?? [])]);
    expect((d.choices().expertiseSkillKeys ?? []).filter((k) => !trained.has(k))).toEqual([]);
  });

  it('(b2) adopt() does not keep expertise on a skill it then trims', () => {
    const old = new LevelUpDraft(fighter(2), WIZARD_KEYS, catalog);
    old.setSubclass('sub:lore');
    old.toggleSkill('skill:stealth');
    old.toggleSkill('skill:perception');
    old.toggleExpertise('skill:perception');
    // The sheet is read again and the level now asks for one skill only.
    const fresh = new LevelUpDraft(fighter(1), WIZARD_KEYS, catalog);
    fresh.adopt(old);
    const trained = new Set([...WIZARD_KEYS.skills, ...(fresh.choices().skillProficiencyKeys ?? [])]);
    expect((fresh.choices().expertiseSkillKeys ?? []).filter((k) => !trained.has(k))).toEqual([]);
  });

  it('(c) adopt() keeps the prepared picks the old draft had under the raised maximum', () => {
    const old = raised();
    const fresh = wizardWith();
    fresh.adopt(old);
    expect(fresh.choices().preparedSpellKeys).toHaveLength(2);
  });

  it('(c2) setSubclass() keeps the prepared picks under the preview maximum', () => {
    const d = new LevelUpDraft(
      wizardOptions({
        preparedMaxAfter: 3,
        subclassDue: true,
        subclasses: [create(LevelUpSubclassSchema, { key: 'sub:x', namePt: 'X' })],
      }),
      WIZARD_KEYS,
      catalog,
    );
    d.toggleSpell('spell:misty-step');
    d.toggleSpell('spell:mirror-image');
    d.preparedMaxAfter.set(4);
    d.togglePrepared('spell:misty-step');
    d.togglePrepared('spell:mirror-image');
    d.setSubclass('sub:x');
    expect(d.choices().preparedSpellKeys).toHaveLength(2);
  });

  it('(d) adopt() into an average-only table goes back to the average with no roll', () => {
    const old = wizardWith();
    old.setHpCard('roll');
    old.rolled.set({ kind: 'app', value: 5 });
    const fresh = wizardWith({ hitPointsRule: LevelUpHitPointsRule.AVERAGE_ONLY });
    fresh.adopt(old);
    expect(fresh.hpCard()).toBe('average');
    expect(fresh.rolled()).toBeNull();
    expect(fresh.choices().hitPoints?.method).toBe(1); // AVERAGE
  });
});
