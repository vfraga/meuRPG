import { TestBed } from '@angular/core/testing';

import { DiceMode, DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { DicePreferencePanel } from './dice-preference-panel';

function render(
  mode: DiceMode,
  preference = DicePreference.APP,
  setDicePreference = (_id: string, p: DicePreference) => Promise.resolve({ preference: p }),
) {
  TestBed.configureTestingModule({
    providers: [{ provide: CampaignsService, useValue: { setDicePreference } }],
  });
  const fixture = TestBed.createComponent(DicePreferencePanel);
  fixture.componentRef.setInput('campaignId', 'camp-1');
  fixture.componentRef.setInput('campaignName', 'Mirathel');
  fixture.componentRef.setInput('mode', mode);
  fixture.componentRef.setInput('savedPreference', preference);
  fixture.detectChanges();
  return fixture;
}

describe('DicePreferencePanel', () => {
  it('offers the choice while the campaign lets players choose', () => {
    const el: HTMLElement = render(DiceMode.PLAYERS_CHOOSE, DicePreference.PHYSICAL).nativeElement;
    expect(el.textContent).toContain('Vale só para Mirathel.');
    const radios = el.querySelectorAll<HTMLInputElement>('input[type=radio]');
    expect(radios[0].checked).toBe(false);
    expect(radios[1].checked).toBe(true);
    expect(radios[0].disabled).toBe(false);
    expect(el.textContent).toContain('Os inimigos sempre rolam pelo app');
  });

  it('shows the master decision, locked, when the app is forced', () => {
    const el: HTMLElement = render(DiceMode.APP, DicePreference.PHYSICAL).nativeElement;
    expect(el.textContent).toContain('O mestre decidiu: todos rolam no app.');
    const radios = el.querySelectorAll<HTMLInputElement>('input[type=radio]');
    expect(radios[0].checked).toBe(true);
    expect(radios[0].disabled && radios[1].disabled).toBe(true);
    expect(el.textContent).toContain('Indisponível nesta campanha.');
    expect(el.querySelector('button')).toBeNull();
  });

  it('shows the physical decision when the master forces own dice', () => {
    const el: HTMLElement = render(DiceMode.PHYSICAL).nativeElement;
    expect(el.textContent).toContain('O mestre decidiu: todos rolam os próprios dados.');
    expect(el.querySelectorAll<HTMLInputElement>('input[type=radio]')[1].checked).toBe(true);
  });

  it('saves the new choice', async () => {
    const calls: DicePreference[] = [];
    const fixture = render(DiceMode.PLAYERS_CHOOSE, DicePreference.APP, (_id, p) => {
      calls.push(p);
      return Promise.resolve({ preference: p });
    });
    const el: HTMLElement = fixture.nativeElement;
    const save = el.querySelector('button') as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    el.querySelectorAll<HTMLInputElement>('input[type=radio]')[1].click();
    fixture.detectChanges();
    save.click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(calls).toEqual([DicePreference.PHYSICAL]);
    expect(el.textContent).toContain('Salvo.');
  });
});
