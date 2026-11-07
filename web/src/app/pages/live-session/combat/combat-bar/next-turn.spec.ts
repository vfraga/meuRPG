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

  it('stays in its place, off, with the reason, in a joint turn; pressing it passes nothing', () => {
    const { fixture, el, sent } = setup(null);
    fixture.componentRef.setInput('joint', true);
    fixture.detectChanges();
    const button = el.querySelector<HTMLButtonElement>('.next')!;
    expect(button.textContent).toContain('Próximo turno');
    expect(button.classList.contains('next--off')).toBe(true);
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(el.querySelector('.why')?.textContent).toContain(
      'Cada um encerra a parte no painel Turno',
    );
    button.click();
    expect(sent).toEqual([]);
  });

  it('asks the idle character without a pronoun, whoever the character is', () => {
    for (const name of ['Toren', 'Brisa', 'Seraphin']) {
      const { fixture, el } = setup(null);
      fixture.componentRef.setInput('idle', name);
      fixture.detectChanges();
      el.querySelector<HTMLButtonElement>('.next')!.click();
      fixture.detectChanges();
      expect(el.querySelector('#idle-title')?.textContent?.trim()).toBe(
        `Passar a vez de ${name}, que ainda não agiu?`,
      );
    }
  });

  describe('a player character who has not acted', () => {
    function idle() {
      const opened = setup(null);
      opened.fixture.componentRef.setInput('idle', 'Toren');
      opened.fixture.detectChanges();
      opened.el.querySelector<HTMLButtonElement>('.next')!.click();
      opened.fixture.detectChanges();
      return opened;
    }

    it('asks in place before passing; "Cancelar" keeps the turn, "Passar a vez" passes it', () => {
      const { fixture, el, sent } = idle();
      expect(sent).toEqual([]);
      expect(el.querySelector('[role="alertdialog"]')?.textContent).toContain(
        'Passar a vez de Toren, que ainda não agiu?',
      );
      const [cancel, pass] = Array.from(
        el.querySelectorAll<HTMLButtonElement>('.ask__buttons button'),
      );
      expect(cancel.textContent?.trim()).toBe('Cancelar');
      cancel.click();
      fixture.detectChanges();
      expect(el.querySelector('[role="alertdialog"]')).toBeNull();
      el.querySelector<HTMLButtonElement>('.next')!.click();
      fixture.detectChanges();
      el.querySelectorAll<HTMLButtonElement>('.ask__buttons button')[1].click();
      expect(sent).toEqual([false]);
      expect(pass.textContent?.trim()).toBe('Passar a vez');
    });

    it('closes on Escape', () => {
      const { fixture, el, sent } = idle();
      el.querySelector('[role="alertdialog"]')!.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Escape' }),
      );
      fixture.detectChanges();
      expect(el.querySelector('[role="alertdialog"]')).toBeNull();
      expect(sent).toEqual([]);
    });
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

  it('is off with the reason written under it and linked to it while a question holds the turn', () => {
    const fixture = TestBed.createComponent(NextTurn);
    fixture.componentRef.setInput('waiting', true);
    fixture.componentRef.setInput('why', 'Responda ao pedido abaixo para seguir.');
    const sent: boolean[] = [];
    fixture.componentInstance.next.subscribe((d) => sent.push(d));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const button = el.querySelector<HTMLButtonElement>('.next')!;
    const why = el.querySelector('.why')!;
    expect(why.textContent).toContain('Responda ao pedido abaixo para seguir.');
    expect(button.getAttribute('aria-describedby')).toBe(why.id);
    expect(button.getAttribute('aria-disabled')).toBe('true');
    button.click();
    expect(sent).toEqual([]);
  });

  it('says what legendary actions are left under the button, and nothing when none is', () => {
    const fixture = TestBed.createComponent(NextTurn);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('[data-testid="legendary-left"]')).toBeNull();
    fixture.componentRef.setInput('legendary', 'Restam 2 ações lendárias: Dragão vermelho adulto.');
    fixture.detectChanges();
    const line = el.querySelector('[data-testid="legendary-left"]');
    expect(line?.textContent).toContain('Restam 2 ações lendárias: Dragão vermelho adulto.');
    // Under the button, and the button still passes the turn.
    expect(line?.previousElementSibling?.classList.contains('next')).toBe(true);
  });
});
