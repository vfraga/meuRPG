import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import { AdvantageSourceSchema, RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { RollModeSources } from './roll-mode-sources';

describe('RollModeSources (#938)', () => {
  function setup(sources: unknown[], label = 'Por que o app sugere desvantagem') {
    const fixture = TestBed.createComponent(RollModeSources);
    fixture.componentRef.setInput(
      'sources',
      sources.map((s) => create(AdvantageSourceSchema, s as never)),
    );
    fixture.componentRef.setInput('label', label);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('tags each circumstance with its word and says the server sentence beside it', () => {
    const el = setup([
      { effect: RollMode.DISADVANTAGE, textPt: 'Ogro a mais de 1,5 m: ataque à distância' },
      { effect: RollMode.ADVANTAGE, textPt: 'Alvo Paralisado' },
    ]);
    const rows = Array.from(el.querySelectorAll('li'), (li) =>
      li.textContent?.replace(/\s+/g, ' ').trim(),
    );
    expect(rows).toEqual([
      'arrow_downwardDesvantagem Ogro a mais de 1,5 m: ataque à distância',
      'arrow_upwardVantagem Alvo Paralisado',
    ]);
    expect(el.querySelector('ul')!.getAttribute('aria-label')).toBe(
      'Por que o app sugere desvantagem',
    );
    expect(el.querySelector('.tag--dis')).not.toBeNull();
    expect(el.querySelector('.tag--adv')).not.toBeNull();
  });

  it('draws nothing for a roll with no circumstance', () => {
    expect(setup([]).querySelector('ul')).toBeNull();
  });
});
