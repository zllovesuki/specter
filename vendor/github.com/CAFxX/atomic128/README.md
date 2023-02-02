# atomic128

[![GoDoc](https://godoc.org/github.com/CAFxX/atomic128?status.svg)](https://godoc.org/github.com/CAFxX/atomic128)
[![Build Status](https://github.com/CAFxX/atomic128/actions/workflows/build.yml/badge.svg)](https://github.com/CAFxX/atomic128/actions/workflows/build.yml)
[![codecov](https://codecov.io/gh/CAFxX/atomic128/branch/master/graph/badge.svg?token=03A5UVYW3K)](https://codecov.io/gh/CAFxX/atomic128)
[![Go Report Card](https://goreportcard.com/badge/github.com/CAFxX/atomic128)](https://goreportcard.com/report/github.com/CAFxX/atomic128)

128-bit atomic operations for Golang, using [CMPXCHG16B](http://www.felixcloutier.com/x86/CMPXCHG8B:CMPXCHG16B.html)
when available. 

Partially based on [github.com/tmthrgd/atomic128](https://github.com/tmthrgd/atomic128), without the problematic parts.

## Performance

```
$ benchstat <(go test -bench=. -cpu=8 -benchtime=0.1s -count=5)
name              time/op
Load/native-8     28.1ns ± 2%
Load/fallback-8    106ns ± 3%
Store/native-8     106ns ± 2%
Store/fallback-8   109ns ± 5%
Swap/native-8      110ns ± 1%
Swap/fallback-8    127ns ± 2%
Add/native-8       129ns ± 2%
Add/fallback-8     142ns ± 2%
CAS/native-8      31.4ns ± 1%
CAS/fallback-8     135ns ± 2%

$ benchstat <(go test -bench=. -cpu=1 -benchtime=0.1s -count=5)
name            time/op
Load/native     16.2ns ± 3%
Load/fallback   17.0ns ± 2%
Store/native    16.1ns ± 2%
Store/fallback  19.9ns ± 2%
Swap/native     19.3ns ± 1%
Swap/fallback   23.5ns ± 3%
Add/native      20.3ns ± 2%
Add/fallback    31.6ns ± 2%
CAS/native      17.3ns ± 6%
CAS/fallback    25.3ns ± 2%
```

## TODO

- Add ARM/aarch64 assembly version
- Add shift/rotate operations
