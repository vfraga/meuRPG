import { TestBed } from '@angular/core/testing';

import { SunlightTrait } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant } from '../../../../core/combat/combat-testing';
import { SunlightNotice } from './sunlight-notice';

function notice(over: Record<string, unknown>, sunlightOn = true) {
  const fixture = TestBed.createComponent(SunlightNotice);
  fixture.componentRef.setInput('creature', combatant({ id: 'g', label: 'Ghoul', ...over }));
  fixture.componentRef.setInput('sunlightOn', sunlightOn);
  fixture.detectChanges();
  return (fixture.nativeElement as HTMLElement).querySelector('[data-testid="sunlight-notice"]');
}

describe('SunlightNotice', () => {
  it('warns of the attacks and the sight-based Perception of a Sensitivity creature in the sun', () => {
    const el = notice({ sunlightTrait: SunlightTrait.SENSITIVITY, inSunlight: true })!;
    expect(el.textContent).toContain(
      'Ghoul está na luz do sol. Desvantagem nos ataques e nos testes de Percepção que dependem da visão.',
    );
    expect(el.classList).toContain('mr-notice--warning');
    expect(el.getAttribute('role')).toBe('status');
  });

  it('warns of the attacks, the saving throws and the ability checks of a Weakness creature in the sun', () => {
    const el = notice({ sunlightTrait: SunlightTrait.WEAKNESS, inSunlight: true })!;
    expect(el.textContent).toContain(
      'Ghoul está na luz do sol. Desvantagem nos ataques, nas salvaguardas e nos testes de atributo.',
    );
  });

  it('says it rolls normally out of the sun while the combat has one', () => {
    const el = notice({ sunlightTrait: SunlightTrait.SENSITIVITY, inSunlight: false })!;
    expect(el.textContent).toContain('Ghoul está fora da luz do sol. Rola normal.');
    expect(el.classList).not.toContain('mr-notice--warning');
  });

  it('says nothing with the sun off or without the trait', () => {
    expect(notice({ sunlightTrait: SunlightTrait.WEAKNESS, inSunlight: false }, false)).toBeNull();
    expect(notice({ sunlightTrait: SunlightTrait.UNSPECIFIED, inSunlight: true })).toBeNull();
  });
});
