package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	version := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	if (*version) {
		fmt.Println(Version)
		return
	}

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s LISTEN-ON=CONNECT-TO ...\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  %s unix:/var/run/listen.sock=tcp:192.168.0.1:12345 tcp::443=tcp:10.0.2.2:8443", os.Args[0])
		os.Exit(1)
	}

	for _, arg := range os.Args[1:] {
		listen, connect, found := strings.Cut(arg, "=")
		if !found {
			log.Fatalf("Invalid address pair: %s", arg)
		}

		ln, la, found := strings.Cut(listen, ":")
		if !found {
			log.Fatalf("Invalid listen address: %s", listen)
		}

		cn, ca, found := strings.Cut(connect, ":")
		if !found {
			log.Fatalf("Invalid connect address: %s", connect)
		}

		mode := os.FileMode(0600)

		if strings.HasPrefix(ln, "unix") {
			var m string
			ln, m, _ = strings.Cut(ln, "@")
			if len(m) > 0 {
				mv, err := strconv.ParseInt(m, 8, 32)
				if err != nil {
					log.Fatalf("Invalid socket mode %s: %v", m, err)
				}
				mode = os.FileMode(mv) & 0777
			}

			if err := os.MkdirAll(filepath.Dir(la), 0755); err != nil {
				log.Fatal(err)
			}
		}

		l, err := net.Listen(ln, la)
		if err != nil {
			log.Fatal(err)
		}

		if ln == "unix" {
			defer os.Remove(la)
			os.Chmod(la, mode)
		}

		go proxy(l, cn, ca)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}

func proxy(l net.Listener, network, address string) {
	for {
		accepted, err := l.Accept()
		if err != nil {
			log.Printf("Failed to accept connection on %s: %v", l.Addr(), err)
			continue
		}

		go handle(accepted, network, address)
	}
}

func handle(accepted net.Conn, network, address string) {
	defer accepted.Close()

	dialed, err := net.DialTimeout(network, address, 5 * time.Second)
	if err != nil {
		log.Printf("Failed to dial for connection accepted on %s: %v", accepted.LocalAddr(), err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer accepted.Close()
		defer dialed.Close()

		io.Copy(dialed, accepted)
		wg.Done()
	}()

	go func() {
		defer accepted.Close()
		defer dialed.Close()

		io.Copy(accepted, dialed)
		wg.Done()
	}()

	wg.Wait()
}

var (
	Version = "dev"
)
