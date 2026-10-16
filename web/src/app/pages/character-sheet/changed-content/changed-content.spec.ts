import { TestBed } from '@angular/core/testing';

import type { ChangedContentVm } from '../character-sheet.types';
import {
  changeAnnouncement,
  changeIntro,
  changeSheetTitle,
  changeTitle,
} from './changed-content-format';
import { ChangedContentNotice } from './changed-content';
import { nextTurn } from '../../../../test-settle';

// "A classe mudou" (E10-02 state 8, RN-23 question 80): the sentences are the server's, shown as they come.

const SENTENCE = 'Guardião do Vale agora dá 2 perícias no nível 1; esta ficha tem 3.';
const change = (over: Partial<ChangedContentVm> = {}): ChangedContentVm => ({
  key: 'class:guardiao-do-vale@mesa',
  namePt: 'Guardião do Vale',
  changedAt: new Date(2026, 9, 5, 14, 0),
  messages: [SENTENCE],
  ...over,
});

describe('the words around the change sentences', () => {
  it('names the kind of entry from its key', () => {
    expect(changeTitle(change())).toBe('A classe mudou.');
    expect(changeTitle(change({ key: 'subclass:x@mesa' }))).toBe('A subclasse mudou.');
    expect(changeTitle(change({ key: 'race:corujeiro@mesa' }))).toBe('A raça mudou.');
    expect(changeTitle(change({ key: 'subrace:x@mesa' }))).toBe('A sub-raça mudou.');
    expect(changeTitle(change({ key: 'background:x@mesa' }))).toBe('O antecedente mudou.');
    expect(changeTitle(change({ key: 'spell:x@mesa' }))).toBe('A magia mudou.');
    expect(changeTitle(change({ key: 'nothing' }))).toBe('O conteúdo mudou.');
  });

  it('says who changed it only as "o mestre", with the date, and never the history', () => {
    expect(changeIntro(change())).toBe(
      'O mestre mudou a classe Guardião do Vale em 05/10/2026. Os números da ficha já usam as regras novas. O que não combina mais:',
    );
    expect(changeIntro(change({ changedAt: null }))).toBe(
      'O mestre mudou a classe Guardião do Vale. Os números da ficha já usam as regras novas. O que não combina mais:',
    );
  });

  it('writes the title of the sheet and the live-region sentence', () => {
    expect(changeSheetTitle(change())).toBe('O que mudou: Guardião do Vale');
    expect(changeAnnouncement(change())).toBe('A classe mudou: 1 aviso.');
    expect(changeAnnouncement(change({ messages: ['a', 'b'] }))).toBe('A classe mudou: 2 avisos.');
  });
});

