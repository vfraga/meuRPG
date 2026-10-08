import path from 'node:path';

import { expect, type APIResponse, type Browser, type BrowserContext, type BrowserContextOptions, type Locator, type Page } from '@playwright/test';

// devidp's issuer (deploy/local/compose.yaml). The browser resolves any
// *.localhost name to 127.0.0.1 by itself.
export const idpOrigin = process.env.E2E_IDP_ORIGIN ?? 'http://idp.localhost:9090';

export const sessionCookie = '__Host-meurpg_session';
export const loginCookie = '__Host-meurpg_login';

/** The test users devidp lists on its login page (oidctest.TestUsers). */
// 'Sessões Teste' is not in auth.setup.ts: only sessions.spec.ts uses it, and signs in for real.
export type TestUser = 'Mestre Teste' | 'Jogador Teste' | 'E-mail Não Verificado' | 'Sessões Teste';

/**
 * Signs in through devidp, the way a person would: open the sign-in URL,
 * pick a user on the provider's page, and land back on returnTo.
 *
 * It opens /auth/login directly, which is faster and keeps these tests about
 * the server. ui.spec.ts covers the same flow through the app's buttons.
 *
 * This hits the real, rate-limited `/auth/login` (`backend/internal/
 * identity/login.go`, shared with `/auth/callback`: 40 per client, so 20
 * sign-ins, then 1 every 3s). Only call it where a
 * fresh sign-in is the point of the test — `auth.setup.ts` (once per user,
 * per whole run), `login.spec.ts`, `ui.spec.ts`, and the signed-out half of
 * `invite.spec.ts`'s intent=campaign_invite test. Everything else reuses
 * the state `auth.setup.ts` already saved: see `authStatePath` and
 * `newSignedInContext` below. In particular, never call `signIn` for a
 * context that also calls `signOut` — signing out deletes the session on
 * the server, so it would break every other test sharing that state.
 */
export async function signIn(page: Page, user: TestUser = 'Mestre Teste', returnTo = '/'): Promise<void> {
  await page.goto(`/auth/login?return_to=${encodeURIComponent(returnTo)}`);
  await expect(page).toHaveURL((url) => url.origin === idpOrigin && url.pathname === '/authorize');
  await page.getByRole('button', { name: user, exact: true }).click();
  // A relative URL is resolved against baseURL: back on the app, at returnTo.
  await expect(page).toHaveURL(returnTo);
}

/**
 * Where `auth.setup.ts` saves each test user's signed-in `storageState`
 * (cookies), gitignored (`e2e/.gitignore`) since a session cookie is a
 * secret. `playwright.config.ts`'s "chrome" project depends on "setup", so
 * these files exist by the time any other test runs.
 */
export function authStatePath(user: TestUser): string {
  const slugs: Record<TestUser, string> = {
    'Mestre Teste': 'mestre-teste',
    'Jogador Teste': 'jogador-teste',
    // The third account is a second player where a table needs two (the fog of war, E9-03): its e-mail
    // is unverified, which changes nothing for a player.
    'E-mail Não Verificado': 'e-mail-nao-verificado',
    'Sessões Teste': 'sessoes-teste', // never saved: sessions.spec.ts signs in for real
  };
  const slug = slugs[user];
  return path.join(__dirname, '..', '.auth', `${slug}.json`);
}

/**
 * A new `BrowserContext` already signed in as `user`, from the state
 * `auth.setup.ts` saved — no `/auth/login` hit, so it doesn't touch the
 * rate limit. Use this instead of `browser.newContext()` + `signIn()` for
 * every context that just needs to act as a signed-in user, which is most
 * of them (see `signIn`'s doc comment for the exceptions).
 */
export function newSignedInContext(browser: Browser, user: TestUser, options: BrowserContextOptions = {}): Promise<BrowserContext> {
  return browser.newContext({ ...options, storageState: authStatePath(user) });
}

/**
 * Calls a unary Connect RPC with the JSON codec, through the page's own
 * cookies. Every Connect call must carry Connect-Protocol-Version: 1 (the
 * server's CSRF protection, docs/architecture.md#csrf).
 *
 * Use it for what the page does not show; ui.spec.ts asserts on the page.
 */
