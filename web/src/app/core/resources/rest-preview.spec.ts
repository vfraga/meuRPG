import { RestKind } from '../../../gen/meurpg/play/v1/resources_pb';
import {
  LONG_REST_TAKEN_WARNING,
  hitDiceChoices,
  restDoneText,
  restLabel,
  restSummary,
  type RestLine,
} from './rest-preview';
import { boardParty, restPreview, restPreviewResponse } from './resources-testing';

const text = (line: RestLine) => line.map((p) => p.text).join('');
const boldOf = (line: RestLine) => line.filter((p) => p.bold).map((p) => p.text);

/** Ki, Channel Divinity... come back on a short rest too; the rest only on a long one. */
const rechargeOf = (_id: string, key: string) =>
  key === 'ki' ? ('short_rest' as const) : ('long_rest' as const);

describe('restSummary, a long rest', () => {
  const summary = restSummary(RestKind.LONG, boardParty(), rechargeOf);

  it('opens as the board does', () => {
    expect(summary.title).toBe('Começar o descanso longo?');
    expect(summary.intro).toBe('Pelo menos 8 horas. Volta, para quem estiver na campanha:');
    expect(summary.warning).toBeNull();
  });

  it('lists the hit points, the hit dice, the resources and the slots', () => {
    expect(summary.lines.map(text)).toEqual([
      'Todos os PV voltam ao máximo (e a Ajuda acaba).',
      'Metade dos dados de vida gastos (no mínimo 1) voltam.',
      'Fúria: Ragna 3 de 3 · Cura pelas Mãos: Tavo 25 de 25 · Pontos de Feitiçaria: Nael 5 de 5 (o espaço criado some) · Inspiração de Bardo: Orla 3 de 3.',
      'Chi (Kai 5 de 5): voltam (também voltariam num descanso curto).',
      'espaços de magia: todos.',
    ]);
  });

  it('writes the names in bold, as the board does', () => {
    expect(boldOf(summary.lines[0])).toEqual(['PV']);
    expect(boldOf(summary.lines[1])).toEqual(['dados de vida']);
    expect(boldOf(summary.lines[2])).toEqual([
      'Fúria',
      'Cura pelas Mãos',
      'Pontos de Feitiçaria',
      'Inspiração de Bardo',
    ]);
    expect(boldOf(summary.lines[3])).toEqual(['Chi']);
  });

  it('takes every name from the preview, never from a list of its own', () => {
    const other = restSummary(
      RestKind.LONG,
      restPreviewResponse([
        restPreview({
          characterId: 'x',
          name: 'Zuri',
          resources: [{ key: 'invented', namePt: 'Recurso Inventado', spent: 1, total: 2 }],
        }),
      ]),
      () => undefined,
    );
    expect(text(other.lines[2])).toBe('Recurso Inventado: Zuri 2 de 2.');
  });

  it('lists a resource several characters have once, with each of them', () => {
    const two = restSummary(
      RestKind.LONG,
      restPreviewResponse([
        restPreview({
          characterId: 'a',
          name: 'Ana',
          resources: [{ key: 'rage', namePt: 'Fúria', spent: 1, total: 3 }],
        }),
        restPreview({
          characterId: 'b',
          name: 'Beto',
          resources: [{ key: 'rage', namePt: 'Fúria', spent: 2, total: 4 }],
        }),
      ]),
      rechargeOf,
    );
    expect(text(two.lines[2])).toBe('Fúria: Ana 3 de 3, Beto 4 de 4.');
  });

  it('warns above the list when a long rest was already taken', () => {
    const again = restSummary(RestKind.LONG, restPreviewResponse([], true), rechargeOf);
    expect(again.warning).toBe(LONG_REST_TAKEN_WARNING);
    expect(LONG_REST_TAKEN_WARNING).toBe(
      'Já houve um descanso longo nesta sessão. O SRD permite um a cada 24 horas de jogo: você decide.',
    );
  });

  it('tells a character at 0 hit points that the rest does nothing, and leaves it out of the lines', () => {
    const down = restSummary(
      RestKind.LONG,
      restPreviewResponse([
        restPreview({
          characterId: 'a',
          name: 'Ana',
          noBenefit: true,
          resources: [{ key: 'rage', namePt: 'Fúria', spent: 1, total: 3 }],
        }),
        restPreview({ characterId: 'b', name: 'Beto' }),
      ]),
      rechargeOf,
    );
    expect(down.noBenefit).toEqual(['Ana está a 0 PV: o descanso longo não faz efeito.']);
    expect(down.lines.map(text).join(' ')).not.toContain('Ana');
  });

  it('says a created slot that vanishes even when no sorcery points are spent', () => {
    const created = restSummary(
      RestKind.LONG,
      restPreviewResponse([restPreview({ characterId: 'n', name: 'Nael', createdSlotsLost: 2 })]),
      rechargeOf,
    );
    expect(created.lines.map(text)).toContain('espaços criados: Nael (somem).');
  });

  it('is empty when the campaign has no living player character', () => {
    expect(restSummary(RestKind.LONG, restPreviewResponse([]), rechargeOf).empty).toBe(true);
    expect(summary.empty).toBe(false);
  });
});

