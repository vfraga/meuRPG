import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import {
  type AttunementEntryVm,
  MagicCategories,
  magicCategoryIcon,
} from '../../../core/characters/attunement';
import type { FullSheetVm } from '../character-sheet.types';
import { ItemChargesVm } from '../../../core/characters/charges';
import {
  ItemEmptyRule,
  ItemLoss,
  ItemRechargeKind,
  ItemRechargeSchema,
} from '../../../../gen/meurpg/characters/v1/characters_pb';
import { create, type MessageInitShape } from '@bufbuild/protobuf';
import { EquipmentPanel } from './equipment-panel';

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();

const entry = (over: Partial<AttunementEntryVm>): AttunementEntryVm => ({
  id: 'm1',
  key: 'ring',
  namePt: 'Anel de proteção',
  kind: 'magic',
  attuned: false,
  asks: true,
  restrictionPt: '',
  restriction: 'none',
  sameItemAttuned: false,
  ...over,
});

const piece = (key: string, namePt: string, equipped = true) => ({
  key,
  namePt,
  kind: 'weapon' as const,
  equipped,
  twoHanded: false,
  armorClassIfToggled: 0,
  count: 1,
});

function sheetWith(
  entries: AttunementEntryVm[],
  gear: unknown[] = [],
  extra: Partial<FullSheetVm> = {},
): FullSheetVm {
  return {
    ammunition: [],
    armorClass: 14,
    gear,
    equipment: [],
    backgroundEquipment: '',
    coins: { cp: 0, sp: 0, ep: 0, gp: 0, pp: 0 },
    shieldIdle: false,
    attunement: { entries, attuned: entries.filter((e) => e.asks && e.attuned).length },
    ...extra,
  } as unknown as FullSheetVm;
}

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [EquipmentPanel],
  template: `
    <app-equipment-panel
      [sheet]="sheet()"
      [viewer]="viewer()"
      characterName="Tor"
      [canEquip]="true"
      [canManageMagic]="manage()"
      [canChangeAttunement]="change()"
      [attunementIsClosed]="closed()"
      [magicNotice]="notice()"
      [canAddAmmunition]="addAmmo()"
      [combatRunning]="combat()"
      [chargesResult]="result()"
      [chargesError]="chargesError()"
      [ammunitionNotice]="ammoNotice()"
      [ammunitionError]="ammoError()"
      (chargesUsed)="used.push($event.id)"
      (lastChargeRolled)="rolled.push($event.id)"
      (ammunitionAdding)="ammoAdding.push($event)"
      (ammunitionRecovered)="recovered = recovered + 1"
      (attunementOpened)="opened = opened + 1"
      (magicAdding)="adding = adding + 1"
      (magicRemoving)="removed.push($event.id)"
    />
  `,
})
class Host {
  readonly sheet = signal(sheetWith([]));
  readonly viewer = signal<'owner' | 'master'>('owner');
  readonly manage = signal(false);
  readonly change = signal(false);
  readonly closed = signal(false);
  readonly notice = signal('');
  readonly addAmmo = signal(false);
  readonly combat = signal(false);
  readonly result = signal<{ text: string; tone: 'good' | 'bad' } | null>(null);
  readonly chargesError = signal('');
  readonly ammoNotice = signal('');
  readonly ammoError = signal('');
  opened = 0;
  adding = 0;
  recovered = 0;
  readonly removed: string[] = [];
  readonly used: string[] = [];
  readonly rolled: string[] = [];
  readonly ammoAdding: unknown[] = [];
}

function render(init: (h: Host) => void = () => undefined) {
  const fixture = TestBed.createComponent(Host);
  init(fixture.componentInstance);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  return { fixture, host: fixture.componentInstance, el };
}

const CATEGORY_OF = new Map([
  ['ring', 'armor'],
  ['cloak', 'wondrous-item'],
]);