export function callRPC(page: Page, method: string, body: object = {}): Promise<APIResponse> {
  return page.request.post(`/${method}`, {
    data: body,
    headers: { 'Connect-Protocol-Version': '1' },
  });
}

export const getMe = (page: Page) => callRPC(page, 'meurpg.identity.v1.IdentityService/GetMe');
export const signOut = (page: Page) => callRPC(page, 'meurpg.identity.v1.IdentityService/SignOut');

/**
 * The layout size of an element: offsetWidth and offsetHeight, which a CSS
 * transform doesn't change. boundingBox() includes transforms, so two buttons
 * of a dialog still running its opening animation (a scale) can measure
 * different sizes a few milliseconds apart; compare sizes with this instead.
 */
export function layoutSize(locator: Locator): Promise<{ width: number; height: number }> {
  return locator.evaluate((el: HTMLElement) => ({ width: el.offsetWidth, height: el.offsetHeight }));
}

/**
 * The bounding box of an element, once it has one. boundingBox() answers null
 * while the element is detached, hidden or being re-rendered, and a zero-sized
 * box while it hasn't been laid out; this waits for a real one instead of
 * letting the caller read a field of null.
 */
export async function boxOf(locator: Locator): Promise<{ x: number; y: number; width: number; height: number }> {
  let box: { x: number; y: number; width: number; height: number } | null = null;
  await expect
    .poll(async () => {
      box = await locator.boundingBox();
      return box !== null && box.width > 0 && box.height > 0;
    })
    .toBe(true);
  return box!;
}

/**
 * Waits two animation frames: a click's handlers, and the render they cause,
 * have run by then. Use it before asserting that a click did nothing, or the
 * assertion passes before the click's effect could show.
 */
export async function afterRender(page: Page): Promise<void> {
  await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
}

export const day = 24 * 60 * 60 * 1000;

/**
 * Waits until `/campaigns` shows the caller's campaigns (or "Você ainda não
 * tem nenhuma campanha"), before anything touches the "Criar campanha" form.
 *
 * Before the redesign the list sat above the form and pushed it down when
 * it arrived, leaving an open "Modo de XP" panel outside the viewport (seen
 * locally, 30/09/2026, with hundreds of campaigns). The form now comes
 * first, but waiting still keeps every test on a loaded screen. The list is
 * the region named by the page's h1.
 */
export async function waitForCampaignList(page: Page): Promise<void> {
  await expect(
    page
      .getByRole('region', { name: 'Minhas campanhas' })
      .getByRole('list')
      .or(page.getByText('Você ainda não tem nenhuma campanha.')),
  ).toBeVisible();
}

/**
 * Creates a campaign through the "Criar campanha" form on `/campaigns`, the
 * way MR-001 asks for, and returns its id from the `/campaigns/<id>` URL
 * the app navigates to afterwards.
 *
 * Tests whose own story is not campaign creation (MR-002, MR-003) use this
 * as setup, so their assertions stay about invites and membership, not
 * about the form they don't need to prove again.
 */
export async function createCampaign(page: Page, name: string): Promise<string> {
  await page.goto('/campaigns');
  await waitForCampaignList(page);
  await page.getByLabel('Nome da campanha').fill(name);
  await page.getByLabel('Modo de XP').click();
  await page.getByRole('option', { name: 'Por inimigos derrotados' }).click();
  await page.getByRole('button', { name: 'Criar campanha' }).click();

  await expect(page).toHaveURL(/\/campaigns\/[^/]+$/);
  return page.url().split('/').pop()!;
}

// --- Etapa 4: characters, rules and the play stub --------------------------
//
// Phase 2 (integrator, 29/09/2026): the real backend, contracts and web
// screens landed (commits 84e43cd..43208d9). Every name and shape below was
// re-checked against the actual running stack — `proto/meurpg/{characters,
// rules,play}/v1/*.proto` in this worktree, the live `ListContent` and
// `CreateCharacter` responses (probed directly against localhost:8080), and
// the real templates under `web/src/app/pages/character-editor` and
// `character-sheet` — not guessed. Where something is still uncertain, the
// comment next to it says so explicitly; everything else here is confirmed.

/** One class/race/background build, used by both `createCharacterViaUI` (the
 * screen, selects by `name_pt`) and `createCharacterRPC`/`characterRpcBody`
 * (direct API setup, uses content keys) to create the same character two
 * ways. */
