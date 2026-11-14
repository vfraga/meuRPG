package maps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// TreasureService (MR-044, RN-09, RN-10): the master's
// treasure generator. The generator itself is package rules (pure: the same mode,
// party level and seed give the same treasure); this file is what the server does
// around it: the party's level, the answers, and "Pôr no mapa", which makes a hidden
// TREASURE point through the same table a point made by hand uses.
//
// Every method is the master's, and a player gets `not_found` from each of them
// (requireDungeonMaster): a generated treasure never reaches a player before they
// find the point (RN-10). Nothing is stored but the point.

var _ mapsv1connect.TreasureServiceHandler = (*Service)(nil)

// valueLabel is what the app writes next to a magic item's value: the table is the
// 2024 rules'.
const valueLabel = "Valores do SRD 5.2.1 (regras de 2024)"

var (
	modeToRules = map[mapsv1.TreasureMode]string{
		mapsv1.TreasureMode_TREASURE_MODE_INDIVIDUAL: rules.TreasureIndividual,
		mapsv1.TreasureMode_TREASURE_MODE_HOARD:      rules.TreasureHoard,
	}
	modeFromRules = map[string]mapsv1.TreasureMode{
		rules.TreasureIndividual: mapsv1.TreasureMode_TREASURE_MODE_INDIVIDUAL,
		rules.TreasureHoard:      mapsv1.TreasureMode_TREASURE_MODE_HOARD,
	}
	coinFromRules = map[string]mapsv1.TreasureCoinKind{
		rules.CoinCopper:   mapsv1.TreasureCoinKind_TREASURE_COIN_KIND_COPPER,
		rules.CoinSilver:   mapsv1.TreasureCoinKind_TREASURE_COIN_KIND_SILVER,
		rules.CoinElectrum: mapsv1.TreasureCoinKind_TREASURE_COIN_KIND_ELECTRUM,
		rules.CoinGold:     mapsv1.TreasureCoinKind_TREASURE_COIN_KIND_GOLD,
		rules.CoinPlatinum: mapsv1.TreasureCoinKind_TREASURE_COIN_KIND_PLATINUM,
	}
	rarityFromRules = map[string]mapsv1.MagicItemRarity{
		rules.RarityCommon:    mapsv1.MagicItemRarity_MAGIC_ITEM_RARITY_COMMON,
		rules.RarityUncommon:  mapsv1.MagicItemRarity_MAGIC_ITEM_RARITY_UNCOMMON,
		rules.RarityRare:      mapsv1.MagicItemRarity_MAGIC_ITEM_RARITY_RARE,
		rules.RarityVeryRare:  mapsv1.MagicItemRarity_MAGIC_ITEM_RARITY_VERY_RARE,
		rules.RarityLegendary: mapsv1.MagicItemRarity_MAGIC_ITEM_RARITY_LEGENDARY,
		rules.RarityArtifact:  mapsv1.MagicItemRarity_MAGIC_ITEM_RARITY_ARTIFACT,
		rules.RarityVaries:    mapsv1.MagicItemRarity_MAGIC_ITEM_RARITY_VARIES,
	}
	// rarityPT is how a rarity reads in a point's description.
	rarityPT = map[string]string{
		rules.RarityCommon: "comum", rules.RarityUncommon: "incomum", rules.RarityRare: "raro",
		rules.RarityVeryRare: "muito raro", rules.RarityLegendary: "lendário", rules.RarityArtifact: "artefato",
	}
	// coinPT is the abbreviation of each coin; a PP is a "peça de prata".
	coinPT = map[string]string{
		rules.CoinCopper: "PC", rules.CoinSilver: "PP", rules.CoinElectrum: "PE", rules.CoinGold: "PO", rules.CoinPlatinum: "PL",
	}
)

