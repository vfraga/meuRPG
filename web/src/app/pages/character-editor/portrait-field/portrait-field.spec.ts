import { TestBed } from '@angular/core/testing';
import { FormControl } from '@angular/forms';
import { MatDialog } from '@angular/material/dialog';

import type { ImagePickerData } from '../../../shared/gallery-picker/image-picker-dialog/image-picker-dialog';
import { GalleryClient } from '../../../core/images/gallery-client';
import { galleryImage, galleryUsage } from '../../../core/images/gallery-testing';
import { PortraitField } from './portrait-field';
import { nextTurn } from '../../../../test-settle';

/** The text of an element as read, without the icons' ligature names. */
const flat = (e: Element | null | undefined) => {
  if (!e) {
    return undefined;
  }
  const copy = e.cloneNode(true) as Element;
  copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
  return copy.textContent?.replace(/\s+/g, ' ').trim();
};

describe('PortraitField (E8-08)', () => {
  const images = [galleryImage('i1', 'Retrato da Mira'), galleryImage('i2', 'Capitão Goblin')];
  const open = vi.fn();
  const list = vi.fn();

  function setup(initial = '', name = 'Mira') {
    open.mockReset().mockReturnValue({ afterClosed: () => ({ subscribe: () => undefined }) });
    list.mockReset().mockResolvedValue({ images, usage: galleryUsage(images) });
    TestBed.configureTestingModule({
      providers: [
        { provide: MatDialog, useValue: { open } },
        { provide: GalleryClient, useValue: { list } },
      ],
    });
    const control = new FormControl(initial, { nonNullable: true });
    const nameControl = new FormControl(name, { nonNullable: true });
    const fixture = TestBed.createComponent(PortraitField);
    fixture.componentRef.setInput('control', control);
    fixture.componentRef.setInput('nameControl', nameControl);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.detectChanges();
    document.body.append(fixture.nativeElement);
    return { fixture, control, nameControl, el: fixture.nativeElement as HTMLElement };
  }

  async function settle(fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) {
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
      await nextTurn();
      fixture.detectChanges();
    }
  }

  const button = (el: HTMLElement, name: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) => flat(b) === name);
  const pickerData = () =>
    open.mock.calls[0][0] === undefined
      ? undefined
      : (open.mock.calls[0][1] as { data: ImagePickerData }).data;

  afterEach(() => document.body.querySelectorAll('app-portrait-field').forEach((e) => e.remove()));

  describe('without a portrait', () => {
    it('shows the initials in the frame, what that means, and "Escolher retrato"', () => {
      const { el } = setup('', 'Aldo');
      expect(flat(el.querySelector('.pf__label'))).toBe('Retrato');
      expect(flat(el.querySelector('app-portrait .pt__initials'))).toBe('AL');
      expect(el.querySelector('app-portrait img')).toBeNull();
      expect(flat(el.querySelector('.pf__caption'))).toBe('Sem retrato: aparecem as iniciais.');
      expect(button(el, 'Escolher retrato')).toBeDefined();
      expect(button(el, 'Trocar retrato')).toBeUndefined();
      expect(button(el, 'Remover retrato')).toBeUndefined();
    });

    it('draws "CG" for a name of two words, and follows the name as it is typed', async () => {
      const { el, nameControl, fixture } = setup('', 'Capitão Goblin');
      expect(flat(el.querySelector('.pt__initials'))).toBe('CG');
      nameControl.setValue('Barão Ivo');
      await settle(fixture);
      expect(flat(el.querySelector('.pt__initials'))).toBe('BI');
    });
  });

  describe('choosing', () => {
    it('opens the gallery picker with nothing chosen: the dashed button, "Escolha uma imagem.", a title by the name', () => {
      const { el } = setup('', 'Aldo');
      button(el, 'Escolher retrato')!.click();
      const data = pickerData()!;
      expect(data.title).toBe('Escolher o retrato de Aldo');
      expect(data.lead).toBe('Toque numa imagem da galeria da campanha.');
      expect(data.confirmLabel).toBe('Usar este retrato');
      expect(data.emptyError).toBe('Escolha uma imagem.');
      expect(data.dashedUntilPicked).toBe(true);
      expect(data.current).toBeNull();
      expect(data.campaignId).toBe('c1');
    });

    it('puts the chosen image in the control, shows it and names it, without saving anything itself', async () => {
      const { el, control, fixture } = setup('', 'Mira');
      button(el, 'Escolher retrato')!.click();
      await pickerData()!.submit(images[0]);
      await settle(fixture);
      expect(control.value).toBe('i1');
      expect(control.dirty).toBe(true);
      expect(el.querySelector('app-portrait img')?.getAttribute('src')).toBe('/images/i1');
      expect(flat(el.querySelector('.pf__caption'))).toBe('Imagem da galeria: “Retrato da Mira”');
      expect(button(el, 'Trocar retrato')).toBeDefined();
      expect(button(el, 'Remover retrato')).toBeDefined();
    });

    it('changes it with "Trocar retrato", again with nothing chosen first', async () => {
      const { el, control, fixture } = setup('i1', 'Mira');
      button(el, 'Trocar retrato')!.click();
      expect(pickerData()!.current).toBeNull();
      await pickerData()!.submit(images[1]);
      await settle(fixture);
      expect(control.value).toBe('i2');
      expect(flat(el.querySelector('.pf__caption'))).toBe('Imagem da galeria: “Capitão Goblin”');
    });

    it('names a portrait the sheet already had from the gallery', async () => {
      const { el, fixture } = setup('i2', 'Capitão Goblin');
      await settle(fixture);
      expect(list).toHaveBeenCalledWith('c1');
      expect(flat(el.querySelector('.pf__caption'))).toBe('Imagem da galeria: “Capitão Goblin”');
    });

    it('keeps a short caption when the gallery cannot be read', async () => {
      const { el, fixture } = setup('i2');
      list.mockRejectedValue(new Error('network'));
      await settle(fixture);
      expect(flat(el.querySelector('.pf__caption'))).toMatch(/^Imagem da galeria/);
    });
  });

  describe('removing', () => {
    it('asks in place, with "Voltar" first and focused, the two buttons stacked, and says the image stays', async () => {
      const { el, fixture } = setup('i1', 'Mira');
      button(el, 'Remover retrato')!.click();
      await settle(fixture);
      const ask = el.querySelector('.pf__ask')!;
      expect(ask.getAttribute('role')).toBe('group');
      expect(flat(ask.querySelector('.pf__ask-title'))).toBe('Remover o retrato de Mira?');
      expect(flat(ask.querySelector('.pf__ask-text'))).toBe('A imagem continua na galeria.');
      const buttons = Array.from(ask.querySelectorAll('button'));
      expect(buttons.map((b) => flat(b))).toEqual(['Voltar', 'Remover retrato']);
      expect(document.activeElement).toBe(buttons[0]);
      // The picker and the text action are gone while the question is open.
      expect(button(el, 'Trocar retrato')).toBeUndefined();
    });

    it('"Voltar" changes nothing and gives focus back to "Remover retrato"', async () => {
      const { el, control, fixture } = setup('i1');
      button(el, 'Remover retrato')!.click();
      await settle(fixture);
      button(el, 'Voltar')!.click();
      await settle(fixture);
      expect(control.value).toBe('i1');
      expect(el.querySelector('.pf__ask')).toBeNull();
      expect(document.activeElement).toBe(button(el, 'Remover retrato'));
    });

    it('removing goes back to the initials and sends focus to "Escolher retrato"; the image is not deleted from anywhere', async () => {
      const { el, control, fixture } = setup('i1', 'Mira');
      button(el, 'Remover retrato')!.click();
      await settle(fixture);
      el.querySelector<HTMLButtonElement>('.pf__ask-btn--danger')!.click();
      await settle(fixture);
      expect(control.value).toBe('');
      expect(control.dirty).toBe(true);
      expect(flat(el.querySelector('.pt__initials'))).toBe('MI');
      expect(flat(el.querySelector('.pf__caption'))).toBe('Sem retrato: aparecem as iniciais.');
      expect(document.activeElement).toBe(button(el, 'Escolher retrato'));
    });
  });
});
