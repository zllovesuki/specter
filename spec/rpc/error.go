package rpc

import (
	"fmt"

	"kon.nect.sh/specter/spec/chord"

	"github.com/twitchtv/twirp"
)

func GetErrorMeta(err twirp.Error) (cause, kv string) {
	cause = err.Meta("cause")
	kv = err.Meta("kv")
	return
}

func WrapError(err error) error {
	var code twirp.ErrorCode
	if chord.ErrorIsRetryable(err) {
		code = twirp.FailedPrecondition
	} else {
		code = twirp.Internal
	}
	twerr := twirp.NewError(code, err.Error())
	twerr = twerr.WithMeta("cause", fmt.Sprintf("%T", err)) // to easily tell apart wrapped internal errors from explicit ones
	return twirp.WrapError(twerr, err)
}

func WrapErrorKV(key string, err error) error {
	var code twirp.ErrorCode
	if chord.ErrorIsRetryable(err) {
		code = twirp.FailedPrecondition
	} else {
		code = twirp.Internal
	}
	twerr := twirp.NewError(code, err.Error())
	twerr = twerr.WithMeta("cause", fmt.Sprintf("%T", err)) // to easily tell apart wrapped internal errors from explicit ones
	twerr = twerr.WithMeta("kv", key)
	return twirp.WrapError(twerr, err)
}
