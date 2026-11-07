import {
  CombatantKind,
  EncounterStatus,
  ReactionKind,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter, reactionWindow } from './combat-testing';
import {
  LEGENDARY_UNUSABLE,
  legendaryCard,
  legendaryCards,
  legendaryFolded,
  legendaryNames,
  legendaryOptionName,
  NO_HOLD,
  legendaryRecovered,
  waitingBehind,
} from './legendary-card';

const options = [
  {
    key: 'detect',
    name: 'Detect',
    namePt: 'Detectar',
    cost: 1,
    kind: 'text',
    text: 'English text',
    affordable: true,
    attackKey: '',
  },
  {
    key: 'tail',
    name: 'Tail Attack',
    namePt: 'Ataque de cauda',
    cost: 1,
    kind: 'attack',
    text: 'x',
    affordable: true,
    attackKey: 'm#tail',
    attackDetailPt: 'Cauda: +14 · 2d8 + 8 de concussão · alcance 4,5 m',
  },
  {
    key: 'wing',
    name: 'Wing Attack',
    namePt: 'Ataque de asas',
    cost: 2,
    kind: 'move',
    text: 'y',
    affordable: false,
    attackKey: '',
  },
];

function dragon(over: object = {}) {
  return combatant({
    id: 'd',
    label: 'Dragão vermelho adulto',
    legendary: {
      actionsPerRound: 3,
      actionsLeft: 1,
      resistancePerDay: 3,
      resistanceLeft: 2,
      usableNow: true,
      options,
      ...over,
    },
  } as never);
}

const tor = combatant({ id: 't', label: 'Tor', kind: CombatantKind.PLAYER });

