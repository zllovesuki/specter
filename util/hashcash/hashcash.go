package hashcash

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
	"time"
)

// ErrParse is returned when fewer than 6 or more than 7 segments are split
var ErrParse = errors.New("could not split the hashcash parts")

// ErrInvalidTag is returned when the Hashcash version is unsupported
var ErrInvalidTag = errors.New("expected tag to be 'H'")

// ErrInvalidDifficulty is returned when the difficulty is outside of the acceptable range
var ErrInvalidDifficulty = errors.New("the number of bits of difficulty is too low or too high")

// ErrInvalidDate is returned when the date cannot be parsed as a positive int64
var ErrInvalidDate = errors.New("invalid date")

// ErrExpired is returned when the current time is past that of ExpiresAt
var ErrExpired = errors.New("expired hashcash")

// ErrInvalidSubject is returned when the subject is invalid or does not match that passed to Verify()
var ErrInvalidSubject = errors.New("the subject is invalid or rejected")

// ErrInvalidNonce is returned when the nonce
//var ErrInvalidNonce = errors.New("the nonce has been used or is invalid")

// ErrUnsupportedAlgorithm is returned when the given algorithm is not supported
var ErrUnsupportedAlgorithm = errors.New("the given algorithm is invalid or not supported")

// ErrInvalidSolution is returned when the given hashcash is not properly solved
var ErrInvalidSolution = errors.New("the given solution is not valid")

// MaxDifficulty is the upper bound for all Solve() operations
var MaxDifficulty = 26

// Sep is the separator character to use
var Sep = ":"

// no milliseconds
//var isoTS = "2006-01-02T15:04:05Z"

// Hashcash represents a parsed Hashcash string
type Hashcash struct {
	Tag        string    `json:"tag"`        // Always "H" for "HTTP"
	Difficulty int       `json:"difficulty"` // Number of "partial pre-image" (zero) bits in the hashed code
	ExpiresAt  time.Time `json:"exp"`        // The timestamp that the hashcash expires, as seconds since the Unix epoch
	Subject    string    `json:"sub"`        // Resource data string being transmitted, e.g., a domain or URL
	Nonce      string    `json:"nonce"`      // Unique string of random characters, encoded as url-safe base-64
	Alg        string    `json:"alg"`        // always SHA-256 for now
	Solution   string    `json:"solution"`   // Binary counter, encoded as url-safe base-64
}

// New returns a Hashcash with reasonable defaults
func New(h Hashcash) *Hashcash {
	h.Tag = "H"

	if 0 == h.Difficulty {
		// safe for WebCrypto
		h.Difficulty = 10
	}

	if h.ExpiresAt.IsZero() {
		h.ExpiresAt = time.Now().Add(5 * time.Minute)
	}
	h.ExpiresAt = h.ExpiresAt.UTC().Truncate(time.Second)

	if "" == h.Subject {
		h.Subject = "*"
	}

	if "" == h.Nonce {
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); nil != err {
			panic(err)
			return nil
		}
		h.Nonce = base64.RawURLEncoding.EncodeToString(nonce)
	}

	if "" == h.Alg {
		h.Alg = "SHA-256"
	}
	/*
		if "SHA-256" != h.Alg {
			// TODO error
		}
	*/

	return &h
}

// Parse will (obviously) parse the hashcash string, without verifying any
// of the parameters.
func Parse(hc string) (*Hashcash, error) {
	parts := strings.Split(hc, Sep)
	n := len(parts)
	if n < 6 || n > 7 {
		return nil, ErrParse
	}

	tag := parts[0]
	if "H" != tag {
		return nil, ErrInvalidTag
	}

	bits, err := strconv.Atoi(parts[1])
	if nil != err || bits < 0 {
		return nil, ErrInvalidDifficulty
	}

	// Allow empty ExpiresAt
	var exp time.Time
	if "" != parts[2] {
		expAt, err := strconv.ParseInt(parts[2], 10, 64)
		if nil != err || expAt < 0 {
			return nil, ErrInvalidDate
		}
		exp = time.Unix(int64(expAt), 0).UTC()
	}

	/*
		exp, err := time.ParseInLocation(isoTS, parts[2], time.UTC)
		if nil != err {
			return nil, ErrInvalidDate
		}
	*/

	sub := parts[3]

	nonce := parts[4]

	alg := parts[5]

	var solution string
	if n > 6 {
		solution = parts[6]
	}

	h := &Hashcash{
		Tag:        tag,
		Difficulty: bits,
		ExpiresAt:  exp.UTC().Truncate(time.Second),
		Subject:    sub,
		Nonce:      nonce,
		Alg:        alg,
		Solution:   solution,
	}

	return h, nil
}

