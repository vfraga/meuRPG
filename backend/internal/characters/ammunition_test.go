package characters

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

const (
	arrowKey = "equipment:arrow"
	boltKey  = "equipment:crossbow-bolt"
)

func (et *editorTable) setAmmo(t *testing.T, pc *charactersv1.Character, key string, quantity int32) (*charactersv1.SheetAmmunition, error) {
	t.Helper()
	var got *charactersv1.SheetAmmunition
	err := et.inTx(t, func(tx pgx.Tx) error {
		var err error
		got, err = et.h.svc.SetAmmunition(t.Context(), tx, et.campaign, pc.GetId(), key, quantity)
		return err
	})
	return got, err
}

func (et *editorTable) shoot(t *testing.T, pc *charactersv1.Character, key, encounter string) (int32, error) {
	t.Helper()
	left, _, err := et.shootCounted(t, pc, key, encounter)
	return left, err
}

// shootCounted is shoot with whether the kind is counted.
func (et *editorTable) shootCounted(t *testing.T, pc *charactersv1.Character, key, encounter string) (left int32, counted bool, err error) {
	t.Helper()
	err = et.inTx(t, func(tx pgx.Tx) error {
		var err error
		left, counted, err = et.h.svc.SpendAmmunition(t.Context(), tx, et.campaign, pc.GetId(), key, encounter)
		return err
	})
	return left, counted, err
}

func (et *editorTable) recover(t *testing.T, pc *charactersv1.Character, encounter string) ([]*charactersv1.SheetAmmunition, error) {
	t.Helper()
	var got []*charactersv1.SheetAmmunition
	err := et.inTx(t, func(tx pgx.Tx) error {
		var err error
		got, err = et.h.svc.RecoverAmmunition(t.Context(), tx, et.campaign, pc.GetId(), encounter)
		return err
	})
	return got, err
}

func ammoOf(c *charactersv1.Character, key string) *charactersv1.SheetAmmunition {
	for _, a := range c.GetSheet().GetFull().GetAmmunition() {
		if a.GetKey() == key {
			return a
		}
	}
	return nil
}

func TestSetAmmunition(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	got, err := et.setAmmo(t, pc, arrowKey, 20)
	if err != nil || got.GetKey() != arrowKey || got.GetQuantity() != 20 || got.GetNamePt() == "" {
		t.Fatalf("set: %+v %v", got, err)
	}
	if _, err := et.setAmmo(t, pc, boltKey, 5); err != nil {
		t.Fatalf("set bolts: %v", err)
	}
	if _, err := et.setAmmo(t, pc, arrowKey, 0); err != nil {
		t.Fatalf("set to none: %v", err)
	}
	for _, reader := range []*user{et.master, et.dona} {
		read := et.read(t, reader, pc.GetId())
		if a := ammoOf(read, boltKey); a.GetQuantity() != 5 || a.GetNamePt() == "" {
			t.Errorf("bolts: %+v", read.GetSheet().GetFull().GetAmmunition())
		}
		if a := ammoOf(read, arrowKey); a == nil || a.GetQuantity() != 0 {
			t.Errorf("a counted kind set to none is not listed at 0: %+v", read.GetSheet().GetFull().GetAmmunition())
		}
	}
	for _, c := range []struct {
		name, key string
		quantity  int32
	}{
		{"not ammunition", "equipment:longbow", 1},
		{"unknown", "", 1},
		{"negative", arrowKey, -1},
		{"too many", arrowKey, 10000},
	} {
		if _, err := et.setAmmo(t, pc, c.key, c.quantity); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if _, err := et.setAmmo(t, &charactersv1.Character{Id: uuid.NewString()}, arrowKey, 1); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("another character: %v", err)
	}
}

