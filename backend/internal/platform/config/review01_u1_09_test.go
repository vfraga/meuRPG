package config

import "testing"

// Finding U1-09
func TestReview1_CreatorsOnlySeparators(t *testing.T) {
	t.Parallel()
	for _, v := range []string{",", ",,", " , ,"} {
		cfg, err := Load(env(map[string]string{"CAMPAIGN_CREATORS": v}))
		if err == nil {
			t.Errorf("CAMPAIGN_CREATORS=%q: Load() error = nil, creators=%v (empty allowlist means anyone may create)", v, cfg.Limits.CampaignCreators)
		}
	}
}
