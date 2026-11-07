import { By } from '@angular/platform-browser';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { clone, create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CreatureDamageModifierSchema,
  CreatureSchema,
  CreatureSummarySchema,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableContentKind,
  TableContentRefusalSchema,
  TableContentViolationSchema,
  TableCreatureDamageModSchema,
  TableCreatureSchema,
  TableSpellSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { TableContentClient } from '../../../core/content/content-client';
import { catalog, entry } from '../../../core/content/content-testing';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { CreaturePick } from '../../../shared/creature-pick/creature-pick';
import { settle } from '../../../../test-settle';
import { CreatureEditor } from './creature-editor';

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

const wolf = create(CreatureSchema, {
  summary: {
    key: 'monster:wolf',
    name: 'Wolf',
    namePt: 'Lobo',
    typePt: 'fera',
    sizePt: 'Médio',
    challengeRating: '1/4',
  },
  armorClass: 13,
  hitPoints: 11,
  speedWalkFt: 40,
  actions: [
    {
      name: 'Bite',
      namePt: 'Mordida',
      hasAttack: true,
      attackBonus: 4,
      damage: [{ dice: '2d4+2', damageTypePt: 'perfurante' }],
    },
  ],
});

const attack = (n: number) => ({
  namePt: `Ataque ${n}`,
  melee: true,
  reachFt: 5,
  diceCount: 1,
  diceSides: 6,
  damageType: 'damage-type:fire',
});

/** The wolf of these specs, immune to poison. */
function withPoisonImmunity() {
  const block = clone(CreatureSchema, wolf);
  block.immunities = [
    create(CreatureDamageModifierSchema, {
      types: [{ key: 'damage-type:poison', namePt: 'Veneno' }],
    }),
  ];
  return block;
}

describe('CreatureEditor (#578)', () => {
  const save = vi.fn();
  const statBlock = vi.fn();
  const text = (e: Element) => (e.textContent ?? '').replace(/[\s ]+/g, ' ');

  const stored = (over: object = {}) =>
    entry(TableContentKind.CREATURE, 'Lobo do Vale', {
      body: {
        case: 'tableCreature',
        value: create(TableCreatureSchema, {
          namePt: 'Lobo do Vale',
          baseKey: 'monster:wolf',
          hitPointsFormula: '4 + 4 * level()',
          armorClassFormula: '13 + prof()',
          damageBonus: 1,
          traits: [{ namePt: 'Elo com o guardião', textPt: 'O lobo entende os comandos do dono.' }],
          attacks: [
            {
              namePt: 'Investida do Vale',
              melee: true,
              reachFt: 5,
              diceCount: 2,
              diceSides: 6,
              damageType: 'damage-type:fire',
            },
          ],
          damageMods: [
            create(TableCreatureDamageModSchema, {
              kind: 'resist',
              damageTypes: ['damage-type:fire'],
            }),
          ],
          ...over,
        }),
      },
    });

  function setup(
    existing: ReturnType<typeof stored> | null = stored(),
    entries = existing ? [existing] : [],
    block = wolf,
  ) {
    save.mockReset();
    statBlock.mockReset();
    statBlock.mockResolvedValue(block);
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: TableContentClient, useValue: { save } },
        { provide: CreaturesClient, useValue: { statBlock, search: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(CreatureEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('catalog', catalog());
    fixture.componentRef.setInput('entries', entries);
    if (existing) fixture.componentRef.setInput('entry', existing);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const field = (el: HTMLElement, path: string) =>
    el.querySelector<HTMLInputElement>(`[data-field="${path}"]`)!;
  const type = (el: HTMLElement, path: string, value: string) => {
    const input = field(el, path);
    input.value = value;
    input.dispatchEvent(new Event('input'));
  };
  const button = (el: ParentNode, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      text(b).includes(label),
    )!;
  const pick = (fixture: ReturnType<typeof setup>['fixture']) =>
    fixture.debugElement
      .query(By.directive(CreaturePick))
      .componentInstance.picked.emit(
        create(CreatureSummarySchema, { key: 'monster:wolf', namePt: 'Lobo' }),
      );

  it('a new creature asks for the base first and draws the rest of the editor after it', async () => {
    const { fixture, el } = setup(null);
    expect(text(el)).toContain('Criatura do SRD que serve de base');
    expect(text(el)).toContain('Só criaturas do SRD');
    expect(el.querySelector('[data-field="table_creature.name_pt"]')).toBeNull();
    expect(el.querySelector('app-editor-bar')).toBeNull();
    expect(el.querySelector('.preview')).toBeNull();
    pick(fixture);
    await settle(fixture, 3);
    expect(field(el, 'table_creature.name_pt')).not.toBeNull();
    expect(text(el)).toContain('Base: Lobo, do SRD (Fera · Médio · ND 1/4)');
    expect(text(el)).toContain('Salvar criatura');
    expect(field(el, 'table_creature.hit_points').placeholder).toBe('11, da base');
    expect(field(el, 'table_creature.armor_class').placeholder).toBe('13, da base');
    // Before the first save the base may still be changed.
    button(el, 'Trocar a base').click();
    await settle(fixture, 2);
    expect(el.querySelector('app-creature-pick')).not.toBeNull();
  });

  it('draws the saved creature: base read-only with its link, the numbers with the echo under each formula', async () => {
    const { fixture, el } = setup();
    await settle(fixture, 3);
    expect(el.querySelector('app-creature-pick')).toBeNull();
    expect(text(el)).toContain('Não muda depois de salvar.');
    // The line of the base is where a refusal of the base lands: it can take the focus.
    expect(field(el, 'table_creature.base_key').getAttribute('tabindex')).toBe('-1');
    const link = Array.from(el.querySelectorAll('a')).find((a) =>
      text(a).includes('Ver Lobo no bestiário'),
    )!;
    expect(link.getAttribute('href')).toBe('/campaigns/camp-1/bestiary/wolf');
    expect(field(el, 'table_creature.name_pt').value).toBe('Lobo do Vale');
    expect(field(el, 'table_creature.hit_points_formula').value).toBe('4 + 4 * level()');
    expect(text(el)).toContain('= 4 + 4 × nível do dono · de 1 a 9.999');
    expect(text(el)).toContain('= 13 + bônus de proficiência · de 1 a 40');
    expect(field(el, 'table_creature.damage_bonus').value).toBe('1');
    // Every field is named for a screen reader beyond "Fixo" and "Com o dono".
    expect(field(el, 'table_creature.hit_points').getAttribute('aria-label')).toBe(
      'Pontos de vida, valor fixo',
    );
    expect(field(el, 'table_creature.hit_points_formula').getAttribute('aria-label')).toBe(
      'Pontos de vida, fórmula com o dono',
    );
  });

  it('shows the traits and the attacks as one-line rows that open in place', async () => {
    const { fixture, el } = setup(stored({ damageMods: [] }));
    await settle(fixture, 3);
    const rows = el.querySelectorAll('li.row');
    expect(rows).toHaveLength(2);
    expect(text(rows[0])).toContain('Elo com o guardião');
    expect(text(rows[0])).toContain('O lobo entende os comandos do dono.');
    expect(text(rows[1])).toContain('Corpo a corpo, 1,5 m · 2d6 fogo · acerto e dano da criatura');
    expect(field(el, 'table_creature.attacks[0].dice_count')).toBeNull();
    const toggle = rows[1].querySelector<HTMLButtonElement>('.row__toggle')!;
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
    toggle.click();
    await settle(fixture, 2);
    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    expect(field(el, 'table_creature.attacks[0].dice_count').value).toBe('2');
    expect(field(el, 'table_creature.attacks[0].reach_ft')).not.toBeNull();
    expect(field(el, 'table_creature.attacks[0].range_ft')).toBeNull();
  });

  it('writes "Como fica na mesa" from the draft and the base, and lists who brings the creature', async () => {
    const summoner = entry(TableContentKind.SPELL, 'Chamado da Matilha', {
      body: {
        case: 'tableSpell',
        value: create(TableSpellSchema, {
          namePt: 'Chamado da Matilha',
          level: 3,
          summon: { creatureKeys: ['creature:lobo-do-vale@mesa'] },
        }),
      },
    });
    const existing = stored();
    const { fixture, el } = setup(existing, [existing, summoner]);
    await settle(fixture, 3);
    const preview = text(el.querySelector('.preview')!);
    expect(preview).toContain('Lobo do Vale');
    expect(preview).toContain('Da mesa');
    expect(preview).toContain('Fera · Médio · base Lobo · CA 13 · PV 11 · 12 m');
    const rows = Array.from(el.querySelectorAll('.preview__row')).map((r) => [
      text(r.querySelector('dt')!),
      text(r.querySelector('dd')!),
    ]);
    expect(rows).toEqual([
      ['CA', '13 · com dono, 13 + bônus de proficiência'],
      ['PV', '11 · com dono, 4 + 4 × nível do dono'],
      ['Bônus no dano', '+1'],
    ]);
    expect(preview).toContain('Mordida (da base). +4 · 2d4 + 3 perfurante.');
    expect(preview).toContain('Investida do Vale (da mesa). +0 · 2d6 + 1 fogo.');
    expect(preview).toContain('Com dono, o acerto e o dano seguem as fórmulas.');
    expect(preview).toContain('Quem traz esta criatura');
    expect(preview).toContain('Chamado da Matilha');
    expect(preview).toContain('magia da mesa, 3º círculo');
    expect(text(el)).toContain('Os jogadores não veem a lista de criaturas');
    expect(text(el)).toContain('Quem já tem a criatura fica com os números que recebeu.');
    // The preview follows what is typed.
    type(el, 'table_creature.name_pt', 'Lobo Cinzento');
    await settle(fixture, 2);
    expect(text(el.querySelector('.preview')!)).toContain('Lobo Cinzento');
  });

  it('hides "Quem traz esta criatura" when nothing brings it, and "Para os jogadores" for a new creature', async () => {
    const { fixture, el } = setup();
    await settle(fixture, 3);
    expect(text(el)).not.toContain('Quem traz esta criatura');
    const fresh = setup(null);
    pick(fresh.fixture);
    await settle(fresh.fixture, 3);
    expect(text(fresh.el)).not.toContain('Para os jogadores');
  });

  it('a new creature sends the whole body with the base, the name and the numbers as typed', async () => {
    const { fixture, el } = setup(null);
    pick(fixture);
    await settle(fixture, 3);
    type(el, 'table_creature.name_pt', 'Lobo do Vale');
    type(el, 'table_creature.hit_points', '40');
    type(el, 'table_creature.armor_class_formula', ' 13 + prof() ');
    button(el, 'Adicionar traço').click();
    await settle(fixture, 2);
    type(el, 'table_creature.traits[0].name_pt', 'Faro');
    type(el, 'table_creature.traits[0].text_pt', 'Sente o cheiro.');
    button(el, 'Adicionar ataque').click();
    await settle(fixture, 2);
    type(el, 'table_creature.attacks[0].name_pt', 'Mordida forte');
    type(el, 'table_creature.attacks[0].reach_ft', '3');
    const types = field(
      el,
      'table_creature.attacks[0].damage_type',
    ) as unknown as HTMLSelectElement;
    types.value = 'damage-type:fire';
    types.dispatchEvent(new Event('change'));
    save.mockResolvedValue({ entry: stored(), affected: [] });
    button(el, 'Salvar criatura').click();
    await settle(fixture, 3);
    const [id, sent, body] = save.mock.calls[0];
    expect(id).toBe('camp-1');
    expect(sent).toBeNull();
    expect(body.case).toBe('tableCreature');
    expect(body.value).toMatchObject({
      namePt: 'Lobo do Vale',
      baseKey: 'monster:wolf',
      hitPoints: 40,
      armorClass: 0,
      armorClassFormula: '13 + prof()',
      traits: [{ namePt: 'Faro', textPt: 'Sente o cheiro.' }],
      damageMods: [],
    });
    expect(body.value.attacks[0]).toMatchObject({
      namePt: 'Mordida forte',
      melee: true,
      reachFt: 10,
      diceCount: 1,
      diceSides: 6,
      damageType: 'damage-type:fire',
    });
  });

  it('saving a saved creature sends back its resistances as the rows hold them', async () => {
    const { fixture, el } = setup();
    await settle(fixture, 3);
    save.mockResolvedValue({ entry: stored(), affected: [] });
    button(el, 'Salvar criatura').click();
    await settle(fixture, 3);
    const body = save.mock.calls[0][2];
    expect(body.value.baseKey).toBe('monster:wolf');
    expect(body.value.damageMods).toHaveLength(1);
    expect(body.value.damageMods[0]).toMatchObject({
      kind: 'resist',
      damageTypes: ['damage-type:fire'],
    });
  });

  describe('the resistances, immunities and vulnerabilities (#585)', () => {
    const mods = (el: HTMLElement) => el.querySelector('app-creature-damage-mods')!;
    const modRows = (el: HTMLElement) => Array.from(mods(el).querySelectorAll('li.mod'));
    const selectOf = (row: Element, what: 'kind' | 'damage_types' | 'qualifier') =>
      row.querySelector<HTMLElement>(`mat-select[data-field$=".${what}"]`)!;
    /** Opens a select and clicks the option with this text (its panel is drawn in the overlay). */
    const choose = async (
      fixture: ReturnType<typeof setup>['fixture'],
      select: HTMLElement,
      label: string,
    ) => {
      select.click();
      await settle(fixture, 3);
      Array.from(document.querySelectorAll<HTMLElement>('mat-option'))
        .find((o) => text(o).includes(label) || text(o) === label)!
        .click();
      await settle(fixture, 3);
    };
    const closePanel = async (fixture: ReturnType<typeof setup>['fixture']) => {
      document.querySelector('.cdk-overlay-backdrop')?.dispatchEvent(new MouseEvent('click'));
      await settle(fixture, 3);
    };
    const withMods = (...m: object[]) =>
      stored({ damageMods: m.map((x) => create(TableCreatureDamageModSchema, x)) });

    it('says each row in one table line, with the base in the intro, the headers, one info line and a count', async () => {
      const { fixture, el } = setup(
        withMods(
          { kind: 'resist', damageTypes: ['damage-type:fire'], qualifier: '' },
          {
            kind: 'resist',
            damageTypes: ['damage-type:necrotic', 'damage-type:fire'],
            qualifier: 'nonmagical_unless_silvered',
          },
        ),
        undefined,
        withPoisonImmunity(),
      );
      await settle(fixture, 3);
      const t = text(mods(el));
      expect(t).toContain(
        'Somam-se às da base (Lobo: imunidade a veneno) e nunca tiram nenhuma. Até 12.',
      );
      expect(t).toContain('TipoTipos de danoVale contra');
      expect(t).toContain('Passa inteiro: mágica sempre;');
      const rows = modRows(el);
      expect(rows).toHaveLength(2);
      expect(text(selectOf(rows[0], 'kind'))).toContain('Resistência');
      expect(text(selectOf(rows[0], 'damage_types'))).toBe('fogo');
      expect(text(selectOf(rows[1], 'damage_types'))).toBe('necrótico, fogo');
      expect(text(selectOf(rows[1], 'qualifier'))).toBe('Ataque não mágico nem prateado');
      expect(selectOf(rows[1], 'kind').getAttribute('aria-label')).toBe('Tipo da linha 2');
      expect(text(mods(el).querySelector('.add')!)).toContain('2 de 12');
    });

    it('lists the kinds, and the damage types with a check for the chosen ones', async () => {
      const { fixture, el } = setup(
        withMods({ kind: 'resist', damageTypes: ['damage-type:fire'] }),
      );
      await settle(fixture, 3);
      const row = modRows(el)[0];
      selectOf(row, 'kind').click();
      await settle(fixture, 3);
      expect(Array.from(document.querySelectorAll('mat-option'), (o) => text(o))).toEqual([
        'shieldResistência',
        'verified_userImunidade',
        'gpp_maybeVulnerabilidade',
      ]);
      await closePanel(fixture);
      selectOf(row, 'damage_types').click();
      await settle(fixture, 3);
      const options = Array.from(document.querySelectorAll('mat-option'));
      expect(options.length).toBeGreaterThan(1);
      expect(options.filter((o) => o.getAttribute('aria-selected') === 'true').map(text)).toEqual([
        'Fogo',
      ]);
    });

    it('turns "Vale contra" off (dashed, with the block icon) for a vulnerability and for a type that is not physical, and clears it on the change that makes it impossible', async () => {
      const { fixture, el } = setup(
        withMods(
          { kind: 'resist', damageTypes: ['damage-type:fire'] },
          { kind: 'resist', damageTypes: ['damage-type:slashing'], qualifier: 'nonmagical' },
        ),
      );
      await settle(fixture, 3);
      const [fire, slashing] = modRows(el);
      expect(selectOf(fire, 'qualifier').getAttribute('aria-disabled')).toBe('true');
      expect(fire.querySelector('mat-form-field.off')).not.toBeNull();
      expect(
        text(
          fire.querySelector(
            'mat-form-field.off mat-icon[matSuffix], mat-form-field.off .mat-mdc-form-field-icon-suffix',
          )!,
        ),
      ).toContain('block');
      expect(selectOf(fire, 'qualifier').getAttribute('aria-describedby')).toBe('cr-mods-info');
      expect(selectOf(slashing, 'qualifier').getAttribute('aria-disabled')).toBe('false');
      expect(slashing.querySelector('mat-form-field.off')).toBeNull();
      // A vulnerability has none: choosing it clears the one that was chosen.
      await choose(fixture, selectOf(slashing, 'kind'), 'Vulnerabilidade');
      expect(selectOf(modRows(el)[1], 'qualifier').getAttribute('aria-disabled')).toBe('true');
      expect(text(selectOf(modRows(el)[1], 'qualifier'))).toBe('Todo dano desses tipos');
    });

    it('adds a row, checks and unchecks its types, and sends what the row holds', async () => {
      const { fixture, el } = setup(withMods());
      await settle(fixture, 3);
      expect(text(mods(el))).toContain('Nenhuma linha da mesa ainda.');
      button(mods(el), 'Adicionar linha').click();
      await settle(fixture, 3);
      const row = modRows(el)[0];
      // The focus goes to the first control of the new row.
      expect(document.activeElement).toBe(field(el, 'table_creature.damage_mods[0].kind'));
      for (const name of ['Necrótico', 'Fogo', 'Necrótico']) {
        await choose(fixture, selectOf(row, 'damage_types'), name);
      }
      expect(text(selectOf(modRows(el)[0], 'damage_types'))).toBe('fogo');
      await choose(fixture, selectOf(modRows(el)[0], 'kind'), 'Imunidade');
      expect(text(selectOf(modRows(el)[0], 'kind'))).toContain('Imunidade');
      save.mockResolvedValue({ entry: stored(), affected: [] });
      button(el, 'Salvar criatura').click();
      await settle(fixture, 3);
      expect(save.mock.calls[0][2].value.damageMods).toEqual([
        expect.objectContaining({
          kind: 'immune',
          damageTypes: ['damage-type:fire'],
          qualifier: '',
        }),
      ]);
    });

    it('removes a row from its menu and gives the focus to the add button', async () => {
      const { fixture, el } = setup(
        withMods(
          { kind: 'resist', damageTypes: ['damage-type:fire'] },
          { kind: 'immune', damageTypes: ['damage-type:necrotic'] },
        ),
      );
      await settle(fixture, 3);
      modRows(el)[0].querySelector<HTMLButtonElement>('.mod__more')!.click();
      await settle(fixture, 2);
      Array.from(document.querySelectorAll<HTMLButtonElement>('[mat-menu-item]'))
        .find((b) => text(b).includes('Remover linha'))!
        .click();
      await settle(fixture, 3);
      expect(modRows(el)).toHaveLength(1);
      expect(text(selectOf(modRows(el)[0], 'damage_types'))).toBe('necrótico');
      expect(document.activeElement).toBe(button(mods(el), 'Adicionar linha'));
    });

    it('turns the add button off at 12 rows, with the reason beside it', async () => {
      const twelve = Array.from({ length: 12 }, () => ({
        kind: 'resist',
        damageTypes: ['damage-type:fire'],
      }));
      const { fixture, el } = setup(withMods(...twelve));
      await settle(fixture, 3);
      const add = button(mods(el), 'Adicionar linha');
      expect(add.getAttribute('aria-disabled')).toBe('true');
      expect(add.getAttribute('aria-describedby')).toBe('cr-mods-full');
      expect(text(mods(el).querySelector('#cr-mods-full')!)).toContain('No máximo 12 linhas');
    });

    it('puts each refusal under its own select, marks it invalid and names a type that is twice', async () => {
      const { fixture, el } = setup(
        withMods(
          { kind: 'resist', damageTypes: ['damage-type:fire'] },
          { kind: 'vulnerable', damageTypes: ['damage-type:fire'], qualifier: 'nonmagical' },
          { kind: 'immune', damageTypes: ['damage-type:necrotic', 'damage-type:necrotic'] },
        ),
      );
      await settle(fixture, 3);
      save.mockRejectedValue(
        refusal(
          ['table_creature.damage_mods[1].qualifier', 'bad_value'],
          ['table_creature.damage_mods[2].damage_types[1]', 'bad_value'],
        ),
      );
      button(el, 'Salvar criatura').click();
      await settle(fixture, 3);
      const rows = modRows(el);
      const cell = (row: Element, what: 'kind' | 'damage_types' | 'qualifier') =>
        selectOf(row, what).closest('.cell')!;
      expect(text(cell(rows[1], 'qualifier'))).toContain(
        'Só resistência ou imunidade a concussão, perfurante e cortante pode ter esse "Vale contra"',
      );
      const q = selectOf(rows[1], 'qualifier');
      expect(q.getAttribute('aria-invalid')).toBe('true');
      expect(el.querySelector(`#${q.getAttribute('aria-describedby')}`)!.textContent).toContain(
        'Só resistência',
      );
      expect(text(cell(rows[2], 'damage_types'))).toContain(
        'Necrótico está duas vezes. Tire o repetido.',
      );
      expect(selectOf(rows[2], 'damage_types').getAttribute('aria-invalid')).toBe('true');
      // A row with no refusal is not marked.
      expect(selectOf(rows[0], 'kind').getAttribute('aria-invalid')).toBe('false');
      expect(selectOf(rows[0], 'kind').getAttribute('aria-describedby')).toBeNull();
      // The focus goes to the first refused field.
      expect(document.activeElement).toBe(field(el, 'table_creature.damage_mods[1].qualifier'));
    });

    it('puts the refusal of the kind and of the missing types under their selects', async () => {
      const { fixture, el } = setup(
        withMods(
          { kind: 'resist', damageTypes: ['damage-type:fire'] },
          { kind: 'resist', damageTypes: [] },
        ),
      );
      await settle(fixture, 3);
      save.mockRejectedValue(
        refusal(
          ['table_creature.damage_mods[0].kind', 'bad_value'],
          ['table_creature.damage_mods[0].damage_types[0]', 'dangling_reference'],
          ['table_creature.damage_mods[1].damage_types', 'bad_value'],
        ),
      );
      button(el, 'Salvar criatura').click();
      await settle(fixture, 3);
      const [first, second] = modRows(el);
      expect(text(first)).toContain('Escolha resistência, imunidade ou vulnerabilidade.');
      expect(text(first)).toContain('Este tipo de dano não está no SRD. Tire-o da linha.');
      expect(text(second)).toContain('Escolha ao menos um tipo de dano.');
      expect(document.activeElement).toBe(field(el, 'table_creature.damage_mods[0].kind'));
    });

    it('sends a kind, a type or a qualifier this build does not know as the entry has it, and shows it', async () => {
      const { fixture, el } = setup(
        withMods({
          kind: 'reflect',
          damageTypes: ['damage-type:fire', 'damage-type:psychic9'],
          qualifier: 'cold_iron',
        }),
      );
      await settle(fixture, 3);
      const row = modRows(el)[0];
      expect(text(selectOf(row, 'kind'))).toContain('valor desconhecido: reflect');
      expect(text(selectOf(row, 'qualifier'))).toBe('valor desconhecido: cold_iron');
      expect(selectOf(row, 'qualifier').getAttribute('aria-disabled')).toBe('false');
      selectOf(row, 'damage_types').click();
      await settle(fixture, 3);
      expect(Array.from(document.querySelectorAll('mat-option'), (o) => text(o))).toContain(
        'valor desconhecido: damage-type:psychic9',
      );
      await closePanel(fixture);
      save.mockResolvedValue({ entry: stored(), affected: [] });
      button(el, 'Salvar criatura').click();
      await settle(fixture, 3);
      expect(save.mock.calls[0][2].value.damageMods[0]).toMatchObject({
        kind: 'reflect',
        damageTypes: ['damage-type:fire', 'damage-type:psychic9'],
        qualifier: 'cold_iron',
      });
    });

    it('takes off one of a type the row holds twice, not both', async () => {
      const { fixture, el } = setup(
        withMods({ kind: 'immune', damageTypes: ['damage-type:fire', 'damage-type:fire'] }),
      );
      await settle(fixture, 3);
      expect(text(selectOf(modRows(el)[0], 'damage_types'))).toBe('fogo, fogo');
      await choose(fixture, selectOf(modRows(el)[0], 'damage_types'), 'Fogo');
      expect(text(selectOf(modRows(el)[0], 'damage_types'))).toBe('fogo');
    });

    it('joins three parts of the preview with commas and an "e"', async () => {
      const { fixture, el } = setup(
        withMods(
          { kind: 'immune', damageTypes: ['damage-type:necrotic'] },
          { kind: 'immune', damageTypes: ['damage-type:fire'] },
        ),
        undefined,
        withPoisonImmunity(),
      );
      await settle(fixture, 3);
      expect(text(el.querySelector('.preview__mods')!)).toContain(
        'Imunidade a necrótico (da mesa), a fogo (da mesa) e a veneno (da base).',
      );
    });

    it('puts the refusal of the limit above the rows', async () => {
      const { fixture, el } = setup();
      await settle(fixture, 3);
      save.mockRejectedValue(refusal(['table_creature.damage_mods', 'limit']));
      button(el, 'Salvar criatura').click();
      await settle(fixture, 3);
      expect(text(mods(el))).toContain('No máximo 12 linhas');
    });

    it('writes "Resistências e imunidades" in the preview, the table lines and the base ones, and keeps the note that the holder reads them', async () => {
      const { fixture, el } = setup(
        withMods({ kind: 'immune', damageTypes: ['damage-type:necrotic'] }),
        undefined,
        withPoisonImmunity(),
      );
      await settle(fixture, 3);
      const preview = text(el.querySelector('.preview')!);
      expect(preview).toContain('Resistências e imunidades');
      expect(preview).toContain('Imunidade a necrótico (da mesa) e a veneno (da base).');
      expect(preview).toContain(
        'Só você muda estas linhas. Quem tem a criatura as lê na ficha dela.',
      );
    });

    it('has no such block in the preview when there are no lines', async () => {
      const { fixture, el } = setup(withMods());
      await settle(fixture, 3);
      expect(text(el.querySelector('.preview')!)).not.toContain('Resistências e imunidades');
    });
  });

  it('puts each refusal under its own field and opens the rows that hold them', async () => {
    const { fixture, el } = setup();
    await settle(fixture, 3);
    save.mockRejectedValue(
      refusal(
        ['table_creature.hit_points', 'bad_value'],
        ['table_creature.armor_class_formula', 'bad_formula'],
        ['table_creature.traits[0].name_pt', 'bad_name'],
        ['table_creature.attacks[0].damage_type', 'dangling_reference'],
      ),
    );
    button(el, 'Salvar criatura').click();
    await settle(fixture, 3);
    expect(text(el.querySelector('[role="alert"]')!)).toContain('4 campos precisam de ajuste');
    const under = (path: string) =>
      text(field(el, path).closest('app-text-field, app-select-field')!);
    expect(under('table_creature.hit_points')).toContain('Os pontos de vida vão de 1 a 9.999');
    expect(under('table_creature.armor_class_formula')).toContain('Esta fórmula não funciona');
    expect(under('table_creature.traits[0].name_pt')).toContain('Use de 1 a 80 letras');
    expect(under('table_creature.attacks[0].damage_type')).toContain('Escolha um tipo de dano');
    expect(field(el, 'table_creature.hit_points').getAttribute('aria-invalid')).toBe('true');
    // The focus goes to the first refused field.
    expect(document.activeElement).toBe(field(el, 'table_creature.hit_points'));
  });

  it('puts the refusal of the base under the base', async () => {
    const { fixture, el } = setup();
    await settle(fixture, 3);
    save.mockRejectedValue(refusal(['table_creature.base_key', 'dangling_reference']));
    button(el, 'Salvar criatura').click();
    await settle(fixture, 3);
    expect(text(field(el, 'table_creature.base_key').parentElement!)).toContain(
      'Esta criatura não está no SRD',
    );
  });

  it('does not send a number it cannot read: it says so on the field instead', async () => {
    const { fixture, el } = setup();
    await settle(fixture, 3);
    type(el, 'table_creature.hit_points', '12x');
    button(el, 'Salvar criatura').click();
    await settle(fixture, 3);
    expect(save).not.toHaveBeenCalled();
    expect(text(field(el, 'table_creature.hit_points').closest('app-text-field')!)).toContain(
      'Escreva um número inteiro',
    );
  });

  it('turns the add buttons off at the limits, with the reason beside them', async () => {
    const full = stored({
      traits: Array.from({ length: 10 }, (_, i) => ({ namePt: `Traço ${i}`, textPt: 'x' })),
      attacks: Array.from({ length: 8 }, (_, i) => attack(i)),
      damageMods: [],
    });
    const { fixture, el } = setup(full);
    await settle(fixture, 3);
    const addAttack = button(el, 'Adicionar ataque');
    const addTrait = button(el, 'Adicionar traço');
    expect(addAttack.getAttribute('aria-disabled')).toBe('true');
    expect(addTrait.getAttribute('aria-disabled')).toBe('true');
    expect(text(el)).toContain('No máximo 8 ataques. Remova um para pôr outro.');
    expect(text(el)).toContain('No máximo 10 traços. Remova um para pôr outro.');
    expect(addAttack.getAttribute('aria-describedby')).toBe('cr-attacks-full');
    expect(el.querySelectorAll('li.row')).toHaveLength(18);
  });

  it('removes a row from its menu and frees the add button', async () => {
    const full = stored({
      attacks: Array.from({ length: 8 }, (_, i) => attack(i)),
      damageMods: [],
    });
    const { fixture, el } = setup(full);
    await settle(fixture, 3);
    expect(button(el, 'Adicionar ataque').getAttribute('aria-disabled')).toBe('true');
    const more = el.querySelectorAll<HTMLButtonElement>('.row__more');
    more[1].click();
    await settle(fixture, 2);
    const item = Array.from(document.querySelectorAll<HTMLButtonElement>('[mat-menu-item]')).find(
      (b) => text(b).includes('Remover ataque'),
    )!;
    item.click();
    await settle(fixture, 3);
    expect(el.querySelectorAll('li.row')).toHaveLength(8);
    expect(button(el, 'Adicionar ataque').getAttribute('aria-disabled')).toBeNull();
    expect(text(el)).not.toContain('No máximo 8 ataques');
    // The menu that had the focus is gone: it goes to the button that adds the next row.
    expect(document.activeElement).toBe(button(el, 'Adicionar ataque'));
  });

  it('a row opens when it is added, with the focus on its name', async () => {
    const { fixture, el } = setup();
    await settle(fixture, 3);
    button(el, 'Adicionar traço').click();
    await settle(fixture, 3);
    expect(document.activeElement).toBe(field(el, 'table_creature.traits[1].name_pt'));
  });
});
