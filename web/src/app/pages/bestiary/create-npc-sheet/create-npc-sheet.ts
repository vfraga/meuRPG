import { Component, ElementRef, computed, effect, inject, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Creature } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { bestiaryErrorMessage, npcAlreadyMade } from '../../../core/creatures/bestiary-errors';
import { listWithE, npcSpeedFt } from '../../../core/creatures/bestiary-format';
import { CreaturesClient, type NpcRole } from '../../../core/creatures/creatures-client';
import { nameCounter } from '../../../core/creatures/creature-format';
import { newKey } from '../../../core/connect/idempotency';
import { tieNumbers } from '../../../core/format/text';
import { metersText } from '../../../core/units';
import { CreatureArt } from '../../../shared/creatures/creature-art';
import { SheetFrame } from '../../live-session/combat/sheet-frame/sheet-frame';
import { injectSheet } from '../../live-session/combat/sheet-host';

/** What the stat block page hands the dialog. */
export interface CreateNpcData {
  readonly campaignId: string;
  readonly creature: Creature;
}

/** The NPC that was made, for the page's confirmation. */
export interface CreateNpcResult {
  readonly id: string;
  readonly name: string;
  /** The names of the attacks the NPC's sheet really got (the server's answer). */
  readonly attacks: readonly string[];
  /** The server said this dialog's NPC exists already (a retry after a lost answer): `id` is not known then. */
  readonly existed: boolean;
}

/** The longest name an NPC has (`CreateNpcFromCreature`: 1 to 80 characters, one line). */
const NAME_MAX = 80;

/** The roles a basic sheet can take: the full-sheet ones (Inimigo, Boss) need a class and a race, so they are not offered. */
const ROLES: readonly { value: NpcRole; label: string; hint: string }[] = [
  {
    value: 'minion',
    label: 'Minion',
    hint: 'Um capanga: ficha simples, entra nos combates como inimigo.',
  },
  {
    value: 'story',
    label: 'NPC de história',
    hint: 'Um personagem da trama: ficha simples, para conversar e interpretar.',
  },
];

/**
 * "Criar NPC" (MR-042, RN-29, E10-08, state 8): a named NPC with a basic sheet made from an SRD
 * creature. The master gives the NPC its own name (the creature's Portuguese name to start) and a
 * role, Minion or NPC de história; the numbers (CA, PV, speed, the six abilities) and the attacks
 * come from the creature and are only shown, since the NPC is edited as any NPC afterwards. The
 * server makes the NPC (`CreateNpcFromCreature`): the bestiary does not change and the NPC's token
 * is born hidden (RN-10).
 *
 * The create key is made once, when the dialog opens, and sent again on every try, whatever the
 * name or role says by then: a second tap, or a retry after a lost answer, makes one NPC. If the
 * server says the key was used already (the first try did make the NPC), the dialog closes saying
 * "Já foi criado." instead of showing an error.
 *
 * It is a bottom sheet on a phone and a dialog from a tablet up (`openSheet`); the body scrolls inside
 * it and the footer stays in reach (320×568 shows the numbers as one line).
 */
@Component({
  selector: 'app-create-npc-sheet',
  imports: [CreatureArt, MatButtonModule, MatIconModule, SheetFrame],
  templateUrl: './create-npc-sheet.html',
  styleUrl: './create-npc-sheet.scss',
})
export class CreateNpcSheet {
  private readonly client = inject(CreaturesClient);
  private readonly sheet = injectSheet<CreateNpcData, CreateNpcResult>();
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly roles = ROLES;
  protected readonly max = NAME_MAX;
  protected readonly nameCounter = nameCounter;

  protected readonly name = signal(this.data.creature.summary?.namePt ?? '');
  protected readonly role = signal<NpcRole>('minion');
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly nameError = signal('');

  /** One key for this dialog, never replaced. */
  private readonly key = newKey();

  private readonly c = this.data.creature;
  protected readonly creatureName = computed(() => this.c.summary?.namePt ?? '');
  protected readonly englishName = computed(() => this.c.summary?.name ?? '');
  protected readonly challenge = computed(() => `ND ${this.c.summary?.challengeRating ?? '0'}`);
  protected readonly roleHint = computed(
    () => ROLES.find((r) => r.value === this.role())?.hint ?? '',
  );
  protected readonly speed = metersText(npcSpeedFt(this.c));
  protected readonly oneLine = tieNumbers(
    `CA ${this.c.armorClass} · PV ${this.c.hitPoints} · ${this.speed}`,
  );
  /** The attacks that go along come from the server (`npc_attack_names`: the same function that builds the sheet). */
  protected readonly attackNote = computed(() => {
    const names = this.c.npcAttackNames;
    const own = 'A criatura do bestiário não muda.';
    return names.length > 0
      ? `Faz uma ficha básica de NPC, com os ataques da criatura (${listWithE(names)}), que você renomeia e edita depois. ${own}`
      : `Faz uma ficha básica de NPC com os números da criatura; os ataques dela ficam na ficha do bestiário. ${own}`;
  });

  constructor() {
    // While CreateNpc runs the sheet stays: closing would lose the answer and a reopened sheet is a new request.
    effect(() => this.sheet.lock(this.busy()));
  }

  protected setName(value: string): void {
    this.name.set(value);
    this.nameError.set('');
  }

  protected setRole(role: NpcRole): void {
    this.role.set(role);
  }

  protected async create(): Promise<void> {
    if (this.busy()) {
      return;
    }
    const name = this.name().trim();
    if (name === '') {
      this.nameError.set('Dê um nome ao NPC.');
      this.host.nativeElement.querySelector<HTMLInputElement>('input[name=name]')?.focus();
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const made = await this.client.createNpc(
        this.data.campaignId,
        this.c.summary?.key ?? '',
        name,
        this.role(),
        this.key,
      );
      const sheet = made?.sheet?.content;
      const attacks = sheet?.case === 'basic' ? sheet.value.attacks.map((a) => a.name) : [];
      this.sheet.close({ id: made?.id ?? '', name: made?.name ?? name, attacks, existed: false });
    } catch (err) {
      if (npcAlreadyMade(err)) {
        this.sheet.close({ id: '', name, attacks: [], existed: true });
        return;
      }
      this.error.set(bestiaryErrorMessage(err, 'create'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close();
  }
}
