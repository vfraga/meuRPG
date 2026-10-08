import { ComponentFixture, TestBed } from '@angular/core/testing';

import { MapPointKind, TrapState } from '../../../gen/meurpg/maps/v1/maps_pb';
import { mapPoint, mapToken } from '../../core/maps/maps-testing';
import { MapPointsList, pointSub } from './map-points-list';
import { MapTokensList, TokenToggle, tokenSub } from './map-tokens-list';

describe('MapPointsList', () => {
  let fixture: ComponentFixture<MapPointsList>;
  let el: HTMLElement;

  beforeEach(() => {
    fixture = TestBed.createComponent(MapPointsList);
    fixture.componentRef.setInput('points', [
      mapPoint('a', 'Taverna do Javali', { revealed: true }),
      mapPoint('b', 'Covil dos goblins', {
        revealed: false,
        kind: 2,
        targetMap: { id: 'm', name: 'Covil' } as never,
      }),
    ]);
    fixture.detectChanges();
    el = fixture.nativeElement;
  });

  it('says each state in words and offers the opposite action, naming the point', () => {
    const rows = Array.from(el.querySelectorAll('li'));
    expect(rows[0].textContent).toContain('Revelado');
    expect(rows[0].querySelector('button')?.getAttribute('aria-label')).toBe(
      'Esconder Taverna do Javali',
    );
    expect(rows[1].textContent).toContain('Escondido');
    expect(rows[1].querySelector('button')?.getAttribute('aria-label')).toBe(
      'Revelar aos jogadores Covil dos goblins',
    );
  });

  it('asks for the state the button names', () => {
    const asked: boolean[] = [];
    fixture.componentInstance.toggle.subscribe((t) => asked.push(t.revealed));
    el.querySelectorAll('button')[0].click();
    el.querySelectorAll('button')[1].click();
    expect(asked).toEqual([false, true]);
  });

  describe('traps, treasures and lights', () => {
    const found = { seconds: 1n, nanos: 0 } as never;
    function render(inputs: Record<string, unknown> = {}) {
      const f = TestBed.createComponent(MapPointsList);
      f.componentRef.setInput('points', [
        mapPoint('t', 'Fosso escondido', {
          kind: MapPointKind.TRAP,
          revealed: false,
          trap: { state: TrapState.ARMED, areaSize: 2, noticeDc: 15, findDc: 15 } as never,
        }),
        mapPoint('c', 'Baú de moedas', {
          kind: MapPointKind.TREASURE,
          revealed: false,
          treasureValuePo: 250,
        }),
        mapPoint('f', 'Baú achado', {
          kind: MapPointKind.TREASURE,
          revealed: false,
          treasureFoundAt: found,
        }),
        mapPoint('l', 'Tocha', {
          kind: MapPointKind.LIGHT,
          revealed: false,
          light: { presetKey: 'light:torch', brightFt: 20, dimFt: 20 } as never,
        }),
      ]);
      for (const [key, value] of Object.entries(inputs)) {
        f.componentRef.setInput(key, value);
      }
      f.detectChanges();
      return f;
    }
    const rows = (f: ComponentFixture<MapPointsList>) =>
      Array.from((f.nativeElement as HTMLElement).querySelectorAll('li.row'));
    const words = (n: Element) => (n.textContent ?? '').replace(/\s+/g, ' ');

    it('keeps the state words and the numbers of each row (Armada, Não encontrado, the DCs)', () => {
      const r = rows(render());
      expect(words(r[0])).toContain('Armada');
      expect(words(r[0])).toContain('notar CD 15');
      expect(words(r[0])).toContain('achar CD 15');
      expect(words(r[1])).toContain('Não encontrado');
      expect(words(r[2])).toContain('Encontrado');
      expect(words(r[2])).not.toContain('Escondido');
    });

    it('a light is the master\'s alone: "Só você vê", no "Escondido", no reveal button', () => {
      const light = rows(render())[3];
      expect(words(light)).toContain('Só você vê');
      expect(words(light)).not.toContain('Escondido');
      expect(light.querySelector('button')).toBeNull();
    });

    it('draws the chest, not an archive box', () => {
      const r = rows(render());
      expect(r[1].querySelector('app-chest-icon')).not.toBeNull();
      expect(r[1].textContent).not.toContain('inventory_2');
    });

    it('where the screen can mark a treasure, it offers "Marcar como encontrado" in place of "Revelar aos jogadores"', () => {
      const f = render({ people: [{ id: 'p1', name: 'Brisa', sub: 'Ladina 3 · Ana' }] });
      const chest = rows(f)[1];
      const button = chest.querySelector<HTMLButtonElement>('button')!;
      expect(button.textContent).toContain('Marcar como encontrado');
      expect(chest.textContent).not.toContain('Revelar aos jogadores');
      button.click();
      f.detectChanges();
      expect(words(chest)).toContain('Quem encontrou Baú de moedas?');
      expect(words(chest)).toContain('Brisa');
    });

    it('marks it with the people picked, at least one, and says who counts', () => {
      const f = render({
        people: [
          { id: 'p1', name: 'Brisa' },
          { id: 'p2', name: 'Toren' },
        ],
      });
      const marks: string[][] = [];
      f.componentInstance.markFound.subscribe((m) => marks.push([...m.characterIds]));
      rows(f)[1].querySelector<HTMLButtonElement>('button')!.click();
      f.detectChanges();
      const confirm = () =>
        Array.from((f.nativeElement as HTMLElement).querySelectorAll('.row__mark button')).find(
          (b) => b.textContent?.includes('Marcar como encontrado'),
        ) as HTMLButtonElement;
      confirm().click();
      expect(marks).toEqual([]);
      (f.nativeElement as HTMLElement)
        .querySelector<HTMLInputElement>('.row__mark input[type=checkbox]')!
        .click();
      f.detectChanges();
      confirm().click();
      expect(marks).toEqual([['p1']]);
    });

    it("without people (the session's list) a treasure keeps its reveal button", () => {
      const chest = rows(render())[1];
      expect(chest.querySelector('button')?.textContent).toContain('Revelar aos jogadores');
    });
  });

  describe('RP scene points in the live session', () => {
    function withScenes(open: string | null | undefined) {
      const f = TestBed.createComponent(MapPointsList);
      f.componentRef.setInput('points', [
        mapPoint('s1', 'A carroça tombada', {
          revealed: true,
          sceneActions: [{ id: 'a' } as never],
        }),
        mapPoint('s2', 'Vau do riacho', { revealed: true }),
        mapPoint('b', 'Emboscada', { kind: 1, revealed: true }),
      ]);
      f.componentRef.setInput('openScenePointId', open);
      f.detectChanges();
      return f;
    }
    const scenes = (f: ComponentFixture<MapPointsList>) =>
      Array.from(f.nativeElement.querySelectorAll('.row__scene'), (e) =>
        (e as HTMLElement).textContent?.replace(/\s+/g, ' ').trim(),
      );

    it('offers nothing where there is no session', () => {
      expect(withScenes(undefined).nativeElement.querySelector('.row__scene')).toBeNull();
    });

    it('offers "Abrir cena" on every scene, even one with no actions (question 63)', () => {
      const f = withScenes(null);
      expect(scenes(f)).toEqual(['chat_bubble_outlineAbrir cena', 'chat_bubble_outlineAbrir cena']);
      const asked: string[] = [];
      f.componentInstance.openScene.subscribe((p) => asked.push(p.id));
      f.nativeElement.querySelector('.row__scene button').click();
      expect(asked).toEqual(['s1']);
      expect(f.nativeElement.querySelector('.row__scene button').getAttribute('aria-label')).toBe(
        'Abrir cena A carroça tombada',
      );
    });

    it('offers "Trocar para esta cena" when another is open, and says when it is this one', () => {
      expect(scenes(withScenes('s2'))[0]).toContain('Trocar para esta cena');
      expect(scenes(withScenes('s1'))[0]).toBe('castCena aberta agora');
    });
  });

  it('waits on the button whose call is in flight', () => {
    fixture.componentRef.setInput('pendingId', 'a');
    fixture.detectChanges();
    expect(el.querySelectorAll('button')[0].disabled).toBe(true);
    expect(el.querySelectorAll('button')[1].disabled).toBe(false);
  });

  it('names the target of a Submapa under its name', () => {
    expect(
      pointSub(
        mapPoint('x', 'Torre', {
          kind: 2,
          targetMap: { id: 'm', name: 'Torre de Mirathel' } as never,
        }),
      ),
    ).toBe('Submapa: Torre de Mirathel');
    expect(pointSub(mapPoint('y', 'Emboscada', { kind: 1 }))).toBe('Batalha');
  });
});

