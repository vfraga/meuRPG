import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  OnInit,
  afterNextRender,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatMenuModule } from '@angular/material/menu';
import { RouterLink } from '@angular/router';

import { describeCharacterError } from '../../../core/characters/character-errors';
import { characterKindLabel, characterStateLabel } from '../../../core/characters/character-labels';
import { CharacterKind } from '../../../core/characters/characters.types';
import { LevelUpFeed } from '../../../core/levelup/levelup-feed';
import { ExperienceStore } from '../../../core/progression/experience-store';
import { OpenSessions } from '../../../core/play/open-sessions';
import { XpWatcher } from '../../../core/play/xp-watcher';
import { LevelUpTag } from '../../../shared/xp/level-up-tag';
import { characterRowSub, stateTagClass } from './campaign-characters.copy';
import { LevelUpChanges } from './level-up-changes';
import { CharacterCreatures } from '../creatures/character-creatures';
import { ReservedCharacters } from './reserved/reserved-characters';
import {
  CampaignCharacterListItemVm,
  CampaignCharactersSource,
  CampaignCharactersVm,
  OpenChoicesVm,
} from './campaign-characters.types';

type ListState =
  | { status: 'loading' }
  | { status: 'ready'; vm: CampaignCharactersVm }
  | { status: 'error'; message: string };

/** The four NPC kinds a master can create, and the `:kind` route segment
 * each maps to (plan §5: `/campaigns/:id/npcs/new/:kind`). */
const NPC_KINDS: ReadonlyArray<{ kind: CharacterKind; segment: string }> = [
  { kind: 'enemy', segment: 'enemy' },
  { kind: 'boss', segment: 'boss' },
  { kind: 'minion', segment: 'minion' },
  { kind: 'story', segment: 'story' },
];

/**
 * The "Personagens" section on `/campaigns/:id` (MR-003, MR-005, MR-024).
 *
 * Each list is a panel of its own, titled with an h3 under this section's
 * (visually hidden) "Personagens" h2, and every row links to the sheet.
 *
 * - **Master:** "Esperando aprovação" (characters created through an invite
 *   that requires approval, RN-15 — a warning dot and "Revisar"; the sheet
 *   is where the master approves or rejects it), "Personagens dos
 *   jogadores" (name, class and player, state tag) and "NPCs" (name, kind
 *   tag), with the "Novo NPC" menu at the end of its panel.
 * - **Player:** their own characters (a pending one shows "Pendente de
 *   aprovação"), and a "Criar meu personagem" call to action shown only
 *   when they have no living character (RN-03; a pending one counts).
 */
