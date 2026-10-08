package characters

import (
	"sync"
	"testing"
	"uuid"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// The audit of 07/10/2026 (F7): a retried CreateCharacter, GiveCreature or CreateTableEntry must not
// make a second character, creature or entry.

func TestCreateCharacterIsIdempotent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)

	key := uuid.New().String()
	req := &charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: pensantusSheet(), IdempotencyKey: key,
	}
	call := func(r *charactersv1.CreateCharacterRequest) (*charactersv1.Character, error) {
		res, err := player.api.CreateCharacter(t.Context(), connect.NewRequest(r))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetCharacter(), nil
	}
	first, err := call(req)
	if err != nil {
		t.Fatalf("CreateCharacter() error = %v", err)
	}
	// A retry is the first character, not "you already have a living character".
	again, err := call(req)
	if err != nil {
		t.Fatalf("CreateCharacter() retry error = %v", err)
	}
	if again.GetId() != first.GetId() {
		t.Errorf("retry = character %q, want the first, %q", again.GetId(), first.GetId())
	}
	// The same key with another request is refused.
	other := proto.Clone(req).(*charactersv1.CreateCharacterRequest)
	other.Name = "Outro"
	_, err = call(other)
	wantCode(t, "CreateCharacter(same key, other name)", err, connect.CodeInvalidArgument)
	if got := player.list(t, campaign); len(got) != 1 {
		t.Errorf("characters = %d, want 1", len(got))
	}
	// A key that is not a UUID is refused: the column is one.
	bad := proto.Clone(req).(*charactersv1.CreateCharacterRequest)
	bad.IdempotencyKey = "not-a-uuid"
	_, err = call(bad)
	wantCode(t, "CreateCharacter(key not a UUID)", err, connect.CodeInvalidArgument)
	// No key is not deduplicated: a second living character is still refused, as before.
	none := proto.Clone(req).(*charactersv1.CreateCharacterRequest)
	none.IdempotencyKey = ""
	_, err = call(none)
	wantCode(t, "CreateCharacter(no key, a living character)", err, connect.CodeFailedPrecondition)

	// An NPC of the master, with the same two calls at once, is one NPC.
	dbtest.PoolSize(t, 4)
	npcKey := uuid.New().String()
	ids := make([]string, 4)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Go(func() {
			res, err := master.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
				CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, Name: "Orc", Sheet: enemySheet(), IdempotencyKey: npcKey,
			}))
			if err != nil {
				t.Errorf("CreateCharacter(NPC) error = %v", err)
				return
			}
			ids[i] = res.Msg.GetCharacter().GetId()
		})
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("NPC ids = %v, want one NPC", ids)
		}
	}
	if got := master.list(t, campaign); len(got) != 2 {
		t.Errorf("characters = %d, want the player and one NPC", len(got))
	}
}

func TestGiveCreatureIsIdempotent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	pc := player.createPensantus(t, campaign)

	req := &charactersv1.GiveCreatureRequest{CampaignId: campaign, CharacterId: pc.GetId(), MonsterKey: "monster:wolf", Name: "Presa", IdempotencyKey: uuid.New().String()}
	give := func(r *charactersv1.GiveCreatureRequest) (*charactersv1.CharacterCreature, error) {
		res, err := master.api.GiveCreature(t.Context(), connect.NewRequest(r))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetCreature(), nil
	}
	first, err := give(req)
	if err != nil {
		t.Fatalf("GiveCreature() error = %v", err)
	}
	again, err := give(req)
	if err != nil {
		t.Fatalf("GiveCreature() retry error = %v", err)
	}
	if again.GetId() != first.GetId() {
		t.Errorf("retry = creature %q, want the first, %q", again.GetId(), first.GetId())
	}
	other := proto.Clone(req).(*charactersv1.GiveCreatureRequest)
	other.MonsterKey = "monster:bat"
	_, err = give(other)
	wantCode(t, "GiveCreature(same key, other creature)", err, connect.CodeInvalidArgument)
	list, err := master.api.ListCharacterCreatures(t.Context(), connect.NewRequest(&charactersv1.ListCharacterCreaturesRequest{CampaignId: campaign, CharacterId: pc.GetId()}))
	if err != nil || len(list.Msg.GetCreatures()) != 1 {
		t.Errorf("ListCharacterCreatures() = %v, %v; want 1 creature", list, err)
	}
	// A new key, or none, gives another one: that is what the call is for.
	next := proto.Clone(req).(*charactersv1.GiveCreatureRequest)
	next.IdempotencyKey = uuid.New().String()
	if made, err := give(next); err != nil || made.GetId() == first.GetId() {
		t.Errorf("new key = %v, %v; want another creature", made, err)
	}
	next.IdempotencyKey = ""
	if made, err := give(next); err != nil || made.GetId() == first.GetId() {
		t.Errorf("no key = %v, %v; want another creature", made, err)
	}
	bad := proto.Clone(req).(*charactersv1.GiveCreatureRequest)
	bad.IdempotencyKey = "not-a-uuid"
	_, err = give(bad)
	wantCode(t, "GiveCreature(key not a UUID)", err, connect.CodeInvalidArgument)
}

