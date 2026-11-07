import { create } from '@bufbuild/protobuf';
import { ComponentFixture, TestBed } from '@angular/core/testing';

import {
  EffectMenuListSchema,
  EffectMenuTypeSchema,
  EffectMenuValueSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { menuResponse } from '../../core/content/content-testing-menu';
import {
  type EffectDraft,
  EffectMenuVm,
  type FeatChoice,
  draftToEffect,
  emptyEffect,
  featChoices,
} from '../../core/content/effect-draft';
import { emptyFeature } from '../../core/content/feature-draft';
import { FeatureEditor } from '../feature-editor/feature-editor';
import { EffectPicker } from './effect-picker';
import { CHOICE_GROUP_NOTE, OPTION_NOTE, choiceSide } from './effect-picker-choices';

const BASE = 'table_feat.effects[2]';

/** The test menu with the kinds of a feat's choice, the targets of a score, a weapon rule and a resistance. */
function featMenu(): EffectMenuVm {
  const res = menuResponse();
  const list = (name: string) => res.lists.find((l) => l.name === name)!;
  list('choice_kinds').values.push(
    create(EffectMenuValueSchema, { key: 'ability', namePt: 'Uma habilidade (só em talento)' }),
    create(EffectMenuValueSchema, {
      key: 'damage_type',
      namePt: 'Um tipo de dano (só em talento)',
    }),
  );
  const field = (name: string, kind: string, list = '', min = 0, max = 0) => ({
    name,
    kind,
    list,
    min,
    max,
  });
  res.types.push(
    create(EffectMenuTypeSchema, {
      type: 'weapon_rule',
      namePt: 'Regra de arma',
      hintPt: 'Muda as armas.',
      fields: [
        field('attack_ability', 'choice', 'abilities'),
        field('damage_ability', 'choice', 'abilities'),
      ],
    }),
    create(EffectMenuTypeSchema, {
      type: 'resistance',
      namePt: 'Resistência',
      hintPt: 'Sofre menos dano.',
      fields: [
        field('damage_types', 'choices', 'damage_types'),
        field('except', 'choice', 'damage_types'),
      ],
    }),
    create(EffectMenuTypeSchema, {
      type: 'roll_mode',
      namePt: 'Vantagem ou desvantagem',
      hintPt: 'Em certas rolagens.',
      fields: [field('targets', 'choices', 'roll_targets')],
    }),
  );
  res.lists.push(
    create(EffectMenuListSchema, {
      name: 'damage_types',
      values: [
        { key: 'damage-type:psychic', namePt: 'Psíquico' },
        { key: 'damage-type:fire', namePt: 'Fogo' },
      ],
    }),
    create(EffectMenuListSchema, {
      name: 'roll_targets',
      values: [{ key: 'save.all', namePt: 'Todos os testes de resistência' }],
    }),
  );
  return new EffectMenuVm(res);
}

const named = (choice: string, name: string, from: string[] = []): EffectDraft => ({
  ...emptyEffect('choice'),
  choice,
  name,
  count: 1,
  from,
});

describe('EffectPicker: the named choices of a feat', () => {
  const emitted: EffectDraft[] = [];
  let fixture: ComponentFixture<EffectPicker>;
  let el: HTMLElement;

  /** The effect sits at index 2 of `effects`, after the choices the caller lists. */
  function setup(
    effect: EffectDraft,
    over: {
      effects?: EffectDraft[];
      choices?: readonly FeatChoice[] | null;
      index?: number;
      featOption?: boolean;
      featEditor?: boolean;
    } = {},
  ) {
    emitted.length = 0;
    const effects = over.effects ?? [
      named('ability', 'habilidade'),
      named('ability', 'habilidade_2'),
    ];
    const all = [...effects.slice(0, 2), effect, ...effects.slice(3)];
    TestBed.resetTestingModule();
    fixture = TestBed.createComponent(EffectPicker);
    fixture.componentRef.setInput('effect', effect);
    fixture.componentRef.setInput('menu', featMenu());
    fixture.componentRef.setInput('basePath', BASE);
    fixture.componentRef.setInput('siblings', all);
    fixture.componentRef.setInput('featEditor', over.featEditor ?? true);
    fixture.componentRef.setInput('effectIndex', over.index ?? 2);
    fixture.componentRef.setInput('featOption', over.featOption ?? false);
    fixture.componentRef.setInput(
      'choices',
      over.choices === undefined ? featChoices(all) : over.choices,
    );
    fixture.componentInstance.effectChange.subscribe((e) => emitted.push(e));
    fixture.detectChanges();
    el = fixture.nativeElement as HTMLElement;
    // The optional fields are folded under "Mais opções".
    el.querySelector<HTMLButtonElement>('button.more')?.click();
    fixture.detectChanges();
  }

  const text = (e: Element) => (e.textContent ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();
  const field = (path: string) => el.querySelector<HTMLElement>(`[data-field="${BASE}.${path}"]`);
  const groups = (select: HTMLElement) =>
    Array.from(select.querySelectorAll('optgroup')).map((g) => g.label);
  const entries = (select: HTMLElement) =>
    Array.from(select.querySelectorAll('optgroup')[0].querySelectorAll('option')).map((o) => ({
      value: o.value,
      label: text(o),
      side: o.dataset['side'] ?? '',
    }));
  const footer = (select: HTMLElement) =>
    select.querySelector('option.footer')?.textContent?.trim();
  const choose = (select: HTMLSelectElement, value: string) => {
    select.selectedIndex = Array.from(select.options).findIndex((o) => o.value === value);
    select.dispatchEvent(new Event('change'));
  };
  const click = (label: string) =>
    Array.from(el.querySelectorAll('button'))
      .find((b) => text(b).includes(label))!
      .click();

  describe('the group above the fixed values', () => {
    it('lists the earlier choices of the kind in the field own wording, first, with where each one is and a footer', () => {
      setup({ ...emptyEffect('modifier'), target: 'speed.walk' });
      const target = field('target') as HTMLSelectElement;
      expect(groups(target)).toEqual(['A escolha do talento', 'Valores fixos']);
      expect(entries(target)).toEqual([
        { value: 'score.@habilidade', label: 'Valor · Habilidade (1ª)', side: 'do efeito 1' },
        { value: 'score.@habilidade_2', label: 'Valor · Habilidade (2ª)', side: 'do efeito 2' },
        {
          value: 'save.@habilidade',
          label: 'Teste de resistência · Habilidade (1ª)',
          side: 'do efeito 1',
        },
        {
          value: 'save.@habilidade_2',
          label: 'Teste de resistência · Habilidade (2ª)',
          side: 'do efeito 2',
        },
      ]);
      expect(footer(target)).toBe(CHOICE_GROUP_NOTE);
      expect(footer(target)).toBe('A escolha do talento aparece nos efeitos que vêm depois dela.');
      const fixed = target.querySelectorAll('optgroup')[1];
      expect(fixed.querySelectorAll('option')).toHaveLength(2);
    });

    it('keeps the group above the fixed values, with the unset option before both', () => {
      setup({ ...emptyEffect('modifier'), target: '' });
      const target = field('target') as HTMLSelectElement;
      expect(Array.from(target.children).map((c) => c.tagName)).toEqual([
        'OPTION',
        'OPTGROUP',
        'OPTGROUP',
        'OPTION',
      ]);
    });

    it('writes the group header with its icon where the browser lets the list be drawn', () => {
      setup({ ...emptyEffect('modifier'), target: '' });
      const legend = (field('target') as HTMLElement).querySelector('optgroup legend')!;
      expect(text(legend)).toBe('alt_route A escolha do talento');
    });

    it('offers a field only the choices of its own kind', () => {
      const effects = [
        named('damage_type', 'tipo_de_dano', ['damage-type:psychic']),
        named('ability', 'habilidade'),
      ];
      setup({ ...emptyEffect('resistance'), kind: 'resist' }, { effects });
      const except = field('except') as HTMLSelectElement;
      expect(entries(except)).toEqual([
        {
          value: 'damage-type:@tipo_de_dano',
          label: 'Escolha · Tipo de dano',
          side: 'do efeito 1 · psíquico',
        },
      ]);
      const ability = featChoices(effects).filter((c) => c.kind === 'ability');
      setup({ ...emptyEffect('weapon_rule'), type: 'weapon_rule' }, { effects });
      expect(entries(field('attack_ability') as HTMLElement).map((e) => e.value)).toEqual([
        '@habilidade',
      ]);
      expect(ability).toHaveLength(1);
    });

    it('offers only the choices that come before the effect', () => {
      const effects = [
        named('ability', 'habilidade'),
        named('ability', 'depois'),
        { ...emptyEffect('modifier'), target: '' },
      ];
      setup(effects[2], { effects, index: 1 });
      const target = field('target') as HTMLSelectElement;
      expect(entries(target).map((e) => e.value)).toEqual([
        'score.@habilidade',
        'save.@habilidade',
      ]);
      setup(effects[2], { effects, index: 0 });
      const values = Array.from((field('target') as HTMLSelectElement).options).map((o) => o.value);
      expect(values).not.toContain('score.@habilidade');
    });

    it('locks the group when the only choice of the kind comes below, and says where it is', () => {
      const mod = { ...emptyEffect('modifier'), target: '' };
      const effects = [mod, named('ability', 'depois'), mod, named('ability', 'ainda_depois')];
      setup(mod, { effects, index: 0 });
      const target = field('target') as HTMLSelectElement;
      expect(groups(target)).toEqual(['A escolha do talento', 'Valores fixos']);
      const row = target.querySelector<HTMLOptionElement>('optgroup option')!;
      expect(row.disabled).toBe(true);
      expect(text(row)).toBe(
        '“Habilidade (1ª)” está no efeito 2, abaixo deste. Suba a escolha para usá-la aqui.',
      );
      expect(target.querySelector('optgroup.accent')).toBeNull();
      target.selectedIndex = Array.from(target.options).indexOf(row);
      target.dispatchEvent(new Event('change'));
      expect(emitted).toEqual([]);
    });

    it('lets the master list the damage types a choice of a damage type offers, in sentence case', () => {
      setup({ ...named('damage_type', 'tipo'), from: ['damage-type:fire'] }, { index: 2 });
      const list = field('from')!.closest('app-pick-list')!;
      expect(text(list)).toContain('Fogo');
      const add = Array.from(list.querySelectorAll('option')).map((o) => o.value);
      expect(add).toEqual(['', 'damage-type:psychic']);
    });

    it('shows "Escolha…" and the refusal for a field whose choice is gone, and keeps its text', () => {
      const gone = { ...emptyEffect('modifier'), target: 'score.@sumiu' };
      setup(gone);
      const target = field('target') as HTMLSelectElement;
      expect(target.selectedOptions[0].text).toBe('Escolha…');
      expect(target.getAttribute('aria-invalid')).toBe('true');
      expect(text(target.closest('app-select-field')!)).toContain(
        'A escolha “sumiu” que este campo usava foi removida. Escolha outro valor.',
      );
      expect(emitted).toEqual([]);
    });

    it('says the same of a list that holds a pick whose choice is gone, once', () => {
      const gone = { ...emptyEffect('resistance'), damageTypes: ['damage-type:@sumiu'] };
      setup(gone);
      const list = field('damage_types')!;
      expect(text(list.closest('app-pick-list')!).split('foi removida')).toHaveLength(2);
    });

    it('names the choice a list pick lost, on its chip and on the button that takes it off', () => {
      const cases: [EffectDraft, string][] = [
        [{ ...emptyEffect('resistance'), damageTypes: ['damage-type:@sumiu'] }, 'damage_types'],
        [{ ...emptyEffect('roll_mode'), targets: ['check.@sumiu'] }, 'targets'],
      ];
      for (const [gone, key] of cases) {
        setup(gone);
        const list = field(key)!.closest('app-pick-list')!;
        const chip = list.querySelector('.chip')!;
        expect(text(chip)).toContain('Escolha removida (sumiu)');
        expect(text(chip)).not.toContain('@');
        expect(chip.querySelector('button')!.getAttribute('aria-label')).toBe(
          'Tirar a escolha removida sumiu',
        );
        expect(text(list)).toContain('A escolha “sumiu” que este campo usava foi removida.');
      }
    });

    it('says a list pick sits below its choice, naming it, when the choice exists after the effect', () => {
      const mod = { ...emptyEffect('resistance'), damageTypes: ['damage-type:@tipo'] };
      const effects = [mod, named('damage_type', 'tipo')];
      setup(mod, { effects, index: 0 });
      const chip = field('damage_types')!.closest('app-pick-list')!.querySelector('.chip')!;
      expect(text(chip)).toContain('Escolha abaixo (tipo)');
      expect(chip.querySelector('button')!.getAttribute('aria-label')).toBe(
        'Tirar a escolha abaixo tipo',
      );
    });

    it("writes its own one-line hint for the effect type in the feat editor, and the menu's elsewhere", () => {
      const resistance = emptyEffect('resistance');
      setup(resistance);
      const note = () => text(field('type')!.closest('app-select-field')!.querySelector('.note')!);
      expect(note()).toBe('Resistência ou imunidade a dano.');
      setup(resistance, { featEditor: false });
      expect(note()).toBe('Sofre menos dano.');
    });

    it('keeps the hint under a formula to one line in the feat editor', () => {
      setup({ ...emptyEffect('modifier'), target: 'ac', mode: 'add', value: '8' });
      expect(text(field('value')!.closest('.frow')!)).toContain(
        'Um número ou uma fórmula, como 8 + prof().',
      );
      expect(text(field('value')!.closest('.frow')!)).not.toContain('Funções:');
    });

    it('says nothing of a field whose pick reads, or of a feature that has no choices', () => {
      const ok = { ...emptyEffect('modifier'), target: 'score.@habilidade' };
      setup(ok);
      expect(text(el)).not.toContain('foi removida');
      setup({ ...ok, target: 'score.@sumiu' }, { choices: null });
      expect(text(el)).not.toContain('foi removida');
    });

    it('is absent without the choices input, and the list is the fixed values as before', () => {
      setup({ ...emptyEffect('modifier'), target: '' }, { choices: null });
      const target = field('target') as HTMLSelectElement;
      expect(groups(target)).toEqual([]);
      expect(target.querySelector('option.footer')).toBeNull();
      expect(Array.from(target.options).map((o) => o.value)).toEqual(['', 'speed.walk', 'ac']);
    });

    it('is absent when the feat has no choice before the effect', () => {
      setup({ ...emptyEffect('modifier'), target: '' }, { choices: [] });
      expect(groups(field('target') as HTMLElement)).toEqual([]);
    });

    it('is absent in an option of the feat, which says its fields take fixed values only', () => {
      setup({ ...emptyEffect('modifier'), target: '' }, { featOption: true });
      const target = field('target') as HTMLSelectElement;
      expect(groups(target)).toEqual([]);
      expect(footer(target)).toBe(OPTION_NOTE);
      expect(footer(target)).toBe(
        'As opções do talento não usam as escolhas dele: só valores fixos.',
      );
    });

    it('offers the choices in the list of a field that takes several, as a group above the fixed values', () => {
      setup({ ...emptyEffect('roll_mode'), roll: 'advantage' });
      const add = el.querySelector<HTMLSelectElement>('app-pick-list select')!;
      expect(Array.from(add.querySelectorAll('optgroup')).map((g) => g.label)).toEqual([
        'A escolha do talento',
        'Valores fixos',
      ]);
      expect(
        Array.from(add.querySelectorAll('optgroup')[0].querySelectorAll('option')).map(
          (o) => o.value,
        ),
      ).toEqual([
        'check.@habilidade',
        'check.@habilidade_2',
        'save.@habilidade',
        'save.@habilidade_2',
      ]);
    });
  });

  describe('picking a choice', () => {
    it('stores @name in an ability field', () => {
      setup({ ...emptyEffect('weapon_rule'), type: 'weapon_rule' });
      choose(field('attack_ability') as HTMLSelectElement, '@habilidade_2');
      expect(emitted.at(-1)?.attackAbility).toBe('@habilidade_2');
    });

    it('stores the form of the field, with the name, in a proficiency and a damage type', () => {
      const effects = [named('ability', 'habilidade'), named('damage_type', 'tipo')];
      setup({ ...emptyEffect('proficiency'), proficiency: '' }, { effects });
      choose(field('proficiency') as HTMLSelectElement, 'save.@habilidade');
      expect(emitted.at(-1)?.proficiency).toBe('save.@habilidade');
      setup({ ...emptyEffect('resistance'), kind: 'resist' }, { effects });
      choose(field('except') as HTMLSelectElement, 'damage-type:@tipo');
      expect(emitted.at(-1)?.except).toBe('damage-type:@tipo');
    });

    it('adds the choice to a list of values, and the chip is named like the entry', () => {
      setup({ ...emptyEffect('roll_mode'), roll: 'advantage', targets: ['save.@habilidade'] });
      expect(text(field('targets')!)).toContain('Teste de resistência · Habilidade (1ª)');
      const add = el.querySelector<HTMLSelectElement>('app-pick-list select')!;
      choose(add, 'check.@habilidade_2');
      expect(emitted.at(-1)?.targets).toEqual(['save.@habilidade', 'check.@habilidade_2']);
    });

    it('says what the pick is beside the field', () => {
      setup({ ...emptyEffect('proficiency'), proficiency: 'save.@habilidade' });
      expect(text(el)).toContain(
        'Proficiência na resistência da habilidade escolhida no efeito 1.',
      );
    });
  });

  describe('after the pick of an ability score', () => {
    const score = (over: Partial<EffectDraft> = {}): EffectDraft => ({
      ...emptyEffect('modifier'),
      target: 'score.@habilidade',
      mode: 'add',
      value: '1',
      cap: 20,
      ...over,
    });

    it('locks "Como muda" on Somar with its lock and reason', () => {
      setup(score());
      const mode = field('mode') as HTMLSelectElement;
      expect(mode.disabled).toBe(true);
      expect(text(mode.closest('.frow')!)).toContain('Uma habilidade só soma.');
      expect(mode.closest('.frow')!.querySelector('.frow__hint--lock mat-icon')).not.toBeNull();
    });

    it('shows the limit, from 20 to 30', () => {
      setup(score());
      const cap = field('cap') as HTMLInputElement;
      expect(cap.value).toBe('20');
      expect(text(cap.closest('.frow')!)).toContain('Limite');
    });

    it('sets Somar and the limit when the pick is made, and takes the limit off with another target', () => {
      setup({ ...emptyEffect('modifier'), target: 'speed.walk' });
      expect(field('cap')).toBeNull();
      choose(field('target') as HTMLSelectElement, 'score.@habilidade');
      expect(emitted.at(-1)).toMatchObject({ target: 'score.@habilidade', mode: 'add', cap: 20 });
      setup(score({ cap: 25 }));
      choose(field('target') as HTMLSelectElement, 'speed.walk');
      expect(emitted.at(-1)).toMatchObject({ target: 'speed.walk', cap: 0 });
    });

    it('keeps the limit the master typed when the target stays a score, and sends it with the effect', () => {
      setup(score({ cap: 24 }));
      choose(field('target') as HTMLSelectElement, 'save.@habilidade');
      expect(emitted.at(-1)?.cap).toBe(0);
      const sent = draftToEffect(score({ cap: 24 }), featMenu());
      expect(sent).toMatchObject({ target: 'score.@habilidade', mode: 'add', cap: 24 });
    });
  });

  describe('the choice effect', () => {
    it('shows "Quantos" locked at 1 with its reason, and the line the other effects use', () => {
      setup(named('ability', 'habilidade'), {
        effects: [],
        index: 0,
        choices: featChoices([named('ability', 'habilidade')]),
      });
      const count = field('count') as HTMLInputElement;
      expect(count.value).toBe('1');
      expect(count.disabled).toBe(true);
      expect(text(count.closest('.frow')!)).toContain(
        'Uma por escolha. Para duas, adicione outra escolha.',
      );
      expect(text(el.querySelector('.choice-banner span')!)).toBe(
        'Os outros efeitos chamam esta escolha de Habilidade; o jogador a escolhe ao pegar o talento.',
      );
    });

    it('says a damage type the same way, in the masculine', () => {
      const choice = named('damage_type', 'tipo_de_dano');
      setup(choice, { effects: [], index: 0, choices: featChoices([choice]) });
      expect(text(el.querySelector('.choice-banner')!)).toContain(
        'Tipo de dano; o jogador o escolhe',
      );
      expect(text((field('count') as HTMLElement).closest('.frow')!)).toContain(
        'Um tipo por escolha.',
      );
    });

    it('leaves "Quantos" free for a choice with no name', () => {
      setup(
        { ...emptyEffect('choice'), choice: 'skill', count: 2 },
        { effects: [], index: 0, choices: [] },
      );
      const count = field('count') as HTMLInputElement;
      expect(count.disabled).toBe(false);
      expect(el.querySelector('.choice-banner')).toBeNull();
    });

    it('names the choice by itself when the kind is an ability or a damage type, and not otherwise', () => {
      setup({ ...emptyEffect('choice'), choice: '' }, { effects: [], index: 0, choices: [] });
      const kind = field('choice') as HTMLSelectElement;
      expect(
        Array.from(kind.options)
          .find((o) => o.value === 'ability')!
          .textContent!.trim(),
      ).toBe('Uma habilidade (só em talento)');
      choose(kind, 'ability');
      expect(emitted.at(-1)).toMatchObject({ choice: 'ability', name: 'habilidade' });
      choose(kind, 'skill');
      expect(emitted.at(-1)).toMatchObject({ choice: 'skill', name: '' });
    });

    it('does not repeat a name the feat already uses', () => {
      const effects = [named('ability', 'habilidade')];
      setup({ ...emptyEffect('choice'), choice: '' }, { effects, index: 1, choices: [] });
      const own = { ...emptyEffect('choice'), choice: '' };
      fixture.componentRef.setInput('siblings', [effects[0], own]);
      fixture.componentRef.setInput('effectIndex', 1);
      fixture.detectChanges();
      choose(field('choice') as HTMLSelectElement, 'ability');
      expect(emitted.at(-1)?.name).toBe('habilidade_2');
    });

    it('does not ask the name of the choice', () => {
      setup(named('ability', 'habilidade'), {
        effects: [],
        index: 0,
        choices: featChoices([named('ability', 'habilidade')]),
      });
      expect(field('name')).toBeNull();
    });
  });

  describe('a formula that reads a choice', () => {
    const note = (value: string, textPt = ''): EffectDraft => ({
      ...emptyEffect('note'),
      value,
      textPt,
    });

    it('has the buttons of the abilities before it, with the note under them', () => {
      setup(note(''));
      const labels = Array.from(el.querySelectorAll('.insert button')).map((b) => text(b));
      expect(labels).toEqual([
        'alt_routeModificador · Habilidade (1ª)',
        'alt_routeValor · Habilidade (1ª)',
        'alt_routeModificador · Habilidade (2ª)',
        'alt_routeValor · Habilidade (2ª)',
      ]);
      expect(text(el.querySelector('.insert')!)).toContain('Pôr na fórmula, onde está o cursor');
      expect(text(el.querySelector('.insert__note')!)).toBe(
        'O botão escreve a escolha por você; o mestre não digita o nome dela. Só as escolhas de habilidade entram numa fórmula.',
      );
    });

    it('has no buttons for a damage type, without the input or in an option of the feat', () => {
      const effects = [named('damage_type', 'tipo')];
      setup(note(''), { effects });
      expect(el.querySelector('.insert')).toBeNull();
      setup(note(''), { choices: null });
      expect(el.querySelector('.insert')).toBeNull();
      setup(note(''), { featOption: true });
      expect(el.querySelector('.insert')).toBeNull();
    });

    it('writes mod("@name") where the cursor is', () => {
      setup(note('8 + prof() + '));
      const input = field('value') as HTMLInputElement;
      input.setSelectionRange(4, 4);
      click('Modificador · Habilidade (1ª)');
      expect(emitted.at(-1)?.value).toBe('8 + mod("@habilidade") + prof() + ');
    });

    it('puts a " + " between a formula that ends in a call and the next one', () => {
      setup(note('8 + prof() + mod("@habilidade")'));
      click('Valor · Habilidade (1ª)');
      expect(emitted.at(-1)?.value).toBe('8 + prof() + mod("@habilidade") + score("@habilidade")');
    });

    it('replaces the selection and writes score("@name")', () => {
      setup(note('8 + XX'));
      const input = field('value') as HTMLInputElement;
      input.setSelectionRange(4, 6);
      click('Valor · Habilidade (2ª)');
      expect(emitted.at(-1)?.value).toBe('8 + score("@habilidade_2")');
    });

    it('writes at the end when the field was never focused', () => {
      setup(note('1 + '));
      click('Modificador · Habilidade (1ª)');
      expect(emitted.at(-1)?.value).toBe('1 + mod("@habilidade")');
    });

    it('reads the formula out under the field', () => {
      setup(note('8 + prof() + mod("@habilidade")'));
      expect(text(el.querySelector('.reading')!)).toBe(
        'Lê-se: 8 + bônus de proficiência + modificador da escolha Habilidade (1ª).',
      );
      expect(text(el.querySelector('.reading__choice')!)).toBe(
        'modificador da escolha Habilidade (1ª)',
      );
    });

    it('reads nothing for a formula that points at no choice', () => {
      setup(note('8 + prof()'));
      expect(el.querySelector('.reading')).toBeNull();
    });

    it('says where the number goes in the text of the sheet', () => {
      setup(note('8 + mod("@habilidade")', 'CD da Voz de Comando: {value}'));
      const readings = Array.from(el.querySelectorAll('.reading')).map((r) => text(r));
      expect(readings).toContain('Lê-se: {value} é onde o número da CD entra.');
    });

    it('says nothing of the slot when the text has no place for it', () => {
      setup(note('8 + mod("@habilidade")', 'CD da Voz de Comando'));
      expect(Array.from(el.querySelectorAll('.reading')).map((r) => text(r))).toHaveLength(1);
    });
  });
});

describe('choiceSide', () => {
  const choice = (kind: FeatChoice['kind']): FeatChoice => ({
    kind,
    name: 'x',
    number: 1,
    ofKind: 1,
    label: 'x',
    index: 0,
  });

  it('says the effect, and for a damage type its list', () => {
    expect(choiceSide(choice('ability'), [])).toBe('do efeito 1');
    expect(choiceSide(choice('damage_type'), [])).toBe('do efeito 1');
    expect(choiceSide(choice('damage_type'), ['ácido'])).toBe('do efeito 1 · ácido');
    expect(
      choiceSide(choice('damage_type'), ['ácido', 'elétrico', 'fogo', 'frio', 'trovejante']),
    ).toBe('do efeito 1 · ácido, elétrico, fogo, frio ou trovejante');
  });
});

describe('FeatureEditor with the choices of a feat', () => {
  let el: HTMLElement;

  function setup(over: { choices?: readonly FeatChoice[] | null; featOption?: boolean }) {
    TestBed.resetTestingModule();
    const effects = [named('ability', 'habilidade'), { ...emptyEffect('modifier'), target: '' }];
    const f = TestBed.createComponent(FeatureEditor);
    f.componentRef.setInput('feature', { ...emptyFeature(), name: 'Resiliente', effects });
    f.componentRef.setInput('menu', featMenu());
    f.componentRef.setInput('basePath', 'table_feat');
    f.componentRef.setInput('featEditor', true);
    f.componentRef.setInput(
      'choices',
      over.choices === undefined ? featChoices(effects) : over.choices,
    );
    f.componentRef.setInput('featOption', over.featOption ?? false);
    f.detectChanges();
    el = f.nativeElement as HTMLElement;
  }

  const target = () =>
    el.querySelector<HTMLSelectElement>('[data-field="table_feat.effects[1].target"]')!;

  it('offers the earlier choice to the second effect and not to the first', () => {
    setup({});
    expect(Array.from(target().querySelectorAll('optgroup')).map((g) => g.label)).toEqual([
      'A escolha do talento',
      'Valores fixos',
    ]);
    expect(el.querySelector('[data-field="table_feat.effects[0].choice"] optgroup')).toBeNull();
  });

  it('offers nothing without the choices', () => {
    setup({ choices: null });
    expect(target().querySelector('optgroup')).toBeNull();
    expect(target().querySelector('option.footer')).toBeNull();
  });

  it('says in an option of the feat that only fixed values are offered', () => {
    setup({ choices: null, featOption: true });
    expect(target().querySelector('option.footer')?.textContent?.trim()).toBe(
      'As opções do talento não usam as escolhas dele: só valores fixos.',
    );
  });
});
