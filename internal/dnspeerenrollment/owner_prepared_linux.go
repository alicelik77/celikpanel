//go:build linux

package dnspeerenrollment

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"golang.org/x/crypto/ssh"
)

// PreparedOwnerPublic returns the public half of a credential PrepareOwner
// created earlier, so an interrupted enrollment can continue with the same
// key instead of preparing another one. It writes nothing: the private key is
// read only to derive its public key and is wiped at once.
func PreparedOwnerPublic(credentialID string) (PreparedOwner, error) {
	return preparedOwnerPublicAt("/", credentialID)
}

func preparedOwnerPublicAt(root, credentialID string) (PreparedOwner, error) {
	var zero PreparedOwner
	if !id16(credentialID) {
		return zero, errors.New("prepared credential is invalid")
	}
	if err := ownerEnrollmentPrivateDir(root, false); err != nil {
		return zero, err
	}
	key, _, err := readSecure(root, CredentialDir+"/"+credentialID+".key", maxKeyBytes, 0600, false)
	if err != nil {
		return zero, errors.New("prepared credential cannot be verified")
	}
	signer, err := ssh.ParsePrivateKey(key)
	for i := range key {
		key[i] = 0
	}
	if err != nil || signer.PublicKey().Type() != ssh.KeyAlgoED25519 {
		return zero, errors.New("prepared credential is not Ed25519")
	}
	digest := sha256.Sum256(signer.PublicKey().Marshal())
	return PreparedOwner{
		CredentialID:    credentialID,
		PublicKey:       strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))),
		PublicKeySHA256: hex.EncodeToString(digest[:]),
	}, nil
}
