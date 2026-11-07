import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { provideRouter } from '@angular/router';

import {
  TableClassSchema,
  TableContentKind,
  TableContentRefusalSchema,
  TableContentViolationSchema,
  TableOptionSchema,
  TableSubclassSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { TableContentClient } from '../../../core/content/content-client';
import { catalog, entry, feature } from '../../../core/content/content-testing';
import { menu } from '../../../core/content/content-testing-menu';
import { OptionEditor } from './option-editor';
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

describe('OptionEditor (MR-025)', () => {
  const save = vi.fn();
  const stored = () =>
    entry(TableContentKind.OPTION, 'Olhar do Corvo', {
      body: {
        case: 'tableOption',
        value: create(TableOptionSchema, {
          namePt: 'Olhar do Corvo',
          descPt: ['Você enxerga o que o corvo vê.'],
          set: 'invocation',
          prerequisite: {
            classLevel: { classKey: 'class:wizard', level: 5 },
            requiresPact: 'feature:pact-of-the-tome',
          },
          effects: [{ type: 'note', textPt: 'Enxerga pelos olhos de um corvo.' }],
        }),
      },
    });

  /** A subclass of the table whose level 3 feature lists three options, and a class whose level 2 feature lists two. */
  const owners = () => {
    const withOptions = (name: string, n: number) => ({
      ...feature(name),
      options: Array.from({ length: n }, (_, i) => feature(`${name} ${i + 1}`)),
    });
    return [
      entry(TableContentKind.SUBCLASS, 'Guarda do Corvo', {
        body: {
          case: 'tableSubclass',
          value: create(TableSubclassSchema, {
            namePt: 'Guarda do Corvo',
            classKey: 'class:paladin',
            levels: [
              { level: 3, features: [withOptions('Truques de lâmina', 3), feature('Sem lista')] },
            ],
          }),
        },
      }),
      entry(TableContentKind.CLASS, 'Guardião do Vale', {
        body: {
          case: 'tableClass',
          value: create(TableClassSchema, {
            namePt: 'Guardião do Vale',
            levels: [{ features: [] }, { features: [withOptions('Votos do Vale', 2)] }],
          }),
        },
      }),
      entry(TableContentKind.SUBCLASS, 'Arquivada', {
        archived: true,
        body: {
          case: 'tableSubclass',
          value: create(TableSubclassSchema, {
            namePt: 'Arquivada',
            levels: [{ level: 3, features: [withOptions('Perdida', 2)] }],
          }),
        },
      }),
    ];
  };
  const crow = 'feature:Truques de lâmina';

  function setup(existing = true, entries = owners(), saved = stored()) {
    save.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: TableContentClient, useValue: { save } }],
    });
    const fixture = TestBed.createComponent(OptionEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('catalog', catalog(entries));
    fixture.componentRef.setInput('menu', menu());
    fixture.componentRef.setInput('entries', entries);
    if (existing) fixture.componentRef.setInput('entry', saved);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const text = (e: Element) => (e.textContent ?? '').replace(/\s+/g, ' ');
  const field = (el: HTMLElement, path: string) =>
    el.querySelector<HTMLElement>(`[data-field="${path}"]`)!;
  const click = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => text(b).includes(label))!
      .click();

  it('draws the form from the entry: name, text, the set read-only, the class level and the pact', () => {
    const { el } = setup();
    expect((field(el, 'table_option.name_pt') as HTMLInputElement).value).toBe('Olhar do Corvo');
    expect((field(el, 'table_option.desc_pt') as HTMLTextAreaElement).value).toBe(
      'Você enxerga o que o corvo vê.',
    );
    expect(text(field(el, 'table_option.set'))).toContain('Invocação mística');
    expect(field(el, 'table_option.set').tagName).not.toBe('SELECT');
    expect(
      (field(el, 'table_option.prerequisite.class_level.level') as HTMLInputElement).value,
    ).toBe('5');
    expect(
      (field(el, 'table_option.prerequisite.requires_pact') as HTMLSelectElement).selectedOptions[0]
        .text,
    ).toBe('Pacto do Tomo');
  });

  it('writes "Como os jogadores veem" from the draft', () => {
    const { el } = setup();
    expect(text(el.querySelector('.preview')!)).toContain('Você enxerga o que o corvo vê.');
  });

  it('a new option picks the set from the menu and sends it with the whole body', async () => {
    const { fixture, el } = setup(false);
    const set = field(el, 'table_option.set') as HTMLSelectElement;
    expect(Array.from(set.options).map((o) => o.text)).toContain('Invocação mística');
    set.value = 'invocation';
    set.dispatchEvent(new Event('change'));
    const name = field(el, 'table_option.name_pt') as HTMLInputElement;
    name.value = 'Olhar do Corvo';
    name.dispatchEvent(new Event('input'));
    save.mockResolvedValue({ entry: stored(), affected: [] });
    click(el, 'Salvar opção');
    await settle(fixture, 3);
    const [, sent, body] = save.mock.calls[0];
    expect(sent).toBeNull();
    expect(body.case).toBe('tableOption');
    expect(body.value.set).toBe('invocation');
    expect(body.value.namePt).toBe('Olhar do Corvo');
    expect(body.value.prerequisite).toEqual({
      requiresOptionKeys: [],
      requiresPact: '',
      requiresSpellKey: '',
    });
  });

  it("puts the server's paths back on their inputs: the set, the level and the name", async () => {
    const { fixture, el } = setup();
    save.mockRejectedValue(
      refusal(
        ['table_option.name_pt', 'duplicate_name'],
        ['table_option.prerequisite.class_level.level', 'bad_value'],
        ['table_option.prerequisite.requires_pact', 'dangling_reference'],
      ),
    );
    click(el, 'Salvar opção');
    await settle(fixture, 3);
    expect(text(el.querySelector('[role="alert"]')!)).toContain('3 campos precisam de ajuste');
    expect(
      text(field(el, 'table_option.prerequisite.class_level.level').closest('app-text-field')!),
    ).toContain('O nível vai de 1 a 20');
    expect(document.activeElement).toBe(field(el, 'table_option.name_pt'));
  });

  it('a new option starts empty and is called "Nova opção" in the preview', () => {
    const { el } = setup(false);
    expect((field(el, 'table_option.name_pt') as HTMLInputElement).value).toBe('');
    expect(text(el.querySelector('.preview')!)).toContain('Nova opção');
  });

  describe('the feature that offers the option', () => {
    const offered = (offeredBy = '') =>
      entry(TableContentKind.OPTION, 'Golpe do Corvo', {
        body: {
          case: 'tableOption',
          value: create(TableOptionSchema, {
            namePt: 'Golpe do Corvo',
            set: 'table_feature',
            offeredBy,
          }),
        },
      });
    const pickSet = (el: HTMLElement, set: string) => {
      const select = field(el, 'table_option.set') as HTMLSelectElement;
      select.value = set;
      select.dispatchEvent(new Event('change'));
    };
    const offerer = (el: HTMLElement) =>
      field(el, 'table_option.offered_by') as HTMLSelectElement | null;

    it('asks for the feature only for the set of a table feature', () => {
      const { fixture, el } = setup(false);
      expect(offerer(el)).toBeNull();
      pickSet(el, 'table_feature');
      fixture.detectChanges();
      expect(offerer(el)).not.toBeNull();
      pickSet(el, 'invocation');
      fixture.detectChanges();
      expect(offerer(el)).toBeNull();
    });

    it('lists the table features with options of their own, by owner, and no archived owner', () => {
      const { fixture, el } = setup(false);
      pickSet(el, 'table_feature');
      fixture.detectChanges();
      const groups = Array.from(offerer(el)!.querySelectorAll('optgroup')).map((g) => g.label);
      expect(groups).toEqual([
        'Guarda do Corvo · subclasse de Paladino',
        'Guardião do Vale · classe da mesa',
      ]);
      const rows = Array.from(offerer(el)!.options).map((o) => o.text);
      expect(rows).toContain('Truques de lâmina · nível 3 · 3 opções');
      expect(rows).toContain('Votos do Vale · nível 2 · 2 opções');
      expect(rows.join()).not.toContain('Sem lista');
      expect(rows.join()).not.toContain('Perdida');
      expect(
        text(el.querySelector('app-select-field:has([data-field="table_option.offered_by"])')!),
      ).toContain('Só as características com lista de opções própria');
    });

    it('does not save without the feature: it says so at the field and takes the focus there', async () => {
      const { fixture, el } = setup(false);
      pickSet(el, 'table_feature');
      fixture.detectChanges();
      click(el, 'Salvar opção');
      await settle(fixture, 3);
      expect(save).not.toHaveBeenCalled();
      expect(text(offerer(el)!.closest('app-select-field')!)).toContain(
        'Escolha a característica que oferece a opção.',
      );
      expect(document.activeElement).toBe(offerer(el));
    });

    it('sends the chosen feature, and clears it when the set changes', async () => {
      const { fixture, el } = setup(false);
      pickSet(el, 'table_feature');
      fixture.detectChanges();
      offerer(el)!.value = crow;
      offerer(el)!.dispatchEvent(new Event('change'));
      const name = field(el, 'table_option.name_pt') as HTMLInputElement;
      name.value = 'Golpe do Corvo';
      name.dispatchEvent(new Event('input'));
      save.mockResolvedValue({ entry: offered(crow), affected: [] });
      click(el, 'Salvar opção');
      await settle(fixture, 3);
      expect(save.mock.calls[0][2].value.offeredBy).toBe(crow);

      pickSet(el, 'invocation');
      fixture.detectChanges();
      click(el, 'Salvar opção');
      await settle(fixture, 3);
      expect(save.mock.calls[1][2].value.offeredBy).toBe('');
    });

    it('shows the saved feature with its owner, the options it lists and a link to the owner', () => {
      const { el } = setup(true, owners(), offered(crow));
      expect(offerer(el)!.selectedOptions[0].text).toContain('Truques de lâmina');
      const note = text(
        el.querySelector('app-select-field:has([data-field="table_option.offered_by"])')!,
      );
      expect(note).toContain('Guarda do Corvo · subclasse de Paladino · nível 3');
      expect(text(el)).toContain(
        'Entra na lista com Truques de lâmina 1, Truques de lâmina 2, Truques de lâmina 3',
      );
      const link = Array.from(el.querySelectorAll('a')).find((a) =>
        text(a).includes('Abrir Guarda do Corvo'),
      )!;
      expect(link.getAttribute('href')).toContain('/campaigns/camp-1/content/entries/');
      expect(text(el.querySelector('.preview')!)).toContain(
        'Oferecida porTruques de lâmina (Guarda do Corvo, Paladino 3)',
      );
    });

    it('is disabled with the reason and a link to the subclasses when nothing is offered', () => {
      const { fixture, el } = setup(false, []);
      pickSet(el, 'table_feature');
      fixture.detectChanges();
      expect(offerer(el)!.disabled).toBe(true);
      expect(offerer(el)!.options[0].text).toBe('Nenhuma disponível');
      expect(text(el)).toContain('Crie a lista lá e volte aqui.');
      expect(text(el)).not.toContain('Só as características com lista de opções própria');
      const link = Array.from(el.querySelectorAll('a')).find((a) =>
        text(a).includes('Ver as subclasses da mesa'),
      )!;
      expect(link.getAttribute('href')).toContain('kind=subclasses');
    });

    it('takes the focus to the link when a save is tried with nothing to choose from', async () => {
      const { fixture, el } = setup(false, []);
      pickSet(el, 'table_feature');
      fixture.detectChanges();
      click(el, 'Salvar opção');
      await settle(fixture, 3);
      expect(save).not.toHaveBeenCalled();
      expect(document.activeElement).toBe(field(el, 'table_option.offered_by.help'));
      expect(text(offerer(el)!.closest('app-select-field')!)).toContain(
        'Escolha a característica que oferece a opção.',
      );
    });

    it('names the feature of a saved option whose owner was archived, not its key', () => {
      const archivedOwners = owners().map((e) => ({ ...e, archived: true }));
      const { el } = setup(true, archivedOwners as ReturnType<typeof owners>, offered(crow));
      expect(offerer(el)!.selectedOptions[0].text).toBe('Truques de lâmina (Guarda do Corvo)');
    });

    it('shows only the name of a feature the server refused, and describes it again once the refusal is gone', async () => {
      const { fixture, el } = setup(true, owners(), offered(crow));
      const row = () => offerer(el)!.selectedOptions[0].text;
      expect(row()).toBe('Truques de lâmina · nível 3 · 3 opções');
      expect(text(el)).toContain('Entra na lista com');
      save.mockRejectedValue(refusal(['table_option.offered_by', 'dangling_reference']));
      click(el, 'Salvar opção');
      await settle(fixture, 3);
      expect(row()).toBe('Truques de lâmina');
      expect(text(el)).not.toContain('Entra na lista com');
      expect(text(el)).not.toContain('A opção entra na lista');
      expect(Array.from(el.querySelectorAll('a')).some((a) => text(a).includes('Abrir '))).toBe(
        false,
      );
      const note = text(offerer(el)!.closest('app-select-field')!);
      expect(note).toContain('Escolha outra.');
      expect(note).not.toContain('nível 3');
    });

    it('keeps a saved feature that is gone on screen and puts the refusal at the field', async () => {
      const gone = 'feature:class-guardiao-3f9a1c--votos-antigos@mesa';
      const { fixture, el } = setup(true, owners(), offered(gone));
      expect(offerer(el)!.selectedOptions[0].text).toBe('Votos antigos');
      save.mockRejectedValue(refusal(['table_option.offered_by', 'dangling_reference']));
      click(el, 'Salvar opção');
      await settle(fixture, 3);
      expect(text(offerer(el)!.closest('app-select-field')!)).toContain(
        'Esta característica não existe mais ou não tem lista de opções. Escolha outra.',
      );
      expect(offerer(el)!.getAttribute('aria-invalid')).toBe('true');
      expect(document.activeElement).toBe(offerer(el));
    });
  });
});