export interface CharacterBuild {
  name: string;
  raceKey: string;
  /** The race's `name_pt` (confirmed live: `ListContent` on this stack). */
  race: string;
  subraceKey?: string;
  subrace?: string;
  classKey: string;
  class: string;
  subclassKey?: string;
  subclass?: string;
  level: number;
  /** An SRD background's content key (e.g. "background:acolyte"). Mutually
   * exclusive with `background` (FullSheet.background is a oneof). */
  backgroundKey?: string;
  /** Custom background name (ADR-0008: the SRD only has "Acólito"/Acolyte).
   * Omitted picks `backgroundKey` instead of "Outro (personalizado)". */
  background?: string;
  backgroundSkillKeys: string[];
  backgroundSkills: string[];
  /** Skill proficiencies chosen beyond the background (the class's own skill
   * picks): content keys for the RPC path, `name_pt` for the UI path. */
  extraSkillKeys: string[];
  extraSkills: string[];
  scores: { for: number; des: number; con: number; int: number; sab: number; car: number };
}

/**
 * The Pensantus reference build (Rock Gnome Wizard 3, School of Evocation,
 * custom background "Sábio"): scores FOR 12 / DES 16 / CON 15 / INT 16 /
 * SAB 13 / CAR 12, class skills Investigação and Intuição. Fixed by the
 * Etapa 4 plan (§6) and the private ADR-0008 (not reproduced here: see
 * docs/adr/ in the main clone, gitignored in worktrees).
 *
 * Every content key and `name_pt` below was read live from this stack's
 * `ListContent` response and from `backend/internal/rules/srd51/effects/
 * names_pt.json` (both agree): race:gnome → "Gnomo", subrace:rock-gnome →
 * "Gnomo das Rochas", class:wizard → "Mago", subclass:evocation → "Escola
 * de Evocação", skill:arcana → "Arcanismo", skill:history → "História",
 * skill:investigation → "Investigação", skill:insight → "Intuição".
 *
 * `pensantusDerived` (below) records the numbers MR-004's spec checks
 * against the server's `GetCharacter` response — also confirmed live by
 * creating this exact build against the running stack: INT 18 after the +2
 * racial bonus gives modifier +4; proficiency +2 gives spell save DC 14 and
 * spell attack +6; HP 23 (the d6 hit die's max at level 1, plus two levels
 * of the fixed average, plus the +3 CON modifier each level); AC 13
 * unarmored (10 + the +3 DEX modifier, no shield or armor worn).
 */
export const pensantus: CharacterBuild = {
  name: 'Pensantus',
  raceKey: 'race:gnome',
  race: 'Gnomo',
  subraceKey: 'subrace:rock-gnome',
  subrace: 'Gnomo das Rochas',
  classKey: 'class:wizard',
  class: 'Mago',
  subclassKey: 'subclass:evocation',
  subclass: 'Escola de Evocação',
  level: 3,
  background: 'Sábio', // custom background, not an SRD content key (ADR-0008)
  backgroundSkillKeys: ['skill:arcana', 'skill:history'],
  backgroundSkills: ['Arcanismo', 'História'],
  extraSkillKeys: ['skill:investigation', 'skill:insight'],
  extraSkills: ['Investigação', 'Intuição'],
  scores: { for: 12, des: 16, con: 15, int: 16, sab: 13, car: 12 },
};

/** The server-computed numbers MR-004's spec checks for the `pensantus`
 * build, confirmed live against the running stack (see `pensantus`'s doc
 * comment). */
export const pensantusDerived = {
  intModifier: '+4',
  spellSaveDc: 14,
  spellAttackBonus: '+6',
  armorClass: 13,
  hitPointsMax: 23,
};

/**
 * The exact shape of `CharacterService`'s `failed_precondition` responses
 * (confirmed live): HTTP 400, `{code, message, details: [{type, value,
 * debug: {reason, characterId}}]}`. `aborted` (a stale `revision`) is HTTP
 * 409 instead, with no detail — Connect's standard code→HTTP mapping, not
 * the plan's guess of 400 for everything.
 */
export interface CharacterBlockedBody {
  code: string;
  message: string;
  details: Array<{ type: string; value: string; debug?: { reason: string; characterId?: string } }>;
}