describe('ChangedContentNotice', () => {
  function render(changes: ChangedContentVm[], canEdit = false) {
    const fixture = TestBed.createComponent(ChangedContentNotice);
    fixture.componentRef.setInput('changes', changes);
    fixture.componentRef.setInput('canEdit', canEdit);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  it('shows the title, the intro and the sentence exactly as the server wrote it', () => {
    const { el } = render([change()]);
    const notice = el.querySelector('.mr-notice--warning')!;
    expect(notice.textContent).toContain('A classe mudou.');
    expect(notice.textContent).toContain('O mestre mudou a classe Guardião do Vale em 05/10/2026.');
    expect(Array.from(notice.querySelectorAll('li')).map((li) => li.textContent)).toEqual([
      SENTENCE,
    ]);
    // A live region says it once, in a few words, for a screen reader.
    expect(notice.querySelector('[role="status"]')?.textContent).toBe('A classe mudou: 1 aviso.');
  });

  it('makes a notice per changed entry, the same for a race', () => {
    const { el } = render([
      change(),
      change({
        key: 'race:corujeiro@mesa',
        namePt: 'Corujeiro',
        messages: ['Corujeiro agora tem 2 traços; esta ficha tem 3.'],
      }),
    ]);
    const titles = Array.from(el.querySelectorAll('.change__text strong')).map(
      (s) => s.textContent,
    );
    expect(titles).toEqual(['A classe mudou.', 'A raça mudou.']);
  });

  it('shows nothing when nothing changed', () => {
    const { el } = render([]);
    expect(el.querySelector('.mr-notice')).toBeNull();
  });

  it('opens "O que mudou" with the sentences, who fixes it, and closes with "Fechar"', async () => {
    const { fixture, el } = render([change()], false);
    (el.querySelector('.change__open') as HTMLButtonElement).click();
    fixture.detectChanges();
    await fixture.whenStable();
    const sheet = document.querySelector('app-changed-content-sheet')!;
    expect(sheet.querySelector('h2')?.textContent).toBe('O que mudou: Guardião do Vale');
    expect(sheet.textContent).toContain(SENTENCE);
    expect(sheet.textContent).toContain(
      'Quem ajusta: o mestre, na ficha. O aviso some sozinho quando os números voltam a combinar.',
    );
    (
      Array.from(sheet.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Fechar',
      ) as HTMLButtonElement
    ).click();
    await fixture.whenStable();
    await nextTurn();
    expect(document.querySelector('app-changed-content-sheet')).toBeNull();
  });

  it('"Quem ajusta" follows who can edit the sheet: the owner of an unlocked draft fixes it themselves', async () => {
    const { fixture, el } = render([change()], true);
    (el.querySelector('.change__open') as HTMLButtonElement).click();
    fixture.detectChanges();
    await fixture.whenStable();
    expect(document.querySelector('app-changed-content-sheet')?.textContent).toContain(
      'Quem ajusta: você, na ficha.',
    );
    (
      Array.from(document.querySelectorAll('app-changed-content-sheet button')).find(
        (b) => b.textContent?.trim() === 'Fechar',
      ) as HTMLButtonElement
    ).click();
    await nextTurn();
  });

  it("says the sheet's skills under a sentence about skills, as drawn, and not under another", async () => {
    const fixture = TestBed.createComponent(ChangedContentNotice);
    fixture.componentRef.setInput('changes', [
      change(),
      change({
        key: 'race:x@mesa',
        namePt: 'X',
        messages: ['X agora tem 2 traços; esta ficha tem 3.'],
      }),
    ]);
    fixture.componentRef.setInput('skills', ['Atletismo', 'Natureza', 'Sobrevivência']);
    fixture.detectChanges();
    const buttons = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll<HTMLButtonElement>('.change__open'),
    );
    buttons[0].click();
    fixture.detectChanges();
    await fixture.whenStable();
    expect(document.querySelector('app-changed-content-sheet')?.textContent).toContain(
      'Perícias da ficha: Atletismo, Natureza e Sobrevivência.',
    );
    (
      Array.from(document.querySelectorAll('app-changed-content-sheet button')).find(
        (b) => b.textContent?.trim() === 'Fechar',
      ) as HTMLButtonElement
    ).click();
    await nextTurn();
    buttons[1].click();
    fixture.detectChanges();
    await fixture.whenStable();
    expect(document.querySelector('app-changed-content-sheet')?.textContent).not.toContain(
      'Perícias da ficha',
    );
    (
      Array.from(document.querySelectorAll('app-changed-content-sheet button')).find(
        (b) => b.textContent?.trim() === 'Fechar',
      ) as HTMLButtonElement
    ).click();
    await nextTurn();
  });

  it('tells the master that the fix is theirs', async () => {
    const { fixture, el } = render([change()], true);
    (el.querySelector('.change__open') as HTMLButtonElement).click();
    fixture.detectChanges();
    await fixture.whenStable();
    expect(document.querySelector('app-changed-content-sheet')?.textContent).toContain(
      'Quem ajusta: você, na ficha.',
    );
    (
      Array.from(document.querySelectorAll('app-changed-content-sheet button')).find(
        (b) => b.textContent?.trim() === 'Fechar',
      ) as HTMLButtonElement
    ).click();
    await nextTurn();
  });
});
