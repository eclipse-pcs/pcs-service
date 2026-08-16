package test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/eclipse-pcs/pcs"

	"github.com/eclipse-pcs/pcs-service/internal/client"
	"github.com/eclipse-pcs/pcs-service/internal/protocol"
	"github.com/eclipse-pcs/pcs-service/internal/store"
)

func writeSplitParticlesToDir(t *testing.T, dir, base string, secret []byte) {
	t.Helper()
	addr, stop := startTestServer(t)
	defer stop()
	cfg := testClientConfig(addr, "TEST_TOKEN")
	result, err := client.SplitSession(cfg, secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureStorageDirs(dir); err != nil {
		t.Fatal(err)
	}
	for kind, data := range result.Particles {
		path := filepath.Join(dir, store.ParticleRelPath(base, kind))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func mergeFromDir(t *testing.T, dir, base string, profile protocol.MergeProfile) ([]byte, *protocol.Trailer) {
	t.Helper()
	addr, stop := startTestServer(t)
	defer stop()
	cfg := testClientConfig(addr, "TEST_TOKEN")
	var out bytes.Buffer
	trailer, err := client.MergeFromFiles(cfg, dir, base, profile, &out)
	if err != nil {
		t.Fatal(err)
	}
	return out.Bytes(), trailer
}

func removeParticle(t *testing.T, dir, base string, kind pcs.ParticleKind) {
	t.Helper()
	path := filepath.Join(dir, store.ParticleRelPath(base, kind))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func TestMergeSendPlanFullMergeUsesFourCores(t *testing.T) {
	dir := t.TempDir()
	base := "four-core.txt"
	secret := []byte("merge send plan full merge")
	writeSplitParticlesToDir(t, dir, base, secret)

	profile := protocol.MergeProfile{}
	plan, err := client.PrepareMergeSendPlan(dir, base, profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range pcs.CoreParticleKinds {
		if !plan.Send[kind] {
			t.Fatalf("%s should be sent", kind)
		}
	}
	if plan.Send[pcs.CypherParity] || plan.Send[pcs.NoiseParity] {
		t.Fatalf("parity should not be sent: cp=%v np=%v", plan.Send[pcs.CypherParity], plan.Send[pcs.NoiseParity])
	}

	got, trailer := mergeFromDir(t, dir, base, profile)
	if !bytes.Equal(secret, got) {
		t.Fatalf("merge mismatch: %q vs %q", got, secret)
	}
	if trailer.HashValid != nil && !*trailer.HashValid {
		t.Fatal("expected hash_valid true")
	}
	if len(trailer.Recoveries) != 0 {
		t.Fatalf("unexpected recovery notes: %v", trailer.Recoveries)
	}
}

func TestMergeFromFilesRecoveryMissingCore(t *testing.T) {
	cases := []struct {
		name       string
		removeKind pcs.ParticleKind
	}{
		{"missing_ec", pcs.EvenCypher},
		{"missing_oc", pcs.OddCypher},
		{"missing_en", pcs.EvenNoise},
		{"missing_on", pcs.OddNoise},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			base := "recover.bin"
			secret := []byte("recovery merge from disk")
			writeSplitParticlesToDir(t, dir, base, secret)
			removeParticle(t, dir, base, tc.removeKind)

			profile, err := client.PrepareMergeFromFiles(dir, base)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := client.PrepareMergeSendPlan(dir, base, profile)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.Send[pcs.CypherParity] && (tc.removeKind == pcs.EvenCypher || tc.removeKind == pcs.OddCypher) {
				t.Fatal("expected cypher parity in send plan")
			}
			if !plan.Send[pcs.NoiseParity] && (tc.removeKind == pcs.EvenNoise || tc.removeKind == pcs.OddNoise) {
				t.Fatal("expected noise parity in send plan")
			}

			got, trailer := mergeFromDir(t, dir, base, profile)
			if !bytes.Equal(secret, got) {
				t.Fatalf("merge mismatch: %q vs %q", got, secret)
			}
			if len(trailer.Recoveries) == 0 {
				t.Fatal("expected recovery note in trailer")
			}
		})
	}
}

func TestMergeFromFilesSkipCore(t *testing.T) {
	dir := t.TempDir()
	base := "skip-oc.txt"
	secret := []byte("skip oc but file remains")
	writeSplitParticlesToDir(t, dir, base, secret)

	profile, err := protocol.MergeProfileFromSkipKinds([]string{"oc"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := client.PrepareMergeSendPlan(dir, base, profile)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Send[pcs.OddCypher] {
		t.Fatal("oc should not be sent when skipped")
	}
	if !plan.Send[pcs.CypherParity] {
		t.Fatal("cp should be sent for oc recovery")
	}
	if plan.Send[pcs.NoiseParity] {
		t.Fatal("np should not be sent")
	}

	got, trailer := mergeFromDir(t, dir, base, profile)
	if !bytes.Equal(secret, got) {
		t.Fatalf("merge mismatch: %q vs %q", got, secret)
	}
	if len(trailer.Recoveries) == 0 {
		t.Fatal("expected recovery note in trailer")
	}
}

func TestMergeSendPlanRejectsInvalidParticleSets(t *testing.T) {
	cases := []struct {
		name       string
		remove     []pcs.ParticleKind
		skip       []string
		wantSubstr string
	}{
		{
			name:       "both_cypher_cores_missing",
			remove:     []pcs.ParticleKind{pcs.EvenCypher, pcs.OddCypher},
			wantSubstr: "both cypher core particles missing",
		},
		{
			name:       "both_noise_cores_missing",
			remove:     []pcs.ParticleKind{pcs.EvenNoise, pcs.OddNoise},
			wantSubstr: "both noise core particles missing",
		},
		{
			name:       "skip_both_cypher",
			skip:       []string{"ec", "oc"},
			wantSubstr: "both cypher core particles missing",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			base := "invalid.bin"
			secret := []byte("invalid merge subset")
			writeSplitParticlesToDir(t, dir, base, secret)
			for _, kind := range tc.remove {
				removeParticle(t, dir, base, kind)
			}

			var profile protocol.MergeProfile
			var err error
			if len(tc.skip) > 0 {
				profile, err = protocol.MergeProfileFromSkipKinds(tc.skip)
			} else {
				profile, err = client.PrepareMergeFromFiles(dir, base)
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.PrepareMergeSendPlan(dir, base, profile)
			if err == nil {
				t.Fatal("expected error")
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tc.wantSubstr)) {
				t.Fatalf("error %q should contain %q", err, tc.wantSubstr)
			}
		})
	}
}
