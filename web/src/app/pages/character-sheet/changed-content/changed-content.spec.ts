import { TestBed } from '@angular/core/testing';

import type { ChangedContentVm } from '../character-sheet.types';
import {
  changeAnnouncement,
  changeIntro,
  changeSheetTitle,
  changeTitle,
  isShieldLine,
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
  codes: ['skill_count'],
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
    expect(changeTitle(change({ key: 'weapon:machado-de-lenhador@mesa' }))).toBe('A arma mudou.');
    expect(changeTitle(change({ key: 'nothing' }))).toBe('O conteúdo mudou.');
  });

  it('names a feat, an option and a class feature as the server does (580-05)', () => {
    expect(changeTitle(change({ key: 'feat:pede-depois@mesa' }))).toBe('O talento mudou.');
    expect(changeTitle(change({ key: 'option:fighting_style:x@mesa' }))).toBe('A opção mudou.');
    expect(changeTitle(change({ key: 'classfeature:x@mesa' }))).toBe('A característica mudou.');
    expect(
      changeIntro(
        change({ key: 'feat:pede-depois@mesa', namePt: 'Pede depois', changedAt: undefined }),
      ),
    ).toContain('O mestre mudou o talento Pede depois.');
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

  it('names a weapon the master changed, with the board sentence, and puts the button 8 px under it (580-16, state 8)', () => {
    const sentence =
      'A arma Machado de Lenhador passou a exigir as duas mãos. Com ela em uso, o escudo de Tor não conta na CA (CA 18 → 16).';
    const { el } = render([
      change({
        key: 'weapon:machado-de-lenhador@mesa',
        namePt: 'Machado de Lenhador',
        changedAt: new Date(2026, 10, 14, 9, 0),
        messages: [sentence],
        codes: ['shield_idle'],
      }),
    ]);
    const notice = el.querySelector('.mr-notice--warning')!;
    expect(notice.querySelector('.change__text')?.textContent?.replace(/\s+/g, ' ').trim()).toBe(
      'A arma mudou. O mestre mudou a arma Machado de Lenhador em 14/11/2026. Os números da ficha já usam as regras novas. O que não combina mais:',
    );
    // "CA 18 → 16" is tied with no-break spaces: a line never breaks inside the two numbers.
    expect(Array.from(notice.querySelectorAll('li')).map((li) => li.textContent)).toEqual([
      sentence.replace('CA 18 → 16', 'CA\u00a018\u00a0→\u00a016'),
    ]);
    // The board has 8 px between the last line and the button, the gap of the column and nothing more.
    const open = notice.querySelector<HTMLElement>('.change__open')!;
    expect(['', '0', '0px']).toContain(getComputedStyle(open).marginTop);
  });

  describe('the shield notice (580-16, state 8): the owner of the sheet stows the gear, locked or not', () => {
    const shield = (): ChangedContentVm =>
      change({
        key: 'weapon:machado-de-lenhador@mesa',
        namePt: 'Machado de Lenhador',
        messages: [
          'A arma Machado de Lenhador passou a exigir as duas mãos. Com ela em uso, o escudo de Tor não conta na CA (CA 18 → 16).',
        ],
        codes: ['shield_idle'],
      });
    const WHO =
      'Quem ajusta: o dono da ficha, guardando o escudo ou a arma. O aviso some quando um dos dois deixa de estar em uso.';

    async function openSheet(canEdit: boolean, vm = shield()): Promise<Element> {
      const { fixture, el } = render([vm], canEdit);
      (el.querySelector('.change__open') as HTMLButtonElement).click();
      fixture.detectChanges();
      await fixture.whenStable();
      return document.querySelector('app-changed-content-sheet')!;
    }
    async function closeSheet(): Promise<void> {
      (
        Array.from(document.querySelectorAll('app-changed-content-sheet button')).find(
          (b) => b.textContent?.trim() === 'Fechar',
        ) as HTMLButtonElement
      ).click();
      await nextTurn();
    }

    it('says the same two sentences to the owner of a locked sheet and to whoever can edit it', async () => {
      for (const canEdit of [false, true]) {
        const sheet = await openSheet(canEdit);
        const who = sheet.querySelector('.sheet__who')?.textContent?.replace(/\s+/g, ' ').trim();
        expect(who).toBe(WHO);
        expect(sheet.textContent).not.toContain('o mestre, na ficha');
        expect(sheet.textContent).not.toContain('voltam a combinar');
        await closeSheet();
      }
    });

    it('is told by the code alone: the same words with another code, or with no code, are a plain sentence', async () => {
      const words = shield().messages;
      for (const [codes, isShield] of [
        [['skill_count'], false],
        [[], false],
        [['shield_idle'], true],
      ] as const) {
        expect(isShieldLine({ ...shield(), messages: words, codes }, 0)).toBe(isShield);
      }
      const sheet = await openSheet(false, { ...shield(), codes: [] });
      expect(sheet.textContent).not.toContain('Quem ajusta: o dono da ficha');
      expect(sheet.textContent).toContain('Quem ajusta: o mestre, na ficha.');
      await closeSheet();
    });

    it("is decided for each line by that line's code, never for the whole entry", async () => {
      const mixed = {
        ...shield(),
        messages: [shield().messages[0], 'Corujeiro agora dá 2 perícias; esta ficha tem 3.'],
        codes: ['shield_idle', 'skill_count'],
      };
      expect(isShieldLine(mixed, 0)).toBe(true);
      expect(isShieldLine(mixed, 1)).toBe(false);
      const sheet = await openSheet(false, mixed);
      expect(
        Array.from(sheet.querySelectorAll('.sheet__who')).map((p) =>
          p.textContent?.replace(/\s+/g, ' ').trim(),
        ),
      ).toEqual([
        WHO,
        'Quem ajusta: o mestre, na ficha. O aviso some sozinho quando os números voltam a combinar.',
      ]);
      await closeSheet();
      // The skill line is not the shield one: with the codes the other way round, the same two sentences swap places.
      expect(isShieldLine({ ...mixed, codes: ['skill_count', 'shield_idle'] }, 0)).toBe(false);
      expect(isShieldLine({ ...mixed, codes: ['skill_count', 'shield_idle'] }, 1)).toBe(true);
    });

    it("reads the code at the line's own place: a line with no code at that place is a plain sentence", () => {
      const two = { ...shield(), messages: ['a', 'b'], codes: ['shield_idle'] };
      expect(isShieldLine(two, 0)).toBe(true);
      expect(isShieldLine(two, 1)).toBe(false);
      expect(isShieldLine({ ...two, codes: [] }, 0)).toBe(false);
      expect(isShieldLine({ ...two, codes: ['skill_count', 'skill_count'] }, 0)).toBe(false);
    });

    it('keeps the old sentences for every other notice', async () => {
      const sheet = await openSheet(false, change());
      expect(sheet.textContent).toContain(
        'Quem ajusta: o mestre, na ficha. O aviso some sozinho quando os números voltam a combinar.',
      );
      await closeSheet();
    });
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