func TestReadListsTheKindsInOrderWithTheirFiredPieces(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	enc := uuid.NewString()
	for _, k := range []string{boltKey, arrowKey} {
		if _, err := et.setAmmo(t, pc, k, 1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := et.shoot(t, pc, boltKey, enc); err != nil { // 0 pieces left, 1 fired: still listed
		t.Fatal(err)
	}
	got := et.read(t, et.master, pc.GetId()).GetSheet().GetFull().GetAmmunition()
	if len(got) != 2 || got[0].GetKey() != arrowKey || got[1].GetKey() != boltKey || got[1].GetQuantity() != 0 || got[1].GetSpentLastCombat() != 1 || got[1].GetRecovered() {
		t.Errorf("ammunition: %+v", got)
	}
}

func TestACountedKindAtZeroIsListedOnTheSheet(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	if got := et.read(t, et.master, pc.GetId()).GetSheet().GetFull().GetAmmunition(); len(got) != 0 {
		t.Fatalf("a character with no count lists %+v", got)
	}
	if _, err := et.setAmmo(t, pc, arrowKey, 0); err != nil {
		t.Fatal(err)
	}
	got := et.read(t, et.master, pc.GetId()).GetSheet().GetFull().GetAmmunition()
	if len(got) != 1 || got[0].GetKey() != arrowKey || got[0].GetQuantity() != 0 || got[0].GetSpentLastCombat() != 0 {
		t.Errorf("a count of 0 is listed as %+v, want the arrows at 0", got)
	}
}

func TestSaveIgnoresAndKeepsTheAmmunition(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	if _, err := et.setAmmo(t, pc, arrowKey, 12); err != nil {
		t.Fatal(err)
	}
	req := editorSaveOf(et.read(t, et.master, pc.GetId()))
	req.Sheet = editorClone(req.GetSheet(), func(f *charactersv1.FullSheet) {
		f.Ammunition = []*charactersv1.SheetAmmunition{{Key: arrowKey, Quantity: 12}, {Key: boltKey, Quantity: 7}}
	})
	saved, err := editorUpdate(et.master, req)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := saved.GetSheet().GetFull().GetAmmunition(); len(got) != 1 || got[0].GetKey() != arrowKey || got[0].GetQuantity() != 12 {
		t.Errorf("after the save: %+v", got)
	}
	// The stored document never holds it either.
	var doc string
	if err := et.h.pool.QueryRow(t.Context(), "SELECT sheet::text FROM characters WHERE id = $1", pc.GetId()).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	// The bolts only came with the request: a document that holds the key or the field kept what the save sent.
	if strings.Contains(doc, "ammunition") || strings.Contains(doc, boltKey) {
		t.Errorf("the document holds ammunition: %s", doc)
	}
}

func TestSpendAmmunition(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	encA, encB := uuid.NewString(), uuid.NewString()
	if left, counted, err := et.shootCounted(t, pc, arrowKey, encA); err != nil || counted || left != 0 {
		t.Fatalf("a kind with no count: %d %v %v, want 0, not counted, no refusal", left, counted, err)
	}
	if got := et.read(t, et.master, pc.GetId()).GetSheet().GetFull().GetAmmunition(); len(got) != 0 {
		t.Fatalf("the shot of an uncounted kind wrote a row: %+v", got)
	}
	if _, err := et.setAmmo(t, pc, arrowKey, 3); err != nil {
		t.Fatal(err)
	}
	for want := int32(2); want >= 1; want-- {
		if left, err := et.shoot(t, pc, arrowKey, encA); err != nil || left != want {
			t.Fatalf("shot: %d %v, want %d", left, err, want)
		}
	}
	if a := ammoOf(et.read(t, et.master, pc.GetId()), arrowKey); a.GetQuantity() != 1 || a.GetSpentLastCombat() != 2 {
		t.Errorf("after two shots: %+v", a)
	}
	// Another combat starts the count over.
	if left, err := et.shoot(t, pc, arrowKey, encB); err != nil || left != 0 {
		t.Fatalf("shot in B: %d %v", left, err)
	}
	if a := ammoOf(et.read(t, et.master, pc.GetId()), arrowKey); a.GetSpentLastCombat() != 1 || a.GetRecovered() {
		t.Errorf("after the new combat: %+v", a)
	}
	// None left: refused, nothing written.
	if _, err := et.shoot(t, pc, arrowKey, encB); !errors.Is(err, ErrNoAmmunition) {
		t.Fatalf("empty: %v", err)
	}
	if a := ammoOf(et.read(t, et.master, pc.GetId()), arrowKey); a.GetSpentLastCombat() != 1 || a.GetQuantity() != 0 {
		t.Errorf("after the refusal: %+v", a)
	}
	for _, key := range []string{"equipment:longbow", ""} { // not a kind of ammunition: refused, not "none left"
		if _, err := et.shoot(t, pc, key, encA); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("spend of %q: %v", key, err)
		}
	}
	if _, err := et.shoot(t, pc, arrowKey, "nope"); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a bad encounter id: %v", err)
	}
	if _, err := et.shoot(t, &charactersv1.Character{Id: uuid.NewString()}, arrowKey, encA); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("another character: %v", err)
	}
}

