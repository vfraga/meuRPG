import { ComponentFixture, TestBed } from '@angular/core/testing';

import { create } from '@bufbuild/protobuf';

import {
  OptionSwitchEntrySchema,
  TableContentKind,
  TableEntrySchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { catalog } from '../../../core/content/content-testing';
import { menu } from '../../../core/content/content-testing-menu';
import { emptyEffect } from '../../../core/content/effect-draft';
import { SwitchesStore } from '../../../core/content/switches-store';
import { emptySummon } from '../../../core/content/summon-draft';
import { emptyFeature, type FeatureDraft, withAsks } from '../../../core/content/feature-draft';
import { FeaturePreview } from './feature-preview';

describe('"Como os jogadores veem" for the open feature (#580, board 580-03)', () => {
  let fixture: ComponentFixture<FeaturePreview>;
  const option = (key: string, name: string, text: string): FeatureDraft => ({
    ...emptyFeature(),
    key,
    name,
    text,
  });
  const feature = (): FeatureDraft => ({
    ...emptyFeature(),
    name: 'Truques de lâmina',
    text: 'Escolha entre as opções da lista.',
    effects: [{ ...emptyEffect('choice'), choice: 'feature', count: 2 }],
    options: [
      option('feature:golpe@mesa', 'Golpe do Corvo', 'Soma o dado ao dano.'),
      withAsks(option('feature:asa@mesa', 'Asa Partida', 'Soma o dado ao ataque.'), {
        classKey: 'class:paladin',
        classLevel: '7',
        requiresOptions: ['feature:golpe@mesa'],
      }),
      option('', '', 'Sem nome não é salva.'),
    ],
  });
  const text = () => (fixture.nativeElement.textContent as string).replace(/\s+/g, ' ');

  function setFeature(f: FeatureDraft | null): void {
    fixture.componentRef.setInput('features', f ? [{ id: 'f1', level: 1, feature: f }] : []);
    fixture.componentRef.setInput('openId', f ? 'f1' : '');
  }

  function setup(f: FeatureDraft | null): void {
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({ providers: [SwitchesStore] });
    fixture = TestBed.createComponent(FeaturePreview);
    setFeature(f);
    fixture.componentRef.setInput('menu', menu());
    fixture.componentRef.setInput('catalog', catalog());
    fixture.detectChanges();
  }

  it('draws nothing while no feature is open', () => {
    setup(null);
    expect(fixture.nativeElement.querySelector('section')).toBeNull();
  });

  it('names the feature with "Da mesa", the choice, and each option with the "Pede" line the players read', () => {
    setup(feature());
    const el: HTMLElement = fixture.nativeElement;
    expect(el.querySelector('h2')?.textContent).toBe('Como os jogadores veem');
    expect(el.querySelector('.preview__name span')?.textContent).toBe('Truques de lâmina');
    expect(el.querySelector('.preview__name .mr-tag')?.textContent).toContain('Da mesa');
    expect(
      Array.from(el.querySelectorAll('dl div')).map((r) => [
        r.querySelector('dt')?.textContent,
        r.querySelector('dd')?.textContent,
      ]),
    ).toEqual([['Escolha', '2 de 2 opções']]);
    expect(text()).toContain('Golpe do Corvo. Soma o dado ao dano.');
    expect(text()).toContain(
      'Asa Partida. Soma o dado ao ataque. Pede Paladino 7 e Golpe do Corvo.',
    );
    // An option with no name is not sent, so players never read it.
    expect(text()).not.toContain('Sem nome não é salva');
    expect(el.querySelector('section')?.getAttribute('aria-labelledby')).toBe(
      el.querySelector('h2')?.id,
    );
  });

  it('follows the draft as it changes', () => {
    setup(feature());
    const f = feature();
    f.options[1] = withAsks(f.options[1], {
      classKey: 'class:paladin',
      classLevel: '9',
      requiresOptions: [],
    });
    setFeature(f);
    fixture.detectChanges();
    expect(text()).toContain('Pede Paladino 9.');
    expect(text()).not.toContain('Golpe do Corvo. Soma o dado ao dano. Pede');
  });

  describe('reads what a player reads: a class, pact, spell or option that is off or archived is not named (#580)', () => {
    const entry = (
      key: string,
      kind: TableContentKind,
      over: { off?: boolean; archived?: boolean },
    ) => create(TableEntrySchema, { key, kind, namePt: key, ...over });
    const asking = (): FeatureDraft => {
      const f = feature();
      f.options[1] = withAsks(f.options[1], {
        classKey: 'class:paladin',
        classLevel: '7',
        requiresPact: 'feature:pacto-da-lamina',
        requiresSpell: 'spell:light',
        requiresOptions: ['feature:golpe@mesa', 'feature:oculta@mesa'],
      });
      return f;
    };

    it('names every key while all of them are on', () => {
      setup(asking());
      fixture.componentRef.setInput('entries', []);
      fixture.detectChanges();
      expect(text()).toContain('Pede Paladino 7');
      expect(text()).toContain('Golpe do Corvo');
    });

    it('drops the class that is off, the pact that is archived, the spell that is off and the option that is archived', () => {
      setup(asking());
      fixture.componentRef.setInput('entries', [
        entry('class:paladin', TableContentKind.CLASS, { off: true }),
        entry('feature:pacto-da-lamina', TableContentKind.OPTION, { archived: true }),
        entry('spell:light', TableContentKind.SPELL, { off: true }),
        entry('feature:oculta@mesa', TableContentKind.OPTION, { archived: true }),
      ]);
      fixture.detectChanges();
      expect(text()).toContain('Asa Partida. Soma o dado ao ataque. Pede Golpe do Corvo.');
      expect(text()).not.toContain('Paladino');
      expect(text()).not.toContain('pacto-da-lamina');
      expect(text()).not.toContain('spell:light');
      expect(text()).not.toContain('oculta');
    });

    it('says nothing of what an option asks when every key it asks for is hidden', () => {
      setup(asking());
      fixture.componentRef.setInput('entries', [
        entry('class:paladin', TableContentKind.CLASS, { archived: true }),
        entry('feature:pacto-da-lamina', TableContentKind.OPTION, { off: true }),
        entry('spell:light', TableContentKind.SPELL, { archived: true }),
        entry('feature:oculta@mesa', TableContentKind.OPTION, { off: true }),
        entry('feature:golpe@mesa', TableContentKind.OPTION, { off: true }),
      ]);
      fixture.detectChanges();
      expect(text()).toContain('Asa Partida. Soma o dado ao ataque.');
      expect(text()).not.toContain('Pede');
    });
  });

  describe('what the web can know a player reads (#580)', () => {
    const sw = (key: string, over: { off?: boolean; hidden?: boolean }) =>
      create(OptionSwitchEntrySchema, { key, namePt: key, ...over });
    const asking = (): FeatureDraft => {
      const f = feature();
      f.options[1] = withAsks(f.options[1], {
        classKey: 'class:paladin',
        classLevel: '7',
        requiresOptions: ['feature:golpe@mesa'],
      });
      return f;
    };

    it('leaves the summon line out', () => {
      setup({ ...feature(), summon: { ...emptySummon(), creatureKeys: ['monster:wolf'] } });
      expect(text()).not.toContain('Invoca');
    });

    it('does not name an SRD class the master switched off, read from the switch list', () => {
      setup(asking());
      TestBed.inject(SwitchesStore).options.set([sw('class:paladin', { off: true, hidden: true })]);
      fixture.detectChanges();
      expect(text()).not.toContain('Paladino');
      expect(text()).toContain('Pede Golpe do Corvo.');
    });

    it('leaves out an entry hidden through its owner, though its own switch is on', () => {
      setup(asking());
      TestBed.inject(SwitchesStore).options.set([
        sw('feature:golpe@mesa', { off: false, hidden: true }),
      ]);
      fixture.detectChanges();
      expect(text()).not.toContain('Pede Paladino 7 e Golpe do Corvo');
      expect(text()).toContain('Pede Paladino 7.');
    });

    it('says in one line that the switches could not be read, and only then', () => {
      setup(asking());
      expect(text()).not.toContain('Não deu para ler');
      TestBed.inject(SwitchesStore).unread.set(true);
      fixture.detectChanges();
      expect(text()).toContain('Não deu para ler quais opções do SRD estão desligadas');
    });

    it('marks itself as not saved while a refusal stands', () => {
      setup(asking());
      expect(text()).not.toContain('Ainda não salvo');
      fixture.componentRef.setInput('unsaved', true);
      fixture.detectChanges();
      expect(text()).toContain('Ainda não salvo');
    });
  });

  it('calls a feature with no name "Nova característica"', () => {
    setup({ ...emptyFeature() });
    expect(text()).toContain('Nova característica');
  });
});
