import { DOCUMENT } from '@angular/common';
import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
  untracked,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { Code, ConnectError } from '@connectrpc/connect';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';

import type { SavingState } from '../../core/connect/saving-state';
import { setPageSubject } from '../../core/title/page-title';
import { formatModifier } from '../../core/characters/character-labels';
import { describeCharacterError } from '../../core/characters/character-errors';
import { openSpellDetails } from '../../shared/spell-details/open-spell-details';
import type { SpellDetailsVm } from '../../shared/spell-details/spell-details.types';
import type { LevelUpDone } from '../../core/levelup/levelup-flow';
import { takeLevelUpDone } from '../../core/levelup/levelup-done';
import { ReviveBlocked } from '../../shared/revive/revive-blocked';
import { ReviveConfirm } from '../../shared/revive/revive-confirm';
import { AfterDeath } from './after-death/after-death';
import { PendingChoicesBanner } from './pending-choices-banner/pending-choices-banner';
import { LevelUpBanner } from './level-up-banner/level-up-banner';
import { LevelUpDoneNotice } from './level-up-banner/level-up-done';
import { AbilityMedallions } from './ability-medallions/ability-medallions';
import { BasicSheet } from './basic-sheet/basic-sheet';
import { OpenSessions } from '../../core/play/open-sessions';
import {
  BasicSheetVm,
  CampaignXpMode,
  CharacterSheetSource,
  CharacterSheetVm,
  FullSheetVm,
  IssueVm,
} from './character-sheet.types';
import { CombatColumn } from './combat-column/combat-column';
import { EquipmentPanel } from './equipment-panel/equipment-panel';
import { SheetEquip } from './character-sheet-equip';
import { openWeaponMarks } from './weapon-marks/weapon-marks-sheet';
import { CreaturesPanel } from './creatures-panel/creatures-panel';
import { FeaturesPanel } from './features-panel/features-panel';
import { MasterNotes } from './master-notes/master-notes';
import { mediaQuery } from '../../shared/map-view/media-query';
import { NotesPanel } from '../../shared/notes/notes-panel';
import { ProficiencyColumn } from './proficiency-column/proficiency-column';
import { SheetHeader } from './sheet-header/sheet-header';
import { ChangedContentNotice } from './changed-content/changed-content';
import { OwnerChanges } from './owner-changes/owner-changes';
import { OwnerResubmit } from './owner-resubmit/owner-resubmit';
import { RejectConfirm } from './reject-confirm/reject-confirm';
import { RequestChanges } from './request-changes/request-changes';
import { formatWhen, issueTitle } from './sheet-format';
import { SheetWrites, playerNameOf } from './character-sheet-writes';
import { StoryPanel } from './story-panel/story-panel';
import { XpWatcher } from '../../core/play/xp-watcher';
import { ResourceCounters } from './resource-counters/resource-counters';
import type { VitalsVm } from '../../core/play/live-session.types';

type PageState =
  | { status: 'loading' }
  | { status: 'not-found' }
  | { status: 'error'; message: string }
  | { status: 'ready'; vm: CharacterSheetVm };

/**
 * "/campaigns/:id/characters/:characterId" (MR-004): the sheet as the paper
 * sheet (docs/design/README.md, direction A). The header (name, state, identity
 * fields and the viewer's actions), then the notices (approval, rules
 * issues), then the sheet: four columns from 1200px (ability medallions;
 * proficiencies; combat, spells and equipment; features and story), the
 * medallions in a row over two columns on a tablet, one column in the paper
 * sheet's order on a phone. Each column is a child component in this
 * folder, which keeps every stylesheet under the 4 kB budget.
 *
 * The browser never computes a rule (ADR-0008): everything under `vm.sheet`
 * is exactly what `GetCharacter` sent, only formatted for display.
 *
 * Player-only: the "Anotações" panel (E8-07, MR-030), the first block of the
 * fourth column (right after the header on a narrower screen). The notes are
 * the player's alone: the master never gets the panel, even on the player's
 * sheet, and it stays editable on a locked sheet (the notes are not the sheet).
 *
 * Master-only: the "Notas do mestre" panel and "Marcar como morto", never
 * fetched or rendered for a player (RN-11; `character-sheet.spec.ts` checks
 * `getMasterNotes` is never called for one). "Marcar como morto" asks for a
 * second click ("Confirmar morte"), since a death can't be undone.
 *
 * The story (personality, appearance, backstory, allies) is independent of
 * "Editar ficha" (RN-01's lock, driven by `canEdit`): the master can always
 * edit it, and can toggle whether the player currently can too, "Permitir
 * editar a história" / "Travar a história", in the header's actions
 * (`canToggleStoryEditing`, `storyEditingAllowed`). "Editar história", in
 * the story panel, only ever reads `canEditStory`, whatever the caller's
 * role (integrator amendment to A3, 29/09/2026).
 */
