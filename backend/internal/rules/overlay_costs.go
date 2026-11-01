package rules

import (
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
)

// checkGrantAction checks the resource an action spends and that it can pay the cost.
func (b *overlayBuilder) checkGrantAction(site effectSite, e *Effect) error {
	if e.Resource != "" && !b.resourceDefined(site.owner, e.Resource) {
		return site.fail(".resource", ReasonReference, "the action spends %q, which no resource effect of this entry or its class defines", e.Resource)
	}
	return b.checkAffordable(site, e)
}

// checkSlotRecovery checks the resource a slot_recovery spends: the entry (or its class) defines it.
func (b *overlayBuilder) checkSlotRecovery(site effectSite, e *Effect) error {
	if !b.resourceDefined(site.owner, e.Resource) {
		return site.fail(".resource", ReasonReference, "the recovery spends %q, which no resource effect of this entry or its class defines", e.Resource)
	}
	return nil
}

// checkAffordable refuses an action whose cost no level of the character can pay: the
// resource it spends never has that many uses (a cost of 3 of a resource of 2).
func (b *overlayBuilder) checkAffordable(site effectSite, e *Effect) error {
	if e.Cost == "" || e.Resource == "" {
		return nil
	}
	if src := b.resourceMaxOf(site.owner, e.Resource); src != "" && b.neverPayable(e.Cost, src) {
		return site.fail(".cost", ReasonValue, "the cost %q is more than %q ever has: the action could never be used", e.Cost, e.Resource)
	}
	return nil
}

// checkOnEvent checks the resources a hook spends and restores, which the entry
// (or its class) defines, and that it can pay what it spends.
func (b *overlayBuilder) checkOnEvent(site effectSite, e *Effect) error {
	if e.Resource != "" && !b.resourceDefined(site.owner, e.Resource) {
		return site.fail(".resource", ReasonReference, "the hook spends %q, which no resource effect of this entry or its class defines", e.Resource)
	}
	if e.Restore != "" && !b.resourceDefined(site.owner, e.Restore) {
		return site.fail(".restore", ReasonReference, "the hook restores %q, which no resource effect of this entry or its class defines", e.Restore)
	}
	return b.checkAffordable(site, e)
}

// resourceMaxOf is the formula of the maximum of a resource the owner's entry or
// class declares, "" when it is an SRD resource (its maximum is not known here).
func (b *overlayBuilder) resourceMaxOf(owner, name string) string {
	if src := b.resourceMax[b.featureEntry[owner]][name]; src != "" {
		return src
	}
	return b.resourceMax[b.featureClass[owner]][name]
}

// stubbedHelpers are the formula functions whose value depends on the sheet: a
// formula that calls one has no answer without a character.
var stubbedHelpers = []string{"mod(", "score(", "classLevel(", "armor(", "shield("}

// neverPayable says the cost is above the maximum at every level from 1 to 20. It
// judges only formulas that read the level and the proficiency bonus alone; one that
// reads the sheet, or does not compile, is not judged here.
func (b *overlayBuilder) neverPayable(cost, maxSrc string) bool {
	for _, src := range []string{cost, maxSrc} {
		for _, h := range stubbedHelpers {
			if strings.Contains(src, h) {
				return false
			}
		}
	}
	costProg, err1 := b.n.compileFormula(cost, formula.Int)
	maxProg, err2 := b.n.compileFormula(maxSrc, formula.Int)
	if err1 != nil || err2 != nil {
		return false
	}
	for level := 1; level <= MaxLevel; level++ {
		env := levelOnlyEnv(level)
		c, err1 := costProg.Int(env)
		m, err2 := maxProg.Int(env)
		if err1 != nil || err2 != nil || c <= m {
			return false
		}
	}
	return true
}

// plainScore is an ability score whose modifier is 0.
const plainScore = 10

// levelOnlyEnv is a formula environment of a character of the level with nothing else:
// every ability at 10, no armor.
func levelOnlyEnv(level int) *formula.Env {
	prof := 2 + (level-1)/4
	return &formula.Env{
		Level:      func() int { return level },
		ClassLevel: func(string) int { return 0 },
		Mod:        func(string) int { return 0 },
		Score:      func(string) int { return plainScore },
		Prof:       func() int { return prof },
		Armor:      func() string { return "none" },
		Shield:     func() bool { return false },
	}
}
