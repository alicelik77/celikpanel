package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/alicelik/celikpanel/internal/passwordpolicy"
)

// errCodePasswordWeak is the one refusal for a new password that breaks the
// owner's rule (6 Nov 2026): too short or too long for its field, no
// uppercase letter, no lowercase letter, no special character, or control
// characters. Reason names the first rule broken; vars carry min and max, so
// the screen says it in the interface language (err.PASSWORD_WEAK.<reason>).
// Details lists every rule broken.
//
// errCodePasswordWeak, sahibin kuralina (6 Kas 2026) uymayan yeni parolanin
// tek retidir. Reason ilk bozulan kurali adlandirir; vars min ve max tasir.
const errCodePasswordWeak = passwordpolicy.ErrorCode

// writePasswordRefusal answers 400 PASSWORD_WEAK for a password the policy
// refused. The sentence names the rule; the password itself is never written.
// writePasswordRefusal, kuralin reddettigi parola icin 400 PASSWORD_WEAK yazar.
func writePasswordRefusal(w http.ResponseWriter, refusal *passwordpolicy.Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(passwordRefusalBody(refusal))
}

func passwordRefusalBody(refusal *passwordpolicy.Error) apiErrorBody {
	return apiErrorBody{
		Error:   refusal.Error(),
		Code:    errCodePasswordWeak,
		Reason:  refusal.Reason(),
		Vars:    refusal.Vars(),
		Details: append([]string(nil), refusal.Rules...),
	}
}

// passwordRefusalFrom finds a policy refusal carried up an error chain (team
// member validation, site feature input), so it reaches the response typed.
// passwordRefusalFrom, bir hata zincirinde tasinan kural retini bulur.
func passwordRefusalFrom(err error) (*passwordpolicy.Error, bool) {
	var refusal *passwordpolicy.Error
	if errors.As(err, &refusal) && refusal != nil {
		return refusal, true
	}
	return nil, false
}

// writePasswordRefusalIf writes the refusal when err carries one and reports
// whether it did.
// writePasswordRefusalIf, err bir kural reti tasiyorsa onu yazar.
func writePasswordRefusalIf(w http.ResponseWriter, err error) bool {
	refusal, ok := passwordRefusalFrom(err)
	if !ok {
		return false
	}
	writePasswordRefusal(w, refusal)
	return true
}