describe('EquipmentPanel, the magic items and the attunement', () => {
  beforeEach(() =>
    TestBed.configureTestingModule({
      providers: [
        {
          provide: MagicCategories,
          useValue: { iconOf: (k: string) => magicCategoryIcon(CATEGORY_OF.get(k)) },
        },
      ],
    }),
  );

  it('shows neither the header nor the list when nothing is magic and nobody can add', () => {
    const { el } = render();
    expect(el.querySelector('.att-head')).toBeNull();
    expect(el.querySelector('.magic')).toBeNull();
  });

  it('counts what is attuned in the header and words each row as a pill', () => {
    const { el } = render((h) =>
      h.sheet.set(
        sheetWith([
          entry({ attuned: true }),
          entry({ id: 'm2', key: 'cloak', namePt: 'Manto élfico' }),
          entry({ id: 'm3', key: 'bag', namePt: 'Bolsa de carga', asks: false }),
        ]),
      ),
    );
    expect(flat(el.querySelector('.att-head'))).toContain('Sintonizados: 1 de 3');
    expect(flat(el.querySelector('#magic-heading'))).toContain('Itens mágicos 3');
    const rows = Array.from(el.querySelectorAll('.magic-row'), flat);
    expect(rows[0]).toContain('Anel de proteçãoSintonizado');
    expect(rows[1]).toContain('Manto élficoPede sintonização');
    expect(rows[2]).toContain('Bolsa de carga');
    expect(rows[2]).not.toContain('sintonização');
  });

  it('says who fails what an item asks, the same words as the rest sheet', () => {
    const unmet = entry({ restrictionPt: 'Só para um conjurador', restriction: 'notMet' });
    const owner = render((h) => h.sheet.set(sheetWith([unmet])));
    expect(flat(owner.el.querySelector('.magic-row__line'))).toContain(
      'Só para um conjurador: você não cumpre.',
    );
    expect(owner.el.querySelector('.magic-row__line mat-icon')?.textContent).toBe('block');

    const master = render((h) => {
      h.viewer.set('master');
      h.sheet.set(sheetWith([unmet]));
    });
    expect(flat(master.el.querySelector('.magic-row__line'))).toContain(
      'Só para um conjurador: Tor não cumpre.',
    );
  });

  it('draws a condition of use with the moon, not with the block that means a refusal', () => {
    const { el } = render((h) =>
      h.sheet.set(sheetWith([entry({ restrictionPt: 'Só ao ar livre, à noite' })])),
    );
    expect(flat(el.querySelector('.magic-row__line'))).toContain('Só ao ar livre, à noite.');
    expect(el.querySelector('.magic-row__line mat-icon')?.textContent).toBe('dark_mode');
  });

  it("draws the icon of the item's category by its key", () => {
    const { el } = render((h) =>
      h.sheet.set(
        sheetWith([
          entry({}),
          entry({ id: 'm2', key: 'cloak', namePt: 'Manto' }),
          entry({ id: 'm3', key: 'unknown', namePt: 'Outro' }),
        ]),
      ),
    );
    const icons = Array.from(el.querySelectorAll('.magic-row__icon'), (i) => i.textContent);
    expect(icons).toEqual(['shield', 'diamond', 'diamond']);
  });

  it('offers "Mudar" while the window is open, and "Muda num descanso" when it is not', () => {
    const open = render((h) => {
      h.sheet.set(sheetWith([entry({})]));
      h.change.set(true);
    });
    const button = open.el.querySelector<HTMLButtonElement>('.att-head__button')!;
    expect(button.getAttribute('aria-label')).toBe('Mudar sintonização');
    button.click();
    expect(open.host.opened).toBe(1);
    expect(open.el.querySelector('.att-head__later')).toBeNull();

    const shut = render((h) => h.sheet.set(sheetWith([entry({})])));
    expect(flat(shut.el.querySelector('.att-head__later'))).toBe('Muda num descanso');
    expect(shut.el.querySelector('.att-head__button')).toBeNull();
  });

  it('says neither for a dead character', () => {
    const { el } = render((h) => {
      h.sheet.set(sheetWith([entry({})]));
      h.closed.set(true);
    });
    expect(el.querySelector('.att-head__later')).toBeNull();
    expect(el.querySelector('.att-head__button')).toBeNull();
  });

  it('gives whoever manages the items "Adicionar item mágico" and a "Remover item" menu on each row', () => {
    const { el, host } = render((h) => {
      h.sheet.set(sheetWith([entry({})]));
      h.manage.set(true);
    });
    el.querySelector<HTMLButtonElement>('.magic__add')!.click();
    expect(host.adding).toBe(1);
    const more = el.querySelector<HTMLButtonElement>('.magic-row__more')!;
    expect(more.getAttribute('aria-label')).toBe('Remover item · Anel de proteção');
    more.click();
    expect(host.removed).toEqual(['m1']);
  });

  it('says "Nenhum item mágico." for a manager with an empty list, and gives the others no controls', () => {
    const manager = render((h) => h.manage.set(true));
    expect(flat(manager.el.querySelector('.magic'))).toContain('Nenhum item mágico.');
    const other = render((h) => h.sheet.set(sheetWith([entry({})])));
    expect(other.el.querySelector('.magic__add')).toBeNull();
    expect(other.el.querySelector('.magic-row__more')).toBeNull();
  });

  it('keeps the last add or remove in a live region', () => {
    const { el } = render((h) => {
      h.sheet.set(sheetWith([entry({})]));
      h.notice.set('Item mágico adicionado.');
    });
    expect(flat(el.querySelector('.magic [role=status]'))).toBe('Item mágico adicionado.');
  });

  it('marks a gear row with the chip of its attunement: attuned, or asking for it', () => {
    const { el } = render((h) =>
      h.sheet.set(
        sheetWith(
          [
            entry({
              id: 'bonus:equipment:rapier',
              kind: 'piece',
              gearKey: 'equipment:rapier',
              attuned: true,
            }),
            entry({ id: 'bonus:equipment:mace', kind: 'piece', gearKey: 'equipment:mace' }),
          ],
          [
            piece('equipment:rapier', 'Rapieira'),
            piece('equipment:mace', 'Maça'),
            piece('equipment:dagger', 'Adaga'),
          ],
        ),
      ),
    );
    const rows = Array.from(el.querySelectorAll('.gear-row'));
    expect(flat(rows[0].querySelector('.gear-row__att'))).toBe('linkSintonizado');
    expect(rows[0].querySelector('.gear-row__att--on')).not.toBeNull();
    expect(flat(rows[1].querySelector('.gear-row__att'))).toBe('linkPede sintonização');
    expect(rows[1].querySelector('.gear-row__att--on')).toBeNull();
    expect(rows[2].querySelector('.gear-row__att')).toBeNull();
  });

  it('says what the lock leaves to the owner, once nobody here can add or mark', () => {
    const { el } = render((h) => h.sheet.set(sheetWith([entry({})])));
    expect(flat(el.querySelector('.gear__lock'))).toContain(
      'Travada: equipar e guardar continuam valendo; marcas, bônus e itens mágicos, só o mestre muda.',
    );
  });

  it('says on a sheet with charges that using them goes on working: "Travada: usar cargas continua valendo, o resto, só o mestre muda."', () => {
    const { el } = render((h) => h.sheet.set(sheetWith([charged('ready', 5)])));
    expect(flat(el.querySelector('.gear__lock'))).toContain(
      'Travada: usar cargas continua valendo, o resto, só o mestre muda.',
    );
  });

  it('has no lock line for whoever can manage the items', () => {
    const { el } = render((h) => h.manage.set(true));
    expect(el.querySelector('.gear__lock')).toBeNull();
  });

  it('puts the armor class on the "Equipado" head, not on a line of its own', () => {
    const { el } = render((h) =>
      h.sheet.set(sheetWith([entry({})], [piece('equipment:mace', 'Maça')])),
    );
    expect(el.querySelector('.gear__ac')).toBeNull();
    expect(flat(el.querySelector('.gear-group__ac'))).toBe('CA 14');
    const said = Array.from(el.querySelectorAll('span')).find((n) =>
      flat(n)?.includes('Classe de armadura'),
    )!;
    expect(flat(said)).toBe('Classe de armadura 14');
    expect(said.classList).toContain('mr-visually-hidden');
  });

  it('marks the shield "Não conta na CA" on its row when a two-handed weapon is equipped', () => {
    const shield = { ...piece('equipment:shield', 'Escudo'), kind: 'shield' as const };
    const { el } = render((h) =>
      h.sheet.set({ ...sheetWith([], [shield]), shieldIdle: true } as FullSheetVm),
    );
    expect(flat(el.querySelector('.gear-row--idle .gear-row__idle'))).toBe('Não conta na CA');
  });

  it('leaves the idle shield note to the combat column', () => {
    const shield = { ...piece('equipment:shield', 'Escudo'), kind: 'shield' as const };
    const { el } = render((h) =>
      h.sheet.set({ ...sheetWith([], [shield]), shieldIdle: true } as FullSheetVm),
    );
    expect(el.textContent).not.toContain('O escudo não');
  });

  it('draws the legend of the chips on the rows: the marks, the attunement and the bonus', () => {
    const { el } = render((h) =>
      h.sheet.set(
        sheetWith(
          [],
          [
            {
              ...piece('equipment:rapier', 'Rapieira'),
              marks: { silvered: true, adamantine: false, magical: true },
              bonus: 1,
              bonusCounts: true,
            },
          ],
        ),
      ),
    );
    const legend = el.querySelector('ul.legend')!;
    expect(legend.getAttribute('aria-label')).toBe('Legenda dos selos');
    expect(Array.from(legend.querySelectorAll('li')).map((li) => flat(li))).toEqual([
      expect.stringContaining('Prateada'),
      expect.stringContaining('Mágica'),
      expect.stringContaining('Bônus mágico, já somado'),
    ]);
  });

  it('reads the legend’s bonus once: the chip is hidden and the words stay', () => {
    const { el } = render((h) =>
      h.sheet.set(
        sheetWith([], [{ ...piece('equipment:rapier', 'Rapieira'), bonus: 1, bonusCounts: true }]),
      ),
    );
    const li = el.querySelector('ul.legend li')!;
    expect(li.querySelector('[aria-label]')).toBeNull();
    expect(li.querySelector('.bonus-chip')!.getAttribute('aria-hidden')).toBe('true');
    expect(flat(li)).toContain('Bônus mágico, já somado');
  });

  it('says the legend’s bonus does not count, with a dashed chip, when every bonus is dashed', () => {
    const { el } = render((h) =>
      h.sheet.set(
        sheetWith([], [{ ...piece('equipment:rapier', 'Rapieira'), bonus: 1, bonusCounts: false }]),
      ),
    );
    const li = el.querySelector('ul.legend li')!;
    expect(flat(li)).toContain('Bônus mágico, não conta agora');
    expect(li.querySelector('.bonus-chip--idle')).not.toBeNull();
  });

  it('draws no legend when no row has a chip', () => {
    const { el } = render((h) => h.sheet.set(sheetWith([], [piece('equipment:mace', 'Maça')])));
    expect(el.querySelector('ul.legend')).toBeNull();
  });
});

