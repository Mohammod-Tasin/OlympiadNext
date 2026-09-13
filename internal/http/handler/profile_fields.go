package handler

import "strings"

// maxProfileFieldLength caps full_name and institution_name, matching the
// VARCHAR(255) columns they're stored in.
const maxProfileFieldLength = 255

// allowedLevels and allowedMediums are the fixed onboarding options the
// frontend presents; any other value is rejected rather than silently
// stored.
var allowedLevels = map[string]bool{
	"Junior":           true,
	"Secondary":        true,
	"Higher Secondary": true,
}

var allowedMediums = map[string]bool{
	"Bangla":  true,
	"English": true,
}

// validateProfileFields trims full_name, institution_name, level, and
// medium and enforces the onboarding rules: none may be empty,
// full_name/institution_name must fit their DB column, and level/medium
// must be one of the fixed allowed values. errMsg is empty when
// validation passes.
func validateProfileFields(fullName, institution, level, medium string) (trimmedFullName, trimmedInstitution, trimmedLevel, trimmedMedium, errMsg string) {
	trimmedFullName = strings.TrimSpace(fullName)
	trimmedInstitution = strings.TrimSpace(institution)
	trimmedLevel = strings.TrimSpace(level)
	trimmedMedium = strings.TrimSpace(medium)

	if trimmedFullName == "" || trimmedInstitution == "" || trimmedLevel == "" || trimmedMedium == "" {
		return "", "", "", "", "full_name, institution_name, level, and medium are required"
	}
	if len(trimmedFullName) > maxProfileFieldLength {
		return "", "", "", "", "full_name must be 255 characters or fewer"
	}
	if len(trimmedInstitution) > maxProfileFieldLength {
		return "", "", "", "", "institution_name must be 255 characters or fewer"
	}
	if !allowedLevels[trimmedLevel] {
		return "", "", "", "", "level must be one of: Junior, Secondary, Higher Secondary"
	}
	if !allowedMediums[trimmedMedium] {
		return "", "", "", "", "medium must be one of: Bangla, English"
	}
	return trimmedFullName, trimmedInstitution, trimmedLevel, trimmedMedium, ""
}

// requiresNewVerificationDoc reports whether changing level or
// institutionName away from the currently stored value (currentLevel /
// currentInstitution) demands a new verification document in this same
// request: changing either invalidates the admin's existing review of the
// student's identity, so a fresh document must accompany the change. A
// brand-new account (both current values "") always has doc set already
// by the caller's own onboarding flow, so this never blocks first-time
// submission.
func requiresNewVerificationDoc(currentLevel, currentInstitution, newLevel, newInstitution, doc string) bool {
	if doc != "" {
		return false
	}
	return newLevel != currentLevel || newInstitution != currentInstitution
}

// strOrEmpty dereferences an optional profile field (Level /
// InstitutionName are nil until onboarding sets them) to "" rather than
// panicking on a nil pointer.
func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
