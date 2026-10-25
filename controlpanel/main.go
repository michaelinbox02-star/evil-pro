package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const version = "1.0.0"

// configDirOverride is set from the -config flag.
var configDirOverride string

func banner(url, store string, auth bool, user string) {
	fmt.Println()
	fmt.Println("  evil-pro · control panel  v" + version)
	fmt.Println("  ---------------------------------------------------------")
	fmt.Printf("  panel   : %s\n", url)
	fmt.Printf("  store   : %s\n", store)
	if auth {
		fmt.Printf("  login   : %s / (as configured)\n", user)
	} else {
		fmt.Println("  login   : DISABLED — set -user/-pass when hosting it")
	}
	fmt.Println("  ---------------------------------------------------------")
	fmt.Println("  Ctrl+C to stop.")
	fmt.Println()
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func main() {
	bind := flag.String("bind", "127.0.0.1:8080", "listen address (use 0.0.0.0:8080 when hosting)")
	user := flag.String("user", "admin", "panel username")
	pass := flag.String("pass", "", "panel password (empty = no auth)")
	open := flag.Bool("open", true, "open the panel in the default browser")
	tlsCert := flag.String("tls-cert", "", "optional TLS certificate file")
	tlsKey := flag.String("tls-key", "", "optional TLS key file")
	showVer := flag.Bool("v", false, "print version and exit")
	cfgDir := flag.String("config", "", "directory for profiles.json (default: user config dir)")
	flag.Parse()

	if *showVer {
		fmt.Println("evil-pro control panel v" + version)
		return
	}
	configDirOverride = *cfgDir

	log.SetFlags(0)

	st, err := NewStore()
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	app := NewApp(st, *user, *pass)

	host := *bind
	urlHost := host
	if strings.HasPrefix(host, "0.0.0.0:") {
		urlHost = "127.0.0.1:" + strings.TrimPrefix(host, "0.0.0.0:")
	} else if strings.HasPrefix(host, "[::]:") {
		urlHost = "127.0.0.1:" + strings.TrimPrefix(host, "[::]:")
	}
	scheme := "http"
	if *tlsCert != "" && *tlsKey != "" {
		scheme = "https"
	}
	url := scheme + "://" + urlHost
	banner(url, st.Path(), app.authEnabled(), *user)

	srv := &http.Server{
		Addr:              *bind,
		Handler:           app.buildMux(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ln, err := net.Listen("tcp", *bind)
	if err != nil {
		log.Fatalf("listen %s: %v", *bind, err)
	}

	if *open {
		go func() { time.Sleep(400 * time.Millisecond); openBrowser(url) }()
	}

	go func() {
		if *tlsCert != "" && *tlsKey != "" {
			if err := srv.ServeTLS(ln, *tlsCert, *tlsKey); err != nil && err != http.ErrServerClosed {
				log.Fatalf("serve tls: %v", err)
			}
		} else {
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Fatalf("serve: %v", err)
			}
		}
	}()

	// wait for Ctrl+C
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	fmt.Println("bye.")
}