func (et *editorTable) refund(t *testing.T, pc *charactersv1.Character, key, encounter string, pieces int32) (int32, error) {
	t.Helper()
	var left int32
	err := et.inTx(t, func(tx pgx.Tx) error {
		var err error
		left, err = et.h.svc.RefundAmmunition(t.Context(), tx, et.campaign, pc.GetId(), key, encounter, pieces)
		return err
	})
	return left, err
}

// A refund is the undo of a shot: the piece is held again and the combat's count of what was fired goes down by it.
func TestRefundAmmunition(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	enc, other := uuid.NewString(), uuid.NewString()
	if _, err := et.setAmmo(t, pc, arrowKey, 10); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := et.shoot(t, pc, arrowKey, enc); err != nil {
			t.Fatal(err)
		}
	}
	if left, err := et.refund(t, pc, arrowKey, enc, 1); err != nil || left != 8 {
		t.Fatalf("refund: %d %v, want 8", left, err)
	}
	if a := ammoOf(et.read(t, et.master, pc.GetId()), arrowKey); a.GetQuantity() != 8 || a.GetSpentLastCombat() != 2 || a.GetRecovered() {
		t.Errorf("after the refund: %+v, want 8 held, 2 fired", a)
	}
	// The recovery counts the two that stay fired.
	got, err := et.recover(t, pc, enc)
	if err != nil || len(got) != 1 || got[0].GetQuantity() != 9 {
		t.Fatalf("recover: %+v %v, want 9 held (8 and half of 2)", got, err)
	}
	// A refund in another combat than the one the row counts only gives the piece back.
	if left, err := et.refund(t, pc, arrowKey, other, 1); err != nil || left != 10 {
		t.Fatalf("refund in another combat: %d %v, want 10", left, err)
	}
	if a := ammoOf(et.read(t, et.master, pc.GetId()), arrowKey); a.GetSpentLastCombat() != 2 || !a.GetRecovered() {
		t.Errorf("the count of the other combat changed: %+v", a)
	}
}

// The undo of the only shot of a combat leaves the row as if nothing was fired: nothing to recover, no count.
func TestRefundOfTheOnlyShotLeavesNothingFired(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	enc := uuid.NewString()
	if _, err := et.setAmmo(t, pc, arrowKey, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := et.shoot(t, pc, arrowKey, enc); err != nil {
		t.Fatal(err)
	}
	if left, err := et.refund(t, pc, arrowKey, enc, 1); err != nil || left != 5 {
		t.Fatalf("refund: %d %v, want 5", left, err)
	}
	if a := ammoOf(et.read(t, et.master, pc.GetId()), arrowKey); a.GetQuantity() != 5 || a.GetSpentLastCombat() != 0 || a.GetRecovered() {
		t.Errorf("after the refund: %+v", a)
	}
	if _, err := et.recover(t, pc, enc); !errors.Is(err, ErrNothingToRecover) {
		t.Errorf("recover after an undone only shot: %v, want nothing to recover", err)
	}
}

func TestRefundAmmunitionEdges(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	enc := uuid.NewString()
	// A kind the character holds no row of gets the pieces.
	if left, err := et.refund(t, pc, boltKey, enc, 2); err != nil || left != 2 {
		t.Errorf("refund with no row: %d %v, want 2", left, err)
	}
	// The count never goes below 0 and the stock never over the maximum.
	if _, err := et.setAmmo(t, pc, arrowKey, MaxAmmunition); err != nil {
		t.Fatal(err)
	}
	if left, err := et.refund(t, pc, arrowKey, enc, 5); err != nil || left != MaxAmmunition {
		t.Errorf("refund over the maximum: %d %v, want %d", left, err, MaxAmmunition)
	}
	if a := ammoOf(et.read(t, et.master, pc.GetId()), arrowKey); a.GetSpentLastCombat() != 0 {
		t.Errorf("fired count after a refund of nothing fired: %+v", a)
	}
	for _, c := range []struct {
		name, key, enc string
		pieces         int32
	}{
		{"not ammunition", "equipment:longbow", enc, 1},
		{"bad encounter", arrowKey, "nope", 1},
		{"no piece", arrowKey, enc, 0},
	} {
		if _, err := et.refund(t, pc, c.key, c.enc, c.pieces); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if _, err := et.refund(t, &charactersv1.Character{Id: uuid.NewString()}, arrowKey, enc, 1); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("another character: %v", err)
	}
}

func TestAmmunitionLeft(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	left := func(key string) (n int32, counted bool) {
		if err := et.inTx(t, func(tx pgx.Tx) error {
			var err error
			n, counted, err = et.h.svc.AmmunitionLeft(t.Context(), tx, et.campaign, pc.GetId(), key)
			return err
		}); err != nil {
			t.Fatalf("AmmunitionLeft: %v", err)
		}
		return n, counted
	}
	if n, counted := left(arrowKey); n != 0 || counted {
		t.Errorf("no row: %d counted %v, want 0 and not counted", n, counted)
	}
	if _, err := et.setAmmo(t, pc, arrowKey, 9); err != nil {
		t.Fatal(err)
	}
	if n, counted := left(arrowKey); n != 9 || !counted {
		t.Errorf("held: %d counted %v", n, counted)
	}
	if _, err := et.setAmmo(t, pc, arrowKey, 0); err != nil {
		t.Fatal(err)
	}
	if n, counted := left(arrowKey); n != 0 || !counted {
		t.Errorf("a count of 0: %d counted %v, want 0 and counted", n, counted)
	}
	// The campaign is part of the question: the character of another campaign is not found.
	err := et.inTx(t, func(tx pgx.Tx) error {
		_, _, err := et.h.svc.AmmunitionLeft(t.Context(), tx, uuid.NewString(), pc.GetId(), arrowKey)
		return err
	})
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("another campaign: %v", err)
	}
}

