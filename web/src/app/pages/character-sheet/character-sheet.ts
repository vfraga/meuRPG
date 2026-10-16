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

import { setPageSubject } from '../../core/title/page-title';
import { formatModifier } from '../../core/characters/character-labels';
import {
  describeCharacterError,
  livingRefusal,
  type LivingRefusal,
} from '../../core/characters/character-errors';
import { openSpellDetails } from '../../shared/spell-details/open-spell-details';
import type { SpellDetailsVm } from '../../shared/spell-details/spell-details.types';
import { ActionKey } from '../../core/connect/idempotency';
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
import { CreaturesPanel } from './creatures-panel/creatures-panel';
import { FeaturesPanel } from './features-panel/features-panel';
import { MasterNotes } from './master-notes/master-notes';
import { mediaQuery } from '../../shared/map-view/media-query';
import { NotesPanel } from '../../shared/notes/notes-panel';
import { ProficiencyColumn } from './proficiency-column/proficiency-column';
import { SheetHeader } from './sheet-header/sheet-header';
import { ChangedContentNotice } from './changed-content/changed-content';
import { OwnerChanges } from './owner-changes/owner-changes';
import { RejectConfirm } from './reject-confirm/reject-confirm';
import { RequestChanges } from './request-changes/request-changes';
import { formatWhen, issueTitle } from './sheet-format';
import { StoryPanel } from './story-panel/story-panel';
import { XpWatcher } from '../../core/play/xp-watcher';
import { ResourceCounters } from './resource-counters/resource-counters';
import type { VitalsVm } from '../../core/play/live-session.types';

type PageState =
  | { status: 'loading' }
  | { status: 'not-found' }
  | { status: 'error'; message: string }
  | { status: 'ready'; vm: CharacterSheetVm };

type SavingState = { status: 'idle' } | { status: 'saving' } | { status: 'error'; message: string };

