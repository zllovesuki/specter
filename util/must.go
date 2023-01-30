package util

func Must[V any](value V, err error) V {
	if err != nil {
		panic(err)
	}
	return value
}
