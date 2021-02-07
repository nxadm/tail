![ci](https://github.com/nxadm/tail/workflows/ci/badge.svg)[![Go Reference](https://pkg.go.dev/badge/github.com/nxadm/tail.svg)](https://pkg.go.dev/github.com/nxadm/tail)

This project is an active, drop-in replacement for the
[abandoned](https://en.wikipedia.org/wiki/HPE_Helion) Go tail library at
[hpcloud](https://github.com/hpcloud/tail). Next to
[addressing open issues/PRs of the original project](https://github.com/nxadm/tail/issues/6),
nxadm/tail continues the development by keeping up to date with the Go toolchain
(e.g. go modules) and dependencies, completing the documentation, adding features
and fixing bugs.

Go 1.9 is the oldest compiler release supported.

# Go package for tail-ing files

A Go package striving to emulate the features of the BSD `tail` program. `nxadm/tail`
comes with full support for truncation/move detection as it is designed to work with
log rotation tools.

```Go
t, err := tail.TailFile("/var/log/nginx.log", tail.Config{Follow: true})
if err != nil {
    panic(err)
}

for line := range t.Lines {
    fmt.Println(line.Text)
}
```

See [API documentation](https://pkg.go.dev/github.com/nxadm/tail).

## Installing

    go get github.com/nxadm/tail/...
