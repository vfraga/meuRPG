import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  TableContentBlockedReason,
  TableContentBlockedSchema,
  TableContentKind,
  TableContentRefusalSchema,
  TableContentViolationSchema,
  TableSpellSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
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

  function setup(existing = false) {
    save.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [{ provide: TableContentClient, useValue: { save } }],
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
    expect(text(el.querySelector('[role="alert"]')!)).toContain(
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
});
