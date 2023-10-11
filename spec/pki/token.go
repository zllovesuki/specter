package pki

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/util"

	"go.uber.org/zap/zapcore"
)

type TokenVersion string

const (
	TokenSeparator string       = ":"
	TokenV1        TokenVersion = "v1" // CommonName: v1:clientID:oldToken 							before PKI is implemented, only the oldToken will be used
	TokenV2        TokenVersion = "v2" // CommonName: v2:clientID:base64url(sha256(public key)) 	initial implementation of PKI, the entire CommonName will be used
)

type Identity struct {
	Token []byte
	ID    uint64
}

func (n *Identity) NodeIdentity() *protocol.Node {
	return &protocol.Node{
		Id:         n.ID,
		Address:    string(n.Token),
		Rendezvous: true,
	}
}

func MakeSubjectV1(id uint64, token string) pkix.Name {
	return pkix.Name{
		CommonName: strings.Join([]string{
			string(TokenV1),
			strconv.FormatUint(id, 10),
			token,
		}, TokenSeparator),
	}
}

func MakeSubjectV2(id uint64, hash []byte) pkix.Name {
	return pkix.Name{
		CommonName: strings.Join([]string{
			string(TokenV2),
			strconv.FormatUint(id, 10),
			base64.URLEncoding.EncodeToString(hash),
		}, TokenSeparator),
	}
}

func ExtractCertificateIdentity(cert *x509.Certificate) (*Identity, error) {
	cn := cert.Subject.CommonName
	parts := strings.SplitN(cn, ":", 3)

	switch parts[0] {
	case string(TokenV1):
		return &Identity{
			ID:    util.Must(strconv.ParseUint(parts[1], 10, 64)),
			Token: []byte(parts[2]),
		}, nil
	case string(TokenV2):
		return &Identity{
			ID:    util.Must(strconv.ParseUint(parts[1], 10, 64)),
			Token: []byte(cn),
		}, nil
	default:
		return nil, errors.New("pki: unknown subject in certificate")
	}
}

var _ zapcore.ObjectMarshaler = (*Identity)(nil)

func (n *Identity) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("id", n.ID)
	enc.AddString("token", string(n.Token))
	return nil
}
