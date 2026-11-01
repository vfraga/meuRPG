import { type ComponentFixture, TestBed } from '@angular/core/testing';

import { type HeldRollOffer, HeldRollKind } from '../../../gen/meurpg/play/v1/combat_pb';
import type { HeldAnswer } from '../../core/held-rolls/held-roll-offer';
import {
  heldDie,
  heldIndomitable,
  heldLucky,
  heldStroke,
} from '../../core/held-rolls/held-roll-testing';
import { HeldRollPrompt, type HeldRollLook } from './held-roll-prompt';

describe('HeldRollPrompt', () => {
  let answers: HeldAnswer[];

  function show(
    offer: HeldRollOffer,
    inputs: { look?: HeldRollLook; canApp?: boolean; canType?: boolean; busy?: boolean } = {},
  ) {
    const fixture: ComponentFixture<HeldRollPrompt> = TestBed.createComponent(HeldRollPrompt);
    fixture.componentRef.setInput('offer', offer);
    for (const [name, value] of Object.entries(inputs)) {
      fixture.componentRef.setInput(name, value);
    }
    fixture.componentInstance.answer.subscribe((a) => answers.push(a));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const text = () => (el.textContent ?? '').replace(/\s+/g, ' ');
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.includes(name),
      );
    return { fixture, el, text, button };
  }

  beforeEach(() => {
    answers = [];
  });

  describe('Stroke of Luck’s question', () => {
    it('asks "Errou o ataque" with "Usar" and "Manter" and no roll picker', () => {
      const { el, text, button } = show(heldStroke(), { look: 'sheet' });
      expect(el.querySelector('h3')?.textContent).toBe(
        'Errou o ataque. Usar Golpe de Sorte para acertar?',
      );
      expect(text()).toContain('Gasta o uso');
      expect(button('Usar')).toBeTruthy();
      expect(button('Manter')).toBeTruthy();
      expect(button('Digitar')).toBeUndefined();
      expect(el.querySelector('input')).toBeNull();
    });

    it('says nothing of the result when the table hides it', () => {
      const { el, text } = show(heldStroke(2, 0, false));
      expect(el.querySelector('h3')?.textContent).toBe(
        'Usar Golpe de Sorte para acertar o ataque?',
      );
      expect(text()).not.toContain('Errou');
      expect(text()).not.toContain('Falhou');
    });

    it('asks about a failed check as a check', () => {
      const { el } = show(heldStroke(2, 0, true, { kind: HeldRollKind.SCENE_CHECK }));
      expect(el.querySelector('h3')?.textContent).toBe(
        'Falhou no teste. Usar Golpe de Sorte para contar o d20 como 20?',
      );
    });

    it('answers "Usar" and "Manter" with no roll', () => {
      const { button } = show(heldStroke());
      button('Usar')!.click();
      button('Manter')!.click();
      expect(answers).toEqual([
        { kind: 'stroke', use: true },
        { kind: 'stroke', use: false },
      ]);
    });
  });

  describe('Indomitable’s question', () => {
    it('asks "Falhou na salvaguarda" with the rule that the new roll must be used', () => {
      const { el, text, button } = show(heldIndomitable(), { look: 'sheet' });
      expect(el.querySelector('h3')?.textContent).toBe(
        'Falhou na salvaguarda. Usar Indomitável para rolar de novo?',
      );
      expect(text()).toContain('precisa ficar com o novo resultado, mesmo que seja pior');
      expect(button('Rolar de novo')).toBeTruthy();
      expect(button('Digitar o resultado')).toBeTruthy();
      expect(button('Manter')).toBeTruthy();
    });

    it('says nothing of the result when the table hides it', () => {
      const { el, text } = show(heldIndomitable(5, 0, false));
      expect(el.querySelector('h3')?.textContent).toBe(
        'Usar Indomitável para rolar a salvaguarda de novo?',
      );
      expect(text()).not.toContain('Falhou');
    });

    it('answers "Rolar de novo" with the save rolled in the app and "Manter" with no roll', () => {
      const { button } = show(heldIndomitable());
      button('Rolar de novo')!.click();
      button('Manter')!.click();
      expect(answers).toEqual([
        { kind: 'indomitable', reroll: true, roll: { inApp: true } },
        { kind: 'indomitable', reroll: false, roll: null },
      ]);
    });

    it('answers with the typed face, 1 to 20, when the table rolls real dice', () => {
      const { fixture, el, button } = show(heldIndomitable(), { canApp: false });
      expect(button('Rolar de novo')).toBeUndefined();
      const field = el.querySelector<HTMLInputElement>('input')!;
      field.value = '15';
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      button('Confirmar 15')!.click();
      expect(answers).toEqual([{ kind: 'indomitable', reroll: true, roll: { face: 15 } }]);
    });

    it('types both faces for a save rolled with two dice and answers them as `faces`', () => {
      const pair = heldIndomitable();
      pair.d20!.faces = [5, 8];
      const { fixture, el, button } = show(pair);
      expect(button('Rolar de novo')).toBeTruthy();
      button('Digitar o resultado')!.click();
      fixture.detectChanges();
      const fields = el.querySelectorAll<HTMLInputElement>('input');
      expect(fields).toHaveLength(2);
      [
        ['4', fields[0]],
        ['15', fields[1]],
      ].forEach(([v, f]) => {
        (f as HTMLInputElement).value = v as string;
        (f as HTMLInputElement).dispatchEvent(new Event('input'));
      });
      fixture.detectChanges();
      el.querySelector('form')!.dispatchEvent(new Event('submit'));
      expect(answers).toEqual([{ kind: 'indomitable', reroll: true, roll: { faces: [4, 15] } }]);
    });

    it('rolls a pair again in the app too', () => {
      const pair = heldIndomitable();
      pair.d20!.faces = [5, 8];
      const { button } = show(pair);
      button('Rolar de novo')!.click();
      expect(answers).toEqual([{ kind: 'indomitable', reroll: true, roll: { inApp: true } }]);
    });
  });

  describe('Lucky’s question', () => {
    it('asks "Tirou 1: rolar de novo?" with the way to roll again and "Ficar com o 1"', () => {
      const { el, text, button } = show(heldLucky(), { look: 'attack' });
      expect(el.querySelector('h3')?.textContent).toBe('Tirou 1: rolar de novo?');
      expect(text()).toContain(
        'Sortudo: você pode rolar esse d20 de novo e fica com o novo resultado.',
      );
      expect(button('Rolar de novo')).toBeTruthy();
      expect(button('Digitar o resultado')).toBeTruthy();
      expect(button('Ficar com o 1')).toBeTruthy();
    });

    it('names the die that showed the 1 when the roll has two', () => {
      const { text } = show(heldLucky([13, 1]));
      expect(text()).toContain('rolar de novo o dado que deu 1');
    });

    it('answers "Rolar de novo" with the d20 rolled in the app', () => {
      const { button } = show(heldLucky());
      button('Rolar de novo')!.click();
      expect(answers).toEqual([{ kind: 'lucky', reroll: true, roll: { inApp: true } }]);
    });

    it('answers "Ficar com o 1" with no roll', () => {
      const { button } = show(heldLucky());
      button('Ficar com o 1')!.click();
      expect(answers).toEqual([{ kind: 'lucky', reroll: false, roll: null }]);
    });

    it('answers with the typed face, 1 to 20, when the table rolls real dice', () => {
      const { fixture, el, text, button } = show(heldLucky(), { canApp: false });
      expect(button('Rolar de novo')).toBeUndefined();
      const field = el.querySelector<HTMLInputElement>('input')!;
      expect(text()).toContain('Role 1d20 de novo');
      field.value = '14';
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      button('Confirmar 14')!.click();
      expect(answers).toEqual([{ kind: 'lucky', reroll: true, roll: { face: 14 } }]);
    });

    it('shows the d20 over the question in the look of the sheet and the card, and nothing in the attack’s', () => {
      expect(show(heldLucky(), { look: 'attack' }).text()).not.toContain('Seu d20');
      expect(show(heldLucky(), { look: 'sheet' }).text()).toContain(
        'Seu d20: 1d20 (1) + 5 = 6. O resultado só aparece depois da sua escolha.',
      );
      const card = show(heldLucky([1, 13], 5), { look: 'card' });
      expect(card.text()).toContain('1 e 13');
      expect(card.text()).toContain(
        'Seu d20 (+5 de bônus). O resultado só aparece depois da sua escolha.',
      );
    });

    it('does not answer while an answer is on the way', () => {
      const { button } = show(heldLucky(), { busy: true });
      button('Ficar com o 1')!.click();
      expect(answers).toEqual([]);
    });
  });

  describe('the die’s question', () => {
    it('asks "Usar a Inspiração de Bardo (d8)?" with the die’s giver in the button', () => {
      const { el, button } = show(heldDie(11, 5, 8), { look: 'attack' });
      expect(el.querySelector('h3')?.textContent).toBe('Usar a Inspiração de Bardo (d8)?');
      expect(button('Somar o d8 (Inspiração de Orla)')).toBeTruthy();
      expect(button('Digitar o resultado')).toBeTruthy();
      expect(button('Guardar o dado')).toBeTruthy();
    });

    it('answers "Somar o d8" with the die rolled in the app, and "Guardar o dado" with no roll', () => {
      const { button } = show(heldDie(11, 5, 8), { look: 'attack' });
      button('Somar o d8')!.click();
      button('Guardar o dado')!.click();
      expect(answers).toEqual([
        { kind: 'die', use: true, roll: { inApp: true } },
        { kind: 'die', use: false, roll: null },
      ]);
    });

    it('takes the typed face of the die, 1 to its size', () => {
      const { fixture, el, text, button } = show(heldDie(11, 5, 6), {
        look: 'attack',
        canApp: false,
      });
      const field = el.querySelector<HTMLInputElement>('input')!;
      field.value = '7';
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      expect(text()).toContain('Digite um número de 1 a 6');
      field.value = '4';
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      button('Confirmar 4')!.click();
      expect(answers).toEqual([{ kind: 'die', use: true, roll: { face: 4 } }]);
    });

    it('in the card look shows the d20 that counts, who gave the die, and what Lucky did to the d20', () => {
      const { text } = show(heldDie(14, 5, 8, {}, 1), { look: 'card' });
      expect(text()).toContain('14');
      expect(text()).toContain(
        'Seu d20 (+5 de bônus). O resultado só aparece depois da sua escolha.',
      );
      expect(text()).toContain('Sortudo: 1 → 14');
      expect(text()).toContain('Dada por Orla.');
      expect(text()).toContain('Usar a Inspiração de Bardo (d8)?');
    });

    it('in the card look rolls in the app, or types the face (1 to the die’s size) and uses it', () => {
      const { fixture, el, button } = show(heldDie(11, 5, 6), { look: 'card' });
      button('Usar e rolar o d6')!.click();
      button('Digitar o dado')!.click();
      fixture.detectChanges();
      const field = el.querySelector<HTMLInputElement>('input[type="number"]')!;
      expect(field.max).toBe('6');
      expect(button('Usar o dado digitado')!.getAttribute('aria-disabled')).toBe('true');
      field.value = '7';
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      expect(button('Usar o dado digitado')!.getAttribute('aria-disabled')).toBe('true');
      field.value = '5';
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      button('Usar o dado digitado')!.click();
      expect(answers).toEqual([
        { kind: 'die', use: true, roll: { inApp: true } },
        { kind: 'die', use: true, roll: { face: 5 } },
      ]);
    });

    it('in the card look hides the way the table does not allow', () => {
      const typedOnly = show(heldDie(), { look: 'card', canApp: false });
      expect(typedOnly.button('Usar e rolar')).toBeUndefined();
      expect(typedOnly.button('Digitar o dado')).toBeTruthy();
      const appOnly = show(heldDie(), { look: 'card', canType: false });
      expect(appOnly.button('Usar e rolar')).toBeTruthy();
      expect(appOnly.button('Digitar o dado')).toBeUndefined();
    });
  });

  it('goes on to the die when Lucky’s answer brings it, and the typing left open does not carry over', () => {
    const { fixture, el, text, button } = show(heldLucky(), { look: 'card' });
    button('Digitar o resultado')!.click();
    fixture.detectChanges();
    expect(fixture.componentInstance.typing()).toBe(true);
    fixture.componentRef.setInput('offer', heldDie(14, 5, 8, {}, 1));
    fixture.detectChanges();
    expect(text()).toContain('Usar a Inspiração de Bardo (d8)?');
    expect(fixture.componentInstance.typing()).toBe(false);
    expect(el.querySelector('input')).toBeNull();
  });
});