func errNoParty() error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New("the campaign has no living player character: give the party level"))
	if detail, detailErr := connect.NewErrorDetail(&mapsv1.TreasureBlocked{Reason: mapsv1.TreasureBlockedReason_TREASURE_BLOCKED_REASON_NO_PARTY}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func errContentChanged() error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New("the rules content changed since the treasure was generated: generate it again"))
	if detail, detailErr := connect.NewErrorDetail(&mapsv1.TreasureBlocked{Reason: mapsv1.TreasureBlockedReason_TREASURE_BLOCKED_REASON_CONTENT_CHANGED}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func errKeyUsedForAnotherChange() error {
	return badTreasure("idempotency_key", "was used for another change")
}

// badTreasure is `invalid_argument` with the TreasureInvalidField detail: it names the
// request field that breaks a rule, never the value.
func badTreasure(field, format string, a ...any) error {
	return withTreasureField(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(field+" "+format, a...)), field)
}

// withTreasureField names the request field on an `invalid_argument` a shared check made.
func withTreasureField(err error, field string) error {
	var ce *connect.Error
	if errors.As(err, &ce) && ce.Code() == connect.CodeInvalidArgument {
		if detail, detailErr := connect.NewErrorDetail(&mapsv1.TreasureInvalidField{Field: field}); detailErr == nil {
			ce.AddDetail(detail)
		}
	}
	return err
}

// GetTreasureParty implements mapsv1connect.TreasureServiceHandler.
func (s *Service) GetTreasureParty(
	ctx context.Context,
	req *connect.Request[mapsv1.GetTreasurePartyRequest],
) (*connect.Response[mapsv1.GetTreasurePartyResponse], error) {
	m, err := requireDungeonMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	levels, err := s.characters.PartyTotalLevels(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read the party's levels", err)
	}
	out := &mapsv1.GetTreasurePartyResponse{LivingCount: int32(len(levels))} //nolint:gosec // G115: a handful of characters
	if len(levels) > 0 {
		out.LowestLevel = int32(slices.Min(levels))  //nolint:gosec // G115: 1 to 20
		out.HighestLevel = int32(slices.Max(levels)) //nolint:gosec // G115: 1 to 20
	}
	return connect.NewResponse(out), nil
}

// GenerateTreasure implements mapsv1connect.TreasureServiceHandler.
func (s *Service) GenerateTreasure(
	ctx context.Context,
	req *connect.Request[mapsv1.GenerateTreasureRequest],
) (*connect.Response[mapsv1.GenerateTreasureResponse], error) {
	m, err := requireDungeonMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	mode, err := treasureMode(req.Msg.GetMode())
	if err != nil {
		return nil, err
	}
	level, err := s.partyLevel(ctx, m.CampaignID, req.Msg.PartyLevel)
	if err != nil {
		return nil, err
	}
	seed, err := seedOf(req.Msg.Seed)
	if err != nil {
		return nil, s.dbError(ctx, "draw a seed", err)
	}
	t, err := s.rules.GenerateTreasure(mode, level, seed)
	if err != nil {
		return nil, badTreasure("party_level", "must be 1 to 20")
	}
	return connect.NewResponse(&mapsv1.GenerateTreasureResponse{Treasure: treasureToProto(t)}), nil
}

func treasureMode(m mapsv1.TreasureMode) (string, error) {
	mode, ok := modeToRules[m]
	if !ok {
		return "", badTreasure("mode", "must be INDIVIDUAL or HOARD")
	}
	return mode, nil
}

// partyLevel is the level the tables are read for: the one the master gave (1 to
// 20), or the lowest level among the living player characters.
func (s *Service) partyLevel(ctx context.Context, campaignID string, given *int32) (int, error) {
	if given != nil {
		if *given < rules.MinTreasureLevel || *given > rules.MaxTreasureLevel {
			return 0, badTreasure("party_level", "must be %d to %d", rules.MinTreasureLevel, rules.MaxTreasureLevel)
		}
		return int(*given), nil
	}
	levels, err := s.characters.PartyTotalLevels(ctx, nil, campaignID)
	if err != nil {
		return 0, s.dbError(ctx, "read the party's levels", err)
	}
	if len(levels) == 0 {
		return 0, errNoParty()
	}
	return max(slices.Min(levels), rules.MinTreasureLevel), nil
}

