import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  afterNextRender,
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
import { ActivatedRoute, Router, RouterLink } from '@angular/router';

import { setPageSubject } from '../../core/title/page-title';
import { formatModifier } from '../../core/characters/character-labels';
import { describeCharacterError } from '../../core/characters/character-errors';
import type { LevelUpDone } from '../../core/levelup/levelup-flow';
import { takeLevelUpDone } from '../../core/levelup/levelup-done';
import { LevelUpBanner } from './level-up-banner/level-up-banner';
import { LevelUpDoneNotice } from './level-up-banner/level-up-done';
import { AbilityMedallions } from './ability-medallions/ability-medallions';
import { BasicSheet } from './basic-sheet/basic-sheet';
import { OpenSessions } from '../../shell/live-notice/open-sessions';
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
import { NotesPanel } from '../../shared/notes/notes-panel';
import { ProficiencyColumn } from './proficiency-column/proficiency-column';
import { SheetHeader } from './sheet-header/sheet-header';
import { ChangedContentNotice } from './changed-content/changed-content';
import { issueTitle } from './sheet-format';
import { StoryPanel } from './story-panel/story-panel';
import { XpWatcher } from './xp-watcher';

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
    BasicSheet,
    ChangedContentNotice,
    CombatColumn,
    CreaturesPanel,
    FeaturesPanel,
    LevelUpBanner,
    LevelUpDoneNotice,
    MasterNotes,
    NotesPanel,
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    ProficiencyColumn,
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

  protected readonly state = signal<PageState>({ status: 'loading' });
  /** The campaign from the route, for "Voltar para a campanha" in every
   * state, including the error one. */
  protected readonly campaignId = signal('');
  protected readonly markDeadState = signal<SavingState>({ status: 'idle' });
  /** A death can't be undone, so it takes a second click ("Confirmar
   * morte") after "Marcar como morto". */
  protected readonly confirmingDeath = signal(false);
  protected readonly storyToggleState = signal<SavingState>({ status: 'idle' });
  /** "Aprovar personagem" / "Recusar personagem" (MR-024). */
  protected readonly approvalState = signal<SavingState>({ status: 'idle' });
  /** Rejecting deletes the character for good, so it takes a second click
   * ("Confirmar recusa") after "Recusar personagem". */
  protected readonly confirmingReject = signal(false);

  /** Bumped when the stream says the character's creatures changed (the panel reads its list again). */
  protected readonly creaturesTick = signal(0);
  /** Bumped when this character's vitals or the combat changed: a Wild Shape form may have ended. */
  protected readonly formTick = signal(0);

  /** How the campaign levels: decides whether the header has an XP block or only the tag. */
  protected readonly xpMode = signal<CampaignXpMode | null>(null);

  /** "Pensantus subiu para o nível 4. O mestre foi avisado.": what the level-up page leaves in the
   * navigation state (not Web Storage), shown until it is dismissed. */
  protected readonly levelUpDone = signal<LevelUpDone | null>(takeLevelUpDone(this.router));

  protected readonly formatModifier = formatModifier;
  protected readonly issueTitle = issueTitle;

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
      untracked(() =>
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
        ),
      );
    });
    this.destroyRef.onDestroy(() => {
      this.destroyed = true;
      this.xpWatcher.follow(null, () => undefined);
    });
  }

  private characterId = '';
  private destroyed = false;
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
      }
    } catch {
      // Keep what is on screen: the next change reads again.
    }
  }

  private load(campaignId: string, characterId: string): void {
    const seq = ++this.sheetSeq;
    this.state.set({ status: 'loading' });
    this.confirmingDeath.set(false);
    this.confirmingReject.set(false);
    this.markDeadState.set({ status: 'idle' });
    this.storyToggleState.set({ status: 'idle' });
    this.approvalState.set({ status: 'idle' });
    this.source.getCharacterSheet(campaignId, characterId).then(
      (vm) => {
        if (seq === this.sheetSeq) {
          this.state.set({ status: 'ready', vm });
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

  protected askToConfirmReject(): void {
    this.confirmingReject.set(true);
    this.focusAfterRender('.js-confirm-reject');
  }

  protected cancelReject(): void {
    this.confirmingReject.set(false);
    this.focusAfterRender('.js-reject');
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