func TestRecoverAmmunition(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	enc, other := uuid.NewString(), uuid.NewString()
	if _, err := et.recover(t, pc, enc); !errors.Is(err, ErrNothingToRecover) {
		t.Fatalf("nothing fired: %v", err)
	}
	for k, n := range map[string]int32{arrowKey: 10, boltKey: 10} {
		if _, err := et.setAmmo(t, pc, k, n); err != nil {
			t.Fatal(err)
		}
	}
	for range 5 {
		if _, err := et.shoot(t, pc, arrowKey, enc); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := et.shoot(t, pc, boltKey, enc); err != nil { // one piece: nothing comes back, still marked
		t.Fatal(err)
	}
	if _, err := et.recover(t, pc, other); !errors.Is(err, ErrNothingToRecover) {
		t.Fatalf("another combat: %v", err)
	}
	got, err := et.recover(t, pc, enc)
	if err != nil || len(got) != 2 {
		t.Fatalf("recover: %+v %v", got, err)
	}
	read := et.read(t, et.master, pc.GetId())
	if a := ammoOf(read, arrowKey); a.GetQuantity() != 7 || !a.GetRecovered() || a.GetSpentLastCombat() != 5 {
		t.Errorf("arrows: %+v", a)
	}
	if a := ammoOf(read, boltKey); a.GetQuantity() != 9 || !a.GetRecovered() {
		t.Errorf("bolts: %+v", a)
	}
	if _, err := et.recover(t, pc, enc); !errors.Is(err, ErrAmmunitionRecovered) {
		t.Errorf("twice: %v", err)
	}
	if a := ammoOf(et.read(t, et.master, pc.GetId()), arrowKey); a.GetQuantity() != 7 {
		t.Errorf("the second try gave pieces: %+v", a)
	}
}

func TestAmmunitionGoesWithTheCharacter(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	if _, err := et.setAmmo(t, pc, arrowKey, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := et.h.pool.Exec(t.Context(), "DELETE FROM characters WHERE id = $1", pc.GetId()); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := et.h.pool.QueryRow(t.Context(), "SELECT count(*) FROM character_ammunition WHERE character_id = $1", pc.GetId()).Scan(&n); err != nil || n != 0 {
		t.Errorf("rows left: %d %v", n, err)
	}
}

// Two shots at the last piece in parallel transactions: exactly one fires. The first transaction takes the row's lock
// and holds it until the second one has started, so the shots meet on the lock and the first always wins.
func TestTwoShotsAtTheLastPieceFireOnce(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	if _, err := et.setAmmo(t, pc, arrowKey, 1); err != nil {
		t.Fatal(err)
	}
	enc := uuid.NewString()
	first, second := et.raceTwo(t,
		func(tx pgx.Tx) error {
			_, _, err := et.h.svc.SpendAmmunition(t.Context(), tx, et.campaign, pc.GetId(), arrowKey, enc)
			return err
		},
		func(tx pgx.Tx) error {
			_, _, err := et.h.svc.SpendAmmunition(t.Context(), tx, et.campaign, pc.GetId(), arrowKey, enc)
			return err
		})
	if first != nil || !errors.Is(second, ErrNoAmmunition) {
		t.Errorf("first %v, second %v", first, second)
	}
	if a := ammoOf(et.read(t, et.master, pc.GetId()), arrowKey); a.GetQuantity() != 0 || a.GetSpentLastCombat() != 1 {
		t.Errorf("state: %+v", a)
	}
}

// raceTwo runs two transactions on the same lock, the first one ahead. The first runs its work, which takes the lock,
// and then waits, still open, until the second one is about to run its own work; the second starts only once the first
// holds the lock. So the first always wins and the second meets either its lock or its commit, never a state before
// it. It returns what each work returned.
func (et *editorTable) raceTwo(t *testing.T, firstWork, secondWork func(tx pgx.Tx) error) (first, second error) {
	t.Helper()
	var lockedOnce, startedOnce sync.Once
	locked, started := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		first = et.inTx(t, func(tx pgx.Tx) error {
			err := firstWork(tx)
			lockedOnce.Do(func() { close(locked) })
			<-started
			return err
		})
	})
	wg.Go(func() {
		<-locked
		startedOnce.Do(func() { close(started) })
		second = et.inTx(t, secondWork)
	})
	wg.Wait()
	return first, second
}

