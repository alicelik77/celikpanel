package auth

import (
	"errors"
	"fmt"
	"strings"
)

// ReservedUsernameSystem is the actor the Panel writes to the audit log for an
// action without a signed-in user; the Dashboard says it "System" / "Sistem".
// A person must not be able to take that name, or their actions would be
// shown as the Panel's own.
//
// ReservedUsernameSystem, Panel'in oturum açmış kullanıcısı olmayan eylemler
// için denetim günlüğüne yazdığı aktördür; Pano onu "Sistem" diye söyler.
// Biri bu adı alamaz; yoksa eylemleri Panel'inkiymiş gibi görünürdü.
const ReservedUsernameSystem = "system"

// ErrReservedUsername is returned for a name that is reserved (case-insensitive).
// Existing rows are not touched; only a new or changed name is refused.
//
// ErrReservedUsername, ayrılmış bir ad için döner (büyük/küçük harf duyarsız).
// Var olan satırlara dokunulmaz; yalnız yeni ya da değişen ad reddedilir.
var ErrReservedUsername = errors.New("username is reserved")

// IsReservedUsername reports whether the name is reserved, ignoring case.
func IsReservedUsername(username string) bool {
	return strings.EqualFold(username, ReservedUsernameSystem)
}

// ValidateUsername enforces a conservative username policy: 3–32 characters
// of letters, digits, underscore, hyphen or dot, starting with a letter or
// digit. Usernames may map to display and lookup, so predictability is
// preferred over permissiveness.
//
// ValidateUsername, muhafazakâr bir kullanıcı adı politikası uygular: harf,
// rakam, alt çizgi, tire veya nokta içeren 3–32 karakter; harf ya da
// rakamla başlar. Kullanıcı adları görüntüleme ve aramaya karşılık
// gelebildiğinden, öngörülebilirlik esnekliğe tercih edilir.
func ValidateUsername(username string) error {
	if len(username) < 3 || len(username) > 32 {
		return fmt.Errorf("username must be 3–32 characters")
	}
	for i, r := range username {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		if i == 0 {
			if !isLetter && !isDigit {
				return fmt.Errorf("username must start with a letter or digit")
			}
			continue
		}
		if !isLetter && !isDigit && r != '_' && r != '-' && r != '.' {
			return fmt.Errorf("username contains an invalid character %q", r)
		}
	}
	if IsReservedUsername(username) {
		return ErrReservedUsername
	}
	return nil
}