func TestCreateTableEntryIsIdempotent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master, "Mirathel")

	req := createReq(campaign, testBackground("guarda de farol"))
	req.IdempotencyKey = "key-1"
	create := func(r *rulesv1.CreateTableEntryRequest) (*rulesv1.CreateTableEntryResponse, error) {
		res, err := master.table.CreateTableEntry(t.Context(), connect.NewRequest(r))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	first, err := create(req)
	if err != nil {
		t.Fatalf("CreateTableEntry() error = %v", err)
	}
	// A retry is the first entry, not "an entry with that name exists", and it keeps the revision.
	again, err := create(req)
	if err != nil {
		t.Fatalf("CreateTableEntry() retry error = %v", err)
	}
	if again.GetEntry().GetKey() != first.GetEntry().GetKey() || again.GetTableRevision() != first.GetTableRevision() {
		t.Errorf("retry = %q at revision %d, want %q at %d", again.GetEntry().GetKey(), again.GetTableRevision(), first.GetEntry().GetKey(), first.GetTableRevision())
	}
	other := createReq(campaign, testBackground("capitão do porto"))
	other.IdempotencyKey = "key-1"
	_, err = create(other)
	wantCode(t, "CreateTableEntry(same key, other entry)", err, connect.CodeInvalidArgument)
	list, err := master.table.ListTableEntries(t.Context(), connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: campaign}))
	if err != nil || len(list.Msg.GetEntries()) != 1 {
		t.Errorf("ListTableEntries() = %v, %v; want 1 entry", list, err)
	}
	// Racing with the same key makes one entry too.
	dbtest.PoolSize(t, 4)
	racing := createReq(campaign, testBackground("faroleiro"))
	racing.IdempotencyKey = "racing"
	var wg sync.WaitGroup
	keys := make([]string, 4)
	for i := range keys {
		wg.Go(func() {
			res, err := create(racing)
			if err != nil {
				t.Errorf("CreateTableEntry() racing error = %v", err)
				return
			}
			keys[i] = res.GetEntry().GetKey()
		})
	}
	wg.Wait()
	for _, k := range keys {
		if k != keys[0] {
			t.Fatalf("keys = %v, want one entry", keys)
		}
	}
	list, err = master.table.ListTableEntries(t.Context(), connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: campaign}))
	if err != nil || len(list.Msg.GetEntries()) != 2 {
		t.Errorf("ListTableEntries() = %v, %v; want 2 entries", list, err)
	}
}

// The create key is the caller's: another member sending the same key with an identical request
// gets a character of their own, not the first member's, and neither sees the other's key as used.
func TestCreateCharacterKeyIsPerCaller(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, a, b := h.newUser("Mestre"), h.newUser("Jogadora A"), h.newUser("Jogador B")
	campaign := h.newCampaign(master, "Mirathel", a, b)

	req := &charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: pensantusSheet(), IdempotencyKey: uuid.New().String(),
	}
	create := func(u *user, r *charactersv1.CreateCharacterRequest) (*charactersv1.Character, error) {
		res, err := u.api.CreateCharacter(t.Context(), connect.NewRequest(r))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetCharacter(), nil
	}
	charA, err := create(a, req)
	if err != nil {
		t.Fatalf("A CreateCharacter() error = %v", err)
	}
	charB, err := create(b, req)
	if err != nil {
		t.Fatalf("B CreateCharacter() with A's key error = %v", err)
	}
	if charB.GetId() == charA.GetId() {
		t.Errorf("B got A's character %q as a replay; want a new character of B", charA.GetId())
	}
	if charB.GetPlayerUserId() != b.id {
		t.Errorf("B's character player_user_id = %q, want B's %q; A is %q", charB.GetPlayerUserId(), b.id, a.id)
	}
	// B's own retry is B's character, and a different request with B's key is refused.
	again, err := create(b, req)
	if err != nil || again.GetId() != charB.GetId() {
		t.Errorf("B's retry = %v, %v; want B's character %q", again.GetId(), err, charB.GetId())
	}
	other := proto.Clone(req).(*charactersv1.CreateCharacterRequest)
	other.Name = "Outro"
	_, err = create(b, other)
	wantCode(t, "B CreateCharacter(same key, other name)", err, connect.CodeInvalidArgument)
	// A's retry still finds A's character.
	if again, err := create(a, req); err != nil || again.GetId() != charA.GetId() {
		t.Errorf("A's retry = %v, %v; want A's character %q", again.GetId(), err, charA.GetId())
	}
}
