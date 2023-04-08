# hashcash

HTTP Hashcash implemented in Go.

Explanation at https://therootcompany.com/blog/http-hashcash/

Go API docs at https://pkg.go.dev/git.rootprojects.org/root/hashcash?tab=doc

# CLI Usage

Install:

```bash
go get git.rootprojects.org/root/hashcash/cmd/hashcash
```

Usage:

```txt
Usage:
	hashcash new [subject *] [expires in 5m] [difficulty 10]
	hashcash parse <hashcash>
	hashcash solve <hashcash>
	hashcash hash <hashcash>
	hashcash verify <hashcash> [subject *]
```

Example:

```bash
my_hc=$(hashcash new)
echo New: $my_hc
hashcash parse "$my_hc"
echo ""

my_hc=$(hashcash solve "$my_hc")
echo Solved: $my_hc
hashcash parse "$my_hc"
echo ""

hashcash verify "$my_hc"
```
