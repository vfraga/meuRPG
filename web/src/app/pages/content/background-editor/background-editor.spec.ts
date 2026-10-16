import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  TableBackgroundSchema,
  TableContentKind,
  TableContentRefusalSchema,
  TableContentViolationSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { TableContentClient } from '../../../core/content/content-client';
import { catalog, entry, feature, menu } from '../../../core/content/content-testing';
import { BackgroundEditor } from './background-editor';
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

describe('BackgroundEditor (E10-01 state 7)', () => {
  const save = vi.fn();
  const stored = () =>
    entry(TableContentKind.BACKGROUND, 'Cartógrafo do Vale', {
      body: {
        case: 'tableBackground',
        value: create(TableBackgroundSchema, {
          namePt: 'Cartógrafo do Vale',
          skills: ['skill:investigation', 'skill:perception'],
          tools: ['proficiency:cartographers-tools'],
          languageChoices: 1,
          equipmentPt: 'Um estojo de mapas, tinta e 10 PO',
          feature: {
            ...feature('Mapas na memória', [{ type: 'note' }]),
            descPt: ['Você lembra o desenho de qualquer lugar que já mapeou.'],
          },
        }),
      },
    });

  function setup(existing = true) {
    save.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [{ provide: TableContentClient, useValue: { save } }],
    });
    const fixture = TestBed.createComponent(BackgroundEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('catalog', catalog());
    fixture.componentRef.setInput('menu', menu());
    if (existing) fixture.componentRef.setInput('entry', stored());
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const text = (e: Element) => (e.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const field = (el: HTMLElement, path: string) =>
    el.querySelector<HTMLElement>(`[data-field="${path}"]`)!;
  const click = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => text(b).includes(label))!
      .click();

  it('draws the form from the entry: two skills, the tools as chips, the equipment and the one feature', () => {
    const { el } = setup();
    expect((field(el, 'table_background.name_pt') as HTMLInputElement).value).toBe(
      'Cartógrafo do Vale',
    );
    expect(
      (field(el, 'table_background.skills[0]') as HTMLSelectElement).selectedOptions[0].text,
    ).toBe('Investigação');
    expect(
      (field(el, 'table_background.skills[1]') as HTMLSelectElement).selectedOptions[0].text,
    ).toBe('Percepção');
    expect(text(field(el, 'table_background.tools'))).toContain('Ferramentas de cartógrafo');
    expect((field(el, 'table_background.feature.name_pt') as HTMLInputElement).value).toBe(
      'Mapas na memória',
    );
  });

  it('writes "Como os jogadores veem" with the same text as the player\'s page, tools named, with final stops', () => {
    const { el } = setup();
    const rows = Array.from(el.querySelectorAll('.preview .rows__row')).map(
      (r) => `${r.querySelector('dt')!.textContent}: ${r.querySelector('dd')!.textContent}`,
    );
    expect(rows).toContain('Perícias: Investigação, Percepção');
    expect(rows).toContain('Ferramentas: Ferramentas de cartógrafo');
    expect(text(el.querySelector('.preview')!)).toContain(
      'Mapas na memória. Você lembra o desenho de qualquer lugar que já mapeou.',
    );
    expect(text(el.querySelector('.preview')!)).not.toMatch(/\b(proficiency|skill|language):/);
  });

  it("sends the whole entry: both skills, the tools, and a note that says nothing of its own takes the feature's text", async () => {
    const { fixture, el } = setup();
    save.mockResolvedValue({ entry: stored(), affected: [] });
    click(el, 'Salvar antecedente');
    await settle(fixture, 3);
    const [, sent, body] = save.mock.calls[0];
    expect(sent.key).toBe('background:cart-grafo-do-vale@mesa');
    expect(body.case).toBe('tableBackground');
    expect(body.value).toMatchObject({
      namePt: 'Cartógrafo do Vale',
      skills: ['skill:investigation', 'skill:perception'],
      tools: ['proficiency:cartographers-tools'],
      languageChoices: 1,
      equipmentPt: 'Um estojo de mapas, tinta e 10 PO',
    });
    expect(body.value.feature.effects).toEqual([
      { type: 'note', textPt: 'Você lembra o desenho de qualquer lugar que já mapeou.' },
    ]);
  });

  it("puts the server's paths back on their inputs: a skill, the tools, the equipment, the feature's name and its effect", async () => {
    const { fixture, el } = setup();
    save.mockRejectedValue(
      refusal(
        ['table_background.skills[1]', 'dangling_reference'],
        ['table_background.tools[0]', 'dangling_reference'],
        ['table_background.equipment_pt', 'bad_text'],
        ['table_background.feature.name_pt', 'bad_name'],
        ['table_background.feature.effects[0]', 'forbidden_effect'],
      ),
    );
    click(el, 'Salvar antecedente');
    await settle(fixture, 3);
    const alert = text(el.querySelector('[role="alert"]')!);
    expect(alert).toContain('5 campos precisam de ajuste');
    expect(text(field(el, 'table_background.skills[1]').closest('app-select-field')!)).toContain(
      'Esta perícia não existe.',
    );
    expect(
      text(field(el, 'table_background.tools').closest('app-pick-list')!.parentElement!),
    ).toContain('Esta não é uma ferramenta do SRD.');
    expect(text(field(el, 'table_background.equipment_pt').closest('app-text-field')!)).toContain(
      'O equipamento é longo demais.',
    );
    expect(
      text(field(el, 'table_background.feature.name_pt').closest('app-text-field')!),
    ).toContain('O nome tem de 1 a 60 letras.');
    expect(text(el.querySelector('app-feature-editor')!)).toContain(
      'Este efeito não está no menu.',
    );
    // Every message the summary counts is on the screen, and the focus is on the first.
    expect(document.activeElement).toBe(field(el, 'table_background.skills[1]'));
  });

  it('a new background starts with two empty skills and the feature as "Só texto"', () => {
    const { el } = setup(false);
    expect((field(el, 'table_background.skills[0]') as HTMLSelectElement).selectedIndex).toBe(0);
    expect(
      (field(el, 'table_background.feature.effects[0].type') as HTMLSelectElement)
        .selectedOptions[0].text,
    ).toBe('Só texto');
    expect(text(el.querySelector('.preview')!)).toContain('Novo antecedente');
  });
});
