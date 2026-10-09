import {
  formatClock,
  formatDayAt,
  formatSessionSpan,
  formatSessionStart,
  sessionSince,
} from './session-time';

describe('session time', () => {
  it('writes the clock and the day with the clock', () => {
    const at = new Date(2026, 8, 30, 20, 5);
    expect(formatClock(at)).toBe('20:05');
    expect(formatDayAt(at).replace(/\u00a0/g, ' ')).toBe('30/09 às 20:05');
  });

  it('says only the clock for a session that started today (shared decision 8)', () => {
    const now = new Date(2026, 9, 3, 22, 40);
    expect(sessionSince(new Date(2026, 9, 3, 20, 5), now)).toBe('Em andamento desde 20:05');
  });

  it('adds the day when it started on another one', () => {
    const now = new Date(2026, 9, 3, 1, 10);
    expect(sessionSince(new Date(2026, 9, 2, 23, 30), now).replace(/\u00a0/g, ' ')).toBe(
      'Em andamento desde 02/10 às 23:30',
    );
    expect(sessionSince(new Date(2025, 9, 3, 20, 5), now).replace(/\u00a0/g, ' ')).toBe(
      'Em andamento desde 03/10 às 20:05',
    );
  });

  describe('the way a session is dated', () => {
    const now = new Date(2026, 9, 8, 12, 0);
    const plain = (text: string) => text.replace(/\u00a0/g, ' ');

    it('writes the weekday, the day, the month and the hour of a start', () => {
      expect(plain(formatSessionStart(new Date(2026, 9, 8, 19, 5), now))).toBe(
        'qui., 8 de out., 19h05',
      );
    });

    it('writes the span of a session that ended the same day', () => {
      const span = formatSessionSpan(
        new Date(2026, 9, 1, 19, 5),
        new Date(2026, 9, 1, 22, 47),
        now,
      );
      expect(plain(span)).toBe('qui., 1 de out., 19h05 às 22h47');
    });

    it('does not pad the hour and pads the minutes', () => {
      const span = formatSessionSpan(new Date(2026, 8, 3, 9, 0), new Date(2026, 8, 3, 11, 7), now);
      expect(plain(span)).toBe('qui., 3 de set., 9h00 às 11h07');
    });

    it('gives the day of the end to a session that crossed midnight', () => {
      const span = formatSessionSpan(
        new Date(2026, 9, 2, 22, 30),
        new Date(2026, 9, 3, 1, 12),
        now,
      );
      expect(plain(span)).toBe('sex., 2 de out., 22h30 às 1h12 (sáb.)');
    });

    it('gives the date of the end when it came more than a day later', () => {
      const span = formatSessionSpan(
        new Date(2026, 9, 2, 22, 30),
        new Date(2026, 9, 4, 1, 12),
        now,
      );
      expect(plain(span)).toBe('sex., 2 de out., 22h30 às 1h12 (dom., 4 de out.)');
    });

    it('adds the year to the end when it fell in another one', () => {
      const span = formatSessionSpan(
        new Date(2026, 11, 31, 23, 0),
        new Date(2027, 0, 1, 1, 30),
        new Date(2027, 0, 5),
      );
      expect(plain(span)).toBe('qui., 31 de dez. de 2026, 23h00 às 1h30 (sex.)');
    });

    it('adds the year when the session is not from this one', () => {
      const span = formatSessionSpan(
        new Date(2025, 9, 8, 19, 5),
        new Date(2025, 9, 8, 22, 47),
        now,
      );
      expect(plain(span)).toBe('qua., 8 de out. de 2025, 19h05 às 22h47');
      expect(plain(formatSessionStart(new Date(2025, 9, 8, 19, 5), now))).toBe(
        'qua., 8 de out. de 2025, 19h05',
      );
    });

    it('keeps "8 de out." on one line and lets the rest wrap', () => {
      const span = formatSessionSpan(
        new Date(2026, 9, 1, 19, 5),
        new Date(2026, 9, 1, 22, 47),
        now,
      );
      expect(span).toContain('1\u00a0de\u00a0out.');
      expect(span).toContain(', 19h05 às 22h47');
    });
  });
});