// The ammunition of a character reaches the master and its player only. Another player's reads find no character
// (not_found), which is the whole guarantee for them; the positive control below proves the data is there on that
// same read path, and the canary in the character's name must be in no answer another reader gets.
func TestAnotherPlayerNeverReadsTheAmmunition(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	if _, err := et.setAmmo(t, pc, arrowKey, 4); err != nil {
		t.Fatal(err)
	}
	pc = et.renamed(t, pc, leakCanary)
	for _, reader := range []*user{et.master, et.dona} {
		read := et.read(t, reader, pc.GetId())
		if a := ammoOf(read, arrowKey); a.GetQuantity() != 4 || read.GetName() != leakCanary {
			t.Fatalf("the owner and the master read the ammunition: %+v", read.GetSheet().GetFull().GetAmmunition())
		}
	}
	et.requireHiddenFromOthers(t, pc, arrowKey)
}

// The infusions of a sheet are the server's, like the ammunition: a save that carries some changes nothing, and the
// stored document never holds them.
func TestSaveIgnoresTheInfusionsOfASheet(t *testing.T) {
	t.Parallel()
	et, pc := attuneTable(t)
	req := editorSaveOf(et.read(t, et.master, pc.GetId()))
	req.Sheet = editorClone(req.GetSheet(), func(f *charactersv1.FullSheet) {
		f.Infusions = []*charactersv1.SheetInfusion{{Id: "inf-1", InfusionKey: "feature:forjada@mesa", NamePt: "Fio forjado", HolderCharacterId: pc.GetId(), ItemKey: "weapon:longsword", Bonus: 2}}
	})
	saved, err := editorUpdate(et.master, req)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := saved.GetSheet().GetFull().GetInfusions(); len(got) != 0 {
		t.Errorf("after the save: %+v", got)
	}
	var doc string
	if err := et.h.pool.QueryRow(t.Context(), "SELECT sheet::text FROM characters WHERE id = $1", pc.GetId()).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc, "infusions") || strings.Contains(doc, "Fio forjado") {
		t.Errorf("the document holds infusions: %s", doc)
	}
}

// A new sheet that carries infusions stores none: the create path ignores them as a save does.
func TestCreateIgnoresTheInfusionsOfASheet(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	sheet := editorFighter()
	sheet.GetFull().Infusions = []*charactersv1.SheetInfusion{{Id: "inf-1", InfusionKey: "feature:forjada@mesa", NamePt: "Fio forjado", ItemKey: "weapon:longsword", Bonus: 2}}
	created, err := editorCreate(et.dona, &charactersv1.CreateCharacterRequest{
		CampaignId: et.campaign, Kind: editorPlayer, Name: "Bruna", Sheet: sheet,
		AbilityMethod: charactersv1.AbilityMethod_ABILITY_METHOD_STANDARD_ARRAY,
	})
	if err != nil {
		t.Fatalf("CreateCharacter error = %v", err)
	}
	if got := created.GetSheet().GetFull().GetInfusions(); len(got) != 0 {
		t.Errorf("the created sheet carries %+v", got)
	}
	var doc string
	if err := et.h.pool.QueryRow(t.Context(), "SELECT sheet::text FROM characters WHERE id = $1", created.GetId()).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc, "infusions") || strings.Contains(doc, "Fio forjado") {
		t.Errorf("the document holds infusions: %s", doc)
	}
}