/**
 * "/campaigns/:id/characters/:characterId" (MR-004): the sheet as the paper
 * sheet (docs/design.md, direction A). The header (name, state, identity
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
    CreaturesPanel,
    FeaturesPanel,
    LevelUpBanner,
    LevelUpDoneNotice,
    MasterNotes,
    NotesPanel,
    OwnerChanges,
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
  protected readonly markDeadState = signal<SavingState>({ status: 'idle' });
  /** A death can't be undone, so it takes a second click ("Confirmar
   * morte") after "Marcar como morto". */
  protected readonly confirmingDeath = signal(false);
  protected readonly storyToggleState = signal<SavingState>({ status: 'idle' });
  /** "Reviver" (master, a dead player character): the question is open in place of the button. */
  protected readonly confirmingRevive = signal(false);
  protected readonly reviveState = signal<SavingState>({ status: 'idle' });
  /** The player's other living character that refused the revival: the card says so, with the way to it. */
  protected readonly reviveRefused = signal<LivingRefusal | null>(null);
  /** The words a screen reader gets once the character lives again. */
  protected readonly reviveAnnouncement = signal('');
  private readonly reviveKey = new ActionKey();
  /** Whether the owner of a dead character already has another living one: "E agora?" waits for the answer. */
  protected readonly ownerHasLiving = signal<boolean | null>(null);
  /** "Aprovar personagem" / "Recusar personagem" (MR-024). */
  protected readonly approvalState = signal<SavingState>({ status: 'idle' });
  /** Rejecting deletes the character for good, so it takes a second click
   * ("Confirmar recusa") after "Recusar personagem". */
  protected readonly confirmingReject = signal(false);
  /** "Pedir ajustes" (master): the form is open in place of the approval notice. */
  protected readonly requestingChanges = signal(false);
  protected readonly requestChangesState = signal<SavingState>({ status: 'idle' });
  /** "Enviar de novo" (the owning player). */
  protected readonly resubmitState = signal<SavingState>({ status: 'idle' });
  /** The player just sent the sheet again: the waiting notice says so, until the page is left. */
  protected readonly resubmitted = signal(false);
  /** The words a screen reader gets once the master's request went out. */
  protected readonly announcement = signal('');
  private readonly requestKey = new ActionKey();
  private readonly resubmitKey = new ActionKey();

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

  /** Reads the character again without the loading state, so the page does not blink. */
  private async reloadQuietly(): Promise<void> {
    const campaignId = this.campaignId();
    if (!campaignId || !this.characterId) {
      return;
    }
    const seq = ++this.sheetSeq;
    try {
      const vm = await this.source.getCharacterSheet(campaignId, this.characterId);
      if (seq === this.sheetSeq && this.state().status === 'ready') {
        this.state.set({ status: 'ready', vm });
        this.refreshPendingChoices(campaignId, vm, seq);
      }
    } catch {
      // Keep what is on screen: the next change reads again.
    }
  }

  private load(campaignId: string, characterId: string): void {
    const seq = ++this.sheetSeq;
    this.wasLive = null;
    this.state.set({ status: 'loading' });
    this.vitals.set(null);
    this.confirmingDeath.set(false);
    this.confirmingReject.set(false);
    this.requestingChanges.set(false);
    this.requestChangesState.set({ status: 'idle' });
    this.resubmitState.set({ status: 'idle' });
    this.resubmitted.set(false);
    this.announcement.set('');
    this.markDeadState.set({ status: 'idle' });
    this.confirmingRevive.set(false);
    this.reviveState.set({ status: 'idle' });
    this.reviveRefused.set(null);
    this.reviveAnnouncement.set('');
    this.ownerHasLiving.set(null);
    this.livingCheckedFor = '';
    this.storyToggleState.set({ status: 'idle' });
    this.approvalState.set({ status: 'idle' });
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

  protected askToConfirmDeath(): void {
    this.confirmingDeath.set(true);
    this.focusAfterRender('.js-confirm-death');
  }

  protected cancelDeath(): void {
    this.confirmingDeath.set(false);
    this.focusAfterRender('.js-mark-dead');
  }

  protected askToRevive(): void {
    this.reviveState.set({ status: 'idle' });
    this.reviveRefused.set(null);
    this.confirmingRevive.set(true);
  }

  protected cancelRevive(): void {
    this.confirmingRevive.set(false);
    this.reviveRefused.set(null);
    this.focusAfterRender('.js-revive');
  }

  /** Master only: the character lives again; the page shows it as it was before the death. */
  protected async revive(campaignId: string, characterId: string, name: string): Promise<void> {
    this.reviveState.set({ status: 'saving' });
    try {
      const key = this.reviveKey.keyFor({ campaignId, characterId });
      const vm = await this.source.reviveCharacter(campaignId, characterId, key);
      if (!this.applyWrite(characterId, vm)) {
        return;
      }
      this.reviveKey.renew();
      this.confirmingRevive.set(false);
      this.reviveState.set({ status: 'idle' });
      this.reviveAnnouncement.set(`${name} voltou à vida`);
    } catch (err) {
      this.confirmingRevive.set(false);
      const living = livingRefusal(err);
      if (living) {
        this.reviveRefused.set(living);
        this.focusAfterRender('.js-open-living');
        return;
      }
      this.reviveState.set({ status: 'error', message: describeCharacterError(err) });
      // Someone else may have changed the character: read it again.
      void this.reloadQuietly();
    }
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

  protected askToConfirmReject(): void {
    this.confirmingReject.set(true);
    this.focusAfterRender('.js-cancel-reject');
  }

  protected cancelReject(): void {
    this.confirmingReject.set(false);
    this.focusAfterRender('.js-reject');
  }

  protected askForChanges(): void {
    this.confirmingReject.set(false);
    this.announcement.set('');
    this.requestChangesState.set({ status: 'idle' });
    this.requestingChanges.set(true);
  }

  protected cancelRequestChanges(): void {
    this.requestingChanges.set(false);
    this.focusAfterRender('.js-ask-changes');
  }

  /** Master only (RN-15): the character goes back to its player with the reason and stays pending. */
  protected async requestChanges(
    campaignId: string,
    characterId: string,
    reason: string,
  ): Promise<void> {
    this.requestChangesState.set({ status: 'saving' });
    try {
      const key = this.requestKey.keyFor({ campaignId, characterId, reason });
      const vm = await this.source.requestCharacterChanges(campaignId, characterId, reason, key);
      if (!this.applyWrite(characterId, vm)) {
        return;
      }
      this.requestKey.renew();
      this.requestChangesState.set({ status: 'idle' });
      this.requestingChanges.set(false);
      this.announcement.set(`Pedido de ajustes enviado a ${this.playerName(vm)}`);
      this.focusAfterRender('.js-ask-changes');
    } catch (err) {
      this.requestChangesState.set({ status: 'error', message: describeCharacterError(err) });
    }
  }

  /** Owning player only (RN-15): the sheet goes to the master again. */
  protected async resubmit(campaignId: string, characterId: string): Promise<void> {
    this.resubmitState.set({ status: 'saving' });
    try {
      const key = this.resubmitKey.keyFor({ campaignId, characterId });
      const vm = await this.source.resubmitCharacter(campaignId, characterId, key);
      if (!this.applyWrite(characterId, vm)) {
        return;
      }
      this.resubmitKey.renew();
      this.resubmitState.set({ status: 'idle' });
      this.resubmitted.set(true);
    } catch (err) {
      this.resubmitState.set({ status: 'error', message: describeCharacterError(err) });
    }
  }

  protected playerName(vm: CharacterSheetVm): string {
    return vm.playerDisplayName ?? 'o jogador';
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

  protected async markDead(campaignId: string, characterId: string): Promise<void> {
    this.markDeadState.set({ status: 'saving' });
    try {
      const vm = await this.source.markCharacterDead(campaignId, characterId);
      if (!this.applyWrite(characterId, vm)) {
        return;
      }
      this.confirmingDeath.set(false);
      this.markDeadState.set({ status: 'idle' });
    } catch (err) {
      this.confirmingDeath.set(false);
      this.markDeadState.set({ status: 'error', message: describeCharacterError(err) });
    }
  }

  /** Master only: flips whether the player may currently edit the story
   * (integrator amendment to A3, 29/09/2026; `SetStoryEditing` takes no
   * revision and never changes one; see `CharacterSheetSource`'s doc
   * comment). Never changes `canEditStory` directly: the response is what
   * updates it, same as every other server-computed flag. */
  protected async toggleStoryEditingAllowed(
    campaignId: string,
    characterId: string,
    currentlyAllowed: boolean,
  ): Promise<void> {
    this.storyToggleState.set({ status: 'saving' });
    try {
      const vm = await this.source.setStoryEditingAllowed(
        campaignId,
        characterId,
        !currentlyAllowed,
      );
      if (!this.applyWrite(characterId, vm)) {
        return;
      }
      this.storyToggleState.set({ status: 'idle' });
    } catch (err) {
      this.storyToggleState.set({ status: 'error', message: describeCharacterError(err) });
    }
  }

  /** Master only (MR-024): the character joins the campaign as a draft, and
   * its player as a member. The response is the approved character, so the
   * page simply shows it. */
  protected async approve(campaignId: string, characterId: string): Promise<void> {
    this.approvalState.set({ status: 'saving' });
    try {
      const vm = await this.source.approveCharacter(campaignId, characterId);
      if (!this.applyWrite(characterId, vm)) {
        return;
      }
      this.approvalState.set({ status: 'idle' });
    } catch (err) {
      this.approvalState.set({ status: 'error', message: describeCharacterError(err) });
    }
  }

  /** Master only (MR-024), after "Confirmar recusa": the character is gone,
   * so the master goes back to the campaign's page. */
  protected async reject(campaignId: string, characterId: string): Promise<void> {
    this.approvalState.set({ status: 'saving' });
    try {
      await this.source.rejectCharacter(campaignId, characterId);
      if (this.destroyed || characterId !== this.characterId) {
        return;
      }
      this.approvalState.set({ status: 'idle' });
      await this.router.navigate(['/campaigns', campaignId]);
    } catch (err) {
      this.confirmingReject.set(false);
      this.approvalState.set({ status: 'error', message: describeCharacterError(err) });
    }
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
