import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  TableContentKind,
  TableContentRefusalSchema,
  TableContentViolationSchema,
  TableFeatSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { TableContentClient } from '../../../core/content/content-client';
import { catalog, entry, menu } from '../../../core/content/content-testing';
import { FeatEditor } from './feat-editor';
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

describe('FeatEditor (MR-025)', () => {
  const save = vi.fn();
  const stored = () =>
    entry(TableContentKind.FEAT, 'Mestre das Cordas', {
      body: {
        case: 'tableFeat',
        value: create(TableFeatSchema, {
          namePt: 'Mestre das Cordas',
          descPt: ['Você prende quem quiser.'],
          prerequisite: { minimums: { strength: 13 }, level: 4, raceKey: 'race:human' },
          effects: [
            { type: 'ability_increase', count: 1, from: ['strength', 'dexterity'], value: '1' },
          ],
        }),
      },
    });

  function setup(existing = true) {
    save.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [{ provide: TableContentClient, useValue: { save } }],
    });
    const fixture = TestBed.createComponent(FeatEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('catalog', catalog());
    fixture.componentRef.setInput('menu', menu());
    if (existing) fixture.componentRef.setInput('entry', stored());
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const text = (e: Element) => (e.textContent ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ');
  const field = (el: HTMLElement, path: string) =>
    el.querySelector<HTMLElement>(`[data-field="${path}"]`)!;
  const click = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => text(b).includes(label))!
      .click();

  it('draws the form from the entry: name, text, the minimum scores, the level, the race and the ability increase', () => {
    const { el } = setup();
    expect((field(el, 'table_feat.name_pt') as HTMLInputElement).value).toBe('Mestre das Cordas');
    expect((field(el, 'table_feat.desc_pt') as HTMLTextAreaElement).value).toBe(
      'Você prende quem quiser.',
    );
    expect((field(el, 'table_feat.prerequisite.minimums.strength') as HTMLInputElement).value).toBe(
      '13',
    );
    expect(
      (field(el, 'table_feat.prerequisite.minimums.dexterity') as HTMLInputElement).value,
    ).toBe('');
    expect((field(el, 'table_feat.prerequisite.level') as HTMLInputElement).value).toBe('4');
    expect(
      (field(el, 'table_feat.prerequisite.race_key') as HTMLSelectElement).selectedOptions[0].text,
    ).toBe('Humano');
    expect(
      (field(el, 'table_feat.effects[0].type') as HTMLSelectElement).selectedOptions[0].text,
    ).toBe('Aumento de habilidade');
    expect(text(field(el, 'table_feat.effects[0].from'))).toContain('Força');
  });

  it('writes "Como os jogadores veem" with the same text as the player\'s page', () => {
    const { el } = setup();
    const rows = Array.from(el.querySelectorAll('.preview .rows__row')).map(
      (r) => `${r.querySelector('dt')!.textContent}: ${r.querySelector('dd')!.textContent}`,
    );
    expect(rows[0]).toContain('Pré-requisito: Força 13, ser Humano, nível 4 ou mais');
    expect(text(el.querySelector('.preview')!)).toContain('Você prende quem quiser.');
  });

  it('sends the whole feat: the prerequisite as numbers, the effect and no key of its own', async () => {
    const { fixture, el } = setup();
    save.mockResolvedValue({ entry: stored(), affected: [] });
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    const [, sent, body, key] = save.mock.calls[0];
    expect(sent.key).toBe('feat:mestre-das-cordas@mesa');
    expect(key).toEqual(expect.any(String));
    expect(body.case).toBe('tableFeat');
    expect(body.value).toEqual({
      namePt: 'Mestre das Cordas',
      descPt: ['Você prende quem quiser.'],
      prerequisite: {
        minimums: { strength: 13 },
        proficiencyKey: '',
        spellcasting: false,
        raceKey: 'race:human',
        level: 4,
      },
      effects: [
        { type: 'ability_increase', count: 1, from: ['strength', 'dexterity'], value: '1' },
      ],
    });
  });

  it("puts the server's paths back on their inputs: the prerequisite, the ability increase and the name", async () => {
    const { fixture, el } = setup();
    save.mockRejectedValue(
      refusal(
        ['table_feat.name_pt', 'duplicate_name'],
        ['table_feat.prerequisite.level', 'bad_value'],
        ['table_feat.prerequisite.race_key', 'dangling_reference'],
        ['table_feat.effects[0].count', 'bad_value'],
      ),
    );
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    expect(text(el.querySelector('[role="alert"]')!)).toContain('4 campos precisam de ajuste');
    expect(text(field(el, 'table_feat.name_pt').closest('app-text-field')!)).toContain(
      'Já existe um talento da mesa com este nome.',
    );
    expect(text(field(el, 'table_feat.prerequisite.level').closest('app-text-field')!)).toContain(
      'O nível vai de 1 a 20',
    );
    expect(
      text(field(el, 'table_feat.prerequisite.race_key').closest('app-select-field')!),
    ).toContain('Esta raça ou sub-raça não existe mais.');
    expect(text(field(el, 'table_feat.effects[0].count').closest('app-text-field')!)).toContain(
      'Quantos o jogador escolhe',
    );
    expect(document.activeElement).toBe(field(el, 'table_feat.name_pt'));
  });

  it('a new feat starts empty, asks nothing and is called "Novo talento" in the preview', () => {
    const { el } = setup(false);
    expect((field(el, 'table_feat.name_pt') as HTMLInputElement).value).toBe('');
    expect((field(el, 'table_feat.prerequisite.level') as HTMLInputElement).value).toBe('');
    expect(text(el.querySelector('.preview')!)).toContain('Novo talento');
    expect(text(el.querySelector('.preview')!)).toContain('Pré-requisitoNenhum');
  });
});
