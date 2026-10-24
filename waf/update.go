package waf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.miragespace.co/specter/spec/protocol"

	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
)

type ruleSource struct {
	client      *http.Client
	releasesURL string
	downloadURL string
	keys        openpgp.EntityList
}

func officialRuleSource() (*ruleSource, error) {
	keys, err := trustedCRSKeys()
	if err != nil {
		return nil, err
	}
	return &ruleSource{
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(r *http.Request, via []*http.Request) error {
				if len(via) >= 5 || r.URL.Scheme != "https" {
					return fmt.Errorf("invalid CRS download redirect")
				}
				switch r.URL.Hostname() {
				case "github.com", "api.github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
					return nil
				default:
					return fmt.Errorf("unexpected CRS download host %q", r.URL.Hostname())
				}
			},
		},
		releasesURL: "https://api.github.com/repos/coreruleset/coreruleset/releases",
		downloadURL: "https://github.com/coreruleset/coreruleset/releases/download",
		keys:        keys,
	}, nil
}

func (s *ruleSource) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("User-Agent", "Specter-WAF")
	r.Header.Set("Accept", "application/vnd.github+json")
	response, err := s.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CRS source returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("CRS source response exceeds size limit")
	}
	return data, nil
}

func (s *ruleSource) latest(ctx context.Context, channel protocol.WAFSettings_Channel) (string, error) {
	latest := ""
	// A bounded release listing covers the supported LTS window without following
	// arbitrary links from unsigned discovery metadata.
	for page := 1; page <= 3; page++ {
		data, err := s.get(ctx, fmt.Sprintf("%s?per_page=100&page=%d", s.releasesURL, page), 4<<20)
		if err != nil {
			return "", err
		}
		var releases []struct {
			Tag        string `json:"tag_name"`
			Prerelease bool   `json:"prerelease"`
			Draft      bool   `json:"draft"`
		}
		if err := json.Unmarshal(data, &releases); err != nil {
			return "", fmt.Errorf("read CRS release listing: %w", err)
		}
		for _, release := range releases {
			if !strings.HasPrefix(release.Tag, "v") || release.Prerelease || release.Draft {
				continue
			}
			version := strings.TrimPrefix(release.Tag, "v")
			if releaseInChannel(version, channel) && (latest == "" || compareReleases(version, latest) > 0) {
				latest = version
			}
		}
		if len(releases) < 100 {
			break
		}
	}
	if latest == "" {
		return "", fmt.Errorf("no published CRS release found for %s", channel)
	}
	return latest, nil
}

func (s *ruleSource) download(ctx context.Context, version string) (*ruleRelease, error) {
	if !validReleaseVersion(version) {
		return nil, fmt.Errorf("invalid CRS release version")
	}
	url := fmt.Sprintf("%s/v%s/coreruleset-%s-minimal.zip", s.downloadURL, version, version)
	archive, err := s.get(ctx, url, maxArchiveBytes)
	if err != nil {
		return nil, err
	}
	signature, err := s.get(ctx, url+".asc", maxSignatureBytes)
	if err != nil {
		return nil, err
	}
	return verifyRelease(version, archive, signature, s.keys)
}

func validReleaseVersion(version string) bool {
	if len(version) > 32 || !releaseVersion.MatchString(version) {
		return false
	}
	for _, part := range strings.Split(version, ".") {
		if _, err := strconv.ParseUint(part, 10, 32); err != nil || (len(part) > 1 && part[0] == '0') {
			return false
		}
	}
	return true
}

func releaseInChannel(version string, channel protocol.WAFSettings_Channel) bool {
	return validReleaseVersion(version) && (channel == protocol.WAFSettings_STABLE || (channel == protocol.WAFSettings_LTS && strings.HasPrefix(version, "4.25.")))
}

func compareReleases(a, b string) int {
	ap, bp := strings.Split(a, "."), strings.Split(b, ".")
	for i := range ap {
		av, _ := strconv.ParseUint(ap[i], 10, 32)
		bv, _ := strconv.ParseUint(bp[i], 10, 32)
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}
