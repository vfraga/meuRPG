import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';

import {
  type ChoiceGroup,
  ChoiceGroupSchema,
  ChoiceKind,
  ChoiceOptionSchema,
  ChoiceOrigin,
  ChoiceSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { ChoiceGroups, type ChoiceSelection } from './choice-groups';
import { applySelection } from './choice-picks';

function option(key: string, reasonPt = '') {
  return create(ChoiceOptionSchema, {
    key,
    storedKey: key,
    namePt: key.toUpperCase(),
    summaryPt: `s-${key}`,
    reasonPt,
  });
}

function group(picks: number, picked: string[] = []) {
  return create(ChoiceGroupSchema, {
    origin: ChoiceOrigin.CLASS,
    sourceNamePt: 'Estilo de Luta',
    level: 1,
    choices: [
      create(ChoiceSchema, {
        key: 'c1',
        featureKey: 'f1',
        kind: ChoiceKind.OPTIONS,
        titlePt: 'Estilo de Luta',
        labelPt: 'Estilo de Luta',
        picks,
        picked,
        missing: Math.max(picks - picked.length, 0),
        options: [option('a'), option('b'), option('c', 'Pede nível 5')],
      }),
    ],
  });
}

describe('ChoiceGroups', () => {
  function setup(g = group(1)) {
    const fixture = TestBed.createComponent(ChoiceGroups);
    fixture.componentRef.setInput('groups', [g]);
    const sent: ChoiceSelection[] = [];
    fixture.componentInstance.selected.subscribe((s) => sent.push(s));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const cards = () => Array.from(el.querySelectorAll<HTMLElement>('.card'));
    return { fixture, el, cards, sent };
  }

  it('draws a one-pick choice as radios and reports the pick', () => {
    const { cards, sent, fixture } = setup();
    expect(cards().map((c) => c.getAttribute('role'))).toEqual(['radio', 'radio', 'radio']);
    cards()[1].click();
    fixture.detectChanges();
    expect(sent[0].optionKeys).toEqual(['b']);
    expect(cards()[1].getAttribute('aria-checked')).toBe('true');
  });

  it('keeps a blocked option focusable, disabled and described by its reason', () => {
    const { cards, sent } = setup();
    const blocked = cards()[2];
    expect(blocked.getAttribute('aria-disabled')).toBe('true');
    expect(blocked.getAttribute('tabindex')).not.toBeNull();
    expect(blocked.textContent).toContain('Pede nível 5');
    blocked.click();
    expect(sent.length).toBe(0);
  });

  it('blocks the unpicked options once a multiple choice is full', () => {
    const { cards, sent } = setup(group(2, ['a', 'b']));
    expect(cards()[0].getAttribute('role')).toBe('checkbox');
    expect(cards()[2].getAttribute('aria-disabled')).toBe('true');
    cards()[0].click();
    expect(sent[0].optionKeys).toEqual(['b']);
  });

  describe('a language or a tool the race or the background lets the player pick', () => {
    function pickGroup(origin: ChoiceOrigin, sourceNamePt: string, key: string, picks: number) {
      const stored = (k: string) => `${key}=${k}`;
      return create(ChoiceGroupSchema, {
        origin,
        sourceKey: 'race:half-elf',
        sourceNamePt,
        choices: [
          create(ChoiceSchema, {
            key,
            featureKey: 'race:half-elf',
            kind: ChoiceKind.OPTIONS,
            titlePt: picks === 1 ? 'Idioma' : 'Idiomas',
            labelPt: `${picks === 1 ? 'Idioma' : 'Idiomas'} (${sourceNamePt})`,
            picks,
            missing: picks,
            options: [
              create(ChoiceOptionSchema, {
                key: 'language:elvish',
                storedKey: stored('language:elvish'),
                namePt: 'Élfico',
                reasonPt: 'Você já fala este idioma.',
              }),
              create(ChoiceOptionSchema, {
                key: 'language:dwarvish',
                storedKey: stored('language:dwarvish'),
                namePt: 'Anão',
              }),
              create(ChoiceOptionSchema, {
                key: 'language:giant',
                storedKey: stored('language:giant'),
                namePt: 'Gigante',
              }),
            ],
          }),
        ],
      });
    }

    it('is a pick-one list under the race, with the language the character speaks blocked and told why', () => {
      const { el, cards, sent, fixture } = setup(
        pickGroup(ChoiceOrigin.RACE, 'Meio-elfo', 'race:half-elf#language', 1),
      );
      expect(el.querySelector('.choice__origin')?.textContent).toContain(
        'Raça · Meio-elfo · 1 escolha',
      );
      expect(cards().map((c) => c.getAttribute('role'))).toEqual(['radio', 'radio', 'radio']);
      const card = (name: string) =>
        cards().find((c) => (c.textContent ?? '').replace(/\s+/g, ' ').trim().startsWith(name))!;
      expect(card('Élfico').textContent).toContain('Você já fala este idioma.');
      expect(card('Élfico').getAttribute('aria-disabled')).toBe('true');
      card('Élfico').click();
      expect(sent).toEqual([]);

      card('Anão').click();
      fixture.detectChanges();
      expect(sent[0].optionKeys).toEqual(['language:dwarvish']);
      expect(applySelection([], sent[0].choice, sent[0].optionKeys)).toEqual([
        'race:half-elf#language=language:dwarvish',
      ]);
    });

    it("is a pick-many list under the background's name", () => {
      const { el, cards, sent } = setup(
        pickGroup(ChoiceOrigin.BACKGROUND, 'Acólito', 'background:acolyte#language', 2),
      );
      expect(el.querySelector('.choice__origin')?.textContent).toContain(
        'Antecedente · Acólito · 2 escolhas',
      );
      expect(cards().map((c) => c.getAttribute('role'))).toEqual([
        'checkbox',
        'checkbox',
        'checkbox',
      ]);
      cards()[1].click();
      expect(sent[0].optionKeys).toEqual(['language:dwarvish']);
    });
  });

  describe('two choices of one round that offer the same language', () => {
    const NAMES: Record<string, string> = {
      'language:elvish': 'Élfico',
      'language:dwarvish': 'Anão',
      'language:abyssal': 'Abissal',
    };
    function languages(
      origin: ChoiceOrigin,
      sourceNamePt: string,
      key: string,
      picks: number,
      picked: string[] = [],
      extra: Partial<{ unmet: string[]; blocked: string[] }> = {},
    ) {
      return create(ChoiceGroupSchema, {
        origin,
        sourceNamePt,
        choices: [
          create(ChoiceSchema, {
            key,
            featureKey: key,
            kind: ChoiceKind.OPTIONS,
            titlePt: picks === 1 ? 'Idioma' : 'Idiomas',
            labelPt: `${picks === 1 ? 'Idioma' : 'Idiomas'} (${sourceNamePt})`,
            picks,
            picked,
            unmet: extra.unmet ?? [],
            missing: Math.max(picks - picked.length, 0),
            advisory: true,
            options: Object.entries(NAMES).map(([k, namePt]) =>
              create(ChoiceOptionSchema, {
                key: k,
                storedKey: `${key}=${k}`,
                namePt,
                reasonPt: (extra.blocked ?? []).includes(k) ? 'Você já escolheu esta opção.' : '',
              }),
            ),
          }),
        ],
      });
    }
    const race = (picked: string[] = [], extra = {}) =>
      languages(ChoiceOrigin.RACE, 'Humano', 'race:human#language', 1, picked, extra);
    const background = (picked: string[] = [], extra = {}) =>
      languages(
        ChoiceOrigin.BACKGROUND,
        'Acólito',
        'background:acolyte#language',
        2,
        picked,
        extra,
      );

    function setupBoth(groups: ChoiceGroup[], done: number | null = null) {
      const fixture = TestBed.createComponent(ChoiceGroups);
      fixture.componentRef.setInput('groups', groups);
      if (done !== null) {
        fixture.componentRef.setInput('done', done);
        fixture.componentRef.setInput('total', 3);
      }
      const sent: ChoiceSelection[] = [];
      fixture.componentInstance.selected.subscribe((s) => sent.push(s));
      fixture.detectChanges();
      const el = fixture.nativeElement as HTMLElement;
      const sections = () => Array.from(el.querySelectorAll<HTMLElement>('section.choice'));
      const card = (section: number, name: string) =>
        Array.from(sections()[section].querySelectorAll<HTMLElement>('.card')).find(
          (c) => (c.querySelector('.card__name')?.textContent ?? '').trim() === name,
        )!;
      return { fixture, el, sent, sections, card };
    }

    it('does not offer, in the second choice, the language the first took before the server answers', () => {
      const { fixture, sent, card } = setupBoth([race(), background()]);
      card(0, 'Abissal').click();
      fixture.detectChanges();
      expect(sent.map((s) => s.optionKeys)).toEqual([['language:abyssal']]);
      expect(card(1, 'Abissal').getAttribute('aria-disabled')).toBe('true');
      expect(card(1, 'Abissal').textContent).toContain('Você já escolheu esta opção.');
      card(1, 'Abissal').click();
      fixture.detectChanges();
      expect(sent.length).toBe(1);
      card(1, 'Anão').click();
      fixture.detectChanges();
      expect(sent[1].optionKeys).toEqual(['language:dwarvish']);
    });

    it('does not offer, in the first choice, the language the second took', () => {
      const { fixture, sent, card } = setupBoth([race(), background()]);
      card(1, 'Abissal').click();
      fixture.detectChanges();
      expect(card(0, 'Abissal').getAttribute('aria-disabled')).toBe('true');
      card(0, 'Abissal').click();
      expect(sent.length).toBe(1);
    });

    it('frees the language again when the first choice lets it go', () => {
      const { fixture, card } = setupBoth([race(), background()]);
      card(1, 'Abissal').click();
      fixture.detectChanges();
      card(1, 'Abissal').click();
      fixture.detectChanges();
      expect(card(0, 'Abissal').getAttribute('aria-disabled')).toBeNull();
    });

    it('never disables a checked option, says it is a repeat and does not count it', () => {
      const { sent, sections, card, el, fixture } = setupBoth(
        [
          race(['language:abyssal']),
          background(['language:abyssal', 'language:dwarvish'], {
            blocked: ['language:abyssal'],
          }),
        ],
        3,
      );
      const repeated = card(1, 'Abissal');
      expect(repeated.getAttribute('aria-checked')).toBe('true');
      expect(repeated.getAttribute('aria-disabled')).toBeNull();
      expect(sections()[1].textContent).toContain('Abissal já foi escolhido em Idioma (raça).');
      // The step is not complete: the repeat is not a pick made.
      expect(sections()[1].querySelector('.choice__origin')?.textContent).toContain('1 de 2');
      expect(sections()[1].hasAttribute('data-pending')).toBe(true);
      expect(el.querySelector('.choices__counter')?.textContent).toContain(
        'Escolhas feitas: 2 de 3',
      );
      repeated.click();
      fixture.detectChanges();
      expect(sent[0].optionKeys).toEqual(['language:dwarvish']);
      expect(sections()[1].textContent).not.toContain('já foi escolhido em');
      expect(el.querySelector('.choices__counter')?.textContent).toContain(
        'Escolhas feitas: 3 de 3',
      );
    });
  });

  it('leads to the choice that stops the sheet before one that only warns', () => {
    const languages = create(ChoiceGroupSchema, {
      origin: ChoiceOrigin.BACKGROUND,
      sourceNamePt: 'Acólito',
      choices: [
        create(ChoiceSchema, {
          key: 'background:acolyte#language',
          kind: ChoiceKind.OPTIONS,
          titlePt: 'Idiomas',
          labelPt: 'Idiomas (Acólito)',
          picks: 2,
          missing: 2,
          advisory: true,
          options: [option('language:giant')],
        }),
      ],
    });
    const fixture = TestBed.createComponent(ChoiceGroups);
    fixture.componentRef.setInput('groups', [languages, group(1)]);
    fixture.detectChanges();
    const titles = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll<HTMLElement>('.choice__title'),
    );
    expect(fixture.componentInstance.focusFirstPending()).toBe(true);
    expect(document.activeElement).toBe(titles[1]);
    expect((titles[1].textContent ?? '').trim()).toContain('Estilo de Luta');
  });

  describe('a feat the player chooses', () => {
    function featGroup(origin: ChoiceOrigin, sourceNamePt: string) {
      return create(ChoiceGroupSchema, {
        origin,
        sourceKey: 'race:humano-variante@mesa',
        sourceNamePt,
        choices: [
          create(ChoiceSchema, {
            key: 'trait:talento@mesa#feat',
            featureKey: 'trait:talento@mesa',
            kind: ChoiceKind.OPTIONS,
            titlePt: 'Talento',
            labelPt: 'Talento (Humano Variante)',
            picks: 1,
            missing: 1,
            options: [
              create(ChoiceOptionSchema, {
                key: 'feat:alerta@mesa',
                storedKey: 'trait:talento@mesa#feat=feat:alerta@mesa',
                namePt: 'Alerta',
                summaryPt: '+5 na Iniciativa.',
              }),
              create(ChoiceOptionSchema, {
                key: 'feat:lutador@mesa',
                storedKey: 'trait:talento@mesa#feat=feat:lutador@mesa',
                namePt: 'Lutador',
                summaryPt: 'Pede: Força 13.',
                reasonPt: 'Você ainda não cumpre o pré-requisito: Precisa de Força 13.',
              }),
            ],
          }),
        ],
      });
    }

    it('names the race that grants it and offers the feats with their text', () => {
      const { el, cards } = setup(featGroup(ChoiceOrigin.RACE, 'Humano Variante'));
      expect(el.querySelector('.choice__origin')?.textContent).toContain(
        'Raça · Humano Variante · 1 escolha',
      );
      expect(el.querySelector('.choice__title')?.textContent).toContain('Talento');
      expect(cards()[0].textContent).toContain('+5 na Iniciativa.');
    });

    it('shows the unmet prerequisite and does not let the feat be picked', () => {
      const { cards, sent } = setup(featGroup(ChoiceOrigin.BACKGROUND, 'Artesão'));
      expect(cards()[1].getAttribute('aria-disabled')).toBe('true');
      expect(cards()[1].textContent).toContain('Precisa de Força 13.');
      cards()[1].click();
      expect(sent.length).toBe(0);
    });

    it('says where a feat granted by a background or another feat comes from', () => {
      const bg = setup(featGroup(ChoiceOrigin.BACKGROUND, 'Artesão'));
      expect(bg.el.querySelector('.choice__origin')?.textContent).toContain(
        'Antecedente · Artesão',
      );
      const feat = setup(featGroup(ChoiceOrigin.FEAT, 'Mestre de Armas'));
      expect(feat.el.querySelector('.choice__origin')?.textContent).toContain(
        'Talento · Mestre de Armas',
      );
    });
  });

  describe('a trait of the race that is a choice of one (T4)', () => {
    function traitGroup(picked: string[] = []) {
      return create(ChoiceGroupSchema, {
        origin: ChoiceOrigin.RACE,
        sourceKey: 'race:corujeiro@mesa',
        sourceNamePt: 'Corujeiro',
        choices: [
          create(ChoiceSchema, {
            key: 'trait:sentidos@mesa',
            featureKey: 'trait:sentidos@mesa',
            kind: ChoiceKind.OPTIONS,
            titlePt: 'Sentidos da noite',
            labelPt: 'Sentidos da noite (Corujeiro)',
            picks: 1,
            picked,
            missing: 1 - picked.length,
            options: [
              create(ChoiceOptionSchema, {
                key: 'trait:sentidos--coruja@mesa',
                storedKey: 'trait:sentidos--coruja@mesa',
                namePt: 'Olhos de coruja',
                summaryPt: 'Visão no escuro 36 m.',
              }),
              create(ChoiceOptionSchema, {
                key: 'trait:sentidos--pena@mesa',
                storedKey: 'trait:sentidos--pena@mesa',
                namePt: 'Passo de pena',
                summaryPt: 'Proficiência em Furtividade.',
              }),
            ],
          }),
        ],
      });
    }
    const head = (el: HTMLElement) => ({
      title: el.querySelector('.choice__title')?.textContent?.trim(),
      origin: el.querySelector('.choice__origin')?.textContent?.replace(/\s+/g, ' ').trim(),
      tag: el.querySelector('.choice__bar .mr-tag')?.textContent?.trim(),
    });

    it('titles the card with the trait, says whose it is and that it is made once, and tags what is missing', () => {
      const { el, cards } = setup(traitGroup());
      expect(head(el)).toEqual({
        title: 'Sentidos da noite',
        origin: 'Traço de Corujeiro · escolha uma vez · 0 de 1',
        tag: 'Falta escolher',
      });
      expect(cards().map((c) => c.querySelector('.card__name')?.textContent)).toEqual([
        'Olhos de coruja',
        'Passo de pena',
      ]);
      expect(cards()[0].textContent).toContain('Visão no escuro 36 m.');
    });

    it('names the status as the description of the card', () => {
      const { el, fixture, cards } = setup(traitGroup());
      const section = el.querySelector('section.choice')!;
      const status = () => el.querySelector(`#${section.getAttribute('aria-describedby')}`);
      expect(status()?.textContent).toContain('Falta escolher');
      cards()[0].click();
      fixture.detectChanges();
      expect(status()?.textContent).toContain('Feito');
    });

    it('turns the tag into "Feito" once an option is picked', () => {
      const { el, fixture, cards } = setup(traitGroup());
      cards()[1].click();
      fixture.detectChanges();
      expect(head(el).tag).toContain('Feito');
      expect(head(el).origin).toBe('Traço de Corujeiro · escolha uma vez · 1 de 1');
    });

    it('keeps "Raça · …" for the choices of a race that are not its trait\'s options', () => {
      const g = traitGroup();
      g.choices[0].key = 'trait:sentidos@mesa#feat';
      const { el } = setup(g);
      expect(head(el).origin).toBe('Raça · Corujeiro · 1 escolha');
    });
  });
});
