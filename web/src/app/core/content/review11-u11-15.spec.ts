// Finding U11-15: rangeFeet turns unreadable text ("18 m", "1.000,5", "abc") into 0; for darkvisionFt 0 is a valid
// value ("none"), so the race saves with no darkvision and no error. Correct behaviour: unreadable input is not
// silently turned into a valid-looking 0 (it must be flagged or not sent as a plain 0).
import { draftToRace, emptyRace } from './feature-draft';
import { menu } from './content-testing';

describe('Review11 U11-15: unreadable darkvision text is not silently saved as none', () => {
  for (const text of ['18 m', '18m', '1.000,5', 'abc']) {
    it(`"${text}" does not become a silent 0`, () => {
      const race = draftToRace({ ...emptyRace(), name: 'X', darkvisionM: text }, menu());
      // Silent 0 means "no darkvision": the master typed something and it vanished without any message.
      expect(race.darkvisionFt, `darkvisionM "${text}" saved as ${race.darkvisionFt}`).not.toBe(0);
    });
  }
});
