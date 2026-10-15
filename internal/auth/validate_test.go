package auth

import (
	"errors"
	"testing"
)

// The audit actor "system" is the Panel's own; a person cannot take it, in
// any letter case, and no other name is touched.
// Denetim aktörü "system" Panel'in kendisidir; kimse, hangi harf biçimiyle
// olursa olsun, onu alamaz; başka hiçbir ada dokunulmaz.
func TestValidateUsernameReservesSystemActor(t *testing.T) {
	for _, name := range []string{"system", "System", "SYSTEM", "sYsTeM"} {
		if err := ValidateUsername(name); !errors.Is(err, ErrReservedUsername) {
			t.Errorf("ValidateUsername(%q) = %v; want ErrReservedUsername", name, err)
		}
	}
	for _, name := range []string{"systems", "system1", "my-system", "sys", "system.admin", "root"} {
		if err := ValidateUsername(name); err != nil {
			t.Errorf("ValidateUsername(%q) = %v; want nil", name, err)
		}
	}
	// A malformed name keeps its own refusal, not the reserved one.
	if err := ValidateUsername("-system"); err == nil || errors.Is(err, ErrReservedUsername) {
		t.Errorf("ValidateUsername(\"-system\") = %v; want the format error", err)
	}
}