// GetMagicItem implements mapsv1connect.TreasureServiceHandler.
func (s *Service) GetMagicItem(
	ctx context.Context,
	req *connect.Request[mapsv1.GetMagicItemRequest],
) (*connect.Response[mapsv1.GetMagicItemResponse], error) {
	if _, err := requireDungeonMaster(ctx, req.Msg.GetCampaignId()); err != nil {
		return nil, err
	}
	item, ok := s.rules.MagicItem(req.Msg.GetKey())
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("magic item not found"))
	}
	out := &mapsv1.GetMagicItemResponse{
		Key: item.Key, Name: item.Name, NamePt: item.NamePT, Category: item.Category,
		Rarity: rarityFromRules[item.Rarity], Attunement: item.Attunement,
		AttunementBy: item.AttunementBy, AttunementByPt: item.AttunementByPT,
		Consumable: item.Consumable, SpellScroll: item.SpellScroll, ValueLabel: valueLabel,
		Description: item.Desc, DescriptionPt: item.DescPT, DescriptionPtMissing: item.DescPTMissing(), Variants: item.Variants, VariantOf: item.VariantOf,
	}
	if v, ok := s.rules.MagicItemValue(item.Key); ok {
		out.Priceless, out.Halved = v.Priceless, v.Halved
		if !v.Priceless {
			po := int32(v.PO) //nolint:gosec // G115: at most 200,000
			out.ValuePo = &po
		}
	}
	return connect.NewResponse(out), nil
}

