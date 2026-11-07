import { ComponentFixture, TestBed } from '@angular/core/testing';

import { create } from '@bufbuild/protobuf';

import {
  Ability,
  type FeatOption,
  FeatOptionSchema,
  FeatUnmetKind,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { featOptions } from '../../core/levelup/levelup-testing';
import { FeatPicker } from './feat-picker';

describe('FeatPicker', () => {
  let fixture: ComponentFixture<FeatPicker>;
  const picked: string[] = [];
  const toggled: Ability[] = [];

  function setup(over: { feats?: FeatOption[]; selected?: string; abilities?: Ability[] } = {}) {
    picked.length = 0;
    toggled.length = 0;
    TestBed.resetTestingModule();
    fixture = TestBed.createComponent(FeatPicker);
    fixture.componentRef.setInput('feats', over.feats ?? featOptions());
    fixture.componentRef.setInput('selected', over.selected ?? '');
    fixture.componentRef.setInput('abilities', over.abilities ?? []);
    fixture.componentInstance.picked.subscribe((k) => picked.push(k));
    fixture.componentInstance.abilityToggled.subscribe((a) => toggled.push(a));
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  const text = (e: Element) => (e.textContent ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ');
  const cards = (el: HTMLElement) => Array.from(el.querySelectorAll('.card'));

  it('lists each feat with its prerequisite, the "Da mesa" tag and the increase it gives', () => {
    const el = setup();
    const [grappler, athlete] = cards(el).map(text);
    expect(grappler).toContain('Agarrador');
    expect(grappler).toContain('Pede: Força 13.');
    expect(grappler).not.toContain('Da mesa');
    expect(athlete).toContain('Atleta');
    expect(athlete).toContain('Da mesa');
    expect(athlete).toContain('Não pede nada.');
    expect(athlete).toContain('Escolha 1 habilidade entre Força e Destreza: +1.');
  });

  it('shows the feat the character does not qualify for, off, with every reason in Portuguese', () => {
    const el = setup();
    const card = cards(el)[2];
    expect(card.classList).toContain('card--off');
    expect(card.querySelector<HTMLInputElement>('input')!.disabled).toBe(true);
    expect(text(card)).toContain('Precisa de Força 15.');
    expect(text(card)).toContain('Precisa de nível 8 ou mais.');
  });

  it('shows the Portuguese text first, with "Ver em inglês" for the SRD English, and the table\'s own text as it is', () => {
    const el = setup();
    const [grappler, athlete] = cards(el);
    expect(text(grappler)).toContain('Você tem vantagem em jogadas de ataque');
    expect(text(grappler)).not.toContain('(em inglês)');
    expect(text(grappler)).toContain('Ver em inglês');
    expect(text(athlete)).toContain('Você corre e escala melhor.');
    expect(text(athlete)).not.toContain('(em inglês)');
    expect(text(athlete)).not.toContain('Ver em inglês');

    Array.from(grappler.querySelectorAll('button'))
      .find((b) => text(b).includes('Ver em inglês'))!
      .click();
    TestBed.tick();
    expect(text(grappler)).toContain('You have advantage on attack rolls');
    expect(text(grappler)).toContain('(em inglês)');
    expect(grappler.querySelector('[lang="en"]')).toBeTruthy();
    expect(text(athlete)).toContain('Você corre e escala melhor.');
  });

  it('shows the English, flagged, for an SRD feat with no translation yet', () => {
    const feats = featOptions();
    feats[0].descPt = [];
    feats[0].descPtMissing = true;
    const el = setup({ feats });
    const grappler = cards(el)[0];
    expect(text(grappler)).toContain('You have advantage on attack rolls');
    expect(text(grappler)).toContain('(em inglês)');
    expect(text(grappler)).not.toContain('Ver em inglês');
  });

  it('says which feat was picked', () => {
    const el = setup();
    cards(el)[1].querySelector<HTMLInputElement>('input')!.click();
    expect(picked).toEqual(['feat:atleta@mesa']);
    expect(setupChecked(setup({ selected: 'feat:grappler' }))).toEqual([true, false, false]);
  });

  function setupChecked(el: HTMLElement) {
    return cards(el).map((c) => c.querySelector<HTMLInputElement>('input[type="radio"]')!.checked);
  }

  it('lets the player tick the abilities of the picked feat, from its own list', () => {
    const el = setup({ selected: 'feat:atleta@mesa' });
    const boxes = Array.from(el.querySelectorAll<HTMLInputElement>('.ability input'));
    expect(Array.from(el.querySelectorAll('.ability')).map((a) => text(a).trim())).toEqual([
      'Força',
      'Destreza',
    ]);
    boxes[1].click();
    expect(toggled).toEqual([Ability.DEXTERITY]);
  });

  it('asks for no tick when the feat raises every ability it lists, or none', () => {
    const feats = featOptions();
    feats[1].increase!.count = 2;
    const el = setup({ feats, selected: 'feat:atleta@mesa' });
    expect(el.querySelector('.ability')).toBeNull();
    expect(text(cards(el)[1])).toContain('Aumenta Força e Destreza em +1.');
    expect(el.querySelector('.ability')).toBeNull();
    expect(setup({ selected: 'feat:grappler' }).querySelector('.ability')).toBeNull();
  });

  it('turns off the abilities that do not fit once the feat has all it asks for', () => {
    const feats = featOptions();
    feats[1].increase = {
      ...feats[1].increase!,
      count: 2,
      from: [Ability.STRENGTH, Ability.DEXTERITY, Ability.CONSTITUTION],
    } as never;
    const el = setup({
      feats,
      selected: 'feat:atleta@mesa',
      abilities: [Ability.STRENGTH, Ability.DEXTERITY],
    });
    const boxes = Array.from(el.querySelectorAll<HTMLInputElement>('.ability input'));
    expect(boxes.map((b) => b.disabled)).toEqual([false, false, true]);
  });

  it('puts the feats the character meets first under "Disponíveis" and the others under "Ainda não cumpre", each with its reason', () => {
    const el = setup();
    const headings = Array.from(el.querySelectorAll('.group')).map(text);
    expect(headings).toEqual(['Disponíveis', 'Ainda não cumpre']);
    const lists = Array.from(el.querySelectorAll('.cards'));
    expect(lists.map((l) => l.querySelectorAll('.card').length)).toEqual([2, 1]);
    expect(text(lists[1])).toContain('Precisa de Força 15.');
  });

  it('filters by name with no case and no accents, and says how many were found', () => {
    const el = setup();
    const search = el.querySelector<HTMLInputElement>('input')!;
    search.value = 'AGARRADOR';
    search.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(cards(el).map((c) => text(c).includes('Agarrador'))).toEqual([true]);
    expect(el.querySelector('.group')?.textContent).toBe('Disponíveis');
    expect(text(el.querySelector('[role="status"]')!)).toBe('1 talento encontrado.');
    search.value = 'mestre das';
    search.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(Array.from(el.querySelectorAll('.group')).map(text)).toEqual(['Ainda não cumpre']);
    expect(text(el.querySelector('[role="status"]')!)).toBe('1 talento encontrado.');
  });

  it('says so when no feat has the name', () => {
    const el = setup();
    const search = el.querySelector<HTMLInputElement>('input')!;
    search.value = 'zzz';
    search.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(cards(el)).toEqual([]);
    expect(text(el)).toContain('Nenhum talento com esse nome.');
    expect(text(el.querySelector('[role="status"]')!)).toBe('Nenhum talento com esse nome.');
  });

  it('turns off an ability the server says would pass 20, and notes it for a feat that raises every one', () => {
    const feats = featOptions();
    feats[1].cappedAbilities = [Ability.STRENGTH];
    const el = setup({ feats, selected: 'feat:atleta@mesa' });
    const boxes = Array.from(el.querySelectorAll<HTMLInputElement>('.ability input'));
    expect(boxes.map((b) => b.disabled)).toEqual([true, false]);
    expect(text(el.querySelector('.ability')!)).toContain('Força · passaria de 20');
    feats[1].increase!.count = 2;
    const fixed = setup({ feats });
    expect(text(cards(fixed)[1])).toContain('O aumento para em 20 em Força.');
    expect(fixed.querySelector('.ability')).toBeNull();
  });

  it("writes the server's wording for race, size and lack, and never prints a key for a hidden condition", () => {
    const base = { desc: ['Texto.'], descPt: ['Texto.'], descPtOnly: true };
    const hidden = create(FeatOptionSchema, {
      ...base,
      key: 'feat:quiet',
      namePt: 'Passo Silencioso',
      prerequisite: { minimums: { dexterity: 13 } },
      lacksFeatureHidden: true,
      qualifies: false,
      unmet: [{ kind: FeatUnmetKind.LACKS_FEATURE, hidden: true }],
    });
    const open = create(FeatOptionSchema, {
      ...base,
      key: 'feat:small',
      namePt: 'Pequeno',
      prerequisite: {
        minimums: { dexterity: 13 },
        races: ['race:gnome', 'race:halfling'],
        size: 'Small',
        lacksFeatureKey: 'feat:grappler',
      },
      lacksFeatureNamePt: 'Agarrador',
      qualifies: false,
      unmet: [
        { kind: FeatUnmetKind.RACE, keys: ['race:gnome', 'race:halfling'] },
        { kind: FeatUnmetKind.SIZE, key: 'Small' },
        { kind: FeatUnmetKind.LACKS_FEATURE, key: 'feat:grappler', namePt: 'Agarrador' },
      ],
    });
    const names: Record<string, string> = { 'race:gnome': 'Gnomo', 'race:halfling': 'Halfling' };
    const el = setup({ feats: [hidden, open] });
    fixture.componentRef.setInput('nameOf', (k: string) => names[k] ?? k);
    fixture.detectChanges();
    const [h, o] = cards(el).map(text);
    expect(h).toContain('Pede: Destreza 13 e uma condição que a mesa não mostra.');
    expect(h).toContain('Pede uma condição que a mesa não mostra.');
    expect(h).not.toMatch(/feat:|race:/);
    expect(o).toContain(
      'Pede: Destreza 13, ser Gnomo ou Halfling, ter tamanho pequeno e não ter Agarrador.',
    );
    expect(o).toContain('Só para Gnomo ou Halfling.');
    expect(o).toContain('Só para tamanho pequeno.');
    expect(o).toContain('Não pode ter Agarrador.');
    expect(cards(el)[1].querySelectorAll('.card__why li')).toHaveLength(3);
    // A feat the character cannot take yet is dimmed and carries its reasons, not the text to read.
    expect(cards(el)[1].querySelector('details')).toBeNull();
  });
});
