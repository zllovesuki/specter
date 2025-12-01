package acme

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/go-chi/chi/v5"
)

const certsPrefix = "certificates"

type issuerInfo struct {
	Issuer string `json:"issuer"`
	Count  int    `json:"count"`
}

type issuersResponse struct {
	Issuers []issuerInfo `json:"issuers"`
}

type certSummary struct {
	Name   string `json:"name"`   // safe key name (e.g., wildcard_.example.com)
	Domain string `json:"domain"` // original domain (e.g., *.example.com)
}

type certsListResponse struct {
	Issuer string        `json:"issuer"`
	Count  int           `json:"count"`
	Certs  []certSummary `json:"certs"`
}

type publicKeyInfo struct {
	Algorithm string `json:"algorithm"`
	Size      int    `json:"size,omitempty"`
}

type subjectInfo struct {
	CommonName         string   `json:"common_name,omitempty"`
	Organization       []string `json:"organization,omitempty"`
	OrganizationalUnit []string `json:"organizational_unit,omitempty"`
	Country            []string `json:"country,omitempty"`
	Province           []string `json:"province,omitempty"`
	Locality           []string `json:"locality,omitempty"`
}

type certInspectResponse struct {
	IssuerKey string `json:"issuer_key"`
	Name      string `json:"name"`
	Domain    string `json:"domain"`

	Version            int           `json:"version"`
	SerialNumber       string        `json:"serial_number"`
	SignatureAlgorithm string        `json:"signature_algorithm"`
	Subject            subjectInfo   `json:"subject"`
	Issuer             subjectInfo   `json:"issuer"`
	NotBefore          time.Time     `json:"not_before"`
	NotAfter           time.Time     `json:"not_after"`
	PublicKey          publicKeyInfo `json:"public_key"`

	DNSNames       []string `json:"dns_names,omitempty"`
	EmailAddresses []string `json:"email_addresses,omitempty"`
	IPAddresses    []string `json:"ip_addresses,omitempty"`
	URIs           []string `json:"uris,omitempty"`
	KeyUsage       []string `json:"key_usage,omitempty"`
	ExtKeyUsage    []string `json:"ext_key_usage,omitempty"`
	IsCA           bool     `json:"is_ca"`

	Metadata json.RawMessage `json:"metadata,omitempty"`
}

