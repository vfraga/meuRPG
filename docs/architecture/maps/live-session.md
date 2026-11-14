# Maps in the live session

With the session open, each change to a map reaches whoever is on the session page at once, through `play`'s stream.
`maps` decides each event's audience, because only it knows what the change touched: the master always receives, and a player only when the change touches something they see, before or after it.
A change only to hidden things does not reach the player, not even as a `map_changed` on a map they see.
Without an open session nobody is subscribed and nothing is published; the app reads the maps when it opens the screen.

Each of these changes is one write under `play`'s funnel (`InSession`): the session's row is locked first, the number moves once, and the events below go out after the commit carrying it. A change that tells nobody (a paint of what was painted, a trap already disarmed) moves nothing. A repeat of a keyed call (`CreateMap`, a point, a treasure, a dungeon) moves nothing and tells the master again.

| Event | When | Who receives it |
| --- | --- | --- |
| `current_map_changed{map_id}` | The master changes the current map, or deletes it (empty) | Everyone |
| `map_changed{map_id}` | A point is created, changes, is deleted, revealed or hidden, or its scene's action changes; a token is placed (`PutPartyOnMap` too), hidden, shown or removed; `SetMapPlayersMove` changes who moves the tokens; the map changes name or image, is revealed, hidden, created or deleted; a layer is painted; the fog changes; a treasure is marked or unmarked; a trap is revealed | The master; the players, if they see the map before or after **and** the changed thing before or after. Exceptions: a trap revealed to some characters notifies only their players; a carried light only the owner; a light layer only the master; on a fogged map a change to a point notifies only players who see or remember one of its squares, and a layer only goes through `vision_changed` |
| `token_moved{map_id, character_id, x_bp, y_bp}` | The master (or, when allowed, the token's player) moves a token that was already on the map, one event for each token of a group move or of `PutPartyOnMap` that changed place | Everyone, if the token is visible on a map the players see; otherwise only the master. On a fogged map only a player character's: an NPC's reaches the player as `vision_changed` |
| `trap_noticed{map_id, point_id}` | A player's character noticed a trap while passing near (MR-035) | Only their player, and only if they see the map; never another player nor the master (RN-10). The screen shows "Você notou uma armadilha." and reads the map again |
| `vision_changed{map_id}` | On a fogged map, what a player sees or remembers changed, or an NPC moved on a square they see | The players it touches, at most one in each stream's queue; the master too, when a sight zone or a Wild Shape changes ([Fog](fog.md#the-stream)) |

- **`map_changed` is only a notice.** It carries no content: the app reads the map again (`GetMap`), already filtered, so a point's name or description never travels through the stream. If the player no longer sees the map, `GetMap` answers `not_found` and the app leaves it; a `map_changed` for a map the app does not list yet (just revealed) means the list changed (`ListMaps`).
- **`token_moved` carries the position:** the app moves the token without reading the map again. A map read that began before that edit (or before the answer of the master's own call) is dropped and made again, so an older answer never puts the token back and the screen's other changes are not lost. The same rule holds for the scene (`SceneState`) and the notes (`NotesState`): a local answer does not discard a read in flight, it makes the screen read once more. When two reads of the map overlap (a screen's own refresh and the stream's), the older answer is dropped and whoever awaited it waits for the newer read, so a screen that reads the points right after its refresh (the trap search naming what it found) sees the newest map. An answer for a map other than the open one (the master switched maps before the reply) is not put into the open map. Stage calls of different NPCs overlap, so an older answer arriving after a newer one is not applied. A new token comes as `map_changed`, because the app needs the name.
- **Changing a map's visibility** also notifies the maps with a sub-map point leading to it: the point's destination appears or disappears for the player.
- **The master's light warning** has no event of its own: the master's screen reads `ListCharactersInTheDark` again on `map_changed`, `token_moved` and `vision_changed`, which every change to a light, a Darkness or where a character stands sends the master ([Fog](fog.md#the-masters-light-warning)).
- **On the master's map card** (web) the warning is the amber notice over the fog map: one name with "Ver no mapa" and "Acender luz", or the count, the cause once and a pair of buttons per character; nothing when nobody is in the dark, the fog is off or the read fails (no error in that card). Players' pages read nothing and draw nothing of it (RN-10). `core/maps/dark-watch.ts` makes one more read for a burst of hints (`scheduleVision`: `token_moved`, `vision_changed`, combat moves, `map_changed`; and a change of the map's tokens), and says in a polite live region when a carried light went out or a character lit one. "Ver no mapa" centers the map and labels only the chosen character (`app-dark-marks`; everyone in the dark has the dashed ring); "Acender luz" opens the existing carried-light sheet. The party panel shows a "No escuro" badge. The map page shows the same notice, one line in "Pintar" and without "Acender luz" (that page has no quick light control). Text: `core/maps/in-the-dark.ts`; component: `shared/dark-warning`.
- **Painting** sends `map_changed` at most once per second per map ([Editor](editor.md#layers)).
- **The refresh after a write queues behind other writers of the session.** `rememberFor` stores each player's grown memory in a write of its own that takes the session row `FOR UPDATE` first and moves no number, so a refresh waits for a session writer that holds the row. No read reaches it: `GetMapVision` and the tiles never write a memory ([Fog](fog.md)).

```mermaid
sequenceDiagram
    participant M as Master
    participant MS as maps
    participant P as play
    participant H as hub, in memory
    participant DB as CockroachDB
    participant J as Player's app
    M->>MS: SetMapPointRevealed(point, revealed)
    MS->>MS: authz, master only
    MS->>P: CurrentMapID
    P->>DB: open session's current map
    MS->>DB: BEGIN, checks the map, locks the point, reveals, COMMIT
    MS->>MS: does the player see the map, and was the point or did it become revealed?
    MS->>P: Publish(map_changed, players: yes)
    P->>H: to the master and the players
    H-->>J: map_changed(map)
    J->>MS: GetMap
    MS-->>J: the map, with the point revealed and without the hidden ones
```
