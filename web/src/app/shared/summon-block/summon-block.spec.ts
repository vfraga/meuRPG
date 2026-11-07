import { ComponentFixture, TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import { CreatureSummarySchema, SpellDurationKind } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableContentKind,
  TableEntrySchema,
  TableSummonReplaces,
  TableSummonSchema,
  TableSummonTurnMode,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { type SummonDraft, draftToSummon, emptySummon } from '../../core/content/summon-draft';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { SummonBlock } from './summon-block';

describe('SummonBlock ("Invoca criaturas", board C2)', () => {
  const resources = [{ key: 'companheiro', label: 'Companheiro do Vale (1)' }];
  const entries = [
    create(TableEntrySchema, {
      key: 'creature:lobo-do-vale@mesa',
      namePt: 'Lobo do Vale',
      kind: TableContentKind.CREATURE,
    }),
  ];
  const statBlock = vi.fn();
  const search = vi.fn();
  const emitted: (SummonDraft | null)[] = [];
  let fixture: ComponentFixture<SummonBlock>;
  let el: HTMLElement;

  function on(over: Partial<SummonDraft> = {}): SummonDraft {
    return {
      ...emptySummon('companheiro'),
      creatureKeys: ['creature:lobo-do-vale@mesa', 'monster:wolf'],
      count: '1',
      maxLive: '1',
      ...over,
    };
  }

  function setup(
    summon: SummonDraft | null,
    over: { resources?: typeof resources; issues?: Record<string, string[]> } = {},
  ) {
    emitted.length = 0;
    statBlock.mockReset().mockResolvedValue({ summary: { namePt: 'Lobo' } });
    search.mockReset().mockResolvedValue({ creatures: [], total: 0 });
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [{ provide: CreaturesClient, useValue: { statBlock, search } }],
    });
    fixture = TestBed.createComponent(SummonBlock);
    fixture.componentRef.setInput('summon', summon);
    fixture.componentRef.setInput('resources', over.resources ?? resources);
    fixture.componentRef.setInput('basePath', 'table_class.levels[2].features[0]');
    fixture.componentRef.setInput('issuesOf', (path: string) => over.issues?.[path] ?? []);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('entries', entries);
    fixture.componentInstance.summonChange.subscribe((s) => emitted.push(s));
    fixture.detectChanges();
    el = fixture.nativeElement as HTMLElement;
  }

  const text = (e: Element) => (e.textContent ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ');
  const at = (field: string) =>
    `table_class.levels[2].features[0].summon${field ? '.' + field : ''}`;
  const field = (path: string) => el.querySelector<HTMLElement>(`[data-field="${path}"]`);
  const sw = (label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button[role="switch"]')).find((b) =>
      text(b.closest('.row')!).includes(label),
    )!;

  it('off: only the switch, "Desligado", and turning it on offers the board\'s defaults with the first resource', () => {
    setup(null);
    expect(sw('Invoca criaturas').getAttribute('aria-checked')).toBe('false');
    expect(text(el)).toContain('Desligado');
    expect(el.querySelector('app-summon-creatures')).toBeNull();
    sw('Invoca criaturas').click();
    expect(emitted).toHaveLength(1);
    expect(emitted[0]).toMatchObject({
      resourceKey: 'companheiro',
      count: '1',
      maxLive: '0',
      creatureKeys: [],
    });
  });

  it('on: the switch is "Ligado" and the block shows the line, the creatures, the three number fields, the turn and the time', () => {
    setup(on());
    expect(sw('Invoca criaturas').getAttribute('aria-checked')).toBe('true');
    const t = text(el);
    expect(t).toContain('Ligado');
    expect(t).toContain('Quem tem a característica a usa como uma magia de invocação');
    expect(t).toContain('De 1 a 8. Quem usa escolhe uma delas para cada criatura que vem.');
    for (const label of [
      'Quantas vêm',
      'No máximo ao mesmo tempo',
      'Gasta 1 uso de',
      'Turno',
      'Ao usar de novo',
      'Tempo',
      'Duração',
      'Concentração',
    ]) {
      expect(t).toContain(label);
    }
    expect(t).toContain('Fórmula do dono; de 1 a 20.');
    expect(t).toContain('0: sem limite. Até 40.');
  });

  it('turning it off sends no summon and the next turn-on brings back what was there', () => {
    setup(on({ count: '2' }));
    sw('Invoca criaturas').click();
    expect(emitted).toEqual([null]);
    fixture.componentRef.setInput('summon', null);
    fixture.detectChanges();
    sw('Invoca criaturas').click();
    expect(emitted[1]).toMatchObject({
      count: '2',
      creatureKeys: ['creature:lobo-do-vale@mesa', 'monster:wolf'],
    });
  });

  it('the chips: "Da mesa" on the table creature, "· SRD" on the SRD one, and each can be taken off', () => {
    setup(on());
    const chips = Array.from(el.querySelectorAll('.chip'));
    expect(chips).toHaveLength(2);
    expect(text(chips[0])).toContain('Lobo do Vale');
    expect(text(chips[0])).toContain('Da mesa');
    expect(text(chips[1])).toContain('· SRD');
    el.querySelector<HTMLButtonElement>('button[aria-label="Tirar Lobo do Vale"]')!.click();
    expect(emitted[0]).toMatchObject({ creatureKeys: ['monster:wolf'] });
  });

  it('names an SRD creature loaded with the entry by the bestiary', async () => {
    setup(on());
    await fixture.whenStable();
    fixture.detectChanges();
    expect(statBlock).toHaveBeenCalledWith('camp-1', 'monster:wolf');
    expect(text(el.querySelectorAll('.chip')[1])).toContain('Lobo');
  });

  it('takes 1 to 8 creatures: a ninth cannot be searched and none twice', () => {
    setup(on({ creatureKeys: Array.from({ length: 8 }, (_, i) => `monster:c${i}`) }));
    expect(el.querySelector<HTMLInputElement>('input[role="combobox"]')!.disabled).toBe(true);
    setup(on({ creatureKeys: ['monster:wolf'] }));
    expect(el.querySelector<HTMLInputElement>('input[role="combobox"]')!.disabled).toBe(false);
    const pick = fixture.debugElement.query(
      (d) => d.name === 'app-creature-pick',
    ).componentInstance;
    pick.picked.emit(create(CreatureSummarySchema, { key: 'monster:wolf', namePt: 'Lobo' }));
    expect(emitted).toHaveLength(0);
    pick.picked.emit(create(CreatureSummarySchema, { key: 'monster:bear', namePt: 'Urso' }));
    expect(emitted[0]).toMatchObject({ creatureKeys: ['monster:wolf', 'monster:bear'] });
  });

  it('the number fields keep what is typed: the formula and the most at a time (0 is no limit, up to 40)', () => {
    setup(on());
    const count = field(at('count')) as HTMLInputElement;
    count.value = '1 + mod("cha")';
    count.dispatchEvent(new Event('input'));
    expect(emitted[0]).toMatchObject({ count: '1 + mod("cha")' });
    const live = field(at('max_live')) as HTMLInputElement;
    expect(live.getAttribute('inputmode')).toBe('numeric');
    live.value = '0';
    live.dispatchEvent(new Event('input'));
    expect(emitted[1]).toMatchObject({ maxLive: '0' });
  });

  it('the resource select lists what the feature defines; a stored key it does not define is one more option, kept', () => {
    setup(on());
    const select = field(at('resource_key')) as HTMLSelectElement;
    expect(Array.from(select.options).map((o) => o.textContent?.trim())).toEqual([
      'Companheiro do Vale (1)',
    ]);
    expect(select.value).toBe('companheiro');
    setup(on({ resourceKey: 'da_classe' }));
    const withClass = field(at('resource_key')) as HTMLSelectElement;
    expect(Array.from(withClass.options).map((o) => o.textContent?.trim())).toEqual([
      'Companheiro do Vale (1)',
      'da_classe (da classe)',
    ]);
    expect(withClass.value).toBe('da_classe');
    expect(emitted).toHaveLength(0);
  });

  it('a class resource with no resource of the feature is a select, not the no-resource note', () => {
    setup(on({ resourceKey: 'da_classe' }), { resources: [] });
    expect(el.querySelector('select[data-field$="resource_key"]')).not.toBeNull();
    expect(text(el)).not.toContain('Esta característica não define um recurso.');
  });

  it('offers "Usar o padrão" for a stored role, cost or per-slot count the block cannot change', async () => {
    setup(on());
    expect(text(el)).not.toContain('Usar o padrão');
    const odd = on();
    odd.base = create(TableSummonSchema, { role: 2, resourceCost: 4, countPerSlotLevel: 1 });
    setup(odd);
    const btn = Array.from(el.querySelectorAll('button')).find((b) => text(b) === 'Usar o padrão')!;
    btn.click();
    const out = draftToSummon(emitted[0]!);
    expect([out.role, out.resourceCost, out.countPerSlotLevel]).toEqual([1, 1, 0]);
  });

  it('a feature with no resource says so in one line, with the field a blocked save lands on', () => {
    setup(on({ resourceKey: '' }), { resources: [] });
    const why = field(at('resource_key'))!;
    expect(text(why)).toContain('Esta característica não define um recurso.');
    expect(why.getAttribute('tabindex')).toBe('-1');
    expect(el.querySelector('select[data-field$="resource_key"]')).toBeNull();
  });

  it('the turn and the replacement are radio cards; the first is the default for a stored "unspecified"', () => {
    setup(
      on({ turnMode: TableSummonTurnMode.UNSPECIFIED, replaces: TableSummonReplaces.UNSPECIFIED }),
    );
    const radios = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
    expect(radios.filter((r) => r.checked).map((r) => r.value)).toEqual(['1', '1']);
    radios[1].click();
    expect(emitted[0]).toMatchObject({ turnMode: TableSummonTurnMode.OWNER_ROLL_GROUP });
    radios[3].click();
    expect(emitted[1]).toMatchObject({ replaces: TableSummonReplaces.SAME_SUMMON });
    expect(text(el)).toContain('Iniciativa própria');
    expect(text(el)).toContain('As novas tomam o lugar');
  });

  it("time and duration are selects with the board's words; an instant duration says how long the creatures stay", () => {
    setup(on());
    const time = field(at('casting_time')) as HTMLSelectElement;
    expect(time.selectedOptions[0].textContent?.trim()).toBe('1 ação');
    const duration = field(at('duration')) as HTMLSelectElement;
    expect(duration.selectedOptions[0].textContent?.trim()).toBe('Instantânea');
    expect(text(el)).toContain('Fica até cair ou ser dispensada.');
    duration.selectedIndex = 3;
    duration.dispatchEvent(new Event('change'));
    expect(emitted[0]!.duration.kind).toBe(SpellDurationKind.TIMED);
  });

  it('concentration is off and says why while the duration is not by time, and goes back off when the duration stops being by time', () => {
    setup(on());
    expect(sw('Concentração').disabled).toBe(true);
    expect(text(el)).toContain('Pede uma duração por tempo.');
    setup(
      on({
        duration: { kind: SpellDurationKind.TIMED, amount: 1, unit: 3, upTo: false },
        concentration: true,
      }),
    );
    expect(sw('Concentração').disabled).toBe(false);
    expect(sw('Concentração').getAttribute('aria-checked')).toBe('true');
    expect(text(el)).not.toContain('Pede uma duração por tempo.');
    const duration = field(at('duration')) as HTMLSelectElement;
    duration.selectedIndex = 0;
    duration.dispatchEvent(new Event('change'));
    expect(emitted[0]).toMatchObject({ concentration: false });
  });

  it('puts each refusal under its own field, with the aria wiring', () => {
    setup(on());
    // The refusal comes after the first render, as the server's answer does.
    const issues: Record<string, string[]> = {
      [at('')]: ['Confira as criaturas.'],
      [at('creature_keys')]: ['Escolha de 1 a 8 criaturas.'],
      [at('count')]: ['Esta fórmula não funciona.'],
      [at('max_live')]: ['De 0 a 40.'],
      [at('turn_mode')]: ['Escolha o turno.'],
      [at('replaces')]: ['Escolha o que acontece.'],
      [at('casting_time')]: ['Escolha o tempo.'],
      [at('duration')]: ['Escolha a duração.'],
    };
    fixture.componentRef.setInput('issuesOf', (path: string) => issues[path] ?? []);
    fixture.detectChanges();
    const t = text(el);
    for (const m of [
      'Confira as criaturas.',
      'Escolha de 1 a 8 criaturas.',
      'Esta fórmula não funciona.',
      'De 0 a 40.',
      'Escolha o turno.',
      'Escolha o que acontece.',
      'Escolha o tempo.',
      'Escolha a duração.',
    ]) {
      expect(t).toContain(m);
    }
    expect(field(at('count'))!.getAttribute('aria-invalid')).toBe('true');
    expect(field(at('max_live'))!.getAttribute('aria-describedby')).toBeTruthy();
    expect(field(at('creature_keys'))!.getAttribute('aria-invalid')).toBe('true');
    expect(field(at(''))).not.toBeNull();
  });

  it('a stored concentration with a duration that is not by time shows checked and enabled, so it can be turned off', () => {
    setup(on({ concentration: true }));
    expect(sw('Concentração').getAttribute('aria-checked')).toBe('true');
    expect(sw('Concentração').disabled).toBe(false);
    sw('Concentração').click();
    expect(emitted[0]).toMatchObject({ concentration: false });
  });

  it('the switches point at their refusals and are marked invalid', () => {
    setup(on());
    const issues: Record<string, string[]> = {
      [at('')]: ['Confira.'],
      [at('concentration')]: ['Concentração pede uma duração por tempo.'],
    };
    fixture.componentRef.setInput('issuesOf', (path: string) => issues[path] ?? []);
    fixture.detectChanges();
    const main = sw('Invoca criaturas');
    expect(main.getAttribute('aria-invalid')).toBe('true');
    expect(el.querySelector(`#${main.getAttribute('aria-describedby')}`)?.textContent).toContain(
      'Confira.',
    );
    const conc = sw('Concentração');
    expect(conc.getAttribute('aria-invalid')).toBe('true');
    expect(el.querySelector(`#${conc.getAttribute('aria-describedby')}`)?.textContent).toContain(
      'duração por tempo',
    );
  });
});
