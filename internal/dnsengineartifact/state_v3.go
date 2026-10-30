package dnsengineartifact

import (
	"errors"
	"github.com/alicelik/celikpanel/internal/transport"
)

// StateDocumentSchemaV3 records the narrowly measured native catalog tenure.
// Its marker is publication-independent, so later member edits preserve the
// producer kind while the publication serial evolves.
const StateDocumentSchemaV3 = "celikpanel-dns-engine-state/v3"
const OwnershipDocumentSchemaV3 = "celikpanel-dns-engine-ownership/v3"
const NativeCatalogDebian49V3 = "pdns-fresh-paired-primary/debian-4.9/v1"

// NativeCatalogArch51V3 is the Arch Linux tenure (PowerDNS 5.1 from extra).
// Readers accept every marker of PDNSFreshPrimaryBuildForKind; a reader that
// predates this marker refuses an Arch document instead of misreading it.
const NativeCatalogArch51V3 = PDNSFreshPrimaryKindArch51V3

type stateDocumentV3 struct {
	Schema        string              `json:"schema"`
	Acquisition   AcquisitionRecordV1 `json:"acquisition"`
	Publication   PublicationRecordV1 `json:"publication"`
	NativeCatalog string              `json:"native_catalog"`
}

func ValidateNativeCatalogStateV3(state StateV1) error {
	if !IsNativeCatalogV3(state.NativeCatalogV3) ||
		state.Schema != StateSchemaV1 ||
		state.Mode != transport.DNSEngineSwitchModeSwitch ||
		state.Engine != transport.DNSEnginePowerDNS ||
		state.PairRole != transport.DNSPairRolePrimary ||
		state.PrimaryCatalogSerial <= 1 {
		return errors.New("unsupported native PowerDNS state identity")
	}
	return ValidateV1(state)
}

func CanonicalStateDocumentV3(state StateV1) ([]byte, error) {
	return canonicalDocumentV3(StateDocumentSchemaV3, state)
}

func CanonicalOwnershipDocumentV3(state StateV1) ([]byte, error) {
	return canonicalDocumentV3(OwnershipDocumentSchemaV3, state)
}

func canonicalDocumentV3(schema string, state StateV1) ([]byte, error) {
	if err := ValidateNativeCatalogStateV3(state); err != nil {
		return nil, err
	}
	a, p, err := SeparateV1(state)
	if err != nil {
		return nil, err
	}
	return canonicalRecord(stateDocumentV3{Schema: schema,
		Acquisition: a, Publication: p, NativeCatalog: state.NativeCatalogV3})
}

func DecodeStateDocumentV3(data []byte) (StateV1, error) {
	return decodeDocumentV3(StateDocumentSchemaV3, data)
}

func DecodeOwnershipDocumentV3(data []byte) (StateV1, error) {
	return decodeDocumentV3(OwnershipDocumentSchemaV3, data)
}

func decodeDocumentV3(schema string, data []byte) (StateV1, error) {
	var doc stateDocumentV3
	var state StateV1
	err := decodeCanonicalRecord(data, recordLimit, &doc, func() ([]byte, error) {
		if doc.Schema != schema || !IsNativeCatalogV3(doc.NativeCatalog) {
			return nil, errors.New("unsupported native PowerDNS state document")
		}
		var err error
		state, err = CombineV1(doc.Acquisition, doc.Publication)
		if err != nil {
			return nil, err
		}
		state.NativeCatalogV3 = doc.NativeCatalog
		return canonicalDocumentV3(schema, state)
	})
	if err != nil {
		return StateV1{}, err
	}
	return state, nil
}