/**
 * Asserts an `APIResponse` is a `CharacterService` `failed_precondition`
 * with a `CharacterBlocked` detail carrying the given reason (one of
 * `CHARACTER_BLOCKED_REASON_SHEET_LOCKED`, `_CHARACTER_DEAD`,
 * `_LIVING_CHARACTER_EXISTS`, `_STORY_LOCKED` — characters.proto's
 * `CharacterBlockedReason`).
 */
export async function expectCharacterBlocked(res: APIResponse, reason: string): Promise<void> {
  expect(res.status()).toBe(400);
  const body = (await res.json()) as CharacterBlockedBody;
  expect(body.code).toBe('failed_precondition');
  expect(body.details?.[0]?.type).toBe('meurpg.characters.v1.CharacterBlocked');
  expect(body.details?.[0]?.debug?.reason).toBe(reason);
}

/**
 * Opens an invite link in a new page of the given browser context, the way a
 * signed-in person accepting an invite would, and waits for the app to land
 * on the campaign it belongs to (MR-003's "o mestre já vê" half). The
 * context must already be signed in — typically via `newSignedInContext`,
 * not a fresh `signIn` call; the signed-out path is its own flow, already
 * covered directly in invite.spec.ts.
 */
export async function acceptInvite(context: BrowserContext, link: string): Promise<{ page: Page; campaignId: string }> {
  const page = await context.newPage();
  await page.goto(link);
  await expect(page).toHaveURL(/\/campaigns\/[^/]+$/);
  return { page, campaignId: page.url().split('/').pop()! };
}

/**
 * Opens a `mat-select` by its field label and picks the named option.
 * Opens by focusing the control and pressing Enter, not by clicking its
 * (pixel) center: the "Básico" step's fields sit in a compact CSS grid
 * (character-editor.scss) where an empty field's floating label can overlap
 * its own trigger's click point — observed live, a plain `.click()` on
 * "Antecedente" retries for the full timeout against "mat-label
 * intercepts pointer events" and never gets through. Keyboard interaction
 * sidesteps that entirely.
 *
 * Locates the control by role "combobox", not `getByLabel`: while open, the
 * option listbox shares the same `aria-labelledby` as the trigger (both
 * point at the form field's floating label), so `getByLabel` matches both
 * and turns strict mode against itself the moment the panel opens.
 *
 * An option of the table's own content carries its mark's words in its
 * accessible name ("Corujeiro Da mesa", table-mark.ts), on purpose: a screen
 * reader hears where the option comes from. So the option matches the name
 * alone or the name followed by those words, and nothing else ("Anão" never
 * matches "Anão da Colina").
 */
async function selectMatOption(page: Page, label: string, optionName: string): Promise<void> {
  const control = page.getByRole('combobox', { name: label, exact: true });
  await control.focus();
  await control.press('Enter');
  const name = optionName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const marks = '(?:\\s*(?:Da mesa|Arquivada|Arquivado|Desligada para os jogadores))*';
  await page.getByRole('option', { name: new RegExp(`^${name}${marks}$`) }).click();
  await expect(control).toHaveAttribute('aria-expanded', 'false');
}

/**
 * Creates a player character through the "Criar personagem" screen
 * (`/campaigns/:id/characters/new`), filling `build` (default:
 * `pensantus`) into the real `character-editor` (`web/src/app/pages/
 * character-editor/character-editor.html`).
 *
 * The stepper (`editor-stepper/`, on the CDK stepper) is non-linear: each
 * step's header is its own tab (`getByRole('tab', {name})`), and the submit
 * button sits under whichever step is open, so it is reachable from any
 * step (the "Passo anterior"/"Próximo passo" buttons at the end of each
 * step are a convenience only). This helper only visits the steps it
 * needs (Básico, Habilidades, Perícias) — Magias and Equipamento are both
 * fully optional and left at their defaults, and there is no separate
 * "História" step: the story is its own screen/RPC (amendment A3).
 *
 * `entryPath` defaults to the player's own "criar personagem" route; pass
 * `/campaigns/:id/npcs/new/:kind` (kind: enemy/boss — the full-sheet NPC
 * kinds) to fill the same stepper from the master's "Novo NPC" menu
 * instead. Either way it lands on, and returns, the same kind of URL.
 *
 * Returns the created character's id, read from the sheet page's URL after
 * submit (`/campaigns/:id/characters/:characterId`).
 */