describe("the master's legendary actions card", () => {
  it('counts the actions, names the options in Portuguese only and says what each costs', () => {
    const e = encounter({ combatants: [dragon(), tor], currentCombatantId: 't' });
    const card = legendaryCard(e, e.combatants[0]);
    expect(card?.title).toBe('Dragão vermelho adulto');
    expect(card?.counts).toBe('1 de 3');
    expect(card?.pips).toEqual([true, false, false]);
    expect(card?.spent).toBe(2);
    expect(card?.badge).toBe('Opcional');
    expect(card?.hint).toBe('Use no fim do turno de Tor, uma opção de cada vez.');
    expect(card?.rows.map((r) => r.name)).toEqual([
      'Detectar',
      'Ataque de cauda',
      'Ataque de asas',
    ]);
    expect(card?.rows.map((r) => r.costText)).toEqual(['custa 1', 'custa 1', 'custa 2']);
    const everything = JSON.stringify(card);
    expect(everything).not.toContain('Tail Attack');
    expect(everything).not.toContain('English text');
  });

  it('gives each kind its button: Usar, Atacar (with the attack key) and Usar to open the move', () => {
    const e = encounter({ combatants: [dragon(), tor], currentCombatantId: 't' });
    const rows = legendaryCard(e, e.combatants[0])?.rows ?? [];
    expect(rows.map((r) => [r.buttonLabel, r.action])).toEqual([
      ['Usar', 'spend'],
      ['Atacar', 'attack'],
      ['Usar', 'move'],
    ]);
    expect(rows[1].attackKey).toBe('m#tail');
    expect(rows[0].detail).toBe('Você resolve: o app só conta o gasto.');
    // An attack row wears the server's numbers; a move row says nothing under its name.
    expect(rows[1].detail).toBe('Cauda: +14 · 2d8 + 8 de concussão · alcance 4,5 m');
    expect(rows[2].detail).toBe('');
  });

  it('turns off the option that costs more than what is left, and says so', () => {
    const e = encounter({ combatants: [dragon(), tor], currentCombatantId: 't' });
    const rows = legendaryCard(e, e.combatants[0])?.rows ?? [];
    expect(rows[0].disabled).toBe(false);
    expect(rows[2].disabled).toBe(true);
    expect(rows[2].reason).toBe('Custa 2; restam 1.');
  });

  it('turns every row off and reads "Depois" when the creature cannot act', () => {
    const e = encounter({
      combatants: [dragon({ usableNow: false, actionsLeft: 3 }), tor],
      currentCombatantId: 't',
    });
    const card = legendaryCard(e, e.combatants[0]);
    expect(card?.badge).toBe('Depois');
    expect(card?.unusable).toBe(LEGENDARY_UNUSABLE);
    expect(card?.rows.every((r) => r.disabled && r.reason === LEGENDARY_UNUSABLE)).toBe(true);
  });

  it('waits behind the open windows: "Depois", the rows held and the reason said', () => {
    const win = (id: string) =>
      reactionWindow({ id, kind: ReactionKind.PARRY, reactorId: 'c', reactorLabel: 'Capitão' });
    const e = encounter({
      combatants: [dragon({ actionsLeft: 3 }), tor],
      currentCombatantId: 't',
      reactionWindows: [win('a'), win('b')],
    });
    const card = legendaryCard(e, e.combatants[0], { ahead: 2, playerWaits: true });
    expect(card?.badge).toBe('Depois');
    expect(card?.blocked).toBe(true);
    expect(card?.waiting).toBe(
      'Depois das duas janelas acima: enquanto uma reação espera, nada anda.',
    );
    expect(waitingBehind({ ahead: 1, playerWaits: true })).toBe(
      'Depois da janela acima: enquanto uma reação espera, nada anda.',
    );
    expect(waitingBehind(NO_HOLD)).toBe('');
  });

  it('folds to "Ações lendárias · 3 de 3" as plain data', () => {
    const e = encounter({
      combatants: [dragon({ actionsLeft: 3 }), tor],
      currentCombatantId: 't',
    });
    const card = legendaryCard(e, e.combatants[0]);
    expect(card && legendaryFolded(card)).toBe('Ações lendárias · 3 de 3');
  });

  it('has no card for a creature without options or on its own turn, and says what came back instead', () => {
    const e = encounter({ combatants: [dragon({ actionsLeft: 3 }), tor], currentCombatantId: 'd' });
    expect(legendaryCards(e)).toEqual([]);
    const note = legendaryRecovered(e, e.combatants[0]);
    expect(note?.actions).toBe('Ações lendárias: 3 de 3, renovadas agora');
    expect(note?.resistance).toBe('Resistência Lendária: 2 de 3 hoje');
    expect(note?.note).toBe('Ações lendárias se usam no turno de outra criatura, nunca no dele.');
    expect(legendaryRecovered(e, tor)).toBeNull();
    // What the server says is left, not the round's whole: a creature that could not renew says so, and a fallen one has no note.
    const short = encounter({
      combatants: [dragon({ actionsLeft: 1 }), tor],
      currentCombatantId: 'd',
    });
    expect(legendaryRecovered(short, short.combatants[0])?.actions).toBe('Ações lendárias: 1 de 3');
    const down = encounter({
      combatants: [{ ...dragon({ actionsLeft: 3 }), defeated: true } as never, tor],
      currentCombatantId: 'd',
    });
    expect(legendaryRecovered(down, down.combatants[0])).toBeNull();
    const plain = encounter({ combatants: [tor], currentCombatantId: 'x' });
    expect(legendaryCards(plain)).toEqual([]);
  });

  it('leaves the resistance line out for a stat block without the trait', () => {
    const e = encounter({
      combatants: [dragon({ resistancePerDay: 0, resistanceLeft: 0 })],
      currentCombatantId: 'd',
    });
    expect(legendaryRecovered(e, e.combatants[0])?.resistance).toBe('');
  });

  it('says a player waits when the table is held but the master has nothing to answer', () => {
    const e = encounter({ combatants: [dragon({ actionsLeft: 3 }), tor], currentCombatantId: 't' });
    const card = legendaryCard(e, e.combatants[0], { ahead: 0, playerWaits: true });
    expect(card?.waiting).toBe('Esperando a reação de um jogador.');
    expect(card?.blocked).toBe(true);
    expect(card?.badge).toBe('Depois');
  });

  it('words the unusable reason for a fallen creature and for a combat that is not running', () => {
    const fallen = { ...dragon({ usableNow: false }), defeated: true } as never;
    const e = encounter({ combatants: [fallen, tor], currentCombatantId: 't' });
    expect(legendaryCard(e, e.combatants[0])?.unusable).toBe(
      'Caído: a criatura não pode usar ações lendárias agora.',
    );
    const over = encounter({
      status: EncounterStatus.ENDED,
      combatants: [dragon({ usableNow: false }), tor],
      currentCombatantId: '',
    });
    expect(legendaryCard(over, over.combatants[0])?.unusable).toBe(
      'O combate não está em andamento.',
    );
  });

  it('trusts the server on what is affordable and spells the cost only when it is the reason', () => {
    const odd = [{ ...options[0], affordable: false, cost: 1 }];
    const e = encounter({
      combatants: [dragon({ options: odd, actionsLeft: 2 }), tor],
      currentCombatantId: 't',
    });
    const row = legendaryCard(e, e.combatants[0])?.rows[0];
    expect(row?.disabled).toBe(true);
    expect(row?.reason).toBe('Indisponível agora.');
    const ok = [{ ...options[2], affordable: true, cost: 2 }];
    const f = encounter({
      combatants: [dragon({ options: ok, actionsLeft: 1 }), tor],
      currentCombatantId: 't',
    });
    expect(legendaryCard(f, f.combatants[0])?.rows[0].disabled).toBe(false);
  });
});

describe('the Portuguese name of a spent option in the log', () => {
  const other = combatant({
    id: 'd2',
    label: 'Outro',
    legendary: {
      actionsPerRound: 3,
      actionsLeft: 3,
      resistancePerDay: 0,
      resistanceLeft: 0,
      usableNow: true,
      options: [{ ...options[2], namePt: 'Golpe de asa' }],
    },
  } as never);

  it('keeps two creatures with the same English option name apart', () => {
    const e = encounter({ combatants: [dragon(), other] });
    const names = legendaryNames(e);
    expect(legendaryOptionName(names, 'd', 'Wing Attack')).toBe('Ataque de asas');
    expect(legendaryOptionName(names, 'd2', 'Wing Attack')).toBe('Golpe de asa');
  });

  it('keeps the names of a creature that left the combat, and guesses none that is ambiguous', () => {
    const before = legendaryNames(encounter({ combatants: [dragon(), other] }));
    const after = legendaryNames(encounter({ combatants: [other] }), before);
    expect(legendaryOptionName(after, 'd', 'Wing Attack')).toBe('Ataque de asas');
    // An actor nobody knows: the one name only if every creature agrees, else unknown (never a wrong creature's).
    expect(legendaryOptionName(before, 'gone', 'Wing Attack')).toBeNull();
    expect(legendaryOptionName(before, 'gone', 'Detect')).toBe('Detectar');
    expect(legendaryOptionName(undefined, 'd', 'Detect')).toBeNull();
  });
});
