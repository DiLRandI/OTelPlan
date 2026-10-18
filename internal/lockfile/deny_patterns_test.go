package lockfile_test

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestPolicyDenyPatternsDigest(t *testing.T) {
	t.Parallel()

	policy := policyDigestFixture()
	baseline := digestPolicy(t, policy)

	policy.DenyPatterns = []string{}
	if digestPolicy(t, policy) != baseline {
		t.Fatal("empty denyPatterns changed an existing policy digest")
	}

	policy.DenyPatterns = []string{"bank_reference", "account_number", "bank_reference"}

	before, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}

	configured := digestPolicy(t, policy)
	if configured == baseline {
		t.Fatal("configured deny patterns did not change policy identity")
	}

	after, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(before, after) {
		t.Fatal("deny-pattern canonicalization mutated the caller policy")
	}

	policy.DenyPatterns = []string{"account_number", "bank_reference"}
	if digestPolicy(t, policy) != configured {
		t.Fatal("deny-pattern ordering or duplicates changed policy identity")
	}

	policy.DenyPatterns = []string{"account_number", "credit_card"}
	if digestPolicy(t, policy) == configured {
		t.Fatal("changed deny-pattern settings retained policy identity")
	}
}