const charged = (
  state: ItemChargesVm['state'],
  left: number,
  over: Partial<AttunementEntryVm> = {},
): AttunementEntryVm =>
  entry({
    id: 'w1',
    key: 'wand',
    namePt: 'Varinha de mísseis mágicos',
    asks: false,
    charges: { max: 7, left, state },
    ...over,
  });

describe('EquipmentPanel, the item rows with charges', () => {
  beforeEach(() =>
    TestBed.configureTestingModule({
      providers: [{ provide: MagicCategories, useValue: { iconOf: () => 'auto_fix_high' } }],
    }),
  );

  const row = (el: HTMLElement) => el.querySelector('.magic-row--charges')!;

  it('draws the name, "Usar", the pips and "N de M cargas" of an item with charges', () => {
    const { el } = render((h) => h.sheet.set(sheetWith([charged('ready', 5)])));
    const r = row(el);
    expect(flat(r.querySelector('.magic-row__name'))).toBe('Varinha de mísseis mágicos');
    expect(flat(r.querySelector('.magic-row__count'))).toBe('5 de 7 cargas');
    expect(flat(r.querySelector('.magic-row__amount'))).toBe('5 de 7 cargas');
    expect(r.querySelectorAll('app-count-pips .pip')).toHaveLength(7);
    expect(r.querySelectorAll('app-count-pips .pip--left')).toHaveLength(5);
    const use = r.querySelector<HTMLButtonElement>('.magic-row__use')!;
    expect(flat(use)).toBe('Usar');
    expect(use.getAttribute('aria-label')).toBe('Usar Varinha de mísseis mágicos');
    expect(use.getAttribute('aria-disabled')).toBeNull();
  });

  it('asks the page to open the use sheet of that item', () => {
    const { el, host } = render((h) => h.sheet.set(sheetWith([charged('ready', 5)])));
    row(el).querySelector<HTMLButtonElement>('.magic-row__use')!.click();
    expect(host.used).toEqual(['w1']);
  });

  it('turns "Usar" off at 0 and says why, and a tap does nothing', () => {
    const { el, host } = render((h) => h.sheet.set(sheetWith([charged('empty', 0)])));
    const use = row(el).querySelector<HTMLButtonElement>('.magic-row__use')!;
    expect(use.getAttribute('aria-disabled')).toBe('true');
    expect(flat(row(el).querySelector('.magic-row__line'))).toContain('Sem cargas.');
    expect(use.getAttribute('aria-describedby')).toBe(
      row(el).querySelector('.magic-row__line')!.id,
    );
    use.click();
    expect(host.used).toEqual([]);
  });

  it('marks a destroyed item with the dashed "Destruída" tag and no "Usar"', () => {
    const { el } = render((h) => h.sheet.set(sheetWith([charged('destroyed', 0)])));
    expect(flat(row(el).querySelector('.magic-row__tag'))).toBe('Destruída');
    expect(row(el).querySelector('.magic-row__use')).toBeNull();
    expect(row(el).querySelector('.magic-row__count')).toBeNull();
    expect(flat(row(el).querySelector('.magic-row__line'))).toContain('A última carga se foi.');
  });

  it('marks an item that lost its magic with "Sem magia"', () => {
    const plain = render((h) => h.sheet.set(sheetWith([charged('plain', 0)])));
    expect(flat(row(plain.el).querySelector('.magic-row__tag'))).toBe('Sem magia');
    expect(flat(row(plain.el).querySelector('.magic-row__line'))).toContain('Virou um item comum.');
    const partly = render((h) => h.sheet.set(sheetWith([charged('partly', 0)])));
    expect(flat(row(partly.el).querySelector('.magic-row__tag'))).toBe('Sem magia');
    expect(flat(row(partly.el).querySelector('.magic-row__line'))).toContain('Ficou só o bônus');
  });

  it('waits for the master on a rolled start: "Usar" off and the line says so', () => {
    const { el } = render((h) => h.sheet.set(sheetWith([charged('unset', 0)])));
    expect(row(el).querySelector('.magic-row__use')!.getAttribute('aria-disabled')).toBe('true');
    expect(flat(row(el).querySelector('.magic-row__line'))).toContain(
      'O mestre define as cargas iniciais.',
    );
  });

  it('asks for the d20 of the last charge with "Rolar" in place of "Usar"', () => {
    const { el, host } = render((h) => h.sheet.set(sheetWith([charged('pending', 0)])));
    expect(flat(row(el).querySelector('.magic-row__line'))).toContain(
      'Falta rolar o d20 da última carga.',
    );
    const roll = row(el).querySelector<HTMLButtonElement>('.magic-row__use')!;
    expect(flat(roll)).toBe('Rolar');
    roll.click();
    expect(host.rolled).toEqual(['w1']);
    expect(host.used).toEqual([]);
  });

  it('works on a locked sheet: "Usar" needs no right to edit, only that the character is not dead', () => {
    const { el } = render((h) => {
      h.sheet.set(sheetWith([charged('ready', 5)]));
      h.manage.set(false);
    });
    expect(row(el).querySelector('.magic-row__use')).not.toBeNull();
    expect(row(el).querySelector('.magic-row__more')).toBeNull();
  });

  it('keeps the link chip of an attuned item', () => {
    const { el } = render((h) =>
      h.sheet.set(sheetWith([charged('ready', 5, { attuned: true, asks: true })])),
    );
    expect(flat(row(el).querySelector('.magic-row__pill'))).toContain('Sintonizado');
  });

  it('leaves a row with no charge data as it was', () => {
    const { el } = render((h) => h.sheet.set(sheetWith([entry({})])));
    expect(el.querySelector('.magic-row--charges')).toBeNull();
    expect(el.querySelector('.magic-row__use')).toBeNull();
  });

  it('says what the last use did, and a refusal, in live regions', () => {
    const { el } = render((h) => {
      h.sheet.set(sheetWith([charged('ready', 5)]));
      h.result.set({ text: 'Gastou 2 cargas.', tone: 'good' });
      h.chargesError.set('Cargas a menos.');
    });
    expect(flat(el.querySelector('.charges__result--good'))).toContain('Gastou 2 cargas.');
    expect(el.querySelector('[role="alert"].charges__result--bad')?.textContent).toContain(
      'Cargas a menos.',
    );
  });
});

