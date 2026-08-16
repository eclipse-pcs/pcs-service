package mergeplan_test

import (
	"testing"

	"github.com/eclipse-pcs/pcs"

	"github.com/eclipse-pcs/pcs-service/internal/mergeplan"
)

func fullInventory() *pcs.ParticleInventory {
	return pcs.NewParticleInventory(map[pcs.ParticleKind]bool{
		pcs.EvenCypher: true, pcs.OddCypher: true,
		pcs.EvenNoise: true, pcs.OddNoise: true,
		pcs.CypherParity: true, pcs.NoiseParity: true,
	})
}

func TestMergeSendPlanFullMergeSendsFourCoresOnly(t *testing.T) {
	plan, err := mergeplan.MergeSendPlanFrom(fullInventory(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range pcs.CoreParticleKinds {
		if !plan.Send[kind] {
			t.Fatalf("%s should be sent", kind)
		}
	}
	if plan.Send[pcs.CypherParity] || plan.Send[pcs.NoiseParity] {
		t.Fatalf("parity should not be sent on full merge: cp=%v np=%v", plan.Send[pcs.CypherParity], plan.Send[pcs.NoiseParity])
	}
}

func TestMergeSendPlanRecoveryMissingOC(t *testing.T) {
	missing := map[pcs.ParticleKind]bool{pcs.OddCypher: true}
	plan, err := mergeplan.MergeSendPlanFrom(fullInventory(), missing)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Send[pcs.EvenCypher] || plan.Send[pcs.OddCypher] {
		t.Fatalf("ec=%v oc=%v", plan.Send[pcs.EvenCypher], plan.Send[pcs.OddCypher])
	}
	if !plan.Send[pcs.CypherParity] {
		t.Fatal("cp should be sent for oc recovery")
	}
	if plan.Send[pcs.NoiseParity] {
		t.Fatal("np should not be sent")
	}
}

func TestMergeSendPlanRejectsBothCypherMissing(t *testing.T) {
	missing := map[pcs.ParticleKind]bool{pcs.EvenCypher: true, pcs.OddCypher: true}
	_, err := mergeplan.MergeSendPlanFrom(fullInventory(), missing)
	if err == nil {
		t.Fatal("expected error for both cypher cores missing")
	}
}

func TestMergeSendPlanFromCore(t *testing.T) {
	core := map[pcs.ParticleKind][]byte{
		pcs.EvenCypher:   []byte("a"),
		pcs.OddCypher:    []byte("b"),
		pcs.EvenNoise:    []byte("c"),
		pcs.OddNoise:     []byte("d"),
		pcs.CypherParity: []byte("p"),
		pcs.NoiseParity:  []byte("q"),
	}
	plan, err := mergeplan.MergeSendPlanFromCore(core, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Send[pcs.CypherParity] || plan.Send[pcs.NoiseParity] {
		t.Fatal("full in-memory merge should not send parity")
	}
}