describe('MapTokensList', () => {
  const info = new Map([['p', { classSummary: 'Mago 3', playerName: 'Vinicius' }]]);

  it('says "Mago 3, de Vinicius" for a player and "NPC, inimigo" for an enemy', () => {
    expect(tokenSub(mapToken('p', 'Pensantus'), info)).toBe('Mago 3, de Vinicius');
    expect(tokenSub(mapToken('e', 'Capitão', { kind: 2 }), info)).toBe('NPC, inimigo');
    expect(tokenSub(mapToken('x', 'Sem info'), new Map())).toBe('Personagem de jogador');
  });

  it('says whose creature a creature token is: its character_id is the owner\'s, never "Personagem de jogador"', () => {
    const fixture = TestBed.createComponent(MapTokensList);
    fixture.componentRef.setInput('tokens', [
      mapToken('p', 'Pensantus'),
      mapToken('p', 'Nanquim', { creatureId: 'raven' }),
    ]);
    fixture.componentRef.setInput('info', info);
    fixture.detectChanges();
    const subs = [...(fixture.nativeElement as HTMLElement).querySelectorAll('.row__sub')].map(
      (e) => e.textContent?.trim(),
    );
    expect(subs).toEqual(['Mago 3, de Vinicius', 'Criatura de Pensantus']);
    expect(tokenSub(mapToken('p', 'Nanquim', { creatureId: 'raven' }), info)).toBe(
      'Criatura de um personagem',
    );
  });

  it('shows Visível or Escondido and asks for the opposite', () => {
    const fixture = TestBed.createComponent(MapTokensList);
    fixture.componentRef.setInput('tokens', [
      mapToken('p', 'Pensantus'),
      mapToken('e', 'Capitão', { kind: 2, hidden: true }),
    ]);
    fixture.componentRef.setInput('info', info);
    fixture.detectChanges();
    const el: HTMLElement = fixture.nativeElement;
    const asked: TokenToggle[] = [];
    fixture.componentInstance.toggle.subscribe((t) => asked.push(t));
    expect(el.textContent).toContain('Visível');
    expect(el.textContent).toContain('Escondido');
    const buttons = el.querySelectorAll('button');
    expect(buttons[0].getAttribute('aria-label')).toBe('Esconder Pensantus');
    expect(buttons[1].getAttribute('aria-label')).toBe('Revelar aos jogadores Capitão');
    buttons[1].click();
    expect(asked[0].hidden).toBe(false);
    expect(asked[0].token.characterId).toBe('e');
  });

  describe('with a character and its creature on the map', () => {
    const tokens = [mapToken('p', 'Pensantus'), mapToken('p', 'Corvo', { creatureId: 'raven' })];

    function mount(pendingId: string | null = null) {
      const fixture = TestBed.createComponent(MapTokensList);
      fixture.componentRef.setInput('tokens', tokens);
      fixture.componentRef.setInput('info', info);
      fixture.componentRef.setInput('pendingId', pendingId);
      fixture.detectChanges();
      return fixture.nativeElement as HTMLElement;
    }

    it("offers no Esconder on the creature's row, which would hide its owner", () => {
      const rows = mount().querySelectorAll('li');
      expect(rows[0].querySelector('button')?.getAttribute('aria-label')).toBe(
        'Esconder Pensantus',
      );
      expect(rows[1].textContent).toContain('Corvo');
      expect(rows[1].textContent).toContain('Visível');
      expect(rows[1].querySelector('button')).toBeNull();
    });

    it('keys the rows by the token, so the two rows stay apart', () => {
      const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
      mount();
      const duplicates = warn.mock.calls.filter((c) => String(c[0]).includes('NG0955'));
      warn.mockRestore();
      expect(duplicates).toEqual([]);
    });
  });
});
