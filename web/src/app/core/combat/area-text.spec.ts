import { type MessageInitShape, create } from '@bufbuild/protobuf';

import {
  AreaPlacement,
  AreaTargetSchema,
  CombatantState,
  CoverDegree,
  CoverSource,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { SpellAreaShape } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  allyWarning,
  announcement,
  areaRows,
  beforeHint,
  countText,
  masterAllyWarning,
  nobodyText,
  outOfRangeText,
  shapeText,
} from './area-text';

const plain = (t: string) => t.replace(/ /g, ' ');

const target = (over: MessageInitShape<typeof AreaTargetSchema>) =>
  create(AreaTargetSchema, {
    combatantId: 'x',
    label: 'Goblin 1',
    state: CombatantState.UNHURT,
    cover: CoverDegree.NONE,
    distanceFt: 10,
    ...over,
  });

describe('what the area picker says', () => {
  it('names each shape with its size in meters', () => {
    expect(plain(shapeText({ shape: SpellAreaShape.SPHERE, sizeFt: 20, widthFt: 0 }))).toBe(
      'esfera de 6 m de raio',
    );
    expect(plain(shapeText({ shape: SpellAreaShape.CONE, sizeFt: 15, widthFt: 0 }))).toBe(
      'cone de 4,5 m',
    );
    expect(plain(shapeText({ shape: SpellAreaShape.LINE, sizeFt: 100, widthFt: 5 }))).toBe(
      'linha de 30 m × 1,5 m',
    );
    expect(plain(shapeText({ shape: SpellAreaShape.CUBE, sizeFt: 15, widthFt: 0 }))).toBe(
      'cubo de 4,5 m',
    );
  });

  it('says how to place a sphere, with its radius in squares', () => {
    expect(
      plain(
        beforeHint(AreaPlacement.POINT, { shape: SpellAreaShape.SPHERE, sizeFt: 20, widthFt: 0 }),
      ),
    ).toBe(
      'Toque no mapa ou arraste para escolher o ponto. A esfera tem 6 m de raio (4 quadrados).',
    );
  });

  it('refuses a point out of range with the range and the distance', () => {
    expect(plain(outOfRangeText('Bola de Fogo', 150, 195))).toBe(
      'A Bola de Fogo vai até 45 m; esse ponto está a 58,5 m.',
    );
  });

  it('announces the distance, and the count only once the server answered', () => {
    expect(plain(announcement(AreaPlacement.POINT, 25, null, null))).toBe('Ponto a 7,5 m de você');
    expect(plain(announcement(AreaPlacement.POINT, 25, null, 4))).toBe(
      'Ponto a 7,5 m de você, 4 criaturas na área',
    );
    expect(announcement(AreaPlacement.DIRECTION, 0, { dx: 1, dy: 0 }, null)).toBe('Direção: leste');
  });

  it('warns the player of an ally in the area, of themselves, and of both', () => {
    const toren = target({ combatantId: 't', label: 'Toren', ally: true });
    const brisa = target({ combatantId: 'b', label: 'Brisa', ally: true });
    const self = target({ combatantId: 'p', label: 'Pensantus', self: true, ally: true });
    expect(allyWarning([toren], 'Bola de Fogo', 'Pensantus')).toEqual({
      strong: 'Toren está na área.',
      rest: 'A Bola de Fogo atinge aliados também. Mude o local se não quiser atingi-lo.',
    });
    expect(allyWarning([self], 'Bola de Fogo', 'Pensantus')?.strong).toBe(
      'Pensantus (você) está na área.',
    );
    const both = allyWarning([self, brisa, toren], 'Bola de Fogo', 'Pensantus');
    expect(both?.strong).toBe('Você e 2 aliados estão na área');
    expect(both?.rest).toBe('(Brisa e Toren).');
    expect(allyWarning([target({})], 'Bola de Fogo', 'Pensantus')).toBeNull();
  });

  it("warns the master of the NPC caster's allies", () => {
    const g1 = target({ combatantId: 'g1', label: 'Goblin 1', ally: true });
    const g2 = target({ combatantId: 'g2', label: 'Goblin 2', ally: true });
    expect(plain(masterAllyWarning([g1, g2], 'Zuk')?.strong ?? '')).toBe(
      'Goblin 1 e Goblin 2 são aliados do Zuk.',
    );
  });

  it('writes the cover against the point of origin, and no bonus for a save that is not Dexterity', () => {
    const rows = areaRows(
      [
        target({ cover: CoverDegree.THREE_QUARTERS, coverSource: CoverSource.MAP }),
        target({ combatantId: 'y', cover: CoverDegree.NONE }),
      ],
      true,
      'Destreza',
      AreaPlacement.POINT,
    );
    expect(rows[0].cover).toBe('Três quartos (do mapa): +5 no teste de Destreza');
    expect(rows[0].mark).toBe('three');
    expect(rows[1].cover).toBe('Sem cobertura');
    expect(plain(rows[1].distance)).toBe('a 3,0 m do ponto');
    const thunder = areaRows(
      [target({ cover: CoverDegree.HALF })],
      false,
      'Constituição',
      AreaPlacement.DIRECTION,
    );
    expect(thunder[0].cover).toBe('Sem bônus de cobertura: o teste é de Constituição');
    expect(thunder[0].mark).toBeNull();
  });

  it("counts the master's hidden ones, and only what a player sees for a player", () => {
    expect(plain(countText([target({}), target({ hidden: true })]))).toBe(
      '2 criaturas · 1 escondida',
    );
    expect(plain(countText([target({})]))).toBe('1 criatura');
  });

  it('confirms an area with nobody seen in it, and a list with nobody ticked, saying what is spent', () => {
    const grid = nobodyText(true, 3, false);
    expect(grid.strong).toBe('Ninguém que você vê está na área.');
    expect(plain(grid.rest)).toBe('A magia gasta o espaço de 3º nível e a sua ação mesmo assim.');
    expect(nobodyText(false, 3, false).strong).toBe('Ninguém está marcado.');
    expect(nobodyText(true, 0, true).rest).toBe('A magia gasta a sua ação bônus mesmo assim.');
  });

  it('says a released readied spell spends only the reaction: its slot went at Ready (#857)', () => {
    expect(nobodyText(true, 3, false, true).rest).toBe('A magia gasta a sua reação mesmo assim.');
  });
});