func AcmeManagerHandler(m *Manager) http.Handler {
	router := chi.NewRouter()

	router.Post("/clean", func(w http.ResponseWriter, r *http.Request) {
		certmagic.CleanStorage(r.Context(), m.chordStorage, certmagic.CleanStorageOptions{
			OCSPStaples:            true,
			ExpiredCerts:           true,
			ExpiredCertGracePeriod: time.Hour * 24 * 30,
		})
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /certs - list issuers and certificate counts
	router.Get("/certs", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		keys, err := m.chordStorage.List(ctx, certsPrefix, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		issuers := make([]issuerInfo, 0, len(keys))
		for _, key := range keys {
			issuer := strings.TrimPrefix(key, certsPrefix+"/")
			if issuer == "" {
				continue
			}
			// Count certs under this issuer
			issuerPrefix := path.Join(certsPrefix, issuer)
			certKeys, err := m.chordStorage.List(ctx, issuerPrefix, false)
			if err != nil {
				continue
			}
			issuers = append(issuers, issuerInfo{
				Issuer: issuer,
				Count:  len(certKeys),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(issuersResponse{Issuers: issuers})
	})

	// GET /certs/{issuer} - list certificates for an issuer
	router.Get("/certs/{issuer}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		issuer := chi.URLParam(r, "issuer")

		issuerPrefix := path.Join(certsPrefix, issuer)
		keys, err := m.chordStorage.List(ctx, issuerPrefix, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		certs := make([]certSummary, 0, len(keys))
		for _, key := range keys {
			safeName := strings.TrimPrefix(key, issuerPrefix+"/")
			if safeName == "" {
				continue
			}
			certs = append(certs, certSummary{
				Name:   safeName,
				Domain: unsafeName(safeName),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(certsListResponse{
			Issuer: issuer,
			Count:  len(certs),
			Certs:  certs,
		})
	})

	// GET /certs/{issuer}/{name} - inspect a certificate
	router.Get("/certs/{issuer}/{name}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		issuer := chi.URLParam(r, "issuer")
		name := chi.URLParam(r, "name")
		safeName := certmagic.StorageKeys.Safe(name)

		// Load certificate
		certKey := certmagic.StorageKeys.SiteCert(issuer, name)
		certPEM, err := m.chordStorage.Load(ctx, certKey)
		if err != nil {
			if err == fs.ErrNotExist {
				http.Error(w, "certificate not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Parse certificate
		block, _ := pem.Decode(certPEM)
		if block == nil {
			http.Error(w, "failed to decode certificate PEM", http.StatusInternalServerError)
			return
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			http.Error(w, "failed to parse certificate: "+err.Error(), http.StatusInternalServerError)
			return
		}

		resp := certInspectResponse{
			IssuerKey:          issuer,
			Name:               safeName,
			Domain:             unsafeName(safeName),
			Version:            cert.Version,
			SerialNumber:       cert.SerialNumber.String(),
			SignatureAlgorithm: cert.SignatureAlgorithm.String(),
			Subject:            extractSubjectInfo(cert.Subject),
			Issuer:             extractSubjectInfo(cert.Issuer),
			NotBefore:          cert.NotBefore,
			NotAfter:           cert.NotAfter,
			PublicKey:          extractPublicKeyInfo(cert),
			DNSNames:           cert.DNSNames,
			EmailAddresses:     cert.EmailAddresses,
			IPAddresses:        formatIPAddresses(cert),
			URIs:               formatURIs(cert),
			KeyUsage:           formatKeyUsage(cert.KeyUsage),
			ExtKeyUsage:        formatExtKeyUsage(cert.ExtKeyUsage),
			IsCA:               cert.IsCA,
		}

		// Load metadata if available
		metaKey := certmagic.StorageKeys.SiteMeta(issuer, name)
		metaData, err := m.chordStorage.Load(ctx, metaKey)
		if err == nil && len(metaData) > 0 {
			resp.Metadata = metaData
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// DELETE /certs/{issuer}/{name} - delete a certificate
	router.Delete("/certs/{issuer}/{name}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		issuer := chi.URLParam(r, "issuer")
		name := chi.URLParam(r, "name")

		// Delete cert, key, and meta files
		certKey := certmagic.StorageKeys.SiteCert(issuer, name)
		keyKey := certmagic.StorageKeys.SitePrivateKey(issuer, name)
		metaKey := certmagic.StorageKeys.SiteMeta(issuer, name)

		var errs []string
		if err := m.chordStorage.Delete(ctx, certKey); err != nil && err != fs.ErrNotExist {
			errs = append(errs, "cert: "+err.Error())
		}
		if err := m.chordStorage.Delete(ctx, keyKey); err != nil && err != fs.ErrNotExist {
			errs = append(errs, "key: "+err.Error())
		}
		if err := m.chordStorage.Delete(ctx, metaKey); err != nil && err != fs.ErrNotExist {
			errs = append(errs, "meta: "+err.Error())
		}

		if len(errs) > 0 {
			http.Error(w, strings.Join(errs, "; "), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	return router
}

// unsafeName converts a safe storage key name back to the original domain
// e.g., "wildcard_.example.com" -> "*.example.com"
func unsafeName(name string) string {
	if strings.HasPrefix(name, "wildcard_") {
		return "*" + strings.TrimPrefix(name, "wildcard_")
	}
	return name
}

func extractSubjectInfo(name pkix.Name) subjectInfo {
	return subjectInfo{
		CommonName:         name.CommonName,
		Organization:       name.Organization,
		OrganizationalUnit: name.OrganizationalUnit,
		Country:            name.Country,
		Province:           name.Province,
		Locality:           name.Locality,
	}
}

func extractPublicKeyInfo(cert *x509.Certificate) publicKeyInfo {
	info := publicKeyInfo{}
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		info.Algorithm = "RSA"
		info.Size = pub.N.BitLen()
	case *ecdsa.PublicKey:
		info.Algorithm = fmt.Sprintf("ECDSA (%s)", pub.Curve.Params().Name)
		info.Size = pub.Curve.Params().BitSize
	case ed25519.PublicKey:
		info.Algorithm = "Ed25519"
		info.Size = 256
	default:
		info.Algorithm = "Unknown"
	}
	return info
}

func formatIPAddresses(cert *x509.Certificate) []string {
	if len(cert.IPAddresses) == 0 {
		return nil
	}
	ips := make([]string, len(cert.IPAddresses))
	for i, ip := range cert.IPAddresses {
		ips[i] = ip.String()
	}
	return ips
}

func formatURIs(cert *x509.Certificate) []string {
	if len(cert.URIs) == 0 {
		return nil
	}
	uris := make([]string, len(cert.URIs))
	for i, u := range cert.URIs {
		uris[i] = u.String()
	}
	return uris
}

func formatKeyUsage(ku x509.KeyUsage) []string {
	if ku == 0 {
		return nil
	}
	var usages []string
	if ku&x509.KeyUsageDigitalSignature != 0 {
		usages = append(usages, "Digital Signature")
	}
	if ku&x509.KeyUsageContentCommitment != 0 {
		usages = append(usages, "Content Commitment")
	}
	if ku&x509.KeyUsageKeyEncipherment != 0 {
		usages = append(usages, "Key Encipherment")
	}
	if ku&x509.KeyUsageDataEncipherment != 0 {
		usages = append(usages, "Data Encipherment")
	}
	if ku&x509.KeyUsageKeyAgreement != 0 {
		usages = append(usages, "Key Agreement")
	}
	if ku&x509.KeyUsageCertSign != 0 {
		usages = append(usages, "Certificate Sign")
	}
	if ku&x509.KeyUsageCRLSign != 0 {
		usages = append(usages, "CRL Sign")
	}
	if ku&x509.KeyUsageEncipherOnly != 0 {
		usages = append(usages, "Encipher Only")
	}
	if ku&x509.KeyUsageDecipherOnly != 0 {
		usages = append(usages, "Decipher Only")
	}
	return usages
}

func formatExtKeyUsage(ekus []x509.ExtKeyUsage) []string {
	if len(ekus) == 0 {
		return nil
	}
	var usages []string
	for _, eku := range ekus {
		switch eku {
		case x509.ExtKeyUsageAny:
			usages = append(usages, "Any")
		case x509.ExtKeyUsageServerAuth:
			usages = append(usages, "TLS Web Server Authentication")
		case x509.ExtKeyUsageClientAuth:
			usages = append(usages, "TLS Web Client Authentication")
		case x509.ExtKeyUsageCodeSigning:
			usages = append(usages, "Code Signing")
		case x509.ExtKeyUsageEmailProtection:
			usages = append(usages, "Email Protection")
		case x509.ExtKeyUsageTimeStamping:
			usages = append(usages, "Time Stamping")
		case x509.ExtKeyUsageOCSPSigning:
			usages = append(usages, "OCSP Signing")
		default:
			usages = append(usages, fmt.Sprintf("Unknown (%d)", eku))
		}
	}
	return usages
}
