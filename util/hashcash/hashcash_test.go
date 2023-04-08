package hashcash

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

var never = time.Date(3006, time.January, 1, 15, 4, 5, 0, time.UTC)

func TestNew(t *testing.T) {
	h1 := New(Hashcash{})

	h2, err := Parse(h1.String())
	if nil != err {
		t.Fatal(err)
		return
	}

	if h1.Tag != h2.Tag {
		t.Fatal("wrong tag version")
		return
	}

	if h1.Difficulty != h2.Difficulty {
		t.Fatal("wrong difficulty")
		return
	}

	if h1.ExpiresAt != h2.ExpiresAt {
		t.Fatal("wrong expiresat")
		return
	}

	if h1.Subject != h2.Subject {
		t.Fatal("wrong subject")
		return
	}

	if h1.Nonce != h2.Nonce {
		t.Fatal("wrong nonce")
		return
	}

	if h1.Alg != h2.Alg {
		t.Fatal("wrong algorithm")
		return
	}

	if h1.Solution != h2.Solution {
		t.Fatal("wrong solution")
		return
	}
}

func TestExplicit(t *testing.T) {
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	h0 := Hashcash{
		Tag:        "H",
		Difficulty: 1000,
		ExpiresAt:  time.Now().UTC().Truncate(time.Second),
		Subject:    "example.com",
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
		Alg:        "FOOBAR",
		Solution:   "incorrect",
	}
	h1 := New(h0)

	h2, err := Parse(h1.String())
	if nil != err {
		t.Fatal(err)
		return
	}
	h3 := New(*h2)

	if h0.Tag != h3.Tag {
		t.Fatal("wrong tag version")
		return
	}

	if h0.Difficulty != h3.Difficulty {
		t.Fatal("wrong difficulty")
		return
	}

	if h0.ExpiresAt != h3.ExpiresAt {
		t.Fatal("wrong expiresat")
		return
	}

	if h0.Subject != h3.Subject {
		t.Fatal("wrong subject")
		return
	}

	if h0.Nonce != h3.Nonce {
		t.Fatal("wrong nonce")
		return
	}

	if h0.Alg != h3.Alg {
		t.Fatal("wrong algorithm")
		return
	}

	if h0.Solution != h3.Solution {
		t.Fatal("wrong solution")
		return
	}
}

func TestExpired(t *testing.T) {
	h := New(Hashcash{
		Alg:        "SHA-256",
		Difficulty: 10,
		ExpiresAt:  time.Now().Add(-5 * time.Second),
	})

	err := h.Verify(h.Subject)
	if nil == err {
		t.Error("verified expired token")
		return
	}
	if err != ErrExpired {
		t.Error("expired token error is not ErrExpired")
		return
	}
}

func TestNoExpiry(t *testing.T) {
	h := New(Hashcash{})
	h.ExpiresAt = time.Time{}

	if err := h.Solve(20); nil != err {
		t.Error(err)
		return
	}

	if err := h.Verify(h.Subject); nil != err {
		t.Error(err)
		return
	}
}

func TestSolve0(t *testing.T) {
	h := New(Hashcash{
		ExpiresAt: never,
		Nonce:     "DeadBeef",
	})
	h.Difficulty = 0

	if err := h.Solve(1); nil != err {
		t.Error(err)
		return
	}

	if err := h.Verify(h.Subject); nil != err {
		t.Error(err)
		return
	}
}

func TestSolve1(t *testing.T) {
	h := New(Hashcash{
		Difficulty: 1,
		ExpiresAt:  never,
		Nonce:      "DeadBeef",
	})

	if err := h.Solve(2); nil != err {
		t.Error(err)
		return
	}

	if err := h.Verify(h.Subject); nil != err {
		t.Error(err)
		return
	}
}

func TestSolve2(t *testing.T) {
	h := New(Hashcash{
		Difficulty: 2,
		ExpiresAt:  never,
		Nonce:      "DeadBeef",
	})

	if err := h.Solve(3); nil != err {
		t.Error(err)
		return
	}

	if err := h.Verify(h.Subject); nil != err {
		t.Error(err)
		return
	}
}

func TestSolve7(t *testing.T) {
	h := New(Hashcash{
		Difficulty: 7,
		ExpiresAt:  never,
		Nonce:      "DeadBeef",
	})

	if err := h.Solve(8); nil != err {
		t.Error(err)
		return
	}

	if err := h.Verify(h.Subject); nil != err {
		t.Error(err)
		return
	}
}

func TestSolve8(t *testing.T) {
	h := New(Hashcash{
		Difficulty: 8,
		ExpiresAt:  never,
		Nonce:      "DeadBeef",
	})

	if err := h.Solve(9); nil != err {
		t.Error(err)
		return
	}

	if err := h.Verify(h.Subject); nil != err {
		t.Error(err)
		return
	}
}

func TestSolve9(t *testing.T) {
	h := New(Hashcash{
		Difficulty: 9,
		ExpiresAt:  never,
		Nonce:      "DeadBeef",
	})

	if err := h.Solve(10); nil != err {
		t.Error(err)
		return
	}

	if err := h.Verify(h.Subject); nil != err {
		t.Error(err)
		return
	}
}

func TestSolve15(t *testing.T) {
	h := New(Hashcash{
		Difficulty: 15,
		ExpiresAt:  never,
		Nonce:      "DeadBeef",
	})

	if err := h.Solve(16); nil != err {
		t.Error(err)
		return
	}

	if err := h.Verify(h.Subject); nil != err {
		t.Error(err)
		return
	}
}

func TestSolve16(t *testing.T) {
	h := New(Hashcash{
		Difficulty: 16,
		ExpiresAt:  never,
		Nonce:      "DeadBeef",
	})

	if err := h.Solve(17); nil != err {
		t.Error(err)
		return
	}

	if err := h.Verify(h.Subject); nil != err {
		t.Error(err)
		return
	}
}

func TestSolve17(t *testing.T) {
	h := New(Hashcash{
		Difficulty: 17,
		ExpiresAt:  never,
		Nonce:      "DeadBeef",
	})

	if err := h.Solve(18); nil != err {
		t.Error(err)
		return
	}

	if err := h.Verify(h.Subject); nil != err {
		t.Error(err)
		return
	}

	if "H:17:32693036645:*:DeadBeef:SHA-256:R7sIAA" != h.String() {
		t.Errorf("unexpected hashcash string: %s, has the implementation changed?", h.String())
	}
}

func TestTooHard(t *testing.T) {
	h := New(Hashcash{
		Alg:        "SHA-256",
		Difficulty: 24,
		ExpiresAt:  never,
		Nonce:      "DeadBeef",
	})

	err := h.Solve(20)
	if nil == err {
		t.Error(errors.New("the challenge is too hard, should've quite"))
		return
	}
	if ErrInvalidDifficulty != err {
		t.Error(errors.New("incorrect error"))
		return
	}
}
