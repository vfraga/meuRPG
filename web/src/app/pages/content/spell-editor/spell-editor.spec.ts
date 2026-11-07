import { type ComponentFixture, TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { CastingTimeUnit } from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableContentBlockedReason,
  TableContentBlockedSchema,
  TableContentKind,
  TableContentRefusalSchema,
  TableContentViolationSchema,
  TableSpellSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { rememberCreatureName } from '../../../core/content/summon-names';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { TableContentClient } from '../../../core/content/content-client';
import { catalog, entry } from '../../../core/content/content-testing';
import { SpellEditor } from './spell-editor';
import { settle } from '../../../../test-settle';

function refusal(...violations: [string, string][]) {
  return new ConnectError('refused', Code.InvalidArgument, undefined, [
    {
      desc: TableContentRefusalSchema,
      value: create(TableContentRefusalSchema, {
        violations: violations.map(([field, reason]) =>
          create(TableContentViolationSchema, { field, reason }),
        ),
      }),
    },
  ]);
}

describe('SpellEditor', () => {
  const save = vi.fn();
  const cat = catalog();

  function setup(existing = false, extra: Record<string, unknown> = {}) {
    save.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: TableContentClient, useValue: { save } },
        {
          provide: CreaturesClient,
          useValue: {
            statBlock: vi.fn().mockResolvedValue({ summary: { namePt: 'Lobo' } }),
            search: vi.fn().mockResolvedValue({ creatures: [], total: 0 }),
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(SpellEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('catalog', cat);
    if (existing) {
      fixture.componentRef.setInput(
        'entry',
        entry(TableContentKind.SPELL, 'Lâmina de Nanquim', {
          body: {
            case: 'tableSpell',
            value: create(TableSpellSchema, {
              namePt: 'Lâmina de Nanquim',
              level: 1,
              schoolKey: 'school:evocation',
              castingTime: { unit: 1, amount: 1 },
              range: { kind: 3, distanceFt: 60 },
              duration: { kind: 1 },
              target: { kind: 1 },
              components: { verbal: true },
              ...extra,
            }),
          },
        }),
      );
    }
    fixture.detectChanges();
    return {
      fixture,
      el: fixture.nativeElement as HTMLElement,
      comp: fixture.componentInstance as unknown as Record<string, any>,
    };
  }

  const text = (el: Element) => (el.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const field = (el: HTMLElement, path: string) =>
    el.querySelector<HTMLElement>(`[data-field="${path}"]`)!;
  const click = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => text(b).includes(label))!
      .click();

  it('offers the casting classes, the table\'s marked "Da mesa", and no class that never casts', () => {
    const { el } = setup();
    const classes = Array.from(
      el.querySelectorAll('[data-field="table_spell.class_keys"] app-check-row'),
    ).map((r) =>
      (r.textContent ?? '')
        .replace('check', '')
        .replace('menu_book', ' ')
        .replace(/\s+/g, ' ')
        .trim(),
    );
    expect(classes).toEqual(['Mago']);
  });

  it('shows the size label of the shape, and the area only for "Área"', async () => {
    const { fixture, el } = setup();
    expect(el.querySelector('[data-field="table_spell.target.size_ft"]')).toBeNull();
    (
      Array.from(el.querySelectorAll('app-segmented input[type="radio"]')).find((i) =>
        i.closest('label')?.textContent?.includes('Área'),
      ) as HTMLInputElement
    ).click();
    await settle(fixture, 3);
    expect(text(el)).toContain('Comprimento');
    const shape = field(el, 'table_spell.target.shape') as HTMLSelectElement;
    shape.value = 'cube';
    shape.selectedIndex = Array.from(shape.options).findIndex((o) => o.text === 'Cubo');
    shape.dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    expect(text(el)).toContain('Lado');
    expect(text(el)).not.toContain('Comprimento');
  });

  it('draws "Como os jogadores veem" from the form as it is typed', async () => {
    const { fixture, el } = setup();
    const name = field(el, 'table_spell.name_pt') as HTMLInputElement;
    name.value = 'Sopro de Nanquim';
    name.dispatchEvent(new Event('input'));
    const range = field(el, 'table_spell.range.distance_ft') as HTMLInputElement;
    range.value = '18';
    range.dispatchEvent(new Event('input'));
    await settle(fixture, 3);
    const preview = text(el.querySelector('.preview')!);
    expect(preview).toContain('Sopro de Nanquim');
    const rows = Array.from(el.querySelectorAll('.preview .rows__row')).map((r) =>
      `${r.querySelector('dt')!.textContent}: ${r.querySelector('dd')!.textContent}`.replace(
        /\u00a0/g,
        ' ',
      ),
    );
    expect(rows).toContain('Alcance: 18 m');
    expect(rows).toContain('Alvo: Uma criatura');
    expect(preview).toContain('Alcance');
  });

  it('sends the form with the revision it read, and tells the page what was saved', async () => {
    const { fixture, el } = setup(true);
    const saved = vi.fn();
    fixture.componentInstance.saved.subscribe(saved);
    save.mockResolvedValue({
      entry: entry(TableContentKind.SPELL, 'Lâmina de Nanquim'),
      affected: [],
    });
    click(el, 'Salvar magia');
    await settle(fixture, 3);
    expect(save).toHaveBeenCalledTimes(1);
    const [campaignId, sent, body] = save.mock.calls[0];
    expect(campaignId).toBe('camp-1');
    expect(sent.revision).toBe(3);
    expect(body.case).toBe('tableSpell');
    expect(body.value.range).toEqual({ kind: 3, distanceFt: 60 });
    expect(saved).toHaveBeenCalled();
  });

  it('puts a refusal back on its fields, with the reason in Portuguese, the summary on top, and the focus on the first one', async () => {
    const { fixture, el } = setup(true);
    save.mockRejectedValue(
      refusal(
        ['table_spell.name_pt', 'duplicate_name'],
        ['table_spell.range.distance_ft', 'limit'],
        ['table_spell.damage[0].dice', 'bad_value'],
      ),
    );
    click(el, 'Salvar magia');
    await settle(fixture, 3);
    const alert = el.querySelector('[role="alert"]')!;
    expect(text(alert)).toContain('Não foi possível salvar a magia.');
    expect(text(alert)).toContain('3 campos precisam de ajuste');
    expect(field(el, 'table_spell.name_pt').getAttribute('aria-invalid')).toBe('true');
    expect(text(el)).toContain('Já existe uma magia da mesa com este nome. Escolha outro.');
    expect(field(el, 'table_spell.range.distance_ft').getAttribute('aria-invalid')).toBe('true');
    expect(text(el)).toContain('O alcance vai de 1,5 m a 1.584 m, em passos de 1,5 m.');
    // The path of a mechanic that is not on screen has no input above it either: it goes to the top, in words.
    expect(text(alert)).toContain('Escreva o dado assim: 2d8');
    expect(document.activeElement).toBe(field(el, 'table_spell.name_pt'));
    // What was typed is still there.
    expect((field(el, 'table_spell.name_pt') as HTMLInputElement).value).toBe('Lâmina de Nanquim');
  });

  it('says "Esta entrada mudou enquanto você editava" for a stale write and offers to reload', async () => {
    const { fixture, el } = setup(true);
    const reload = vi.fn();
    fixture.componentInstance.reload.subscribe(reload);
    save.mockRejectedValue(
      new ConnectError('stale', Code.Aborted, undefined, [
        {
          desc: TableContentBlockedSchema,
          value: create(TableContentBlockedSchema, { reason: TableContentBlockedReason.STALE }),
        },
      ]),
    );
    click(el, 'Salvar magia');
    await settle(fixture, 3);
    expect(text(el.querySelector('.mr-live[role="alert"]')!)).toContain(
      'Esta entrada mudou enquanto você editava.',
    );
    click(el, 'Recarregar');
    expect(reload).toHaveBeenCalled();
  });

  it('does not let a spell cross between truque and leveled when editing', () => {
    const { el } = setup(true);
    const level = field(el, 'table_spell.level') as HTMLSelectElement;
    const options = Array.from(level.options).map((o) => `${o.text}${o.disabled ? ' (off)' : ''}`);
    expect(options[0]).toBe('Truque (off)');
    expect(options[1]).toBe('1º nível');
  });

  it('turns the save button off, with the reason written, while the archive question is open', () => {
    const { fixture, el } = setup(true);
    fixture.componentRef.setInput(
      'saveBlocked',
      'Responda à pergunta de arquivar para voltar a salvar.',
    );
    fixture.detectChanges();
    expect(text(el)).toContain('Responda à pergunta de arquivar para voltar a salvar.');
    click(el, 'Salvar magia');
    expect(save).not.toHaveBeenCalled();
  });

  it('keeps "Mais por nível de espaço" folded under "Uma criatura" until asked, and never offers it to a truque', async () => {
    const { fixture, el } = setup(true);
    expect(el.querySelector('[data-field="table_spell.target.per_slot_level"]')).toBeNull();
    click(el, 'Mais criaturas por nível de espaço');
    await settle(fixture, 3);
    expect(el.querySelector('[data-field="table_spell.target.per_slot_level"]')).not.toBeNull();
    const level = field(el, 'table_spell.level') as HTMLSelectElement;
    level.selectedIndex = 0;
    level.dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    expect(el.querySelector('[data-field="table_spell.target.per_slot_level"]')).toBeNull();
    expect(text(el)).not.toContain('acima do 0º');
  });

  const type = (input: HTMLElement, value: string) => {
    (input as HTMLInputElement).value = value;
    input.dispatchEvent(new Event('input'));
  };

  it('does not save a count whose text is no whole number: it says so on the field and lands on it', async () => {
    const { fixture, el } = setup(true);
    click(el, 'Mais criaturas por nível de espaço');
    await settle(fixture, 3);
    const more = field(el, 'table_spell.target.per_slot_level');
    type(more, 'dois');
    await settle(fixture, 3);
    expect(text(more.closest('app-text-field')!)).toContain('Escreva um número inteiro');
    save.mockClear();
    click(el, 'Salvar magia');
    await settle(fixture, 3);
    expect(save).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(more);
    type(more, '+2');
    await settle(fixture, 3);
    expect((more as HTMLInputElement).value).toBe('+2');
    save.mockResolvedValue({
      entry: entry(TableContentKind.SPELL, 'Lâmina de Nanquim'),
      affected: [],
    });
    click(el, 'Salvar magia');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value.target.perSlotLevel).toBe(2);
  });

  it('blocks a minutes amount that is no whole number, and shows the stored number again after a reload', async () => {
    const { fixture, el } = setup(true, {
      castingTime: { unit: CastingTimeUnit.MINUTE, amount: 10 },
    });
    const amount = field(el, 'table_spell.casting_time.amount') as HTMLInputElement;
    expect(amount.value).toBe('10');
    type(amount, '1e1');
    await settle(fixture, 3);
    save.mockClear();
    click(el, 'Salvar magia');
    await settle(fixture, 3);
    expect(save).not.toHaveBeenCalled();
    expect(text(amount.closest('app-text-field')!)).toContain('Escreva um número inteiro');
    type(amount, '010');
    await settle(fixture, 3);
    expect(amount.value).toBe('010');
    const current = fixture.componentInstance.entry()!;
    fixture.componentRef.setInput('entry', { ...current, revision: current.revision + 1 });
    await settle(fixture, 3);
    expect((field(el, 'table_spell.casting_time.amount') as HTMLInputElement).value).toBe('10');
  });

  it('keeps "Pessoal" when the target moves to an area', async () => {
    const { fixture, el } = setup();
    const radio = (label: string) =>
      Array.from(el.querySelectorAll('app-segmented input[type="radio"]')).find((i) =>
        i.closest('label')?.textContent?.includes(label),
      ) as HTMLInputElement;
    radio('Só quem conjura').click();
    await settle(fixture, 3);
    expect((field(el, 'table_spell.range.kind') as HTMLSelectElement).selectedOptions[0].text).toBe(
      'Pessoal',
    );
    radio('Área').click();
    await settle(fixture, 3);
    expect((field(el, 'table_spell.range.kind') as HTMLSelectElement).selectedOptions[0].text).toBe(
      'Pessoal',
    );
  });

  it('names the saving throw ability and the damage types as the server does, and says the extra text is only text', async () => {
    const { fixture, el } = setup();
    const radio = Array.from(el.querySelectorAll('app-segmented input[type="radio"]')).find((i) =>
      i.closest('label')?.textContent?.includes('Teste de resistência'),
    ) as HTMLInputElement;
    radio.click();
    await settle(fixture, 3);
    const ability = field(el, 'table_spell.save.ability') as HTMLSelectElement;
    expect(Array.from(ability.options).map((o) => o.text.trim())).toEqual([
      'Força',
      'Destreza',
      'Constituição',
      'Inteligência',
      'Sabedoria',
      'Carisma',
    ]);
    const type = field(el, 'table_spell.damage[0].damage_type_key') as HTMLSelectElement;
    expect(
      Array.from(type.options)
        .map((o) => o.text.trim())
        .sort(),
    ).toEqual(['Fogo', 'Necrótico']);
    expect(text(el)).toContain('Em níveis superiores (opcional, só texto)');
  });

  it('warns about damage with no attack, saving throw or area, and still saves', async () => {
    const damage = [{ damageTypeKey: 'damage-type:fire', dice: '2d6' }];
    const { fixture, el } = setup(true, { damage });
    const warning = () => el.querySelector('[data-warning="unapplied-damage"]');
    expect(text(warning()!)).toContain('O app não sabe aplicar este dano');
    save.mockResolvedValue({
      entry: entry(TableContentKind.SPELL, 'Lâmina de Nanquim'),
      affected: [],
    });
    click(el, 'Salvar magia');
    await settle(fixture, 3);
    expect(save).toHaveBeenCalledTimes(1);
  });

  it('does not warn once the spell has an attack, and not for a spell without damage', async () => {
    const { fixture, el } = setup(true, {
      damage: [{ damageTypeKey: 'damage-type:fire', dice: '2d6' }],
    });
    const radio = Array.from(el.querySelectorAll('app-segmented input[type="radio"]')).find((i) =>
      i.closest('label')?.textContent?.includes('Ataque'),
    ) as HTMLInputElement;
    radio.click();
    await settle(fixture, 3);
    expect(el.querySelector('[data-warning="unapplied-damage"]')).toBeNull();
    expect(setup(true).el.querySelector('[data-warning="unapplied-damage"]')).toBeNull();
  });

  describe('"Invocar" (#578, board C3)', () => {
    const SUMMON = {
      level: 2,
      target: { kind: 4 },
      range: { kind: 2 },
      summon: {
        creatureKeys: ['monster:wolf'],
        count: '1',
        countPerSlotLevel: 1,
        maxLive: 4,
      },
    };
    const mechanic = (el: HTMLElement, label: string) =>
      Array.from(el.querySelectorAll<HTMLInputElement>('app-segmented input[type="radio"]')).find(
        (i) =>
          i.closest('label')?.textContent?.includes(label) && i.closest('[aria-label="Mecânica"]'),
      )!;
    const choose = async (fixture: ComponentFixture<unknown>, el: HTMLElement, label: string) => {
      mechanic(el, label).click();
      await settle(fixture, 3);
    };

    it('offers "Invocar" as the fifth mechanic, and draws the board\'s fields and lines', async () => {
      const { fixture, el } = setup();
      expect(el.querySelector('app-spell-summon')).toBeNull();
      await choose(fixture, el, 'Invocar');
      const labels = Array.from(el.querySelectorAll('[aria-label="Mecânica"] label')).map((l) =>
        text(l).replace('check', '').trim(),
      );
      expect(labels).toEqual(['Só texto', 'Ataque', 'Teste de resistência', 'Cura', 'Invocar']);
      const block = text(el.querySelector('app-spell-summon')!);
      for (const t of [
        'Quantas vêm',
        'Fórmula; pode usar o círculo do espaço.',
        'A mais por círculo acima',
        'De 0 a 10.',
        'No máximo ao mesmo tempo',
        '0: sem limite. Até 40.',
        'Turno',
        'Ao usar de novo',
        'Sem recurso, tempo nem duração: a magia tem os seus.',
        '“Invocar” pede uma magia de 1º círculo ou mais, com alvo “Só quem conjura”, sem ataque, teste, dano, cura ou área, e que não seja ritual.',
      ]) {
        expect(block).toContain(t);
      }
      expect(el.querySelector('app-spell-summon .lock mat-icon')?.textContent).toContain('lock');
    });

    it('opens a spell that summons on "Invocar", and the preview says who is summoned', async () => {
      rememberCreatureName('monster:wolf', 'Lobo');
      const { fixture, el } = setup(true, SUMMON);
      expect(
        (el.querySelector('[aria-label="Mecânica"] input:checked') as HTMLInputElement).closest(
          'label',
        )!.textContent,
      ).toContain('Invocar');
      const preview = text(el.querySelector('.preview')!);
      expect(preview).toContain('Invoca: Lobo.');
      expect(preview).toContain('Só quem conjura');
      await choose(fixture, el, 'Cura');
      expect(el.querySelector('app-spell-summon')).toBeNull();
      expect(text(el.querySelector('.preview')!)).not.toContain('Invoca:');
    });

    it('sends the summon for "Invocar" and none for another mechanic', async () => {
      const { fixture, el } = setup(true, SUMMON);
      save.mockResolvedValue({
        entry: entry(TableContentKind.SPELL, 'Lâmina de Nanquim'),
        affected: [],
      });
      const count = field(el, 'table_spell.summon.count') as HTMLInputElement;
      count.value = '2 + slot()';
      count.dispatchEvent(new Event('input'));
      await settle(fixture, 3);
      click(el, 'Salvar magia');
      await settle(fixture, 3);
      expect(save.mock.calls[0][2].value.summon).toMatchObject({
        creatureKeys: ['monster:wolf'],
        count: '2 + slot()',
        countPerSlotLevel: 1,
        maxLive: 4,
      });
      await choose(fixture, el, 'Só texto');
      click(el, 'Salvar magia');
      await settle(fixture, 3);
      expect(save.mock.calls[1][2].value.summon).toBeUndefined();
    });

    it('gives the old target and range back when "Invocar" is left, and keeps what the master chose since', async () => {
      const { fixture, el } = setup(true, {
        target: { kind: 1 },
        range: { kind: 3, distanceFt: 60 },
      });
      expect(text(el.querySelector('.preview')!)).toContain('Uma criatura');
      await choose(fixture, el, 'Invocar');
      expect(text(el.querySelector('.preview')!)).toContain('Só quem conjura');
      expect(text(el.querySelector('.preview')!)).toContain('Pessoal');
      await choose(fixture, el, 'Só texto');
      const back = text(el.querySelector('.preview')!);
      expect(back).toContain('Uma criatura');
      expect(back).toContain('60 m'.replace('60', '18'));
      // a target the master picked on "Invocar" is theirs: leaving does not undo it
      await choose(fixture, el, 'Invocar');
      const area = Array.from(
        el.querySelectorAll<HTMLInputElement>(
          '[data-field="table_spell.target.kind"] input[type="radio"]',
        ),
      ).find((i) => i.closest('label')?.textContent?.includes('Várias'))!;
      area.click();
      await settle(fixture, 3);
      await choose(fixture, el, 'Só texto');
      expect(text(el.querySelector('.preview')!)).toContain('criaturas');
    });

    it('refuses in advance a stored effect that the caster alone cannot have, and one on an instantaneous spell', async () => {
      const effect = { applies: 2, conditions: ['condition:blinded'] };
      const { fixture, el } = setup(true, { ...SUMMON, effect });
      expect(text(el.querySelector('app-spell-summon')!)).toContain('Escolha outra mecânica.');
      expect(text(field(el, 'table_spell.duration.kind').closest('app-select-field')!)).toContain(
        'escolha uma duração que não seja instantânea',
      );
      click(el, 'Salvar magia');
      await settle(fixture, 3);
      expect(save).not.toHaveBeenCalled();
      expect(document.activeElement).toBe(field(el, 'table_spell.duration.kind'));
    });

    it('says before the save what the engine refuses, on each field, blocks the save and lands on the first', async () => {
      const { fixture, el } = setup(true, { ...SUMMON, level: 0 });
      // a cantrip stays a cantrip: the editor does not raise it
      expect(text(field(el, 'table_spell.level').closest('app-select-field')!)).toContain(
        'Um truque não invoca',
      );
      expect(field(el, 'table_spell.level').getAttribute('aria-invalid')).toBe('true');
      const ritual = el.querySelector('[data-field="table_spell.ritual"]') as HTMLElement;
      ritual.click();
      await settle(fixture, 3);
      expect(text(el)).toContain('Uma magia que invoca não é ritual');
      click(el, 'Salvar magia');
      await settle(fixture, 3);
      expect(save).not.toHaveBeenCalled();
      expect(document.activeElement).toBe(field(el, 'table_spell.level'));
    });

    it('puts every refusal of the summon on its own field, and the rest on the lock line', async () => {
      const { fixture, el } = setup(true, SUMMON);
      save.mockRejectedValue(
        refusal(
          ['table_spell.summon.creature_keys[0]', 'dangling_reference'],
          ['table_spell.summon.count', 'bad_formula'],
          ['table_spell.summon.count_per_slot_level', 'bad_value'],
          ['table_spell.summon.max_live', 'bad_value'],
          ['table_spell.summon.turn_mode', 'bad_value'],
          ['table_spell.summon.replaces', 'bad_value'],
          ['table_spell.summon', 'bad_value'],
        ),
      );
      click(el, 'Salvar magia');
      await settle(fixture, 3);
      expect(text(el.querySelector('[role="alert"]')!)).toContain('7 campos precisam de ajuste');
      const group = field(el, 'table_spell.summon.creature_keys');
      expect(el.querySelector(`#${group.getAttribute('aria-describedby')}`)?.textContent).toContain(
        'Esta criatura não existe mais.',
      );
      for (const f of ['count', 'count_per_slot_level', 'max_live']) {
        expect(field(el, `table_spell.summon.${f}`).getAttribute('aria-invalid')).toBe('true');
      }
      expect(text(el.querySelector('app-spell-summon')!)).toContain(
        'De 0 a 10 criaturas a mais por círculo acima do da magia.',
      );
      expect(text(el.querySelector('app-spell-summon')!)).toContain('Confira a magia:');
      expect(field(el, 'table_spell.summon.creature_keys').contains(document.activeElement)).toBe(
        true,
      );
    });
  });
});
