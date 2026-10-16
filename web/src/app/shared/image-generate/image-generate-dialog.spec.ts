import { create } from '@bufbuild/protobuf';
import { TestBed, type ComponentFixture } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';
import { Subject } from 'rxjs';

import {
  GetImageGenerationResponseSchema,
  ImageAspectRatio,
  ImageGenerationBlockedReason,
  ImageGenerationFailure,
  ImageGenerationKind,
  ImageGenerationState,
  ImageStyle,
} from '../../../gen/meurpg/maps/v1/imagegen_pb';
import { galleryImage } from '../../core/images/gallery-testing';
import { ImageGenClient } from '../../core/images/imagegen-client';
import {
  FakeImageGenClient,
  NPCS,
  blocked,
  done,
  edit,
  generation,
  imageStatus,
  invalid,
  reference,
} from '../../core/images/imagegen-testing';
import { ImageRun } from '../../core/images/imagegen-run';
import { ShownImageClient } from '../../core/images/shown-client';
import { MapsClient } from '../../core/maps/maps-client';
import { RosterClient, type RosterEntry } from '../../core/maps/roster-client';
import { OpenSessionLookup } from '../../core/play/open-session';
import { mapMessage } from '../../core/maps/maps-testing';
import {
  type GenerateOutcome,
  type ImageGenerateData,
  ImageGenerateDialog,
} from './image-generate-dialog';
import { nextTurn } from '../../../test-settle';

const plain = (t: string | null | undefined) =>
  (t ?? '')
    .replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
const flush = async () => {
  for (let i = 0; i < 6; i++) {
    await nextTurn();
  }
};

class FakeShown {
  shown: string[] = [];
  failWith: unknown = null;
  async show(_c: string, imageId: string): Promise<void> {
    if (this.failWith) {
      throw this.failWith;
    }
    this.shown.push(imageId);
  }
}

