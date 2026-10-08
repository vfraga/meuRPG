import { TestBed } from '@angular/core/testing';

import { NextTurn } from './next-turn';

describe('NextTurn', () => {
  function setup(pendingNote: string | null) {
    const fixture = TestBed.createComponent(NextTurn);
    fixture.componentRef.setInput('pendingNote', pendingNote);
    const sent: boolean[] = [];
    fixture.componentInstance.next.subscribe((discard) => sent.push(discard));
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, sent };
  }

  it('passes the turn at once when nothing is owed', () => {
    const { el, sent } = setup(null);
    el.querySelector<HTMLButtonElement>('.next')!.click();
    expect(sent).toEqual([false]);
  });

  it('asks in place when a damage is owed, "Voltar" keeps the turn and "Passar o turno" discards it', async () => {
    const { fixture, el, sent } = setup('Falta aplicar 5 de dano');
    el.querySelector<HTMLButtonElement>('.next')!.click();
    fixture.detectChanges();
    await fixture.whenStable();
    expect(sent).toEqual([]);
    expect(el.querySelector('[role="alertdialog"]')?.textContent).toContain(
      'Há dano sem aplicar. Passar o turno mesmo assim?',
    );
    const [back, go] = Array.from(el.querySelectorAll<HTMLButtonElement>('.ask__buttons button'));
    expect(back.textContent?.trim()).toBe('Voltar');
    back.click();
    fixture.detectChanges();
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    el.querySelector<HTMLButtonElement>('.next')!.click();
    fixture.detectChanges();
    el.querySelectorAll<HTMLButtonElement>('.ask__buttons button')[1].click();
    expect(sent).toEqual([true]);
    expect(go.textContent?.trim()).toBe('Passar o turno');
  });

  describe('the question about the owed damage', () => {
    function ask() {
      const opened = setup('Falta aplicar 5 de dano');
      opened.el.querySelector<HTMLButtonElement>('.next')!.click();
      opened.fixture.detectChanges();
      expect(opened.el.querySelector('[role="alertdialog"]')).not.toBeNull();
      return opened;
    }

    it('closes when the owed damage is gone (applied elsewhere)', () => {
      const { fixture, el } = ask();
      fixture.componentRef.setInput('pendingNote', null);
      fixture.detectChanges();
      expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    });

    it('does not discard once the owed damage is gone', () => {
      const { fixture, el, sent } = ask();
      fixture.componentRef.setInput('pendingNote', null);
      fixture.detectChanges();
      el.querySelector<HTMLButtonElement>('.ask__go')?.click();
      expect(sent).not.toContain(true);
    });

    it('closes when the turn moves on', () => {
      const { fixture, el } = ask();
      fixture.componentRef.setInput('turn', 'g2:1');
      fixture.detectChanges();
      expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    });
  });
});
