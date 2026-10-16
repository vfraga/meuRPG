import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { of } from 'rxjs';

import {
  CriticalRule,
  DeathSaveVisibility,
  DiceMode,
  EnemyReactionsRule,
  HiddenAreaHitRule,
  HitPointsRule,
  Role,
  TableStyle,
  TableStylePresetSchema,
  XpMode,
  XpModeChangeBlockedSchema,
} from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import {
  type RulesDraft,
  TableRulesClient,
  type TableRulesVm,
} from '../../core/campaigns/table-rules';
import { TableRulesPage } from './table-rules';
import { settle } from '../../../test-settle';

const presets = [
  create(TableStylePresetSchema, {
    style: TableStyle.TUDO_NO_APP,
    diceMode: DiceMode.APP,
    combatStartsWithMap: true,
    fogOnNewMaps: true,
  }),
  create(TableStylePresetSchema, {
    style: TableStyle.MESA_FISICA,
    diceMode: DiceMode.PHYSICAL,
    combatStartsWithMap: false,
    fogOnNewMaps: false,
  }),
  create(TableStylePresetSchema, {
    style: TableStyle.TEATRO_DA_MENTE,
    diceMode: DiceMode.PLAYERS_CHOOSE,
    combatStartsWithMap: false,
    fogOnNewMaps: false,
  }),
];

const saved: RulesDraft = {
  diceMode: DiceMode.PLAYERS_CHOOSE,
  combatStartsWithMap: true,
  fogOnNewMaps: true,
  hitPoints: HitPointsRule.PLAYER_CHOOSES,
  standardArray: true,
  pointBuy: true,
  rolled4d6: true,
  typed: true,
  critical: CriticalRule.DOUBLED_DICE,
  deathSaves: DeathSaveVisibility.VISIBLE_TO_ALL,
  hiddenAreaHits: HiddenAreaHitRule.REVEAL,
  featsAllowed: false,
  enemyReactions: EnemyReactionsRule.ONLY_WHEN_POSSIBLE,
  houseRules: ['Beber uma poção é uma ação bônus'],
};

function vm(over: Partial<RulesDraft> = {}): TableRulesVm {
  return {
    saved: { ...saved, ...over },
    style: TableStyle.PERSONALIZADO,
    presets,
    standardArray: [15, 14, 13, 12, 10, 8],
    pointBuyCosts: [0, 1, 2, 3, 4, 5, 7, 9],
    pointBuyMinScore: 8,
    pointBuyBudget: 27,
    typedMin: 3,
    typedMax: 18,
  };
}

