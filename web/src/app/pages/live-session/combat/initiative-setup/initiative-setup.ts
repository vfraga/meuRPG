import {
  Component,
  ElementRef,
  computed,
  effect,
  input,
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Combatant, Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  combatantInitial,
  initiativeFormula,
  isPlayer,
  moveInGroup,
  pendingFormula,
  signed,
  tieGroups,
  tieSentence,
} from '../../../../core/combat/combat-view';
import { isCreature } from '../../../../core/combat/creature-names';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import type { CombatantInfo } from '../combat-info';

/** A row of the list: a combatant, or the sentence over a tie group. */
type Item =
  | { readonly kind: 'tie'; readonly text: string }
  | {
      readonly kind: 'row';
      readonly c: Combatant;
      readonly place: number;
      readonly tie: string[] | null;
    };

/**
 * The master's initiative list (E6-04): every combatant with the formula of
 * its roll, in the order the combat will play. A tie that is not ordered yet
 * is one yellow group with arrows that move a combatant inside it only
 * (`SetInitiativeOrder`); a missing roll says who it waits for, and the
 * master may type it for the player ("Digitar pelo jogador"); "Editar" types
 * a new face for anyone else. Typing a face is the one inline edit: a field
 * inside the formula, `1d20 ( 7 ) + 2 = 9`, which updates as the number does.
 */
@Component({
  selector: 'app-initiative-setup',
  imports: [CombatantToken, MatButtonModule, MatIconModule],
  templateUrl: './initiative-setup.html',
  styleUrl: './initiative-setup.scss',
})
export class InitiativeSetup {
  readonly encounter = input.required<Encounter>();
  readonly info = input<ReadonlyMap<string, CombatantInfo>>(new Map());
  readonly busy = input(false);

  /** The master typed a d20 face for a combatant. */
  readonly submit = output<{ id: string; face: number }>();
  /** The new order of a tie group. */
  readonly order = output<string[]>();

  protected readonly editing = signal<string | null>(null);
  protected readonly typed = signal('');
  /** The face the combatant had when the field opened. */
  private readonly editedFace = signal<number | undefined>(undefined);
  private readonly field = viewChild<ElementRef<HTMLInputElement>>('field');

  protected readonly groups = computed(() => tieGroups(this.encounter().combatants));
  protected readonly items = computed<Item[]>(() => {
    const groups = this.groups();
    const list = this.encounter().combatants;
    const items: Item[] = [];
    list.forEach((c, index) => {
      const group = groups.find((g) => g.includes(c.id)) ?? null;
      if (group && group[0] === c.id) {
        const labels = group.map((id) => list.find((x) => x.id === id)?.label ?? '');
        items.push({ kind: 'tie', text: tieSentence(labels, c.initiative) });
      }
      items.push({ kind: 'row', c, place: index + 1, tie: group });
    });
    return items;
  });
  protected readonly face = computed(() => {
    const value = this.typed().trim();
    return /^\d{1,2}$/.test(value) && +value >= 1 && +value <= 20 ? +value : null;
  });
  protected readonly invalid = computed(() => this.typed().trim() !== '' && this.face() === null);

  constructor() {
    // Opening the edit puts the cursor in the field.
    effect(() => this.field()?.nativeElement.focus());
    // The roll the field was opened over changed (the player rolled meanwhile):
    // the field closes and the roll that stands is what the list shows.
    effect(() => {
      const combatants = this.encounter().combatants;
      const id = untracked(this.editing);
      if (id === null) {
        return;
      }
      const c = combatants.find((x) => x.id === id);
      if (!c || c.initiativeFace !== untracked(this.editedFace)) {
        this.editing.set(null);
      }
    });
  }

  protected initial(c: Combatant): string {
    return combatantInitial(c.label);
  }

  protected npc(c: Combatant): boolean {
    return !isPlayer(c) && !isCreature(c);
  }

  protected creature(c: Combatant): boolean {
    return isCreature(c);
  }

  protected formula(c: Combatant): string {
    return initiativeFormula(c) ?? pendingFormula(c);
  }

  protected bonus(c: Combatant): string {
    return signed(c.initiativeBonus ?? 0);
  }

  /** The total the typed face would make: `2 + 7`. */
  protected total(c: Combatant): string {
    const face = this.face();
    return face === null ? '—' : String(face + (c.initiativeBonus ?? 0));
  }

  protected sub(c: Combatant): string {
    const info = this.info().get(c.characterId);
    if (!isPlayer(c)) {
      const parts = [info?.kindLabel ?? 'NPC'];
      if (c.hidden) {
        parts.push('escondido');
      }
      parts.push('o app rolou');
      return parts.join(' · ');
    }
    return [info?.classSummary, info?.playerName ? `de ${info.playerName}` : null]
      .filter(Boolean)
      .join(' · ');
  }

  protected waitingFor(c: Combatant): string {
    return this.info().get(c.characterId)?.playerName ?? c.label;
  }

  protected edit(c: Combatant): void {
    this.typed.set(c.initiativeFace === undefined ? '' : String(c.initiativeFace));
    this.editedFace.set(c.initiativeFace);
    this.editing.set(c.id);
  }

  protected save(c: Combatant): void {
    const face = this.face();
    if (face === null || this.busy()) {
      return;
    }
    this.editing.set(null);
    this.submit.emit({ id: c.id, face });
  }

  protected onType(event: Event): void {
    this.typed.set((event.target as HTMLInputElement).value);
  }

  protected canUp(group: string[], c: Combatant): boolean {
    return group.indexOf(c.id) > 0;
  }

  protected canDown(group: string[], c: Combatant): boolean {
    return group.indexOf(c.id) < group.length - 1;
  }

  protected move(group: string[], c: Combatant, direction: -1 | 1): void {
    const next = moveInGroup(group, c.id, direction);
    if (next.join() !== group.join()) {
      this.order.emit(next);
    }
  }
}