describe('ImageGenerateDialog (MR-039, RN-28; E10-07)', () => {
  let api: FakeImageGenClient;
  let shown: FakeShown;
  let closed: GenerateOutcome[];
  let escape: Subject<KeyboardEvent>;
  let sessionOpen: boolean;
  let roster: RosterEntry[];
  let mapTokens: { characterId: string }[];

  const mapOrigin: ImageGenerateData['origin'] = {
    kind: 'map',
    mapId: 'map-1',
    name: 'Masmorra de Mirathel',
    hasGrid: true,
    revealed: true,
  };

  async function setup(data: Partial<ImageGenerateData> = {}) {
    closed = [];
    escape = new Subject<KeyboardEvent>();
    const dialogData: ImageGenerateData = { campaignId: 'camp-1', origin: mapOrigin, ...data };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: ImageGenClient, useValue: api },
        { provide: ShownImageClient, useValue: shown },
        {
          provide: OpenSessionLookup,
          useValue: {
            currentMap: async () => (sessionOpen ? { sessionNumber: 7, mapId: '' } : null),
          },
        },
        { provide: RosterClient, useValue: { list: async () => roster } },
        { provide: MapsClient, useValue: { get: async () => ({ tokens: mapTokens }) } },
        { provide: MAT_DIALOG_DATA, useValue: dialogData },
        {
          provide: MatDialogRef,
          useValue: {
            close: (r: GenerateOutcome) => closed.push(r),
            keydownEvents: () => escape,
            backdropClick: () => new Subject(),
            disableClose: false,
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(ImageGenerateDialog);
    fixture.detectChanges();
    await settle(fixture);
    return { fixture, el: fixture.nativeElement as HTMLElement, ui: ui(fixture) };
  }

  async function settle(fixture: ComponentFixture<unknown>) {
    await flush();
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
  }

  function ui(fixture: ComponentFixture<unknown>) {
    const el = fixture.nativeElement as HTMLElement;
    const buttons = () => Array.from(el.querySelectorAll<HTMLButtonElement>('button'));
    const button = (text: string) => buttons().find((b) => plain(b.textContent).includes(text));
    return {
      button,
      async press(text: string) {
        const b = button(text);
        if (!b) {
          throw new Error(
            `no button "${text}" in: ${buttons()
              .map((x) => plain(x.textContent))
              .join(' | ')}`,
          );
        }
        b.click();
        await settle(fixture);
      },
      async type(label: string, value: string) {
        const area = Array.from(el.querySelectorAll('mat-form-field'))
          .find((f) => plain(f.querySelector('mat-label')?.textContent) === label)
          ?.querySelector('textarea');
        if (!area) {
          throw new Error(`no field "${label}"`);
        }
        area.value = value;
        area.dispatchEvent(new Event('input'));
        await settle(fixture);
      },
      radio: (name: string) =>
        Array.from(el.querySelectorAll<HTMLElement>('[role="radio"]')).find((r) =>
          plain(r.textContent).includes(name),
        ),
      text: () => plain(el.textContent),
      title: () => plain(el.querySelector('h2')?.textContent),
      sub: () => plain(el.querySelector('.frame__sub')?.textContent),
      alert: () => plain(el.querySelector('[role="alert"] p')?.textContent),
    };
  }

  beforeEach(() => {
    api = new FakeImageGenClient();
    api.references.set(ImageGenerationKind.MAP_SCENE, reference({ creatures: NPCS.slice(0, 3) }));
    api.references.set(ImageGenerationKind.TEXTURED_MAP, reference({ roomsListed: 9 }));
    shown = new FakeShown();
    sessionOpen = true;
    roster = [];
    mapTokens = [];
    ImageRun.retryDelayMs = 0;
  });

  afterEach(() => {
    ImageRun.retryDelayMs = 1000;
  });

  describe('from a map: the form (state 1)', () => {
    it('opens on "Gerar imagem" with the map\'s name, the three ways, what goes along, who appears, the references, the text, the style and the month', async () => {
      const { ui: u } = await setup();
      expect(u.title()).toBe('Gerar imagem');
      expect(u.sub()).toBe('Masmorra de Mirathel');
      expect(
        ['Arte da cena', 'Vista isométrica', 'O mapa com textura'].map((n) =>
          u.radio(n)?.getAttribute('aria-checked'),
        ),
      ).toEqual(['true', 'false', 'false']);
      expect(u.text()).toContain('O que vai junto');
      expect(u.text()).toContain('A imagem parte do que os jogadores veem agora.');
      expect(u.text()).toContain('Quem aparece na imagem');
      expect(u.text()).toContain('0 de 3 marcados');
      expect(u.text()).toContain('Capitão Goblin');
      expect(u.text()).toContain('uma criatura que eles não veem não está na lista');
      expect(u.text()).toContain('Referências da galeria');
      expect(u.text()).toContain('Objetos: 0 de 10 · Personagens: 0 de 4.');
      expect(u.text()).toContain('Descreva o lugar');
      expect(u.text()).toContain('0 de 500 caracteres');
      expect(u.text()).toContain('Estilo');
      expect(u.text()).toContain('Restam 17 de 20 imagens em outubro.');
      expect(u.text()).toContain('marca d’água invisível (SynthID)');
    });

    it('says under every field what goes to Google: the text and references, the style, the ratio and the references', async () => {
      const { ui: u } = await setup();
      expect(u.text()).toContain(
        'vão para o Google (API do Gemini) para gerar a imagem. Não escreva nomes de pessoas.',
      );
      expect(u.text()).toContain('O estilo vai ao Google como uma palavra');
      expect(u.text()).toContain('A proporção vai ao Google');
      expect(u.text()).toContain(
        'As imagens escolhidas vão ao Google como referência. Não use foto de pessoa.',
      );
    });

    it("draws the server's drawing of the players' view, and asked for it for the scene art and for the textured map", async () => {
      const { el } = await setup();
      expect(el.querySelector('img.along__shot')?.getAttribute('src')).toMatch(
        /^data:image\/png;base64,/,
      );
      expect(api.referenced.sort()).toEqual(
        [ImageGenerationKind.MAP_SCENE, ImageGenerationKind.TEXTURED_MAP].sort(),
      );
    });

    it('keeps "Gerar imagem" dashed (aria-disabled, with the reason beside it) until there is text', async () => {
      const { ui: u } = await setup();
      const generate = u.button('Gerar imagem')!;
      expect(generate.getAttribute('aria-disabled')).toBe('true');
      expect(
        plain(document.getElementById(generate.getAttribute('aria-describedby')!)?.textContent),
      ).toBe('Escreva o que a imagem mostra para gerar.');
      generate.click();
      await flush();
      expect(api.asks).toEqual([]);
      await u.type('Descreva o lugar', 'Uma cripta úmida');
      expect(u.button('Gerar imagem')!.getAttribute('aria-disabled')).not.toBe('true');
    });

    it("counts the text against the server's limit", async () => {
      const { ui: u } = await setup();
      await u.type('Descreva o lugar', 'Uma cripta úmida, tochas apagadas');
      expect(u.text()).toContain('33 de 500 caracteres');
    });
  });

  describe('the ways, and what each one hides', () => {
    it('the textured map takes no creature: "Quem aparece na imagem" and the ratio are gone, and the sentence says the map as a whole', async () => {
      const { ui: u } = await setup();
      expect(u.text()).toContain('Proporção');
      await u.radio('O mapa com textura')!.click();
      await flush();
      expect(u.text()).not.toContain('Quem aparece na imagem');
      expect(u.text()).not.toContain('Proporção');
      expect(u.text()).toContain('O desenho do chão e das paredes e a lista das 9 salas');
      expect(u.text()).toContain('ajustado à grade de 31 × 21 quadrados');
    });

    it("the textured map is dashed, with the reason, when the map's image is over 16 megapixels (`texture_too_large`)", async () => {
      api.references.set(
        ImageGenerationKind.TEXTURED_MAP,
        reference({ textureTooLarge: true, maxTexturePixels: 16000000n }),
      );
      const { ui: u } = await setup();
      const texture = u.radio('O mapa com textura')!;
      expect(texture.getAttribute('aria-disabled')).toBe('true');
      texture.click();
      await flush();
      expect(u.radio('Arte da cena')!.getAttribute('aria-checked')).toBe('true');
      expect(u.text()).toContain('A imagem deste mapa tem mais de 16 megapixels');
    });

    it("from a scene (no map) only the scene art is on, the others are dashed with the reason, and the text starts with the scene's name", async () => {
      const { ui: u, el } = await setup({
        origin: { kind: 'scene', name: 'Taverna do Corvo Branco' },
      });
      expect(u.sub()).toBe('Cena: Taverna do Corvo Branco');
      expect(
        ['Vista isométrica', 'O mapa com textura'].map((n) =>
          u.radio(n)?.getAttribute('aria-disabled'),
        ),
      ).toEqual(['true', 'true']);
      expect(u.text()).toContain(
        'As outras duas precisam de um mapa com grade. Abra o diálogo a partir de um mapa para usá\u2011las.',
      );
      expect(u.text()).toContain('Descreva a cena');
      expect(el.querySelector('textarea')?.value).toBe('Taverna do Corvo Branco');
      expect(u.text()).not.toContain('Quem aparece na imagem');
      expect(api.referenced).toEqual([]);
    });

    it('from a map without a grid: the same, with the reason about the grid', async () => {
      const { ui: u } = await setup({ origin: { ...mapOrigin, hasGrid: false } });
      expect(u.text()).toContain('Este mapa não tem grade');
      expect(u.radio('Vista isométrica')!.getAttribute('aria-disabled')).toBe('true');
    });

    it('arrow keys move to the next way that can be chosen, skipping the dashed one', async () => {
      api.references.set(ImageGenerationKind.TEXTURED_MAP, reference({ textureTooLarge: true }));
      const { ui: u, el } = await setup();
      el.querySelector('[role="radiogroup"]')!.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }),
      );
      await flush();
      expect(u.radio('Vista isométrica')!.getAttribute('aria-checked')).toBe('true');
      el.querySelector('[role="radiogroup"]')!.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }),
      );
      await flush();
      // The textured map is dashed: it wraps to the scene art.
      expect(u.radio('Arte da cena')!.getAttribute('aria-checked')).toBe('true');
    });
  });

  describe("the players' view", () => {
    it('lists only the NPCs the players see, marks them, and counts the portraits that go as references', async () => {
      const { ui: u } = await setup();
      const chip = (name: string) =>
        Array.from(document.querySelectorAll<HTMLElement>('[role="checkbox"]')).find((c) =>
          plain(c.textContent).includes(name),
        )!;
      chip('Capitão Goblin').click();
      await flush();
      chip('Goblin 1').click();
      await settle(TestBed.createComponent(ImageGenerateDialog) as never).catch(() => undefined);
      expect(u.text()).toContain('2 de 3 marcados');
      expect(u.text()).toContain('Personagens: 1 de 4 (Capitão Goblin, pelo marcado acima).');
    });

    it('with nobody on the map the drawing is black: it says so, and "Gerar imagem" is dashed with that reason', async () => {
      api.references.set(ImageGenerationKind.MAP_SCENE, reference({ playersSeeSomething: false }));
      const { ui: u } = await setup();
      await u.type('Descreva o lugar', 'Uma cripta');
      expect(u.text()).toContain('Os jogadores não veem nada deste mapa agora');
      expect(u.button('Gerar imagem')!.getAttribute('aria-disabled')).toBe('true');
      // The textured map starts from the whole map: it is not blocked by this.
      await u.radio('O mapa com textura')!.click();
      await flush();
      expect(u.button('Gerar imagem')!.getAttribute('aria-disabled')).not.toBe('true');
    });

    it('the list says there is no NPC the players see when it is empty, and keeps the sentence on why', async () => {
      api.references.set(ImageGenerationKind.MAP_SCENE, reference({ creatures: [] }));
      const { ui: u } = await setup();
      expect(u.text()).toContain('Nenhum NPC ou inimigo que os jogadores vejam agora.');
      expect(u.text()).toContain('uma criatura que eles não veem não está na lista');
    });

    it('says that the drawing could not be made, and tries again', async () => {
      api.references.set(ImageGenerationKind.MAP_SCENE, new Error('down'));
      const { ui: u } = await setup();
      expect(u.alert()).toContain('Não deu para montar o desenho do mapa.');
      api.references.set(ImageGenerationKind.MAP_SCENE, reference({ creatures: NPCS }));
      await u.press('Tentar de novo');
      expect(u.text()).toContain('Quem aparece na imagem');
    });
  });

  describe('asking and waiting (state 4)', () => {
    async function ask(u: ReturnType<typeof ui>) {
      await u.type('Descreva o lugar', 'Uma cripta úmida, tochas apagadas');
      await u.press('Gerar imagem');
    }

    it('scene art from a map: sends the kind, the marked NPCs by id and no portraits, then waits with words and a cancel', async () => {
      let release!: () => void;
      api.hold = new Promise<void>((r) => (release = r));
      api.polls = [done('img-1', 1)];
      const { ui: u } = await setup();
      Array.from(document.querySelectorAll<HTMLElement>('[role="checkbox"]'))
        .find((c) => plain(c.textContent).includes('Capitão Goblin'))!
        .click();
      await flush();
      await ask(u);
      const sent = api.asks[0];
      expect(sent).toMatchObject({
        via: 'map',
        request: {
          mapId: 'map-1',
          kind: ImageGenerationKind.MAP_SCENE,
          prompt: 'Uma cripta úmida, tochas apagadas',
          style: ImageStyle.OIL_PAINTING,
          objectImageIds: [],
          characterImageIds: [],
          npcCharacterIds: ['npc-capitao'],
        },
      });
      expect(u.title()).toBe('Gerar imagem');
      expect(u.sub()).toBe('O servidor está gerando a imagem');
      expect(u.text()).toContain('Seu pedido · Arte da cena · Pintura a óleo · 1 NPC');
      expect(u.text()).toContain('Uma cripta úmida, tochas apagadas');
      expect(u.text()).toContain('Gerando a imagem…');
      expect(u.text()).toContain('Costuma levar de 10 a 20 s.');
      expect(u.button('Parar de esperar')).toBeDefined();
      expect(u.button('Gerando…')!.getAttribute('aria-disabled')).toBe('true');
      expect(u.text()).toContain('Se o pedido ainda não saiu do servidor, a vaga do mês volta.');
      expect(api.polled[0]).toEqual({ generationId: 'gen-1', waitSeconds: 25 });
      release();
      await flush();
    });

    it('the textured map sends no creature and no ratio', async () => {
      api.polls = [done('img-t', 1, {}, { showsWholeMap: true })];
      const { ui: u } = await setup();
      await u.radio('O mapa com textura')!.click();
      await flush();
      await ask(u);
      expect(api.asks[0]).toMatchObject({
        via: 'map',
        request: {
          kind: ImageGenerationKind.TEXTURED_MAP,
          npcCharacterIds: [],
          characterImageIds: [],
          aspectRatio: ImageAspectRatio.IMAGE_ASPECT_RATIO_UNSPECIFIED,
        },
      });
    });

    it('from a scene it is `GenerateSceneImage`, with the ratio chosen', async () => {
      api.polls = [done('img-s', 1)];
      const { ui: u, el } = await setup({ origin: { kind: 'scene', name: 'Taverna' } });
      const select = el.querySelector('select')!;
      select.value = String(ImageAspectRatio.IMAGE_ASPECT_RATIO_21_9);
      select.dispatchEvent(new Event('change'));
      await flush();
      await u.press('Gerar imagem');
      expect(api.asks[0]).toMatchObject({
        via: 'scene',
        request: { prompt: 'Taverna', aspectRatio: ImageAspectRatio.IMAGE_ASPECT_RATIO_21_9 },
      });
    });

    it('"Cancelar" before the request left gives the slot back: back to the form, the text kept, and a calm line says so', async () => {
      let release!: () => void;
      api.hold = new Promise<void>((r) => (release = r));
      const { ui: u, el } = await setup();
      await ask(u);
      await u.press('Parar de esperar');
      release();
      await flush();
      expect(api.canceled).toEqual(['gen-1']);
      expect(u.text()).toContain('Pedido cancelado. A vaga do mês voltou.');
      expect(el.querySelector('textarea')?.value).toBe('Uma cripta úmida, tochas apagadas');
    });

    it('"Parar de esperar" after it left: the slot stays spent and the line says the picture, if it comes, goes to the gallery', async () => {
      let release!: () => void;
      api.hold = new Promise<void>((r) => (release = r));
      api.cancelResult = {
        generation: generation({ state: ImageGenerationState.CANCELED, slotSpent: true }),
        status: imageStatus({ remaining: 16 }),
      };
      const { ui: u } = await setup();
      await ask(u);
      await u.press('Parar de esperar');
      release();
      await flush();
      expect(u.text()).toContain(
        'A vaga do mês continua gasta e a imagem, se chegar, vai para a galeria.',
      );
    });

    it('closing while the request is in the air asks first ("Parar de esperar a imagem?"); "Continuar esperando" goes on', async () => {
      let release!: () => void;
      api.hold = new Promise<void>((r) => (release = r));
      api.polls = [done('img-1', 1)];
      const { ui: u, fixture } = await setup().then((s) => ({ ...s, fixture: s.fixture }));
      await ask(u);
      escape.next(new KeyboardEvent('keydown', { key: 'Escape' }));
      await settle(fixture);
      expect(u.text()).toContain('Parar de esperar a imagem?');
      expect(closed).toEqual([]);
      await u.press('Continuar esperando');
      expect(u.text()).not.toContain('Parar de esperar a imagem?');
      release();
      await flush();
    });

    it('confirming "Parar de esperar a imagem?" cancels the request and closes', async () => {
      let release!: () => void;
      api.hold = new Promise<void>((r) => (release = r));
      const { ui: u, fixture } = await setup();
      await ask(u);
      escape.next(new KeyboardEvent('keydown', { key: 'Escape' }));
      await settle(fixture);
      await u.press('Parar de esperar');
      release();
      await flush();
      expect(api.canceled).toEqual(['gen-1']);
      expect(closed).toHaveLength(1);
    });

    it('Esc with nothing in the air just closes, saying what was made', async () => {
      const { fixture } = await setup();
      escape.next(new KeyboardEvent('keydown', { key: 'Escape' }));
      await settle(fixture);
      expect(closed).toEqual([{ generated: 0, lastImageId: null, map: null }]);
    });
  });

  describe('the errors (state 8), by their detail', () => {
    async function writeAndAsk(u: ReturnType<typeof ui>) {
      await u.type('Descreva o lugar', 'Uma cripta');
      await u.press('Gerar imagem');
    }

    it('the month\'s limit: says how many were used and when they come back, the count goes to 0 and "Gerar imagem" is dashed', async () => {
      api.started = blocked(
        ImageGenerationBlockedReason.LIMIT_REACHED,
        imageStatus({ usedThisMonth: 20, remaining: 0 }),
      );
      const { ui: u } = await setup();
      await writeAndAsk(u);
      expect(u.alert()).toBe('Você usou as 20 imagens de outubro. Volta em 1º de novembro.');
      expect(u.text()).toContain('Restam 0 de 20 imagens em outubro.');
      expect(u.button('Gerar imagem')!.getAttribute('aria-disabled')).toBe('true');
    });

    it('a service that will not generate: the form returns with the words, the text kept and the slot said to be back', async () => {
      api.polls = [
        create(GetImageGenerationResponseSchema, {
          generation: generation({
            state: ImageGenerationState.FAILED,
            failure: ImageGenerationFailure.NO_IMAGE,
            slotSpent: false,
          }),
        }),
      ];
      const { ui: u, el } = await setup();
      await writeAndAsk(u);
      expect(u.alert()).toBe(
        'O serviço não gerou esta imagem. Tente descrever a cena de outro jeito. Esta tentativa não gastou nenhuma imagem do mês.',
      );
      expect(el.querySelector('textarea')?.value).toBe('Uma cripta');
    });

    it('generation off: the dialog says so and "Gerar imagem" is dashed', async () => {
      api.statusResult = imageStatus({ enabled: false });
      const { ui: u } = await setup();
      expect(u.text()).toContain('A geração de imagens não está ligada neste servidor.');
      expect(u.button('Gerar imagem')!.getAttribute('aria-disabled')).toBe('true');
    });

    it('the players see nothing (said by the server when the app asked anyway)', async () => {
      api.started = blocked(ImageGenerationBlockedReason.PLAYERS_SEE_NOTHING);
      const { ui: u } = await setup();
      await writeAndAsk(u);
      expect(u.alert()).toContain('Os jogadores não veem nada deste mapa agora');
    });

    it('a map image too large, and a refused NPC', async () => {
      api.started = blocked(ImageGenerationBlockedReason.MAP_IMAGE_TOO_LARGE);
      const { ui: u } = await setup();
      await writeAndAsk(u);
      expect(u.alert()).toContain('mais de 16 megapixels');
      api.started = invalid('npc_character_ids');
      await u.press('Gerar imagem');
      expect(u.alert()).toContain('Um dos NPCs marcados não aparece mais para os jogadores');
    });

    it('a request the server could not take because the service is down says so, with the text kept', async () => {
      api.started = new ConnectError('x', Code.Unavailable);
      const { ui: u } = await setup();
      await writeAndAsk(u);
      expect(u.alert()).toContain('Não foi possível falar com o servidor');
    });
  });

  describe('the result (states 5 and 6)', () => {
    async function generate(u: ReturnType<typeof ui>, kind?: string) {
      if (kind) {
        await u.radio(kind)!.click();
        await flush();
      }
      await u.type('Descreva o lugar', 'Uma cripta úmida');
      await u.press('Gerar imagem');
    }

    it('the scene art: "Imagem 1", saved in the gallery and hidden from the players until "Mostrar aos jogadores"', async () => {
      api.polls = [done('img-1', 1)];
      const { ui: u, el } = await setup();
      await generate(u);
      expect(u.title()).toBe('Imagem 1');
      expect(u.sub()).toBe('Masmorra de Mirathel · Arte da cena');
      expect(u.text()).toContain('Guardada na galeria');
      expect(u.text()).toContain('Escondida dos jogadores até você mostrar.');
      expect(u.text()).toContain('Gerada a partir do mapa · Pintura a óleo');
      expect(el.querySelector('img')?.getAttribute('src')).toBe('/images/img-1');
      expect(u.text()).not.toContain('Mostrada aos jogadores');
      expect(shown.shown).toEqual([]);
    });

    it('"Mostrar aos jogadores" is one touch, no question: the tag becomes "Mostrada aos jogadores" and the button goes', async () => {
      api.polls = [done('img-1', 1)];
      const { ui: u } = await setup();
      await generate(u);
      await u.press('Mostrar aos jogadores');
      expect(shown.shown).toEqual(['img-1']);
      expect(u.text()).toContain('Mostrada aos jogadores');
      expect(u.text()).toContain('Os jogadores veem esta imagem.');
      expect(u.button('Mostrar aos jogadores')).toBeUndefined();
    });

    it('showing with no open session says what to do', async () => {
      api.polls = [done('img-1', 1)];
      shown.failWith = new ConnectError('x', Code.FailedPrecondition);
      const { ui: u } = await setup();
      await generate(u);
      await u.press('Mostrar aos jogadores');
      expect(u.alert()).toContain('Não há uma sessão aberta');
      expect(u.button('Mostrar aos jogadores')).toBeDefined();
    });

    it('"Pedir o ajuste": needs the text, then asks `EditGeneratedImage` for this image, waits, and shows the chain with the current one marked', async () => {
      api.polls = [done('img-1', 1)];
      const { ui: u, el } = await setup();
      await generate(u);
      expect(u.button('Pedir o ajuste')!.getAttribute('aria-disabled')).toBe('true');
      expect(u.text()).toContain('Escreva o ajuste que você quer pedir.');
      expect(u.text()).toContain('gasta 1 das suas 16.');
      await u.type('Pedir um ajuste', 'mais escura, com uma ponte sobre o poço');
      api.polls = [done('img-2', 2)];
      api.editsResult = [
        edit(galleryImage('img-1', 'Imagem 1'), 1, 'Uma cripta úmida'),
        edit(galleryImage('img-2', 'Imagem 2'), 2, 'mais escura, com uma ponte sobre o poço'),
      ];
      await u.press('Pedir o ajuste');
      expect(api.asks.at(-1)).toEqual({
        via: 'edit',
        imageId: 'img-1',
        instruction: 'mais escura, com uma ponte sobre o poço',
      });
      expect(u.title()).toBe('Imagem 2');
      expect(u.sub()).toContain('Ajuste');
      expect(u.text()).toContain('A cadeia de ajustes');
      expect(plain(el.querySelector('.chain__now')?.textContent)).toBe('Imagem 2');
      expect(plain(el.querySelector('.chain__link')?.textContent)).toBe('Imagem 1');
      // The chain's link shows the other picture.
      await u.press('Imagem 1');
      expect(u.title()).toBe('Imagem 1');
      expect(el.querySelector('.shot img')?.getAttribute('src')).toBe('/images/img-1');
    });

    it('an adjustment that the service refused returns to the picture with the words, keeping the instruction', async () => {
      api.polls = [done('img-1', 1)];
      const { ui: u, el } = await setup();
      await generate(u);
      await u.type('Pedir um ajuste', 'mais escura');
      api.polls = [
        create(GetImageGenerationResponseSchema, {
          generation: generation({
            state: ImageGenerationState.REFUSED,
            failure: ImageGenerationFailure.REFUSED,
            slotSpent: false,
          }),
        }),
      ];
      await u.press('Pedir o ajuste');
      expect(u.title()).toBe('Imagem 1');
      expect(u.alert()).toContain('O serviço recusou este texto');
      expect((el.querySelector('textarea') as HTMLTextAreaElement).value).toBe('mais escura');
    });

    it('the textured map: the grid goes over it, "Usar como imagem do mapa" is there, and no "Mostrar aos jogadores"', async () => {
      api.polls = [done('img-t', 3, {}, { showsWholeMap: true })];
      const { ui: u, el } = await setup();
      await generate(u, 'O mapa com textura');
      expect(u.sub()).toBe('Masmorra de Mirathel · O mapa com textura');
      expect(u.text()).toContain('O mapa visto de cima, com a grade por cima (31 × 21 quadrados)');
      expect(el.querySelector('svg.shot__grid path')).not.toBeNull();
      expect(u.button('Usar como imagem do mapa')).toBeDefined();
      expect(u.button('Mostrar aos jogadores')).toBeUndefined();
    });

    it('"Usar como imagem do mapa" asks in place, says what stays, and uses the picture only on the second press', async () => {
      api.polls = [done('img-t', 3, {}, { showsWholeMap: true })];
      api.useResult = mapMessage('map-1', 'Masmorra de Mirathel');
      const { ui: u } = await setup();
      await generate(u, 'O mapa com textura');
      await u.press('Usar como imagem do mapa');
      expect(u.text()).toContain('Usar como imagem do mapa?');
      expect(u.text()).toContain(
        'A grade, as camadas e o que os jogadores já viram continuam. Os jogadores veem a nova imagem.',
      );
      // The adjustment's footer is out of the way while the question is open.
      expect(u.button('Pedir o ajuste')).toBeUndefined();
      expect(api.used).toEqual([]);
      // "Voltar" asks nothing.
      await u.press('Voltar');
      expect(u.text()).not.toContain('Usar como imagem do mapa?');
      expect(api.used).toEqual([]);
      await u.press('Usar como imagem do mapa');
      await u.press('Usar como imagem do mapa');
      expect(api.used).toEqual(['img-t']);
      expect(u.text()).toContain('Agora esta é a imagem do mapa.');
      // Closing tells the opener which map to show.
      escape.next(new KeyboardEvent('keydown', { key: 'Escape' }));
      await flush();
      expect(closed[0].map?.id).toBe('map-1');
      expect(closed[0].generated).toBe(1);
    });

    it('"gere de novo" when the map changed since: the question stays open with the reason above its buttons', async () => {
      api.polls = [done('img-t', 3, {}, { showsWholeMap: true })];
      api.useResult = blocked(ImageGenerationBlockedReason.MAP_CHANGED);
      const { ui: u } = await setup();
      await generate(u, 'O mapa com textura');
      await u.press('Usar como imagem do mapa');
      await u.press('Usar como imagem do mapa');
      expect(u.alert()).toContain('a imagem, a grade ou as paredes');
      expect(u.alert()).toContain('Gere de novo');
      expect(u.text()).toContain('Usar como imagem do mapa?');
    });
  });

  describe('opened on an older generated image (the gallery, E10-07 7)', () => {
    it('opens on its result with the chain and the adjustment, no form', async () => {
      api.editsResult = [
        edit(galleryImage('img-1', 'Imagem 1'), 1, 'Uma cripta'),
        edit(galleryImage('img-2', 'Imagem 2', { generated: true }), 2, 'mais escura'),
      ];
      const image = galleryImage('img-2', 'Imagem 2', { generated: true, parentImageId: 'img-1' });
      const { ui: u } = await setup({ origin: { kind: 'gallery' }, image });
      expect(u.title()).toBe('Imagem 2');
      expect(u.text()).toContain('A cadeia de ajustes');
      expect(u.text()).toContain('Pedido: mais escura');
      expect(u.button('Mostrar aos jogadores')).toBeDefined();
      expect(u.button('Pedir o ajuste')).toBeDefined();
      expect(api.referenced).toEqual([]);
    });
  });
  describe('fix round 1: the whole map, the keys, the name, the question', () => {
    async function ask(u: ReturnType<typeof ui>) {
      await u.type('Descreva o lugar', 'Uma cripta úmida');
      await u.press('Gerar imagem');
    }

    it('a textured map and every edit of it never offer "Mostrar aos jogadores": only "Usar como imagem do mapa" (RN-10)', async () => {
      api.polls = [done('img-t', 3, {}, { showsWholeMap: true })];
      const { ui: u, el } = await setup();
      await u.radio('O mapa com textura')!.click();
      await ask(u);
      expect(u.button('Mostrar aos jogadores')).toBeUndefined();
      expect(u.button('Usar como imagem do mapa')).toBeDefined();
      expect(u.text()).toContain(
        'Mostra o mapa inteiro, também o que os jogadores ainda não descobriram.',
      );
      // The edit of it is made at the map's size, says the same, and gets the grid too.
      await u.type('Pedir um ajuste', 'pedra mais clara');
      api.polls = [done('img-t2', 4, {}, { showsWholeMap: true })];
      api.editsResult = [
        edit(galleryImage('img-t', 'a', { showsWholeMap: true }), 3),
        edit(galleryImage('img-t2', 'b', { showsWholeMap: true }), 4),
      ];
      await u.press('Pedir o ajuste');
      expect(u.title()).toBe('Imagem 4');
      expect(u.button('Mostrar aos jogadores')).toBeUndefined();
      expect(u.button('Usar como imagem do mapa')).toBeDefined();
      expect(el.querySelector('svg.shot__grid path')).not.toBeNull();
      // And so does the chain's other picture.
      await u.press('Imagem 3');
      expect(u.button('Mostrar aos jogadores')).toBeUndefined();
      expect(shown.shown).toEqual([]);
    });

    it('a scene art is not whole-map: its edit still shows with one touch', async () => {
      api.polls = [done('img-1', 1)];
      const { ui: u } = await setup();
      await ask(u);
      expect(u.button('Mostrar aos jogadores')).toBeDefined();
      expect(u.button('Usar como imagem do mapa')).toBeUndefined();
    });

    it('from the gallery, showing a picture of the whole map asks first, in place, and shows only on the second press', async () => {
      api.editsResult = [
        edit(
          galleryImage('img-t', 'Mapa', { generated: true, showsWholeMap: true }),
          3,
          'a caverna',
        ),
      ];
      const image = galleryImage('img-t', 'Mapa', { generated: true, showsWholeMap: true });
      const { ui: u } = await setup({ origin: { kind: 'gallery' }, image });
      expect(u.button('Usar como imagem do mapa')).toBeUndefined();
      await u.press('Mostrar aos jogadores');
      expect(u.text()).toContain('Mostrar o mapa inteiro?');
      expect(u.text()).toContain(
        'Esta imagem mostra o mapa inteiro, também o que os jogadores ainda não descobriram.',
      );
      expect(shown.shown).toEqual([]);
      // The adjustment's button is out of the way while the question is open.
      expect(u.button('Pedir o ajuste')).toBeUndefined();
      await u.press('Voltar');
      expect(u.text()).not.toContain('Mostrar o mapa inteiro?');
      expect(shown.shown).toEqual([]);
      await u.press('Mostrar aos jogadores');
      await u.press('Mostrar mesmo assim');
      expect(shown.shown).toEqual(['img-t']);
      expect(u.text()).toContain('Mostrada aos jogadores');
    });

    it('"Mostrar aos jogadores" without an open session is dashed and says why, before any press', async () => {
      sessionOpen = false;
      api.polls = [done('img-1', 1)];
      const { ui: u } = await setup();
      await ask(u);
      const button = u.button('Mostrar aos jogadores')!;
      expect(button.getAttribute('aria-disabled')).toBe('true');
      expect(
        plain(document.getElementById(button.getAttribute('aria-describedby')!)?.textContent),
      ).toContain('Não há uma sessão aberta');
      button.click();
      await flush();
      expect(shown.shown).toEqual([]);
    });

    it('a lost answer is retried with the same key and spends one slot; the key changes only when the answer came or the form did', async () => {
      api.loseNextAnswer = true;
      api.polls = [done('img-1', 1)];
      const { ui: u } = await setup();
      await ask(u);
      expect(api.keys).toHaveLength(2);
      expect(api.keys[0]).toBe(api.keys[1]);
      expect(api.slotsSpent).toBe(1);
      expect(u.title()).toBe('Imagem 1');
    });

    it('a typed refusal is an answer: the next try has another key; so does a changed text', async () => {
      api.started = blocked(ImageGenerationBlockedReason.GALLERY_FULL);
      const { ui: u } = await setup();
      await ask(u);
      const firstKey = (api.asks[0] as { request: { idempotencyKey: string } }).request
        .idempotencyKey;
      await u.press('Gerar imagem');
      const secondKey = (api.asks[1] as { request: { idempotencyKey: string } }).request
        .idempotencyKey;
      expect(secondKey).not.toBe(firstKey);
      await u.type('Descreva o lugar', 'Outra cripta');
      await u.press('Gerar imagem');
      expect(
        (api.asks[2] as { request: { idempotencyKey: string } }).request.idempotencyKey,
      ).not.toBe(secondKey);
    });

    it('the same lost-answer try pressed again reuses its key', async () => {
      api.started = new ConnectError('down', Code.Unavailable);
      const { ui: u } = await setup();
      await ask(u);
      await u.press('Gerar imagem');
      const keys = api.asks.map(
        (a) => (a as { request: { idempotencyKey: string } }).request.idempotencyKey,
      );
      expect(keys.length).toBe(4); // two asks per press: the app tries once more
      expect(new Set(keys).size).toBe(1);
    });

    it('the close question puts its two answers in the footer, so there is one "Parar de esperar" and both buttons are always in view', async () => {
      let release!: () => void;
      api.hold = new Promise<void>((r) => (release = r));
      api.polls = [done('img-1', 1)];
      const { ui: u, fixture, el } = await setup();
      await ask(u);
      escape.next(new KeyboardEvent('keydown', { key: 'Escape' }));
      await settle(fixture);
      const foot = el.querySelector('.frame__foot')!;
      expect(Array.from(foot.querySelectorAll('button')).map((b) => plain(b.textContent))).toEqual([
        'Continuar esperando',
        'Parar de esperar',
      ]);
      const stops = Array.from(el.querySelectorAll('button')).filter(
        (b) => plain(b.textContent) === 'Parar de esperar',
      );
      expect(stops).toHaveLength(1);
      expect(el.querySelector('.ask__title')?.textContent).toContain('Parar de esperar a imagem?');
      // Esc again answers "Continuar esperando".
      escape.next(new KeyboardEvent('keydown', { key: 'Escape' }));
      await settle(fixture);
      expect(u.text()).not.toContain('Parar de esperar a imagem?');
      release();
      await flush();
    });

    it('polling stops when the dialog is closed', async () => {
      let release!: () => void;
      api.hold = new Promise<void>((r) => (release = r));
      api.polls = [done('img-1', 1)];
      const { ui: u, fixture } = await setup();
      await ask(u);
      const polled = api.polled.length;
      expect(polled).toBeGreaterThan(0);
      fixture.destroy();
      release();
      await flush();
      expect(api.polled.length).toBe(polled);
    });

    it('the picture is named: the form carries the name the master sees, and it follows the way until he writes one', async () => {
      const { ui: u, el } = await setup();
      const name = () =>
        Array.from(el.querySelectorAll('mat-form-field'))
          .find((f) => plain(f.querySelector('mat-label')?.textContent) === 'Nome da imagem')
          ?.querySelector('input') as HTMLInputElement;
      expect(name().value).toBe('Masmorra de Mirathel · arte da cena');
      await u.radio('Vista isométrica')!.click();
      await flush();
      expect(name().value).toBe('Masmorra de Mirathel · vista isométrica');
      name().value = 'A cripta vista de cima';
      name().dispatchEvent(new Event('input'));
      await u.radio('O mapa com textura')!.click();
      await flush();
      expect(name().value).toBe('A cripta vista de cima');
      api.polls = [done('img-1', 1)];
      expect(u.text()).toContain('Os jogadores leem este nome');
      await ask(u);
      expect(api.asks[0]).toMatchObject({
        via: 'map',
        request: { name: 'A cripta vista de cima' },
      });
    });

    it("from a revealed scene the name starts as the scene's; a hidden scene, a hidden map and the gallery start with the way and the day, never a secret name (RN-10)", async () => {
      const nameOf = (el: HTMLElement) =>
        (
          el.querySelector(
            'mat-form-field input[matinput]:not(textarea)',
          ) as HTMLInputElement | null
        )?.value ?? '';
      const scene = await setup({
        origin: { kind: 'scene', name: 'Taverna do Corvo Branco', revealed: true },
      });
      expect(nameOf(scene.el)).toBe('Taverna do Corvo Branco');
      const hiddenScene = await setup({ origin: { kind: 'scene', name: 'O covil secreto' } });
      expect(nameOf(hiddenScene.el)).toMatch(/^Arte da cena · \d\d\/\d\d$/);
      const hiddenMap = await setup({ origin: { ...mapOrigin, revealed: false } });
      expect(nameOf(hiddenMap.el)).toMatch(/^Arte da cena · \d\d\/\d\d$/);
      const gallery = await setup({ origin: { kind: 'gallery' } });
      expect(nameOf(gallery.el)).toMatch(/^Arte da cena · \d\d\/\d\d$/);
    });

    it('a refusal that says the form is out of date reads the list again, drops the NPCs no longer there and points at the field', async () => {
      const { ui: u, el } = await setup();
      Array.from(document.querySelectorAll<HTMLElement>('[role="checkbox"]'))
        .find((c) => plain(c.textContent).includes('Capitão Goblin'))!
        .click();
      await flush();
      await u.type('Descreva o lugar', 'Uma cripta');
      const readsBefore = api.referenced.length;
      api.references.set(ImageGenerationKind.MAP_SCENE, reference({ creatures: NPCS.slice(1) }));
      api.started = invalid('npc_character_ids');
      await u.press('Gerar imagem');
      await flush();
      expect(api.referenced.length).toBeGreaterThan(readsBefore);
      expect(u.alert()).toContain('Um dos NPCs marcados não aparece mais');
      expect(u.text()).toContain('0 de 2 marcados');
      expect(document.activeElement?.getAttribute('data-field')).toBe('npc');
      void el;
    });

    it('a map that changed or a view with nobody in it also reads the list again', async () => {
      const { ui: u } = await setup();
      await u.type('Descreva o lugar', 'Uma cripta');
      const readsBefore = api.referenced.length;
      api.started = blocked(ImageGenerationBlockedReason.PLAYERS_SEE_NOTHING);
      await u.press('Gerar imagem');
      await flush();
      expect(api.referenced.length).toBeGreaterThan(readsBefore);
    });

    it('the portraits of NPCs on the map that the players do not see are known, to leave them out of the reference picker', async () => {
      roster = [
        {
          id: 'npc-capitao',
          name: 'Capitão Goblin',
          kind: 2,
          playerUserId: '',
          classSummary: '',
          raceName: '',
          playerName: null,
          portraitImageId: 'img-capitao',
        },
        {
          id: 'npc-emboscado',
          name: 'Emboscado',
          kind: 2,
          playerUserId: '',
          classSummary: '',
          raceName: '',
          playerName: null,
          portraitImageId: 'img-emboscado',
        },
        {
          id: 'npc-longe',
          name: 'Longe',
          kind: 2,
          playerUserId: '',
          classSummary: '',
          raceName: '',
          playerName: null,
          portraitImageId: 'img-longe',
        },
      ];
      // Capitão is on the list the players see; Emboscado is on the map but not seen; Longe has no token here.
      mapTokens = [{ characterId: 'npc-capitao' }, { characterId: 'npc-emboscado' }];
      const { fixture } = await setup();
      const hidden = (
        fixture.componentInstance as unknown as { hiddenPortraits: () => ReadonlySet<string> }
      ).hiddenPortraits();
      expect([...hidden]).toEqual(['img-emboscado']);
    });
  });
  describe('fix round 2: the footer, the names', () => {
    it("the result's main action is in the fixed footer, after \"Pedir o ajuste\"; the question's two answers take the footer's place", async () => {
      api.polls = [done('img-t', 3, {}, { showsWholeMap: true })];
      const { ui: u, el } = await setup();
      await u.radio('O mapa com textura')!.click();
      await u.type('Descreva o lugar', 'Uma caverna');
      await u.press('Gerar imagem');
      // The words of each footer button, without the name of its icon.
      const foot = () =>
        Array.from(el.querySelector('.frame__foot')!.querySelectorAll('button')).map((b) =>
          plain(b.textContent).replace(/^(auto_awesome|map|visibility)/, ''),
        );
      expect(foot()).toEqual(['Pedir o ajuste', 'Usar como imagem do mapa']);
      await u.press('Usar como imagem do mapa');
      expect(foot()).toEqual(['Voltar', 'Usar como imagem do mapa']);
      expect(el.querySelector('.frame__body')?.textContent).toContain('Usar como imagem do mapa?');
      await u.press('Voltar');
      expect(foot().length).toBe(2);
    });

    it('a scene art\'s main action is "Mostrar aos jogadores" in the footer', async () => {
      api.polls = [done('img-1', 1)];
      const { ui: u, el } = await setup();
      await u.type('Descreva o lugar', 'Uma sala');
      await u.press('Gerar imagem');
      expect(
        Array.from(el.querySelector('.frame__foot')!.querySelectorAll('button')).map((b) =>
          plain(b.textContent).replace(/^(auto_awesome|map|visibility)/, ''),
        ),
      ).toEqual(['Pedir o ajuste', 'Mostrar aos jogadores']);
    });

    it("the heading is the image's name, never its number; from the gallery a textured map says its way", async () => {
      api.polls = [done('img-1', 7)];
      const first = await setup();
      await first.ui.type('Descreva o lugar', 'Uma sala');
      await first.ui.press('Gerar imagem');
      expect(first.ui.title()).toBe('Imagem 7');
      const image = galleryImage('img-t', 'Masmorra de Mirathel · mapa com textura', {
        generated: true,
        showsWholeMap: true,
      });
      api.editsResult = [edit(image, 5, 'a caverna')];
      const gallery = await setup({ origin: { kind: 'gallery' }, image });
      expect(gallery.ui.title()).toBe('Masmorra de Mirathel · mapa com textura');
      expect(gallery.ui.sub()).toBe('O mapa com textura');
    });

    it("the close question's buttons are the stacked pair (one over the other at 320)", async () => {
      api.hold = new Promise<void>(() => undefined);
      const { ui: u, fixture, el } = await setup();
      await u.type('Descreva o lugar', 'Uma sala');
      await u.press('Gerar imagem');
      escape.next(new KeyboardEvent('keydown', { key: 'Escape' }));
      await settle(fixture);
      expect(el.querySelector('.frame__foot .pair--stack')).not.toBeNull();
    });
  });
  describe('fix round 3', () => {
    it('while a question is open the body is its title and words only: no adjust field, no chain', async () => {
      api.polls = [done('img-t', 3, {}, { showsWholeMap: true })];
      api.editsResult = [
        edit(galleryImage('img-t0', 'a', { showsWholeMap: true }), 2),
        edit(galleryImage('img-t', 'b', { showsWholeMap: true }), 3),
      ];
      const { ui: u, el } = await setup();
      await u.radio('O mapa com textura')!.click();
      await u.type('Descreva o lugar', 'Uma caverna');
      await u.press('Gerar imagem');
      expect(el.querySelector('.edit')).not.toBeNull();
      await u.press('Usar como imagem do mapa');
      expect(el.querySelector('.edit')).toBeNull();
      expect(el.querySelector('.chain')).toBeNull();
      expect(u.text()).toContain('Usar como imagem do mapa?');
      await u.press('Voltar');
      expect(el.querySelector('.edit')).not.toBeNull();
    });

    it('every footer pair that can squeeze a label is the stacked pair, and the subtitle does not repeat the map in the heading', async () => {
      api.polls = [
        done(
          'img-t',
          3,
          {},
          { showsWholeMap: true, name: 'Masmorra de Mirathel · mapa com textura' },
        ),
      ];
      const { ui: u, el } = await setup();
      await u.radio('O mapa com textura')!.click();
      await u.type('Descreva o lugar', 'Uma caverna');
      await u.press('Gerar imagem');
      // The default name holds the map's name: the subtitle says only the way.
      expect(u.sub()).toBe('O mapa com textura');
      expect(el.querySelector('.frame__foot .pair--stack')).not.toBeNull();
      await u.press('Usar como imagem do mapa');
      expect(el.querySelector('.frame__foot .pair--stack')).not.toBeNull();
    });
  });
});
