package signing

import "errors"

// Sentinel errors. Match with errors.Is. Wrapped copies add the quoted key id
// and never key material.
var (
	// ErrUnknownKey: no key with the signature's key id.
	ErrUnknownKey = errors.New("signing: unknown key")
	// ErrRevoked: the key id is revoked.
	ErrRevoked = errors.New("signing: key revoked")
	// ErrWrongPurpose: the key's purpose is not the verifier's purpose.
	ErrWrongPurpose = errors.New("signing: wrong key purpose")
	// ErrWrongScope: the key's scope is not the verifier's scope.
	ErrWrongScope = errors.New("signing: wrong key scope")
	// ErrBadSignature: the signature does not verify over the message.
	ErrBadSignature = errors.New("signing: bad signature")
	// ErrMalformed: an input (key id, signature, key, file, envelope or
	// configuration) is not in its canonical form.
	ErrMalformed = errors.New("signing: malformed input")
	// ErrExpired: the key's NotAfter has passed.
	ErrExpired = errors.New("signing: key expired")
	// ErrNotYetValid: the key's NotBefore has not been reached.
	ErrNotYetValid = errors.New("signing: key not yet valid")
)
