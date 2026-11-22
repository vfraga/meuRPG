# #857 review round shots (branch feat/ready-spells-857-web, head f8f8016f)

Each screen is a PNG at 390, 768 and 1280 px, light (`claro`) and dark (`escuro`), deviceScaleFactor 2, taken by the a11y scan with `E2E_SHOTS_DIR`.
Name: `857-ready-map-<screen>-<width>-<theme>.png`.

## New states

| screen | what it shows |
|---|---|
| `folha-recusada` | the player's release refused as a closed window (the master let it pass): the gone state, its sentence from the next read |
| `mestre-recusada` | the master's release dialog refused as a closed window (the player let it pass) |
| `mestre-desfeito` | the master's dialog after the trigger was undone: "O gatilho foi desfeito: a Bola de Fogo continua preparada." |
| `mestre-mover-fechou` | the master's Mover page after the window closed: "A janela fechou. Pensantus não se moveu, e a reação continua livre." |
| `janela-ao-redor`, `mapa-ao-redor` | a spell around the caster (Cúpula Antivida): the window and the map |
| `janela-nomes-longos`, `mestre-nomes-longos`, `mapa-nomes-longos` | a long spell name and a long creature name |
| `janela-so-em-quem-conjura` | a self-only release (Detectar Magia) |
| `janela-sem-mapa` | an area in a combat with no map (Sono): the window's list of creatures |

## Retaken on the final build

`mestre-fechou` (the master's "Passou sozinho: Pensantus não respondeu em 30 s."), `folha-desfeita`, `dado-digitado`, `resultado`, `janela-passou-sozinha`, `mover-bloqueado`, `mapa-muralha`, `mapa-direcao`.

## Not reached

- The cantrip band ("Truque · sem espaço"): no SRD cantrip is placed on the map, and the band shows only on a map release.
- The master's refusal for a spent reaction: the server closes a readied window as soon as the reaction is spent, so a release never reaches that refusal with the window open.
