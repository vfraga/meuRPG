import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { splitXp } from '../../core/progression/xp-math';
import { XpActions } from './xp-actions';
import { type Recipient, XpRecipients } from './xp-recipients';
import { XpSplit } from './xp-split';

@Component({
  imports: [XpActions, XpRecipients, XpSplit],
  template: `
    <app-xp-recipients [rows]="rows()" (toggle)="toggled.push($event)" />
    <app-xp-split [split]="split()" [total]="350" />
    <app-xp-actions
      primaryLabel="Dar 116 XP a cada um"
      secondaryLabel="Agora não"
      [primaryFirst]="first()"
      [blocked]="blocked()"
      [reason]="reason()"
      (primary)="primary = primary + 1"
      (secondary)="secondary = secondary + 1"
    />
  `,
})
class Host {
  readonly rows = signal<Recipient[]>([
    { id: 'p', name: 'Pensantus', sub: 'Mago 3', checked: true, amount: '+116 XP' },
    {
      id: 'b',
      name: 'Brisa',
      sub: 'Ladina 3',
      checked: false,
      amount: 'Não recebe',
      tag: { label: 'Caído', icon: 'warning' },
      note: 'Morreu: não recebe XP.',
      disabled: true,
    },
  ]);
  readonly split = signal(splitXp(350, 3));
  readonly first = signal(true);
  readonly blocked = signal(false);
  readonly reason = signal('');
  readonly toggled: string[] = [];
  primary = 0;
  secondary = 0;
}

describe('the XP pieces', () => {
  function setup() {
    TestBed.configureTestingModule({ imports: [Host] });
    const fixture = TestBed.createComponent(Host);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, host: fixture.componentInstance };
  }

  describe('XpRecipients', () => {
    it('has a labelled checkbox per character and says what each gets, in words', () => {
      const { el } = setup();
      const boxes = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]'));
      expect(boxes.map((b) => b.checked)).toEqual([true, false]);
      expect(el.querySelectorAll('li')[0].textContent).toContain('+116 XP');
      // Not receiving is a word, never only an empty box.
      expect(el.querySelectorAll('li')[1].textContent).toContain('Não recebe');
      expect(el.querySelectorAll('li')[1].textContent).toContain('Caído');
      expect(el.querySelectorAll('li')[1].textContent).toContain('Morreu: não recebe XP.');
      expect(el.querySelector('ul')?.getAttribute('aria-label')).toBe('Quem recebe');
    });

    it('says who was tapped, and does not offer a dead character', () => {
      const { el, host } = setup();
      el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')[0].click();
      expect(host.toggled).toEqual(['p']);
      expect(el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')[1].disabled).toBe(
        true,
      );
    });
  });

  describe('XpSplit', () => {
    it('shows the big number and the sum, and announces only the number', () => {
      const { el } = setup();
      expect(el.querySelector('.split__big')?.textContent).toBe('116 XP para cada');
      expect(el.querySelector('.split__sum')?.textContent).toContain('arredondado para baixo');
      const status = el.querySelector('app-xp-split [role="status"]');
      expect(status?.textContent).toBe('116 XP para cada.');
      expect(status?.getAttribute('aria-live')).toBe('polite');
    });

    it('says nobody is checked', () => {
      const { fixture, el, host } = setup();
      host.split.set(splitXp(350, 0));
      fixture.detectChanges();
      expect(el.querySelector('.split__big')?.textContent).toBe('Ninguém marcado');
      expect(el.querySelector('.split__sum')).toBeNull();
    });
  });

  describe('XpActions', () => {
    const buttons = (el: HTMLElement) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('app-xp-actions button'));

    it('has two real buttons, the filled one first when it leads', () => {
      const { el } = setup();
      expect(buttons(el).map((b) => b.textContent?.trim())).toEqual([
        'Dar 116 XP a cada um',
        'Agora não',
      ]);
      expect(buttons(el)[0].classList.contains('mat-mdc-unelevated-button')).toBe(true);
      expect(buttons(el)[1].classList.contains('mat-mdc-outlined-button')).toBe(true);
    });

    it('puts the way out first in a dialog footer, for the keyboard too', () => {
      const { fixture, el, host } = setup();
      host.first.set(false);
      fixture.detectChanges();
      expect(buttons(el).map((b) => b.textContent?.trim())).toEqual([
        'Agora não',
        'Dar 116 XP a cada um',
      ]);
    });

    it('says why it waits, linked to the button, and still lets a press through', () => {
      const { fixture, el, host } = setup();
      host.blocked.set(true);
      host.reason.set('Marque pelo menos um personagem');
      fixture.detectChanges();

      const why = el.querySelector('app-xp-actions .reason')!;
      expect(why.textContent).toBe('Marque pelo menos um personagem');
      expect(buttons(el)[0].getAttribute('aria-describedby')).toBe(why.id);
      expect(buttons(el)[0].getAttribute('aria-disabled')).toBe('true');
      expect(buttons(el)[0].disabled).toBe(false);
      buttons(el)[0].click();
      expect(host.primary).toBe(1);
    });

    it('is not "disabled" while it can go', () => {
      const { el } = setup();
      expect(buttons(el)[0].getAttribute('aria-disabled')).not.toBe('true');
      buttons(el)[1].click();
      expect(el.querySelector('app-xp-actions .reason')).toBeNull();
    });
  });
});