// String will return the formatted Hashcash, omitting the solution if it has not be solved.
func (h *Hashcash) String() string {
	var solution string
	if "" != h.Solution {
		solution = Sep + h.Solution
	}

	var expAt string
	if !h.ExpiresAt.IsZero() {
		expAt = strconv.FormatInt(h.ExpiresAt.UTC().Truncate(time.Second).Unix(), 10)
	}
	return strings.Join(
		[]string{
			"H",
			strconv.Itoa(h.Difficulty),
			//h.ExpiresAt.UTC().Format(isoTS),
			expAt,
			h.Subject,
			h.Nonce,
			h.Alg,
		},
		Sep,
	) + solution
}

// Verify the Hashcash based on Difficulty, Algorithm, ExpiresAt, Subject and,
// of course, the Solution and hash.
func (h *Hashcash) Verify(subject string) error {
	if h.Difficulty < 0 {
		return ErrInvalidDifficulty
	}

	if "SHA-256" != h.Alg {
		return ErrUnsupportedAlgorithm
	}

	if !h.ExpiresAt.IsZero() && h.ExpiresAt.Sub(time.Now()) < 0 {
		return ErrExpired
	}

	if subject != h.Subject {
		return ErrInvalidSubject
	}

	bits := h.Difficulty
	hash := sha256.Sum256([]byte(h.String()))
	n := bits / 8 // 10 / 8 = 1
	m := bits % 8 // 10 % 8 = 2
	if m > 0 {
		n++ // 10 bits = 2 bytes
	}

	if !verifyBits(hash[:n], bits, n) {
		return ErrInvalidSolution
	}
	return nil
}

func verifyBits(hash []byte, bits, n int) bool {
	if 0 == bits {
		return true
	}

	for i := 0; i < n; i++ {
		if bits > 8 {
			bits -= 8
			if 0 != hash[i] {
				return false
			}
			continue
		}

		// (bits % 8) == bits
		pad := 8 - bits
		if 0 == hash[i]>>pad {
			return true
		}
	}

	return false
}

// Solve will search for a solution, returning an error if the difficulty is
// above the local or global MaxDifficulty, the Algorithm is unsupported.
func (h *Hashcash) Solve(maxDifficulty int) error {
	if "SHA-256" != h.Alg {
		return ErrUnsupportedAlgorithm
	}

	if h.Difficulty < 0 {
		return ErrInvalidDifficulty
	}

	if h.Difficulty > maxDifficulty || h.Difficulty > MaxDifficulty {
		return ErrInvalidDifficulty
	}

	if "" != h.Solution {
		if nil == h.Verify(h.Subject) {
			return nil
		}
		h.Solution = ""
	}

	hashcash := h.String()
	bits := h.Difficulty
	n := bits / 8 // 10 / 8 = 1
	m := bits % 8 // 10 % 8 = 2
	if m > 0 {
		n++ // 10 bits = 2 bytes
	}

	var solution uint32 = 0
	sb := make([]byte, 4)
	for {
		// Note: it's not actually important what method of change or encoding is used
		// but incrementing by 1 on an int32 is good enough, and makes for a small base64 encoding
		binary.LittleEndian.PutUint32(sb, solution)
		h.Solution = base64.RawURLEncoding.EncodeToString(sb)
		hash := sha256.Sum256([]byte(hashcash + Sep + h.Solution))
		if verifyBits(hash[:n], bits, n) {
			return nil
		}
		solution++
	}
}
