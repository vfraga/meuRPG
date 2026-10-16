import { Component, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';

import { aidTagForMaster } from '../../../core/combat/combat-effects';
import { NudgeButton } from '../nudge-button/nudge-button';
import { PartyMemberInfoVm, VitalsVm } from '../../../core/play/live-session.types';
import { SessionXp } from '../session-xp/session-xp';
import { SlotDots } from '../../../shared/slot-dots/slot-dots';
import {
  freeWords,
  hitPointsPercent,
  hitPointsState,
  partyRowSub,
  slotRowLabel,
} from '../../../core/play/vitals';

/**
 * The master's "Grupo" panel on the session page (artboards E5-04 and
 * E5-06): each living player character with its hit points first (current
 * at 30px, "de 24 PV"), a word when they're low ("Abaixo da metade",
 * "Inconsciente"), the temporary HP, a bar and the spell slots, and a
 * stroked "Ajustar" that opens the adjust sheet (RN-02), and "Chamar a atenção", which makes the character's player's screen alert. No filled button:
 * "Ajustar" repeats per row, and filling one would single a character out.
 */
@Component({
  selector: 'app-party-panel',
  imports: [MatButtonModule, NudgeButton, SessionXp, SlotDots],
  templateUrl: './party-panel.html',
  styleUrl: './party-panel.scss',
})
export class PartyPanel {
  readonly campaignId = input.required<string>();
  readonly campaignName = input('');
  readonly party = input.required<readonly VitalsVm[]>();
  readonly info = input<ReadonlyMap<string, PartyMemberInfoVm>>(new Map());

  /** "Ajustar" on a row. */
  readonly adjust = output<VitalsVm>();

  protected readonly aidTag = aidTagForMaster;
  protected readonly percent = hitPointsPercent;
  protected readonly hpState = hitPointsState;
  protected readonly sub = partyRowSub;
  protected readonly slotRowLabel = slotRowLabel;
  protected readonly free = freeWords;
}