describe('EquipmentPanel, the recharge rules on the item rows', () => {
  beforeEach(() =>
    TestBed.configureTestingModule({
      providers: [{ provide: MagicCategories, useValue: { iconOf: () => 'auto_fix_high' } }],
    }),
  );

  const rule = (init: MessageInitShape<typeof ItemRechargeSchema>) =>
    create(ItemRechargeSchema, init);
  const lineOf = (e: AttunementEntryVm) => {
    const { el } = render((h) => h.sheet.set(sheetWith([e])));
    const line = el.querySelector('.magic-row--charges .magic-row__line');
    return { text: flat(line), icon: line?.querySelector('mat-icon')?.textContent };
  };
  const dawn = rule({ kind: ItemRechargeKind.DAWN, regainDice: '1d6+1' });

  it('says when a ready item gets its charges back, under the count', () => {
    const line = lineOf(
      charged('ready', 5, { charges: { max: 7, left: 5, state: 'ready', recharge: dawn } }),
    );
    expect(line.text).toContain('Volta 1d6 + 1 ao amanhecer');
    expect(line.icon).toBe('wb_twilight');
  });

  it('says an item that does not come back', () => {
    const never = rule({ kind: ItemRechargeKind.UNSPECIFIED });
    expect(
      lineOf(charged('ready', 2, { charges: { max: 3, left: 2, state: 'ready', recharge: never } }))
        .text,
    ).toContain('Não volta: cada carga gasta se vai.');
  });

  it('says when an empty item gets them back', () => {
    const wand = rule({ kind: ItemRechargeKind.DAWN, regainDice: '1d3' });
    expect(
      lineOf(charged('empty', 0, { charges: { max: 3, left: 0, state: 'empty', recharge: wand } }))
        .text,
    ).toContain('Sem cargas: volta 1d3 ao amanhecer.');
  });

  it('says the dice of the rolled start', () => {
    const blade = rule({ kind: ItemRechargeKind.UNSPECIFIED, startDice: '1d4-1' });
    expect(
      lineOf(charged('unset', 0, { charges: { max: 3, left: 0, state: 'unset', recharge: blade } }))
        .text,
    ).toContain('O mestre define as cargas iniciais (1d4 − 1).');
  });

  it('says what a staff became, by its name and the rules', () => {
    const plain = rule({ empty: ItemEmptyRule.D20, onLoss: ItemLoss.PLAIN_ITEM });
    expect(
      lineOf(
        charged('plain', 0, {
          namePt: 'Bordão de encanto',
          charges: { max: 10, left: 0, state: 'plain', recharge: plain },
        }),
      ).text,
    ).toContain('Virou um bordão comum.');
    const partly = rule({ empty: ItemEmptyRule.D20, onLoss: ItemLoss.PARTLY_MAGIC });
    expect(
      lineOf(
        charged('partly', 0, {
          namePt: 'Bordão do poder',
          charges: { max: 20, left: 0, state: 'partly', recharge: partly },
        }),
      ).text,
    ).toContain('Ficou só o +2 no ataque e no dano.');
  });

  it('says a wand turned to ashes with the d20 that did it', () => {
    const gone = rule({ empty: ItemEmptyRule.D20, onLoss: ItemLoss.GONE });
    expect(
      lineOf(
        charged('destroyed', 0, {
          charges: { max: 7, left: 0, state: 'destroyed', recharge: gone },
        }),
      ).text,
    ).toContain('A última carga se foi e o d20 deu 1.');
  });
});

