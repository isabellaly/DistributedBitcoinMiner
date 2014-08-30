p1
==

This repository contains the starter code for project 1 (15-440, Spring 2014). It also contains
the tests that we will use to grade your implementation, and two simple echo server/client
(`srunner` and `crunner`, respectively) programs that you might find useful for your own testing
purposes. These instructions assume you have set your `GOPATH` to point to the repository's
root `p1/` directory.

If at any point you have any trouble with building, installing, or testing your code, the article
titled [How to Write Go Code](http://golang.org/doc/code.html) is a great resource for understanding
how Go workspaces are built and organized. You might also find the documentation for the
[`go` command](http://golang.org/cmd/go/) to be helpful. As always, feel free to post your questions
on Piazza.

This project was designed for, and tested on AFS cluster machines, though you may choose to
write and build your code locally as well.

## Reading the LSP API Documentation

Before you begin the project, you should read and understand all of the starter code we provide.
To make this experience a little less traumatic (we know, it's a lot :P), 
fire up a web server and read the documentation in a browser by executing the following command:

```sh
godoc -http=:6060 &
```

Then, navigate to [localhost:6060/pkg/github.com/cmu440/lsp](http://localhost:6060/pkg/github.com/cmu440/lsp) in a browser.
Note that you can execute this command from anywhere in your system (assuming your `GOPATH`
is pointing to the project's root `p1/` directory).

## Testing your implementation using `srunner` and `crunner`

To make testing your server a bit easier we have provided two simple echo server/client
programs called `srunner` and `crunner`. If you look at the source code for the two programs,
you'll notice that they import the `github.com/cmu440/lsp` package (in other words, they compile
against the current state of your LSP implementation). We believe you will find these programs
useful in the early stages of development when your client and server implementations are
largely incomplete.

To compile, build, and run these programs, use the `go run` command from inside the directory
storing the file (assumes your `GOPATH` is pointing to the project's root `p1/` directory):

```bash
$ go run strunner.go
```

The `srunner` and `crunner` programs may be customized using command line flags. For more
information, specify the `-h` flag at the command line. For example,

```sh
$ go run srunner.go -h
Usage of bin/srunner:
  -elim=5: epoch limit
  -ems=2000: epoch duration (ms)
  -port=9999: port number
  -rdrop=0: network read drop percent
  -v=false: show srunner logs
  -wdrop=0: network write drop percent
  -wsize=1: window size
```

We have also provided pre-compiled executables for you to use called `srunner-sols` and `crunner-sols`. 
These binaries were compiled against our reference LSP implementation,
so you might find them useful in the early stages of the development process (for example, if you wanted to test your 
`Client` implementation but haven't finished implementing the `Server` yet, etc.). Two separate binaries
are provided for Linux and Mac OS X machines (Windows is not supported at this time). 

As an example, to start an echo server on port `6060` on an AFS cluster machine, execute the following command:

```sh
$GOPATH/bin/linux_amd64/srunner-sols -port=6060
```

## Running the official tests

As with project 0, we will be using Autolab to grade your submissions for this project. Tests
for part A will be released about a week before the deadline. We will run some&mdash;but not all&mdash;of
the tests with the race detector enabled.

To test your submission, we will execute the following command from inside the
`src/github.com/cmu440/p1` directory:

```sh
$ go test
```

We will also check your code for race conditions using Go's race detector by executing
the following command:

```sh
$ go test -race
```

To execute a single unit test, you can use the `-test.run` flag and specify a regular expression
identifying the name of the test to run. For example,

```sh
$ go test -race -test.run TestBasic1
```

## Using Go on AFS

For those students who wish to write their Go code on AFS (either in a cluster or remotely), you will
need to set the `GOROOT` environment variable as follows (this is required because Go is installed
in a custom location on AFS machines):

```bash
$ export GOROOT=/usr/local/lib/go
```
