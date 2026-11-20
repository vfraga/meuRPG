import { type Signal, computed, effect, untracked } from '@angular/core';

import { pickRefusalOf } from '../../core/characters/character-errors';
import { isFullSheetKind } from '../../core/characters/characters.types';
import { missingSentence, pendingLabels } from '../../shared/choice-groups/choice-picks';
import type {
  AbilityTableVm,
  CharacterEditorSource,
  CharacterFormValue,
} from './character-editor.types';
import type { EditorCatalog } from './editor-catalog';
import { EDITOR_STEP_LABELS } from './editor-labels';
import type { EditorPicks } from './editor-picks';
import { type PageState, previewInputOf } from './editor-state';
import { type PickSentence, pickSentenceOf } from './pick-refusals';
import { choicesAfterAnswer, serverDraftNeeded } from './full-draft';
import { ServerHitPoints } from './hit-points-server';
import { ServerChoices } from './server-choices';

/** The server cannot derive a draft while the page has no lists to read it by. */
function pageNotReady(): Promise<never> {
  return Promise.reject(new Error('the page is not ready'));
}

/** What `EditorServerDraft` reads from the page. */
export interface ServerDraftHost {
  readonly source: Pick<CharacterEditorSource, 'previewCharacter' | 'previewChoices'>;
  readonly state: Signal<PageState>;
  readonly views: EditorCatalog;
  readonly picks: EditorPicks;
  readonly abilityTable: Signal<AbilityTableVm | null>;
  /** The full sheet the draft stands for now. */
  readonly build: () => CharacterFormValue;
}

/**
 * What the server says about the draft on screen: the hit points and the spell numbers (`PreviewCharacter`) while the
 * "Pontos de vida" box or the "Magias" step is on screen, and the choices (`PreviewChoices`) the "Escolhas" step shows
 * with what is still open. A plain class the editor builds in its injection context: it keeps the effects that ask
 * again when the draft changes and the pick the draft stores that no choice offers any more.
 */
export class EditorServerDraft {
  /** The hit points the server derives for the draft while the "Pontos de vida" box is on screen. */
  readonly hitPoints = new ServerHitPoints(() => {
    const s = this.host.state();
    return s.status === 'ready'
      ? this.host.source.previewCharacter(previewInputOf(s, this.host.build(), true))
      : pageNotReady();
  });
  /** The choices the server reads in the draft while the "Escolhas" step may be on screen. */
  readonly choices = new ServerChoices(() => {
    const s = this.host.state();
    return s.status === 'ready'
      ? this.host.source.previewChoices(previewInputOf(s, this.host.build(), false))
      : pageNotReady();
  });
  /**
   * The pick of the draft the server cannot place, with its sentence (`PreviewCharacter` refuses the draft with the
   * first such pick); `null` when the draft fits, or the server refuses it for another reason. Asked only while the
   * draft holds a skill or an expertise that the lists `PreviewChoices` answers leave out (the class changed under the
   * pick): that answer no longer refuses such a draft, so the sentence comes from here.
   */
  readonly pickFit = new ServerChoices<PickSentence | null>(async () => {
    const s = this.host.state();
    if (s.status !== 'ready') {
      return pageNotReady();
    }
    const input = previewInputOf(s, this.host.build(), true);
    try {
      await this.host.source.previewCharacter(input);
    } catch (err) {
      const refusal = pickRefusalOf(err);
      return refusal ? pickSentenceOf(refusal, input.full) : null;
    }
    return null;
  });
  /** What the server said about the draft's feats (a prerequisite not met), by feat key. */
  readonly featIssues = computed(() => this.hitPoints.answer()?.featIssues ?? {});
  /** What the server says the effects add to the hit points of the draft; `null` while it has no answer (the box adds up by itself). */
  readonly hitPointsFromEffects = computed(
    () => this.hitPoints.answer()?.hitPointsFromEffects ?? null,
  );
  readonly choicesAnswer = this.choices.answer.asReadonly();
  readonly choiceGroups = computed(() => this.choicesAnswer()?.groups ?? []);
  /** The step exists only when the race or a class asks a choice (a human rogue 1 has none). */
  readonly choicesStep = computed(() => this.choiceGroups().length > 0);
  private readonly pending = computed(() => pendingLabels(this.choiceGroups()));
  /** The open choices that stop a player's sheet: the languages and tools only warn. */
  private readonly blocking = computed(() => pendingLabels(this.choiceGroups(), false));
  /** The tab carries the "!" while a choice is open. */
  readonly pendingSteps = computed(() =>
    this.choicesStep() && this.pending().length > 0 ? [EDITOR_STEP_LABELS.escolhas] : [],
  );
  /** The steps built from the start: the ones whose content the save depends on. */
  readonly eagerSteps = computed(() => [
    ...(this.host.abilityTable() ? [EDITOR_STEP_LABELS.atributos] : []),
    ...(this.choicesStep() ? [EDITOR_STEP_LABELS.escolhas] : []),
  ]);
  /** A player's sheet cannot be saved with a choice open (the server refuses it); the master's NPCs and edits can. */
  readonly choicesBlocked = computed(() => {
    const s = this.host.state();
    return (
      s.status === 'ready' &&
      s.kind === 'player' &&
      !s.catalog.viewerIsMaster &&
      this.blocking().length > 0
    );
  });
  /** "Falta uma escolha: Estilo de Luta.", the description of the blocked button. */
  readonly missingText = computed(() => missingSentence(this.choiceGroups()));

