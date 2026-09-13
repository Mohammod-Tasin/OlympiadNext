package handler

import "testing"

func TestRequiresNewVerificationDoc(t *testing.T) {
	cases := []struct {
		name                             string
		currentLevel, currentInstitution string
		newLevel, newInstitution, doc    string
		want                             bool
	}{
		{"no change, no doc", "Junior", "ABC School", "Junior", "ABC School", "", false},
		{"level changed, no doc", "Junior", "ABC School", "Secondary", "ABC School", "", true},
		{"institution changed, no doc", "Junior", "ABC School", "Junior", "XYZ School", "", true},
		{"both changed, no doc", "Junior", "ABC School", "Secondary", "XYZ School", "", true},
		{"level changed, doc provided", "Junior", "ABC School", "Secondary", "ABC School", "/uploads/users/u1/doc.pdf", false},
		{"institution changed, doc provided", "Junior", "ABC School", "Junior", "XYZ School", "/uploads/users/u1/doc.pdf", false},
		{"first-time onboarding, doc provided", "", "", "Junior", "ABC School", "/uploads/users/u1/doc.pdf", false},
		{"neither field present yet, no doc", "", "", "", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := requiresNewVerificationDoc(tc.currentLevel, tc.currentInstitution, tc.newLevel, tc.newInstitution, tc.doc)
			if got != tc.want {
				t.Fatalf("requiresNewVerificationDoc() = %v, want %v", got, tc.want)
			}
		})
	}
}
