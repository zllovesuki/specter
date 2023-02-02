# fastrand

`fastrand` is the fastest pseudo-random number generator in Go. Support most common APIs of `math/rand`.

This generator base on the Go runtime per-M structure, and the init-seed provided by the Go runtime, which means you can't add your seed, but these methods scale very well on multiple cores.

## Compare to math/rand

- **2 ~ 200x faster**
- Scales well on multiple cores
- **Not** provide a stable value stream (can't inject init-seed)
- Fix bugs in math/rand `Float64` and `Float32`  (since no need to preserve the value stream)

## Benchmark

Go version: go1.19 linux/amd64

CPU: AMD 3700x(8C16T), running at 3.6GHz

OS: ubuntu 18.04

MEMORY: 16G x 2 (3200MHz)


```
name                        old time/op  new time/op  delta
SingleCore/Uint32()-16      10.7ns ± 1%   2.3ns ± 1%  -78.58%  (p=0.008 n=5+5)
SingleCore/Uint64()-16      11.3ns ± 1%   2.3ns ± 0%  -79.59%  (p=0.008 n=5+5)
SingleCore/Int()-16         10.9ns ± 2%   3.6ns ± 0%  -66.89%  (p=0.016 n=5+4)
SingleCore/Intn(32)-16      12.0ns ± 2%   4.2ns ± 2%  -64.74%  (p=0.016 n=5+4)
SingleCore/Read/1024-16      937ns ± 2%   157ns ± 2%  -83.30%  (p=0.008 n=5+5)
SingleCore/Read/10240-16    9.29µs ± 2%  1.82µs ±60%  -80.42%  (p=0.008 n=5+5)
SingleCore/Perm/1024-16     23.9µs ± 1%   9.2µs ± 2%  -61.42%  (p=0.008 n=5+5)
SingleCore/Shuffle/1024-16  12.3µs ± 0%   5.4µs ± 1%  -56.34%  (p=0.008 n=5+5)
MultipleCore/Uint32()-16     146ns ±11%     0ns ± 2%  -99.87%  (p=0.008 n=5+5)
MultipleCore/Uint64()-16     151ns ± 4%     0ns ± 1%  -99.86%  (p=0.008 n=5+5)
MultipleCore/Int()-16        148ns ± 4%     0ns ± 1%  -99.79%  (p=0.008 n=5+5)
MultipleCore/Intn(32)-16     154ns ± 8%     0ns ± 1%  -99.71%  (p=0.008 n=5+5)
MultipleCore/Read/1024-16   1.64µs ± 6%  0.10µs ± 1%  -94.04%  (p=0.008 n=5+5)
MultipleCore/Read/10240-16  15.5µs ± 5%   0.6µs ± 1%  -96.20%  (p=0.008 n=5+5)
MultipleCore/Perm/1024-16    233µs ± 6%     2µs ± 6%  -99.01%  (p=0.008 n=5+5)
```
