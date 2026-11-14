import { ComponentFixture, TestBed } from '@angular/core/testing';

import { create } from '@bufbuild/protobuf';

import { TableEffectSchema } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { powerAttackMenu } from '../../core/content/content-testing-menu-power';
import { BAD_EFFECT_VALUE, DANGLING_TEXT } from '../../core/content/content-violation-texts';
import {
  type EffectDraft,
  draftToEffect,
  effectToDraft,
  emptyEffect,
  unreadableNumberPaths,
} from '../../core/content/effect-draft';
import { EffectPicker } from './effect-picker';

const BASE = 'table_feat.effects[0]';
const HEAVY = 'weapon-property:heavy';

const resource = (name = 'folego_do_atirador'): EffectDraft => ({
  ...emptyEffect('resource'),
  resource: name,
  max: '3',
  recharge: 'short_rest',
});

const trade = (over: Partial<EffectDraft> = {}): EffectDraft => ({
  ...emptyEffect('power_attack'),
  attackPenalty: 5,
  damageBonus: 10,
  ...over,
});

describe('EffectPicker: the power_attack block ("Ataque poderoso")', () => {
  const emitted: EffectDraft[] = [];
  let fixture: ComponentFixture<EffectPicker>;
  let el: HTMLElement;

  function setup(
    effect: EffectDraft,
    over: {
      siblings?: EffectDraft[];
      issues?: Record<string, string[]>;
      featEditor?: boolean;
    } = {},
  ) {
    emitted.length = 0;
    TestBed.resetTestingModule();
    fixture = TestBed.createComponent(EffectPicker);
    fixture.componentRef.setInput('effect', effect);
    fixture.componentRef.setInput('menu', powerAttackMenu());
    fixture.componentRef.setInput('basePath', BASE);
    fixture.componentRef.setInput('featEditor', over.featEditor ?? true);
    fixture.componentRef.setInput('siblings', over.siblings ?? [effect]);
    fixture.componentRef.setInput('issuesOf', (p: string) => over.issues?.[p] ?? []);
    fixture.componentInstance.effectChange.subscribe((e) => emitted.push(e));
    fixture.detectChanges();
    el = fixture.nativeElement as HTMLElement;
  }

  const text = (e: Element) => (e.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const field = (name: string) => el.querySelector<HTMLElement>(`[data-field="${BASE}.${name}"]`);
  const typeInto = (name: string, value: string) => {
    const input = field(name) as HTMLInputElement;
    input.value = value;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    return emitted.at(-1)!;
  };
  const cards = () =>
    Array.from(el.querySelectorAll<HTMLInputElement>('[role=radiogroup] input[type=radio]'));
  const cardTitles = () =>
    cards().map((c) => text(c.closest('label')!.querySelector('.title')!).trim());
  const reading = () => el.querySelector('[data-power-reading]');
  const readingText = () => text(reading()!.querySelector('span')!).trim();
  /** The same effect, changed after the first render, as it is when someone types: Material's own `aria-invalid` binding runs last on a first render. */
  const rerender = (effect: EffectDraft) => {
    fixture.componentRef.setInput('effect', effect);
    fixture.detectChanges();
  };
  const more = () =>
    Array.from(el.querySelectorAll('button')).find((b) => text(b).includes('opções')) as
      HTMLButtonElement | undefined;
  const chips = () =>
    Array.from(el.querySelectorAll<HTMLElement>('ul.chips li')).map((c) =>
      text(c)
        .replace(/ close$/, '')
        .trim(),
    );

  it('is chosen from the "Efeito" menu: a new effect of the type starts empty', () => {
    setup(emptyEffect('proficiency'));
    const type = field('type') as HTMLSelectElement;
    const labels = Array.from(type.options).map((o) => o.text.trim());
    expect(labels.filter((l) => l === 'Ataque poderoso')).toHaveLength(1);
    type.selectedIndex = labels.indexOf('Ataque poderoso');
    type.dispatchEvent(new Event('change'));
    expect(emitted.at(-1)).toMatchObject({
      type: 'power_attack',
      attackPenalty: 0,
      damageBonus: 0,
      weapon: '',
      weaponProperties: [],
      onlyProficient: false,
    });
  });

  it("says the short hint under the type in the feat editor, and the menu's own elsewhere", () => {
    setup(trade());
    expect(text(el)).toContain('Troca acerto por dano num ataque com arma.');
    setup(trade(), { featEditor: false });
    expect(text(el)).toContain('Uma troca que o jogador faz ao atacar com arma');
  });

  it('draws the block: the two numbers with their ranges, the weapon cards, the properties search and the proficiency switch', () => {
    setup(trade());
    expect(text(field('attack_penalty')!.closest('app-text-field')!)).toContain('Tira do ataque');
    expect(text(field('damage_bonus')!.closest('app-text-field')!)).toContain('Soma no dano');
    expect((field('attack_penalty') as HTMLInputElement).value).toBe('5');
    expect((field('damage_bonus') as HTMLInputElement).value).toBe('10');
    const t = text(el);
    expect(t).toContain('De 1 a 10: sai da jogada de ataque.');
    expect(t).toContain('De 1 a 20, só se acertar; o crítico não dobra.');
    expect(t).toContain('Com que armas');
    expect(cardTitles()).toEqual(['Qualquer arma', 'Só corpo a corpo', 'Só à distância']);
    expect(text(field('weapon_properties')!.closest('app-search-pick')!)).toContain(
      'A arma precisa ter',
    );
    expect(t).toContain('Até 4; a arma precisa ter todas. Vazio: qualquer arma do tipo escolhido.');
    expect(t).toContain('Só armas em que o personagem tem proficiência');
    expect((field('only_proficient') as HTMLInputElement).type).toBe('checkbox');
  });

  it('draws the type\'s own fields once, not again as generic ones: no "Arma", no "Bônus no dano"', () => {
    setup(trade());
    for (const name of [
      'attack_penalty',
      'damage_bonus',
      'weapon',
      'weapon_properties',
      'only_proficient',
    ]) {
      expect(el.querySelectorAll(`[data-field="${BASE}.${name}"]`)).toHaveLength(1);
    }
    expect(text(el)).not.toContain('Bônus no dano');
    expect(text(el)).not.toContain('Penalidade no ataque');
  });

  it('writes a number as typed: "+5" stays in the field, the draft holds 5', () => {
    setup(trade({ attackPenalty: 0 }));
    const input = field('attack_penalty') as HTMLInputElement;
    input.value = '+5';
    input.dispatchEvent(new Event('input'));
    expect(emitted.at(-1)).toMatchObject({ attackPenalty: 5, typed: { attack_penalty: '+5' } });
    typeInto('damage_bonus', '12');
    expect(emitted.at(-1)?.damageBonus).toBe(12);
  });

  describe('Com que armas', () => {
    it('checks "Qualquer arma" for an empty weapon and for "any"', () => {
      for (const weapon of ['', 'any']) {
        setup(trade({ weapon }));
        expect(cards().map((c) => c.checked)).toEqual([true, false, false]);
      }
    });

    it('checks the card of the stored kind and writes the picked one', () => {
      setup(trade({ weapon: 'ranged' }));
      expect(cards().map((c) => c.checked)).toEqual([false, false, true]);
      cards()[1].click();
      expect(emitted.at(-1)?.weapon).toBe('melee');
      cards()[0].click();
      expect(emitted.at(-1)?.weapon).toBe('');
    });

    it('says what each card means, so the master picks without reading the rules', () => {
      setup(trade());
      const t = text(el);
      expect(t).toContain('Corpo a corpo ou à distância');
      expect(t).toContain('Espada, machado, malho');
      expect(t).toContain('Arco, besta, arma arremessada');
    });

    it('is a radio group named "Com que armas", the first card carrying the field\'s path', () => {
      setup(trade());
      const group = el.querySelector('[role=radiogroup]')!;
      expect(group.getAttribute('aria-label')).toBe('Com que armas');
      expect(cards()[0].getAttribute('data-field')).toBe(`${BASE}.weapon`);
    });

    it('lands a refusal of the weapon on the cards, in words', () => {
      setup(trade(), { issues: { [`${BASE}.weapon`]: [BAD_EFFECT_VALUE] } });
      expect(text(el.querySelector('[role=radiogroup]')!.parentElement!)).toContain(
        'Escolha uma das três opções.',
      );
    });
  });

  describe('A arma precisa ter', () => {
    const search = () => field('weapon_properties') as HTMLInputElement;
    const open = (query = '') => {
      search().value = query;
      search().dispatchEvent(new Event('input'));
      fixture.detectChanges();
      return Array.from(el.querySelectorAll<HTMLElement>('[role=option]')).map((o) =>
        text(o).trim(),
      );
    };

    it('offers the properties of the menu by name, searchable, without the ones already picked', () => {
      setup(trade({ weaponProperties: [HEAVY] }));
      const all = open();
      expect(all).toContain('Leve');
      expect(all).toContain('Duas mãos');
      expect(all).not.toContain('Pesada');
      expect(open('mão')).toEqual(['Duas mãos']);
      expect(open('PESADA')).toEqual([]);
    });

    it('picks one as a chip, in the order picked, and writes its key', () => {
      setup(trade());
      open('pesa');
      (el.querySelector('[role=option]') as HTMLElement).click();
      expect(emitted.at(-1)?.weaponProperties).toEqual([HEAVY]);
      setup(trade({ weaponProperties: [HEAVY] }));
      expect(chips()).toEqual(['Pesada']);
    });

    it('takes a chip off with a button that names it, and keeps the others', () => {
      setup(trade({ weaponProperties: [HEAVY, 'weapon-property:light'] }));
      const x = el.querySelector<HTMLButtonElement>('button[aria-label="Tirar Pesada"]')!;
      x.click();
      expect(emitted.at(-1)?.weaponProperties).toEqual(['weapon-property:light']);
    });

    it('marks the fifth chip and says "Até 4 propriedades.", as it stays in the draft for the master to remove', () => {
      setup(trade({ weaponProperties: [HEAVY] }));
      rerender(
        trade({
          weaponProperties: [
            HEAVY,
            'weapon-property:light',
            'weapon-property:reach',
            'weapon-property:finesse',
            'weapon-property:versatile',
          ],
        }),
      );
      const marked = Array.from(el.querySelectorAll('ul.chips li')).map((c) =>
        c.classList.contains('chip--invalid'),
      );
      expect(marked).toEqual([false, false, false, false, true]);
      expect(text(el)).toContain('Até 4 propriedades.');
      expect(search().getAttribute('aria-invalid')).toBe('true');
    });

    it('marks a chip whose property the menu no longer has, and keeps its key as the name', () => {
      setup(trade({ weaponProperties: ['weapon-property:gone'] }));
      expect(chips()).toEqual(['weapon-property:gone']);
      expect(el.querySelector('ul.chips li')!.classList.contains('chip--invalid')).toBe(true);
    });

    it('turns a refusal of the server into the same words, whether too many or unknown', () => {
      for (const server of [BAD_EFFECT_VALUE, DANGLING_TEXT]) {
        setup(trade({ weaponProperties: [HEAVY] }), {
          issues: { [`${BASE}.weapon_properties`]: [server] },
        });
        expect(text(el)).toContain('Até 4 propriedades, cada uma uma vez e da lista.');
        expect(text(el)).not.toContain(server);
      }
    });
  });

  describe('Só armas em que o personagem tem proficiência', () => {
    it('is a checkbox that writes the switch', () => {
      setup(trade());
      const box = field('only_proficient') as HTMLInputElement;
      expect(box.checked).toBe(false);
      box.click();
      expect(emitted.at(-1)?.onlyProficient).toBe(true);
      setup(trade({ onlyProficient: true }));
      expect((field('only_proficient') as HTMLInputElement).checked).toBe(true);
    });
  });

  describe('Vale para:', () => {
    it('reads the filter back and follows it as it changes', () => {
      setup(trade({ weapon: 'ranged' }));
      expect(readingText()).toBe(
        'Vale para: qualquer arma à distância, mesmo uma em que o personagem não tem proficiência.',
      );
      setup(trade({ weapon: 'melee', weaponProperties: [HEAVY], onlyProficient: true }));
      expect(readingText()).toBe(
        'Vale para: armas corpo a corpo, com a propriedade Pesada, em que o personagem tem proficiência.',
      );
    });

    it('is a status line, so a screen reader hears it change', () => {
      setup(trade());
      expect(reading()!.getAttribute('role')).toBe('status');
    });

    it('says what a use spends, with the resource of the entry', () => {
      setup(trade({ weapon: 'ranged', resource: 'folego_do_atirador' }), {
        siblings: [trade({ weapon: 'ranged', resource: 'folego_do_atirador' }), resource()],
      });
      expect(readingText()).toContain('Cada troca gasta 1 uso de Folego Do Atirador.');
    });
  });

  describe('Mais opções: the resource, when it holds and the text', () => {
    it('folds them, saying which are inside, and opens them on request', () => {
      setup(trade());
      expect(field('resource')).toBeNull();
      expect(field('when')).toBeNull();
      expect(field('text_pt')).toBeNull();
      expect(text(more()!)).toContain('Mais opções');
      expect(text(more()!)).toContain('recurso, quando vale, texto do efeito');
      more()!.click();
      fixture.detectChanges();
      expect(field('resource')).not.toBeNull();
      expect(field('when')).not.toBeNull();
      expect(field('text_pt')).not.toBeNull();
      expect(text(more()!)).toContain('Menos opções');
    });

    it('puts the button under the block and the reading, and the fields under the button', () => {
      setup(trade());
      more()!.click();
      fixture.detectChanges();
      const order = (a: Element, b: Element) =>
        a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING;
      expect(order(reading()!, more()!)).toBeTruthy();
      expect(order(more()!, field('resource')!)).toBeTruthy();
    });

    it('opens by itself on a resource that is set, and names the fields in the words of the block', () => {
      setup(trade({ resource: 'folego_do_atirador' }), {
        siblings: [trade({ resource: 'folego_do_atirador' }), resource()],
      });
      const t = text(el);
      expect(t).toContain('Gasta um uso de');
      expect(t).toContain('Um recurso deste talento. Vazio: não gasta nada.');
      const select = field('resource') as HTMLSelectElement;
      expect(Array.from(select.options).map((o) => o.text.trim())).toEqual([
        'Nenhum: não gasta nada',
        'Folego Do Atirador (3 por descanso curto)',
      ]);
      expect(select.value).toBe('folego_do_atirador');
    });

    it('asks for the name as text when the entry has no resource of its own', () => {
      setup(trade({ resource: 'resource:fury' }));
      expect(field('resource')!.tagName).toBe('INPUT');
      expect(text(el)).toContain('Gasta um uso de');
      expect(text(el)).not.toContain('Gasta o custo');
    });

    it('draws "Quando vale" as the board has it: "Sempre" while it is empty, and the line about a condition', () => {
      setup(trade());
      more()!.click();
      fixture.detectChanges();
      const when = field('when') as HTMLInputElement;
      expect(when.getAttribute('placeholder')).toBe('Sempre');
      expect(text(when.closest('.frow')!)).toContain('Uma condição, como um nível mínimo.');
    });

    it('labels the condition and the text in the words of the block', () => {
      setup(trade({ when: 'level() >= 3', textPt: 'Mira firme.' }));
      expect(text(el)).toContain('Quando vale');
      expect(text(el)).toContain('Texto do efeito');
    });
  });

  describe('the error states', () => {
    it('says "De 1 a 10." and "De 1 a 20." under the numbers outside their ranges, in the danger state', () => {
      setup(trade());
      rerender(trade({ attackPenalty: 12, damageBonus: 0, typed: { damage_bonus: '0' } }));
      const penalty = field('attack_penalty') as HTMLInputElement;
      const damage = field('damage_bonus') as HTMLInputElement;
      expect(penalty.getAttribute('aria-invalid')).toBe('true');
      expect(damage.getAttribute('aria-invalid')).toBe('true');
      expect(text(penalty.closest('app-text-field')!)).toContain('De 1 a 10.');
      expect(text(damage.closest('app-text-field')!)).toContain('De 1 a 20.');
      expect(text(damage.closest('app-text-field')!)).not.toContain('só se acertar');
    });

    it('says nothing of an empty number, and gives the hint back when the number is right', () => {
      setup(trade({ attackPenalty: 0 }));
      const penalty = field('attack_penalty') as HTMLInputElement;
      expect(penalty.getAttribute('aria-invalid')).not.toBe('true');
      expect(text(penalty.closest('app-text-field')!)).toContain(
        'De 1 a 10: sai da jogada de ataque.',
      );
    });

    it('says one message when the server and the editor both refuse the same number', () => {
      setup(trade({ attackPenalty: 12 }), {
        issues: { [`${BASE}.attack_penalty`]: [BAD_EFFECT_VALUE] },
      });
      const note = text(field('attack_penalty')!.closest('app-text-field')!);
      expect(note.match(/De 1 a 10\./g)).toHaveLength(1);
    });

    it("turns the server's refusal of a number into the range, in the block's words", () => {
      setup(trade(), {
        issues: {
          [`${BASE}.attack_penalty`]: [BAD_EFFECT_VALUE],
          [`${BASE}.damage_bonus`]: [BAD_EFFECT_VALUE],
        },
      });
      expect(text(field('attack_penalty')!.closest('app-text-field')!)).toContain('De 1 a 10.');
      expect(text(field('damage_bonus')!.closest('app-text-field')!)).toContain('De 1 a 20.');
    });

    it('says the number is no whole number, and the draft cannot be saved', () => {
      setup(trade());
      const sent = typeInto('damage_bonus', 'dez');
      expect(sent.damageBonus).toBeNaN();
      setup(sent);
      expect(text(field('damage_bonus')!.closest('app-text-field')!)).toContain(
        'Escreva um número inteiro, como 2.',
      );
      expect(unreadableNumberPaths('x', [sent], powerAttackMenu())).toEqual(['x[0].damage_bonus']);
    });

    it("says why the resource or the weapon was refused, at the block's own fields", () => {
      setup(trade({ resource: 'gone' }), {
        issues: { [`${BASE}.resource`]: [DANGLING_TEXT] },
      });
      expect(text(el)).toContain('Esta entrada não define mais este recurso.');
    });
  });

  describe('one power attack for each feature', () => {
    const other = (): EffectDraft => trade();
    const typeOptions = () =>
      Array.from((field('type') as HTMLSelectElement).options).filter(
        (o) => o.value === 'power_attack',
      );

    it('dashes the second one in the "Efeito" menu with the reason, and still lets the first one stay', () => {
      const second = emptyEffect('proficiency');
      setup(second, { siblings: [other(), second] });
      const [opt] = typeOptions();
      expect(opt.disabled).toBe(true);
      expect(opt.getAttribute('data-reason')).toBe('Este talento já tem um ataque poderoso.');
    });

    it('does not dash the type of the effect that is the power attack itself', () => {
      const first = trade();
      setup(first, { siblings: [first, emptyEffect('proficiency')] });
      expect(typeOptions()[0].disabled).toBe(false);
      expect((field('type') as HTMLSelectElement).value).toBe('power_attack');
    });

    it('leaves the type open when no other effect of the entry has it', () => {
      const e = emptyEffect('proficiency');
      setup(e, { siblings: [e] });
      expect(typeOptions()[0].disabled).toBe(false);
    });

    it('words the reason for an entry that is not a feat', () => {
      const second = emptyEffect('proficiency');
      setup(second, { siblings: [other(), second], featEditor: false });
      expect(typeOptions()[0].getAttribute('data-reason')).toBe(
        'Já há um ataque poderoso nesta entrada.',
      );
    });

    it('says the same in words when the server refuses the second one at the type', () => {
      setup(trade(), { issues: { [`${BASE}.type`]: ['Este efeito não está no menu.'] } });
      expect(text(field('type')!.closest('app-select-field')!)).toContain(
        'Este talento já tem um ataque poderoso.',
      );
    });
  });

  describe('a stored power_attack', () => {
    it('opens in the block, editable, with the stored numbers, kind, properties and switch', () => {
      setup(
        effectToDraft(
          create(TableEffectSchema, {
            type: 'power_attack',
            attackPenalty: 5,
            damageBonus: 10,
            weapon: 'melee',
            weaponProperties: [HEAVY],
            onlyProficient: true,
          }),
        ),
      );
      expect((field('attack_penalty') as HTMLInputElement).value).toBe('5');
      expect((field('damage_bonus') as HTMLInputElement).value).toBe('10');
      expect(cards().map((c) => c.checked)).toEqual([false, true, false]);
      expect(chips()).toEqual(['Pesada']);
      expect((field('only_proficient') as HTMLInputElement).checked).toBe(true);
      expect((field('type') as HTMLSelectElement).disabled).toBe(false);
      expect(text(el)).not.toContain('fora do que o editor aceita');
    });

    it('goes back as it was stored when nothing is touched', () => {
      const d = effectToDraft(
        create(TableEffectSchema, {
          type: 'power_attack',
          attackPenalty: 5,
          damageBonus: 10,
          weapon: 'melee',
          weaponProperties: [HEAVY],
          onlyProficient: true,
          textPt: 'Texto.',
        }),
      );
      expect(draftToEffect(d, powerAttackMenu())).toEqual({
        type: 'power_attack',
        attackPenalty: 5,
        damageBonus: 10,
        weapon: 'melee',
        weaponProperties: [HEAVY],
        onlyProficient: true,
        textPt: 'Texto.',
      });
    });

    it('with numbers the block cannot hold stays read-only, as stored, with a select that cannot switch', () => {
      setup(
        effectToDraft(
          create(TableEffectSchema, { type: 'power_attack', attackPenalty: 15, damageBonus: 10 }),
        ),
      );
      const t = text(el);
      expect(t).toContain('Penalidade no ataque−15');
      expect(t).toContain('Bônus no dano+10');
      expect(t).toContain('fora do que o editor aceita');
      expect(field('attack_penalty')).toBeNull();
      expect(field('damage_bonus')).toBeNull();
      expect(cards()).toHaveLength(0);
      expect((field('type') as HTMLSelectElement).disabled).toBe(true);
    });
  });
});
