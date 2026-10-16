import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';

import { Nudger } from '../../../core/combat/nudger';

import { VitalsVm } from '../../../core/play/live-session.types';
import { SessionXp } from '../session-xp/session-xp';
import { pensantusVitals } from '../../../core/play/live-testing';
import { PartyPanel } from './party-panel';

@Component({
  selector: 'app-session-xp',
  template: '',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
class SessionXpStub {
  readonly campaignId = input('');
  readonly campaignName = input('');
}

describe('PartyPanel: a druid in Wild Shape (MR-037)', () => {
  const wolf = (current: number) => ({
    beastKey: 'monster:wolf',
    beastNamePt: 'Lobo',
    hitPointsCurrent: current,
    hitPointsMax: 11,
  });
  const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();

  function render(party: readonly VitalsVm[]) {
    // The XP button reads the campaign's experience; the party rows do not need it.
    TestBed.configureTestingModule({}).overrideComponent(PartyPanel, {
      remove: { imports: [SessionXp] },
      add: { imports: [SessionXpStub] },
    });
    const fixture = TestBed.createComponent(PartyPanel);
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('party', party);
    fixture.detectChanges();
    return fixture;
  }

  it("shows the beast and its pool beside the druid's own, updates when the beast takes damage, and drops it when the form ends", () => {
    const fixture = render([pensantusVitals({ wildShape: wolf(11) })]);
    const el = fixture.nativeElement as HTMLElement;
    expect(flat(el.querySelector('.js-beast'))).toBe('Lobo 11 de 11 PV');

    fixture.componentRef.setInput('party', [pensantusVitals({ wildShape: wolf(8) })]);
    fixture.detectChanges();
    expect(flat(el.querySelector('.js-beast'))).toBe('Lobo 8 de 11 PV');

    fixture.componentRef.setInput('party', [pensantusVitals({ wildShape: null })]);
    fixture.detectChanges();
    expect(el.querySelector('.js-beast')).toBeNull();
  });

  it('shows no beast for a character in its own shape, or when the read carries no form', () => {
    const el = render([pensantusVitals(), pensantusVitals({ characterId: 'b' })])
      .nativeElement as HTMLElement;
    expect(el.querySelectorAll('.member').length).toBe(2);
    expect(el.querySelector('.js-beast')).toBeNull();
  });
});

describe('PartyPanel under Ajuda (PM-03a)', () => {
  const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();

  it('says "+5 Ajuda" by the maximum the master reads, and nothing without it', () => {
    TestBed.configureTestingModule({}).overrideComponent(PartyPanel, {
      remove: { imports: [SessionXp] },
      add: { imports: [SessionXpStub] },
    });
    const fixture = TestBed.createComponent(PartyPanel);
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('party', [
      pensantusVitals({ hitPointsCurrent: 31, hitPointsMax: 43, hitPointsMaxBonus: 5 }),
    ]);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(flat(el.querySelector('.js-aid'))).toBe('+5 Ajuda');
    fixture.componentRef.setInput('party', [pensantusVitals()]);
    fixture.detectChanges();
    expect(el.querySelector('.js-aid')).toBeNull();
  });
});

describe('PartyPanel: Chamar a atenção', () => {
  it('has the master call each character by name, and calls with that character', () => {
    TestBed.configureTestingModule({}).overrideComponent(PartyPanel, {
      remove: { imports: [SessionXp] },
      add: { imports: [SessionXpStub] },
    });
    const call = vi.spyOn(TestBed.inject(Nudger), 'call').mockResolvedValue(undefined);
    const fixture = TestBed.createComponent(PartyPanel);
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('party', [
      pensantusVitals(),
      pensantusVitals({ characterId: 'b', name: 'Brisa' }),
    ]);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const buttons = el.querySelectorAll<HTMLButtonElement>('app-nudge-button button');
    expect([...buttons].map((b) => b.getAttribute('aria-label'))).toEqual([
      'Chamar a atenção de Pensantus',
      'Chamar a atenção de Brisa',
    ]);
    buttons[1].click();
    expect(call).toHaveBeenCalledWith('camp', 'b');
  });
});
