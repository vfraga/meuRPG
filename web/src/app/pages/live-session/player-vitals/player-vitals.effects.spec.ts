import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import type { EffectCardView } from '../../../core/effects/effects';
import { pensantusVitals } from '../../../core/play/live-testing';
import { PlayerVitals } from './player-vitals';

const flat = (n: Element | null) => n?.textContent?.replace(/\s+/g, ' ').trim();

const hold: EffectCardView = {
  key: 'e1',
  name: 'Imobilizar Pessoa',
  origin: 'De alguém que você não vê',
  tags: [],
  clock: 'Resta o tempo da magia: acaba no turno de quem a conjurou, na rodada 12.',
  save: 'No fim de cada turno seu: teste de resistência de Sabedoria.',
  changes: ['Você não age nem se move, e não fala.'],
};

describe('PlayerVitals: effects and exhaustion (RN-22)', () => {
  function render(
    over: Parameters<typeof pensantusVitals>[0],
    effects: { cards?: readonly EffectCardView[]; conditions?: readonly string[] } = {},
    compact = false,
  ) {
    TestBed.configureTestingModule({ providers: [provideRouter([])] });
    const fixture = TestBed.createComponent(PlayerVitals);
    fixture.componentRef.setInput('vitals', pensantusVitals({ name: 'Brisa', ...over }));
    fixture.componentRef.setInput('sheet', { armorClass: 15, summary: 'Ladino 5', senses: [] });
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('compact', compact);
    fixture.componentRef.setInput('effectCards', effects.cards ?? []);
    fixture.componentRef.setInput('conditionNames', effects.conditions ?? []);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('is the ordinary card without effects and without exhaustion: the section is gone', () => {
    const el = render({});
    expect(el.querySelector('app-effect-cards')).toBeNull();
    expect(flat(el.querySelector('.effects'))).toBe('');
  });

  it.each([
    ['390 px (the compact card of a combat)', true],
    ['1280 px (the card of the sheet)', false],
  ])(
    'puts the condition once in the header label and the cards under the numbers, at %s',
    (_n, compact) => {
      const el = render({}, { cards: [hold], conditions: ['Paralisado'] }, compact);
      const labels = el.querySelector('.effects')!;
      expect(flat(labels)).toContain('Paralisado');
      expect(labels.getAttribute('role')).toBe('status');
      // The card is not the label again: it carries the effect, not the condition.
      const cards = el.querySelector('app-effect-cards')!;
      expect(flat(cards)).toContain('Imobilizar Pessoa');
      expect(flat(cards)).not.toContain('Paralisado');
      // Under the hit points and the armor class, after the label.
      const order = Array.from(el.querySelectorAll('.stats, .effects, app-effect-cards'));
      expect(order.map((n) => n.tagName === 'APP-EFFECT-CARDS' || n.className)).toEqual([
        'stats',
        'effects',
        true,
      ]);
    },
  );

  it('shows the exhaustion level in the label and in a card with the lines up to it', () => {
    const el = render({ exhaustionLevel: 4, hitPointsCurrent: 20, hitPointsMax: 20 });
    expect(flat(el.querySelector('.effects'))).toContain('Exaustão 4');
    const card = el.querySelector('app-effect-cards')!;
    expect(flat(card)).toContain('Nível 4');
    expect(flat(card)).toContain('PV máximos pela metade');
    expect(flat(card)).not.toContain('Deslocamento 0');
    // The server already halved the maximum: the card reads "20 de 20".
    expect(flat(el.querySelector('.hp__current'))).toBe('20');
    expect(flat(el.querySelector('.hp__max'))).toBe('de 20');
  });
});
