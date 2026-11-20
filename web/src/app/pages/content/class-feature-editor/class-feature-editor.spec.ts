import { TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  TableClassFeatureSchema,
  TableContentKind,
  TableContentRefusalSchema,
  TableContentViolationSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { TableContentClient } from '../../../core/content/content-client';
import { catalog, entry } from '../../../core/content/content-testing';
import { menu } from '../../../core/content/content-testing-menu';
import { FeatureEditor } from '../../../shared/feature-editor/feature-editor';
import { ClassFeatureEditor } from './class-feature-editor';
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

describe('ClassFeatureEditor (MR-025)', () => {
  const save = vi.fn();
  const stored = () =>
    entry(TableContentKind.CLASS_FEATURE, 'Rastro do Corvo', {
      body: {
        case: 'tableClassFeature',
        value: create(TableClassFeatureSchema, {
          namePt: 'Rastro do Corvo',
          descPt: ['Você segue o rastro de um corvo.'],
          classKey: 'class:wizard',
          level: 1,
          replacesFeatureKey: 'feature:favored-enemy-1-type',
        }),
      },
    });

  function setup(existing = true) {
    save.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [{ provide: TableContentClient, useValue: { save } }],
    });
    const fixture = TestBed.createComponent(ClassFeatureEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('catalog', catalog());
    fixture.componentRef.setInput('menu', menu());
    if (existing) fixture.componentRef.setInput('entry', stored());
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

  it('does not offer "Ou as opções de outra característica": the server refuses from_feature on a table class feature (#574)', () => {
    const { fixture } = setup();
    const editor = fixture.debugElement.query(By.directive(FeatureEditor));
    expect(editor.componentInstance.offerFeatures()).toBe(false);
  });

  it('draws the form from the entry: name, text, the class read-only, the level and what it replaces', () => {
    const { el } = setup();
    expect((field(el, 'table_class_feature.name_pt') as HTMLInputElement).value).toBe(
      'Rastro do Corvo',
    );
    expect(field(el, 'table_class_feature.class_key').tagName).not.toBe('SELECT');
    expect((field(el, 'table_class_feature.level') as HTMLInputElement).value).toBe('1');
    expect((field(el, 'table_class_feature.replaces_feature_key') as HTMLInputElement).value).toBe(
      'feature:favored-enemy-1-type',
    );
  });

  it('writes "Como os jogadores veem" from the draft, with the class and the replaced feature', () => {
    const { el } = setup();
    const preview = text(el.querySelector('.preview')!);
    expect(preview).toContain('Você segue o rastro de um corvo.');
    expect(preview).toContain('Favored enemy 1 type');
  });

  it('a new feature picks the class and sends the whole body', async () => {
    const { fixture, el } = setup(false);
    const cls = field(el, 'table_class_feature.class_key') as HTMLSelectElement;
    cls.value = 'class:wizard';
    cls.dispatchEvent(new Event('change'));
    const level = field(el, 'table_class_feature.level') as HTMLInputElement;
    level.value = '3';
    level.dispatchEvent(new Event('input'));
    const name = field(el, 'table_class_feature.name_pt') as HTMLInputElement;
    name.value = 'Rastro do Corvo';
    name.dispatchEvent(new Event('input'));
    save.mockResolvedValue({ entry: stored(), affected: [] });
    click(el, 'Salvar característica');
    await settle(fixture, 3);
    const [, sent, body] = save.mock.calls[0];
    expect(sent).toBeNull();
    expect(body.case).toBe('tableClassFeature');
    expect(body.value.classKey).toBe('class:wizard');
    expect(body.value.level).toBe(3);
    expect(body.value.replacesFeatureKey).toBe('');
  });

  it("puts the server's paths back on their inputs: the level and what it replaces", async () => {
    const { fixture, el } = setup();
    save.mockRejectedValue(
      refusal(
        ['table_class_feature.replaces_feature_key', 'dangling_reference'],
        ['table_class_feature.level', 'bad_value'],
      ),
    );
    click(el, 'Salvar característica');
    await settle(fixture, 3);
    expect(text(el.querySelector('[role="alert"]')!)).toContain('2 campos precisam de ajuste');
    expect(text(field(el, 'table_class_feature.level').closest('app-text-field')!)).toContain(
      'O nível vai de 1 a 20',
    );
    expect(
      text(field(el, 'table_class_feature.replaces_feature_key').closest('app-text-field')!),
    ).toContain('A classe não tem essa característica');
  });

  it('a new feature starts empty and is called "Nova característica" in the preview', () => {
    const { el } = setup(false);
    expect((field(el, 'table_class_feature.name_pt') as HTMLInputElement).value).toBe('');
    expect(text(el.querySelector('.preview')!)).toContain('Nova característica');
  });
});
