package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wanstu/nginx-manager/internal/deploy"
	"github.com/wanstu/nginx-manager/internal/doctor"
	"github.com/wanstu/nginx-manager/internal/nginxmgr"
	"github.com/wanstu/nginx-manager/internal/privilege"
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
	case "privileged":
		if len(args) == 2 && args[1] == "apply" {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return privilege.RunPrivilegedApply(ctx, os.Stdin, os.Stdout)
		}
		if len(args) == 2 && args[1] == "renew-certificates" {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()
			return privilege.RunPrivilegedRenewCertificates(ctx, os.Stdout)
		}
		if len(args) >= 2 && args[1] == "sudoers" {
			fs := flag.NewFlagSet("privileged sudoers", flag.ContinueOnError)
			serviceUser := fs.String("service-user", "nginx-manager", "dedicated API service user")
			helper := fs.String("helper", "/usr/local/bin/nginx-manager", "absolute installed helper path")
			if err := fs.Parse(args[2:]); err != nil {
				return err
			}
			rule, err := privilege.SudoersRule(*serviceUser, *helper)
			if err != nil {
				return err
			}
			fmt.Print(rule)
			return nil
		}
		return errors.New("usage: nginx-manager privileged apply | privileged renew-certificates | privileged sudoers [--service-user USER] [--helper PATH]")
	case "service":
		if len(args) >= 2 && args[1] == "systemd" {
			fs := flag.NewFlagSet("service systemd", flag.ContinueOnError)
			serviceUser := fs.String("service-user", "nginx-manager", "dedicated API service user")
			binary := fs.String("binary", "/usr/local/bin/nginx-manager", "absolute installed binary path")
			listen := fs.String("listen", "127.0.0.1:8020", "loopback HTTP listen address")
			if err := fs.Parse(args[2:]); err != nil {
				return err
			}
			unit, err := deploy.SystemdUnit(deploy.SystemdOptions{
				ServiceUser: *serviceUser,
				BinaryPath:  *binary,
				Listen:      *listen,
			})
			if err != nil {
				return err
			}
			fmt.Print(unit)
			return nil
		}
		if len(args) >= 2 && args[1] == "renewal-service" {
			fs := flag.NewFlagSet("service renewal-service", flag.ContinueOnError)
			binary := fs.String("binary", "/usr/local/bin/nginx-manager", "absolute installed binary path")
			if err := fs.Parse(args[2:]); err != nil {
				return err
			}
			unit, err := deploy.RenewalServiceUnit(*binary)
			if err != nil {
				return err
			}
			fmt.Print(unit)
			return nil
		}
		if len(args) == 2 && args[1] == "renewal-timer" {
			fmt.Print(deploy.RenewalTimerUnit())
			return nil
		}
		return errors.New("usage: nginx-manager service systemd | service renewal-service | service renewal-timer")
	case "config":
		if len(args) == 2 && args[1] == "path" {
			path, err := server.ConfigPath()
			if err != nil {
				return err
			}
			fmt.Println(path)
			return nil
		}
		if len(args) == 2 && args[1] == "paths-file" {
			fmt.Println(nginxmgr.PathsConfigPath())
			return nil
		}
		if len(args) == 2 && args[1] == "paths-template" {
			data, err := json.MarshalIndent(nginxmgr.PathsConfigTemplate(), "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}
		return errors.New("usage: nginx-manager config path | config paths-file | config paths-template")
	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
		jsonOutput := fs.Bool("json", false, "output JSON report")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		report := doctor.Run(ctx, version)
		if *jsonOutput {
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(report); err != nil {
				return err
			}
		} else {
			fmt.Print(doctor.FormatText(report))
		}
		if !report.Healthy {
			return errors.New("doctor found blocking errors")
		}
		return nil
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
  nginx-manager privileged apply
  nginx-manager privileged renew-certificates
  nginx-manager privileged sudoers [--service-user nginx-manager] [--helper /usr/local/bin/nginx-manager]
  nginx-manager service systemd [--service-user nginx-manager] [--binary /usr/local/bin/nginx-manager] [--listen 127.0.0.1:8020]
  nginx-manager service renewal-service [--binary /usr/local/bin/nginx-manager]
  nginx-manager service renewal-timer
  nginx-manager config path
  nginx-manager config paths-file
  nginx-manager config paths-template
  nginx-manager doctor [--json]
  nginx-manager version

The API refuses to start until a management password is configured.
For remote management, terminate TLS in front of the CLI service.`)
}
