// Finding U11-10: LevelUpDraft.setSubclass trims the skills but does not rerun the
// "expertise only in what is trained" filter, so a stale expertise key is still sent.
import { create } from '@bufbuild/protobuf';

import { LevelUpSubclassSchema } from '../../../gen/meurpg/characters/v1/characters_pb';
import { LevelUpDraft } from './levelup-draft';
import { SKILLS, SPELLS, WIZARD_KEYS, fighterOptions } from './levelup-testing';

const catalog = { spells: SPELLS, skills: SKILLS };
const options = () =>
  fighterOptions({
    subclassDue: true,
    subclasses: [
      create(LevelUpSubclassSchema, { key: 'sub:lore', namePt: 'Lore', skillChoices: 1 }),
      create(LevelUpSubclassSchema, { key: 'sub:champion', namePt: 'Campeão' }),
    ],
    expertiseChoices: 1,
  });

describe('Review11 U11-10: stale expertise after a subclass switch drops a skill', () => {
  it('setSubclass sends no expertise for a skill that is no longer trained', () => {
    const d = new LevelUpDraft(options(), WIZARD_KEYS, catalog);
    d.setSubclass('sub:lore');
    d.toggleSkill('skill:stealth');
    d.toggleExpertise('skill:stealth');
    d.setSubclass('sub:champion');
    expect([...d.skills()]).toEqual([]);
    expect(d.choices().expertiseSkillKeys).toEqual([]);
  });

  it('adopt keeps no expertise for a skill trimmed by the new counts', () => {
    const before = new LevelUpDraft(options(), WIZARD_KEYS, catalog);
    before.setSubclass('sub:lore');
    before.toggleSkill('skill:stealth');
    before.toggleExpertise('skill:stealth');
    // Same options, but the lore subclass no longer asks a skill (sheet re-read).
    const fresh = new LevelUpDraft(
      fighterOptions({
        subclassDue: true,
        subclasses: [create(LevelUpSubclassSchema, { key: 'sub:lore', namePt: 'Lore' })],
        expertiseChoices: 1,
      }),
      WIZARD_KEYS,
      catalog,
    );
    fresh.adopt(before);
    expect([...fresh.skills()]).toEqual([]);
    expect(fresh.choices().expertiseSkillKeys).toEqual([]);
  });
});
