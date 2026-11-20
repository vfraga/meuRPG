import { signal } from '@angular/core';

import {
  describeCharacterError,
  invalidFieldPath,
  pickRefusalOf,
  switchedOffKey,
} from '../../core/characters/character-errors';
import { ActionKey } from '../../core/connect/idempotency';
import { IDLE_SAVING, type SavingState } from '../../core/connect/saving-state';
import type {
  AbilityMethodKey,
  BasicCharacterFormValue,
  CharacterEditorSource,
  CharacterFormValue,
  CreateCharacterInput,
  UpdateCharacterInput,
} from './character-editor.types';
import type { ReadyState } from './editor-state';
import type { EditorStepKey } from './editor-labels';

/** The class block a refusal points at (`full.classes[i]`), so the block says so; `null` for any other refusal. */
export function classBlockOfRefusal(err: unknown): number | null {
  const field = invalidFieldPath(err);
  const block = field ? /^full\.classes\[(\d+)\]/.exec(field) : null;
  return block ? Number(block[1]) : null;
}

/** The request that makes a character, without its idempotency key. */
export function createInputOf(
  s: ReadyState,
  parts: {
    readonly full: CharacterFormValue | null;
    readonly basic: BasicCharacterFormValue | null;
    /** How a player's scores were made, when the table has ways to make them. */
    readonly abilityMethod: AbilityMethodKey | null;
  },
): Omit<CreateCharacterInput, 'idempotencyKey'> {
  return {
    campaignId: s.campaignId,
    kind: s.kind,
    full: parts.full,
    basic: parts.basic,
    ...(parts.abilityMethod !== null && parts.full ? { abilityMethod: parts.abilityMethod } : {}),
    ...(s.reserved ? { forPlayer: true } : {}),
  };
}

/** Where a saved new character goes: a reserved one to the list, where its link is made; any other to its sheet. */
export function afterCreateTarget(s: ReadyState, characterId: string): string[] {
  return s.reserved
    ? ['/campaigns', s.campaignId]
    : ['/campaigns', s.campaignId, 'characters', characterId];
}

/** What `SaveFlow` reads from the page it saves for. */
export interface SaveFlowContext {
  readonly source: Pick<CharacterEditorSource, 'createCharacter' | 'updateCharacter'>;
  readonly navigate: (commands: string[]) => Promise<boolean>;
  /** The full sheet and the short one the forms stand for now. */
  readonly full: () => CharacterFormValue;
  readonly basic: () => BasicCharacterFormValue;
  /** The name typed in the form of the kind being saved. */
  readonly name: (isBasic: boolean) => string;
  /** The way the player chose to make the scores, `null` when the table offers none. */
  readonly abilityMethod: () => AbilityMethodKey | null;
  /** Taken when a save starts: it says whether the person is still on the page and route the save began on. */
  readonly stillHere: () => () => boolean;
  readonly nameOfKey: (key: string) => string | undefined;
  /** Gives the field holding a switched-off key its error; false when no field holds it. */
  readonly markSwitchedOff: (key: string) => boolean;
  readonly refreshCatalog: () => Promise<void>;
  /** Opens a step of the editor, so a refusal that is about its field is on screen. */
  readonly openStep?: (step: EditorStepKey) => void;
}

/**
 * The save of the editor: the request for a new sheet or an edit, the navigation after it and what a refusal does to
 * the page. Plain class the editor builds; the editor decides whether the forms may be saved at all.
 */
export class SaveFlow {
  readonly state = signal<SavingState>(IDLE_SAVING);
  /** The class block a refusal points at (`full.classes[i]`), so the block says so. */
  readonly classProblem = signal<number | null>(null);
  /** The server's sentence for a skill or an expertise no list allows, shown on the "Perícias" step; empty when none. */
  readonly pickProblem = signal('');
  private readonly createKey = new ActionKey();

  constructor(private readonly host: SaveFlowContext) {}

  /** The refusal of a pick goes, and so does the notice that points to it. */
  clearPickProblem(): void {
    if (this.pickProblem() === '') {
      return;
    }
    this.pickProblem.set('');
    this.state.update((s) => (s.status === 'error' ? { status: 'idle' } : s));
  }

  /** A reused page starts from nothing. */
  reset(): void {
    this.classProblem.set(null);
    this.pickProblem.set('');
    this.state.set({ status: 'idle' });
  }

  /** Saves; the person may have left meanwhile, and then the save still ends on the server but nothing else happens. */
  async run(s: ReadyState, isBasic: boolean): Promise<void> {
    this.state.set({ status: 'saving' });
    this.pickProblem.set('');
    // The save finishes on the server whatever the person does meanwhile; only the navigation depends on still being here.
    const stillHere = this.host.stillHere();
    try {
      if (!(await this.persist(s, isBasic, stillHere))) {
        return;
      }
      this.state.set({ status: 'idle' });
    } catch (err) {
      if (!stillHere()) {
        return;
      }
      this.refused(err);
    }
  }

  /** Sends the sheet and goes where it leads; false when the person left meanwhile (nothing more to do). */
  private async persist(
    s: ReadyState,
    isBasic: boolean,
    stillHere: () => boolean,
  ): Promise<boolean> {
    if (s.mode === 'create') {
      const input = createInputOf(s, {
        full: isBasic ? null : this.host.full(),
        basic: isBasic ? this.host.basic() : null,
        abilityMethod: this.host.abilityMethod(),
      });
      // A retry of the same form (a lost answer, a second tap) sends the same key and makes one character.
      const res = await this.host.source.createCharacter({
        ...input,
        idempotencyKey: this.createKey.keyFor(input),
      });
      this.createKey.renew();
      if (!stillHere()) {
        return false;
      }
      await this.host.navigate(afterCreateTarget(s, res.characterId));
    } else if (s.characterId) {
      await this.host.source.updateCharacter(this.updateInputOf(s, s.characterId, isBasic));
      if (!stillHere()) {
        return false;
      }
      await this.host.navigate(['/campaigns', s.campaignId, 'characters', s.characterId]);
    }
    return true;
  }

  private updateInputOf(
    s: ReadyState,
    characterId: string,
    isBasic: boolean,
  ): UpdateCharacterInput {
    return {
      campaignId: s.campaignId,
      characterId,
      revision: s.revision,
      name: this.host.name(isBasic),
      full: isBasic ? null : this.host.full(),
      basic: isBasic ? this.host.basic() : null,
    };
  }

  /** The server refused: a refusal that names a key says it by name; one that points at a class block marks that block. */
  private refused(err: unknown): void {
    this.classProblem.set(classBlockOfRefusal(err));
    const pick = pickRefusalOf(err);
    this.pickProblem.set(pick ?? '');
    if (pick) {
      this.host.openStep?.('pericias');
    }
    // An option the master switched off after the lists were read is an error on its field (E10-01 state 5). The name is
    // taken before the lists are read again, because the entry may be gone from them.
    const refused = switchedOffKey(err);
    if (refused !== null && this.host.markSwitchedOff(refused)) {
      void this.host.refreshCatalog();
      return;
    }
    this.state.set({
      status: 'error',
      message: pick
        ? 'Veja o passo Perícias.'
        : describeCharacterError(err, (key) => this.host.nameOfKey(key)),
    });
  }
}
