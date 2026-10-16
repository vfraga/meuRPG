import { TestBed } from '@angular/core/testing';

import { NotesClient } from '../../core/notes/notes-client';
import { FakeNotesClient, note, scene } from '../../core/notes/notes-testing';
import { NotesPanel } from './notes-panel';
import { nextTurn } from '../../../test-settle';

describe('NotesPanel (the character sheet)', () => {
  let api: FakeNotesClient;

  async function setup(
    notes = [
      note('n1', 'Perguntar ao ferreiro', new Date(2026, 9, 3, 21, 24), {
        sceneId: 's1',
        sceneName: 'A carroça tombada',
      }),
      note('c1', 'Um brasão de lobo', new Date(2026, 9, 3, 21, 20), {
        sceneId: 's1',
        sceneName: 'A carroça tombada',
        clue: true,
      }),
    ],
    collapsible = false,
  ) {
    api = new FakeNotesClient();
    api.notes = notes;
    api.scenesList = [scene('s1', 'A carroça tombada')];
    TestBed.configureTestingModule({ providers: [{ provide: NotesClient, useValue: api }] });
    Element.prototype.scrollIntoView = vi.fn();
    const fixture = TestBed.createComponent(NotesPanel);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('collapsible', collapsible);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        await fixture.whenStable();
        await nextTurn();
        fixture.detectChanges();
      }
    };
    await settle();
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.trim().includes(name),
      )!;
    const flat = (e: Element | null | undefined) => e?.textContent?.replace(/\s+/g, ' ').trim();
    const type = (value: string) => {
      const field = el.querySelector<HTMLTextAreaElement>('textarea')!;
      field.value = value;
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
    };
    return { fixture, el, settle, button, flat, type };
  }

  it('is a region named "Anotações" with the count, the privacy line and the same list as the session', async () => {
    const { el, flat } = await setup();
    expect(el.querySelector('section')?.getAttribute('aria-labelledby')).toBe('np-title');
    expect(flat(el.querySelector('.np__count'))).toBe('2 anotações');
    expect(flat(el.querySelector('.np__lock'))).toContain(
      'Só você lê as suas anotações. O mestre não vê.',
    );
    expect(el.querySelectorAll('.nl__row')).toHaveLength(2);
    expect(flat(el.querySelector('.nl__tag'))).toContain('Pista do mestre');
    expect(api.calls).toContain('list');
  });

  it('with no notes shows the invitation and "Nova anotação", outlined: the page\'s filled button is "Editar ficha"', async () => {
    const { el, button, flat } = await setup([]);
    expect(flat(el.querySelector('.np__empty'))).toContain('Nenhuma anotação ainda');
    expect(button('Nova anotação').classList).toContain('mat-mdc-outlined-button');
  });

  it('opens the form in place, hides the list while it is open, and the filled button is "Salvar anotação" beside an outlined "Cancelar" of the same size', async () => {
    const { el, button, settle } = await setup();
    button('Nova anotação').click();
    await settle();
    expect(el.querySelector('.nl')).toBeNull();
    expect(el.querySelector('.np__form-t')?.textContent).toBe('Nova anotação');
    const [save, cancel] = Array.from(el.querySelectorAll<HTMLButtonElement>('.np__pair button'));
    expect(save.textContent?.trim()).toBe('Salvar anotação');
    expect(save.classList).toContain('mat-mdc-unelevated-button');
    expect(cancel.textContent?.trim()).toBe('Cancelar');
    expect(cancel.classList).toContain('mat-mdc-outlined-button');
    expect(document.activeElement).toBe(el.querySelector('textarea'));
  });

  it('saves a note, goes back to the list with it on top, and keeps working on a locked sheet (the notes are not the sheet)', async () => {
    const { el, button, settle, type, flat } = await setup();
    button('Nova anotação').click();
    await settle();
    type('Comprar tinta');
    button('Salvar anotação').click();
    await settle();
    expect(api.calls).toContain('create Comprar tinta ');
    expect(flat(el.querySelector('.nl__text'))).toBe('Comprar tinta');
    expect(flat(el.querySelector('.np__count'))).toBe('3 anotações');
  });

  it('asks in place before discarding written text', async () => {
    const { el, button, settle, type } = await setup();
    button('Nova anotação').click();
    await settle();
    type('Algo');
    button('Cancelar').click();
    await settle();
    expect(el.querySelector('[role="alertdialog"]')?.textContent).toContain(
      'Descartar o que você escreveu?',
    );
    expect(document.activeElement).toBe(button('Continuar'));
  });

  it('says the limit in words above a disabled "Nova anotação" at 300 notes', async () => {
    const many = Array.from({ length: 300 }, (_, i) =>
      note(`x${i}`, `n${i}`, new Date(2026, 9, 1)),
    );
    const { el, flat, button } = await setup(many);
    expect(flat(el.querySelector('.np__limit'))).toBe(
      'Limite de 300 anotações. Apague uma para escrever outra.',
    );
    expect(button('Nova anotação').getAttribute('aria-describedby')).toBe('np-limit');
    // 300 rows render in about half a second alone, but past the 5 s default while the whole suite runs in parallel on a busy machine.
  }, 20_000);

  describe('collapsed to one row (a phone and a tablet)', () => {
    const toggle = (el: HTMLElement) => el.querySelector<HTMLButtonElement>('.np__toggle')!;

    it('is one "Anotações (N)" row with the list shut, and opens on a tap', async () => {
      const { el, settle, flat } = await setup(undefined, true);
      expect(flat(toggle(el))).toContain('Anotações (2)');
      expect(toggle(el).getAttribute('aria-expanded')).toBe('false');
      expect(el.querySelector<HTMLElement>('#np-body')?.hidden).toBe(true);

      toggle(el).click();
      await settle();
      expect(toggle(el).getAttribute('aria-expanded')).toBe('true');
      expect(el.querySelector<HTMLElement>('#np-body')?.hidden).toBe(false);
      expect(el.querySelectorAll('.nl__row')).toHaveLength(2);

      toggle(el).click();
      await settle();
      expect(el.querySelector<HTMLElement>('#np-body')?.hidden).toBe(true);
    });

    it('stays open while a note is being written', async () => {
      const { el, settle, button } = await setup(undefined, true);
      toggle(el).click();
      await settle();
      button('Nova anotação').click();
      await settle();
      toggle(el).click();
      await settle();
      expect(el.querySelector<HTMLElement>('#np-body')?.hidden).toBe(false);
      expect(el.querySelector('.np__form-t')).not.toBeNull();
    });

    it('is the full panel, with no row to tap, when it is not collapsible (the four-column sheet)', async () => {
      const { el } = await setup();
      expect(el.querySelector('.np__toggle')).toBeNull();
      expect(el.querySelector<HTMLElement>('#np-body')?.hidden).toBe(false);
    });
  });
});
