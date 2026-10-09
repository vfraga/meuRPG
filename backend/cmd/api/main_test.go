package main

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// TestConnectGETOnlyForRequestsWithoutData enforces a rule of
// docs/architecture.md#api-contracts-protobuf: NO_SIDE_EFFECTS, which lets a
// client call a method with HTTP GET, is only for requests with no fields.
// A GET puts the whole request in the URL, and URLs end up in the
// platform's request logs, so an ID or a personal detail must never ride
// there. A read whose request has fields is IDEMPOTENT instead, and stays
// POST-only.
//
// Every service this command serves is linked into this test, so their
// descriptors are all in protoregistry.GlobalFiles.
func TestConnectGETOnlyForRequestsWithoutData(t *testing.T) {
	t.Parallel()
	services := map[string]bool{}
	protoregistry.GlobalFiles.RangeFiles(func(file protoreflect.FileDescriptor) bool {
		if !strings.HasPrefix(string(file.Package()), "meurpg.") {
			return true
		}
		for i := range file.Services().Len() {
			service := file.Services().Get(i)
			services[string(service.FullName())] = true
			for j := range service.Methods().Len() {
				method := service.Methods().Get(j)
				opts, _ := method.Options().(*descriptorpb.MethodOptions)
				if opts.GetIdempotencyLevel() == descriptorpb.MethodOptions_NO_SIDE_EFFECTS && method.Input().Fields().Len() > 0 {
					t.Errorf("%s is NO_SIDE_EFFECTS (HTTP GET) but %s has fields; make it IDEMPOTENT (POST-only)",
						method.FullName(), method.Input().FullName())
				}
			}
		}
		return true
	})
	for _, want := range []string{
		"meurpg.system.v1.SystemService",
		"meurpg.identity.v1.IdentityService",
		"meurpg.campaigns.v1.CampaignService",
		"meurpg.campaigns.v1.CampaignDocumentService",
		"meurpg.characters.v1.CharacterService",
		"meurpg.rules.v1.ContentService",
		"meurpg.rules.v1.TableContentService",
		"meurpg.play.v1.PlayService",
		"meurpg.play.v1.PuzzleService",
		"meurpg.play.v1.EncounterService",
		"meurpg.play.v1.CreatureService",
		"meurpg.maps.v1.GalleryService",
		"meurpg.maps.v1.MapService",
		"meurpg.maps.v1.ImageGenerationService",
		"meurpg.maps.v1.TreasureService",
	} {
		if !services[want] {
			t.Errorf("%s was not checked; is it still linked into cmd/api?", want)
		}
	}
}
