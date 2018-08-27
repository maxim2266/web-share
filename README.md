## web-share: Minimalist HTTP file server

The program starts a basic HTTP file server, serving files from the directory of invocation and all its sub-directories. Useful for quick sharing of files via HTTP.

**Note**: This is a really, really simple program that initially started from
[this](https://golang.org/pkg/net/http/#example_FileServer) code example, and later evolved
into something occasionally useful. It does _not_ represent any major achievement in the world of
software development. I am just leaving the code here for reference.

### Compilation
Assuming that Go is already installed and configured, from the directory of the project,
first install the project dependency:
```sh
go get github.com/juju/gnuflag
go get github.com/maxim2266/mvr
```
Then compile the program:
```sh
go build -o web-share web-share.go
```
or, if debugging information in the binary is not required:
```sh
go build -o web-share -ldflags="-s -w" web-share.go
```
Finally, copy the resulting binary `web-share` to any location listed on your `PATH`
environment variable.

### Usage
The most basic usage is just to `cd` to a directory and type `web-share -i lo`. This will start
an HTTP file server, listening on `127.0.0.1:8080`. Directing the browser to
`http://127.0.0.1:8080` will list all files in the directory. On Linux all the available network
interfaces can be found using `ip address` command.

Command line options:
```sh
$ web-share --help
Usage of web-share:
-d, --directory (= ".")
    Root directory to serve files from.
-i, --interface (= "")
    Network interface to run the server on.
-p, --port  (= 8080)
    Network port number to listen on.
```

###### Tested on Linux Mint 18.3 using Go v1.10.3.
