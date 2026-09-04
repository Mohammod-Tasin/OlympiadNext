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