// PlaceTreasure implements mapsv1connect.TreasureServiceHandler.
func (s *Service) PlaceTreasure(
	ctx context.Context,
	req *connect.Request[mapsv1.PlaceTreasureRequest],
) (*connect.Response[mapsv1.PlaceTreasureResponse], error) {
	m, err := requireDungeonMaster(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	plan, err := s.planTreasure(req.Msg)
	if err != nil {
		return nil, err
	}
	current, err := s.currentMap(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	// The key is unique in the campaign, and it is kept with a hash of the whole request:
	// a retry returns the first point only when it is the same request.
	scopedKey := m.CampaignID + ":" + plan.key
	requestHash := hashTreasureRequest(plan.mapID, plan.column, plan.row, plan.mode, plan.level, plan.seed, plan.name, plan.version)

	var placed treasurePlacement
	err = s.inSession(ctx, m.CampaignID, func(tx pgx.Tx, t sessiontell.Tell) error {
		var err error
		if placed, err = s.placeTreasureTx(ctx, s.queries.WithTx(tx), m.CampaignID, plan, scopedKey, requestHash); err != nil {
			return err
		}
		if placed.replayed {
			t.Replay() // the first call may have died before it told: the notice goes out again, with the number as it stands
		}
		// A new point is hidden: only the master hears about it (MR-009).
		s.announce(t, func(ctx context.Context) {
			s.publishPointsChanged(ctx, m.CampaignID, placed.mapRow, playersSee(plan.mapID, placed.mapRow.RevealedAt, current) && everyoneSees(placed.point), placed.point)
		})
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "place a treasure", err)
	}
	out, err := s.masterPoint(ctx, m, placed.point)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&mapsv1.PlaceTreasureResponse{Point: out, Treasure: treasureToProto(plan.treasure)}), nil
}

// treasurePlan is a PlaceTreasure request after its checks: the inputs, the treasure the
// server rolled from them and the description the point will carry.
type treasurePlan struct {
	mapID, mode, key, name, version string
	level                           int32
	column, row                     int32
	seed                            uint64
	treasure                        rules.Treasure
	description                     string
}

// treasurePlacement is what one attempt of the PlaceTreasure transaction ends with.
type treasurePlacement struct {
	point    mapsdb.MapPoint
	mapRow   mapsdb.Map
	replayed bool
}

// planTreasure checks a PlaceTreasure request, field by field in a fixed order (the first
// fault is the one the caller reads), and rolls the treasure again.
func (s *Service) planTreasure(msg *mapsv1.PlaceTreasureRequest) (treasurePlan, error) {
	plan, err := s.treasureInputs(msg)
	if err != nil {
		return treasurePlan{}, err
	}
	// The server rolls the treasure again: what the app computed or showed is never
	// trusted, only the three inputs.
	plan.treasure, err = s.rules.GenerateTreasure(plan.mode, int(plan.level), plan.seed)
	if err != nil {
		return treasurePlan{}, badTreasure("party_level", "must be 1 to 20")
	}
	plan.description, err = cleanDescription(treasureDescription(plan.treasure))
	if err != nil {
		return treasurePlan{}, err
	}
	return plan, nil
}

// treasureInputs is the part of a PlaceTreasure request the app sends: the map, the mode,
// the level, the seed, the key, the content version and the name.
func (s *Service) treasureInputs(msg *mapsv1.PlaceTreasureRequest) (treasurePlan, error) {
	mapID, ok := uid.Canonical(msg.GetMapId())
	if !ok {
		return treasurePlan{}, errMapNotFound()
	}
	mode, err := treasureMode(msg.GetMode())
	if err != nil {
		return treasurePlan{}, err
	}
	level := msg.GetPartyLevel()
	if level < rules.MinTreasureLevel || level > rules.MaxTreasureLevel {
		return treasurePlan{}, badTreasure("party_level", "must be %d to %d", rules.MinTreasureLevel, rules.MaxTreasureLevel)
	}
	if msg.Seed == nil {
		return treasurePlan{}, badTreasure("seed", "is required: the treasure is rolled again from it")
	}
	key, err := cleanKey(msg.GetIdempotencyKey())
	if err != nil {
		return treasurePlan{}, withTreasureField(err, "idempotency_key")
	}
	if err := s.checkTreasureVersion(msg.GetContentVersion()); err != nil {
		return treasurePlan{}, err
	}
	name, err := treasureName(msg, mode)
	if err != nil {
		return treasurePlan{}, err
	}
	return treasurePlan{
		mapID: mapID, mode: mode, key: key, name: name, version: msg.GetContentVersion(),
		level: level, column: msg.GetColumn(), row: msg.GetRow(), seed: msg.GetSeed(),
	}, nil
}

// checkTreasureVersion refuses a missing content version, and one the server no longer has.
func (s *Service) checkTreasureVersion(version string) error {
	if version == "" {
		return badTreasure("content_version", "is required: send the one the treasure came with")
	}
	if version != s.rules.Version() {
		return errContentChanged()
	}
	return nil
}

// treasureName is the name the master gave the point, or the default of the mode.
func treasureName(msg *mapsv1.PlaceTreasureRequest, mode string) (string, error) {
	if msg.Name == nil {
		return defaultTreasureName(mode), nil
	}
	name, err := cleanName("name", msg.GetName())
	if err != nil {
		return "", withTreasureField(err, "name")
	}
	return name, nil
}

// placeTreasureTx is the transaction of PlaceTreasure: the map, the replay of the key, the
// square, the cap and the insert.
func (s *Service) placeTreasureTx(ctx context.Context, q *mapsdb.Queries, campaignID string, plan treasurePlan, scopedKey, requestHash string) (treasurePlacement, error) {
	mapRow, err := s.campaignMap(ctx, q, campaignID, plan.mapID)
	if err != nil {
		return treasurePlacement{}, err
	}
	// A retry finds the point the first call made.
	prior, err := q.GetMapPointByCreateKey(ctx, &scopedKey)
	if err == nil {
		point, err := replayOf(prior, requestHash)
		return treasurePlacement{point: point, mapRow: mapRow, replayed: err == nil}, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return treasurePlacement{}, fmt.Errorf("find the point of the key: %w", err)
	}
	x, y, err := s.treasureSquareCenter(ctx, q, campaignID, plan)
	if err != nil {
		return treasurePlacement{}, err
	}
	point, replayed, err := s.insertTreasurePoint(ctx, q, plan, x, y, scopedKey, requestHash)
	return treasurePlacement{point: point, mapRow: mapRow, replayed: replayed}, err
}

// treasureSquareCenter is the middle of the square asked, in basis points of the map.
func (s *Service) treasureSquareCenter(ctx context.Context, q *mapsdb.Queries, campaignID string, plan treasurePlan) (int, int, error) {
	size, err := q.GetMapGrid(ctx, mapsdb.GetMapGridParams{CampaignID: campaignID, ID: plan.mapID})
	if err != nil {
		return 0, 0, fmt.Errorf("read the map's grid: %w", err)
	}
	g := gridOf(size.GridColumns, size.GridFactor, size.ImageWidth, size.ImageHeight)
	if !g.Valid() {
		return 0, 0, errNoGrid()
	}
	sq := grid.Square{Col: int(plan.column), Row: int(plan.row)}
	if !g.Contains(sq) {
		if sq.Col < 0 || sq.Col >= g.Columns {
			return 0, 0, badTreasure("column", "is outside the map's grid")
		}
		return 0, 0, badTreasure("row", "is outside the map's grid")
	}
	x, y := g.CenterOf(sq)
	return x, y, nil
}

// insertTreasurePoint counts the map's points against the cap and inserts the hidden
// point; when another call with the same key won the race, its point is ours.
func (s *Service) insertTreasurePoint(ctx context.Context, q *mapsdb.Queries, plan treasurePlan, x, y int, scopedKey, requestHash string) (point mapsdb.MapPoint, replayed bool, err error) {
	value := int32(plan.treasure.GoldPO) //nolint:gosec // G115: at most 1,000,000
	count, err := q.CountMapPoints(ctx, plan.mapID)
	if err != nil {
		return point, false, fmt.Errorf("count points: %w", err)
	}
	if count >= s.maxPoints {
		return point, false, connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("the map already has %d points", s.maxPoints))
	}
	point, err = q.InsertTreasurePoint(ctx, mapsdb.InsertTreasurePointParams{
		MapID: plan.mapID, Name: plan.name, Description: plan.description, XBp: int32(x), YBp: int32(y), //nolint:gosec // G115: 0 to 10000
		TreasureValuePo: &value, CreateKey: &scopedKey, CreateHash: &requestHash, Now: s.now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Another call with the same key won the race: its point is ours.
		prior, err := q.GetMapPointByCreateKey(ctx, &scopedKey)
		if err != nil {
			return point, false, fmt.Errorf("find the point of the key: %w", err)
		}
		point, err = replayOf(prior, requestHash)
		return point, err == nil, err
	}
	if err != nil {
		return point, false, fmt.Errorf("insert point: %w", err)
	}
	return point, false, nil
}

// replayOf is the point an earlier call with the same key made, once it is the same
// request (the same hash): a key reused for another change is a mistake of the app.
func replayOf(prior mapsdb.MapPoint, hash string) (mapsdb.MapPoint, error) {
	if prior.CreateHash == nil || *prior.CreateHash != hash {
		return mapsdb.MapPoint{}, errKeyUsedForAnotherChange()
	}
	return prior, nil
}

// hashTreasureRequest is the hash of everything that makes a PlaceTreasure what it is.
func hashTreasureRequest(mapID string, column, row int32, mode string, level int32, seed uint64, name, version string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d\x00%s\x00%d\x00%d\x00%s\x00%s", mapID, column, row, mode, level, seed, name, version)))
	return hex.EncodeToString(sum[:])
}

func defaultTreasureName(mode string) string {
	if mode == rules.TreasureHoard {
		return "Tesouro de covil"
	}
	return "Tesouro individual"
}

// treasureDescription is what the players read when they find the point: the coins,
// the gems, the art objects and the magic items, in Portuguese, one thing to a line. The
// values of the items are left out (they are the master's number), identical items are
// grouped ("2 × Pergaminho de magia (1º nível)"), and the text always fits the 2,000
// characters of a description: the generator never gives more than 12 gems, 6 art
// objects and 6 items.
func treasureDescription(t rules.Treasure) string {
	var lines []string
	lines = append(lines, coinsLines(t.Coins)...)
	lines = append(lines, piecesLines("Gemas", t.Gems)...)
	lines = append(lines, piecesLines("Obras de arte", t.Art)...)
	lines = append(lines, itemsLines(t.Items)...)
	return strings.Join(lines, "\n")
}

// coinsLines is the one line of coins, "Moedas: 1.200 PP e 340 PO.", or none.
func coinsLines(coins []rules.TreasureCoin) []string {
	if len(coins) == 0 {
		return nil
	}
	parts := make([]string, len(coins))
	for i, c := range coins {
		parts[i] = ptInt(c.Count) + " " + coinPT[c.Coin]
	}
	return []string{"Moedas: " + joinPT(parts) + "."}
}

// piecesLines is a title and a line for each gem or art object, or none without any.
func piecesLines(title string, list []rules.TreasurePiece) []string {
	if len(list) == 0 {
		return nil
	}
	lines := []string{title + ":"}
	for _, p := range list {
		if p.Count == 1 {
			lines = append(lines, fmt.Sprintf("- %s (%s PO)", p.NamePT, ptInt(p.ValuePO)))
		} else {
			lines = append(lines, fmt.Sprintf("- %d × %s (%s PO cada)", p.Count, p.NamePT, ptInt(p.ValuePO)))
		}
	}
	return lines
}

// itemsLines is "Itens mágicos:" and a line for each different item, in the order the
// generator gave them, with the count of the identical ones and the note of the rarity.
func itemsLines(items []rules.TreasureItem) []string {
	if len(items) == 0 {
		return nil
	}
	lines := []string{"Itens mágicos:"}
	var order []string
	count := map[string]int{}
	first := map[string]rules.TreasureItem{}
	for _, it := range items {
		if count[it.Key] == 0 {
			order = append(order, it.Key)
			first[it.Key] = it
		}
		count[it.Key]++
	}
	for _, key := range order {
		lines = append(lines, itemLine(first[key], count[key]))
	}
	return lines
}

// itemLine is one magic item's line: "- 2 × Name (item rare, exige sintonização)".
func itemLine(it rules.TreasureItem, count int) string {
	note := "item " + rarityPT[it.Rarity]
	if it.Attunement {
		note += ", exige sintonização"
	}
	label := it.NamePT
	if count > 1 {
		label = fmt.Sprintf("%d × %s", count, it.NamePT)
	}
	return "- " + label + " (" + note + ")"
}

// joinPT joins "a", "b" and "c" the Portuguese way: "a, b e c".
func joinPT(parts []string) string {
	if len(parts) < 2 {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " e " + parts[len(parts)-1]
}

// ptInt writes a whole number the Brazilian way: 1.200.
func ptInt(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + ptInt(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return s
}

func treasureToProto(t rules.Treasure) *mapsv1.Treasure {
	out := &mapsv1.Treasure{
		Mode: modeFromRules[t.Mode], PartyLevel: int32(t.Level), Seed: t.Seed, ContentVersion: t.ContentVersion, //nolint:gosec // G115: 1 to 20
		CoinsPo: int32(t.CoinsPO), GemsPo: int32(t.GemsPO), ArtPo: int32(t.ArtPO), //nolint:gosec // G115: bounded by the tables (1,000,000)
		GoldPo: int32(t.GoldPO), ItemsPo: int32(t.ItemsPO), //nolint:gosec // G115: the same
	}
	for _, c := range t.Coins {
		out.Coins = append(out.Coins, &mapsv1.TreasureCoinStack{Coin: coinFromRules[c.Coin], Count: int32(c.Count), ValuePo: int32(c.ValuePO)}) //nolint:gosec // G115: bounded by the tables
	}
	piece := func(in []rules.TreasurePiece) []*mapsv1.TreasurePiece {
		var list []*mapsv1.TreasurePiece
		for _, p := range in {
			list = append(list, &mapsv1.TreasurePiece{NamePt: p.NamePT, ValuePo: int32(p.ValuePO), Count: int32(p.Count)}) //nolint:gosec // G115: bounded by the tables
		}
		return list
	}
	out.Gems, out.Art = piece(t.Gems), piece(t.Art)
	for _, it := range t.Items {
		out.Items = append(out.Items, &mapsv1.TreasureItem{
			Key: it.Key, Name: it.Name, NamePt: it.NamePT, Category: it.Category, Rarity: rarityFromRules[it.Rarity],
			ValuePo: int32(it.ValuePO), Consumable: it.Consumable, Halved: it.Halved, SpellScroll: it.SpellScroll, //nolint:gosec // G115: at most 200,000
			Attunement: it.Attunement, AttunementByPt: it.AttunementByPT,
		})
	}
	return out
}
