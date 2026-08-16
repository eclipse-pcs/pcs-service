package mergeplan

import (
	"fmt"

	"github.com/eclipse-pcs/pcs"
)

// MergeSendPlan declares which particle streams the merge client will upload.
type MergeSendPlan struct {
	Send map[pcs.ParticleKind]bool
}

// MergeSendPlanFrom builds a send plan from on-disk inventory and missing-core flags.
func MergeSendPlanFrom(inv *pcs.ParticleInventory, missing map[pcs.ParticleKind]bool) (MergeSendPlan, error) {
	if inv == nil {
		return MergeSendPlan{}, fmt.Errorf("particle inventory is required")
	}
	plan := MergeSendPlan{Send: make(map[pcs.ParticleKind]bool, len(pcs.AllParticleKinds))}

	sendCore := func(kind pcs.ParticleKind) bool {
		return inv.Present[kind] && !missing[kind]
	}

	plan.Send[pcs.EvenCypher] = sendCore(pcs.EvenCypher)
	plan.Send[pcs.OddCypher] = sendCore(pcs.OddCypher)
	plan.Send[pcs.EvenNoise] = sendCore(pcs.EvenNoise)
	plan.Send[pcs.OddNoise] = sendCore(pcs.OddNoise)

	cypherCoreMissing := missing[pcs.EvenCypher] || missing[pcs.OddCypher]
	cypherAnyCore := plan.Send[pcs.EvenCypher] || plan.Send[pcs.OddCypher]
	plan.Send[pcs.CypherParity] = cypherCoreMissing && inv.Present[pcs.CypherParity] && cypherAnyCore

	noiseCoreMissing := missing[pcs.EvenNoise] || missing[pcs.OddNoise]
	noiseAnyCore := plan.Send[pcs.EvenNoise] || plan.Send[pcs.OddNoise]
	plan.Send[pcs.NoiseParity] = noiseCoreMissing && inv.Present[pcs.NoiseParity] && noiseAnyCore

	if err := ValidateMergeSendPlan(plan, missing, inv); err != nil {
		return MergeSendPlan{}, err
	}
	return plan, nil
}

// MergeSendPlanFromCore builds a send plan from in-memory particle bytes.
func MergeSendPlanFromCore(core map[pcs.ParticleKind][]byte, missing map[pcs.ParticleKind]bool) (MergeSendPlan, error) {
	present := make(map[pcs.ParticleKind]bool, len(pcs.AllParticleKinds))
	for _, kind := range pcs.AllParticleKinds {
		_, ok := core[kind]
		present[kind] = ok
	}
	return MergeSendPlanFrom(pcs.NewParticleInventory(present), missing)
}

// ValidateMergeSendPlan rejects unrecoverable or parity-only send sets.
func ValidateMergeSendPlan(plan MergeSendPlan, missing map[pcs.ParticleKind]bool, inv *pcs.ParticleInventory) error {
	if missing[pcs.EvenCypher] && missing[pcs.OddCypher] {
		return fmt.Errorf("cannot merge: both cypher core particles missing")
	}
	if missing[pcs.EvenNoise] && missing[pcs.OddNoise] {
		return fmt.Errorf("cannot merge: both noise core particles missing")
	}
	if plan.Send[pcs.CypherParity] && !plan.Send[pcs.EvenCypher] && !plan.Send[pcs.OddCypher] {
		return fmt.Errorf("cannot merge: cypher parity without a cypher core particle")
	}
	if plan.Send[pcs.NoiseParity] && !plan.Send[pcs.EvenNoise] && !plan.Send[pcs.OddNoise] {
		return fmt.Errorf("cannot merge: noise parity without a noise core particle")
	}

	for _, kind := range pcs.CoreParticleKinds {
		if missing[kind] {
			if plan.Send[kind] {
				return fmt.Errorf("cannot merge: %s marked missing but included in send plan", kind)
			}
			continue
		}
		if !inv.Present[kind] {
			return fmt.Errorf("cannot merge: core particle %s required but not available", kind)
		}
		if !plan.Send[kind] {
			return fmt.Errorf("cannot merge: core particle %s required but not in send plan", kind)
		}
	}

	if missing[pcs.EvenCypher] != missing[pcs.OddCypher] {
		if inv.Present[pcs.CypherParity] && !plan.Send[pcs.CypherParity] {
			return fmt.Errorf("cannot merge: cypher parity required for recovery")
		}
	}
	if missing[pcs.EvenNoise] != missing[pcs.OddNoise] {
		if inv.Present[pcs.NoiseParity] && !plan.Send[pcs.NoiseParity] {
			return fmt.Errorf("cannot merge: noise parity required for recovery")
		}
	}
	return nil
}