export async function createCharacterViaUI(
  page: Page,
  campaignId: string,
  build: CharacterBuild = pensantus,
  entryPath: string = `/campaigns/${campaignId}/characters/new`,
): Promise<string> {
  await page.goto(entryPath);

  // `exact: true` throughout: a loose substring match can hit another field
  // of the same step, e.g. "Raça" also matching "Sub-raça", and "Força" also
  // matching the manual bonus field "Força (bônus manual)". (A step's content
  // is built when the step first opens, so this helper opens each one it
  // needs by its tab.)

  // Passo "Básico" (selected by default).
  await page.getByLabel('Nome do personagem', { exact: true }).fill(build.name);
  await selectMatOption(page, 'Raça', build.race);
  if (build.subrace) {
    await selectMatOption(page, 'Sub-raça', build.subrace);
  }
  await selectMatOption(page, 'Classe', build.class);
  // The subclass field waits for the level the class chooses it at: the level goes in first.
  await page.getByLabel('Nível', { exact: true }).fill(String(build.level));
  if (build.subclass) {
    await selectMatOption(page, 'Subclasse', build.subclass);
  }
  await selectMatOption(page, 'Antecedente', build.background ? 'Outro (personalizado)' : 'Acólito');
  if (build.background) {
    await page.getByLabel('Nome do antecedente', { exact: true }).fill(build.background);
    // The background's own two skills: a separate checkbox group
    // (aria-labelledby="background-skills-label"), right below "Nome do
    // antecedente" on this same "Básico" step. Scoped to that group, not
    // just matched by name: the same skill also has a checkbox of its own
    // in the "Perícias" step's full skill list (also always in the DOM —
    // see the note above), so an unscoped role/name match would be
    // ambiguous whenever a background skill and a class skill coincide.
    const backgroundSkillsGroup = page.locator('[aria-labelledby="background-skills-label"]');
    for (const skill of build.backgroundSkills) {
      await backgroundSkillsGroup.getByRole('checkbox', { name: skill, exact: true }).check();
    }
  }

  // Passo "Habilidades": jump there by clicking its tab (no "next" button).
  await page.getByRole('tab', { name: 'Habilidades' }).click();
  // A player makes the scores by one of the table's ways (RN-24, all four allowed by default): "Digitar" is the one
  // that takes the six numbers as they are. The master's NPCs have the free fields and no choice of way.
  const typedWay = page.locator('.seg__item').filter({ hasText: 'Digitar' });
  if ((await typedWay.count()) > 0) {
    await typedWay.click();
  }
  // The input itself: while the page switches how the scores are made, another
  // element labelled with the ability's name (a total, a medallion) can be the first match.
  await page.locator('input').and(page.getByLabel('Força', { exact: true })).fill(String(build.scores.for));
  await page.locator('input').and(page.getByLabel('Destreza', { exact: true })).fill(String(build.scores.des));
  await page.locator('input').and(page.getByLabel('Constituição', { exact: true })).fill(String(build.scores.con));
  await page.locator('input').and(page.getByLabel('Inteligência', { exact: true })).fill(String(build.scores.int));
  await page.locator('input').and(page.getByLabel('Sabedoria', { exact: true })).fill(String(build.scores.sab));
  await page.locator('input').and(page.getByLabel('Carisma', { exact: true })).fill(String(build.scores.car));
  // Pontos de Vida (Média/Rolado) defaults to "Média" — matches `pensantus`
  // (fixed/average hit points), so nothing to select here.

  // Passo "Perícias": only the class's own skill picks, scoped to this
  // step's own group (see `backgroundSkillsGroup` above for why — the same
  // skill name can also appear as a background skill). Each skill has two
  // checkboxes here, itself and "Expertise"; this checks only the first.
  await page.getByRole('tab', { name: 'Perícias' }).click();
  const classSkillsGroup = page.getByRole('group', { name: 'Perícias', exact: true });
  for (const skill of build.extraSkills) {
    await classSkillsGroup.getByRole('checkbox', { name: skill, exact: true }).check();
  }

  // "Criar personagem" for a player, "Criar NPC" from the master's menu.
  await page.getByRole('button', { name: /^Criar (personagem|NPC)$/ }).click();

  // `(?!new$)`: the player's own entry path, `/characters/new`, already
  // matches `/characters/<anything>`, so without it this would return
  // "new" whenever the check ran before the app navigated to the sheet.
  await expect(page).toHaveURL(/\/campaigns\/[^/]+\/characters\/(?!new$)[^/]+$/);
  return page.url().split('/').pop()!;
}

