import { WHOLE_MAX, WHOLE_MIN, readWhole, wholeText } from './whole-number';

describe('readWhole', () => {
  it('reads an empty field as 0, "not set"', () => {
    expect(readWhole('', false)).toBe(0);
    expect(readWhole('  ', true)).toBe(0);
  });

  it('reads a leading + everywhere and a leading - only where a sign is allowed', () => {
    expect(readWhole('+3', false)).toBe(3);
    expect(readWhole('-3', true)).toBe(-3);
    expect(readWhole('-3', false)).toBeNaN();
    expect(readWhole('+1', true)).toBe(1);
  });

  it('reads "-0" as 0, not as the negative zero', () => {
    expect(Object.is(readWhole('-0', true), 0)).toBe(true);
  });

  it('refuses text that is no whole number', () => {
    for (const bad of ['abc', '1.5', '-', '+', '1e3', '0x10', '1 2']) {
      expect(readWhole(bad, true)).toBeNaN();
    }
  });

  it('refuses what the server cannot store: 400 digits, past int32, an unsafe integer', () => {
    expect(readWhole('9'.repeat(400), false)).toBeNaN();
    expect(readWhole('3000000000', false)).toBeNaN();
    expect(readWhole(String(WHOLE_MAX + 1), false)).toBeNaN();
    expect(readWhole(String(WHOLE_MIN - 1), true)).toBeNaN();
    expect(readWhole('9007199254740993', false)).toBeNaN();
    expect(readWhole(String(WHOLE_MAX), false)).toBe(WHOLE_MAX);
    expect(readWhole(String(WHOLE_MIN), true)).toBe(WHOLE_MIN);
  });
});

describe('wholeText', () => {
  it('shows nothing for 0 and for an unreadable number', () => {
    expect(wholeText(0)).toBe('');
    expect(wholeText(Number.NaN)).toBe('');
    expect(wholeText(7)).toBe('7');
    expect(wholeText(-2)).toBe('-2');
  });
});