describe('restSummary, a short rest', () => {
  it('asks for at least 1 hour and lists only what a short rest gives back', () => {
    const summary = restSummary(
      RestKind.SHORT,
      restPreviewResponse([
        restPreview({
          characterId: 'k',
          name: 'Kai',
          resources: [{ key: 'ki', namePt: 'Chi', spent: 2, total: 5 }],
        }),
        restPreview({ characterId: 'z', name: 'Zed', pactSlotsBack: 1 }),
      ]),
      rechargeOf,
    );
    expect(summary.title).toBe('Começar o descanso curto?');
    expect(summary.intro).toBe('Pelo menos 1 hora. Volta, para quem estiver na campanha:');
    expect(summary.lines.map(text)).toEqual([
      'Os jogadores podem gastar dados de vida para curar.',
      'Chi: Kai 5 de 5.',
      'espaços de pacto: Zed.',
    ]);
    expect(summary.warning).toBeNull();
    expect(summary.noBenefit).toEqual([]);
  });

  it('never carries the long rest warning', () => {
    expect(
      restSummary(RestKind.SHORT, restPreviewResponse([], true), rechargeOf).warning,
    ).toBeNull();
  });
});

describe('labels', () => {
  it('names the kinds and says the rest is done', () => {
    expect(restLabel(RestKind.SHORT)).toBe('Descanso curto');
    expect(restLabel(RestKind.LONG)).toBe('Descanso longo');
    expect(restDoneText(RestKind.SHORT)).toBe('Descanso curto feito.');
    expect(restDoneText(RestKind.LONG)).toBe('Descanso longo feito.');
  });
});

describe('hitDiceChoices', () => {
  const multi = restPreview({
    characterId: 'f',
    name: 'Fenn',
    hitDiceSpent: [
      { faces: 10, count: 3 },
      { faces: 6, count: 2 },
    ],
    hitDiceBack: [{ faces: 10, count: 2 }],
    hitDiceBackLimit: 2,
  });

  it('offers a choice to a character with several sizes spent and a limit below what is spent', () => {
    expect(hitDiceChoices(restPreviewResponse([multi]))).toEqual([
      {
        characterId: 'f',
        name: 'Fenn',
        limit: 2,
        sizes: [
          { faces: 10, spent: 3, start: 2 },
          { faces: 6, spent: 2, start: 0 },
        ],
      },
    ]);
  });

  it('offers none for one size, for a limit that covers all that is spent, or for a character at 0 hit points', () => {
    const one = restPreview({
      characterId: 'a',
      name: 'Ana',
      hitDiceSpent: [{ faces: 8, count: 4 }],
      hitDiceBackLimit: 2,
    });
    const roomy = restPreview({
      characterId: 'b',
      name: 'Beto',
      hitDiceSpent: [
        { faces: 8, count: 1 },
        { faces: 6, count: 1 },
      ],
      hitDiceBackLimit: 3,
    });
    const down = restPreview({
      characterId: 'c',
      name: 'Caio',
      noBenefit: true,
      hitDiceSpent: multi.hitDiceSpent,
      hitDiceBackLimit: 2,
    });
    expect(hitDiceChoices(restPreviewResponse([one, roomy, down]))).toEqual([]);
  });
});

