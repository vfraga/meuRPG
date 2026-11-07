import type { TvScreenVm } from '../../core/play/table-screen-view';

/** What the header says of the stage: the icon, the small line above and the title. */
export interface TvHeading {
  readonly icon: string;
  readonly kicker: string;
  readonly title: string;
}

/** The header of each kind of screen ("Combate · Cripta do Corvo", "Sessão 3"). */
export function tvHeading(screen: TvScreenVm): TvHeading {
  const where = `${screen.campaignName} · ${screen.sessionLabel}`;
  const stage = screen.stage;
  switch (stage.kind) {
    case 'map':
      return { icon: 'map', kicker: `Mapa · ${where}`, title: stage.map.name || 'Mapa' };
    case 'image':
      return { icon: 'image', kicker: `Imagem · ${where}`, title: stage.name || 'Imagem' };
    case 'scene':
      return {
        icon: 'chat_bubble_outline',
        kicker: `Cena · ${where}`,
        title: stage.name || 'Cena',
      };
    case 'combat':
      return { icon: 'swords', kicker: where, title: combatTitle(stage.map?.name ?? null) };
    default:
      return { icon: 'tv', kicker: screen.campaignName, title: screen.sessionLabel };
  }
}

function combatTitle(mapName: string | null): string {
  if (mapName === null) {
    return 'Combate sem mapa';
  }
  return mapName ? `Combate · ${mapName}` : 'Combate';
}

/** What a screen reader hears when the stage changes ("Na tela: Combate · Cripta do Corvo"). */
export function tvAnnouncement(screen: TvScreenVm): string {
  return `Na tela: ${tvHeading(screen).title}`;
}