/**
 * Creates a character directly through the API
 * (`meurpg.characters.v1.CharacterService/CreateCharacter`), bypassing the
 * editor screen. For specs whose acceptance criterion isn't character
 * creation itself (MR-005, MR-006/RN-01, RN-03, RN-11): they need a
 * character to exist, not a second proof that the form works.
 *
 * `body` is sent as-is next to `campaignId` (protojson accepts both
 * camelCase and proto_name fields); pass at least `kind` and `name`, and a
 * `sheet` for a full-sheet kind. See `characterRpcBody` for a body built
 * from a `CharacterBuild`.
 */
export function createCharacterRPC(page: Page, campaignId: string, body: Record<string, unknown>): Promise<APIResponse> {
  return callRPC(page, 'meurpg.characters.v1.CharacterService/CreateCharacter', { campaignId, ...body });
}

/**
 * Builds a `CreateCharacter` request body from a `CharacterBuild`, in
 * `FullSheet`'s exact wire shape (characters.proto, confirmed live against
 * this stack): `baseScores` uses the ability's full name (`strength`, not
 * `str`); the race/subrace/class/subclass are content keys under `raceKey`
 * /`subraceKey`/`classes[].classKey`/`classes[].subclassKey`; the
 * background is a oneof (`backgroundKey` xor `customBackground`); skills
 * the player picked beyond the background are `skillProficiencyKeys`; and
 * `hitPoints.method` is `"HIT_POINTS_METHOD_AVERAGE"` for `pensantus`
 * (`HitPointsMethod` enum, characters.proto).
 */
export function characterRpcBody(kind: 'PLAYER' | 'ENEMY' | 'BOSS' | 'MINION' | 'STORY', build: CharacterBuild): Record<string, unknown> {
  return {
    kind: `CHARACTER_KIND_${kind}`,
    name: build.name,
    sheet: {
      full: {
        baseScores: {
          strength: build.scores.for,
          dexterity: build.scores.des,
          constitution: build.scores.con,
          intelligence: build.scores.int,
          wisdom: build.scores.sab,
          charisma: build.scores.car,
        },
        raceKey: build.raceKey,
        subraceKey: build.subraceKey,
        classes: [{ classKey: build.classKey, level: build.level, subclassKey: build.subclassKey }],
        ...(build.background
          ? { customBackground: { name: build.background, skillKeys: build.backgroundSkillKeys } }
          : { backgroundKey: build.backgroundKey }),
        skillProficiencyKeys: build.extraSkillKeys,
        hitPoints: { method: 'HIT_POINTS_METHOD_AVERAGE' },
      },
    },
  };
}

/**
 * Starts a game session for a campaign
 * (`meurpg.play.v1.PlayService/StartGameSession`), the RN-01 lock trigger
 * (MR-006): starting a session locks, in the same transaction, every
 * unlocked player character in the campaign (Etapa 4 plan, amendment A5/V1,
 * the `play` stub). Master-only.
 *
 * Defaults to the campaign in the page's current URL
 * (`/campaigns/<id>/...`), so a spec already on the campaign page can just
 * call `startGameSession(page)`, as the plan's helper list (§6) has it; pass
 * `campaignId` explicitly from any other page.
 */
export async function startGameSession(page: Page, campaignId?: string): Promise<APIResponse> {
  const id = campaignId ?? page.url().match(/\/campaigns\/([^/]+)/)?.[1];
  if (!id) {
    throw new Error('startGameSession: no campaignId given, and none found in the current page URL');
  }
  return callRPC(page, 'meurpg.play.v1.PlayService/StartGameSession', { campaignId: id });
}

/**
 * Opens a long level-up pick list ("Ver os outros N ...", it shows 4 rows
 * alphabetically) so a row picked by name is there whatever the names sort like.
 */
export async function showAllPicks(panel: import('@playwright/test').Locator): Promise<void> {
  const more = panel.getByRole('button', { name: /^Ver os outros \d+/ });
  if ((await more.count()) > 0) {
    await more.click();
  }
}