describe("restSummary: the owner's hooks and the one-use recovery", () => {
  const hookOf = (over: object) => ({ sourceNamePt: 'Fôlego da Mesa', ...over });

  it('lists a hook of the rest: temporary hit points and a heal "em média", and the use it gives back', () => {
    const preview = restPreviewResponse([
      restPreview({
        characterId: 'k',
        name: 'Kai',
        hooks: [
          hookOf({ tempHpGained: 3 }),
          hookOf({
            sourceNamePt: 'Brasa de Teste',
            healed: 4,
            restored: 1,
            restoredResource: 'reserva_de_teste',
          }),
        ] as never,
      }),
    ]);
    const lines = restSummary(RestKind.LONG, preview, rechargeOf).lines.map(text);
    expect(lines).toContain(
      'Ao terminar o descanso longo, Kai: Fôlego da Mesa entrou em ação: ganha 3 PV temporários.',
    );
    expect(lines).toContain(
      'Ao terminar o descanso longo, Kai: Brasa de Teste entrou em ação: recupera 4 PV (em média), devolve 1 uso de reserva de teste.',
    );
  });

  it('says a short rest for a short rest, and nothing for a character with no hook', () => {
    const preview = restPreviewResponse([
      restPreview({ characterId: 'k', name: 'Kai', hooks: [hookOf({ tempHpGained: 2 })] as never }),
      restPreview({ characterId: 'm', name: 'Mira' }),
    ]);
    const lines = restSummary(RestKind.SHORT, preview, rechargeOf).lines.map(text);
    expect(lines.filter((l) => l.startsWith('Ao terminar'))).toEqual([
      'Ao terminar o descanso curto, Kai: Fôlego da Mesa entrou em ação: ganha 2 PV temporários.',
    ]);
  });

  it('says "+1 uso" for a resource that gives back one use, not all of them', () => {
    const preview = restPreviewResponse([
      restPreview({
        characterId: 'k',
        name: 'Kai',
        resources: [
          { key: 'reserva_de_teste', namePt: 'Reserva de Teste', spent: 2, total: 3, back: 1 },
          { key: 'outra', namePt: 'Outra', spent: 1, total: 2 },
        ],
      }),
    ]);
    const lines = restSummary(RestKind.SHORT, preview, rechargeOf).lines.map(text);
    expect(lines).toContain('Reserva de Teste: Kai +1 uso · Outra: Kai 2 de 2.');
  });
});

describe('restSummary, the NPCs of the campaign that kept their uses', () => {
  it("names them in a line of the master's preview, and says nothing when there are none", () => {
    const preview = restPreviewResponse([restPreview({ characterId: 'k', name: 'Kai' })]);
    const none = restSummary(RestKind.LONG, preview, rechargeOf).lines.map(text);
    expect(none.some((l) => l.includes('NPCs da campanha'))).toBe(false);
    preview.npcsBack = [
      { $typeName: 'meurpg.play.v1.RestNPCBack', characterId: 'b', name: 'Bartolo' },
      { $typeName: 'meurpg.play.v1.RestNPCBack', characterId: 'z', name: 'Zélia' },
    ];
    const lines = restSummary(RestKind.LONG, preview, rechargeOf).lines.map(text);
    expect(lines).toContain('NPCs da campanha recuperam os usos: Bartolo e Zélia.');
  });
});
