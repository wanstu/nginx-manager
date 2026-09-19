package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/wanstu/nginx-manager/internal/server"
	"golang.org/x/term"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "nginx-manager:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}
	switch args[0] {
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		listen := fs.String("listen", "127.0.0.1:8020", "HTTP listen address")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return server.Serve(ctx, *listen, version)
	case "auth":
		return runAuth(args[1:])
	case "config":
		if len(args) == 2 && args[1] == "path" {
			path, err := server.ConfigPath()
			if err != nil {
				return err
			}
			fmt.Println(path)
			return nil
		}
		return errors.New("usage: nginx-manager config path")
	case "version":
		fmt.Println(version)
		return nil
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runAuth(args []string) error {
	if len(args) != 1 || args[0] != "set-password" {
		return errors.New("usage: nginx-manager auth set-password")
	}
	fmt.Print("New management password: ")
	var password string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return err
		}
		password = string(raw)
	} else {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
			return err
		}
		password = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	}
	if err := server.SetPassword(password); err != nil {
		return err
	}
	path, _ := server.ConfigPath()
	fmt.Println("Password configured.")
	fmt.Println("Config:", path)
	return nil
}

func printUsage() {
	fmt.Println(`Nginx Manager

Usage:
  nginx-manager auth set-password
  nginx-manager serve [--listen 127.0.0.1:8020]
  nginx-manager config path
  nginx-manager version

The API refuses to start until a management password is configured.
For remote management, terminate TLS in front of the CLI service.`)
}