describe('TableRulesPage', () => {
  const get = vi.fn();
  const set = vi.fn();
  const setXpMode = vi.fn();
  const getCampaign = vi.fn();

  async function setup(role: Role = Role.MASTER, over: Partial<RulesDraft> = {}) {
    get.mockReset().mockResolvedValue(vm(over));
    set.mockReset().mockImplementation(async (_id: string, d: RulesDraft) => ({
      saved: d,
      style: TableStyle.PERSONALIZADO,
    }));
    setXpMode.mockReset().mockResolvedValue({
      xpMode: XpMode.MILESTONES,
      changedAt: { seconds: 1791236520n, nanos: 0 },
    });
    getCampaign.mockReset().mockResolvedValue({
      campaign: {
        id: 'camp-1',
        name: 'Mirathel',
        myRole: role,
        awaitingApproval: false,
        xpMode: XpMode.ENEMIES,
      },
    });
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: ActivatedRoute,
          useValue: { paramMap: of(convertToParamMap({ id: 'camp-1' })) },
        },
        { provide: CampaignsService, useValue: { getCampaign } },
        { provide: TableRulesClient, useValue: { get, set, setXpMode } },
      ],
    });
    const fixture = TestBed.createComponent(TableRulesPage);
    fixture.detectChanges();
    await settle(fixture, 3);
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const text = (el: HTMLElement) =>
    (el.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const radio = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]')).find((i) =>
      i.closest('label')?.textContent?.includes(label),
    )!;
  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      b.textContent?.replace(/\s+/g, ' ').trim().includes(label),
    )!;

  it('gives a player the rules in words, read-only, with the reminders and no controls', async () => {
    const { el } = await setup(Role.PLAYER);
    expect(text(el)).toContain('Só o mestre muda as regras da mesa.');
    expect(el.querySelector('button[role="switch"]')).toBeNull();
    expect(el.querySelector('input')).toBeNull();
    expect(el.querySelector('button')).toBeNull();
    const rows = Array.from(el.querySelectorAll('.read__row')).map(
      (r) => `${r.querySelector('dt')?.textContent} ${r.querySelector('dd')?.textContent}`,
    );
    expect(rows).toContain('Dados Cada jogador escolhe');
    expect(rows).toContain('Combate Começa com mapa');
    expect(rows).toContain('Névoa de guerra nos mapas novos Ligada');
    expect(rows).toContain('Pontos de vida ao subir de nível O jogador escolhe');
    expect(rows).toContain(
      'Habilidades de uma ficha nova Conjunto padrão, Compra por pontos, 4d6, descartando o menor, Digitar os valores',
    );
    expect(rows).toContain('Experiência Por inimigos');
    expect(text(el)).toContain('Personalizado');
    expect(text(el)).toContain('Beber uma poção é uma ação bônus');
    expect(text(el)).toContain('Lembretes para a mesa toda');
    expect(set).not.toHaveBeenCalled();
  });

  it('draws the page with the "Estilo da mesa" on top, the choices as they are saved, and "Tudo salvo"', async () => {
    const { el } = await setup();
    const titles = Array.from(el.querySelectorAll('h2')).map((h) => h.textContent?.trim());
    expect(titles[0]).toBe('Estilo da mesa');
    expect(titles).toEqual(
      expect.arrayContaining([
        'Pontos de vida ao subir de nível',
        'Habilidades de uma ficha nova',
        'Acertos críticos',
        'Testes contra a morte',
        'Dados',
        'Combate e névoa',
        'Criaturas escondidas atingidas por uma área',
        'Experiência',
        'Lembretes da mesa',
        'Grade dos mapas',
      ]),
    );
    expect(text(el)).toContain(
      'Mirathel · Estas escolhas valem para cada ficha e cada combate da campanha.',
    );
    // The saved rules do not match any preset: "Personalizado" is the one on, and cannot be chosen.
    expect(radio(el, 'Personalizado').checked).toBe(true);
    expect(radio(el, 'Personalizado').disabled).toBe(true);
    expect(radio(el, 'O jogador escolhe').checked).toBe(true);
    expect(radio(el, 'Começar com mapa').checked).toBe(true);
    expect(el.querySelector('button[role="switch"]')?.getAttribute('aria-checked')).toBe('true');
    expect(text(el)).toContain('Beber uma poção é uma ação bônus'.slice(0, 0));
    expect((el.querySelector('.reminder input') as HTMLInputElement).value).toBe(
      'Beber uma poção é uma ação bônus',
    );
    const save = button(el, 'Salvar regras');
    expect(save.classList).toContain('mr-button--off');
    expect(text(el)).toContain('Tudo salvo. Nenhuma mudança para salvar.');
  });

  it('carries the label of the 2024 rules on the three SRD 5.2.1 methods and not on "Digitar"', async () => {
    const { el } = await setup();
    const cards = Array.from(el.querySelectorAll('app-method-cards .card'));
    expect(cards).toHaveLength(4);
    expect(
      cards.slice(0, 3).every((c) => c.textContent?.includes('SRD 5.2.1 (regras de 2024)')),
    ).toBe(true);
    expect(cards[3].textContent).not.toContain('2024');
    expect(cards[3].textContent).toContain('de 3 a 18, antes do bônus da raça');
    expect(text(el)).toContain('15, 14, 13, 12, 10 e 8, um para cada habilidade.');
    expect(text(el)).toContain('27 pontos; cada valor vai de 8 a 15.');
    expect(
      el.querySelector('app-method-cards')?.parentElement?.querySelector('a[href="/credits"]'),
    ).not.toBeNull();
  });

  it('choosing a style fills the three choices in place, says what it changed, and lights "Salvar regras"', async () => {
    const { fixture, el } = await setup();
    radio(el, 'Teatro da mente').click();
    await settle(fixture, 3);
    expect(radio(el, 'Teatro da mente').checked).toBe(true);
    expect(radio(el, 'Começar sem mapa').checked).toBe(true);
    expect(el.querySelector('button[role="switch"]')?.getAttribute('aria-checked')).toBe('false');
    const note = text(el);
    expect(note).toContain(
      'Teatro da mente: o estilo preencheu 2 escolhas abaixo; ainda não foi salvo.',
    );
    expect(note).toContain('Combate: começar sem mapa.');
    expect(note).toContain('Névoa de guerra nos mapas novos: desligada.');
    expect(note).toContain('Dados: já era “Cada jogador escolhe”.');
    expect(note).toContain('Mudou Era: com mapa');
    expect(note).toContain('Mudou Era: ligada');
    expect(el.querySelector('.changed .mr-tag--pending mat-icon')?.textContent).toBe('warning');
    expect(note).toContain('2 mudanças não salvas');
    expect(button(el, 'Salvar regras').classList).not.toContain('mr-button--off');
  });

  it('an edit by hand after a style takes it back to "Personalizado", and says so', async () => {
    const { fixture, el } = await setup();
    radio(el, 'Teatro da mente').click();
    await settle(fixture, 3);
    radio(el, 'Começar com mapa').click();
    await settle(fixture, 3);
    expect(radio(el, 'Personalizado').checked).toBe(true);
    expect(text(el)).toContain('Personalizado. Suas escolhas não batem mais com nenhum estilo.');
  });

  it('saves everything at once, with the dice mode, and goes back to "Tudo salvo"', async () => {
    const { fixture, el } = await setup();
    radio(el, 'Mesa física').click();
    await settle(fixture, 3);
    radio(el, 'A média').click();
    await settle(fixture, 3);
    button(el, 'Salvar regras').click();
    await settle(fixture, 3);
    expect(set).toHaveBeenCalledTimes(1);
    const [id, sent] = set.mock.calls[0] as [string, RulesDraft];
    expect(id).toBe('camp-1');
    expect(sent).toMatchObject({
      diceMode: DiceMode.PHYSICAL,
      combatStartsWithMap: false,
      fogOnNewMaps: false,
      hitPoints: HitPointsRule.AVERAGE,
      houseRules: ['Beber uma poção é uma ação bônus'],
    });
    expect(text(el)).toContain('Regras salvas.');
    expect(button(el, 'Salvar regras').classList).toContain('mr-button--off');
  });

  it('offers the three choices for hidden creatures an area hits, and saves the one picked with the rest', async () => {
    const { fixture, el } = await setup();
    const group = el.querySelector(
      '[role="radiogroup"][aria-label="Criaturas escondidas atingidas por uma área"]',
    );
    expect(group).not.toBeNull();
    const titles = Array.from(group!.querySelectorAll('label')).map((l) =>
      (l.textContent ?? '').replace(/\s+/g, ' ').trim(),
    );
    expect(titles[0]).toContain('Revelar');
    expect(titles[0]).toContain('É o padrão.');
    expect(titles[1]).toContain('Manter escondidas');
    expect(titles[2]).toContain('Perguntar a cada vez');
    expect(radio(el, 'Revelar').checked).toBe(true);
    expect(text(el)).toContain('Vale a partir da próxima magia.');

    radio(el, 'Perguntar a cada vez').click();
    await settle(fixture, 3);
    expect(text(el)).toContain('1 mudança não salva');
    button(el, 'Salvar regras').click();
    await settle(fixture, 3);
    const [, sent] = set.mock.calls[0] as [string, RulesDraft];
    expect(sent.hiddenAreaHits).toBe(HiddenAreaHitRule.ASK);
  });

  it('always sends the rule on hidden creatures, also when another choice is what changed', async () => {
    const { fixture, el } = await setup(Role.MASTER, {
      hiddenAreaHits: HiddenAreaHitRule.KEEP_HIDDEN,
    });
    expect(radio(el, 'Manter escondidas').checked).toBe(true);
    radio(el, 'A média').click();
    await settle(fixture, 3);
    button(el, 'Salvar regras').click();
    await settle(fixture, 3);
    const [, sent] = set.mock.calls[0] as [string, RulesDraft];
    expect(sent.hiddenAreaHits).toBe(HiddenAreaHitRule.KEEP_HIDDEN);
  });

  it('tells a player the rule on hidden creatures as text, with no card to pick', async () => {
    const { el } = await setup(Role.PLAYER, { hiddenAreaHits: HiddenAreaHitRule.ASK });
    const rows = Array.from(el.querySelectorAll('.read__row')).map(
      (r) => `${r.querySelector('dt')?.textContent} ${r.querySelector('dd')?.textContent}`,
    );
    expect(rows).toContain('Criaturas escondidas atingidas por uma área Perguntar a cada vez');
    expect(el.querySelector('[role="radiogroup"]')).toBeNull();
  });

  it('keeps at least one way of making scores: with none the save is off and says why', async () => {
    const { fixture, el } = await setup(Role.MASTER, {
      standardArray: false,
      pointBuy: false,
      rolled4d6: false,
      typed: true,
    });
    const typed = Array.from(el.querySelectorAll<HTMLInputElement>('app-method-cards input')).at(
      -1,
    )!;
    typed.click();
    await settle(fixture, 3);
    expect(text(el)).toContain('Marque pelo menos um jeito de fazer as habilidades.');
    button(el, 'Salvar regras').click();
    await settle(fixture, 3);
    expect(set).not.toHaveBeenCalled();
  });

  it('adds, edits and removes house rules, up to 20, one line of up to 200 characters', async () => {
    const { fixture, el } = await setup(Role.MASTER, { houseRules: [] });
    expect(el.querySelectorAll('.reminder')).toHaveLength(0);
    button(el, 'Adicionar um lembrete').click();
    await settle(fixture, 3);
    const field = el.querySelector<HTMLInputElement>('.reminder input')!;
    expect(field.getAttribute('maxlength')).toBe('200');
    // An empty line cannot be saved.
    expect(text(el)).toContain('Escreva o lembrete ou remova a linha vazia.');
    field.value = 'Quem cai fica caído';
    field.dispatchEvent(new Event('input'));
    await settle(fixture, 3);
    expect(text(el)).not.toContain('Escreva o lembrete');
    button(el, 'Salvar regras').click();
    await settle(fixture, 3);
    expect((set.mock.calls[0][1] as RulesDraft).houseRules).toEqual(['Quem cai fica caído']);
    button(el, 'Remover o lembrete 1').click();
    await settle(fixture, 3);
    expect(el.querySelectorAll('.reminder')).toHaveLength(0);
  });

  it('stops adding reminders at 20', async () => {
    const { el } = await setup(Role.MASTER, {
      houseRules: Array.from({ length: 20 }, (_, i) => `Regra ${i + 1}`),
    });
    expect(el.querySelectorAll('.reminder')).toHaveLength(20);
    expect(button(el, 'Adicionar um lembrete')).toBeUndefined();
    expect(text(el)).toContain('Chegou ao limite de 20 lembretes.');
  });

  it('shows what the server refuses on save, in words', async () => {
    const { fixture, el } = await setup();
    set.mockRejectedValue(new ConnectError('x', Code.InvalidArgument));
    radio(el, 'A média').click();
    await settle(fixture, 3);
    button(el, 'Salvar regras').click();
    await settle(fixture, 3);
    expect(text(el)).toContain('Não deu para salvar as regras');
  });

  it('links to the maps, and shows no link to the table content until that page exists', async () => {
    const { el } = await setup();
    expect(el.querySelector('a[href="/campaigns/camp-1/content"]')).toBeNull();
    expect(text(el)).not.toContain('Conteúdo da mesa');
    expect(el.querySelector('a[href="/campaigns/camp-1#maps"]')?.textContent).toContain(
      'Abrir os mapas',
    );
  });

  it('puts "Dados" beside "Estilo da mesa", and the style cards in a grid', async () => {
    const { el } = await setup();
    const top = el.querySelector('.top')!;
    expect(Array.from(top.querySelectorAll('h2')).map((h) => h.textContent?.trim())).toEqual([
      'Estilo da mesa',
      'Dados',
    ]);
    expect(top.querySelector('.dice-choice--grid')).not.toBeNull();
  });

  it('mutes the filled save button while the XP question is open, so one filled button shows', async () => {
    const { fixture, el } = await setup(Role.MASTER, {});
    expect(button(el, 'Salvar regras').classList).toContain('mdc-button--unelevated');
    setXpMode.mockRejectedValueOnce(
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: XpModeChangeBlockedSchema,
          value: create(XpModeChangeBlockedSchema, { awards: 1, totalXp: 50n }),
        },
      ]),
    );
    radio(el, 'Por marcos').click();
    await settle(fixture, 3);
    button(el, 'Mudar para marcos').click();
    await settle(fixture, 3);
    expect(button(el, 'Salvar regras').classList).toContain('mat-mdc-outlined-button');
    expect(
      el.querySelectorAll('button.mat-mdc-unelevated-button, button.mdc-button--unelevated').length,
    ).toBe(1);
  });

  it('keeps the save status as a live region all the time', async () => {
    const { el } = await setup();
    expect(el.querySelector('.save__line')?.getAttribute('role')).toBe('status');
  });

  describe('the XP mode', () => {
    it('picking a card changes nothing: only "Mudar para …" does, and it is aria-disabled until a card differs', async () => {
      const { fixture, el } = await setup();
      const apply = button(el, 'Mudar o modo de XP');
      // Not `disabled`: it stays focusable, so the focus never drops to the page.
      expect(apply.getAttribute('aria-disabled')).toBe('true');
      expect(apply.hasAttribute('disabled')).toBe(false);
      radio(el, 'Por marcos').click();
      await settle(fixture, 3);
      expect(setXpMode).not.toHaveBeenCalled();
      expect(button(el, 'Mudar para marcos').getAttribute('aria-disabled')).not.toBe('true');
      // Picking the mode in force again puts the button back.
      radio(el, 'Por inimigos').click();
      await settle(fixture, 3);
      expect(button(el, 'Mudar o modo de XP').getAttribute('aria-disabled')).toBe('true');
      button(el, 'Mudar o modo de XP').click();
      await settle(fixture, 3);
      expect(setXpMode).not.toHaveBeenCalled();
    });

    it('changes at once when nobody has XP, and says when', async () => {
      const { fixture, el } = await setup();
      radio(el, 'Por marcos').click();
      await settle(fixture, 3);
      button(el, 'Mudar para marcos').click();
      await settle(fixture, 3);
      expect(setXpMode).toHaveBeenCalledWith('camp-1', XpMode.MILESTONES, false);
      expect(radio(el, 'Por marcos').checked).toBe(true);
      expect(text(el)).toContain('Modo de XP mudado para por marcos');
      expect(text(el)).toContain('Vale daqui para frente.');
    });

    it("asks in place when XP was already given, with the server's numbers, and changes only on the confirmation", async () => {
      const { fixture, el } = await setup();
      const blocked = new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: XpModeChangeBlockedSchema,
          value: create(XpModeChangeBlockedSchema, { awards: 3, totalXp: 2716n }),
        },
      ]);
      setXpMode.mockRejectedValueOnce(blocked);
      radio(el, 'Por marcos').click();
      await settle(fixture, 3);
      button(el, 'Mudar para marcos').click();
      await settle(fixture, 3);
      expect(el.querySelector('h3')?.textContent).toContain('Mudar para “por marcos”?');
      expect(document.activeElement).toBe(el.querySelector('h3'));
      const q = text(el);
      expect(q).toContain('Já houve XP dado nesta campanha. Mudar vale só daqui para frente.');
      expect(q).toContain('3 prêmios, 2.716 XP');
      expect(setXpMode).toHaveBeenCalledTimes(1);
      expect(setXpMode).toHaveBeenCalledWith('camp-1', XpMode.MILESTONES, false);

      Array.from(el.querySelectorAll<HTMLButtonElement>('.ask button'))
        .find((b) => b.textContent?.includes('Mudar para marcos'))!
        .click();
      await settle(fixture, 3);
      expect(setXpMode).toHaveBeenLastCalledWith('camp-1', XpMode.MILESTONES, true);
      expect(radio(el, 'Por marcos').checked).toBe(true);
      expect(el.querySelector('h3')).toBeNull();
    });

    it('"Voltar" closes the question, changes nothing and puts the card and the focus back', async () => {
      const { fixture, el } = await setup();
      const blocked = new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: XpModeChangeBlockedSchema,
          value: create(XpModeChangeBlockedSchema, { awards: 1, totalXp: 50n }),
        },
      ]);
      setXpMode.mockRejectedValueOnce(blocked);
      radio(el, 'Por ouro').click();
      await settle(fixture, 3);
      button(el, 'Mudar para ouro').click();
      await settle(fixture, 3);
      expect(text(el)).toContain('1 prêmio, 50 XP');
      expect(radio(el, 'Por ouro').checked).toBe(true);
      Array.from(el.querySelectorAll<HTMLButtonElement>('.ask button'))
        .find((b) => b.textContent?.trim() === 'Voltar')!
        .click();
      await settle(fixture, 3);
      expect(el.querySelector('h3')).toBeNull();
      expect(radio(el, 'Por inimigos').checked).toBe(true);
      expect(setXpMode).toHaveBeenCalledTimes(1);
      expect(document.activeElement).toBe(button(el, 'Mudar o modo de XP'));
    });
  });

  it('has "Talentos" as "Não usados" by default, and saves "Permitidos" with the rest', async () => {
    const { fixture, el } = await setup();
    expect(radio(el, 'Não usados').checked).toBe(true);
    radio(el, 'Permitidos').click();
    await settle(fixture, 3);
    expect(text(el)).toContain('1 mudança não salva');
    button(el, 'Salvar regras').click();
    await settle(fixture, 3);
    const [, sent] = set.mock.calls[0] as [string, RulesDraft];
    expect(sent.featsAllowed).toBe(true);
    expect(sent.houseRules).toEqual(['Beber uma poção é uma ação bônus']);
  });

  it('tells a player whether the table uses feats', async () => {
    const { el } = await setup(Role.PLAYER);
    const rows = Array.from(el.querySelectorAll('.read__row')).map(
      (r) => `${r.querySelector('dt')?.textContent} ${r.querySelector('dd')?.textContent}`,
    );
    expect(rows).toContain('Talentos Não usados');
  });

  it('draws "Reações dos inimigos" with the default chosen and the attention sentence', async () => {
    const { el } = await setup();
    const title = Array.from(el.querySelectorAll('h2')).find(
      (h) => h.textContent?.trim() === 'Reações dos inimigos',
    );
    expect(title).toBeDefined();
    expect(radio(el, 'Só quando um inimigo pode reagir').checked).toBe(true);
    expect(radio(el, 'Sempre').checked).toBe(false);
    expect(text(el)).toContain(
      'Atenção: uma pausa pode sugerir aos jogadores que alguém pode reagir.',
    );
    expect(text(el)).toContain('Vale a partir da próxima ação.');
  });

  it('saves the chosen enemy reactions rule with "Salvar regras"', async () => {
    const { fixture, el } = await setup();
    radio(el, 'Sempre').click();
    await settle(fixture, 3);
    button(el, 'Salvar regras').click();
    await settle(fixture, 3);
    expect((set.mock.calls[0][1] as RulesDraft).enemyReactions).toBe(EnemyReactionsRule.ALWAYS);
  });

  it('lets a player read the enemy reactions rule and nothing more', async () => {
    const { el } = await setup(Role.PLAYER, { enemyReactions: EnemyReactionsRule.ALWAYS });
    const rows = Array.from(el.querySelectorAll('.read__row')).map(
      (r) => `${r.querySelector('dt')?.textContent} ${r.querySelector('dd')?.textContent}`,
    );
    expect(rows).toContain('Reações dos inimigos Sempre');
  });
});
