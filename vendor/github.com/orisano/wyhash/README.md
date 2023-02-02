# wyhash
A pure-Go wyhash implementation.

## How to use
```go
import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/orisano/wyhash"
)

func main() {
	seed := flag.Int("s", 0, "seed value")
	flag.Parse()

	h := wyhash.New(uint64(*seed))
	io.Copy(h, os.Stdin)
	fmt.Printf("%x\n", h.Sum64())
}
```

## Benchmark
```
$ go test -bench . crypto/sha1
goos: darwin
goarch: amd64
pkg: crypto/sha1
BenchmarkHash8Bytes-4     	10000000	       175 ns/op	  45.48 MB/s
BenchmarkHash320Bytes-4   	 2000000	       700 ns/op	 456.49 MB/s
BenchmarkHash1K-4         	 1000000	      1377 ns/op	 743.30 MB/s
BenchmarkHash8K-4         	  200000	      9047 ns/op	 905.41 MB/s
PASS
ok  	crypto/sha1	7.375s
```

```
$ go test -bench . github.com/orisano/wyhash
goos: darwin
goarch: amd64
pkg: github.com/orisano/wyhash
BenchmarkHash8Bytes-4     	50000000	        24.1 ns/op	 332.51 MB/s
BenchmarkHash320Bytes-4   	20000000	        78.7 ns/op	4066.50 MB/s
BenchmarkHash1K-4         	10000000	       223 ns/op	4582.73 MB/s
BenchmarkHash8K-4         	 1000000	      1633 ns/op	5015.29 MB/s
PASS
ok  	github.com/orisano/wyhash	7.017s
```

## Author
Nao YONASHIRO (@orisano)

## License
MIT

## Reference
* [wangyi-fudan/wyhash](https://github.com/wangyi-fudan/wyhash)