@Component({
  selector: 'app-character-sheet',
  imports: [
    AbilityMedallions,
    AfterDeath,
    BasicSheet,
    ChangedContentNotice,
    CombatColumn,
    EquipmentPanel,
    CreaturesPanel,
    FeaturesPanel,
    LevelUpBanner,
    LevelUpDoneNotice,
    MasterNotes,
    NotesPanel,
    OwnerChanges,
    OwnerResubmit,
    PendingChoicesBanner,
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    ProficiencyColumn,
    RejectConfirm,
    RequestChanges,
    ReviveBlocked,
    ReviveConfirm,
    ResourceCounters,
    RouterLink,
    SheetHeader,
    StoryPanel,
  ],
  templateUrl: './character-sheet.html',
  styleUrl: './character-sheet.scss',
})
export class CharacterSheetPage {
  private readonly source = inject(CharacterSheetSource);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly destroyRef = inject(DestroyRef);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);
  private readonly openSessions = inject(OpenSessions);
  private readonly xpWatcher = inject(XpWatcher);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  /** The descriptions already asked for, by spell: a second tap opens at once. */
  private readonly spellDetails = new Map<string, Promise<SpellDetailsVm>>();

  /** Under 1200px the player's notes are a row that opens; the four-column sheet shows them. */
  protected readonly narrow = mediaQuery('(max-width: 1199.98px)');
  protected readonly state = signal<PageState>({ status: 'loading' });
  /** The campaign from the route, for "Voltar para a campanha" in every
   * state, including the error one. */
  protected readonly campaignId = signal('');
  /** The master's and the owner's writes on the character (`SheetWrites`). */
  protected readonly writes = new SheetWrites({
    source: this.source,
    navigate: (commands) => this.router.navigate(commands),
    stillOn: (characterId) => !this.destroyed && characterId === this.characterId,
    apply: (characterId, vm) => this.applyWrite(characterId, vm),
    reload: () => void this.reloadQuietly(),
    focus: (selector) => this.focusAfterRender(selector),
  });
  /** Whether the owner of a dead character already has another living one: "E agora?" waits for the answer. */
  protected readonly ownerHasLiving = signal<boolean | null>(null);

  /** Bumped when the stream says the character's creatures changed (the panel reads its list again). */
  protected readonly creaturesTick = signal(0);
  /** Bumped when this character's vitals or the combat changed: a Wild Shape form may have ended. */
  protected readonly formTick = signal(0);

  /** How many selections of the class and race choices are still open (PM-05): the banner "Completar" shows with 1 or more. */
  protected readonly pendingChoices = signal(0);

  /** The character's live numbers while the campaign has an open session: the resource counters and the spell slots. `null` outside a session, where the sheet has only the maximums. */
  protected readonly vitals = signal<VitalsVm | null>(null);
  /** Whether there are slots to count live: the sheet's own circles give way to them. */
  protected readonly hasLiveSlots = computed(() => {
    const v = this.vitals();
    return v !== null && (v.spellSlots.length > 0 || v.pactSlots !== null);
  });

  /** How the campaign levels: decides whether the header has an XP block or only the tag. */
  protected readonly xpMode = signal<CampaignXpMode | null>(null);

  /** "Pensantus subiu para o nível 4. O mestre foi avisado.": what the level-up page leaves in the
   * navigation state (not Web Storage), shown until it is dismissed. */
  protected readonly levelUpDone = signal<LevelUpDone | null>(takeLevelUpDone(this.router));

  protected readonly formatModifier = formatModifier;
  protected readonly issueTitle = issueTitle;

  /** A tap on a spell of the list (or its "?"): its description, a bottom sheet on a phone and a dialog from a tablet up. */
  protected describeSpell(key: string): void {
    const campaignId = this.campaignId();
    const s = this.state();
    const spell =
      s.status === 'ready' && s.vm.sheet.kind === 'full'
        ? s.vm.sheet.spells.find((x) => x.key === key)
        : undefined;
    if (!spell) {
      return;
    }
    openSpellDetails(this.dialog, this.bottomSheet, {
      namePt: spell.namePt,
      load: () => {
        let pending = this.spellDetails.get(key);
        if (!pending) {
          pending = this.source.loadSpellDetails(campaignId, key);
          this.spellDetails.set(key, pending);
          pending.catch(() => this.spellDetails.delete(key));
        }
        return pending;
      },
    });
  }

  constructor() {
    this.destroyRef.onDestroy(this.gear.refreshWhenShown(inject(DOCUMENT)));
    // The tab's title carries the character's name once the sheet is loaded.
    setPageSubject(() => {
      const s = this.state();
      return s.status === 'ready' ? s.vm.name : null;
    });
    this.route.paramMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
      const campaignId = params.get('id');
      const characterId = params.get('characterId');
      if (campaignId && characterId) {
        this.campaignId.set(campaignId);
        this.characterId = characterId;
        this.load(campaignId, characterId);
      }
    });

    // While the campaign has an open session, the master's awards arrive on its
    // stream (`xp_changed`): the character is read again, so the XP block is
    // never stale (E7-10). Without a session, it is read on load only.
    effect(() => {
      const id = this.campaignId();
      // Only a player character has XP, or a level-up tag, to keep fresh.
      const s = this.state();
      const player = s.status === 'ready' && s.vm.characterKind === 'player';
      const live =
        player && id !== '' && this.openSessions.sessions().some((o) => o.campaignId === id);
      untracked(() => {
        if (!live) {
          this.vitals.set(null);
        }
        // A session that opens while the sheet is on screen locks it (MR-006, RN-01): the sheet is read again, so
        // "Rascunho" and "Editar ficha" give way to "Travada" without a reload. The notice poll (30 s) tells us.
        if (player) {
          if (this.wasLive === false && live) {
            void this.reloadQuietly();
          }
          this.wasLive = live;
        }
        this.xpWatcher.follow(
          live ? id : null,
          () => void this.reloadQuietly(),
          () => this.creaturesTick.update((n) => n + 1),
          // The table's content changed (RN-23, "A classe mudou"): the same stream, one more kind of hint, the sheet read again.
          () => void this.reloadQuietly(),
          (who) => {
            if (who === null || who === this.characterId) {
              this.formTick.update((n) => n + 1);
            }
          },
          (v) => this.takeVitals(v),
          // The master asked for changes, the player sent the sheet again, or the character lives again: read it again.
          (who) => {
            if (who === this.characterId) {
              void this.reloadQuietly();
            }
          },
        );
      });
    });
    // "E agora?" is for the owner of a dead character who has no living one: ask once per dead character on screen.
    effect(() => {
      const s = this.state();
      const id =
        s.status === 'ready' &&
        s.vm.state === 'dead' &&
        s.vm.characterKind === 'player' &&
        !s.vm.isMaster
          ? s.vm.id
          : '';
      untracked(() => void this.checkOwnerLiving(id));
    });
    this.destroyRef.onDestroy(() => {
      this.destroyed = true;
      this.xpWatcher.follow(null, () => undefined);
    });
  }

  private characterId = '';
  /** Whether the campaign had an open session the last time the sheet was a player's; `null` before the first one. */
  private wasLive: boolean | null = null;
  private destroyed = false;
  private livingCheckedFor = '';
  /** Numbers every read and every answer that sets the sheet: an answer older than the latest one, or for a character the page left, is dropped. */
  private sheetSeq = 0;

  /** The answer of a write is the newest word on its character: reads still in flight no longer count. */
  private applyWrite(characterId: string, vm: CharacterSheetVm): boolean {
    if (this.destroyed || characterId !== this.characterId) {
      return false;
    }
    this.sheetSeq++;
    this.state.set({ status: 'ready', vm });
    return true;
  }

  /** The open choices of a living player character with a full sheet; asked of the server, never counted here. */
  private refreshPendingChoices(campaignId: string, vm: CharacterSheetVm, seq: number): void {
    if (vm.characterKind !== 'player' || vm.sheet.kind !== 'full' || vm.state === 'dead') {
      this.pendingChoices.set(0);
      return;
    }
    this.source.getPendingChoiceCount(campaignId, vm.id).then(
      (n) => {
        if (seq === this.sheetSeq && !this.destroyed) {
          this.pendingChoices.set(n);
        }
      },
      () => undefined,
    );
  }

  /** The live numbers of this character (the session's snapshot or a `vitals_changed`): the newer copy wins; another character's are not kept. */
  private takeVitals(v: VitalsVm): void {
    if (v.characterId !== this.characterId) {
      return;
    }
    this.vitals.update((current) => (current && current.revision > v.revision ? current : v));
  }

  /** "Equipar" / "Guardar" on the sheet (`SheetEquip`). */
  protected readonly gear = new SheetEquip({
    source: this.source,
    campaignId: () => this.campaignId(),
    characterId: () => (this.destroyed ? '' : this.characterId),
    reload: () => this.reloadQuietly(),
    sheet: () => {
      const st = this.state();
      return st.status === 'ready' && st.vm.sheet.kind === 'full' ? st.vm.sheet : null;
    },
    vm: () => {
      const st = this.state();
      return st.status === 'ready' ? st.vm : null;
    },
    apply: (characterId, vm) => this.applyWrite(characterId, vm),
    openMarksSheet: (data) => openWeaponMarks(this.dialog, this.bottomSheet, data),
  });

  /** Reads the character again without the loading state, so the page does not blink; `true` when the new sheet is on screen. */
  private async reloadQuietly(): Promise<boolean> {
    const campaignId = this.campaignId();
    if (!campaignId || !this.characterId) {
      return false;
    }
    const seq = ++this.sheetSeq;
    try {
      const vm = await this.source.getCharacterSheet(campaignId, this.characterId);
      if (seq !== this.sheetSeq) {
        return true; // a newer read owns the screen (a stream's, a tab's): the sheet shown is at least as new
      }
      if (this.state().status === 'ready') {
        this.state.set({ status: 'ready', vm });
        this.refreshPendingChoices(campaignId, vm, seq);
        return true;
      }
      return false;
    } catch {
      // Keep what is on screen: the next change reads again.
      return false;
    }
  }

  private load(campaignId: string, characterId: string): void {
    const seq = ++this.sheetSeq;
    this.wasLive = null;
    this.state.set({ status: 'loading' });
    this.vitals.set(null);
    this.writes.reset();
    this.ownerHasLiving.set(null);
    this.livingCheckedFor = '';
    this.pendingChoices.set(0);
    this.source.getCharacterSheet(campaignId, characterId).then(
      (vm) => {
        if (seq === this.sheetSeq) {
          this.state.set({ status: 'ready', vm });
          this.refreshPendingChoices(campaignId, vm, seq);
        }
      },
      (err: unknown) => {
        if (seq !== this.sheetSeq) {
          return;
        }
        if (ConnectError.from(err, Code.Unavailable).code === Code.NotFound) {
          // The same page for "does not exist" and "not yours to see" (RN-20, ADR-0011).
          this.state.set({ status: 'not-found' });
          return;
        }
        this.state.set({ status: 'error', message: describeCharacterError(err) });
      },
    );
    // Only a detail of the header: without it the sheet shows as for an XP campaign.
    this.source.getXpMode(campaignId).then(
      (mode) => this.xpMode.set(mode),
      () => this.xpMode.set('enemies'),
    );
  }

  /** A child (the story panel) saved and got the updated character back. */
  protected replaceVm(vm: CharacterSheetVm): void {
    this.applyWrite(vm.id, vm);
  }

  private async checkOwnerLiving(characterId: string): Promise<void> {
    if (characterId === '' || characterId === this.livingCheckedFor) {
      return;
    }
    this.livingCheckedFor = characterId;
    const campaignId = this.campaignId();
    try {
      const living = await this.source.hasLivingCharacter(campaignId);
      if (!this.destroyed && this.livingCheckedFor === characterId) {
        this.ownerHasLiving.set(living);
      }
    } catch {
      // Without the answer the panel stays out: "Criar meu personagem" is on the campaign's page.
    }
  }

  protected playerName(vm: CharacterSheetVm): string {
    return playerNameOf(vm);
  }

  protected playerNameCapital(vm: CharacterSheetVm): string {
    const name = this.playerName(vm);
    return name.charAt(0).toUpperCase() + name.slice(1);
  }

  protected whenOf(date: Date | null | undefined): string {
    return date ? formatWhen(date) : '';
  }

  protected errorOf(state: SavingState): string | null {
    return state.status === 'error' ? state.message : null;
  }

  /** A confirmation replaces the button that asked for it, so the focus
   * moves to its replacement instead of falling back to the page. */
  private focusAfterRender(selector: string): void {
    afterNextRender(() => this.host.nativeElement.querySelector<HTMLElement>(selector)?.focus(), {
      injector: this.injector,
    });
  }

  /** Explicit casts for the template, which narrows `vm.sheet.kind` in an
   * `@if` but cannot carry that narrowing into a `@let` binding. */
  /** The skills the sheet is trained in, by name (the line of "O que mudou"). */
  protected trainedSkills(full: FullSheetVm): string[] {
    return full.skills
      .filter((s) => s.proficiency === 'proficient' || s.proficiency === 'expertise')
      .map((s) => s.namePt);
  }

  /** The sheet's own issues: the ones "A classe mudou" tells (code `table_content_changed`) are not repeated in the list. */
  protected ownIssues(full: FullSheetVm): readonly IssueVm[] {
    return full.changedContent.length > 0
      ? full.issues.filter((i) => i.code !== 'table_content_changed')
      : full.issues;
  }

  protected asFullSheet(sheet: FullSheetVm | BasicSheetVm): FullSheetVm {
    return sheet as FullSheetVm;
  }

  protected asBasicSheet(sheet: FullSheetVm | BasicSheetVm): BasicSheetVm {
    return sheet as BasicSheetVm;
  }
}