  constructor(private readonly host: ServerDraftHost) {
    // While the "Pontos de vida" box or the "Magias" step is on screen, the server derives the draft (a race and a
    // class at least): the box adds up what the effects give, and the spell pickers count against the class's numbers.
    effect(() => {
      const { views, picks, state } = this.host;
      const boxOn = serverDraftNeeded({
        ready: state().status === 'ready',
        rolledBox:
          views.selectedHitPointsMethod() === 'rolled' &&
          views.rollsNeeded() > 0 &&
          views.hitDie() > 0,
        caster: views.isCaster(),
        feats: views.master() && picks.featKeys().length > 0,
      });
      if (!boxOn) {
        untracked(() => this.hitPoints.stop());
        return;
      }
      const ready = this.draftReady();
      untracked(() => this.hitPoints.draftChanged(ready));
    });
    // The server reads the draft for its choices (a race and a class at least) and the "Escolhas" step shows them.
    effect(() => {
      const s = this.host.state();
      if (s.status !== 'ready' || !isFullSheetKind(s.kind)) {
        untracked(() => this.choices.stop());
        return;
      }
      const ready = this.draftReady();
      untracked(() => this.choices.draftChanged(ready));
    });
    // The server says which pick of the draft it cannot place when the lists it answered leave a held pick out.
    effect(() => {
      const s = this.host.state();
      // The master is never refused for the fit of a pick, so the server has no sentence to give.
      const ready =
        s.status === 'ready' &&
        isFullSheetKind(s.kind) &&
        !s.catalog.viewerIsMaster &&
        this.draftReady();
      const leftOut = this.holdsPickOutsideLists();
      untracked(() => this.pickFit.draftChanged(ready && leftOut));
    });
    // A pick the draft stores that no choice offers any more (the class or the race changed) goes, with its texts.
    effect(() => {
      const answer = this.choicesAnswer();
      if (!answer) {
        return;
      }
      const { featureChoiceKeys, featureChoiceText } = this.host.picks;
      untracked(() => {
        const next = choicesAfterAnswer(answer, featureChoiceKeys(), featureChoiceText());
        if (next.keys) {
          featureChoiceKeys.set(next.keys);
        }
        if (next.texts) {
          featureChoiceText.set(next.texts);
        }
      });
    });
  }

  /** Whether the draft holds a new skill or expertise (one the saved sheet did not have) that the lists of the server's last answer do not allow. */
  private holdsPickOutsideLists(): boolean {
    const answer = this.choicesAnswer();
    if (!answer) {
      return false;
    }
    const outside = (
      held: Iterable<string>,
      allowed: { readonly any: boolean; readonly keys: readonly string[] } | null,
    ): boolean =>
      allowed !== null &&
      !allowed.any &&
      Array.from(held).some((key) => !allowed.keys.includes(key));
    const { picks, views } = this.host;
    const newSkills = Array.from(views.chosenSkills()).filter((k) => !picks.heldAtOpen().has(k));
    const newExpertise = Array.from(picks.expertiseSkills()).filter(
      (k) => !picks.expertiseHeldAtOpen().has(k),
    );
    return (
      outside(newSkills, answer.skillsAllowed) || outside(newExpertise, answer.expertiseAllowed)
    );
  }

  /** The page is gone: nothing more is asked, and no late answer lands. */
  stop(): void {
    this.choices.stop();
    this.pickFit.stop();
    this.hitPoints.stop();
  }

  /** The draft has what the server needs to read it (a race and a class at least). Reading the draft here makes
   * every signal it is built from a reason to ask again. */
  private draftReady(): boolean {
    const { views } = this.host;
    views.formChanges();
    const ready = views.selectedRaceKey() !== '' && views.selectedClassKey() !== '';
    if (ready) {
      this.host.build();
    }
    return ready;
  }
}