describe('EquipmentPanel, the ammunition list', () => {
  beforeEach(() =>
    TestBed.configureTestingModule({
      providers: [{ provide: MagicCategories, useValue: { iconOf: () => 'diamond' } }],
    }),
  );

  const arrows = (over: Record<string, unknown> = {}) => ({
    key: 'equipment:arrow',
    namePt: 'Flechas',
    quantity: 15,
    spent: 6,
    back: 3,
    recovered: false,
    ...over,
  });
  const withAmmo = (list: unknown[], extra: Partial<FullSheetVm> = {}) =>
    sheetWith([], [], { ammunition: list as never, ...extra });

  it('draws nothing without a list, a hint or the right to add', () => {
    const { el } = render();
    expect(el.querySelector('.ammo')).toBeNull();
  });

  it('draws the kinds the app counts, with what the last combat fired', () => {
    const { el } = render((h) => h.sheet.set(withAmmo([arrows()])));
    expect(flat(el.querySelector('#ammo-heading'))).toContain('Munição contada pelo app');
    const r = el.querySelector('.ammo-row')!;
    expect(flat(r)).toContain('Flechas');
    expect(flat(r)).toContain('6 gastas no último combate · voltam 3');
    expect(flat(r.querySelector('.ammo-row__qty'))).toBe('15');
  });

  it('offers to recover half, and asks the page when tapped', () => {
    const { el, host } = render((h) => h.sheet.set(withAmmo([arrows()])));
    const b = el.querySelector<HTMLButtonElement>('.ammo__recover')!;
    expect(flat(b)).toContain('Recuperar a metade: 3 flechas');
    expect(b.getAttribute('aria-disabled')).toBeNull();
    b.click();
    expect(host.recovered).toBe(1);
  });

  it('turns the recovery off during a combat, with the reason', () => {
    const { el, host } = render((h) => {
      h.sheet.set(withAmmo([arrows()]));
      h.combat.set(true);
    });
    const b = el.querySelector<HTMLButtonElement>('.ammo__recover')!;
    expect(b.getAttribute('aria-disabled')).toBe('true');
    expect(flat(el.querySelector('#ammo-recover-why'))).toContain('Só depois do combate');
    b.click();
    expect(host.recovered).toBe(0);
  });

  it('says it is done once the pieces came back, and when there is nothing to recover', () => {
    const done = render((h) => h.sheet.set(withAmmo([arrows({ recovered: true })])));
    expect(done.el.querySelector('.ammo__recover')!.getAttribute('aria-disabled')).toBe('true');
    expect(flat(done.el.querySelector('#ammo-recover-why'))).toContain(
      'Recuperou 3 flechas deste combate. O resto se perdeu.',
    );
    const none = render((h) => h.sheet.set(withAmmo([arrows({ spent: 0, back: 0 })])));
    expect(none.el.querySelector('.ammo__recover')!.getAttribute('aria-disabled')).toBe('true');
    expect(flat(none.el.querySelector('#ammo-recover-why'))).toContain(
      'Nada gasto no último combate: não há o que recuperar.',
    );
  });

  it('shows the hint only while the list is empty and the free text looks like ammunition', () => {
    const text = [
      { name: 'Mochila', quantity: 1 },
      { name: 'Flechas (20)', quantity: 1 },
    ];
    const hinted = render((h) => {
      h.sheet.set(withAmmo([], { equipment: text }));
      h.addAmmo.set(true);
    });
    expect(flat(hinted.el.querySelector('.ammo__hint'))).toContain(
      '“Flechas (20)” parece munição: para o app contar, adicione em Munição.',
    );
    expect(flat(hinted.el.querySelector('.ammo'))).toContain('Nenhuma munição contada.');
    hinted.el.querySelector<HTMLButtonElement>('.ammo__hint button')!.click();
    expect(hinted.host.ammoAdding).toEqual([
      { name: 'Flechas (20)', key: 'equipment:arrow', quantity: 20 },
    ]);

    const counted = render((h) => h.sheet.set(withAmmo([arrows()], { equipment: text })));
    expect(counted.el.querySelector('.ammo__hint')).toBeNull();
    const plain = render((h) => {
      h.sheet.set(withAmmo([], { equipment: [{ name: 'Mochila', quantity: 1 }] }));
      h.addAmmo.set(true);
    });
    expect(plain.el.querySelector('.ammo__hint')).toBeNull();
  });

  it('gives whoever may add "Adicionar munição", and nobody else', () => {
    const can = render((h) => {
      h.sheet.set(withAmmo([arrows()]));
      h.addAmmo.set(true);
    });
    const add = Array.from(can.el.querySelectorAll<HTMLButtonElement>('.ammo .magic__add')).find(
      (b) => flat(b)?.includes('Adicionar munição'),
    )!;
    add.click();
    expect(can.host.ammoAdding).toEqual([null]);
    const cannot = render((h) => h.sheet.set(withAmmo([arrows()])));
    expect(cannot.el.querySelector('.ammo .magic__add')).toBeNull();
  });

  it('says what a write did and a refusal', () => {
    const { el } = render((h) => {
      h.sheet.set(withAmmo([arrows()]));
      h.ammoNotice.set('Flechas: 20.');
      h.ammoError.set('Não foi possível.');
    });
    expect(flat(el.querySelector('.ammo [role="status"]'))).toBe('Flechas: 20.');
    expect(flat(el.querySelector('.ammo [role="alert"]'))).toContain('Não foi possível.');
  });
});
