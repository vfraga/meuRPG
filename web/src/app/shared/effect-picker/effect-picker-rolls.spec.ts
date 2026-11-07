import { ComponentFixture, TestBed } from '@angular/core/testing';

import { rollsMenu } from '../../core/content/content-testing-menu-rolls';
import { type EffectDraft, draftToEffect, emptyEffect } from '../../core/content/effect-draft';
import { EffectPicker } from './effect-picker';

const BASE = 'table_feat.effects[0]';

const resource = (name: string, max = '3', recharge = 'long_rest'): EffectDraft => ({
  ...emptyEffect('resource'),
  resource: name,
  max,
  recharge,
});

describe('EffectPicker: reroll, roll minimum and the critical range', () => {
  const emitted: EffectDraft[] = [];
  let fixture: ComponentFixture<EffectPicker>;
  let el: HTMLElement;

  function setup(
    effect: EffectDraft,
    over: { siblings?: EffectDraft[]; issues?: Record<string, string[]> } = {},
  ) {
    emitted.length = 0;
    TestBed.resetTestingModule();
    fixture = TestBed.createComponent(EffectPicker);
    fixture.componentRef.setInput('effect', effect);
    fixture.componentRef.setInput('menu', rollsMenu());
    fixture.componentRef.setInput('basePath', BASE);
    fixture.componentRef.setInput('siblings', over.siblings ?? [effect]);
    fixture.componentRef.setInput('issuesOf', (p: string) => over.issues?.[p] ?? []);
    fixture.componentInstance.effectChange.subscribe((e) => emitted.push(e));
    fixture.detectChanges();
    el = fixture.nativeElement as HTMLElement;
  }

  const text = (e: Element) => (e.textContent ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ');
  const field = (name: string) => el.querySelector<HTMLElement>(`[data-field="${BASE}.${name}"]`);
  const row = (name: string) => field(name)!.closest('.frow')!;
  const boxes = () =>
    Array.from(field('rolls')!.querySelectorAll<HTMLInputElement>('input[type=checkbox]'));
  const labels = (select: HTMLSelectElement) =>
    Array.from(select.options).map((o) => o.text.trim());
  const boxText = (b: HTMLInputElement) =>
    text(b.closest('label')!.querySelector('.label')!).trim();
  const check = (label: string) => {
    const box = boxes().find((b) => boxText(b) === label)!;
    box.click();
    fixture.detectChanges();
    return emitted.at(-1)!;
  };
  const typeInto = (name: string, value: string) => {
    const input = field(name) as HTMLInputElement;
    input.value = value;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    return input;
  };
  const pick = (name: string, label: string) => {
    const select = field(name) as HTMLSelectElement;
    select.selectedIndex = labels(select).indexOf(label);
    select.dispatchEvent(new Event('change'));
    fixture.detectChanges();
    return emitted.at(-1)!;
  };

  describe('reroll', () => {
    it('is empty when just chosen: four unchecked rolls, "ou menos", "Nenhum" and no cost', () => {
      setup(emptyEffect('reroll'));
      expect(boxes().map(boxText)).toEqual([
        'Jogada de ataque (o d20)',
        'Teste de habilidade (o d20)',
        'Teste de resistência (o d20)',
        'Dado de dano (de arma ou magia)',
      ]);
      expect(boxes().every((b) => !b.checked)).toBe(true);
      expect(text(el)).toContain('No d20 o jogador responde "rolar de novo?" antes do resultado.');
      expect(text(field('at_most')!.closest('mat-form-field')!)).toContain('ou menos');
      expect(text(row('at_most'))).toContain('De 1 a 20.');
      expect(field('cost')).toBeNull();
      expect(row('rolls').classList).toContain('frow--wide');
    });

    it('checks rolls in the list order and writes them with the face, the resource and its cost', () => {
      setup(emptyEffect('reroll'), {
        siblings: [emptyEffect('reroll'), resource('sorte_teimosa')],
      });
      check('Teste de resistência (o d20)');
      expect(emitted.at(-1)!.rolls).toEqual(['save']);
      setup({ ...emptyEffect('reroll'), rolls: ['save'] });
      expect(check('Jogada de ataque (o d20)').rolls).toEqual(['attack', 'save']);
      const d = { ...emitted.at(-1)!, atMost: 2, resource: 'sorte_teimosa', cost: ' 1 ' };
      expect(draftToEffect(d, rollsMenu())).toEqual({
        type: 'reroll',
        rolls: ['attack', 'save'],
        atMost: 2,
        resource: 'sorte_teimosa',
        cost: '1',
      });
    });

    it('keeps what was typed in the face and writes the number', () => {
      setup(emptyEffect('reroll'));
      typeInto('at_most', '02');
      expect(emitted.at(-1)).toMatchObject({ atMost: 2, typed: { at_most: '02' } });
      expect((field('at_most') as HTMLInputElement).value).toBe('02');
      typeInto('at_most', 'x');
      expect(emitted.at(-1)!.atMost).toBeNaN();
    });

    it('lists the entry\'s own resources with their uses, and "Nenhum: vale sempre" first', () => {
      const me = { ...emptyEffect('reroll'), rolls: ['attack'], atMost: 2 };
      setup(me, { siblings: [me, resource('sorte_teimosa'), resource('fe', '1', 'short_rest')] });
      const select = field('resource') as HTMLSelectElement;
      expect(labels(select)).toEqual([
        'Nenhum: vale sempre',
        'Sorte Teimosa (3 por descanso longo)',
        'Fe (1 por descanso curto)',
      ]);
      expect(select.disabled).toBe(false);
      expect(text(row('resource'))).toContain('O recurso que esta entrada');
      expect(field('cost')).toBeNull();
      expect(pick('resource', 'Sorte Teimosa (3 por descanso longo)').resource).toBe(
        'sorte_teimosa',
      );
    });

    it('shows "Custo (usos)" only once a resource is picked, and drops the cost when it is taken away', () => {
      const me = { ...emptyEffect('reroll'), resource: 'sorte_teimosa', cost: '2' };
      setup(me, { siblings: [me, resource('sorte_teimosa')] });
      expect(text(row('cost'))).toContain('Quantos usos cada vez gasta; vazio é 1.');
      expect(text(row('cost').querySelector('mat-label')!)).toBe('Custo (usos)');
      pick('resource', 'Nenhum: vale sempre');
      expect(emitted.at(-1)).toMatchObject({ resource: '', cost: '' });
    });

    it("is a text with no lock when the entry defines no resource: the server also takes the class's and the SRD's", () => {
      setup(emptyEffect('reroll'));
      const input = field('resource') as HTMLInputElement;
      expect(input.tagName).toBe('INPUT');
      expect(input.disabled).toBe(false);
      expect(row('resource').querySelector('.frow__hint--lock')).toBeNull();
      expect(text(row('resource'))).not.toContain('Esta entrada não define recurso');
      typeInto('resource', 'recurso_da_classe');
      expect(emitted.at(-1)!.resource).toBe('recurso_da_classe');
    });

    it("keeps a resource the entry does not list as a text, flags nothing itself and words the server's refusal", () => {
      const me = { ...emptyEffect('reroll'), resource: ' perdido ', cost: '1' };
      setup(me, {
        siblings: [me, resource('sorte')],
        issues: {
          [`${BASE}.resource`]: ['Isto aponta para algo que não existe mais. Escolha de novo.'],
        },
      });
      expect((field('resource') as HTMLElement).tagName).toBe('INPUT');
      expect(text(row('resource'))).toContain(
        'Esta entrada não define mais este recurso. Escolha outro, ou deixe vazio.',
      );
      setup(me, { siblings: [me, resource('sorte')] });
      expect(row('resource').querySelector('.note--issue')).toBeNull();
    });

    it("says a server refusal of the rolls, the face, the resource and the cost in the board's words", () => {
      const me = { ...emptyEffect('reroll'), resource: 'sorte', cost: '0', atMost: 0 };
      setup(me, {
        siblings: [me, resource('sorte')],
        issues: {
          [`${BASE}.rolls`]: ['Marque ao menos uma jogada.'],
          [`${BASE}.at_most`]: ['A face vai de 1 a 20.'],
          [`${BASE}.cost`]: ['Este valor não serve para este efeito.'],
        },
      });
      fixture.detectChanges();
      expect(text(row('rolls'))).toContain('Marque ao menos uma jogada.');
      expect(field('rolls')!.querySelector('.legend--issue')).not.toBeNull();
      expect(text(row('at_most'))).toContain('A face vai de 1 a 20.');
      expect(row('at_most').querySelector('.note--issue')).not.toBeNull();
      expect(text(row('cost'))).toContain('O custo é um número de 1 em diante, e pede um recurso.');
      expect(text(row('cost'))).not.toContain('Este valor não serve');
    });
  });

  describe('roll_minimum', () => {
    it('asks "Valor mínimo" and hides the damage type until damage is the only roll', () => {
      setup(emptyEffect('roll_minimum'));
      expect(text(row('value'))).toContain('De 2 a 20.');
      expect(text(row('value').querySelector('mat-label')!)).toBe('Valor mínimo');
      expect(field('damage_type')).toBeNull();
      expect(text(el)).toContain('um d4 com mínimo 6 conta 4');
      check('Dado de dano (de arma ou magia)');
      setup({ ...emptyEffect('roll_minimum'), rolls: ['damage'], value: '2' });
      const select = field('damage_type') as HTMLSelectElement;
      expect(labels(select)).toEqual(['Qualquer tipo', 'Frio', 'Fogo']);
      expect(text(row('damage_type'))).toContain('Opcional, só com o dado de dano sozinho');
    });

    it('drops the damage type when another roll is checked, and writes the type only on damage alone', () => {
      setup({
        ...emptyEffect('roll_minimum'),
        rolls: ['damage'],
        value: '2',
        damageType: 'damage-type:fire',
      });
      expect(check('Jogada de ataque (o d20)')).toMatchObject({
        rolls: ['attack', 'damage'],
        damageType: '',
      });
      const d = {
        ...emptyEffect('roll_minimum'),
        rolls: ['damage'],
        value: ' 2 ',
        damageType: 'damage-type:fire',
      };
      expect(draftToEffect(d, rollsMenu())).toEqual({
        type: 'roll_minimum',
        rolls: ['damage'],
        value: '2',
        damageType: 'damage-type:fire',
      });
      // the same draft on two rolls (the field is hidden): the type is not sent either
      expect(
        draftToEffect({ ...d, rolls: ['attack', 'damage'], damageType: '' }, rollsMenu()),
      ).toEqual({
        type: 'roll_minimum',
        rolls: ['attack', 'damage'],
        value: '2',
      });
    });

    it('keeps a damage type that came with other rolls on screen, refused, so the refusal has somewhere to land', () => {
      setup(
        {
          ...emptyEffect('roll_minimum'),
          rolls: ['attack', 'damage'],
          value: '1',
          damageType: 'damage-type:fire',
        },
        {
          issues: {
            [`${BASE}.value`]: ['Este valor não serve para este efeito.'],
            [`${BASE}.damage_type`]: ['Este valor não serve para este efeito.'],
          },
        },
      );
      expect(text(row('value'))).toContain('O valor mínimo vai de 2 a 20.');
      expect(text(row('damage_type'))).toContain(
        'O tipo de dano só vale com o dado de dano sozinho. Desmarque as outras jogadas ou escolha "Qualquer tipo".',
      );
      expect(field('damage_type')!.getAttribute('aria-invalid')).toBe('true');
    });
  });

  describe('critical range', () => {
    const crit = (over: Partial<EffectDraft> = {}) => ({
      ...emptyEffect('modifier'),
      target: 'critical_range',
      mode: 'set',
      value: '19',
      applies: 'both',
      ...over,
    });

    it('locks "Como muda" on Definir with the note, relabels the value and asks "Vale para" as three cards', () => {
      setup(crit());
      expect(text(field('target')!.closest('.frow')!)).toContain(
        'O menor d20 natural que é acerto crítico.',
      );
      expect(labels(field('target') as HTMLSelectElement)).toContain('Margem de acerto crítico');
      const mode = field('mode') as HTMLSelectElement;
      expect(mode.disabled).toBe(true);
      expect(mode.value).toBe('set');
      expect(text(row('mode'))).toContain('A margem de crítico só aceita Definir.');
      const lock = row('mode').querySelector('.frow__hint--lock')!;
      expect(mode.getAttribute('aria-describedby')).toContain(lock.id);
      expect(text(row('value').querySelector('mat-label')!)).toBe('Crítico a partir do d20');
      expect(text(row('value'))).toContain('De 15 a 20, um número ou uma fórmula.');
      const cards = Array.from(
        el.querySelectorAll<HTMLInputElement>('app-radio-cards input[type=radio]'),
      );
      expect(cards.map((c) => c.checked)).toEqual([false, false, true]);
      expect(text(el.querySelector('app-radio-cards')!)).toContain('Vale para');
      expect(text(el.querySelector('app-radio-cards')!)).toContain('Armas e magias');
      expect(text(el)).toContain('Crítico Aprimorado e Crítico Superior nunca valem para magia.');
      expect(row('applies').classList).toContain('frow--wide');
    });

    it('writes the scope the card picks, and the first card is the default', () => {
      setup(crit({ applies: '' }));
      const cards = el.querySelectorAll<HTMLInputElement>('app-radio-cards input[type=radio]');
      expect(cards[0].checked).toBe(true);
      cards[1].dispatchEvent(new Event('change'));
      expect(emitted.at(-1)!.applies).toBe('spell');
      expect(draftToEffect(crit({ applies: 'spell' }), rollsMenu())).toEqual({
        type: 'modifier',
        target: 'critical_range',
        mode: 'set',
        value: '19',
        applies: 'spell',
      });
    });

    it('sets the mode when the target is chosen and drops the scope when the target is another', () => {
      setup({ ...emptyEffect('modifier'), target: 'ac', mode: 'add', value: '1' });
      expect(field('applies')).toBeNull();
      expect(el.querySelector('app-radio-cards')).toBeNull();
      expect((field('mode') as HTMLSelectElement).disabled).toBe(false);
      expect(pick('target', 'Margem de acerto crítico')).toMatchObject({
        target: 'critical_range',
        mode: 'set',
      });
      setup(crit());
      expect(pick('target', 'Classe de Armadura')).toMatchObject({ target: 'ac', applies: '' });
      // another target never writes the scope, even if the draft still holds one
      expect(
        draftToEffect({ ...crit(), target: 'ac', mode: 'add', applies: 'both' }, rollsMenu()),
      ).toEqual({ type: 'modifier', target: 'ac', mode: 'add', value: '19' });
    });

    it('leaves a stored mode that is not Definir free to fix, and says what the server refused', () => {
      setup(crit({ mode: 'add', value: '14' }), {
        issues: {
          [`${BASE}.mode`]: ['Este valor não serve para este efeito.'],
          [`${BASE}.value`]: ['Este valor não serve para este efeito.'],
        },
      });
      expect((field('mode') as HTMLSelectElement).disabled).toBe(false);
      expect(text(row('mode'))).toContain('A margem de crítico só aceita Definir.');
      expect(text(row('value'))).toContain('A margem vai de 15 a 20.');
      expect(row('mode').querySelector('.frow__hint--lock')).toBeNull();
      pick('mode', 'Definir');
      expect(emitted.at(-1)!.mode).toBe('set');
    });
  });
});