@Component({
  selector: 'app-campaign-characters',
  imports: [
    CharacterCreatures,
    LevelUpChanges,
    LevelUpTag,
    MatButtonModule,
    ReservedCharacters,
    MatIconModule,
    MatMenuModule,
    RouterLink,
  ],
  templateUrl: './campaign-characters.html',
  styleUrl: './campaign-characters.scss',
})
export class CampaignCharacters implements OnInit {
  private readonly source = inject(CampaignCharactersSource);
  // The page's XP store (RN-12, D4): which characters "can level up". Optional:
  // the list also works on its own, without the tag.
  private readonly experience = inject(ExperienceStore, { optional: true });
  // The master's level-ups (MR-040): "Subiu para o nível N" and "O que mudou". Optional like the store.
  private readonly levelUps = inject(LevelUpFeed, { optional: true });
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);
  private readonly openSessions = inject(OpenSessions, { optional: true });
  private readonly xpWatcher = inject(XpWatcher, { optional: true });

  readonly campaignId = input.required<string>();
  readonly isMaster = input.required<boolean>();

  protected readonly state = signal<ListState>({ status: 'loading' });
  protected readonly characterKindLabel = characterKindLabel;
  protected readonly characterStateLabel = characterStateLabel;
  protected readonly npcKinds = NPC_KINDS;
  protected readonly characterRowSub = characterRowSub;
  protected readonly stateTagClass = stateTagClass;

  /** Bumped when the stream says a character's creatures changed (the master's lines read again). */
  protected readonly creaturesTick = signal(0);
  /** "Mastim dado ao Toren. Ele aparece na ficha dele, que foi avisado.": the live region's words after a gift. */
  protected readonly giftNotice = signal('');

  /** The master's "N escolhas em aberto", by character (PM-05). Empty for a player, who never asks. */
  protected readonly openChoices = signal<ReadonlyMap<string, OpenChoicesVm>>(new Map());

  /** Whose "O que mudou" is open, in place under its row. */
  protected readonly openChanges = signal<string | null>(null);

  constructor() {
    // While the campaign has an open session, a player's level-up arrives on its stream (`xp_changed`):
    // the list and the XP are read again, so "Subiu para o nível N" shows without a reload.
    effect(() => {
      const id = this.campaignId();
      const live =
        this.isMaster() &&
        (this.openSessions?.sessions().some((o) => o.campaignId === id) ?? false);
      untracked(() =>
        this.xpWatcher?.follow(
          live ? id : null,
          () => {
            void this.levelUps?.refresh();
            void this.experience?.refresh();
          },
          () => this.creaturesTick.update((n) => n + 1),
        ),
      );
    });
    inject(DestroyRef).onDestroy(() => this.xpWatcher?.follow(null, () => undefined));

    // "Ver o que mudou" in the master's status line: the row opens and comes into view.
    effect(() => {
      const r = this.levelUps?.reveal();
      if (!r) {
        return;
      }
      untracked(() => {
        this.openChanges.set(r.characterId);
        afterNextRender(
          () => {
            const toggle = this.host.nativeElement.querySelector<HTMLElement>(
              `#changes-toggle-${r.characterId}`,
            );
            toggle?.scrollIntoView({ block: 'center' });
            toggle?.focus({ preventScroll: true });
          },
          { injector: this.injector },
        );
      });
    });
  }

  /** The master gave a creature (E9-10, quadro 6). */
  protected gave(character: string, creature: string): void {
    this.giftNotice.set(
      `${creature} dado a ${character}. A criatura aparece na ficha do personagem, e o jogador foi avisado.`,
    );
  }

  /** The newest level-up of a character, for the master's list. */
  protected levelUpOf(characterId: string) {
    return this.isMaster() ? this.levelUps?.latestOf(characterId) : undefined;
  }

  protected isFresh(levelUp: NonNullable<ReturnType<CampaignCharacters['levelUpOf']>>): boolean {
    return this.levelUps?.isFresh(levelUp) ?? false;
  }

  protected toggleChanges(characterId: string): void {
    this.openChanges.update((open) => (open === characterId ? null : characterId));
  }

  /** Whether the character can go up a level now (the tag beside its state). */
  protected canLevelUp(characterId: string): boolean {
    return this.experience?.rows().some((r) => r.id === characterId && r.canLevelUp) ?? false;
  }

  /** Master only: the players' characters that wait for approval. */
  protected awaitingApproval(vm: CampaignCharactersVm): readonly CampaignCharacterListItemVm[] {
    return vm.playerCharacters.filter((c) => c.state === 'pending');
  }

  /** The master's "Personagens dos jogadores": everything already in the
   * campaign. A player sees all of their own, pending included. */
  protected listedPlayerCharacters(
    vm: CampaignCharactersVm,
  ): readonly CampaignCharacterListItemVm[] {
    return this.isMaster()
      ? vm.playerCharacters.filter((c) => c.state !== 'pending' && c.reserved !== true)
      : vm.playerCharacters;
  }

  /** Master only: the reserved characters (nobody's yet) and the ones a player took through a link, which can go back. */
  protected reservedRows(vm: CampaignCharactersVm): readonly CampaignCharacterListItemVm[] {
    return vm.playerCharacters.filter((c) => c.reserved === true || c.claim?.state === 'used');
  }

  /** The list changed under a write of the reserved characters: read it again, keeping what is on screen meanwhile. */
  protected refresh(): void {
    this.source.listCharacters(this.campaignId()).then(
      (vm) => this.state.set({ status: 'ready', vm }),
      (err: unknown) => this.state.set({ status: 'error', message: describeCharacterError(err) }),
    );
  }

  ngOnInit(): void {
    // Not the constructor — see CampaignInvites's doc comment on the same
    // pattern: a required signal input is only set before the first
    // change-detection pass, and ngOnInit is guaranteed to run after that.
    this.load();
    if (this.isMaster()) {
      void this.levelUps?.load(this.campaignId());
      this.loadOpenChoices();
    }
  }

  /** "1 escolha em aberto" / "3 escolhas em aberto"; empty when the character has none. */
  protected openChoicesText(characterId: string): string {
    const n = this.openChoices().get(characterId)?.count ?? 0;
    return n === 0 ? '' : n === 1 ? '1 escolha em aberto' : `${n} escolhas em aberto`;
  }

  /** What is open, for a screen reader: "Em aberto: Estilo de Luta (Guerreiro, nível 1)". */
  protected openChoicesLabels(characterId: string): string {
    const labels = this.openChoices().get(characterId)?.labels ?? [];
    return labels.length === 0 ? '' : `Em aberto: ${labels.join('; ')}.`;
  }

  /** Master only: a failure here only leaves the tags out, the list is still right. */
  private loadOpenChoices(): void {
    this.source.openChoices(this.campaignId()).then(
      (list) => this.openChoices.set(new Map(list.map((o) => [o.characterId, o]))),
      () => this.openChoices.set(new Map()),
    );
  }

  private load(): void {
    this.state.set({ status: 'loading' });
    this.source.listCharacters(this.campaignId()).then(
      (vm) => this.state.set({ status: 'ready', vm }),
      (err: unknown) => this.state.set({ status: 'error', message: describeCharacterError(err) }),
    );
  }
}
